package layerfs_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/sylvanld/airfs/sdk/layerfs"
)

// layer names an in-memory tree, which is a valid layer: the merge semantics
// are testable exhaustively without mounting anything.
func layer(name string, files fstest.MapFS) layerfs.Layer {
	return layerfs.Layer{Name: name, FS: files}
}

func file(content string) *fstest.MapFile {
	return &fstest.MapFile{Data: []byte(content), Mode: 0o644}
}

func names(t *testing.T, entries []fs.DirEntry) []string {
	t.Helper()
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Name()
	}
	return out
}

func read(t *testing.T, f *layerfs.FS, name string) string {
	t.Helper()
	data, err := fs.ReadFile(f, name)
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	return string(data)
}

func equal(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestLastLayerWins(t *testing.T) {
	f := layerfs.New(
		layer("global", fstest.MapFS{"commit/SKILL.md": file("global")}),
		layer("project", fstest.MapFS{"commit/SKILL.md": file("project")}),
	)
	if got := read(t, f, "commit/SKILL.md"); got != "project" {
		t.Errorf("got %q, want the last layer's content", got)
	}
	if origin, ok := f.Origin("commit"); !ok || origin.Name != "project" {
		t.Errorf("Origin = %q, %v; want project", origin.Name, ok)
	}
}

// An entry wins whole: a half-merged skill assembled from two repositories
// would be a skill that exists in no repository.
func TestWinningEntryContributesItsWholeSubtree(t *testing.T) {
	f := layerfs.New(
		layer("global", fstest.MapFS{
			"commit/SKILL.md":  file("global"),
			"commit/helper.sh": file("only in global"),
		}),
		layer("project", fstest.MapFS{"commit/SKILL.md": file("project")}),
	)
	if _, err := f.Open("commit/helper.sh"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the losing layer's file is visible: err = %v", err)
	}
	equal(t, names(t, mustReadDir(t, f, "commit")), []string{"SKILL.md"})
}

func TestRootListingMergesDedupsAndSortsLexically(t *testing.T) {
	f := layerfs.New(
		layer("global", fstest.MapFS{"review/SKILL.md": file(""), "commit/SKILL.md": file("")}),
		layer("org", fstest.MapFS{"commit/SKILL.md": file(""), "apply.md": file("")}),
	)
	equal(t, names(t, mustReadDir(t, f, ".")), []string{"apply.md", "commit", "review"})
}

// A listed name and a looked-up name always resolve to the same layer.
func TestListingAndLookupAgreeOnTheWinner(t *testing.T) {
	f := layerfs.New(
		layer("global", fstest.MapFS{"commit/SKILL.md": file("global")}),
		layer("project", fstest.MapFS{"commit/SKILL.md": file("project")}),
	)
	entries := mustReadDir(t, f, ".")
	if len(entries) != 1 {
		t.Fatalf("got %v, want one entry", names(t, entries))
	}
	info, err := entries[0].Info()
	if err != nil {
		t.Fatal(err)
	}
	stat, err := f.Stat("commit")
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode() != stat.Mode() {
		t.Errorf("listing says %v, lookup says %v", info.Mode(), stat.Mode())
	}
}

// An entry may be a single file, since agent tooling commonly stores a command
// as one Markdown document.
func TestFileEntriesAreEntriesToo(t *testing.T) {
	f := layerfs.New(
		layer("global", fstest.MapFS{"commit.md": file("global")}),
		layer("project", fstest.MapFS{"commit.md": file("project")}),
	)
	if got := read(t, f, "commit.md"); got != "project" {
		t.Errorf("got %q, want project", got)
	}
}

// A name that is a directory in one layer and a file in a later one resolves as
// a file, with no attempt to reconcile the disagreement.
func TestTypeComesFromTheWinningLayer(t *testing.T) {
	f := layerfs.New(
		layer("global", fstest.MapFS{"commit/SKILL.md": file("global")}),
		layer("project", fstest.MapFS{"commit": file("project")}),
	)
	info, err := f.Stat("commit")
	if err != nil {
		t.Fatal(err)
	}
	if info.IsDir() {
		t.Error("resolved as a directory; the winning layer says it is a file")
	}
	if got := read(t, f, "commit"); got != "project" {
		t.Errorf("got %q, want project", got)
	}
}

func TestWritePermissionBitsAreCleared(t *testing.T) {
	f := layerfs.New(layer("global", fstest.MapFS{
		"commit/SKILL.md": &fstest.MapFile{Data: []byte("x"), Mode: 0o664},
	}))
	info, err := f.Stat("commit/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o222 != 0 {
		t.Errorf("mode %v still reports itself writable", info.Mode())
	}
	if info.Mode().Perm() != 0o444 {
		t.Errorf("mode %v: read bits should survive", info.Mode())
	}
}

// Dotfiles are ordinary entries and are neither hidden nor filtered.
func TestDotfilesAreOrdinaryEntries(t *testing.T) {
	f := layerfs.New(layer("global", fstest.MapFS{".hidden/SKILL.md": file("x")}))
	equal(t, names(t, mustReadDir(t, f, ".")), []string{".hidden"})
}

func TestShadowedNamesEveryWinnerAndLoserInOrder(t *testing.T) {
	f := layerfs.New(
		layer("global", fstest.MapFS{"commit/SKILL.md": file(""), "review/SKILL.md": file("")}),
		layer("org", fstest.MapFS{"commit/SKILL.md": file("")}),
		layer("project", fstest.MapFS{"commit/SKILL.md": file("")}),
	)
	shadows, err := f.Shadowed()
	if err != nil {
		t.Fatal(err)
	}
	if len(shadows) != 1 {
		t.Fatalf("got %d shadows, want 1 (review is not shadowed)", len(shadows))
	}
	if shadows[0].Name != "commit" || shadows[0].Winner.Name != "project" {
		t.Errorf("got %s won by %s, want commit won by project", shadows[0].Name, shadows[0].Winner.Name)
	}
	equal(t, []string{shadows[0].Losers[0].Name, shadows[0].Losers[1].Name}, []string{"global", "org"})
}

// A directory shadowing a file is a shadowing event like any other, because it
// is far more likely to be a mistake than an intention.
func TestShadowingAcrossTypesIsReported(t *testing.T) {
	f := layerfs.New(
		layer("global", fstest.MapFS{"commit/SKILL.md": file("")}),
		layer("project", fstest.MapFS{"commit": file("")}),
	)
	shadows, err := f.Shadowed()
	if err != nil {
		t.Fatal(err)
	}
	if len(shadows) != 1 {
		t.Fatalf("got %d shadows, want 1", len(shadows))
	}
}

func TestUnknownNamesDoNotExist(t *testing.T) {
	f := layerfs.New(layer("global", fstest.MapFS{"commit/SKILL.md": file("")}))
	for _, name := range []string{"absent", "absent/deep", "commit/absent"} {
		if _, err := f.Open(name); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("opening %s: err = %v, want ErrNotExist", name, err)
		}
	}
}

