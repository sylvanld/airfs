# Linking project skills to tools

## Purpose

A project keeps its own AI resources in a directory it owns and commits —
`.ai/`, holding `skills/`. Every tool looks for skills somewhere else, and
somewhere different from every other tool: `.claude/skills/` for one,
`.opencode/skills/` for the next. The project's skills are already on disk; they
are simply not where anything looks.

`airfs link` gathers them into one root the project owns, and makes them visible
to each tool named on the command line by creating one relative symlink per
tool. Resources a tool already holds are adopted into the root on the way, since
a project that has been using a tool for a week is the normal case, not an
error.

## Scope

The `airfs link` command, the table of tool layouts it consults, and what it
writes into a project. It has nothing to do with workspaces, sources,
precedence, mounts, or the daemon, and none of those specs are affected by it.

## Principle: a scaffold, not a declaration

Everything else `airfs` does is declarative and reconciled — the configuration
says what should be true, and the daemon makes the machine match it, repeatedly,
forever. This is not that. It is a one-shot mutation of a project directory,
recorded nowhere, reconciled by nothing, and undone with `rm`.

That is the right shape for it, because the artifact it produces — a committed
symlink — has to keep working on machines where `airfs` is not installed. A
declaration that needed a daemon to have any effect would defeat the point.

But it means `airfs status` and `airfs ls` will never mention a symlink this
created, and no command will offer to remove one. The help text says so, because
a person who assumes the usual reconciliation will wait forever for something to
notice.

It is also why the command is named for its mechanism rather than its intent. It
links; it does not initialize, declare, or serve. Nothing about a project
changes state because this ran, except that a symlink exists.

## Principle: tool knowledge is spent here and nowhere else

`airfs` attaches no meaning to any folder name — `skills` is a string a
workspace declares, and the union treats it exactly as it would treat `prompts`.
That property is load-bearing and stays.

This command is the single exception, and it is a contained one: it knows where
a tool keeps its skills, and it *spends* that knowledge producing a symlink on
disk. Nothing it knows survives into the configuration, the SDK, or the mount
layer. A tool added to the table changes what `airfs link --<tool>` writes and
changes nothing else in the program.

If a future feature wants a tool's name anywhere but here, that is the signal
the boundary is eroding.

## The tool table

Each entry is a tool's name as a flag, and the path — relative to the project
root — at which that tool expects to find skills.

Each row is a tool, a **resource type**, and the path — relative to the project
root — at which that tool expects that type. The resource type is the name of
the subdirectory under the root, so a row maps `<tool>` to
`<root>/<type>` ⇄ `<tool path>`.

| Flag | Type | Path |
| --- | --- | --- |
| `--claude` | `skills` | `.claude/skills` |
| `--opencode` | `skills` | `.opencode/skills` |

The table is deliberately short and grows by evidence: an entry is either
correct or it silently points a tool at nothing, so each one is added only
against that tool's documented layout, never against a plausible guess. A
wrong row is the worst kind of bug this command can have — the symlink is
created, the report says it succeeded, and the tool goes on seeing no skills,
because nothing looks at the path the row names.

The type column exists so that agents, commands and prompts can be added
without changing the model: a tool with three rows gets three symlinks into
three subdirectories of one root. No such rows are listed yet, because none has
been verified.

`airfs link --list` prints the table and exits, so what the binary believes
about a tool is inspectable without reading this file.

## `airfs link`

```
airfs link --claude [--<tool> …] [--root <dir>] [--dry-run]
airfs link --list
```

Operates on the current working directory, which is the project root. This is
the one command in `airfs` whose frame of reference is where you are standing,
rather than a workspace named on the command line.

At least one tool flag is required. Given none, it fails and prints the table:
guessing which tools a project uses is exactly the kind of inference that
produces a directory nobody asked for.

### What it does

1. **The resource root.** `--root` names the directory holding the project's own
   AI resources, defaulting to `.ai`. Its `skills/` subdirectory is what the
   tools are pointed at, and both are created if absent. These are the only
   directories the command creates rather than links.

   The root must resolve **inside the working directory**: an absolute path, or
   one climbing out with `..`, is refused. The symlinks are relative and meant
   to be committed, so a root outside the project produces links that resolve to
   nothing on every other machine — and adoption would move the project's
   resources somewhere the project does not contain.

   A root rather than a bare `skills/` for two reasons. One is that a project
   accumulates agents, commands and prompts as readily as skills, and a single
   `.ai/` keeps them together instead of scattering a directory per kind across
   the project root. The other is that `<root>/<folder>/` is exactly the stratum
   layout of [layered-resources.md](layered-resources.md): the directory this
   command creates is already a well-formed `airfs` **source**, so a project that
   later wants its skills merged with an organisation's declares a workspace
   listing `.ai` among its sources and moves nothing.

2. **Adoption.** A tool's path that already exists as a real directory holds
   resources someone put there, and they are the project's — they were simply
   filed under one tool. Each entry is **moved** into `<root>/<type>/`, and the
   now-empty directory is removed. See *Adoption*, below.

3. **One symlink per tool.** For each row, the symlink is created at the tool's
   path, pointing at `<root>/<type>` **relatively** — `../.ai/skills` from
   `.claude/skills`. An absolute target would resolve to a home directory that
   exists on exactly one machine, and the symlink is meant to be committed.

