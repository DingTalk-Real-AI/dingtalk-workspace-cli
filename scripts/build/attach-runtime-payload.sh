#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
GOOS="${1:-}"
GOARCH="${2:-}"
BINARY="${3:-}"

[ -n "$GOOS" ] && [ -n "$GOARCH" ] && [ -n "$BINARY" ] || {
  printf 'usage: %s <goos> <goarch> <dws-binary>\n' "$0" >&2
  exit 2
}
case "$BINARY" in
  /*) ;;
  *) BINARY="$(pwd)/$BINARY" ;;
esac
[ -f "$BINARY" ] || { printf 'dws binary not found: %s\n' "$BINARY" >&2; exit 1; }

work="$(mktemp -d "${TMPDIR:-/tmp}/dws-runtime-bundle.XXXXXX")"
trap 'rm -rf "$work"' EXIT HUP INT TERM
"$ROOT/scripts/build/prepare-runtime-payload.sh" "$GOOS" "$GOARCH" "$work"
payload="$work/.dws-runtime/20260825"

hash_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

if [ "$GOOS" = darwin ]; then
  library="$payload/x7k2m9p4q1w8.dylib"
  if command -v codesign >/dev/null 2>&1; then
    codesign --force --sign - "$library"
  elif command -v rcodesign >/dev/null 2>&1; then
    rcodesign sign "$library"
  else
    printf 'codesign or rcodesign is required to sign the macOS runtime payload\n' >&2
    exit 1
  fi
  library_sha="$(hash_file "$library")"
  sed "s/\"library_sha256\": \"[0-9a-f]*\"/\"library_sha256\": \"$library_sha\"/" \
    "$payload/manifest.json" > "$payload/manifest.json.new"
  mv "$payload/manifest.json.new" "$payload/manifest.json"
fi

(cd "$ROOT" && go run ./scripts/build/runtime-payload inject "$BINARY" "$payload")

if [ "$GOOS" = darwin ]; then
  if command -v codesign >/dev/null 2>&1; then
    codesign --force --sign - "$BINARY"
  else
    rcodesign sign "$BINARY"
  fi
fi

printf 'Attached runtime payload for %s/%s.\n' "$GOOS" "$GOARCH"
