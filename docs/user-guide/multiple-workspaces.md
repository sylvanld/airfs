# Multiple workspaces 🪟

The same layers can feed more than one workspace, and each workspace sees only
the layers it declared. This is what makes a workspace a *context* — personal,
work, one per project — rather than one global pile.

## Two targets, two layer lists 🗂️

Each workspace is a directory with its own `sources.txt`:

```
~/.ai-resources/          # the default: personal + shared
  sources.txt
~/work/.ai-resources/     # work context: organization + client
  sources.txt
```

```bash
# ~/work/.ai-resources/sources.txt
~/ai/personal
$WORK/ai-platform
$WORK/client-acme
```

Serve them independently — they are separate processes and separate mounts:

```bash
airfs mount --detach
airfs mount --detach --target ~/work/.ai-resources
```

Layers are shared without either workspace seeing the other's. `~/ai/personal`
is read by both; neither can write to it.

Every command takes `--target`, so the rest follows:

```bash
airfs status  --target ~/work/.ai-resources
airfs sources --target ~/work/.ai-resources
airfs umount  --target ~/work/.ai-resources
```

## Trying a layer list without committing to it 🧪

`--target` and `--config` are independent: overriding the config does **not**
move the target. Two useful shapes fall out of that.

**Try an alternative list against a scratch target** — leaves your real workspace
untouched:

```bash
mkdir -p /tmp/try
airfs sources --target /tmp/try --config ./experiment.txt
airfs mount   --target /tmp/try --config ./experiment.txt --detach
ls /tmp/try/skills
airfs umount  --target /tmp/try
```

**Rebuild your usual workspace from a different list** — same place, different
layers, when you want to check what a proposed change to `sources.txt` would
actually serve:

```bash
airfs umount
airfs mount --config ~/ai/proposed-sources.txt --detach
```

Remember that relative paths *inside* a config file resolve against **that
file's** directory. `./experiment.txt` above declares its layers relative to the
directory holding `experiment.txt`, not relative to `/tmp/try` and not relative
to your shell — which is exactly what lets you keep an experimental list next to
the repositories it names.

## Answering "which workspace am I looking at?" 🧭

`airfs sources` prints both paths before anything else:

```
target  /home/you/work/.ai-resources
config  /home/you/work/.ai-resources/sources.txt
```

Worth checking first whenever a result surprises you, because the most common
cause of "my edit did nothing" is having inspected a different workspace than the
one your tooling is reading.

## One process per workspace 🔢

Each `airfs mount` serves exactly one target, with its four mounts. Nothing is
shared between workspaces at runtime and nothing coordinates them; stopping one
has no effect on another. Mount state lives only in the kernel, so `airfs status`
against a target is always answering about that target alone.
