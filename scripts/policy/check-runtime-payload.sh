#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
PAYLOAD="$ROOT/third_party/runtimepayload/20260825"
ALLOW_UNSUPPORTED_TOOLS=0

if [ "${1:-}" = "--allow-unsupported-tools" ]; then
  ALLOW_UNSUPPORTED_TOOLS=1
  shift
fi
[ "$#" -eq 0 ] || {
  printf 'usage: %s [--allow-unsupported-tools]\n' "$0" >&2
  exit 2
}

fail() {
  printf 'runtime payload verification failed: %s\n' "$*" >&2
  exit 1
}

hash_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

check_hash() {
  relative="$1"
  expected="$2"
  path="$PAYLOAD/$relative"
  [ -f "$path" ] || fail "missing $relative"
  actual="$(hash_file "$path")"
  [ "$actual" = "$expected" ] || fail "checksum mismatch for $relative"
}

check_hash darwin/universal/x7k2m9p4q1w8.dylib a6f92e7ea30eadb68ff6e5f425166d7644842003abc72a27ec8186145f36b1f9
check_hash linux/amd64/libx7k2m9p4q1w8.so 174b59ba2e46195e81dbcbe3aac83dedbf5baaceec41d02a684e623ddaace481
check_hash linux/arm64/libx7k2m9p4q1w8.so aa81cd1c19493ead17e54a61b1845acd0e2f28fb61a3a7914e5e6a669bcaa83d
check_hash windows/amd64/x7k2m9p4q1w864.dll 7a19607bfda0dc827e1005cd3601d297fa380eedd9c8a3f17a15637fc1b6e6bf
check_hash windows/arm64/x7k2m9p4q1w864.dll 1faf52132bda5e64051610e08bab40c9be4df4c873c4694db6d4489ab5efec9a

checksum_count="$(wc -l < "$PAYLOAD/SHA256SUMS" | tr -d ' ')"
[ "$checksum_count" = 128 ] || fail "expected 128 SHA-256 entries, found $checksum_count"
while read -r expected relative; do
  [ -n "$expected" ] || continue
  check_hash "$relative" "$expected"
done < "$PAYLOAD/SHA256SUMS"

ps_count="$(find "$PAYLOAD/ps" -type f | wc -l | tr -d ' ')"
[ "$ps_count" = 123 ] || fail "expected 123 ps files, found $ps_count"

ps_digest="$({
  find "$PAYLOAD/ps" -type f | LC_ALL=C sort | while IFS= read -r path; do
    printf '%s  ps/%s\n' "$(hash_file "$path")" "$(basename "$path")"
  done
} | if command -v sha256sum >/dev/null 2>&1; then sha256sum; else shasum -a 256; fi | awk '{print $1}')"
[ "$ps_digest" = 45ae147697c1f8683df3f232d0ba792b807179bbe22fdac8225a0cf25fc33e7e ] || fail "ps manifest checksum mismatch"

darwin_exports="$(nm -gU "$PAYLOAD/darwin/universal/x7k2m9p4q1w8.dylib" 2>/dev/null || true)"
if [ -z "$darwin_exports" ] && command -v llvm-nm >/dev/null 2>&1; then
  darwin_exports="$(llvm-nm -g --defined-only "$PAYLOAD/darwin/universal/x7k2m9p4q1w8.dylib" 2>/dev/null || true)"
fi
linux_amd64_exports="$(nm -D --defined-only "$PAYLOAD/linux/amd64/libx7k2m9p4q1w8.so" 2>/dev/null || true)"
linux_arm64_exports="$(nm -D --defined-only "$PAYLOAD/linux/arm64/libx7k2m9p4q1w8.so" 2>/dev/null || true)"
windows_amd64_exports="$(objdump -p "$PAYLOAD/windows/amd64/x7k2m9p4q1w864.dll" 2>/dev/null || true)"
windows_arm64_exports="$(objdump -p "$PAYLOAD/windows/arm64/x7k2m9p4q1w864.dll" 2>/dev/null || true)"
if [ -z "$windows_amd64_exports" ] && command -v llvm-objdump >/dev/null 2>&1; then
  windows_amd64_exports="$(llvm-objdump -p "$PAYLOAD/windows/amd64/x7k2m9p4q1w864.dll" 2>/dev/null || true)"
