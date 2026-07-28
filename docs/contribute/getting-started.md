# Getting started

## Prerequisites

| Requirement | Version | Why |
| --- | --- | --- |
| Go | 1.26 or later | the SDK's toolchain |
| GNU make, bash | any current | the targets in `Makefile` |
| [uv](https://docs.astral.sh/uv/) | 0.11 or later | the `docs-*` targets; it installs Python and the site generator for you |

Running the FUSE mount additionally needs two things from the host, neither of
which this project installs and neither of which can be worked around from
userspace:

| Requirement | Check | Provided by |
| --- | --- | --- |
| `/dev/fuse`, readable and writable by you | `ls -l /dev/fuse` | the kernel; absent in some containers |
| a **setuid** `fusermount3` | `ls -l /usr/bin/fusermount3` — the mode must begin `-rws` | `libfuse3-3`, present on a normal desktop Linux |

No `mergerfs` and no other filesystem binary is required: the mount speaks the
FUSE protocol from Go, and every dependency is cgo-free so that `go install` is
the whole installation. See
[`docs/specs/fuse-mount.md`](../specs/fuse-mount.md) for why the two requirements
above nevertheless remain.

## What you can run today

Every spec in [`docs/specs/`](../specs/index.md) is `implemented`: the library
and the `airfs` command exist and are covered by tests.

- `make` — list targets.
- `make check` — run every quality gate. See [quality-gates.md](quality-gates.md).
- `make build` — build the command into `bin/airfs`.
- `make test` — run the Go test suite.
- `make format` — rewrite Go files into their canonical formatting, which is what
  `make lint` verifies.
- `make docs-serve`, `make docs-build`, `make docs-clean` — work on this site.
  See [documentation-site.md](documentation-site.md).

The mount tests establish real FUSE mounts. On a host missing `/dev/fuse` or a
setuid `fusermount3` they skip rather than fail, since those are the host's to
provide and a container that lacks them is not a broken build — but a run that
skipped them has not exercised the mount.

## Before you change anything

One rule shapes everything else: no implementation change without an agreed
spec. Start from [working-on-specs.md](working-on-specs.md).
