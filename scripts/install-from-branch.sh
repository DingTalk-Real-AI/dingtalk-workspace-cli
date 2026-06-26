#!/bin/sh
# Copyright 2026 Alibaba Group
# Licensed under the Apache License, Version 2.0
#
# Build and install dws directly from a Git branch checkout.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/shangguanxuan633-lab/dingtalk-workspace-cli/codex/dws-multi-profile-login/scripts/install-from-branch.sh | sh
#
# Environment variables:
#   DWS_SOURCE_REPO      owner/repo to clone (default: shangguanxuan633-lab/dingtalk-workspace-cli)
#   DWS_SOURCE_BRANCH    branch to build (default: codex/dws-multi-profile-login)
#   DWS_INSTALL_DIR      passed through to scripts/install.sh (default there: ~/.local/bin)
#   DWS_INSTALL_NAME     passed through to scripts/install.sh (default: dws)
#   DWS_NO_SKILLS        passed through to scripts/install.sh (set 1 to skip skills)
#   DWS_KEEP_SOURCE      set 1 to keep the temporary source checkout

set -eu

REPO="${DWS_SOURCE_REPO:-shangguanxuan633-lab/dingtalk-workspace-cli}"
BRANCH="${DWS_SOURCE_BRANCH:-codex/dws-multi-profile-login}"
KEEP_SOURCE="${DWS_KEEP_SOURCE:-0}"

say() {
  printf '  %s\n' "$@"
}

err() {
  printf '  ❌ %s\n' "$@" >&2
  exit 1
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || err "Missing required command: $1"
}

need_cmd git
need_cmd sh

tmpdir="$(mktemp -d 2>/dev/null || mktemp -d -t dws-src)"
cleanup() {
  if [ "$KEEP_SOURCE" != "1" ]; then
    rm -rf "$tmpdir"
  else
    say "Source checkout kept at: $tmpdir"
  fi
}
trap cleanup EXIT INT TERM

say "Cloning dws source:"
say "  repo:   https://github.com/${REPO}.git"
say "  branch: ${BRANCH}"

git clone --depth 1 --branch "$BRANCH" "https://github.com/${REPO}.git" "$tmpdir"

say "Building and installing from source..."
sh "$tmpdir/scripts/install.sh"
