#!/usr/bin/env bash
set -euo pipefail

ROOT="$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)"
VERSION="${VERSION:-0.0.0-SNAPSHOT}"
SEMVER="${VERSION#v}"

cd "$ROOT"
DWS_PACKAGE_VERSION="$SEMVER" \
GORELEASER_CURRENT_TAG="v$SEMVER" \
  ./scripts/release/run-goreleaser-cross.sh \
    release --snapshot --clean --skip=publish --parallelism=2
