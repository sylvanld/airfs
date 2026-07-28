# Changelog

All notable changes to `airfs` are documented here. Versions follow [semantic versioning](https://semver.org/).

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
