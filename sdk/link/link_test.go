package link_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/sylvanld/airfs/sdk"
	"github.com/sylvanld/airfs/sdk/link"
)

// project builds a directory tree from a map of project-relative path to file
// content, and returns the project root. A skill is a directory holding a file,
// which is what makes name collisions worth comparing rather than trivial.
func project(t *testing.T, files map[string]string) string {
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
	return dir
}

// tool is the row a test names, so that a test reads like a command line.
func tool(t *testing.T, flag string) link.Tool {
	t.Helper()
	found, ok := link.Lookup(flag)
	if !ok {
		t.Fatalf("no such tool %q", flag)
	}
	return found
}

func run(t *testing.T, dir, root string, dryRun bool, flags ...string) *link.Report {
	t.Helper()
	var tools []link.Tool
	for _, flag := range flags {
		tools = append(tools, tool(t, flag))
	}
	report, err := link.Run(link.Options{Project: dir, Root: root, Tools: tools, DryRun: dryRun})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return report
}

// tree is every path under dir with what it holds — a file's content, or a
// symlink's target — so that two states of a project can be compared whole.
func tree(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(dir, path)
		if err != nil || relative == "." {
			return err
		}
		switch {
		case entry.Type()&os.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			out = append(out, relative+" -> "+target)
		case entry.IsDir():
			out = append(out, relative+"/")
		default:
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			out = append(out, relative+": "+strings.TrimSpace(string(content)))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(out)
	return out
}

func want(t *testing.T, dir string, paths ...string) {
	t.Helper()
	got := strings.Join(tree(t, dir), "\n")
	expected := strings.Join(paths, "\n")
	if got != expected {
		t.Errorf("the project holds:\n%s\n\nwanted:\n%s", got, expected)
	}
}

// moves renders a tool's adoption the way the report reads, so that a test
// states the whole outcome rather than probing one field of it.
func moves(o link.Outcome) []string {
	var out []string
	for _, m := range o.Adopted {
		switch {
		case m.Deduped:
			out = append(out, "deduped "+m.Name+" (identical to "+m.Taken+")")
		case m.Renamed():
			out = append(out, "renamed "+m.Name+" -> "+m.As+" (taken by "+m.Taken+")")
		default:
			out = append(out, "moved "+m.Name)
		}
	}
	return out
}

func TestLinksAToolAtAProjectThatHasNothingYet(t *testing.T) {
	dir := project(t, nil)

	report := run(t, dir, "", false, "claude")

	if report.Outcomes[0].Action != link.Linked {
		t.Errorf("action = %q, want %q", report.Outcomes[0].Action, link.Linked)
	}
	// The root and its resource directory are the only directories the command
	// creates rather than links.
	want(t, dir,
		".ai/",
		".ai/skills/",
		".claude/",
		".claude/skills -> ../.ai/skills",
	)
}

func TestTheSymlinkIsRelativeSoItSurvivesBeingCommitted(t *testing.T) {
	dir := project(t, nil)

	run(t, dir, "", false, "claude")

	target, err := os.Readlink(filepath.Join(dir, ".claude", "skills"))
	if err != nil {
		t.Fatal(err)
	}
	if filepath.IsAbs(target) {
		t.Errorf("symlink target %q is absolute; it would name a directory that exists on one machine", target)
	}
	// It has to lead somewhere, not merely be relative.
	if _, err := os.Stat(filepath.Join(dir, ".claude", "skills")); err != nil {
		t.Errorf("the symlink resolves to nothing: %v", err)
	}
}

func TestAdoptsWhatEachToolHoldsAndTheFirstFlagKeepsAContestedName(t *testing.T) {
	dir := project(t, map[string]string{
		".claude/settings.json":            "{}",
		".claude/skills/commit/SKILL.md":   "claude's commit",
		".claude/skills/review/SKILL.md":   "one review",
		".opencode/skills/commit/SKILL.md": "opencode's commit",
		".opencode/skills/review/SKILL.md": "one review",
		".opencode/skills/run/SKILL.md":    "run",
	})

	report := run(t, dir, "", false, "claude", "opencode")

	if got := moves(report.Outcomes[0]); strings.Join(got, ", ") != "moved commit, moved review" {
		t.Errorf("claude adopted %v", got)
	}
	if got := strings.Join(moves(report.Outcomes[1]), ", "); got !=
		"renamed commit -> commit-opencode (taken by claude), deduped review (identical to claude), moved run" {
		t.Errorf("opencode adopted %s", got)
	}
	// Everything the tools held is in one root, the emptied directories are
	// symlinks, and what the tool kept beside them is untouched.
	want(t, dir,
		".ai/",
		".ai/skills/",
		".ai/skills/commit-opencode/",
		".ai/skills/commit-opencode/SKILL.md: opencode's commit",
		".ai/skills/commit/",
		".ai/skills/commit/SKILL.md: claude's commit",
		".ai/skills/review/",
		".ai/skills/review/SKILL.md: one review",
		".ai/skills/run/",
		".ai/skills/run/SKILL.md: run",
		".claude/",
		".claude/settings.json: {}",
		".claude/skills -> ../.ai/skills",
		".opencode/",
		".opencode/skills -> ../.ai/skills",
	)
}

func TestAnEntryAlreadyInTheRootOutranksEveryTool(t *testing.T) {
	dir := project(t, map[string]string{
		".ai/skills/commit/SKILL.md":     "the one someone wrote",
		".claude/skills/commit/SKILL.md": "claude's",
	})

	report := run(t, dir, "", false, "claude")

	if got := strings.Join(moves(report.Outcomes[0]), ", "); got != "renamed commit -> commit-claude (taken by "+link.RootHeld+")" {
		t.Errorf("adopted %s", got)
	}
	// Nothing already in the root is moved, renamed or overwritten.
	content, err := os.ReadFile(filepath.Join(dir, ".ai", "skills", "commit", "SKILL.md"))
	if err != nil || string(content) != "the one someone wrote" {
		t.Errorf("the root entry now says %q (%v)", content, err)
	}
}

func TestASuffixedNameThatIsTakenTooGetsANumber(t *testing.T) {
	dir := project(t, map[string]string{
		".ai/skills/commit/SKILL.md":        "first",
		".ai/skills/commit-claude/SKILL.md": "second",
		".claude/skills/commit/SKILL.md":    "third",
	})

	report := run(t, dir, "", false, "claude")

	if got := report.Outcomes[0].Adopted[0].As; got != "commit-claude-2" {
		t.Errorf("named it %q; no entry may be dropped for want of a name", got)
	}
}

func TestAByteIdenticalCopyIsDroppedRatherThanDuplicated(t *testing.T) {
	dir := project(t, map[string]string{
		".ai/skills/commit/SKILL.md":     "the same bytes",
		".claude/skills/commit/SKILL.md": "the same bytes",
	})

	report := run(t, dir, "", false, "claude")

	if !report.Outcomes[0].Adopted[0].Deduped {
		t.Error("kept a byte-identical copy, which loses the point of a single root")
	}
	want(t, dir,
		".ai/",
		".ai/skills/",
		".ai/skills/commit/",
		".ai/skills/commit/SKILL.md: the same bytes",
		".claude/",
		".claude/skills -> ../.ai/skills",
	)
}

func TestContentDecidesWhetherAnEntryIsADuplicate(t *testing.T) {
	// Same name, same file names, one byte apart: this is the comparison that
	// decides whether something may be dropped, so it is exact.
	dir := project(t, map[string]string{
		".ai/skills/commit/SKILL.md":     "a",
		".claude/skills/commit/SKILL.md": "b",
	})

	report := run(t, dir, "", false, "claude")

	if report.Outcomes[0].Adopted[0].Deduped {
		t.Fatal("dropped an entry that was not identical")
	}
	if got := report.Outcomes[0].Adopted[0].As; got != "commit-claude" {
		t.Errorf("named it %q", got)
	}
}

func TestAnExtraFileMakesTwoEntriesDifferent(t *testing.T) {
	dir := project(t, map[string]string{
		".ai/skills/commit/SKILL.md":         "same",
		".claude/skills/commit/SKILL.md":     "same",
		".claude/skills/commit/reference.md": "and one more",
	})

	report := run(t, dir, "", false, "claude")

	if report.Outcomes[0].Adopted[0].Deduped {
		t.Error("dropped an entry holding a file the other one does not")
	}
}

func TestRunningAgainChangesNothing(t *testing.T) {
	dir := project(t, map[string]string{".claude/skills/commit/SKILL.md": "one"})
	run(t, dir, "", false, "claude")
	before := tree(t, dir)

	report := run(t, dir, "", false, "claude")

	if report.Outcomes[0].Action != link.Unchanged {
		t.Errorf("action = %q, want %q: re-running after adding a tool is the expected way to use this",
			report.Outcomes[0].Action, link.Unchanged)
	}
	if strings.Join(tree(t, dir), "\n") != strings.Join(before, "\n") {
		t.Error("the second run changed the project")
	}
}

func TestADryRunReportsTheWholeRunAndWritesNothing(t *testing.T) {
	files := map[string]string{
		".claude/skills/commit/SKILL.md":   "claude's",
		".opencode/skills/commit/SKILL.md": "opencode's",
	}
	dir := project(t, files)
	before := tree(t, dir)

	dry := run(t, dir, "", true, "claude", "opencode")
	if strings.Join(tree(t, dir), "\n") != strings.Join(before, "\n") {
		t.Fatal("a dry run wrote something")
	}

	// It has to report what the real run does, or it is worth less than not
	// having it: the first run in a real project rearranges its resources.
	real := run(t, project(t, files), "", false, "claude", "opencode")
	for i := range dry.Outcomes {
		got, wanted := strings.Join(moves(dry.Outcomes[i]), ", "), strings.Join(moves(real.Outcomes[i]), ", ")
		if got != wanted {
			t.Errorf("%s: dry run said %q, the run did %q", dry.Outcomes[i].Tool.Flag, got, wanted)
		}
	}
}

func TestARefusalFailsOneToolAndLeavesTheOthersLinked(t *testing.T) {
	dir := project(t, map[string]string{".claude/skills": "a file, not a directory"})

	report := run(t, dir, "", false, "claude", "opencode")

	if report.Outcomes[0].Err == nil {
		t.Error("adopted something that is not a directory")
	}
	if report.Outcomes[0].Action != link.Refused {
		t.Errorf("action = %q, want %q", report.Outcomes[0].Action, link.Refused)
	}
	if report.Outcomes[1].Action != link.Linked {
		t.Errorf("opencode = %q; a project scaffolded by hand for one tool still gets the rest",
			report.Outcomes[1].Action)
	}
	if !report.Refused() {
		t.Error("the report does not say anything was refused")
	}
}

func TestASymlinkSomewhereElseIsRefused(t *testing.T) {
	dir := project(t, map[string]string{"elsewhere/commit/SKILL.md": "someone's"})
	if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../elsewhere", filepath.Join(dir, ".claude", "skills")); err != nil {
		t.Fatal(err)
	}

	report := run(t, dir, "", false, "claude")

	if report.Outcomes[0].Err == nil {
		t.Fatal("replaced a symlink something else established deliberately")
	}
	if target, _ := os.Readlink(filepath.Join(dir, ".claude", "skills")); target != "../elsewhere" {
		t.Errorf("the symlink now points at %q", target)
	}
}

