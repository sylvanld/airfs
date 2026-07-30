// Package daemon holds every workspace declared in the configuration, in one
// process per user. See docs/specs/daemon.md.
//
// A FUSE mount lives exactly as long as the process serving it, so something
// must stay alive; what this package decides is that it is one thing, not one
// per workspace.
package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/sylvanld/airfs/sdk"
	"github.com/sylvanld/airfs/sdk/config"
	"github.com/sylvanld/airfs/sdk/mount"
)

// A Daemon serves every enabled workspace of one configuration file.
type Daemon struct {
	// ConfigPath is the file this daemon loaded, and holds for its whole life.
	ConfigPath string

	mu sync.Mutex
	// servers holds one server per established workspace, and served holds the
	// declaration each was established from. The pair is what lets
	// reconciliation tell "mounted and still correct" from "mounted and out of
	// date" without asking a mount what it is serving, which it cannot answer.
	servers map[string]*mount.Server
	served  map[string]*config.Workspace
	// failed holds why an enabled workspace is not established, so that status
	// can say more than "no".
	failed     map[string]string
	config     *config.Config
	startedAt  time.Time
	reloadedAt time.Time

	listener net.Listener
	stopped  chan struct{}
	stopOnce sync.Once
}

// Start resolves the configuration, reconciles, and begins listening.
//
// It returns once the first reconciliation is complete, with what that
// reconciliation did — so a caller that succeeds knows which workspaces are
// established, and which were not.
//
// Starting when a daemon is already running is refused. Two daemons
// reconciling the same machine against the same file would fight: each would
// find the other's mounts unaccounted for and release them.
func Start(configPath string) (*Daemon, []Outcome, error) {
	socket, err := SocketPath()
	if err != nil {
		return nil, nil, err
	}
	if Running() {
		return nil, nil, airfs.Precondition(fmt.Errorf(
			"a daemon is already running on %s; stop it with: airfs down", socket))
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, nil, err
	}

	d := &Daemon{
		ConfigPath: configPath,
		servers:    map[string]*mount.Server{},
		served:     map[string]*config.Workspace{},
		failed:     map[string]string{},
		config:     cfg,
		startedAt:  time.Now(),
		stopped:    make(chan struct{}),
	}
	d.reloadedAt = d.startedAt
	outcomes := d.reconcile(cfg)

	if err := d.listen(socket); err != nil {
		// Nothing can reach a daemon that is not listening, so what it has
		// mounted would be unmanageable. Release it and fail.
		d.releaseEverything()
		return nil, nil, err
	}
	return d, outcomes, nil
}

// listen binds the control socket, clearing one left behind by a daemon that
// is no longer there. Running has already established that nothing answers on
// it, so what is left is a file rather than a peer.
func (d *Daemon) listen(socket string) error {
	if err := os.MkdirAll(filepath.Dir(socket), 0o700); err != nil {
		return err
	}
	if err := os.Remove(socket); err != nil && !os.IsNotExist(err) {
		return err
	}
	listener, err := net.Listen("unix", socket)
	if err != nil {
		return err
	}
	d.listener = listener
	return nil
}

// Wait serves control requests until the daemon is told to stop, and releases
// every mount before returning.
//
// A signal that permits cleanup unmounts first. Termination that does not —
// SIGKILL, a crash, power loss — leaves stale mounts, which the next
// reconciliation releases.
func (d *Daemon) Wait() error {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	go func() {
		select {
		case <-signals:
			d.Stop()
		case <-d.stopped:
		}
	}()

	for {
		conn, err := d.listener.Accept()
		if err != nil {
			select {
			case <-d.stopped:
				return d.releaseEverything()
			default:
			}
			return err
		}
		go d.handle(conn)
	}
}

// Stop releases every mount the daemon holds and ends Wait.
//
// A stop that cannot release a mount reports which, and still stops: a daemon
// that refuses to die because a directory is busy is worse than a stale mount,
// which is recoverable.
func (d *Daemon) Stop() {
	d.stopOnce.Do(func() {
		close(d.stopped)
		if d.listener != nil {
			d.listener.Close()
		}
	})
}

func (d *Daemon) releaseEverything() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	var errs []error
	for name, server := range d.servers {
		if err := server.Unmount(); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
		delete(d.servers, name)
		delete(d.served, name)
	}
	return errors.Join(errs...)
}

