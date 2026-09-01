#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
DIST_DIR="${DWS_PACKAGE_DIST_DIR:-$ROOT/dist}"
FORMULA_PATH="$DIST_DIR/homebrew/dingtalk-workspace-cli-local.rb"
NPM_STAGE_DIR="$DIST_DIR/npm/dingtalk-workspace-cli"
RUN_NPM=1
RUN_BREW=1
EXPECTED_VERSION=""
VERIFY_SKILL_TARGETS=1

say() {
  printf '%s\n' "$*"
}

err() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || err "missing required command: $1"
}

need_file() {
  [ -f "$1" ] || err "required file not found: $1"
}

usage() {
  printf '%s\n' "usage: $0 [--npm-only|--brew-only] [--expected-version <vX.Y.Z>] [--skip-skill-targets]" >&2
}

mode_seen=0
while [ "$#" -gt 0 ]; do
  case "$1" in
    --npm-only)
      [ "$mode_seen" -eq 0 ] || { usage; exit 2; }
      RUN_NPM=1; RUN_BREW=0; mode_seen=1; shift
      ;;
    --brew-only)
      [ "$mode_seen" -eq 0 ] || { usage; exit 2; }
      RUN_NPM=0; RUN_BREW=1; mode_seen=1; shift
      ;;
    --expected-version)
      [ "$#" -ge 2 ] || { usage; exit 2; }
      EXPECTED_VERSION="${2#v}"
      shift 2
      ;;
    --skip-skill-targets) VERIFY_SKILL_TARGETS=0; shift ;;
    -h|--help) usage; exit 0 ;;
    *) usage; exit 2 ;;
  esac
done

TMP_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/dws-package-verify-XXXXXX")"
# vercel-labs/skills agents.ts (c6f69c6), exactly 76 upstream IDs.
# Format: id|universal(1/0)|effective default global root; '-' has no global root.
UPSTREAM_AGENT_REGISTRY='aider-desk|0|.aider-desk/skills
amp|1|.config/agents/skills
antigravity|1|.gemini/antigravity/skills
antigravity-cli|1|.gemini/antigravity-cli/skills
astrbot|0|.astrbot/data/skills
autohand-code|0|.autohand/skills
augment|0|.augment/skills
bob|0|.bob/skills
claude-code|0|.claude/skills
openclaw|0|.openclaw/skills
cline|1|.agents/skills
codearts-agent|0|.codeartsdoer/skills
codebuddy|0|.codebuddy/skills
codemaker|0|.codemaker/skills
codestudio|0|.codestudio/skills
codex|1|.codex/skills
command-code|0|.commandcode/skills
continue|0|.continue/skills
cortex|0|.snowflake/cortex/skills
crush|0|.config/crush/skills
cursor|1|.cursor/skills
deepagents|1|.deepagents/agent/skills
devin|0|.config/devin/skills
dexto|1|.agents/skills
droid|0|.factory/skills
eve|0|-
firebender|1|.firebender/skills
forgecode|0|.forge/skills
gemini-cli|1|.gemini/skills
github-copilot|1|.copilot/skills
goose|0|.config/goose/skills
grok|0|.grok/skills
hermes-agent|0|.hermes/skills
inference-sh|0|.inferencesh/skills
jazz|0|.jazz/skills
junie|0|.junie/skills
iflow-cli|0|.iflow/skills
kilo|0|.kilocode/skills
kimchi|0|.config/kimchi/harness/skills
kimi-code-cli|1|.agents/skills
kiro-cli|0|.kiro/skills
kode|0|.kode/skills
lingma|0|.lingma/skills
loaf|1|.agents/skills
mcpjam|0|.mcpjam/skills
minimax-code|0|.minimax/skills
mistral-vibe|0|.vibe/skills
moxby|0|.moxby/skills
mux|0|.mux/skills
opencode|1|.config/opencode/skills
openhands|0|.openhands/skills
ona|0|.ona/skills
pi|0|.pi/agent/skills
qoder|0|.qoder/skills
qoder-cn|0|.qoder-cn/skills
qwen-code|0|.qwen/skills
replit|1|.config/agents/skills
reasonix|0|.reasonix/skills
rovodev|0|.rovodev/skills
roo|0|.roo/skills
tabnine-cli|0|.tabnine/agent/skills
terramind|0|.terramind/skills
tinycloud|0|.tinycloud/skills
trae|0|.trae/skills
trae-cn|0|.trae-cn/skills
warp|1|.agents/skills
windsurf|0|.codeium/windsurf/skills
zed|1|.agents/skills
zcode|0|.zcode/skills
zencoder|0|.zencoder/skills
zenflow|0|.zencoder/skills
neovate|0|.neovate/skills
pochi|0|.pochi/skills
promptscript|1|-
adal|0|.adal/skills
universal|1|.config/agents/skills'

