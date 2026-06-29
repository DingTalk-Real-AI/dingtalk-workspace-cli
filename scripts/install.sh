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
#   DWS_SKILL_MODE    — mono | multi (default: prompt if TTY, else mono)
#   DWS_GITEE_REPO    — "owner/repo" on Gitee; when set, version + assets resolve
#                       via the Gitee API instead of GitHub (China mirror)
#
# Agent skills paths follow build/npm/install.js AGENT_DIRS (order and entries must match).

set -eu

REPO="DingTalk-Real-AI/dingtalk-workspace-cli"
BIN_NAME="dws"
# China mirror: Gitee repo "owner/repo". When set, version + asset URLs resolve via
# the Gitee API (https://gitee.com/api/v5) instead of GitHub. Gitee attachment URLs
# carry an unstable numeric id, so every asset is resolved by name at install time.
GITEE_REPO="${DWS_GITEE_REPO:-}"
# Auto-fallback: when DWS_GITEE_REPO is not set, the installer probes GitHub and,
# if it is unreachable (typical in mainland China), automatically switches to this
# Gitee mirror — so a plain `curl … | sh` works everywhere with no env var.
# Set DWS_NO_FALLBACK=1 to disable the probe and force GitHub.
GITEE_FALLBACK_REPO="${DWS_GITEE_FALLBACK_REPO:-DingTalk-Real-AI/dingtalk-workspace-cli}"
INSTALL_DIR="${DWS_INSTALL_DIR:-$HOME/.local/bin}"
INSTALL_NAME="${DWS_INSTALL_NAME:-$BIN_NAME}"
VERSION="${DWS_VERSION:-latest}"
NO_SKILLS="${DWS_NO_SKILLS:-0}"
SKILLS_ONLY="${DWS_SKILLS_ONLY:-0}"
SKILL_NAME="dws"
SKILL_MODE=""

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
    err "Neither curl nor wget found. Please install one and retry."
  fi
}

# Fetch a Gitee API endpoint, retrying transient failures. Gitee's gateway
# returns sporadic 502/503, so a single curl is unreliable; retry a few times
# before giving up. Prints the response body on success.
gitee_api() {
  _url="$1"
  _try=1
  while [ "$_try" -le 4 ]; do
    if _resp="$(curl -fsSL "$_url" 2>/dev/null)" && [ -n "$_resp" ]; then
      printf '%s' "$_resp"
      return 0
    fi
    _try=$((_try + 1))
    sleep 2
  done
  return 1
}

# Resolve the download URL of a release asset by file name.
#   GitHub: deterministic template <repo>/releases/download/<version>/<file>.
#   Gitee : query the release API and pick the asset whose name matches
#           (Gitee attachment URLs carry an unstable numeric id, so we never
#            template them — we read browser_download_url straight from the API).
asset_url() {
  _name="$1"
  if [ -z "$GITEE_REPO" ]; then
    printf '%s' "https://github.com/${REPO}/releases/download/${VERSION}/${_name}"
    return 0
  fi
  gitee_api "https://gitee.com/api/v5/repos/${GITEE_REPO}/releases/tags/${VERSION}" \
    | tr '}' '\n' \
    | grep "\"name\":[ ]*\"${_name}\"" \
    | grep -o '"browser_download_url":[ ]*"[^"]*"' \
    | head -1 \
    | sed 's/.*"browser_download_url":[ ]*"//;s/"$//'
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

# Decide the download source. An explicit DWS_GITEE_REPO always wins. Otherwise
# probe GitHub Releases; if it is unreachable (typical in mainland China), switch
# GITEE_REPO to the mirror so every subsequent resolve/download uses Gitee.
pick_source() {
  [ -n "$GITEE_REPO" ] && return 0
  [ "${DWS_NO_FALLBACK:-0}" = "1" ] && return 0
  if need_cmd curl; then
    curl -fsS --connect-timeout 5 --max-time 12 -o /dev/null "https://github.com/${REPO}/releases/latest" 2>/dev/null && return 0
  elif need_cmd wget; then
    wget -q --timeout=12 --tries=1 -O /dev/null "https://github.com/${REPO}/releases/latest" 2>/dev/null && return 0
  fi
  GITEE_REPO="$GITEE_FALLBACK_REPO"
  say "⚠ GitHub 不可达，自动切换国内 Gitee 镜像: ${GITEE_REPO}"
}

# Resolve the latest version tag from GitHub
resolve_version() {
  if [ "$VERSION" = "latest" ]; then
    if [ -n "$GITEE_REPO" ]; then
      # Gitee's /releases/latest and /releases endpoints are unreliable — they
      # return 404 / an empty list even when releases exist — so resolve the
      # newest version from the git tags endpoint instead: keep only vN.N.N
      # tags and pick the highest by version order.
      VERSION="$(gitee_api "https://gitee.com/api/v5/repos/${GITEE_REPO}/tags" \
        | grep -o '"name":[ ]*"v[0-9][0-9.]*"' \
        | sed 's/.*"name":[ ]*"//;s/"$//' \
        | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' \
        | sort -V | tail -1)"
    elif need_cmd curl; then
      # Follow the redirect from /releases/latest to get the tag
      VERSION="$(curl -fsSI "https://github.com/${REPO}/releases/latest" 2>/dev/null \
        | grep -i '^location:' | sed 's|.*/tag/||;s/[[:space:]]*$//')"
    elif need_cmd wget; then
      VERSION="$(wget --spider --max-redirect=0 "$LATEST_URL" 2>&1 \
        | grep -i 'Location:' | sed 's|.*/tag/||;s/[[:space:]]*$//')"
    fi
    if [ -z "$VERSION" ]; then
      err "Could not determine the latest version. Set DWS_VERSION explicitly."
    fi
  fi
}

