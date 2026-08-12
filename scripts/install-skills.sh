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
#   DWS_SKILL_MODE     — mono | multi (default: multi)
#   DWS_GITEE_REPO     — "owner/repo" on Gitee; resolve version + assets via the
#                        Gitee API instead of GitHub (China mirror)

REPO="DingTalk-Real-AI/dingtalk-workspace-cli"
# China mirror: Gitee repo "owner/repo". When set, version + asset URLs resolve via Gitee API.
GITEE_REPO="${DWS_GITEE_REPO:-}"
# Auto-fallback Gitee mirror used when GitHub is unreachable (see pick_source).
GITEE_FALLBACK_REPO="${DWS_GITEE_FALLBACK_REPO:-DingTalk-Real-AI/dingtalk-workspace-cli}"
VERSION="${DWS_VERSION:-latest}"
SKILL_NAME="dws"
ROOT="${DWS_SKILLS_ROOT:-$PWD}"
DWS_CACHE_ROOT="${DWS_CACHE_ROOT:-$HOME/.dws}"
DWS_STATE_ROOT="${DWS_CONFIG_DIR:-$DWS_CACHE_ROOT}"
MANAGED_SKILL_DIGEST_SCOPE="skill-directory-v1"
SKILL_MODE="$(printf '%s' "${DWS_SKILL_MODE:-multi}" | tr '[:upper:]' '[:lower:]')"
case "$SKILL_MODE" in
  mono|multi) ;;
  *) printf '❌ Invalid DWS_SKILL_MODE=%s. Use mono or multi.\n' "${DWS_SKILL_MODE:-}" >&2; exit 1 ;;
esac

# ── Helpers ──────────────────────────────────────────────────────────────────

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf '❌ Missing required command: %s\n' "$1" >&2
    exit 1
  fi
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
      printf '  ⚠️  备份目录冲突，保留原目录 %s\n' "$_bed_dir"
      return 1
    fi
  done
  mkdir -p "$(dirname "$_bed_target")" 2>/dev/null || {
    printf '  ⚠️  无法创建备份目录，保留原目录 %s\n' "$_bed_dir"
    return 1
  }
  if mv "$_bed_dir" "$_bed_target" 2>/dev/null; then
    DWS_LAST_SKILL_BACKUP="$_bed_target"
    printf '  × 已备份并移除 %s → %s\n' "$_bed_dir" "$_bed_target"
    return 0
  fi
  printf '  ⚠️  备份失败，保留原目录 %s\n' "$_bed_dir"
  return 1
}