# DWS-only compatibility and old wrong global paths. Qoderwork remains a
# non-universal install target; the other entries verify migration cleanup.
LEGACY_AGENT_CLEANUP_REGISTRY='dws-qoderwork|0|.qoderwork/skills
dws-legacy-github|1|.github/skills
dws-legacy-amp|1|.amp/skills
dws-legacy-cline|1|.cline/skills
dws-legacy-windsurf|1|.windsurf/skills'
cleanup() {
  if [ "$RUN_BREW" -eq 1 ] && command -v brew >/dev/null 2>&1 && [ -n "${BREW_TAP_NAME:-}" ]; then
    HOME="$TMP_ROOT/brew-home" HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_INSTALL_CLEANUP=1 \
      brew uninstall --force dingtalk-workspace-cli-local >/dev/null 2>&1 || true
    HOME="$TMP_ROOT/brew-home" HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_INSTALL_CLEANUP=1 \
      brew untap --force "$BREW_TAP_NAME" >/dev/null 2>&1 || true
  fi
  rm -rf "$TMP_ROOT"
}
trap cleanup EXIT INT TERM

seed_specific_agent_homes() {
  home_root="$1"
  seen=''
  for row in $UPSTREAM_AGENT_REGISTRY $LEGACY_AGENT_CLEANUP_REGISTRY; do
    id=${row%%|*}; rest=${row#*|}; path=${rest#*|}
    [ "$path" != '-' ] || continue
    base=$(resolve_agent_base "$home_root" "$id" "$path")
    [ -n "$base" ] || continue
    key=$(printf '%s' "$base" | tr '[:upper:]' '[:lower:]')
    printf '%s\n' "$seen" | grep -Fqx "$key" && continue
    seen="${seen}${key}
"
    mkdir -p "${base%/skills}"
  done
}

resolve_agent_base() {
  home_root="$1"; id="$2"; default_path="$3"
  case "$id" in
    autohand-code) printf '%s\n' "${AUTOHAND_HOME:-$home_root/.autohand}/skills" ;;
    claude-code) printf '%s\n' "${CLAUDE_CONFIG_DIR:-$home_root/.claude}/skills" ;;
    codex) printf '%s\n' "${CODEX_HOME:-$home_root/.codex}/skills" ;;
    grok) printf '%s\n' "${GROK_HOME:-$home_root/.grok}/skills" ;;
    hermes-agent) printf '%s\n' "${HERMES_HOME:-$home_root/.hermes}/skills" ;;
    mistral-vibe) printf '%s\n' "${VIBE_HOME:-$home_root/.vibe}/skills" ;;
    amp|replit|universal) printf '%s\n' "${XDG_CONFIG_HOME:-$home_root/.config}/agents/skills" ;;
    crush|devin|goose|kimchi|opencode) printf '%s\n' "${XDG_CONFIG_HOME:-$home_root/.config}/${default_path#.config/}" ;;
    openclaw)
      for alias in .openclaw .clawdbot .moltbot; do
        if [ -d "$home_root/$alias" ]; then printf '%s\n' "$home_root/$alias/skills"; return; fi
      done
      printf '%s\n' "$home_root/.openclaw/skills"
      ;;
    *) printf '%s\n' "$home_root/$default_path" ;;
  esac
}

# Package verification must never inherit the runner's real Agent roots. In
# particular, hosted Linux runners can define XDG_CONFIG_HOME globally; two
# parallel verification jobs would then publish links into the same directory
# and make one another's links appear broken. Keep every supported override
# inside the scenario-specific temporary HOME instead.
with_isolated_agent_env() (
  isolated_home=$1
  shift
  HOME="$isolated_home"
  AUTOHAND_HOME="$isolated_home/.autohand"
  CLAUDE_CONFIG_DIR="$isolated_home/.claude"
  CODEX_HOME="$isolated_home/.codex"
  GROK_HOME="$isolated_home/.grok"
  HERMES_HOME="$isolated_home/.hermes"
  VIBE_HOME="$isolated_home/.vibe"
  XDG_CONFIG_HOME="$isolated_home/.config"
  export HOME AUTOHAND_HOME CLAUDE_CONFIG_DIR CODEX_HOME GROK_HOME HERMES_HOME VIBE_HOME XDG_CONFIG_HOME
  "$@"
)

