#!/bin/sh
set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
ROOT="$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)"
DIST_DIR="${DWS_PACKAGE_DIST_DIR:-$ROOT/dist}"
VERSION="${1:-${DWS_PACKAGE_VERSION:-}}"

[ -n "$VERSION" ] || { printf 'expected release version is required\n' >&2; exit 2; }
SEMVER="${VERSION#v}"
CHECKSUMS="$DIST_DIR/checksums.txt"
EXPECTED_PLATFORM_ASSETS="
dws-darwin-amd64.tar.gz
dws-darwin-arm64.tar.gz
dws-linux-amd64.tar.gz
dws-linux-arm64.tar.gz
dws-windows-amd64.zip
dws-windows-arm64.zip
"
EXPECTED_ASSETS="$EXPECTED_PLATFORM_ASSETS
dws-skills.zip
"

[ -f "$CHECKSUMS" ] || { printf 'missing checksums.txt in %s\n' "$DIST_DIR" >&2; exit 1; }

tmp="$(mktemp -d "${TMPDIR:-/tmp}/dws-release-binary.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

# The public asset namespace is an exact set. GoReleaser may also leave local
# metadata files (artifacts.json/config.yaml/metadata.json), but the upload and
# npm staging globs never publish them.
printf '%s\nchecksums.txt\n' "$EXPECTED_ASSETS" | sed '/^$/d' | LC_ALL=C sort > "$tmp/expected-root"
for path in "$DIST_DIR"/dws-*.tar.gz "$DIST_DIR"/dws-*.zip "$CHECKSUMS"; do
  [ -f "$path" ] || continue
  basename "$path"
done | LC_ALL=C sort > "$tmp/actual-root"
if ! diff -u "$tmp/expected-root" "$tmp/actual-root"; then
  printf 'public release assets must contain exactly the supported files\n' >&2
  exit 1
fi

checksum_format_ok=1
awk '
  NF != 2 { bad = 1; next }
  $1 !~ /^[0-9a-fA-F]{64}$/ { bad = 1; next }
  { print $2 }
  END { if (bad) exit 1 }
' "$CHECKSUMS" > "$tmp/checksum-assets" || checksum_format_ok=0
[ "$checksum_format_ok" -eq 1 ] || {
  printf 'checksums.txt must contain only SHA-256 and filename pairs\n' >&2
  exit 1
}

skills_checksum_count="$(awk '$2 == "dws-skills.zip" { count++ } END { print count + 0 }' "$CHECKSUMS")"
[ "$skills_checksum_count" -eq 1 ] || {
  printf 'checksums.txt must contain dws-skills.zip exactly once (found %s)\n' "$skills_checksum_count" >&2
  exit 1
}
printf '%s\n' "$EXPECTED_ASSETS" | sed '/^$/d' | LC_ALL=C sort > "$tmp/expected-checksums"
LC_ALL=C sort "$tmp/checksum-assets" > "$tmp/actual-checksums"
if ! diff -u "$tmp/expected-checksums" "$tmp/actual-checksums"; then
  printf 'checksums.txt must describe exactly the supported release assets\n' >&2
  exit 1
fi

if command -v sha256sum >/dev/null 2>&1; then
  (cd "$DIST_DIR" && sha256sum --check checksums.txt)
elif command -v shasum >/dev/null 2>&1; then
  (cd "$DIST_DIR" && shasum -a 256 --check checksums.txt)
else
  printf 'sha256sum or shasum is required\n' >&2
  exit 1
fi

verify_binary_version() {
  asset="$1"
  extract_dir="$tmp/extract-${asset}"
  mkdir -p "$extract_dir"
  case "$asset" in
    *.tar.gz)
      tar -xzf "$DIST_DIR/$asset" -C "$extract_dir"
      binary="$extract_dir/dws"
      ;;
    *.zip)
      unzip -q "$DIST_DIR/$asset" -d "$extract_dir"
      binary="$extract_dir/dws.exe"
      ;;
    *)
      printf 'unsupported release archive: %s\n' "$asset" >&2
      return 1
      ;;
  esac
  [ -f "$binary" ] || {
    printf '%s does not contain the expected dws binary\n' "$asset" >&2
    return 1
  }
  LC_ALL=C grep -aFq "v$SEMVER" "$binary" || {
    printf '%s binary does not embed expected version v%s\n' "$asset" "$SEMVER" >&2
    return 1
  }
}

for asset in $EXPECTED_PLATFORM_ASSETS; do
  verify_binary_version "$asset"
done

printf 'Release artifacts verified for v%s.\n' "$SEMVER"
