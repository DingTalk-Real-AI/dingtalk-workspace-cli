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
  [ -e "$_bed_dir" ] || [ -L "$_bed_dir" ] || return 0
  _bed_root="${HOME}/.dws/skill-backups"
  _bed_stamp="$(date -u +%Y%m%d-%H%M%S)"
  _bed_name="$(basename "$_bed_dir")"
  _bed_stamp_root="$_bed_root/$_bed_stamp"
  _bed_target="$_bed_stamp_root/$_bed_name"
  _bed_i=1
  # Bump not only when the payload path is taken but also when the stamp
  # root exists without a verified ownership marker: a same-second foreign
  # directory must never be stamped DWS-owned and made prunable. A
  # marker-verified root from this run's same second stays reusable.
  while [ -e "$_bed_target" ] || [ -L "$_bed_target" ] ||
    { [ -d "$_bed_stamp_root" ] && ! is_current_run_backup_stamp "$_bed_stamp_root" &&
      [ "$(cat "$_bed_stamp_root/.dws-skill-backup" 2>/dev/null)" != "dws skill backup v1" ]; }; do
    _bed_stamp_root="$_bed_root/$_bed_stamp-$_bed_i"
    _bed_target="$_bed_stamp_root/$_bed_name"
    _bed_i=$((_bed_i + 1))
    if [ "$_bed_i" -gt 1000 ]; then
      say "  ⚠️  备份目录冲突，保留原目录 $_bed_dir"
      return 1
    fi
  done
  _bed_stamp_root="$(dirname "$_bed_target")"
  record_current_run_backup_stamp "$_bed_stamp_root"
  # Freshness must be sampled before mkdir: the collision loop tests the
  # payload path, so a second backup in the same stamp second reuses this
  # root while a sibling payload from this run already lives in it.
  _bed_fresh=1
  [ ! -d "$_bed_stamp_root" ] || _bed_fresh=0
  mkdir -p "$_bed_stamp_root" 2>/dev/null || {
    say "  ⚠️  无法创建备份目录，保留原目录 $_bed_dir"
    return 1
  }
  # Ownership proof, the exact bytes Go's writeSkillBackupMarker stamps: a
  # stamp-shaped name alone is not evidence, so pruning only ever removes
  # roots carrying this marker — it must exist before any payload moves in.
  printf '%s\n' 'dws skill backup v1' > "$_bed_stamp_root/.dws-skill-backup" 2>/dev/null || {
    # Non-recursive cleanup, sibling protection: only a root this call
    # created may be dropped, and only the marker plus an empty root (rmdir
    # refuses a non-empty directory) — rm -rf here would destroy a completed
    # same-second sibling backup whose original is already moved away.
    if [ "$_bed_fresh" -eq 1 ]; then
      rm -f "$_bed_stamp_root/.dws-skill-backup"
      rmdir "$_bed_stamp_root" 2>/dev/null || true
    fi
    say "  ⚠️  无法写入备份所有权标记，保留原目录 $_bed_dir"
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

# prune_skill_backups keeps only the newest DWS_SKILL_BACKUP_KEEP stamped
# backup directories from earlier runs under $HOME/.dws/skill-backups,
# matching Go's skillBackupKeep / pruneSkillBackups. Only stamp-shaped roots
# carrying the .dws-skill-backup marker with the exact expected content are
# counted or removed: foreign data with a stamp-like name never consumes a
# keep slot and is never deleted, silently. Stamp directories created
# by the current process are never pruned (mirroring Go's run-root registry
# and install.js's currentRunBackupRoots), so a migration that retires more
# than DWS_SKILL_BACKUP_KEEP batches stays reversible. Best-effort: a removal
# failure is reported, never fatal.
DWS_SKILL_BACKUP_KEEP=5

# Stamp directories created by this run, recorded by
# backup_and_remove_skill_dir so pruning can never delete a backup the
# running migration may still need to roll back to.
DWS_CURRENT_RUN_BACKUP_STAMPS=""

record_current_run_backup_stamp() {
  case " $DWS_CURRENT_RUN_BACKUP_STAMPS " in
    *" $1 "*) return 0 ;;
  esac
  DWS_CURRENT_RUN_BACKUP_STAMPS="${DWS_CURRENT_RUN_BACKUP_STAMPS} $1"
}

# is_current_run_backup_stamp reports whether the stamp root was created by
# this very process. Such a root is ours by construction and stays reusable
# even when its marker cannot be re-verified mid-run (for example a
# permission failure after the first payload moved in).
is_current_run_backup_stamp() {
  case " $DWS_CURRENT_RUN_BACKUP_STAMPS " in
    *" $1 "*) return 0 ;;
  esac
  return 1
}

# is_skill_backup_stamp accepts only directory names with the stamp shape DWS
# itself writes: UTC YYYYmmdd-HHMMSS, with an optional -N collision suffix.
# Shape is necessary but not sufficient — pruning additionally verifies the
# .dws-skill-backup ownership marker — while any other entry in the backup
# root is foreign (user data, unrelated tooling) and is preserved so pruning
# can never remove a directory it cannot prove DWS created.
is_skill_backup_stamp() {
  case "$1" in
    [0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9]-[0-9][0-9][0-9][0-9][0-9][0-9])
      return 0 ;;
    [0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9]-[0-9][0-9][0-9][0-9][0-9][0-9]-*)
      # base stamp is YYYYMMDD-HHMMSS (15 chars) + suffix dash (16th); strip
      # those 16 chars and require the remainder to be all digits.
      _isbs_suffix="${1#????????????????}"
      case "$_isbs_suffix" in
        ""|*[!0-9]*) return 1 ;;
      esac
      return 0 ;;
  esac
  return 1
}

