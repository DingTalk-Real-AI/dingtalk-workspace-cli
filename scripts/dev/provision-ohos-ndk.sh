#!/usr/bin/env bash
# Provisions the pinned OpenHarmony NDK from the fork-release parts when the
# environment does not preinstall OHOS_NDK. Prints the NDK root on stdout.
set -euo pipefail

readonly OHOS_NDK_PARTS_URL_DEFAULT='https://github.com/typefield/dingtalk-workspace-cli/releases/download/ohos-ndk-26.0.0.851-toolchain'
readonly OHOS_NDK_SHA256_DEFAULT='35f63f4882f5845615057264027ffde78ab1b837ba7497f3641fb080062a93d7'

root="${OHOS_NDK_ROOT:-${TMPDIR:-/tmp}/dws-ohos-ndk}"
expected_sha="${OHOS_NDK_SHA256_OVERRIDE:-$OHOS_NDK_SHA256_DEFAULT}"
parts_url="${OHOS_NDK_PARTS_URL:-$OHOS_NDK_PARTS_URL_DEFAULT}"

if [ -x "$root/native/llvm/bin/aarch64-unknown-linux-ohos-clang" ]; then
  printf '%s\n' "$root"
  exit 0
fi

command -v curl >/dev/null 2>&1 || { printf 'curl is required\n' >&2; exit 1; }

work="${OHOS_NDK_WORK:-${TMPDIR:-/tmp}/dws-ohos-ndk-build}"
rm -rf "$work"
mkdir -p "$work"

printf 'provision-ohos-ndk: fetching pinned NDK parts from %s\n' "$parts_url" >&2
for part in ohos-ndk.tar.gz.part-00 ohos-ndk.tar.gz.part-01 ohos-ndk.tar.gz.part-02 ohos-ndk.tar.gz.part-03 ohos-ndk.tar.gz.part-04 ohos-ndk.tar.gz.part-05; do
  curl -fL --retry 3 -o "$work/$part" "$parts_url/$part"
done
cat "$work"/ohos-ndk.tar.gz.part-* > "$work/ohos-ndk.tar.gz"

if command -v sha256sum >/dev/null 2>&1; then
  printf '%s  %s\n' "$expected_sha" "$work/ohos-ndk.tar.gz" | sha256sum -c - >/dev/null
else
  actual="$(shasum -a 256 "$work/ohos-ndk.tar.gz" | awk '{print $1}')"
  [ "$actual" = "$expected_sha" ] || {
    printf 'provision-ohos-ndk: archive sha256 mismatch: %s\n' "$actual" >&2
    exit 1
  }
fi

tar -xzf "$work/ohos-ndk.tar.gz" -C "$work"
if [ ! -x "$work/native/llvm/bin/aarch64-unknown-linux-ohos-clang" ]; then
  printf 'provision-ohos-ndk: archive does not contain an OpenHarmony NDK clang\n' >&2
  exit 1
fi

rm -rf "$root"
mkdir -p "$(dirname "$root")"
mv "$work" "$root"
printf 'provision-ohos-ndk: ready at %s\n' "$root" >&2
printf '%s\n' "$root"
