#!/usr/bin/env bash
# Verify public DWS delivery channels without replacing the caller's dws.

set -uo pipefail

REPO="${DWS_VERIFY_REPO:-DingTalk-Real-AI/dingtalk-workspace-cli}"
TAP="${DWS_VERIFY_HOMEBREW_TAP:-DingTalk-Real-AI/dingtalk-workspace-cli}"
TAP_REPO_URL="${DWS_VERIFY_HOMEBREW_REPO_URL:-https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli.git}"
PACKAGE="${DWS_VERIFY_NPM_PACKAGE:-dingtalk-workspace-cli}"
ROOT="$(mktemp -d "${TMPDIR:-/tmp}/dws-six-channel-XXXXXX")"
RESULTS="$ROOT/results"
mkdir -p "$RESULTS"

cleanup() { rm -rf "$ROOT"; }
trap cleanup EXIT INT TERM

pass() { printf 'PASS\t%s\t%s\n' "$1" "$2" > "$RESULTS/$1"; }
fail() { printf 'FAIL\t%s\t%s\n' "$1" "$2" > "$RESULTS/$1"; }
skip() { printf 'SKIP\t%s\t%s\n' "$1" "$2" > "$RESULTS/$1"; }

run_step() {
  name="$1"
  shift
  printf '\n==> %s\n' "$name"
  if "$@"; then
    pass "$name" "install, version check and smoke test succeeded"
  else
    status=$?
    fail "$name" "command failed with exit code $status"
  fi
}

smoke() {
  binary="$1"
  test -x "$binary" || return 1
  "$binary" version
  "$binary" --help >/dev/null
}

verify_curl() (
  set -e
  home="$ROOT/curl/home"
  bin="$ROOT/curl/bin"
  mkdir -p "$home" "$bin"
  HOME="$home" DWS_INSTALL_DIR="$bin" DWS_NO_SKILLS=1 \
    bash <(curl -fsSL "https://raw.githubusercontent.com/$REPO/main/scripts/install.sh")
  smoke "$bin/dws"
  rm -f "$bin/dws"
)

verify_powershell() (
  set -e
  home="$ROOT/powershell/home"
  bin="$ROOT/powershell/bin"
  mkdir -p "$home" "$bin"
  HOME="$home" DWS_INSTALL_DIR="$bin" DWS_NO_SKILLS=1 pwsh -NoLogo -NoProfile -Command \
    "Invoke-RestMethod 'https://raw.githubusercontent.com/$REPO/main/scripts/install.ps1' | Invoke-Expression"
  smoke "$bin/dws.exe"
  rm -f "$bin/dws.exe"
)

verify_npm() (
  set -e
  tag="$1"
  home="$ROOT/npm-$tag/home"
  prefix="$ROOT/npm-$tag/prefix"
  cache="$ROOT/npm-$tag/cache"
  mkdir -p "$home" "$prefix" "$cache"
  HOME="$home" npm_config_prefix="$prefix" npm_config_cache="$cache" \
    npm uninstall -g "$PACKAGE" >/dev/null 2>&1 || true
  HOME="$home" npm_config_prefix="$prefix" npm_config_cache="$cache" \
    npm install -g "$PACKAGE@$tag"
  smoke "$prefix/bin/dws"
  HOME="$home" npm_config_prefix="$prefix" npm_config_cache="$cache" \
    npm uninstall -g "$PACKAGE" >/dev/null
)

verify_homebrew() (
  set -e
  installed_formula=""
  added_tap=0
  cleanup_brew() {
    [[ -n "$installed_formula" ]] && brew uninstall "$installed_formula" >/dev/null 2>&1 || true
    [[ "$added_tap" == "1" ]] && brew untap "$TAP" >/dev/null 2>&1 || true
  }
  trap cleanup_brew EXIT INT TERM

  for existing_formula in "$PACKAGE" "$PACKAGE-beta"; do
    if brew list --formula "$existing_formula" >/dev/null 2>&1; then
      printf 'Refusing to remove the existing Homebrew installation of %s.\n' "$existing_formula" >&2
      return 1
    fi
  done
  if ! brew tap | grep -Fx "$TAP" >/dev/null 2>&1; then
    brew tap "$TAP" "$TAP_REPO_URL"
    added_tap=1
  fi
  brew install -y "$TAP/$PACKAGE"
  installed_formula="$PACKAGE"
  smoke "$(brew --prefix "$PACKAGE")/bin/dws"
  brew uninstall "$installed_formula" >/dev/null
  installed_formula=""

  brew install -y "$TAP/$PACKAGE-beta"
  installed_formula="$PACKAGE-beta"
  smoke "$(brew --prefix "$PACKAGE-beta")/bin/dws"
)

verify_upgrade() (
  set -e
  home="$ROOT/upgrade/home"
  bin="$ROOT/upgrade/bin"
  mkdir -p "$home" "$bin"
  HOME="$home" DWS_INSTALL_DIR="$bin" DWS_NO_SKILLS=1 \
    bash <(curl -fsSL "https://raw.githubusercontent.com/$REPO/main/scripts/install.sh")
  printf 'before: %s\n' "$($bin/dws version --format json)"
  HOME="$home" "$bin/dws" upgrade --force --skip-skills -y
  printf 'after:  %s\n' "$($bin/dws version --format json)"
  smoke "$bin/dws"
  rm -f "$bin/dws"
)

run_step curl verify_curl

if [[ "${OS:-}" == "Windows_NT" ]] && command -v pwsh >/dev/null 2>&1; then
  run_step powershell verify_powershell
else
  skip powershell "requires native Windows and pwsh"
fi

if command -v npm >/dev/null 2>&1; then
  run_step npm-stable verify_npm latest
  run_step npm-beta verify_npm beta
else
  skip npm-stable "npm is not installed"
  skip npm-beta "npm is not installed"
fi

if [[ "$(uname -s)" == "Darwin" ]] && command -v brew >/dev/null 2>&1; then
  run_step homebrew verify_homebrew
else
  skip homebrew "requires macOS and Homebrew"
fi

run_step dws-upgrade verify_upgrade

printf '\n%-8s  %-14s  %s\n' STATUS CHANNEL DETAIL
printf '%s\n' '--------  --------------  ----------------------------------------------'
failed=0
for name in curl powershell npm-stable npm-beta homebrew dws-upgrade; do
  IFS=$'\t' read -r status channel detail < "$RESULTS/$name"
  printf '%-8s  %-14s  %s\n' "$status" "$channel" "$detail"
  [[ "$status" == "FAIL" ]] && failed=1
done

exit "$failed"
