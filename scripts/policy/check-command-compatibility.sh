#!/bin/sh
set -eu

# Compare the committed candidate Cobra surface with the PR merge-base and the
# latest stable GA tag. Once the merge-base contains the migration governance
# helper, that revision owns the generator, comparator, and approved manifest.
# The candidate manifest is lifecycle input only and cannot authorize a change
# introduced by the same PR.

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
BASE_REF=""
STABLE_REF=""
CANDIDATE_REF="HEAD"
MIGRATION_MANIFEST_REL="scripts/policy/interface-migrations/approved-flag-migrations-v1.json"
ALIAS_CONTRACT_REL="internal/corecmd/runtimeannotate/interface_alias.go"

usage() {
  printf '%s\n' "usage: $0 --base-ref <ref> --stable-ref <ref> [--candidate-ref <ref>]" >&2
}

while [ "$#" -gt 0 ]; do
  case "$1" in
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
      printf 'unknown argument: %s\n' "$1" >&2
      usage
      exit 2
      ;;
  esac
done

[ -n "$BASE_REF" ] && [ -n "$STABLE_REF" ] || { usage; exit 2; }

cd "$ROOT"
. "$ROOT/scripts/policy/policy-runtime.sh"
. "$ROOT/scripts/release/release-lib.sh"
policy_prepare_runtime "$ROOT"

BASE_COMMIT="$(git rev-parse --verify "${BASE_REF}^{commit}")" || {
  printf 'base ref is not available locally: %s\n' "$BASE_REF" >&2
  exit 2
}
STABLE_COMMIT="$(git rev-parse --verify "${STABLE_REF}^{commit}")" || {
  printf 'stable ref is not available locally: %s\n' "$STABLE_REF" >&2
  exit 2
}
CANDIDATE_COMMIT="$(git rev-parse --verify "${CANDIDATE_REF}^{commit}")" || {
  printf 'candidate ref is not available locally: %s\n' "$CANDIDATE_REF" >&2
  exit 2
}
EXPECTED_STABLE_REF=""
for tag in $(git tag --merged "$BASE_REF" --list 'v*' --sort=-version:refname); do
  release_is_stable_version "$tag" || continue
  if git rev-parse --verify --quiet "refs/tags/withdrawn/$tag" >/dev/null; then
    continue
  fi
  EXPECTED_STABLE_REF="$tag"
  break
done
if [ -z "$EXPECTED_STABLE_REF" ]; then
  printf 'no stable GA tag is reachable from compatibility base %s\n' "$BASE_REF" >&2
  exit 2
fi
if [ "$(git rev-parse "${STABLE_REF}^{commit}")" != "$(git rev-parse "${EXPECTED_STABLE_REF}^{commit}")" ]; then
  printf 'stable ref %s is not the expected highest GA tag %s reachable from %s\n' \
    "$STABLE_REF" "$EXPECTED_STABLE_REF" "$BASE_REF" >&2
  exit 2
fi

TMP_ROOT="$(policy_runtime_mktemp_dir dws-command-compat)"
BASE_WORKTREE="$TMP_ROOT/base-worktree"
STABLE_WORKTREE="$TMP_ROOT/stable-worktree"
CANDIDATE_WORKTREE="$TMP_ROOT/candidate-worktree"
CANDIDATE_WORKTREE_ADDED=false

cleanup() {
  if [ "$CANDIDATE_WORKTREE_ADDED" = true ]; then
    git -C "$ROOT" worktree remove --force "$CANDIDATE_WORKTREE" >/dev/null 2>&1 || true
  fi
  git -C "$ROOT" worktree remove --force "$BASE_WORKTREE" >/dev/null 2>&1 || true
  git -C "$ROOT" worktree remove --force "$STABLE_WORKTREE" >/dev/null 2>&1 || true
  rm -rf "$TMP_ROOT"
}
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

git -C "$ROOT" worktree add --detach "$BASE_WORKTREE" "$BASE_COMMIT" >/dev/null
git -C "$ROOT" worktree add --detach "$STABLE_WORKTREE" "$STABLE_COMMIT" >/dev/null
git -C "$ROOT" worktree add --detach "$CANDIDATE_WORKTREE" "$CANDIDATE_COMMIT" >/dev/null
CANDIDATE_WORKTREE_ADDED=true

