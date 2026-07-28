package sources_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sylvanld/airfs/sdk"
	"github.com/sylvanld/airfs/sdk/sources"
)

// write creates a configuration file in a fresh target directory and returns
// its path.
func write(t *testing.T, body string) (target, config string) {
	t.Helper()
	target = t.TempDir()
	config = filepath.Join(target, sources.FileName)
	if err := os.WriteFile(config, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return target, config
}

// source creates a source directory under dir, with the given relative files.
func source(t *testing.T, dir, name string, files ...string) string {
	t.Helper()
	root := filepath.Join(dir, name)
	for _, f := range files {
		path := filepath.Join(root, f)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if len(files) == 0 {
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestOrderIsPreservedExactlyAsWritten(t *testing.T) {
	target, config := write(t, "b\na\nc\n")
	for _, name := range []string{"a", "b", "c"} {
		source(t, target, name)
	}
	cfg, err := sources.Load(config)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, s := range cfg.Sources {
		got = append(got, s.Declared)
	}
	if strings.Join(got, ",") != "b,a,c" {
		t.Errorf("got %v, want the declared order", got)
	}
}

func TestCommentsAndBlankLinesDeclareNothing(t *testing.T) {
	target, config := write(t, "# a comment\n\n   \na  # trailing\n")
	source(t, target, "a")
	cfg, err := sources.Load(config)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Sources) != 1 || cfg.Sources[0].Declared != "a" {
		t.Errorf("got %+v, want one source named a", cfg.Sources)
	}
}

// A relative path resolves against the directory containing the configuration
// file, so the file means the same thing wherever it is invoked from.
func TestRelativePathsResolveAgainstTheConfigurationFile(t *testing.T) {
	target, config := write(t, "repo\n")
	root := source(t, target, "repo")
	elsewhere := t.TempDir()
	t.Chdir(elsewhere)

	cfg, err := sources.Load(config)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Sources[0].Path != root {
		t.Errorf("resolved to %s, want %s", cfg.Sources[0].Path, root)
	}
}

func TestEnvironmentVariablesExpand(t *testing.T) {
	target, config := write(t, "$REPO_ROOT/repo\n${REPO_ROOT}/other\n")
	source(t, target, "repo")
	source(t, target, "other")
	t.Setenv("REPO_ROOT", target)

	cfg, err := sources.Load(config)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Sources[0].Path != filepath.Join(target, "repo") {
		t.Errorf("got %s", cfg.Sources[0].Path)
	}
	if cfg.Sources[1].Path != filepath.Join(target, "other") {
		t.Errorf("got %s", cfg.Sources[1].Path)
	}
}

// An unset variable is an error, not an empty string: silently resolving to a
// shorter path would layer something nobody declared.
func TestUnsetVariableIsAnError(t *testing.T) {
	_, config := write(t, "$NOT_SET_ANYWHERE/repo\n")
	_, err := sources.Load(config)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !airfs.IsPrecondition(err) {
		t.Errorf("err = %v; want a precondition", err)
	}
	if !strings.Contains(err.Error(), "NOT_SET_ANYWHERE") {
		t.Errorf("err = %v; should name the variable", err)
	}
}

func TestTildeExpandsToHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	_, config := write(t, "~/repo\n")
	source(t, home, "repo")

	cfg, err := sources.Load(config)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Sources[0].Path != filepath.Join(home, "repo") {
		t.Errorf("got %s, want %s", cfg.Sources[0].Path, filepath.Join(home, "repo"))
	}
	if cfg.Sources[0].Declared != "~/repo" {
		t.Errorf("reported as %s; reports name what the author wrote", cfg.Sources[0].Declared)
	}
}

// A duplicate means the author believes the file says something it does not.
func TestDuplicateAfterExpansionIsAnError(t *testing.T) {
	target, config := write(t, "repo\n./repo\n")
	source(t, target, "repo")
	_, err := sources.Load(config)
	if err == nil || !airfs.IsPrecondition(err) {
		t.Fatalf("err = %v; want a precondition error", err)
	}
	if !strings.Contains(err.Error(), "duplicates line 1") {
		t.Errorf("err = %v; should point at the first declaration", err)
	}
}

// Proceeding with a view quietly missing a repository's resources is the harder
// failure to diagnose.
func TestMissingSourceIsAnError(t *testing.T) {
	_, config := write(t, "absent\n")
	_, err := sources.Load(config)
	if err == nil || !airfs.IsPrecondition(err) {
		t.Fatalf("err = %v; want a precondition error", err)
	}
}

func TestMissingConfigurationIsAPrecondition(t *testing.T) {
	_, err := sources.Load(filepath.Join(t.TempDir(), "sources.txt"))
	if err == nil || !airfs.IsPrecondition(err) {
		t.Fatalf("err = %v; want a precondition error", err)
	}
}

// Every source contributes every kind, so adding a resource of a new kind to a
// repository is a mkdir-free act.
func TestMissingKindDirectoriesAreCreatedEmpty(t *testing.T) {
	target, config := write(t, "repo\n")
	root := source(t, target, "repo", "skills/commit/SKILL.md")

	if _, err := sources.Load(config); err != nil {
		t.Fatal(err)
	}
	for _, kind := range airfs.Kinds {
		entries, err := os.ReadDir(filepath.Join(root, kind.String()))
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if kind == airfs.Skills && len(entries) != 1 {
			t.Errorf("skills: got %d entries, want the existing one untouched", len(entries))
		}
		if kind != airfs.Skills && len(entries) != 0 {
			t.Errorf("%s: got %d entries, want it created empty", kind, len(entries))
		}
	}
}

func TestMergedViewLayersSourcesInDeclaredOrder(t *testing.T) {
	target, config := write(t, "global\nproject\n")
	source(t, target, "global", "skills/commit/SKILL.md", "skills/review/SKILL.md")
	source(t, target, "project", "skills/commit/SKILL.md")

	cfg, err := sources.Load(config)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(target, "project", "skills", "commit", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	merged := cfg.Merged(airfs.Skills)
	got, err := merged.Open("commit/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	defer got.Close()
	buf := make([]byte, len(data))
	if _, err := got.Read(buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != string(data) {
		t.Errorf("got %q, want the last source's content %q", buf, data)
	}

	shadows, err := merged.Shadowed()
	if err != nil {
		t.Fatal(err)
	}
	if len(shadows) != 1 || shadows[0].Name != "commit" || shadows[0].Winner.Name != "project" {
		t.Errorf("got %+v, want commit won by project", shadows)
	}
}

func TestCountsReportEachSourcesContribution(t *testing.T) {
	target, config := write(t, "global\nproject\n")
	source(t, target, "global", "skills/a/SKILL.md", "skills/b/SKILL.md")
	source(t, target, "project", "skills/a/SKILL.md")

	cfg, err := sources.Load(config)
	if err != nil {
		t.Fatal(err)
	}
	counts, err := cfg.Counts(airfs.Skills)
	if err != nil {
		t.Fatal(err)
	}
	if counts[0] != 2 || counts[1] != 1 {
		t.Errorf("got %v, want [2 1]", counts)
	}
	empty, err := cfg.Counts(airfs.Scripts)
	if err != nil {
		t.Fatal(err)
	}
	if empty[0] != 0 || empty[1] != 0 {
		t.Errorf("got %v, want a kind no source contributes to", empty)
	}
}
