#!/bin/sh
# Point this clone at committed hooks. Does not mutate other repositories.
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repo_root"

git config core.hooksPath .githooks
echo "core.hooksPath=.githooks"
echo "pre-commit: gofmt, cyclop, gitleaks"
echo "pre-push: ./scripts/check.sh"
