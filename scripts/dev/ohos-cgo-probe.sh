#!/usr/bin/env bash
# Minimal OpenHarmony CGO link probe (M2 step 1 of the SafeChat integration).
#
# Compiles a tiny cgo program with the ohos-go toolchain and the OHOS NDK
# clang, links it statically, and verifies the ELF shape (ET_EXEC, no
# PT_INTERP/PT_DYNAMIC). This isolates toolchain/NDK problems from the full
# dws build before the SafeChat library enters the picture. Run it on a Linux
# host with OHOS_GO and OHOS_CC (or OHOS_NDK) set; then optionally copy
# ./ohos-cgo-probe to a HarmonyOS PC and execute it.
set -euo pipefail

ROOT="$(CDPATH='' cd -- "$(dirname "$0")/../.." && pwd)"
# Developer shells may export GOROOT pointing at the official toolchain; the
# ohos-go binary must resolve its own root even for `tool dist list`.
unset GOROOT
OHOS_GO="${OHOS_GO:-}"

if [ -z "$OHOS_GO" ] || [ ! -x "$OHOS_GO" ]; then
  printf 'OHOS_GO not provided; provisioning the pinned OpenHarmony toolchain\n' >&2
  OHOS_GO="$(bash "$ROOT/scripts/dev/provision-ohos-go.sh")"
fi

if ! "$OHOS_GO" tool dist list | grep -Fx 'openharmony/arm64' >/dev/null; then
  printf '%s does not support openharmony/arm64\n' "$OHOS_GO" >&2
  exit 1
fi

# Keep this resolution block in sync with scripts/dev/build-openharmony.sh.
ohos_cc="${OHOS_CC:-}"
if [ -z "$ohos_cc" ] && [ -n "${OHOS_NDK:-}" ]; then
  for candidate in \
    "$OHOS_NDK/native/llvm/bin/aarch64-unknown-linux-ohos-clang" \
    "$OHOS_NDK/llvm/bin/aarch64-unknown-linux-ohos-clang" \
    "$OHOS_NDK"/toolchains/llvm/prebuilt/*/bin/aarch64-unknown-linux-ohos-clang; do
    if [ -x "$candidate" ]; then
      ohos_cc="$candidate"
      break
    fi
  done
fi
if [ -z "$ohos_cc" ]; then
  printf 'OHOS_CC or OHOS_NDK (with aarch64-unknown-linux-ohos clang) is required\n' >&2
  exit 2
fi

command -v readelf >/dev/null 2>&1 || command -v llvm-readelf >/dev/null 2>&1 \
  || command -v llvm-objdump >/dev/null 2>&1 || command -v objdump >/dev/null 2>&1 || {
  printf 'readelf, llvm-readelf, llvm-objdump, or objdump is required for the ELF shape check\n' >&2
  exit 2
}

stage="$(mktemp -d "${TMPDIR:-/tmp}/dws-ohos-cgo-probe.XXXXXX")"
trap 'rm -rf "$stage"' EXIT

cat > "$stage/go.mod" <<'EOF'
module ohoscgoprobe

go 1.26.7
EOF
cat > "$stage/main.go" <<'EOF'
package main

/*
#include <stdlib.h>
static int probe_add(int a, int b) { return a + b; }
*/
import "C"

import "fmt"

func main() {
	fmt.Println("ohos cgo probe:", C.probe_add(1, 2))
}
EOF

(
  cd "$stage"
  CC="$ohos_cc" CXX="${ohos_cc}++" GOOS=openharmony GOARCH=arm64 CGO_ENABLED=1 \
    GOTOOLCHAIN=local "$OHOS_GO" build -tags netgo \
    -ldflags '-extldflags -static' -o probe .
)

if command -v readelf >/dev/null 2>&1 || command -v llvm-readelf >/dev/null 2>&1; then
  re=readelf
  command -v readelf >/dev/null 2>&1 || re=llvm-readelf
  header=''
  program_headers=''
  dynamic=''
  if ! header="$("$re" -h "$stage/probe" 2>/dev/null)"; then
    printf 'probe ELF header could not be read\n' >&2
    exit 1
  fi
  if ! printf '%s\n' "$header" | grep -q 'Type:.*EXEC'; then
    printf 'probe binary is not ET_EXEC\n' >&2
    exit 1
  fi
  if ! program_headers="$("$re" -l "$stage/probe" 2>/dev/null)"; then
    printf 'probe ELF program headers could not be read\n' >&2
    exit 1
  fi
  if printf '%s\n' "$program_headers" | grep -Eq 'INTERP|DYNAMIC'; then
    printf 'probe binary has an INTERP or DYNAMIC segment\n' >&2
    exit 1
  fi
  if ! dynamic="$("$re" -d "$stage/probe" 2>/dev/null)"; then
    printf 'probe ELF dynamic section could not be read\n' >&2
    exit 1
  fi
  if ! printf '%s\n' "$dynamic" | grep -q 'There is no dynamic section in this file'; then
    printf 'probe binary has a dynamic section\n' >&2
    exit 1
  fi
else
  # macOS fallback: objdump prints ELF program headers via --private-headers.
  od=llvm-objdump
  command -v llvm-objdump >/dev/null 2>&1 || od=objdump
  headers=''
  if ! headers="$("$od" --private-headers "$stage/probe" 2>/dev/null)"; then
    printf 'probe ELF headers could not be read by %s\n' "$od" >&2
    exit 1
  fi
  if ! printf '%s\n' "$headers" | grep -Eiq 'Type:[[:space:]]*EXEC|file type is EXEC|ET_EXEC'; then
    printf 'probe binary is not ET_EXEC\n' >&2
    exit 1
  fi
  if printf '%s\n' "$headers" | grep -Eiq 'INTERP|DYNAMIC'; then
    printf 'probe binary has an INTERP or DYNAMIC segment\n' >&2
    exit 1
  fi
fi

cp "$stage/probe" "$ROOT/dist/ohos-cgo-probe-arm64" 2>/dev/null || {
  mkdir -p "$ROOT/dist"
  cp "$stage/probe" "$ROOT/dist/ohos-cgo-probe-arm64"
}
printf 'OpenHarmony CGO probe linked statically: %s\n' "$ROOT/dist/ohos-cgo-probe-arm64"
printf 'Copy it to a HarmonyOS PC and run it to finish the probe.\n'