prune_skill_backups() {
  _psb_root="${HOME}/.dws/skill-backups"
  [ -d "$_psb_root" ] || return 0
  _psb_total=0
  for _psb_entry in "$_psb_root"/*; do
    [ -d "$_psb_entry" ] || continue
    is_skill_backup_stamp "${_psb_entry##*/}" || continue
    [ "$(cat "$_psb_entry/.dws-skill-backup" 2>/dev/null)" = "dws skill backup v1" ] || continue
    _psb_total=$((_psb_total + 1))
  done
  [ "$_psb_total" -gt "$DWS_SKILL_BACKUP_KEEP" ] || return 0
  _psb_drop=$((_psb_total - DWS_SKILL_BACKUP_KEEP))
  for _psb_entry in "$_psb_root"/*; do
    [ "$_psb_drop" -gt 0 ] || break
    [ -d "$_psb_entry" ] || continue
    is_skill_backup_stamp "${_psb_entry##*/}" || continue
    [ "$(cat "$_psb_entry/.dws-skill-backup" 2>/dev/null)" = "dws skill backup v1" ] || continue
    case " $DWS_CURRENT_RUN_BACKUP_STAMPS " in
      *" $_psb_entry "*) continue ;;
    esac
    rm -rf "$_psb_entry" || say "  ⚠️  旧 Skill 备份清理失败: $_psb_entry"
    _psb_drop=$((_psb_drop - 1))
  done
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

# sha256_file <path>: content digest used by skill_dir_content_digest. Never
# emits on failure. Callers that capture its status directly detect that
# failure; skill_dir_content_digest cannot — see the pipeline caveat there:
# POSIX sh returns only the right-hand pipeline status, so a subshell exit on
# its left side is discarded and digest-failure detection stays best-effort.
sha256_file() {
  if need_cmd sha256sum; then
    sha256sum "$1" 2>/dev/null | awk '{print $1}'
  elif need_cmd shasum; then
    shasum -a 256 "$1" 2>/dev/null | awk '{print $1}'
  elif need_cmd openssl; then
    openssl dgst -sha256 -r "$1" 2>/dev/null | awk '{print $1}'
  else
    return 1
  fi
}

verify_release_asset_checksum() {
  _checksum_asset="$1"
  _checksum_path="$2"
  _checksum_dir="$3"
  _checksum_url="$(asset_url checksums.txt)"
  [ -n "$_checksum_url" ] || err "Could not resolve checksums.txt for ${VERSION}."
  download "$_checksum_url" "$_checksum_dir/checksums.txt" 2>/dev/null || \
    err "Could not download checksums.txt for ${VERSION}; refusing unverified ${_checksum_asset}."
  _checksum_expected="$(awk -v file="$_checksum_asset" '$2 == file {print $1; exit}' "$_checksum_dir/checksums.txt")"
  [ -n "$_checksum_expected" ] || err "${_checksum_asset} is missing from checksums.txt."
  _checksum_actual="$(sha256_stdin < "$_checksum_path")" || \
    err "Could not compute SHA256 for ${_checksum_asset}; install sha256sum, shasum, or openssl."
  [ "$_checksum_actual" = "$_checksum_expected" ] || \
    err "SHA256 checksum mismatch for ${_checksum_asset}. Expected ${_checksum_expected}, got ${_checksum_actual}."
  say "✅ SHA256 checksum verified: ${_checksum_asset}"
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
      mv "$DWS_LAST_SKILL_BACKUP" "$_bars_victim" 2>/dev/null || say "  ⚠️  备份记录失败且无法自动恢复: ${_bars_victim}（备份位于 ${DWS_LAST_SKILL_BACKUP}）"
      return 1
    fi
  fi
}

