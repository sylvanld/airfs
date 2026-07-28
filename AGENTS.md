# AGENTS.md

Guidance to work in this repo.

## Spec first

No implementation change without a spec: write or update the spec, get it
agreed, then implement - and update the spec if reality forces a change.
Exempt: typos, formatting, dependency bumps.

## Specs live in `docs/specs/`

- One file per spec, `<kebab-case-topic>.md`. It states *what* and *why*:
  purpose, scope, behaviour, data shapes, edge cases, non-goals.
- A spec may describe non-trivial interfaces and which implementations should
  exist, and may document an implementation's behaviour when that behaviour is
  non-trivial - including the algorithm itself when the algorithm *is* the
  requirement (ordering guarantees, retry and backoff, dedup rules).
- What it must not contain is the *code*: a spec is not the place for the
  concrete structure of a function, its naming, or a snippet standing in for
  the real thing. Describe the algorithm so it can be judged and re-derived,
  not so it can be pasted.
- [`docs/specs/index.md`](docs/specs/index.md) indexes them all, with a
  one-line description and a status (`draft` | `accepted` | `implemented` |
  `superseded`). Update it in the same change as the spec.
- Read the index first, to see whether a spec already covers what you are about
  to touch.

## User documentation lives in `docs/user-guide/`

How to *use* what is built: the commands, the configuration file, the Go API,
and what to do when something does not work. One document per question, listed
in [`docs/user-guide/index.md`](docs/user-guide/index.md).

It describes behaviour that exists, so it is written or updated in the same
change that implements the behaviour - a spec says what should be true, this
says how to rely on it. `docs/get-started.md` is the one path through it that
must stay working end to end; verify its commands against a real run rather than
from memory.

## Release notes live in `docs/changelog.md`

One section per released version, newest first, titled with the tag and a short
name for the release. Its body is what *changed* in that version, as a flat list
of what a user gains or loses - no sub-headings to navigate, since a release is
read top to bottom once.

Write it as an announcement, not a diff: address the reader, say why an entry
matters to them rather than only what it is, and let a leading blockquote carry
the one-line pitch and a closing one carry the caveats and the link to what to
read next, each opening on a bold sentence that stands in for its title. An emoji per entry is welcome - it makes a long list
skimmable - but one, and only where it means something.

This is the one document written in plain CommonMark rather than in the
Markdown extensions the rest of `docs/` uses: a section of it gets pasted into
a GitHub release page, which renders neither `!!!` admonitions nor `> [!NOTE]`
alerts - a plain `>` blockquote is what carries an aside there. Same reason, links out of it are absolute URLs to the published site,
not relative paths to other files - a relative link is dead once it leaves the
site. Lines are not wrapped at 80 characters either, since a release page
reflows them itself.

A changelog entry does not repeat the documentation. Installation commands,
usage, limitations, and anything a reader would still need next month belong in
`docs/get-started.md` or `docs/user-guide/`; the entry links there instead of
restating them. If a bullet would stay true forever, it is documentation that
wandered into the wrong file.

## How-to lives in `docs/contribute/`

Commands, tooling, setup, pre-push checks - one document per question, listed in
[`docs/contribute/index.md`](docs/contribute/index.md). Check it before starting
work instead of guessing; update it (and its index) when practices change.

The `Makefile` is the executable source of truth for how the project is run:
`make` lists the targets, `make check` runs every quality gate. Documents name
targets rather than restating the commands behind them, and a new command means
a new target plus the line in the doc that points at it.
