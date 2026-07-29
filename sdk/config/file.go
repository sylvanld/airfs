package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"gopkg.in/yaml.v3"

	"github.com/sylvanld/airfs/sdk"
)

// A File is the configuration document as written, opened for editing.
//
// It holds the parsed document rather than the model, because an edit must
// preserve what a person wrote: comments, key order, anchors and aliases
// survive it. What does not survive is the exact bytes around them — airfs
// owns the file's formatting, which is what makes a one-field change a
// one-field diff every time. See docs/specs/workspace-config.md.
type File struct {
	Path string

	doc      yaml.Node
	original []byte
}

// Read opens path for editing. A file that is not there yet is an empty
// document rather than an error: `airfs add` is what brings the first
// workspace, and the file holding it, into being.
func Read(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, airfs.Precondition(err)
	}
	f := &File{Path: path, original: data}
	if len(bytes.TrimSpace(data)) > 0 {
		if err := yaml.Unmarshal(data, &f.doc); err != nil {
			return nil, airfs.Precondition(fmt.Errorf("%s: %w", path, err))
		}
	}
	return f, nil
}

// A Declaration is a workspace as a command states it: paths as they were
// typed, so that `~` and `$VAR` survive into the file.
type Declaration struct {
	Name    string
	Target  string
	Sources []string
	Folders []string
	Enabled bool
}

// Declare writes the declaration, replacing an existing workspace of the same
// name whole.
//
// A replacement is never a merge: what the block said is gone. Someone who
// typed the sources they want is stating the whole list, and a flag that
// quietly appended to what was there would produce an order nobody wrote. The
// key node is kept, so a comment written above the block outlives the block.
func (f *File) Declare(d Declaration) error {
	if !nameRE.MatchString(d.Name) {
		return airfs.Precondition(fmt.Errorf(
			"workspace name %q must be non-empty and match [a-zA-Z0-9_-]+", d.Name))
	}
	if d.Target == "" {
		return airfs.Precondition(errors.New("a workspace needs a target"))
	}
	if len(d.Sources) == 0 {
		return airfs.Precondition(errors.New("a workspace needs at least one source"))
	}

	block := &yaml.Node{Kind: yaml.MappingNode}
	block.Content = append(block.Content, scalar("target"), scalar(d.Target))
	if len(d.Folders) > 0 {
		folders := &yaml.Node{Kind: yaml.SequenceNode, Style: yaml.FlowStyle}
		for _, name := range d.Folders {
			folders.Content = append(folders.Content, scalar(name))
		}
		block.Content = append(block.Content, scalar("folders"), folders)
	}
	sources := &yaml.Node{Kind: yaml.SequenceNode}
	for _, s := range d.Sources {
		sources.Content = append(sources.Content, scalar(s))
	}
	block.Content = append(block.Content, scalar("sources"), sources)
	if !d.Enabled {
		block.Content = append(block.Content,
			scalar("enabled"), &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "false"})
	}

	declarations := f.declarations()
	if i := indexOf(declarations, d.Name); i >= 0 {
		// The anchor names the workspace, not the fields being replaced, so it
		// is carried over: commands never author an anchor, and one a person
		// wrote keeps working. Losing it would leave every alias to it
		// dangling, which is a broken file rather than a replaced block.
		previous := declarations.Content[i+1]
		block.Anchor, block.LineComment = previous.Anchor, previous.LineComment
		declarations.Content[i+1] = block
		return nil
	}
	declarations.Content = append(declarations.Content, scalar(d.Name), block)
	return nil
}

// Remove deletes a workspace's declaration and returns the block it removed,
// rendered, so that an unintended removal is recoverable from the terminal's
// scrollback rather than only from git.
func (f *File) Remove(name string) ([]byte, error) {
	declarations := f.declarations()
	i := indexOf(declarations, name)
	if i < 0 {
		return nil, airfs.Precondition(fmt.Errorf("no workspace named %s in %s", name, f.Path))
	}
	block, err := render(&yaml.Node{
		Kind:    yaml.MappingNode,
		Content: []*yaml.Node{declarations.Content[i], declarations.Content[i+1]},
	})
	if err != nil {
		return nil, err
	}
	declarations.Content = slices.Delete(declarations.Content, i, i+2)
	return block, nil
}

// SetEnabled writes the workspace's enabled field.
//
// The field is written explicitly rather than removed when it matches the
// default, since a workspace can inherit the field through a merge key and an
// explicit value is the only one that certainly wins.
func (f *File) SetEnabled(name string, on bool) error {
	declarations := f.declarations()
	i := indexOf(declarations, name)
	if i < 0 {
		return airfs.Precondition(fmt.Errorf("no workspace named %s in %s", name, f.Path))
	}
	block := declarations.Content[i+1]
	if block.Kind != yaml.MappingNode {
		return airfs.Precondition(fmt.Errorf("workspace %s is not a map", name))
	}
	value := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: fmt.Sprint(on)}
	for j := 0; j+1 < len(block.Content); j += 2 {
		if block.Content[j].Value == "enabled" {
			block.Content[j+1] = value
			return nil
		}
	}
	block.Content = append(block.Content, scalar("enabled"), value)
	return nil
}

