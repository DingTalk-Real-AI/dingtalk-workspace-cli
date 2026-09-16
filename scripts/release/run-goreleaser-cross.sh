#!/usr/bin/env bash
set -euo pipefail

ROOT="$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)"
CROSS_IMAGE="ghcr.io/goreleaser/goreleaser-cross:v1.26.2@sha256:fadba0d4577866eb2588d46ea6b604c73ef45ee55f044acbc17cc49aa435fd04"
GORELEASER_VERSION="2.16.0"
ZIG_VERSION="0.15.2"
TOOL_CACHE="${DWS_RELEASE_TOOL_CACHE:-${XDG_CACHE_HOME:-$HOME/.cache}/dws/release-tools}"

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
    return
  fi
  shasum -a 256 "$1" | awk '{print $1}'
}

# A cache hit is only trusted when the marker records the pinned archive digest
# and the executable still hashes to the digest captured at install time. An
# executable alone proves nothing: a truncated or replaced tool would otherwise
# be reused as if it had been verified.
verify_cached_tool() {
  local cached_dir="$1"
  local cached_exe="$2"
  local want_archive_sha="$3"
  local marker="$1/.dws-verified"
  local recorded_exe_sha
  [ -x "$cached_dir/$cached_exe" ] || return 1
  [ -f "$marker" ] || return 1
  [ "$(sed -n 's/^archive_sha256=//p' "$marker")" = "$want_archive_sha" ] || return 1
  recorded_exe_sha="$(sed -n 's/^exe_sha256=//p' "$marker")"
  [ -n "$recorded_exe_sha" ] || return 1
  [ "$(sha256_file "$cached_dir/$cached_exe")" = "$recorded_exe_sha" ] || return 1
  return 0
}

# Download, verify, and publish one pinned tool.
#
# Everything happens in a staging directory that is renamed into place only
# after the executable verifies, so an interrupted download or extraction can
# never leave a half-installed tool behind. The mkdir lock serialises
# concurrent `make package` runs, which would otherwise extract over each
# other. Remaining arguments are passed to tar after the archive.
install_pinned_tool() {
  local tool_label="$1"
  local tool_dir="$2"
  local tool_exe="$3"
  local archive_url="$4"
  local pinned_sha="$5"
  local tar_args=("${@:6}")
  local lock_dir="$2.lock"
  local lock_attempts=0
  local stage actual_sha

  mkdir -p "$TOOL_CACHE"
  until mkdir "$lock_dir" 2>/dev/null; do
    lock_attempts=$((lock_attempts + 1))
    if [ "$lock_attempts" -ge 300 ]; then
      printf 'timed out waiting for another release to finish installing %s\n' "$tool_label" >&2
      exit 1
    fi
    sleep 1
  done
  trap 'rmdir "$lock_dir" 2>/dev/null || true' EXIT HUP INT TERM

  if verify_cached_tool "$tool_dir" "$tool_exe" "$pinned_sha"; then
    rmdir "$lock_dir"
    trap - EXIT HUP INT TERM
    return 0
  fi

  stage="$(mktemp -d "$TOOL_CACHE/.stage.XXXXXX")"
  trap 'rm -rf "$stage"; rmdir "$lock_dir" 2>/dev/null || true' EXIT HUP INT TERM

  curl -fsSL "$archive_url" -o "$stage/archive"
  actual_sha="$(sha256_file "$stage/archive")"
  if [ "$actual_sha" != "$pinned_sha" ]; then
    printf '%s archive checksum mismatch: got %s, want %s\n' "$tool_label" "$actual_sha" "$pinned_sha" >&2
    exit 1
  fi
  tar -xf "$stage/archive" -C "$stage" "${tar_args[@]}"
  rm -f "$stage/archive"
  chmod 0755 "$stage/$tool_exe"
  if [ ! -x "$stage/$tool_exe" ]; then
    printf '%s archive did not contain an executable %s\n' "$tool_label" "$tool_exe" >&2
    exit 1
  fi
  printf 'archive_sha256=%s\nexe_sha256=%s\n' \
    "$pinned_sha" "$(sha256_file "$stage/$tool_exe")" >"$stage/.dws-verified"

  # Publish by rename so readers see either the previous tool or the complete
  # new one, never a directory that is still being filled.
  rm -rf "$tool_dir"
  mv "$stage" "$tool_dir"
  rmdir "$lock_dir"
  trap - EXIT HUP INT TERM
}

