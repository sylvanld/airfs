package daemon

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/sylvanld/airfs/sdk"
)

// DetachedEnv marks the re-executed child that actually serves. A detached
// daemon is reached exactly like a foreground one — through the control socket
// — so nothing about it has to be recorded anywhere.
const DetachedEnv = "AIRFS_DETACHED"

// Detached reports whether this process is the serving child of a detach.
func Detached() bool { return os.Getenv(DetachedEnv) == "1" }

// Detach re-executes this command in its own session and returns once the
// daemon is listening, which is once its first reconciliation is complete.
//
// The child is found through the socket rather than through a handshake, the
// same place every other client looks, so a detached daemon and a foreground
// one are the same thing to everything that comes after.
func Detach() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Env = append(os.Environ(), DetachedEnv+"=1")
	// The child's own report would duplicate the caller's, so only its errors
	// are kept: those are what a caller that got no daemon needs to read.
	cmd.Stdout, cmd.Stderr = nil, os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}

	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()

	deadline := time.After(30 * time.Second)
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case err := <-exited:
			// The child has already reported why it failed, on the stderr it
			// shares with this process, so repeating it would say it twice.
			// What must not be lost is which kind of failure it was: an
			// unsatisfied precondition stays one across the fork.
			var exit *exec.ExitError
			if errors.As(err, &exit) && exit.ExitCode() == airfs.ExitPrecondition {
				return airfs.Precondition(airfs.ErrReported)
			}
			if err == nil {
				return errors.New("the detached daemon stopped without serving")
			}
			return fmt.Errorf("the detached daemon did not start: %w", err)
		case <-deadline:
			_ = cmd.Process.Kill()
			return errors.New("the detached daemon did not start within 30s")
		case <-tick.C:
			if Running() {
				return nil
			}
		}
	}
}
