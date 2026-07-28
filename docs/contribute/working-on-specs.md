# Working on specs

## When a spec is required

Before any implementation change. Write or update the spec, get it agreed, then
implement — and update the spec if reality forces a change while implementing.

Exempt: typos, formatting, dependency bumps.

## Before writing one

Read [`docs/specs/index.md`](../specs/index.md) first. Most of the time the topic
is already covered and the change belongs in an existing spec; a second spec on
the same topic leaves two documents that disagree, and neither is authoritative.

## Writing one

One file per spec, `<kebab-case-topic>.md`, stating *what* and *why*: purpose,
scope, behaviour, data shapes, edge cases, non-goals.

The line that takes the most judgement is how much of the implementation belongs
in the spec. A spec may describe non-trivial interfaces, which implementations
should exist, and an implementation's behaviour when that behaviour is non-trivial
— including the algorithm itself when the algorithm *is* the requirement, as with
the precedence and dedup rules in
[`layered-fs.md`](../specs/layered-fs.md). What it must not contain is the
*code*: not the concrete structure of a function, not its naming, not a snippet
standing in for the real thing. Describe the algorithm so it can be judged and
re-derived, not so it can be pasted.

The `Non-goals` section does more work than it looks like it does. Most of this
project's design pressure is toward doing more — merging entry contents, caching
for speed, writing through to sources — and a non-goal is how a spec records that
the option was considered and declined, so the next contributor does not relitigate
it silently.

## Indexing it

Update [`docs/specs/index.md`](../specs/index.md) in the same change: a row with a
one-line description and a status.

| Status | Means |
| --- | --- |
| `draft` | proposed, not yet agreed |
| `accepted` | agreed, not yet built |
| `implemented` | matches the code |
| `superseded` | kept for history; the row points at its replacement |

`make lint` fails on a spec that is not indexed and on an index row pointing at a
document that does not exist. See [quality-gates.md](quality-gates.md).

## Keeping it true

A spec marked `implemented` that no longer matches the code is worse than no spec,
because it is trusted. When implementation forces a change, the spec changes in the
same commit as the code. When a spec is replaced, mark the old one `superseded` and
point at the replacement rather than deleting it — the history of why a design was
abandoned is the part nobody can reconstruct later.
