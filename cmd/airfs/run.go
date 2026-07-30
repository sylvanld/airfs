package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sylvanld/airfs/sdk"
	"github.com/sylvanld/airfs/sdk/daemon"
	"github.com/sylvanld/airfs/sdk/mount"
)

// cmdUp starts the daemon: resolve the configuration, reconcile every enabled
// workspace, and serve.
func cmdUp(args []string) error {
	var detach bool
	path, rest, err := bind("up", args, func(fs *flag.FlagSet) {
		fs.BoolVar(&detach, "detach", false, "return once the workspaces are established instead of blocking")
	})
	if err != nil {
		return err
	}
	if err := none("up", rest); err != nil {
		return err
	}

	// The detached child re-runs this command with the same flags, so it must
	// not detach again. Only the caller reports; the child serves silently.
	if detach && !daemon.Detached() {
		if err := daemon.Detach(); err != nil {
			return err
		}
		client, err := daemon.Dial()
		if err != nil {
			return err
		}
		status, err := client.Status()
		if err != nil {
			return err
		}
		reportStatus(status, path)
		mounted, err := reportMounts(status, "")
		if err != nil {
			return err
		}
		fmt.Println("\nServing in the background. Stop it with: airfs down")
		if !status.Served() || !mounted {
			return airfs.Precondition(airfs.ErrReported)
		}
		return nil
	}

	d, outcomes, err := daemon.Start(path)
	if err != nil {
		return err
	}
	if !daemon.Detached() {
		fmt.Printf("config  %s\n", path)
		report(outcomes)
		fmt.Println("\nServing. Stop with Ctrl-C, or from another terminal: airfs down")
	}
	if daemon.Failures(outcomes) && daemon.Detached() {
		// The parent reads this code to learn the child could not serve
		// everything, without the child having to report it twice.
		d.Stop()
		return airfs.Precondition(airfs.ErrReported)
	}
	return d.Wait()
}

// cmdDown stops the daemon and releases every airfs mount on the machine,
// including those a previous daemon left behind and those whose serving process
// died.
//
// It works with no daemon running, which is what makes it the single recovery
// command: "release what should not be mounted" is one operation regardless of
// what is alive.
func cmdDown(args []string) error {
	_, rest, err := bind("down", args, nil)
	if err != nil {
		return err
	}
	if err := none("down", rest); err != nil {
		return err
	}

	if client, err := daemon.Dial(); err == nil {
		outcomes, err := client.Shutdown()
		if err != nil {
			return err
		}
		fmt.Println("Stopped the daemon.")
		report(outcomes)
		// Stopping releases what the daemon held; anything else on the machine
		// is released below.
		if err := waitForRelease(); err != nil {
			return err
		}
	}

	mounts, err := mount.Mounts()
	if err != nil {
		return err
	}
	if len(mounts) == 0 {
		fmt.Println("Nothing is mounted.")
		return nil
	}
	released, err := mount.UnmountAll(mounts)
	for _, dir := range released {
		fmt.Printf("Released %s\n", dir)
	}
	return err
}

// waitForRelease gives a stopping daemon a moment to let go of its mounts, so
// that what is reported next is what is really left rather than what was on its
// way out.
func waitForRelease() error {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		mounts, err := mount.Mounts()
		if err != nil || len(mounts) == 0 {
			return err
		}
		time.Sleep(20 * time.Millisecond)
	}
	return nil
}

