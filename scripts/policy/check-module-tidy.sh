#!/bin/sh
set -eu

# go.mod/go.sum must be `go mod tidy`-clean. Release automation reruns tidy and
# fails on any diff, so drift here would break the release validation
# deterministically; policy catches it before merge instead.

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
cd "$ROOT"

tidy_diff="$(go mod tidy -diff 2>&1)" || {
	printf '%s\n' 'module tidy check: go.mod/go.sum are not tidy' >&2
	printf '%s\n' "$tidy_diff" >&2
	printf '%s\n' 'run: go mod tidy' >&2
	exit 1
}

printf '%s\n' 'module tidy check: ok'
