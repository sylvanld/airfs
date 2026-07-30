package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sylvanld/airfs/sdk"
	"github.com/sylvanld/airfs/sdk/link"
)

// cmdLink points each named tool at the project's own resource root. It is the
// one command whose frame of reference is the working directory rather than a
// named workspace, and the one that writes into a project rather than into the
// configuration or the mount table.
func cmdLink(args []string) error {
	fs := flag.NewFlagSet("link", flag.ContinueOnError)
	root := fs.String("root", "", "directory holding the project's own AI resources")
	dryRun := fs.Bool("dry-run", false, "report what would happen and write nothing")
	list := fs.Bool("list", false, "print the table of known tools and exit")
	for _, tool := range link.Tools() {
		fs.Bool(tool.Flag, false, "link "+tool.Flag)
	}
	flags, operands := permute(fs, args)
	if err := fs.Parse(flags); err != nil {
		return err
	}
	if err := none("link", operands); err != nil {
		return err
	}
	if *list {
		printTools(os.Stdout)
		return nil
	}

	tools := named(fs, args)
	if len(tools) == 0 {
		// Guessing which tools a project uses is exactly the kind of inference
		// that produces a directory nobody asked for.
		fmt.Fprintln(os.Stderr, "airfs: link needs at least one tool flag. Known tools:")
		printTools(os.Stderr)
		return airfs.Precondition(airfs.ErrReported)
	}

	project, err := os.Getwd()
	if err != nil {
		return err
	}
	report, err := link.Run(link.Options{
		Project: project,
		Root:    *root,
		Tools:   tools,
		DryRun:  *dryRun,
	})
	if err != nil {
		return err
	}
	printLinked(report, *dryRun)
	if report.Refused() {
		return airfs.Precondition(airfs.ErrReported)
	}
	return nil
}

// named returns the tools whose flags were given, in the order they were given.
//
// That order is what decides who keeps a contested name, and the standard
// parser does not preserve it — a flag set answers what was set, never in what
// sequence. So the command line is read again for the sequence, and the parser
// is still what says whether a flag ended up true: `--claude=false` is not a
// request to link Claude.
func named(fs *flag.FlagSet, args []string) []link.Tool {
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = f.Value.String() == "true" })

	var tools []link.Tool
	seen := map[string]bool{}
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			continue
		}
		name, _, _ := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		tool, known := link.Lookup(name)
		if !known || !set[name] || seen[name] {
			continue
		}
		seen[name] = true
		tools = append(tools, tool)
	}
	return tools
}

func printTools(w *os.File) {
	for _, tool := range link.Tools() {
		fmt.Fprintf(w, "  --%-12s %-8s %s\n", tool.Flag, tool.Type, tool.Path)
	}
}

// printLinked reports every path created, linked or left alone, every entry
// moved, and the root they point into.
//
// Every move is named rather than counted. A summary line is enough for work a
// person expects; adoption moves files they did not ask to have moved, and the
// only acceptable version of that is one whose report can be read against
// `git status` line for line.
func printLinked(report *link.Report, dryRun bool) {
	if dryRun {
		fmt.Println("Dry run — nothing below was written.")
		fmt.Println()
	}

	var types []string
	width := 0
	for _, o := range report.Outcomes {
		if len(o.Tool.Path) > width {
			width = len(o.Tool.Path)
		}
		where := filepath.Join(report.Root, o.Tool.Type)
		if !contains(types, where) {
			types = append(types, where)
		}
	}
	for _, where := range types {
		fmt.Printf("root       %s\n", where)
	}

	for _, o := range report.Outcomes {
		if len(o.Adopted) == 0 {
			continue
		}
		fmt.Println()
		fmt.Printf("adopted    %-*s  %s\n", width, o.Tool.Path, plural(len(o.Adopted), "entry", "entries"))

		// The reason a move needs one is printed in a column of its own, so
		// that the entries stay readable down the left edge.
		labels := make([]string, len(o.Adopted))
		entries := 0
		for i, m := range o.Adopted {
			labels[i] = m.Name
			if m.Renamed() {
				labels[i] = m.Name + " -> " + m.As
			}
			if len(labels[i]) > entries {
				entries = len(labels[i])
			}
		}
		for i, m := range o.Adopted {
			switch {
			case m.Deduped:
				fmt.Printf("  %-10s %-*s  (identical to %s)\n", "deduped", entries, labels[i], holder(m.Taken))
			case m.Renamed():
				fmt.Printf("  %-10s %-*s  (name taken by %s)\n", "renamed", entries, labels[i], held(m.Taken))
			default:
				fmt.Printf("  %-10s %s\n", "moved", labels[i])
			}
		}
	}

	fmt.Println()
	for _, o := range report.Outcomes {
		switch {
		case o.Err != nil:
			fmt.Printf("%-10s %-*s  — %v\n", o.Action, width, o.Tool.Path, o.Err)
		default:
			fmt.Printf("%-10s %-*s  -> %s\n", o.Action, width, o.Tool.Path,
				mustRel(o.Tool.Path, filepath.Join(report.Root, o.Tool.Type)))
		}
	}

	fmt.Println()
	// The two things a person is about to get wrong: the symlink is committable
	// and portable, and the root — not the tool's path — is where resources are
	// written from now on.
	fmt.Println("Relative and safe to commit.")
	for _, where := range types {
		fmt.Printf("Write %s in %s/.\n", filepath.Base(where), where)
	}
	if report.Refused() {
		fmt.Fprintln(os.Stderr, "\nairfs: some tools were refused and are not linked")
	}
}

// held names who took a contested name, the way the command line spells it.
func held(by string) string {
	if by == "" || by == link.RootHeld {
		return "what is already in the root"
	}
	return "--" + by
}

// holder names what a dropped entry was identical to.
func holder(by string) string {
	if by == "" || by == link.RootHeld {
		return "the one already in the root"
	}
	return "what --" + by + " contributed"
}

// mustRel is the symlink as it was written, from the tool's own directory.
func mustRel(from, to string) string {
	relative, err := filepath.Rel(filepath.Dir(from), to)
	if err != nil {
		return to
	}
	return relative
}

func contains(all []string, s string) bool {
	for _, each := range all {
		if each == s {
			return true
		}
	}
	return false
}