verify_skill_base() {
  home_root="$1"
  base="$2"
  need_file "$home_root/$base/dingtalk-shared/SKILL.md"
  need_file "$home_root/$base/dingtalk-misc/SKILL.md"
  [ ! -e "$home_root/$base/dws" ] || err "unexpected mono Skill layout found in $home_root/$base/dws"
}

verify_compatible_skill_base() {
  home_root="$1"; base="$2"; canonical="$home_root/.agents/skills"
  for skill_md in "$canonical"/*/SKILL.md; do
    [ -f "$skill_md" ] || continue
    name=$(basename "$(dirname "$skill_md")")
    target="$base/$name"
    if [ -L "$target" ]; then
      link_target="$(readlink "$target")" || err "could not read canonical Skill link at $target"
      # Relative link targets are the headline invariant of the canonical store:
      # an absolute target breaks as soon as the home directory moves or the
      # store is relocated, so reject it even though it resolves today.
      case "$link_target" in
        /*) err "canonical Skill link must be relative at $target (target=$link_target)" ;;
      esac
      linked_real="$(CDPATH= cd -- "$target" 2>/dev/null && pwd -P)" || err "broken canonical Skill link at $target"
      canonical_real="$(CDPATH= cd -- "$canonical/$name" && pwd -P)" || err "canonical Skill missing at $canonical/$name"
      [ "$linked_real" = "$canonical_real" ] || err "Skill link does not resolve to canonical source at $target"
    elif [ -d "$target" ]; then
      diff -qr "$canonical/$name" "$target" >/dev/null || err "Skill copy fallback differs from canonical source at $target"
    else
      err "canonical Skill compatibility target missing at $target"
    fi
  done
  [ ! -e "$base/dws" ] && [ ! -L "$base/dws" ] || err "unexpected mono Skill layout found in $base/dws"
}

# Universal Agents read the canonical store directly, so they must own no copy
# of ANY canonical Skill. Iterate the real store instead of a hardcoded name
# list: a hardcoded list silently passed leftover copies of every other
# dingtalk-* Skill from earlier betas.
verify_universal_skill_base() {
  home_root="$1"; base="$2"; canonical="$home_root/.agents/skills"
  for skill_md in "$canonical"/*/SKILL.md; do
    [ -f "$skill_md" ] || continue
    name=$(basename "$(dirname "$skill_md")")
    { [ ! -e "$base/$name" ] && [ ! -L "$base/$name" ]; } || \
      err "unexpected duplicate Skill found in universal Agent root $base/$name"
  done
  # The mono layout is absent from a multi canonical store, so gate it directly.
  { [ ! -e "$base/dws" ] && [ ! -L "$base/dws" ]; } || \
    err "unexpected duplicate Skill found in universal Agent root $base/dws"
}

