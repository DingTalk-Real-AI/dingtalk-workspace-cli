#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
PAYLOAD="$ROOT/third_party/runtimepayload/20260909"
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

# Read the PE/COFF Machine field directly. Host objdump builds may guess an
# unsupported PE target's architecture, and file(1) descriptions vary by database.
# Bytes are read individually so this is independent of the host's endianness.
check_pe_arch() {
  pe_path="$PAYLOAD/$1"
  pe_machine="$2"
  pe_label="$3"

  pe_bytes="$(od -An -tu1 -j 60 -N 4 "$pe_path")" || fail "cannot read $pe_label PE offset"
  set -- $pe_bytes
  [ "$#" -eq 4 ] || fail "invalid $pe_label PE offset"
  pe_offset=$(( $1 + ($2 << 8) + ($3 << 16) + ($4 << 24) ))

  pe_bytes="$(od -An -tx1 -j "$pe_offset" -N 6 "$pe_path")" || fail "cannot read $pe_label PE header"
  set -- $pe_bytes
  [ "$#" -eq 6 ] || fail "invalid $pe_label PE header"
  [ "$1$2$3$4" = 50450000 ] || fail "invalid $pe_label PE signature"
  [ "$5$6" = "$pe_machine" ] || fail "invalid $pe_label library architecture (PE Machine bytes $5$6, expected $pe_machine)"
}

check_hash darwin/universal/x7k2m9p4q1w8.dylib bc9c5b94b710f043a448e1e89f12de7723a7d2468450f127c1de2efdc6b90742
check_hash linux/amd64/libx7k2m9p4q1w8.so 174b59ba2e46195e81dbcbe3aac83dedbf5baaceec41d02a684e623ddaace481
check_hash linux/arm64/libx7k2m9p4q1w8.so aa81cd1c19493ead17e54a61b1845acd0e2f28fb61a3a7914e5e6a669bcaa83d
check_hash windows/amd64/x7k2m9p4q1w864.dll 7b8b06f1776eb02f7101c0999e53eec04984cc51334ca2795b3bfdb44a4f449b
check_hash windows/arm64/x7k2m9p4q1w864.dll 057854bb037509659b1daab239de16e28a88e327cf302cd3312c8c72c5d05dfb

checksum_count="$(wc -l < "$PAYLOAD/SHA256SUMS" | tr -d ' ')"
[ "$checksum_count" = 5 ] || fail "expected 5 SHA-256 entries, found $checksum_count"
while read -r expected relative; do
  [ -n "$expected" ] || continue
  check_hash "$relative" "$expected"
done < "$PAYLOAD/SHA256SUMS"

[ ! -e "$PAYLOAD/ps" ] && [ ! -L "$PAYLOAD/ps" ] || fail "retired ps directory must not be distributed"

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
check_pe_arch windows/amd64/x7k2m9p4q1w864.dll 6486 'Windows amd64'
check_pe_arch windows/arm64/x7k2m9p4q1w864.dll 64aa 'Windows arm64'

printf 'Runtime payload verified: 5 libraries, 6 targets, no auxiliary data files.\n'
if [ "$ALLOW_UNSUPPORTED_TOOLS" -eq 0 ]; then
  "$ROOT/scripts/build/generate-runtime-payload-assets.sh" --check
fi
