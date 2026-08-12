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
#   DWS_SKILL_MODE    — mono | multi (default: prompt if TTY, else multi)
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
SKILL_STATE_ROOT="${DWS_CONFIG_DIR:-$HOME/.dws}"
SKILL_NAME="dws"
SKILL_MODE=""
MANAGED_SKILL_DIGEST_SCOPE="skill-directory-v1"

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

# backup_and_remove_skill_dir <dir>
# Moves <dir> into $HOME/.dws/skill-backups/<stamp>/<name> instead of
# destroying it (non-interactive installs cannot confirm, so removals must
# stay reversible). Missing paths are a no-op success. On any backup failure
# the directory is left in place and a non-zero status is returned so callers
# skip that target rather than silently deleting data.
DWS_LAST_SKILL_BACKUP=""
backup_and_remove_skill_dir() {
  _bed_dir="$1"
  DWS_LAST_SKILL_BACKUP=""
  [ -d "$_bed_dir" ] || return 0
  _bed_root="${HOME}/.dws/skill-backups"
  _bed_stamp="$(date -u +%Y%m%d-%H%M%S)"
  _bed_name="$(basename "$_bed_dir")"
  _bed_target="$_bed_root/$_bed_stamp/$_bed_name"
  _bed_i=1
  while [ -e "$_bed_target" ]; do
    _bed_target="$_bed_root/$_bed_stamp-$_bed_i/$_bed_name"
    _bed_i=$((_bed_i + 1))
    if [ "$_bed_i" -gt 1000 ]; then
      say "  ⚠️  备份目录冲突，保留原目录 $_bed_dir"
      return 1
    fi
  done
  mkdir -p "$(dirname "$_bed_target")" 2>/dev/null || {
    say "  ⚠️  无法创建备份目录，保留原目录 $_bed_dir"
    return 1
  }
  if mv "$_bed_dir" "$_bed_target" 2>/dev/null; then
    DWS_LAST_SKILL_BACKUP="$_bed_target"
    say "  × 已备份并移除 $_bed_dir → $_bed_target"
    return 0
  fi
  say "  ⚠️  备份失败，保留原目录 $_bed_dir"
  return 1
}

# A dingtalk-* prefix alone is not ownership evidence: market/user skills may
# use it too. Ownership comes from the centralized skills-state.json.
is_managed_multi_skill_dir() {
  _managed_dir="$1"
  _managed_name="$(basename "$_managed_dir")"
  is_legacy_official_multi_skill_name "$_managed_name" && return 0
  [ -f "$SKILL_STATE_ROOT/skills-state.json" ] || return 1
  _managed_json_name="$(json_escape "$_managed_name")"
  _managed_compact='"name":"'"$_managed_json_name"'"'
  _managed_spaced='"name": "'"$_managed_json_name"'"'
  DWS_MANAGED_COMPACT="$_managed_compact" DWS_MANAGED_SPACED="$_managed_spaced" awk '
    /^[[:space:]]*"managed_skills"[[:space:]]*:[[:space:]]*\[[[:space:]]*$/ { inside = 1; next }
    inside && /^[[:space:]]*\][[:space:]]*,?[[:space:]]*$/ { closed = 1; exit }
    inside && (index($0, ENVIRON["DWS_MANAGED_COMPACT"]) || index($0, ENVIRON["DWS_MANAGED_SPACED"])) { found = 1 }
    END { exit !(closed && found) }
  ' "$SKILL_STATE_ROOT/skills-state.json"
}

# Frozen exact names shipped before centralized ownership metadata. Never replace this
# with a dingtalk-* prefix check: user/market Skills may use that prefix.
is_legacy_official_multi_skill_name() {
  case "$1" in
    dingtalk-agoal|dingtalk-aiapp|dingtalk-aisearch|dingtalk-aitable|dingtalk-attendance|dingtalk-calendar|dingtalk-chat|dingtalk-contact|dingtalk-dev|dingtalk-devapp|dingtalk-devdoc|dingtalk-ding|dingtalk-doc|dingtalk-drive|dingtalk-event|dingtalk-hrbrain|dingtalk-live|dingtalk-mail|dingtalk-markdown|dingtalk-minutes|dingtalk-misc|dingtalk-oa|dingtalk-pat|dingtalk-profile|dingtalk-report|dingtalk-shared|dingtalk-sheet|dingtalk-skill|dingtalk-todo|dingtalk-wiki|dws-shared) return 0 ;;
  esac
  return 1
}

