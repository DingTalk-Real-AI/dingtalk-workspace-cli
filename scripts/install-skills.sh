#!/bin/sh
set -eu

# Install DWS agent skills from GitHub Releases into agent skill directories.
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/DingTalk-Real-AI/dingtalk-workspace-cli/main/scripts/install-skills.sh | sh
#
# Downloads dws-skills.zip from GitHub Releases and copies it under each target
# path using the same rules as build/npm/install.js installSkillsToHomes
# (AGENT_DIRS + parent-directory gate), with root defaulting to the current
# directory. Set DWS_SKILLS_ROOT=$HOME to match npm install layout exactly.
#
# Environment variables (optional):
#   DWS_VERSION        — release tag (default: latest)
#   DWS_SKILLS_ROOT    — base path for agent dirs (default: $PWD)

REPO="DingTalk-Real-AI/dingtalk-workspace-cli"
VERSION="${DWS_VERSION:-latest}"
SKILL_NAME="dws"
ROOT="${DWS_SKILLS_ROOT:-$PWD}"

# ── Helpers ──────────────────────────────────────────────────────────────────

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf '❌ Missing required command: %s\n' "$1" >&2
    exit 1
  fi
}

resolve_version() {
  if [ "$VERSION" = "latest" ]; then
    VERSION="$(curl -fsSI "https://github.com/${REPO}/releases/latest" 2>/dev/null \
      | grep -i '^location:' | sed 's|.*/tag/||;s/[[:space:]]*$//')"
    if [ -z "$VERSION" ]; then
      printf '❌ Could not determine the latest version. Set DWS_VERSION explicitly.\n' >&2
      exit 1
    fi
  fi
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

# One-line summary copy (2nd+ targets).
_copy_skill_summary() {
  _src="$1"
  _dest="$2"
  _label="$3"

  if [ -d "$_dest" ]; then
    rm -rf "$_dest"
  fi

  mkdir -p "$_dest"
  cp -R "$_src/"* "$_dest/" 2>/dev/null || cp -r "$_src/"* "$_dest/"
  file_count="$(find "$_dest" -type f | wc -l | tr -d ' ')"

  printf '  ✅ Skills → %s (%s files)\n' "$_label" "$file_count"
}

# Full copy with top-level listing (1st target).
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

  printf '  ✅ Skills → %s (%s files)\n' "$_label" "$file_count"

  for entry in "$_dest"/*; do
    entry_name="$(basename "$entry")"
    if [ -d "$entry" ]; then
      sub_count="$(find "$entry" -type f | wc -l | tr -d ' ')"
      printf '     📁 %s/ (%s files)\n' "$entry_name" "$sub_count"
    else
      printf '     📄 %s\n' "$entry_name"
    fi
  done
}

# Same semantics as build/npm/install.js installSkillsToHomes (root = DWS_SKILLS_ROOT or PWD).
install_skills_to_root() {
  skill_src="$1"
  root="$2"
  installed=0
  idx=0
  for agent_dir in \
    ".agents/skills" \
    ".claude/skills" \
    ".cursor/skills" \
    ".gemini/skills" \
    ".codex/skills" \
    ".github/skills" \
    ".windsurf/skills" \
    ".augment/skills" \
    ".cline/skills" \
    ".amp/skills" \
    ".kiro/skills" \
    ".trae/skills" \
    ".openclaw/skills" \
    ".hermes/skills"
  do
    base_dir="$root/$agent_dir"
    parent_gate="$(dirname "$base_dir")"
    if [ "$idx" -gt 0 ] && [ ! -e "$parent_gate" ]; then
      idx=$((idx + 1))
      continue
    fi
    dest="$base_dir/$SKILL_NAME"
    if [ "$root" = "$HOME" ]; then
      label="~/$agent_dir/$SKILL_NAME"
    else
      label="$root/$agent_dir/$SKILL_NAME"
    fi
    if [ "$installed" -eq 0 ]; then
      _copy_skill "$skill_src" "$dest" "$label"
    else
      _copy_skill_summary "$skill_src" "$dest" "$label"
    fi
    installed=$((installed + 1))
    idx=$((idx + 1))
  done
  if [ "$installed" -eq 0 ]; then
    if [ "$root" = "$HOME" ]; then
      flabel="~/.agents/skills/$SKILL_NAME"
    else
      flabel="$root/.agents/skills/$SKILL_NAME"
    fi
    _copy_skill "$skill_src" "$root/.agents/skills/$SKILL_NAME" "$flabel"
  fi
}

# ── Main ─────────────────────────────────────────────────────────────────────

main() {
  need_cmd curl
  resolve_version

  printf '\n'
  printf '  ┌──────────────────────────────────────┐\n'
  printf '  │     DWS Skill Installer              │\n'
  printf '  │     DingTalk Workspace CLI            │\n'
  printf '  └──────────────────────────────────────┘\n'
  printf '\n'

  TMPDIR_WORK="$(mktemp -d)"
  trap 'rm -rf "$TMPDIR_WORK"' EXIT INT TERM

  ASSET_URL="https://github.com/${REPO}/releases/download/${VERSION}/dws-skills.zip"
  printf '  ⬇  Downloading skills from GitHub Releases: %s (%s)\n' "$REPO" "$VERSION"
  curl -fsSL "$ASSET_URL" -o "$TMPDIR_WORK/dws-skills.zip"
  extract_zip "$TMPDIR_WORK/dws-skills.zip" "$TMPDIR_WORK/extracted"

  SKILL_SRC="$TMPDIR_WORK/extracted"
  if [ -f "$TMPDIR_WORK/extracted/${SKILL_NAME}/SKILL.md" ]; then
    SKILL_SRC="$TMPDIR_WORK/extracted/${SKILL_NAME}"
  fi

  if [ ! -f "$SKILL_SRC/SKILL.md" ]; then
    printf '  ❌ Skill source not found in release asset\n' >&2
    exit 1
  fi

  printf '\n'
  printf '  Installing under root: %s\n' "$ROOT"
  install_skills_to_root "$SKILL_SRC" "$ROOT"

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

main