# restore_quarantine_no_replace <quarantined> <dest>
# Puts an unmatched quarantined object back onto dest without replacing
# anything that occupied dest after the claim. Directories use mkdir-claim
# plus child moves; links are created at dest; files hard-link then drop
# the quarantine copy.
restore_quarantine_no_replace() {
  _rqn_src="$1"
  _rqn_dest="$2"
  if [ -L "$_rqn_src" ]; then
    _rqn_target="$(readlink "$_rqn_src")" || return 1
    if ! ln -s "$_rqn_target" "$_rqn_dest" 2>/dev/null || ! [ -L "$_rqn_dest" ]; then
      _rqn_nested="$_rqn_dest/${_rqn_target##*/}"
      if [ -L "$_rqn_nested" ] && [ "$(readlink "$_rqn_nested" 2>/dev/null)" = "$_rqn_target" ]; then
        rm -f "$_rqn_nested"
      fi
      return 1
    fi
    rm -f "$_rqn_src"
    return 0
  fi
  if [ -d "$_rqn_src" ]; then
    mkdir "$_rqn_dest" 2>/dev/null || return 1
    _rqn_failed=0
    for _rqn_child in "$_rqn_src"/* "$_rqn_src"/.[!.]* "$_rqn_src"/..?*; do
      [ -e "$_rqn_child" ] || [ -L "$_rqn_child" ] || continue
      if ! mv "$_rqn_child" "$_rqn_dest/"; then
        _rqn_failed=1
        break
      fi
    done
    if [ "$_rqn_failed" -eq 0 ]; then
      rmdir "$_rqn_src" 2>/dev/null || return 1
      return 0
    fi
    for _rqn_child in "$_rqn_dest"/* "$_rqn_dest"/.[!.]* "$_rqn_dest"/..?*; do
      [ -e "$_rqn_child" ] || [ -L "$_rqn_child" ] || continue
      mv "$_rqn_child" "$_rqn_src/" || return 1
    done
    rmdir "$_rqn_dest" 2>/dev/null || return 1
    return 1
  fi
  if [ -f "$_rqn_src" ]; then
    # Shell `ln` treats an occupied directory as a container. Refuse that
    # dest and drop any nested name instead of leaving a hardlink inside it.
    if ! ln "$_rqn_src" "$_rqn_dest" 2>/dev/null || [ -d "$_rqn_dest" ]; then
      _rqn_nested="$_rqn_dest/$(basename "$_rqn_src")"
      [ -f "$_rqn_nested" ] && rm -f "$_rqn_nested"
      return 1
    fi
    rm -f "$_rqn_src"
    return 0
  fi
  return 1
}

# quarantine_retract_or_restore <dest> <expected-inode> [expected-link-target]
# Prove ownership at dest first (inode plus child names for directories).
# Quarantining before that check would move a concurrent replacement off its
# original path. Linux overlayfs recycles inodes, so inode alone is not
# enough — a dest whose children no longer match the recorded publication
# stays put. After a matching dest is claimed, identity is re-checked in
# quarantine; a mismatch is restored with no-replace.
quarantine_retract_or_restore() {
  _qrr_dest="$1"
  _qrr_inode="$2"
  _qrr_link_target="${3-}"
  if [ ! -e "$_qrr_dest" ] && [ ! -L "$_qrr_dest" ]; then
    return 0
  fi
  _qrr_owned=0
  if [ -n "$_qrr_link_target" ]; then
    if [ -L "$_qrr_dest" ] &&
       [ "$(readlink "$_qrr_dest" 2>/dev/null)" = "$_qrr_link_target" ] &&
       [ "$(skill_link_inode "$_qrr_dest")" = "$_qrr_inode" ]; then
      _qrr_owned=1
    fi
  elif [ "$(skill_dir_identity "$_qrr_dest")" = "$_qrr_inode" ]; then
    _qrr_owned=1
  fi
  if [ "$_qrr_owned" -eq 0 ]; then
    say "  ⚠️  跳过回滚已被并发修改的 Skill 路径: $_qrr_dest"
    return 1
  fi
  _qrr_parent="$(dirname "$_qrr_dest")"
  _qrr_base="$(basename "$_qrr_dest")"
  _qrr_root="$(mktemp -d "$_qrr_parent/.${_qrr_base}.rollback.XXXXXX")" || return 1
  _qrr_payload="$_qrr_root/payload"
  if ! mv "$_qrr_dest" "$_qrr_payload"; then
    rmdir "$_qrr_root" 2>/dev/null || rm -rf "$_qrr_root"
    if [ ! -e "$_qrr_dest" ] && [ ! -L "$_qrr_dest" ]; then
      return 0
    fi
    say "  ⚠️  无法隔离待回滚 Skill: $_qrr_dest"
    return 1
  fi
  _qrr_owned=0
  if [ -n "$_qrr_link_target" ]; then
    if [ -L "$_qrr_payload" ] &&
       [ "$(readlink "$_qrr_payload" 2>/dev/null)" = "$_qrr_link_target" ] &&
       [ "$(skill_link_inode "$_qrr_payload")" = "$_qrr_inode" ]; then
      _qrr_owned=1
    fi
  elif [ "$(skill_dir_identity "$_qrr_payload")" = "$_qrr_inode" ]; then
    _qrr_owned=1
  fi
  if [ "$_qrr_owned" -eq 1 ]; then
    rm -rf "$_qrr_root" || return 1
    return 0
  fi
  if restore_quarantine_no_replace "$_qrr_payload" "$_qrr_dest"; then
    rmdir "$_qrr_root" 2>/dev/null || true
    say "  ⚠️  跳过回滚已被并发修改的 Skill 路径: $_qrr_dest"
    return 1
  fi
  say "  ⚠️  跳过回滚已被并发修改的 Skill 路径: $_qrr_dest（并发对象保留于 $_qrr_payload）"
  return 1
}

# restore_multi_skill_set <published-manifest> <backup-manifest>
# Removes partial new publications, then restores every old directory from
# its exact backup path. Publication manifest entries are <dest>:<inode>
# pairs recorded at publish time. Dest is claimed into quarantine first;
# identity is re-checked there so a concurrent replacement is renamed back
# instead of being deleted by path. Paths containing newlines are outside
# the supported installer path contract; spaces are preserved.
restore_multi_skill_set() {
  _rms_published="$1"
  _rms_backups="$2"
  _rms_ok=1
  if [ -f "$_rms_published" ]; then
    while IFS= read -r _rms_entry; do
      [ -n "$_rms_entry" ] || continue
      _rms_dest="${_rms_entry%:*}"
      _rms_inode="${_rms_entry##*:}"
      [ "$_rms_dest" != "$_rms_entry" ] || { say "  ⚠️  跳过格式异常的发布记录: $_rms_entry"; _rms_ok=0; continue; }
      quarantine_retract_or_restore "$_rms_dest" "$_rms_inode" || _rms_ok=0
    done < "$_rms_published"
  fi
  if [ -f "$_rms_backups" ]; then
    while IFS= read -r _rms_original && IFS= read -r _rms_backup; do
      [ -n "$_rms_backup" ] || continue
      if ! mkdir -p "$(dirname "$_rms_original")" || ! restore_quarantine_no_replace "$_rms_backup" "$_rms_original"; then
        say "  ⚠️  无法恢复原 Skill: ${_rms_original}（备份保留于 ${_rms_backup}）"
        _rms_ok=0
      fi
    done < "$_rms_backups"
  fi
  [ "$_rms_ok" -eq 1 ]
}

# A link publication manifest stores destination/target/inode triples. Rollback
# only removes links that still have the identity created by this transaction;
# a path concurrently replaced with a file, directory, or new link is preserved.
skill_link_inode() {
  _sli_entry="$(ls -di "$1" 2>/dev/null)" || return 1
  printf '%s\n' "$_sli_entry" | awk '{print $1}'
}

# Directory publication identity is inode, a stable child-name list, and a
# recursive content digest. Inode reuse on Linux overlayfs would otherwise
# make a concurrent directory look owned, and an in-place content edit after
# publish (same inode, same names) would otherwise be retracted as this
# transaction's object. The token has no colon so dest:identity manifest
# parsing stays dest="${entry%:*}" / identity="${entry##*:}".
skill_dir_identity() {
  _sdi_inode="$(skill_link_inode "$1")" || return 1
  if [ -L "$1" ] || [ ! -d "$1" ]; then
    printf '%s\n' "$_sdi_inode"
    return 0
  fi
  _sdi_names="$(LC_ALL=C ls -A "$1" 2>/dev/null | tr '\n' ',')"
  _sdi_digest="$(skill_dir_content_digest "$1")" || return 1
  printf '%s,%s,%s\n' "$_sdi_inode" "$_sdi_names" "$_sdi_digest"
}

# Recursive content digest over a published tree: for each path (LC_ALL=C
# sorted, relative) emit path, type+permission bits from ls -ld, and either
# the sha256 of the file content or the symlink target. Sorted-path lines are
# piped through sha256_stdin, so one opaque token covers renames, mode flips,
# content edits and link retargets — the same in-place-edit guarantee the Go
# fingerprint provides. Failure detection is asymmetric and best-effort: with
# no hash tool installed sha256_stdin itself returns 1 and the identity
# genuinely fails, but an unreadable file only exits the left-hand subshell —
# /bin/sh has no pipefail, so the pipeline returns sha256_stdin's status and
# the digest is computed over a truncated stream instead of failing.
skill_dir_content_digest() {
  (
    cd "$1" 2>/dev/null || exit 1
    find . -print 2>/dev/null |
      LC_ALL=C sort |
      while IFS= read -r _sdcd_path; do
        _sdcd_mode="$(ls -ld "$_sdcd_path" 2>/dev/null | cut -c1-10)"
        if [ -L "$_sdcd_path" ]; then
          printf '%s|%s|link|%s\n' "$_sdcd_path" "$_sdcd_mode" "$(readlink "$_sdcd_path" 2>/dev/null)"
        elif [ -d "$_sdcd_path" ]; then
          printf '%s|%s|dir\n' "$_sdcd_path" "$_sdcd_mode"
        elif [ -f "$_sdcd_path" ]; then
          _sdcd_hash="$(sha256_file "$_sdcd_path")" || exit 1
          printf '%s|%s|file|%s\n' "$_sdcd_path" "$_sdcd_mode" "$_sdcd_hash"
        else
          printf '%s|%s|other\n' "$_sdcd_path" "$_sdcd_mode"
        fi
      done
  ) | sha256_stdin
}

skill_link_matches() {
  [ -L "$1" ] || return 1
  _slm_target="$(readlink "$1" 2>/dev/null)" || return 1
  [ "$_slm_target" = "$2" ] || return 1
  _slm_expected_inode="${3-}"
  [ -z "$_slm_expected_inode" ] || [ "$(skill_link_inode "$1")" = "$_slm_expected_inode" ]
}

restore_linked_skill_set() {
  _rls_published="$1"
  _rls_backups="$2"
  _rls_ok=1
  if [ -f "$_rls_published" ]; then
    while IFS= read -r _rls_dest && IFS= read -r _rls_target && IFS= read -r _rls_inode; do
      [ -n "$_rls_dest" ] || continue
      quarantine_retract_or_restore "$_rls_dest" "$_rls_inode" "$_rls_target" || _rls_ok=0
    done < "$_rls_published"
  fi
  restore_multi_skill_set /dev/null "$_rls_backups" || _rls_ok=0
  [ "$_rls_ok" -eq 1 ]
}

cleanup_nested_staged_link() {
  _cnsl_nested="$1/$2"
  _cnsl_target="$3"
  _cnsl_inode="${4-}"
  if skill_link_matches "$_cnsl_nested" "$_cnsl_target" "$_cnsl_inode"; then
    rm -f "$_cnsl_nested"
  fi
}

# publish_skill_dir_no_replace <staged-dir> <dest> <published-manifest>
# Publishes a staged Skill directory with an atomic no-replace claim: mkdir
# fails with EEXIST when anything a concurrent writer created occupies the
# destination, so the claim itself is the existence check — plain `mv` after a
# backup could still replace a concurrently created object. Staged children
# then move into the claim one by one (each rename targets a path inside a
# directory this transaction owns, so no step replaces a foreign object; the
# staging directory is created under the same umask as the claim, so their
# modes match by construction). A failed child move relocates the children
# back and removes only the claim. On success the destination's inode is
# recorded as <dest>:<inode,child-names,content-digest> so rollback only ever
# deletes this transaction's object even when the filesystem recycles the
# claim inode or the published files are edited in place afterwards.
publish_skill_dir_no_replace() {
  _psd_stage="$1"
  _psd_dest="$2"
  _psd_manifest="$3"

  if ! mkdir "$_psd_dest" 2>/dev/null; then
    return 1
  fi
  _psd_failed=0
  for _psd_child in "$_psd_stage"/* "$_psd_stage"/.[!.]* "$_psd_stage"/..?*; do
    [ -e "$_psd_child" ] || [ -L "$_psd_child" ] || continue
    if ! mv "$_psd_child" "$_psd_dest/"; then
      _psd_failed=1
      break
    fi
  done
  if [ "$_psd_failed" -eq 0 ]; then
    _psd_inode="$(skill_dir_identity "$_psd_dest")" || _psd_failed=1
    if [ "$_psd_failed" -eq 0 ] && printf '%s:%s\n' "$_psd_dest" "$_psd_inode" >> "$_psd_manifest"; then
      return 0
    fi
    _psd_failed=1
  fi
  for _psd_child in "$_psd_dest"/* "$_psd_dest"/.[!.]* "$_psd_dest"/..?*; do
    [ -e "$_psd_child" ] || [ -L "$_psd_child" ] || continue
    mv "$_psd_child" "$_psd_stage/" || return 1
  done
  rmdir "$_psd_dest" 2>/dev/null || return 1
  return 1
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
  staged_bin="$INSTALL_DIR/.${INSTALL_NAME}.tmp.$$"
  cp "$built_bin" "$staged_bin"
  chmod +x "$staged_bin"
  mv "$staged_bin" "$INSTALL_DIR/$INSTALL_NAME"

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
  if ! publish_skill_dir_no_replace "$_mono_stage/$SKILL_NAME" "$_mono_dest" "$_mono_published"; then
    say "  ⚠️  mono Skill 集合发布失败，正在恢复原集合: $_mono_dest"
    restore_multi_skill_set "$_mono_published" "$_mono_backups" || say "  ⚠️  原 Skill 集合自动恢复不完整，请检查上方备份路径"
    rm -rf "$_mono_stage"
    return 1
  fi
  rm -rf "$_mono_stage" || return 1
  _mono_count="$(find "$_mono_dest" -type f | wc -l | tr -d ' ')"
  say "✅ Skills → ${_mono_label} (${_mono_count} files)"
}

# Exact upstream registry (76 IDs): id|classification|effective-global-root.
# `-` means no global directory; `.agents/skills` means canonical-direct.
# Not readonly: the library must stay re-sourceable inside one shell.
DWS_UPSTREAM_AGENT_REGISTRY='aider-desk|N|.aider-desk/skills amp|U|.config/agents/skills antigravity|U|.gemini/antigravity/skills antigravity-cli|U|.gemini/antigravity-cli/skills astrbot|N|.astrbot/data/skills autohand-code|N|.autohand/skills augment|N|.augment/skills bob|N|.bob/skills claude-code|N|.claude/skills openclaw|N|.openclaw/skills cline|U|.agents/skills codearts-agent|N|.codeartsdoer/skills codebuddy|N|.codebuddy/skills codemaker|N|.codemaker/skills codestudio|N|.codestudio/skills codex|U|.codex/skills command-code|N|.commandcode/skills continue|N|.continue/skills cortex|N|.snowflake/cortex/skills crush|N|.config/crush/skills cursor|U|.cursor/skills deepagents|U|.deepagents/agent/skills devin|N|.config/devin/skills dexto|U|.agents/skills droid|N|.factory/skills eve|N|- firebender|U|.firebender/skills forgecode|N|.forge/skills gemini-cli|U|.gemini/skills github-copilot|U|.copilot/skills goose|N|.config/goose/skills grok|N|.grok/skills hermes-agent|N|.hermes/skills inference-sh|N|.inferencesh/skills jazz|N|.jazz/skills junie|N|.junie/skills iflow-cli|N|.iflow/skills kilo|N|.kilocode/skills kimchi|N|.config/kimchi/harness/skills kimi-code-cli|U|.agents/skills kiro-cli|N|.kiro/skills kode|N|.kode/skills lingma|N|.lingma/skills loaf|U|.agents/skills mcpjam|N|.mcpjam/skills minimax-code|N|.minimax/skills mistral-vibe|N|.vibe/skills moxby|N|.moxby/skills mux|N|.mux/skills opencode|U|.config/opencode/skills openhands|N|.openhands/skills ona|N|.ona/skills pi|N|.pi/agent/skills qoder|N|.qoder/skills qoder-cn|N|.qoder-cn/skills qwen-code|N|.qwen/skills replit|U|.config/agents/skills reasonix|N|.reasonix/skills rovodev|N|.rovodev/skills roo|N|.roo/skills tabnine-cli|N|.tabnine/agent/skills terramind|N|.terramind/skills tinycloud|N|.tinycloud/skills trae|N|.trae/skills trae-cn|N|.trae-cn/skills warp|U|.agents/skills windsurf|N|.codeium/windsurf/skills zed|U|.agents/skills zcode|N|.zcode/skills zencoder|N|.zencoder/skills zenflow|N|.zencoder/skills neovate|N|.neovate/skills pochi|N|.pochi/skills promptscript|U|- adal|N|.adal/skills universal|U|.config/agents/skills'
upstream_agent_registry() {
  for _uar_record in $DWS_UPSTREAM_AGENT_REGISTRY; do printf '%s\n' "$_uar_record"; done
}

# DWS-only compatibility roots, same id|classification|root format. Qoderwork
# stays a real non-universal install target; the other four are global paths
# older DWS installers wrote by mistake, so they are retired like universal
# roots. Kept separate so the upstream pin above stays byte-comparable.
DWS_LEGACY_AGENT_ROOTS='dws-qoderwork|N|.qoderwork/skills dws-legacy-github|U|.github/skills dws-legacy-windsurf|U|.windsurf/skills dws-legacy-cline|U|.cline/skills dws-legacy-amp|U|.amp/skills'
legacy_agent_roots() {
  for _lar_record in $DWS_LEGACY_AGENT_ROOTS; do printf '%s\n' "$_lar_record"; done
}

# ~/.agents/skills is the canonical store under the universal convention.
# Universal Agents read it directly; other Agents receive relative links and
# fall back to copies only when links are unavailable.
#
# Both agent_skill_dirs and is_universal_agent_dir are DERIVED from the pinned
# registries above, so a registry edit changes real install behaviour instead of
# only a test count, and pin/behaviour drift is impossible by construction.
is_universal_agent_dir() {
  for _iua_record in $DWS_UPSTREAM_AGENT_REGISTRY $DWS_LEGACY_AGENT_ROOTS; do
    [ "${_iua_record##*|}" = "$1" ] || continue
    case "$_iua_record" in
      *"|U|"*) return 0 ;;
    esac
  done
  return 1
}

# Effective global roots from vercel-labs/skills, in pinned registry order and
# de-duplicated (amp/replit/universal and zencoder/zenflow share a root).
# Records without a global root (`-`) and canonical-direct records
# (`.agents/skills`) are not per-Agent targets and are skipped.
agent_skill_dirs() {
  { upstream_agent_registry; legacy_agent_roots; } | awk -F'|' '
    $3 != "-" && $3 != ".agents/skills" && !seen[$3]++ { print $3 }'
}

resolve_agent_skill_base() {
  _ras_root="$1"; _ras_agent="$2"
  case "$_ras_agent" in
    ".claude/skills") [ -n "${CLAUDE_CONFIG_DIR:-}" ] && { printf '%s\n' "$CLAUDE_CONFIG_DIR/skills"; return; } ;;
    ".codex/skills") [ -n "${CODEX_HOME:-}" ] && { printf '%s\n' "$CODEX_HOME/skills"; return; } ;;
    ".hermes/skills") [ -n "${HERMES_HOME:-}" ] && { printf '%s\n' "$HERMES_HOME/skills"; return; } ;;
    ".autohand/skills") [ -n "${AUTOHAND_HOME:-}" ] && { printf '%s\n' "$AUTOHAND_HOME/skills"; return; } ;;
    ".grok/skills") [ -n "${GROK_HOME:-}" ] && { printf '%s\n' "$GROK_HOME/skills"; return; } ;;
    ".vibe/skills") [ -n "${VIBE_HOME:-}" ] && { printf '%s\n' "$VIBE_HOME/skills"; return; } ;;
    ".openclaw/skills")
      for _ras_name in .openclaw .clawdbot .moltbot; do
        [ -d "$_ras_root/$_ras_name" ] && { printf '%s\n' "$_ras_root/$_ras_name/skills"; return; }
      done ;;
    ".config/opencode/skills") printf '%s\n' "${XDG_CONFIG_HOME:-$_ras_root/.config}/opencode/skills"; return ;;
    ".config/agents/skills") printf '%s\n' "${XDG_CONFIG_HOME:-$_ras_root/.config}/agents/skills"; return ;;
    ".config/crush/skills") printf '%s\n' "${XDG_CONFIG_HOME:-$_ras_root/.config}/crush/skills"; return ;;
    ".config/devin/skills") printf '%s\n' "${XDG_CONFIG_HOME:-$_ras_root/.config}/devin/skills"; return ;;
    ".config/goose/skills") printf '%s\n' "${XDG_CONFIG_HOME:-$_ras_root/.config}/goose/skills"; return ;;
    ".config/kimchi/harness/skills") printf '%s\n' "${XDG_CONFIG_HOME:-$_ras_root/.config}/kimchi/harness/skills"; return ;;
  esac
  printf '%s\n' "$_ras_root/$_ras_agent"
}

agent_skill_base_detected() {
  _asbd_agent="$1"; _asbd_base="$2"
  case "$_asbd_agent" in
    ".config/kimchi/harness/skills"|".tabnine/agent/skills") [ -d "$(dirname "$(dirname "$_asbd_base")")" ] ;;
    ".zcode/skills") [ -d "$(dirname "$_asbd_base")" ] || [ -d "/Applications/ZCode.app" ] ;;
    ".minimax/skills") [ -d "$(dirname "$_asbd_base")" ] || [ -d "/Applications/MiniMax Code.app" ] ;;
    *) [ -d "$(dirname "$_asbd_base")" ] ;;
  esac
}

same_physical_skill_root() {
  [ -d "$1" ] && [ -d "$2" ] || return 1
  # CDPATH= and -- are required: an exported CDPATH makes `cd` echo the resolved
  # directory into the command substitution, which would silently report two
  # identical roots as different.
  _sps_left="$(CDPATH= cd -- "$1" 2>/dev/null && pwd -P)" || return 1
  _sps_right="$(CDPATH= cd -- "$2" 2>/dev/null && pwd -P)" || return 1
  [ "$_sps_left" = "$_sps_right" ]
}

retire_agent_skill_root() {
  _rgs_root="$1"
  _rgs_base="$2"
  _rgs_stage="$(mktemp -d "${TMPDIR:-/tmp}/dws-retire-agent.XXXXXX")" || return 1
  _rgs_backups="$_rgs_stage/backups"
  : > "$_rgs_backups" || { rm -rf "$_rgs_stage"; return 1; }
  for _rgs_victim in "$_rgs_base/dws" "$_rgs_base"/*; do
    [ -e "$_rgs_victim" ] || [ -L "$_rgs_victim" ] || continue
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

# link_canonical_skills_to_base <root> <base> <mode> [bundle-src]
# <bundle-src> is the installed multi bundle and is REQUIRED in multi mode: the
# link set must come from the bundle, never from the whole canonical store.
# ~/.agents/skills is now shared, so third-party/user Skills live there too and
# must not be published into every detected Agent root (they are absent from
# skills-state.json, so nothing could ever reclaim those links). Go
# publishLinkedUpgradeTarget and build/npm/install.js pass the bundle skill list
# the same way.
link_canonical_skills_to_base() {
  _lcs_root="$1"
  _lcs_base="$2"
  _lcs_mode="$3"
  _lcs_bundle="${4-}"
  _lcs_canonical="$_lcs_root/.agents/skills"
  mkdir -p "$_lcs_base" || return 1
  same_physical_skill_root "$_lcs_base" "$_lcs_canonical" && return 0
  if [ "$_lcs_mode" = "mono" ]; then
    _lcs_names="dws"
  else
    [ -n "$_lcs_bundle" ] && [ -d "$_lcs_bundle" ] || return 1
    _lcs_names=""
    for _lcs_skill in "$_lcs_bundle"/*/; do
      [ -f "${_lcs_skill}SKILL.md" ] || continue
      _lcs_names="$_lcs_names $(basename "$_lcs_skill")"
    done
    [ -n "$_lcs_names" ] || return 1
  fi
  # Collision pre-flight: the victim loop below deliberately refuses to back up
  # entries that are not DWS-managed, and publication must never replace them.
  # Report the exact colliding paths before touching anything, so the fall back
  # to the legacy full-copy layout is observable and actionable.
  _lcs_collision=0
  for _lcs_name in $_lcs_names; do
    _lcs_dest="$_lcs_base/$_lcs_name"
    { [ -e "$_lcs_dest" ] || [ -L "$_lcs_dest" ]; } || continue
    same_physical_skill_root "$_lcs_dest" "$_lcs_canonical/$_lcs_name" && continue
    [ "$_lcs_name" = "dws" ] && continue
    is_managed_multi_skill_dir "$_lcs_dest" && continue
    say "  ⚠️  $_lcs_dest 已存在且不是 DWS 安装的 Skill，无法在此建立指向 $_lcs_canonical/$_lcs_name 的共享链接"
    _lcs_collision=1
  done
  if [ "$_lcs_collision" -eq 1 ]; then
    say "  ⚠️  已保留上述目录（不会删除用户数据），该 Agent 改用独立副本方式安装；"
    say "      移走或改名后重新运行安装即可切换为共享 $_lcs_canonical 布局。"
    return 1
  fi
  _lcs_base_real="$(CDPATH= cd -- "$_lcs_base" && pwd -P)" || return 1
  _lcs_stage="$(mktemp -d "$_lcs_base/.dws-link-set.XXXXXX")" || return 1
  _lcs_backups="$_lcs_stage/.backups"
  _lcs_published="$_lcs_stage/.published"
  _lcs_stage_token="$(basename "$_lcs_stage")"
  : > "$_lcs_backups" || { rm -rf "$_lcs_stage"; return 1; }
  : > "$_lcs_published" || { rm -rf "$_lcs_stage"; return 1; }
  _lcs_publish_names=""
  for _lcs_name in $_lcs_names; do
    if same_physical_skill_root "$_lcs_base/$_lcs_name" "$_lcs_canonical/$_lcs_name"; then
      continue
    fi
    _lcs_target_real="$(CDPATH= cd -- "$_lcs_canonical/$_lcs_name" && pwd -P)" || { rm -rf "$_lcs_stage"; return 1; }
    _lcs_link_target="$(awk -v from="$_lcs_base_real" -v to="$_lcs_target_real" 'BEGIN { nf=split(from,f,"/"); nt=split(to,t,"/"); i=1; while(i<=nf&&i<=nt&&f[i]==t[i])i++; out=""; for(j=i;j<=nf;j++)if(f[j]!="")out=out"../"; for(j=i;j<=nt;j++)if(t[j]!="")out=out t[j](j<nt?"/":""); if(out=="")out="."; print out }')"
    _lcs_stage_name="${_lcs_stage_token}.${_lcs_name}"
    ln -s "$_lcs_link_target" "$_lcs_stage/$_lcs_stage_name" || { rm -rf "$_lcs_stage"; return 1; }
    _lcs_publish_names="$_lcs_publish_names $_lcs_name"
  done
  for _lcs_victim in "$_lcs_base/dws" "$_lcs_base"/*; do
    [ -e "$_lcs_victim" ] || [ -L "$_lcs_victim" ] || continue
    [ "$_lcs_victim" = "$_lcs_stage" ] && continue
    same_physical_skill_root "$_lcs_victim" "$_lcs_canonical/$(basename "$_lcs_victim")" && continue
    if [ "$(basename "$_lcs_victim")" != "dws" ] && ! is_managed_multi_skill_dir "$_lcs_victim"; then
      continue
    fi
    if ! backup_and_record_skill_dir "$_lcs_victim" "$_lcs_backups"; then
      restore_multi_skill_set /dev/null "$_lcs_backups" || true
      rm -rf "$_lcs_stage"
      return 1
    fi
  done
  for _lcs_name in $_lcs_publish_names; do
    _lcs_dest="$_lcs_base/$_lcs_name"
    _lcs_stage_name="${_lcs_stage_token}.${_lcs_name}"
    _lcs_staged="$_lcs_stage/$_lcs_stage_name"
    _lcs_link_target="$(readlink "$_lcs_staged" 2>/dev/null)" || {
      restore_linked_skill_set "$_lcs_published" "$_lcs_backups" || true
      rm -rf "$_lcs_stage"
      return 1
    }
    # Publish by creating the link directly at the destination: symlink(2)
    # refuses an occupied path with EEXIST, so the creation itself is the
    # atomic no-replace check. `mv` after any pre-check could still replace a
    # file or symlink a concurrent writer created in between, and `ln -P`
    # stays unusable (absent from BusyBox `ln`, which silently degraded every
    # non-universal Agent to the copy layout on Alpine and most containers).
    if ! ln -s "$_lcs_link_target" "$_lcs_dest" 2>/dev/null; then
      restore_linked_skill_set "$_lcs_published" "$_lcs_backups" || true
      rm -rf "$_lcs_stage"
      return 1
    fi
    # A directory that appeared at the destination turns `ln -s` into a
    # container: the link lands inside it under the target basename. Remove
    # exactly our link after an identity check and roll back; the foreign
    # directory stays untouched.
    if [ ! -L "$_lcs_dest" ]; then
      _lcs_nested_name="${_lcs_link_target##*/}"
      _lcs_nested_inode="$(skill_link_inode "$_lcs_dest/$_lcs_nested_name" 2>/dev/null)" || _lcs_nested_inode=""
      cleanup_nested_staged_link "$_lcs_dest" "$_lcs_nested_name" "$_lcs_link_target" "$_lcs_nested_inode" || true
      restore_linked_skill_set "$_lcs_published" "$_lcs_backups" || true
      rm -rf "$_lcs_stage"
      return 1
    fi
    _lcs_inode="$(skill_link_inode "$_lcs_dest")" || {
      restore_linked_skill_set "$_lcs_published" "$_lcs_backups" || true
      rm -rf "$_lcs_stage"
      return 1
    }
    if ! skill_link_matches "$_lcs_dest" "$_lcs_link_target" "$_lcs_inode"; then
      restore_linked_skill_set "$_lcs_published" "$_lcs_backups" || true
      rm -rf "$_lcs_stage"
      return 1
    fi
    if ! printf '%s\n%s\n%s\n' "$_lcs_dest" "$_lcs_link_target" "$_lcs_inode" >> "$_lcs_published"; then
      if skill_link_matches "$_lcs_dest" "$_lcs_link_target" "$_lcs_inode"; then rm -f "$_lcs_dest" || true; fi
      restore_linked_skill_set "$_lcs_published" "$_lcs_backups" || true
      rm -rf "$_lcs_stage"
      return 1
    fi
    say "  ↪ Skills → $_lcs_dest"
  done
  rm -rf "$_lcs_stage"
}