// Reload re-reads the configuration from disk and reconciles.
//
// A configuration that does not parse or does not validate changes nothing at
// all — neither the running mounts nor the daemon's idea of what is declared —
// and is reported to whoever asked for the reload.
func (d *Daemon) Reload() ([]Outcome, error) {
	cfg, err := config.Load(d.ConfigPath)
	if err != nil {
		return nil, err
	}
	d.mu.Lock()
	d.config = cfg
	d.reloadedAt = time.Now()
	d.mu.Unlock()
	return d.reconcile(cfg), nil
}

// reconcile makes what is mounted match what is declared. It is everything the
// daemon does, it runs at startup and on reload and nowhere else, and it is
// idempotent: running it twice against an unchanged configuration is a no-op
// the second time.
//
// Its second input is every airfs mount on the machine, not only the ones under
// a currently declared target. That is what makes the daemon able to account
// for mounts left by a previous configuration or a previous daemon.
func (d *Daemon) reconcile(cfg *config.Config) []Outcome {
	d.mu.Lock()
	defer d.mu.Unlock()

	all, err := mount.Mounts()
	if err != nil {
		return []Outcome{{Action: Failed, Reason: fmt.Sprintf("reading the mount table: %v", err)}}
	}

	var outcomes []Outcome
	// held is what the daemon now vouches for; gone is what it released along
	// the way. The mount table was read once, at the top, so a mountpoint this
	// pass has already released is still listed in it — and unmounting it a
	// second time fails, which would report a failure for work that succeeded.
	held, gone := map[string]bool{}, map[string]bool{}
	clear(d.failed)

	// A workspace this daemon established and that is no longer declared at all
	// is released before the declared ones are considered, so that a target
	// reused by another workspace is free by the time it is needed.
	for name := range d.servers {
		if cfg.Lookup(name) == nil {
			d.releaseWorkspace(name, gone)
			outcomes = append(outcomes, Outcome{Workspace: name, Action: Released, Reason: "no longer declared"})
		}
	}

	for _, w := range cfg.Workspaces {
		// Everything turns on being *wanted* — declared and enabled — rather
		// than on being declared, which is what makes disabling a workspace and
		// deleting it do the same thing to the machine while doing very
		// different things to the file.
		if !w.Enabled {
			outcomes = append(outcomes, d.stopServing(w, all, gone))
			continue
		}
		if d.holds(w, all) {
			// Adding a workspace to the file must not disturb the one currently
			// being read by a running agent.
			for _, folder := range w.Folders {
				held[w.FolderDir(folder)] = true
			}
			outcomes = append(outcomes, Outcome{Workspace: w.Name, Action: Unchanged})
			continue
		}

		action, reason := Established, ""
		if _, established := d.served[w.Name]; established {
			action, reason = Reestablished, "its declaration changed"
		} else if len(mount.Under(all, w.Target.Resolved)) > 0 {
			// The daemon cannot verify what an existing mount is serving, and a
			// mount it cannot vouch for is worse than a brief gap.
			action, reason = Reestablished, "it was mounted by something this daemon cannot vouch for"
		}
		d.releaseWorkspace(w.Name, gone)
		d.releaseUnder(all, w.Target.Resolved, gone)

		server, err := mount.Serve(w)
		if err != nil {
			// A workspace that cannot be established fails that workspace and
			// no other: one mistyped path must not take down the rest.
			d.failed[w.Name] = err.Error()
			outcomes = append(outcomes, Outcome{Workspace: w.Name, Action: Failed, Reason: err.Error()})
			continue
		}
		d.servers[w.Name], d.served[w.Name] = server, w
		for _, folder := range w.Folders {
			held[w.FolderDir(folder)] = true
		}
		outcomes = append(outcomes, Outcome{Workspace: w.Name, Action: action, Reason: reason})
	}

	// Whatever is left is a mount at no wanted target: one this configuration
	// does not account for, or one left behind by a previous daemon. Leaving it
	// would make the configuration a partial account of the machine. A stale
	// mount is released here too, by the same rule and with no separate concept.
	for _, m := range all {
		if held[m.Dir] || gone[m.Dir] {
			continue
		}
		if err := mount.Unmount(m.Dir); err != nil {
			outcomes = append(outcomes, Outcome{Action: Failed, Reason: err.Error()})
			continue
		}
		outcomes = append(outcomes, Outcome{Action: Released, Reason: "mounted at no enabled target: " + m.Dir})
	}
	return outcomes
}

