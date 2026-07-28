# Specs index

Every spec in this directory, with a one-line description and a status.

Status values: `draft` (proposed, not agreed) | `accepted` (agreed, not built) |
`implemented` (matches the code) | `superseded` (kept for history, points at its
replacement).

Read this index before touching anything, to see whether a spec already covers
it.

| Spec | Description | Status |
| --- | --- | --- |
| [layered-resources.md](layered-resources.md) | The core model: sources, resource kinds, precedence, shadowing | implemented |
| [source-config.md](source-config.md) | Declaring sources: file location, format, path resolution, ordering | implemented |
| [layered-fs.md](layered-fs.md) | Ordered read-only `fs.FS` union: lookup, listing, dedup, edge cases | implemented |
| [fuse-mount.md](fuse-mount.md) | Serving the view as one read-only FUSE mount per kind, in pure Go | implemented |
| [cli.md](cli.md) | The `airfs` command surface and its exit codes | implemented |
