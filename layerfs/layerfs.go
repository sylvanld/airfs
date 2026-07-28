// Package layerfs is the merged view: an ordered, read-only union of
// filesystem trees. See docs/specs/layered-fs.md.
package layerfs

import (
	"io"
	"io/fs"
	"sort"
	"strings"
	"time"
)

// A Layer is one tree contributing to the union.
//
// Root is the real directory backing FS, when there is one. It is empty for a
// layer that is not disk-backed, such as an in-memory tree in a test. Frontends
// use it to read through to the backing file; the union itself never does.
type Layer struct {
	Name string
	FS   fs.FS
	Root string
}

// FS is the ordered union of its layers. Later layers win.
//
// An FS is immutable once constructed and safe for concurrent use. It holds no
// content cache and no directory cache, so an edit within a layer is visible
// through the union immediately. Changing the set of layers means constructing
// a new FS.
type FS struct {
	layers []Layer
}

// New builds the union of layers, in declared order: the last layer wins.
func New(layers ...Layer) *FS {
	return &FS{layers: append([]Layer(nil), layers...)}
}

// Layers returns the union's layers in declared order.
func (f *FS) Layers() []Layer { return append([]Layer(nil), f.layers...) }

var (
	_ fs.FS        = (*FS)(nil)
	_ fs.ReadDirFS = (*FS)(nil)
	_ fs.StatFS    = (*FS)(nil)
)

// index maps every top-level name to the layer that wins it.
//
// Lookup and listing are both derived from this one map, which is what makes a
// listed name and a looked-up name resolve to the same layer by construction.
// It is rebuilt per operation: the union caches nothing, and a kind directory
// holds few entries.
func (f *FS) index() (map[string]winner, error) {
	owner := make(map[string]winner)
	for i, l := range f.layers {
		entries, err := fs.ReadDir(l.FS, ".")
		if err != nil {
			return nil, &fs.PathError{Op: "readdir", Path: l.Name, Err: err}
		}
		for _, e := range entries {
			owner[e.Name()] = winner{layer: i, entry: e}
		}
	}
	return owner, nil
}

// winner is the layer that serves a top-level name, and that layer's directory
// entry for it, so that listing the root needs no second pass.
type winner struct {
	layer int
	entry fs.DirEntry
}

// split separates a slash path into its top-level entry name and the rest.
func split(name string) (top, rest string) {
	if i := strings.IndexByte(name, '/'); i >= 0 {
		return name[:i], name[i+1:]
	}
	return name, "."
}

// Origin reports which layer serves name, which must be a top-level entry.
// Frontends use it to read through to a backing file and to name a winner.
func (f *FS) Origin(name string) (Layer, bool) {
	owner, err := f.index()
	if err != nil {
		return Layer{}, false
	}
	w, ok := owner[name]
	if !ok {
		return Layer{}, false
	}
	return f.layers[w.layer], true
}

// Open resolves name against the union. The root merges; every entry below it
// comes wholly from the one layer that wins its top-level name.
func (f *FS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	if name == "." {
		entries, err := f.ReadDir(".")
		if err != nil {
			return nil, err
		}
		return &rootDir{entries: entries}, nil
	}
	top, _ := split(name)
	owner, err := f.index()
	if err != nil {
		return nil, err
	}
	w, ok := owner[top]
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	file, err := f.layers[w.layer].FS.Open(name)
	if err != nil {
		return nil, err
	}
	return &roFile{File: file}, nil
}

// Stat resolves name's metadata from the winning layer, with write bits
// cleared.
func (f *FS) Stat(name string) (fs.FileInfo, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrInvalid}
	}
	if name == "." {
		return rootInfo{}, nil
	}
	top, _ := split(name)
	owner, err := f.index()
	if err != nil {
		return nil, err
	}
	w, ok := owner[top]
	if !ok {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
	}
	info, err := fs.Stat(f.layers[w.layer].FS, name)
	if err != nil {
		return nil, err
	}
	return roInfo{info}, nil
}

// ReadDir lists name. The root is the merge of every layer's root,
// deduplicated with the last occurrence winning; any deeper directory comes
// wholly from the layer that won its top-level entry. Order is lexical.
func (f *FS) ReadDir(name string) ([]fs.DirEntry, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrInvalid}
	}
	if name != "." {
		top, _ := split(name)
		owner, err := f.index()
		if err != nil {
			return nil, err
		}
		w, ok := owner[top]
		if !ok {
			return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
		}
		entries, err := fs.ReadDir(f.layers[w.layer].FS, name)
		if err != nil {
			return nil, err
		}
		return roEntries(entries), nil
	}

	owner, err := f.index()
	if err != nil {
		return nil, err
	}
	merged := make([]fs.DirEntry, 0, len(owner))
	for _, w := range owner {
		merged = append(merged, roEntry{w.entry})
	}
	sort.Slice(merged, func(a, b int) bool { return merged[a].Name() < merged[b].Name() })
	return merged, nil
}

