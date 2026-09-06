#!/bin/sh
# Copyright 2026 Alibaba Group
# Licensed under the Apache License, Version 2.0
#
# One-command installer for dws dev — pre-built binary, no build tools.
# Downloads the dev binary + dingtalk-misc skill (hosts open-platform app docs) from the DingTalk-Real-AI GitHub Releases.
# Requires only curl + tar (no go / make / git).
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/DingTalk-Real-AI/dingtalk-workspace-cli/main/scripts/install-devapp.sh | sh
#
# Env (all optional):
#   DEVAPP_REPO      repo holding dev releases (default: DingTalk-Real-AI/dingtalk-workspace-cli)
#   DEVAPP_VERSION   pin a release tag (default: latest release)
#   DWS_INSTALL_DIR  binary dir (default: ~/.local/bin)
#   DWS_NO_SKILLS    set 1 to skip the dev skill
set -eu

DEVAPP_REPO="${DEVAPP_REPO:-DingTalk-Real-AI/dingtalk-workspace-cli}"
DEVAPP_VERSION="${DEVAPP_VERSION:-}"
INSTALL_DIR="${DWS_INSTALL_DIR:-$HOME/.local/bin}"
NO_SKILLS="${DWS_NO_SKILLS:-0}"
SKILL_NAME="dingtalk-misc"

say() { printf '  %s\n' "$@"; }
err() { printf '  ❌ %s\n' "$@" >&2; exit 1; }
need_cmd() { command -v "$1" >/dev/null 2>&1 || err "Missing required command: $1"; }

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then shasum -a 256 "$1" | awk '{print $1}'
  elif command -v openssl >/dev/null 2>&1; then openssl dgst -sha256 "$1" | awk '{print $NF}'
  else return 1
  fi
}

verify_release_asset() {
  name="$1"; file="$2"
  checksums="$tmp/checksums.txt"
  if [ ! -f "$checksums" ]; then
    curl -fsSL "https://github.com/${DEVAPP_REPO}/releases/download/${DEVAPP_VERSION}/checksums.txt" -o "$checksums" \
      || err "Could not download checksums.txt; refusing unverified release assets."
  fi
  expected="$(awk -v asset="$name" '$2 == asset {print $1; exit}' "$checksums")"
  [ -n "$expected" ] || err "${name} is missing from checksums.txt."
  actual="$(sha256_file "$file")" || err "Could not compute SHA256 for ${name}."
  [ "$actual" = "$expected" ] || err "SHA256 checksum mismatch for ${name}."
  say "✅ SHA256 checksum verified: ${name}"
}


# perm_of <path>
# Prints the octal permission bits of path. Linux stat and BSD/macOS stat
# disagree on flags, so try both spellings and fail loudly when neither is
# available.
perm_of() {
  _po_mode="$(stat -c %a "$1" 2>/dev/null || stat -f %Lp "$1" 2>/dev/null)" || return 1
  printf '%s\n' "$_po_mode"
}

# discard_stage <dir>
# Best-effort removal of our own staging tree. Read-only directories copied
# from the skill bundle resist rm -rf, so grant owner write access first.
discard_stage() {
  chmod -R u+w "$1" 2>/dev/null || true
  rm -rf "$1" 2>/dev/null || true
}

