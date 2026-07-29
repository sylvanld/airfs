# Daemon

## Purpose

One process establishes and holds every workspace declared in the
configuration. A FUSE mount lives exactly as long as the process serving it, so
something must stay alive; the question this spec answers is *how many* things,
and the answer is one per user, not one per workspace.

## Scope

The daemon's lifecycle, how it reconciles what is declared against what is
mounted, how it is reached, and how failure in one workspace is contained. What
is declared is [workspace-config.md](workspace-config.md); what a mount
guarantees is [fuse-mount.md](fuse-mount.md); the commands that drive this are
[cli.md](cli.md).

## Shape

A single daemon per user serves every workspace's every folder. Each folder is
its own mount, and the daemon holds them all: mounts are cheap, processes are
not, and a process per workspace multiplies the things that can be alive when
they should not be, or dead when they should not be.

The daemon does the mounting itself rather than supervising children. There is
nothing for a supervisor to add — a FUSE mount cannot outlive its server, so a
restarted child would produce a new mount, not a recovered one.

## Reconciliation

Everything the daemon does is one operation: make what is mounted match what is
declared. It runs at startup, on reload, and nowhere else.

Its two inputs are:

1. The resolved configuration.
2. **Every** `airfs` mount on the machine, read from the kernel's mount table by
   the filesystem name set at mount time, per [fuse-mount.md](fuse-mount.md) —
   not only the ones under a currently declared target.

The second input is what makes the daemon able to answer "what is `airfs` doing
on this host" completely, including mounts left by a previous configuration or a
previous daemon. It is read from the kernel rather than from a file the daemon
keeps, because a file it keeps disagrees with reality exactly when it matters:
after a crash, after a manual `fusermount3 -u`, after the configuration changed
while nothing was running.

A workspace is **wanted** when it is declared and `enabled`, per
[workspace-config.md](workspace-config.md). Everything below turns on that word
rather than on being declared, which is what makes disabling a workspace and
deleting it do the same thing to the machine while doing very different things
to the file.

For each wanted workspace:

- **Wanted, nothing mounted** — establish it: mount every folder.
- **Wanted, mounted, and the daemon established it from a configuration
  identical to the current one** — leave it alone. This is the guarantee that
  makes reload usable: adding a workspace to the file must not disturb the one
  currently being read by a running agent.
- **Wanted, mounted, and anything about it changed** — its sources, their
  order, or its folders — release it and establish it again. A union is
  immutable once built, and remounting is the honest way to change one.
- **Wanted and mounted, but the daemon has no record of establishing it** —
  which is every mount it finds at startup — release it and establish it again.
  The daemon cannot verify what an existing mount is serving, and a mount it
  cannot vouch for is worse than a brief gap.

And for what is not wanted:

- **An `airfs` mount belonging to a disabled workspace** — release it. The
  workspace stays declared and stays in every report; nothing of it is mounted.
- **An `airfs` mount at no declared target** — release it. It was declared once
  and is not any more; leaving it would make the configuration a partial account
  of the machine.
- **A stale mount** — one still listed by the kernel but failing every access,
  the signature of a server that died — is released the same way, at whichever
  of the above steps meets it. Recovering it needs no separate command and no
  separate concept.

Reconciliation is idempotent. Running it twice against an unchanged
configuration is a no-op the second time, which is what makes it safe to run on
every reload without asking whether it is needed.

### Per-workspace isolation

A workspace that cannot be established — a source that does not exist, a
non-empty mountpoint, a target that cannot be created — fails **that workspace**
and no other. The failure is reported, reconciliation continues, and every other
declared workspace is served.

This is the change the centralized file forces. When one file described one
workspace, refusing to proceed was right: everything the file described was
broken. When one file describes the machine, refusing to proceed would let one
mistyped path in one workspace take down the rest, and the blast radius of an
edit would be everything rather than the block that was edited.

Within a workspace the previous rule stands: its folders are established
together and released together, and if any fails, those already mounted for it
are released. A workspace is either serving or not, because a half-served
workspace is a view that lies about what is available.

A failed workspace is not retried. It is reported, and it is established on the
next reconciliation — by which time the reason it failed has been fixed or has
not.

## Being reached

The daemon listens on a unix socket at `$XDG_RUNTIME_DIR/airfs/control.sock`.

