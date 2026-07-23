#!/bin/sh
set -eu

TAG="${1:-}"
[ -n "$TAG" ] || {
  printf 'usage: release-tag-oss-mode.sh <tag>\n' >&2
  exit 2
}

tag_ref="refs/tags/$TAG"
git rev-parse --verify --quiet "$tag_ref" >/dev/null || {
  printf 'release tag is not available: %s\n' "$TAG" >&2
  exit 1
}
tag_object="$(git rev-parse "$tag_ref")"
[ "$(git cat-file -t "$tag_object")" = tag ] || {
  printf 'release tag must be annotated: %s\n' "$TAG" >&2
  exit 1
}
tag_contents="$(git cat-file tag "$tag_object")" || {
  printf 'release tag could not be read: %s\n' "$TAG" >&2
  exit 1
}

mode="$(
  printf '%s\n' "$tag_contents" |
    awk '
      {
        line = $0
        sub(/\r$/, "", line)
      }
      line ~ /^OSS-Mirror: / {
        count++
        value = substr(line, length("OSS-Mirror: ") + 1)
      }
      END {
        if (count > 1) exit 2
        if (count == 0) print "enabled"
        else print value
      }
    '
)" || {
  printf 'release tag contains duplicate OSS-Mirror metadata: %s\n' "$TAG" >&2
  exit 1
}
case "$mode" in
  enabled|deferred) printf '%s\n' "$mode" ;;
  *)
    printf 'release tag contains invalid OSS-Mirror metadata: %s (%s)\n' "$TAG" "$mode" >&2
    exit 1
    ;;
esac
