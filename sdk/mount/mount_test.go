package mount_test

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/sylvanld/airfs/sdk"
	"github.com/sylvanld/airfs/sdk/config"
	"github.com/sylvanld/airfs/sdk/mount"
)

// A workspace under test: sources are directories beside the target, since a
// target holds nothing but its mounted subfolders.
type fixture struct {
	dir       string
	workspace *config.Workspace
}

// declare builds a workspace over the named sources, in precedence order. The
// sources are not created; repo does that.
func declare(t *testing.T, folders []string, names ...string) fixture {
	t.Helper()
	dir := t.TempDir()
	w := &config.Workspace{
		Name:    "test",
		Target:  path(filepath.Join(dir, "target")),
		Folders: folders,
		Enabled: true,
	}
	for _, name := range names {
		source := filepath.Join(dir, name)
		if err := os.MkdirAll(source, 0o755); err != nil {
			t.Fatal(err)
		}
		w.Sources = append(w.Sources, path(source))
	}
	return fixture{dir: dir, workspace: w}
}

func path(p string) config.Path { return config.Path{Declared: p, Resolved: p} }

// serve establishes the workspace and releases it when the test ends.
//
// The test skips rather than fails when the host cannot mount: /dev/fuse and a
// setuid fusermount3 are the host's to provide, and a container that lacks them
// is not a broken build.
func (f fixture) serve(t *testing.T) string {
	t.Helper()
	if err := mount.Preflight(); err != nil {
		t.Skipf("host cannot mount: %v", err)
	}
	server, err := mount.Serve(f.workspace)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := server.Unmount(); err != nil {
			t.Errorf("unmounting: %v", err)
		}
	})
	return f.workspace.Target.Resolved
}

