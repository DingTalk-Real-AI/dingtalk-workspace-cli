#!/bin/sh
# Copyright 2026 Alibaba Group
# Licensed under the Apache License, Version 2.0
#
# One-command installer for dws personal events.
# Downloads the official dws binary and installs:
#   - multi skill: dingtalk-event
#   - mono skill:  dws
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/DingTalk-Real-AI/dingtalk-workspace-cli/main/scripts/install-event.sh | sh
#
# Env (all optional):
#   EVENT_REPO       repo holding official releases (default: DingTalk-Real-AI/dingtalk-workspace-cli)
#   EVENT_VERSION    pin a release tag (default: latest stable release)
#   DWS_VERSION      alias for EVENT_VERSION when EVENT_VERSION is empty
#   DWS_INSTALL_DIR  binary dir (default: ~/.local/bin)
#   DWS_NO_SKILLS    set 1 to skip skill installation
#   DWS_SKILLS_ONLY  set 1 to install only skills, without touching the binary
set -eu

EVENT_REPO="${EVENT_REPO:-DingTalk-Real-AI/dingtalk-workspace-cli}"
EVENT_VERSION="${EVENT_VERSION:-${DWS_VERSION:-}}"
INSTALL_DIR="${DWS_INSTALL_DIR:-$HOME/.local/bin}"
NO_SKILLS="${DWS_NO_SKILLS:-0}"
SKILLS_ONLY="${DWS_SKILLS_ONLY:-0}"
BIN_NAME="dws"
EVENT_SKILL_NAME="dingtalk-event"
MONO_SKILL_NAME="dws"

if [ "$EVENT_VERSION" = "latest" ]; then
  EVENT_VERSION=""
fi

say() { printf '  %s\n' "$@"; }
err() { printf '  ERROR: %s\n' "$@" >&2; exit 1; }
need_cmd() { command -v "$1" >/dev/null 2>&1 || err "Missing required command: $1"; }

detect_os() {
  case "$(uname -s)" in
    Linux*)  echo linux ;;
    Darwin*) echo darwin ;;
    MINGW*|MSYS*|CYGWIN*) echo windows ;;
    *) err "Unsupported OS: $(uname -s). On native Windows use a PowerShell installer." ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64)  echo amd64 ;;
    arm64|aarch64) echo arm64 ;;
    *) err "Unsupported architecture: $(uname -m)" ;;
  esac
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
  err "Missing required command: unzip (or tar with zip support)"
}

resolve_event_version() {
  [ -n "$EVENT_VERSION" ] && return 0

  if command -v gh >/dev/null 2>&1; then
    EVENT_VERSION="$(gh api "repos/${EVENT_REPO}/releases/latest" --jq '.tag_name' 2>/dev/null || true)"
    [ -n "$EVENT_VERSION" ] && [ "$EVENT_VERSION" != "null" ] && return 0
    EVENT_VERSION=""
  fi

  tmpfile="$(mktemp)"
  http_code="$(curl -sSL -o "$tmpfile" -w '%{http_code}' "https://api.github.com/repos/${EVENT_REPO}/releases/latest" 2>/dev/null || echo "000")"
  if [ "$http_code" = "403" ] || [ "$http_code" = "429" ]; then
    rm -f "$tmpfile"
    err "GitHub API rate limit hit (HTTP ${http_code}). Install gh or set EVENT_VERSION explicitly."
  fi
  EVENT_VERSION="$(grep -m1 '"tag_name"' "$tmpfile" | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/' || true)"
  rm -f "$tmpfile"
  [ -n "$EVENT_VERSION" ] || err "No stable release found on ${EVENT_REPO}. Set EVENT_VERSION to a published release tag."
}

copy_tree() {
  src="$1"
  dest="$2"
  rm -rf "$dest"
  mkdir -p "$dest"
  cp -R "$src/." "$dest/"
}

install_skill_to_homes() {
  src="$1"
  skill_name="$2"

  installed=0
  idx=0
  for agent_dir in \
    .agents/skills .claude/skills .cursor/skills .qoder/skills .qoderwork/skills \
    .gemini/skills .codex/skills .github/skills .windsurf/skills .augment/skills \
    .cline/skills .amp/skills .kiro/skills .trae/skills .openclaw/skills .hermes/skills \
    .config/opencode/skills
  do
    base="$HOME/$agent_dir"
    if [ "$idx" -gt 0 ] && [ ! -e "$(dirname "$base")" ]; then
      idx=$((idx + 1))
      continue
    fi
    copy_tree "$src" "$base/$skill_name"
    installed=$((installed + 1))
    idx=$((idx + 1))
  done

  if [ "$installed" -eq 0 ]; then
    copy_tree "$src" "$HOME/.agents/skills/$skill_name"
    installed=1
  fi
  printf '%s\n' "$installed"
}

find_event_skill_src() {
  bundle="$1"
  for c in \
    "$bundle/multi/$EVENT_SKILL_NAME" \
    "$bundle/skills/multi/$EVENT_SKILL_NAME" \
    "$bundle/$EVENT_SKILL_NAME"
  do
    if [ -f "$c/SKILL.md" ]; then
      printf '%s\n' "$c"
      return 0
    fi
  done
  return 1
}