fi
if [ -z "$windows_arm64_exports" ] && command -v llvm-objdump >/dev/null 2>&1; then
  windows_arm64_exports="$(llvm-objdump -p "$PAYLOAD/windows/arm64/x7k2m9p4q1w864.dll" 2>/dev/null || true)"
fi

check_darwin=1
check_linux_amd64=1
check_linux_arm64=1
check_windows_amd64=1
check_windows_arm64=1

if [ -z "$darwin_exports" ]; then
  [ "$ALLOW_UNSUPPORTED_TOOLS" -eq 1 ] || fail "cannot inspect macOS library exports"
  check_darwin=0
  printf 'Skipping macOS library export inspection: compatible tooling is unavailable.\n'
fi
if [ -z "$linux_amd64_exports" ]; then
  [ "$ALLOW_UNSUPPORTED_TOOLS" -eq 1 ] || fail "cannot inspect Linux amd64 library exports"
  check_linux_amd64=0
  printf 'Skipping Linux amd64 library export inspection: compatible tooling is unavailable.\n'
fi
if [ -z "$linux_arm64_exports" ]; then
  [ "$ALLOW_UNSUPPORTED_TOOLS" -eq 1 ] || fail "cannot inspect Linux arm64 library exports"
  check_linux_arm64=0
  printf 'Skipping Linux arm64 library export inspection: compatible tooling is unavailable.\n'
fi
if [ -z "$windows_amd64_exports" ]; then
  [ "$ALLOW_UNSUPPORTED_TOOLS" -eq 1 ] || fail "cannot inspect Windows amd64 library exports"
  check_windows_amd64=0
  printf 'Skipping Windows amd64 library export inspection: compatible tooling is unavailable.\n'
fi
if [ -z "$windows_arm64_exports" ]; then
  [ "$ALLOW_UNSUPPORTED_TOOLS" -eq 1 ] || fail "cannot inspect Windows arm64 library exports"
  check_windows_arm64=0
  printf 'Skipping Windows arm64 library export inspection: compatible tooling is unavailable.\n'
fi

for symbol in k9Xm2pQv d4Rw7Lnz h6Yb3Jtq m8Vc5Kxf p2Zn9Gsa t5Qe1Hud b7Uj4Myr f3Wi8Olc; do
  [ "$check_darwin" -eq 0 ] || printf '%s\n' "$darwin_exports" | grep -Eq "[[:space:]]_?${symbol}$" || fail "missing macOS export $symbol"
  [ "$check_linux_amd64" -eq 0 ] || printf '%s\n' "$linux_amd64_exports" | grep -Eq "[[:space:]]${symbol}$" || fail "missing Linux amd64 export $symbol"
  [ "$check_linux_arm64" -eq 0 ] || printf '%s\n' "$linux_arm64_exports" | grep -Eq "[[:space:]]${symbol}$" || fail "missing Linux arm64 export $symbol"
  [ "$check_windows_amd64" -eq 0 ] || printf '%s\n' "$windows_amd64_exports" | grep -Eq "[[:space:]]${symbol}$" || fail "missing Windows amd64 export $symbol"
  [ "$check_windows_arm64" -eq 0 ] || printf '%s\n' "$windows_arm64_exports" | grep -Eq "[[:space:]]${symbol}$" || fail "missing Windows arm64 export $symbol"
done

file "$PAYLOAD/darwin/universal/x7k2m9p4q1w8.dylib" | grep -Eiq 'universal|arm64.*x86_64|x86_64.*arm64' || fail "invalid macOS library architecture"
file "$PAYLOAD/linux/amd64/libx7k2m9p4q1w8.so" | grep -Eiq 'ELF 64-bit.*x86-64' || fail "invalid Linux amd64 library architecture"
file "$PAYLOAD/linux/arm64/libx7k2m9p4q1w8.so" | grep -Eiq 'ELF 64-bit.*(ARM aarch64|ARM64)' || fail "invalid Linux arm64 library architecture"
file "$PAYLOAD/windows/amd64/x7k2m9p4q1w864.dll" | grep -Eiq 'PE32\+.*x86-64' || fail "invalid Windows amd64 library architecture"
file "$PAYLOAD/windows/arm64/x7k2m9p4q1w864.dll" | grep -Eiq 'PE32\+.*Aarch64' || fail "invalid Windows arm64 library architecture"

printf 'Runtime payload verified: 5 libraries, 6 targets, 123 ps files.\n'
