# Get started 🚀

Install `airfs`, declare a workspace over two repositories, and read them as one
directory. About five minutes, ending with a workspace you can keep.

## 1. Install 📦

`airfs` links no C library, so installing is one command and needs no toolchain
beyond Go:

```bash
go install github.com/sylvanld/airfs/cmd/airfs@latest
```

Make sure `$(go env GOPATH)/bin` is on your `PATH`, then check it runs:

```bash
airfs help
```

!!! tip "No Go on this machine?"

    Every release ships a prebuilt binary for Linux and macOS on `amd64` and
    `arm64`. One command picks the right one, checks it against the published
    SHA-256, and installs it into `~/.local/bin` — no toolchain, no root:

    ```bash
    curl -fsSL https://raw.githubusercontent.com/sylvanld/airfs/main/scripts/get-airfs.sh | sh
    ```

    Two environment variables change what it does:

    ```bash
    curl -fsSL https://raw.githubusercontent.com/sylvanld/airfs/main/scripts/get-airfs.sh \
      | AIRFS_VERSION=v0.1.0 AIRFS_INSTALL_DIR=/usr/local/bin sh
    ```

    `AIRFS_VERSION` installs a specific release tag instead of the latest, and
    `AIRFS_INSTALL_DIR` chooses where the binary lands — a system-wide directory
    needs a `sudo` you type yourself, since the script never asks for one.

    Prefer doing it by hand? The archives and their `checksums.txt` are on the
    [releases page](https://github.com/sylvanld/airfs/releases); unpack the one
    for your platform and `install -m 755 airfs ~/.local/bin/airfs`.

## 2. Check the host 🩺

Mounting needs two things an unprivileged process cannot provide for itself:
`/dev/fuse`, and a **setuid** `fusermount3`. Both ship with your distribution's
FUSE package and are already present on a normal desktop Linux.

Now that `airfs` is installed, ask it — before installing anything else:

```bash
airfs doctor
```

```
  ok       /dev/fuse        readable and writable by you
  ok       fusermount3      /usr/bin/fusermount3, setuid
  ok       control socket   /run/user/1000/airfs/control.sock

Every prerequisite is satisfied.
```

If something is missing, `doctor` names the package that provides it and exits
`2`. Install it with your package manager — `fuse3` on Debian, Ubuntu, Fedora,
and Arch — then run `airfs doctor` again.

!!! info "Why airfs will not install it for you"

    Installing a system package needs root, and a tool that asks you for root to
    run a package manager is a tool that should have printed the command instead.

## 3. Make two source repositories 🗂️

For this walkthrough, two sources — a personal one and a project one:

```bash
mkdir -p ~/ai/personal/skills/commit ~/ai/personal/skills/review
mkdir -p ~/ai/project/skills/commit

echo "personal commit skill" > ~/ai/personal/skills/commit/SKILL.md
echo "personal review skill" > ~/ai/personal/skills/review/SKILL.md
echo "project commit skill"  > ~/ai/project/skills/commit/SKILL.md
```

In real use these are git working copies you already have — nothing about a
source is `airfs`-specific, and **`airfs` never writes into one**. ✋

## 4. Declare a workspace ✍️

One command brings it into being, no editor required. The order of the `-s`
flags **is** the precedence order: most general first, last wins.

```bash
airfs add personal \
  --target ~/.ai-resources \
  -s ~/ai/personal \
  -s ~/ai/project \
  -f skills
```

```
Declared personal in /home/you/.config/airfs/config.yaml:
  target   ~/.ai-resources
  folders  skills
  sources, in precedence order — the last declaration wins:
    1. ~/ai/personal
    2. ~/ai/project
Changed: personal

No daemon is running, so nothing was mounted or released.
The configuration is written. Serve it with: airfs up
```

Two things to notice. The paths were written down **as you typed them** — `~/ai/personal`
stays that, rather than being frozen into `/home/you/...` — so the file stays
something you would have written by hand. And the command wrote the file even
though nothing is serving it yet: the declaration is the durable half. It exits
`2` to say the machine has not caught up. 📝

`-f skills` says which subdirectories to merge. Give none and you get the default
set, `agents skills commands scripts`. **`airfs` attaches no meaning to any of
these names** — declare `-f prompts` and it merges `prompts/`.

Now ask what that declaration actually means, before serving it:

```bash
airfs inspect personal
```

```
workspace  personal
target     ~/.ai-resources
folders    skills

Sources, in precedence order — the last declaration wins:
  1. ~/ai/personal  skills 2
  2. ~/ai/project   skills 1

Shadowed entries — the winner is what the view serves:
  skills/commit  wins ~/ai/project  over ~/ai/personal
```

Both sources ship a `commit` skill, so one has to lose. The report says which, by
name — the point being that **shadowing is never silent**. 🕵️ `inspect` reads and
reports only; it mounts nothing and needs no daemon.

## 5. Serve it 🧵

```bash
airfs up --detach
```

```
daemon  running since Mon, 28 Jul 2026 09:14:02 CEST
config  /home/you/.config/airfs/config.yaml

  personal  served     ~/.ai-resources

Mounted for personal:
  /home/you/.ai-resources/skills  2 entries

Serving in the background. Stop it with: airfs down
```

One daemon holds every workspace you declare — not one process each. Without
`--detach` it blocks in the foreground and releases everything on ++ctrl+c++,
which is what you want from a service manager unit, or while you are still
experimenting.

## 6. Read the merged view 👀

```bash
ls ~/.ai-resources/skills
```

```
commit  review
```

One directory, both sources. And the winner won *whole*:

```bash
cat ~/.ai-resources/skills/commit/SKILL.md
```

```
project commit skill
```

Now the part that makes this worth doing — edit the file **in its repository**
and read it again through the view:

```bash
echo "edited in place" > ~/ai/project/skills/commit/SKILL.md
cat ~/.ai-resources/skills/commit/SKILL.md
```

```
edited in place
```

No sync step, no reload. 🔄 The view holds no copy and caches nothing; it reads
through to the file you just edited. Adding a whole new skill to a source works
the same way — it appears in the listing immediately.

The other direction is refused, by the kernel rather than by convention:

```bash
touch ~/.ai-resources/skills/new.md
```

```
touch: cannot touch '/home/you/.ai-resources/skills/new.md': Read-only file system
```

A new file in the merged view would belong to *some* source, and the view has no
basis to pick one. Create it in the repository that owns it. 🛡️

## 7. Check, pause, stop 📊

```bash
airfs status
```

```
daemon  running since Mon, 28 Jul 2026 09:14:02 CEST
config  /home/you/.config/airfs/config.yaml

  personal  served     ~/.ai-resources

Mounted for personal:
  /home/you/.ai-resources/skills  2 entries
```

`status` exits `0` when every enabled workspace is fully served and `2` when any
is not, so a shell profile can branch on it without reading the prose.

To stop serving one workspace without losing what produced it:

```bash
airfs disable personal    # released from the machine, untouched in the file
airfs enable personal     # back
```

And to stop everything:

```bash
airfs down
```

```
Stopped the daemon.

  released       personal — the daemon is stopping
```

`down` releases **every** `airfs` mount on the machine — including anything a
previous daemon left behind, or a mount whose serving process died. That is what
makes it the one recovery command you need. 🧹

Your declaration is still in `~/.config/airfs/config.yaml`. `airfs up` restores
exactly what was running.

## Where next 👉

- 📚 [User guide](user-guide/index.md) — sources, precedence, the daemon, the Go API
- 🧭 [Declaring workspaces](user-guide/declaring-workspaces.md) — the full file format
- 🧵 [Running the daemon](user-guide/running-the-daemon.md) — reload, systemd, reconciliation
- 🩹 [Troubleshooting](user-guide/troubleshooting.md) — when something is not served
- 📐 [Specs](specs/index.md) — why it behaves the way it does
