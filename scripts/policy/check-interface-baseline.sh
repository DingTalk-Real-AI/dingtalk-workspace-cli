#!/bin/sh
set -eu

# Compare the public Cobra command tree with the reviewed interface baseline.
# The Go snapshot helper builds the tree once in-process, so this remains fast
# even when the CLI has hundreds of command nodes.

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
BASELINE="${INTERFACE_BASELINE:-$ROOT/test/fixtures/cli-interface-baseline.txt}"
cd "$ROOT"
. "$ROOT/scripts/policy/policy-runtime.sh"
policy_prepare_runtime "$ROOT"

TMP_ROOT="$(policy_runtime_mktemp_dir dws-interface-baseline)"
SNAPSHOT_HOME="$TMP_ROOT/home"
SNAPSHOT_BIN="$TMP_ROOT/interface-baseline"
CURRENT="$TMP_ROOT/current.txt"
mkdir -p "$SNAPSHOT_HOME"
trap 'rm -rf "$TMP_ROOT"' EXIT HUP INT TERM

# Compile with the caller's normal Go cache, then isolate only the execution
# HOME so user-installed DWS plugins cannot alter the public command tree.
go build -o "$SNAPSHOT_BIN" ./scripts/policy/interface-baseline
if [ "${1:-}" = "--reset" ]; then
  mkdir -p "$(dirname "$BASELINE")"
  HOME="$SNAPSHOT_HOME" DWS_LANG=zh "$SNAPSHOT_BIN" >"$CURRENT"
  cp "$CURRENT" "$BASELINE"
  printf 'WARNING: interface compatibility history replaced: %s (%s command nodes)\n' \
    "${BASELINE#"$ROOT"/}" "$(grep -c '^\[' "$BASELINE")"
  exit 0
fi

if [ "${1:-}" = "--update" ]; then
  mkdir -p "$(dirname "$BASELINE")"

  if [ -f "$BASELINE" ]; then
    HOME="$SNAPSHOT_HOME" DWS_LANG=zh "$SNAPSHOT_BIN" --merge "$BASELINE" >"$CURRENT"
  else
    HOME="$SNAPSHOT_HOME" DWS_LANG=zh "$SNAPSHOT_BIN" >"$CURRENT"
  fi
  cp "$CURRENT" "$BASELINE"
  printf 'interface baseline extended: %s (%s historical command nodes)\n' \
    "${BASELINE#"$ROOT"/}" "$(grep -c '^\[' "$BASELINE")"
  exit 0
fi

if [ "$#" -gt 0 ]; then
  printf 'error: unknown option: %s\n' "$1" >&2
  exit 2
fi

if [ ! -f "$BASELINE" ]; then
  printf 'error: baseline missing at %s — run make update-interface-baseline\n' \
    "${BASELINE#"$ROOT"/}" >&2
  exit 1
fi

HOME="$SNAPSHOT_HOME" DWS_LANG=zh "$SNAPSHOT_BIN" --check "$BASELINE"
