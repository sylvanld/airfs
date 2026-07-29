// Package config reads the one file that declares every workspace on the
// machine, and resolves it into the model everything else works from. See
// docs/specs/workspace-config.md.
//
// The file is authored by hand and edited by command, so this package both
// resolves it (Load) and rewrites it while preserving what a person wrote
// (Edit).
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/sylvanld/airfs/sdk"
	"github.com/sylvanld/airfs/sdk/layerfs"
)

// DefaultFolders are the subdirectories a workspace merges when it declares
// none. They are a default rather than a fixed set: airfs attaches no meaning
// to any of these names, and a workspace serving a tool that expects something
// else declares that instead.
var DefaultFolders = []string{"agents", "skills", "commands", "scripts"}

// nameRE is what a workspace name must match, so that it survives a command
// line and a log line without quoting.
var nameRE = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// A Path is one declared path: as the author wrote it, and as it resolves.
//
// Declared is what reports name, so that a person recognises what they typed;
// Resolved is what is read. A symlinked path is followed on access and still
// reported as declared.
type Path struct {
	Declared string
	Resolved string
}

func (p Path) String() string { return p.Declared }

// A Workspace is one resolved declaration: what it is assembled from, where it
// is exposed, and which subfolders it carries.
type Workspace struct {
	Name    string
	Target  Path
	Folders []string
	Sources []Path
	Enabled bool
}

// A Config is a whole resolved configuration file, with its workspaces in the
// order the file declares them.
type Config struct {
	// Path is the file this was read from.
	Path       string
	Workspaces []*Workspace
}

// Lookup returns the named workspace, or nil.
func (c *Config) Lookup(name string) *Workspace {
	for _, w := range c.Workspaces {
		if w.Name == name {
			return w
		}
	}
	return nil
}

// Names lists every declared workspace, in file order.
func (c *Config) Names() []string {
	names := make([]string, 0, len(c.Workspaces))
	for _, w := range c.Workspaces {
		names = append(names, w.Name)
	}
	return names
}

// Load reads and resolves the configuration at path.
//
// It resolves every path, validates the whole file, and enumerates nothing: a
// source that does not exist fails the workspace declaring it when that
// workspace is established, not the file. Nothing is written, to a source or
// anywhere else.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, airfs.Precondition(fmt.Errorf(
				"no configuration at %s; declare a workspace with: airfs add <name> --target <dir> --source <dir>", path))
		}
		return nil, airfs.Precondition(err)
	}
	return Parse(path, data)
}

// Parse resolves data as the configuration that would be read from path. The
// path is what relative declarations resolve against and what errors name; the
// file itself is not read. It exists so that a configuration can be validated
// before it is put in place.
func Parse(path string, data []byte) (*Config, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, airfs.Precondition(fmt.Errorf("%s: %w", path, err))
	}
	cfg, err := resolve(path, &doc)
	if err != nil {
		return nil, airfs.Precondition(err)
	}
	return cfg, nil
}

// resolve turns a parsed document into the model, reporting every problem it
// finds rather than the first: a person fixing one typo should not have to run
// the command again to discover the next.
func resolve(path string, doc *yaml.Node) (*Config, error) {
	cfg := &Config{Path: path}
	root := documentRoot(doc)
	if root == nil {
		// An empty file declares no workspaces, which is what a fresh
		// installation has and is not an error.
		return cfg, nil
	}
	at := func(n *yaml.Node, format string, args ...any) error {
		return fmt.Errorf("%s:%d: %s", path, n.Line, fmt.Sprintf(format, args...))
	}
	if root.Kind != yaml.MappingNode {
		return nil, at(root, "the configuration must be a map with a workspaces key")
	}

	var problems []error
	var declarations *yaml.Node
	for i := 0; i+1 < len(root.Content); i += 2 {
		key, value := root.Content[i], root.Content[i+1]
		if key.Value != "workspaces" {
			problems = append(problems, at(key, "unknown top-level key %q; only workspaces is allowed", key.Value))
			continue
		}
		declarations = value
	}
	if declarations == nil || declarations.Tag == "!!null" {
		return cfg, errors.Join(problems...)
	}
	if declarations.Kind != yaml.MappingNode {
		problems = append(problems, at(declarations, "workspaces must be a map of name to workspace"))
		return cfg, errors.Join(problems...)
	}

	base := filepath.Dir(path)
	for i := 0; i+1 < len(declarations.Content); i += 2 {
		key, value := declarations.Content[i], declarations.Content[i+1]
		if !nameRE.MatchString(key.Value) {
			problems = append(problems, at(key, "workspace name %q must be non-empty and match [a-zA-Z0-9_-]+", key.Value))
			continue
		}
		w, errs := resolveWorkspace(key.Value, value, base, at)
		problems = append(problems, errs...)
		if w != nil {
			cfg.Workspaces = append(cfg.Workspaces, w)
		}
	}

	problems = append(problems, checkTargets(cfg, declarations, at)...)
	return cfg, errors.Join(problems...)
}

