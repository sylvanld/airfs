# Layered resources

## Purpose

AI agent tooling expects to find its resources — skills, agents, commands,
prompts — under one directory per category, and different tools expect different
categories under different names. In practice those resources are authored in
several different git repositories, each of which owns its own subset. Copying
them into one place drifts from the originals; hand-made symlinks drift from the
source list.

`airfs` presents the resources of many repositories as one merged, read-only view,
without moving or duplicating anything. Each resource keeps living in — and is
edited in — the repository that owns it.

## Scope

This spec defines the vocabulary and the precedence model that every other spec
builds on. It does not define the configuration file format
([workspace-config.md](workspace-config.md)), the merge algorithm
([layered-fs.md](layered-fs.md)), how the view is exposed
([fuse-mount.md](fuse-mount.md)), or what establishes it
([daemon.md](daemon.md)).

## Vocabulary

**Source** — one contributing directory tree, normally a git repository working
copy. Sources are an *ordered* list; the order is the precedence order. When the
stack rather than the individual tree is meant, the same thing is called a
*layer*.

**Folder** — one subdirectory name that is merged and exposed, shared by the
sources and the target: `<source>/<folder>/` is merged and served at
`<target>/<folder>/`. Which folders a workspace carries is declared per
workspace, so that a workspace serving a tool that expects `prompts/` and one
serving a tool that expects `skills/` can be built from the same sources. No
other directory of a source participates, which is what keeps a repository's
documentation, CI configuration and own tooling out of the view.

`airfs` attaches no meaning to a folder name. It merges the directory called
what you called it, and folders differ only in name.

A source that lacks one of the workspace's folders contributes nothing to it.
The directory is **not** created: `airfs` performs no write against a source.

**Entry** — one resource within a folder, identified by the name it presents
directly under that folder. An entry may be a directory — a skill is a directory
containing `SKILL.md` and its supporting files — or a single file, since agent
tooling commonly stores a command or an agent as one Markdown document. Which of
the two it is comes from the source; `airfs` imposes neither. The entry is the
unit that collides and the unit that is shadowed, and it is always the *whole*
name: a directory entry wins with its entire subtree.

**Stratum** — one source's contribution to one folder: the
`<source>/<folder>/` directory. A folder's merged view is the ordered stack of
its strata.

**Target** — the directory a workspace's merged view is exposed under, holding
one subdirectory per folder and nothing else. The target is not a mountpoint;
each of its folder directories is.

**Workspace** — a named target, an ordered list of sources, and the folders it
carries. It is the unit that is declared, established, and reported on; a
machine has as many as its configuration names, and they are independent of one
another.

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
3. Precedence is independent per folder. A source may win `commit` under
   `skills` while losing `commit` under `commands`.
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

- Writing to a source at all, whether content or an empty directory, and any
  write policy that would pick a destination branch.
- Merging the *contents* of two colliding entries — appending sections, resolving
  frontmatter, or taking a union of a directory's files.
- Selecting a subset of a source's entries. A source contributes every entry of
  every folder the workspace declares; curation is which sources a workspace
  layers, not which entries survive the layering.
- Understanding what a resource means. `airfs` layers directories; it does not
  parse `SKILL.md`, validate frontmatter, or know which fields an agent
  definition needs. Folders differ only in name.
- Fetching, cloning, or updating sources. A source is a directory that already
  exists on disk; keeping it current is git's job.