// declarations returns the workspaces mapping, creating the document and the
// key when the file is empty or does not have them yet.
func (f *File) declarations() *yaml.Node {
	if f.doc.Kind == 0 {
		f.doc = yaml.Node{
			Kind:    yaml.DocumentNode,
			Content: []*yaml.Node{{Kind: yaml.MappingNode}},
		}
	}
	root := documentRoot(&f.doc)
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "workspaces" {
			value := root.Content[i+1]
			if value.Kind != yaml.MappingNode {
				// An empty or null workspaces key is a map waiting to be
				// filled, not something to refuse to edit.
				*value = yaml.Node{Kind: yaml.MappingNode}
			}
			return value
		}
	}
	declarations := &yaml.Node{Kind: yaml.MappingNode}
	root.Content = append(root.Content, scalar("workspaces"), declarations)
	return declarations
}

func indexOf(declarations *yaml.Node, name string) int {
	for i := 0; i+1 < len(declarations.Content); i += 2 {
		if declarations.Content[i].Value == name {
			return i
		}
	}
	return -1
}

// Bytes renders the document in canonical form.
func (f *File) Bytes() ([]byte, error) { return render(&f.doc) }

// render encodes a node the way airfs writes YAML: two-space indentation, and
// a merge key written as the `<<` a person types rather than as the explicit
// tag the parser resolved it to.
func render(node *yaml.Node) ([]byte, error) {
	untagMerges(node)
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(node); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// untagMerges clears the !!merge tag the parser attaches to a `<<` key, which
// the encoder would otherwise write out as `!!merge <<:`. The parse and the
// re-encode have to agree on the same spelling, or every edit would rewrite
// every merge key in the file.
func untagMerges(node *yaml.Node) {
	if node == nil {
		return
	}
	if node.Kind == yaml.ScalarNode && node.Tag == "!!merge" && node.Value == "<<" {
		node.Tag = ""
	}
	for _, child := range node.Content {
		untagMerges(child)
	}
}

// Edit is the one operation behind every declarative command: read the file,
// apply the change, validate the result, write it. See docs/specs/cli.md.
//
// The result is validated before it is written, so a mistyped path leaves the
// existing configuration standing rather than replacing it with something that
// does not resolve. What comes back is every workspace whose *resolved*
// configuration differs from before — not only the one named, since a merge key
// can carry an edit to its aliases, and an alias must never change something
// silently.
func Edit(path string, mutate func(*File) error) (changed []string, err error) {
	f, err := Read(path)
	if err != nil {
		return nil, err
	}
	// A file that does not resolve yet still gets edited — that is how a broken
	// declaration is fixed. Nothing can be diffed against it, so everything the
	// result declares is reported as changed.
	before, _ := Parse(path, f.original)

	if err := mutate(f); err != nil {
		return nil, err
	}
	data, err := f.Bytes()
	if err != nil {
		return nil, err
	}
	after, err := Parse(path, data)
	if err != nil {
		return nil, err
	}
	if err := write(path, data); err != nil {
		return nil, err
	}
	return difference(before, after), nil
}

// difference names every workspace the edit changed the meaning of, including
// the ones it removed.
func difference(before, after *Config) []string {
	var changed []string
	seen := map[string]bool{}
	for _, w := range after.Workspaces {
		seen[w.Name] = true
		var was *Workspace
		if before != nil {
			was = before.Lookup(w.Name)
		}
		if !w.SameAs(was) || (was != nil && was.Enabled != w.Enabled) {
			changed = append(changed, w.Name)
		}
	}
	if before != nil {
		for _, w := range before.Workspaces {
			if !seen[w.Name] {
				changed = append(changed, w.Name)
			}
		}
	}
	return changed
}

// write puts data in place through a temporary file in the same directory, so
// that a reader never sees a half-written configuration and a failed write
// leaves the previous one standing.
func write(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, FileName+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// CreateTemp opens at 0600; the configuration is an ordinary readable file.
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// AsDeclared is how a path typed on the command line is written down. It is
// kept as typed, so that `~` and `$VAR` survive into the file and it keeps the
// form its author would recognise.
//
// A path still relative after expansion is the exception: on the command line
// it means "from the working directory", in the file it would mean "from the
// file's directory", so it is made absolute first. Written verbatim it would
// silently name a different directory.
func AsDeclared(typed string) (string, error) {
	expanded, err := Expand(typed, "")
	if err != nil {
		return "", airfs.Precondition(err)
	}
	if filepath.IsAbs(expanded) {
		return typed, nil
	}
	abs, err := filepath.Abs(typed)
	if err != nil {
		return "", err
	}
	return abs, nil
}
