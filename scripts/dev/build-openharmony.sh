#!/usr/bin/env bash
set -euo pipefail

ROOT="$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)"
OHOS_GO="${OHOS_GO:-}"
OUTPUT="${OHOS_OUTPUT:-$ROOT/dist/dws-openharmony-arm64}"

if [ -z "$OHOS_GO" ] || [ ! -x "$OHOS_GO" ]; then
  printf 'OHOS_GO must point to an executable Go 1.26.7 OpenHarmony toolchain\n' >&2
  exit 2
fi

if ! "$OHOS_GO" tool dist list | grep -Fxq 'openharmony/arm64'; then
  printf '%s does not support openharmony/arm64\n' "$OHOS_GO" >&2
  exit 1
fi

mkdir -p "$(dirname -- "$OUTPUT")"
cd "$ROOT"
GOOS=openharmony GOARCH=arm64 CGO_ENABLED=0 GOTOOLCHAIN=local \
  "$OHOS_GO" build -trimpath -ldflags='-s -w' -o "$OUTPUT" ./cmd

printf 'Built %s\n' "$OUTPUT"
