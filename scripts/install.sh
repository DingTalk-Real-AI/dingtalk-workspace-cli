#!/bin/sh
# Copyright 2026 Alibaba Group
# Licensed under the Apache License, Version 2.0
#
# Installer for dws (DingTalk Workspace CLI).
# Downloads the pre-built binary from GitHub Releases and installs agent skills.
# No Go, Node.js, or other dependencies required.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/DingTalk-Real-AI/dingtalk-workspace-cli/main/scripts/install.sh | sh
#
# Environment variables (all optional):
#   DWS_INSTALL_DIR   — where to put the binary       (default: ~/.local/bin)
#   DWS_VERSION       — version to install             (default: latest)
#   DWS_NO_SKILLS     — set to 1 to skip skills install
#   DWS_SKILLS_ONLY   — set to 1 to install only skills (skip binary)

set -eu

REPO="DingTalk-Real-AI/dingtalk-workspace-cli"
BIN_NAME="dws"
INSTALL_DIR="${DWS_INSTALL_DIR:-$HOME/.local/bin}"
INSTALL_NAME="${DWS_INSTALL_NAME:-$BIN_NAME}"
VERSION="${DWS_VERSION:-latest}"
NO_SKILLS="${DWS_NO_SKILLS:-0}"
SKILLS_ONLY="${DWS_SKILLS_ONLY:-0}"
SKILL_NAME="dws"

# ── Agent directory to install skills into ───────────────────────────────────
# Only install to .agents/skills — most agents can fall back to this directory.
AGENT_DIR=".agents/skills"

# ── Helpers ──────────────────────────────────────────────────────────────────

say() {
  printf '  %s\n' "$@"
}

err() {
  printf '  ❌ %s\n' "$@" >&2
  exit 1
}

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    return 1
  fi
  return 0
}

resolve_source_root() {
  script_path="$0"
  if [ ! -f "$script_path" ]; then
    return 1
  fi

  script_dir="$(CDPATH= cd -- "$(dirname -- "$script_path")" && pwd)"
  candidate_root="$(CDPATH= cd -- "$script_dir/.." && pwd)"
  if [ -f "$candidate_root/go.mod" ] && [ -d "$candidate_root/cmd" ]; then
    printf '%s\n' "$candidate_root"
    return 0
  fi

  return 1
}

# Download a URL to a file. Uses curl or wget, whichever is available.
download() {
  url="$1"
  dest="$2"
  if need_cmd curl; then
    curl -fsSL "$url" -o "$dest"
  elif need_cmd wget; then
    wget -qO "$dest" "$url"
  else
    return 1
  fi
}

extract_zip() {
  archive="$1"
  dest="$2"
  if need_cmd unzip; then
    unzip -q "$archive" -d "$dest"
    return 0
  fi
  if need_cmd tar && tar -xf "$archive" -C "$dest" >/dev/null 2>&1; then
    return 0
  fi
  return 1
}

# Detect OS
detect_os() {
  os="$(uname -s)"
  case "$os" in
    Linux*)  echo "linux" ;;
    Darwin*) echo "darwin" ;;
    MINGW*|MSYS*|CYGWIN*) echo "windows" ;;
    *) err "Unsupported OS: $os. Use the PowerShell installer on Windows." ;;
  esac
}

# Detect architecture
detect_arch() {
  arch="$(uname -m)"
  case "$arch" in
    x86_64|amd64)  echo "amd64" ;;
    arm64|aarch64) echo "arm64" ;;
    *) err "Unsupported architecture: $arch" ;;
  esac
}

# Resolve the latest version tag from GitHub
resolve_version() {
  if [ "$VERSION" = "latest" ]; then
    # Follow the redirect from /releases/latest to get the tag
    if need_cmd curl; then
      VERSION="$(curl -fsSI "https://github.com/${REPO}/releases/latest" 2>/dev/null \
        | grep -i '^location:' | sed 's|.*/tag/||;s/[[:space:]]*$//')"
    elif need_cmd wget; then
      VERSION="$(wget --spider --max-redirect=0 "https://github.com/${REPO}/releases/latest" 2>&1 \
        | grep -i 'Location:' | sed 's|.*/tag/||;s/[[:space:]]*$//')"
    else
      return 1
    fi
    if [ -z "$VERSION" ]; then
      return 1
    fi
  fi
  return 0
}

