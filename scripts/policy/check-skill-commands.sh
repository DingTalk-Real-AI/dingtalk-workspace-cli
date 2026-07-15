#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
CHECKER="$(mktemp)"
CHECK_HOME="$(mktemp -d)"
trap 'rm -rf "$CHECKER" "$CHECK_HOME"' EXIT

cd "$ROOT"
go build -o "$CHECKER" ./scripts/policy/skill-command-check
HOME="$CHECK_HOME" DWS_LANG=zh "$CHECKER"
