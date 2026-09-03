#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
OUTPUT="$ROOT/internal/runtimepayload/assets"
CHECK=0

if [ "${1:-}" = "--check" ]; then
  CHECK=1
  shift
fi
[ "$#" -eq 0 ] || { printf 'usage: %s [--check]\n' "$0" >&2; exit 2; }

work="$(mktemp -d "${TMPDIR:-/tmp}/dws-runtime-assets.XXXXXX")"
trap 'rm -rf "$work"' EXIT HUP INT TERM
generated="$work/generated"
mkdir -p "$generated"

for target in darwin-amd64 darwin-arm64 linux-amd64 linux-arm64 windows-amd64 windows-arm64; do
  target_os="${target%-*}"
  target_arch="${target##*-}"
  case "$target" in
    darwin-*|linux-*) capacity=6291456 ;;
    windows-amd64) capacity=12582912 ;;
    windows-arm64) capacity=8388608 ;;
  esac
  target_root="$work/$target"
  "$ROOT/scripts/build/prepare-runtime-payload.sh" "$target_os" "$target_arch" "$target_root" >/dev/null
  (cd "$ROOT" && go run ./scripts/build/runtime-payload generate \
    "$generated/$target.payload" "$target_root/.dws-runtime/20260825" "$capacity")
done

if [ "$CHECK" -eq 1 ]; then
  for generated_file in "$generated"/*.payload; do
    name="$(basename "$generated_file")"
    cmp "$generated_file" "$OUTPUT/$name" >/dev/null || {
      printf 'runtime payload asset is stale: %s\n' "$name" >&2
      exit 1
    }
  done
  printf 'Embedded runtime payload assets are current.\n'
  exit 0
fi

mkdir -p "$OUTPUT"
for generated_file in "$generated"/*.payload; do
  cp "$generated_file" "$OUTPUT/$(basename "$generated_file")"
done
printf 'Generated embedded runtime payload assets.\n'