# Install skill tree into all agent homes (same rules as build/npm/install.js installSkillsToHomes).
# Installing mono removes proven DWS-managed multi leftovers for mutual exclusion,
# mirroring `dws skill setup --mode mono`.
install_skills_to_homes() {
  skill_src="$1"
  root="${HOME}"
  installed=0
  attempted=1
  failed=0
  retire_failed=0
  if _install_mono_to_base "$skill_src" "$root/.agents/skills" "~/.agents/skills/$SKILL_NAME"; then installed=1; else failed=1; fi
  [ "$installed" -gt 0 ] || { say "  ⚠️  未安装任何 mono Skill：所有检测到的 Agent 目标均失败"; return 1; }
  for agent_dir in $(agent_skill_dirs)
  do
    base_dir="$(resolve_agent_skill_base "$root" "$agent_dir")"
    agent_skill_base_detected "$agent_dir" "$base_dir" || continue
    same_physical_skill_root "$base_dir" "$root/.agents/skills" && continue
    attempted=$((attempted + 1))
    if is_universal_agent_dir "$agent_dir"; then
      retire_agent_skill_root "$root" "$base_dir" || retire_failed=$((retire_failed + 1))
      continue
    fi
    if link_canonical_skills_to_base "$root" "$base_dir" mono; then
      installed=$((installed + 1))
    else
      if _install_mono_to_base "$skill_src" "$base_dir" "$base_dir/$SKILL_NAME"; then
        say "  ℹ️  $base_dir 已自动使用兼容方式安装，可正常使用"
        installed=$((installed + 1))
      else
        failed=$((failed + 1))
      fi
    fi
  done
  if [ "$installed" -eq 0 ]; then
    say "  ⚠️  未安装任何 mono Skill：所有检测到的 Agent 目标均失败"
    return 1
  fi
  if [ "$retire_failed" -gt 0 ]; then
    printf '  ⚠️  有 %s 个 Agent 旧副本未能迁移（安装已完成，可稍后手动删除）\n' "$retire_failed"
  fi
  if [ "$failed" -gt 0 ]; then
    say "  ⚠️  有 ${failed} 个 Agent 目标安装 mono Skill 失败"
    return 1
  fi
  rm -f "$SKILL_STATE_ROOT/skills-state.json"
  say "✅ DWS Skills 安装完成"
  say "   统一安装位置：$root/.agents/skills"
  say "   已自动适配本机上检测到的 Agent"
  say "ℹ️  下一步：请重启已打开的 Agent，使新 Skills 生效"
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
  attempted=1
  failed=0
  retire_failed=0
  if _install_multi_to_base "$multi_src" "$root/.agents/skills" "$root" ".agents/skills"; then installed=1; else failed=1; fi
  [ "$installed" -gt 0 ] || { say "  ⚠️  未安装任何 multi Skill：所有检测到的 Agent 目标均失败"; return 1; }
  for agent_dir in $(agent_skill_dirs)
  do
    base_dir="$(resolve_agent_skill_base "$root" "$agent_dir")"
    agent_skill_base_detected "$agent_dir" "$base_dir" || continue
    same_physical_skill_root "$base_dir" "$root/.agents/skills" && continue
    attempted=$((attempted + 1))
    if is_universal_agent_dir "$agent_dir"; then
      retire_agent_skill_root "$root" "$base_dir" || retire_failed=$((retire_failed + 1))
      continue
    fi
    if link_canonical_skills_to_base "$root" "$base_dir" multi "$multi_src"; then
      installed=$((installed + 1))
    else
      if _install_multi_to_base "$multi_src" "$base_dir" "$root" "$agent_dir"; then
        say "  ℹ️  $base_dir 已自动使用兼容方式安装，可正常使用"
        installed=$((installed + 1))
      else
        failed=$((failed + 1))
      fi
    fi
  done
  if [ "$installed" -eq 0 ]; then
    say "  ⚠️  未安装任何 multi Skill：所有检测到的 Agent 目标均失败"
    return 1
  fi
  if [ "$retire_failed" -gt 0 ]; then
    printf '  ⚠️  有 %s 个 Agent 旧副本未能迁移（安装已完成，可稍后手动删除）\n' "$retire_failed"
  fi
  if [ "$failed" -gt 0 ]; then
    say "  ⚠️  有 ${failed} 个 Agent 目标安装失败"
    return 1
  fi
  write_skills_state "$multi_src" "install.sh" || return 1
  say "✅ DWS Skills 安装完成"
  say "   统一安装位置：$root/.agents/skills"
  say "   已自动适配本机上检测到的 Agent"
  say "ℹ️  下一步：请重启已打开的 Agent，使新 Skills 生效"
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
    if ! publish_skill_dir_no_replace "$_ms_stage/$_name" "$_dest" "$_ms_published"; then
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

  verify_release_asset_checksum "$archive_name" "$tmpdir/$archive_name" "$tmpdir"

  say "📦 Extracting..."
  tar xzf "$tmpdir/$archive_name" -C "$tmpdir"

  mkdir -p "$INSTALL_DIR"

  # The archive may contain a top-level directory or just the binary.
  if [ -f "$tmpdir/$BIN_NAME" ]; then
    found="$tmpdir/$BIN_NAME"
  elif [ -f "$tmpdir/${BIN_NAME}-${os}-${arch}/$BIN_NAME" ]; then
    found="$tmpdir/${BIN_NAME}-${os}-${arch}/$BIN_NAME"
  else
    found="$(find "$tmpdir" -name "$BIN_NAME" -type f | head -1)"
    [ -n "$found" ] || err "Could not find the ${BIN_NAME} binary in the downloaded archive."
  fi

  staged_bin="$INSTALL_DIR/.${INSTALL_NAME}.tmp.$$"
  cp "$found" "$staged_bin"
  chmod +x "$staged_bin"
  mv "$staged_bin" "$INSTALL_DIR/$INSTALL_NAME"

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

  verify_release_asset_checksum "$skills_archive" "$tmpdir_skills/$skills_archive" "$tmpdir_skills"

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

  # Every transaction of this run has finished, so old stamped archives can no
  # longer be needed for a rollback.
  prune_skill_backups

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
