#!/bin/sh
# Copyright 2026 Alibaba Group
# Licensed under the Apache License, Version 2.0 (the "License");

set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"

usage() {
  printf '%s\n' "usage: $0 <changed|list> BASE_REF HEAD_REF" >&2
  exit 2
}

[ "${1:-}" = changed ] || [ "${1:-}" = list ] || usage
[ "$#" -eq 3 ] || usage

MODE="$1"
BASE_REF="$2"
HEAD_REF="$3"

cd "$ROOT"
git rev-parse --verify --quiet "${BASE_REF}^{commit}" >/dev/null || {
  printf 'changed test package base is not a commit: %s\n' "$BASE_REF" >&2
  exit 2
}
git rev-parse --verify --quiet "${HEAD_REF}^{commit}" >/dev/null || {
  printf 'changed test package head is not a commit: %s\n' "$HEAD_REF" >&2
  exit 2
}
resolved_head="$(git rev-parse "${HEAD_REF}^{commit}")"
current_head="$(git rev-parse HEAD)"
[ "$current_head" = "$resolved_head" ] || {
  printf 'changed test package head does not match the checked-out revision: expected %s, got %s\n' \
    "$resolved_head" "$current_head" >&2
  exit 2
}
git diff --quiet HEAD -- && git diff --cached --quiet HEAD -- || {
  printf '%s\n' 'changed test package planning requires a clean tracked worktree' >&2
  exit 2
}

workdir="$(mktemp -d "${TMPDIR:-/tmp}/dws-changed-packages.XXXXXX")"
trap 'rm -rf "$workdir"' EXIT HUP INT TERM
files="$workdir/files"
changed_packages="$workdir/changed-packages"
impacted_packages="$workdir/impacted-packages"
all_packages="$workdir/all-packages"
package_dependencies="$workdir/package-dependencies"
embed_owners="$workdir/embed-owners"
: > "$changed_packages"
: > "$impacted_packages"

git diff --no-ext-diff --find-renames --name-only \
  --diff-filter=ACMRD "$BASE_REF" "$HEAD_REF" > "$files"

if ! go list ./... > "$all_packages"; then
  printf '%s\n' 'failed to resolve the module package graph' >&2
  exit 1
fi
if ! go list -f \
  '{{- $import := .ImportPath -}}{{- $dir := .Dir -}}{{- range .EmbedFiles }}{{$import}}|{{$dir}}|{{.}}{{"\n"}}{{- end -}}{{- range .TestEmbedFiles }}{{$import}}|{{$dir}}|{{.}}{{"\n"}}{{- end -}}{{- range .XTestEmbedFiles }}{{$import}}|{{$dir}}|{{.}}{{"\n"}}{{- end -}}' \
  ./... > "$embed_owners"; then
  printf '%s\n' 'failed to resolve embedded file ownership' >&2
  exit 1
fi

while IFS= read -r file; do
  [ -n "$file" ] || continue
  case "$file" in
    CHANGELOG.md|README.md|README_zh.md|CONTRIBUTING.md|SECURITY.md|\
      CODE_OF_CONDUCT.md|LICENSE|NOTICE|docs/*|\
      .github/PULL_REQUEST_TEMPLATE.md|.github/ISSUE_TEMPLATE/*)
      continue
      ;;
  esac

  directory="$(dirname "$file")"
  if [ "$directory" = . ]; then
    pattern=.
  else
    pattern="./$directory"
  fi
  if [ -d "$directory" ]; then
    has_go_files=false
    for go_file in "$directory"/*.go; do
      if [ -f "$go_file" ]; then
        has_go_files=true
        break
      fi
    done
    if [ "$has_go_files" = true ]; then
      if ! package="$(go list -f '{{.ImportPath}}' "$pattern")"; then
        printf 'failed to resolve changed Go package: %s\n' "$directory" >&2
        exit 1
      fi
      printf '%s\n' "$package" >> "$changed_packages"
      continue
    fi
  fi

  embedded_owner_found=false
  while IFS='|' read -r package package_dir embedded_file; do
    [ -n "$package" ] || continue
    case "$package_dir" in
      "$ROOT") embedded_path="$embedded_file" ;;
      "$ROOT"/*)
        embedded_path="${package_dir#"$ROOT"/}/$embedded_file"
        ;;
      *) continue ;;
    esac
    if [ "$embedded_path" = "$file" ]; then
      printf '%s\n' "$package" >> "$changed_packages"
      embedded_owner_found=true
    fi
  done < "$embed_owners"
  [ "$embedded_owner_found" = true ] || continue
done < "$files"

LC_ALL=C sort -u "$changed_packages" -o "$changed_packages"
if [ "$MODE" = changed ] || [ ! -s "$changed_packages" ]; then
  cat "$changed_packages"
  exit 0
fi

while IFS= read -r package; do
  if ! go list -deps -test "$package" > "$package_dependencies"; then
    printf 'failed to resolve dependencies for Go package: %s\n' "$package" >&2
    exit 1
  fi
  if grep -Fxf "$changed_packages" "$package_dependencies" >/dev/null; then
    printf '%s\n' "$package"
  fi
done < "$all_packages" > "$impacted_packages"

cat "$changed_packages" >> "$impacted_packages"
LC_ALL=C sort -u "$impacted_packages"
