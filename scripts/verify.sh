#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

boundary_pattern='ph[a]edrus|/U[s]ers/|d[a]ybook|m[o]nologue|s[u]per whisper|finance[_]private|relationship[_]private|q[m]d'
backlog_oracle='backlog.d/001-public-boundary-and-frozen-shape-closure.md'

echo "==> public boundary grep"
if ! grep -F "$boundary_pattern" "$backlog_oracle" >/dev/null; then
  echo "public boundary grep pattern drifted from $backlog_oracle" >&2
  exit 1
fi

if rg -n -i "$boundary_pattern" . --glob '!.git/**'; then
  echo "public boundary grep found private or public-ineligible terms" >&2
  exit 1
else
  status=$?
  if [ "$status" -ne 1 ]; then
    echo "public boundary grep failed with exit code $status" >&2
    exit "$status"
  fi
fi

echo "==> first-packet fixture regeneration"
python3 scripts/build-first-packet.py

echo "==> first-packet fixture diff"
git diff --exit-code -- tests/fixtures/first-packet/evidence-packet.md

echo "verify: ok"