# ── Banner ───────────────────────────────────────────────────────────────────

print_banner() {
  printf '\n'
  say "┌──────────────────────────────────────┐"
  say "│     DWS Installer                    │"
  say "│     DingTalk Workspace CLI           │"
  say "└──────────────────────────────────────┘"
  printf '\n'
}

# ── Skill Mode Resolution ────────────────────────────────────────────────────
#
# Priority (highest first):
#   1. DWS_SKILL_MODE env var (mono | multi, case-insensitive)
#   2. Interactive prompt when both stdin and stdout are TTYs (default: mono)
#   3. Fallback: mono (non-TTY without env var, e.g. curl | sh)
resolve_skill_mode() {
  if [ -n "${DWS_SKILL_MODE:-}" ]; then
    raw="$DWS_SKILL_MODE"
    # Lower-case without bash-specific ${var,,}; tr is POSIX.
    normalized="$(printf '%s' "$raw" | tr '[:upper:]' '[:lower:]')"
    case "$normalized" in
      mono|multi)
        SKILL_MODE="$normalized"
        say "Skill mode: ${SKILL_MODE} (from DWS_SKILL_MODE)"
        return 0
        ;;
      *)
        err "Invalid DWS_SKILL_MODE='${raw}'. Use 'mono' or 'multi'."
        ;;
    esac
  fi

  if [ -t 0 ] && [ -t 1 ]; then
    printf '\n'
    say "Select skill installation mode:"
    say "  1) mono                  — install one bundled dws skill (stable / recommended)"
    say "  2) multi 🧪 EXPERIMENTAL — split each product into its own skill (preview; run 'dws skill setup --mode multi' afterwards)"
    say "     ⚠ multi is not yet stable — interface, naming and cross-skill references may change"
    printf '  Choice [1]: '
    read choice || choice=""
    case "$choice" in
      ""|1|mono)  SKILL_MODE="mono" ;;
      2|multi)    SKILL_MODE="multi" ;;
      *)
        say "Unrecognized choice '${choice}', defaulting to mono."
        SKILL_MODE="mono"
        ;;
    esac
    say "Skill mode: ${SKILL_MODE}"
    return 0
  fi

  SKILL_MODE="mono"
}

