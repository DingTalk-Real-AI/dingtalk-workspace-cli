#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
PAYLOAD="$ROOT/third_party/runtimepayload/20261028"
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

# Windows architecture assertion. Byte integrity is already pinned by the
# SHA-256 checks above; the COFF machine field is the byte-level ground truth
# for architecture and is read directly with od(1), which is POSIX and
# present on every supported host. Tool magic databases are not stable across
# toolchain releases: GNU objdump 2.46 on Ubuntu 26 cannot parse ARM64 PE
# images at all, and file(1) 5.46 renamed COFF machine 0xaa64 from "Aarch64"
# to "ARM64", so tool output is advisory evidence only - it can confirm the
# expected architecture but never overrule the machine field, and a probe
# that cannot decide falls through instead of failing.
pe_coff_machine() {
  # e_lfanew (4 bytes LE at offset 0x3C) locates the PE signature; the COFF
  # machine field is 2 bytes LE at e_lfanew+4. Prints the machine value as
  # decimal, or nothing when the header cannot be parsed.
  pe_off_bytes="$(od -A n -j 60 -N 4 -t u1 "$1" 2>/dev/null || true)"
  pe_off="$(printf '%s\n' "$pe_off_bytes" | awk 'NF==4 {print $1 + 256*$2 + 65536*$3 + 16777216*$4}')"
  [ -n "$pe_off" ] || return 2
  pe_sig="$(od -A n -j "$pe_off" -N 4 -t u1 "$1" 2>/dev/null || true)"
  printf '%s\n' "$pe_sig" | awk 'NF==4 && $1==80 && $2==69 && $3==0 && $4==0 {print 1}' | grep -q '^1$' || return 2
  pe_machine_bytes="$(od -A n -j "$((pe_off + 4))" -N 2 -t u1 "$1" 2>/dev/null || true)"
  printf '%s\n' "$pe_machine_bytes" | awk 'NF==2 {print $1 + 256*$2}'
}

check_pe_arch() {
  pe_relative="$1"
  pe_objdump_pattern="$2"
  pe_file_pattern="$3"
  pe_label="$4"
  pe_expected_machine="$5"
  pe_path="$PAYLOAD/$pe_relative"

  pe_machine="$(pe_coff_machine "$pe_path" || true)"
  if [ -n "$pe_machine" ]; then
    [ "$pe_machine" -eq "$pe_expected_machine" ] || fail "invalid $pe_label library architecture (COFF machine 0x$(printf '%x' "$pe_machine"))"
    return 0
  fi

  # Advisory probes as a legacy fallback when the header cannot be extracted.
  pe_probe="$(objdump -f "$pe_path" 2>/dev/null || true)"
  # Keep only the architecture line: the probe echoes the file path first, and
  # the amd64/arm64 token in that path is not architecture evidence.
  pe_arch_line="$(printf '%s\n' "$pe_probe" | grep -i '^architecture:' || true)"
  if printf '%s\n' "$pe_arch_line" | grep -Eiq "$pe_objdump_pattern"; then
    return 0
  fi
  if command -v llvm-objdump >/dev/null 2>&1; then
    pe_probe="$(llvm-objdump -f "$pe_path" 2>/dev/null || true)"
    pe_arch_line="$(printf '%s\n' "$pe_probe" | grep -i '^architecture:' || true)"
    if printf '%s\n' "$pe_arch_line" | grep -Eiq "$pe_objdump_pattern"; then
      return 0
    fi
  fi
  # Likewise file(1) prints "<path>: <description>"; keep the description.
  pe_probe="$(file "$pe_path" 2>/dev/null || true)"
  pe_desc="${pe_probe#*: }"
  if printf '%s\n' "$pe_desc" | grep -Eiq "$pe_file_pattern"; then
    return 0
  fi

  [ "$ALLOW_UNSUPPORTED_TOOLS" -eq 1 ] || fail "cannot inspect $pe_label library architecture"
  printf 'Skipping %s library architecture inspection: compatible tooling is unavailable.\n' "$pe_label"
}

