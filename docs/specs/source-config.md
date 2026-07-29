# Source configuration

> **Superseded by [workspace-config.md](workspace-config.md).**
>
> This spec placed one `sources.txt` at the root of each target: a target carried
> its own declaration, and a second workspace was a second folder with its own
> file. It is kept for history, and because the code implementing it is what
> [workspace-config.md](workspace-config.md) replaces.

## Why it was superseded

Three properties of this design did not survive contact with more than one
workspace:

**No inventory.** A declaration that lives inside the thing it produces can only
be found by already knowing where to look. There was no way to ask what `airfs`
did on a machine short of walking the filesystem for `sources.txt` files, and no
single artifact to review in git.

**Fixed kinds.** `agents`, `skills`, `commands` and `scripts` were built in and
not configurable, on the reasoning that a repository opts out of a kind by
leaving it empty. That holds for opting out and not for opting in: a tool
expecting `prompts/` could not be served at all. Folders are now declared per
workspace.

**Writing into sources.** Resolution created a source's missing kind
subdirectories, so that every source contributed every kind and the stack was
uniform. It meant a read-only command wrote empty directories into every
repository it was pointed at, and with folder names now arbitrary it would
scatter directories nobody asked for. Resolution no longer writes to a source at
all; a source lacking a folder contributes nothing to it.

What did survive, and is carried into
[workspace-config.md](workspace-config.md) unchanged: the path expansion rules
(`~`, `$VAR` with an unset variable being an error, relative paths resolving
against the file's own directory), duplicate sources being an error rather than
silently collapsed, and sources being named explicitly rather than discovered.

`mount --source`, which replaced a target's whole source list from the command
line, survives as `airfs add` in [cli.md](cli.md) — with its replace-never-merge
rule and its path-writing rules intact. What it loses is the verb: it declared a
workspace and served it under one word, which was defensible when the file it
rewrote described exactly the thing being mounted, and is not now that mounts
are established by a daemon reconciling a file that describes the machine.
