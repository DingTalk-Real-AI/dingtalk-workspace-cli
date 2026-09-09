#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
GOOS="${1:-}"
GOARCH="${2:-}"
DEST_ROOT="${3:-}"
VERSION=20260825
SOURCE="$ROOT/third_party/runtimepayload/$VERSION"

[ -n "$GOOS" ] && [ -n "$GOARCH" ] && [ -n "$DEST_ROOT" ] || {
  printf 'usage: %s <goos> <goarch> <destination-root>\n' "$0" >&2
  exit 2
}
[ "$DEST_ROOT" != / ] || { printf 'destination root must not be /\n' >&2; exit 2; }

case "$GOOS/$GOARCH" in
  darwin/amd64|darwin/arm64)
    RELATIVE_LIBRARY=darwin/universal/x7k2m9p4q1w8.dylib
    LIBRARY_NAME=x7k2m9p4q1w8.dylib
    ;;
  linux/amd64)
    RELATIVE_LIBRARY=linux/amd64/libx7k2m9p4q1w8.so
    LIBRARY_NAME=libx7k2m9p4q1w8.so
    ;;
  linux/arm64)
    RELATIVE_LIBRARY=linux/arm64/libx7k2m9p4q1w8.so
    LIBRARY_NAME=libx7k2m9p4q1w8.so
    ;;
  windows/amd64)
    RELATIVE_LIBRARY=windows/amd64/x7k2m9p4q1w864.dll
    LIBRARY_NAME=x7k2m9p4q1w864.dll
    ;;
  windows/arm64)
    RELATIVE_LIBRARY=windows/arm64/x7k2m9p4q1w864.dll
    LIBRARY_NAME=x7k2m9p4q1w864.dll
    ;;
  *)
    printf 'unsupported runtime target: %s/%s\n' "$GOOS" "$GOARCH" >&2
    exit 1
    ;;
esac

hash_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

TARGET="$DEST_ROOT/.dws-runtime/$VERSION"
rm -rf "$TARGET"
mkdir -p "$TARGET/ps"
cp "$SOURCE/$RELATIVE_LIBRARY" "$TARGET/$LIBRARY_NAME"
cp -R "$SOURCE/ps/." "$TARGET/ps/"

LIBRARY_SHA="$(hash_file "$TARGET/$LIBRARY_NAME")"
cat > "$TARGET/manifest.json" <<EOF
{
  "format_version": 1,
  "payload_version": "$VERSION",
  "target": "$GOOS/$GOARCH",
  "library": "$LIBRARY_NAME",
  "library_sha256": "$LIBRARY_SHA",
  "ps_file_count": 123,
  "ps_manifest_sha256": "45ae147697c1f8683df3f232d0ba792b807179bbe22fdac8225a0cf25fc33e7e"
}
EOF

printf 'Prepared runtime payload for %s/%s.\n' "$GOOS" "$GOARCH"
