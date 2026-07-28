# Declaring layers 🧭

A workspace is defined by one plain-text file: `sources.txt` at the root of the
target. One path per line, in precedence order. The most common diff to it is a
single added line.

```bash
# ~/.ai-resources/sources.txt
# Layers, most general first — the last declaration wins.

~/ai/personal              # everything I carry between jobs
$WORK/ai-platform          # my organization's shared capabilities
./scratch                  # relative to this file, not to $PWD
```

## The format 📝

| Rule | Detail |
| --- | --- |
| One path per line | Anything else is a syntax the file does not have. |
| Order is precedence | The **last** declaration wins a collision. Nothing reorders it. |
| `#` starts a comment | To end of line, whether the line begins with it or not. |
| Blank lines are ignored | Including whitespace-only lines. Group your layers. |
| `~` expands to your home | Only as a leading `~` or `~/`. |
| `$VAR` and `${VAR}` expand | From the environment of whoever runs `airfs`. |
| A relative path resolves against **this file's directory** | Never against your working directory. |

There is no directive syntax, no include, and no per-kind override. A file whose
whole grammar is "one path per line" is a file that reviews cleanly in a pull
request, and the ordering it expresses is the entire configuration.

## Path resolution, precisely 🎯

Expansion happens in this order: leading `~`, then variables, then — if the
result is still relative — resolution against the directory holding
`sources.txt`.

That last rule is the one worth internalising. It means a layer list means the
same thing wherever it is read from: from your home directory, from a script, or
from a service manager unit with no working directory to speak of.

!!! warning "An unset variable is an error, not an empty string"

    `$NOT_SET/ai` does not quietly become `/ai`. `airfs` refuses to resolve it and
    names the variable:

    ```
    airfs: /home/you/.ai-resources/sources.txt:3: $NOT_SET/ai: $NOT_SET is not set
    ```

    Silently resolving to a shorter path would layer something nobody declared —
    and a workspace quietly built from the wrong directory is the harder failure
    to notice.

## What is checked, and when ✅

Resolving the file — which every command does — also verifies it. Each is a
precondition failure, exit `2`:

- **The file is missing.** `airfs` tells you to create it rather than serving an
  empty workspace.
- **A declared layer does not exist**, or is not a directory. Proceeding with a
  workspace quietly missing a repository's capabilities is worse than stopping.
- **Two lines resolve to the same directory.** The message points at the first
  one, because a duplicate means you believe the file says something it does not:

  ```
  airfs: sources.txt:4: ./ai-platform duplicates line 2 (~/ai/ai-platform); both resolve to /home/you/ai/ai-platform
  ```

Resolution also **creates the missing kind directories** inside each layer —
`agents/`, `skills/`, `commands/`, `scripts/`. This is the only thing `airfs`
ever writes into a layer, and it is why every layer contributes every kind: you
never have to `mkdir` before adding a resource of a new kind.

## What does not go in the file 🚫

- **The target.** The same layers may be exposed at more than one workspace, so
  where a view lives is a property of the invocation — `--target` — not of the
  layer list.
- **Anything about git.** `airfs` does not clone, pull, or report on the
  repositories behind your layers. They are directories to it.
- **Per-kind configuration.** The kinds are fixed and built in, which is what
  keeps unrelated top-level directories from leaking into a workspace.

`airfs` never rewrites this file either. It is authored by hand and reviewed in
git; a command that edited it would produce diffs nobody wrote.

## Checking what it means 🔍

Never guess. `airfs sources` resolves the file, reports the order, counts what
each layer contributes per kind, and names every shadowed entry — while touching
nothing and mounting nothing:

```bash
airfs sources
```

It is the fastest way to answer "is my repository actually being layered, and
where in the order?" See [precedence.md](precedence.md) for how to read the
shadowing report.
