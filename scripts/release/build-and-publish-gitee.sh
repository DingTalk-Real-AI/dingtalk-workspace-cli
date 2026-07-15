#!/usr/bin/env bash
# Build release artifacts locally in the current runner and publish them to Gitee.

set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
SCRIPT_ROOT="$ROOT"
cd "$SCRIPT_ROOT"

VERSION="${VERSION:-$(git describe --tags --exact-match 2>/dev/null || true)}"
[ -n "$VERSION" ] || {
  echo "error: VERSION is required or HEAD must be an exact release tag" >&2
  exit 1
}
case "$VERSION" in
  v*) TAG="$VERSION"; SEMVER="${VERSION#v}" ;;
  *) TAG="v$VERSION"; SEMVER="$VERSION" ;;
esac

git fetch --tags origin "refs/tags/${TAG}:refs/tags/${TAG}" >/dev/null 2>&1 || true
target_commit="$(git rev-parse "${TAG}^{commit}" 2>/dev/null || true)"
[ -n "$target_commit" ] || {
  echo "error: could not resolve tag ${TAG}" >&2
  exit 1
}

current_commit="$(git rev-parse HEAD)"
WORKDIR="$SCRIPT_ROOT"
cleanup_worktree() {
  if [ "$WORKDIR" != "$SCRIPT_ROOT" ]; then
    git -C "$SCRIPT_ROOT" worktree remove --force "$WORKDIR" >/dev/null 2>&1 || rm -rf "$WORKDIR"
  fi
}
trap cleanup_worktree EXIT

if [ "$current_commit" != "$target_commit" ]; then
  WORKDIR="$(mktemp -d)"
  rm -rf "$WORKDIR"
  git worktree add --detach "$WORKDIR" "$TAG"
  mkdir -p "$WORKDIR/scripts/release"
  for release_script in publish-gitee-local.sh reconcile-gitee-assets.sh; do
    cp "$SCRIPT_ROOT/scripts/release/$release_script" "$WORKDIR/scripts/release/$release_script"
    chmod +x "$WORKDIR/scripts/release/$release_script"
  done
fi

cd "$WORKDIR"

echo "==> Building ${TAG} locally for Gitee"
VERSION="$SEMVER" ./scripts/dev/build-all.sh
DWS_PACKAGE_VERSION="$TAG" ./scripts/release/post-goreleaser.sh
VERSION="$TAG" ./scripts/release/publish-gitee-local.sh
