# Working on the documentation site

Everything under `docs/` is the site: the Markdown you edit is published as-is,
with no separate source tree. [Zensical](https://zensical.org) renders it, and
`zensical.toml` at the repository root configures it — `docs_dir = "docs"`.

The configuration sits at the root rather than next to the Markdown so that what
the generator writes — the built site in `site/`, the build cache in `.cache/` —
lands outside `docs/`. Everything inside `docs/` is published verbatim, and
neither of those belongs in a published site.

## The targets

| Target | Does |
| --- | --- |
| `make docs-serve` | Builds and serves the site on <http://127.0.0.1:10000> with live reload, and opens it in your browser. Leave it running while you write. Another port: `make docs-serve DOCS_PORT=8000`. |
| `make docs-build` | Builds the site into `site/`, in strict mode: any warning — a link to a page that does not exist, most often — fails the build. Run it before pushing; publishing rebuilds from scratch. |
| `make docs-deploy VERSION=0.1` | Publishes that version to the `gh-pages` branch and points `latest` at it. CI runs this on release; you should not need it by hand. |
| `make docs-clean` | Deletes `site/` and the `.cache/` build cache. Use it when a stale build confuses you; a normal build does not need it. |

`uv` installs Python and the generator on first run, so no manual environment
setup is needed. See [getting-started.md](getting-started.md) for prerequisites.

Because the whole directory is content, anything sitting in it is copied into the
site verbatim — which is why the `Makefile` keeps the Python environment outside
it, in `.venv-docs/`. `docs/pyproject.toml` and `docs/uv.lock` are still copied;
they are two small files and they belong with the docs, but do not add anything
larger or private to `docs/` expecting it to stay unpublished.

## How the site is published

The site is versioned, and a version only exists because a release does: pushing
to `main` publishes nothing. Publishing a GitHub release runs the `Documentation`
workflow, which uses [mike](https://github.com/squidfunk/mike) — the version
manager Zensical currently
[delegates versioning to](https://zensical.org/docs/setup/versioning/) — to build
the site and commit it to the `gh-pages` branch, then serves that branch through
GitHub Pages.

Each version is a directory on that branch, alongside the `versions.json` that
feeds the selector in the header:

```
gh-pages
├── 0.1/            the docs as of the 0.1 releases
├── 0.2/            the docs as of the 0.2 releases
├── latest/         a symlink to the newest of them
└── versions.json   what the version selector reads
```

A tag maps to its `major.minor`, so `v0.2.0` and `v0.2.3` both publish to `0.2`:
a patch release documents the same feature set it fixes. The `latest` alias moves
to whatever was published last, and the bare site URL redirects there, so a link
to the site without a version always lands on the current release. Older versions
stay reachable and untouched; earlier releases are never rebuilt, which is the
point — their documentation describes the code someone is actually running.

Each version is a full copy of the site, so anything sitting in `docs/` is stored
once per release. That is the other reason the build output and the cache are
kept out of it.

Two consequences while writing:

- The version selector is fed by `versions.json`, which only exists on a
  deployment. `make docs-serve` shows no selector; this is expected.
- The workflow can also be run manually from the Actions tab. That re-publishes
  the `gh-pages` branch as it stands — it repairs a broken Pages deployment, it
  does not pick up new writing.

## Links must stay inside `docs/`

Strict mode rejects a relative link that leaves `docs/`, because such a link
resolves to nothing once the site is published: a file at the repository root is
not part of the site. Refer to such a file by name in prose, or link its
canonical URL, but do not link it relatively.

## Adding a page

A new page under `docs/specs/` or `docs/contribute/` must be listed in that
directory's `index.md`, which `make check` enforces. See
[quality-gates.md](quality-gates.md).
