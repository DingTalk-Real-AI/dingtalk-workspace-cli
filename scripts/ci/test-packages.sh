#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
MODULE="$(cd "$ROOT" && go list -m)"

usage() {
  printf '%s\n' \
    "usage: $0 list <app|generators|helpers|cli|smoke|remaining|release-scripts>" \
    "       $0 list-coverage <app|cli|generators|helpers|remaining>" \
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
    cli)
      # Owns package-cli Schema TestMain assembly (cmd_schema_catalog dump).
      go list ./internal/cli/...
      ;;
    smoke)
      # Owns app.NewRootCommand public-tree smoke under -race (heavy assembly).
      go list ./test/smoke/...
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
          "$MODULE/internal/cli"|"$MODULE/internal/cli/"*) ;;
          "$MODULE/test/smoke"|"$MODULE/test/smoke/"*) ;;
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

# Full-suite coverage measurement shards the same authoritative package set the
# single serial run used: ./ ./cmd/... ./internal/... ./skills/... (pkg/ and
# scripts/ belong to the supporting profile; test/ suites carry no production
# statements for the candidate profile). Shards stay disjoint so their merged
# profile is block-for-block equivalent to one serial invocation.
list_coverage_scope() {
  cd "$ROOT"
  go list ./ ./cmd/... ./internal/... ./skills/...
}

list_coverage_shard() {
  shard="$1"
  cd "$ROOT"

  case "$shard" in
    app)
      go list ./internal/app/...
      ;;
    cli)
      go list ./internal/cli/...
      ;;
    generators)
      go list ./internal/generator/...
      ;;
    helpers)
      go list ./internal/helpers/...
      ;;
    remaining)
      scope_packages="$(list_coverage_scope)"
      printf '%s\n' "$scope_packages" | while IFS= read -r package; do
        case "$package" in
          "$MODULE/internal/app"|"$MODULE/internal/app/"*) ;;
          "$MODULE/internal/cli"|"$MODULE/internal/cli/"*) ;;
          "$MODULE/internal/generator"|"$MODULE/internal/generator/"*) ;;
          "$MODULE/internal/helpers"|"$MODULE/internal/helpers/"*) ;;
          *) printf '%s\n' "$package" ;;
        esac
      done
      ;;
    *)
      printf 'unknown coverage package shard: %s\n' "$shard" >&2
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

  for shard in app generators helpers cli smoke remaining release-scripts; do
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

  coverage_expected="$workdir/coverage-expected"
  coverage_assigned="$workdir/coverage-assigned"
  coverage_unique="$workdir/coverage-unique"
  coverage_duplicates="$workdir/coverage-duplicates"
  coverage_missing="$workdir/coverage-missing"
  coverage_unexpected="$workdir/coverage-unexpected"

  list_coverage_scope > "$workdir/coverage-scope"
  LC_ALL=C sort -u "$workdir/coverage-scope" > "$coverage_expected"
  : > "$coverage_assigned"

  for shard in app cli generators helpers remaining; do
    shard_packages="$workdir/coverage-$shard"
    unsorted_shard_packages="$workdir/coverage-$shard.unsorted"
    list_coverage_shard "$shard" > "$unsorted_shard_packages"
    LC_ALL=C sort "$unsorted_shard_packages" > "$shard_packages"
    if [ ! -s "$shard_packages" ]; then
      printf 'coverage package shard is empty: %s\n' "$shard" >&2
      exit 1
    fi
    cat "$shard_packages" >> "$coverage_assigned"
  done

  LC_ALL=C sort "$coverage_assigned" -o "$coverage_assigned"
  uniq -d "$coverage_assigned" > "$coverage_duplicates"
  LC_ALL=C sort -u "$coverage_assigned" > "$coverage_unique"
  comm -23 "$coverage_expected" "$coverage_unique" > "$coverage_missing"
  comm -13 "$coverage_expected" "$coverage_unique" > "$coverage_unexpected"

  coverage_failed=0
  if [ -s "$coverage_duplicates" ]; then
    printf '%s\n' 'coverage packages assigned to more than one shard:' >&2
    sed 's/^/  /' "$coverage_duplicates" >&2
    coverage_failed=1
  fi
  if [ -s "$coverage_missing" ]; then
    printf '%s\n' 'full-suite coverage packages missing from the CI coverage plan:' >&2
    sed 's/^/  /' "$coverage_missing" >&2
    coverage_failed=1
  fi
  if [ -s "$coverage_unexpected" ]; then
    printf '%s\n' 'CI coverage plan contains packages outside its scope:' >&2
    sed 's/^/  /' "$coverage_unexpected" >&2
    coverage_failed=1
  fi
  if [ "$coverage_failed" -ne 0 ]; then
    exit 1
  fi

  coverage_count="$(wc -l < "$coverage_expected" | tr -d ' ')"
  printf 'coverage package plan covers %s full-suite packages exactly once\n' "$coverage_count"
}

case "${1:-}" in
  list)
    [ "$#" -eq 2 ] || usage
    list_shard "$2"
    ;;
  list-coverage)
    [ "$#" -eq 2 ] || usage
    list_coverage_shard "$2"
    ;;
  verify)
    [ "$#" -eq 1 ] || usage
    verify_plan
    ;;
  *)
    usage
    ;;
esac
