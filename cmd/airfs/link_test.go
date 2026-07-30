package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sylvanld/airfs/sdk"
)

// standing puts the test in a project directory, since link is the one command
// whose frame of reference is where you are standing.
func standing(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(dir)
	return dir
}

func TestLinkReportsTheRootAndTheSymlinkItWrote(t *testing.T) {
	dir := standing(t, nil)

	out, code := invoke(t, "link", "--claude")

	if code != airfs.ExitOK {
		t.Fatalf("exit %d\n%s", code, out)
	}
	for _, want := range []string{"root       .ai/skills", "linked", "-> ../.ai/skills", "Write skills in .ai/skills/."} {
		if !strings.Contains(out, want) {
			t.Errorf("the report does not mention %q:\n%s", want, out)
		}
	}
	if _, err := os.Lstat(filepath.Join(dir, ".claude", "skills")); err != nil {
		t.Errorf("nothing was linked: %v", err)
	}
}

func TestLinkOrdersTheToolsTheWayTheFlagsWereGiven(t *testing.T) {
	// The order is the one thing the person running the command controls, so it
	// is the one the conflict rule turns on — and the flag package does not
	// preserve it.
	dir := standing(t, map[string]string{
		".claude/skills/commit/SKILL.md":   "claude's",
		".opencode/skills/commit/SKILL.md": "opencode's",
	})

	out, code := invoke(t, "link", "--opencode", "--claude")

	if code != airfs.ExitOK {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if !strings.Contains(out, "commit -> commit-claude") {
		t.Errorf("the second flag did not give up the name:\n%s", out)
	}
	kept, err := os.ReadFile(filepath.Join(dir, ".ai", "skills", "commit", "SKILL.md"))
	if err != nil || string(kept) != "opencode's" {
		t.Errorf("commit holds %q (%v); the first flag keeps the bare name", kept, err)
	}
}

func TestLinkNamingNoToolPrintsTheTableRatherThanGuessing(t *testing.T) {
	standing(t, nil)

	out, code := invoke(t, "link")

	if code != airfs.ExitPrecondition {
		t.Errorf("exit %d, want %d", code, airfs.ExitPrecondition)
	}
	if !strings.Contains(out, "--claude") {
		t.Errorf("it did not say which tools it knows:\n%s", out)
	}
	if _, err := os.Stat(".ai"); err == nil {
		t.Error("it created a directory nobody asked for")
	}
}

func TestLinkListPrintsWhatTheBinaryBelievesAboutEachTool(t *testing.T) {
	standing(t, nil)

	out, code := invoke(t, "link", "--list")

	if code != airfs.ExitOK {
		t.Fatalf("exit %d\n%s", code, out)
	}
	// A wrong row is the worst bug this command can have, so what it believes
	// is inspectable without reading the spec.
	if !strings.Contains(out, ".claude/skills") || !strings.Contains(out, ".opencode/skills") {
		t.Errorf("the table does not name the paths:\n%s", out)
	}
}

func TestLinkDryRunReportsAndWritesNothing(t *testing.T) {
	dir := standing(t, map[string]string{".claude/skills/commit/SKILL.md": "one"})

	out, code := invoke(t, "link", "--claude", "--dry-run")

	if code != airfs.ExitOK {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if !strings.Contains(out, "moved      commit") {
		t.Errorf("it did not name the move it would make:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, ".ai")); err == nil {
		t.Error("a dry run wrote something")
	}
}

func TestLinkExitsTwoWhenAToolIsRefused(t *testing.T) {
	standing(t, map[string]string{".claude/skills": "a file, not a directory"})

	out, code := invoke(t, "link", "--claude", "--opencode")

	if code != airfs.ExitPrecondition {
		t.Errorf("exit %d, want %d\n%s", code, airfs.ExitPrecondition, out)
	}
	if !strings.Contains(out, "refused") || !strings.Contains(out, "linked") {
		t.Errorf("a refusal must fail one tool and leave the others linked:\n%s", out)
	}
}
