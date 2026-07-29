# Several workspaces 🪟

The same sources can feed more than one workspace, and each workspace sees only
the sources it declared. This is what makes a workspace a *context* — personal,
work, one per project — rather than one global pile.

## Several blocks in one file 🗂️

```yaml
workspaces:
  personal:
    target: ~/.ai-resources
    folders: [agents, skills, commands]
    sources:
      - ~/ai/personal
      - ~/ai/shared

  work:
    target: ~/work/.ai-resources
    folders: [skills, prompts]
    sources:
      - ~/ai/personal          # read by both; written by neither
      - $WORK/ai-platform
      - $WORK/client-acme
```

One daemon serves both. `~/ai/personal` contributes to each without either
workspace seeing the other's sources, and neither can write to it. ✋

Commands take the workspace **by name** — there is no `--target` override any
more, because a target is a property of a declared workspace rather than of an
invocation:

```bash
airfs inspect work
airfs status work
airfs disable work
```

## Two workspaces cannot share a target 🚧

A shared `target`, or one target nested inside another, is a **hard error**: both
would have two workspaces writing mountpoints into one tree, and the inner one
would be mounting inside a directory that is itself being mounted over.

Different `folders` over the same sources is fine, and is often the point — one
workspace exposing `skills/` for the tool that wants that, another exposing
`prompts/` for the tool that wants this, over the same repositories.

## Trying a stack without committing to it 🧪

Declare it, look at it, and delete it — or better, disable it:

```bash
airfs add scratch --target /tmp/scratch-ws \
  -s ~/ai/personal -s ~/ai/experiment -f skills
airfs inspect scratch          # what would it merge? what shadows what?
ls /tmp/scratch-ws/skills      # it is being served now
airfs disable scratch          # released, still declared
```

`add` then `disable` is how a stack is tried without committing to it, and it
leaves a **record of what you tried**. An ephemeral mount would leave none.

You can also point one invocation at a completely different file:

```bash
airfs ls --config ./experiment.yaml
```

Remember that relative paths *inside* a config file resolve against **that
file's** directory — which is exactly what lets you keep an experimental
configuration next to the repositories it names.

!!! warning "`--config` on `up` is sticky"

    A daemon started with `--config` holds that file for its **whole life**. If
    you start one against an experimental file and then edit your real one,
    nothing will happen and it will look like a bug. `airfs status` reports which
    file the daemon actually loaded, and says so loudly when it is not the one
    your command would read. 🚨

## Answering "which workspace am I looking at?" 🧭

`airfs ls` is the whole inventory in one screen:

```
config  /home/you/.config/airfs/config.yaml

  personal  enabled   ~/.ai-resources       [agents skills commands]  2 sources
  work      enabled   ~/work/.ai-resources  [skills prompts]          3 sources
  scratch   disabled  /tmp/scratch-ws       [skills]                  2 sources
```

Worth checking first whenever a result surprises you: the most common cause of
"my edit did nothing" is having inspected a different workspace than the one your
tooling is reading. 🔍

## One daemon, not one per workspace 🔢

Every workspace on the machine is held by the same process. Mounts are cheap and
processes are not, and a process per workspace multiplies the things that can be
alive when they should not be — or dead when they should not be.

Stopping the daemon stops all of them; `airfs disable <name>` stops exactly one.
See [Running the daemon](running-the-daemon.md).
