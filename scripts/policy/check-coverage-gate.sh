#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
BASE_REF=""
OVERALL_PROFILE="coverage.txt"
ADDITIONAL_DIFF_PROFILE="${COVERAGE_ADDITIONAL_DIFF_PROFILE:-${COVERAGE_ADDITIONAL_PROFILE:-}}"
BASELINE_PROFILE="coverage-base.txt"
DIFF_PROFILE="coverage-policy.txt"
TARGET="${COVERAGE_TARGET:-100}"
OVERALL_TOLERANCE="${COVERAGE_OVERALL_TOLERANCE:-0}"
ENFORCE_OVERALL="${COVERAGE_ENFORCE_OVERALL:-false}"
CHANGED_ONLY="false"
SCOPE_BUILDABLE="false"

usage() {
  printf '%s\n' "usage: $0 --base-ref <ref> [--changed-only] [--scope-buildable] [--overall-profile <file>] [--additional-diff-profile <file>] [--baseline-profile <file>] [--diff-profile <file>]" >&2
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --base-ref)
      [ "$#" -ge 2 ] || { usage; exit 2; }
      BASE_REF="$2"
      shift 2
      ;;
    --overall-profile)
      [ "$#" -ge 2 ] || { usage; exit 2; }
      OVERALL_PROFILE="$2"
      shift 2
      ;;
    --additional-diff-profile|--additional-profile)
      [ "$#" -ge 2 ] || { usage; exit 2; }
      ADDITIONAL_DIFF_PROFILE="$2"
      shift 2
      ;;
    --baseline-profile)
      [ "$#" -ge 2 ] || { usage; exit 2; }
      BASELINE_PROFILE="$2"
      shift 2
      ;;
    --diff-profile)
      [ "$#" -ge 2 ] || { usage; exit 2; }
      DIFF_PROFILE="$2"
      shift 2
      ;;
    --changed-only)
      CHANGED_ONLY="true"
      shift
      ;;
    --scope-buildable)
      SCOPE_BUILDABLE="true"
      shift
      ;;
    *)
      printf 'error: unknown argument: %s\n' "$1" >&2
      usage
      exit 2
      ;;
  esac
done

[ -n "$BASE_REF" ] || { usage; exit 2; }
cd "$ROOT"
. "$ROOT/scripts/policy/policy-runtime.sh"
policy_prepare_runtime "$ROOT"

git rev-parse --verify --quiet "${BASE_REF}^{commit}" >/dev/null || {
  printf 'error: coverage base ref is not available locally: %s\n' "$BASE_REF" >&2
  exit 2
}

TMP_ROOT="$(policy_runtime_mktemp_dir dws-coverage-gate)"
CHECKER="$TMP_ROOT/coverage-gate"
trap 'rm -rf "$TMP_ROOT"' EXIT HUP INT TERM
go build -o "$CHECKER" ./scripts/policy/coverage-gate

module="$(go list -m -f '{{.Path}}')"
set -- "$CHECKER" \
  --diff-profile "$DIFF_PROFILE" \
  --base-ref "$BASE_REF" \
  --module "$module" \
  --overall-tolerance "$OVERALL_TOLERANCE" \
  --target "$TARGET" \
  --enforce-overall-target="$ENFORCE_OVERALL"

if [ "$CHANGED_ONLY" = "true" ]; then
  set -- "$@" --changed-only
else
  set -- "$@" \
    --overall-profile "$OVERALL_PROFILE" \
    --diff-profile "$OVERALL_PROFILE" \
    --baseline-profile "$BASELINE_PROFILE"
  if [ -n "$ADDITIONAL_DIFF_PROFILE" ]; then
    set -- "$@" --diff-profile "$ADDITIONAL_DIFF_PROFILE"
  fi
fi
if [ "$SCOPE_BUILDABLE" = "true" ]; then
  set -- "$@" --scope-buildable
fi
"$@"