json_escape() {
  printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
}

sha256_stdin() {
  if need_cmd sha256sum; then
    sha256sum | awk '{print $1}'
  elif need_cmd shasum; then
    shasum -a 256 | awk '{print $1}'
  elif need_cmd openssl; then
    openssl dgst -sha256 | awk '{print $NF}'
  else
    return 1
  fi
}

digest_skill_dir() {
  _digest_dir="$1"
  _digest="$({
    find "$_digest_dir" -type f -print | LC_ALL=C sort | while IFS= read -r _digest_file; do
      _digest_rel="${_digest_file#"$_digest_dir"/}"
      printf '%s\0' "$_digest_rel"
      cat "$_digest_file"
      printf '\0'
    done
  } | sha256_stdin)" || return 1
  printf 'sha256:%s' "$_digest"
}

write_skills_state() {
  _state_multi="$1"
  _state_source="$2"
  _state_root="$SKILL_STATE_ROOT"
  mkdir -p "$_state_root" || return 1
  _state_tmp="$(mktemp "$_state_root/.skills-state.XXXXXX")" || return 1
  _state_version="$(json_escape "$VERSION")"
  _state_names=""
  for _state_dir in "$_state_multi"/*/; do
    [ -f "${_state_dir}SKILL.md" ] || continue
    _state_name="$(basename "$_state_dir")"
    _state_names="${_state_names}${_state_name}\n"
  done
  {
    printf '{\n  "version": "%s",\n' "$_state_version"
    printf '  "official_skills": ['
    _state_first=1
    printf '%b' "$_state_names" | LC_ALL=C sort | while IFS= read -r _state_name; do
      [ -n "$_state_name" ] || continue
      [ "$_state_first" -eq 1 ] || printf ', '
      printf '"%s"' "$(json_escape "$_state_name")"
      _state_first=0
    done
    printf '],\n  "updated_skills": ['
    _state_first=1
    printf '%b' "$_state_names" | LC_ALL=C sort | while IFS= read -r _state_name; do
      [ -n "$_state_name" ] || continue
      [ "$_state_first" -eq 1 ] || printf ', '
      printf '"%s"' "$(json_escape "$_state_name")"
      _state_first=0
    done
    printf '],\n  "managed_skills": [\n'
    _state_first=1
    printf '%b' "$_state_names" | LC_ALL=C sort | while IFS= read -r _state_name; do
      [ -n "$_state_name" ] || continue
      _state_digest="$(digest_skill_dir "$_state_multi/$_state_name")" || exit 1
      [ "$_state_first" -eq 1 ] || printf ',\n'
      printf '    {"name":"%s","version":"%s","source":"%s","digest":"%s","digest_scope":"%s"}' "$(json_escape "$_state_name")" "$_state_version" "$_state_source" "$_state_digest" "$MANAGED_SKILL_DIGEST_SCOPE"
      _state_first=0
    done
    printf '\n  ],\n  "updated_at": "%s"\n}\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  } > "$_state_tmp" || { rm -f "$_state_tmp"; return 1; }
  mv "$_state_tmp" "$_state_root/skills-state.json"
}

# backup_and_record_skill_dir <victim> <manifest>
# Records exact original/backup pairs so a multi-set transaction can restore
# earlier moves when any later backup or publication fails.
backup_and_record_skill_dir() {
  _bars_victim="$1"
  _bars_manifest="$2"
  backup_and_remove_skill_dir "$_bars_victim" || return 1
  if [ -n "$DWS_LAST_SKILL_BACKUP" ]; then
    if ! printf '%s\n%s\n' "$_bars_victim" "$DWS_LAST_SKILL_BACKUP" >> "$_bars_manifest"; then
      mv "$DWS_LAST_SKILL_BACKUP" "$_bars_victim" 2>/dev/null || say "  ⚠️  备份记录失败且无法自动恢复: $_bars_victim（备份位于 $DWS_LAST_SKILL_BACKUP）"
      return 1
    fi
  fi
}

# restore_multi_skill_set <published-manifest> <backup-manifest>
# Removes partial new publications, then restores every old directory from
# its exact backup path. Paths containing newlines are outside the supported
# installer path contract; spaces are preserved.
restore_multi_skill_set() {
  _rms_published="$1"
  _rms_backups="$2"
  _rms_ok=1
  if [ -f "$_rms_published" ]; then
    while IFS= read -r _rms_dest; do
      [ -n "$_rms_dest" ] || continue
      rm -rf "$_rms_dest" || _rms_ok=0
    done < "$_rms_published"
  fi
  if [ -f "$_rms_backups" ]; then
    while IFS= read -r _rms_original && IFS= read -r _rms_backup; do
      [ -n "$_rms_backup" ] || continue
      if [ -e "$_rms_original" ] || ! mkdir -p "$(dirname "$_rms_original")" || ! mv "$_rms_backup" "$_rms_original"; then
        say "  ⚠️  无法恢复原 Skill: $_rms_original（备份保留于 $_rms_backup）"
        _rms_ok=0
      fi
    done < "$_rms_backups"
  fi
  [ "$_rms_ok" -eq 1 ]
}

# publish_skill_cache <source> <cache-dir>
# Copies a complete cache into a sibling staging directory, then publishes it
# with rename. Copy/publish failures retain the previous cache; if restoration
# itself fails, the recovery directory is reported and left untouched.
publish_skill_cache() {
  _psc_src="$1"
  _psc_cache="$2"
  _psc_parent="$(dirname "$_psc_cache")"
  _psc_name="$(basename "$_psc_cache")"
  _psc_stage=""
  _psc_old=""

  mkdir -p "$_psc_parent" || return 1
  _psc_stage="$(mktemp -d "$_psc_parent/.${_psc_name}.tmp.XXXXXX")" || return 1
  if ! cp -R "$_psc_src/." "$_psc_stage/" 2>/dev/null && \
     ! cp -r "$_psc_src/." "$_psc_stage/" 2>/dev/null; then
    rm -rf "$_psc_stage"
    return 1
  fi

  if [ -e "$_psc_cache" ]; then
    _psc_old="$(mktemp -d "$_psc_parent/.${_psc_name}.old.XXXXXX")" || {
      rm -rf "$_psc_stage"
      return 1
    }
    rmdir "$_psc_old" || {
      rm -rf "$_psc_stage" "$_psc_old"
      return 1
    }
    if ! mv "$_psc_cache" "$_psc_old"; then
      rm -rf "$_psc_stage"
      return 1
    fi
  fi

  if mv "$_psc_stage" "$_psc_cache"; then
    if [ -n "$_psc_old" ] && ! rm -rf "$_psc_old"; then
      say "  ⚠️ 新 Skill 缓存已生效，但旧缓存清理失败: $_psc_old"
    fi
    return 0
  fi

  rm -rf "$_psc_stage"
  if [ -n "$_psc_old" ] && ! mv "$_psc_old" "$_psc_cache"; then
    say "  ⚠️ Skill 缓存发布失败，原缓存保留在 $_psc_old"
  fi
  return 1
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
#   2. Interactive prompt when both stdin and stdout are TTYs (default: multi)
#   3. Fallback: multi (non-TTY without env var, e.g. curl | sh)
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
    say "  1) multi (default) — split each product into its own skill (dingtalk-*)"
    say "  2) mono            — install one bundled dws skill (legacy)"
    printf '  Choice [1]: '
    read choice || choice=""
    case "$choice" in
      ""|1|multi) SKILL_MODE="multi" ;;
      2|mono)     SKILL_MODE="mono" ;;
      *)
        say "Unrecognized choice '${choice}', defaulting to multi."
        SKILL_MODE="multi"
        ;;
    esac
    say "Skill mode: ${SKILL_MODE}"
    return 0
  fi

  SKILL_MODE="multi"
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

  if [ "$SKILL_MODE" = "multi" ] && multi_tree_has_skills "$multi_src"; then
    say ""
    say "📦 Installing agent skills (multi) from local source: ${multi_src}"
    install_multi_skills_to_homes "$multi_src"
  else
    if [ "$SKILL_MODE" = "multi" ]; then
      say "⚠️  Multi skill tree not found or empty at ${multi_src}; falling back to mono."
    fi
    if [ ! -d "$skill_src" ]; then
      say "⚠️  Local skills directory not found: ${skill_src}"
      say "   Skipping skills installation."
      return 1
    fi

    say ""
    say "📦 Installing agent skills from local source: ${skill_src}"

    install_skills_to_homes "$skill_src"
  fi

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

  # Never let an empty/corrupt multi/ tree wipe a previously good cache.
  multi_tree_has_skills "$src" || return 0

  if ! publish_skill_cache "$src" "$cache_dir"; then
    say "⚠️  Multi Skill 缓存刷新失败，未覆盖原缓存: ${cache_dir}"
    return 0
  fi

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

  # Only refresh when the new bundle actually carries a mono tree — a
  # multi-only bundle must never wipe a previously good mono cache.
  if [ ! -f "$src/SKILL.md" ]; then
    return 0
  fi

  if ! publish_skill_cache "$src" "$cache_dir"; then
    say "⚠️  Mono Skill 缓存刷新失败，未覆盖原缓存: ${cache_dir}"
  fi
}

# Publish mono and all mutually-exclusive managed multi directories as one
# transaction. The complete dws/ tree is staged before any visible directory
# moves; any later backup or publish failure restores the exact old set.
_install_mono_to_base() {
  _mono_src="$1"
  _mono_base="$2"
  _mono_label="$3"

  mkdir -p "$_mono_base" || return 1
  _mono_stage="$(mktemp -d "$_mono_base/.dws-mono-set.XXXXXX")" || return 1
  _mono_backups="$_mono_stage/.backups"
  _mono_published="$_mono_stage/.published"
  : > "$_mono_backups" || { rm -rf "$_mono_stage"; return 1; }
  : > "$_mono_published" || { rm -rf "$_mono_stage"; return 1; }
  mkdir -p "$_mono_stage/$SKILL_NAME" || { rm -rf "$_mono_stage"; return 1; }
  if ! cp -R "$_mono_src/." "$_mono_stage/$SKILL_NAME/" 2>/dev/null && ! cp -r "$_mono_src/." "$_mono_stage/$SKILL_NAME/"; then
    rm -rf "$_mono_stage"
    return 1
  fi

  if ! backup_and_record_skill_dir "$_mono_base/$SKILL_NAME" "$_mono_backups"; then
    restore_multi_skill_set "$_mono_published" "$_mono_backups" || true
    rm -rf "$_mono_stage"
    return 1
  fi
  for existing in "$_mono_base"/*/; do
    [ -d "$existing" ] || continue
    is_managed_multi_skill_dir "$existing" || continue
    if ! backup_and_record_skill_dir "$existing" "$_mono_backups"; then
      restore_multi_skill_set "$_mono_published" "$_mono_backups" || true
      rm -rf "$_mono_stage"
      return 1
    fi
  done

  _mono_dest="$_mono_base/$SKILL_NAME"
  printf '%s\n' "$_mono_dest" >> "$_mono_published" || {
    restore_multi_skill_set "$_mono_published" "$_mono_backups" || true
    rm -rf "$_mono_stage"
    return 1
  }
  if ! mv "$_mono_stage/$SKILL_NAME" "$_mono_dest"; then
    say "  ⚠️  mono Skill 集合发布失败，正在恢复原集合: $_mono_dest"
    restore_multi_skill_set "$_mono_published" "$_mono_backups" || say "  ⚠️  原 Skill 集合自动恢复不完整，请检查上方备份路径"
    rm -rf "$_mono_stage"
    return 1
  fi
  rm -rf "$_mono_stage" || return 1
  _mono_count="$(find "$_mono_dest" -type f | wc -l | tr -d ' ')"
  say "✅ Skills → ${_mono_label} (${_mono_count} files)"
}

