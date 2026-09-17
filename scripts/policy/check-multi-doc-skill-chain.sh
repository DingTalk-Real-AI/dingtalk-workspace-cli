#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
cd "$ROOT"
. "$ROOT/scripts/policy/policy-runtime.sh"
policy_prepare_runtime "$ROOT"

TMP_ROOT="$(policy_runtime_mktemp_dir dws-multi-doc-skill-chain)"
CHECKER="$TMP_ROOT/multi-doc-skill-chain"
CHECK_HOME="$TMP_ROOT/home"
mkdir -p "$CHECK_HOME"
trap 'rm -rf "$TMP_ROOT"' EXIT HUP INT TERM

go build -o "$CHECKER" ./scripts/policy/multi-doc-skill-chain
HOME="$CHECK_HOME" DWS_LANG=zh "$CHECKER"
