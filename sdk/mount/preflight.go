package mount

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"strings"

	"github.com/sylvanld/airfs/sdk"
)

// A Requirement is one thing the host must provide for a mount to be possible.
// Neither can be worked around from userspace, so each is reported by name
// along with what provides it: the requirements are the project's most likely
// first failure, and a mount error alone does not explain which is missing.
type Requirement struct {
	Name       string
	Satisfied  bool
	Detail     string
	ProvidedBy string
}

// Requirements checks the host's mount prerequisites and reports every one of
// them, satisfied or not — the second missing one is worth knowing before
// installing the first.
func Requirements() []Requirement {
	return []Requirement{devFuse(), fusermount()}
}

// Preflight fails when any requirement is unsatisfied.
func Preflight() error {
	var missing []string
	for _, r := range Requirements() {
		if !r.Satisfied {
			missing = append(missing, fmt.Sprintf("%s: %s (provided by %s)", r.Name, r.Detail, r.ProvidedBy))
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return airfs.Precondition(errors.New("mount prerequisites are not satisfied:\n  " + strings.Join(missing, "\n  ")))
}

func devFuse() Requirement {
	r := Requirement{
		Name:       "/dev/fuse",
		ProvidedBy: "the kernel; absent in some containers",
	}
	file, err := os.OpenFile("/dev/fuse", os.O_RDWR, 0)
	if err != nil {
		r.Detail = fmt.Sprintf("not readable and writable by you (%v)", err)
		return r
	}
	file.Close()
	r.Satisfied = true
	r.Detail = "readable and writable by you"
	return r
}

func fusermount() Requirement {
	r := Requirement{
		Name:       "fusermount3",
		ProvidedBy: "the system FUSE package, libfuse3-3 on Debian and Ubuntu",
	}
	path, err := exec.LookPath("fusermount3")
	if err != nil {
		r.Detail = "not found on PATH"
		return r
	}
	info, err := os.Stat(path)
	if err != nil {
		r.Detail = fmt.Sprintf("%s: %v", path, err)
		return r
	}
	// An unprivileged process cannot mount without the setuid helper, so the
	// binary existing is not enough.
	if info.Mode()&fs.ModeSetuid == 0 {
		r.Detail = fmt.Sprintf("%s is not setuid (mode %s)", path, info.Mode())
		return r
	}
	r.Satisfied = true
	r.Detail = fmt.Sprintf("%s, setuid", path)
	return r
}