// A Shadow is one entry present in more than one layer: the winner is what the
// union serves, and the losers contribute nothing.
type Shadow struct {
	Name   string
	Winner Layer
	Losers []Layer
}

// Shadowed enumerates every shadowed entry, by name. Losers are in declared
// order. It is computed on demand, so constructing a union costs nothing for a
// frontend that never asks.
func (f *FS) Shadowed() ([]Shadow, error) {
	holders := make(map[string][]int)
	for i, l := range f.layers {
		entries, err := fs.ReadDir(l.FS, ".")
		if err != nil {
			return nil, &fs.PathError{Op: "readdir", Path: l.Name, Err: err}
		}
		for _, e := range entries {
			holders[e.Name()] = append(holders[e.Name()], i)
		}
	}
	var shadows []Shadow
	for name, idx := range holders {
		if len(idx) < 2 {
			continue
		}
		s := Shadow{Name: name, Winner: f.layers[idx[len(idx)-1]]}
		for _, i := range idx[:len(idx)-1] {
			s.Losers = append(s.Losers, f.layers[i])
		}
		shadows = append(shadows, s)
	}
	sort.Slice(shadows, func(a, b int) bool { return shadows[a].Name < shadows[b].Name })
	return shadows, nil
}

// Read-only projections. Write permission bits are cleared because the view
// cannot honour them, and a file that reports itself writable and then rejects
// every write is worse than one that reports the truth.

const writeBits = 0o222

type roInfo struct{ fs.FileInfo }

func (i roInfo) Mode() fs.FileMode { return i.FileInfo.Mode() &^ writeBits }

type roEntry struct{ fs.DirEntry }

func (e roEntry) Type() fs.FileMode { return e.DirEntry.Type() }

func (e roEntry) Info() (fs.FileInfo, error) {
	info, err := e.DirEntry.Info()
	if err != nil {
		return nil, err
	}
	return roInfo{info}, nil
}

func roEntries(entries []fs.DirEntry) []fs.DirEntry {
	out := make([]fs.DirEntry, len(entries))
	for i, e := range entries {
		out[i] = roEntry{e}
	}
	return out
}

type roFile struct{ fs.File }

func (f *roFile) Stat() (fs.FileInfo, error) {
	info, err := f.File.Stat()
	if err != nil {
		return nil, err
	}
	return roInfo{info}, nil
}

// ReadDir forwards to the wrapped file when it can list, so that a caller
// holding an open directory sees the same read-only projection.
func (f *roFile) ReadDir(n int) ([]fs.DirEntry, error) {
	d, ok := f.File.(fs.ReadDirFile)
	if !ok {
		return nil, &fs.PathError{Op: "readdir", Path: ".", Err: fs.ErrInvalid}
	}
	entries, err := d.ReadDir(n)
	return roEntries(entries), err
}

// rootInfo describes the union's root, which belongs to no single layer.
type rootInfo struct{}

func (rootInfo) Name() string       { return "." }
func (rootInfo) Size() int64        { return 0 }
func (rootInfo) Mode() fs.FileMode  { return fs.ModeDir | 0o555 }
func (rootInfo) ModTime() time.Time { return time.Time{} }
func (rootInfo) IsDir() bool        { return true }
func (rootInfo) Sys() any           { return nil }

// rootDir is an open handle on the merged root.
type rootDir struct {
	entries []fs.DirEntry
	offset  int
}

func (d *rootDir) Stat() (fs.FileInfo, error) { return rootInfo{}, nil }
func (d *rootDir) Close() error               { return nil }

func (d *rootDir) Read([]byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: ".", Err: fs.ErrInvalid}
}

func (d *rootDir) ReadDir(n int) ([]fs.DirEntry, error) {
	rest := d.entries[d.offset:]
	if n <= 0 {
		d.offset = len(d.entries)
		return rest, nil
	}
	if len(rest) == 0 {
		return nil, io.EOF
	}
	if n > len(rest) {
		n = len(rest)
	}
	d.offset += n
	return rest[:n], nil
}
