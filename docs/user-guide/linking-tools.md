# Linking a project's skills to your tools 🔗

Your project owns skills. Every tool looks for them **somewhere else**, and
somewhere different from every other tool:

```
myproject/
  .claude/skills/     # one tool looks here
  .opencode/skills/   # the next one looks here
```

Copy them into both and the copies drift apart by Friday. `airfs link` puts them
in **one directory the project owns** and leaves a relative symlink where each
tool looks. 🪄

```bash
cd ~/work/myproject
airfs link --claude --opencode
```

```
root       .ai/skills

adopted    .claude/skills    2 entries
  moved      commit
  moved      review

adopted    .opencode/skills  3 entries
  renamed    commit -> commit-opencode  (name taken by --claude)
  deduped    review                     (identical to what --claude contributed)
  moved      running

linked     .claude/skills    -> ../.ai/skills
linked     .opencode/skills  -> ../.ai/skills

Relative and safe to commit.
Write skills in .ai/skills/.
```

From now on you write skills in `.ai/skills/`, and **both tools see them.** ✍️

!!! tip "Run it with `--dry-run` first"

    The first run in a real project rearranges its resources. `--dry-run` prints
    the report above under a `Dry run — nothing below was written.` line and
    writes nothing, so you can read the moves before they happen. It costs one
    command. 👀

## It adopts what your tools already hold 📦

A project that has been using a tool for a week is the **normal case**, not an
error. So `airfs link` does not refuse to run: every entry under a tool's
directory is **moved** into the root, and the emptied directory becomes the
symlink.

Nothing is deleted, so a run that did the wrong thing is undone with `mv`, and
the report names **every single move** — read it against `git status` line for
line. 🧾

Two tools shipping the same name is the interesting case:

| What happened | What you get |
| --- | --- |
| Two tools ship `commit`, with **different** content | The **first flag you typed** keeps `commit`; the other becomes `commit-opencode`. |
| Two tools ship `commit`, **byte for byte identical** | One copy, reported as `deduped`. Two copies of one thing lose the point of a single root. |
| `.ai/skills/commit` **already exists** | It wins, whatever the flag order. Someone put it there deliberately. The adopted one is suffixed. |

!!! warning "A renamed entry is renamed for good"

    Anything that referred to it by name — another skill, a prompt, a README —
    now points at something that is not there. That is why every rename is on
    its own line in the report. 🔍

The **order of the flags is the only thing you control here**, so it is what the
rule turns on: `--claude --opencode` and `--opencode --claude` give different
winners.

## The root is yours to name 🏠

```bash
airfs link --claude --root .agents
```

Defaults to `.ai`. It has to be **inside the project**: the symlinks are
relative so they can be committed, and a root outside the project would produce
links that resolve to nothing on every other machine.

`.ai/` is also, exactly as it stands, a well-formed `airfs` **source** — it is
`<source>/<folder>/`, the layout every source has. So the day this project's
skills should be merged with your organisation's, you declare a workspace and
move nothing:

```bash
airfs add myproject \
  --target '~/work/myproject/.claude' \
  -s '~/repos/org-capabilities' \
  -s '~/work/myproject/.ai' \
  -f skills
```

…but that is a different tool for a different job. See
[Declaring workspaces](declaring-workspaces.md). 🧩

## What it refuses 🚫

Each refusal fails **that tool only** — the others are still linked, and the
command exits `2`.

- The tool's path is a **regular file**. Nothing to adopt, nothing safe to
  replace.
- It is a **symlink pointing somewhere else**. Something established it
  deliberately.
- A `--root` that lands **inside a tool's own directory**, or the reverse.
- The tool's path is **outside the project** — a symlink out of it, most likely.
  Adopting would move your resources somewhere the project does not contain, and
  the whole frame of reference here is the directory you are standing in.

A symlink that already points at the root is **success**, reported as
`unchanged`. Re-running after adding a tool is the expected way to use this. ♻️

## What it is not 🧭

!!! danger "Nothing reconciles these symlinks"

    Everything else `airfs` does is declarative: you write the configuration, the
    daemon makes the machine match it, forever. **`link` is not that.** It is a
    one-shot mutation of a project directory, recorded nowhere and undone with
    `rm`.

    So `airfs status` and `airfs ls` will never mention a symlink it created, and
    no command will offer to remove one. If you are waiting for something to
    notice, nothing will. ⏳

That is deliberate: the artifact is a **committed symlink**, and it has to keep
working for teammates who do not have `airfs` installed at all. A declaration
that needed a daemon to have any effect would defeat the point. 🤝

It also **merges nothing**. A tool's global skills are its own business; this
points a tool at one more directory. A merged view of several sources is a
workspace, which is what the rest of `airfs` is for.

## The tools it knows 🛠️

```bash
airfs link --list
```

```
  --claude       skills   .claude/skills
  --opencode     skills   .opencode/skills
```

The table is short on purpose and grows by **evidence**. A wrong row is the
worst thing this command can do to you — the symlink gets created, the report
says it worked, and the tool goes on seeing nothing — so a tool is added against
its documented layout, never against a plausible guess. Missing yours?
[Open an issue](https://github.com/sylvanld/airfs/issues) with the path it
reads. 📮

## Where next 👉

- [Declaring workspaces](declaring-workspaces.md) — merging this project's `.ai`
  with other repositories
- [Precedence](precedence.md) — which layer wins, once you do
