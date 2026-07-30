# Using the Go API 🐹

`airfs` is a Go SDK first; the command line is a thin frontend over it. A program
that needs the merged view consumes it **in process**, with no mount, no
subprocess, and nothing to parse.

```bash
go get github.com/sylvanld/airfs/sdk/...
```

There is deliberately no machine-readable output mode on the CLI. A second,
stringly-typed interface for the same data would be a contract to keep stable for
no caller that could not do better than parsing prose.

## The merged view is an `fs.FS` 📂

`layerfs.FS` implements `fs.FS`, `fs.ReadDirFS`, and `fs.StatFS`, so everything
in the standard library that consumes a filesystem already works — `fs.WalkDir`,
`fs.ReadFile`, `template.ParseFS`, `http.FS`.

```go
cfg, err := config.Load(configPath)
if err != nil {
    return err
}

w := cfg.Lookup("personal")          // *config.Workspace
skills := w.Merged("skills")         // *layerfs.FS

data, err := fs.ReadFile(skills, "commit/SKILL.md")
```

`Merged` gives the union for one folder, and `w.Folders` is what that workspace
declared. `config.DefaultFolders` is the set used when a workspace declares
none.

An `FS` is immutable once constructed and safe for concurrent use, and it holds
no cache of any kind — an edit inside a layer is visible through it immediately.
Changing the *set* of layers means constructing a new one.

## Building a union directly 🧩

You do not need a config file, or even a disk. `layerfs.New` takes any `fs.FS`,
last winning — which is what makes merge behaviour testable without mounting
anything:

```go
merged := layerfs.New(
    layerfs.Layer{Name: "personal", FS: os.DirFS(personalSkills)},
    layerfs.Layer{Name: "project",  FS: fstest.MapFS{
        "commit/SKILL.md": {Data: []byte("project version")},
    }},
)
```

`Layer.Root` is the real directory backing `FS`, when there is one; it is empty
for an in-memory layer. The union never uses it — only a frontend that wants to
read through to the actual file does.

## Asking where an entry came from 🔎

```go
layer, ok := merged.Origin("commit")   // which layer serves this entry
```

`Origin` and the directory listing derive from the same index, so a name you
listed and a name you looked up always resolve to the same layer.

## Reporting shadowing 🕵️

```go
shadows, err := merged.Shadowed()
for _, s := range shadows {
    log.Printf("%s: %s wins over %d other layer(s)", s.Name, s.Winner.Name, len(s.Losers))
}
```

This is what makes precedence auditable from your own tooling, in the same terms
`airfs inspect` prints.

## Mounting from Go 🧵

```go
if err := mount.Preflight(); err != nil {
    return err   // /dev/fuse or a setuid fusermount3 is missing
}

server, err := mount.Serve(w)   // one *config.Workspace
if err != nil {
    return err
}
defer server.Unmount()

server.Wait()   // blocks until unmounted
```

`Serve` establishes every folder together and releases them all if any fails — a
partially mounted workspace is a view that lies about what is available.

For inspection without serving, `mount.Mounts()` returns **every `airfs` mount on
the machine**, read from the kernel's mount table — not just the ones you know
about. A `Mount` that is `Stale` is the signature of a serving process that died:
still listed by the kernel, failing every access. `mount.Under(mounts, dir)`
narrows the list to one target.

`mount.Requirements()` gives the same host check `airfs doctor` prints, as data —
each with `Name`, `Satisfied`, `Detail`, and `ProvidedBy`.

## Linking a project's tools 🔗

`sdk/link` is what `airfs link` is a frontend over, and it is the one package
that knows anything about a specific tool. It operates on a project directory,
not on a workspace: no configuration is read and no daemon is involved.

```go
claude, ok := link.Lookup("claude")   // one row of the tool table
if !ok {
    return fmt.Errorf("no such tool")
}

report, err := link.Run(link.Options{
    Project: projectDir,          // absolute; the frame of reference
    Tools:   []link.Tool{claude}, // linked in this order, and order decides
    Root:    link.DefaultRoot,    // ".ai", and it must resolve inside Project
})
```

A `*link.Report` is the printed report as data: `Root`, and one `Outcome` per
tool whose `Action` is `link.Linked`, `link.Unchanged` or `link.Refused`. A
refusal fails that tool alone and carries its `Err`; `report.Refused()` is what
the CLI turns into exit `2`. The error `Run` itself returns is for a failure of
the whole run — an unusable root, a project that is not there.

Each outcome's `Adopted` is every entry moved out of that tool's directory, by
name: `Name` as the tool had it, `As` as the root names it, `Taken` naming who
held the name when `Renamed()`, and `Deduped` for the one thing a run
discards — a byte-identical copy. `Options.DryRun` computes all of it and writes
nothing.

```go
for _, t := range link.Tools() {   // the whole table, as --list prints it
    fmt.Println(t.Flag, t.Type, t.Path)
}
```

`Tools` is the same table `airfs link --list` prints. It is the only place a
tool's name means anything to `airfs`, and nothing it knows reaches the
configuration, the merge, or the mount layer.

## Driving the daemon 🧭

```go
d, outcomes, err := daemon.Start(configPath)   // reconciles, then listens
if err != nil {
    return err
}
defer d.Stop()

for _, o := range outcomes {
    log.Println(o)   // established / re-established / unchanged / released / failed
}
return d.Wait()
```

From another process, `daemon.Dial` reaches a running one: `Status`, `Reload`,
`Shutdown`. `daemon.Running()` is the liveness check, and it verifies itself — a
socket left behind by a dead daemon refuses connections, so "can I connect" and
"is it alive" are the same question.

!!! note "The control socket is private"

    It is an implementation detail between a daemon and the CLI built from the
    same source: not documented for third parties, not versioned. The **SDK** is
    the supported programmatic interface, and a program that wants the merged
    view just reads the mounted directory. 🔒

## Errors you should branch on ⚠️

Failures that mean *the host or the configuration needs attention* — rather than
*`airfs` malfunctioned* — are marked as preconditions:

```go
if airfs.IsPrecondition(err) {
    // missing config, unresolvable layer, non-empty mountpoint,
    // absent mount prerequisite — tell the user what to fix
}
```

It unwraps, so it works through your own `fmt.Errorf("%w")` wrapping. This is the
same distinction the CLI turns into exit code `2`.

## Package map 🗺️

Everything importable lives under `sdk/`; `cmd/airfs` is the frontend and is not
part of the API. The `sdk` directory holds package `airfs`, so an import of
`.../airfs/sdk` is used as `airfs.IsPrecondition`, `airfs.ExitPrecondition`, and
so on.

| Package | Holds |
| --- | --- |
| `github.com/sylvanld/airfs/sdk` | Package `airfs`: the precondition contract and the exit codes. |
| `.../sdk/layerfs` | The ordered read-only union, as a standard `fs.FS`. |
| `.../sdk/config` | Reading, resolving and **editing** the workspace configuration. |
| `.../sdk/mount` | Exposing a workspace at a real path via FUSE, and reading the kernel's mount table. |
| `.../sdk/daemon` | Reconciling declared workspaces against what is mounted, and the control socket. |
| `.../sdk/link` | Pointing a project's tools at the resources it owns, and the table of tool layouts. |

Every dependency is cgo-free, so a program embedding `airfs` still builds with
`CGO_ENABLED=0`.