// errorAt formats a problem against the line of the node that caused it.
type errorAt func(n *yaml.Node, format string, args ...any) error

// resolveWorkspace reads one declaration. Its fields are read through a map so
// that a merge key is resolved first: the keys a workspace has are the ones it
// ends up with, not the ones written under its own name.
func resolveWorkspace(name string, node *yaml.Node, base string, at errorAt) (*Workspace, []error) {
	if node.Kind != yaml.MappingNode {
		return nil, []error{at(node, "workspace %s must be a map", name)}
	}
	var fields map[string]yaml.Node
	if err := node.Decode(&fields); err != nil {
		return nil, []error{at(node, "workspace %s: %v", name, err)}
	}

	var problems []error
	w := &Workspace{Name: name, Enabled: true}
	for key := range fields {
		switch key {
		case "target", "sources", "folders", "enabled":
		default:
			f := fields[key]
			problems = append(problems, at(&f, "workspace %s: unknown key %q", name, key))
		}
	}

	if field, ok := fields["target"]; ok {
		p, err := resolvePath(&field, base, at, "workspace %s: target", name)
		if err != nil {
			problems = append(problems, err)
		} else {
			w.Target = p
		}
	} else {
		problems = append(problems, at(node, "workspace %s: no target", name))
	}

	if field, ok := fields["sources"]; ok {
		var declared []string
		if err := field.Decode(&declared); err != nil {
			problems = append(problems, at(&field, "workspace %s: sources must be a list of paths", name))
		} else if len(declared) == 0 {
			problems = append(problems, at(&field, "workspace %s: no sources", name))
		}
		seen := make(map[string]string, len(declared))
		for _, d := range declared {
			p, err := resolvePath(scalar(d), base, at, "workspace %s: source %s", name, d)
			if err != nil {
				problems = append(problems, at(&field, "%v", err))
				continue
			}
			// A duplicate means the author believes the file says something it
			// does not, so it is an error rather than a silent collapse.
			if first, dup := seen[p.Resolved]; dup {
				problems = append(problems, at(&field,
					"workspace %s: source %s duplicates %s; both resolve to %s", name, d, first, p.Resolved))
				continue
			}
			seen[p.Resolved] = d
			w.Sources = append(w.Sources, p)
		}
	} else {
		problems = append(problems, at(node, "workspace %s: no sources", name))
	}

	if field, ok := fields["folders"]; ok {
		if err := field.Decode(&w.Folders); err != nil {
			problems = append(problems, at(&field, "workspace %s: folders must be a list of names", name))
		} else if len(w.Folders) == 0 {
			problems = append(problems, at(&field, "workspace %s: folders is empty; omit it for the default set", name))
		}
		for _, folder := range w.Folders {
			// A folder is a name shared by the sources and the target, not a
			// path into either.
			if folder == "" || folder != filepath.Clean(folder) || strings.ContainsRune(folder, filepath.Separator) ||
				folder == "." || folder == ".." {
				problems = append(problems, at(&field,
					"workspace %s: folder %q must be a single path element", name, folder))
			}
		}
	} else {
		w.Folders = append([]string(nil), DefaultFolders...)
	}

	if field, ok := fields["enabled"]; ok {
		if err := field.Decode(&w.Enabled); err != nil {
			problems = append(problems, at(&field, "workspace %s: enabled must be true or false", name))
		}
	}

	return w, problems
}

// resolvePath expands one declared path and confirms the invariant the
// expansion rules already guarantee: everything the model holds is absolute.
func resolvePath(node *yaml.Node, base string, at errorAt, what string, args ...any) (Path, error) {
	var declared string
	if err := node.Decode(&declared); err != nil || declared == "" {
		return Path{}, at(node, "%s must be a non-empty path", fmt.Sprintf(what, args...))
	}
	resolved, err := Expand(declared, base)
	if err != nil {
		return Path{}, at(node, "%s: %v", fmt.Sprintf(what, args...), err)
	}
	if !filepath.IsAbs(resolved) {
		return Path{}, at(node, "%s: %s does not resolve to an absolute path", fmt.Sprintf(what, args...), declared)
	}
	return Path{Declared: declared, Resolved: resolved}, nil
}