print_multi_mode_notice() {
  say ""
  say "🧪 Skill mode: multi (EXPERIMENTAL / preview) — automatic skill install skipped."
  say "   ⚠ multi is not yet stable. 20 product-scoped skills pass dispatch verifier,"
  say "     but interface, naming and cross-skill references may change in future releases."
  say "     For production / shared environments, use mono mode (--mode mono)."
  say ""
  say "   To install split skills, run:"
  say "     ${BIN_NAME} skill setup --mode multi"
  say "   (One skill per product family; requires the dws binary installed above.)"
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
  skill_src="${root}/skills/mono"
  multi_src="${root}/skills/multi"

  if [ ! -d "$skill_src" ]; then
    say "⚠️  Local skills directory not found: ${skill_src}"
    say "   Skipping skills installation."
    return 1
  fi

  say ""
  say "📦 Installing agent skills from local source: ${skill_src}"

  install_skills_to_homes "$skill_src"

  # Cache multi source for later `dws skill setup --mode multi`.
  if [ -d "$multi_src" ]; then
    cache_multi_skills "$multi_src"
  fi

  # Also cache a mono copy so `dws skill setup --mode mono` has a fallback
  # under ~/.dws/skills/mono when invoked without --source.
  cache_mono_skills "$skill_src"

  return 0
}

