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
APPLE_CERTIFICATE_P12="${DWS_APPLE_CERTIFICATE_P12:-}"
APPLE_CERTIFICATE_PASSWORD_FILE="${DWS_APPLE_CERTIFICATE_PASSWORD_FILE:-}"
REQUIRE_DEVELOPER_ID_SIGNING="${DWS_REQUIRE_DEVELOPER_ID_SIGNING:-false}"

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

file_url() {
  path="$1"
  encoded_path="$(printf '%s' "$path" | sed -e 's/%/%25/g' -e 's/ /%20/g' -e 's/#/%23/g')"
  printf 'file://%s\n' "$encoded_path"
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
  DWS_PACKAGE_SOURCE_ROOT="$ROOT" \
    DWS_PACKAGE_DIST_DIR="$DIST_DIR" \
    "$ROOT/scripts/release/stage-npm-package.sh" "$version"
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

render_homebrew_release_formula() {
  class_name="$1"
  version="$2"
  release_url_base="$3"
  darwin_amd64_sha="$4"
  darwin_arm64_sha="$5"
  linux_amd64_sha="$6"
  linux_arm64_sha="$7"
  skills_sha="$8"
  keg_only_line="$9"
  channel_caveat="${10}"
  output_path="${11}"
  description="Automate DingTalk workspace tasks from the terminal"
  if [ "$class_name" = "DingtalkWorkspaceCliBeta" ]; then
    description="$description (beta channel)"
  fi

  sed \
    -e "s|__CLASS_NAME__|$class_name|g" \
    -e "s|__DESCRIPTION__|$description|g" \
    -e "s|__VERSION__|$version|g" \
    -e "s|__DARWIN_AMD64_URL__|$release_url_base/dws-darwin-amd64.tar.gz|g" \
    -e "s|__DARWIN_AMD64_SHA256__|$darwin_amd64_sha|g" \
    -e "s|__DARWIN_ARM64_URL__|$release_url_base/dws-darwin-arm64.tar.gz|g" \
    -e "s|__DARWIN_ARM64_SHA256__|$darwin_arm64_sha|g" \
    -e "s|__LINUX_AMD64_URL__|$release_url_base/dws-linux-amd64.tar.gz|g" \
    -e "s|__LINUX_AMD64_SHA256__|$linux_amd64_sha|g" \
    -e "s|__LINUX_ARM64_URL__|$release_url_base/dws-linux-arm64.tar.gz|g" \
    -e "s|__LINUX_ARM64_SHA256__|$linux_arm64_sha|g" \
    -e "s|__SKILLS_URL__|$release_url_base/dws-skills.zip|g" \
    -e "s|__SKILLS_SHA256__|$skills_sha|g" \
    -e "s|__KEG_ONLY_LINE__|$keg_only_line|g" \
    -e "s|__CHANNEL_CAVEAT__|$channel_caveat|g" \
    "$ROOT/build/homebrew-release.rb.tmpl" > "$output_path"
}

stage_homebrew_formula() {
  version="$1"
  host_os="$(detect_os)"
  host_arch="$(detect_arch)"
  archive_ext=".tar.gz"
  formula_dir="$DIST_DIR/homebrew"
  archive_path="$DIST_DIR/dws-${host_os}-${host_arch}${archive_ext}"
  release_url_base="$(resolve_release_base_url "$version")"
  archive_sha="$(sha256_file "$archive_path")"
  skills_sha="$(sha256_file "$DIST_DIR/dws-skills.zip")"

  darwin_amd64="$DIST_DIR/dws-darwin-amd64.tar.gz"
  darwin_arm64="$DIST_DIR/dws-darwin-arm64.tar.gz"
  linux_amd64="$DIST_DIR/dws-linux-amd64.tar.gz"
  linux_arm64="$DIST_DIR/dws-linux-arm64.tar.gz"

  mkdir -p "$formula_dir"

  if [ ! -f "$archive_path" ]; then
    err "host archive missing for homebrew formula: $archive_path"
  fi
  for release_archive in "$darwin_amd64" "$darwin_arm64" "$linux_amd64" "$linux_arm64"; do
    if [ ! -f "$release_archive" ]; then
      err "release archive missing for Homebrew formula: $release_archive"
    fi
  done

  render_homebrew_formula \
    "DingtalkWorkspaceCliLocal" \
    "$(file_url "$archive_path")" \
    "$(file_url "$DIST_DIR/dws-skills.zip")" \
    "$archive_sha" \
    "$skills_sha" \
    '  keg_only "Local verification formula to avoid linking conflicts"' \
    "$formula_dir/dingtalk-workspace-cli-local.rb"

  formula_class="DingtalkWorkspaceCli"
  formula_path="$formula_dir/dingtalk-workspace-cli.rb"
  keg_only_line=""
  channel_caveat=""
  case "$version" in
    *-*)
      formula_class="DingtalkWorkspaceCliBeta"
      formula_path="$formula_dir/dingtalk-workspace-cli-beta.rb"
      keg_only_line='  keg_only "it is the beta channel and conflicts with dingtalk-workspace-cli"'
      channel_caveat='      This beta is keg-only. Add #{opt_bin} to PATH to use its `dws` binary.'
      ;;
  esac

  render_homebrew_release_formula \
    "$formula_class" \
    "$version" \
    "$release_url_base" \
    "$(sha256_file "$darwin_amd64")" \
    "$(sha256_file "$darwin_arm64")" \
    "$(sha256_file "$linux_amd64")" \
    "$(sha256_file "$linux_arm64")" \
    "$skills_sha" \
    "$keg_only_line" \
    "$channel_caveat" \
    "$formula_path"
}

