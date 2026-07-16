#!/bin/sh
set -eu

TAG="${1:-}"
EXPECTED_COMMIT="${2:-}"
REPOSITORY="${GITHUB_REPOSITORY:-}"

[ -n "$TAG" ] && [ -n "$EXPECTED_COMMIT" ] && [ -n "$REPOSITORY" ] || {
  printf 'usage: GITHUB_REPOSITORY=owner/repo verify-github-tag-authority.sh <tag> <commit>\n' >&2
  exit 2
}
command -v gh >/dev/null 2>&1 || { printf 'gh is required\n' >&2; exit 1; }

local_object="$(git rev-parse "refs/tags/$TAG")"
remote_ref="$(
  gh api -H 'Accept: application/vnd.github+json' \
    "repos/$REPOSITORY/git/ref/tags/$TAG" \
    --jq '[.object.type, .object.sha] | @tsv'
)"
[ "$remote_ref" = "$(printf 'tag\t%s' "$local_object")" ] || {
  printf 'remote tag object for %s differs from the sealed annotated tag (local=%s remote=%s)\n' \
    "$TAG" "$local_object" "$remote_ref" >&2
  exit 1
}

remote_tag="$(
  gh api -H 'Accept: application/vnd.github+json' \
    "repos/$REPOSITORY/git/tags/$local_object" \
    --jq '[.tag, .object.type, .object.sha] | @tsv'
)"
[ "$remote_tag" = "$(printf '%s\tcommit\t%s' "$TAG" "$EXPECTED_COMMIT")" ] || {
  printf 'remote annotated tag %s does not peel to expected commit %s (got: %s)\n' \
    "$TAG" "$EXPECTED_COMMIT" "$remote_tag" >&2
  exit 1
}

printf 'GitHub tag authority verified: %s -> %s\n' "$TAG" "$EXPECTED_COMMIT"
