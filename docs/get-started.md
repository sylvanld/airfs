# Get started 🚀

Install `airfs`, declare two layers, and read them as one directory. About five
minutes, ending with a working workspace you can keep.

## 1. Check the host 🩺

Mounting needs two things an unprivileged process cannot provide for itself:
`/dev/fuse`, and a **setuid** `fusermount3`. Both ship with your distribution's
FUSE package and are already present on a normal desktop Linux.

Ask before installing anything:

```bash
airfs doctor
```

```
  ok       /dev/fuse      readable and writable by you
  ok       fusermount3    /usr/bin/fusermount3, setuid

Every mount prerequisite is satisfied.
```

If something is missing, `doctor` names the package that provides it and exits
`2`. Install it with your package manager — `fuse3` on Debian, Ubuntu, Fedora,
and Arch — then run `airfs doctor` again.

!!! info "Why airfs will not install it for you"

    Installing a system package needs root, and a tool that asks you for root to
    run a package manager is a tool that should have printed the command instead.

## 2. Install 📦

`airfs` links no C library, so installing is one command and needs no toolchain
beyond Go:

```bash
go install github.com/sylvanld/airfs/cmd/airfs@latest
```

Make sure `$(go env GOPATH)/bin` is on your `PATH`, then check it runs:

```bash
airfs help
```

## 3. Create a workspace 🗂️

A workspace is an ordinary directory holding one plain-text file. The default is
`~/.ai-resources`, which is where agent tooling usually looks:

```bash
mkdir -p ~/.ai-resources
```

For this walkthrough, two layers — a personal one and a project one:

```bash
mkdir -p ~/ai/personal/skills/commit ~/ai/personal/skills/review
mkdir -p ~/ai/project/skills/commit

echo "personal commit skill" > ~/ai/personal/skills/commit/SKILL.md
echo "personal review skill" > ~/ai/personal/skills/review/SKILL.md
echo "project commit skill"  > ~/ai/project/skills/commit/SKILL.md
```

In real use these are git working copies you already have — nothing about a
layer is `airfs`-specific, and `airfs` never writes into one.

## 4. Declare the layers ✍️

One path per line, in `~/.ai-resources/sources.txt`. **The order is the
precedence order: the last line wins.** Declare from the most general to the most
specific.

```bash
cat > ~/.ai-resources/sources.txt <<'EOF'
# Layers, most general first — the last declaration wins.
~/ai/personal
~/ai/project
EOF
```

Now ask `airfs` what that file means, before mounting anything:

```bash
airfs sources
```

```
target  /home/you/.ai-resources
config  /home/you/.ai-resources/sources.txt

Sources, in precedence order — the last declaration wins:
  1. ~/ai/personal  agents 0  skills 2  commands 0  scripts 0
  2. ~/ai/project   agents 0  skills 1  commands 0  scripts 0

Empty kinds: agents, commands, scripts

Shadowed entries — the winner is what the view serves:
  skills/commit  wins ~/ai/project  over ~/ai/personal
```

Both layers ship a `commit` skill, so one has to lose. The report says which, by
name — the point being that shadowing is never silent. `sources` reads and
reports only; it mounts nothing.

!!! note "Where did the empty kinds come from?"

    `airfs` creates the missing `agents/`, `commands/`, and `scripts/`
    directories inside each layer, so that every layer contributes every kind and
    adding a resource of a new kind never needs a `mkdir` first.

## 5. Mount it 🧵

```bash
airfs mount --detach
```

```
Serving /home/you/.ai-resources from 2 sources:
  1. ~/ai/personal
  2. ~/ai/project

Serving in the background. Stop it with: airfs umount --target /home/you/.ai-resources
```

Without `--detach`, `airfs mount` blocks in the foreground and unmounts on
++ctrl+c++ — which is what you want from a service manager unit, or while you are
still experimenting.

## 6. Read the merged view 👀

```bash
ls ~/.ai-resources/skills
```

```
commit  review
```

One directory, both layers. And the winner won *whole*:

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

No sync step, no remount. The view holds no copy and caches nothing; it reads
through to the file you just edited. Adding a whole new skill to a layer works
the same way — it appears in the listing immediately.

The other direction is refused, by the kernel rather than by convention:

```bash
touch ~/.ai-resources/skills/new.md
```

```
touch: cannot touch '/home/you/.ai-resources/skills/new.md': Read-only file system
```

A new file in the merged view would belong to *some* layer, and the view has no
basis to pick one. Create it in the repository that owns it.

## 7. Check and stop 📊

```bash
airfs status
```

```
target  /home/you/.ai-resources

  agents    served, 0 entries
  skills    served, 2 entries
  commands  served, 0 entries
  scripts   served, 0 entries
```

`status` exits `0` when the target is fully served and `2` when it is not, so a
shell profile can branch on it without reading the prose.

```bash
airfs umount
```

```
Released /home/you/.ai-resources/agents
Released /home/you/.ai-resources/skills
Released /home/you/.ai-resources/commands
Released /home/you/.ai-resources/scripts
```

## Where next 👉

- 📚 [User guide](user-guide/index.md) — layers, precedence, mounting, the Go API
- 🧭 [Declaring layers](user-guide/declaring-layers.md) — the full file format
- 🩹 [Troubleshooting](user-guide/troubleshooting.md) — when something is not served
- 📐 [Specs](specs/index.md) — why it behaves the way it does
