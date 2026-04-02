#!/bin/sh
set -eu

# post-goreleaser.sh — Post-build packaging for npm and Homebrew.
#
# Run after `goreleaser release` or `goreleaser release --snapshot` to stage
# the npm package and render the Homebrew formula from goreleaser's dist/ output.

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
DIST_DIR="${DWS_PACKAGE_DIST_DIR:-$ROOT/dist}"
PACKAGE_VERSION="${DWS_PACKAGE_VERSION:-}"
RELEASE_BASE_URL="${DWS_RELEASE_BASE_URL:-}"

export LANG=C
export LC_ALL=C
export LC_CTYPE=C

say() {
  printf '%s\n' "$*"
}

err() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

sha256_file() {
  target="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$target" | awk '{print $1}'
    return
  fi
  shasum -a 256 "$target" | awk '{print $1}'
}

detect_os() {
  os="$(uname -s)"
  case "$os" in
    Linux*) printf 'linux\n' ;;
    Darwin*) printf 'darwin\n' ;;
    MINGW*|MSYS*|CYGWIN*) printf 'windows\n' ;;
    *) err "unsupported OS: $os" ;;
  esac
}

detect_arch() {
  arch="$(uname -m)"
  case "$arch" in
    x86_64|amd64) printf 'amd64\n' ;;
    arm64|aarch64) printf 'arm64\n' ;;
    *) err "unsupported architecture: $arch" ;;
  esac
}

resolve_version() {
  # Priority 1: Use DWS_PACKAGE_VERSION environment variable (set by CI)
  if [ -n "$PACKAGE_VERSION" ]; then
    # Strip leading 'v' if present for semver compatibility
    printf '%s\n' "$PACKAGE_VERSION" | sed 's/^v//'
    return
  fi

  # Priority 2: Get version from git tag (for local snapshot builds with tag)
  if git describe --tags --exact-match HEAD >/dev/null 2>&1; then
    git describe --tags --exact-match HEAD | sed 's/^v//'
    return
  fi

  # Priority 3: Read from version.go (for local development without tag)
  version_line="$(sed -n 's/^var version = "v\{0,1\}\([^"]*\)".*/\1/p' "$ROOT/internal/app/version.go" | head -1)"
  if [ -z "$version_line" ] || [ "$version_line" = "dev" ]; then
    err "could not resolve package version - set DWS_PACKAGE_VERSION or create a git tag"
  fi
  printf '%s\n' "$version_line"
}

resolve_release_base_url() {
  version="$1"
  if [ -n "$RELEASE_BASE_URL" ]; then
    printf '%s\n' "${RELEASE_BASE_URL%/}"
    return
  fi
  printf 'https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v%s\n' "$version"
}

# ---------- npm staging ----------

stage_npm_package() {
  version="$1"
  pkg_root="$DIST_DIR/npm/dingtalk-workspace-cli"

  rm -rf "$pkg_root"
  mkdir -p "$pkg_root/assets" "$pkg_root/bin"

  cp "$ROOT/build/npm/install.js" "$pkg_root/install.js"
  cp "$ROOT/build/npm/bin/dws.js" "$pkg_root/bin/dws.js"
  cp "$ROOT/build/npm/README.md" "$pkg_root/README.md"
  sed "s|__VERSION__|$version|g" "$ROOT/build/npm/package.json.tmpl" > "$pkg_root/package.json"

  for artifact in "$DIST_DIR"/dws-*.tar.gz "$DIST_DIR"/dws-*.zip "$DIST_DIR"/dws-skills.zip; do
    if [ -f "$artifact" ]; then
      cp "$artifact" "$pkg_root/assets/"
    fi
  done
}

# ---------- Homebrew formula staging ----------

render_homebrew_formula() {
  class_name="$1"
  archive_url="$2"
  skills_url="$3"
  archive_sha="$4"
  skills_sha="$5"
  keg_only_line="$6"
  output_path="$7"

  sed \
    -e "s|__CLASS_NAME__|$class_name|g" \
    -e "s|__ARCHIVE_URL__|$archive_url|g" \
    -e "s|__ARCHIVE_SHA256__|$archive_sha|g" \
    -e "s|__SKILLS_URL__|$skills_url|g" \
    -e "s|__SKILLS_SHA256__|$skills_sha|g" \
    -e "s|__KEG_ONLY_LINE__|$keg_only_line|g" \
    "$ROOT/build/homebrew.rb.tmpl" > "$output_path"
}

stage_homebrew_formula() {
  version="$1"
  host_os="$(detect_os)"
  host_arch="$(detect_arch)"
  archive_ext=".tar.gz"
  formula_dir="$DIST_DIR/homebrew"
  archive_path="$DIST_DIR/dws-${host_os}-${host_arch}${archive_ext}"
  release_url_base="$(resolve_release_base_url "$version")"
  archive_name="$(basename "$archive_path")"
  skills_name="$(basename "$DIST_DIR/dws-skills.zip")"
  archive_sha="$(sha256_file "$archive_path")"
  skills_sha="$(sha256_file "$DIST_DIR/dws-skills.zip")"

  mkdir -p "$formula_dir"

  if [ ! -f "$archive_path" ]; then
    err "host archive missing for homebrew formula: $archive_path"
  fi

  render_homebrew_formula \
    "DingtalkWorkspaceCliLocal" \
    "file://$archive_path" \
    "file://$DIST_DIR/dws-skills.zip" \
    "$archive_sha" \
    "$skills_sha" \
    '  keg_only "Local verification formula to avoid linking conflicts"' \
    "$formula_dir/dingtalk-workspace-cli-local.rb"

  render_homebrew_formula \
    "DingtalkWorkspaceCli" \
    "$release_url_base/$archive_name" \
    "$release_url_base/$skills_name" \
    "$archive_sha" \
    "$skills_sha" \
    "" \
    "$formula_dir/dingtalk-workspace-cli.rb"
}

# ---------- skills zip ----------

create_skills_zip() {
  skills_zip="$DIST_DIR/dws-skills.zip"
  rm -f "$skills_zip"
  (
    cd "$ROOT/skills"
    env -u LC_ALL -u LC_CTYPE LANG=C LC_ALL=C LC_CTYPE=C zip -qr "$skills_zip" .
  )
}

write_checksums() {
  checksum_path="$DIST_DIR/checksums.txt"
  # Append skills zip checksum to goreleaser's checksums file
  if [ -f "$DIST_DIR/dws-skills.zip" ]; then
    printf '%s  %s\n' "$(sha256_file "$DIST_DIR/dws-skills.zip")" "dws-skills.zip" >> "$checksum_path"
  fi
}

# ---------- main ----------

version="$(resolve_version)"

say "==> Creating skills zip"
create_skills_zip

say "==> Updating checksums"
write_checksums

say "==> Staging npm package (v$version)"
stage_npm_package "$version"

say "==> Rendering Homebrew formula (v$version)"
stage_homebrew_formula "$version"

say ""
say "Post-goreleaser packaging complete:"
say "  skills: $DIST_DIR/dws-skills.zip"
say "  npm:     $DIST_DIR/npm/dingtalk-workspace-cli/"
say "  homebrew: $DIST_DIR/homebrew/"