func TestEmptyUnionIsAnEmptyDirectory(t *testing.T) {
	f := layerfs.New()
	equal(t, names(t, mustReadDir(t, f, ".")), nil)
	info, err := f.Stat(".")
	if err != nil || !info.IsDir() {
		t.Fatalf("Stat(.) = %v, %v; want a directory", info, err)
	}
}

// Reads reach the backing layer, so an edit is visible with no refresh: the
// editing loop the whole project exists to serve depends on it.
func TestReadsAreAlwaysCurrent(t *testing.T) {
	backing := fstest.MapFS{"commit/SKILL.md": file("before")}
	f := layerfs.New(layer("global", backing))
	if got := read(t, f, "commit/SKILL.md"); got != "before" {
		t.Fatalf("got %q", got)
	}
	backing["commit/SKILL.md"] = file("after")
	backing["added.md"] = file("new")
	if got := read(t, f, "commit/SKILL.md"); got != "after" {
		t.Errorf("got %q, want the edited content with no refresh", got)
	}
	equal(t, names(t, mustReadDir(t, f, ".")), []string{"added.md", "commit"})
}

// fstest.TestFS exercises the standard library's own expectations of an fs.FS,
// including that Open, Stat, ReadDir and ReadFile agree with each other.
func TestSatisfiesTheStandardFilesystemContract(t *testing.T) {
	f := layerfs.New(
		layer("global", fstest.MapFS{
			"commit/SKILL.md":  file("global"),
			"commit/helper.sh": file("helper"),
			"review/SKILL.md":  file("review"),
		}),
		layer("project", fstest.MapFS{
			"commit/SKILL.md": file("project"),
			"apply.md":        file("apply"),
		}),
	)
	if err := fstest.TestFS(f, "commit/SKILL.md", "review/SKILL.md", "apply.md"); err != nil {
		t.Error(err)
	}
}

func mustReadDir(t *testing.T, f *layerfs.FS, name string) []fs.DirEntry {
	t.Helper()
	entries, err := f.ReadDir(name)
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	return entries
}

// A layer whose root is not there is the ordinary case, not a broken one: a
// workspace declares the folders it wants merged, a source contains the ones it
// has, and airfs creates neither.
func TestAbsentLayerContributesNothing(t *testing.T) {
	absent := layerfs.Layer{Name: "absent", FS: os.DirFS(filepath.Join(t.TempDir(), "nothing-here"))}
	f := layerfs.New(
		layer("global", fstest.MapFS{"commit/SKILL.md": file("global")}),
		absent,
	)

	equal(t, names(t, mustReadDir(t, f, ".")), []string{"commit"})
	if got := read(t, f, "commit/SKILL.md"); got != "global" {
		t.Errorf("got %q", got)
	}
	// It shadows nothing either, so inspecting a workspace over it still works.
	shadows, err := f.Shadowed()
	if err != nil {
		t.Fatalf("Shadowed over an absent layer: %v", err)
	}
	if len(shadows) != 0 {
		t.Errorf("shadows = %v", shadows)
	}
}
