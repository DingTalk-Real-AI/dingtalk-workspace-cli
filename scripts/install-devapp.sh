#!/bin/sh
# Copyright 2026 Alibaba Group
# Licensed under the Apache License, Version 2.0
#
# One-command installer for the dws Dev preview — pre-built binary, no build tools.
# Downloads the dev binary + dingtalk-dev skill from the fork's GitHub Releases.
# Requires only curl + tar (no go / make / git).
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/wxianfeng/dingtalk-workspace-cli/feat/dws-devapp/scripts/install-devapp.sh | sh
#
# Env (all optional):
#   DEVAPP_REPO      fork holding dev releases (default: wxianfeng/dingtalk-workspace-cli)
#   DEVAPP_VERSION   pin a dev release tag (default: latest release on the fork)
#   DWS_INSTALL_DIR  binary dir (default: ~/.local/bin)
#   DWS_NO_SKILLS    set 1 to skip the dev skill
set -eu

DEVAPP_REPO="${DEVAPP_REPO:-wxianfeng/dingtalk-workspace-cli}"
DEVAPP_VERSION="${DEVAPP_VERSION:-}"
INSTALL_DIR="${DWS_INSTALL_DIR:-$HOME/.local/bin}"
NO_SKILLS="${DWS_NO_SKILLS:-0}"
SKILL_NAME="dingtalk-dev"

say() { printf '  %s\n' "$@"; }
err() { printf '  ❌ %s\n' "$@" >&2; exit 1; }
need_cmd() { command -v "$1" >/dev/null 2>&1 || err "Missing required command: $1"; }

need_cmd curl

detect_os() {
  case "$(uname -s)" in
    Linux*)  echo linux ;;
    Darwin*) echo darwin ;;
    MINGW*|MSYS*|CYGWIN*) echo windows ;;  # Git Bash / MSYS2 / Cygwin on Windows
    *) err "Unsupported OS: $(uname -s). On native Windows use install-devapp.ps1." ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64)  echo amd64 ;;
    arm64|aarch64) echo arm64 ;;
    *) err "Unsupported architecture: $(uname -m)" ;;
  esac
}

# GitHub's /releases/latest excludes prereleases, so read the releases list
# (newest first) and take the top tag — the dev preview is published as a prerelease.
# Prefer `gh` CLI (authenticated, 5 000 req/h) over raw curl (60 req/h, easily rate-limited).
resolve_version() {
  [ -n "$DEVAPP_VERSION" ] && return 0

  # Try gh CLI first (authenticated, much higher rate limit)
  if command -v gh >/dev/null 2>&1; then
    DEVAPP_VERSION="$(gh api "repos/${DEVAPP_REPO}/releases?per_page=1" --jq '.[0].tag_name' 2>/dev/null || true)"
    [ -n "$DEVAPP_VERSION" ] && return 0
  fi

  # Fallback: unauthenticated curl (may be rate-limited)
  _tmpfile="$(mktemp)"
  _http_code="$(curl -sSL -o "$_tmpfile" -w '%{http_code}' "https://api.github.com/repos/${DEVAPP_REPO}/releases?per_page=1" 2>/dev/null || echo "000")"

  if [ "$_http_code" = "403" ] || [ "$_http_code" = "429" ]; then
    rm -f "$_tmpfile"
    err "GitHub API rate limit hit (HTTP ${_http_code}). Install the GitHub CLI (gh) or set DEVAPP_VERSION explicitly."
  fi

  DEVAPP_VERSION="$(grep -m1 '"tag_name"' "$_tmpfile" | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')"
  rm -f "$_tmpfile"
  [ -n "$DEVAPP_VERSION" ] || err "No release found on ${DEVAPP_REPO}. Push a dev tag (e.g. v1.0.39-dev.1) to trigger CI, or set DEVAPP_VERSION."
}