// cmdReload tells the running daemon to re-read the configuration and
// reconcile. It exists for a file edited by hand; the declarative commands
// reload on their own.
func cmdReload(args []string) error {
	_, rest, err := bind("reload", args, nil)
	if err != nil {
		return err
	}
	if err := none("reload", rest); err != nil {
		return err
	}
	client, err := daemon.Dial()
	if err != nil {
		return err
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

// cmdStatus reports the daemon's state.
func cmdStatus(args []string) error {
	path, rest, err := bind("status", args, nil)
	if err != nil {
		return err
	}
	var only string
	if len(rest) > 0 {
		if only, err = one("status", rest); err != nil {
			return err
		}
	}

	var status *daemon.Status
	if client, err := daemon.Dial(); err == nil {
		if status, err = client.Status(); err != nil {
			return err
		}
	}
	served := reportStatus(status, path)
	mounted, err := reportMounts(status, only)
	if err != nil {
		return err
	}
	if served && mounted {
		return nil
	}
	// A disabled workspace serving nothing is the configuration being honoured,
	// so it does not reach here; what does is a condition to act on.
	return airfs.Precondition(errors.New("some enabled workspaces are not served"))
}

// reportStatus prints the first three points of the report and says whether
// every enabled workspace is served.
//
// Points 1 and 2 are about the daemon; point 3 is about the file the daemon
// loaded, which is not necessarily the file this invocation would read.
func reportStatus(status *daemon.Status, wouldRead string) bool {
	if status == nil {
		fmt.Println("daemon  not running")
		fmt.Printf("config  %s (what this command would read; no daemon has loaded it)\n", wouldRead)
		fmt.Println("\nStart one with: airfs up")
		return false
	}

	fmt.Printf("daemon  running since %s\n", status.StartedAt.Format(time.RFC1123))
	fmt.Printf("config  %s\n", status.ConfigPath)
	if !status.ReloadedAt.Equal(status.StartedAt) {
		fmt.Printf("        last reloaded %s\n", status.ReloadedAt.Format(time.RFC1123))
	}
	// A daemon started with an explicit configuration holds that file for its
	// whole life, so the file being edited and the file being served can differ
	// — and every other symptom of that looks like airfs ignoring an edit. It is
	// said as its own line rather than left to be inferred from a path printed
	// in passing.
	if status.ConfigPath != wouldRead {
		fmt.Printf("\n! The daemon is serving %s, but this command reads %s.\n",
			status.ConfigPath, wouldRead)
		fmt.Println("  Edits to the second will not take effect until the daemon is restarted against it.")
	}

	fmt.Println()
	if len(status.Workspaces) == 0 {
		fmt.Println("It declares no workspaces.")
		return true
	}
	width := 0
	for _, w := range status.Workspaces {
		width = max(width, len(w.Name))
	}
	for _, w := range status.Workspaces {
		switch {
		case !w.Enabled:
			// The configuration being honoured, not a failure.
			fmt.Printf("  %-*s  disabled\n", width, w.Name)
		case w.Established:
			fmt.Printf("  %-*s  served     %s\n", width, w.Name, w.Target)
		default:
			reason := w.Reason
			if reason == "" {
				reason = "not established"
			}
			fmt.Printf("  %-*s  NOT SERVED %s — %s\n", width, w.Name, w.Target, reason)
		}
	}
	return status.Served()
}

// reportMounts prints the fourth point: what is mounted, from the kernel. It
// also says whether every folder of every established workspace is there and
// live, which is the exit code's other half.
//
// That is a different question from the one the daemon answers. The daemon
// reports what it established; the kernel reports what is mounted now, and the
// two part company the moment something releases a mount from under a running
// daemon — leaving it saying "served" about a directory the kernel no longer
// carries. Only the kernel can catch that, which is why the code turns on it.
//
// It is answerable with no daemon alive, which is the state that most needs
// reporting: status on a dead daemon still reports every airfs mount left on
// the machine.
func reportMounts(status *daemon.Status, only string) (bool, error) {
	mounts, err := mount.Mounts()
	if err != nil {
		return false, err
	}
	live := map[string]mount.Mount{}
	for _, m := range mounts {
		live[m.Dir] = m
	}
	fmt.Println()

	// Mounts are grouped under the workspace whose target holds them, so that
	// what the kernel reports lines up with what the file declares. One that
	// belongs to no declared workspace is named as such rather than omitted.
	complete, reported := true, false
	accounted := map[string]bool{}
	if status != nil {
		for _, w := range status.Workspaces {
			if only != "" && w.Name != only {
				continue
			}
			var lines []string
			for _, folder := range w.Folders {
				// Matched against the resolved target: the kernel reports
				// absolute paths, and a target declared as `~/x` is not one.
				dir := filepath.Join(strings.TrimSuffix(w.TargetDir, "/"), folder)
				m, ok := live[dir]
				if !ok {
					// A folder of an established workspace that the kernel does
					// not carry is the state most worth naming: every other
					// report on the machine still calls the workspace served.
					if w.Established {
						complete = false
						lines = append(lines, dir+"  NOT MOUNTED — recover with airfs down, then airfs up")
					}
					continue
				}
				accounted[dir] = true
				if w.Established && m.Stale {
					complete = false
				}
				lines = append(lines, describe(m))
			}
			// Anything else the kernel carries under this target belongs to the
			// workspace too — a folder it no longer declares, most likely — and
			// is named here rather than counted a stray.
			for _, m := range mounts {
				if accounted[m.Dir] || !strings.HasPrefix(m.Dir, strings.TrimSuffix(w.TargetDir, "/")+"/") {
					continue
				}
				accounted[m.Dir] = true
				lines = append(lines, describe(m))
			}
			if len(lines) == 0 {
				continue
			}
			reported = true
			fmt.Printf("Mounted for %s:\n", w.Name)
			for _, line := range lines {
				fmt.Printf("  %s\n", line)
			}
		}
	}
	if only != "" {
		if !reported {
			fmt.Println("Nothing is mounted.")
		}
		return complete, nil
	}

	var stray []mount.Mount
	for _, m := range mounts {
		if !accounted[m.Dir] {
			stray = append(stray, m)
		}
	}
	if len(stray) > 0 {
		reported = true
		fmt.Println("Mounted, belonging to no declared workspace:")
		for _, m := range stray {
			fmt.Printf("  %s\n", describe(m))
		}
		fmt.Println("\nRelease these with: airfs down")
	}
	if !reported {
		fmt.Println("Nothing is mounted.")
	}
	return complete, nil
}

// describe names one mount and its condition. A stale mountpoint looks mounted
// and serves nothing, and a person who cannot tell them apart cannot act.
func describe(m mount.Mount) string {
	if m.Stale {
		return m.Dir + "  STALE — its serving process died; recover with airfs down"
	}
	entries, err := os.ReadDir(m.Dir)
	if err != nil {
		return fmt.Sprintf("%s  unreadable: %v", m.Dir, err)
	}
	return fmt.Sprintf("%s  %s", m.Dir, plural(len(entries), "entry", "entries"))
}

// cmdDoctor reports every prerequisite either way, since the second missing one
// is worth knowing before installing the first.
func cmdDoctor(args []string) error {
	_, rest, err := bind("doctor", args, nil)
	if err != nil {
		return err
	}
	if err := none("doctor", rest); err != nil {
		return err
	}

	ok := true
	for _, r := range mount.Requirements() {
		mark := "ok  "
		if !r.Satisfied {
			mark, ok = "MISSING", false
		}
		fmt.Printf("  %-8s %-16s %s\n", mark, r.Name, r.Detail)
		if !r.Satisfied {
			fmt.Printf("  %-8s %-16s provided by %s\n", "", "", r.ProvidedBy)
		}
	}

	// The control socket is a prerequisite of the daemon in the same way
	// /dev/fuse is a prerequisite of a mount, and it fails the same way: early,
	// and with nothing else to explain it.
	socket, err := daemon.SocketPath()
	if err != nil {
		ok = false
		fmt.Printf("  %-8s %-16s %v\n", "MISSING", "XDG_RUNTIME_DIR", err)
		fmt.Printf("  %-8s %-16s provided by %s\n", "", "", "the login session; systemd sets it")
	} else {
		fmt.Printf("  %-8s %-16s %s\n", "ok  ", "control socket", socket)
	}

	if ok {
		fmt.Println("\nEvery prerequisite is satisfied.")
		return nil
	}
	// Installing needs root, and a tool that asks for root to install a system
	// package is a tool that should have printed the command instead.
	return airfs.Precondition(errors.New(
		"a prerequisite is missing; install what provides it, then run airfs doctor again"))
}