find_mono_skill_src() {
  bundle="$1"
  for c in \
    "$bundle/mono" \
    "$bundle/skills/mono" \
    "$bundle/$MONO_SKILL_NAME" \
    "$bundle"
  do
    if [ -f "$c/SKILL.md" ]; then
      printf '%s\n' "$c"
      return 0
    fi
  done
  return 1
}

install_skills_from_bundle() {
  bundle="$1"

  event_src="$(find_event_skill_src "$bundle" || true)"
  [ -n "$event_src" ] || err "${EVENT_SKILL_NAME} not found in dws-skills.zip"
  mono_src="$(find_mono_skill_src "$bundle" || true)"
  [ -n "$mono_src" ] || err "mono ${MONO_SKILL_NAME} skill not found in dws-skills.zip"

  event_cache="$HOME/.dws/skills/multi/$EVENT_SKILL_NAME"
  copy_tree "$event_src" "$event_cache"

  mono_cache="$HOME/.dws/skills/mono"
  copy_tree "$mono_src" "$mono_cache"

  event_installed="$(install_skill_to_homes "$event_src" "$EVENT_SKILL_NAME")"
  mono_installed="$(install_skill_to_homes "$mono_src" "$MONO_SKILL_NAME")"

  say "Skill ${EVENT_SKILL_NAME} -> ${event_installed} agent dir(s)"
  say "Skill ${MONO_SKILL_NAME} -> ${mono_installed} agent dir(s)"
  say "Cached ${EVENT_SKILL_NAME} -> ${event_cache}"
  say "Cached mono ${MONO_SKILL_NAME} -> ${mono_cache}"
}

install_binary() {
  os="$(detect_os)"
  arch="$(detect_arch)"
  if [ "$os" = "windows" ]; then
    asset="${BIN_NAME}-windows-${arch}.zip"
    binname="${BIN_NAME}.exe"
  else
    asset="${BIN_NAME}-${os}-${arch}.tar.gz"
    binname="${BIN_NAME}"
  fi

  say "Downloading ${asset} ..."
  curl -fsSL "https://github.com/${EVENT_REPO}/releases/download/${EVENT_VERSION}/${asset}" -o "$tmp/$asset" \
    || err "Binary download failed. Does release ${EVENT_VERSION} have ${asset}?"

  if [ "$os" = "windows" ]; then
    extract_zip "$tmp/$asset" "$tmp/bin"
  else
    need_cmd tar
    mkdir -p "$tmp/bin"
    tar -xzf "$tmp/$asset" -C "$tmp/bin"
  fi

  found=""
  for c in "$tmp/bin/$binname" "$tmp/bin/${BIN_NAME}-${os}-${arch}/$binname"; do
    [ -f "$c" ] && found="$c" && break
  done
  if [ -z "$found" ]; then
    found="$(find "$tmp/bin" -name "$binname" -type f | head -1 || true)"
  fi
  [ -n "$found" ] || err "${binname} not found inside ${asset}"

  mkdir -p "$INSTALL_DIR"
  cp "$found" "$INSTALL_DIR/$binname"
  chmod +x "$INSTALL_DIR/$binname" 2>/dev/null || true
  say "Binary -> ${INSTALL_DIR}/${binname}"
}

install_skills() {
  say "Downloading dws-skills.zip ..."
  curl -fsSL "https://github.com/${EVENT_REPO}/releases/download/${EVENT_VERSION}/dws-skills.zip" -o "$tmp/dws-skills.zip" \
    || err "dws-skills.zip download failed for release ${EVENT_VERSION}"

  mkdir -p "$tmp/skills"
  extract_zip "$tmp/dws-skills.zip" "$tmp/skills"
  install_skills_from_bundle "$tmp/skills"
}

main() {
  need_cmd curl
  if [ "$NO_SKILLS" = "1" ] && [ "$SKILLS_ONLY" = "1" ]; then
    err "DWS_NO_SKILLS=1 cannot be combined with DWS_SKILLS_ONLY=1"
  fi

  resolve_event_version
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT INT TERM

  printf '\n'
  say "dws event installer"
  say "Repo:    ${EVENT_REPO}"
  say "Version: ${EVENT_VERSION}"
  say "Install: ${INSTALL_DIR}"
  printf '\n'

  if [ "$SKILLS_ONLY" != "1" ]; then
    install_binary
  else
    say "DWS_SKILLS_ONLY=1, skipping binary installation."
  fi

  if [ "$NO_SKILLS" != "1" ]; then
    install_skills
  else
    say "DWS_NO_SKILLS=1, skipping skill installation."
  fi

  printf '\n'
  say "Done. Verify with:"
  if [ "$SKILLS_ONLY" != "1" ]; then
    say "  dws version"
  fi
  say "  dws event schema user_im_message_receive_o2o"
  say "  dws event consume user_im_message_receive_o2o --user <userId> -f ndjson"
  say ""
  say "If an Agent session is already open, restart it or reload skills before testing event routing."
  case ":$PATH:" in
    *":$INSTALL_DIR:"*) ;;
    *) [ "$SKILLS_ONLY" = "1" ] || say "Note: ${INSTALL_DIR} is not on PATH." ;;
  esac
}

main
