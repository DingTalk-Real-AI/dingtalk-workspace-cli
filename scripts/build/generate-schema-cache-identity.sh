#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)
cd "$root"
# Identity and the release core must use the same exact compiler/runtime.
# An installed newer Go must not silently replace the release toolchain.
GOTOOLCHAIN=go1.25.9
export GOTOOLCHAIN
[ "$(go env GOVERSION)" = "$GOTOOLCHAIN" ] || {
  echo "error: Schema identity requires $GOTOOLCHAIN" >&2
  exit 1
}
exec go run "$root/internal/generator/cmd_schema_cache_identity" -root "$root" "$@"
