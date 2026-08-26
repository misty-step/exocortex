#!/bin/sh
set -eu
# Copy the repository skill (source) to a destination directory as SKILL.md.
# omp-config and live harness trees are generated this way; do not hand-edit them.

DEST=${1:?destination directory}
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_DIR=$(dirname "$SCRIPT_DIR")
SRC="$REPO_DIR/skills/exocortex/SKILL.md"

[ -f "$SRC" ] || {
	printf 'missing skill source: %s\n' "$SRC" >&2
	exit 1
}

mkdir -p "$DEST"
cp "$SRC" "$DEST/SKILL.md"
printf 'installed %s -> %s/SKILL.md\n' "$SRC" "$DEST"