# ---------- skills zip ----------

create_skills_zip() {
  skills_zip="$DIST_DIR/dws-skills.zip"
  rm -f "$skills_zip"

  staging="$(mktemp -d)"
  # Layout inside dws-skills.zip:
  #   <root>/SKILL.md + references/ + scripts/   ← copy of mono/, kept at root
  #                                                 for backward compatibility
  #                                                 with older install scripts
  #                                                 that look for SKILL.md at
  #                                                 the zip root.
  #   <root>/mono/                               ← explicit mono source tree
  #   <root>/multi/                              ← multi source tree (one
  #                                                 subdir per product skill)
  cp -R "$ROOT/skills/mono/." "$staging/"
  mkdir -p "$staging/mono"
  cp -R "$ROOT/skills/mono/." "$staging/mono/"
  mkdir -p "$staging/multi"
  if [ -d "$ROOT/skills/multi" ]; then
    cp -R "$ROOT/skills/multi/." "$staging/multi/"
  fi

  (
    cd "$staging"
    find . -exec touch -t 202001010000 {} +
    find . -type f | LC_ALL=C sort \
      | env -u LC_ALL -u LC_CTYPE LANG=C LC_ALL=C LC_CTYPE=C zip -X -q "$skills_zip" -@
  )

  rm -rf "$staging"
}

# ---------- runtime payload ----------

repack_platform_archive() {
  stage="$1"
  archive="$2"
  new_archive="$archive.new"
  case "$archive" in
    *.tar.gz)
      deterministic_tar=""
      if command -v gtar >/dev/null 2>&1; then
        deterministic_tar=gtar
      elif tar --version 2>/dev/null | grep -q 'GNU tar'; then
        deterministic_tar=tar
      fi
      if [ -n "$deterministic_tar" ]; then
        (
          cd "$stage" \
            && "$deterministic_tar" --sort=name --owner=0 --group=0 --numeric-owner \
                  --mtime='2020-01-01 00:00Z' \
                  -czf "$archive.new" .
        )
      else
        (
          cd "$stage"
          find . -exec touch -t 202001010000 {} +
          find . -print | LC_ALL=C sort \
            | COPYFILE_DISABLE=1 tar --no-recursion \
                --uid 0 --gid 0 --uname root --gname root \
                --options gzip:!timestamp -czf "$archive.new" -T -
        )
      fi
      ;;
    *.zip)
      new_archive="${archive%.zip}.new.zip"
      (
        cd "$stage"
        find . -exec touch -t 202001010000 {} +
        find . -type f | LC_ALL=C sort \
          | env -u LC_ALL -u LC_CTYPE LANG=C LC_ALL=C LC_CTYPE=C zip -X -q "$new_archive" -@
      )
      ;;
    *) err "unsupported platform archive: $archive" ;;
  esac
  mv "$new_archive" "$archive"
}

