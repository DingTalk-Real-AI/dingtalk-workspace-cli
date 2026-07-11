#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
BIN="${DWS_BIN:-$ROOT/dws}"
BASELINE="$ROOT/test/fixtures/schema-list-baseline.json"
CURRENT="$(mktemp)"
OUTPUT="$(mktemp)"
CHECKER="$(mktemp)"
CHECK_HOME="$(mktemp -d)"
trap 'rm -rf "$CURRENT" "$OUTPUT" "$CHECKER" "$CHECK_HOME"' EXIT

if [ ! -x "$BIN" ]; then
  printf 'error: dws binary not found at %s (run make build first)\n' "$BIN" >&2
  exit 1
fi

cd "$ROOT"
go build -o "$CHECKER" ./scripts/policy/schema-compat
HOME="$CHECK_HOME" DWS_LANG=zh "$BIN" schema list --format json >"$CURRENT"

if [ "${1:-}" = "--update" ]; then
  mkdir -p "$(dirname "$BASELINE")"
  if [ -f "$BASELINE" ]; then
    "$CHECKER" --merge "$BASELINE" --current "$CURRENT" >"$OUTPUT"
  else
    "$CHECKER" --normalize "$CURRENT" >"$OUTPUT"
  fi
  cp "$OUTPUT" "$BASELINE"
  printf 'schema list baseline extended: %s\n' "${BASELINE#"$ROOT"/}"
  exit 0
fi

if [ ! -f "$BASELINE" ]; then
  printf 'error: schema baseline missing at %s — run make update-schema-baseline\n' \
    "${BASELINE#"$ROOT"/}" >&2
  exit 1
fi

"$CHECKER" --check "$BASELINE" --current "$CURRENT"
