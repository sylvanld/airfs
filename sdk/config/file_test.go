package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sylvanld/airfs/sdk/config"
)

// read is what the file says after an edit.
func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestEditPreservesWhatAPersonWrote(t *testing.T) {
	t.Setenv("HOME", "/home/tester")
	path := write(t, `# Every workspace on this machine.
workspaces:
  # My own capabilities, shared by everything below.
  personal: &base
    target: ~/.ai
    folders: [agents, skills]
    sources:
      - ~/repos/mine # authored by me
  scratch:
    <<: *base
    target: /tmp/scratch
`)

	if _, err := config.Edit(path, func(f *config.File) error {
		return f.SetEnabled("scratch", false)
	}); err != nil {
		t.Fatal(err)
	}

	got := read(t, path)
	// Comments, key order, anchors, aliases and flow style survive an edit;
	// the exact bytes around them do not.
	for _, want := range []string{
		"# Every workspace on this machine.",
		"# My own capabilities, shared by everything below.",
		"# authored by me",
		"personal: &base",
		"<<: *base",
		"folders: [agents, skills]",
		"enabled: false",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("%q did not survive the edit:\n%s", want, got)
		}
	}
	// The parse and the re-encode have to agree on how a merge key is spelled,
	// or every edit would rewrite every merge key in the file.
	if strings.Contains(got, "!!merge") {
		t.Errorf("a merge key was rewritten with its resolved tag:\n%s", got)
	}
	if _, err := config.Load(path); err != nil {
		t.Fatalf("the file no longer resolves after an edit: %v", err)
	}
}

func TestEditIsStableOnceCanonical(t *testing.T) {
	path := write(t, "workspaces:\n  a:\n    target: /t\n    sources:\n      - /one\n")
	noop := func(f *config.File) error { return f.SetEnabled("a", true) }
	if _, err := config.Edit(path, noop); err != nil {
		t.Fatal(err)
	}
	first := read(t, path)
	if _, err := config.Edit(path, noop); err != nil {
		t.Fatal(err)
	}
	// A file already in canonical form is left byte-identical by an edit that
	// changes nothing — the gofmt bargain.
	if second := read(t, path); second != first {
		t.Errorf("a repeated edit rewrote the file:\n%s\n---\n%s", first, second)
	}
}

