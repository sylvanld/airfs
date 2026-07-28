# airfs

A Go SDK for layering AI resource directories — skills, agents, commands — from
several repositories into one read-only merged view, served as a FUSE mount
implemented in pure Go.

```
sylvan/ai-resources/skills/  ┐
sylvan/ai-tools/skills/      ├─ airfs (ro) ─→  ~/.ai-resources/skills/
sylvan/ai-maintainer/skills/ ┘
```

The problem: agent tooling wants every skill under one directory, but skills are
authored in the repository that owns them. Copying drifts. Hand-made symlinks
drift. `airfs` merges the directories in place, so each resource keeps living in —
and is edited in — its own repository, and the merged view is only a view.

On a name collision the earliest declared source wins, the entry wins whole, and
the shadowed entries are reported rather than silently dropped.

## Status

Specification stage. The specs in [`docs/specs/`](docs/specs/index.md) are
`draft`; no implementation exists yet. It replaces a `mergerfs`-based `Makefile`
setup in [`ai-resources`](https://github.com/hoshiyosan/ai-resources).

## Why pure Go

The predecessor needed a `mergerfs` binary, which the distribution packages only
with root, so it had to be unpacked by hand into a user-local prefix. `airfs`
speaks the FUSE protocol over `/dev/fuse` from Go, linking no C library and
requiring no filesystem binary.

Two host requirements remain, because an unprivileged process cannot mount
without them: `/dev/fuse`, and a setuid `fusermount3` — which ships with the
system FUSE package and is already present on a normal desktop Linux. Where
neither is available, the same merged view can be materialised as a symlink
directory instead, trading the kernel-enforced read-only guarantee for portability.

## Design

Read [`docs/specs/index.md`](docs/specs/index.md). The load-bearing decision is
that the merge is a standard read-only filesystem interface and every frontend
reads through it, so the merge semantics are defined and tested once — against
in-memory layers, with no mount, no root, and no cleanup.

| Spec | |
| --- | --- |
| [layered-resources.md](docs/specs/layered-resources.md) | sources, kinds, precedence, shadowing, read-only |
| [source-config.md](docs/specs/source-config.md) | declaring sources and kinds |
| [layered-fs.md](docs/specs/layered-fs.md) | the merge itself |
| [fuse-mount.md](docs/specs/fuse-mount.md) | serving it at a real path |
| [symlink-farm.md](docs/specs/symlink-farm.md) | the fallback frontend |
| [cli.md](docs/specs/cli.md) | the `airfs` command surface |

## Contributing

No implementation change without an agreed spec — see [AGENTS.md](AGENTS.md).

Run `make` for the available targets and `make check` before pushing. Setup,
prerequisites, and what each gate verifies are in
[`docs/contribute/`](docs/contribute/index.md).
