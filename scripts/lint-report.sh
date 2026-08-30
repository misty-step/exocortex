#!/bin/sh
# Non-blocking visibility report. Fuse lives in .golangci.yml via check.sh.
# Prints band counts plus every remaining report-only issue. Exit 0 even when loud.
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repo_root"

if ! command -v golangci-lint >/dev/null 2>&1; then
	echo "missing golangci-lint: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.1" >&2
	exit 2
fi
if ! command -v python3 >/dev/null 2>&1; then
	echo "missing python3: needed to rank the lint report" >&2
	exit 2
fi

json="$repo_root/.lint-report.json"
text="$repo_root/.lint-report.txt"
rm -f "$json" "$text"

# Band visibility uses .golangci-report.yml so fuse thresholds do not hide 1–15.
golangci-lint run --config "$repo_root/.golangci-report.yml" --issues-exit-code 0 \
	--max-issues-per-linter 0 --max-same-issues 0 --uniq-by-line=false \
	--output.json.path "$json" --output.text.path "$text" ./... >/dev/null


python3 - "$json" <<'PY'
import json, re, sys
from collections import Counter

data = json.load(open(sys.argv[1]))
issues = data.get("Issues") or []

def is_test(path):
    return path.endswith("_test.go")

def cog(text):
    m = re.search(r"cognitive complexity (\d+)", text or "")
    return int(m.group(1)) if m else None

def cyclo(text):
    m = re.search(r"cyclomatic complexity (\d+)", text or "")
    return int(m.group(1)) if m else None

def nest(text):
    m = re.search(r"complexity: (\d+)", text or "")
    return int(m.group(1)) if m else None

print("== lint report (non-blocking) ==")
print("issues", len(issues))
by = Counter(i.get("FromLinter") for i in issues)
for name, n in sorted(by.items(), key=lambda kv: (-kv[1], kv[0])):
    print(f"  {name:12} {n}")

prod = [i for i in issues if not is_test(i.get("Pos", {}).get("Filename", ""))]
test = [i for i in issues if is_test(i.get("Pos", {}).get("Filename", ""))]
print(f"production {len(prod)}  tests {len(test)}")

cogs = [(cog(i.get("Text")), i) for i in prod if i.get("FromLinter") == "gocognit" and cog(i.get("Text")) is not None]
test_cogs = [(cog(i.get("Text")), i) for i in test if i.get("FromLinter") == "gocognit" and cog(i.get("Text")) is not None]
bands = [("1-7", 1, 7), ("8-14", 8, 14), ("15-19", 15, 19), ("20+", 20, 10**9)]
print("gocognit production bands:")
for label, lo, hi in bands:
    n = sum(1 for v, _ in cogs if lo <= v <= hi)
    print(f"  {label:6} {n}")
print("gocognit test bands:")
for label, lo, hi in bands:
    n = sum(1 for v, _ in test_cogs if lo <= v <= hi)
    print(f"  {label:6} {n}")

def print_hot(title, rows):
    print(f"\n-- {title} --")
    hot = []
    for i in rows:
        path = i.get("Pos", {}).get("Filename", "")
        line = i.get("Pos", {}).get("Line", 0)
        lint = i.get("FromLinter", "")
        text = i.get("Text", "")
        score = cog(text) or cyclo(text) or nest(text) or 0
        hot.append((score, lint, path, line, text))
    hot.sort(key=lambda r: (-r[0], r[1], r[2], r[3]))
    for score, lint, path, line, text in hot:
        if lint == "gocognit" and score < 8:
            continue
        if lint == "nestif" and score < 2:
            continue
        print(f"{score:3} {lint:12} {path}:{line} {text}")

print_hot("production hot list", prod)
print_hot("test hot list", test)

print("\n-- correctness (all files) --")
for i in issues:
    if i.get("FromLinter") not in ("staticcheck", "errcheck"):
        continue
    pos = i.get("Pos", {})
    print(f"{i['FromLinter']:12} {pos.get('Filename')}:{pos.get('Line')} {i.get('Text')}")

print("\n-- suggested ratchet tickets --")
print("Fuse: cyclop<=15, staticcheck, production errcheck, gocognit>=16, nestif>=5.")
print("Report-only: funlen, maintidx, dupl. Keep cyclop at 15 until dispatch/Register are not why you would cut.")
PY

rm -f "$json" "$text"
echo "==> lint report done (non-blocking)"
