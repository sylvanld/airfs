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

## How-to lives in `docs/contribute/`

Commands, tooling, setup, pre-push checks - one document per question, listed in
[`docs/contribute/index.md`](docs/contribute/index.md). Check it before starting
work instead of guessing; update it (and its index) when practices change.

The `Makefile` is the executable source of truth for how the project is run:
`make` lists the targets, `make check` runs every quality gate. Documents name
targets rather than restating the commands behind them, and a new command means
a new target plus the line in the doc that points at it.