# count_tree_entries <dir>
count_tree_entries() {
  _cte_n=0
  for _cte_e in "$1"/* "$1"/.[!.]* "$1"/..?*; do
    [ -e "$_cte_e" ] || [ -L "$_cte_e" ] || continue
    _cte_n=$((_cte_n+1))
  done
  printf '%s\n' "$_cte_n"
}

# publish_tree_children <stage> <dest> <manifest>
# Publishes every child of the staging directory into the claimed dest through
# an atomic no-clobber primitive: directories claim with mkdir and recurse,
# regular files publish via ln (a hard link is a single atomic rename-class
# syscall that fails with EEXIST when the target exists) and then drop the
# staging source, and symlinks are recreated with ln -s. A POSIX `mv` cannot
# be used here: renaming a directory onto a concurrently created same-name
# (empty) directory replaces it, and mv offers no kernel-level no-replace.
# Every primitive therefore refuses a concurrently created object instead of
# replacing it. The absolute destination path of each published entry is
# appended to the manifest so the rollback only ever retracts entries this
# transaction published. The recursion runs in a subshell so the caller's
# loop variables survive; manifest appends and filesystem effects persist.
# After each level the destination is re-counted: more entries than the
# staging side had means a different-named concurrent writer landed
# mid-publish, and the whole publication aborts with the destination kept.
publish_tree_children() {
  _ptc_stage="$1"; _ptc_dest="$2"; _ptc_manifest="$3"
  _ptc_staged="$(count_tree_entries "$_ptc_stage")"
  for _ptc_child in "$_ptc_stage"/* "$_ptc_stage"/.[!.]* "$_ptc_stage"/..?*; do
    [ -e "$_ptc_child" ] || [ -L "$_ptc_child" ] || continue
    _ptc_name="$(basename "$_ptc_child")"
    _ptc_target="${_ptc_dest%/}/$_ptc_name"
    if [ -L "$_ptc_child" ]; then
      _ptc_link="$(readlink "$_ptc_child")" || return 1
      ln -s "$_ptc_link" "$_ptc_target" 2>/dev/null || {
        [ -e "$_ptc_target" ] || [ -L "$_ptc_target" ] || return 1
        printf '  并发目录项已保留，拒绝覆盖 %s\n' "$_ptc_target" >&2
        return 1
      }
      rm -f "$_ptc_child" || return 1
    elif [ -d "$_ptc_child" ]; then
      _ptc_mode="$(perm_of "$_ptc_child")" || return 1
      # The staging tree is ours alone: skill trees ship read-only
      # directories (0555), which would block unlinking the children and
      # removing the emptied shell. Grant owner write access for the move;
      # the published target receives the recorded mode below.
      chmod u+w "$_ptc_child" || return 1
      mkdir "$_ptc_target" 2>/dev/null || {
        [ -e "$_ptc_target" ] || [ -L "$_ptc_target" ] || return 1
        printf '  并发目录项已保留，拒绝覆盖 %s\n' "$_ptc_target" >&2
        return 1
      }
      ( publish_tree_children "$_ptc_child" "$_ptc_target" "$_ptc_manifest" ) || return 1
      chmod "$_ptc_mode" "$_ptc_target" || return 1
      rmdir "$_ptc_child" 2>/dev/null || return 1
    else
      ln "$_ptc_child" "$_ptc_target" 2>/dev/null || {
        [ -e "$_ptc_target" ] || [ -L "$_ptc_target" ] || return 1
        printf '  并发目录项已保留，拒绝覆盖 %s\n' "$_ptc_target" >&2
        return 1
      }
      rm -f "$_ptc_child" || return 1
    fi
    printf '%s\n' "$_ptc_target" >> "$_ptc_manifest"
  done
  _ptc_live="$(count_tree_entries "$_ptc_dest")"
  if [ "$_ptc_live" -gt "$_ptc_staged" ]; then
    printf '  目标出现并发目录项，中止发布并保留 %s\n' "$_ptc_dest" >&2
    return 1
  fi
  return 0
}

# retract_published_children <stage_root> <dest_root> <manifest>
# Moves the manifest-recorded entries back into the staging tree in reverse
# publication order, so deeper entries move before (or together with, when a
# containing directory retraction carries them) the directories that hold
# them. Only entries this transaction published are touched; a concurrently
# created object is never retracted. An entry that no longer exists was
# carried away with an ancestor; a genuine move failure stops the retraction
# so the caller reports the retained destination.
retract_published_children() {
  _rpc_stage_root="$1"; _rpc_dest_root="$2"; _rpc_manifest="$3"
  [ -s "$_rpc_manifest" ] || return 0
  _rpc_rev="${_rpc_manifest}.rev"
  sed '1!G;h;$!d' "$_rpc_manifest" > "$_rpc_rev" || { rm -f "$_rpc_rev"; return 1; }
  while IFS= read -r _rpc_entry; do
    [ -n "$_rpc_entry" ] || continue
    _rpc_rel="${_rpc_entry#"${_rpc_dest_root%/}/"}"
    _rpc_back="${_rpc_stage_root%/}/$_rpc_rel"
    mkdir -p "$(dirname "$_rpc_back")" 2>/dev/null || break
    rmdir "$_rpc_back" 2>/dev/null || true
    mv "$_rpc_entry" "$_rpc_back" 2>/dev/null || {
      [ -e "$_rpc_entry" ] || [ -L "$_rpc_entry" ] || continue
      break
    }
  done < "$_rpc_rev"
  rm -f "$_rpc_rev"
  return 0
}

copy_tree() {
  src="$1"
  dest="$2"
  parent="$(dirname "$dest")"
  mkdir -p "$parent" || return 1
  stage="$(mktemp -d "$parent/.dws-skill.tmp.XXXXXX")" || return 1
  manifest="$(mktemp "$parent/.dws-skill.txn.XXXXXX")" || { rm -rf "$stage"; return 1; }
  if ! cp -R "$src/." "$stage/"; then discard_stage "$stage"; rm -f "$manifest"; return 1; fi
  chmod u+w "$stage" 2>/dev/null || true
  backup="$(backup_skill_dir "$dest")" || { discard_stage "$stage"; rm -f "$manifest"; return 1; }
  _ct_mode="$(perm_of "$src")" || { discard_stage "$stage"; rm -f "$manifest"; return 1; }
  # mkdir is the atomic no-replace claim: it fails when the path is already
  # occupied, and the claim is then held for the whole transaction. Children
  # publish into the claim through per-entry no-clobber primitives (see
  # publish_tree_children) so a concurrent writer can never be replaced; the
  # rollback only retracts what the manifest recorded, and the claim itself
  # is removed only while still empty, so foreign entries always survive.
  if mkdir "$dest" 2>/dev/null; then
    if publish_tree_children "$stage" "$dest" "$manifest"; then
      chmod "$_ct_mode" "$dest" 2>/dev/null || true
      discard_stage "$stage"
      rm -f "$manifest"
      return 0
    fi
    retract_published_children "$stage" "$dest" "$manifest"
    rmdir "$dest" 2>/dev/null || true
  fi
  discard_stage "$stage"
  rm -f "$manifest"
  if [ -n "$backup" ]; then
    restore_manifest="$(mktemp "$parent/.dws-skill.txn.XXXXXX")" || return 1
    if mkdir "$dest" 2>/dev/null; then
      # The restore publishes the backup through the same no-clobber
      # discipline: a concurrent writer that took the emptied path (or any
      # entry inside it) is refused and the backup is kept intact instead
      # of being partially drained by an overwriting move.
      if publish_tree_children "$backup" "$dest" "$restore_manifest"; then
        rmdir "$backup" 2>/dev/null || true
      else
        retract_published_children "$backup" "$dest" "$restore_manifest"
        rmdir "$dest" 2>/dev/null || true
        printf '  Skill rollback failed; backup retained at %s\n' "$backup" >&2
      fi
    else
      printf '  Skill rollback failed; backup retained at %s\n' "$backup" >&2
    fi
    rm -f "$restore_manifest"
  fi
  return 1
}

# refresh_cache_tree <src> <dest>
# Ephemeral ~/.dws/skills cache refresh, identical to install-event.sh. The tree
# is byte-reproducible from the release asset, so it is never archived into
# ~/.dws/skill-backups; unlike the previous `rm -rf` + `cp` it only deletes the
# old cache after the replacement is fully staged, so an interrupted install can
# no longer leave a half-copied cache behind.
refresh_cache_tree() {
  src="$1"; dest="$2"; parent="$(dirname "$dest")"
  mkdir -p "$parent" || return 1
  stage="$(mktemp -d "$parent/.dws-cache.tmp.XXXXXX")" || return 1
  if ! cp -R "$src/." "$stage/"; then rm -rf "$stage"; return 1; fi
  old=""
  if [ -e "$dest" ] || [ -L "$dest" ]; then
    old="$(mktemp -d "$parent/.dws-cache.old.XXXXXX")" || { rm -rf "$stage"; return 1; }
    rmdir "$old" || { rm -rf "$stage" "$old"; return 1; }
    if ! mv "$dest" "$old"; then rm -rf "$stage"; return 1; fi
  fi
  if mv "$stage" "$dest"; then
    if [ -n "$old" ] && ! rm -rf "$old"; then
      printf '  ⚠️  新缓存已生效，但旧缓存清理失败: %s\n' "$old" >&2
    fi
    return 0
  fi
  rm -rf "$stage"
  if [ -n "$old" ] && ! mv "$old" "$dest"; then
    printf '  ⚠️  缓存刷新失败，原缓存保留在 %s\n' "$old" >&2
  fi
  return 1
}

backup_skill_dir() {
  victim="$1"
  [ -e "$victim" ] || [ -L "$victim" ] || { printf '\n'; return 0; }
  # Prune before creating this run's archive; every stamp this run has already
  # created is registered, so pruning can only remove earlier-run batches.
  prune_skill_backups
  stamp="$(date -u +%Y%m%d-%H%M%S)"
  name="$(printf '%s' "$victim" | sed 's#[/\\]#-#g; s#^-##')"
  backup_root="$HOME/.dws/skill-backups/$stamp"; backup="$backup_root/$name"; i=0
  # Bump not only when the payload path is taken but also when the stamp
  # root exists without a verified ownership marker: a same-second foreign
  # directory must never be stamped DWS-owned and made prunable. A
  # marker-verified root from this run's same second stays reusable.
  while [ -e "$backup" ] || [ -L "$backup" ] ||
    { [ -d "$backup_root" ] && ! is_current_run_backup_stamp "$backup_root" &&
      [ "$(cat "$backup_root/.dws-skill-backup" 2>/dev/null)" != "dws skill backup v1" ]; }; do
    i=$((i + 1)); backup_root="$HOME/.dws/skill-backups/$stamp-$i"; backup="$backup_root/$name"
    # Same bail-out as install-skills.sh: never spin forever on a pathological
    # backup root, report it and keep the original directory instead.
    if [ "$i" -gt 1000 ]; then
      printf '  ⚠️  备份目录冲突，保留原目录 %s\n' "$victim" >&2
      return 1
    fi
  done
  record_current_run_backup_stamp "$backup_root"
  # Freshness must be sampled before mkdir: the collision loop tests the
  # payload path, so a second backup in the same stamp second reuses this
  # root while a sibling payload from this run already lives in it.
  fresh=1
  [ ! -d "$backup_root" ] || fresh=0
  mkdir -p "$backup_root" || return 1
  # Ownership proof, the exact bytes Go's writeSkillBackupMarker stamps: a
  # stamp-shaped name alone is not evidence, so pruning only ever removes
  # roots carrying this marker — it must exist before the payload moves in.
  printf '%s\n' 'dws skill backup v1' > "$backup_root/.dws-skill-backup" || {
    # Non-recursive cleanup, sibling protection: only a root this call
    # created may be dropped, and only the marker plus an empty root (rmdir
    # refuses a non-empty directory) — rm -rf here would destroy a completed
    # same-second sibling backup whose original is already moved away.
    if [ "$fresh" -eq 1 ]; then
      rm -f "$backup_root/.dws-skill-backup"
      rmdir "$backup_root" 2>/dev/null || true
    fi
    return 1
  }
  mv "$victim" "$backup" || return 1
  printf '%s\n' "$backup"
}

# prune_skill_backups keeps only the newest SKILL_BACKUP_KEEP stamped backup
# directories from earlier runs under $HOME/.dws/skill-backups, matching Go's
# skillBackupKeep / pruneSkillBackups. Only stamp-shaped roots carrying the
# .dws-skill-backup marker with the exact expected content are counted or
# removed: foreign data with a stamp-like name never consumes a keep slot
# and is never deleted, silently. Stamp directories created by the
# current process are never pruned (mirroring Go's run-root registry and
# install.js's currentRunBackupRoots), so a migration that retires more than
# SKILL_BACKUP_KEEP batches stays reversible. Glob order is lexicographic and
# the stamps are UTC YYYYmmdd-HHMMSS, so the oldest sort first. Best-effort:
# a removal failure is reported and never fatal.
SKILL_BACKUP_KEEP=5

# Stamp directories created by this run, recorded by backup_skill_dir so
# pruning can never delete a backup the running migration may still need to
# roll back to.
CURRENT_RUN_BACKUP_STAMPS=""

record_current_run_backup_stamp() {
  case " $CURRENT_RUN_BACKUP_STAMPS " in
    *" $1 "*) return 0 ;;
  esac
  CURRENT_RUN_BACKUP_STAMPS="${CURRENT_RUN_BACKUP_STAMPS} $1"
}

# is_current_run_backup_stamp reports whether the stamp root was created by
# this very process. Such a root is ours by construction and stays reusable
# even when its marker cannot be re-verified mid-run (for example a
# permission failure after the first payload moved in).
is_current_run_backup_stamp() {
  case " $CURRENT_RUN_BACKUP_STAMPS " in
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
  prune_root="$HOME/.dws/skill-backups"
  [ -d "$prune_root" ] || return 0
  prune_total=0
  for prune_entry in "$prune_root"/*; do
    [ -d "$prune_entry" ] || continue
    is_skill_backup_stamp "${prune_entry##*/}" || continue
    [ "$(cat "$prune_entry/.dws-skill-backup" 2>/dev/null)" = "dws skill backup v1" ] || continue
    prune_total=$((prune_total + 1))
  done
  [ "$prune_total" -gt "$SKILL_BACKUP_KEEP" ] || return 0
  prune_drop=$((prune_total - SKILL_BACKUP_KEEP))
  for prune_entry in "$prune_root"/*; do
    [ "$prune_drop" -gt 0 ] || break
    [ -d "$prune_entry" ] || continue
    is_skill_backup_stamp "${prune_entry##*/}" || continue
    [ "$(cat "$prune_entry/.dws-skill-backup" 2>/dev/null)" = "dws skill backup v1" ] || continue
    case " $CURRENT_RUN_BACKUP_STAMPS " in
      *" $prune_entry "*) continue ;;
    esac
    rm -rf "$prune_entry" || printf '  ⚠️  旧 Skill 备份清理失败: %s\n' "$prune_entry" >&2
    prune_drop=$((prune_drop - 1))
  done
}

