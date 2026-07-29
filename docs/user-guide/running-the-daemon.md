# Running the daemon 🧵

**One daemon per user holds every workspace you declared.** Not one process per
workspace: a FUSE mount lives exactly as long as the process serving it, so
something has to stay alive — and one thing is far easier to reason about than
one per workspace.

```bash
airfs up              # serve every enabled workspace, blocking
airfs up --detach     # ...and give the terminal back
airfs down            # stop, and release every airfs mount on the machine
airfs reload          # re-read the config and catch up with it
airfs status          # what is the daemon doing?
```

Each **folder** of each workspace is its own mount:

```
~/.ai-resources/
  agents/         # mountpoint
  skills/         # mountpoint
  commands/       # mountpoint
```

A target holds nothing but those mountpoints. Your configuration lives in
`~/.config/airfs/config.yaml` — see [Declaring workspaces](declaring-workspaces.md). 🗂️

## Reconciliation: the only thing it does 🔁

Everything the daemon does is **one operation**: make what is mounted match what
is declared. It runs at startup, on reload, and nowhere else.

It takes two inputs — your resolved configuration, and **every `airfs` mount on
the machine** as reported by the kernel. Not just the ones under a target you
currently declare.

That second input is the point. It is what lets one command account for mounts
left behind by a previous configuration, or by a previous daemon, or by one that
crashed. And it comes from the kernel rather than a file `airfs` keeps, because a
file `airfs` keeps disagrees with reality exactly when it matters: after a crash,
after a manual `fusermount3 -u`, after you edited the config while nothing was
running. 🗝️

> **The kernel is the inventory; the configuration is the intent.**

A workspace is **wanted** when it is declared *and* enabled. From there:

| Situation | What happens |
| --- | --- |
| Wanted, nothing mounted | ✅ Established |
| Wanted, mounted, config identical | 😴 Left alone |
| Wanted, mounted, anything changed | 🔄 Released and established again |
| Wanted, mounted, no record of it | 🔄 Released and established again |
| Disabled, mounted | 🧹 Released |
| Mounted at no declared target | 🧹 Released |
| Stale — listed by the kernel, failing every access | 🧹 Released, by whichever rule meets it |

"Left alone" is the guarantee that makes `reload` usable at all: **adding a
workspace must not disturb the one an agent is reading right now.** 🤝

"Released and established again" is the honest answer to a changed source list. A
union is immutable once built; there is no adjusting one in place.

!!! info "Reconciliation is idempotent"

    Running it twice against an unchanged configuration is a no-op the second
    time. That is what makes it safe to run on every reload without asking
    whether it is needed.

## One bad workspace does not take down the rest 🛡️

A workspace that cannot be established — a source that is not there, a non-empty
mountpoint, a target that cannot be created — **fails that workspace and no
other**. It is reported, reconciliation continues, and everything else is served.

This is the change one shared file forces. When one file described one workspace,
refusing to proceed was right: everything it described was broken. Now that one
file describes the machine, one mistyped path must not take down the rest, and
the blast radius of an edit has to be the block you edited.

Within a workspace the rule is unchanged: its folders are established together
and released together. A half-served workspace is a view that lies about what is
available. ⚖️

A failed workspace is **not retried on a timer**. It is reported, and it is tried
again on the next reconciliation — by which time you have fixed the reason, or
you have not.

## `reload` is a command, not a file watch 👀

The daemon does not watch your configuration file. An editor writing a file is
not one event, and a daemon reacting to the first of them would act on half a
document.

The declaring commands (`add`, `rm`, `enable`, `disable`) reload on their own, so
`airfs reload` is for a file you edited **by hand**.

A configuration that does not parse or does not validate changes **nothing at
all** — neither the running mounts nor the daemon's idea of what is declared —
and the error comes straight back to you.

## `down` stops the machine; `disable` stops a workspace 🛑

They are not the same thing, and neither writes anything the other reads:

- **`airfs down`** stops the daemon and releases every `airfs` mount there is,
  including ones a previous daemon left and ones whose serving process died. It
  works with **no daemon running**, which makes it the single recovery command —
  "release what should not be mounted" is one operation regardless of what is
  alive. Every declaration is left intact, and `airfs up` restores exactly what
  was running.
- **`airfs disable <name>`** takes one workspace out of service and leaves the
  daemon serving the rest.

## Reading `airfs status` 📊

```
daemon  running since Mon, 28 Jul 2026 09:14:02 CEST
config  /home/you/.config/airfs/config.yaml

  personal  served     ~/.ai-resources
  work      NOT SERVED ~/work/.ai-resources — source ~/work/project-capabilities: no such file or directory
  scratch   disabled

Mounted for personal:
  /home/you/.ai-resources/agents  0 entries
  /home/you/.ai-resources/skills  7 entries
```

In order, it tells you:

1. **Whether a daemon is running**, and since when.
2. **Which configuration file it loaded** — the one the daemon actually holds,
   not the one this command would read.
3. **Every workspace that file declares**: enabled or disabled, served or not,
   with the reason when an enabled one is not.
4. **What is mounted**, per workspace, per folder — including anything stale.

!!! danger "Watch line 2"

    A daemon started with `--config` holds that file for its **whole life**. So
    the file you are editing and the file being served can drift apart — and
    every symptom of that looks like `airfs` ignoring your edit. When they
    differ, `status` says so on its own line. 🚨

Points 1 and 4 come from the kernel, so `status` works fine with **no daemon
alive** — which is exactly the state that most needs reporting. Points 2 and 3
come from the daemon and are reported as unavailable when there is none.

`status <name>` limits the report to one workspace. It never tells you what a
workspace *merges* — that is [`airfs inspect`](precedence.md), and it is true
whether or not anything is running.

**Exit code:** `0` when every enabled workspace is fully served, `2` when any is
not. A disabled workspace serving nothing is the configuration being honoured, so
it does not affect the code.

## Across reboots: systemd, optionally ♻️

Nothing survives a reboot, and the daemon supervises nothing. If your host runs
systemd, the unit shipped in the repository at `dist/airfs.service` starts it at
login:

```bash
install -Dm644 dist/airfs.service ~/.config/systemd/user/airfs.service
```

```ini title="dist/airfs.service"
[Unit]
Description=airfs — merged read-only view of AI resource repositories
Documentation=https://sylvanld.github.io/airfs/

[Service]
Type=simple
ExecStart=%h/.local/bin/airfs up
ExecReload=%h/.local/bin/airfs reload
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=default.target
```

`up` runs in the foreground here on purpose — it releases every mount it holds
when systemd stops it, which is exactly what a `simple` service should do.
Detaching would leave systemd supervising a process that exits immediately.

```bash
systemctl --user daemon-reload
systemctl --user enable --now airfs
systemctl --user reload airfs      # after editing the config by hand
```

`airfs` does not require systemd, does not detect it, and behaves identically
under it. Without it, `airfs up --detach` from your shell profile does the same
job.

!!! note "Termination"

    A signal that permits cleanup unmounts first. One that does not — `SIGKILL`,
    a crash, power loss — leaves stale mounts, which the next reconciliation
    releases. There is nothing to clean up by hand. 🧹

## Where next 👉

- [Precedence](precedence.md) — which layer wins, and how to see it
- [Several workspaces](multiple-workspaces.md) — sharing sources between them
- [Troubleshooting](troubleshooting.md) — when something is not served