verify_skill_targets() {
  home_root="$1"
  # The universal-convention canonical store is always present, regardless of
  # which concrete Agent homes existed at install time.
  verify_skill_base "$home_root" ".agents/skills"
  seen=''
  for row in $UPSTREAM_AGENT_REGISTRY $LEGACY_AGENT_CLEANUP_REGISTRY; do
    id=${row%%|*}; rest=${row#*|}; universal=${rest%%|*}; path=${rest#*|}
    [ "$path" != '-' ] || continue
    base=$(resolve_agent_base "$home_root" "$id" "$path")
    [ -n "$base" ] || continue
    key=$(printf '%s' "$base" | tr '[:upper:]' '[:lower:]')
    printf '%s\n' "$seen" | grep -Fqx "$key" && continue
    seen="${seen}${key}
"
    [ "$base" != "$home_root/.agents/skills" ] || continue
    [ -d "${base%/skills}" ] || continue
    if [ "$universal" -eq 1 ]; then
      verify_universal_skill_base "$home_root" "$base"
      continue
    fi
    verify_compatible_skill_base "$home_root" "$base"
  done
}

verify_npm_install() {
  tarball_path="$1"
  scenario="$2"
  npm_home="$TMP_ROOT/npm-home-$scenario"
  npm_prefix="$TMP_ROOT/npm-prefix-$scenario"
  npm_cache="$TMP_ROOT/npm-cache-$scenario"
  mkdir -p "$npm_home" "$npm_prefix" "$npm_cache"
  if [ "$scenario" = "specific-agent-roots" ]; then
    with_isolated_agent_env "$npm_home" seed_specific_agent_homes "$npm_home"
  fi

  say "==> verifying npm package install ($scenario)"
  with_isolated_agent_env "$npm_home" env npm_config_cache="$npm_cache" npm_config_prefix="$npm_prefix" \
    npm install -g "$tarball_path" >/dev/null

  [ -x "$npm_prefix/bin/dws" ] || err "npm install did not expose dws in $npm_prefix/bin"
  need_file "$npm_prefix/lib/node_modules/dingtalk-workspace-cli/vendor/.dws-runtime/20260825/manifest.json"
  with_isolated_agent_env "$npm_home" "$npm_prefix/bin/dws" --help >/dev/null
  if [ -n "$EXPECTED_VERSION" ]; then
    vendor_bin="$npm_prefix/lib/node_modules/dingtalk-workspace-cli/vendor/dws"
    need_file "$vendor_bin"
    LC_ALL=C grep -aFq "v$EXPECTED_VERSION" "$vendor_bin" || \
      err "npm-installed binary does not contain expected version marker v$EXPECTED_VERSION"
    EXPECTED_VERSION="$EXPECTED_VERSION" node -e '
      const pkg = require(process.argv[1]);
      if (pkg.version !== process.env.EXPECTED_VERSION) process.exit(1);
    ' "$NPM_STAGE_DIR/package.json" || err "npm package.json version mismatch"
  fi
  [ "$VERIFY_SKILL_TARGETS" -eq 0 ] || with_isolated_agent_env "$npm_home" verify_skill_targets "$npm_home"

  with_isolated_agent_env "$npm_home" env npm_config_cache="$npm_cache" npm_config_prefix="$npm_prefix" \
    npm uninstall -g dingtalk-workspace-cli >/dev/null
}

verify_npm() {
  need_cmd npm
  need_cmd node
  need_cmd tar
  need_cmd unzip
  need_cmd diff
  need_file "$NPM_STAGE_DIR/package.json"

  tarball_name="$(
    cd "$NPM_STAGE_DIR"
    HOME="$TMP_ROOT/npm-pack-home" npm_config_cache="$TMP_ROOT/npm-pack-cache" npm pack --silent
  )"
  tarball_path="$NPM_STAGE_DIR/$tarball_name"
  need_file "$tarball_path"

  verify_npm_install "$tarball_path" "specific-agent-roots"
  verify_npm_install "$tarball_path" "generic-fallback"

  rm -f "$tarball_path"
}

verify_brew() {
  need_cmd brew
  need_file "$FORMULA_PATH"

  brew_home="$TMP_ROOT/brew-home"
  mkdir -p "$brew_home"
  BREW_TAP_NAME="local/dws-package-verify-$$"

  say "==> verifying Homebrew formula install"
  HOME="$brew_home" HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_INSTALL_CLEANUP=1 \
    brew uninstall --force dingtalk-workspace-cli-local >/dev/null 2>&1 || true
  HOME="$brew_home" HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_INSTALL_CLEANUP=1 \
    brew untap --force "$BREW_TAP_NAME" >/dev/null 2>&1 || true
  HOME="$brew_home" HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_INSTALL_CLEANUP=1 \
    brew tap-new --no-git "$BREW_TAP_NAME" >/dev/null

  tap_repo="$(
    HOME="$brew_home" HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_INSTALL_CLEANUP=1 \
      brew --repository "$BREW_TAP_NAME"
  )"
  mkdir -p "$tap_repo/Formula"
  cp "$FORMULA_PATH" "$tap_repo/Formula/dingtalk-workspace-cli-local.rb"

  HOME="$brew_home" HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_INSTALL_CLEANUP=1 \
    brew install -y "$BREW_TAP_NAME/dingtalk-workspace-cli-local" >/dev/null

  prefix="$(
    HOME="$brew_home" HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_INSTALL_CLEANUP=1 \
      brew --prefix dingtalk-workspace-cli-local
  )"
  [ -x "$prefix/bin/dws" ] || err "brew install did not create $prefix/bin/dws"
  need_file "$prefix/libexec/.dws-runtime/20260825/manifest.json"
  runtime_ps_count="$(find "$prefix/libexec/.dws-runtime/20260825/ps" -type f | wc -l | tr -d ' ')"
  [ "$runtime_ps_count" = 123 ] || err "Homebrew runtime payload contains $runtime_ps_count ps files; expected 123"
  "$prefix/bin/dws" --help >/dev/null
  if [ -n "$EXPECTED_VERSION" ]; then
    LC_ALL=C grep -aFq "v$EXPECTED_VERSION" "$prefix/bin/dws" || \
      err "Homebrew-installed binary does not contain expected version marker v$EXPECTED_VERSION"
  fi
  need_file "$prefix/share/dingtalk-workspace-cli-local/skills/dws/SKILL.md"
}

need_file "$DIST_DIR/dws-skills.zip"

[ "$RUN_NPM" -eq 0 ] || verify_npm
[ "$RUN_BREW" -eq 0 ] || verify_brew

say "Package-manager verification complete."