commit_path_exists() {
  tree_commit="$1"
  tree_path="$2"
  tree_entry="$(git -C "$ROOT" ls-tree "$tree_commit" -- "$tree_path")"
  [ -n "$tree_entry" ]
}

commit_path_is_regular_file() {
  tree_commit="$1"
  tree_path="$2"
  tree_entry="$(git -C "$ROOT" ls-tree "$tree_commit" -- "$tree_path")"
  [ -n "$tree_entry" ] || return 1
  set -- $tree_entry
  [ "$#" -eq 4 ] || return 1
  case "$1" in
    100644|100755) ;;
    *) return 1 ;;
  esac
  [ "$2" = blob ] && [ "$4" = "$tree_path" ]
}

has_modern_interface_helper() {
  governance_commit="$1"
  commit_path_is_regular_file "$governance_commit" cmd/interface-snapshot/main.go &&
    commit_path_is_regular_file "$governance_commit" internal/interfacesnapshot/snapshot.go &&
    commit_path_is_regular_file "$governance_commit" internal/interfacesnapshot/compare.go
}

has_complete_migration_governance() {
  governance_commit="$1"
  has_modern_interface_helper "$governance_commit" &&
    commit_path_is_regular_file "$governance_commit" internal/interfacesnapshot/migrations.go &&
    commit_path_is_regular_file "$governance_commit" "$ALIAS_CONTRACT_REL" &&
    commit_path_is_regular_file "$governance_commit" "$MIGRATION_MANIFEST_REL"
}

has_any_migration_governance_artifact() {
  governance_commit="$1"
  for relative_path in \
    internal/interfacesnapshot/migrations.go \
    "$ALIAS_CONTRACT_REL" \
    "$MIGRATION_MANIFEST_REL"; do
    if commit_path_exists "$governance_commit" "$relative_path"; then
      return 0
    fi
  done
  return 1
}

require_complete_candidate_governance() {
  if ! has_complete_migration_governance "$CANDIDATE_COMMIT"; then
    printf 'candidate must preserve the complete flag migration governance artifact set at %s\n' \
      "$CANDIDATE_COMMIT" >&2
    exit 2
  fi
}

check_candidate_alias_source_policy() {
  source_policy_failed=false
  for token in \
    dws.compat.alias_of \
    dws.compat.alias_origin \
    corecmd.flag_spec_aliases.v1 \
    AnnotationFlagAliasOf \
    AnnotationFlagAliasOrigin \
    FlagAliasOriginCorecmdV1; do
    matches="$(git -C "$CANDIDATE_WORKTREE" grep -l -F -e "$token" -- '*.go' || true)"
    [ -z "$matches" ] && continue
    while IFS= read -r relative_path; do
      [ -n "$relative_path" ] || continue
      case "$relative_path" in
        *_test.go)
          continue
          ;;
        internal/corecmd/corecmd.go|\
        internal/corecmd/runtimeannotate/interface_alias.go|\
        internal/interfacesnapshot/snapshot.go)
          ;;
        *)
          if [ "$source_policy_failed" = false ]; then
            printf 'candidate production source may not forge framework-owned flag alias evidence:\n' >&2
          fi
          printf '  %s: protected alias evidence token %s\n' "$relative_path" "$token" >&2
          source_policy_failed=true
          ;;
      esac
    done <<EOF
$matches
EOF
  done
  if [ "$source_policy_failed" = true ]; then
    exit 2
  fi
}

install_authority_helper() {
  source_root="$1"
  worktree="$2"
  include_alias_contract="$3"

  [ "$source_root" = "$worktree" ] && return
  for helper_path in cmd/interface-snapshot internal/interfacesnapshot; do
    rm -rf "$worktree/$helper_path"
    mkdir -p "$worktree/$helper_path"
    cp -R "$source_root/$helper_path/." "$worktree/$helper_path/"
  done
  if [ "$include_alias_contract" = true ]; then
    mkdir -p "$worktree/$(dirname "$ALIAS_CONTRACT_REL")"
    rm -f "$worktree/$ALIAS_CONTRACT_REL"
    cp "$source_root/$ALIAS_CONTRACT_REL" "$worktree/$ALIAS_CONTRACT_REL"
  fi
}

