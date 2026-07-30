package daemon_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sylvanld/airfs/sdk/config"
	"github.com/sylvanld/airfs/sdk/daemon"
	"github.com/sylvanld/airfs/sdk/mount"
)

// host prepares a machine a daemon can reconcile, and returns the directory
// the test builds its sources and targets in.
//
// Reconciliation acts on every airfs mount on the machine, which is the point
// of it and also means these tests must not run against a machine that has
// any: releasing a contributor's live workspaces to test the daemon would be
// an unpleasant surprise. Both conditions skip rather than fail — neither is a
// broken build.
func host(t *testing.T) string {
	t.Helper()
	if err := mount.Preflight(); err != nil {
		t.Skipf("host cannot mount: %v", err)
	}
	mounts, err := mount.Mounts()
	if err != nil {
		t.Fatal(err)
	}
	if len(mounts) > 0 {
		t.Skipf("this host has airfs mounts of its own (%d); refusing to reconcile them away", len(mounts))
	}
	// A per-test runtime directory keeps the control socket out of the real
	// one, so a test daemon and a real one never meet.
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	return t.TempDir()
}

// declare writes a configuration and returns its path. Sources named in it are
// created, so that a workspace fails only when a test means it to.
func declare(t *testing.T, dir, body string) string {
	t.Helper()
	for _, name := range []string{"global", "project"} {
		skills := filepath.Join(dir, name, "skills")
		if err := os.MkdirAll(skills, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(skills, name+".md"), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(dir, config.FileName)
	if err := os.WriteFile(path, []byte(strings.ReplaceAll(body, "$DIR", dir)), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// start runs a daemon and stops it when the test ends.
func start(t *testing.T, configPath string) (*daemon.Daemon, []daemon.Outcome) {
	t.Helper()
	d, outcomes, err := daemon.Start(configPath)
	if err != nil {
		t.Fatal(err)
	}
	served := make(chan error, 1)
	go func() { served <- d.Wait() }()
	t.Cleanup(func() {
		d.Stop()
		select {
		case <-served:
		case <-time.After(5 * time.Second):
			t.Error("the daemon did not stop")
		}
	})
	return d, outcomes
}

// action is what reconciliation did to one workspace.
func action(outcomes []daemon.Outcome, name string) daemon.Action {
	for _, o := range outcomes {
		if o.Workspace == name {
			return o.Action
		}
	}
	return ""
}

func mountedFolders(t *testing.T, target string) []string {
	t.Helper()
	all, err := mount.Mounts()
	if err != nil {
		t.Fatal(err)
	}
	var folders []string
	for _, m := range mount.Under(all, target) {
		folders = append(folders, filepath.Base(m.Dir))
	}
	return folders
}

func TestOneDaemonHoldsEveryWorkspace(t *testing.T) {
	dir := host(t)
	path := declare(t, dir, `
workspaces:
  personal:
    target: $DIR/personal
    folders: [skills]
    sources: [$DIR/global, $DIR/project]
  work:
    target: $DIR/work
    folders: [skills]
    sources: [$DIR/global]
`)
	_, outcomes := start(t, path)

	for _, name := range []string{"personal", "work"} {
		if got := action(outcomes, name); got != daemon.Established {
			t.Errorf("%s was %q, want established", name, got)
		}
	}
	// Both are held by this one process, and each folder is its own mount.
	if got := mountedFolders(t, filepath.Join(dir, "personal")); len(got) != 1 || got[0] != "skills" {
		t.Errorf("personal has %v mounted", got)
	}
	// The merged view is what the layers say: the last source wins.
	served := filepath.Join(dir, "personal", "skills")
	entries, err := os.ReadDir(served)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Errorf("%s lists %v, want both sources' entries", served, entries)
	}
}

// A report names a target as its author declared it, but a mountpoint read from
// the kernel is always absolute. Both forms have to reach a client, or matching
// what is mounted against what is declared silently finds nothing whenever a
// target was written with a `~`.
func TestStatusCarriesTheTargetInBothForms(t *testing.T) {
	dir := host(t)
	t.Setenv("HOME", dir)
	path := declare(t, dir, `
workspaces:
  personal:
    target: ~/personal
    folders: [skills]
    sources: [$DIR/global]
`)
	d, _ := start(t, path)

	w := d.Status().Workspaces[0]
	if w.Target != "~/personal" {
		t.Errorf("Target = %q, want the path as declared", w.Target)
	}
	if w.TargetDir != filepath.Join(dir, "personal") {
		t.Errorf("TargetDir = %q, want the resolved path", w.TargetDir)
	}
	// The resolved form is what a mountpoint is matched against.
	if len(mount.Under(mountsOf(t), w.TargetDir)) != 1 {
		t.Errorf("no mount found under the resolved target %q", w.TargetDir)
	}
}

func mountsOf(t *testing.T) []mount.Mount {
	t.Helper()
	all, err := mount.Mounts()
	if err != nil {
		t.Fatal(err)
	}
	return all
}

func TestReloadLeavesUnchangedWorkspacesAlone(t *testing.T) {
	dir := host(t)
	path := declare(t, dir, `
workspaces:
  personal:
    target: $DIR/personal
    folders: [skills]
    sources: [$DIR/global]
`)
	d, _ := start(t, path)

	// Adding a workspace must not disturb the one currently being read by a
	// running agent, which is what makes reload usable at all.
	declare(t, dir, `
workspaces:
  personal:
    target: $DIR/personal
    folders: [skills]
    sources: [$DIR/global]
  work:
    target: $DIR/work
    folders: [skills]
    sources: [$DIR/global]
`)
	outcomes, err := d.Reload()
	if err != nil {
		t.Fatal(err)
	}
	if got := action(outcomes, "personal"); got != daemon.Unchanged {
		t.Errorf("personal was %q, want unchanged", got)
	}
	if got := action(outcomes, "work"); got != daemon.Established {
		t.Errorf("work was %q, want established", got)
	}
}

func TestReloadReestablishesAChangedWorkspace(t *testing.T) {
	dir := host(t)
	path := declare(t, dir, `
workspaces:
  personal:
    target: $DIR/personal
    folders: [skills]
    sources: [$DIR/global]
`)
	d, _ := start(t, path)
	served := filepath.Join(dir, "personal", "skills")
	if entries, _ := os.ReadDir(served); len(entries) != 1 {
		t.Fatalf("%s lists %v, want one source's entry", served, entries)
	}

	// A union is immutable once built, so a changed source list means
	// releasing and establishing again.
	declare(t, dir, `
workspaces:
  personal:
    target: $DIR/personal
    folders: [skills]
    sources: [$DIR/global, $DIR/project]
`)
	outcomes, err := d.Reload()
	if err != nil {
		t.Fatal(err)
	}
	if got := action(outcomes, "personal"); got != daemon.Reestablished {
		t.Errorf("personal was %q, want re-established", got)
	}
	if entries, _ := os.ReadDir(served); len(entries) != 2 {
		t.Errorf("%s lists %v after re-establishing, want both sources", served, entries)
	}
}

func TestDisablingReleasesAndKeepsDeclaring(t *testing.T) {
	dir := host(t)
	path := declare(t, dir, `
workspaces:
  personal:
    target: $DIR/personal
    folders: [skills]
    sources: [$DIR/global]
`)
	d, _ := start(t, path)

	declare(t, dir, `
workspaces:
  personal:
    target: $DIR/personal
    folders: [skills]
    sources: [$DIR/global]
    enabled: false
`)
	outcomes, err := d.Reload()
	if err != nil {
		t.Fatal(err)
	}
	if got := action(outcomes, "personal"); got != daemon.Released {
		t.Errorf("personal was %q, want released", got)
	}
	if got := mountedFolders(t, filepath.Join(dir, "personal")); len(got) != 0 {
		t.Errorf("a disabled workspace still has %v mounted", got)
	}
	// The mount table was read once at the top of reconciliation, so a
	// mountpoint released during the pass is still listed in it. Releasing it a
	// second time fails, and reporting that would claim a failure for work that
	// succeeded.
	for _, o := range outcomes {
		if o.Action == daemon.Failed {
			t.Errorf("releasing a disabled workspace reported a failure: %s", o)
		}
	}
	// It stays declared and stays in every report; nothing of it is mounted.
	status := d.Status()
	if len(status.Workspaces) != 1 || status.Workspaces[0].Enabled || status.Workspaces[0].Established {
		t.Errorf("status = %+v, want personal declared, disabled and not established", status.Workspaces)
	}
}

func TestRemovingAWorkspaceReleasesIt(t *testing.T) {
	dir := host(t)
	path := declare(t, dir, `
workspaces:
  personal:
    target: $DIR/personal
    folders: [skills]
    sources: [$DIR/global]
  work:
    target: $DIR/work
    folders: [skills]
    sources: [$DIR/global]
`)
	d, _ := start(t, path)

	// A mount at no declared target would make the configuration a partial
	// account of the machine.
	declare(t, dir, `
workspaces:
  personal:
    target: $DIR/personal
    folders: [skills]
    sources: [$DIR/global]
`)
	if _, err := d.Reload(); err != nil {
		t.Fatal(err)
	}
	if got := mountedFolders(t, filepath.Join(dir, "work")); len(got) != 0 {
		t.Errorf("an undeclared target still has %v mounted", got)
	}
	if got := mountedFolders(t, filepath.Join(dir, "personal")); len(got) != 1 {
		t.Errorf("the declared workspace lost its mounts: %v", got)
	}
}

func TestOneBadWorkspaceDoesNotTakeDownTheRest(t *testing.T) {
	dir := host(t)
	path := declare(t, dir, `
workspaces:
  broken:
    target: $DIR/broken
    folders: [skills]
    sources: [$DIR/not-there]
  personal:
    target: $DIR/personal
    folders: [skills]
    sources: [$DIR/global]
`)
	d, outcomes := start(t, path)

	if got := action(outcomes, "broken"); got != daemon.Failed {
		t.Errorf("broken was %q, want failed", got)
	}
	// This is the change the centralized file forces: when one file describes
	// the machine, one mistyped path must not take down the rest.
	if got := action(outcomes, "personal"); got != daemon.Established {
		t.Errorf("personal was %q, want established despite its neighbour", got)
	}
	status := d.Status()
	for _, w := range status.Workspaces {
		if w.Name == "broken" && w.Reason == "" {
			t.Error("a failed workspace must report why")
		}
	}
	if status.Served() {
		t.Error("a status with an enabled, unestablished workspace reports itself served")
	}
}

func TestAnInvalidReloadChangesNothing(t *testing.T) {
	dir := host(t)
	path := declare(t, dir, `
workspaces:
  personal:
    target: $DIR/personal
    folders: [skills]
    sources: [$DIR/global]
`)
	d, _ := start(t, path)

	if err := os.WriteFile(path, []byte("workspaces:\n  personal:\n    target: $DIR/personal\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Reload(); err == nil {
		t.Fatal("a configuration with no sources was accepted")
	}
	// Neither the running mounts nor the daemon's idea of what is declared.
	if got := mountedFolders(t, filepath.Join(dir, "personal")); len(got) != 1 {
		t.Errorf("a refused reload changed what is mounted: %v", got)
	}
	if got := d.Status().Workspaces; len(got) != 1 || !got[0].Established {
		t.Errorf("a refused reload changed the daemon's state: %+v", got)
	}
}

func TestStartupReestablishesWhatItCannotVouchFor(t *testing.T) {
	dir := host(t)
	path := declare(t, dir, `
workspaces:
  personal:
    target: $DIR/personal
    folders: [skills]
    sources: [$DIR/global]
`)
	// A mount from a previous daemon, which this one has no record of.
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mount.Serve(cfg.Workspaces[0]); err != nil {
		t.Fatal(err)
	}

	_, outcomes := start(t, path)
	if got := action(outcomes, "personal"); got != daemon.Reestablished {
		t.Errorf("personal was %q; a mount the daemon cannot vouch for must be re-established", got)
	}
	if got := mountedFolders(t, filepath.Join(dir, "personal")); len(got) != 1 {
		t.Errorf("mounted folders = %v", got)
	}
}

func TestReconciliationIsIdempotent(t *testing.T) {
	dir := host(t)
	path := declare(t, dir, `
workspaces:
  personal:
    target: $DIR/personal
    folders: [skills, prompts]
    sources: [$DIR/global]
`)
	d, _ := start(t, path)

	// Running it twice against an unchanged configuration is a no-op the
	// second time, which is what makes it safe to run on every reload without
	// asking whether it is needed.
	for i := range 2 {
		outcomes, err := d.Reload()
		if err != nil {
			t.Fatal(err)
		}
		if got := action(outcomes, "personal"); got != daemon.Unchanged {
			t.Errorf("reload %d was %q, want unchanged", i+1, got)
		}
		if len(outcomes) != 1 {
			t.Errorf("reload %d did more than nothing: %v", i+1, outcomes)
		}
	}
}

func TestSecondDaemonIsRefused(t *testing.T) {
	dir := host(t)
	path := declare(t, dir, `
workspaces:
  personal:
    target: $DIR/personal
    folders: [skills]
    sources: [$DIR/global]
`)
	start(t, path)

	// Two daemons reconciling the same machine would fight: each would find
	// the other's mounts unaccounted for and release them.
	if _, _, err := daemon.Start(path); err == nil {
		t.Fatal("a second daemon started")
	}
}
