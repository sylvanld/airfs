// Package mount exposes a merged view at a real filesystem path, so that tools
// which are not written in Go can read it with ordinary file operations. See
// docs/specs/fuse-mount.md.
package mount

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	gofs "github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/sylvanld/airfs"
	"github.com/sylvanld/airfs/sources"
)

// FSName is the filesystem name set at mount time. Finding a mount means
// looking for it in the kernel's mount table against the target's kind
// directories: a second record of what is mounted would disagree with the
// kernel exactly when it matters, after a crash or a manual unmount.
const FSName = "airfs"

// fsType is how the kernel reports a mount made under FSName.
const fsType = "fuse." + FSName

// A Server holds every mount of one target. A target is either serving or not:
// the mounts are established together and released together, since a partially
// mounted target is a view that lies about what is available.
type Server struct {
	Target string

	mu      sync.Mutex
	servers map[airfs.Kind]*fuse.Server
	kinds   []airfs.Kind
}

// KindDir is where a kind is served under target.
func KindDir(target string, kind airfs.Kind) string {
	return filepath.Join(target, kind.String())
}

// Serve mounts every kind of cfg under target and returns once the view is
// ready. It does not block; call Wait to serve until unmounted.
//
// If any kind fails to mount, every kind already mounted for that target is
// released before returning, so a failed attempt leaves nothing behind.
func Serve(target string, cfg *sources.Config) (*Server, error) {
	if err := Preflight(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return nil, err
	}

	s := &Server{Target: target, servers: map[airfs.Kind]*fuse.Server{}}
	for _, kind := range airfs.Kinds {
		dir := KindDir(target, kind)
		if err := prepareMountpoint(dir); err != nil {
			s.releaseAll()
			return nil, err
		}
		server, err := mountKind(dir, cfg, kind)
		if err != nil {
			s.releaseAll()
			return nil, fmt.Errorf("mounting %s: %w", dir, err)
		}
		s.servers[kind] = server
		s.kinds = append(s.kinds, kind)
	}
	return s, nil
}

func mountKind(dir string, cfg *sources.Config, kind airfs.Kind) (*fuse.Server, error) {
	root := &rootNode{fsys: cfg.Merged(kind)}
	// Zero attribute, entry and negative timeouts: the kernel revalidates every
	// time, so an edit or a newly added entry appears without intervention.
	// These are small text files read occasionally, so throughput is irrelevant
	// and staleness is a bug that presents as "my edit did nothing".
	var fresh time.Duration
	opts := &gofs.Options{
		MountOptions: fuse.MountOptions{
			FsName:  FSName,
			Name:    FSName,
			Options: []string{"ro"},
		},
		AttrTimeout:     &fresh,
		EntryTimeout:    &fresh,
		NegativeTimeout: &fresh,
	}
	return gofs.Mount(dir, root, opts)
}

// prepareMountpoint creates a kind's directory if it is missing and refuses one
// that would hide something.
func prepareMountpoint(dir string) error {
	if mounted, stale, err := mountState(dir); err != nil {
		return err
	} else if mounted {
		what := "already served"
		if stale {
			what = "held by a stale mount; unmount it first"
		}
		return airfs.Precondition(fmt.Errorf("%s is %s", dir, what))
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		// Mounting over a populated directory hides its contents, and the
		// hidden files are a trap.
		return airfs.Precondition(fmt.Errorf("%s is not empty", dir))
	}
	return nil
}

// Kinds are the kinds this server has mounted.
func (s *Server) Kinds() []airfs.Kind { return append([]airfs.Kind(nil), s.kinds...) }

// Wait blocks until every mount has been released. The mount lives as long as
// the process serving it.
func (s *Server) Wait() {
	for _, kind := range s.Kinds() {
		s.mu.Lock()
		server := s.servers[kind]
		s.mu.Unlock()
		if server != nil {
			server.Wait()
		}
	}
}

// Unmount releases every mount this server holds.
func (s *Server) Unmount() error { return s.releaseAll() }

