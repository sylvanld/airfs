# Source configuration

## Purpose

Declare which sources contribute to the merged view and in what precedence
order. The configuration is the only input that decides what appears in the view,
so it must be readable at a glance, diffable, and reviewable in the same way as
the resources it points at.

## Scope

The configuration file's location and format, how paths in it are resolved, and
how the resolved result is reported. The precedence model itself is
[layered-resources.md](layered-resources.md).

## Location

The configuration is `sources.txt` at the root of the target directory — the
resource folder itself, alongside the kind directories, not inside them. A target
therefore carries its own definition: the folder and the declaration that
produced it travel together, and a second target is a second folder with its own
file rather than a section in a shared one.

The file is never masked by a mount, because the mountpoints are the kind
directories one level below it.

Both the target and the configuration file within it can be overridden per
invocation; see [cli.md](cli.md).

The file is normally authored by hand. It can also be produced whole from a
command line, in which case what is written down is what was typed, in the order
it was typed, and the file it replaces is gone — see [cli.md](cli.md).

## Format

A line-oriented plain text file whose meaningful lines are an ordered list of
source paths, and nothing else. Line orientation is chosen over a structured
format because that ordered list is the file's entire content, which a
line-oriented file expresses with no syntax at all, and because its most common
diff is a one-line addition.

Each meaningful line declares one source: a path to a directory that contains, or
will be given, the kind subdirectories. The path points at the source root, not
at a kind directory inside it. Order is from the most general source to the most
specific, since the last declaration wins.

Recognised on every line, in this order:

1. A `#` begins a comment, which runs to end of line. A line that is empty or
   comment-only declares nothing.
2. Surrounding whitespace is insignificant.
3. A leading `~` expands to the invoking user's home directory.
4. `$VAR` and `${VAR}` expand from the environment. An unset variable is an
   error, not an empty string: silently resolving to a shorter path, or to the
   target directory, would layer something nobody declared.
5. A path that is not absolute after expansion resolves relative to the
   directory containing the configuration file, never to the current working
   directory. The file must mean the same thing regardless of where the command
   was invoked from, and — when the file is read from somewhere other than the
   target — regardless of which target it is being applied to. In the default
   case the two coincide, since the file lives at the target's root.

There is no other directive. Kinds are fixed and built in, so they are not
declared here.

## Resolution

Resolving a configuration yields the ordered list of sources and, for each, the
strata it contributes.

- Order is preserved exactly as written. Duplicate paths after expansion are an
  error rather than a silently collapsed entry, because a duplicate means the
  author believes the file says something it does not.
- A declared source whose directory does not exist is an error. A configuration
  that points at nothing is a mistake worth stopping for, and the alternative —
  proceeding with a view that is quietly missing a repository's resources — is
  the harder failure to diagnose.
- A source that exists but lacks a kind's subdirectory has it created, empty, per
  [layered-resources.md](layered-resources.md). The stratum then exists and
  contributes no entries.
- A kind for which no source contributes an entry yields an empty kind, not an
  error.
- Resolution performs no I/O beyond confirming that the source directories exist
  and creating the missing kind subdirectories. Enumerating entries is the
  merge's job.

Symlinked source paths are followed, and the resolved source is recorded as the
path the author wrote, not its target. Reports name what the author would
recognise.

## Reporting

The resolved configuration is inspectable without building a view: the ordered
sources, each with the number of entries it contributes per kind, and the kinds
that ended up empty. Counting entries enumerates the strata, which is the merge's
work rather than resolution's, and is done on demand by the command that reports.
This is what a contributor consults to answer "is my repository being layered,
and where in the order?" — the question that precedence makes inevitable and that
guessing answers badly.

Entry-level shadowing is reported by the merge, not here, since it requires
comparing entries across strata.

## Non-goals

- Declaring where the view is exposed. The configuration says what is layered and
  in what order; the target is the frontend's, and lives in [cli.md](cli.md).
- Per-source kind subsets, or an order that differs by kind. One order, one kind
  set; a repository that should not contribute a kind leaves that kind's
  directory empty.
- Globs or recursive discovery of sources. Each source is named explicitly, so
  that adding one is a reviewable diff rather than a consequence of where a
  directory happens to sit.
- Cloning or updating source repositories.
- Any configuration format beyond this file — no environment-variable source
  lists, and no command-line source list that *accumulates* into this one. A
  command line may replace the file's entire content, per [cli.md](cli.md),
  which leaves this file as the single declaration of what is layered; nothing
  merges into it, and no second place holds part of the answer.
