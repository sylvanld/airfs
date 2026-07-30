package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sylvanld/airfs/sdk"
	"github.com/sylvanld/airfs/sdk/config"
)

// write puts a configuration in a temporary directory and returns its path.
func write(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), config.FileName)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadResolvesWorkspaces(t *testing.T) {
	t.Setenv("HOME", "/home/tester")
	t.Setenv("PROJECTS", "/srv/projects")
	path := write(t, `
workspaces:
  personal:
    target: ~/.ai
    folders: [agents, skills]
    sources:
      - ~/repos/mine
      - $PROJECTS/shared
  work:
    target: /work/.ai
    sources: [./relative-source]
    enabled: false
`)

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Names(); len(got) != 2 || got[0] != "personal" || got[1] != "work" {
		t.Fatalf("workspaces in file order = %v", got)
	}

	personal := cfg.Lookup("personal")
	if personal.Target.Resolved != "/home/tester/.ai" {
		t.Errorf("target resolved to %q", personal.Target.Resolved)
	}
	if personal.Target.Declared != "~/.ai" {
		t.Errorf("target declared as %q; reports must name what the author wrote", personal.Target.Declared)
	}
	if got := personal.Sources[1].Resolved; got != "/srv/projects/shared" {
		t.Errorf("$VAR source resolved to %q", got)
	}
	if !personal.Enabled {
		t.Error("a workspace without an enabled field must default to enabled")
	}
	if got := strings.Join(personal.Folders, ","); got != "agents,skills" {
		t.Errorf("folders = %q", got)
	}

	work := cfg.Lookup("work")
	if work.Enabled {
		t.Error("enabled: false must disable the workspace")
	}
	if got := strings.Join(work.Folders, ","); got != strings.Join(config.DefaultFolders, ",") {
		t.Errorf("folders without a declaration = %q, want the default set", got)
	}
	// A relative path in the file means "from the file's directory", never the
	// working directory, so that the file means the same thing wherever it is
	// read from.
	if want := filepath.Join(filepath.Dir(path), "relative-source"); work.Sources[0].Resolved != want {
		t.Errorf("relative source resolved to %q, want %q", work.Sources[0].Resolved, want)
	}
}

func TestLoadResolvesMergeKeys(t *testing.T) {
	t.Setenv("HOME", "/home/tester")
	cfg, err := config.Load(write(t, `
workspaces:
  personal: &base
    target: ~/.ai
    folders: [skills]
    sources: [~/repos/mine]
  scratch:
    <<: *base
    target: /tmp/scratch
`))
	if err != nil {
		t.Fatal(err)
	}
	scratch := cfg.Lookup("scratch")
	if scratch.Target.Resolved != "/tmp/scratch" {
		t.Errorf("the merging workspace's own key must win: target = %q", scratch.Target.Resolved)
	}
	if len(scratch.Sources) != 1 || scratch.Sources[0].Resolved != "/home/tester/repos/mine" {
		t.Errorf("merged sources = %v", scratch.Sources)
	}
	if got := strings.Join(scratch.Folders, ","); got != "skills" {
		t.Errorf("merged folders = %q", got)
	}
}

func TestLoadMissingFileIsAPrecondition(t *testing.T) {
	_, err := config.Load(filepath.Join(t.TempDir(), "absent.yaml"))
	if !airfs.IsPrecondition(err) {
		t.Fatalf("a missing configuration must be a precondition, got %v", err)
	}
	if !strings.Contains(err.Error(), "airfs add") {
		t.Errorf("the first thing a new user hits must say what to do next: %v", err)
	}
}

