#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 2 ]; then
  printf 'usage: %s <binary> <expected-version>\n' "$0" >&2
  exit 2
fi

binary="$1"
expected_version="$2"

[ -f "$binary" ] || {
  printf 'OpenHarmony artifact does not exist: %s\n' "$binary" >&2
  exit 1
}
[ -x "$binary" ] || {
  printf 'OpenHarmony artifact is not executable: %s\n' "$binary" >&2
  exit 1
}

file_info="$(file -b "$binary" 2>/dev/null)" || {
  printf 'file could not read the OpenHarmony artifact\n' >&2
  exit 1
}
printf '%s\n' "$file_info" | grep -Eiq 'ELF' || {
  printf 'OpenHarmony artifact is not ELF: %s\n' "$file_info" >&2
  exit 1
}

if command -v readelf >/dev/null 2>&1; then
  reader=readelf
elif command -v llvm-readelf >/dev/null 2>&1; then
  reader=llvm-readelf
else
  printf 'readelf or llvm-readelf is required for fail-closed OpenHarmony verification\n' >&2
  exit 1
fi

header=''
program_headers=''
dynamic=''
if ! header="$("$reader" -h "$binary" 2>/dev/null)"; then
  printf '%s could not read the OpenHarmony ELF header\n' "$reader" >&2
  exit 1
fi
if ! printf '%s\n' "$header" | grep -Eq 'Type:[[:space:]]*EXEC([[:space:]]|\(|$)'; then
  printf 'OpenHarmony artifact is not ET_EXEC\n' >&2
  exit 1
fi
if ! printf '%s\n' "$header" | grep -Eq 'Machine:[[:space:]]*AArch64([[:space:]]|$)'; then
  printf 'OpenHarmony artifact is not AArch64\n' >&2
  exit 1
fi
if ! program_headers="$("$reader" -l "$binary" 2>/dev/null)"; then
  printf '%s could not read the OpenHarmony ELF program headers\n' "$reader" >&2
  exit 1
fi
if printf '%s\n' "$program_headers" | grep -Eq 'INTERP|DYNAMIC'; then
  printf 'OpenHarmony artifact has an interpreter or dynamic segment\n' >&2
  exit 1
fi
if ! dynamic="$("$reader" -d "$binary" 2>/dev/null)"; then
  printf '%s could not read the OpenHarmony ELF dynamic section\n' "$reader" >&2
  exit 1
fi
if ! printf '%s\n' "$dynamic" | grep -q 'There is no dynamic section in this file'; then
  printf 'OpenHarmony artifact has a dynamic section or NEEDED entry\n' >&2
  exit 1
fi

go_bin="$(command -v go 2>/dev/null || true)"
[ -n "$go_bin" ] || {
  printf 'go is required to inspect OpenHarmony build metadata\n' >&2
  exit 1
}
build_info=""
if ! build_info="$("$go_bin" version -m "$binary" 2>/dev/null)"; then
  printf 'go could not read OpenHarmony build metadata\n' >&2
  exit 1
fi
printf '%s\n' "$build_info" | grep -Eq 'build[[:space:]]+GOOS=openharmony([[:space:]]|$)' || {
  printf 'OpenHarmony build metadata is missing GOOS=openharmony\n' >&2
  exit 1
}
printf '%s\n' "$build_info" | grep -Eq 'build[[:space:]]+GOARCH=arm64([[:space:]]|$)' || {
  printf 'OpenHarmony build metadata is missing GOARCH=arm64\n' >&2
  exit 1
}
printf '%s\n' "$build_info" | grep -Eq 'build[[:space:]]+CGO_ENABLED=0([[:space:]]|$)' || {
  printf 'OpenHarmony build metadata is missing CGO_ENABLED=0\n' >&2
  exit 1
}

grep -aFq -- "$expected_version" "$binary" || {
  printf 'OpenHarmony artifact does not contain expected version: %s\n' "$expected_version" >&2
  exit 1
}
if [ -n "${DWS_EXPECTED_COMMIT:-}" ]; then
  grep -aFq -- "$DWS_EXPECTED_COMMIT" "$binary" || {
    printf 'OpenHarmony artifact does not contain expected commit: %s\n' "$DWS_EXPECTED_COMMIT" >&2
    exit 1
  }
fi

printf 'Verified OpenHarmony artifact: %s (%s)\n' "$binary" "$expected_version"
