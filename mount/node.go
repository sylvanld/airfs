package mount

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"syscall"

	gofs "github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/sylvanld/airfs/layerfs"
)

// The mount reports every entry as owned by the invoking user, whose mount it
// is and who is the only one able to see it.
var (
	mountUID = uint32(os.Getuid())
	mountGID = uint32(os.Getgid())
)

const writeBits = 0o222

// rootNode serves one kind: a merged directory whose entries each resolve
// wholly to one layer.
type rootNode struct {
	gofs.Inode
	fsys *layerfs.FS
}

var (
	_ gofs.NodeReaddirer = (*rootNode)(nil)
	_ gofs.NodeLookuper  = (*rootNode)(nil)
	_ gofs.NodeGetattrer = (*rootNode)(nil)
)

func (n *rootNode) Getattr(ctx context.Context, f gofs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Mode = fuse.S_IFDIR | 0o555
	out.Owner = fuse.Owner{Uid: mountUID, Gid: mountGID}
	return 0
}

func (n *rootNode) Readdir(ctx context.Context) (gofs.DirStream, syscall.Errno) {
	entries, err := n.fsys.ReadDir(".")
	if err != nil {
		return nil, errno(err)
	}
	listing := make([]fuse.DirEntry, 0, len(entries))
	for _, e := range entries {
		listing = append(listing, fuse.DirEntry{
			Name: e.Name(),
			Mode: uint32(e.Type()) & syscall.S_IFMT,
		})
	}
	return gofs.NewListDirStream(listing), 0
}

// Lookup resolves a top-level entry to the layer that wins it, and hands the
// rest of the subtree to a node backed by that layer's real path.
func (n *rootNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*gofs.Inode, syscall.Errno) {
	layer, ok := n.fsys.Origin(name)
	if !ok || layer.Root == "" {
		return nil, syscall.ENOENT
	}
	path := filepath.Join(layer.Root, name)
	return n.child(ctx, path, out)
}

func (n *rootNode) child(ctx context.Context, path string, out *fuse.EntryOut) (*gofs.Inode, syscall.Errno) {
	var st syscall.Stat_t
	if err := syscall.Lstat(path, &st); err != nil {
		return nil, errno(err)
	}
	fill(&out.Attr, &st)
	child := &pathNode{path: path}
	return n.NewInode(ctx, child, gofs.StableAttr{Mode: out.Attr.Mode & syscall.S_IFMT}), 0
}

// pathNode serves a subtree that comes wholly from one layer, by reading
// through to the real path backing it. Nothing below the root merges, so this
// is an ordinary read-only passthrough.
type pathNode struct {
	gofs.Inode
	path string
}

var (
	_ gofs.NodeReaddirer  = (*pathNode)(nil)
	_ gofs.NodeLookuper   = (*pathNode)(nil)
	_ gofs.NodeGetattrer  = (*pathNode)(nil)
	_ gofs.NodeOpener     = (*pathNode)(nil)
	_ gofs.NodeReadlinker = (*pathNode)(nil)
)

func (n *pathNode) Getattr(ctx context.Context, f gofs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	var st syscall.Stat_t
	if err := syscall.Lstat(n.path, &st); err != nil {
		return errno(err)
	}
	fill(&out.Attr, &st)
	return 0
}

func (n *pathNode) Readdir(ctx context.Context) (gofs.DirStream, syscall.Errno) {
	entries, err := os.ReadDir(n.path)
	if err != nil {
		return nil, errno(err)
	}
	// Lexical order, as the union guarantees, so a listing is reproducible
	// across machines rather than reflecting the underlying filesystem's order.
	sort.Slice(entries, func(a, b int) bool { return entries[a].Name() < entries[b].Name() })
	listing := make([]fuse.DirEntry, 0, len(entries))
	for _, e := range entries {
		listing = append(listing, fuse.DirEntry{
			Name: e.Name(),
			Mode: uint32(e.Type()) & syscall.S_IFMT,
		})
	}
	return gofs.NewListDirStream(listing), 0
}

func (n *pathNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*gofs.Inode, syscall.Errno) {
	path := filepath.Join(n.path, name)
	var st syscall.Stat_t
	if err := syscall.Lstat(path, &st); err != nil {
		return nil, errno(err)
	}
	fill(&out.Attr, &st)
	child := &pathNode{path: path}
	return n.NewInode(ctx, child, gofs.StableAttr{Mode: out.Attr.Mode & syscall.S_IFMT}), 0
}

func (n *pathNode) Readlink(ctx context.Context) ([]byte, syscall.Errno) {
	target, err := os.Readlink(n.path)
	if err != nil {
		return nil, errno(err)
	}
	return []byte(target), 0
}

// Open reads through to the backing file rather than buffering its content in
// the serving process, which keeps memory-mapping correct and keeps a large
// file from being materialised in memory.
func (n *pathNode) Open(ctx context.Context, flags uint32) (gofs.FileHandle, uint32, syscall.Errno) {
	if flags&(syscall.O_WRONLY|syscall.O_RDWR|syscall.O_APPEND|syscall.O_CREAT|syscall.O_TRUNC) != 0 {
		return nil, 0, syscall.EROFS
	}
	fd, err := syscall.Open(n.path, syscall.O_RDONLY, 0)
	if err != nil {
		return nil, 0, errno(err)
	}
	// No FOPEN_KEEP_CACHE: the kernel drops stale page-cache content, so an
	// edit in a source repository is visible on the next read.
	return gofs.NewLoopbackFile(fd), 0, 0
}

// fill projects a real file's metadata into what the mount reports: the
// backing values, with ownership rewritten to the invoking user and write bits
// cleared, per docs/specs/layered-fs.md.
func fill(attr *fuse.Attr, st *syscall.Stat_t) {
	attr.FromStat(st)
	attr.Mode &^= writeBits
	attr.Owner = fuse.Owner{Uid: mountUID, Gid: mountGID}
	// Inode numbers are assigned by go-fuse per node, stable for the mount's
	// lifetime. The backing device's numbers are not reused, since two sources
	// on different filesystems can collide.
	attr.Ino = 0
}

func errno(err error) syscall.Errno {
	var e syscall.Errno
	switch {
	case err == nil:
		return 0
	case os.IsNotExist(err):
		return syscall.ENOENT
	case os.IsPermission(err):
		return syscall.EACCES
	}
	if pe, ok := err.(*fs.PathError); ok {
		if se, ok := pe.Err.(syscall.Errno); ok {
			return se
		}
	}
	if se, ok := err.(syscall.Errno); ok {
		return se
	}
	return e | syscall.EIO
}
