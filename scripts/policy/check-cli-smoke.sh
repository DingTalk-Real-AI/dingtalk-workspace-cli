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
SNAPSHOT_BIN="$(mktemp)"
CURRENT="$(mktemp)"
trap 'rm -rf "$CURRENT" "$SNAPSHOT_HOME" "$SNAPSHOT_BIN"' EXIT

cd "$ROOT"
go build -o "$SNAPSHOT_BIN" ./scripts/policy/interface-baseline
HOME="$SNAPSHOT_HOME" DWS_DISABLE_KEYCHAIN=1 DWS_LANG=zh "$SNAPSHOT_BIN" >"$CURRENT"

root_commands() {
  awk '
    /^\[root\]$/ {
      in_root = 1
      next
    }
    /^\[/ {
      if (in_root) {
        exit
      }
      next
    }
    in_root {
      line = $0
      sub(/^[[:space:]]*/, "", line)
      if (line ~ /^commands:[[:space:]]*/) {
        sub(/^commands:[[:space:]]*/, "", line)
        gsub(/,[[:space:]]*/, " ", line)
        print line
        found = 1
        exit
      }
    }
    END {
      if (!found) {
        exit 1
      }
    }
  ' "$1"
}

baseline_commands="$(root_commands "$BASELINE")" || {
  printf 'error: no root commands found in interface baseline\n' >&2
  exit 1
}
current_commands="$(root_commands "$CURRENT")" || {
  printf 'error: no root commands found in current interface contract\n' >&2
  exit 1
}
if [ "$baseline_commands" != "$current_commands" ]; then
  printf 'error: interface baseline root commands are stale (run make update-interface-baseline)\n' >&2
  printf '  baseline: %s\n  current:  %s\n' "$baseline_commands" "$current_commands" >&2
  exit 1
fi

# DWS_DISABLE_KEYCHAIN=1 routes the DEK to a file (the Linux scheme) instead of
# the macOS system Keychain. Without it, running the binary under this fresh
# HOME triggers a GUI Keychain authorization prompt that blocks the
# non-interactive smoke test indefinitely. Linux CI already uses the file DEK.
capture_help() {
  HOME="$SNAPSHOT_HOME" DWS_DISABLE_KEYCHAIN=1 DWS_LANG=zh "$BIN" "$@" --help 2>&1
}

root_help="$(capture_help)" || {
  printf 'error: dws --help exited non-zero\n%s\n' "$root_help" >&2
  exit 1
}

run_command_help() {
  output="$(HOME="$SNAPSHOT_HOME" DWS_DISABLE_KEYCHAIN=1 DWS_LANG=zh "$BIN" "$@" --help 2>&1)" || {
    printf 'error: dws %s --help exited non-zero\n%s\n' "$*" "$output" >&2
    return 1
  }
  if [ "$output" = "$root_help" ]; then
    printf 'error: dws %s --help resolved to root help instead of command-specific help\n' "$*" >&2
    return 1
  fi
  usage_line="$(printf '%s\n' "$output" | awk '/^Usage:$/ { getline; print; exit }')"
  expected_usage="  dws $*"
  case "$usage_line" in
    "$expected_usage"|"$expected_usage "*) ;;
    *)
      printf 'error: dws %s --help reported unexpected usage identity: %s\n' "$*" "$usage_line" >&2
      return 1
      ;;
  esac
}

# Cobra currently treats an unknown command followed by --help as root help
# with exit status 0. Keep a negative case here so exit status alone can never
# make the smoke check pass for a command that did not resolve.
unknown_command="__dws_cli_smoke_unknown_command__"
if run_command_help "$unknown_command" >/dev/null 2>&1; then
  printf 'error: unknown command %s unexpectedly produced command-specific help\n' "$unknown_command" >&2
  exit 1
fi

count=0
for command in $current_commands; do
  run_command_help "$command"
  count=$((count + 1))
done

printf 'CLI smoke check: ok (root + %s public top-level commands)\n' "$count"
