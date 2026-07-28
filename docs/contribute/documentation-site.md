# Working on the documentation site

Everything under `docs/` is the site: the Markdown you edit is published as-is,
with no separate source tree. [Zensical](https://zensical.org) renders it, and
`docs/zensical.toml` configures it — `docs_dir = "."`, so `docs/` is both the
source and the project root for the generator.

## The targets

| Target | Does |
| --- | --- |
| `make docs-serve` | Builds and serves the site on <http://127.0.0.1:10000> with live reload, and opens it in your browser. Leave it running while you write. Another port: `make docs-serve DOCS_PORT=8000`. |
| `make docs-build` | Builds the site into `docs/build/`, in strict mode: any warning — a link to a page that does not exist, most often — fails the build. This is what CI publishes. |
| `make docs-clean` | Deletes `docs/build/` and the `docs/.cache/` build cache. Use it when a stale build confuses you; a normal build does not need it. |

`uv` installs Python and the generator on first run, so no manual environment
setup is needed. See [getting-started.md](getting-started.md) for prerequisites.

Because the whole directory is content, anything sitting in it is copied into the
site verbatim — which is why the `Makefile` keeps the Python environment outside
it, in `.venv-docs/`. `pyproject.toml`, `uv.lock`, `zensical.toml`, and the build
cache are still copied; harmless, but do not add anything larger or private to
`docs/` expecting it to stay unpublished.

## Links must stay inside `docs/`

Strict mode rejects a relative link that leaves `docs/`, because such a link
resolves to nothing once the site is published: a file at the repository root is
not part of the site. Refer to such a file by name in prose, or link its
canonical URL, but do not link it relatively.

## Adding a page

A new page under `docs/specs/` or `docs/contribute/` must be listed in that
directory's `index.md`, which `make check` enforces. See
[quality-gates.md](quality-gates.md).