func TestLoadEmptyFileDeclaresNothing(t *testing.T) {
	cfg, err := config.Load(write(t, "# nothing yet\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Workspaces) != 0 {
		t.Fatalf("an empty file declares %d workspaces", len(cfg.Workspaces))
	}
}

func TestValidationRejects(t *testing.T) {
	t.Setenv("HOME", "/home/tester")
	for name, test := range map[string]struct{ body, want string }{
		"unknown top-level key": {
			"targets: {}\n", "unknown top-level key",
		},
		"unknown workspace key": {
			"workspaces:\n  a:\n    target: /t\n    sources: [/s]\n    kinds: [skills]\n", `unknown key "kinds"`,
		},
		"no sources": {
			"workspaces:\n  a:\n    target: /t\n", "no sources",
		},
		"no target": {
			"workspaces:\n  a:\n    sources: [/s]\n", "no target",
		},
		"empty folders": {
			"workspaces:\n  a:\n    target: /t\n    sources: [/s]\n    folders: []\n", "folders is empty",
		},
		"folder is a path": {
			"workspaces:\n  a:\n    target: /t\n    sources: [/s]\n    folders: [a/b]\n", "single path element",
		},
		"folder is a parent": {
			"workspaces:\n  a:\n    target: /t\n    sources: [/s]\n    folders: ['..']\n", "single path element",
		},
		"duplicate source": {
			"workspaces:\n  a:\n    target: /t\n    sources: [/s, /s/]\n", "duplicates",
		},
		"shared target": {
			"workspaces:\n  a:\n    target: /t\n    sources: [/s]\n  b:\n    target: /t\n    sources: [/s]\n", "share the target",
		},
		"nested target": {
			"workspaces:\n  a:\n    target: /t\n    sources: [/s]\n  b:\n    target: /t/inner\n    sources: [/s]\n", "is inside",
		},
		"bad name": {
			"workspaces:\n  my workspace:\n    target: /t\n    sources: [/s]\n", "must be non-empty and match",
		},
		"unset variable": {
			"workspaces:\n  a:\n    target: /t\n    sources: [$AIRFS_UNSET_VARIABLE/x]\n", "is not set",
		},
		"enabled is not a bool": {
			"workspaces:\n  a:\n    target: /t\n    sources: [/s]\n    enabled: sometimes\n", "must be true or false",
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := config.Load(write(t, test.body))
			if err == nil {
				t.Fatalf("%s was accepted", name)
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Errorf("error %q does not mention %q", err, test.want)
			}
			// The configuration is what the host or the author must fix, never
			// airfs malfunctioning.
			if !airfs.IsPrecondition(err) {
				t.Errorf("%s is not reported as a precondition", name)
			}
		})
	}
}

func TestValidationReportsEveryProblem(t *testing.T) {
	_, err := config.Load(write(t, "workspaces:\n  a:\n    sources: []\n  b:\n    target: /t\n"))
	if err == nil {
		t.Fatal("a file with several problems was accepted")
	}
	for _, want := range []string{"workspace a: no sources", "workspace b: no sources"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%q is missing from:\n%v", want, err)
		}
	}
}

func TestErrorsNameTheLine(t *testing.T) {
	path := write(t, "workspaces:\n  a:\n    target: /t\n    sources: [/s]\n    folders: [a/b]\n")
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("accepted a folder that is a path")
	}
	if !strings.Contains(err.Error(), path+":5:") {
		t.Errorf("the error must point at the line that caused it: %v", err)
	}
}

func TestWorkspaceSameAs(t *testing.T) {
	load := func(body string) *config.Workspace {
		t.Helper()
		cfg, err := config.Load(write(t, body))
		if err != nil {
			t.Fatal(err)
		}
		return cfg.Workspaces[0]
	}
	base := load("workspaces:\n  a:\n    target: /t\n    folders: [skills]\n    sources: [/one, /two]\n")

	same := load("workspaces:\n  a:\n    target: /t\n    folders: [skills]\n    sources: [/one, /two]\n")
	if !base.SameAs(same) {
		t.Error("an identical declaration must compare same, or reload would remount everything")
	}
	// A union is immutable once built, so each of these means releasing and
	// establishing again rather than adjusting what is mounted.
	for name, body := range map[string]string{
		"reordered sources": "workspaces:\n  a:\n    target: /t\n    folders: [skills]\n    sources: [/two, /one]\n",
		"added source":      "workspaces:\n  a:\n    target: /t\n    folders: [skills]\n    sources: [/one, /two, /three]\n",
		"changed folders":   "workspaces:\n  a:\n    target: /t\n    folders: [prompts]\n    sources: [/one, /two]\n",
		"moved target":      "workspaces:\n  a:\n    target: /other\n    folders: [skills]\n    sources: [/one, /two]\n",
	} {
		if base.SameAs(load(body)) {
			t.Errorf("%s must not compare same", name)
		}
	}
	if base.SameAs(nil) {
		t.Error("a workspace nothing was established for must not compare same")
	}
}

func TestCountsAndMergeSkipAbsentFolders(t *testing.T) {
	dir := t.TempDir()
	full, sparse := filepath.Join(dir, "full"), filepath.Join(dir, "sparse")
	if err := os.MkdirAll(filepath.Join(full, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(full, "skills", "one.md"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sparse, 0o755); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(write(t, "workspaces:\n  a:\n    target: /t\n    folders: [skills]\n    sources: ["+full+", "+sparse+"]\n"))
	if err != nil {
		t.Fatal(err)
	}
	counts, err := cfg.Workspaces[0].Counts("skills")
	if err != nil {
		t.Fatalf("a source without the folder must contribute nothing, not fail: %v", err)
	}
	if counts[0] != 1 || counts[1] != 0 {
		t.Errorf("counts = %v, want [1 0]", counts)
	}
	// airfs performs no write against a source: the missing folder stays missing.
	if _, err := os.Stat(filepath.Join(sparse, "skills")); !os.IsNotExist(err) {
		t.Error("reading a workspace created a folder inside a source")
	}
}