// checkTargets rejects two workspaces writing mountpoints into one tree. A
// shared target has two workspaces mounting over each other; a target nested in
// another has the inner one mounting inside a directory that is itself being
// mounted over.
func checkTargets(cfg *Config, node *yaml.Node, at errorAt) []error {
	var problems []error
	for i, w := range cfg.Workspaces {
		if w.Target.Resolved == "" {
			continue
		}
		for _, other := range cfg.Workspaces[:i] {
			if other.Target.Resolved == "" {
				continue
			}
			switch {
			case other.Target.Resolved == w.Target.Resolved:
				problems = append(problems, at(node,
					"workspaces %s and %s share the target %s", other.Name, w.Name, w.Target.Resolved))
			case within(w.Target.Resolved, other.Target.Resolved):
				problems = append(problems, at(node,
					"workspace %s's target %s is inside workspace %s's target %s",
					w.Name, w.Target.Resolved, other.Name, other.Target.Resolved))
			case within(other.Target.Resolved, w.Target.Resolved):
				problems = append(problems, at(node,
					"workspace %s's target %s is inside workspace %s's target %s",
					other.Name, other.Target.Resolved, w.Name, w.Target.Resolved))
			}
		}
	}
	return problems
}

// within reports whether path lies under dir, comparing whole path elements so
// that /a/bc is not inside /a/b.
func within(path, dir string) bool {
	return strings.HasPrefix(path, strings.TrimSuffix(dir, string(filepath.Separator))+string(filepath.Separator))
}

func scalar(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}

// documentRoot unwraps the document node an empty file does not have.
func documentRoot(doc *yaml.Node) *yaml.Node {
	if doc.Kind == yaml.DocumentNode {
		if len(doc.Content) == 0 {
			return nil
		}
		return doc.Content[0]
	}
	if doc.Kind == 0 {
		return nil
	}
	return doc
}

// FolderDir is where a folder is served under a workspace's target.
func (w *Workspace) FolderDir(folder string) string {
	return filepath.Join(w.Target.Resolved, folder)
}

// Layers builds the ordered layers one folder's merged view is composed of:
// one stratum per source, in declared order, named as the author declared it.
//
// A source that lacks the folder still contributes a layer, which reads as
// empty. Nothing is created to make it exist: airfs performs no write against
// a source.
func (w *Workspace) Layers(folder string) []layerfs.Layer {
	layers := make([]layerfs.Layer, 0, len(w.Sources))
	for _, s := range w.Sources {
		root := filepath.Join(s.Resolved, folder)
		layers = append(layers, layerfs.Layer{
			Name: s.Declared,
			FS:   os.DirFS(root),
			Root: root,
		})
	}
	return layers
}

// Merged is the union for one folder, over every source's stratum.
func (w *Workspace) Merged(folder string) *layerfs.FS {
	return layerfs.New(w.Layers(folder)...)
}

// Counts reports how many entries each source contributes to folder, indexed
// the same way as Sources. A source without the folder contributes none.
//
// Counting enumerates the strata, which is the merge's work rather than
// resolution's, so it is done here on demand by whoever reports.
func (w *Workspace) Counts(folder string) ([]int, error) {
	counts := make([]int, len(w.Sources))
	for i, layer := range w.Layers(folder) {
		entries, err := fs.ReadDir(layer.FS, ".")
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		counts[i] = len(entries)
	}
	return counts, nil
}

// Readable reports the first declared source that is not a readable
// directory. A workspace cannot be established over one, and the reason names
// the source as its author wrote it.
func (w *Workspace) Readable() error {
	for _, s := range w.Sources {
		info, err := os.Stat(s.Resolved)
		if err != nil {
			return airfs.Precondition(fmt.Errorf("source %s: %w", s.Declared, err))
		}
		if !info.IsDir() {
			return airfs.Precondition(fmt.Errorf("source %s is not a directory", s.Declared))
		}
	}
	return nil
}

// SameAs reports whether two resolved workspaces would be served identically.
// It is what reconciliation asks to decide whether a mounted workspace can be
// left alone: a union is immutable once built, so anything this compares
// unequal means releasing and establishing again.
func (w *Workspace) SameAs(other *Workspace) bool {
	if other == nil || w.Name != other.Name || w.Target.Resolved != other.Target.Resolved {
		return false
	}
	if len(w.Folders) != len(other.Folders) || len(w.Sources) != len(other.Sources) {
		return false
	}
	for i := range w.Folders {
		if w.Folders[i] != other.Folders[i] {
			return false
		}
	}
	for i := range w.Sources {
		if w.Sources[i].Resolved != other.Sources[i].Resolved {
			return false
		}
	}
	return true
}
