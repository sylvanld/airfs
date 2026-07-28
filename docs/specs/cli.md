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
[source-config.md](source-config.md), and every command takes the same two
overrides, so that an experiment against an alternative source list is one flag
rather than an edited file:

- `--target <dir>` — the directory the view is exposed under. Defaults to
  `$HOME/.ai-resources`: the directory holding the configuration file and one
  mounted subdirectory per kind. The target is a property of the frontend rather
  than of the source list, so it is a default and an override, never a line in
  the configuration file — the same sources may be exposed at more than one
  target.
- `--config <file>` — the configuration file to read. Defaults to `sources.txt`
  inside the target. Overriding it does not move the target: the two are
  independent, which is what allows one source list to be tried against a
  scratch target, and one target to be rebuilt from an alternative list.

Both accept a `~`-prefixed path, expanded as in
[source-config.md](source-config.md). A relative `--target` or `--config`
resolves against the current working directory, because it was written on the
command line, where the working directory is the obvious frame of reference.
Paths *inside* the configuration file are unaffected: they resolve against the
directory containing that file, per
[source-config.md](source-config.md), so a source list means the same thing
wherever it is read from.

## Commands

**`sources`** — resolve the configuration and report it: the ordered sources, the
entry count each contributes per kind, and every shadowed entry with its winner and
losers. Touches nothing. This is the command that answers "is my repository being
layered, and where in the order?", and the shadowing report is what makes
precedence debuggable rather than mysterious.

**`mount`** — serve the merged view under the target per
[fuse-mount.md](fuse-mount.md), reporting the layers behind it. Blocks while
serving, since the mount lives as long as the process; `--detach` returns once
the view is ready and reports how to stop it, which is what a service manager or
a shell profile needs.

`--source <path>`, short `-s`, declares one source and is accepted more than
once; the order the flags are given in is the precedence order, most general
first. Giving any of them **replaces** the configuration file with exactly that
list before the view is served, and creates the target directory if it is not
there yet — so that a workspace can be brought into being by one command instead
of by a file that has to be authored first.

It is a replacement, never a merge: whatever the file said, comments included,
is gone, and nothing is kept alongside it. That is the point of it being one
flag on one command rather than a general editing facility — someone who typed
the list they want is stating the whole list, and a flag that quietly appended
to what was already there would produce an order nobody wrote. The new list is
resolved *before* the old file is replaced, though, so a mistyped path leaves
the existing configuration standing rather than replacing it with something that
does not resolve.

A path given this way is written down as it was typed, so that `~` and `$VAR`
survive into the file and it keeps the form its author would recognise. The one
exception is a path still relative after expansion: on the command line it means
"from the working directory", inside the file it would mean "from the file's
directory", so it is made absolute before being written. Writing it verbatim
would silently change which directory it names.

The flag is on `mount` alone. `sources` and `status` report and touch nothing; a
read-only command that rewrote the configuration as a side effect would be a
trap.

**`umount`** — release the target's mounts, including recovering a stale mountpoint
left by a serving process that died. Reports that nothing was mounted rather than
failing.

**`status`** — report whether the target is being served, and what is visible
through it. Distinguishing a live mount from a stale one matters here: a stale
mountpoint looks mounted and serves nothing, and a contributor who cannot tell them
apart cannot act.

**`doctor`** — check the host's mount prerequisites: `/dev/fuse`, and a setuid
`fusermount3`. For each, report whether it is satisfied and, when it is not, which
system package provides it. This exists as its own command because the
requirements are the project's most likely first failure, and a mount error alone
does not explain which requirement is missing or what to install.

## Reporting and exit codes

Output is for a person reading a terminal: the resolved state, then what changed.
Reports name sources by the path the configuration declared, not by a resolved
symlink target.

There is no machine-readable output mode. A program that needs the merged view
consumes the SDK, in-process and without parsing anything; adding a second,
stringly-typed interface for the same data would be a contract to keep stable for
no caller that could not do better.

Exit codes are `0` for success, `2` for an unsatisfied precondition — a missing
configuration, an unresolvable source, a non-empty mountpoint, an absent mount
prerequisite — and `1` for everything else that failed. The precondition code is
separate because it is the outcome a caller acts on differently: it means the host
or the configuration needs attention, not that `airfs` malfunctioned.

Two commands report a *state* rather than an outcome, and their codes say which
state, so that a shell profile can branch without parsing prose:

- `status` exits `0` when the target is fully served, and `2` when it is not —
  nothing mounted, only some kinds mounted, or a mountpoint gone stale. Each of
  those is a condition to act on, and each is named in the output.
- `doctor` exits `0` when every prerequisite is satisfied and `2` when any is
  missing. It reports every prerequisite either way, since the second missing one
  is worth knowing before installing the first.

Shadowing is not a failure. `sources` exits `0` with shadowed entries reported,
because shadowing is the mechanism working, and an exit code that punished it
would make the normal case indistinguishable from a broken configuration.

## Non-goals

- Editing the configuration file *in place*. `mount --source` replaces it whole;
  nothing appends a line to it, rewrites one line of it, or reorders it. A
  command that edited it piecewise would produce diffs nobody wrote, and the
  file is otherwise authored by hand and reviewed in git.
- Installing mount prerequisites. `doctor` reports and explains; installing needs
  root, and a tool that asks for root to install a system package is a tool that
  should have printed the command instead.
- Managing source repositories: cloning, pulling, or reporting their git state.
- Supervision. Keeping a mount alive across reboots belongs to the host's service
  manager; the SDK provides no daemon of its own beyond detaching one.
- Authoring or validating resources. `airfs` layers directories; it does not know
  what a skill is.
