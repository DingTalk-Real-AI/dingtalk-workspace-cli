#!/usr/bin/env bash
set -euo pipefail

ROOT="$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)"
CROSS_IMAGE="ghcr.io/goreleaser/goreleaser-cross:v1.26.2@sha256:fadba0d4577866eb2588d46ea6b604c73ef45ee55f044acbc17cc49aa435fd04"
GORELEASER_VERSION="2.16.0"
TOOL_CACHE="${DWS_RELEASE_TOOL_CACHE:-${XDG_CACHE_HOME:-$HOME/.cache}/dws/release-tools}"

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
    return
  fi
  shasum -a 256 "$1" | awk '{print $1}'
}

case "$(uname -m)" in
  x86_64|amd64)
    docker_arch="amd64"
    archive_arch="x86_64"
    archive_sha="eaae05b5eba07533bd0f06846b68c808399504784df00c62eb219541fc04e5e2"
    ;;
  arm64|aarch64)
    docker_arch="arm64"
    archive_arch="arm64"
    archive_sha="0102d974373fcdeb77042d1f5897caffa193be36620fdc6c1da43a01ef8e10d3"
    ;;
  *)
    printf 'unsupported Docker host architecture: %s\n' "$(uname -m)" >&2
    exit 1
    ;;
esac

command -v curl >/dev/null 2>&1 || {
  printf 'curl is required to install the pinned GoReleaser binary\n' >&2
  exit 1
}
command -v docker >/dev/null 2>&1 || {
  printf 'Docker is required for SafeChat-enabled cross-platform release builds\n' >&2
  exit 1
}

tool_dir="$TOOL_CACHE/goreleaser-$GORELEASER_VERSION-linux-$archive_arch"
goreleaser_bin="$tool_dir/goreleaser"
if [ ! -x "$goreleaser_bin" ]; then
  mkdir -p "$tool_dir"
  archive="$(mktemp "$tool_dir/goreleaser.XXXXXX.tar.gz")"
  trap 'rm -f "$archive"' EXIT HUP INT TERM
  curl -fsSL \
    "https://github.com/goreleaser/goreleaser/releases/download/v${GORELEASER_VERSION}/goreleaser_Linux_${archive_arch}.tar.gz" \
    -o "$archive"
  actual_sha="$(sha256_file "$archive")"
  if [ "$actual_sha" != "$archive_sha" ]; then
    printf 'GoReleaser archive checksum mismatch: got %s, want %s\n' "$actual_sha" "$archive_sha" >&2
    exit 1
  fi
  tar -xzf "$archive" -C "$tool_dir" goreleaser
  chmod 0755 "$goreleaser_bin"
  rm -f "$archive"
  trap - EXIT HUP INT TERM
fi

mounts=(
  --volume "$ROOT:$ROOT"
  --volume "$goreleaser_bin:/usr/local/bin/goreleaser:ro"
)

git_common_dir="$(git -C "$ROOT" rev-parse --path-format=absolute --git-common-dir)"
case "$git_common_dir" in
  "$ROOT"/*) ;;
  *) mounts+=(--volume "$git_common_dir:$git_common_dir") ;;
esac

for arg in "$@"; do
  case "$arg" in
    --release-notes=/*)
      notes_path="${arg#--release-notes=}"
      notes_dir="$(dirname "$notes_path")"
      case "$notes_dir" in
        "$ROOT"|"$ROOT"/*) ;;
        *) mounts+=(--volume "$notes_dir:$notes_dir:ro") ;;
      esac
      ;;
  esac
done

env_args=()
for name in \
  DWS_PACKAGE_VERSION \
  GITHUB_REPOSITORY_OWNER \
  GORELEASER_CURRENT_TAG \
  GORELEASER_PREVIOUS_TAG
do
  if [ "${!name+x}" = x ]; then
    env_args+=(--env "$name")
  fi
done

docker run --rm \
  --platform "linux/$docker_arch" \
  --user "$(id -u):$(id -g)" \
  --entrypoint /usr/local/bin/goreleaser \
  --env HOME=/tmp \
  "${env_args[@]}" \
  "${mounts[@]}" \
  --workdir "$ROOT" \
  "$CROSS_IMAGE" \
  "$@"
