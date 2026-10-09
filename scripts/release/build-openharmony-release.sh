#!/usr/bin/env bash
# Builds the self-signed openharmony/arm64 package and stages it into the
# GoReleaser dist tree so the ordinary release upload publishes it next to
# the six-platform archives. The OpenHarmony Go toolchain is reassembled
# from the pinned fork-release parts and validated against the recorded
# SHA-256 before use; the binary must be a static ET_EXEC ELF carrying the
# fs-verity .codesign self-signature.
set -euo pipefail

SCRIPT_DIR="$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)"
ROOT="$(CDPATH='' cd -- "$SCRIPT_DIR/../.." && pwd)"

OHOS_GO_SHA256_DEFAULT="e757acdc005098f1debc888cdbaa13e26faf48e2bcf38baa17cbc5df49d8d125"
OHOS_GO_PARTS_URL_DEFAULT="https://github.com/typefield/dingtalk-workspace-cli/releases/download/ohos-go1.26.7-toolchain"
OHOS_GO_SHA256="${OHOS_GO_SHA256:-$OHOS_GO_SHA256_DEFAULT}"
OHOS_GO_PARTS_URL="${OHOS_GO_PARTS_URL:-$OHOS_GO_PARTS_URL_DEFAULT}"

usage() {
  printf 'usage: %s --version vX.Y.Z... --commit SHA --dist-dir DIR\n' "$0" >&2
  exit 2
}

version=""
commit=""
dist_dir=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    --version) [ "$#" -ge 2 ] || usage; version="$2"; shift 2 ;;
    --commit) [ "$#" -ge 2 ] || usage; commit="$2"; shift 2 ;;
    --dist-dir) [ "$#" -ge 2 ] || usage; dist_dir="$2"; shift 2 ;;
    *) usage ;;
  esac
done
[ -n "$version" ] && [ -n "$commit" ] && [ -n "$dist_dir" ] || usage
case "$version" in
  v[0-9]*) ;;
  *) printf 'release version must look like vX.Y.Z: %s\n' "$version" >&2; exit 2 ;;
esac

command -v curl >/dev/null 2>&1 || { printf 'curl is required\n' >&2; exit 1; }

work="$(mktemp -d "${TMPDIR:-/tmp}/dws-openharmony-release.XXXXXX")"
trap 'rm -rf "$work"' EXIT HUP INT TERM

mkdir -p "$work/toolchain"
cd "$work/toolchain"
for part in ohos-go.tar.gz.part-00 ohos-go.tar.gz.part-01 ohos-go.tar.gz.part-02 ohos-go.tar.gz.part-03 ohos-go.tar.gz.part-04; do
  curl -fL --retry 3 -o "$part" "$OHOS_GO_PARTS_URL/$part"
done
cat ohos-go.tar.gz.part-* > ohos-go.tar.gz
printf '%s  ohos-go.tar.gz\n' "$OHOS_GO_SHA256" | sha256sum -c -

DWS_PACKAGE_VERSION="$version" \
DWS_GIT_COMMIT="$commit" \
DWS_REQUIRE_ELF_STATIC=1 \
DWS_OPENHARMONY_CGO=1 \
OHOS_GO_ARCHIVE="$work/toolchain/ohos-go.tar.gz" \
  "$ROOT/scripts/dev/package-openharmony.sh" "$work/package"

cd "$work/package"
sha256sum -c checksums.txt
tar -tzf dws-openharmony-arm64.tar.gz >/dev/null
extract="$work/extract"
mkdir -p "$extract"
tar -xzf dws-openharmony-arm64.tar.gz -C "$extract"
"$ROOT/scripts/dev/verify-openharmony-artifact.sh" "$extract/dws" "$version"

asset="dws-openharmony-arm64.tar.gz"
cp "$asset" "$dist_dir/$asset"
archive_sha="$(sha256sum "$dist_dir/$asset" | awk '{print $1}')"
if grep -Fq "  $asset" "$dist_dir/checksums.txt"; then
  printf 'checksums.txt already describes %s\n' "$asset" >&2
  exit 1
fi
printf '%s  %s\n' "$archive_sha" "$asset" >> "$dist_dir/checksums.txt"
printf 'OpenHarmony release asset staged: %s (%s)\n' "$asset" "$archive_sha"
