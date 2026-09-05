#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
GO_BIN="${GO:-go}"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

cp -R "$ROOT/third_party/cobra/." "$tmp/"
(
	cd "$tmp"
	patch -R -p1 < traverse-flag-error.patch
	shasum -a 256 -c UPSTREAM.sha256 >/dev/null
)
(
	cd "$ROOT/third_party/cobra"
	"$GO_BIN" test -count=1 ./...
)
printf '%s\n' 'Cobra upstream integrity and dependency tests: ok'