write_runtime_manifest() {
  runtime_root="$1"
  target_os="$2"
  target_arch="$3"
  library_name="$4"
  library_sha="$(sha256_file "$runtime_root/$library_name")"
  cat > "$runtime_root/manifest.json" <<EOF
{
  "format_version": 1,
  "payload_version": "20260825",
  "target": "$target_os/$target_arch",
  "library": "$library_name",
  "library_sha256": "$library_sha",
  "ps_file_count": 123,
  "ps_manifest_sha256": "45ae147697c1f8683df3f232d0ba792b807179bbe22fdac8225a0cf25fc33e7e"
}
EOF
}

prepare_runtime_archives() {
  "$ROOT/scripts/policy/check-runtime-payload.sh" --allow-unsupported-tools
  work="$(mktemp -d)"
  found_any=0
  for archive in "$DIST_DIR"/dws-darwin-*.tar.gz "$DIST_DIR"/dws-linux-*.tar.gz "$DIST_DIR"/dws-windows-*.zip; do
    [ -f "$archive" ] || continue
    found_any=1
    name="$(basename "$archive")"
    target="${name#dws-}"
    target="${target%.tar.gz}"
    target="${target%.zip}"
    target_os="${target%-*}"
    target_arch="${target##*-}"
    stage="$work/$target"
    rm -rf "$stage"
    mkdir -p "$stage"
    case "$archive" in
      *.tar.gz) tar -xzf "$archive" -C "$stage" ;;
      *.zip) unzip -q "$archive" -d "$stage" ;;
    esac
    "$ROOT/scripts/build/prepare-runtime-payload.sh" "$target_os" "$target_arch" "$stage"
    repack_platform_archive "$stage" "$archive"
    update_checksum_entry "$name" "$(sha256_file "$archive")"
  done
  rm -rf "$work"
  [ "$found_any" -eq 1 ] || err "no platform archives found for runtime payload preparation"
}

# ---------- darwin signing ----------
#
# Unsigned arm64 binaries are SIGKILL'd by amfid on Apple Silicon (macOS 11+).
# Official releases use an Apple Developer ID certificate loaded from GitHub
# Secrets. Fork/local builds retain ad-hoc signing so they remain runnable.
# We unpack each dws-darwin-*.tar.gz, sign the runtime library and dws binary,
# repack deterministically,
# and rewrite the corresponding line in checksums.txt.

configure_darwin_signing() {
  case "$REQUIRE_DEVELOPER_ID_SIGNING" in
    1|true|yes) require_developer_id=1 ;;
    0|false|no|"") require_developer_id=0 ;;
    *) err "invalid DWS_REQUIRE_DEVELOPER_ID_SIGNING value: $REQUIRE_DEVELOPER_ID_SIGNING" ;;
  esac

  if [ -n "$APPLE_CERTIFICATE_P12" ] || [ -n "$APPLE_CERTIFICATE_PASSWORD_FILE" ]; then
    [ -n "$APPLE_CERTIFICATE_P12" ] || err "DWS_APPLE_CERTIFICATE_P12 is required when Developer ID signing is configured"
    [ -n "$APPLE_CERTIFICATE_PASSWORD_FILE" ] || err "DWS_APPLE_CERTIFICATE_PASSWORD_FILE is required when Developer ID signing is configured"
    [ -f "$APPLE_CERTIFICATE_P12" ] || err "Developer ID P12 file not found: $APPLE_CERTIFICATE_P12"
    [ -f "$APPLE_CERTIFICATE_PASSWORD_FILE" ] || err "Developer ID password file not found: $APPLE_CERTIFICATE_PASSWORD_FILE"
    command -v rcodesign >/dev/null 2>&1 || err "rcodesign is required for Developer ID signing"
    DARWIN_SIGNING_MODE="developer-id"
    return
  fi

  if [ "$require_developer_id" -eq 1 ]; then
    err "Developer ID signing is required but DWS_APPLE_CERTIFICATE_P12 and DWS_APPLE_CERTIFICATE_PASSWORD_FILE are not configured"
  fi
  DARWIN_SIGNING_MODE="ad-hoc"
}

