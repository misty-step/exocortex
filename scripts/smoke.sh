#!/bin/sh
# Public-interface smoke: register → put → get → lint on a temp cortex.
# Does not touch the operator XDG config or daybook.
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repo_root"

need() {
	if ! command -v "$1" >/dev/null 2>&1; then
		printf '%s\n' "missing $1: $2" >&2
		exit 2
	fi
}

need go "install Go from go.mod"
need python3 "install python3 to assert JSON payloads"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
export XDG_CONFIG_HOME="$tmp/xdg"
mkdir -p "$XDG_CONFIG_HOME"

bin="$tmp/exocortex"
go build -trimpath -o "$bin" ./cmd/exocortex

cortex="$tmp/cortex"
mkdir -p "$cortex"

"$bin" register smoke "$cortex" --vcs none --json >/dev/null

cat >"$tmp/note.md" <<'EOF'
---
type: note
status: active
created: 2026-08-26T00:00:00Z
description: foundation smoke
tags: [foundation]
---
foundation smoke
EOF

"$bin" put notes/hello.md --from "$tmp/note.md" --cortex smoke --json >"$tmp/put.json"
python3 -c '
import json, sys
d = json.load(open(sys.argv[1]))
assert d.get("operation") == "create", d
assert d.get("path") == "notes/hello.md", d
assert d.get("revision"), d
assert not d.get("error"), d
' "$tmp/put.json"

"$bin" get notes/hello.md --cortex smoke --json >"$tmp/get.json"
python3 -c '
import json, sys
d = json.load(open(sys.argv[1]))
assert d.get("path") == "notes/hello.md", d
assert "foundation smoke" in d.get("content", ""), d
assert d.get("revision"), d
assert d.get("frontmatter", {}).get("type") == "note", d
' "$tmp/get.json"

"$bin" lint notes/hello.md --cortex smoke --json >"$tmp/lint.json"
python3 -c '
import json, sys
d = json.load(open(sys.argv[1]))
assert "error" not in d or d.get("error") in (None, ""), d
' "$tmp/lint.json"

printf '%s\n' "smoke passed: register/put/get/lint on temp none-vcs cortex"
