#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"

usage() { printf '%s\n' 'usage: check-release-fragments.sh BASE HEAD' >&2; }

[ "$#" -eq 2 ] || { usage; exit 2; }
base="$(git -C "$ROOT" rev-parse --verify --quiet "$1^{commit}")" || { usage; exit 2; }
head="$(git -C "$ROOT" rev-parse --verify --quiet "$2^{commit}")" || { usage; exit 2; }
merge_base="$(git -C "$ROOT" merge-base "$base" "$head")"
tmp_root="$(mktemp -d "${TMPDIR:-/tmp}/dws-release-fragment-policy.XXXXXX")"
cleanup() { rm -rf "$tmp_root"; }
trap cleanup EXIT HUP INT TERM

git -C "$ROOT" diff --no-ext-diff --find-renames --name-status "$merge_base" "$head" >"$tmp_root/status"

archive_changed=false
if awk -F '\t' '{ for (field = 2; field <= NF; field++) if ($field ~ /^\.changes\/released\//) found = 1 } END { exit !found }' "$tmp_root/status"; then
  archive_changed=true
fi

if [ "$archive_changed" = true ]; then
  release_version="$(git -C "$ROOT" diff --no-ext-diff --unified=0 "$merge_base" "$head" -- CHANGELOG.md | sed -n 's/^+## \[\([0-9][0-9.]*\(-beta\.[1-9][0-9]*\)\{0,1\}\)\] - .*/\1/p')"
  [ "$(printf '%s\n' "$release_version" | sed '/^$/d' | wc -l | tr -d '[:space:]')" -eq 1 ] || {
    printf '%s\n' 'error: release-fragment archival requires exactly one newly added versioned CHANGELOG section' >&2
    exit 1
  }
  if ! awk -F '\t' -v version="$release_version" '
    $1 == "M" && $2 == "CHANGELOG.md" && NF == 2 { changelog = 1; next }
    $1 ~ /^R100$/ && $2 ~ /^\.changes\/[a-z0-9][a-z0-9._-]*\.md$/ && $3 ~ ("^\\.changes/released/" version "/[a-z0-9][a-z0-9._-]*\\.md$") && NF == 3 {
      source = $2; target = $3; sub(/^.*\//, "", source); sub(/^.*\//, "", target)
      if (source != target) invalid = 1
      moved++; next
    }
    { invalid = 1 }
    END { exit !(changelog && moved > 0 && !invalid) }
  ' "$tmp_root/status"; then
    printf '%s\n' 'error: release fragments must be unchanged R100 moves from .changes/<name>.md to .changes/released/<new-version>/<name>.md in the matching release-seal PR' >&2
    exit 1
  fi
  source_changes="$tmp_root/source-changes"
  mkdir -p "$source_changes"
  git -C "$ROOT" ls-tree -r --name-only "$merge_base" -- .changes |
    while IFS= read -r path; do
      case "$path" in
        .changes/*.md)
          name="${path#.changes/}"
          mkdir -p "$(dirname "$source_changes/$name")"
          git -C "$ROOT" show "$merge_base:$path" >"$source_changes/$name"
          ;;
      esac
    done
  "$ROOT/scripts/release/render-release-fragments.sh" "$source_changes" >"$tmp_root/expected-notes"
  git -C "$ROOT" show "$head:CHANGELOG.md" |
    awk -v heading="## [$release_version] - " '
      index($0, heading) == 1 { found = 1; next }
      found && /^## / { exit }
      found { print }
    ' >"$tmp_root/actual-notes"
  normalize_notes() {
    awk '
      /^[[:space:]]*$/ && !started { next }
      { started = 1; lines[++count] = $0 }
      END {
        while (count > 0 && lines[count] ~ /^[[:space:]]*$/) count--
        for (line_no = 1; line_no <= count; line_no++) print lines[line_no]
      }
    ' "$1"
  }
  normalize_notes "$tmp_root/expected-notes" >"$tmp_root/expected-notes.normalized"
  normalize_notes "$tmp_root/actual-notes" >"$tmp_root/actual-notes.normalized"
  if ! cmp -s "$tmp_root/expected-notes.normalized" "$tmp_root/actual-notes.normalized"; then
    printf '%s\n' 'error: release-seal CHANGELOG section does not exactly match the rendered active release fragments' >&2
    diff -u "$tmp_root/expected-notes.normalized" "$tmp_root/actual-notes.normalized" >&2 || true
    exit 1
  fi
else
  if awk -F '\t' '{ for (field = 2; field <= NF; field++) if ($field ~ /^\.changes\/released\//) invalid = 1 } END { exit !invalid }' "$tmp_root/status"; then
    printf '%s\n' 'error: archived release fragments are immutable outside their release-seal PR' >&2
    exit 1
  fi
	if awk -F '\t' '
		$1 == "D" && $2 ~ /^\.changes\/[a-z0-9][a-z0-9._-]*\.md$/ { invalid = 1 }
		$1 ~ /^R[0-9]+$/ && $2 ~ /^\.changes\/[a-z0-9][a-z0-9._-]*\.md$/ { invalid = 1 }
		END { exit !invalid }
	' "$tmp_root/status"; then
		printf '%s\n' 'error: active release fragments may be deleted or renamed only by the matching release-seal archival move' >&2
		exit 1
	fi
fi

if git -C "$ROOT" diff --no-ext-diff --name-only "$merge_base" "$head" -- .changes | grep -Eq '^\.changes/[a-z0-9][a-z0-9._-]*\.md$'; then
  "$ROOT/scripts/release/render-release-fragments.sh" "$ROOT/.changes" >/dev/null
fi
