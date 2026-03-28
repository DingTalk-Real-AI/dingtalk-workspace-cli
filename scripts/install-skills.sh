#!/bin/sh
set -eu

# Install DWS agent skills from GitHub into detected agent directories.
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/DingTalk-Real-AI/dingtalk-workspace-cli/main/scripts/install-skills.sh | sh
#
# The script downloads the dws-skills.zip release asset from GitHub Releases
# and copies it into every detected agent skills directory in the current
# project. If the release asset is unavailable, it clones the repository and
# installs skills from the cloned source tree.

REPO="DingTalk-Real-AI/dingtalk-workspace-cli"
VERSION="${DWS_VERSION:-latest}"
SKILL_NAME="dws"

# ── Agent directory to install skills into ───────────────────────────────────
# Only install to .agents/skills — most agents can fall back to this directory.
AGENT_DIR=".agents/skills"

# ── Helpers ──────────────────────────────────────────────────────────────────

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    return 1
  fi
}

resolve_source_root() {
  script_path="$0"
  if [ ! -f "$script_path" ]; then
    return 1
  fi

  script_dir="$(CDPATH= cd -- "$(dirname -- "$script_path")" && pwd)"
  candidate_root="$(CDPATH= cd -- "$script_dir/.." && pwd)"
  if [ -f "$candidate_root/skills/SKILL.md" ]; then
    printf '%s\n' "$candidate_root"
    return 0
  fi

  return 1
}

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