# Move DWS-owned copies out of the generic root once a concrete Agent root is
# active. This prevents Agents such as Codex from discovering the same Skill
# through both ~/.agents/skills and ~/.codex/skills.
retire_generic_skill_root() {
  _rgs_root="$1"
  _rgs_base="$_rgs_root/.agents/skills"
  _rgs_stage="$(mktemp -d "${TMPDIR:-/tmp}/dws-retire-generic.XXXXXX")" || return 1
  _rgs_backups="$_rgs_stage/backups"
  : > "$_rgs_backups" || { rm -rf "$_rgs_stage"; return 1; }
  for _rgs_victim in "$_rgs_base/dws" "$_rgs_base"/*; do
    [ -d "$_rgs_victim" ] || continue
    if [ "$(basename "$_rgs_victim")" != "dws" ] && ! is_managed_multi_skill_dir "$_rgs_victim"; then
      continue
    fi
    if ! backup_and_record_skill_dir "$_rgs_victim" "$_rgs_backups"; then
      restore_multi_skill_set /dev/null "$_rgs_backups" || true
      rm -rf "$_rgs_stage"
      return 1
    fi
  done
  rm -rf "$_rgs_stage"
}

# Install skill tree into all agent homes (same rules as build/npm/install.js installSkillsToHomes).
# Installing mono removes proven DWS-managed multi leftovers for mutual exclusion,
# mirroring `dws skill setup --mode mono`.
install_skills_to_homes() {
  skill_src="$1"
  root="${HOME}"
  installed=0
  attempted=0
  failed=0
  idx=0
  specific_agents=0
  for specific_dir in \
    ".claude/skills" ".cursor/skills" ".qoder/skills" ".qoderwork/skills" \
    ".gemini/skills" ".codex/skills" ".zcode/skills" ".github/skills" ".windsurf/skills" \
    ".augment/skills" ".cline/skills" ".amp/skills" ".kiro/skills" \
    ".trae/skills" ".openclaw/skills" ".hermes/skills"
  do
    [ -e "$root/$(dirname "$specific_dir")" ] && specific_agents=$((specific_agents + 1))
  done
  for agent_dir in \
    ".agents/skills" \
    ".claude/skills" \
    ".cursor/skills" \
    ".qoder/skills" \
    ".qoderwork/skills" \
    ".gemini/skills" \
    ".codex/skills" \
    ".zcode/skills" \
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
    if [ "$idx" -eq 0 ] && [ "$specific_agents" -gt 0 ]; then
      idx=$((idx + 1))
      continue
    fi
    base_dir="$root/$agent_dir"
    parent_gate="$(dirname "$base_dir")"
    if [ "$idx" -gt 0 ] && [ ! -e "$parent_gate" ]; then
      idx=$((idx + 1))
      continue
    fi
    attempted=$((attempted + 1))
    case "$root" in
      "$HOME")
        label="~/$agent_dir/$SKILL_NAME"
        ;;
      *)
        label="$root/$agent_dir/$SKILL_NAME"
        ;;
    esac
    if _install_mono_to_base "$skill_src" "$base_dir" "$label"; then
      installed=$((installed + 1))
    else
      failed=$((failed + 1))
    fi
    idx=$((idx + 1))
  done
  if [ "$specific_agents" -gt 0 ] && [ "$installed" -gt 0 ]; then
    retire_generic_skill_root "$root" || failed=$((failed + 1))
  fi
  if [ "$attempted" -eq 0 ]; then
    case "$root" in
      "$HOME")
        flabel="~/.agents/skills/$SKILL_NAME"
        ;;
      *)
        flabel="$root/.agents/skills/$SKILL_NAME"
        ;;
    esac
    if _install_mono_to_base "$skill_src" "$root/.agents/skills" "$flabel"; then
      installed=$((installed + 1))
    else
      failed=$((failed + 1))
    fi
  fi
  if [ "$installed" -eq 0 ]; then
    say "  ⚠️  未安装任何 mono Skill：所有检测到的 Agent 目标均失败"
    return 1
  fi
  if [ "$failed" -gt 0 ]; then
    say "  ⚠️  有 ${failed} 个 Agent 目标安装 mono Skill 失败"
    return 1
  fi
  rm -f "$SKILL_STATE_ROOT/skills-state.json"
}

# multi_tree_has_skills returns 0 only when the given multi bundle directory
# contains at least one product skill (a subdirectory with a SKILL.md). An
# empty or corrupt multi/ tree must never select the multi branch: installing
# it would delete existing dws/ + dingtalk-* skills and lay down nothing.
# (Go bundleSkillNames and install.js multiTreeHasSkills guard the same way.)
multi_tree_has_skills() {
  _dir="$1"
  [ -d "$_dir" ] || return 1
  for _sub in "$_dir"/*/; do
    if [ -f "${_sub}SKILL.md" ]; then
      return 0
    fi
  done
  return 1
}