# ── Banner ───────────────────────────────────────────────────────────────────

print_banner() {
  printf '\n'
  say "┌──────────────────────────────────────┐"
  say "│     DWS Installer                    │"
  say "│     DingTalk Workspace CLI            │"
  say "└──────────────────────────────────────┘"
  printf '\n'
}

clone_source_checkout() {
  clone_url="https://github.com/${REPO}.git"

  if ! need_cmd git; then
    say "⚠️  Missing required command: git; cannot clone source checkout."
    CLONED_SOURCE_TMPDIR=""
    CLONED_SOURCE_ROOT=""
    return 1
  fi

  CLONED_SOURCE_TMPDIR="$(mktemp -d)"
  CLONED_SOURCE_ROOT="${CLONED_SOURCE_TMPDIR}/repo"

  say "Cloning source checkout from ${clone_url}"

  if [ "$VERSION" = "latest" ]; then
    if ! git clone --depth 1 "$clone_url" "$CLONED_SOURCE_ROOT" >/dev/null 2>&1; then
      rm -rf "$CLONED_SOURCE_TMPDIR"
      CLONED_SOURCE_TMPDIR=""
      CLONED_SOURCE_ROOT=""
      say "⚠️  Could not clone source checkout from ${clone_url}."
      return 1
    fi
    return 0
  fi

  if ! git clone --depth 1 --branch "$VERSION" "$clone_url" "$CLONED_SOURCE_ROOT" >/dev/null 2>&1; then
    rm -rf "$CLONED_SOURCE_TMPDIR"
    CLONED_SOURCE_TMPDIR=""
    CLONED_SOURCE_ROOT=""
    say "⚠️  Could not clone source checkout ${clone_url} at ref ${VERSION}."
    return 1
  fi
}

cleanup_cloned_source() {
  if [ -n "${CLONED_SOURCE_TMPDIR:-}" ] && [ -d "$CLONED_SOURCE_TMPDIR" ]; then
    rm -rf "$CLONED_SOURCE_TMPDIR"
  fi
  CLONED_SOURCE_TMPDIR=""
  CLONED_SOURCE_ROOT=""
}

acquire_source_checkout() {
  # 1. Prefer an already-resolved local source root (set in main).
  if [ -n "${SOURCE_ROOT:-}" ]; then
    ACQUIRED_SOURCE_ROOT="$SOURCE_ROOT"
    say "Using local source checkout: ${ACQUIRED_SOURCE_ROOT}"
    return 0
  fi

  # 2. Try to resolve a local source root from the script location.
  _resolved="$(resolve_source_root || true)"
  if [ -n "$_resolved" ]; then
    ACQUIRED_SOURCE_ROOT="$_resolved"
    say "Using local source checkout: ${ACQUIRED_SOURCE_ROOT}"
    return 0
  fi

  # 3. Reuse an already-cloned checkout.
  if [ -n "${CLONED_SOURCE_ROOT:-}" ] && [ -d "$CLONED_SOURCE_ROOT" ]; then
    ACQUIRED_SOURCE_ROOT="$CLONED_SOURCE_ROOT"
    return 0
  fi

  # 4. Clone from GitHub (non-fatal on failure).
  if clone_source_checkout; then
    ACQUIRED_SOURCE_ROOT="$CLONED_SOURCE_ROOT"
    return 0
  fi

  ACQUIRED_SOURCE_ROOT=""
  return 1
}

install_binary_from_source() {
  root="$1"

  need_cmd go || err "Missing required command: go"
  need_cmd make || err "Missing required command: make"

  say "Installing dws from source checkout: ${root}"
  say "Install dir: ${INSTALL_DIR}"

  # Build using make (produces ./dws in the project root)
  make -C "$root" build

  built_bin="$root/$BIN_NAME"
  if [ ! -f "$built_bin" ]; then
    err "make build did not produce ${built_bin}"
  fi

  mkdir -p "$INSTALL_DIR"
  cp "$built_bin" "$INSTALL_DIR/$INSTALL_NAME"
  chmod +x "$INSTALL_DIR/$INSTALL_NAME"

  say "✅ Binary installed:"
  say "   → ${INSTALL_DIR}/${INSTALL_NAME}"
}