same_physical_skill() {
  [ -d "$1" ] && [ -d "$2" ] || return 1
  left="$(CDPATH= cd -- "$1" 2>/dev/null && pwd -P)" || return 1
  right="$(CDPATH= cd -- "$2" 2>/dev/null && pwd -P)" || return 1
  [ "$left" = "$right" ]
}

is_cleanup_only_agent_dir() {
  case "$1" in
    .config/agents/skills|.gemini/antigravity/skills|.gemini/antigravity-cli/skills|.codex/skills|.cursor/skills|.deepagents/agent/skills|.firebender/skills|.gemini/skills|.copilot/skills|.config/opencode/skills|.github/skills|.windsurf/skills|.cline/skills|.amp/skills) return 0 ;;
    *) return 1 ;;
  esac
}

agent_skill_dirs() {
  printf '%s\n' \
    .config/agents/skills .gemini/antigravity/skills .gemini/antigravity-cli/skills .codex/skills .cursor/skills .deepagents/agent/skills .firebender/skills .gemini/skills .copilot/skills .config/opencode/skills \
    .aider-desk/skills .astrbot/data/skills .autohand/skills .augment/skills .bob/skills .claude/skills .openclaw/skills .codeartsdoer/skills .codebuddy/skills .codemaker/skills .codestudio/skills .commandcode/skills .continue/skills .snowflake/cortex/skills .config/crush/skills .config/devin/skills .factory/skills .forge/skills .config/goose/skills .grok/skills .hermes/skills .inferencesh/skills .jazz/skills .junie/skills .iflow/skills .kilocode/skills .config/kimchi/harness/skills .kiro/skills .kode/skills .lingma/skills .mcpjam/skills .minimax/skills .vibe/skills .moxby/skills .mux/skills .openhands/skills .ona/skills .pi/agent/skills .qoder/skills .qoder-cn/skills .qwen/skills .reasonix/skills .rovodev/skills .roo/skills .tabnine/agent/skills .terramind/skills .tinycloud/skills .trae/skills .trae-cn/skills .codeium/windsurf/skills .zcode/skills .zencoder/skills .neovate/skills .pochi/skills .adal/skills \
    .qoderwork/skills .github/skills .windsurf/skills .cline/skills .amp/skills
}

