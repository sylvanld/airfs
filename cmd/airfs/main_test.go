package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sylvanld/airfs/sdk"
	"github.com/sylvanld/airfs/sdk/sources"
)

// An absent configuration is the first thing a new user hits, so it is answered
// with what to do rather than with the bare open error.
func TestMissingConfigurationSaysHowToCreateIt(t *testing.T) {
	p := paths{target: t.TempDir()}
	p.config = filepath.Join(p.target, sources.FileName)

	_, err := p.load()
	if err == nil || !airfs.IsPrecondition(err) {
		t.Fatalf("err = %v; want a precondition error", err)
	}
	if !strings.Contains(err.Error(), "no source list") {
		t.Errorf("err = %v; should say the list is missing", err)
	}
}

// A missing *source* fails with the same os.ErrNotExist as a missing
// configuration and means something else entirely: reporting it as an absent
// source list sends the reader to a file that is sitting right there.
func TestMissingSourceIsNotReportedAsAMissingConfiguration(t *testing.T) {
	p := paths{target: t.TempDir()}
	p.config = filepath.Join(p.target, sources.FileName)
	if err := os.WriteFile(p.config, []byte("absent-layer\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := p.load()
	if err == nil || !airfs.IsPrecondition(err) {
		t.Fatalf("err = %v; want a precondition error", err)
	}
	if strings.Contains(err.Error(), "no source list") {
		t.Errorf("err = %v; blames the configuration file, which exists", err)
	}
	if !strings.Contains(err.Error(), "absent-layer") {
		t.Errorf("err = %v; should name the source that is missing", err)
	}
}

// Declaring sources on the command line states the whole list: what the file
// said before is gone, comments included, rather than being merged with.
func TestDeclaredSourcesReplaceTheConfiguration(t *testing.T) {
	root := t.TempDir()
	p := paths{target: filepath.Join(root, "workspace")}
	p.config = filepath.Join(p.target, sources.FileName)
	if err := os.MkdirAll(p.target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.config, []byte("old  # my notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	one, two := filepath.Join(root, "one"), filepath.Join(root, "two")
	for _, dir := range []string{one, two} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	if err := p.write([]string{one, two}); err != nil {
		t.Fatal(err)
	}

	got := read(t, p.config)
	if want := one + "\n" + two + "\n"; got != want {
		t.Errorf("configuration = %q; want %q", got, want)
	}
}

// The flag is what brings a workspace into being, so it cannot require the
// directory that holds the configuration to have been created first.
func TestDeclaredSourcesCreateTheTarget(t *testing.T) {
	root := t.TempDir()
	p := paths{target: filepath.Join(root, "absent", "workspace")}
	p.config = filepath.Join(p.target, sources.FileName)
	layer := filepath.Join(root, "layer")
	if err := os.Mkdir(layer, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := p.write([]string{layer}); err != nil {
		t.Fatal(err)
	}

	if _, err := p.load(); err != nil {
		t.Fatalf("the written configuration does not load: %v", err)
	}
}

// A relative path means "from the working directory" when typed and "from the
// file's directory" when read back. Written verbatim it would name something
// else entirely, so it is made absolute first.
func TestRelativeDeclarationIsWrittenAbsolute(t *testing.T) {
	root := t.TempDir()
	p := paths{target: filepath.Join(root, "workspace")}
	p.config = filepath.Join(p.target, sources.FileName)
	layer := filepath.Join(root, "layer")
	if err := os.Mkdir(layer, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)

	if err := p.write([]string{"layer"}); err != nil {
		t.Fatal(err)
	}

	if got := read(t, p.config); got != layer+"\n" {
		t.Errorf("configuration = %q; want the path rooted at %q", got, layer)
	}
}

// Replacing keeps no history, so the one protection against a typo is that the
// new list has to resolve before the old one is given up.
func TestUnresolvableDeclarationLeavesTheConfigurationAlone(t *testing.T) {
	root := t.TempDir()
	p := paths{target: filepath.Join(root, "workspace")}
	p.config = filepath.Join(p.target, sources.FileName)
	if err := os.MkdirAll(p.target, 0o755); err != nil {
		t.Fatal(err)
	}
	const before = "kept\n"
	if err := os.WriteFile(p.config, []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}

	err := p.write([]string{filepath.Join(root, "absent-layer")})
	if err == nil || !airfs.IsPrecondition(err) {
		t.Fatalf("err = %v; want a precondition error", err)
	}
	if got := read(t, p.config); got != before {
		t.Errorf("configuration = %q; the failed write replaced it", got)
	}
	// A temporary file left beside it would be read as a second source list by
	// anyone looking at the directory.
	entries, err := os.ReadDir(p.target)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("target holds %d entries; want only the configuration", len(entries))
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