The socket is the liveness check, and it is self-verifying in the same way the
mount table is: a socket left behind by a dead daemon refuses connections, so
"can I connect" and "is it alive" are the same question. There is no PID file,
because a PID file answers that question with something that can be wrong.

`$XDG_RUNTIME_DIR` is the right home for it: the directory is per-user, mode
`0700`, and cleared when the session ends, so a stale socket cannot outlive the
boot that produced it. When `XDG_RUNTIME_DIR` is unset the daemon refuses to
start rather than choosing a world-writable fallback.

The protocol carries three requests — reload, status, shut down — and their
replies. It is a private implementation detail between a daemon and the CLI
built from the same source: it is not documented for third parties, not
versioned, and a client that finds a socket it cannot speak to reports a version
mismatch and says to restart the daemon. A program that wants the merged view
reads the mounted directory, and a program that wants the model uses the SDK.

The status reply carries what only the daemon knows:

- **The path of the configuration it loaded**, and when it last reloaded it.
  This is the daemon's own answer, not a path the client recomputed. A daemon
  started with an explicit configuration holds that file for its whole life, so
  the file a person is editing and the file being served can differ — and every
  symptom of that looks like `airfs` ignoring an edit. Nothing else on the
  machine records which file a running daemon read.
- **Every workspace that file declares**, each as enabled or disabled, and
  established or not, with the reason for each it could not establish.

Neither is recoverable from the filesystem, which is why the daemon must be
asked. Everything else `status` reports — that a daemon is or is not running,
and every `airfs` mount on the machine — comes from the socket and the mount
table respectively, so a dead daemon is a fact to report rather than an error
that prevents reporting.

## Lifecycle

**Starting** resolves the configuration, reconciles, and then serves. It runs in
the foreground by default, and detaches on request, returning once the first
reconciliation is complete — so a caller that succeeds knows the workspaces are
established, or knows which ones were not.

Starting when a daemon is already running is refused, and the refusal names the
running one. Two daemons reconciling the same machine against the same file
would fight: each would find the other's mounts unaccounted for and release
them.

**Reloading** re-reads the configuration from disk and reconciles. A
configuration that does not parse or does not validate changes nothing at all —
neither the running mounts nor the daemon's idea of what is declared — and is
reported to the caller that asked for the reload. Reload is explicit; the daemon
does not watch the file. An editor writing a file is not one event, and a daemon
that reacted to the first of them would act on half a document.

Every declarative command of [cli.md](cli.md) — `add`, `rm`, `enable`,
`disable` — writes the file and then reloads, so an edit takes effect without a
second command. Each is therefore the same operation with a different mutation
in the middle: read the file, apply the change, validate the result, write it,
reload. A command that finds no daemon running has still written the file, and
says so rather than failing: the declaration is the durable half, and serving it
is what `up` is for.

**Stopping** releases every mount the daemon holds, then exits. A stop that
cannot release a mount reports which, and still exits: a daemon that refuses to
die because a directory is busy is worse than a stale mount, which is
recoverable.

Termination by a signal that permits cleanup unmounts first. Termination that
does not — `SIGKILL`, a crash, power loss — leaves stale mounts, which the next
reconciliation releases.

**Across reboots**, nothing survives, and the daemon supervises nothing. A
systemd user unit is provided so that a host with systemd can start the daemon
at login and reload it with `systemctl --user reload airfs`; it is one file,
`airfs.service`, and installing it is optional. The daemon does not require
systemd, does not detect it, and behaves identically under it.

## Non-goals

- Watching the configuration file, watching source directories, or any
  event-driven reconciliation. Reads through the mount are always current, per
  [fuse-mount.md](fuse-mount.md), so a source that changes needs no reaction;
  only the *set* of workspaces does, and that changes when a person edits a
  file.
- Retrying a failed workspace on a timer, backing off, or holding a queue of
  work. Reconciliation is a function of the configuration and the mount table,
  and running it again is the retry.
- A daemon per workspace, or supervising one.
- Serving other users, a system-wide daemon, or anything requiring root.
- A stable, documented control protocol. The socket is private; the SDK is the
  supported programmatic interface.
- Reporting or acting on what is *inside* a workspace beyond establishing it —
  no access logs, no metrics, no notification when a source changes.
