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

Every command reads the configuration of
[workspace-config.md](workspace-config.md), and every command takes the same
override:

- `--config <file>` — the configuration file to read. Defaults to
  `$XDG_CONFIG_HOME/airfs/config.yaml`. A `~`-prefixed path is expanded, and a
  relative path resolves against the current working directory, because it was
  written on the command line, where the working directory is the obvious frame
  of reference. Paths *inside* the file are unaffected: they resolve against the
  directory containing that file, per
  [workspace-config.md](workspace-config.md), so a configuration means the same
  thing wherever it is read from.

There is no `--target` override. A target is a property of a declared workspace
now, not of an invocation; the thing an invocation names is a workspace, by name.

Commands that act on workspaces take zero or more names as arguments. Zero means
every declared workspace, which is the common case and is what a service manager
invokes. A name that is not declared is an error rather than a silent no-op.

The surface splits in three, by what a command is *about*:

- **Declaring** — `add`, `rm`, `enable`, `disable`. Change what is written down,
  then reload.
- **Inspecting** — `ls`, `inspect`. Change nothing, and need nothing running.
- **Running** — `up`, `down`, `reload`, `status`, `doctor`. Change or report what
  the daemon is doing, and write nothing.

No command spans two groups, which is the distinction the previous design's
`mount` lost: it served a view and rewrote a source list under one verb, and
with one file describing the whole machine that verb would have described
neither half honestly.

The split also decides where a *report* belongs, which is subtler than where an
action belongs. A report goes with the thing it is a report *of*: `status` is
about the daemon, so it is a running command, and what it reports is the
daemon's own state. What a workspace merges is not the daemon's state — it is
true whether or not anything is running — so it is `inspect`, and it sits with
the other command that needs no daemon.

## Declaring

These read the configuration, change one workspace, validate the whole result,
write it, and reload the daemon — one operation with a different mutation in the
middle, per [daemon.md](daemon.md). Each reports every workspace whose resolved
configuration changed, not only the one named, since a merge key can carry an
edit to its aliases; see [workspace-config.md](workspace-config.md).

The file is validated *before* it is written, so a mistyped path leaves the
existing configuration standing rather than replacing it with something that
does not resolve. When there is no daemon running, the file is still written and
the command says so; the declaration is the durable half.

**`add <name>`** — declare a workspace, or replace an existing one whole.
`--target`, `--source` (repeatable, in precedence order, most general first),
`--folder` (repeatable, defaulting to the built-in set). The workspace is
enabled unless `--disabled` is given.

On an existing name it is a replacement, never a merge: what the block said is
gone. Someone who typed the sources they want is stating the whole list, and a
flag that quietly appended to what was there would produce an order nobody
wrote. This is the one command that brings a workspace into being without
authoring YAML first, which is the reason it exists.

A path is written down as it was typed, so that `~` and `$VAR` survive into the
file and it keeps the form its author would recognise. The exception is a path
still relative after expansion: on the command line it means "from the working
directory", in the file it would mean "from the file's directory", so it is made
absolute before being written. Written verbatim it would silently name a
different directory.

**`rm <name>`** — remove a workspace's declaration and release its mounts. It
prints the block it removed before removing it, so that an unintended `rm` is
recoverable from the terminal's scrollback rather than only from git.

**`enable <name>` / `disable <name>`** — set the workspace's `enabled` field.
Disabling releases its mounts and keeps everything else: the declaration, its
comments, its place in the file. This is how a workspace stops being served, and
it is why `rm` is rare — losing a mount should not require losing what produced
it. Enabling or disabling something already in that state is a no-op that
reports as one.

## Inspecting

Both commands resolve the configuration and report it. Neither mounts anything,
writes anything, or needs a daemon. Resolving a configuration to report it is
also what validates it, so either on a broken file reports the same errors a
reload would, without the risk of having reloaded — which is what makes them the
commands to run before trusting an edit.

**`ls`** — one line per declared workspace: its name, whether it is enabled, its
target, its folders, and how many sources it layers. The declared inventory, and
nothing about what is running. It takes no name; a single workspace is what
`inspect` is for.

**`inspect <name>`** — everything about one workspace: its target, its folders,
its sources in precedence order, the entry count each source contributes per
folder, the folders that end up empty, and every shadowed entry with its winner
and its losers.

This is the command that answers "is my repository being layered, and where in
the order?", and its shadowing report is what makes precedence debuggable rather
than mysterious. It reads the sources themselves, which is why it is separate
from `ls`: listing what is declared is cheap and reads one file, while
inspecting a workspace enumerates every source's every folder.

It reports what the *declaration* produces, not what is currently mounted. The
two coincide when the workspace is served, and when they do not, the difference
between `inspect` and `status` is the diagnosis: `inspect` says what should be
there, `status` says what is.

## Running

**`up`** — start the daemon of [daemon.md](daemon.md): resolve the
configuration, reconcile every enabled workspace, and serve. Reports each
workspace as it is established, and each that failed with the reason. Blocks
while serving; `--detach` returns once the first reconciliation is complete and
reports how to stop it, which is what a service manager or a shell profile
needs. Refuses to start when a daemon is already running, and says so rather
than producing a second one.

**`down`** — stop the daemon and release every `airfs` mount on the machine,
including those a previous daemon left behind and those whose serving process
died. Reports what it released, and reports that nothing was mounted rather than
failing. It works with no daemon running, which is what makes it the single
recovery command: there is no separate `umount`, because "release what should not
be mounted" is one operation regardless of what is alive.

