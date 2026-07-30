# Declaring workspaces 📝

Everything `airfs` does on your machine is written in **one file**:

```
$XDG_CONFIG_HOME/airfs/config.yaml     # ~/.config/airfs/config.yaml if XDG_CONFIG_HOME is unset
```

One file, so that "what is `airfs` doing on this host?" has one answer you can
read, diff and review — instead of a declaration hidden inside each directory it
produces, findable only by already knowing where to look. 🔍

```yaml
workspaces:
  personal:
    target: ~/.ai-resources
    folders: [agents, skills, commands]
    sources:
      - ~/repos/personal-capabilities   # 1st — most general
      - ~/repos/org-capabilities        # 2nd — wins every collision

  work:
    target: ~/work/.ai-resources
    folders: [skills, prompts]
    sources:
      - ~/repos/org-capabilities
      - ~/work/project-capabilities
```

Point a command at a different file with `--config <file>`. 🎛️

## A workspace, field by field 🗂️

The map key is the workspace **name** — what commands take as an argument and
what every report calls it. It must match `[a-zA-Z0-9_-]+`, so it survives a
command line without quoting.

| Field | Required | Means |
| --- | --- | --- |
| `target` | ✅ | The directory the merged view is exposed under. `airfs` claims one mounted subdirectory per folder inside it and **nothing more**. |
| `sources` | ✅ | The contributing directory trees, in precedence order — most general first, last wins. At least one. |
| `folders` | — | The subdirectory names to merge and mount. Defaults to `[agents, skills, commands, scripts]`. |
| `enabled` | — | Whether it is served. Defaults to `true`. |

### `target` — a directory `airfs` shares 🏠

A target does **not** have to be a directory `airfs` owns. It claims the folder
subdirectories and nothing more, so pointing a workspace at a directory a tool
already fills with its own files is normal use:

```yaml
workspaces:
  myproject-claude:
    target: ~/work/myproject/.claude   # settings.json, CLAUDE.md — all untouched
    folders: [skills]                  # only .claude/skills/ is a mountpoint
    sources:
      - ~/repos/org-capabilities
      - ~/work/myproject/.skills       # the project's own, winning
```

The one requirement is on the **folder subdirectories**: each must be absent, or
present and empty. `airfs` creates the missing ones and refuses a populated one,
because mounting over it would hide the files inside. 🫥

### `folders` — you name them, not `airfs` 🏷️

`folders` is what makes the merged subdirectories a property of *your workspace*
rather than of `airfs`. A tool that expects `prompts/` gets a workspace declaring
`prompts`; a tool that expects `skills/` gets one declaring `skills`; the two can
sit over the same source repositories.

`airfs` attaches **no meaning** to any name — it merges the directory called what
you called it. Each name must be a single path element: no `/`, no `..`.

!!! warning "A source is never written to"

    A source that lacks one of the folders simply contributes nothing to it. The
    directory is **not created** — `airfs` performs no write against a source,
    ever. A folder no source contributes to is mounted and empty. 🫙

### `enabled` — how to stop serving something 🔌

```bash
airfs disable work     # released from the machine, untouched in the file
airfs enable work      # back
```

Deleting a workspace to stop serving it destroys a block you wrote by hand —
its comments, its ordering, the reasoning that produced it — and re-adding it
brings none of that back. Setting a flag loses nothing, and a reader can still
see the workspace exists and is deliberately off.

**A disabled workspace is information; a deleted one is a gap.** 💡 It is also
what you want when a target has to be an ordinary writable directory for an
afternoon.

## How paths are resolved 🧭

`target` and every entry of `sources` follow the same three rules:

1. A leading `~` expands to your home directory.
2. `$VAR` and `${VAR}` expand from the environment. **An unset variable is an
   error**, never an empty string — silently resolving to a shorter path would
   layer, or expose, something nobody declared. 🚧
3. A still-relative path resolves against **the directory holding the config
   file**, never your working directory. The file must mean the same thing
   wherever it is read from.

