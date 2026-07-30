// Package link points the AI tools a project uses at the resources the project
// itself owns, by adopting what each tool already holds into one root and
// leaving a relative symlink where the tool looks. See docs/specs/link.md.
//
// It is the one part of airfs that knows anything about a specific tool, and it
// spends that knowledge here: nothing it knows survives into the configuration,
// the merge, or the mount layer. Everywhere else a folder name is a string a
// workspace declared, and `skills` means no more than `prompts` does.
//
// It is also the one part that is not declarative. A run is a one-shot mutation
// of a project directory, recorded nowhere and reconciled by nothing — which is
// what lets the symlink it produces keep working on a machine where airfs is
// not installed.
package link

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sylvanld/airfs/sdk"
)

// DefaultRoot is the directory a project keeps its own AI resources in, holding
// one subdirectory per resource type. It is also a well-formed airfs source, so
// a project that later wants its skills merged with an organisation's declares
// a workspace listing it and moves nothing.
const DefaultRoot = ".ai"

// A Tool is one row of the table: a tool's name as a flag, a resource type, and
// the path — relative to the project root — at which that tool expects that
// type.
//
// The table grows by evidence. A wrong row is the worst bug this package can
// have: the symlink is created, the report says it succeeded, and the tool goes
// on seeing nothing, because nothing looks at the path the row names. So a row
// is added against a tool's documented layout, never against a plausible guess.
type Tool struct {
	Flag string // as it is given on the command line, without dashes
	Type string // the subdirectory of the root it is linked to
	Path string // where the tool looks, relative to the project root
}

var table = []Tool{
	{Flag: "claude", Type: "skills", Path: filepath.Join(".claude", "skills")},
	{Flag: "opencode", Type: "skills", Path: filepath.Join(".opencode", "skills")},
}

// Tools returns every known tool, in table order.
func Tools() []Tool { return append([]Tool(nil), table...) }

// Lookup returns the tool a flag names.
func Lookup(flag string) (Tool, bool) {
	for _, t := range table {
		if t.Flag == flag {
			return t, true
		}
	}
	return Tool{}, false
}

// Options is one invocation: which tools, in which order, into which root.
type Options struct {
	// Project is the directory to operate on, which is the project root. It
	// must be absolute.
	Project string
	// Root holds the project's own resources, relative to Project. Empty means
	// DefaultRoot.
	Root string
	// Tools are linked in this order, which is the order their flags were
	// given, because that order decides who keeps a contested name.
	Tools []Tool
	// DryRun computes and reports the whole run without writing anything.
	DryRun bool
}

// What happened to one tool.
const (
	// Linked: the symlink was created, after any adoption.
	Linked = "linked"
	// Unchanged: the symlink was already there and already correct.
	Unchanged = "unchanged"
	// Refused: something was in the way, and the other tools were linked
	// anyway.
	Refused = "refused"
)

// An Outcome is what one named tool got. A refusal fails that tool alone: a
// project where one tool was scaffolded by hand still gets the rest.
type Outcome struct {
	Tool    Tool
	Action  string
	Adopted []Move
	Err     error
}

// A Move is one entry taken out of a tool's directory. Every one is reported by
// name rather than counted: adoption moves files nobody asked to have moved,
// and the only acceptable version of that is one whose report can be read
// against `git status` line for line.
type Move struct {
	Name string // as the tool named it
	As   string // as the root names it; equal to Name unless it was contested
	// Taken names who held the name this entry wanted: another tool's Flag, or
	// RootHeld. Empty when nothing did. A report that says an entry was renamed
	// without saying who took its name gives a reader nothing to act on.
	Taken string
	// Deduped: dropped instead of moved, being byte-identical to what already
	// held the name. The one thing a run discards, and only when discarding
	// loses nothing.
	Deduped bool
}

// RootHeld is Move.Taken when the name belonged to something already in the
// root. It outranks every tool, whatever the flag order: someone put it there
// deliberately, under the name they chose.
const RootHeld = "the root"

// Renamed reports whether the entry had to give up its name.
func (m Move) Renamed() bool { return !m.Deduped && m.As != m.Name }

// A Report is the whole run.
type Report struct {
	// Root is where resources are written from now on, relative to the project.
	Root     string
	Outcomes []Outcome
}

// Refused reports whether any tool was refused, which is what the frontend
// turns into an exit code.
func (r *Report) Refused() bool {
	for _, o := range r.Outcomes {
		if o.Err != nil {
			return true
		}
	}
	return false
}

