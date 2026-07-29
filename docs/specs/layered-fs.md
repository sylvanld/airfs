# Layered filesystem

## Purpose

The merged view, as an ordered read-only union of filesystem trees. This is the
core of the SDK: the mount and any validator or indexer reads through this one
abstraction, so the merge semantics are defined and tested once.

## Scope

How a lookup and a directory listing resolve across an ordered stack of trees,
and what happens at the edges. It does not cover where the stack comes from
([workspace-config.md](workspace-config.md)) or how the result is exposed to
other processes ([fuse-mount.md](fuse-mount.md)).

## Interface

The union is expressed as the standard library's read-only filesystem interface,
and it composes over layers expressed the same way. Two consequences follow, and
both are deliberate:

- A layer need not be a directory on disk. An in-memory tree is a valid layer,
  which is what allows the merge semantics to be tested exhaustively without
  mounting anything, without root, and without touching a real filesystem.
- Anything that consumes a filesystem can consume the merged view directly,
  in-process, with no mount involved.

One union is built per folder of a workspace, over that folder's strata in
declared order. A union knows nothing about folders, workspaces, targets, or
configuration; it is the stack it was given.

The union additionally exposes, for each name it resolves, which layer the name
came from. Frontends need this: the mount needs a backing file to read through
to, and shadowing reports need to name the winner and the losers.

## Resolution

**Lookup.** Layers are consulted in reverse order and the last layer containing
the name wins, per [layered-resources.md](layered-resources.md). The type of the
winning entry is whatever that layer says it is: a name that is a directory in one
layer and a file in a later one resolves as a file, with no attempt to reconcile
the disagreement.

**Listing.** A directory listing is the merge of that directory's listing in
every layer that has it, deduplicated by name, with the last occurrence winning
so that a listed name and a looked-up name always resolve to the same layer.
Iteration order is lexical by name — not layer order, and not the order any
underlying filesystem happens to return — so that a listing is reproducible and
diffable across machines. A directory that exists in no layer does not exist.

**Depth of merging.** The union merges the root of the stack and stops there.
Names directly under the root — the entries of a folder — resolve to exactly one
layer, whether the name is a file or a directory, and a directory entry's subtree
comes wholly from that layer with no deeper merging inside it. This is the rule
from
[layered-resources.md](layered-resources.md) expressed in the tree: folder
directories merge, entries do not. There is no configurable depth; the entry is
the unit, always.

**Metadata.** Size, modification time, and mode come from the winning layer
unchanged. Permissions are not rewritten, with one exception: write permission
bits are cleared, because the view cannot honour them and reporting a writable
file that rejects every write is worse than reporting the truth.

## Edge cases

- **Symlinks within a layer** are ordinary entries and are served as the
  underlying layer serves them.
- **Dotfiles** are ordinary entries and are neither hidden nor filtered.
- **Case** is treated as the underlying layer treats it. The union does not impose
  case-insensitive matching.
- **An entry named identically in two layers with different types** is a
  shadowing event like any other, and is reported as one, because a directory
  shadowing a file is far more likely to be a mistake than an intention.
- **A layer whose root does not exist** contributes nothing, and the union of
  the remaining layers is served normally. This is the ordinary case rather than
  a broken one: a workspace declares the folders it wants merged, a source
  contains the ones it has, and airfs creates neither. It is also what lets a
  folder added to a source appear through the view without re-establishing it.
- **A layer that becomes unreadable while the view is live** — permissions
  revoked, the underlying device gone — yields errors from the operations that
  touch it, not a silently different view. The union does not cache the
  existence of names in order to paper over it. A source repository that was
  moved or deleted falls under the case above and reads as empty; that it is
  gone is reported by the daemon reconciling it, per
  [daemon.md](daemon.md), which is where a missing *source* is a fact about the
  configuration rather than about one folder.
- **Concurrent reads** are safe: a union is immutable once constructed, and reads
  hold no shared mutable state. Changing the set of layers means constructing a
  new union.

## Freshness

The union holds no content cache and no directory cache. A read reaches the
backing layer, so an edit to a file in a source repository is visible through the
merged view immediately, with no refresh. This is a requirement, not an
optimisation trade-off: the editing loop in
[layered-resources.md](layered-resources.md) depends on it, and the cost of
re-reading a small Markdown file is not worth the class of bug that stale caching
introduces.

Adding or removing a *layer* requires a new union, since the layer set is
immutable.

## Shadowing report

A union can enumerate every shadowed entry: the name, the winning layer, and the
losing layers in order. The report is derived by listing each layer's root and
collecting names that appear in more than one. It is computed on demand rather
than at construction, so that building a union stays cheap and no frontend pays
for a report it does not use.

## Non-goals

- Writes of any kind, and therefore any write policy, copy-up, or whiteout
  mechanism.
- Content-level merging of colliding entries.
- Caching for performance. If a frontend needs caching, that frontend's spec
  states its invalidation rules; the union stays cache-free.
- Watching layers for changes. Reads are always current, which removes the need.
