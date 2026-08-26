#!/bin/sh
# Owned green gate. CI and pre-push run this unchanged.
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repo_root"

need() {
	if ! command -v "$1" >/dev/null 2>&1; then
		printf '%s\n' "missing $1: $2" >&2
		exit 2
	fi
}

need go "install Go from go.mod (mise or https://go.dev/dl)"

echo "==> gofmt"
bad=$(gofmt -l cmd internal)
if [ -n "$bad" ]; then
	printf '%s\n' "gofmt: files need formatting:" $bad >&2
	echo "repair: gofmt -w cmd internal" >&2
	exit 1
fi

echo "==> go vet"
go vet ./...

need golangci-lint "go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.1"
echo "==> golangci-lint (cyclop)"
golangci-lint run ./...

need gitleaks "install gitleaks v8.30.1 from https://github.com/gitleaks/gitleaks/releases"
echo "==> gitleaks"
gitleaks detect --source "$repo_root" --no-banner --redact --exit-code 1
gitleaks detect --no-git --source "$repo_root" --no-banner --redact --exit-code 1
echo "==> govulncheck"
govulncheck ./...

echo "==> module token budgets"
"$repo_root/scripts/check-budget.sh"

echo "==> go test -race"
go test -race -count=1 ./...

echo "==> CLI smoke"
"$repo_root/scripts/smoke.sh"

echo "==> gate passed"
