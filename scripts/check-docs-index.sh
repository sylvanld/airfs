#!/usr/bin/env bash
# Enforce the indexing rule from AGENTS.md: every document in docs/specs/,
# docs/user-guide/, and docs/contribute/ is listed in that directory's index.md,
# and every document an index lists exists.
set -uo pipefail

status=0

for dir in docs/specs docs/user-guide docs/contribute; do
	index="$dir/index.md"

	if [ ! -f "$index" ]; then
		echo "$index: missing" >&2
		status=1
		continue
	fi

	# Documents present but not linked from the index.
	for doc in "$dir"/*.md; do
		[ -e "$doc" ] || continue
		name="$(basename "$doc")"
		[ "$name" = "index.md" ] && continue
		if ! grep -qF "($name)" "$index"; then
			echo "$doc: not listed in $index" >&2
			status=1
		fi
	done

	# Documents linked from the index but absent.
	while IFS= read -r name; do
		[ -n "$name" ] || continue
		if [ ! -f "$dir/$name" ]; then
			echo "$index: links $name, which does not exist" >&2
			status=1
		fi
	done < <(grep -o '(\([A-Za-z0-9._-]*\.md\))' "$index" | tr -d '()' | sort -u)
done

if [ "$status" -eq 0 ]; then
	echo "docs indexes consistent"
fi

exit "$status"
