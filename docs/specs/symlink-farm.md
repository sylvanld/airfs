# Symlink farm

## Purpose

Materialise the merged view as an ordinary directory of symlinks, for hosts where
a FUSE mount is unavailable — no `/dev/fuse`, no setuid helper, a container
without the device, a CI runner. It is the fallback frontend, not the primary one.

## Scope

How the merged view is reconciled into a real directory of links, what the
reconciliation owns and refuses to touch, and how it differs from a mount. The
view being materialised is [layered-fs.md](layered-fs.md); the mount it falls back
from is [fuse-mount.md](fuse-mount.md).

## Shape

The target directory contains one subdirectory per kind, and within it one symlink
per visible entry, pointing at that entry inside the source repository that won
it. Links are created at the kind's entry granularity: a directory-granular entry
is one link to a directory, a file-granular entry is one link to a file.

Linking at entry granularity rather than per file is what makes the farm useful
rather than merely correct. Because a link points at the entry, edits to files
inside a skill — and files added inside it — are visible through the farm
immediately, with no reconciliation. Only adding or removing a whole entry, or a
whole source, requires reconciling again. This is finer-grained than the mergerfs
setup it replaces, which needed a full remount whenever the source list changed.

Links are absolute, so that the farm does not break if it is moved.

## Reconciliation

Reconciliation makes the target directory match the merged view, and is
idempotent: running it twice in a row makes no changes the second time.

The distinguishing property against a hand-made symlink tree — which drifts
precisely because nobody re-runs it — is ownership. Reconciliation owns the links
it created and nothing else:

- A link that the view no longer contains is removed.
- A link that points somewhere other than where the view says is repointed.
- A missing link is created.
- Anything in the target directory that is *not* a link the farm created — a real
  file, a real directory, a link to somewhere outside any declared source — is
  never modified or deleted. It is reported as foreign, and the reconciliation
  fails rather than proceeding, because a target directory containing unexpected
  real content is more likely to be the wrong directory than a directory needing
  cleanup.

Every change is reported: what was created, repointed, removed, and what was
skipped as foreign. A verification mode performs no writes and exits non-zero if
any change would be needed, which is what makes drift detectable from CI.

## Difference from a mount

The farm does not provide the kernel-enforced read-only guarantee of
[fuse-mount.md](fuse-mount.md). A write through a link follows it into the source
repository and succeeds. Nothing in userspace can prevent that.

This is the reason the farm is the fallback rather than the default, and it must be
stated wherever the farm is offered rather than discovered later by someone whose
agent tooling wrote into a git working copy. What partially compensates:
verification mode in CI detects a farm that has drifted, and the source
repository's own git status makes an accidental write visible as an unexpected
diff. Neither prevents the write.

The second difference is refresh: a mount reflects a new entry with no action,
while the farm needs reconciling. Both reflect edits to existing files
immediately.

## Non-goals

- Copying or hard-linking content. A copy drifts from its source and a hard link
  cannot span filesystems or point at a directory; a symlink is the only mechanism
  that keeps the source authoritative.
- Cleaning a target directory of foreign content, interactively or otherwise.
- Per-file links inside a directory-granular entry, which would make new files
  inside a skill require reconciliation.
- Watching for changes and reconciling automatically. Reconciliation is explicit,
  so that its report is read by whoever asked for it.
- Emulating read-only enforcement by manipulating source permissions. Making a
  git working copy read-only to protect a merged view would be a surprising and
  damaging side effect on the repository.
