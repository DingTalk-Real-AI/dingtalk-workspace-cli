#!/usr/bin/env bash
set -euo pipefail

ROOT="$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)"
OHOS_GO="${OHOS_GO:-}"
OUTPUT="${OHOS_OUTPUT:-$ROOT/dist/dws-openharmony-arm64}"

# setup-env and developer shells may export GOROOT pointing at the official
# runner Go; that override would make the OpenHarmony toolchain resolve
# official build tools that reject the openharmony GOOS, so restore the
# toolchain's own root before any go invocation.
unset GOROOT

if [ -z "$OHOS_GO" ] || [ ! -x "$OHOS_GO" ]; then
  printf 'OHOS_GO not provided; provisioning the pinned OpenHarmony toolchain\n' >&2
  OHOS_GO="$(bash "$ROOT/scripts/dev/provision-ohos-go.sh")"
fi

if ! "$OHOS_GO" tool dist list | grep -Fx 'openharmony/arm64' >/dev/null; then
  printf '%s does not support openharmony/arm64\n' "$OHOS_GO" >&2
  exit 1
fi

mkdir -p "$(dirname -- "$OUTPUT")"
cd "$ROOT"
version="${DWS_PACKAGE_VERSION:-0.0.0-test}"
version="v${version#v}"
git_commit="$(git rev-parse --verify HEAD^{commit})"
build_time="$(sh "$ROOT/scripts/build/release-build-time.sh" "$git_commit")"
ldflags="-s -w -X github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app.version=$version -X github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app.gitCommit=$git_commit -X github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app.buildTime=$build_time"
# ohos-go defaults CC to a bare `clang` with no sysroot injection, so the
# OHOS NDK cross compiler must be resolved explicitly for CGO builds.
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

cgo_mode="${DWS_OPENHARMONY_CGO:-auto}"
case "$cgo_mode" in
  auto|0|1) ;;
  *)
    printf 'DWS_OPENHARMONY_CGO must be auto, 0, or 1\n' >&2
    exit 2
    ;;
esac

# When CGO is enabled (1) or auto, if compiler is not set, attempt provisioning
# the pinned OpenHarmony NDK when allowed or requested.
if [ "$cgo_mode" = 1 ] || [ "$cgo_mode" = auto ]; then
  if [ -z "$ohos_cc" ]; then
    if [ "${DWS_PROVISION_OHOS_NDK:-1}" = 1 ]; then
      printf 'OHOS_CC/OHOS_NDK not provided; provisioning the pinned OpenHarmony NDK\n' >&2
      if OHOS_NDK="$(bash "$ROOT/scripts/dev/provision-ohos-ndk.sh")"; then
        for candidate in \
          "$OHOS_NDK/native/llvm/bin/aarch64-unknown-linux-ohos-clang" \
          "$OHOS_NDK/llvm/bin/aarch64-unknown-linux-ohos-clang" \
          "$OHOS_NDK"/toolchains/llvm/prebuilt/*/bin/aarch64-unknown-linux-ohos-clang; do
          if [ -x "$candidate" ]; then
            ohos_cc="$candidate"
            break
          fi
        done
      else
        if [ "$cgo_mode" = auto ]; then
          printf 'OpenHarmony NDK provisioning failed; auto mode will continue without CGO\n' >&2
        else
          printf 'OpenHarmony NDK provisioning failed; forced CGO remains enabled\n' >&2
        fi
      fi
    fi
  fi
  if [ "$cgo_mode" = auto ]; then
    if [ -n "$ohos_cc" ]; then cgo_mode=1; else cgo_mode=0; fi
  fi
fi

build_tags=()
if [ "$cgo_mode" = 1 ]; then
  [ -n "$ohos_cc" ] || {
    printf 'CGO build requires OHOS_CC or OHOS_NDK with an aarch64-unknown-linux-ohos clang\n' >&2
    exit 2
  }
  export CC="$ohos_cc"
  export CXX="${ohos_cc}++"
  # Do NOT add the netgo tag here: the ohos-go net port's interface table
  # (interface_table_openharmony.go) depends on cgo helpers defined in
  # cgo_unix_cgo.go, which netgo excludes.
  # -static is what keeps the Huawei-reviewed "musl-free static ELF" shape:
  # no PT_INTERP, no PT_DYNAMIC, no NEEDED, with musl libc.a linked in.
  # No -d here: it forbids the dynamic imports cgo needs; external linking
  # with -static owns the final layout.
  ldflags="$ldflags -extldflags -static"
  printf 'OpenHarmony CGO build (SafeChat crypto enabled) via %s\n' "$ohos_cc" >&2
else
  # -d suppresses the dynamic loader format that go1.26 would otherwise
  # emit; pure-Go static builds need it to stay PT_INTERP-free.
  ldflags="-d $ldflags"
  printf 'OpenHarmony non-CGO build (SafeChat crypto stub; encrypted messages return as-is)\n' >&2
fi

GOOS=openharmony GOARCH=arm64 CGO_ENABLED=$cgo_mode GOTOOLCHAIN=local \
  "$OHOS_GO" build -trimpath ${build_tags[@]+"${build_tags[@]}"} -ldflags="$ldflags" -o "$OUTPUT" ./cmd

printf 'Built %s\n' "$OUTPUT"
