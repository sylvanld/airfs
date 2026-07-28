package mount_test

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/sylvanld/airfs"
	"github.com/sylvanld/airfs/mount"
	"github.com/sylvanld/airfs/sources"
)

// serve mounts a target built from the given source list and returns the
// target directory. The mount is released when the test ends.
//
// The test skips rather than fails when the host cannot mount: /dev/fuse and a
// setuid fusermount3 are the host's to provide, and a container that lacks them
// is not a broken build.
func serve(t *testing.T, body string, build func(target string)) string {
	t.Helper()
	if err := mount.Preflight(); err != nil {
		t.Skipf("host cannot mount: %v", err)
	}

	target := t.TempDir()
	build(target)
	config := filepath.Join(target, sources.FileName)
	if err := os.WriteFile(config, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := sources.Load(config)
	if err != nil {
		t.Fatal(err)
	}
	server, err := mount.Serve(target, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := server.Unmount(); err != nil {
			t.Errorf("unmounting: %v", err)
		}
	})
	return target
}

// repo creates a source with the given files relative to its root.
func repo(t *testing.T, target, name string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		path := filepath.Join(target, name, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMountServesTheMergedView(t *testing.T) {
	target := serve(t, "global\nproject\n", func(target string) {
		repo(t, target, "global", map[string]string{
			"skills/commit/SKILL.md":  "global",
			"skills/commit/helper.sh": "only in global",
			"skills/review/SKILL.md":  "review",
			"commands/deploy.md":      "deploy",
		})
		repo(t, target, "project", map[string]string{
			"skills/commit/SKILL.md": "project",
		})
	})

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

	// A file entry is an entry, and each kind is its own mount.
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

// The configuration file stays readable while the view is live, because the
// mountpoints are the kind directories one level below it.
func TestConfigurationFileIsNotMaskedByTheMounts(t *testing.T) {
	target := serve(t, "global\n", func(target string) {
		repo(t, target, "global", map[string]string{"skills/commit/SKILL.md": "x"})
	})
	if got := readFile(t, filepath.Join(target, sources.FileName)); got != "global\n" {
		t.Errorf("got %q", got)
	}
}

// Read-only is enforced by the kernel, not by convention: no process can write
// through the mount even by mistake.
func TestWritesAreRejected(t *testing.T) {
	target := serve(t, "global\n", func(target string) {
		repo(t, target, "global", map[string]string{"skills/commit/SKILL.md": "x"})
	})
	skills := filepath.Join(target, "skills")

	if err := os.WriteFile(filepath.Join(skills, "new.md"), []byte("x"), 0o644); err == nil {
		t.Error("creating a file through the mount succeeded")
	}
	if err := os.Remove(filepath.Join(skills, "commit", "SKILL.md")); err == nil {
		t.Error("deleting through the mount succeeded")
	}
	f, err := os.OpenFile(filepath.Join(skills, "commit", "SKILL.md"), os.O_WRONLY, 0)
	if err == nil {
		f.Close()
		t.Error("opening for writing through the mount succeeded")
	}
}

func TestModesReportThemselvesReadOnly(t *testing.T) {
	target := serve(t, "global\n", func(target string) {
		repo(t, target, "global", map[string]string{"skills/commit/SKILL.md": "x"})
	})
	info, err := os.Stat(filepath.Join(target, "skills", "commit", "SKILL.md"))
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
	var backing string
	target := serve(t, "global\n", func(target string) {
		repo(t, target, "global", map[string]string{"skills/commit/SKILL.md": "before"})
		backing = filepath.Join(target, "global", "skills")
	})
	through := filepath.Join(target, "skills")

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
	var backing string
	target := serve(t, "global\n", func(target string) {
		repo(t, target, "global", map[string]string{"skills/commit/SKILL.md": "x"})
		backing = filepath.Join(target, "global", "skills", "commit")
	})
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

// Mounting over a populated directory hides its contents, and the hidden files
// are a trap.
func TestNonEmptyMountpointIsRefused(t *testing.T) {
	if err := mount.Preflight(); err != nil {
		t.Skipf("host cannot mount: %v", err)
	}
	target := t.TempDir()
	repo(t, target, "global", map[string]string{"skills/commit/SKILL.md": "x"})
	if err := os.MkdirAll(filepath.Join(target, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "skills", "stray.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(target, sources.FileName)
	if err := os.WriteFile(config, []byte("global\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := sources.Load(config)
	if err != nil {
		t.Fatal(err)
	}

	server, err := mount.Serve(target, cfg)
	if err == nil {
		server.Unmount()
		t.Fatal("mounting over a populated directory succeeded")
	}
	if !airfs.IsPrecondition(err) {
		t.Errorf("err = %v; want a precondition error", err)
	}

	// A failed attempt leaves nothing behind: no kind is left mounted.
	states, err := mount.Status(target)
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range states {
		if st.Mounted {
			t.Errorf("%s stayed mounted after a failed attempt", st.Kind)
		}
	}
}

func TestStatusAndUnmountReadTheKernel(t *testing.T) {
	if err := mount.Preflight(); err != nil {
		t.Skipf("host cannot mount: %v", err)
	}
	target := t.TempDir()
	repo(t, target, "global", map[string]string{"skills/commit/SKILL.md": "x"})
	config := filepath.Join(target, sources.FileName)
	if err := os.WriteFile(config, []byte("global\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := sources.Load(config)
	if err != nil {
		t.Fatal(err)
	}

	// Nothing mounted yet, and unmounting is safe rather than an error.
	states, err := mount.Status(target)
	if err != nil {
		t.Fatal(err)
	}
	if mount.Served(states) {
		t.Error("reported as served before mounting")
	}
	released, err := mount.Unmount(target)
	if err != nil || len(released) != 0 {
		t.Errorf("unmounting nothing: %v, %v", released, err)
	}

	if _, err := mount.Serve(target, cfg); err != nil {
		t.Fatal(err)
	}
	states, err = mount.Status(target)
	if err != nil {
		t.Fatal(err)
	}
	if !mount.Served(states) {
		t.Errorf("not reported as served: %+v", states)
	}

	// A second serve of the same target is detected rather than stacked.
	if _, err := mount.Serve(target, cfg); err == nil {
		t.Error("mounting an already served target succeeded")
	} else if !airfs.IsPrecondition(err) {
		t.Errorf("err = %v; want a precondition error", err)
	}

	released, err = mount.Unmount(target)
	if err != nil {
		t.Fatal(err)
	}
	if len(released) != len(airfs.Kinds) {
		t.Errorf("released %v, want every kind", released)
	}
	waitUnmounted(t, target)
}

func waitUnmounted(t *testing.T, target string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		states, err := mount.Status(target)
		if err != nil {
			t.Fatal(err)
		}
		mounted := false
		for _, st := range states {
			mounted = mounted || st.Mounted
		}
		if !mounted {
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