check_hash darwin/universal/x7k2m9p4q1w8.dylib 8e4460791f12381863f5844ded6423b2a9b75d6f394e9bd146410e7f160b0c5c
check_hash linux/amd64/libx7k2m9p4q1w8.so d4e849134c311e9af71ef8bd018e5b4b94617edd79a30cc13a998caaf6ed10e0
check_hash linux/arm64/libx7k2m9p4q1w8.so c160e5a5205ddc0f8320b616c0d5404899601cd6683b4e2ce351c87f2a146c36
check_hash windows/amd64/x7k2m9p4q1w864.dll 2c9a0eff98bc7c387e21224037f51da011797416e17504387c8f6311d773bd84
check_hash windows/arm64/x7k2m9p4q1w864.dll b12ed8d654628f789b6c1aff795b6208da228306a80a2486178bf2b36b52917b

check_hash linux/skt.bin f7aa6d4fb8140f0e393efbdce55ad833df64aa5984698ca6ebc9e19bb35bd79e
check_hash windows/skt.bin fcec7d34327bc53cc2aa0dcc5aca3c4a76d7e2f5783350595eb25f1909345dae
check_hash darwin/skt.bin 32e91c83c3d327e8192035f03c01bce663e3792c7082b7b45cc8b551ac75c61b

checksum_count="$(wc -l < "$PAYLOAD/SHA256SUMS" | tr -d ' ')"
[ "$checksum_count" = 8 ] || fail "expected 8 SHA-256 entries, found $checksum_count"
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

for symbol in k9Xm2pQv d4Rw7Lnz h6Yb3Jtq m8Vc5Kxf p2Zn9Gsa t5Qe1Hud b7Uj4Myr f3Wi8Olc r7Qk3Wzp m2Xt9Bvn; do
  [ "$check_darwin" -eq 0 ] || printf '%s\n' "$darwin_exports" | grep -Eq "[[:space:]]_?${symbol}$" || fail "missing macOS export $symbol"
  [ "$check_linux_amd64" -eq 0 ] || printf '%s\n' "$linux_amd64_exports" | grep -Eq "[[:space:]]${symbol}$" || fail "missing Linux amd64 export $symbol"
  [ "$check_linux_arm64" -eq 0 ] || printf '%s\n' "$linux_arm64_exports" | grep -Eq "[[:space:]]${symbol}$" || fail "missing Linux arm64 export $symbol"
  [ "$check_windows_amd64" -eq 0 ] || printf '%s\n' "$windows_amd64_exports" | grep -Eq "[[:space:]]${symbol}$" || fail "missing Windows amd64 export $symbol"
  [ "$check_windows_arm64" -eq 0 ] || printf '%s\n' "$windows_arm64_exports" | grep -Eq "[[:space:]]${symbol}$" || fail "missing Windows arm64 export $symbol"
done

file "$PAYLOAD/darwin/universal/x7k2m9p4q1w8.dylib" | grep -Eiq 'universal|arm64.*x86_64|x86_64.*arm64' || fail "invalid macOS library architecture"
file "$PAYLOAD/linux/amd64/libx7k2m9p4q1w8.so" | grep -Eiq 'ELF 64-bit.*x86-64' || fail "invalid Linux amd64 library architecture"
file "$PAYLOAD/linux/arm64/libx7k2m9p4q1w8.so" | grep -Eiq 'ELF 64-bit.*(ARM aarch64|ARM64)' || fail "invalid Linux arm64 library architecture"
# Expected machines are the COFF values: 0x8664 = AMD64, 0xaa64 = ARM64.
check_pe_arch windows/amd64/x7k2m9p4q1w864.dll 'x86[_-]64' 'PE32\+.*x86-64' 'Windows amd64' 34404
# file(1) names COFF machine 0xaa64 "Aarch64" in older magic and "ARM64" in newer magic.
check_pe_arch windows/arm64/x7k2m9p4q1w864.dll 'aarch64|arm64' 'PE32\+.*(Aarch64|ARM64)' 'Windows arm64' 43620

printf 'Runtime payload verified: 5 libraries, 3 configurations, 6 targets, no ps files.\n'
if [ "$ALLOW_UNSUPPORTED_TOOLS" -eq 0 ]; then
  "$ROOT/scripts/build/generate-runtime-payload-assets.sh" --check
fi
