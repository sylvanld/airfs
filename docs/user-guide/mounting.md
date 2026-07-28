# Mounting a workspace 🧵

A workspace is exposed as **one FUSE mount per kind**, all served by one
process:

```
~/.ai-resources/
  sources.txt     # the layer list — never masked by a mount
  agents/         # mountpoint
  skills/         # mountpoint
  commands/       # mountpoint
  scripts/        # mountpoint
```

The mountpoints are the kind directories, one level *below* the target, which is
why `sources.txt` stays readable and editable while the view is live.

## Serving 🚀

```bash
airfs mount
```

Establishes all four mounts together and **blocks** while serving — the mount
lives exactly as long as the process. ++ctrl+c++ unmounts cleanly.

```bash
airfs mount --detach
```

Re-runs itself in its own session and returns **once the view is ready**, so a
shell profile or a service manager that gets exit `0` knows the workspace is
readable right now — not "probably, shortly".

```
Serving /home/you/.ai-resources from 3 sources:
  1. ~/ai/personal
  2. $WORK/platform
  3. ~/ai/project

Serving in the background. Stop it with: airfs umount --target /home/you/.ai-resources
```

Mounting is all-or-nothing: if any kind cannot be mounted, the ones already
established are released, and you are left with the state you started in.

### What gets refused, and why 🚧

| Refusal | Why |
| --- | --- |
| `<dir> is not empty` | Mounting over a populated directory hides its contents, and hidden files are a trap. Move them out — or, if they are a leftover copy of what you are about to merge, delete them. |
| `<dir> is already served` | The target is mounted. A second mount would stack a view on a view; `airfs umount` first. |
| `<dir> is held by a stale mount; unmount it first` | A previous serving process died. See below. |

All three exit `2`.

## Inspecting 📊

```bash
airfs status
```

```
target  /home/you/.ai-resources

  agents    served, 0 entries
  skills    served, 3 entries
  commands  served, 1 entry
  scripts   served, 0 entries
```

Exit `0` when the target is fully served; **exit `2`** when it is not — nothing
mounted, only some kinds mounted, or a mountpoint gone stale. Each of those is a
condition to act on, and each is named in the output, so a shell profile can
branch on the code and a human can read the reason:

```bash
airfs status --target ~/.ai-resources >/dev/null 2>&1 || airfs mount --detach
```

There is no PID file and no runtime directory. `airfs` asks the kernel what is
mounted, every time — the kernel is the only thing that actually knows, and a
second record of it could only ever disagree.

## Stale mounts 🧟

If the serving process is killed without a chance to clean up — `kill -9`, an
OOM kill, a crash — the kernel keeps listing the mount, but every access to it
fails with `ENOTCONN`. It looks mounted and serves nothing:

```
  skills    stale — the serving process died; recover with airfs umount
```

This is the failure worth being able to name, because from inside the directory
it presents as "my tooling stopped seeing anything" with no obvious cause.
`airfs status` distinguishes it from a live mount, and `airfs umount` recovers
it — lazily if it is still busy.

## Releasing 🧹

```bash
airfs umount
```

```
Released /home/you/.ai-resources/agents
Released /home/you/.ai-resources/skills
Released /home/you/.ai-resources/commands
Released /home/you/.ai-resources/scripts
```

Releases every kind under the target, stale ones included. If nothing was
mounted it says so and exits `0` — unmounting nothing is not a failure, which is
what makes it safe to put in a teardown script.

## Freshness 🔄

The view **caches nothing**: no content cache, no directory cache, and attribute
and lookup timeouts are all zero. Every read goes through to the real file in its
repository.

So the loop that matters just works:

- Edit a file in its repository → the next read through the view sees it.
- Add a whole new entry to a layer → it appears in the listing.
- Add a *layer* to `sources.txt` → this one **does** need a remount, since the
  layer list is resolved when serving starts.

The cost is that every operation is served live rather than from a cache. For
directories the size of a resource collection this is not measurable, and being
able to trust what you are reading is the entire point.

## Read-only, enforced 🛡️

Writes through the view are rejected by the **kernel**, not by convention:

```
touch: cannot touch '/home/you/.ai-resources/skills/new.md': Read-only file system
```

Modes reported through the view have their write bits cleared, and files are
reported as owned by you, so tools that check before writing get a consistent
answer instead of a surprise. Symlinks in a layer are served as symlinks.

Edit in the repository that owns the resource. A write that succeeded through the
view would bypass that repository's git history, review, and tests.

## Running it at login 🔌

`airfs` provides no supervision of its own beyond `--detach` — keeping a mount
alive across reboots is the host's service manager's job. A user unit is the
whole integration:

```ini
# ~/.config/systemd/user/airfs.service
[Unit]
Description=airfs merged AI resource view
After=default.target

[Service]
Type=forking
ExecStart=%h/go/bin/airfs mount --detach
ExecStop=%h/go/bin/airfs umount
Restart=on-failure

[Install]
WantedBy=default.target
```

```bash
systemctl --user enable --now airfs.service
```

`Type=forking` fits because `--detach` returns only once the view is ready, so
systemd's "started" and yours mean the same thing.
