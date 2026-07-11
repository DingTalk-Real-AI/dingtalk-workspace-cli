#!/bin/sh
set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
. "$SCRIPT_DIR/release-lib.sh"
ROOT="$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)"

CHANNEL="${1:-}"
VERSION="${2:-}"
FROM_BETA=""
FROM_REF=""
CHANGELOG="$ROOT/CHANGELOG.md"

usage() {
  cat >&2 <<'EOF'
usage: prepare-changelog.sh <prerelease|stable> <version> [options]

Options:
  --from-beta <tag>    Required for stable release notes
  --from-ref <ref>     Commit-list baseline for prerelease notes
  --changelog <path>   Override CHANGELOG.md path
EOF
}

[ -n "$CHANNEL" ] && [ -n "$VERSION" ] || { usage; exit 2; }
shift 2
while [ "$#" -gt 0 ]; do
  case "$1" in
    --from-beta) [ "$#" -ge 2 ] || { usage; exit 2; }; FROM_BETA="$2"; shift 2 ;;
    --from-ref) [ "$#" -ge 2 ] || { usage; exit 2; }; FROM_REF="$2"; shift 2 ;;
    --changelog) [ "$#" -ge 2 ] || { usage; exit 2; }; CHANGELOG="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) printf 'unknown argument: %s\n' "$1" >&2; usage; exit 2 ;;
  esac
done

release_validate_version_channel "$CHANNEL" "$VERSION"
cd "$ROOT"
[ -f "$CHANGELOG" ] || { printf 'CHANGELOG not found: %s\n' "$CHANGELOG" >&2; exit 1; }
[ -z "$(git status --porcelain --untracked-files=all)" ] || {
  printf 'prepare changelog requires a clean worktree\n' >&2
  exit 1
}

semver="$(release_semver "$VERSION")"
if grep -Fq "## [$semver] - " "$CHANGELOG"; then
  printf 'CHANGELOG section already exists for %s\n' "$semver" >&2
  exit 1
fi
if [ "$CHANNEL" = "stable" ]; then
  [ -n "$FROM_BETA" ] || { printf 'stable changelog requires --from-beta\n' >&2; exit 1; }
  release_is_prerelease_version "$FROM_BETA" || { printf 'invalid beta baseline: %s\n' "$FROM_BETA" >&2; exit 1; }
  [ "$(release_core_tag "$FROM_BETA")" = "$VERSION" ] || {
    printf 'stable version %s does not match beta baseline %s\n' "$VERSION" "$FROM_BETA" >&2
    exit 1
  }
  FROM_REF="$FROM_BETA"
fi

release_date="${DWS_RELEASE_DATE:-$(TZ=Asia/Shanghai date +%F)}"
section="$(mktemp "${TMPDIR:-/tmp}/dws-changelog-section.XXXXXX")"
output="$(mktemp "${TMPDIR:-/tmp}/dws-changelog-output.XXXXXX")"
cleanup() { rm -f "$section" "$output"; }
trap cleanup EXIT HUP INT TERM

{
  printf '## [%s] - %s\n\n' "$semver" "$release_date"
  if [ "$CHANNEL" = "stable" ]; then
    printf 'This release promotes the sealed `%s` contents to stable.\n\n' "$FROM_BETA"
  else
    printf '<!-- Summarize what this beta validates. Remove every TODO before publishing. -->\n\n'
  fi
  printf '### Changed\n\n'
  if [ "$CHANNEL" = "stable" ]; then
    printf -- '- TODO: summarize the complete user-visible release promoted from `%s`.\n' "$FROM_BETA"
  else
    printf -- '- TODO: summarize this beta candidate and its validation scope.\n'
  fi
} > "$section"

inserted=0
while IFS= read -r line || [ -n "$line" ]; do
  printf '%s\n' "$line" >> "$output"
  if [ "$line" = '## [Unreleased]' ]; then
    printf '\n' >> "$output"
    cat "$section" >> "$output"
    inserted=1
  fi
done < "$CHANGELOG"
[ "$inserted" -eq 1 ] || { printf 'CHANGELOG is missing ## [Unreleased]\n' >&2; exit 1; }
cp "$output" "$CHANGELOG"

printf 'Prepared CHANGELOG template for %s. Replace TODO, review, commit, and merge it before release.\n' "$VERSION"
if [ -n "$FROM_REF" ] && git rev-parse --verify --quiet "$FROM_REF^{commit}" >/dev/null; then
  printf '\nCommits since %s:\n' "$FROM_REF"
  git log --oneline "$FROM_REF..HEAD"
fi