func TestDeclareCreatesTheFile(t *testing.T) {
	t.Setenv("HOME", "/home/tester")
	path := filepath.Join(t.TempDir(), "nested", config.FileName)
	changed, err := config.Edit(path, func(f *config.File) error {
		return f.Declare(config.Declaration{
			Name:    "personal",
			Target:  "~/.ai",
			Sources: []string{"~/repos/mine"},
			Folders: []string{"skills"},
			Enabled: true,
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 1 || changed[0] != "personal" {
		t.Errorf("changed = %v, want [personal]", changed)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if w := cfg.Lookup("personal"); w == nil || !w.Enabled {
		t.Fatalf("the declared workspace is missing or disabled: %+v", w)
	}
	// The path is written as it was typed, so the file keeps the form its
	// author would recognise.
	if got := read(t, path); !strings.Contains(got, "~/.ai") {
		t.Errorf("the target was not written as typed:\n%s", got)
	}
}

func TestDeclareReplacesWholeAndKeepsItsComment(t *testing.T) {
	path := write(t, "workspaces:\n  # why this exists\n  a:\n    target: /t\n    folders: [skills]\n    sources:\n      - /one\n      - /two\n")
	if _, err := config.Edit(path, func(f *config.File) error {
		return f.Declare(config.Declaration{
			Name: "a", Target: "/t", Sources: []string{"/three"}, Enabled: true,
		})
	}); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	a := cfg.Lookup("a")
	// Someone who typed the sources they want is stating the whole list.
	if len(a.Sources) != 1 || a.Sources[0].Declared != "/three" {
		t.Errorf("sources = %v, want exactly [/three]", a.Sources)
	}
	if strings.Join(a.Folders, ",") != strings.Join(config.DefaultFolders, ",") {
		t.Errorf("declaring no folders must fall back to the default set, got %v", a.Folders)
	}
	if got := read(t, path); !strings.Contains(got, "# why this exists") {
		t.Errorf("a comment written above the block must outlive the block:\n%s", got)
	}
}

func TestRemoveReturnsTheBlock(t *testing.T) {
	path := write(t, "workspaces:\n  a:\n    target: /a\n    sources: [/one]\n  b:\n    target: /b\n    sources: [/two]\n")
	var block []byte
	changed, err := config.Edit(path, func(f *config.File) error {
		var err error
		block, err = f.Remove("a")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	// An unintended removal has to be recoverable from the terminal's
	// scrollback rather than only from git.
	for _, want := range []string{"a:", "/a", "/one"} {
		if !strings.Contains(string(block), want) {
			t.Errorf("the printed block is missing %q:\n%s", want, block)
		}
	}
	if len(changed) != 1 || changed[0] != "a" {
		t.Errorf("changed = %v, want [a]", changed)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Lookup("a") != nil || cfg.Lookup("b") == nil {
		t.Errorf("removing a took out the wrong workspace: %v", cfg.Names())
	}
}

func TestRemoveUnknownWorkspaceLeavesTheFileAlone(t *testing.T) {
	path := write(t, "workspaces:\n  a:\n    target: /a\n    sources: [/one]\n")
	before := read(t, path)
	if _, err := config.Edit(path, func(f *config.File) error {
		_, err := f.Remove("nope")
		return err
	}); err == nil {
		t.Fatal("removing an undeclared workspace was accepted")
	}
	if read(t, path) != before {
		t.Error("a failed edit rewrote the file")
	}
}

func TestEditValidatesBeforeWriting(t *testing.T) {
	path := write(t, "workspaces:\n  a:\n    target: /a\n    sources: [/one]\n")
	before := read(t, path)
	// A second workspace at the same target would have two workspaces writing
	// mountpoints into one tree; the existing configuration must stay standing.
	if _, err := config.Edit(path, func(f *config.File) error {
		return f.Declare(config.Declaration{
			Name: "b", Target: "/a", Sources: []string{"/two"}, Enabled: true,
		})
	}); err == nil {
		t.Fatal("an invalid result was written")
	}
	if read(t, path) != before {
		t.Errorf("a mistyped edit replaced the configuration:\n%s", read(t, path))
	}
}

func TestEditReportsChangeByEffectNotByArgument(t *testing.T) {
	// Editing personal's sources here changes scratch too, so an alias never
	// changes something silently.
	path := write(t, `workspaces:
  personal: &base
    target: /personal
    sources: [/one]
  scratch:
    <<: *base
    target: /scratch
`)
	changed, err := config.Edit(path, func(f *config.File) error {
		return f.Declare(config.Declaration{
			Name: "personal", Target: "/personal", Sources: []string{"/one", "/two"}, Enabled: true,
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 2 {
		t.Fatalf("changed = %v, want both personal and scratch", changed)
	}
}

func TestDeclareKeepsAnAnchorWorking(t *testing.T) {
	path := write(t, `workspaces:
  personal: &base
    target: /personal
    sources: [/one]
  scratch:
    <<: *base
    target: /scratch
`)
	if _, err := config.Edit(path, func(f *config.File) error {
		return f.Declare(config.Declaration{
			Name: "personal", Target: "/personal", Sources: []string{"/two"}, Enabled: true,
		})
	}); err != nil {
		t.Fatalf("replacing an anchored block left its aliases dangling: %v", err)
	}
	if got := read(t, path); !strings.Contains(got, "&base") {
		t.Errorf("the anchor did not survive the replacement:\n%s", got)
	}
}

func TestRemovingAnAnchorItsAliasesNeedIsRefused(t *testing.T) {
	path := write(t, `workspaces:
  personal: &base
    target: /personal
    sources: [/one]
  scratch:
    <<: *base
    target: /scratch
`)
	before := read(t, path)
	// Validating the result before writing it is what catches this: the block
	// is removable, the document it would leave behind is not readable.
	if _, err := config.Edit(path, func(f *config.File) error {
		_, err := f.Remove("personal")
		return err
	}); err == nil {
		t.Fatal("removing an anchor that an alias still needs was accepted")
	}
	if read(t, path) != before {
		t.Error("the file was rewritten into something that does not parse")
	}
}

func TestEditReportsNothingWhenNothingChanged(t *testing.T) {
	path := write(t, "workspaces:\n  a:\n    target: /a\n    sources: [/one]\n    enabled: true\n")
	changed, err := config.Edit(path, func(f *config.File) error {
		return f.SetEnabled("a", true)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 0 {
		t.Errorf("enabling something already enabled reported %v; it is a no-op", changed)
	}
}

func TestSetEnabledOverridesAMergedValue(t *testing.T) {
	path := write(t, `workspaces:
  personal: &base
    target: /personal
    sources: [/one]
    enabled: false
  scratch:
    <<: *base
    target: /scratch
`)
	if _, err := config.Edit(path, func(f *config.File) error {
		return f.SetEnabled("scratch", true)
	}); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	// An explicit value is the only one that certainly wins over a merge key.
	if !cfg.Lookup("scratch").Enabled {
		t.Error("enabling a workspace that inherits enabled: false did not take")
	}
	if cfg.Lookup("personal").Enabled {
		t.Error("enabling the alias changed what it merges from")
	}
}

func TestDeclareRejectsAnUnusableName(t *testing.T) {
	path := filepath.Join(t.TempDir(), config.FileName)
	if _, err := config.Edit(path, func(f *config.File) error {
		return f.Declare(config.Declaration{
			Name: "my workspace", Target: "/t", Sources: []string{"/one"}, Enabled: true,
		})
	}); err == nil {
		t.Fatal("a name that does not survive a command line was accepted")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("a rejected declaration created the file")
	}
}

func TestAsDeclared(t *testing.T) {
	t.Setenv("HOME", "/home/tester")
	for _, typed := range []string{"~/repos/mine", "$HOME/repos/mine", "/absolute/path"} {
		got, err := config.AsDeclared(typed)
		if err != nil {
			t.Fatal(err)
		}
		if got != typed {
			t.Errorf("AsDeclared(%q) = %q; a path that already means one thing is written as typed", typed, got)
		}
	}
	// On the command line a relative path means "from the working directory";
	// in the file it would mean "from the file's directory".
	got, err := config.AsDeclared("./relative")
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("AsDeclared(./relative) = %q, want an absolute path", got)
	}
}
