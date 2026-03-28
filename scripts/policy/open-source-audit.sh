#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
cd "$ROOT"

say() {
  printf '%s\n' "$*"
}

err() {
  printf 'open-source audit: %s\n' "$*" >&2
  exit 1
}

need_file() {
  [ -f "$1" ] || err "missing required file: $1"
}

find_matches() {
  pattern="$1"
  shift
  if command -v rg >/dev/null 2>&1; then
    rg -n "$pattern" "$@"
    return $?
  fi
  grep -REn "$pattern" "$@"
}

require_no_match() {
  pattern="$1"
  shift
  if find_matches "$pattern" "$@" >/dev/null 2>&1; then
    find_matches "$pattern" "$@" >&2 || true
    err "found forbidden pattern: $pattern"
  fi
}

require_heading_if_linked() {
  label="$1"
  heading="$2"
  if grep -nF "$label" README.md >/dev/null 2>&1 && ! grep -nF "$heading" README.md >/dev/null 2>&1; then
    err "README.md links to missing heading: $heading"
  fi
}

need_file "LICENSE"
need_file "NOTICE"
need_file "README.md"
need_file "CHANGELOG.md"
need_file "CONTRIBUTING.md"
need_file "SECURITY.md"
need_file "CODE_OF_CONDUCT.md"
need_file ".env.example"
need_file ".github/workflows/ci.yml"
need_file ".github/PULL_REQUEST_TEMPLATE.md"
need_file "docs/architecture.md"
need_file "scripts/policy/open-source-audit.sh"

[ ! -e "internal/hostcompat" ] || err "forbidden private compatibility directory: internal/hostcompat"
[ ! -e "internal/app/skill_command.go" ] || err "forbidden private command source: internal/app/skill_command.go"

require_no_match 'code\.alibaba-inc\.com|gitlab\.alibaba' README.md CONTRIBUTING.md scripts build .github
require_no_match 'scripts/test\.sh|scripts/check-semantic-fixtures\.sh|scripts/run-command-benchmark\.sh|scripts/run-live-smoke\.sh|scripts/run-model-eval\.sh' scripts/README.md
require_no_match 'scripts/check-semantic-fixtures\.py|test/semantic/cli_to_mcp|test/semantic/model_to_cli|python3 -m pytest test/semantic' CONTRIBUTING.md
require_no_match '\.github/CODEOWNERS|Tag-triggered GitHub Release asset publishing' CHANGELOG.md
require_no_match 'buildMode|EmbeddedMode|WriteTokenMarker|CleanTokenOnExpiry|DeleteExeRelativeTokenOnAuthErr|shouldDeleteEmbeddedTokenOnAuthError|com\.dingtalk\.scenario\.wukong|认证信息已失效，请重新执行上一条命令（最多重试两次）' internal

require_heading_if_linked '[Environment Variables](#environment-variables)' '## Environment Variables'
require_heading_if_linked '[Exit Codes](#exit-codes)' '## Exit Codes'

say "open-source audit: ok"
