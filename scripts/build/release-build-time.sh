#!/bin/sh
# Match GoReleaser's .CommitDate: committer time, normalized to UTC RFC3339.
# Both core and launcher must use this reproducible value, never wall-clock time.
set -eu

[ "$#" -eq 1 ] || { echo 'usage: release-build-time.sh <full-commit>' >&2; exit 1; }
printf '%s\n' "$1" | grep -Eq '^[0-9a-f]{40}$' || {
  echo 'release build time requires a full lowercase commit SHA' >&2
  exit 1
}
ROOT="$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)"
commit="$(git -C "$ROOT" rev-parse --verify "$1^{commit}")"
[ "$commit" = "$1" ] || { echo 'release commit did not resolve exactly' >&2; exit 1; }
TZ=UTC git -C "$ROOT" -c log.showSignature=false show --no-patch \
  --format=%cd --date=format-local:%Y-%m-%dT%H:%M:%SZ "$commit"
