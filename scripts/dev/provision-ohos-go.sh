#!/usr/bin/env bash
# Provisions the OpenHarmony Go toolchain from a caller-supplied archive when
# OHOS_GO is not set. Prints the toolchain's go binary path on stdout.
#
# The repository does not host the OpenHarmony Go toolchain. Obtain a
# go1.26.7 toolchain whose `go tool dist list` reports `openharmony/arm64`
# from the OpenHarmony SIG / Huawei distribution channels, then point this
# script at the archive:
#
#   OHOS_GO_ARCHIVE=/path/to/ohos-go.tar.gz            # local file, or
#   OHOS_GO_ARCHIVE=https://example/ohos-go.tar.gz     # https URL
#   OHOS_GO_SHA256=<sha256>                            # required for URLs
set -euo pipefail

readonly OHOS_GO_VERSION='go1.26.7'

root="${OHOS_GO_ROOT:-${TMPDIR:-/tmp}/dws-ohos-go}"
source="${OHOS_GO_ARCHIVE:-}"
expected_sha="${OHOS_GO_SHA256:-}"

if [ -x "$root/bin/go" ] && "$root/bin/go" version 2>/dev/null | grep -Fq "$OHOS_GO_VERSION" \
  && "$root/bin/go" tool dist list 2>/dev/null | grep -Fxq 'openharmony/arm64'; then
  printf '%s/bin/go\n' "$root"
  exit 0
fi

fail() {
  printf 'provision-ohos-go: %s\n' "$1" >&2
  printf 'provision-ohos-go: set OHOS_GO to an executable %s toolchain, or set OHOS_GO_ARCHIVE to its .tar.gz archive (plus OHOS_GO_SHA256 for https URLs)\n' "$OHOS_GO_VERSION" >&2
  exit 1
}

[ -n "$source" ] || fail "OHOS_GO_ARCHIVE is not set"

work="${OHOS_GO_WORK:-${TMPDIR:-/tmp}/dws-ohos-go-build}"
rm -rf "$work"
mkdir -p "$work"
archive="$work/$(basename "${source%%[?#]*}")"

case "$source" in
  http://*|https://*)
    [ -n "$expected_sha" ] || fail "OHOS_GO_SHA256 is required when OHOS_GO_ARCHIVE is a URL"
    printf 'provision-ohos-go: downloading %s\n' "$source" >&2
    if command -v curl >/dev/null 2>&1; then
      curl -fL --retry 3 -o "$archive" "$source" >/dev/null 2>&1 || fail "download failed: $source"
    else
      fail "curl is required to download OHOS_GO_ARCHIVE"
    fi
    ;;
  *)
    [ -f "$source" ] || fail "archive does not exist: $source"
    cp "$source" "$archive"
    ;;
esac

if [ -n "$expected_sha" ]; then
  if command -v sha256sum >/dev/null 2>&1; then
    printf '%s  %s\n' "$expected_sha" "$archive" | sha256sum -c - >/dev/null || fail "archive sha256 mismatch"
  else
    actual="$(shasum -a 256 "$archive" | awk '{print $1}')"
    [ "$actual" = "$expected_sha" ] || fail "archive sha256 mismatch: $actual"
  fi
fi

tar -xzf "$archive" -C "$work"
toolchain_bin="$(find "$work" -mindepth 3 -maxdepth 3 -type f -path '*/bin/go' | head -n1)"
[ -n "$toolchain_bin" ] || fail "archive does not contain a Go toolchain (*/bin/go)"
toolchain_dir="$(dirname "$(dirname "$toolchain_bin")")"

"$toolchain_dir/bin/go" version 2>/dev/null | grep -Fq "$OHOS_GO_VERSION" \
  || fail "toolchain does not report $OHOS_GO_VERSION"
"$toolchain_dir/bin/go" tool dist list 2>/dev/null | grep -Fxq 'openharmony/arm64' \
  || fail "toolchain lacks openharmony/arm64"

rm -rf "$root"
mkdir -p "$(dirname -- "$root")"
mv "$toolchain_dir" "$root"
rm -rf "$work"
printf 'provision-ohos-go: ready at %s/bin/go\n' "$root" >&2
printf '%s/bin/go\n' "$root"