EMPTY_MANIFEST="$TMP_ROOT/empty-migrations.json"
printf '%s\n' '{"version":1,"migrations":[]}' >"$EMPTY_MANIFEST"
USE_MIGRATION_GOVERNANCE=false

if has_complete_migration_governance "$BASE_COMMIT"; then
  USE_MIGRATION_GOVERNANCE=true
  AUTHORITY_ROOT="$BASE_WORKTREE"
  APPROVED_MANIFEST="$BASE_WORKTREE/$MIGRATION_MANIFEST_REL"
  CANDIDATE_MANIFEST="$CANDIDATE_WORKTREE/$MIGRATION_MANIFEST_REL"
  require_complete_candidate_governance
  check_candidate_alias_source_policy

  install_authority_helper "$AUTHORITY_ROOT" "$CANDIDATE_WORKTREE" true
else
  # One-time bootstrap for the governance PR itself. No base approval exists,
  # so the existing base-owned modern helper performs an ordinary comparison
  # and this path admits only the canonical empty ledger. The candidate's new
  # comparator never participates in the bootstrap decision.
  if has_any_migration_governance_artifact "$BASE_COMMIT"; then
    printf 'merge-base contains an incomplete flag migration governance artifact set: %s\n' \
      "$BASE_REF" >&2
    exit 2
  fi
  if ! has_modern_interface_helper "$BASE_COMMIT"; then
    printf 'merge-base lacks the modern interface snapshot helper required for governance bootstrap: %s\n' "$BASE_REF" >&2
    exit 2
  fi
  AUTHORITY_ROOT="$BASE_WORKTREE"
  require_complete_candidate_governance
  check_candidate_alias_source_policy
  BOOTSTRAP_MANIFEST="$CANDIDATE_WORKTREE/$MIGRATION_MANIFEST_REL"
  if ! cmp -s "$BOOTSTRAP_MANIFEST" "$EMPTY_MANIFEST"; then
    printf 'initial flag migration governance requires the canonical empty manifest: %s\n' "$BOOTSTRAP_MANIFEST" >&2
    exit 2
  fi
  install_authority_helper "$AUTHORITY_ROOT" "$CANDIDATE_WORKTREE" false
fi

install_authority_helper "$AUTHORITY_ROOT" "$BASE_WORKTREE" "$USE_MIGRATION_GOVERNANCE"
install_authority_helper "$AUTHORITY_ROOT" "$STABLE_WORKTREE" "$USE_MIGRATION_GOVERNANCE"

generate_snapshot() {
  worktree="$1"
  output="$2"
  (
    cd "$worktree"
    go run ./cmd/interface-snapshot generate --output "$output"
  )
}

CANDIDATE="$TMP_ROOT/candidate.json"
BASELINE="$TMP_ROOT/merge-base.json"
STABLE="$TMP_ROOT/stable.json"

generate_snapshot "$CANDIDATE_WORKTREE" "$CANDIDATE"
generate_snapshot "$BASE_WORKTREE" "$BASELINE"
generate_snapshot "$STABLE_WORKTREE" "$STABLE"

if [ "$USE_MIGRATION_GOVERNANCE" = true ]; then
  (
    cd "$AUTHORITY_ROOT"
    go run ./cmd/interface-snapshot compare \
      --current "$CANDIDATE" \
      --base "$BASELINE" \
      --stable "$STABLE" \
      --approved-flag-migrations "$APPROVED_MANIFEST" \
      --candidate-flag-migrations "$CANDIDATE_MANIFEST"
  )
else
  (
    cd "$AUTHORITY_ROOT"
    go run ./cmd/interface-snapshot compare \
      --current "$CANDIDATE" \
      --base "$BASELINE" \
      --stable "$STABLE"
  )
fi