resolve_agent_skill_base() {
  agent_dir="$1"; base="$HOME/$agent_dir"
  case "$agent_dir" in
    .claude/skills) [ -n "${CLAUDE_CONFIG_DIR:-}" ] && base="$CLAUDE_CONFIG_DIR/skills" ;;
    .codex/skills) [ -n "${CODEX_HOME:-}" ] && base="$CODEX_HOME/skills" ;;
    .hermes/skills) [ -n "${HERMES_HOME:-}" ] && base="$HERMES_HOME/skills" ;;
    .autohand/skills) [ -n "${AUTOHAND_HOME:-}" ] && base="$AUTOHAND_HOME/skills" ;;
    .grok/skills) [ -n "${GROK_HOME:-}" ] && base="$GROK_HOME/skills" ;;
    .vibe/skills) [ -n "${VIBE_HOME:-}" ] && base="$VIBE_HOME/skills" ;;
    .openclaw/skills) for legacy in .openclaw .clawdbot .moltbot; do [ -d "$HOME/$legacy" ] && { base="$HOME/$legacy/skills"; break; }; done ;;
    .config/*) base="${XDG_CONFIG_HOME:-$HOME/.config}/${agent_dir#.config/}" ;;
  esac
  printf '%s\n' "$base"
}

agent_skill_base_detected() {
  agent_dir="$1"; base="$2"
  case "$agent_dir" in
    .config/kimchi/harness/skills|.tabnine/agent/skills) [ -d "$(dirname "$(dirname "$base")")" ] ;;
    .zcode/skills) [ -d "$(dirname "$base")" ] || [ -d "/Applications/ZCode.app" ] ;;
    .minimax/skills) [ -d "$(dirname "$base")" ] || [ -d "/Applications/MiniMax Code.app" ] ;;
    *) [ -d "$(dirname "$base")" ] ;;
  esac
}

link_or_copy_skill() {
  canonical="$1"; src="$2"; dest="$3"
  same_physical_skill "$dest" "$canonical" && return 0
  parent="$(dirname "$dest")"; mkdir -p "$parent" || return 1
  parent_real="$(CDPATH= cd -- "$parent" && pwd -P)" || return 1
  target_real="$(CDPATH= cd -- "$canonical" && pwd -P)" || return 1
  relative="$(awk -v from="$parent_real" -v to="$target_real" 'BEGIN { nf=split(from,f,"/"); nt=split(to,t,"/"); i=1; while(i<=nf&&i<=nt&&f[i]==t[i])i++; out=""; for(j=i;j<=nf;j++)if(f[j]!="")out=out"../"; for(j=i;j<=nt;j++)if(t[j]!="")out=out t[j](j<nt?"/":""); if(out=="")out="."; print out }')"
  stage="$(mktemp -d "$parent/.dws-link.tmp.XXXXXX")" || return 1
  if ! ln -s "$relative" "$stage/skill" 2>/dev/null; then
    rm -rf "$stage"
    if copy_tree "$src" "$dest"; then
      printf '  ℹ️  %s 已自动使用兼容方式安装，可正常使用\n' "$dest" >&2
      return 0
    fi
    return 1
  fi
  backup="$(backup_skill_dir "$dest")" || { rm -rf "$stage"; return 1; }
  # Create the link directly at the destination: symlink(2) refuses an
  # occupied path with EEXIST, so publication itself is the atomic no-replace
  # check (`mv` after the backup could still replace a file or symlink a
  # concurrent writer created in between). A directory that slipped in turns
  # `ln -s` into a container: remove exactly our nested link and roll back.
  if ln -s "$relative" "$dest" 2>/dev/null && [ -L "$dest" ]; then
    rm -rf "$stage"
    return 0
  fi
  nested="$dest/${relative##*/}"
  if [ -L "$nested" ] && [ "$(readlink "$nested" 2>/dev/null)" = "$relative" ]; then
    rm -f "$nested" || true
  fi
  rm -rf "$stage"
  if [ -n "$backup" ]; then
    if [ -e "$dest" ] || [ -L "$dest" ]; then
      printf '  ⚠️  %s 在发布期间被并发写入占用，原 Skill 保留于备份: %s\n' "$dest" "$backup" >&2
    elif ! mv "$backup" "$dest"; then
      printf '  Skill rollback failed; backup retained at %s\n' "$backup" >&2
    fi
  fi
  return 1
}

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

