#!/bin/sh
set -eu

# Compare the public Cobra command tree with the reviewed interface baseline.
# The Go snapshot helper builds the tree once in-process, so this remains fast
# even when the CLI has hundreds of command nodes.

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
BASELINE="$ROOT/test/fixtures/cli-interface-baseline.txt"
CURRENT="$(mktemp)"
SNAPSHOT_HOME="$(mktemp -d)"
SNAPSHOT_BIN="$(mktemp)"
trap 'rm -rf "$CURRENT" "$SNAPSHOT_HOME" "$SNAPSHOT_BIN"' EXIT

cd "$ROOT"
# Compile with the caller's normal Go cache, then isolate only the execution
# HOME so user-installed DWS plugins cannot alter the public command tree.
go build -o "$SNAPSHOT_BIN" ./scripts/policy/interface-baseline
HOME="$SNAPSHOT_HOME" DWS_LANG=zh "$SNAPSHOT_BIN" >"$CURRENT"

if [ "${1:-}" = "--update" ]; then
  mkdir -p "$(dirname "$BASELINE")"
  cp "$CURRENT" "$BASELINE"
  printf 'interface baseline updated: %s (%s command nodes)\n' \
    "${BASELINE#"$ROOT"/}" "$(grep -c '^\[' "$BASELINE")"
  exit 0
fi

if [ ! -f "$BASELINE" ]; then
  printf 'error: baseline missing at %s — run make update-interface-baseline\n' \
    "${BASELINE#"$ROOT"/}" >&2
  exit 1
fi

if ! diff -u "$BASELINE" "$CURRENT"; then
  printf '\nerror: CLI interface changed — commands, aliases, or flags differ from baseline.\n' >&2
  printf 'If intentional, run make update-interface-baseline and commit the reviewed diff.\n' >&2
  exit 1
fi

printf 'interface integrity check: ok (%s command nodes)\n' \
  "$(grep -c '^\[' "$BASELINE")"