# Install the multi skill bundle (one subdirectory per product skill) into all
# agent homes as sibling directories, mirroring `dws skill setup --mode multi`.
# Mutual exclusion: the mono leftover (<home>/dws) and stale DWS-managed Skills
# not present in the new bundle are removed first.
install_multi_skills_to_homes() {
  multi_src="$1"
  root="${HOME}"
  installed=0
  attempted=0
  failed=0
  idx=0
  specific_agents=0
  for specific_dir in \
    ".claude/skills" ".cursor/skills" ".qoder/skills" ".qoderwork/skills" \
    ".gemini/skills" ".codex/skills" ".zcode/skills" ".github/skills" ".windsurf/skills" \
    ".augment/skills" ".cline/skills" ".amp/skills" ".kiro/skills" \
    ".trae/skills" ".openclaw/skills" ".hermes/skills"
  do
    [ -e "$root/$(dirname "$specific_dir")" ] && specific_agents=$((specific_agents + 1))
  done
  for agent_dir in \
    ".agents/skills" \
    ".claude/skills" \
    ".cursor/skills" \
    ".qoder/skills" \
    ".qoderwork/skills" \
    ".gemini/skills" \
    ".codex/skills" \
    ".zcode/skills" \
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
    if [ "$idx" -eq 0 ] && [ "$specific_agents" -gt 0 ]; then
      idx=$((idx + 1))
      continue
    fi
    base_dir="$root/$agent_dir"
    parent_gate="$(dirname "$base_dir")"
    if [ "$idx" -gt 0 ] && [ ! -e "$parent_gate" ]; then
      idx=$((idx + 1))
      continue
    fi
    attempted=$((attempted + 1))
    if _install_multi_to_base "$multi_src" "$base_dir" "$root" "$agent_dir"; then
      installed=$((installed + 1))
    else
      failed=$((failed + 1))
      say "  ⚠️  跳过 ${base_dir}（备份或复制失败，未完成 multi 安装）"
    fi
    idx=$((idx + 1))
  done
  if [ "$specific_agents" -gt 0 ] && [ "$installed" -gt 0 ]; then
    retire_generic_skill_root "$root" || failed=$((failed + 1))
  fi
  if [ "$attempted" -eq 0 ] && _install_multi_to_base "$multi_src" "$root/.agents/skills" "$root" ".agents/skills"; then
    installed=$((installed + 1))
  fi
  if [ "$installed" -eq 0 ]; then
    say "  ⚠️  未安装任何 multi Skill：所有检测到的 Agent 目标均失败"
    return 1
  fi
  if [ "$failed" -gt 0 ]; then
    say "  ⚠️  有 ${failed} 个 Agent 目标安装失败"
    return 1
  fi
  write_skills_state "$multi_src" "install.sh" || return 1
}

