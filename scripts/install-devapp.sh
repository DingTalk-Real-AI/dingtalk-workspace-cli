#!/bin/sh
# Copyright 2026 Alibaba Group
# Licensed under the Apache License, Version 2.0
#
# One-command installer for the DevApp preview branch.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/wxianfeng/dingtalk-workspace-cli/feat/dws-devapp/scripts/install-devapp.sh | sh
#
# Environment variables:
#   DEVAPP_REPO_URL          Git repository URL. Default: https://github.com/wxianfeng/dingtalk-workspace-cli.git
#   DEVAPP_BRANCH            Branch to install. Default: feat/dws-devapp
#   DEVAPP_SOURCE_DIR        Existing source checkout to install from.
#   DEVAPP_KEEP_SOURCE       Set to 1 to keep the temporary source checkout.
#   DEVAPP_SKIP_SKILL_SETUP  Set to 1 to skip automatic DevApp skill install.
#   DEVAPP_SKILL_NAME        Installed DevApp skill directory/name. Default: dws-devapp
#
# Pass-through variables handled by scripts/install.sh:
#   DWS_INSTALL_DIR          Binary install directory. Default: ~/.local/bin
#   DWS_INSTALL_NAME         Installed binary name. Default: dws
#   DWS_SKILL_MODE           mono | multi. Default here: multi
#   DWS_NO_SKILLS            Set to 1 to skip skills.
#   DWS_SKILLS_ONLY          Set to 1 to install only skills.

set -eu

REPO_URL="${DEVAPP_REPO_URL:-https://github.com/wxianfeng/dingtalk-workspace-cli.git}"
BRANCH="${DEVAPP_BRANCH:-feat/dws-devapp}"
SOURCE_DIR="${DEVAPP_SOURCE_DIR:-}"
KEEP_SOURCE="${DEVAPP_KEEP_SOURCE:-0}"
SKIP_SKILL_SETUP="${DEVAPP_SKIP_SKILL_SETUP:-0}"
DEVAPP_SKILL_NAME="${DEVAPP_SKILL_NAME:-dws-devapp}"
TMPDIR_WORK=""

say() {
  printf '  %s\n' "$@"
}

err() {
  printf '  ERROR: %s\n' "$@" >&2
  exit 1
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || err "Missing required command: $1"
}

cleanup() {
  if [ -n "$TMPDIR_WORK" ] && [ "$KEEP_SOURCE" != "1" ]; then
    rm -rf "$TMPDIR_WORK"
  fi
}

trap cleanup EXIT INT TERM

print_banner() {
  printf '\n'
  say "DevApp preview installer"
  say "Repository: ${REPO_URL}"
  say "Branch:     ${BRANCH}"
  printf '\n'
}

resolve_source_dir() {
  if [ -n "$SOURCE_DIR" ]; then
    [ -d "$SOURCE_DIR" ] || err "DEVAPP_SOURCE_DIR does not exist: ${SOURCE_DIR}"
    [ -f "$SOURCE_DIR/scripts/install.sh" ] || err "install.sh not found under DEVAPP_SOURCE_DIR: ${SOURCE_DIR}"
    printf '%s\n' "$SOURCE_DIR"
    return 0
  fi

  need_cmd git

  TMPDIR_WORK="$(mktemp -d)"
  checkout="${TMPDIR_WORK}/dingtalk-workspace-cli"

  say "Cloning DevApp preview source..." >&2
  git clone --depth 1 --branch "$BRANCH" "$REPO_URL" "$checkout"

  [ -f "$checkout/scripts/install.sh" ] || err "install.sh not found after clone"
  printf '%s\n' "$checkout"
}

copy_skill() {
  src="$1"
  dest="$2"

  rm -rf "$dest"
  mkdir -p "$dest"
  cp -R "$src/"* "$dest/" 2>/dev/null || cp -r "$src/"* "$dest/"
}

copy_devapp_skill() {
  src="$1"
  dest="$2"

  copy_skill "$src" "$dest"
  if [ -f "$dest/SKILL.md" ]; then
    tmp_skill="${dest}/SKILL.md.tmp"
    sed "s/^name: .*/name: ${DEVAPP_SKILL_NAME}/" "$dest/SKILL.md" > "$tmp_skill"
    mv "$tmp_skill" "$dest/SKILL.md"
  fi
}

install_skill_to_agent_homes() {
  src="$1"
  skill_name="$2"
  kind="$3"
  installed=0
  idx=0

  for agent_dir in \
    ".agents/skills" \
    ".claude/skills" \
    ".cursor/skills" \
    ".qoder/skills" \
    ".qoderwork/skills" \
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
    base_dir="$HOME/$agent_dir"
    parent_gate="$(dirname "$base_dir")"
    if [ "$idx" -gt 0 ] && [ ! -e "$parent_gate" ]; then
      idx=$((idx + 1))
      continue
    fi

    dest="$base_dir/$skill_name"
    if [ "$kind" = "devapp" ]; then
      copy_devapp_skill "$src" "$dest"
    else
      copy_skill "$src" "$dest"
    fi
    say "✅ Skill installed: ~/${agent_dir}/${skill_name}"
    installed=$((installed + 1))
    idx=$((idx + 1))
  done

  if [ "$installed" -eq 0 ]; then
    dest="$HOME/.agents/skills/$skill_name"
    if [ "$kind" = "devapp" ]; then
      copy_devapp_skill "$src" "$dest"
    else
      copy_skill "$src" "$dest"
    fi
    say "✅ Skill installed: ~/.agents/skills/${skill_name}"
  fi
}

install_devapp_skills() {
  if [ "${DWS_NO_SKILLS:-0}" = "1" ] || [ "$SKIP_SKILL_SETUP" = "1" ]; then
    return 0
  fi

  dws_skill_src="$source_root/skills/mono"
  devapp_skill_src="$source_root/skills/multi/dingtalk-dev"
  [ -f "$dws_skill_src/SKILL.md" ] || err "dws skill source not found: ${dws_skill_src}"
  [ -f "$devapp_skill_src/SKILL.md" ] || err "DevApp skill source not found: ${devapp_skill_src}"

  say ""
  say "Installing agent skills: dws + ${DEVAPP_SKILL_NAME}"
  install_skill_to_agent_homes "$dws_skill_src" "dws" "dws"
  install_skill_to_agent_homes "$devapp_skill_src" "$DEVAPP_SKILL_NAME" "devapp"
}

main() {
  print_banner

  if [ "${DWS_SKILLS_ONLY:-0}" != "1" ]; then
    need_cmd go
    need_cmd make
  fi

  source_root="$(resolve_source_dir)"

  if [ "${DWS_SKILLS_ONLY:-0}" = "1" ]; then
    say ""
    say "Skipping binary install because DWS_SKILLS_ONLY=1."
  else
    say ""
    say "Installing dws from DevApp preview source..."
    DWS_VERSION=latest DWS_NO_SKILLS=1 sh "$source_root/scripts/install.sh"
  fi

  install_devapp_skills

  say ""
  say "DevApp next steps:"
  say "  dws version"
  say "  dws auth login"
  say "  dws devapp --help --format json"
  say "  dws devapp list --format json"

  if [ "$KEEP_SOURCE" = "1" ]; then
    say ""
    say "Source checkout kept at: ${source_root}"
  fi
}

main
