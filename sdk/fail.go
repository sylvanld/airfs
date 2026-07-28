package airfs

import "errors"

// preconditionError marks a failure the host or the configuration must fix,
// rather than a malfunction of airfs. The frontend turns it into its own exit
// code, because it is the outcome a caller acts on differently.
type preconditionError struct{ err error }

func (e preconditionError) Error() string { return e.err.Error() }
func (e preconditionError) Unwrap() error { return e.err }

// Precondition marks err as an unsatisfied precondition. It returns nil for a
// nil error, so it can wrap a call's result directly.
func Precondition(err error) error {
	if err == nil {
		return nil
	}
	return preconditionError{err}
}

// IsPrecondition reports whether err, or anything it wraps, is an unsatisfied
// precondition.
func IsPrecondition(err error) bool {
	var p preconditionError
	return errors.As(err, &p)
}

// Exit codes, per docs/specs/cli.md. They live here rather than in the command
// because a detached mount re-executes the command as a child, and the parent
// has to recognise the child's outcome.
const (
	ExitOK           = 0
	ExitFailed       = 1
	ExitPrecondition = 2
)

// ErrReported marks a failure whose explanation has already reached the user,
// so that the frontend sets the exit code without saying it a second time.
var ErrReported = errors.New("already reported")