_install_multi_to_base() {
  _msrc="$1"
  _base="$2"
  _root="$3"
  _agent_dir="$4"

  mkdir -p "$_base" || return 1

  # Build the complete replacement set before moving any Agent-visible
  # directory. The manifests remain inside the private staging directory.
  _ms_stage="$(mktemp -d "$_base/.dws-multi-set.XXXXXX")" || return 1
  _ms_backups="$_ms_stage/.backups"
  _ms_published="$_ms_stage/.published"
  : > "$_ms_backups" || { rm -rf "$_ms_stage"; return 1; }
  : > "$_ms_published" || { rm -rf "$_ms_stage"; return 1; }
  for skill_dir in "$_msrc"/*/; do
    [ -f "${skill_dir}SKILL.md" ] || continue
    _name="$(basename "$skill_dir")"
    _ms_staged_skill="$_ms_stage/$_name"
    mkdir -p "$_ms_staged_skill" || { rm -rf "$_ms_stage"; return 1; }
    if ! cp -R "$skill_dir/." "$_ms_staged_skill/" 2>/dev/null && ! cp -r "$skill_dir/." "$_ms_staged_skill/"; then
      rm -rf "$_ms_stage"
      return 1
    fi
  done

  # Mutual exclusion: back up + remove the mono leftover.
  if ! backup_and_record_skill_dir "$_base/$SKILL_NAME" "$_ms_backups"; then
    restore_multi_skill_set "$_ms_published" "$_ms_backups" || true
    rm -rf "$_ms_stage"
    return 1
  fi

  # Back up + remove stale, proven DWS-managed skills not in the new bundle.
  # Never infer ownership from the dingtalk-* prefix alone.
  for existing in "$_base"/*/; do
    [ -d "$existing" ] || continue
    _name="$(basename "$existing")"
    if is_managed_multi_skill_dir "$existing" && [ ! -f "$_msrc/$_name/SKILL.md" ]; then
      if ! backup_and_record_skill_dir "$existing" "$_ms_backups"; then
        restore_multi_skill_set "$_ms_published" "$_ms_backups" || true
        rm -rf "$_ms_stage"
        return 1
      fi
    fi
  done
  if [ -d "$_base/dws-shared" ] && [ ! -f "$_msrc/dws-shared/SKILL.md" ]; then
    if ! backup_and_record_skill_dir "$_base/dws-shared" "$_ms_backups"; then
      restore_multi_skill_set "$_ms_published" "$_ms_backups" || true
      rm -rf "$_ms_stage"
      return 1
    fi
  fi

  # Back up all replaced skills as one logical operation. Any failure restores
  # every earlier move before this target reports failure.
  for skill_dir in "$_msrc"/*/; do
    [ -f "${skill_dir}SKILL.md" ] || continue
    _name="$(basename "$skill_dir")"
    _dest="$_base/$_name"
    if ! backup_and_record_skill_dir "$_dest" "$_ms_backups"; then
      restore_multi_skill_set "$_ms_published" "$_ms_backups" || true
      rm -rf "$_ms_stage"
      return 1
    fi
  done

  _count=0
  for skill_dir in "$_msrc"/*/; do
    [ -f "${skill_dir}SKILL.md" ] || continue
    _name="$(basename "$skill_dir")"
    _dest="$_base/$_name"
    printf '%s\n' "$_dest" >> "$_ms_published" || {
      restore_multi_skill_set "$_ms_published" "$_ms_backups" || true
      rm -rf "$_ms_stage"
      return 1
    }
    if ! mv "$_ms_stage/$_name" "$_dest"; then
      say "  ⚠️  multi Skill 集合发布失败，正在恢复原集合: $_dest"
      restore_multi_skill_set "$_ms_published" "$_ms_backups" || say "  ⚠️  原 Skill 集合自动恢复不完整，请检查上方备份路径"
      rm -rf "$_ms_stage"
      return 1
    fi
    _count=$((_count + 1))
  done
  rm -rf "$_ms_stage" || return 1

  case "$_root" in
    "$HOME") _label="~/$_agent_dir/" ;;
    *)       _label="$_root/$_agent_dir/" ;;
  esac
  say "✅ Skills → ${_label} (${_count} product skills)"
}