// Run links every named tool, and reports what it did to each.
//
// The error it returns is for a failure of the run as a whole — an unusable
// root, a project that is not there. Anything that fails one tool is that
// tool's Err, and the run continues.
func Run(o Options) (*Report, error) {
	if !filepath.IsAbs(o.Project) {
		return nil, fmt.Errorf("project %s is not an absolute path", o.Project)
	}
	if len(o.Tools) == 0 {
		return nil, airfs.Precondition(errors.New("no tool named"))
	}
	root, err := resolveRoot(o.Root)
	if err != nil {
		return nil, err
	}
	if info, err := os.Stat(o.Project); err != nil {
		return nil, airfs.Precondition(fmt.Errorf("project %s: %w", o.Project, err))
	} else if !info.IsDir() {
		return nil, airfs.Precondition(fmt.Errorf("project %s is not a directory", o.Project))
	}

	report := &Report{Root: root}
	// Names claimed earlier in this run. On a real run they are also on disk,
	// but a dry run writes nothing and still has to answer who took what.
	claimed := map[string]claim{}
	for _, tool := range o.Tools {
		outcome := Outcome{Tool: tool}
		outcome.Adopted, outcome.Action, outcome.Err = link(o, root, tool, claimed)
		if outcome.Err != nil {
			outcome.Action = Refused
		}
		report.Outcomes = append(report.Outcomes, outcome)
	}
	return report, nil
}

// resolveRoot validates the root and returns it cleaned.
//
// The root must resolve inside the project: the symlinks are relative and meant
// to be committed, so a root outside it produces links that resolve to nothing
// on every other machine — and adoption would move the project's resources
// somewhere the project does not contain.
func resolveRoot(root string) (string, error) {
	if root == "" {
		return DefaultRoot, nil
	}
	if filepath.IsAbs(root) {
		return "", airfs.Precondition(fmt.Errorf(
			"root %s is absolute; it must be a directory inside the project, since the symlinks are relative and meant to be committed", root))
	}
	cleaned := filepath.Clean(root)
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", airfs.Precondition(fmt.Errorf(
			"root %s climbs out of the project; it must be a directory inside it", root))
	}
	return cleaned, nil
}

// link is the whole of one tool: adopt what it holds, then leave a symlink
// where it looks.
func link(o Options, root string, tool Tool, claimed map[string]claim) ([]Move, string, error) {
	toolPath := filepath.Join(o.Project, tool.Path)
	target := filepath.Join(o.Project, root, tool.Type)

	// A root that lands inside a tool's own directory, or the reverse, would
	// have the command adopt into a path it is about to replace with a symlink
	// to itself. It can only come from --root, and it fails that tool alone.
	if within(target, toolPath) || within(toolPath, target) {
		return nil, "", fmt.Errorf("root %s and %s are the same directory, or one contains the other",
			filepath.Join(root, tool.Type), tool.Path)
	}
	if !within(o.Project, toolPath) {
		return nil, "", fmt.Errorf("%s is outside the project", tool.Path)
	}

	existing, err := os.Lstat(toolPath)
	switch {
	case err != nil && !os.IsNotExist(err):
		return nil, "", err
	case err == nil && existing.Mode()&os.ModeSymlink != 0:
		// Already linked here is success, not a conflict: re-running after
		// adding a tool is the expected way to use this.
		points, err := resolves(toolPath, target)
		if err != nil {
			return nil, "", err
		}
		if points {
			return nil, Unchanged, nil
		}
		return nil, "", fmt.Errorf("%s is a symlink to something else; something established it deliberately", tool.Path)
	case err == nil && !existing.IsDir():
		return nil, "", fmt.Errorf("%s is not a directory; there is nothing to adopt and nothing safe to replace", tool.Path)
	}

	if !o.DryRun {
		if err := os.MkdirAll(target, 0o755); err != nil {
			return nil, "", err
		}
	}

	var moves []Move
	if err == nil {
		// The path exists and is a real directory: it holds resources someone
		// put there, and they are the project's — they were simply filed under
		// one tool.
		if moves, err = adopt(o, toolPath, target, tool, claimed); err != nil {
			return moves, "", err
		}
		if !o.DryRun {
			if err := os.Remove(toolPath); err != nil {
				return moves, "", fmt.Errorf("emptying %s: %w", tool.Path, err)
			}
		}
	}

	if o.DryRun {
		return moves, Linked, nil
	}
	if err := os.MkdirAll(filepath.Dir(toolPath), 0o755); err != nil {
		return moves, "", err
	}
	// Relative, because an absolute target would name a home directory that
	// exists on exactly one machine, and this symlink is meant to be committed.
	relative, err := filepath.Rel(filepath.Dir(toolPath), target)
	if err != nil {
		return moves, "", err
	}
	if err := os.Symlink(relative, toolPath); err != nil {
		return moves, "", err
	}
	return moves, Linked, nil
}

// resolves reports whether the symlink at path leads to target.
func resolves(path, target string) (bool, error) {
	read, err := os.Readlink(path)
	if err != nil {
		return false, err
	}
	if !filepath.IsAbs(read) {
		read = filepath.Join(filepath.Dir(path), read)
	}
	return filepath.Clean(read) == filepath.Clean(target), nil
}

// within reports whether path is base or sits under it.
func within(base, path string) bool {
	relative, err := filepath.Rel(base, path)
	if err != nil {
		return false
	}
	return relative == "." || !strings.HasPrefix(relative, "..")
}
