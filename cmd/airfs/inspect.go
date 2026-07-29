package main

import (
	"fmt"
	"strings"

	"github.com/sylvanld/airfs/sdk"
	"github.com/sylvanld/airfs/sdk/config"
)

// cmdList reports the declared inventory, and nothing about what is running.
// It takes no name; a single workspace is what inspect is for.
func cmdList(args []string) error {
	path, rest, err := bind("ls", args, nil)
	if err != nil {
		return err
	}
	if err := none("ls", rest); err != nil {
		return err
	}
	// Resolving a configuration to report it is also what validates it, so this
	// on a broken file reports the same errors a reload would, without the risk
	// of having reloaded.
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}

	fmt.Printf("config  %s\n\n", cfg.Path)
	if len(cfg.Workspaces) == 0 {
		fmt.Println("No workspaces declared. Declare one with: airfs add <name> --target <dir> --source <dir>")
		return nil
	}

	width := 0
	for _, w := range cfg.Workspaces {
		width = max(width, len(w.Name))
	}
	for _, w := range cfg.Workspaces {
		state := "enabled "
		if !w.Enabled {
			state = "disabled"
		}
		fmt.Printf("  %-*s  %s  %s  [%s]  %s\n", width, w.Name, state,
			w.Target.Declared, strings.Join(w.Folders, " "),
			plural(len(w.Sources), "source", "sources"))
	}
	return nil
}

// cmdInspect reports everything about one workspace: what it merges, and what
// that merge shadows.
//
// It reports what the *declaration* produces, not what is currently mounted.
// The two coincide when the workspace is served, and when they do not, the
// difference between this and status is the diagnosis: inspect says what should
// be there, status says what is.
func cmdInspect(args []string) error {
	path, rest, err := bind("inspect", args, nil)
	if err != nil {
		return err
	}
	name, err := one("inspect", rest)
	if err != nil {
		return err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	w := cfg.Lookup(name)
	if w == nil {
		return airfs.Precondition(fmt.Errorf(
			"no workspace named %s in %s; airfs ls lists them", name, cfg.Path))
	}

	fmt.Printf("workspace  %s\n", w.Name)
	fmt.Printf("target     %s\n", w.Target.Declared)
	fmt.Printf("folders    %s\n", strings.Join(w.Folders, ", "))
	if !w.Enabled {
		fmt.Println("enabled    no — it is declared and deliberately not served")
	}
	fmt.Println()

	counts := make(map[string][]int, len(w.Folders))
	for _, folder := range w.Folders {
		if counts[folder], err = w.Counts(folder); err != nil {
			return err
		}
	}

	width := 0
	for _, s := range w.Sources {
		width = max(width, len(s.Declared))
	}
	fmt.Println("Sources, in precedence order — the last declaration wins:")
	for i, s := range w.Sources {
		parts := make([]string, 0, len(w.Folders))
		for _, folder := range w.Folders {
			parts = append(parts, fmt.Sprintf("%s %d", folder, counts[folder][i]))
		}
		fmt.Printf("  %d. %-*s  %s\n", i+1, width, s.Declared, strings.Join(parts, "  "))
	}

	var empty []string
	for _, folder := range w.Folders {
		total := 0
		for _, n := range counts[folder] {
			total += n
		}
		if total == 0 {
			empty = append(empty, folder)
		}
	}
	if len(empty) > 0 {
		// A folder no source contributes to is mounted and empty, not an error.
		fmt.Printf("\nEmpty folders: %s\n", strings.Join(empty, ", "))
	}
	return reportShadowing(w)
}

// reportShadowing names every shadowed entry with its winner and its losers.
//
// This is what makes precedence debuggable rather than mysterious. Shadowing is
// the mechanism working, not a failure, so it never changes the exit code — an
// exit code that punished it would make the normal case indistinguishable from
// a broken configuration.
func reportShadowing(w *config.Workspace) error {
	total := 0
	for _, folder := range w.Folders {
		shadows, err := w.Merged(folder).Shadowed()
		if err != nil {
			return err
		}
		if len(shadows) == 0 {
			continue
		}
		if total == 0 {
			fmt.Println("\nShadowed entries — the winner is what the view serves:")
		}
		total += len(shadows)
		for _, s := range shadows {
			losers := make([]string, 0, len(s.Losers))
			for _, l := range s.Losers {
				losers = append(losers, l.Name)
			}
			fmt.Printf("  %s/%s  wins %s  over %s\n",
				folder, s.Name, s.Winner.Name, strings.Join(losers, ", "))
		}
	}
	if total == 0 {
		fmt.Println("\nNothing is shadowed.")
	}
	return nil
}