# cache_multi_skills copies the multi/ tree (per-product skills) into
# ~/.dws/skills/multi/ so that `dws skill setup --mode multi` can find a
# source without needing the source checkout or a re-download.
cache_multi_skills() {
  src="$1"
  cache_dir="${HOME}/.dws/skills/multi"

  if [ ! -d "$src" ]; then
    return 0
  fi

  rm -rf "$cache_dir"
  mkdir -p "$cache_dir"
  cp -R "$src/"* "$cache_dir/" 2>/dev/null || cp -r "$src/"* "$cache_dir/" 2>/dev/null || true

  file_count="$(find "$cache_dir" -type f | wc -l | tr -d ' ')"
  case "$cache_dir" in
    "$HOME"/*) label="~/${cache_dir#$HOME/}" ;;
    *)         label="$cache_dir" ;;
  esac
  say "✅ Cached multi skills → ${label} (${file_count} files)"
}

# cache_mono_skills mirrors cache_multi_skills for the mono tree. Keeping the
# two modes symmetrical means `dws skill setup` can fall back to ~/.dws/skills
# regardless of which mode the user picks later.
cache_mono_skills() {
  src="$1"
  cache_dir="${HOME}/.dws/skills/mono"

  if [ ! -d "$src" ]; then
    return 0
  fi

  rm -rf "$cache_dir"
  mkdir -p "$cache_dir"
  cp -R "$src/"* "$cache_dir/" 2>/dev/null || cp -r "$src/"* "$cache_dir/" 2>/dev/null || true
}

# Install skill tree into all agent homes (same rules as build/npm/install.js installSkillsToHomes).
install_skills_to_homes() {
  skill_src="$1"
  root="${HOME}"
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
    base_dir="$root/$agent_dir"
    parent_gate="$(dirname "$base_dir")"
    if [ "$idx" -gt 0 ] && [ ! -e "$parent_gate" ]; then
      idx=$((idx + 1))
      continue
    fi
    dest="$base_dir/$SKILL_NAME"
    case "$root" in
      "$HOME")
        label="~/$agent_dir/$SKILL_NAME"
        ;;
      *)
        label="$root/$agent_dir/$SKILL_NAME"
        ;;
    esac
    if [ "$installed" -eq 0 ]; then
      _copy_skill "$skill_src" "$dest" "$label"
    else
      _copy_skill_summary "$skill_src" "$dest" "$label"
    fi
    installed=$((installed + 1))
    idx=$((idx + 1))
  done
  if [ "$installed" -eq 0 ]; then
    case "$root" in
      "$HOME")
        flabel="~/.agents/skills/$SKILL_NAME"
        ;;
      *)
        flabel="$root/.agents/skills/$SKILL_NAME"
        ;;
    esac
    _copy_skill "$skill_src" "$root/.agents/skills/$SKILL_NAME" "$flabel"
  fi
}

# One-line summary copy (used for 2nd+ agent targets).
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

  say "✅ Skills → ${_label} (${file_count} files)"
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
  resolve_version

  archive_name="${BIN_NAME}-${os}-${arch}.tar.gz"
  download_url="$(asset_url "$archive_name")"
  [ -n "$download_url" ] || err "Could not resolve download URL for ${archive_name} (version ${VERSION})."

  say "⬇  Downloading ${BIN_NAME} ${VERSION} (${os}/${arch})..."

  tmpdir="$(mktemp -d)"
  trap 'rm -rf "$tmpdir"' EXIT INT TERM

  download "$download_url" "$tmpdir/$archive_name"

  # Download and verify SHA256 checksum
  checksum_url="$(asset_url checksums.txt)"
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

  resolve_version
  skills_archive="dws-skills.zip"
  download_url="$(asset_url "$skills_archive")"
  [ -n "$download_url" ] || err "Could not resolve download URL for ${skills_archive} (version ${VERSION})."

  tmpdir_skills="$(mktemp -d)"
  trap 'rm -rf "$tmpdir_skills"' EXIT INT TERM

  if ! download "$download_url" "$tmpdir_skills/$skills_archive" 2>/dev/null; then
    say "⚠️  Release asset download failed. Trying local source..."
    rm -rf "$tmpdir_skills"
    local_root="$(resolve_source_root || true)"
    if [ -n "$local_root" ]; then
      install_skills_local "$local_root"
      return
    else
      err "Cannot download skills from GitHub and no local source checkout found."
    fi
  fi

  extract_root="$tmpdir_skills/skills"
  mkdir -p "$extract_root"
  if ! extract_zip "$tmpdir_skills/$skills_archive" "$extract_root" 2>/dev/null; then
    say "⚠️  Could not extract release skill archive. Install unzip, or retry from a source checkout."
    rm -rf "$tmpdir_skills"
    local_root="$(resolve_source_root || true)"
    if [ -n "$local_root" ]; then
      install_skills_local "$local_root"
      return
    fi
    err "Cannot extract release skill archive and no local source checkout found."
  fi

  # New release layout puts mono content both at the zip root (for backward
  # compatibility with older installers) and under ./mono/, with multi/ as a
  # sibling. Prefer ./mono/ when present so we never miss SKILL.md, then fall
  # back to the legacy nested $SKILL_NAME/ shape, then the zip root.
  skill_src="$extract_root"
  if [ -d "$extract_root/mono" ] && [ -f "$extract_root/mono/SKILL.md" ]; then
    skill_src="$extract_root/mono"
  elif [ -f "$extract_root/$SKILL_NAME/SKILL.md" ]; then
    skill_src="$extract_root/$SKILL_NAME"
  fi
  if [ ! -f "$skill_src/SKILL.md" ]; then
    say "⚠️  Skills not found in release asset. Trying local source..."
    rm -rf "$tmpdir_skills"
    local_root="$(resolve_source_root || true)"
    if [ -n "$local_root" ]; then
      install_skills_local "$local_root"
      return
    else
      say "⚠️  No local source checkout found either. Skipping skills installation."
      return
    fi
  fi

  install_skills_to_homes "$skill_src"

  # Cache the multi tree (if present in the release asset) so a later
  # `dws skill setup --mode multi` can find a source without re-downloading.
  if [ -d "$extract_root/multi" ]; then
    cache_multi_skills "$extract_root/multi"
  fi

  # And cache mono too for symmetry with --mode mono fallbacks.
  cache_mono_skills "$skill_src"

  rm -rf "$tmpdir_skills"
}

# ── Main ─────────────────────────────────────────────────────────────────────

main() {
  source_root=""
  if [ "$SKILLS_ONLY" != "1" ] && [ "$VERSION" = "latest" ]; then
    source_root="$(resolve_source_root || true)"
  fi

  print_banner

  # Pick GitHub vs Gitee mirror (auto-fallback when GitHub is unreachable).
  # Skipped when installing from a local source checkout (no download needed).
  [ -z "$source_root" ] && pick_source

  # Resolve skill mode only when we are actually going to touch skills.
  if [ "$NO_SKILLS" != "1" ]; then
    resolve_skill_mode
  fi

  if [ -n "$source_root" ]; then
    install_binary_from_source "$source_root"
    if [ "$NO_SKILLS" != "1" ]; then
      if [ "$SKILL_MODE" = "multi" ]; then
        print_multi_mode_notice
      else
        install_skills_local "$source_root"
      fi
    fi
  elif [ "$SKILLS_ONLY" = "1" ]; then
    if [ "$SKILL_MODE" = "multi" ]; then
      print_multi_mode_notice
    else
      local_root="$(resolve_source_root || true)"
      if [ -n "$local_root" ]; then
        install_skills_local "$local_root"
      else
        install_skills
      fi
    fi
  elif [ "$NO_SKILLS" = "1" ]; then
    install_binary
  else
    install_binary
    if [ "$SKILL_MODE" = "multi" ]; then
      print_multi_mode_notice
    else
      install_skills
    fi
  fi

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
