package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/sylvanld/airfs/sdk"
	"github.com/sylvanld/airfs/sdk/config"
	"github.com/sylvanld/airfs/sdk/daemon"
)

// declare is the one operation behind every declarative command: read the
// file, apply the change, validate the result, write it, reload. Each command
// differs only by the mutation in the middle and by what it says it did.
//
// The file is validated before it is written, so a mistyped path leaves the
// existing configuration standing rather than replacing it with something that
// does not resolve.
func declare(path string, mutate func(*config.File) error, said func()) error {
	changed, err := config.Edit(path, mutate)
	if err != nil {
		return err
	}
	said()
	// An edit is reported by its effect, not by its argument: a merge key means
	// the resolved configuration of several workspaces can change, and an alias
	// must never change something silently.
	if len(changed) > 0 {
		fmt.Printf("Changed: %s\n", strings.Join(changed, ", "))
	} else {
		fmt.Println("Nothing changed.")
	}
	return reloadAfterEditing()
}

// reloadAfterEditing tells the daemon to catch up with the file.
//
// A command that finds no daemon has still written the file, and says so
// rather than failing: the declaration is the durable half, and serving it is
// what `up` is for. It exits 2 all the same, because the machine does not match
// the file yet, which is exactly what that code means.
func reloadAfterEditing() error {
	client, err := daemon.Dial()
	if err != nil {
		fmt.Println("\nNo daemon is running, so nothing was mounted or released.")
		fmt.Println("The configuration is written. Serve it with: airfs up")
		return airfs.Precondition(airfs.ErrReported)
	}
	outcomes, err := client.Reload()
	report(outcomes)
	if err != nil {
		return err
	}
	if daemon.Failures(outcomes) {
		return airfs.Precondition(airfs.ErrReported)
	}
	return nil
}

// cmdAdd declares a workspace, or replaces an existing one whole. This is the
// one command that brings a workspace into being without authoring YAML first,
// which is the reason it exists.
func cmdAdd(args []string) error {
	var target string
	var sources, folders repeated
	var disabled bool
	path, rest, err := bind("add", args, func(fs *flag.FlagSet) {
		fs.StringVar(&target, "target", "", "directory the merged view is exposed under")
		fs.Var(&sources, "source", "declare one source, most general first; repeat for each")
		fs.Var(&sources, "s", "shorthand for -source")
		fs.Var(&folders, "folder", "declare one merged subfolder; repeat for each")
		fs.Var(&folders, "f", "shorthand for -folder")
		fs.BoolVar(&disabled, "disabled", false, "declare the workspace without serving it")
	})
	if err != nil {
		return err
	}
	name, err := one("add", rest)
	if err != nil {
		return err
	}
	if target == "" {
		return airfs.Precondition(errors.New("add needs a --target"))
	}
	if len(sources) == 0 {
		return airfs.Precondition(errors.New("add needs at least one --source"))
	}

	// A path is written down as it was typed, so that `~` and `$VAR` survive
	// into the file and it keeps the form its author would recognise.
	written := config.Declaration{Name: name, Folders: folders, Enabled: !disabled}
	if written.Target, err = config.AsDeclared(target); err != nil {
		return err
	}
	for _, s := range sources {
		declared, err := config.AsDeclared(s)
		if err != nil {
			return err
		}
		written.Sources = append(written.Sources, declared)
	}

	return declare(path, func(f *config.File) error { return f.Declare(written) }, func() {
		fmt.Printf("Declared %s in %s:\n", name, path)
		fmt.Printf("  target   %s\n", written.Target)
		if len(written.Folders) > 0 {
			fmt.Printf("  folders  %s\n", strings.Join(written.Folders, ", "))
		} else {
			fmt.Printf("  folders  %s (the default set)\n", strings.Join(config.DefaultFolders, ", "))
		}
		fmt.Println("  sources, in precedence order — the last declaration wins:")
		for i, s := range written.Sources {
			fmt.Printf("    %d. %s\n", i+1, s)
		}
		if disabled {
			fmt.Println("\nDeclared as disabled: nothing is mounted for it until you run airfs enable.")
		}
	})
}

// cmdRemove deletes a workspace's declaration and releases its mounts.
func cmdRemove(args []string) error {
	path, rest, err := bind("rm", args, nil)
	if err != nil {
		return err
	}
	name, err := one("rm", rest)
	if err != nil {
		return err
	}

	var block []byte
	return declare(path, func(f *config.File) error {
		var err error
		block, err = f.Remove(name)
		return err
	}, func() {
		// Printing the block before removing it is what makes an unintended rm
		// recoverable from the terminal's scrollback rather than only from git.
		fmt.Printf("Removed %s from %s. It said:\n\n", name, path)
		for _, line := range strings.Split(strings.TrimRight(string(block), "\n"), "\n") {
			fmt.Printf("    %s\n", line)
		}
		fmt.Println()
		fmt.Println("To keep a workspace but stop serving it, airfs disable loses nothing.")
	})
}

// cmdEnable sets a workspace's enabled field. This is how a workspace stops
// being served, and it is why rm is rare: losing a mount should not require
// losing what produced it.
func cmdEnable(args []string, on bool) error {
	command := "disable"
	if on {
		command = "enable"
	}
	path, rest, err := bind(command, args, nil)
	if err != nil {
		return err
	}
	name, err := one(command, rest)
	if err != nil {
		return err
	}
	return declare(path, func(f *config.File) error { return f.SetEnabled(name, on) }, func() {
		if on {
			fmt.Printf("Enabled %s in %s.\n", name, path)
			return
		}
		fmt.Printf("Disabled %s in %s. Its declaration, comments and place in the file are kept.\n", name, path)
	})
}

// report prints what reconciliation did, one workspace per line.
func report(outcomes []daemon.Outcome) {
	if len(outcomes) == 0 {
		return
	}
	fmt.Println()
	for _, o := range outcomes {
		if o.Workspace == "" {
			// A mount at no wanted target belongs to no workspace.
			fmt.Printf("  %-14s %s\n", o.Action, o.Reason)
			continue
		}
		fmt.Printf("  %-14s %s", o.Action, o.Workspace)
		if o.Reason != "" {
			fmt.Printf(" — %s", o.Reason)
		}
		fmt.Println()
	}
	if daemon.Failures(outcomes) {
		fmt.Fprintln(os.Stderr, "\nairfs: some workspaces are declared and not served")
	}
}
