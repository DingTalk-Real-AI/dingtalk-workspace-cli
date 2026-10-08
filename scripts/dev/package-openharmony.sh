#!/usr/bin/env bash
set -euo pipefail

ROOT="$(CDPATH='' cd "$(dirname "$0")/../.." && pwd)"
OUTPUT_DIR="${1:-$ROOT/dist/openharmony}"
mkdir -p "$OUTPUT_DIR"
OUTPUT_DIR="$(CDPATH='' cd "$OUTPUT_DIR" && pwd)"
cd "$ROOT"
archive="$OUTPUT_DIR/dws-openharmony-arm64.tar.gz"
checksums="$OUTPUT_DIR/checksums.txt"
# Remove stale outputs before creating temporary files so failures cannot
# expose an older successful package.
rm -f "$archive" "$checksums"
archive_tmp=''
checksums_tmp=''
stage=''
cleanup() {
  [ -z "$stage" ] || rm -rf "$stage"
  [ -z "$archive_tmp" ] || rm -f "$archive_tmp"
  [ -z "$checksums_tmp" ] || rm -f "$checksums_tmp"
}
trap cleanup EXIT
archive_tmp="$(mktemp "$OUTPUT_DIR/.dws-openharmony-arm64.tar.gz.XXXXXX")"
checksums_tmp="$(mktemp "$OUTPUT_DIR/.checksums.txt.XXXXXX")"
stage="$(mktemp -d "${TMPDIR:-/tmp}/dws-openharmony-package.XXXXXX")"
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

# enforce_static_elf verifies the static ELF shape: ET_EXEC with no PT_INTERP,
# no PT_DYNAMIC, and no NEEDED entries. It only runs when DWS_REQUIRE_ELF_STATIC=1;
# the fake-toolchain unit tests and local builds without an ELF reader skip it.
enforce_static_elf() {
  local bin="$1"
  if [ "${DWS_REQUIRE_ELF_STATIC:-0}" != "1" ]; then
    return 0
  fi
  if command -v readelf >/dev/null 2>&1 || command -v llvm-readelf >/dev/null 2>&1; then
    local re=readelf
    command -v readelf >/dev/null 2>&1 || re=llvm-readelf
    if ! "$re" -h "$bin" 2>/dev/null | grep -q 'Type:.*EXEC'; then
      printf 'OpenHarmony binary is not ET_EXEC\n' >&2
      return 1
    fi
    if "$re" -l "$bin" 2>/dev/null | grep -Eq 'INTERP|DYNAMIC'; then
      printf 'OpenHarmony binary has an INTERP or DYNAMIC segment\n' >&2
      return 1
    fi
    if ! "$re" -d "$bin" 2>/dev/null | grep -q 'There is no dynamic section in this file'; then
      printf 'OpenHarmony binary has a dynamic section with NEEDED entries\n' >&2
      return 1
    fi
  elif command -v llvm-objdump >/dev/null 2>&1; then
    if llvm-objdump -p "$bin" 2>/dev/null | grep -Eq 'INTERP|DYNAMIC'; then
      printf 'OpenHarmony binary has an INTERP or DYNAMIC segment\n' >&2
      return 1
    fi
  else
    printf 'DWS_REQUIRE_ELF_STATIC=1 but no readelf/llvm-readelf/llvm-objdump available\n' >&2
    return 1
  fi
  printf 'ELF static shape verified (ET_EXEC, no INTERP/DYNAMIC, no NEEDED)\n' >&2
}

OHOS_OUTPUT="$stage/dws" "$ROOT/scripts/dev/build-openharmony.sh"
if [ ! -s "$stage/dws" ]; then
  printf 'OpenHarmony build did not produce a non-empty binary\n' >&2
  exit 1
fi

if [ -n "${BINARY_SIGN_TOOL:-}" ]; then
  sign_tool="$BINARY_SIGN_TOOL"
  if ! sign_tool="$(command -v "$sign_tool" 2>/dev/null)" || [ ! -x "$sign_tool" ]; then
    printf 'BINARY_SIGN_TOOL must point to an executable signer\n' >&2
    exit 2
  fi
else
  go_bin="$(command -v go 2>/dev/null || true)"
  if [ -z "$go_bin" ] || [ ! -x "$go_bin" ]; then
    printf 'Go is required to build the OpenHarmony signer\n' >&2
    exit 2
  fi
  host_os="$($go_bin env GOHOSTOS)"
  host_arch="$($go_bin env GOHOSTARCH)"
  sign_tool="$stage/binary-sign-tool"
  if ! GOOS="$host_os" GOARCH="$host_arch" CGO_ENABLED=0 GOTOOLCHAIN=local \
    "$go_bin" build -trimpath -o "$sign_tool" ./cmd/binary-sign-tool; then
    printf 'failed to build the OpenHarmony Go signer\n' >&2
    exit 1
  fi
fi
signed="$stage/dws.signed"
"$sign_tool" sign -inFile "$stage/dws" -outFile "$signed" -selfSign "1"
if [ ! -s "$signed" ]; then
  printf 'binary-sign-tool did not produce a non-empty signed binary\n' >&2
  exit 1
fi
chmod +x "$signed"
mv "$signed" "$stage/dws"
if ! enforce_static_elf "$stage/dws"; then
  printf 'static ELF enforcement failed\n' >&2
  exit 1
fi

archive_stage="$stage/archive"
mkdir "$archive_stage"
cp "$ROOT/LICENSE" "$ROOT/NOTICE" "$ROOT/README.md" "$ROOT/CHANGELOG.md" "$stage/dws" "$archive_stage/"
(
  cd "$archive_stage"
  touch -t 202001010000 CHANGELOG.md LICENSE NOTICE README.md dws
  if command -v gtar >/dev/null 2>&1; then
    gtar --sort=name --owner=0 --group=0 --numeric-owner \
      --mtime='2020-01-01 00:00Z' -cf - \
      CHANGELOG.md LICENSE NOTICE README.md dws | gzip -n > "$archive_tmp"
  elif tar --version 2>/dev/null | grep -q 'GNU tar'; then
    tar --sort=name --owner=0 --group=0 --numeric-owner \
      --mtime='2020-01-01 00:00Z' -cf - \
      CHANGELOG.md LICENSE NOTICE README.md dws | gzip -n > "$archive_tmp"
  else
    printf '%s\n' CHANGELOG.md LICENSE NOTICE README.md dws \
      | COPYFILE_DISABLE=1 tar --no-recursion --uid 0 --gid 0 \
          --uname root --gname root --options gzip:!timestamp \
          -czf "$archive_tmp" -T -
  fi
)

if command -v sha256sum >/dev/null 2>&1; then
  digest="$(sha256sum "$archive_tmp" | awk '{print $1}')"
else
  digest="$(shasum -a 256 "$archive_tmp" | awk '{print $1}')"
fi
printf '%s  %s\n' "$digest" "$(basename "$archive")" > "$checksums_tmp"
mv "$archive_tmp" "$archive"
if ! mv "$checksums_tmp" "$checksums"; then
  rm -f "$archive"
  exit 1
fi
printf 'Signed and packaged %s\n' "$archive"
