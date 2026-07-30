# FUSE mount

## Purpose

Expose a merged view at a real filesystem path, so that tools which are not
written in Go — the agent tooling this project exists to serve — can read it with
ordinary file operations. A kernel mount is the only mechanism that makes an
in-process view visible to an unrelated process at a path.

## Scope

Serving read-only filesystems, including but not limited to the per-folder
unions of [layered-fs.md](layered-fs.md), as FUSE mounts: what a mount
guarantees, what it requires of the host, and how one starts and stops. Which
mounts should exist, and what establishes them, is [daemon.md](daemon.md).

## Shape

One mount per folder of a workspace. The target directory holds one subdirectory
per declared folder — `<target>/skills`, `<target>/agents`, and whatever else the
workspace names — and each of those is a mountpoint serving that folder's union.
The target itself is an ordinary directory and is never mounted over.

A workspace's mounts are established together and released together: a workspace
is either serving or not, since a partially mounted workspace is a view that lies
about what is available.

Every mount on the machine, across every workspace, is served by one process, per
[daemon.md](daemon.md). Nothing about a mount depends on that — a mount is
identical whichever process holds it — but it is why this spec describes a mount
rather than a lifetime.

## Pure Go, no install step beyond `go install`

The mount speaks the FUSE protocol over `/dev/fuse` from Go, linking no C library
and requiring no filesystem binary to be installed — specifically, it replaces a
`mergerfs` installation, which previously had to be unpacked by hand into a
user-local prefix because the distribution package needs root.

Go dependencies are permitted, including a FUSE protocol library, provided they
are cgo-free and add nothing to installation: `go install` must remain the whole
of it, on a machine with no compiler toolchain beyond Go and no system package
installed for `airfs`'s sake. Writing the protocol by hand is not a requirement;
not needing a C toolchain is.

What remains required is what the host already provides:

- `/dev/fuse`, readable and writable by the invoking user.
- A setuid `fusermount3` helper, which performs the privileged mount syscall on
  behalf of an unprivileged process. This ships with the system FUSE package and
  is present on a normal desktop Linux installation.

Neither can be worked around from userspace: an unprivileged process cannot mount
without a setuid helper. A host that lacks either cannot be mounted on, and the
correct response is to say so precisely — which requirement is missing, and what
provides it. Reporting the requirements is why a preflight check exists as its
own operation rather than as a mount failure message only.

Running as root is neither required nor expected. A mount created by an
unprivileged user is visible only to that user, which is the correct scope for
resources read by that user's tooling.

## Guarantees

**Read-only, enforced by the kernel.** Every mutating operation fails with the
error a read-only filesystem returns. This is stronger than a convention: no
process can write through the mount even by mistake, which is the property that
makes the merged view safe to point tooling at.

**Freshness.** The mount adds no caching of its own beyond what correctness
requires, and configures the kernel not to cache file content, so an edit in a
source repository is visible through the mount on the next read. Attribute and
directory-entry caching is likewise disabled or held to a duration short enough
that a newly added entry appears without intervention. The trade-off is
deliberate: these are small text files read occasionally by agent tooling, so
throughput is irrelevant and staleness is a bug that presents as "my edit did
nothing".

**Read-through for real files.** When a name resolves to a layer backed by a real
path, reads are served from the backing file rather than copied through
user-space buffers held by the mount. This keeps memory-mapping correct, keeps
reported sizes honest, and keeps large files from being materialised in memory.
A layer that is not backed by a real path is served generically. Detecting the
backed case is possible because the union reports which layer, and which path,
each name came from.

**Ownership and mode.** Every entry is reported as owned by the invoking user,
whose mount it is and who is the only one who can see it. Modes come from the
winning layer with write bits cleared, per [layered-fs.md](layered-fs.md).

**Stable identity.** Inode numbers are stable for the lifetime of a mount, so
that tools which cache by inode are not misled. They are not stable across
mounts, and nothing may depend on that.

## Lifecycle

Establishing a workspace takes its target, its folders, and its resolved sources.
The target and its missing folder directories are created. A folder directory
that exists and is not empty is refused, because mounting over populated
directories hides their contents and the hidden files are a trap; a folder
already serving this view is detected and reported rather than stacked. If any
folder fails to mount, every folder already mounted for that workspace is
released before reporting, so a failed attempt leaves no mount behind. Folder
directories it created on the way are not removed: they are empty, the next
attempt mounts over them, and deleting a directory to undo a failure risks
deleting one that was already there.

Releasing a workspace unmounts every folder under its target, and is safe to
invoke when nothing is mounted, reporting that fact rather than failing.

**Finding a mount holds no state.** Whether something is served, and where, is
read from the kernel's mount table: `airfs` sets a recognisable filesystem name at
mount time, and every mount bearing it is an `airfs` mount. There is no PID file
and no registry, because a second record of what is mounted would disagree with
the kernel exactly when it matters — after a crash, after a manual
`fusermount3 -u`, or after the configuration changed while nothing was running.

This is also what lets every `airfs` mount on the machine be enumerated without
knowing which configuration produced it, which is what
[daemon.md](daemon.md) reconciles against. The kernel is the inventory; the
configuration is the intent.

A mount whose serving process has died leaves a stale mountpoint: still listed by
the kernel, but failing every access with the error a severed FUSE connection
returns. That signature distinguishes a stale mount from a live one and from an
ordinary directory, and unmounting recovers it. Recovering from that state is part
of this spec's surface, not something a contributor should be expected to fix with
manual commands.

A mount lives as long as the process serving it. It does not survive a reboot,
and re-establishing it at login is left to the host's service manager; the SDK
supervises nothing.

## Non-goals

- Writes, and therefore write policies, copy-up, and whiteouts.
- Mounting for other users, system-wide mounts, or anything requiring root.
- Performance tuning: caching layers, readahead, or attribute caching beyond what
  correctness requires. Freshness wins every time it conflicts with throughput.
- Supervision, auto-remount, or reboot persistence. A mount is held by a process
  and dies with it; keeping that process alive is [daemon.md](daemon.md)'s
  concern, and keeping it alive across reboots is the host's.
- Non-Linux hosts. The mechanism is Linux FUSE; macOS would require a third-party
  kernel extension, which is exactly the class of dependency this design removes.
