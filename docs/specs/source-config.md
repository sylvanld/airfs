# Source configuration

## Purpose

Declare which sources contribute to the merged view, in what precedence order,
and which kinds are layered. The configuration is the only input that decides
what appears in the view, so it must be readable at a glance, diffable, and
reviewable in the same way as the resources it points at.

## Scope

The configuration file's format, how paths in it are resolved, and how the
resolved result is reported. The precedence model itself is
[layered-resources.md](layered-resources.md).

## Format

A line-oriented plain text file, by default `sources.txt` alongside the
repository or configuration directory it belongs to. Line orientation is chosen
over a structured format because the file's primary content is an *ordered list
of paths*, which a line-oriented file expresses with no syntax at all, and
because its most common diff is a one-line addition.

Each meaningful line declares one source: a path to a directory that contains
kind subdirectories. The path points at the source root, not at a kind directory
inside it.

Recognised on every line, in this order:

1. A `#` begins a comment, which runs to end of line. A line that is empty or
   comment-only declares nothing.
2. Surrounding whitespace is insignificant.
3. A leading `~` expands to the invoking user's home directory.
4. `$VAR` and `${VAR}` expand from the environment. An unset variable is an
   error, not an empty string: silently resolving to a shorter path, or to the
   configuration file's own directory, would layer something nobody declared.
5. A path that is not absolute after expansion resolves relative to the
   directory containing the configuration file, never to the current working
   directory. The file must mean the same thing regardless of where the command
   was invoked from.

Kinds are declared in the same file, distinguished from source lines by a
`kinds:` prefix followed by a comma-separated list of kind names. Each kind name
optionally carries its granularity as `name=dir` or `name=file`; the default when
omitted is `dir`. A file may declare kinds at most once, and the declaration
applies to every source. When no `kinds:` line is present the default set is
`skills=dir, agents=file, commands=file`.

## Resolution

Resolving a configuration yields the ordered list of sources, and for each the
strata it contributes.

- Order is preserved exactly as written. Duplicate paths after expansion are an
  error rather than a silently collapsed entry, because a duplicate means the
  author believes the file says something it does not.
- A declared source whose directory does not exist is an error. A configuration
  that points at nothing is a mistake worth stopping for, and the alternative —
  proceeding with a view that is quietly missing a repository's resources — is
  the harder failure to diagnose.
- A source that exists but has no directory for a given kind contributes no
  stratum for that kind, silently. This is the normal case: most repositories own
  resources of one or two kinds.
- A kind for which no source contributes a stratum yields an empty kind, not an
  error.
- Resolution performs no I/O beyond confirming that the source directories and
  their kind subdirectories exist. Enumerating entries is the merge's job.

Symlinked source paths are followed, and the resolved source is recorded as the
path the author wrote, not its target. Reports name what the author would
recognise.

## Reporting

The resolved configuration is inspectable without building a view: the ordered
sources, each with the kinds it contributes and the number of entries in each,
and the kinds that ended up empty. This is what a contributor consults to answer
"is my repository being layered, and where in the order?" — the question that
precedence makes inevitable and that guessing answers badly.

Entry-level shadowing is reported by the merge, not here, since it requires
enumerating entries across strata.

## Non-goals

- Per-source kind overrides, or per-source ordering that differs by kind. One
  order, one kind set; if a repository should not contribute a kind, it should not
  contain that kind's directory.
- Globs or recursive discovery of sources. Each source is named explicitly, so
  that adding one is a reviewable diff rather than a consequence of where a
  directory happens to sit.
- Cloning or updating source repositories.
- Any configuration format beyond this file — no environment-variable source
  lists, no command-line source lists that accumulate into a persistent
  configuration.
