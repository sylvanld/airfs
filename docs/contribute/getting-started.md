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
FUSE protocol from Go directly. See
[`docs/specs/fuse-mount.md`](../specs/fuse-mount.md) for why the two requirements
above nevertheless remain, and
[`docs/specs/symlink-farm.md`](../specs/symlink-farm.md) for the frontend that
needs neither.

## What you can run today

The repository is at the specification stage: the specs in
[`docs/specs/`](../specs/index.md) are `draft`, and no implementation exists yet.

- `make` — list targets.
- `make check` — run every quality gate. See [quality-gates.md](quality-gates.md).
- `make docs-serve`, `make docs-build`, `make docs-clean` — work on this site.
  See [documentation-site.md](documentation-site.md).

Build and test targets for the code itself will appear in the change that adds
the code they run against, per the rule that a target must do real work today.

## Before you change anything

One rule shapes everything else: no implementation change without an agreed
spec. Start from [working-on-specs.md](working-on-specs.md).
