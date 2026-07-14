#!/usr/bin/env bash
# Verify public DWS delivery channels without replacing the caller's dws.
#
# Every channel installs into an isolated location, asserts that the binary
# reports the version the channel advertises, runs a smoke test, then cleans
# up. A wrong or stale version fails the channel instead of reporting PASS.

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
    pass "$name" "install, version assertion and smoke test succeeded"
  else
    status=$?
    fail "$name" "command failed with exit code $status"
  fi
}

# smoke <binary> <expected_version>
# Runs `dws version --format json` and requires an exact version match. Missing
# package-manager metadata is a channel failure because it cannot prove that the
# installed binary came from the advertised channel.
smoke() {
  local binary="$1"
  local expected="${2:-}"
  local version_output
  local reported

  test -x "$binary" || { printf 'binary not executable: %s\n' "$binary" >&2; return 1; }
  if [[ -z "$expected" ]]; then
    printf 'expected version is empty; cannot verify channel provenance\n' >&2
    return 1
  fi

  version_output="$("$binary" version --format json 2>&1)" || {
    printf 'dws version failed for %s\n' "$binary" >&2
    return 1
  }
  printf 'version output: %s\n' "$version_output"
  reported="$(
    printf '%s\n' "$version_output" \
      | sed -nE 's/.*"version"[[:space:]]*:[[:space:]]*"v?([^\"]+)".*/\1/p' \
      | head -n1
  )"
  if [[ -z "$reported" ]]; then
    printf 'could not parse version from: %s\n' "$version_output" >&2
    return 1
  fi
  expected="${expected#v}"
  if [[ "$reported" != "$expected" ]]; then
    printf 'version mismatch: expected %s, got %s\n' "$expected" "$reported" >&2
    return 1
  fi
  "$binary" --help >/dev/null
}

# Latest stable release tag drives the curl and dws-upgrade assertions.
resolve_latest_version() {
  tag="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" 2>/dev/null \
    | grep -m1 '"tag_name"' \
    | sed -E 's/.*"tag_name" *: *"([^"]+)".*/\1/')"
  printf '%s' "${tag#v}"
}
LATEST_VERSION="$(resolve_latest_version)"

verify_curl() (
  set -e
  home="$ROOT/curl/home"
  bin="$ROOT/curl/bin"
  mkdir -p "$home" "$bin"
  HOME="$home" DWS_INSTALL_DIR="$bin" DWS_NO_SKILLS=1 \
    bash <(curl -fsSL "https://raw.githubusercontent.com/$REPO/main/scripts/install.sh")
  smoke "$bin/dws" "$LATEST_VERSION"
  rm -f "$bin/dws"
)

verify_powershell() (
  set -e
  home="$ROOT/powershell/home"
  bin="$ROOT/powershell/bin"
  mkdir -p "$home" "$bin"
  HOME="$home" DWS_INSTALL_DIR="$bin" DWS_NO_SKILLS=1 pwsh -NoLogo -NoProfile -Command \
    "Invoke-RestMethod 'https://raw.githubusercontent.com/$REPO/main/scripts/install.ps1' | Invoke-Expression"
  smoke "$bin/dws.exe" "$LATEST_VERSION"
  rm -f "$bin/dws.exe"
)

verify_npm() (
  set -e
  tag="$1"
  home="$ROOT/npm-$tag/home"
  prefix="$ROOT/npm-$tag/prefix"
  cache="$ROOT/npm-$tag/cache"
  mkdir -p "$home" "$prefix" "$cache"
  expected="$(HOME="$home" npm_config_cache="$cache" npm view "$PACKAGE@$tag" version 2>/dev/null | tail -n1)"
  HOME="$home" npm_config_prefix="$prefix" npm_config_cache="$cache" \
    npm uninstall -g "$PACKAGE" >/dev/null 2>&1 || true
  HOME="$home" npm_config_prefix="$prefix" npm_config_cache="$cache" \
    npm install -g "$PACKAGE@$tag"
  smoke "$prefix/bin/dws" "$expected"
  HOME="$home" npm_config_prefix="$prefix" npm_config_cache="$cache" \
    npm uninstall -g "$PACKAGE" >/dev/null
)

verify_homebrew() (
  set -e
  installed_formulae=()
  added_tap=0
  cleanup_brew() {
    for formula in "${installed_formulae[@]:-}"; do
      [[ -n "$formula" ]] && brew uninstall "$formula" >/dev/null 2>&1 || true
    done
    [[ "$added_tap" == "1" ]] && brew untap "$TAP" >/dev/null 2>&1 || true
  }
  trap cleanup_brew EXIT INT TERM

  # Never touch a Homebrew install the caller already owns.
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

  # Install stable and record its state before beta lands.
  brew install "$TAP/$PACKAGE"
  installed_formulae+=("$PACKAGE")
  stable_version="$(brew list --versions "$PACKAGE" | awk '{print $2}')"
  stable_bin="$(brew --prefix "$PACKAGE")/bin/dws"
  linked_dws="$(brew --prefix)/bin/dws"
  smoke "$stable_bin" "$stable_version"
  stable_sha_before="$(shasum -a 256 "$stable_bin" | awk '{print $1}')"
  stable_link_before="$(readlink "$linked_dws" 2>/dev/null || true)"

  # Install keg-only beta alongside the still-present stable formula.
  brew install "$TAP/$PACKAGE-beta"
  installed_formulae+=("$PACKAGE-beta")
  beta_version="$(brew list --versions "$PACKAGE-beta" | awk '{print $2}')"
  smoke "$(brew --prefix "$PACKAGE-beta")/bin/dws" "$beta_version"

  # Coexistence: the beta install must not disturb stable.
  if ! brew list --formula "$PACKAGE" >/dev/null 2>&1; then
    printf 'stable formula disappeared after installing beta\n' >&2
    return 1
  fi
  stable_version_after="$(brew list --versions "$PACKAGE" | awk '{print $2}')"
  stable_sha_after="$(shasum -a 256 "$stable_bin" | awk '{print $1}')"
  stable_link_after="$(readlink "$linked_dws" 2>/dev/null || true)"
  if [[ "$stable_version_after" != "$stable_version" ]]; then
    printf 'stable version changed after beta install: %s -> %s\n' "$stable_version" "$stable_version_after" >&2
    return 1
  fi
  if [[ "$stable_sha_after" != "$stable_sha_before" ]]; then
    printf 'stable binary changed after beta install\n' >&2
    return 1
  fi
  if [[ "$stable_link_after" != "$stable_link_before" ]]; then
    printf 'stable link changed after beta install: %s -> %s\n' "$stable_link_before" "$stable_link_after" >&2
    return 1
  fi
  # The linked dws on PATH must still be stable, not the keg-only beta.
  smoke "$linked_dws" "$stable_version"
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
  smoke "$bin/dws" "$LATEST_VERSION"
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