# ── Install Skills from Local Source ─────────────────────────────────────────

install_skills_local() {
  root="$1"
  skill_src="${root}/skills"

  if [ ! -d "$skill_src" ]; then
    say "⚠️  Local skills directory not found: ${skill_src}"
    say "   Skipping skills installation."
    return 1
  fi

  say ""
  say "📦 Installing agent skills from local source: ${skill_src}"

  dest="$HOME/$AGENT_DIR/$SKILL_NAME"
  display_path="~/$AGENT_DIR/$SKILL_NAME"
  _copy_skill "$skill_src" "$dest" "$display_path"

  return 0
}

# Helper: copy skill files to a destination and print details
_copy_skill() {
  _src="$1"
  _dest="$2"
  _label="$3"

  if [ -d "$_dest" ]; then
    rm -rf "$_dest"
  fi

  mkdir -p "$_dest"
  cp -R "$_src/"* "$_dest/" 2>/dev/null || cp -r "$_src/"* "$_dest/"
  file_count="$(find "$_dest" -type f | wc -l | tr -d ' ')"

  say "✅ Skills → ${_label} (${file_count} files)"

  for entry in "$_dest"/*; do
    entry_name="$(basename "$entry")"
    if [ -d "$entry" ]; then
      sub_count="$(find "$entry" -type f | wc -l | tr -d ' ')"
      say "   📁 ${entry_name}/ (${sub_count} files)"
    else
      say "   📄 ${entry_name}"
    fi
  done
}

# ── Install Binary ───────────────────────────────────────────────────────────

install_binary() {
  os="$(detect_os)"
  arch="$(detect_arch)"
  if ! resolve_version; then
    say "⚠️  Could not determine the latest release version."
    if acquire_source_checkout; then
      install_binary_from_source "$ACQUIRED_SOURCE_ROOT"
    else
      say "⚠️  No source checkout available; skipping binary install."
    fi
    return 0
  fi

  archive_name="${BIN_NAME}-${os}-${arch}.tar.gz"
  download_url="https://github.com/${REPO}/releases/download/${VERSION}/${archive_name}"

  say "⬇  Downloading ${BIN_NAME} ${VERSION} (${os}/${arch})..."

  tmpdir="$(mktemp -d)"
  trap 'rm -rf "$tmpdir"' EXIT INT TERM

  if ! download "$download_url" "$tmpdir/$archive_name"; then
    rm -rf "$tmpdir"
    say "⚠️  Release binary download failed."
    if acquire_source_checkout; then
      install_binary_from_source "$ACQUIRED_SOURCE_ROOT"
    else
      say "⚠️  No source checkout available; skipping binary install."
    fi
    return 0
  fi

  # Download and verify SHA256 checksum
  checksum_url="https://github.com/${REPO}/releases/download/${VERSION}/checksums.txt"
  if download "$checksum_url" "$tmpdir/checksums.txt" 2>/dev/null; then
    expected="$(awk -v file="$archive_name" '$2 == file {print $1; exit}' "$tmpdir/checksums.txt")"
    if [ -n "$expected" ]; then
      if need_cmd sha256sum; then
        actual="$(sha256sum "$tmpdir/$archive_name" | awk '{print $1}')"
      elif need_cmd shasum; then
        actual="$(shasum -a 256 "$tmpdir/$archive_name" | awk '{print $1}')"
      else
        actual=""
      fi
      if [ -n "$actual" ] && [ "$actual" != "$expected" ]; then
        err "SHA256 checksum mismatch! Expected ${expected}, got ${actual}. Aborting."
      fi
      if [ -n "$actual" ]; then
        say "✅ SHA256 checksum verified"
      else
        say "⚠️  Could not compute checksum (sha256sum/shasum not found); skipping verification"
      fi
    else
      say "⚠️  Archive not found in checksums.txt; skipping verification"
    fi
  else
    say "⚠️  Could not download checksums.txt; skipping verification"
  fi

  say "📦 Extracting..."
  tar xzf "$tmpdir/$archive_name" -C "$tmpdir"

  mkdir -p "$INSTALL_DIR"

  # The archive may contain a top-level directory or just the binary
  if [ -f "$tmpdir/$BIN_NAME" ]; then
    cp "$tmpdir/$BIN_NAME" "$INSTALL_DIR/$INSTALL_NAME"
  elif [ -f "$tmpdir/${BIN_NAME}-${os}-${arch}/$BIN_NAME" ]; then
    cp "$tmpdir/${BIN_NAME}-${os}-${arch}/$BIN_NAME" "$INSTALL_DIR/$INSTALL_NAME"
  else
    # Search for the binary
    found="$(find "$tmpdir" -name "$BIN_NAME" -type f | head -1)"
    if [ -n "$found" ]; then
      cp "$found" "$INSTALL_DIR/$INSTALL_NAME"
    else
      err "Could not find the ${BIN_NAME} binary in the downloaded archive."
    fi
  fi

  chmod +x "$INSTALL_DIR/$INSTALL_NAME"

  say "✅ Binary installed: ${INSTALL_DIR}/${INSTALL_NAME}"

  # Check if install dir is in PATH
  case ":$PATH:" in
    *":$INSTALL_DIR:"*) ;;
    *)
      say ""
      say "⚠️  ${INSTALL_DIR} is not in your PATH."
      say "   Add it with:"
      say "     export PATH=\"${INSTALL_DIR}:\$PATH\""
      say "   Or add this line to your ~/.bashrc / ~/.zshrc"
      ;;
  esac
}

# ── Install Skills ───────────────────────────────────────────────────────────

install_skills() {
  say ""
  say "📦 Installing agent skills from GitHub Releases..."

  if ! resolve_version; then
    say "⚠️  Could not determine the latest release version."
    if acquire_source_checkout; then
      install_skills_local "$ACQUIRED_SOURCE_ROOT"
    else
      say "⚠️  No source checkout available; skipping skills install."
    fi
    return 0
  fi
  skills_archive="dws-skills.zip"
  download_url="https://github.com/${REPO}/releases/download/${VERSION}/${skills_archive}"

  tmpdir_skills="$(mktemp -d)"
  trap 'rm -rf "$tmpdir_skills"' EXIT INT TERM

  if ! download "$download_url" "$tmpdir_skills/$skills_archive" 2>/dev/null; then
    say "⚠️  Release asset download failed."
    rm -rf "$tmpdir_skills"
    if acquire_source_checkout; then
      install_skills_local "$ACQUIRED_SOURCE_ROOT"
    else
      say "⚠️  No source checkout available; skipping skills install."
    fi
    return 0
  fi

  extract_root="$tmpdir_skills/skills"
  mkdir -p "$extract_root"
  if ! extract_zip "$tmpdir_skills/$skills_archive" "$extract_root" 2>/dev/null; then
    say "⚠️  Could not extract release skill archive."
    rm -rf "$tmpdir_skills"
    acquire_source_checkout
    install_skills_local "$ACQUIRED_SOURCE_ROOT"
    return 0
  fi

  skill_src="$extract_root"
  if [ -f "$extract_root/$SKILL_NAME/SKILL.md" ]; then
    skill_src="$extract_root/$SKILL_NAME"
  fi
  if [ ! -f "$skill_src/SKILL.md" ]; then
    say "⚠️  Skills not found in release asset."
    rm -rf "$tmpdir_skills"
    acquire_source_checkout
    install_skills_local "$ACQUIRED_SOURCE_ROOT"
    return 0
  fi

  dest="$HOME/$AGENT_DIR/$SKILL_NAME"
  display_path="~/$AGENT_DIR/$SKILL_NAME"
  _copy_skill "$skill_src" "$dest" "$display_path"

  rm -rf "$tmpdir_skills"
}

# ── Main ─────────────────────────────────────────────────────────────────────

main() {
  SOURCE_ROOT="$(resolve_source_root || true)"

  print_banner

  if [ "$SKILLS_ONLY" = "1" ]; then
    install_skills
  elif [ "$NO_SKILLS" = "1" ]; then
    install_binary
  else
    install_binary
    install_skills
  fi

  cleanup_cloned_source

  printf '\n'
  say "🎉 Installation complete!"
  say ""
  say "Next steps:"
  if [ "$SKILLS_ONLY" != "1" ]; then
    say "  ${BIN_NAME} version          # verify installation"
    say "  ${BIN_NAME} auth login       # authenticate with DingTalk"
  fi
  say "  ${BIN_NAME} --help           # explore commands"
  printf '\n'
}

main