4. **Parent directories.** A tool's parent directory (`.claude/`) is created if
   absent. Nothing else in it is touched; a tool that already keeps settings
   there keeps them.

## Adoption

The first run in a real project is the case that matters: the tools already have
resources, and refusing to touch them would make the command useless exactly
when it is first reached for. So it adopts rather than refuses.

**Nothing is deleted.** Every adopted entry is moved, so a run that did the
wrong thing is undone with `mv`, and a run that fails part way leaves every
entry either at its old path or its new one. That is what makes adoption safe
enough to be the default rather than a flag.

### Naming, and the conflict rule

An entry keeps its name: `.claude/skills/commit` becomes `<root>/skills/commit`.

When two tools ship an entry of the same name, the second cannot have it. The
tools are processed **in the order their flags appear on the command line**, the
first to claim a name keeps it, and each later one is suffixed with its own tool
name — `commit` from `--claude`, then `commit-opencode`. The order is the one
thing the person running the command controls, so it is the one the rule turns
on, and it is stated in the report rather than left to be discovered.

A suffixed name can itself be taken — by an entry already in the root, most
likely. The same two rules apply again in the same order: identical content is
deduped, and anything else gets a numeric suffix, `-2` upward, until the name is
free. This is a tiebreak, not a policy: it exists so that no entry is ever
dropped for want of a name, and reaching it means the report has a line worth
reading.

An entry **already in the root** outranks every tool, whatever the flag order.
It is there because someone put it there deliberately, under the name they
chose; an adopted entry colliding with it is the one suffixed. Nothing already
in the root is moved, renamed or overwritten by this command, ever.

Two consequences worth knowing before running it:

- **A renamed entry is renamed for good.** Anything that referred to it by name
  — another skill, a prompt, a README — now refers to something that is not
  there. The report lists every rename for exactly this reason.
- **Identical copies are the likely case.** The same skill copied between two
  tools collides by name and is byte-identical. Suffixing produces two copies of
  one thing, which is worse than either alternative, so when the colliding
  entries have identical content the redundant one is dropped rather than
  suffixed, and the report says `deduped`. Dropping a byte-identical copy loses
  nothing; keeping it loses the point of a single root.

### What it still refuses

- A tool path that exists as a **regular file**, or as anything other than a
  directory or a symlink. There is nothing to adopt and nothing safe to replace.
- A symlink pointing somewhere **other** than `<root>/<type>`. Something else
  established it deliberately.
- Any adoption that would move an entry **out of the project** — a tool path
  that is itself outside the working directory. This command's whole frame of
  reference is the project it is run in.
- A `--root` under which `<root>/<type>` **is, or contains, or sits inside** a
  tool's own path. It would adopt into a directory it is about to replace with a
  symlink to itself. Only `--root` can produce it, and it fails that tool alone,
  since the same root can be perfectly valid for the other tools named.

A symlink already pointing at `<root>/<type>` is **success**, not a conflict.
Re-running after adding a tool is the expected way to use it, so every step is
idempotent and the second run reports `unchanged`.

Refusals are per tool and do not abort the others: a project where one tool was
scaffolded by hand still gets the rest. The exit code reflects that something
was refused.

### What it reports

Every path it created, linked or left alone, **every entry it moved**, and the
root they point into:

```
root       .ai/skills

adopted    .claude/skills    3 entries
  moved      commit
  moved      review
  moved      deploy

adopted    .opencode/skills  3 entries
  moved      running
  renamed    commit -> commit-opencode  (name taken by --claude)
  deduped    review                     (identical to what --claude contributed)

linked     .claude/skills    -> ../.ai/skills
linked     .opencode/skills  -> ../.ai/skills

Relative and safe to commit.
Write skills in .ai/skills/.
```

Every move is named, not counted. A summary line is enough for work a person
expects; adoption moves files they did not ask to have moved, and the only
acceptable version of that is one where the report can be read against
`git status` line for line.

`--dry-run` prints exactly that report and writes nothing. It matters more here
than it would for a command that only creates symlinks: the first run in a real
project rearranges its resources, and seeing the moves and renames listed
beforehand costs one command.

It closes by naming the two things a person is about to get wrong: the symlink
is committable and portable, and `<root>/<type>` — not the tool's path — is
where resources are actually written from now on.

## Exit codes

Per [cli.md](cli.md): `0` when every named tool is linked, `2` when any was
refused, `1` for an unexpected failure.

## Non-goals

- **Merging the project's skills with anything.** A tool's global skills are its
  own business; this points a tool at one more directory and combines nothing.
  A project that wants a merged view of several sources declares a workspace,
  which is what the rest of `airfs` is for.
- **Writing to the configuration.** Nothing declared, nothing served, nothing
  reconciled.
- **Removing what it created.** `rm` is adequate, and a command that deletes
  directories inside a repository to undo something it may not have created is a
  worse trade than the one it saves.
- **Discovering which tools a project uses.** Inferring from a `.claude/`
  directory that a person wants a symlink is how tools acquire a reputation for
  touching things.
- **Knowing anything about git.** Adoption moves files with ordinary renames.
  Git detects them as renames on its own, and a project not under version
  control behaves identically. A command that ran `git mv` would work in one
  kind of directory and surprise everyone in the other.
- **Windows.** `airfs` requires FUSE and is Linux and macOS only, so the symlink
  privilege question does not arise.
