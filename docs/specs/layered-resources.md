# Layered resources

## Purpose

AI agent tooling expects to find its resources — skills, agents, commands,
scripts — under one directory per kind. In practice those resources are authored
in several different git repositories, each of which owns its own subset. Copying
them into one place drifts from the originals; hand-made symlinks drift from the
source list.

`airfs` presents the resources of many repositories as one merged, read-only view,
without moving or duplicating anything. Each resource keeps living in — and is
edited in — the repository that owns it.

## Scope

This spec defines the vocabulary and the precedence model that every other spec
builds on. It does not define the configuration file format
([source-config.md](source-config.md)), the merge algorithm
([layered-fs.md](layered-fs.md)), or how the view is exposed
([fuse-mount.md](fuse-mount.md)).

## Vocabulary

**Source** — one contributing directory tree, normally a git repository working
copy. Sources are an *ordered* list; the order is the precedence order. When the
stack rather than the individual tree is meant, the same thing is called a
*layer*.

**Kind** — a category of resource, corresponding to one subdirectory name within
a source. The kinds are fixed and built into `airfs`: `agents`, `skills`,
`commands`, `scripts`. They are not configurable, and no other top-level
directory of a source participates in the view — which is what keeps a
repository's documentation, CI configuration and own tooling out of it.

Every source contributes every kind. A source that lacks a kind's subdirectory
has it created, empty, so that the stack is uniform and adding a resource of a
new kind to a repository is a `mkdir`-free act. This is the only write `airfs`
performs against a source, and an empty directory is invisible to git, so it
produces no diff.

**Entry** — one resource within a kind, identified by the name it presents
directly under that kind. An entry may be a directory — a skill is a directory
containing `SKILL.md` and its supporting files — or a single file, since agent
tooling commonly stores a command or an agent as one Markdown document. Which of
the two it is comes from the source; `airfs` imposes neither. The entry is the
unit that collides and the unit that is shadowed, and it is always the *whole*
name: a directory entry wins with its entire subtree.

**Stratum** — one source's contribution to one kind: the `<source>/<kind>/`
directory. A kind's merged view is the ordered stack of its strata.

**Target** — the directory the merged view is exposed under, holding one
subdirectory per kind. The target is not a mountpoint; each of its kind
directories is.

**Shadowing** — when two or more strata contain an entry with the same name, the
entry from the *last* source in the order wins and is the only one visible. The
others are said to be *shadowed*.

## Precedence model

1. Later sources win. The order is exactly the order the sources are declared
   in, with no reordering, no scoring, and no dependence on the filesystem.
   Sources are written from the most general to the most specific — global,
   then organisation, then project — so that the most local declaration of a
   resource is the one that takes effect. This is the direction every layered
   configuration system a contributor already knows uses, and inverting it would
   make an override require an edit at the top of the file rather than the
   bottom.
2. Precedence applies at entry granularity, not at path granularity. When an
   entry wins, it wins whole: the losing strata's version of that entry
   contributes nothing, not even files the winner happens to lack. A skill is a
   unit; a half-merged skill assembled from two repositories would be a skill
   that exists in no repository and that nobody can test. An entry that is a
   directory in one stratum and a file in another still resolves to exactly one
   of them — the last — with no reconciliation attempted.
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
or removing a whole source requires re-establishing the view; adding or removing
an entry does not.

## Non-goals

- Writing through to sources, and any write policy that would pick a destination
  branch. The one exception is creating a source's missing kind directories,
  which adds no content.
- Configurable kinds. The set is fixed; a repository opts out of a kind by
  leaving that directory empty.
- Merging the *contents* of two colliding entries — appending sections, resolving
  frontmatter, or taking a union of a directory's files.
- Understanding what a resource means. `airfs` layers directories; it does not
  parse `SKILL.md`, validate frontmatter, or know which fields an agent
  definition needs. Kinds differ only in name.
- Fetching, cloning, or updating sources. A source is a directory that already
  exists on disk; keeping it current is git's job.