`down` stops the machine; `disable` stops a workspace. Neither is the other, and
neither writes anything the other reads: `down` leaves every declaration intact
and `up` restores exactly what was running.

**`reload`** — tell the running daemon to re-read the configuration and
reconcile. Reports what changed: established, released, re-established,
unchanged, failed. A configuration that does not parse or does not validate is
reported and changes nothing. Requires a running daemon, and says to run `up`
when there is none.

Reload is a command rather than a file watch because an editor writing a file is
not one event; see [daemon.md](daemon.md). The declarative commands reload on
their own, so this one exists for a file edited by hand.

**`status`** — the daemon's state. It reports, in this order:

1. **Whether a daemon is running**, and since when.
2. **Which configuration file it loaded.** Not the one this invocation would
   read — the one the daemon actually holds, which it was started with and has
   reloaded from since.
3. **Every workspace that file describes**, each as declared, enabled or
   disabled, and established or not — with the reason when a workspace is
   enabled and was not established.
4. **What is mounted**, per workspace: each folder's mountpoint, and whether it
   is mounted, stale, or absent.

Point 2 is the reason this command reports rather than a person guessing. A
daemon started with `--config` holds that file for its whole life, so the file
being edited and the file being served can differ, and every other symptom of
that looks like `airfs` ignoring an edit. When the configuration this invocation
would read is not the one the daemon loaded, `status` says so as its own line
rather than leaving it to be inferred from a path printed in passing.

The mount facts come from the kernel's mount table, so points 1 and 4 are
answerable with no daemon alive: `status` on a dead daemon reports that it is
dead and still reports every `airfs` mount left on the machine, which is the
state that most needs reporting. Points 2 and 3 come from the daemon and are
reported as unavailable when there is none.

Distinguishing a live mount from a stale one matters here: a stale mountpoint
looks mounted and serves nothing, and a contributor who cannot tell them apart
cannot act. Distinguishing a disabled workspace from a failed one matters for
the same reason — one is the configuration being honoured and the other is not —
and `status` names which it is.

An optional name limits the report to one workspace. It never reports what a
workspace *merges*; that is `inspect`.

**`doctor`** — check the host's mount prerequisites: `/dev/fuse`, a setuid
`fusermount3`, and a usable `$XDG_RUNTIME_DIR` for the control socket. For each,
report whether it is satisfied and, when it is not, which system package or
setting provides it. This exists as its own command because the requirements are
the project's most likely first failure, and a mount error alone does not
explain which requirement is missing or what to install.

## Reporting and exit codes

Output is for a person reading a terminal: the resolved state, then what changed.
Reports name workspaces by their declared name and sources by the path the
configuration declared, not by a resolved symlink target.

There is no machine-readable output mode. A program that needs the merged view
reads the mounted directory; a program that needs the model consumes the SDK,
in-process and without parsing anything. Adding a second, stringly-typed
interface for the same data would be a contract to keep stable for no caller that
could not do better.

Exit codes are `0` for success, `2` for an unsatisfied precondition — a missing
or invalid configuration, an unresolvable source, a non-empty mountpoint, an
absent mount prerequisite, a daemon that is required and not running or already
running — and `1` for everything else that failed. The precondition code is
separate because it is the outcome a caller acts on differently: it means the host
or the configuration needs attention, not that `airfs` malfunctioned.

Two commands report a *state* rather than an outcome, and their codes say which
state, so that a shell profile can branch without parsing prose:

- `status` exits `0` when every enabled workspace is fully served, and `2` when
  any is not — nothing mounted, only some folders mounted, or a mountpoint gone
  stale. Each of those is a condition to act on, and each is named in the output.
  A disabled workspace serving nothing is the configuration being honoured, so
  it does not affect the code.
- `doctor` exits `0` when every prerequisite is satisfied and `2` when any is
  missing. It reports every prerequisite either way, since the second missing one
  is worth knowing before installing the first.

`up`, `reload`, and every declarative command exit `2` when any workspace failed
to establish, having established every other one. The code reports that the
machine does not match the file; the output names which workspaces and why.
Exiting `0` would hide a failed workspace behind the success of its neighbours,
and exiting `1` would claim `airfs` malfunctioned when a declared path simply is
not there.

A declarative command that wrote the file and found no daemon to reload also
exits `2`: the durable half succeeded and the machine does not match it yet,
which is exactly what that code means.

Shadowing is not a failure. `inspect` exits `0` with shadowed entries reported,
because shadowing is the mechanism working, and an exit code that punished it
would make the normal case indistinguishable from a broken configuration.

A daemon serving a different configuration file than this invocation would read
is reported by `status` and does not change its exit code. Running the daemon
against another file is a legitimate thing to do deliberately; the code stays
about whether what is enabled is served.

## Non-goals

- Editing anything but one workspace at a time. `add` replaces a block whole and
  `rm` deletes one; nothing appends to a source list, reorders one, or rewrites
  several workspaces in one invocation. A command that edited piecewise would
  produce diffs nobody wrote, and the file is also authored by hand.
- Installing mount prerequisites. `doctor` reports and explains; installing needs
  root, and a tool that asks for root to install a system package is a tool that
  should have printed the command instead.
- Managing source repositories: cloning, pulling, or reporting their git state.
- Serving a workspace that is not written down. `add` then `disable` is how a
  stack is tried without committing to it, which leaves a record of what was
  tried; an ephemeral mount would leave none.
- Supervision. `up --detach` starts the daemon; keeping it alive across reboots
  belongs to the host's service manager, which the provided systemd user unit
  serves without being required.
- Authoring or validating resources. `airfs` layers directories; it does not know
  what a skill is.
