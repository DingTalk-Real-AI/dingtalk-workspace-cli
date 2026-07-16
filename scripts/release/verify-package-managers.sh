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

[ -z "$EXPECTED_VERSION" ] || need_cmd strings

TMP_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/dws-package-verify-XXXXXX")"
HOME_AGENT_PARENTS="
.claude
.cursor
.qoder
.qoderwork
.gemini
.codex
.github
.windsurf
.augment
.cline
.amp
.kiro
.trae
.openclaw
.hermes
"
HOME_SKILL_TARGETS="
.agents/skills/dws
.claude/skills/dws
.cursor/skills/dws
.qoder/skills/dws
.qoderwork/skills/dws
.gemini/skills/dws
.codex/skills/dws
.github/skills/dws
.windsurf/skills/dws
.augment/skills/dws
.cline/skills/dws
.amp/skills/dws
.kiro/skills/dws
.trae/skills/dws
.openclaw/skills/dws
.hermes/skills/dws
"
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

seed_agent_homes() {
  home_root="$1"
  for parent in $HOME_AGENT_PARENTS; do
    mkdir -p "$home_root/$parent"
  done
}

verify_skill_targets() {
  home_root="$1"
  for target in $HOME_SKILL_TARGETS; do
    need_file "$home_root/$target/SKILL.md"
  done
}

verify_npm() {
  need_cmd npm
  need_cmd node
  need_cmd tar
  need_cmd unzip
  need_file "$NPM_STAGE_DIR/package.json"

  npm_home="$TMP_ROOT/npm-home"
  npm_prefix="$TMP_ROOT/npm-prefix"
  npm_cache="$TMP_ROOT/npm-cache"
  mkdir -p "$npm_home" "$npm_prefix" "$npm_cache"
  seed_agent_homes "$npm_home"

  say "==> verifying npm package install"
  tarball_name="$(
    cd "$NPM_STAGE_DIR"
    HOME="$npm_home" npm_config_cache="$npm_cache" npm pack --silent
  )"
  tarball_path="$NPM_STAGE_DIR/$tarball_name"
  need_file "$tarball_path"

  HOME="$npm_home" npm_config_cache="$npm_cache" npm_config_prefix="$npm_prefix" \
    npm install -g "$tarball_path" >/dev/null

  [ -x "$npm_prefix/bin/dws" ] || err "npm install did not expose dws in $npm_prefix/bin"
  HOME="$npm_home" "$npm_prefix/bin/dws" --help >/dev/null
  if [ -n "$EXPECTED_VERSION" ]; then
    vendor_bin="$npm_prefix/lib/node_modules/dingtalk-workspace-cli/vendor/dws"
    need_file "$vendor_bin"
    strings "$vendor_bin" | grep -Fqx "v$EXPECTED_VERSION" || \
      err "npm-installed binary does not embed expected version v$EXPECTED_VERSION"
    EXPECTED_VERSION="$EXPECTED_VERSION" node -e '
      const pkg = require(process.argv[1]);
      if (pkg.version !== process.env.EXPECTED_VERSION) process.exit(1);
    ' "$NPM_STAGE_DIR/package.json" || err "npm package.json version mismatch"
  fi
  [ "$VERIFY_SKILL_TARGETS" -eq 0 ] || verify_skill_targets "$npm_home"

  HOME="$npm_home" npm_config_cache="$npm_cache" npm_config_prefix="$npm_prefix" \
    npm uninstall -g dingtalk-workspace-cli >/dev/null

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
  "$prefix/bin/dws" --help >/dev/null
  if [ -n "$EXPECTED_VERSION" ]; then
    strings "$prefix/bin/dws" | grep -Fqx "v$EXPECTED_VERSION" || \
      err "Homebrew-installed binary does not embed expected version v$EXPECTED_VERSION"
  fi
  need_file "$prefix/share/dingtalk-workspace-cli-local/skills/dws/SKILL.md"
}

need_file "$DIST_DIR/dws-skills.zip"

[ "$RUN_NPM" -eq 0 ] || verify_npm
[ "$RUN_BREW" -eq 0 ] || verify_brew

say "Package-manager verification complete."
