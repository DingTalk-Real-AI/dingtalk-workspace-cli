#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"

run() {
  printf '+ %s\n' "$*"
  "$@"
}

cd "$ROOT"

run ./scripts/dev/lint.sh
run make test
COVERAGE_PROFILE="$(mktemp "${TMPDIR:-/tmp}/dws-coverage.XXXXXX")"
run ./scripts/dev/coverage.sh "$COVERAGE_PROFILE"
rm -f "$COVERAGE_PROFILE"
run git diff --check
