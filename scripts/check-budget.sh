#!/bin/sh
# Fail if a declared module's packed token count exceeds its budget.
# Current repomix has no --token-budget flag; this compares Total Tokens
# from the same packed output CONTROLS.md names.
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repo_root"

if command -v bunx >/dev/null 2>&1; then
	repack() { bunx --bun repomix@1.10.0 "$@"; }
elif command -v npx >/dev/null 2>&1; then
	repack() { npx --yes repomix@1.10.0 "$@"; }
else
	echo "missing repomix runner: install bun or npm (npx)" >&2
	exit 2
fi

budget_file="$repo_root/modules.budget"
if [ ! -f "$budget_file" ]; then
	echo "missing $budget_file" >&2
	exit 2
fi

failed=0
while IFS="$(printf '\t')" read -r name include budget rest || [ -n "${name:-}" ]; do
	case "$name" in
	'' | \#*) continue ;;
	esac
	if [ -z "$include" ] || [ -z "$budget" ]; then
		echo "malformed budget line: $name" >&2
		exit 2
	fi
	tmp=$(mktemp)
	log=$(mktemp)
	if ! repack --include "$include/**" --no-file-summary --no-directory-structure -o "$tmp" >"$log" 2>&1; then
		echo "repomix failed for $name ($include)" >&2
		cat "$log" >&2
		rm -f "$tmp" "$log"
		exit 1
	fi
	tokens=$(sed -n 's/^ *Total Tokens: *\([0-9,]*\).*/\1/p' "$log" | tr -d ',' | tail -n 1)
	rm -f "$tmp" "$log"
	if [ -z "$tokens" ]; then
		echo "repomix produced no Total Tokens line for $name" >&2
		exit 1
	fi
	printf '  %s %s tokens (budget %s)\n' "$name" "$tokens" "$budget"
	if [ "$tokens" -gt "$budget" ]; then
		echo "token budget exceeded: $name $tokens > $budget. Delete dead weight or split the module; raise only with a recorded reason." >&2
		failed=1
	fi
done <"$budget_file"

exit "$failed"
