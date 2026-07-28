package mount

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/sylvanld/airfs"
)

// DetachedEnv marks the re-executed child that actually serves the mounts. A
// detached process is managed exactly like a foreground one — found through the
// kernel and released by unmounting — so nothing has to be kept anywhere.
const DetachedEnv = "AIRFS_DETACHED"

// Detached reports whether this process is the serving child of a detach.
func Detached() bool { return os.Getenv(DetachedEnv) == "1" }

// Detach re-executes this command in its own session and returns once target's
// mounts are established, so a caller that succeeds knows the view is ready.
func Detach(target string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Env = append(os.Environ(), DetachedEnv+"=1")
	// The child's own report would duplicate the caller's, so only its errors
	// are kept: those are what a caller that got no mount needs to read.
	cmd.Stdout, cmd.Stderr = nil, os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}

	// The child reports readiness through the kernel's mount table, the same
	// place everything else reads it from, rather than through a handshake that
	// would be a second record of what is mounted.
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()

	deadline := time.After(30 * time.Second)
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case err := <-exited:
			// The child has already reported why it failed, on the stderr it
			// shares with this process, so repeating it would say it twice. What
			// this process must not lose is which kind of failure it was: an
			// unsatisfied precondition stays one across the fork.
			var exit *exec.ExitError
			if errors.As(err, &exit) && exit.ExitCode() == airfs.ExitPrecondition {
				return airfs.Precondition(airfs.ErrReported)
			}
			if err == nil {
				return errors.New("the detached process stopped without serving")
			}
			return fmt.Errorf("the detached process did not mount: %w", err)
		case <-deadline:
			_ = cmd.Process.Kill()
			return errors.New("detached process did not mount within 30s")
		case <-tick.C:
			states, err := Status(target)
			if err != nil {
				return err
			}
			if Served(states) {
				return nil
			}
		}
	}
}
