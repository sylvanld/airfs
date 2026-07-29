# Changelog

All notable changes to `airfs` are documented here. Versions follow [semantic versioning](https://semver.org/).

## v0.3.0 — one file, one daemon 🏛️

> **You can finally answer "what is airfs doing on this machine?" in one place.** Until now every workspace hid its own `sources.txt` inside the directory it produced, findable only if you already knew where to look, and every workspace ran its own process. Now one file declares all of them and one daemon holds all of them — and it reconciles against what the *kernel* says is mounted, so it accounts for everything on the host, including what a crashed daemon left behind.

- 🏛️ **One configuration, at `~/.config/airfs/config.yaml`.** Every workspace on the machine in one YAML file you can read, diff and put in git. Each one names its target, its ordered sources, and the folders it merges. Start with [Declaring workspaces](https://sylvanld.github.io/airfs/user-guide/declaring-workspaces/).
- 🏷️ **Folders are yours to name.** `agents`, `skills`, `commands` and `scripts` were hardcoded; now each workspace declares the subdirectories it wants merged, and `airfs` attaches no meaning to any of them. A tool that expects `prompts/` is one `-f prompts` away. The old four are still the default when you declare none.
- ✋ **Nothing is ever written into a source repository.** Resolution used to create missing subdirectories inside every repository you pointed it at. It does not any more — a source that lacks a folder simply contributes nothing to it.
- 🧵 **One daemon instead of one process per workspace.** `airfs up` serves everything enabled, `airfs down` stops it and releases every `airfs` mount there is, `airfs reload` catches up with a file you edited by hand. A systemd user unit is in [Running the daemon](https://sylvanld.github.io/airfs/user-guide/running-the-daemon/) if you want it at login.
- 🛡️ **A broken workspace no longer takes down the rest.** One mistyped path used to stop everything; now it fails that workspace alone, is reported with its reason, and every other workspace is served. When one file describes the machine, the blast radius of an edit has to be the block you edited.
- 🔌 **`airfs disable` — stop serving something without deleting it.** Deleting a workspace to pause it destroys the block you wrote, comments and ordering included, and re-adding brings none of it back. A disabled workspace is information; a deleted one is a gap.
- 🔍 **`airfs ls` and `airfs inspect` replace `airfs sources`.** `ls` is the inventory in one screen; `inspect <name>` is the deep read — sources in precedence order, entry counts per folder, and every shadowed entry with its winner and its losers.
- 🚨 **`airfs status` tells you which config the daemon actually loaded.** A daemon started with `--config` holds that file for its whole life, so the file you are editing and the file being served can drift apart — and every symptom of that looks like `airfs` ignoring your edit. Now it says so, on its own line.
- ✍️ **Editing commands keep what you wrote.** `add`, `rm`, `enable` and `disable` preserve comments, key order, YAML anchors and aliases. And because a merge key carries an edit to its aliases, they report every workspace whose meaning changed — not just the one you named.

> **This replaces v0.2.0's layout entirely. 💥** Your `sources.txt` files are not read any more and there is no automatic migration: run `airfs down` with the old binary, then `airfs add <name> --target <dir> -s <each source> -f <each folder>` once per workspace. Five minutes end to end is [Get started](https://sylvanld.github.io/airfs/get-started/).

## v0.2.0 — a workspace in one command 🚀

> **You no longer need a file before you can mount anything.** Until now, trying `airfs` at all meant opening an editor, writing a `sources.txt`, saving it in the right place, and only then mounting. That is a lot of ceremony to answer "what does this thing actually do?" — so now you can say what your layers are on the command line and be looking at the result a second later.

- ⚡ **`airfs mount -s`, once per layer.** `airfs mount --target ~/scratch-ws -s ~/ai/personal -s ~/ai/project` creates the target, writes its `sources.txt`, and serves the view. The order you pass the flags in is the precedence order, most general first — same rule as the file, because it *is* the file.
- ✍️ **What you typed is what gets written.** `~/ai/personal` stays `~/ai/personal` in the file instead of being frozen into `/home/you/...`, so the configuration you end up with is one you would have written by hand and can keep editing that way.
- 🛑 **A typo cannot cost you your configuration.** `-s` replaces the file whole rather than appending to it — but the new list has to resolve first. Name a directory that is not there and you get exit `2` and the configuration you already had, untouched.

> **It replaces, it does not add. 💥** Every layer you want has to be on the command line; whatever the file said before, comments and ordering included, is gone, and there is no backup. That is deliberate — a flag that appended would build a precedence order nobody wrote — but point it at a workspace you care about only once you have read [Declaring layers](https://sylvanld.github.io/airfs/user-guide/declaring-layers/).

## v0.1.0 — hello, world 🪄

> **Many sources, one read-only view, no copies.** Your AI capabilities are scattered across repositories — personal, work, one per project — and every tool wants a single folder. Copy them together and each copy starts drifting from the day you made it. `airfs` gives you the folder without the copies.

You write down which repositories a workspace is made of, in order. `airfs` shows them to you as one directory 📂. Nothing moves, nothing is duplicated, and editing a skill in the repository that owns it changes what every workspace layering it sees — right away, with no sync step to remember. 🔄

- 🥇 **Order decides.** `sources.txt` is one path per line, most general first. Two layers both shipping a `commit` skill? The one declared last wins — and it wins *whole*, never half of yours stitched onto half of theirs.
- 🕵️ **Nothing gets shadowed behind your back.** `airfs sources` prints the resolved list and names every entry that lost, right next to the one that beat it. A workspace you cannot explain is a workspace you cannot trust.
- 🚪 **One mount per kind.** `airfs mount` serves `agents/`, `skills/`, `commands/`, and `scripts/` together — in the foreground, or with `--detach` when you want your terminal back. `airfs status` says what is being served, and `airfs umount` takes it down, stale leftovers included.
- 🛡️ **Read-only, with the kernel enforcing it.** No tool, no agent, and no stray `mv` can write back into one of your source repositories through the view.
- 🐹 **Pure Go, so installing is one command.** It speaks FUSE over `/dev/fuse` by itself: no C library, no `mergerfs` to hunt down, nothing to unpack by hand.
- 🩺 **`airfs doctor` before you file a bug.** Mounting needs `/dev/fuse` and a setuid `fusermount3`, which no unprivileged process can conjure up. `doctor` checks both and names the package that provides the missing one.
- 📦 **A Go library first, a CLI second.** `sdk/layerfs` is an `fs.FS`, so your own program can read the merged view in process — no mount, no subprocess, nothing to parse — and `fs.WalkDir` and friends just work on it.

> **It is a first release, so: Linux for the mount 🐧** — mounting is Linux-only for now; the Go SDK is plain `fs.FS` and runs anywhere. Best next step is [Get started](https://sylvanld.github.io/airfs/get-started/) — about five minutes, ending with a workspace you can keep. 🚀
