# Workspace configuration

## Purpose

Declare every workspace on the machine in one file: what each one is assembled
from, where it is exposed, and which subfolders it carries. One file is the
whole answer to "what does `airfs` do on this host", so that the state of the
machine can be read, diffed and reviewed without walking the filesystem looking
for scattered declarations.

## Scope

The configuration file's location and format, the shape of a workspace, how
paths in it are resolved, and what makes a configuration invalid. The precedence
model is [layered-resources.md](layered-resources.md); how the declared
workspaces are established is [daemon.md](daemon.md).

This spec supersedes [source-config.md](source-config.md), which put one
`sources.txt` at the root of each target.

## Location

`$XDG_CONFIG_HOME/airfs/config.yaml`, falling back to `~/.config/airfs/config.yaml`
when `XDG_CONFIG_HOME` is unset. It can be overridden per invocation with
`--config`, per [cli.md](cli.md).

The configuration lives with the user's other configuration rather than inside a
target, for the reason that motivates this design: a declaration that lives in
the thing it produces can only be found by already knowing where to look, and
answers questions about one workspace at a time. A workspace is not
self-describing under this spec, and that is deliberate — the file is the
inventory, and a target directory is only ever an output.

A target therefore no longer holds anything but its mounted subfolders. Nothing
is written into a target other than the mountpoints themselves.

## Format

YAML. The content is a map of named workspaces, each with a list of sources, a
target, and a list of folders — a nested structure that a line-oriented file
cannot express without inventing syntax for it, and one whose most common diff
is a line added inside a named block.

```yaml
workspaces:
  personal:
    target: ~/.ai-resources
    folders: [agents, skills, commands]
    sources:
      - ~/repos/personal-capabilities
      - ~/repos/org-capabilities

  work:
    target: ~/work/.ai-resources
    folders: [skills, prompts]
    sources:
      - ~/repos/org-capabilities
      - ~/work/project-capabilities
```

The top-level key `workspaces` is a map. Any other top-level key is an error, so
that a typo is reported rather than ignored.

### A workspace

The map key is the workspace **name**. It identifies the workspace in every
report and is what commands take as an argument. It must be non-empty and match
`[a-zA-Z0-9_-]+`, so that it survives a command line and a log line without
quoting.

| Field | Required | Meaning |
| --- | --- | --- |
| `target` | yes | The directory the merged view is exposed under. Holds one mounted subdirectory per folder, and nothing else. |
| `sources` | yes | The contributing directory trees, in precedence order — most general first, last wins. At least one. |
| `folders` | no | The subdirectory names that are merged and mounted. Defaults to `[agents, skills, commands, scripts]`. |
| `enabled` | no | Whether the workspace is established. Defaults to `true` when absent. |

### `enabled`

A workspace with `enabled: false` is declared and not served: it appears in
every report, it is validated like any other, and nothing is mounted for it. If
it is currently mounted when it becomes disabled, its mounts are released.

This is the field that makes "stop serving this" reversible. Deleting a
workspace to stop serving it destroys a hand-authored block — its comments, its
ordering, the reasoning that produced it — and re-adding it does not bring any
of that back. Setting a flag loses nothing, and the declaration stays in the
file where a reader can see that the workspace exists and is deliberately off.
A disabled workspace is information; a deleted one is a gap.

It is also what a target being temporarily in the way needs. A workspace whose
target must be an ordinary writable directory for an afternoon is disabled, not
removed.

`folders` is what makes the set of merged subdirectories a property of the
workspace rather than of `airfs`. A workspace serving a tool that expects
`prompts/` declares `prompts`; one serving a tool that expects `skills/`
declares `skills`; the two can coexist over the same sources. `airfs` attaches
no meaning to any name — it merges the directory called what you called it.

Each name in `folders` must be a single path element: no `/`, no `.` or `..`.
A folder is a name shared by the sources and the target, not a path into either.

Repetition across workspaces is expressed with YAML anchors, which the format
already provides:

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

There is no bespoke inheritance, defaults block, or include directive. Adding
one would put part of the answer somewhere other than in the workspace that a
reader is looking at.

## Written by hand and by command

The file is authored by hand, and is also edited by the declarative commands of
[cli.md](cli.md) — `add`, `rm`, `enable`, `disable`. Both are first-class, which
puts two obligations on the format.