// repo writes the given files into one of the workspace's sources.
func (f fixture) repo(t *testing.T, name string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		p := filepath.Join(f.dir, name, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMountServesTheMergedView(t *testing.T) {
	f := declare(t, []string{"skills", "commands"}, "global", "project")
	f.repo(t, "global", map[string]string{
		"skills/commit/SKILL.md":  "global",
		"skills/commit/helper.sh": "only in global",
		"skills/review/SKILL.md":  "review",
		"commands/deploy.md":      "deploy",
	})
	f.repo(t, "project", map[string]string{"skills/commit/SKILL.md": "project"})
	target := f.serve(t)

	skills := filepath.Join(target, "skills")

	// The last source wins, whole: the losing layer's extra file is not visible.
	if got := readFile(t, filepath.Join(skills, "commit", "SKILL.md")); got != "project" {
		t.Errorf("got %q, want the last source's content", got)
	}
	if _, err := os.Stat(filepath.Join(skills, "commit", "helper.sh")); !os.IsNotExist(err) {
		t.Errorf("the losing layer's file is visible: %v", err)
	}

	// Unshadowed entries from an earlier source still appear.
	if got := readFile(t, filepath.Join(skills, "review", "SKILL.md")); got != "review" {
		t.Errorf("got %q", got)
	}

	// A file entry is an entry, and each folder is its own mount.
	if got := readFile(t, filepath.Join(target, "commands", "deploy.md")); got != "deploy" {
		t.Errorf("got %q", got)
	}

	entries, err := os.ReadDir(skills)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Name() != "commit" || entries[1].Name() != "review" {
		t.Errorf("listing is %v, want commit then review", entries)
	}
}

// A workspace names the folders it merges; airfs attaches no meaning to any of
// them and creates none of them in a source.
func TestFoldersAreWhateverTheWorkspaceDeclared(t *testing.T) {
	f := declare(t, []string{"prompts", "absent"}, "global")
	f.repo(t, "global", map[string]string{"prompts/review.md": "review"})
	target := f.serve(t)

	if got := readFile(t, filepath.Join(target, "prompts", "review.md")); got != "review" {
		t.Errorf("got %q", got)
	}
	// A folder no source contributes to is mounted and empty, not an error.
	entries, err := os.ReadDir(filepath.Join(target, "absent"))
	if err != nil {
		t.Fatalf("a folder no source has must still be served: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("the empty folder lists %v", entries)
	}
	// airfs performs no write against a source.
	if _, err := os.Stat(filepath.Join(f.dir, "global", "absent")); !os.IsNotExist(err) {
		t.Error("serving created a folder inside a source")
	}
}

// Read-only is enforced by the kernel, not by convention: no process can write
// through the mount even by mistake.
func TestWritesAreRejected(t *testing.T) {
	f := declare(t, []string{"skills"}, "global")
	f.repo(t, "global", map[string]string{"skills/commit/SKILL.md": "x"})
	skills := filepath.Join(f.serve(t), "skills")

	if err := os.WriteFile(filepath.Join(skills, "new.md"), []byte("x"), 0o644); err == nil {
		t.Error("creating a file through the mount succeeded")
	}
	if err := os.Remove(filepath.Join(skills, "commit", "SKILL.md")); err == nil {
		t.Error("deleting through the mount succeeded")
	}
	file, err := os.OpenFile(filepath.Join(skills, "commit", "SKILL.md"), os.O_WRONLY, 0)
	if err == nil {
		file.Close()
		t.Error("opening for writing through the mount succeeded")
	}
}

func TestModesReportThemselvesReadOnly(t *testing.T) {
	f := declare(t, []string{"skills"}, "global")
	f.repo(t, "global", map[string]string{"skills/commit/SKILL.md": "x"})
	info, err := os.Stat(filepath.Join(f.serve(t), "skills", "commit", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o222 != 0 {
		t.Errorf("mode %v reports itself writable", info.Mode())
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatal("no stat available")
	}
	if stat.Uid != uint32(os.Getuid()) {
		t.Errorf("owned by uid %d, want the invoking user %d", stat.Uid, os.Getuid())
	}
}

// The editing loop this project exists to serve: change the file in its
// repository, read it through the mount, see the change with no intervention.
func TestEditsInASourceAreVisibleWithoutRemounting(t *testing.T) {
	f := declare(t, []string{"skills"}, "global")
	f.repo(t, "global", map[string]string{"skills/commit/SKILL.md": "before"})
	backing := filepath.Join(f.dir, "global", "skills")
	through := filepath.Join(f.serve(t), "skills")

	if got := readFile(t, filepath.Join(through, "commit", "SKILL.md")); got != "before" {
		t.Fatalf("got %q", got)
	}
	if err := os.WriteFile(filepath.Join(backing, "commit", "SKILL.md"), []byte("after"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(through, "commit", "SKILL.md")); got != "after" {
		t.Errorf("got %q, want the edited content", got)
	}

	// A newly added entry appears too, without re-establishing the view.
	if err := os.MkdirAll(filepath.Join(backing, "added"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backing, "added", "SKILL.md"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(through, "added", "SKILL.md")); got != "new" {
		t.Errorf("got %q, want the newly added entry", got)
	}
}

func TestSymlinksInASourceAreServedAsSuch(t *testing.T) {
	f := declare(t, []string{"skills"}, "global")
	f.repo(t, "global", map[string]string{"skills/commit/SKILL.md": "x"})
	backing := filepath.Join(f.dir, "global", "skills", "commit")
	target := f.serve(t)

	if err := os.Symlink("SKILL.md", filepath.Join(backing, "alias.md")); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(target, "skills", "commit", "alias.md")
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("mode %v, want a symlink", info.Mode())
	}
	if got := readFile(t, link); got != "x" {
		t.Errorf("following it read %q", got)
	}
}

// A source that is not there fails the workspace declaring it, and does so
// before anything is mounted.
func TestAbsentSourceIsRefused(t *testing.T) {
	if err := mount.Preflight(); err != nil {
		t.Skipf("host cannot mount: %v", err)
	}
	f := declare(t, []string{"skills"}, "global")
	f.workspace.Sources = append(f.workspace.Sources, path(filepath.Join(f.dir, "absent")))

	server, err := mount.Serve(f.workspace)
	if err == nil {
		server.Unmount()
		t.Fatal("a workspace with a missing source was established")
	}
	if !airfs.IsPrecondition(err) {
		t.Errorf("err = %v; want a precondition error", err)
	}
	if mounted := mount.Under(mounts(t), f.workspace.Target.Resolved); len(mounted) != 0 {
		t.Errorf("a failed attempt left %v mounted", mounted)
	}
}

// Mounting over a populated directory hides its contents, and the hidden files
// are a trap.
func TestNonEmptyMountpointIsRefused(t *testing.T) {
	if err := mount.Preflight(); err != nil {
		t.Skipf("host cannot mount: %v", err)
	}
	f := declare(t, []string{"skills", "commands"}, "global")
	f.repo(t, "global", map[string]string{"skills/commit/SKILL.md": "x"})
	stray := filepath.Join(f.workspace.Target.Resolved, "commands", "stray.md")
	if err := os.MkdirAll(filepath.Dir(stray), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stray, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	server, err := mount.Serve(f.workspace)
	if err == nil {
		server.Unmount()
		t.Fatal("mounting over a populated directory succeeded")
	}
	if !airfs.IsPrecondition(err) {
		t.Errorf("err = %v; want a precondition error", err)
	}

	// Folders are established together and released together: the one that did
	// mount is not left behind, because a half-served workspace lies about what
	// is available.
	if mounted := mount.Under(mounts(t), f.workspace.Target.Resolved); len(mounted) != 0 {
		t.Errorf("a failed attempt left %v mounted", mounted)
	}
}

func TestTheKernelIsTheInventory(t *testing.T) {
	if err := mount.Preflight(); err != nil {
		t.Skipf("host cannot mount: %v", err)
	}
	f := declare(t, []string{"skills"}, "global")
	f.repo(t, "global", map[string]string{"skills/commit/SKILL.md": "x"})
	target := f.workspace.Target.Resolved

	// Nothing mounted yet, and releasing nothing is safe rather than an error.
	if mounted := mount.Under(mounts(t), target); len(mounted) != 0 {
		t.Errorf("reported as mounted before mounting: %v", mounted)
	}
	if released, err := mount.UnmountAll(nil); err != nil || len(released) != 0 {
		t.Errorf("releasing nothing: %v, %v", released, err)
	}

	if _, err := mount.Serve(f.workspace); err != nil {
		t.Fatal(err)
	}
	mounted := mount.Under(mounts(t), target)
	if len(mounted) != 1 || mounted[0].Dir != filepath.Join(target, "skills") {
		t.Errorf("the kernel reports %v, want the workspace's one folder", mounted)
	}
	if mounted[0].Stale {
		t.Error("a live mount is reported as stale")
	}

	// A second serve of the same workspace is detected rather than stacked.
	if _, err := mount.Serve(f.workspace); err == nil {
		t.Error("mounting an already served workspace succeeded")
	} else if !airfs.IsPrecondition(err) {
		t.Errorf("err = %v; want a precondition error", err)
	}

	released, err := mount.UnmountAll(mounted)
	if err != nil {
		t.Fatal(err)
	}
	if len(released) != 1 {
		t.Errorf("released %v, want the one folder", released)
	}
	waitUnmounted(t, target)
}

func mounts(t *testing.T) []mount.Mount {
	t.Helper()
	all, err := mount.Mounts()
	if err != nil {
		t.Fatal(err)
	}
	return all
}

func waitUnmounted(t *testing.T, target string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if len(mount.Under(mounts(t), target)) == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Error("mounts were still listed by the kernel after unmounting")
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		var pe *os.PathError
		if errors.As(err, &pe) {
			t.Fatalf("reading %s: %v", path, pe.Err)
		}
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(data)
}
