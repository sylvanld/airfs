# FUSE mount

## Purpose

Expose a merged view at a real filesystem path, so that tools which are not
written in Go — the agent tooling this project exists to serve — can read it with
ordinary file operations. A kernel mount is the only mechanism that makes an
in-process view visible to an unrelated process at a path.

## Scope

Serving any read-only filesystem, including but not limited to the union of
[layered-fs.md](layered-fs.md), as a FUSE mount: what the mount guarantees, what
it requires of the host, and how it starts and stops.

## Pure Go, no third-party binary

The mount is implemented by speaking the FUSE protocol over `/dev/fuse` directly
from Go. It links no C library and requires no filesystem binary to be installed
— specifically, it replaces a `mergerfs` installation, which previously had to be
unpacked by hand into a user-local prefix because the distribution package needs
root.

What remains required is what the host already provides:

- `/dev/fuse`, readable and writable by the invoking user.
- A setuid `fusermount3` helper, which performs the privileged mount syscall on
  behalf of an unprivileged process. This ships with the system FUSE package and
  is present on a normal desktop Linux installation.

Neither can be worked around from userspace: an unprivileged process cannot mount
without a setuid helper. A host that lacks either cannot be mounted on, and the
correct response is to say so precisely — which requirement is missing, and what
provides it — and to point at [symlink-farm.md](symlink-farm.md) as the frontend
that needs neither. Reporting the requirements is why a preflight check exists as
its own operation rather than as a mount failure message only.

Running as root is neither required nor expected. A mount created by an
unprivileged user is visible only to that user, which is the correct scope for
resources read by that user's tooling.

## Guarantees

**Read-only, enforced by the kernel.** Every mutating operation fails with the
error a read-only filesystem returns. This is stronger than a convention: no
process can write through the mount even by mistake, which is the property that
makes the merged view safe to point tooling at, and the property that a symlink
farm cannot provide.

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

**Stable identity.** Inode numbers are stable for the lifetime of a mount, so
that tools which cache by inode are not misled. They are not stable across
mounts, and nothing may depend on that.

## Lifecycle

Mounting takes a mountpoint and a filesystem, and is idempotent in the sense that
mounting over an existing mount of the same view is detected and reported rather
than stacked. A mountpoint that does not exist is created; one that exists and is
not empty is refused, because mounting over a populated directory hides its
contents and the hidden files are a trap.

Unmounting releases the mount and is safe to invoke when nothing is mounted,
reporting that fact rather than failing. A mount whose serving process has died
leaves a stale mountpoint that reports an error on every access; recovering from
that state — detecting it and unmounting it — is part of this spec's surface, not
something a contributor should be expected to fix with manual commands.

The mount lives as long as the process serving it. It does not survive a reboot,
and re-establishing it at login is left to the host's service manager; the SDK
provides no supervision of its own.

Interrupting the serving process unmounts cleanly. Termination that does not
allow cleanup is the stale-mountpoint case above.

## Non-goals

- Writes, and therefore write policies, copy-up, and whiteouts.
- Mounting for other users, system-wide mounts, or anything requiring root.
- Performance tuning: caching layers, readahead, or attribute caching beyond what
  correctness requires. Freshness wins every time it conflicts with throughput.
- Supervision, auto-remount, or reboot persistence.
- Non-Linux hosts. The mechanism is Linux FUSE; macOS would require a third-party
  kernel extension, which is exactly the class of dependency this design removes.
