# Command line interface

## Purpose

`airfs` is a Go SDK first: its capabilities are consumed as a library. The command
line exists so that a person, a shell profile, or a service manager can drive the
same capabilities without writing Go, and so that the SDK's behaviour is
observable without a test harness.

## Scope

The command surface, what each command reports, and exit codes. The behaviour each
command drives is specified elsewhere and is not restated here.

## Principle

The command line is a thin frontend over the library. It adds argument parsing,
human-readable reporting, and exit codes — nothing else. Any behaviour it appears
to add, another spec is missing.

Every command resolves the configuration of
[source-config.md](source-config.md), and takes the same overrides for the
configuration file and the target path, so that an experiment against an
alternative source list is one flag rather than an edited file.

## Commands

**`sources`** — resolve the configuration and report it: the ordered sources, the
kinds each contributes, the entry count per stratum, and every shadowed entry with
its winner and losers. Touches nothing. This is the command that answers "is my
repository being layered, and where in the order?", and the shadowing report is
what makes precedence debuggable rather than mysterious.

**`mount`** — serve the merged view at the target path per
[fuse-mount.md](fuse-mount.md), reporting the layers behind it. Blocks while
serving, since the mount lives as long as the process; a detached mode exists for
service managers and shell profiles, and reports how to stop it.

**`umount`** — release the mount, including recovering a stale mountpoint left by a
serving process that died. Reports that nothing was mounted rather than failing.

**`status`** — report whether the target path is a live mount, a symlink farm, or
neither, and what is visible through it. Distinguishing a live mount from a stale
one matters here: a stale mountpoint looks mounted and serves nothing, and a
contributor who cannot tell them apart cannot act.

**`sync`** — reconcile the symlink farm per [symlink-farm.md](symlink-farm.md),
reporting what changed. A verification mode makes no writes and exits non-zero if
the farm has drifted, for use in CI.

**`doctor`** — check the host's mount prerequisites: `/dev/fuse`, and a setuid
`fusermount3`. For each, report whether it is satisfied and, when it is not, which
system package provides it and that `sync` is the frontend that needs neither.
This exists as its own command because the requirements are the project's most
likely first failure, and a mount error alone does not explain which requirement
is missing or what to install.

## Reporting and exit codes

Output is for a person reading a terminal: the resolved state, then what changed.
Reports name sources by the path the configuration declared, not by a resolved
symlink target.

A machine-readable output mode is available for every reporting command, so that
`status` and `sources` can be consumed by a script without parsing prose.

Exit codes distinguish the three outcomes a caller acts on differently: success;
an unsatisfied precondition, such as a missing configuration, an unresolvable
source, a foreign entry in a farm target, or an absent mount prerequisite; and
drift detected by a verification mode. A verification mode's non-zero exit is not
an error and must be distinguishable from one, because CI treats them differently.

## Non-goals

- Editing the configuration file. It is authored by hand and reviewed in git;
  a command that rewrote it would produce diffs nobody wrote.
- Installing mount prerequisites. `doctor` reports and explains; installing needs
  root, and a tool that asks for root to install a system package is a tool that
  should have printed the command instead.
- Managing source repositories: cloning, pulling, or reporting their git state.
- Supervision. Keeping a mount alive across reboots belongs to the host's service
  manager; the SDK provides no daemon of its own.
- Authoring or validating resources. `airfs` layers directories; it does not know
  what a skill is.
