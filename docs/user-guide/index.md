# User guide 📚

How to use `airfs` day to day, once [Get started](../get-started.md) has you
serving something. One document per question.

| Document | Answers |
| --- | --- |
| [declaring-workspaces.md](declaring-workspaces.md) | What goes in `config.yaml`, and how are paths resolved? |
| [precedence.md](precedence.md) | Two sources ship the same skill — which wins, and how do I see it? |
| [running-the-daemon.md](running-the-daemon.md) | How do I serve, reload, inspect and stop what I declared? |
| [multiple-workspaces.md](multiple-workspaces.md) | How do I run several workspaces over the same sources? |
| [go-api.md](go-api.md) | How do I consume the merged view from Go, without mounting? |
| [troubleshooting.md](troubleshooting.md) | Something is not served, or not visible. What now? |

## The vocabulary, in one table 🗂️

Every page below uses these words in exactly this sense.

| Word | Means |
| --- | --- |
| **Source** (layer) | One contributing directory tree, normally a git working copy. Sources are an *ordered* list. |
| **Folder** | A subdirectory name that gets merged and mounted — `skills`, `prompts`, whatever you declare. `airfs` attaches no meaning to any of them. |
| **Entry** | One resource within a folder, named directly under it — a directory like a skill, or a single file like a command. |
| **Target** | The directory a workspace's merged view is exposed under. `airfs` claims one mountpoint per folder inside it and nothing more; anything else the directory holds is untouched. |
| **Workspace** | One named declaration: a target, an ordered list of sources, and the folders to merge. The unit everything is reported and controlled by. |

## The commands, by what they are about 🛠️

Nothing spans two groups, which is what makes each one predictable.

=== "Declaring ✍️"

    They change what is written down, then tell the daemon to catch up.

    | Command | Does |
    | --- | --- |
    | `add <name>` | Declare a workspace, or replace an existing one **whole**. |
    | `rm <name>` | Remove its declaration and release its mounts. |
    | `enable <name>` | Serve it again. |
    | `disable <name>` | Stop serving it, keeping the declaration, comments and all. |

=== "Inspecting 🔍"

    They change nothing and need nothing running.

    | Command | Does |
    | --- | --- |
    | `ls` | One line per declared workspace. The inventory. |
    | `inspect <name>` | What one workspace merges: sources in order, counts per folder, every shadowed entry. |

=== "Running 🧵"

    They drive the daemon and write nothing.

    | Command | Does |
    | --- | --- |
    | `up` | Start the daemon and serve every enabled workspace. `--detach` to background it. |
    | `down` | Stop it, and release every `airfs` mount on the machine. |
    | `reload` | Re-read the configuration and reconcile. |
    | `status [name]` | Which config the daemon loaded, what it declares, what is mounted. |
    | `doctor` | Check the host's prerequisites and name what to install. |

A report belongs with the thing it is a report *of*. `status` is about the
daemon, so it sits with `up` and `down` despite only printing. What a workspace
merges is not the daemon's state — it is true whether or not anything is
running — so that is `inspect`. 🧭

## The one flag every command takes ⚙️

```bash
airfs <command> [--config <file>]
```

Defaults to `$XDG_CONFIG_HOME/airfs/config.yaml`. A `~` is expanded, and a
relative path resolves against your **working directory** — you typed it on a
command line, so that is the obvious frame of reference. Paths *inside* the file
are different: they resolve against that file's own directory, so a configuration
means the same thing wherever it is read from.

There is no `--target` override. A target is a property of a declared workspace
now, not of an invocation; what an invocation names is a **workspace, by name**.

## Exit codes 🔢

| Code | Means |
| --- | --- |
| `0` | Success. |
| `2` | An unsatisfied **precondition** — the host or the configuration needs attention. |
| `1` | Anything else that failed. |

`2` is separate because it is the outcome you act on differently: a missing
config, an unresolvable source, a non-empty mountpoint, an absent prerequisite, a
daemon that is required and not running. It does not mean `airfs` malfunctioned.

- **`status`** exits `0` when every *enabled* workspace is fully served, `2` when
  any is not. A disabled workspace serving nothing is the configuration being
  honoured, so it does not affect the code.
- **`doctor`** exits `0` when every prerequisite is satisfied, `2` otherwise.
- **`up`, `reload`, and every declaring command** exit `2` when any workspace
  failed to establish, having established every other one. The machine does not
  match the file, and the output names which workspaces and why.
- A declaring command that **wrote the file and found no daemon** also exits `2`:
  the durable half succeeded and the machine has not caught up yet.

**Shadowing is never a failure.** `airfs inspect` exits `0` with shadowed entries
reported — shadowing is the mechanism working. ✅
