#!/bin/sh

# Shared release version and CHANGELOG helpers. This file is sourced by the
# local release command, the CI contract, and mirror publishing so every stage
# agrees on what "prerelease" and "stable" mean.

release_stable_pattern='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$'
release_prerelease_pattern='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)-beta\.[1-9][0-9]*$'

release_is_stable_version() {
  printf '%s\n' "$1" | grep -Eq "$release_stable_pattern"
}

release_is_prerelease_version() {
  printf '%s\n' "$1" | grep -Eq "$release_prerelease_pattern"
}

release_channel_for_version() {
  _rcfv_version="$1"
  if release_is_stable_version "$_rcfv_version"; then
    printf '%s\n' stable
    return 0
  fi
  if release_is_prerelease_version "$_rcfv_version"; then
    printf '%s\n' prerelease
    return 0
  fi
  printf 'invalid release version: %s (expected vX.Y.Z or vX.Y.Z-beta.N)\n' "$_rcfv_version" >&2
  return 1
}

release_validate_version_channel() {
  _rvvc_expected="$1"
  _rvvc_version="$2"
  case "$_rvvc_expected" in
    stable|prerelease) ;;
    *)
      printf 'invalid release channel: %s (expected prerelease or stable)\n' "$_rvvc_expected" >&2
      return 1
      ;;
  esac
  _rvvc_actual="$(release_channel_for_version "$_rvvc_version")" || return 1
  if [ "$_rvvc_actual" != "$_rvvc_expected" ]; then
    printf 'release channel/version mismatch: channel=%s version=%s\n' "$_rvvc_expected" "$_rvvc_version" >&2
    return 1
  fi
}

release_semver() {
  printf '%s\n' "${1#v}"
}

release_core_tag() {
  _rct_version="$1"
  printf '%s\n' "${_rct_version%%-beta.*}"
}

release_beta_number() {
  printf '%s\n' "${1##*.}"
}

release_core_is_greater() {
  _rcig_candidate="$(release_core_tag "$1")"
  _rcig_baseline="$(release_core_tag "$2")"
  _rcig_candidate="${_rcig_candidate#v}"
  _rcig_baseline="${_rcig_baseline#v}"
  awk -v candidate="$_rcig_candidate" -v baseline="$_rcig_baseline" 'BEGIN {
    split(candidate, c, ".")
    split(baseline, b, ".")
    for (i = 1; i <= 3; i++) {
      if ((c[i] + 0) > (b[i] + 0)) exit 0
      if ((c[i] + 0) < (b[i] + 0)) exit 1
    }
    exit 1
  }'
}

release_version_is_greater() {
  _rvig_candidate="$1"
  _rvig_baseline="$2"
  _rvig_candidate_channel="$(release_channel_for_version "$_rvig_candidate")" || return 1
  _rvig_baseline_channel="$(release_channel_for_version "$_rvig_baseline")" || return 1

  if release_core_is_greater "$_rvig_candidate" "$_rvig_baseline"; then
    return 0
  fi
  if release_core_is_greater "$_rvig_baseline" "$_rvig_candidate"; then
    return 1
  fi
  if [ "$_rvig_candidate_channel" = "stable" ] && [ "$_rvig_baseline_channel" = "prerelease" ]; then
    return 0
  fi
  if [ "$_rvig_candidate_channel" = "prerelease" ] && [ "$_rvig_baseline_channel" = "prerelease" ]; then
    [ "$(release_beta_number "$_rvig_candidate")" -gt "$(release_beta_number "$_rvig_baseline")" ]
    return
  fi
  return 1
}

# Extract one exact CHANGELOG section (without its H2 heading) and validate that
# it is dated, unique, non-placeholder content with at least one bullet.
release_extract_changelog() {
  _rec_changelog="$1"
  _rec_semver="$2"
  _rec_output="$3"
  _rec_tmp="$(mktemp "${TMPDIR:-/tmp}/dws-release-notes.XXXXXX")"

  set +e
  awk -v wanted="$_rec_semver" '
    BEGIN {
      prefix = "## [" wanted "] - "
      found = 0
      active = 0
      invalid_date = 0
      meaningful = 0
      bullet = 0
      placeholder = 0
    }
    /^## / {
      active = 0
      if (index($0, prefix) == 1) {
        found++
        active = 1
        date = substr($0, length(prefix) + 1)
        if (date !~ /^[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]$/) invalid_date = 1
      }
      next
    }
    active {
      print
      if ($0 ~ /[^[:space:]]/) meaningful = 1
      if ($0 ~ /^- /) bullet = 1
      lowered = tolower($0)
      if (lowered ~ /todo|tbd/) placeholder = 1
    }
    END {
      if (found != 1) exit 41
      if (invalid_date) exit 42
      if (!meaningful || !bullet) exit 43
      if (placeholder) exit 44
    }
  ' "$_rec_changelog" > "$_rec_tmp"
  _rec_status=$?
  set -e

  case "$_rec_status" in
    0) ;;
    41) printf 'CHANGELOG must contain exactly one section: ## [%s] - YYYY-MM-DD\n' "$_rec_semver" >&2 ;;
    42) printf 'CHANGELOG section for %s has an invalid date\n' "$_rec_semver" >&2 ;;
    43) printf 'CHANGELOG section for %s must contain release notes and at least one bullet\n' "$_rec_semver" >&2 ;;
    44) printf 'CHANGELOG section for %s still contains TODO/TBD placeholders\n' "$_rec_semver" >&2 ;;
    *) printf 'failed to parse CHANGELOG section for %s\n' "$_rec_semver" >&2 ;;
  esac
  if [ "$_rec_status" -ne 0 ]; then
    rm -f "$_rec_tmp"
    return "$_rec_status"
  fi

  if [ "$_rec_output" = "-" ]; then
    cat "$_rec_tmp"
  else
    mkdir -p "$(dirname "$_rec_output")"
    mv "$_rec_tmp" "$_rec_output"
    _rec_tmp=""
  fi
  [ -z "$_rec_tmp" ] || rm -f "$_rec_tmp"
}
