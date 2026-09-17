#!/bin/sh
set -eu

# Release compatibility is one decision seam over the sealed source tree.
# Trusted release tooling may orchestrate this script, but the explicitly
# selected repository remains the authority for both CLI and Schema checks.

REPO_ROOT=""
BASE_REF=""
STABLE_REF=""
CANDIDATE_REF="HEAD"

usage() {
  printf '%s\n' \
    "usage: $0 --repo-root <path> --base-ref <ref> --stable-ref <ref> [--candidate-ref <ref>]" >&2
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --repo-root)
      [ "$#" -ge 2 ] || { usage; exit 2; }
      REPO_ROOT="$2"
      shift 2
      ;;
    --base-ref)
      [ "$#" -ge 2 ] || { usage; exit 2; }
      BASE_REF="$2"
      shift 2
      ;;
    --stable-ref)
      [ "$#" -ge 2 ] || { usage; exit 2; }
      STABLE_REF="$2"
      shift 2
      ;;
    --candidate-ref)
      [ "$#" -ge 2 ] || { usage; exit 2; }
      CANDIDATE_REF="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      printf 'error: unknown argument: %s\n' "$1" >&2
      usage
      exit 2
      ;;
  esac
done

[ -n "$REPO_ROOT" ] && [ -n "$BASE_REF" ] && [ -n "$STABLE_REF" ] || {
  usage
  exit 2
}

REPO_ROOT="$(CDPATH= cd -- "$REPO_ROOT" && pwd -P)" || {
  printf 'error: release source root is not available: %s\n' "$REPO_ROOT" >&2
  exit 2
}
GIT_ROOT="$(git -C "$REPO_ROOT" rev-parse --show-toplevel 2>/dev/null)" || {
  printf 'error: release source root is not a Git worktree: %s\n' "$REPO_ROOT" >&2
  exit 2
}
[ "$GIT_ROOT" = "$REPO_ROOT" ] || {
  printf 'error: release source root must be the Git worktree root: %s\n' "$REPO_ROOT" >&2
  exit 2
}

CLI_CHECK="$REPO_ROOT/scripts/policy/check-authoritative-interface-baselines.sh"
SCHEMA_CHECK="$REPO_ROOT/scripts/policy/check-authoritative-schema-compatibility.sh"
for check in "$CLI_CHECK" "$SCHEMA_CHECK"; do
  [ -x "$check" ] || {
    printf 'error: authoritative release compatibility checker is unavailable: %s\n' "$check" >&2
    exit 2
  }
done

resolve_commit() {
  label="$1"
  ref="$2"
  commit="$(git -C "$REPO_ROOT" rev-parse --verify "${ref}^{commit}" 2>/dev/null)" || {
    printf 'error: release %s ref is not available in sealed source: %s\n' "$label" "$ref" >&2
    return 2
  }
  [ -n "$commit" ] || {
    printf 'error: release %s ref resolved to an empty commit: %s\n' "$label" "$ref" >&2
    return 2
  }
  printf '%s\n' "$commit"
}

# Freeze the complete comparison tuple before either checker runs. Otherwise a
# concurrent ref update (or the first checker itself) could make Schema inspect
# different Git objects from the CLI gate even when both receive the same names.
BASE_COMMIT="$(resolve_commit base "$BASE_REF")"
STABLE_COMMIT="$(resolve_commit stable "$STABLE_REF")"
CANDIDATE_COMMIT="$(resolve_commit candidate "$CANDIDATE_REF")"

printf '==> Checking authoritative CLI compatibility\n'
if "$CLI_CHECK" \
    --base-ref "$BASE_COMMIT" \
    --stable-ref "$STABLE_COMMIT" \
    --candidate-ref "$CANDIDATE_COMMIT"; then
  :
else
  status=$?
  printf 'error: authoritative CLI compatibility failed\n' >&2
  exit "$status"
fi

printf '==> Checking authoritative Schema compatibility\n'
if "$SCHEMA_CHECK" \
    --base-ref "$BASE_COMMIT" \
    --stable-ref "$STABLE_COMMIT" \
    --candidate-ref "$CANDIDATE_COMMIT"; then
  :
else
  status=$?
  printf 'error: authoritative Schema compatibility failed\n' >&2
  exit "$status"
fi