// stopServing releases a disabled workspace. It stays declared and stays in
// every report; nothing of it is mounted.
func (d *Daemon) stopServing(w *config.Workspace, all []mount.Mount, gone map[string]bool) Outcome {
	_, established := d.served[w.Name]
	d.releaseWorkspace(w.Name, gone)
	if established || len(mount.Under(all, w.Target.Resolved)) > 0 {
		return Outcome{Workspace: w.Name, Action: Released, Reason: "disabled"}
	}
	return Outcome{Workspace: w.Name, Action: Disabled}
}

// holds reports whether the daemon established this exact workspace and every
// folder of it is still live. Anything else means releasing and establishing
// again, since a union is immutable once built.
func (d *Daemon) holds(w *config.Workspace, all []mount.Mount) bool {
	if d.servers[w.Name] == nil || !w.SameAs(d.served[w.Name]) {
		return false
	}
	for _, folder := range w.Folders {
		dir := w.FolderDir(folder)
		live := false
		for _, m := range all {
			if m.Dir == dir {
				live = !m.Stale
			}
		}
		if !live {
			return false
		}
	}
	return true
}

// releaseWorkspace unmounts what this daemon holds for a workspace, if
// anything. Its folders are released together: a half-served workspace is a
// view that lies about what is available.
func (d *Daemon) releaseWorkspace(name string, gone map[string]bool) {
	if server := d.servers[name]; server != nil {
		if w := d.served[name]; w != nil {
			for _, folder := range w.Folders {
				gone[w.FolderDir(folder)] = true
			}
		}
		server.Unmount()
	}
	delete(d.servers, name)
	delete(d.served, name)
}

// releaseUnder unmounts every airfs mount under target that this daemon does
// not hold — the ones it found rather than made.
func (d *Daemon) releaseUnder(all []mount.Mount, target string, gone map[string]bool) {
	for _, m := range mount.Under(all, target) {
		if gone[m.Dir] {
			continue
		}
		if err := mount.Unmount(m.Dir); err == nil {
			gone[m.Dir] = true
		}
	}
}

// Status is what the daemon knows and the filesystem does not: which file it
// loaded, and what that file declares.
func (d *Daemon) Status() *Status {
	d.mu.Lock()
	defer d.mu.Unlock()

	s := &Status{
		ConfigPath: d.ConfigPath,
		StartedAt:  d.startedAt,
		ReloadedAt: d.reloadedAt,
	}
	for _, w := range d.config.Workspaces {
		_, established := d.servers[w.Name]
		s.Workspaces = append(s.Workspaces, WorkspaceStatus{
			Name:        w.Name,
			Target:      w.Target.Declared,
			TargetDir:   w.Target.Resolved,
			Folders:     w.Folders,
			Enabled:     w.Enabled,
			Established: established,
			Reason:      d.failed[w.Name],
		})
	}
	return s
}

// handle answers one request on one connection.
func (d *Daemon) handle(conn net.Conn) {
	defer conn.Close()
	var req request
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		json.NewEncoder(conn).Encode(reply{Error: "unreadable request: " + err.Error()})
		return
	}
	var res reply
	switch req.Op {
	case OpStatus:
		res.Status = d.Status()
	case OpReload:
		outcomes, err := d.Reload()
		if err != nil {
			res.Error = err.Error()
		}
		res.Outcomes = outcomes
	case OpShutdown:
		res.Outcomes = d.shutdownOutcomes()
		json.NewEncoder(conn).Encode(res)
		conn.Close()
		// The reply goes out before the mounts go away, so that the caller is
		// told what happened even though answering is the last thing this
		// daemon does.
		d.Stop()
		return
	default:
		res.Error = fmt.Sprintf("unknown operation %q", req.Op)
	}
	json.NewEncoder(conn).Encode(res)
}

// shutdownOutcomes names what stopping will release, read before it is
// released so that the reply can carry it.
func (d *Daemon) shutdownOutcomes() []Outcome {
	d.mu.Lock()
	defer d.mu.Unlock()
	outcomes := make([]Outcome, 0, len(d.servers))
	for name := range d.servers {
		outcomes = append(outcomes, Outcome{Workspace: name, Action: Released, Reason: "the daemon is stopping"})
	}
	return outcomes
}
