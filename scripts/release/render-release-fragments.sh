#!/bin/sh
set -eu

CHANGES_DIR="${1:-.changes}"

usage() {
  printf '%s\n' 'usage: render-release-fragments.sh [changes-dir]' >&2
}

[ "$#" -le 1 ] || { usage; exit 2; }
[ -d "$CHANGES_DIR" ] || { printf 'release fragments directory not found: %s\n' "$CHANGES_DIR" >&2; exit 1; }

tmp_root="$(mktemp -d "${TMPDIR:-/tmp}/dws-release-fragments.XXXXXX")"
cleanup() { rm -rf "$tmp_root"; }
trap cleanup EXIT HUP INT TERM

find "$CHANGES_DIR" -mindepth 1 -maxdepth 1 -type f -name '*.md' ! -name 'README.md' -print |
  LC_ALL=C sort >"$tmp_root/files"

[ -s "$tmp_root/files" ] || {
  printf '%s\n' 'no release fragments found; add .changes/<unique-name>.md before preparing a prerelease changelog' >&2
  exit 1
}

validate_fragment() {
  fragment="$1"
  base="$(basename "$fragment")"
  case "$base" in
    [a-z0-9]*.md) ;;
    *) printf 'invalid release fragment filename: %s\n' "$fragment" >&2; return 1 ;;
  esac

  [ "$(sed -n '1p' "$fragment")" = '---' ] &&
    [ "$(sed -n '3p' "$fragment")" = '---' ] || {
      printf 'invalid release fragment header: %s\n' "$fragment" >&2
      return 1
    }
  case "$(sed -n '2p' "$fragment")" in
    'category: Added'|'category: Changed'|'category: Deprecated'|'category: Removed'|'category: Fixed'|'category: Security') ;;
    *) printf 'invalid release fragment category: %s\n' "$fragment" >&2; return 1 ;;
  esac

  body="$(sed -n '4,$p' "$fragment")"
  printf '%s\n' "$body" | grep -Eq '^- [^[:space:]].*' || {
    printf 'release fragment must contain a non-empty Markdown list item: %s\n' "$fragment" >&2
    return 1
  }
  if printf '%s\n' "$body" | grep -Eqi '(^|[^[:alnum:]_])(TODO|TBD)([^[:alnum:]_]|$)'; then
    printf 'release fragment must not contain TODO/TBD: %s\n' "$fragment" >&2
    return 1
  fi
}

while IFS= read -r fragment; do
  validate_fragment "$fragment"
done <"$tmp_root/files"

for category in Added Changed Deprecated Removed Fixed Security; do
  category_files="$tmp_root/$category"
  : >"$category_files"
  while IFS= read -r fragment; do
    if [ "$(sed -n '2p' "$fragment")" = "category: $category" ]; then
      printf '%s\n' "$fragment" >>"$category_files"
    fi
  done <"$tmp_root/files"
  [ -s "$category_files" ] || continue

  printf '### %s\n\n' "$category"
  while IFS= read -r fragment; do
    awk 'NR >= 4 { if ($0 ~ /[^[:space:]]/) started = 1; if (started) print }' "$fragment"
    printf '\n'
  done <"$category_files"
done
