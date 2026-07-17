#!/bin/sh
set -eu

REPOSITORY="${1:-${GITHUB_REPOSITORY:-}}"

case "$REPOSITORY" in
  */*) ;;
  *)
    printf 'usage: %s <owner/repository>\n' "$0" >&2
    exit 2
    ;;
esac

command -v gh >/dev/null 2>&1 || {
  printf 'gh is required to verify immutable release governance\n' >&2
  exit 1
}

error_log="$(mktemp "${TMPDIR:-/tmp}/dws-immutable-governance.XXXXXX")"
cleanup() { rm -f "$error_log"; }
trap cleanup EXIT HUP INT TERM

if ! immutable_enabled="$(
  gh api \
    -H 'Accept: application/vnd.github+json' \
    -H 'X-GitHub-Api-Version: 2026-03-10' \
    "repos/$REPOSITORY/immutable-releases" \
    --jq '.enabled' \
    2>"$error_log"
)"; then
  if grep -Eqi 'HTTP 403|Resource not accessible by integration|must have admin' "$error_log"; then
    printf '%s\n' \
      "the release governance token cannot read immutable-release settings for $REPOSITORY" \
      "GitHub requires repository Administration: read for this endpoint; the built-in GITHUB_TOKEN is insufficient" \
      "configure RELEASE_GOVERNANCE_TOKEN with that read-only permission, or use an admin-owned HOMEBREW_PR_TOKEN fallback" >&2
  elif grep -Eqi 'HTTP 404|Not Found' "$error_log"; then
    printf 'immutable releases are not enabled for %s, or the governance token cannot read that repository\n' "$REPOSITORY" >&2
  else
    printf 'could not verify immutable release governance for %s\n' "$REPOSITORY" >&2
    sed -n '1,3p' "$error_log" >&2
  fi
  exit 1
fi

[ "$immutable_enabled" = "true" ] || {
  printf 'immutable releases are not enabled for %s\n' "$REPOSITORY" >&2
  exit 1
}

printf 'Immutable release governance is enabled for %s.\n' "$REPOSITORY"