Symlinks are followed on access and reported as you wrote them. Reports always
name what you would recognise, not a resolved link target.

## Repetition: use YAML, it already has anchors ⚓

```yaml
workspaces:
  personal: &base
    target: ~/.ai-resources
    folders: [agents, skills]
    sources: [~/repos/personal-capabilities]

  scratch:
    <<: *base
    target: /tmp/scratch-resources
```

There is no bespoke inheritance, defaults block, or include directive — that
would put part of the answer somewhere other than in the workspace you are
looking at.

!!! tip "An edit is reported by its effect, not by its argument"

    Editing `personal`'s sources above changes `scratch` too. Every declaring
    command reports **every workspace whose resolved configuration changed**, not
    just the one you named — so an alias never changes something behind your
    back. 🔗

## Writing it by hand, and by command ✍️

Both are first-class. `add`, `rm`, `enable` and `disable` edit this file, and
they keep what you wrote: **comments, key order, anchors and aliases survive an
edit.** The exact bytes around them do not.

That is `gofmt`'s bargain. The file stops being formatted the way you typed it —
fixed indentation, no blank lines between workspaces — and in exchange a
one-field change is a one-field diff every time, instead of depending on who
edited it last. A file already in canonical form is left byte-identical by an
edit that changes nothing.

Commands write plain blocks and never author an anchor; one you wrote keeps
working.

!!! info "Prefer your own layout byte for byte?"

    Then edit by hand and run `airfs reload` instead of the declaring commands.
    Nothing else in `airfs` will touch the file.

## `airfs add`: the whole list, every time 💥

```bash
airfs add personal \
  --target ~/.ai-resources \
  -s ~/repos/personal-capabilities \
  -s ~/repos/org-capabilities \
  -f agents -f skills
```

- **`-s` / `--source`** repeats, and the order you give them **is** the
  precedence order, most general first.
- **`-f` / `--folder`** repeats. Give none and you get the default set.
- **`--disabled`** declares it without serving it.

On a name that already exists this is a **replacement, never a merge**: what the
block said is gone. You typed the sources you want, so you are stating the whole
list — a flag that quietly appended would produce an order nobody wrote. ⚠️

The result is validated **before** it is written, so a mistyped path leaves your
existing configuration standing.

Paths are written down **as `airfs` receives them**, so `~` and `$VAR` survive
into the file and it stays something you would have written by hand — but only if
you **quote them**, since otherwise your shell expands them before `airfs` ever
sees them:

```bash
airfs add personal --target '~/.ai-resources' -s '$WORK/ai-platform'
```

Quoting is what you want for a configuration you keep in git and share between
machines. Without it you get the absolute paths your shell produced, which is
also perfectly valid — just machine-specific. 🎒

The one exception to writing a path as received is one still relative after
expansion: on a command line that means "from here", in the file it would mean
"from the config file's directory", so it is made absolute first.

## What makes a configuration invalid 🚫

These stop everything, and nothing is applied from a file that does not resolve:

- An unknown key, at the top level or inside a workspace.
- A workspace with no `target`, no `sources`, or an empty `folders` list.
- A folder name that is not a single path element.
- Two sources within one workspace resolving to the same path — a duplicate
  means you believe the file says something it does not.
- Two workspaces sharing a `target`, or one target nested inside another. Both
  would have two workspaces writing mountpoints into one tree.

Every problem in the file is reported at once, each against the line that caused
it, so fixing one typo does not make you run the command again to find the next.

These are **reported, not fatal** — they fail only the workspace concerned, and
every other workspace is served normally:

- A folder subdirectory inside the target that already exists and is not empty.
- A declared source directory that does not exist.
- A source that exists but lacks one of the folders.
- A folder no source contributes an entry to.

## Where next 👉

- [Precedence](precedence.md) — which layer wins, and how to see it
- [Running the daemon](running-the-daemon.md) — serving what you just declared
- [Several workspaces](multiple-workspaces.md) — sharing sources between them
