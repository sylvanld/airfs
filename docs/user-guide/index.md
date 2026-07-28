# User guide 📚

How to use `airfs` day to day, once [Get started](../get-started.md) has you
mounted. One document per question.

| Document | Answers |
| --- | --- |
| [declaring-layers.md](declaring-layers.md) | What can I write in `sources.txt`, and how are paths resolved? |
| [precedence.md](precedence.md) | Two layers ship the same skill — which wins, and how do I see it? |
| [mounting.md](mounting.md) | How do I serve, inspect, and release a workspace? |
| [multiple-workspaces.md](multiple-workspaces.md) | How do I run more than one workspace from the same layers? |
| [go-api.md](go-api.md) | How do I consume the merged view from Go, without mounting? |
| [troubleshooting.md](troubleshooting.md) | Something is not served, or not visible. What now? |

## The vocabulary, in one table 🗂️

Every page below uses these five words in exactly this sense.

| Word | Means |
| --- | --- |
| **Source** (layer) | One contributing directory tree, normally a git working copy. Sources are an *ordered* list. |
| **Kind** | A category of resource, and one subdirectory name: `agents`, `skills`, `commands`, `scripts`. The set is fixed. |
| **Entry** | One resource within a kind, named directly under it — a directory like a skill, or a single file like a command. |
| **Stratum** | One source's contribution to one kind: `<source>/<kind>/`. |
| **Target** (workspace) | The directory the merged view is exposed under, holding `sources.txt` and one mountpoint per kind. |

## The two flags every command takes ⚙️

```bash
airfs <command> [--target <dir>] [--config <file>]
```

- `--target <dir>` — where the view is exposed. Defaults to `~/.ai-resources`.
- `--config <file>` — the layer list to read. Defaults to `sources.txt` inside
  the target.

They are independent: overriding `--config` does not move the target. That is
what lets one layer list be tried against a scratch target, and one target be
rebuilt from an alternative list. See
[multiple-workspaces.md](multiple-workspaces.md).

Both accept `~`, and a relative path resolves against your **working
directory** — you typed it on a command line, so that is the obvious frame of
reference. Paths *inside* `sources.txt` are different: they resolve against that
file's own directory, so a layer list means the same thing wherever it is read
from.

## Exit codes 🔢

| Code | Means |
| --- | --- |
| `0` | Success. |
| `2` | An unsatisfied **precondition** — the host or the configuration needs attention. |
| `1` | Anything else that failed. |

`2` is separate because it is the outcome you act on differently: a missing
config, an unresolvable layer, a non-empty mountpoint, an absent mount
prerequisite. It does not mean `airfs` malfunctioned.

`status` and `doctor` report a *state* rather than an outcome, so their codes say
which state: `0` when fully served / fully satisfied, `2` otherwise.

**Shadowing is never a failure.** `airfs sources` exits `0` with shadowed entries
reported — shadowing is the mechanism working.
