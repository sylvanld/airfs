# Troubleshooting 🩹

Start with the three commands that only report, in this order. They cost nothing
and they answer most questions before you have to guess:

```bash
airfs doctor    # can this host mount at all?
airfs sources   # what does my layer list actually mean?
airfs status    # is this workspace being served right now?
```

## `airfs doctor` says something is missing 🩺

```
  ok       /dev/fuse      readable and writable by you
  MISSING  fusermount3    not found on PATH
                          provided by the system FUSE package, libfuse3-3 on Debian and Ubuntu

airfs: a mount prerequisite is missing; install what provides it, then run airfs doctor again
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

## My repository's resources are not showing up 🤷

Run `airfs sources` and read the counts:

```
  1. ~/ai/personal    agents 0  skills 2  commands 0  scripts 0
  2. ~/ai/project     agents 0  skills 0  commands 0  scripts 0
```

`skills 0` next to a repository you know has skills means one of:

- **The resources are not under a kind directory.** They must live at
  `<layer>/skills/<entry>`, not at `<layer>/<entry>` or nested deeper.
- **The kind is wrong.** The four are fixed: `agents`, `skills`, `commands`,
  `scripts`. A `tools/` directory contributes nothing.
- **The path points somewhere else than you think.** The report names layers as
  you declared them; a `$VAR` may be resolving to an unexpected directory.

If the layer does not appear at all, it is not in the file — check the `config`
path printed at the top, since `--config` and the default may differ.

## My edit did nothing 🕵️

Two causes, in order of likelihood.

**The entry is shadowed.** A later layer ships the same name and wins *whole*.
Check the shadowing report:

```
Shadowed entries — the winner is what the view serves:
  skills/commit  wins ~/ai/project  over ~/ai/personal
```

You edited `~/ai/personal/skills/commit/`; the view serves `~/ai/project`'s. Edit
the winner, or reorder the layers. Note that this covers files the winner does
not even have — a `helper.sh` in the losing copy is invisible, because entries
win whole rather than file by file.

**The mount is stale.** The serving process died, and the kernel still lists the
mountpoint while every access to it fails:

```
  skills    stale — the serving process died; recover with airfs umount
```

```bash
airfs umount && airfs mount --detach
```

What is *not* a cause: caching. The view caches nothing, so an edit to a file in
a layer that is genuinely winning is visible on the very next read.

## I added a layer and nothing changed 🔄

The layer list is resolved when serving starts. Editing `sources.txt` does not
affect a running mount — remount:

```bash
airfs umount && airfs mount --detach
```

Editing *within* an existing layer needs no remount; only changing the set of
layers does.

## `<dir> is not empty` 🚧

The mountpoint has files in it, and mounting over them would hide them —
silently, until someone spends an afternoon on it. Look before you clear it:

```bash
ls -la ~/.ai-resources/skills
```

If it is a leftover copy of resources you are now layering, delete it. If it is
something you still want, move it into one of your layers so it is served
properly.

## `<dir> is already served` 🔁

The target is mounted. `airfs status` will confirm it. Use `airfs umount` first,
or pick a different `--target` — a second mount over the same directory would
stack a view on a view.

## `Read-only file system` 🛡️

Working as intended: the kernel enforces it. A new file in the merged view would
belong to *some* layer, and the view has no basis to pick one. Create or edit the
file in the repository that owns it, where it gets that repository's git history,
review, and tests.

## Nothing is mounted after a reboot 🔌

`airfs` has no supervision of its own. Mounts do not survive a reboot unless
something restarts them — see the systemd user unit in
[mounting.md](mounting.md#running-it-at-login).

## The report names a path I did not expect 🧭

Every relative path in `sources.txt` resolves against **that file's directory**,
never against your working directory. If you keep an experimental list somewhere
else, its relative layers are relative to *there*. The absolute paths are in the
error messages; `airfs sources` prints the `target` and `config` it used before
anything else.
