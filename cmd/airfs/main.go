// Command airfs is the thin frontend over the airfs library: argument parsing,
// human-readable reporting, and exit codes. See docs/specs/cli.md.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sylvanld/airfs/sdk"
	"github.com/sylvanld/airfs/sdk/config"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		usage(os.Stderr)
		return airfs.ExitFailed
	}
	command, rest := args[0], args[1:]

	var err error
	switch command {
	// Declaring: change what is written down, then reload.
	case "add":
		err = cmdAdd(rest)
	case "rm":
		err = cmdRemove(rest)
	case "enable":
		err = cmdEnable(rest, true)
	case "disable":
		err = cmdEnable(rest, false)

	// Inspecting: change nothing, and need nothing running.
	case "ls":
		err = cmdList(rest)
	case "inspect":
		err = cmdInspect(rest)

	// Running: change or report what the daemon is doing, and write nothing.
	case "up":
		err = cmdUp(rest)
	case "down":
		err = cmdDown(rest)
	case "reload":
		err = cmdReload(rest)
	case "status":
		err = cmdStatus(rest)
	case "doctor":
		err = cmdDoctor(rest)

	// Linking: about a project, not about this machine's workspaces. It reads
	// no configuration and needs no daemon.
	case "link":
		err = cmdLink(rest)

	case "help", "-h", "--help":
		usage(os.Stdout)
		return airfs.ExitOK
	default:
		fmt.Fprintf(os.Stderr, "airfs: unknown command %q\n\n", command)
		usage(os.Stderr)
		return airfs.ExitFailed
	}

	if err == nil {
		return airfs.ExitOK
	}
	if !errors.Is(err, airfs.ErrReported) {
		fmt.Fprintf(os.Stderr, "airfs: %v\n", err)
	}
	if airfs.IsPrecondition(err) {
		return airfs.ExitPrecondition
	}
	return airfs.ExitFailed
}

func usage(w *os.File) {
	fmt.Fprint(w, `airfs presents the resources of many repositories as one merged, read-only view.

Usage: airfs <command> [flags]

Declaring — change what is written down, then reload:
  add <name>      Declare a workspace, or replace an existing one whole
  rm <name>       Remove a workspace's declaration and release its mounts
  enable <name>   Serve a declared workspace again
  disable <name>  Stop serving a workspace, keeping its declaration

Inspecting — change nothing, and need nothing running:
  ls              List every declared workspace
  inspect <name>  Report what one workspace merges, and what it shadows

Running — the daemon:
  up              Start the daemon and serve every enabled workspace
  down            Stop it and release every airfs mount on the machine
  reload          Re-read the configuration and reconcile
  status [name]   Report the daemon's state and what is mounted
  doctor          Check the host's mount prerequisites

Linking — about the project you are standing in, not about workspaces:
  link --<tool>   Point each named tool at the project's own resources, and
                  adopt what it already holds. --list prints the known tools.
                  A symlink it writes is not reconciled by anything: no command
                  will report it, and none will offer to remove it.

Flags, accepted by every command but link:
  --config file   The configuration to read (default $XDG_CONFIG_HOME/airfs/config.yaml)

Flags, accepted by add only:
  --target dir    Where the merged view is exposed
  -s, --source    Declare one source, most general first; repeat for each
  -f, --folder    Declare one merged subfolder; repeat for each
  --disabled      Declare it without serving it

Flags, accepted by up only:
  --detach        Return once the workspaces are established instead of blocking

Flags, accepted by link only:
  --root dir      Where the project keeps its own resources (default .ai)
  --dry-run       Report the moves and links, and write nothing
  --list          Print the table of known tools and exit

Exit codes: 0 success, 2 unsatisfied precondition, 1 anything else.
`)
}

// bind registers --config, which every command takes, and parses args.
//
// A relative path given here resolves against the working directory, which is
// the obvious frame of reference for something typed on a command line. Paths
// *inside* the file are unaffected: they resolve against the directory holding
// that file, so a configuration means the same thing wherever it is read from.
func bind(name string, args []string, extra func(*flag.FlagSet)) (string, []string, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	path := fs.String("config", "", "the configuration to read")
	if extra != nil {
		extra(fs)
	}
	flags, operands := permute(fs, args)
	if err := fs.Parse(flags); err != nil {
		return "", nil, err
	}
	if *path == "" {
		defaulted, err := config.DefaultPath()
		if err != nil {
			return "", nil, err
		}
		return defaulted, operands, nil
	}
	absolute, err := expandOnCommandLine(*path)
	if err != nil {
		return "", nil, err
	}
	return absolute, operands, nil
}

// permute separates flag tokens from operands, so that `airfs add personal
// --target dir` means what it looks like it means.
//
// The standard parser stops at the first operand and treats everything after
// it as one too, which would make every command here take its flags only
// before its workspace name. Deciding whether a flag consumes the token after
// it needs the flag set itself: only that knows which flags are booleans.
func permute(fs *flag.FlagSet, args []string) (flags, operands []string) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			// Everything after the terminator is an operand, including
			// something that looks like a flag.
			return flags, append(operands, args[i+1:]...)
		}
		if len(arg) < 2 || !strings.HasPrefix(arg, "-") {
			operands = append(operands, arg)
			continue
		}
		flags = append(flags, arg)
		name := strings.TrimLeft(arg, "-")
		if strings.Contains(name, "=") {
			// The value came attached; nothing else belongs to this flag.
			continue
		}
		// An unknown flag is left to the parser to reject, which it does with a
		// better message than anything this could invent.
		defined := fs.Lookup(name)
		if defined == nil || isBoolean(defined.Value) {
			continue
		}
		if i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}
	return flags, operands
}

// isBoolean reports whether a flag stands alone, by the same interface the
// standard parser uses to ask.
func isBoolean(value flag.Value) bool {
	boolean, ok := value.(interface{ IsBoolFlag() bool })
	return ok && boolean.IsBoolFlag()
}

func expandOnCommandLine(path string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~"))
	}
	return filepath.Abs(path)
}

// one takes the single workspace name a command requires.
func one(command string, args []string) (string, error) {
	switch len(args) {
	case 1:
		return args[0], nil
	case 0:
		return "", airfs.Precondition(fmt.Errorf("%s needs a workspace name", command))
	default:
		return "", airfs.Precondition(fmt.Errorf(
			"%s takes one workspace name, got %d; it edits one workspace at a time", command, len(args)))
	}
}

// none rejects arguments a command does not take.
func none(command string, args []string) error {
	if len(args) > 0 {
		return airfs.Precondition(fmt.Errorf("%s takes no arguments, got %q", command, args[0]))
	}
	return nil
}

// A repeated flag collects a value each time it is given. The order the flags
// were given in is kept, because for sources that order is the precedence
// order.
type repeated []string

func (r *repeated) String() string { return strings.Join(*r, " ") }

func (r *repeated) Set(value string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("cannot be empty")
	}
	*r = append(*r, value)
	return nil
}

// plural picks the right noun for n, since a report a person reads should not
// say "1 entries".
func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}
