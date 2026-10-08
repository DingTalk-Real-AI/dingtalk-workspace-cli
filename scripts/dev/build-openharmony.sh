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
  printf 'OHOS_GO must point to an executable Go 1.26.7 OpenHarmony toolchain\n' >&2
  exit 2
fi

if ! "$OHOS_GO" tool dist list | grep -Fxq 'openharmony/arm64'; then
  printf '%s does not support openharmony/arm64\n' "$OHOS_GO" >&2
  exit 1
fi

mkdir -p "$(dirname -- "$OUTPUT")"
cd "$ROOT"
version="${DWS_PACKAGE_VERSION:-dev}"
version="v${version#v}"
git_commit="${DWS_GIT_COMMIT:-$(git rev-parse --verify HEAD^{commit})}"
printf '%s\n' "$git_commit" | grep -Eq '^[0-9a-f]{40}$' || {
  printf 'DWS_GIT_COMMIT must be a full lowercase commit SHA\n' >&2
  exit 2
}
git rev-parse --verify "$git_commit^{commit}" >/dev/null || {
  printf 'DWS_GIT_COMMIT does not resolve in this checkout: %s\n' "$git_commit" >&2
  exit 2
}
build_time="${DWS_BUILD_TIME:-$(sh "$ROOT/scripts/build/release-build-time.sh" "$git_commit")}" || {
  printf 'unable to determine a reproducible OpenHarmony build time\n' >&2
  exit 2
}
ldflags="-d -s -w -X github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app.version=$version -X github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app.gitCommit=$git_commit -X github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app.buildTime=$build_time"
GOOS=openharmony GOARCH=arm64 CGO_ENABLED=0 GOTOOLCHAIN=local \
  "$OHOS_GO" build -trimpath -ldflags="$ldflags" -o "$OUTPUT" ./cmd

DWS_EXPECTED_COMMIT="$git_commit" \
  "$ROOT/scripts/dev/verify-openharmony-artifact.sh" "$OUTPUT" "$version"
printf 'Built %s (%s, %s, %s)\n' "$OUTPUT" "$version" "$git_commit" "$build_time"
