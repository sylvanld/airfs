// Command airfs is the thin frontend over the airfs library: argument parsing,
// human-readable reporting, and exit codes. See docs/specs/cli.md.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/sylvanld/airfs/sdk"
	"github.com/sylvanld/airfs/sdk/mount"
	"github.com/sylvanld/airfs/sdk/sources"
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
	case "sources":
		err = cmdSources(rest)
	case "mount":
		err = cmdMount(rest)
	case "umount":
		err = cmdUmount(rest)
	case "status":
		err = cmdStatus(rest)
	case "doctor":
		err = cmdDoctor(rest)
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

Commands:
  sources   Resolve the source list and report it, with every shadowed entry
  mount     Serve the merged view under the target
  umount    Release the target's mounts
  status    Report whether the target is being served
  doctor    Check the host's mount prerequisites

Flags, accepted by every command:
  --target dir   Where the view is exposed (default $HOME/.ai-resources)
  --config file  The source list to read (default <target>/sources.txt)

Exit codes: 0 success, 2 unsatisfied precondition, 1 anything else.
`)
}

// paths carries the two overrides every command takes.
type paths struct {
	target string
	config string
}

// bind registers the common flags and parses args. A relative path given on the
// command line resolves against the working directory, which is the obvious
// frame of reference for something typed there.
func bind(name string, args []string, extra func(*flag.FlagSet)) (paths, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	target := fs.String("target", "", "directory the view is exposed under (default $HOME/.ai-resources)")
	config := fs.String("config", "", "source list to read (default <target>/sources.txt)")
	if extra != nil {
		extra(fs)
	}
	if err := fs.Parse(args); err != nil {
		return paths{}, err
	}
	if fs.NArg() > 0 {
		return paths{}, fmt.Errorf("%s takes no arguments, got %q", name, fs.Arg(0))
	}

	p := paths{target: *target, config: *config}
	if p.target == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return paths{}, err
		}
		p.target = filepath.Join(home, ".ai-resources")
	}
	var err error
	if p.target, err = absolute(p.target); err != nil {
		return paths{}, err
	}
	if p.config == "" {
		p.config = filepath.Join(p.target, sources.FileName)
	} else if p.config, err = absolute(p.config); err != nil {
		return paths{}, err
	}
	return p, nil
}

func absolute(path string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~"))
	}
	return filepath.Abs(path)
}

// load resolves the configuration, replacing the bare open error with an
// instruction when the file is simply absent — the first thing every new user
// hits. The check is on the file itself rather than on the error, because a
// missing *source* fails with the same os.ErrNotExist and means something else
// entirely.
func (p paths) load() (*sources.Config, error) {
	if _, err := os.Stat(p.config); errors.Is(err, os.ErrNotExist) {
		return nil, airfs.Precondition(fmt.Errorf(
			"no source list at %s; create it with one source path per line", p.config))
	}
	return sources.Load(p.config)
}

// cmdSources reports the resolved configuration and the shadowing it produces.
// This is what answers "is my repository being layered, and where in the
// order?" — the question precedence makes inevitable.
func cmdSources(args []string) error {
	p, err := bind("sources", args, nil)
	if err != nil {
		return err
	}
	cfg, err := p.load()
	if err != nil {
		return err
	}

	fmt.Printf("target  %s\nconfig  %s\n\n", p.target, cfg.Path)
	if len(cfg.Sources) == 0 {
		fmt.Println("No sources declared.")
		return nil
	}

	counts := make(map[airfs.Kind][]int, len(airfs.Kinds))
	for _, kind := range airfs.Kinds {
		if counts[kind], err = cfg.Counts(kind); err != nil {
			return err
		}
	}

	fmt.Println("Sources, in precedence order — the last declaration wins:")
	width := 0
	for _, s := range cfg.Sources {
		width = max(width, len(s.Declared))
	}
	for i, s := range cfg.Sources {
		parts := make([]string, 0, len(airfs.Kinds))
		for _, kind := range airfs.Kinds {
			parts = append(parts, fmt.Sprintf("%s %d", kind, counts[kind][i]))
		}
		fmt.Printf("  %d. %-*s  %s\n", i+1, width, s.Declared, strings.Join(parts, "  "))
	}

	var empty []string
	for _, kind := range airfs.Kinds {
		total := 0
		for _, n := range counts[kind] {
			total += n
		}
		if total == 0 {
			empty = append(empty, kind.String())
		}
	}
	if len(empty) > 0 {
		fmt.Printf("\nEmpty kinds: %s\n", strings.Join(empty, ", "))
	}

	return reportShadowing(cfg)
}

// reportShadowing names every shadowed entry with its winner and losers.
// Shadowing is the mechanism working, not a failure, so it never changes the
// exit code — it is reported so that it is never silent.
func reportShadowing(cfg *sources.Config) error {
	total := 0
	for _, kind := range airfs.Kinds {
		shadows, err := cfg.Merged(kind).Shadowed()
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
				kind, s.Name, s.Winner.Name, strings.Join(losers, ", "))
		}
	}
	if total == 0 {
		fmt.Println("\nNothing is shadowed.")
	}
	return nil
}

func cmdMount(args []string) error {
	var detach bool
	p, err := bind("mount", args, func(fs *flag.FlagSet) {
		fs.BoolVar(&detach, "detach", false, "return once the view is ready instead of blocking")
	})
	if err != nil {
		return err
	}
	cfg, err := p.load()
	if err != nil {
		return err
	}

	// The detached child re-runs this command with the same flags, so it must
	// not detach again. Only the caller reports; the child serves silently.
	if detach && !mount.Detached() {
		if err := mount.Detach(p.target); err != nil {
			return err
		}
		reportServing(p.target, cfg)
		fmt.Printf("\nServing in the background. Stop it with: airfs umount --target %s\n", p.target)
		return nil
	}

	server, err := mount.Serve(p.target, cfg)
	if err != nil {
		return err
	}
	if !mount.Detached() {
		reportServing(p.target, cfg)
		for _, kind := range server.Kinds() {
			fmt.Printf("  mounted %s\n", mount.KindDir(p.target, kind))
		}
	}

	// Interrupting unmounts cleanly; termination that does not allow cleanup is
	// the stale-mountpoint case, which umount recovers.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stop
		fmt.Fprintln(os.Stderr, "\nairfs: unmounting")
		if err := server.Unmount(); err != nil {
			fmt.Fprintf(os.Stderr, "airfs: %v\n", err)
		}
	}()
	server.Wait()
	return nil
}

// reportServing names the layers behind the view, in precedence order, so that
// what is being served is visible without a second command.
func reportServing(target string, cfg *sources.Config) {
	fmt.Printf("Serving %s from %s:\n", target, plural(len(cfg.Sources), "source", "sources"))
	for i, s := range cfg.Sources {
		fmt.Printf("  %d. %s\n", i+1, s.Declared)
	}
}

func cmdUmount(args []string) error {
	p, err := bind("umount", args, nil)
	if err != nil {
		return err
	}
	released, err := mount.Unmount(p.target)
	if err != nil {
		return err
	}
	if len(released) == 0 {
		fmt.Printf("Nothing was mounted under %s.\n", p.target)
		return nil
	}
	for _, kind := range released {
		fmt.Printf("Released %s\n", mount.KindDir(p.target, kind))
	}
	return nil
}

// cmdStatus reports which state the target is in, and says so through the exit
// code too: a stale mountpoint looks mounted and serves nothing, and a
// contributor who cannot tell them apart cannot act.
func cmdStatus(args []string) error {
	p, err := bind("status", args, nil)
	if err != nil {
		return err
	}
	states, err := mount.Status(p.target)
	if err != nil {
		return err
	}

	fmt.Printf("target  %s\n\n", p.target)
	for _, st := range states {
		switch {
		case st.Stale:
			fmt.Printf("  %-9s stale — the serving process died; recover with airfs umount\n", st.Kind)
		case st.Mounted:
			entries, err := os.ReadDir(st.Dir)
			if err != nil {
				return err
			}
			fmt.Printf("  %-9s served, %s\n", st.Kind, plural(len(entries), "entry", "entries"))
		default:
			fmt.Printf("  %-9s not mounted\n", st.Kind)
		}
	}
	if mount.Served(states) {
		return nil
	}
	return airfs.Precondition(errors.New("the target is not fully served"))
}

// cmdDoctor reports every prerequisite either way, since the second missing one
// is worth knowing before installing the first.
func cmdDoctor(args []string) error {
	if _, err := bind("doctor", args, nil); err != nil {
		return err
	}
	ok := true
	for _, r := range mount.Requirements() {
		mark := "ok  "
		if !r.Satisfied {
			mark, ok = "MISSING", false
		}
		fmt.Printf("  %-8s %-14s %s\n", mark, r.Name, r.Detail)
		if !r.Satisfied {
			fmt.Printf("  %-8s %-14s provided by %s\n", "", "", r.ProvidedBy)
		}
	}
	if ok {
		fmt.Println("\nEvery mount prerequisite is satisfied.")
		return nil
	}
	// Installing needs root, and a tool that asks for root to install a system
	// package is a tool that should have printed the command instead.
	return airfs.Precondition(errors.New("a mount prerequisite is missing; install what provides it, then run airfs doctor again"))
}

// plural picks the right noun for n, since a report a person reads should not
// say "1 entries".
func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}