case "$(uname -m)" in
  x86_64|amd64)
    docker_arch="amd64"
    archive_arch="x86_64"
    archive_sha="eaae05b5eba07533bd0f06846b68c808399504784df00c62eb219541fc04e5e2"
    zig_arch="x86_64"
    zig_sha="02aa270f183da276e5b5920b1dac44a63f1a49e55050ebde3aecc9eb82f93239"
    ;;
  arm64|aarch64)
    docker_arch="arm64"
    archive_arch="arm64"
    archive_sha="0102d974373fcdeb77042d1f5897caffa193be36620fdc6c1da43a01ef8e10d3"
    zig_arch="aarch64"
    zig_sha="958ed7d1e00d0ea76590d27666efbf7a932281b3d7ba0c6b01b0ff26498f667f"
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
install_pinned_tool "GoReleaser" "$tool_dir" goreleaser \
  "https://github.com/goreleaser/goreleaser/releases/download/v${GORELEASER_VERSION}/goreleaser_Linux_${archive_arch}.tar.gz" \
  "$archive_sha" \
  -z goreleaser

# Zig supplies a versioned Linux libc sysroot instead of inheriting the
# cross image's (newer) glibc. Keep Darwin and Windows on the image toolchains.
zig_dir="$TOOL_CACHE/zig-$ZIG_VERSION-linux-$docker_arch"
install_pinned_tool "Zig" "$zig_dir" zig \
  "https://ziglang.org/download/$ZIG_VERSION/zig-$zig_arch-linux-$ZIG_VERSION.tar.xz" \
  "$zig_sha" \
  -J --strip-components=1

mounts=(
  --volume "$ROOT:$ROOT"
  --volume "$goreleaser_bin:/usr/local/bin/goreleaser:ro"
  --volume "$zig_dir:/opt/dws-zig:ro"
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

# .goreleaser.yaml resolves CC/CXX per target with
#   CC={{ index .Env (print "CC_" .Os "_" .Arch) }}
# and also declares the twelve CC_/CXX_ entries in the same builds.env list, so a
# direct goreleaser invocation stays self-contained. Reading an earlier entry of
# that list from a later one through .Env depends on an undocumented GoReleaser
# internal, so export the same values into the container's process environment as
# well; the template then resolves from an environment that is unambiguously
# present. TestReleaseCrossCompilerEnvMatchesWrapper pins these to the config.
compiler_env=(
  "CC_darwin_amd64=o64-clang"
  "CXX_darwin_amd64=o64-clang++"
  "CC_darwin_arm64=oa64-clang"
  "CXX_darwin_arm64=oa64-clang++"
  "CC_linux_amd64=/opt/dws-zig/zig cc -target x86_64-linux-gnu.2.17"
  "CXX_linux_amd64=/opt/dws-zig/zig c++ -target x86_64-linux-gnu.2.17"
  "CC_linux_arm64=/opt/dws-zig/zig cc -target aarch64-linux-gnu.2.17"
  "CXX_linux_arm64=/opt/dws-zig/zig c++ -target aarch64-linux-gnu.2.17"
  "CC_windows_amd64=x86_64-w64-mingw32-gcc"
  "CXX_windows_amd64=x86_64-w64-mingw32-g++"
  "CC_windows_arm64=/llvm-mingw/bin/aarch64-w64-mingw32-gcc"
  "CXX_windows_arm64=/llvm-mingw/bin/aarch64-w64-mingw32-g++"
)
for entry in "${compiler_env[@]}"; do
  env_args+=(--env "$entry")
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
