#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"

cd "$ROOT"
exec go run ./scripts/policy/coding-agent-harness -root "$ROOT" "$@"