sign_one_darwin_binary() {
  sign_target="$1"
  if [ "$DARWIN_SIGNING_MODE" = "developer-id" ]; then
    rcodesign sign \
      --p12-file "$APPLE_CERTIFICATE_P12" \
      --p12-password-file "$APPLE_CERTIFICATE_PASSWORD_FILE" \
      --for-notarization \
      "$sign_target"
    return
  fi
  if command -v codesign >/dev/null 2>&1; then
    codesign --force --sign - "$sign_target"
    return
  fi
  if command -v rcodesign >/dev/null 2>&1; then
    rcodesign sign "$sign_target"
    return
  fi
  err "neither codesign nor rcodesign found — install rcodesign (cargo install apple-codesign) to ad-hoc sign darwin builds"
}

update_checksum_entry() {
  filename="$1"
  new_sha="$2"
  checksum_path="$DIST_DIR/checksums.txt"
  [ -f "$checksum_path" ] || return 0
  tmp="$(mktemp)"
  grep -v "  ${filename}\$" "$checksum_path" > "$tmp" 2>/dev/null || true
  printf '%s  %s\n' "$new_sha" "$filename" >> "$tmp"
  mv "$tmp" "$checksum_path"
}

sign_darwin_archives() {
  work="$(mktemp -d)"
  found_any=0
  for archive in "$DIST_DIR"/dws-darwin-*.tar.gz; do
    [ -f "$archive" ] || continue
    found_any=1
    name="$(basename "$archive")"
    say "  signing $name"

    stage="$work/${name%.tar.gz}"
    rm -rf "$stage"
    mkdir -p "$stage"
    tar -xzf "$archive" -C "$stage"

    bin="$stage/dws"
    if [ ! -f "$bin" ]; then
      err "dws binary not found inside $name after extraction"
    fi
    runtime_root="$stage/.dws-runtime/20260825"
    runtime_library="$runtime_root/x7k2m9p4q1w8.dylib"
    [ -f "$runtime_library" ] || err "runtime library not found inside $name after extraction"
    sign_one_darwin_binary "$runtime_library"
    target_arch="${name#dws-darwin-}"
    target_arch="${target_arch%.tar.gz}"
    write_runtime_manifest "$runtime_root" darwin "$target_arch" x7k2m9p4q1w8.dylib
    sign_one_darwin_binary "$bin"

    repack_platform_archive "$stage" "$archive"

    update_checksum_entry "$name" "$(sha256_file "$archive")"
  done
  rm -rf "$work"
  if [ "$found_any" -eq 0 ]; then
    say "  (no darwin archives found, skipping)"
  fi
}

write_checksums() {
  # Keep this idempotent: workflow retries must not leave duplicate entries.
  if [ -f "$DIST_DIR/dws-skills.zip" ]; then
    update_checksum_entry "dws-skills.zip" "$(sha256_file "$DIST_DIR/dws-skills.zip")"
  fi
}

# ---------- main ----------

version="$(resolve_version)"
configure_darwin_signing

say "==> Preparing runtime payload"
prepare_runtime_archives

if [ "$DARWIN_SIGNING_MODE" = "developer-id" ]; then
  say "==> Developer ID signing darwin runtime and binaries"
else
  say "==> Ad-hoc signing darwin runtime and binaries"
fi
sign_darwin_archives

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