func (s *Server) releaseAll() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var errs []error
	for kind, server := range s.servers {
		if err := server.Unmount(); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", kind, err))
		}
		delete(s.servers, kind)
	}
	s.kinds = nil
	return errors.Join(errs...)
}

// A State is one kind's mountpoint as the kernel currently sees it.
//
// A mount whose serving process has died is Mounted and Stale: still listed by
// the kernel, but failing every access. That signature is what distinguishes it
// from a live mount and from an ordinary directory.
type State struct {
	Kind    airfs.Kind
	Dir     string
	Mounted bool
	Stale   bool
}

// Status reports every kind's mountpoint under target.
func Status(target string) ([]State, error) {
	states := make([]State, 0, len(airfs.Kinds))
	for _, kind := range airfs.Kinds {
		dir := KindDir(target, kind)
		mounted, stale, err := mountState(dir)
		if err != nil {
			return nil, err
		}
		states = append(states, State{Kind: kind, Dir: dir, Mounted: mounted, Stale: stale})
	}
	return states, nil
}

// Served reports whether every kind of target is mounted and healthy.
func Served(states []State) bool {
	for _, st := range states {
		if !st.Mounted || st.Stale {
			return false
		}
	}
	return len(states) > 0
}

// Unmount releases every mount under target, including recovering a stale
// mountpoint left by a serving process that died. It returns the kinds it
// released, and is safe to call when nothing is mounted.
func Unmount(target string) ([]airfs.Kind, error) {
	states, err := Status(target)
	if err != nil {
		return nil, err
	}
	var released []airfs.Kind
	var errs []error
	for _, st := range states {
		if !st.Mounted {
			continue
		}
		if err := fusermountUnmount(st.Dir); err != nil {
			errs = append(errs, err)
			continue
		}
		released = append(released, st.Kind)
	}
	return released, errors.Join(errs...)
}

// fusermountUnmount releases one mountpoint through the setuid helper, which is
// what an unprivileged process has to use. A stale mount can still be busy, so
// a lazy detach is the fallback rather than the default.
func fusermountUnmount(dir string) error {
	out, err := exec.Command("fusermount3", "-u", dir).CombinedOutput()
	if err == nil {
		return nil
	}
	lazy, lazyErr := exec.Command("fusermount3", "-uz", dir).CombinedOutput()
	if lazyErr == nil {
		return nil
	}
	return fmt.Errorf("unmounting %s: %s: %s", dir, strings.TrimSpace(string(out)), strings.TrimSpace(string(lazy)))
}

// mountState reads the kernel's mount table for dir, then probes it to tell a
// live mount from one whose serving process has died.
func mountState(dir string) (mounted, stale bool, err error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return false, false, err
	}
	mounted, err = inMountTable(abs)
	if err != nil || !mounted {
		return false, false, err
	}
	if _, err := os.ReadDir(abs); err != nil {
		// A severed FUSE connection is the stale-mount signature.
		if errors.Is(err, syscall.ENOTCONN) {
			return true, true, nil
		}
	}
	return true, false, nil
}

// inMountTable reports whether abs is a mountpoint served by airfs, by the
// filesystem name set at mount time.
func inMountTable(abs string) (bool, error) {
	file, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return false, err
	}
	defer file.Close()

	scan := bufio.NewScanner(file)
	for scan.Scan() {
		// Up to the " - " separator the fields are positional; the mountpoint
		// is the fifth. After it come the filesystem type and its source.
		line := scan.Text()
		sep := strings.Index(line, " - ")
		if sep < 0 {
			continue
		}
		fields := strings.Fields(line[:sep])
		if len(fields) < 5 || unescape(fields[4]) != abs {
			continue
		}
		rest := strings.Fields(line[sep+3:])
		if len(rest) >= 1 && rest[0] == fsType {
			return true, nil
		}
	}
	return false, scan.Err()
}

// unescape decodes the octal escapes mountinfo uses for space, tab, newline and
// backslash in a path.
func unescape(s string) string {
	r := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	return r.Replace(s)
}
