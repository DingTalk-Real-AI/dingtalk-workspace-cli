#!/bin/sh
set -eu

# Check command surface for drift.
# Stub placeholder — extend with actual surface comparison logic as needed.

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
BIN="${DWS_BIN:-$ROOT/dws}"

STRICT=0
if [ "${1:-}" = "--strict" ]; then
  STRICT=1
fi

cd "$ROOT"

if [ ! -x "$BIN" ]; then
  printf 'error: dws binary not found at %s (run make build first)\n' "$BIN" >&2
  exit 2
fi

# These utility commands are stable across the current open-source CLI shape.
EXPECTED_COMMANDS="auth cache completion version"
missing=0
for cmd in $EXPECTED_COMMANDS; do
  # Use `help <command>` so hidden commands are also validated.
  if ! "$BIN" help "$cmd" >/dev/null 2>&1; then
    printf 'missing command: %s\n' "$cmd" >&2
    missing=$((missing + 1))
  fi
done

# Full Schema canonical/primary/alias delivery is exercised once by
# check-schema-binary.sh. Keep this check focused on the basic command tree.
if [ "$STRICT" -eq 1 ] && [ "$missing" -gt 0 ]; then
  printf 'command surface check: %d missing commands\n' "$missing" >&2
  exit 1
fi

printf 'command surface check: ok\n'