# One-line summary copy (used for 2nd+ agent targets).
_copy_skill_summary() {
  _src="$1"
  _dest="$2"
  _label="$3"

  if [ -d "$_dest" ]; then
    backup_and_remove_skill_dir "$_dest" || {
      say "  ⚠️  跳过 ${_dest}（保留原目录）"
      return 1
    }
  fi

  mkdir -p "$_dest" || return 1
  if ! cp -R "$_src/"* "$_dest/" 2>/dev/null && ! cp -r "$_src/"* "$_dest/"; then
    say "  ⚠️  Skill 复制失败，目标未计为安装成功: $_dest"
    return 1
  fi
  file_count="$(find "$_dest" -type f | wc -l | tr -d ' ')"

  say "✅ Skills → ${_label} (${file_count} files)"
}

# Helper: copy skill files to a destination and print details
_copy_skill() {
  _src="$1"
  _dest="$2"
  _label="$3"

  if [ -d "$_dest" ]; then
    backup_and_remove_skill_dir "$_dest" || {
      say "  ⚠️  跳过 ${_dest}（保留原目录）"
      return 1
    }
  fi

  mkdir -p "$_dest" || return 1
  if ! cp -R "$_src/"* "$_dest/" 2>/dev/null && ! cp -r "$_src/"* "$_dest/"; then
    say "  ⚠️  Skill 复制失败，目标未计为安装成功: $_dest"
    return 1
  fi
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
  # Multi first: a release may ship only the multi/ tree without the root
  # mono copy, so the mono SKILL.md gate must never block a multi install.
  # An empty/corrupt multi/ tree (no */SKILL.md) falls back to mono with a
  # warning — installing it would wipe existing skills and lay down nothing.
  if [ "$SKILL_MODE" = "multi" ] && multi_tree_has_skills "$extract_root/multi"; then
    install_multi_skills_to_homes "$extract_root/multi"
  else
    if [ "$SKILL_MODE" = "multi" ]; then
      say "⚠️  Multi skill tree not found or empty in release asset; falling back to mono."
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
  fi

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
      install_skills_local "$source_root"
    fi
  elif [ "$SKILLS_ONLY" = "1" ]; then
    local_root="$(resolve_source_root || true)"
    if [ -n "$local_root" ]; then
      install_skills_local "$local_root"
    else
      install_skills
    fi
  elif [ "$NO_SKILLS" = "1" ]; then
    install_binary
  else
    install_binary
    install_skills
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
