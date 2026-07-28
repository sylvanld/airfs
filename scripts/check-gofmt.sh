#!/usr/bin/env bash
# Verify that every Go file is in its canonical formatting.
#
# This gate verifies rather than rewrites, so that `make check` never modifies
# the working tree; `make format` is what fixes what it reports.
set -euo pipefail

if ! command -v gofmt >/dev/null; then
	echo "gofmt: not found; install Go" >&2
	exit 1
fi

unformatted=$(gofmt -l .)
if [[ -n $unformatted ]]; then
	echo "not gofmt-formatted; run 'make format':" >&2
	echo "$unformatted" | sed 's/^/  /' >&2
	exit 1
fi
