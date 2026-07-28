# Specs index

Every spec in this directory, with a one-line description and a status.

Status values: `draft` (proposed, not agreed) | `accepted` (agreed, not built) |
`implemented` (matches the code) | `superseded` (kept for history, points at its
replacement).

Read this index before touching anything, to see whether a spec already covers
it.

| Spec | Description | Status |
| --- | --- | --- |
| [layered-resources.md](layered-resources.md) | The core model: sources, resource kinds, precedence, shadowing | draft |
| [source-config.md](source-config.md) | Declaring sources and kinds: file format, path resolution, ordering | draft |
| [layered-fs.md](layered-fs.md) | Ordered read-only `fs.FS` union: lookup, listing, dedup, edge cases | draft |
| [fuse-mount.md](fuse-mount.md) | Serving any `fs.FS` as a read-only FUSE mount in pure Go | draft |
| [symlink-farm.md](symlink-farm.md) | Materialising the merged view as symlinks where FUSE is unavailable | draft |
| [cli.md](cli.md) | The `airfs` command surface and its exit codes | draft |