func TestTheRootMustBeInsideTheProject(t *testing.T) {
	// The symlinks are relative and meant to be committed, so a root outside
	// the project produces links that resolve to nothing on every other
	// machine — and adoption would move the project's resources out of it.
	for _, root := range []string{"/etc/airfs", "../shared", "..", "../"} {
		dir := project(t, nil)
		_, err := link.Run(link.Options{
			Project: dir,
			Root:    root,
			Tools:   []link.Tool{tool(t, "claude")},
		})
		if err == nil {
			t.Errorf("--root %q was accepted", root)
			continue
		}
		if !airfs.IsPrecondition(err) {
			t.Errorf("--root %q: %v is not reported as a precondition", root, err)
		}
	}
}

func TestARootInsideAToolsOwnPathIsRefused(t *testing.T) {
	// It would adopt into a directory it is about to replace with a symlink to
	// itself. It can only come from --root, and it fails that tool alone.
	dir := project(t, map[string]string{".claude/skills/commit/SKILL.md": "one"})

	report := run(t, dir, ".claude", false, "claude")

	if report.Outcomes[0].Err == nil {
		t.Fatal("linked a tool's directory to itself")
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude", "skills", "commit", "SKILL.md")); err != nil {
		t.Errorf("the refusal cost the project a skill: %v", err)
	}
}

func TestAnAlternativeRootIsUsedAndPointedAt(t *testing.T) {
	dir := project(t, map[string]string{"resources/skills/commit/SKILL.md": "one"})

	report := run(t, dir, "resources", false, "claude")

	if report.Root != "resources" {
		t.Errorf("root = %q", report.Root)
	}
	target, err := os.Readlink(filepath.Join(dir, ".claude", "skills"))
	if err != nil || target != filepath.Join("..", "resources", "skills") {
		t.Errorf("symlink target = %q (%v)", target, err)
	}
}

func TestNamingNoToolIsAnUnsatisfiedPrecondition(t *testing.T) {
	_, err := link.Run(link.Options{Project: project(t, nil)})
	if !airfs.IsPrecondition(err) {
		t.Errorf("err = %v; naming no tool is for the caller to fix", err)
	}
}
