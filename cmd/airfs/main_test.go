package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sylvanld/airfs/sdk"
	"github.com/sylvanld/airfs/sdk/config"
)

// invoke runs a command as the shell would and returns what it printed and the
// code it would exit with.
//
// Every test here points --config at a temporary file and XDG_RUNTIME_DIR at a
// temporary directory, so that no daemon is reachable and the real
// configuration is never read. What is left is the frontend's own job:
// argument parsing, reporting, and exit codes.
func invoke(t *testing.T, args ...string) (string, int) {
	t.Helper()
	stdout, stderr := os.Stdout, os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout, os.Stderr = w, w

	done := make(chan string, 1)
	go func() {
		out, _ := io.ReadAll(r)
		done <- string(out)
	}()

	code := run(args)
	w.Close()
	os.Stdout, os.Stderr = stdout, stderr
	return <-done, code
}

// workspace prepares a machine with a configuration path and two source
// repositories, and returns the directory holding them.
func workspace(t *testing.T) (dir, configPath string) {
	t.Helper()
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	dir = t.TempDir()
	for _, name := range []string{"global", "project"} {
		skills := filepath.Join(dir, name, "skills")
		if err := os.MkdirAll(skills, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir, filepath.Join(dir, config.FileName)
}

func TestAddDeclaresAWorkspaceAndSaysTheFileIsTheDurableHalf(t *testing.T) {
	dir, path := workspace(t)
	out, code := invoke(t, "add", "personal",
		"--config", path,
		"--target", filepath.Join(dir, "target"),
		"-s", filepath.Join(dir, "global"),
		"-s", filepath.Join(dir, "project"),
		"-f", "skills")

	// The declaration is the durable half; serving it is what up is for. The
	// machine does not match the file yet, which is exactly what code 2 means.
	if code != airfs.ExitPrecondition {
		t.Errorf("exit code = %d, want %d when there is no daemon to reload", code, airfs.ExitPrecondition)
	}
	if !strings.Contains(out, "airfs up") {
		t.Errorf("a command that wrote the file with no daemon must say what to do next:\n%s", out)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	w := cfg.Lookup("personal")
	if w == nil {
		t.Fatal("the workspace was not written")
	}
	if len(w.Sources) != 2 || w.Sources[0].Declared != filepath.Join(dir, "global") {
		t.Errorf("sources = %v, in the order they were given", w.Sources)
	}
	if strings.Join(w.Folders, ",") != "skills" {
		t.Errorf("folders = %v", w.Folders)
	}
	if !w.Enabled {
		t.Error("a workspace is enabled unless --disabled is given")
	}
}

func TestAddRequiresWhatAWorkspaceCannotDoWithout(t *testing.T) {
	dir, path := workspace(t)
	for name, args := range map[string][]string{
		"no name":   {"add", "--config", path, "--target", dir, "-s", dir},
		"no target": {"add", "personal", "--config", path, "-s", dir},
		"no source": {"add", "personal", "--config", path, "--target", dir},
		"two names": {"add", "one", "two", "--config", path, "--target", dir, "-s", dir},
	} {
		t.Run(name, func(t *testing.T) {
			out, code := invoke(t, args...)
			if code != airfs.ExitPrecondition {
				t.Errorf("exit code = %d, want %d\n%s", code, airfs.ExitPrecondition, out)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Error("a refused command wrote the configuration")
			}
		})
	}
}

func TestDisableKeepsTheDeclaration(t *testing.T) {
	dir, path := workspace(t)
	invoke(t, "add", "personal", "--config", path,
		"--target", filepath.Join(dir, "target"), "-s", filepath.Join(dir, "global"))

	out, _ := invoke(t, "disable", "personal", "--config", path)
	if !strings.Contains(out, "Disabled personal") {
		t.Errorf("output does not report the change:\n%s", out)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	// Losing a mount should not require losing what produced it.
	w := cfg.Lookup("personal")
	if w == nil {
		t.Fatal("disabling removed the declaration")
	}
	if w.Enabled {
		t.Error("the workspace is still enabled")
	}

	out, _ = invoke(t, "enable", "personal", "--config", path)
	if !strings.Contains(out, "Enabled personal") {
		t.Errorf("output does not report the change:\n%s", out)
	}
	cfg, _ = config.Load(path)
	if !cfg.Lookup("personal").Enabled {
		t.Error("enabling did not take")
	}
}

func TestRemovePrintsTheBlockItRemoved(t *testing.T) {
	dir, path := workspace(t)
	invoke(t, "add", "personal", "--config", path,
		"--target", filepath.Join(dir, "target"), "-s", filepath.Join(dir, "global"))

	out, _ := invoke(t, "rm", "personal", "--config", path)
	// An unintended rm has to be recoverable from the terminal's scrollback.
	for _, want := range []string{"personal", filepath.Join(dir, "global"), "airfs disable"} {
		if !strings.Contains(out, want) {
			t.Errorf("the removal report is missing %q:\n%s", want, out)
		}
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Lookup("personal") != nil {
		t.Error("the workspace is still declared")
	}
}

func TestListReportsTheDeclaredInventory(t *testing.T) {
	dir, path := workspace(t)
	invoke(t, "add", "personal", "--config", path,
		"--target", filepath.Join(dir, "personal"), "-s", filepath.Join(dir, "global"), "-f", "skills")
	invoke(t, "add", "work", "--config", path, "--disabled",
		"--target", filepath.Join(dir, "work"), "-s", filepath.Join(dir, "project"), "-f", "prompts")

	out, code := invoke(t, "ls", "--config", path)
	if code != airfs.ExitOK {
		t.Errorf("exit code = %d\n%s", code, out)
	}
	for _, want := range []string{"personal", "enabled", "work", "disabled", "skills", "prompts"} {
		if !strings.Contains(out, want) {
			t.Errorf("the listing is missing %q:\n%s", want, out)
		}
	}
	// It takes no name; a single workspace is what inspect is for.
	if _, code := invoke(t, "ls", "personal", "--config", path); code == airfs.ExitOK {
		t.Error("ls accepted a workspace name")
	}
}

func TestInspectReportsShadowingWithoutFailing(t *testing.T) {
	dir, path := workspace(t)
	for _, source := range []string{"global", "project"} {
		if err := os.WriteFile(filepath.Join(dir, source, "skills", "commit.md"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	invoke(t, "add", "personal", "--config", path,
		"--target", filepath.Join(dir, "target"),
		"-s", filepath.Join(dir, "global"), "-s", filepath.Join(dir, "project"), "-f", "skills")

	out, code := invoke(t, "inspect", "personal", "--config", path)
	// Shadowing is the mechanism working, not a failure. An exit code that
	// punished it would make the normal case indistinguishable from a broken
	// configuration.
	if code != airfs.ExitOK {
		t.Errorf("exit code = %d, want 0 with shadowing reported\n%s", code, out)
	}
	if !strings.Contains(out, "commit.md") || !strings.Contains(out, "wins") {
		t.Errorf("the shadowing report is missing:\n%s", out)
	}
	if !strings.Contains(out, filepath.Join(dir, "project")) {
		t.Errorf("the report must name the winner as the author declared it:\n%s", out)
	}
}

func TestInspectAnUndeclaredWorkspaceSaysWhereToLook(t *testing.T) {
	dir, path := workspace(t)
	invoke(t, "add", "personal", "--config", path,
		"--target", filepath.Join(dir, "target"), "-s", filepath.Join(dir, "global"))

	out, code := invoke(t, "inspect", "nope", "--config", path)
	if code != airfs.ExitPrecondition {
		t.Errorf("exit code = %d, want %d\n%s", code, airfs.ExitPrecondition, out)
	}
	if !strings.Contains(out, "airfs ls") {
		t.Errorf("the error must say how to find the right name:\n%s", out)
	}
}

func TestAMissingConfigurationSaysHowToStart(t *testing.T) {
	_, path := workspace(t)
	out, code := invoke(t, "ls", "--config", path)
	if code != airfs.ExitPrecondition {
		t.Errorf("exit code = %d, want %d", code, airfs.ExitPrecondition)
	}
	if !strings.Contains(out, "airfs add") {
		t.Errorf("the first thing a new user hits must say what to do next:\n%s", out)
	}
}

func TestAnInvalidConfigurationIsReportedByTheCommandsThatReadIt(t *testing.T) {
	_, path := workspace(t)
	if err := os.WriteFile(path, []byte("workspaces:\n  a:\n    target: /t\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Resolving a configuration to report it is also what validates it, so
	// either inspecting command on a broken file reports what a reload would.
	for _, command := range []string{"ls", "inspect"} {
		args := []string{command, "--config", path}
		if command == "inspect" {
			args = []string{command, "a", "--config", path}
		}
		out, code := invoke(t, args...)
		if code != airfs.ExitPrecondition {
			t.Errorf("%s exit code = %d, want %d\n%s", command, code, airfs.ExitPrecondition, out)
		}
		if !strings.Contains(out, "no sources") {
			t.Errorf("%s does not report the problem:\n%s", command, out)
		}
	}
}

func TestStatusWithNoDaemonReportsThatAndSaysWhatItWouldRead(t *testing.T) {
	_, path := workspace(t)
	out, code := invoke(t, "status", "--config", path)
	// A dead daemon is a fact to report rather than an error that prevents
	// reporting.
	if code != airfs.ExitPrecondition {
		t.Errorf("exit code = %d, want %d", code, airfs.ExitPrecondition)
	}
	if !strings.Contains(out, "not running") {
		t.Errorf("status must say a daemon is not running:\n%s", out)
	}
	if !strings.Contains(out, path) {
		t.Errorf("status must name the configuration in play:\n%s", out)
	}
	if !strings.Contains(out, "airfs up") {
		t.Errorf("status must say how to start one:\n%s", out)
	}
}

func TestCommandsThatNeedADaemonSayWhenThereIsNone(t *testing.T) {
	_, path := workspace(t)
	for _, command := range []string{"reload"} {
		out, code := invoke(t, command, "--config", path)
		if code != airfs.ExitPrecondition {
			t.Errorf("%s exit code = %d, want %d\n%s", command, code, airfs.ExitPrecondition, out)
		}
		if !strings.Contains(out, "airfs up") {
			t.Errorf("%s must say how to start a daemon:\n%s", command, out)
		}
	}
}

func TestUnknownCommandIsNotAPrecondition(t *testing.T) {
	// A typo is not the host or the configuration needing attention.
	if _, code := invoke(t, "mount"); code != airfs.ExitFailed {
		t.Errorf("exit code = %d, want %d", code, airfs.ExitFailed)
	}
}

func TestHelpListsEveryCommand(t *testing.T) {
	out, code := invoke(t, "help")
	if code != airfs.ExitOK {
		t.Errorf("exit code = %d", code)
	}
	for _, command := range []string{"add", "rm", "enable", "disable", "ls", "inspect", "up", "down", "reload", "status", "doctor"} {
		if !strings.Contains(out, command) {
			t.Errorf("help does not mention %q", command)
		}
	}
}
