#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
MODULE="$(cd "$ROOT" && go list -m)"

usage() {
  printf '%s\n' \
    "usage: $0 list <app|generators|helpers|remaining|release-scripts>" \
    "       $0 verify" >&2
  exit 2
}

list_shard() {
  shard="$1"
  cd "$ROOT"

  case "$shard" in
    app)
      go list ./internal/app/...
      ;;
    generators)
      go list ./internal/generator/...
      ;;
    helpers)
      go list ./internal/helpers/...
      ;;
    release-scripts)
      go list ./test/scripts/...
      ;;
    remaining)
      all_packages="$(go list ./...)"
      printf '%s\n' "$all_packages" | while IFS= read -r package; do
        case "$package" in
          "$MODULE/internal/app"|"$MODULE/internal/app/"*) ;;
          "$MODULE/internal/generator"|"$MODULE/internal/generator/"*) ;;
          "$MODULE/internal/helpers"|"$MODULE/internal/helpers/"*) ;;
          "$MODULE/test/scripts"|"$MODULE/test/scripts/"*) ;;
          *) printf '%s\n' "$package" ;;
        esac
      done
      ;;
    *)
      printf 'unknown test package shard: %s\n' "$shard" >&2
      exit 2
      ;;
  esac
}

verify_plan() {
  workdir="$(mktemp -d "${TMPDIR:-/tmp}/dws-test-packages.XXXXXX")"
  trap 'rm -rf "$workdir"' EXIT HUP INT TERM

  expected="$workdir/expected"
  assigned="$workdir/assigned"
  unique="$workdir/unique"
  duplicates="$workdir/duplicates"
  missing="$workdir/missing"
  unexpected="$workdir/unexpected"
  all_packages="$workdir/all-packages"

  cd "$ROOT"
  go list ./... > "$all_packages"
  LC_ALL=C sort -u "$all_packages" > "$expected"
  : > "$assigned"

  for shard in app generators helpers remaining release-scripts; do
    shard_packages="$workdir/$shard"
    unsorted_shard_packages="$workdir/$shard.unsorted"
    list_shard "$shard" > "$unsorted_shard_packages"
    LC_ALL=C sort "$unsorted_shard_packages" > "$shard_packages"
    if [ ! -s "$shard_packages" ]; then
      printf 'test package shard is empty: %s\n' "$shard" >&2
      exit 1
    fi
    cat "$shard_packages" >> "$assigned"
  done

  LC_ALL=C sort "$assigned" -o "$assigned"
  uniq -d "$assigned" > "$duplicates"
  LC_ALL=C sort -u "$assigned" > "$unique"
  comm -23 "$expected" "$unique" > "$missing"
  comm -13 "$expected" "$unique" > "$unexpected"

  failed=0
  if [ -s "$duplicates" ]; then
    printf '%s\n' 'test packages assigned to more than one shard:' >&2
    sed 's/^/  /' "$duplicates" >&2
    failed=1
  fi
  if [ -s "$missing" ]; then
    printf '%s\n' 'default Go packages missing from the CI test plan:' >&2
    sed 's/^/  /' "$missing" >&2
    failed=1
  fi
  if [ -s "$unexpected" ]; then
    printf '%s\n' 'CI test plan contains packages outside go list ./...:' >&2
    sed 's/^/  /' "$unexpected" >&2
    failed=1
  fi
  if [ "$failed" -ne 0 ]; then
    exit 1
  fi

  package_count="$(wc -l < "$expected" | tr -d ' ')"
  printf 'test package plan covers %s default packages exactly once\n' "$package_count"
}

case "${1:-}" in
  list)
    [ "$#" -eq 2 ] || usage
    list_shard "$2"
    ;;
  verify)
    [ "$#" -eq 1 ] || usage
    verify_plan
    ;;
  *)
    usage
    ;;
esac
