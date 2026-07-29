package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/sylvanld/airfs/sdk"
)

// A Client reaches a running daemon over the control socket.
type Client struct{ socket string }

// Dial connects to the running daemon.
//
// The socket is the liveness check, and it is self-verifying in the same way
// the mount table is: a socket left behind by a dead daemon refuses
// connections, so "can I connect" and "is it alive" are the same question.
// There is no PID file, because a PID file answers that question with
// something that can be wrong.
func Dial() (*Client, error) {
	socket, err := SocketPath()
	if err != nil {
		return nil, err
	}
	conn, err := net.Dial("unix", socket)
	if err != nil {
		return nil, airfs.Precondition(errors.New("no daemon is running; start one with: airfs up"))
	}
	conn.Close()
	return &Client{socket: socket}, nil
}

// Running reports whether a daemon is listening.
func Running() bool {
	socket, err := SocketPath()
	if err != nil {
		return false
	}
	conn, err := net.Dial("unix", socket)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// Status asks the daemon which configuration it loaded and what that file
// declares.
func (c *Client) Status() (*Status, error) {
	res, err := c.send(OpStatus)
	if err != nil {
		return nil, err
	}
	if res.Status == nil {
		return nil, errors.New("the daemon returned no status")
	}
	return res.Status, nil
}

// Reload tells the daemon to re-read its configuration and reconcile. A
// configuration that does not resolve changes nothing and comes back as the
// error.
func (c *Client) Reload() ([]Outcome, error) {
	res, err := c.send(OpReload)
	if err != nil {
		return nil, err
	}
	return res.Outcomes, res.failure()
}

// Shutdown stops the daemon, which releases every mount it holds.
func (c *Client) Shutdown() ([]Outcome, error) {
	res, err := c.send(OpShutdown)
	if err != nil {
		return nil, err
	}
	return res.Outcomes, res.failure()
}

// failure turns a reported failure back into an error. It is a precondition:
// what a reload refuses is a configuration the author must fix.
func (r *reply) failure() error {
	if r.Error == "" {
		return nil
	}
	return airfs.Precondition(errors.New(r.Error))
}

// send carries one operation over one connection.
func (c *Client) send(op string) (*reply, error) {
	conn, err := net.Dial("unix", c.socket)
	if err != nil {
		return nil, airfs.Precondition(errors.New("no daemon is running; start one with: airfs up"))
	}
	defer conn.Close()

	if err := json.NewEncoder(conn).Encode(request{Op: op}); err != nil {
		return nil, err
	}
	var res reply
	if err := json.NewDecoder(conn).Decode(&res); err != nil {
		// The socket is private and unversioned, so a daemon that cannot be
		// spoken to is one built from different source. Restarting it is the
		// whole of the fix.
		return nil, airfs.Precondition(fmt.Errorf(
			"the running daemon does not speak this version of the control protocol (%v); restart it with: airfs down && airfs up", err))
	}
	return &res, nil
}

// WaitReady blocks until a daemon is listening, or the deadline passes.
//
// Readiness is the socket accepting connections, because the daemon listens
// only once its first reconciliation is complete: a caller that connects knows
// the workspaces are established, or knows which ones were not.
func WaitReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if Running() {
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return fmt.Errorf("the daemon did not start within %s", timeout)
}