**`airfs` owns the file's formatting.** An edit rewrites the whole document in
canonical form: a fixed indentation, no blank lines between workspaces, and
whatever style the serialiser produces. Comments, key order, anchors and aliases
survive an edit; the exact bytes around them do not. This is `gofmt`'s bargain —
the file stops being formatted the way you typed it, and in exchange a one-field
change is a one-field diff every time instead of depending on who wrote the file
last. A file already in canonical form is left byte-identical by an edit that
changes nothing.

**An edit is reported by its effect, not by its argument.** A command names one
workspace, but a merge key means the resolved configuration of several can
change:

```yaml
personal: &base
  sources: [~/repos/personal-capabilities]
scratch:
  <<: *base
  target: /tmp/scratch-resources
```

Editing `personal`'s sources here changes `scratch` too. The command reports
every workspace whose *resolved* configuration differs from before, not only the
one it was given, so an alias never changes something silently. Commands write
plain blocks and never author an anchor; an anchor exists because a person wrote
it, and it keeps working.

## Path resolution

`target` and every entry of `sources` are paths, resolved identically:

1. A leading `~` expands to the invoking user's home directory.
2. `$VAR` and `${VAR}` expand from the environment. An unset variable is an
   error, never an empty string — silently resolving to a shorter path would
   layer, or expose, something nobody declared.
3. A path still relative after expansion resolves against the directory holding
   the configuration file, never the current working directory. The file must
   mean the same thing regardless of where the command that read it was invoked
   from.

Symlinked paths are followed on access and reported as the author wrote them.
Reports name what the author would recognise.

## Validation

A configuration is resolved as a whole before anything is established. These are
errors:

- An unknown top-level key, an unknown key inside a workspace, or a value of the
  wrong type.
- A workspace with no sources, or with an empty `folders` list.
- A folder name that is not a single path element.
- Two sources within one workspace that resolve to the same path. A duplicate
  means the author believes the file says something it does not.
- Two workspaces with the same resolved `target`, or a target that is a path
  prefix of another workspace's target. Both would have two workspaces writing
  mountpoints into one tree, and the second would mount inside a directory that
  is itself being mounted over.
- A source or target path that is not absolute after resolution — which, given
  the rules above, cannot happen and is checked as an invariant rather than a
  user error.

These are **not** errors, and are reported rather than raised:

- A declared source directory that does not exist. Under one workspace per file
  this stopped everything; now it must not, because one unclonable repository in
  one workspace has no business preventing the other workspaces from being
  served. It fails the workspace that declares it, per
  [daemon.md](daemon.md), and every other workspace is established normally.
- A source that exists but lacks one of the workspace's folders. It contributes
  nothing to that folder. **The directory is not created.** Under
  [source-config.md](source-config.md) a missing subdirectory was created in the
  source repository, which made resolution write into repositories it was only
  supposed to read; with folder names now arbitrary, that would scatter
  directories nobody asked for. `airfs` performs no write against a source.
- A folder for which no source contributes an entry. It is mounted and empty.

An invalid configuration leaves the machine exactly as it was. Nothing is
partially applied from a file that did not parse, since the file is the whole
statement of what should be true.

## Reporting

A resolved configuration is inspectable without establishing anything: every
workspace, its target, its folders, its sources in precedence order, the number
of entries each source contributes per folder, and the folders that end up
empty. Counting entries enumerates the sources, which is the merge's work rather
than resolution's, and is done on demand by the command that reports.

Entry-level shadowing is reported by the merge, not here, since it requires
comparing entries across sources.

## Non-goals

- Any second place that holds part of the answer: no environment-variable source
  lists, no per-target file, no directory of fragments. One file states what
  exists on the machine.
- Preserving the file's exact formatting across an edit. See above: `airfs` owns
  it. A user who needs their own layout preserved byte for byte edits by hand
  and does not run the editing commands.
- Globs or recursive discovery of sources and workspaces. Each is named
  explicitly, so that adding one is a reviewable diff rather than a consequence
  of where a directory happens to sit.
- Cloning, updating, or reporting the git state of source repositories.
- Per-folder source order, or a folder-specific subset of the sources. One
  order per workspace; a source that should not contribute a folder does not
  contain it.
- Options that tune how a workspace is served — caching, permissions, ownership.
  Those are properties of the mount and are fixed by
  [fuse-mount.md](fuse-mount.md).
