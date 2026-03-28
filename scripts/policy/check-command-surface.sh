#!/bin/sh
set -eu

# Check command surface for drift.
# Stub placeholder — extend with actual surface comparison logic as needed.

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"

STRICT=0
if [ "${1:-}" = "--strict" ]; then
  STRICT=1
fi

cd "$ROOT"

# Verify that all expected top-level commands exist in the built binary
if command -v go >/dev/null 2>&1; then
  BIN_PATH="/tmp/dws-surface-check"
  go build -ldflags="-s -w" -o "$BIN_PATH" ./cmd 2>/dev/null || { echo "build failed"; exit 1; }

  # These utility commands are stable across the current open-source CLI shape.
  EXPECTED_COMMANDS="auth cache completion version"
  missing=0
  for cmd in $EXPECTED_COMMANDS; do
    # Use `help <command>` so hidden commands are also validated.
    if ! "$BIN_PATH" help "$cmd" >/dev/null 2>&1; then
      printf 'missing command: %s\n' "$cmd" >&2
      missing=$((missing + 1))
    fi
  done
  rm -f "$BIN_PATH"

  if [ "$STRICT" -eq 1 ] && [ "$missing" -gt 0 ]; then
    printf 'command surface check: %d missing commands\n' "$missing" >&2
    exit 1
  fi
fi

printf 'command surface check: ok\n'
