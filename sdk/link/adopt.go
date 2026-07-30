package link

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
)

// attempts bounds the search for a free name. Reaching it means something is
// generating names faster than this can exhaust them, which is not a project
// layout worth grinding through.
const attempts = 100

// A claim is a name spoken for, and by whom. Attribution is kept because a
// report that says an entry was renamed without saying who took its name gives
// a reader nothing to act on.
type claim struct {
	path string
	by   string
}

// adopt moves every entry of a tool's directory into the root.
//
// Nothing is deleted, with the single exception of a byte-identical duplicate:
// every other entry is moved, so a run that did the wrong thing is undone with
// mv, and a run that fails part way leaves each entry either at its old path or
// its new one. That is what makes adoption safe enough to be the default rather
// than a flag.
func adopt(o Options, from, target string, tool Tool, claimed map[string]claim) ([]Move, error) {
	entries, err := os.ReadDir(from)
	if err != nil {
		return nil, err
	}
	var moves []Move
	for _, entry := range entries {
		move, err := place(o, filepath.Join(from, entry.Name()), target, entry.Name(), tool, claimed)
		if err != nil {
			return moves, err
		}
		moves = append(moves, move)
	}
	return moves, nil
}

// place finds the name an entry gets in the root, and puts it there.
//
// An entry keeps its name when it can. When it cannot, the first claim keeps
// the bare name and this one is suffixed with its tool — and an entry already
// in the root outranks every tool, whatever the flag order, because someone put
// it there deliberately under the name they chose. Nothing already in the root
// is moved, renamed or overwritten.
func place(o Options, source, target, name string, tool Tool, claimed map[string]claim) (Move, error) {
	// Who held the name this entry wanted, carried into the move it settles for
	// so that the report can say why it is not called what it was called.
	var contested string
	for attempt := 0; attempt < attempts; attempt++ {
		candidate := name
		switch {
		case attempt == 1:
			candidate = name + "-" + tool.Flag
		case attempt > 1:
			candidate = name + "-" + tool.Flag + "-" + strconv.Itoa(attempt)
		}
		destination := filepath.Join(target, candidate)

		// Whatever holds the name, where to read it from, and who it belongs to.
		// The path differs between the two kinds of run: on a real run an entry
		// claimed earlier is already at the destination, and on a dry run it is
		// still where it came from.
		holder, taken := claimed[candidate]
		if _, err := os.Lstat(destination); err == nil {
			// Anything on disk that this run did not put there was in the root
			// before, which outranks every tool.
			if !taken {
				holder.by = RootHeld
			}
			holder.path, taken = destination, true
		} else if !os.IsNotExist(err) {
			return Move{}, err
		}
		if !taken {
			claimed[candidate] = claim{path: source, by: tool.Flag}
			if o.DryRun {
				return Move{Name: name, As: candidate, Taken: contested}, nil
			}
			if err := os.Rename(source, destination); err != nil {
				return Move{}, err
			}
			return Move{Name: name, As: candidate, Taken: contested}, nil
		}

		// The same skill copied between two tools collides by name and is
		// byte-identical. Suffixing would produce two copies of one thing,
		// which loses the point of a single root, while dropping a copy that is
		// identical loses nothing at all.
		same, err := identical(source, holder.path)
		if err != nil {
			return Move{}, err
		}
		if same {
			if !o.DryRun {
				if err := os.RemoveAll(source); err != nil {
					return Move{}, err
				}
			}
			return Move{Name: name, As: candidate, Taken: holder.by, Deduped: true}, nil
		}
		contested = holder.by
	}
	return Move{}, fmt.Errorf("no free name for %s after %d attempts", name, attempts)
}

// identical reports whether two paths hold exactly the same bytes: the same
// kind of thing, the same names beneath it, and the same content.
//
// It is deliberately strict. It decides whether something may be dropped, so
// anything it cannot compare exactly — a socket, a device — is not identical.
func identical(a, b string) (bool, error) {
	ai, err := os.Lstat(a)
	if err != nil {
		return false, err
	}
	bi, err := os.Lstat(b)
	if err != nil {
		return false, err
	}
	if ai.Mode().Type() != bi.Mode().Type() {
		return false, nil
	}

	switch {
	case ai.IsDir():
		ae, err := os.ReadDir(a)
		if err != nil {
			return false, err
		}
		be, err := os.ReadDir(b)
		if err != nil {
			return false, err
		}
		if len(ae) != len(be) {
			return false, nil
		}
		for i := range ae {
			if ae[i].Name() != be[i].Name() {
				return false, nil
			}
			same, err := identical(filepath.Join(a, ae[i].Name()), filepath.Join(b, be[i].Name()))
			if err != nil || !same {
				return same, err
			}
		}
		return true, nil

	case ai.Mode().IsRegular():
		if ai.Size() != bi.Size() {
			return false, nil
		}
		return sameContent(a, b)

	case ai.Mode()&os.ModeSymlink != 0:
		at, err := os.Readlink(a)
		if err != nil {
			return false, err
		}
		bt, err := os.Readlink(b)
		if err != nil {
			return false, err
		}
		return at == bt, nil

	default:
		return false, nil
	}
}

func sameContent(a, b string) (bool, error) {
	af, err := os.Open(a)
	if err != nil {
		return false, err
	}
	defer af.Close()
	bf, err := os.Open(b)
	if err != nil {
		return false, err
	}
	defer bf.Close()

	ab := make([]byte, 64*1024)
	bb := make([]byte, 64*1024)
	for {
		an, aerr := io.ReadFull(af, ab)
		bn, berr := io.ReadFull(bf, bb)
		if an != bn || !bytes.Equal(ab[:an], bb[:bn]) {
			return false, nil
		}
		if aerr != nil || berr != nil {
			done := func(err error) bool { return err == io.EOF || err == io.ErrUnexpectedEOF }
			if done(aerr) && done(berr) {
				return true, nil
			}
			if aerr != nil && !done(aerr) {
				return false, aerr
			}
			return false, berr
		}
	}
}
