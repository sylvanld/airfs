# Quality gates

`make check` runs every gate that exists in this repository. It is what to run
before pushing, and it is what CI runs — if CI ever gates something `make check`
does not, the `Makefile` is wrong and should be fixed rather than worked around.

`check` never modifies the working tree, and no gate is allowed to pass by
skipping itself. A gate whose tool is missing fails loudly, so that a missing tool
is never mistaken for a clean run.

## The gates

| Target | Verifies | Why it exists |
| --- | --- | --- |
| `lint` | Every document in `docs/specs/`, `docs/user-guide/`, and `docs/contribute/` is listed in that directory's `index.md`, and every document an index links to exists | The indexes are how anyone finds a spec. An unlisted spec is invisible, so the next contributor writes a second spec for the same topic; a link to a deleted document sends them looking for something that is gone. |
| `lint` | Every Go file is in `gofmt`'s canonical formatting | Formatting that is not canonical turns into diff noise that hides the change a reviewer is looking for. It verifies rather than rewrites, so `check` never modifies the working tree; `make format` fixes what it reports. |
| `lint` | `go vet` finds nothing | It catches the mistakes the compiler accepts — a `Printf` whose arguments do not match, a lock copied by value — which are exactly the ones that survive to runtime. |
| `test` | The Go test suite passes | The merge semantics are the product, and they are tested against in-memory trees so that every edge case runs without a mount. The mount's own tests establish real FUSE mounts and skip on a host that cannot. |

Each gate stays separately invocable, so `make lint` runs just that one.

## When `lint` fails

- **`not listed in docs/specs/index.md`** — add a row to the index with a one-line
  description and a status, in the same change as the spec. See
  [working-on-specs.md](working-on-specs.md).
- **`links X, which does not exist`** — a document was renamed or removed without
  updating the index. Fix the row, or remove it and record the replacement if the
  spec was superseded.
- **`docs/specs/index.md: missing`** — the index was deleted. It is not optional.

## When `lint` fails on Go

- **`not gofmt-formatted`** — run `make format`, which rewrites exactly the files
  it named.
- **a `go vet` diagnostic** — fix the code. Vet reports no false positives worth
  silencing here; a suppression would need to explain itself in the spec first.

## When `test` fails

- **a mount test that skipped** — the host lacks `/dev/fuse` or a setuid
  `fusermount3`. Run `bin/airfs doctor` (after `make build`) for which one and
  what provides it. The suite passes, but it has not exercised the mount.

## Adding a gate

A new gate is a new target plus a row in the table above. Two constraints that
are easy to get wrong:

- Name the target by goal, not by tool or language: `lint`, never `vet` or
  `golangci-lint`; `test`, never `test-go`. If several tools lint, `lint` runs all
  of them.
- A gate that would rewrite files runs in verify mode under `check`, and names the
  target that fixes it. `format` mutates; verifying formatting belongs in `lint`.