# A dingtalk-* prefix alone is not ownership evidence: market/user skills may
# use it too. Ownership comes from the centralized skills-state.json.
is_managed_multi_skill_dir() {
  _managed_dir="$1"
  _managed_name="$(basename "$_managed_dir")"
  is_legacy_official_multi_skill_name "$_managed_name" && return 0
  [ -f "$DWS_STATE_ROOT/skills-state.json" ] || return 1
  _managed_json_name="$(json_escape "$_managed_name")"
  _managed_compact='"name":"'"$_managed_json_name"'"'
  _managed_spaced='"name": "'"$_managed_json_name"'"'
  DWS_MANAGED_COMPACT="$_managed_compact" DWS_MANAGED_SPACED="$_managed_spaced" awk '
    /^[[:space:]]*"managed_skills"[[:space:]]*:[[:space:]]*\[[[:space:]]*$/ { inside = 1; next }
    inside && /^[[:space:]]*\][[:space:]]*,?[[:space:]]*$/ { closed = 1; exit }
    inside && (index($0, ENVIRON["DWS_MANAGED_COMPACT"]) || index($0, ENVIRON["DWS_MANAGED_SPACED"])) { found = 1 }
    END { exit !(closed && found) }
  ' "$DWS_STATE_ROOT/skills-state.json"
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
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 | awk '{print $1}'
  elif command -v openssl >/dev/null 2>&1; then
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
  mkdir -p "$DWS_STATE_ROOT" || return 1
  _state_tmp="$(mktemp "$DWS_STATE_ROOT/.skills-state.XXXXXX")" || return 1
  _state_version="$(json_escape "$VERSION")"
  _state_names=""
  for _state_dir in "$_state_multi"/*/; do
    [ -f "${_state_dir}SKILL.md" ] || continue
    _state_names="${_state_names}$(basename "$_state_dir")\n"
  done
  {
    printf '{\n  "version": "%s",\n' "$_state_version"
    for _state_field in official_skills updated_skills; do
      printf '  "%s": [' "$_state_field"
      _state_first=1
      printf '%b' "$_state_names" | LC_ALL=C sort | while IFS= read -r _state_name; do
        [ -n "$_state_name" ] || continue
        [ "$_state_first" -eq 1 ] || printf ', '
        printf '"%s"' "$(json_escape "$_state_name")"
        _state_first=0
      done
      printf '],\n'
    done
    printf '  "managed_skills": [\n'
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
  mv "$_state_tmp" "$DWS_STATE_ROOT/skills-state.json"
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
      mv "$DWS_LAST_SKILL_BACKUP" "$_bars_victim" 2>/dev/null || printf '  ⚠️  备份记录失败且无法自动恢复: %s（备份位于 %s）\n' "$_bars_victim" "$DWS_LAST_SKILL_BACKUP"
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
        printf '  ⚠️  无法恢复原 Skill: %s（备份保留于 %s）\n' "$_rms_original" "$_rms_backup"
        _rms_ok=0
      fi
    done < "$_rms_backups"
  fi
  [ "$_rms_ok" -eq 1 ]
}

# publish_skill_cache <source> <cache-dir>
# Stages a complete sibling cache before publishing it. Any copy or publish
# failure leaves the previous cache in place (or in the reported recovery dir
# when even restoration fails).
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
      printf '  ⚠️ 新 Skill 缓存已生效，但旧缓存清理失败: %s\n' "$_psc_old"
    fi
    return 0
  fi

  rm -rf "$_psc_stage"
  if [ -n "$_psc_old" ] && ! mv "$_psc_old" "$_psc_cache"; then
    printf '  ⚠️ Skill 缓存发布失败，原缓存保留在 %s\n' "$_psc_old"
  fi
  return 1
}

# Fetch a Gitee API endpoint, retrying transient 502/503 from Gitee's gateway.
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

pick_source() {
  [ -n "$GITEE_REPO" ] && return 0
  [ "${DWS_NO_FALLBACK:-0}" = "1" ] && return 0
  curl -fsS --connect-timeout 5 --max-time 12 -o /dev/null "https://github.com/${REPO}/releases/latest" 2>/dev/null && return 0
  GITEE_REPO="$GITEE_FALLBACK_REPO"
  printf '  ⚠ GitHub 不可达，自动切换国内 Gitee 镜像: %s\n' "$GITEE_REPO"
}

resolve_version() {
  if [ "$VERSION" = "latest" ]; then
    if [ -n "$GITEE_REPO" ]; then
      # Gitee's /releases/latest and /releases endpoints are unreliable, so
      # resolve the newest vN.N.N tag from the git tags endpoint instead.
      VERSION="$(gitee_api "https://gitee.com/api/v5/repos/${GITEE_REPO}/tags" \
        | grep -o '"name":[ ]*"v[0-9][0-9.]*"' \
        | sed 's/.*"name":[ ]*"//;s/"$//' \
        | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' \
        | sort -V | tail -1)"
    else
      VERSION="$(curl -fsSI "https://github.com/${REPO}/releases/latest" 2>/dev/null \
        | grep -i '^location:' | sed 's|.*/tag/||;s/[[:space:]]*$//')"
    fi
    if [ -z "$VERSION" ]; then
      printf '❌ Could not determine the latest version. Set DWS_VERSION explicitly.\n' >&2
      exit 1
    fi
  fi
}

# Resolve a release asset's download URL by name (GitHub template vs Gitee API).
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
    | head -1 | sed 's/.*"browser_download_url":[ ]*"//;s/"$//'
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
    backup_and_remove_skill_dir "$_dest" || {
      printf '  ⚠️  跳过 %s（保留原目录）\n' "$_dest"
      return 1
    }
  fi

  mkdir -p "$_dest" || return 1
  if ! cp -R "$_src/"* "$_dest/" 2>/dev/null && ! cp -r "$_src/"* "$_dest/"; then
    printf '  ⚠️  Skill 复制失败，目标未计为安装成功: %s\n' "$_dest"
    return 1
  fi
  file_count="$(find "$_dest" -type f | wc -l | tr -d ' ')"

  printf '  ✅ Skills → %s (%s files)\n' "$_label" "$file_count"
}

