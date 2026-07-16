#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
cd "$ROOT"
. "$ROOT/scripts/policy/policy-runtime.sh"
policy_prepare_runtime "$ROOT"

TMP_ROOT="$(policy_runtime_mktemp_dir dws-skill-command-check)"
CHECKER="$TMP_ROOT/skill-command-check"
CHECK_HOME="$TMP_ROOT/home"
mkdir -p "$CHECK_HOME"
trap 'rm -rf "$TMP_ROOT"' EXIT HUP INT TERM

go build -o "$CHECKER" ./scripts/policy/skill-command-check
HOME="$CHECK_HOME" DWS_LANG=zh "$CHECKER"