# Read the releases list (newest first) and take the top tag, so this also works
# if a release is ever published as a prerelease (which /releases/latest skips).
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
  [ -n "$DEVAPP_VERSION" ] || err "No release found on ${DEVAPP_REPO}. Set DEVAPP_VERSION to a published release tag."
}

install_skill() {
  bundle="$1"   # extracted skills bundle dir
  src=""
  for c in "$bundle/multi/$SKILL_NAME" "$bundle/skills/multi/$SKILL_NAME" "$bundle/$SKILL_NAME"; do
    [ -f "$c/SKILL.md" ] && src="$c" && break
  done
  [ -n "$src" ] || { say "  (dingtalk-misc not found in skills bundle; skipped)"; return 0; }

  # cache so `dws skill setup --mode multi` can find a source later
  cache="$HOME/.dws/skills/multi/$SKILL_NAME"
  refresh_cache_tree "$src" "$cache" || { printf '  ⚠️  Skill 缓存刷新失败: %s\n' "$cache" >&2; return 1; }

  canonical="$HOME/.agents/skills/$SKILL_NAME"
  copy_tree "$src" "$canonical" || return 1
  installed=1
  failed=0
  retire_failed=0
  for agent_dir in $(agent_skill_dirs); do
    base="$(resolve_agent_skill_base "$agent_dir")"
    agent_skill_base_detected "$agent_dir" "$base" || continue
    same_physical_skill "$base" "$HOME/.agents/skills" && continue
    if is_cleanup_only_agent_dir "$agent_dir"; then
      # Cleanup-only: universal Agents read the canonical store directly, so
      # nothing is installed here. A retire failure only leaves an obsolete copy
      # behind and must never fail an otherwise complete install.
      if ! backup_skill_dir "$base/$SKILL_NAME" >/dev/null; then
        printf '  ⚠️  Agent Skill 旧副本备份失败，保留原目录（不影响本次安装）: %s\n' "$base/$SKILL_NAME" >&2
        retire_failed=$((retire_failed + 1))
      fi
      continue
    fi
    # Per-agent degrade like install.sh: a failed target is reported and
    # skipped loudly instead of aborting the remaining agents mid-loop.
    if ! link_or_copy_skill "$canonical" "$src" "$base/$SKILL_NAME"; then
      printf '  ⚠️  Agent 目标安装失败，已跳过: %s\n' "$base/$SKILL_NAME" >&2
      failed=$((failed + 1))
      continue
    fi
    installed=$((installed + 1))
  done
  if [ "$retire_failed" -gt 0 ]; then
    printf '  ⚠️  有 %s 个 Agent 旧副本未能迁移（安装已完成，可稍后手动删除）\n' "$retire_failed" >&2
  fi
  if [ "$failed" -gt 0 ]; then
    printf '  ⚠️  有 %s 个 Agent 目标安装 %s 失败\n' "$failed" "$SKILL_NAME" >&2
    return 1
  fi
  say "✅ Skill dingtalk-misc → ${installed} agent dir(s)"
}

main() {
  resolve_version
  os="$(detect_os)"; arch="$(detect_arch)"
  tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT INT TERM

  printf '\n'
  say "dws dev installer (pre-built binary)"
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
  verify_release_asset "$asset" "$tmp/$asset"
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
      verify_release_asset "dws-skills.zip" "$tmp/skills.zip"
      mkdir -p "$tmp/sk"
      if command -v unzip >/dev/null 2>&1; then unzip -q "$tmp/skills.zip" -d "$tmp/sk"; else tar -xf "$tmp/skills.zip" -C "$tmp/sk"; fi
      say ""
      install_skill "$tmp/sk" || err "dingtalk-misc Skill 安装失败，详见上方告警"
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