# Full copy with top-level listing (1st target).
_copy_skill() {
  _src="$1"
  _dest="$2"
  _label="$3"

  if [ -d "$_dest" ]; then
    backup_and_remove_skill_dir "$_dest" || {
      printf '  ⚠️  跳过 %s（保留原目录）\n' "$_dest"
      return 1
    }
  fi

  mkdir -p "$_dest" || return 1
  if ! cp -R "$_src/"* "$_dest/" 2>/dev/null && ! cp -r "$_src/"* "$_dest/"; then
    printf '  ⚠️  Skill 复制失败，目标未计为安装成功: %s\n' "$_dest"
    return 1
  fi
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

# Move DWS-owned copies out of the generic root once a concrete Agent root is
# active. This prevents Agents such as Codex from discovering duplicates.
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

# Same semantics as build/npm/install.js installMultiSkillsToHomes (root = DWS_SKILLS_ROOT or PWD).
install_multi_skills_to_root() {
  multi_src="$1"
  root="$2"
  installed=0
  attempted=0
  failed=0
  idx=0
  specific_agents=0
  for specific_dir in \
    ".claude/skills" ".cursor/skills" ".qoder/skills" ".qoderwork/skills" \
    ".gemini/skills" ".codex/skills" ".github/skills" ".windsurf/skills" \
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
      printf '  ⚠️  跳过 %s（备份或复制失败，未完成 multi 安装）\n' "$base_dir"
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
    printf '  ⚠️  未安装任何 multi Skill：所有检测到的 Agent 目标均失败\n'
    return 1
  fi
  if [ "$failed" -gt 0 ]; then
    printf '  ⚠️  有 %s 个 Agent 目标安装失败\n' "$failed"
    return 1
  fi
  write_skills_state "$multi_src" "install-skills.sh" || return 1
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
      printf '  ⚠️  multi Skill 集合发布失败，正在恢复原集合: %s\n' "$_dest"
      restore_multi_skill_set "$_ms_published" "$_ms_backups" || printf '  ⚠️  原 Skill 集合自动恢复不完整，请检查上方备份路径\n'
      rm -rf "$_ms_stage"
      return 1
    fi
    _count=$((_count + 1))
  done
  rm -rf "$_ms_stage" || return 1

  if [ "$_root" = "$HOME" ]; then
    _label="~/$_agent_dir/"
  else
    _label="$_root/$_agent_dir/"
  fi
  printf '  ✅ Skills → %s (%s product skills)\n' "$_label" "$_count"
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
    printf '  ⚠️  mono Skill 集合发布失败，正在恢复原集合: %s\n' "$_mono_dest"
    restore_multi_skill_set "$_mono_published" "$_mono_backups" || printf '  ⚠️  原 Skill 集合自动恢复不完整，请检查上方备份路径\n'
    rm -rf "$_mono_stage"
    return 1
  fi
  rm -rf "$_mono_stage" || return 1
  _mono_count="$(find "$_mono_dest" -type f | wc -l | tr -d ' ')"
  printf '  ✅ Skills → %s (%s files)\n' "$_mono_label" "$_mono_count"
}

# Same semantics as build/npm/install.js installSkillsToHomes (root = DWS_SKILLS_ROOT or PWD).
install_skills_to_root() {
  skill_src="$1"
  root="$2"
  installed=0
  attempted=0
  failed=0
  idx=0
  specific_agents=0
  for specific_dir in \
    ".claude/skills" ".cursor/skills" ".qoder/skills" ".qoderwork/skills" \
    ".gemini/skills" ".codex/skills" ".github/skills" ".windsurf/skills" \
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
    if [ "$root" = "$HOME" ]; then
      label="~/$agent_dir/$SKILL_NAME"
    else
      label="$root/$agent_dir/$SKILL_NAME"
    fi
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
    if [ "$root" = "$HOME" ]; then
      flabel="~/.agents/skills/$SKILL_NAME"
    else
      flabel="$root/.agents/skills/$SKILL_NAME"
    fi
    if _install_mono_to_base "$skill_src" "$root/.agents/skills" "$flabel"; then
      installed=$((installed + 1))
    else
      failed=$((failed + 1))
    fi
  fi
  if [ "$installed" -eq 0 ]; then
    printf '  ⚠️  未安装任何 mono Skill：所有检测到的 Agent 目标均失败\n'
    return 1
  fi
  if [ "$failed" -gt 0 ]; then
    printf '  ⚠️  有 %s 个 Agent 目标安装 mono Skill 失败\n' "$failed"
    return 1
  fi
  rm -f "$DWS_STATE_ROOT/skills-state.json"
}

# ── Main ─────────────────────────────────────────────────────────────────────

main() {
  need_cmd curl
  pick_source
  resolve_version

  printf '\n'
  printf '  ┌──────────────────────────────────────┐\n'
  printf '  │     DWS Skill Installer              │\n'
  printf '  │     DingTalk Workspace CLI            │\n'
  printf '  └──────────────────────────────────────┘\n'
  printf '\n'

  TMPDIR_WORK="$(mktemp -d)"
  trap 'rm -rf "$TMPDIR_WORK"' EXIT INT TERM

  ASSET_URL="$(asset_url dws-skills.zip)"
  [ -n "$ASSET_URL" ] || { printf '❌ Could not resolve download URL for dws-skills.zip (version %s).\n' "$VERSION" >&2; exit 1; }
  printf '  ⬇  Downloading skills from GitHub Releases: %s (%s)\n' "$REPO" "$VERSION"
  curl -fsSL "$ASSET_URL" -o "$TMPDIR_WORK/dws-skills.zip"
  extract_zip "$TMPDIR_WORK/dws-skills.zip" "$TMPDIR_WORK/extracted"

  # Prefer the explicit mono/ subtree; fall back to legacy nested or zip root.
  SKILL_SRC="$TMPDIR_WORK/extracted"
  if [ -d "$TMPDIR_WORK/extracted/mono" ] && [ -f "$TMPDIR_WORK/extracted/mono/SKILL.md" ]; then
    SKILL_SRC="$TMPDIR_WORK/extracted/mono"
  elif [ -f "$TMPDIR_WORK/extracted/${SKILL_NAME}/SKILL.md" ]; then
    SKILL_SRC="$TMPDIR_WORK/extracted/${SKILL_NAME}"
  fi

  printf '\n'
  printf '  Installing under root: %s (mode: %s)\n' "$ROOT" "$SKILL_MODE"
  # Multi first: a release may ship only the multi/ tree without the root
  # mono copy, so the mono SKILL.md gate must never block a multi install.
  # An empty/corrupt multi/ tree (no */SKILL.md) falls back to mono with a
  # warning — installing it would wipe existing skills and lay down nothing.
  if [ "$SKILL_MODE" = "multi" ] && multi_tree_has_skills "$TMPDIR_WORK/extracted/multi"; then
    install_multi_skills_to_root "$TMPDIR_WORK/extracted/multi" "$ROOT"
  else
    if [ "$SKILL_MODE" = "multi" ]; then
      printf '  ⚠️  Multi skill tree not found or empty in release asset; falling back to mono.\n'
    fi
    if [ ! -f "$SKILL_SRC/SKILL.md" ]; then
      printf '  ❌ Skill source not found in release asset\n' >&2
      exit 1
    fi
    install_skills_to_root "$SKILL_SRC" "$ROOT"
  fi

  # Cache multi/ (and a mono copy) under ~/.dws/skills so that subsequent
  # `dws skill setup --mode multi|mono` invocations can find a source. An
  # empty/corrupt tree must never wipe a previously good cache.
  if multi_tree_has_skills "$TMPDIR_WORK/extracted/multi"; then
    cache_dir="${DWS_CACHE_ROOT}/skills/multi"
    if publish_skill_cache "$TMPDIR_WORK/extracted/multi" "$cache_dir"; then
      file_count="$(find "$cache_dir" -type f | wc -l | tr -d ' ')"
      printf '  ✅ Cached multi skills → %s (%s files)\n' "$cache_dir" "$file_count"
    else
      printf '  ⚠️ Multi Skill 缓存刷新失败，未覆盖原缓存: %s\n' "$cache_dir"
    fi
  fi
  if [ -f "$SKILL_SRC/SKILL.md" ]; then
    mono_cache="${DWS_CACHE_ROOT}/skills/mono"
    if ! publish_skill_cache "$SKILL_SRC" "$mono_cache"; then
      printf '  ⚠️ Mono Skill 缓存刷新失败，未覆盖原缓存: %s\n' "$mono_cache"
    fi
  fi

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
