package daemon

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/sylvanld/airfs/sdk"
)

// SocketName is the control socket within the airfs runtime directory.
const SocketName = "control.sock"

// SocketPath is where the daemon listens.
//
// $XDG_RUNTIME_DIR is the right home for it: the directory is per-user, mode
// 0700, and cleared when the session ends, so a stale socket cannot outlive the
// boot that produced it. When XDG_RUNTIME_DIR is unset there is no world of
// equally good fallbacks — only worse ones — so this fails rather than choosing
// a world-writable directory.
func SocketPath() (string, error) {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		return "", airfs.Precondition(errors.New(
			"XDG_RUNTIME_DIR is not set; airfs needs it for the control socket"))
	}
	return filepath.Join(dir, "airfs", SocketName), nil
}

// The operations the control socket carries. The protocol is a private
// implementation detail between a daemon and the CLI built from the same
// source: it is not documented for third parties and not versioned. A program
// that wants the merged view reads the mounted directory, and a program that
// wants the model uses the SDK.
const (
	OpStatus   = "status"
	OpReload   = "reload"
	OpShutdown = "shutdown"
)

// A request is one operation, one connection.
type request struct {
	Op string `json:"op"`
}

// A reply carries whichever of the two answers the operation produces, plus
// the failure when there is one.
type reply struct {
	Error    string    `json:"error,omitempty"`
	Status   *Status   `json:"status,omitempty"`
	Outcomes []Outcome `json:"outcomes,omitempty"`
}

// A Status is what only the daemon knows.
//
// Everything else a report needs — that a daemon is or is not running, and
// every airfs mount on the machine — comes from the socket and the kernel's
// mount table, so a dead daemon is a fact to report rather than an error that
// prevents reporting.
type Status struct {
	// ConfigPath is the file the daemon actually loaded, which is not
	// necessarily the one a client would read. A daemon started with an
	// explicit configuration holds that file for its whole life, so the file a
	// person is editing and the file being served can differ — and every
	// symptom of that looks like airfs ignoring an edit. Nothing else on the
	// machine records which file a running daemon read.
	ConfigPath string    `json:"config_path"`
	StartedAt  time.Time `json:"started_at"`
	ReloadedAt time.Time `json:"reloaded_at"`

	// Workspaces is every workspace that file describes, in file order.
	Workspaces []WorkspaceStatus `json:"workspaces"`
}

// A WorkspaceStatus is one declared workspace as the daemon holds it.
//
// Enabled and Established are reported separately because they are different
// facts: a disabled workspace serving nothing is the configuration being
// honoured, and an enabled one serving nothing is not.
type WorkspaceStatus struct {
	Name string `json:"name"`
	// Target is the path as its author declared it, which is what reports name.
	// TargetDir is that path resolved, which is what a mountpoint read from the
	// kernel is matched against — a declared `~/x` never compares equal to the
	// absolute path the kernel reports.
	Target      string   `json:"target"`
	TargetDir   string   `json:"target_dir"`
	Folders     []string `json:"folders"`
	Enabled     bool     `json:"enabled"`
	Established bool     `json:"established"`
	// Reason says why an enabled workspace was not established.
	Reason string `json:"reason,omitempty"`
}

// Served reports whether every enabled workspace is established. It is what
// the frontend's exit code turns on: a disabled workspace serving nothing does
// not make the machine disagree with the file.
func (s *Status) Served() bool {
	for _, w := range s.Workspaces {
		if w.Enabled && !w.Established {
			return false
		}
	}
	return true
}

// An Action is what reconciliation did to one workspace.
type Action string

const (
	// Established: it was wanted and nothing was mounted for it.
	Established Action = "established"
	// Reestablished: it was wanted and mounted, but something about it
	// changed, or this daemon could not vouch for what was mounted. A union is
	// immutable once built, so remounting is the honest way to change one.
	Reestablished Action = "re-established"
	// Unchanged: wanted, mounted, and identical to what produced the mounts.
	// This is the guarantee that makes reload usable.
	Unchanged Action = "unchanged"
	// Released: it is no longer wanted, and what was mounted for it is gone.
	Released Action = "released"
	// Disabled: declared, not enabled, and nothing was mounted for it anyway.
	Disabled Action = "disabled"
	// Failed: wanted and could not be established. It fails alone.
	Failed Action = "failed"
)

// An Outcome is what reconciliation did to one workspace, and why when that
// needs saying.
type Outcome struct {
	Workspace string `json:"workspace"`
	Action    Action `json:"action"`
	Reason    string `json:"reason,omitempty"`
}

func (o Outcome) String() string {
	if o.Reason == "" {
		return fmt.Sprintf("%s %s", o.Workspace, o.Action)
	}
	return fmt.Sprintf("%s %s: %s", o.Workspace, o.Action, o.Reason)
}

// Failures reports whether any workspace failed to establish. The frontend
// exits on it: the machine does not match the file.
func Failures(outcomes []Outcome) bool {
	for _, o := range outcomes {
		if o.Action == Failed {
			return true
		}
	}
	return false
}
