#!/bin/sh
set -eu

# Verify that the shipped binary can render help for every public top-level
# command without login state or network access. Interface Integrity checks
# the complete tree; this smoke test checks the actual compiled artifact.

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
BIN="${DWS_BIN:-$ROOT/dws}"
BASELINE="$ROOT/test/fixtures/cli-interface-baseline.txt"

if [ ! -x "$BIN" ]; then
  printf 'error: dws binary not found at %s (run make build first)\n' "$BIN" >&2
  exit 1
fi
if [ ! -f "$BASELINE" ]; then
  printf 'error: interface baseline missing at %s\n' "$BASELINE" >&2
  exit 1
fi

SNAPSHOT_HOME="$(mktemp -d)"
trap 'rm -rf "$SNAPSHOT_HOME"' EXIT

# DWS_DISABLE_KEYCHAIN=1 routes the DEK to a file (the Linux scheme) instead of
# the macOS system Keychain. Without it, running the binary under this fresh
# HOME triggers a GUI Keychain authorization prompt that blocks the
# non-interactive smoke test indefinitely. Linux CI already uses the file DEK.
run_help() {
  output="$(HOME="$SNAPSHOT_HOME" DWS_DISABLE_KEYCHAIN=1 DWS_LANG=zh "$BIN" "$@" --help 2>&1)" || {
    printf 'error: dws %s --help exited non-zero\n%s\n' "$*" "$output" >&2
    return 1
  }
}

run_help

command_line="$(sed -n '/^\[root\]$/{n;s/^  commands: //;p;q;}' "$BASELINE")"
if [ -z "$command_line" ]; then
  printf 'error: no root commands found in interface baseline\n' >&2
  exit 1
fi

count=0
for command in $(printf '%s\n' "$command_line" | tr -d ','); do
  run_help "$command"
  count=$((count + 1))
done

printf 'CLI smoke check: ok (root + %s public top-level commands)\n' "$count"
