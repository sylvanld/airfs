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
	"github.com/sylvanld/airfs/sdk"
	"github.com/sylvanld/airfs/sdk/config"
)

// FSName is the filesystem name set at mount time. Finding what airfs has
// mounted means looking for it in the kernel's mount table: a second record of
// what is mounted would disagree with the kernel exactly when it matters,
// after a crash or a manual unmount.
const FSName = "airfs"

// fsType is how the kernel reports a mount made under FSName.
const fsType = "fuse." + FSName

// A Server holds every folder mount of one workspace. A workspace is either
// serving or not: its folders are established together and released together,
// since a half-served workspace is a view that lies about what is available.
type Server struct {
	Workspace *config.Workspace

	mu      sync.Mutex
	servers map[string]*fuse.Server
	folders []string
}

// Serve mounts every folder of w under its target and returns once the view is
// ready. It does not block; call Wait to serve until unmounted.
//
// If any folder fails to mount, every folder already mounted for that
// workspace is released before returning, so a failed attempt leaves nothing
// behind.
func Serve(w *config.Workspace) (*Server, error) {
	if err := Preflight(); err != nil {
		return nil, err
	}
	if err := w.Readable(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(w.Target.Resolved, 0o755); err != nil {
		return nil, err
	}

	s := &Server{Workspace: w, servers: map[string]*fuse.Server{}}
	for _, folder := range w.Folders {
		dir := w.FolderDir(folder)
		if err := prepareMountpoint(dir); err != nil {
			s.release()
			return nil, err
		}
		server, err := mountFolder(dir, w, folder)
		if err != nil {
			s.release()
			return nil, fmt.Errorf("mounting %s: %w", dir, err)
		}
		s.servers[folder] = server
		s.folders = append(s.folders, folder)
	}
	return s, nil
}

func mountFolder(dir string, w *config.Workspace, folder string) (*fuse.Server, error) {
	root := &rootNode{fsys: w.Merged(folder)}
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

// prepareMountpoint creates a folder's directory if it is missing and refuses
// one that would hide something.
func prepareMountpoint(dir string) error {
	if mounted, stale, err := State(dir); err != nil {
		return err
	} else if mounted {
		what := "already served"
		if stale {
			what = "held by a stale mount; release it first"
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

// Folders are the folders this server has mounted.
func (s *Server) Folders() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.folders...)
}

// Wait blocks until every mount has been released. A mount lives as long as
// the process serving it.
func (s *Server) Wait() {
	for _, folder := range s.Folders() {
		s.mu.Lock()
		server := s.servers[folder]
		s.mu.Unlock()
		if server != nil {
			server.Wait()
		}
	}
}

// Unmount releases every mount this server holds.
func (s *Server) Unmount() error { return s.release() }

func (s *Server) release() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var errs []error
	for folder, server := range s.servers {
		if err := server.Unmount(); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", folder, err))
		}
		delete(s.servers, folder)
	}
	s.folders = nil
	return errors.Join(errs...)
}

// A Mount is one airfs mountpoint as the kernel currently reports it.
//
// A mount whose serving process has died is Stale: still listed by the kernel,
// but failing every access. That signature is what distinguishes it from a live
// mount and from an ordinary directory, and a person who cannot tell them apart
// cannot act.
type Mount struct {
	Dir   string
	Stale bool
}

// Mounts lists every airfs mount on the machine, in the order the kernel
// reports them — including those a previous daemon left behind and those whose
// serving process died.
//
// This is the inventory. It is read from the kernel rather than from anything
// airfs keeps, because a record airfs keeps disagrees with reality exactly when
// it matters: after a crash, after a manual unmount, after the configuration
// changed while nothing was running.
func Mounts() ([]Mount, error) {
	dirs, err := mountTable()
	if err != nil {
		return nil, err
	}
	mounts := make([]Mount, 0, len(dirs))
	for _, dir := range dirs {
		mounts = append(mounts, Mount{Dir: dir, Stale: isStale(dir)})
	}
	return mounts, nil
}

// Under lists the airfs mounts within dir, which is how a workspace's own
// mounts are found among the machine's.
func Under(mounts []Mount, dir string) []Mount {
	var found []Mount
	for _, m := range mounts {
		if m.Dir == dir || within(m.Dir, dir) {
			found = append(found, m)
		}
	}
	return found
}

func within(path, dir string) bool {
	return strings.HasPrefix(path, strings.TrimSuffix(dir, "/")+"/")
}

// State reports whether dir is an airfs mountpoint, and whether it is stale.
func State(dir string) (mounted, stale bool, err error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return false, false, err
	}
	dirs, err := mountTable()
	if err != nil {
		return false, false, err
	}
	for _, mounted := range dirs {
		if mounted == abs {
			return true, isStale(abs), nil
		}
	}
	return false, false, nil
}

// isStale probes a mountpoint to tell a live mount from one whose serving
// process has died.
func isStale(dir string) bool {
	_, err := os.ReadDir(dir)
	// A severed FUSE connection is the stale-mount signature.
	return errors.Is(err, syscall.ENOTCONN)
}

// Unmount releases one mountpoint, including recovering a stale one left by a
// serving process that died.
//
// It goes through the setuid helper, which is what an unprivileged process has
// to use. A stale mount can still be busy, so a lazy detach is the fallback
// rather than the default.
func Unmount(dir string) error {
	out, err := exec.Command("fusermount3", "-u", dir).CombinedOutput()
	if err == nil {
		return nil
	}
	lazy, lazyErr := exec.Command("fusermount3", "-uz", dir).CombinedOutput()
	if lazyErr == nil {
		return nil
	}
	return fmt.Errorf("unmounting %s: %s: %s", dir,
		strings.TrimSpace(string(out)), strings.TrimSpace(string(lazy)))
}

// UnmountAll releases every mount given and reports which it released, so that
// a caller can say what it did rather than only that it succeeded. A mount it
// cannot release is reported without stopping the others.
func UnmountAll(mounts []Mount) (released []string, err error) {
	var errs []error
	for _, m := range mounts {
		if err := Unmount(m.Dir); err != nil {
			errs = append(errs, err)
			continue
		}
		released = append(released, m.Dir)
	}
	return released, errors.Join(errs...)
}

// mountTable reads every mountpoint the kernel reports as served by airfs, by
// the filesystem type its mounts are made under.
func mountTable() ([]string, error) {
	file, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var dirs []string
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
		if len(fields) < 5 {
			continue
		}
		rest := strings.Fields(line[sep+3:])
		if len(rest) >= 1 && rest[0] == fsType {
			dirs = append(dirs, unescape(fields[4]))
		}
	}
	return dirs, scan.Err()
}

// unescape decodes the octal escapes mountinfo uses for space, tab, newline and
// backslash in a path.
func unescape(s string) string {
	r := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	return r.Replace(s)
}
