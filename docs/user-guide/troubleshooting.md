# Troubleshooting 🩹

Start with the three commands that only report, in this order. They cost nothing
and they answer most questions before you have to guess:

```bash
airfs doctor           # can this host mount at all?
airfs status           # what is the daemon doing, and what is mounted?
airfs inspect <name>   # what does this workspace actually merge?
```

The last two are the pair worth internalising: **`inspect` says what should be
there, `status` says what is.** When they disagree, that difference is the
diagnosis. 🔎

## `airfs doctor` says something is missing 🩺

```
  ok       /dev/fuse        readable and writable by you
  MISSING  fusermount3      not found on PATH
                            provided by the system FUSE package, libfuse3-3 on Debian and Ubuntu

airfs: a prerequisite is missing; install what provides it, then run airfs doctor again
```

Install the named package (`fuse3` on Debian, Ubuntu, Fedora, and Arch) and run
`doctor` again. It reports every prerequisite either way, so you find out about
the second missing one before installing the first.

**`/dev/fuse` absent** usually means a container without the device exposed. It
is provided by the kernel, not by a package — a container needs
`--device /dev/fuse` and, depending on the runtime, `--cap-add SYS_ADMIN`.

**`fusermount3` present but not setuid** cannot be worked around from userspace:
an unprivileged process genuinely cannot mount without it. Reinstalling the FUSE
package restores the bit.

**`XDG_RUNTIME_DIR` not set** means the daemon has nowhere to put its control
socket, and it refuses to start rather than falling back to a world-writable
directory. It is normally set by your login session; if you are in a bare `su` or
a minimal container, set it to a private directory you own.

## `airfs` is ignoring my edit 🚨

**Check line two of `airfs status` first.**

```
daemon  running since Mon, 28 Jul 2026 09:14:02 CEST
config  /home/you/experiment.yaml

! The daemon is serving /home/you/experiment.yaml, but this command reads
  /home/you/.config/airfs/config.yaml.
  Edits to the second will not take effect until the daemon is restarted against it.
```

A daemon started with `--config` holds that file for its **whole life**. The file
you are editing and the file being served have drifted apart, and every symptom
of that looks exactly like a bug. `airfs down && airfs up` fixes it.

## My repository's resources are not showing up 🤷

Run `airfs inspect <name>` and read the counts:

```
  1. ~/ai/personal    skills 2  commands 0
  2. ~/ai/project     skills 0  commands 0
```

`skills 0` next to a repository you know has skills means one of:

- **The resources are not under a declared folder.** They must live at
  `<source>/skills/<entry>`, not at `<source>/<entry>` or nested deeper.
- **The workspace does not declare that folder.** `folders` is per workspace —
  a repository's `tools/` contributes nothing unless some workspace asks for
  `tools`. Check the `folders` line at the top of the report.
- **The path points somewhere else than you think.** The report names sources as
  you declared them; a `$VAR` may be resolving to an unexpected directory.

If the source does not appear at all, it is not in that workspace's `sources` —
check the `config` path `airfs ls` prints, since `--config` and the default may
differ.

## My edit did nothing 🕵️

Three causes, in order of likelihood.

**The entry is shadowed.** A later source ships the same name and wins *whole*.
Check the shadowing report:

```
Shadowed entries — the winner is what the view serves:
  skills/commit  wins ~/ai/project  over ~/ai/personal
```

You edited `~/ai/personal/skills/commit/`; the view serves `~/ai/project`'s. Edit
the winner, or reorder the sources. Note that this covers files the winner does
not even have — a `helper.sh` in the losing copy is invisible, because entries
win whole rather than file by file.

**The workspace is disabled.** `airfs status` says so plainly, and it is the
configuration being honoured rather than a failure. `airfs enable <name>`.

**The mount is stale.** The serving process died, and the kernel still lists the
mountpoint while every access to it fails:

```
  /home/you/.ai-resources/skills  STALE — its serving process died; recover with airfs down
```

```bash
airfs down && airfs up --detach
```

What is *not* a cause: caching. The view caches nothing, so an edit to a file in
a source that is genuinely winning is visible on the very next read.

## I changed the config and nothing happened 🔄

The daemon **does not watch the file** — an editor writing a file is not one
event, and reacting to the first of them would act on half a document.

- Edited by hand? Run `airfs reload`.
- Used `add`, `rm`, `enable` or `disable`? They reload on their own.

Editing *within* an existing source needs no reload at all; only changing the set
of workspaces, sources or folders does.

## One workspace failed and the others are fine ✅

That is deliberate. When one file describes the whole machine, one mistyped path
must not take down the rest, so a workspace that cannot be established fails
**alone** and everything else is served:

```
  work      NOT SERVED ~/work/.ai-resources — source ~/work/client-acme: no such file or directory
```

Fix the reason and run `airfs reload`. There is no retry timer and no backoff:
reconciliation *is* the retry.

## `<dir> is not empty` 🚧

The mountpoint has files in it, and mounting over them would hide them —
silently, until someone spends an afternoon on it. Look before you clear it:

```bash
ls -la ~/.ai-resources/skills
```

If it is a leftover copy of resources you are now layering, delete it. If it is
something you still want, move it into one of your sources so it is served
properly.

## `Mounted, belonging to no declared workspace` 🧹

`airfs status` found `airfs` mounts the current configuration does not account
for — left by a previous configuration, a previous daemon, or a crash. This is
the whole reason reconciliation reads the kernel rather than a file of its own.

```bash
airfs down     # releases every airfs mount on the machine, alive or stale
```

## `Read-only file system` 🛡️

Working as intended: the kernel enforces it. A new file in the merged view would
belong to *some* source, and the view has no basis to pick one. Create or edit
the file in the repository that owns it, where it gets that repository's git
history, review, and tests.

## Nothing is mounted after a reboot 🔌

`airfs` has no supervision of its own. Nothing survives a reboot unless something
restarts the daemon — see the systemd user unit in
[running-the-daemon.md](running-the-daemon.md#across-reboots-systemd-optionally).

## The report names a path I did not expect 🧭

Every relative path *inside* the configuration resolves against **that file's
directory**, never against your working directory. A path typed on the command
line is the opposite: it resolves against where you are standing.

`airfs ls` prints the `config` path it used before anything else, and every report
names sources and targets **as you declared them** rather than by a resolved
symlink target.
