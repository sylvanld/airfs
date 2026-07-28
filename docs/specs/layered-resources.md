# Layered resources

## Purpose

AI agent tooling expects to find its resources — skills, agents, commands — under
one directory per kind. In practice those resources are authored in several
different git repositories, each of which owns its own subset. Copying them into
one place drifts from the originals; hand-made symlinks drift from the source
list.

`airfs` presents the resources of many repositories as one merged, read-only view,
without moving or duplicating anything. Each resource keeps living in — and is
edited in — the repository that owns it.

## Scope

This spec defines the vocabulary and the precedence model that every other spec
builds on. It does not define the configuration file format
([source-config.md](source-config.md)), the merge algorithm
([layered-fs.md](layered-fs.md)), or how the view is exposed
([fuse-mount.md](fuse-mount.md), [symlink-farm.md](symlink-farm.md)).

## Vocabulary

**Source** — one contributing directory tree, normally a git repository working
copy. Sources are an *ordered* list; the order is the precedence order.

**Kind** — a category of resource, corresponding to one subdirectory name within
a source: `skills`, `agents`, `commands`. Kinds are declared explicitly rather
than discovered, so that unrelated top-level directories in a source repository
(documentation, CI configuration, the repository's own tooling) can never leak
into the merged view.

**Entry** — one resource within a kind, identified by the name it presents to the
merged view. An entry's *granularity* is a property of its kind: a kind is either
directory-granular (a skill is a directory containing `SKILL.md` and its
supporting files) or file-granular (an agent is a single Markdown file). The
granularity determines what unit collides, what unit is shadowed, and what unit a
symlink points at.

**Stratum** — one source's contribution to one kind: the `<source>/<kind>/`
directory. A kind's merged view is the ordered stack of its strata. A source that
does not have a directory for a kind simply contributes no stratum for it; this
is normal and not an error.

**Shadowing** — when two or more strata contain an entry with the same name, the
entry from the earliest source in the order wins and is the only one visible.
The others are said to be *shadowed*.

## Precedence model

1. Earlier sources win. The order is exactly the order the sources are declared
   in, with no reordering, no scoring, and no dependence on the filesystem.
2. Precedence applies at entry granularity, not at path granularity. When a
   directory-granular entry wins, it wins whole: the losing stratum's version of
   that directory contributes nothing, not even files the winner happens to lack.
   A skill is a unit; a half-merged skill assembled from two repositories would
   be a skill that exists in no repository and that nobody can test.
3. Precedence is independent per kind. A source may win `commit` under `skills`
   while losing `commit` under `commands`.
4. Shadowing is reported, never silent. Any operation that builds a merged view
   can enumerate the shadowed entries, with the winning and losing source for
   each. Silent shadowing is the failure mode that makes a merged view
   untrustworthy: a contributor edits a file and sees no effect, with nothing to
   explain why.

## Read-only

The merged view is read-only. Writes are rejected rather than routed to a
source.

The reason is that the destination is ambiguous — a new file in the merged view
belongs to *some* repository, and the view has no basis on which to choose — and
that a write which succeeded would bypass the owning repository's git history,
review, and tests. Editing happens in the source repository, where the resource
belongs and where the tooling that validates it lives.

Edits to an existing resource must become visible through the merged view without
any re-synchronisation, because that is the loop a contributor is actually in:
change the file in its repository, invoke the agent, observe the effect. Adding
or removing a whole source, or a whole entry, may require refreshing the view;
which of those refreshes are needed is a property of the frontend and is stated
in that frontend's spec.

## Non-goals

- Writing through to sources, and any write policy that would pick a destination
  branch.
- Merging the *contents* of two colliding entries — appending sections, resolving
  frontmatter, or taking a union of a directory's files.
- Understanding what a resource means. `airfs` layers directories and files; it
  does not parse `SKILL.md`, validate frontmatter, or know which fields an agent
  definition needs. Kinds differ only in granularity.
- Fetching, cloning, or updating sources. A source is a directory that already
  exists on disk; keeping it current is git's job.
