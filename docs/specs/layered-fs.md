# Layered filesystem

## Purpose

The merged view, as an ordered read-only union of filesystem trees. This is the
core of the SDK: every frontend — the FUSE mount, the symlink farm, any validator
or indexer — reads through this one abstraction, so the merge semantics are
defined and tested once.

## Scope

How a lookup and a directory listing resolve across an ordered stack of trees,
and what happens at the edges. It does not cover where the stack comes from
([source-config.md](source-config.md)) or how the result is exposed to other
processes ([fuse-mount.md](fuse-mount.md), [symlink-farm.md](symlink-farm.md)).

## Interface

The union is expressed as the standard library's read-only filesystem interface,
and it composes over layers expressed the same way. Two consequences follow, and
both are deliberate:

- A layer need not be a directory on disk. An in-memory tree is a valid layer,
  which is what allows the merge semantics to be tested exhaustively without
  mounting anything, without root, and without touching a real filesystem.
- Anything that consumes a filesystem can consume the merged view directly,
  in-process, with no mount involved.

The union additionally exposes, for each name it resolves, which layer the name
came from. Frontends need this: the symlink farm needs a real path to point at,
the FUSE mount needs a backing file to read through to, and shadowing reports
need to name the winner and the losers.

## Resolution

**Lookup.** Layers are consulted in order and the first layer containing the name
wins. Resolution stops there: later layers are not consulted, and the type of the
winning entry is whatever that layer says it is. A name that is a directory in the
first layer and a file in the second resolves as a directory, with no attempt to
reconcile the disagreement.

**Listing.** A directory listing is the ordered merge of that directory's listing
in every layer that has it, deduplicated by name, with the first occurrence
winning. Iteration order is lexical by name — not layer order, and not the order
any underlying filesystem happens to return — so that a listing is reproducible
and diffable across machines. A directory that exists in no layer does not exist.

**Depth of merging.** Merging applies at every level of the tree, not only at the
top. But at the entry granularity a kind declares, the winner wins whole: when a
directory-granular entry resolves to a layer, that entry's subtree comes entirely
from that layer, and no deeper merging happens inside it. This is the rule from
[layered-resources.md](layered-resources.md) expressed in the tree: kind
directories merge, entries do not. A frontend that layers whole kind trees
without knowing about kinds gets plain recursive merging; one that knows the
granularity gets whole-entry semantics. The union supports being configured with
the depth at which merging stops.

**Metadata.** Size, modification time, and mode come from the winning layer
unchanged. Permissions are not rewritten, with one exception: write permission
bits are cleared, because the view cannot honour them and reporting a writable
file that rejects every write is worse than reporting the truth.

## Edge cases

- **Symlinks within a layer** resolve within that layer, and a link that escapes
  its layer's root is not followed. Following it would let a source repository
  reach arbitrary paths on the machine through the merged view, which is both a
  correctness problem and a disclosure problem.
- **Dotfiles** are ordinary entries and are neither hidden nor filtered.
- **Case** is treated as the underlying layer treats it. The union does not impose
  case-insensitive matching, but two entries in the same directory whose names
  differ only by case are reported as a collision when the platform cannot
  distinguish them.
- **An entry named identically in two layers with different types** is a
  shadowing event like any other, and is reported as one, because a directory
  shadowing a file is far more likely to be a mistake than an intention.
- **A layer that disappears while the view is live** yields errors from the
  operations that touch it, not a silently different view. The union does not
  cache the existence of names in order to paper over a source repository that
  was moved or deleted.
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

A union can enumerate every shadowed entry: the name, the kind directory it sits
in, the winning layer, and the losing layers in order. The report is derived by
walking to the merge depth and collecting names that appear in more than one
layer. It is computed on demand rather than at construction, so that building a
union stays cheap and no frontend pays for a report it does not use.

## Non-goals

- Writes of any kind, and therefore any write policy, copy-up, or whiteout
  mechanism.
- Content-level merging of colliding entries.
- Caching for performance. If a frontend needs caching, that frontend's spec
  states its invalidation rules; the union stays cache-free.
- Watching layers for changes. Reads are always current, which removes the need.
