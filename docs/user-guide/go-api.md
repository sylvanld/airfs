# Using the Go API 🐹

`airfs` is a Go SDK first; the command line is a thin frontend over it. A program
that needs the merged view consumes it **in process**, with no mount, no
subprocess, and nothing to parse.

```bash
go get github.com/sylvanld/airfs
```

There is deliberately no machine-readable output mode on the CLI. A second,
stringly-typed interface for the same data would be a contract to keep stable for
no caller that could not do better than parsing prose.

## The merged view is an `fs.FS` 📂

`layerfs.FS` implements `fs.FS`, `fs.ReadDirFS`, and `fs.StatFS`, so everything
in the standard library that consumes a filesystem already works — `fs.WalkDir`,
`fs.ReadFile`, `template.ParseFS`, `http.FS`.

```go
cfg, err := sources.Load(configPath)
if err != nil {
    return err
}

skills := cfg.Merged(airfs.Skills)   // *layerfs.FS

data, err := fs.ReadFile(skills, "commit/SKILL.md")
```

`Merged` gives the union for one kind; `airfs.Kinds` iterates all four
(`airfs.Agents`, `airfs.Skills`, `airfs.Commands`, `airfs.Scripts`).

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
`airfs sources` prints.

## Mounting from Go 🧵

```go
if err := mount.Preflight(); err != nil {
    return err   // /dev/fuse or a setuid fusermount3 is missing
}

server, err := mount.Serve(target, cfg)
if err != nil {
    return err
}
defer server.Unmount()

server.Wait()   // blocks until unmounted
```

`Serve` establishes every kind together and releases them all if any fails — a
partially mounted target is a view that lies about what is available.

For inspection without serving, `mount.Status(target)` returns one `State` per
kind, read from the kernel's mount table. A `State` that is `Mounted` **and**
`Stale` is the signature of a serving process that died: still listed, failing
every access. `mount.Served(states)` collapses the four into the one boolean most
callers want.

`mount.Requirements()` gives the same host check `airfs doctor` prints, as data —
each with `Name`, `Satisfied`, `Detail`, and `ProvidedBy`.

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

| Package | Holds |
| --- | --- |
| `github.com/sylvanld/airfs` | The model: `Kind`, `Kinds`, the precondition contract, exit codes. |
| `.../airfs/layerfs` | The ordered read-only union, as a standard `fs.FS`. |
| `.../airfs/sources` | Reading and resolving `sources.txt` into layers. |
| `.../airfs/mount` | Exposing a view at a real path via FUSE, and reading mount state. |

Every dependency is cgo-free, so a program embedding `airfs` still builds with
`CGO_ENABLED=0`.