install_skill() {
  bundle="$1"   # extracted skills bundle dir
  src=""
  for c in "$bundle/multi/$SKILL_NAME" "$bundle/skills/multi/$SKILL_NAME" "$bundle/$SKILL_NAME"; do
    [ -f "$c/SKILL.md" ] && src="$c" && break
  done
  [ -n "$src" ] || { say "  (dingtalk-dev not found in skills bundle; skipped)"; return 0; }

  # cache so `dws skill setup --mode multi` can find a source later
  cache="$HOME/.dws/skills/multi/$SKILL_NAME"
  rm -rf "$cache"; mkdir -p "$cache"; cp -R "$src/." "$cache/"

  installed=0; idx=0
  for agent_dir in \
    .agents/skills .claude/skills .cursor/skills .qoder/skills .qoderwork/skills \
    .gemini/skills .codex/skills .github/skills .windsurf/skills .augment/skills \
    .cline/skills .amp/skills .kiro/skills .trae/skills .openclaw/skills .hermes/skills \
    .config/opencode/skills
  do
    base="$HOME/$agent_dir"
    if [ "$idx" -gt 0 ] && [ ! -e "$(dirname "$base")" ]; then idx=$((idx + 1)); continue; fi
    dest="$base/$SKILL_NAME"; rm -rf "$dest"; mkdir -p "$dest"; cp -R "$src/." "$dest/"
    installed=$((installed + 1)); idx=$((idx + 1))
  done
  say "✅ Skill dingtalk-dev → ${installed} agent dir(s)"
}

main() {
  resolve_version
  os="$(detect_os)"; arch="$(detect_arch)"
  tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT INT TERM

  printf '\n'
  say "dws Dev preview installer (pre-built binary)"
  say "Repo:    ${DEVAPP_REPO}"
  say "Version: ${DEVAPP_VERSION}"
  say "Target:  ${os}/${arch}"
  printf '\n'

  # 1) binary (already ad-hoc signed by CI; copy does not break the signature)
  if [ "$os" = "windows" ]; then
    asset="dws-windows-${arch}.zip"; binname="dws.exe"
  else
    asset="dws-${os}-${arch}.tar.gz"; binname="dws"
  fi
  say "⬇  Downloading ${asset} ..."
  curl -fsSL "https://github.com/${DEVAPP_REPO}/releases/download/${DEVAPP_VERSION}/${asset}" -o "$tmp/$asset" \
    || err "Binary download failed — does release ${DEVAPP_VERSION} have ${asset}?"
  if [ "$os" = "windows" ]; then
    need_cmd unzip; unzip -q "$tmp/$asset" -d "$tmp"
  else
    need_cmd tar; tar -xzf "$tmp/$asset" -C "$tmp"
  fi
  [ -f "$tmp/$binname" ] || err "${binname} not found inside ${asset}"
  mkdir -p "$INSTALL_DIR"
  cp "$tmp/$binname" "$INSTALL_DIR/$binname"; chmod +x "$INSTALL_DIR/$binname" 2>/dev/null || true
  say "✅ Binary → ${INSTALL_DIR}/${binname}"

  # 2) dev skill from the release's skills bundle
  if [ "$NO_SKILLS" != "1" ]; then
    if curl -fsSL "https://github.com/${DEVAPP_REPO}/releases/download/${DEVAPP_VERSION}/dws-skills.zip" -o "$tmp/skills.zip" 2>/dev/null; then
      mkdir -p "$tmp/sk"
      if command -v unzip >/dev/null 2>&1; then unzip -q "$tmp/skills.zip" -d "$tmp/sk"; else tar -xf "$tmp/skills.zip" -C "$tmp/sk"; fi
      say ""
      install_skill "$tmp/sk"
    else
      say "  (no dws-skills.zip in release ${DEVAPP_VERSION}; skill skipped)"
    fi
  fi

  printf '\n'
  say "🎉 Done. Next steps:"
  say "  dws version"
  say "  dws auth login"
  say "  dws dev --help --format json"
  say "  dws dev app list --format json"
  printf '\n'
  case ":$PATH:" in
    *":$INSTALL_DIR:"*) ;;
    *) say "Note: ${INSTALL_DIR} is not on \$PATH — add it so 'dws' is found." ;;
  esac
}

main