resolve_version() {
  if [ "$VERSION" = "latest" ]; then
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

extract_zip() {
  archive="$1"
  dest="$2"
  if command -v unzip >/dev/null 2>&1; then
    unzip -q "$archive" -d "$dest"
    return 0
  fi
  if command -v tar >/dev/null 2>&1 && tar -xf "$archive" -C "$dest" >/dev/null 2>&1; then
    return 0
  fi
  printf '❌ Missing required command: unzip (or tar with zip support)\n' >&2
  exit 1
}

install_skills_local() {
  root="$1"
  cwd="$2"
  skill_src="${root}/skills"

  if [ ! -f "$skill_src/SKILL.md" ]; then
    printf '  ❌ Local skill source not found: %s\n' "$skill_src" >&2
    exit 1
  fi

  printf '  📦 Installing agent skills from local source: %s\n' "$skill_src"

  dest="$cwd/$AGENT_DIR/$SKILL_NAME"
  if [ -d "$dest" ]; then
    rm -rf "$dest"
  fi

  mkdir -p "$dest"
  cp -R "$skill_src/"* "$dest/"
  file_count="$(find "$dest" -type f | wc -l | tr -d ' ')"

  printf '  ✅ Universal (.agents)\n'
  printf '     → %s/%s (%s files)\n' "$AGENT_DIR" "$SKILL_NAME" "$file_count"
}

clone_source_checkout() {
  clone_url="https://github.com/${REPO}.git"

  if ! need_cmd git; then
    printf '  ⚠️  Missing required command: git; cannot clone source checkout.\n'
    CLONED_SOURCE_TMPDIR=""
    CLONED_SOURCE_ROOT=""
    return 1
  fi

  CLONED_SOURCE_TMPDIR="$(mktemp -d)"
  CLONED_SOURCE_ROOT="${CLONED_SOURCE_TMPDIR}/repo"

  printf '  Cloning source checkout from %s\n' "$clone_url"

  if [ "$VERSION" = "latest" ]; then
    if ! git clone --depth 1 "$clone_url" "$CLONED_SOURCE_ROOT" >/dev/null 2>&1; then
      rm -rf "$CLONED_SOURCE_TMPDIR"
      CLONED_SOURCE_TMPDIR=""
      CLONED_SOURCE_ROOT=""
      printf '  ⚠️  Could not clone source checkout from %s\n' "$clone_url"
      return 1
    fi
    return 0
  fi

  if ! git clone --depth 1 --branch "$VERSION" "$clone_url" "$CLONED_SOURCE_ROOT" >/dev/null 2>&1; then
    rm -rf "$CLONED_SOURCE_TMPDIR"
    CLONED_SOURCE_TMPDIR=""
    CLONED_SOURCE_ROOT=""
    printf '  ⚠️  Could not clone source checkout %s at ref %s\n' "$clone_url" "$VERSION"
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
    printf '  Using local source checkout: %s\n' "$ACQUIRED_SOURCE_ROOT"
    return 0
  fi

  # 2. Try to resolve a local source root from the script location.
  _resolved="$(resolve_source_root || true)"
  if [ -n "$_resolved" ]; then
    ACQUIRED_SOURCE_ROOT="$_resolved"
    printf '  Using local source checkout: %s\n' "$ACQUIRED_SOURCE_ROOT"
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

print_post_install() {
  printf '\n'
  printf '  📖 Skill includes:\n'
  printf '     • SKILL.md — Main skill with product overview and intent routing\n'
  printf '     • references/ — Detailed product command references\n'
  printf '     • scripts/ — Batch operation scripts for all products\n'
  printf '\n'
  printf '  ⚡ Requires: dws CLI installed and on $PATH\n'
  printf '     Install: go install github.com/%s/cmd@latest\n' "$REPO"
  printf '\n'
}

# ── Main ─────────────────────────────────────────────────────────────────────

main() {
  CWD="$(pwd)"
  SOURCE_ROOT="$(resolve_source_root || true)"

  printf '\n'
  printf '  ┌──────────────────────────────────────┐\n'
  printf '  │     DWS Skill Installer              │\n'
  printf '  │     DingTalk Workspace CLI            │\n'
  printf '  └──────────────────────────────────────┘\n'
  printf '\n'

  if ! resolve_version; then
    printf '  ⚠️  Could not determine the latest release version.\n'
    if acquire_source_checkout; then
      install_skills_local "$ACQUIRED_SOURCE_ROOT" "$CWD"
    else
      printf '  ⚠️  No source checkout available; skipping skills install.\n'
    fi
    cleanup_cloned_source
    print_post_install
    exit 0
  fi

  # Download the tarball to a temp directory
  TMPDIR_WORK="$(mktemp -d)"
  trap 'rm -rf "$TMPDIR_WORK"' EXIT INT TERM

  ASSET_URL="https://github.com/${REPO}/releases/download/${VERSION}/dws-skills.zip"
  printf '  ⬇  Downloading skills from GitHub Releases: %s (%s)\n' "$REPO" "$VERSION"
  if ! download "$ASSET_URL" "$TMPDIR_WORK/dws-skills.zip"; then
    printf '  ⚠️  Release asset download failed.\n'
    rm -rf "$TMPDIR_WORK"
    if acquire_source_checkout; then
      install_skills_local "$ACQUIRED_SOURCE_ROOT" "$CWD"
    else
      printf '  ⚠️  No source checkout available; skipping skills install.\n'
    fi
    cleanup_cloned_source
    print_post_install
    exit 0
  fi
  extract_zip "$TMPDIR_WORK/dws-skills.zip" "$TMPDIR_WORK/extracted"

  SKILL_SRC="$TMPDIR_WORK/extracted"
  if [ -f "$TMPDIR_WORK/extracted/${SKILL_NAME}/SKILL.md" ]; then
    SKILL_SRC="$TMPDIR_WORK/extracted/${SKILL_NAME}"
  fi

  if [ ! -f "$SKILL_SRC/SKILL.md" ]; then
    printf '  ⚠️  Skill source not found in release asset.\n'
    rm -rf "$TMPDIR_WORK"
    if acquire_source_checkout; then
      install_skills_local "$ACQUIRED_SOURCE_ROOT" "$CWD"
    else
      printf '  ⚠️  No source checkout available; skipping skills install.\n'
    fi
    cleanup_cloned_source
    print_post_install
    exit 0
  fi

  # Install to .agents/skills only
  dest="$CWD/$AGENT_DIR/$SKILL_NAME"

  # Remove existing installation
  if [ -d "$dest" ]; then
    rm -rf "$dest"
  fi

  # Copy skill files
  mkdir -p "$dest"
  cp -R "$SKILL_SRC/"* "$dest/"
  file_count="$(find "$dest" -type f | wc -l | tr -d ' ')"

  printf '  ✅ Universal (.agents)\n'
  printf '     → %s/%s (%s files)\n' "$AGENT_DIR" "$SKILL_NAME" "$file_count"

  print_post_install
}

main
