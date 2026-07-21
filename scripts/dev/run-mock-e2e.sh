#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"

cd "$ROOT"
DWS_PACKAGE_VERSION="${DWS_PACKAGE_VERSION:-0.0.0-test}" \
  go test -count=1 -timeout=5m \
    ./test/mock_mcp/... \
    ./test/integration/...
