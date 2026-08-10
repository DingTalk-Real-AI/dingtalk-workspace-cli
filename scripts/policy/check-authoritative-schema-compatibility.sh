#!/bin/sh
set -eu

# Compare the committed candidate's complete Schema contract with the Git-owned
# PR merge-base (or previous main SHA on push). The merge-base owns the Schema
# checker and, after bootstrap, the approved flag-migration ledger. Candidate
# code is lifecycle input only and cannot replace either authority.

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
BASE_REF=""
STABLE_REF=""
CANDIDATE_REF="HEAD"
SCHEMA_CHECKER_REL="scripts/policy/schema-compat/main.go"
MIGRATION_MANIFEST_REL="scripts/policy/interface-migrations/approved-flag-migrations-v1.json"
MIGRATIONS_REL="internal/interfacesnapshot/migrations.go"
ALIAS_CONTRACT_REL="internal/corecmd/runtimeannotate/interface_alias.go"

usage() {
	printf '%s\n' "usage: $0 --base-ref <ref> [--stable-ref <ref>] [--candidate-ref <ref>]" >&2
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
		printf 'error: unknown argument: %s\n' "$1" >&2
		usage
		exit 2
		;;
	esac
done

[ -n "$BASE_REF" ] || { usage; exit 2; }

cd "$ROOT"
. "$ROOT/scripts/policy/policy-runtime.sh"
. "$ROOT/scripts/release/release-lib.sh"
policy_prepare_runtime "$ROOT"

BASE_COMMIT="$(git rev-parse --verify "${BASE_REF}^{commit}")" || {
	printf 'error: Schema authority is not available locally: %s\n' "$BASE_REF" >&2
	exit 2
}
CANDIDATE_COMMIT="$(git rev-parse --verify "${CANDIDATE_REF}^{commit}")" || {
	printf 'error: Schema candidate is not available locally: %s\n' "$CANDIDATE_REF" >&2
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
	printf 'error: no stable release tag is reachable from Schema authority %s\n' "$BASE_REF" >&2
	exit 2
fi
if [ -z "$STABLE_REF" ]; then
	STABLE_REF="$EXPECTED_STABLE_REF"
fi
STABLE_COMMIT="$(git rev-parse --verify "${STABLE_REF}^{commit}")" || {
	printf 'error: Schema stable authority is not available locally: %s\n' "$STABLE_REF" >&2
	exit 2
}
if [ "$(git rev-parse "${STABLE_REF}^{commit}")" != "$(git rev-parse "${EXPECTED_STABLE_REF}^{commit}")" ]; then
	printf 'error: stable ref %s is not the expected highest GA tag %s reachable from %s\n' \
		"$STABLE_REF" "$EXPECTED_STABLE_REF" "$BASE_REF" >&2
	exit 2
fi

TMP_ROOT="$(policy_runtime_mktemp_dir dws-schema-authority)"
BASE_WORKTREE="$TMP_ROOT/base-worktree"
STABLE_WORKTREE="$TMP_ROOT/stable-worktree"
CANDIDATE_WORKTREE="$TMP_ROOT/candidate-worktree"
BASE_WORKTREE_ADDED=false
STABLE_WORKTREE_ADDED=false
CANDIDATE_WORKTREE_ADDED=false

cleanup() {
	if [ "$CANDIDATE_WORKTREE_ADDED" = true ]; then
		git -C "$ROOT" worktree remove --force "$CANDIDATE_WORKTREE" >/dev/null 2>&1 || true
	fi
	if [ "$STABLE_WORKTREE_ADDED" = true ]; then
		git -C "$ROOT" worktree remove --force "$STABLE_WORKTREE" >/dev/null 2>&1 || true
	fi
	if [ "$BASE_WORKTREE_ADDED" = true ]; then
		git -C "$ROOT" worktree remove --force "$BASE_WORKTREE" >/dev/null 2>&1 || true
	fi
	rm -rf "$TMP_ROOT"
}
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

git -C "$ROOT" worktree add --detach "$BASE_WORKTREE" "$BASE_COMMIT" >/dev/null
BASE_WORKTREE_ADDED=true
git -C "$ROOT" worktree add --detach "$STABLE_WORKTREE" "$STABLE_COMMIT" >/dev/null
STABLE_WORKTREE_ADDED=true
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

has_complete_schema_migration_governance() {
	governance_commit="$1"
	commit_path_is_regular_file "$governance_commit" "$SCHEMA_CHECKER_REL" &&
		has_modern_interface_helper "$governance_commit" &&
		commit_path_is_regular_file "$governance_commit" "$MIGRATIONS_REL" &&
		commit_path_is_regular_file "$governance_commit" "$ALIAS_CONTRACT_REL" &&
		commit_path_is_regular_file "$governance_commit" "$MIGRATION_MANIFEST_REL"
}

has_any_schema_migration_governance_artifact() {
	governance_commit="$1"
	for relative_path in \
		"$MIGRATIONS_REL" \
		"$ALIAS_CONTRACT_REL" \
		"$MIGRATION_MANIFEST_REL"; do
		if commit_path_exists "$governance_commit" "$relative_path"; then
			return 0
		fi
	done
	return 1
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
					printf 'error: candidate production source may not forge framework-owned flag alias evidence:\n' >&2
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

if ! commit_path_is_regular_file "$BASE_COMMIT" "$SCHEMA_CHECKER_REL"; then
	printf 'error: merge-base lacks the base-owned Schema checker: %s\n' "$BASE_REF" >&2
	exit 2
fi
if ! has_modern_interface_helper "$BASE_COMMIT"; then
	printf 'error: merge-base lacks the modern interface snapshot helper required for Schema governance: %s\n' "$BASE_REF" >&2
	exit 2
fi
if ! has_complete_schema_migration_governance "$CANDIDATE_COMMIT"; then
	printf 'error: candidate must preserve the complete Schema flag migration governance artifact set at %s\n' \
		"$CANDIDATE_COMMIT" >&2
	exit 2
fi
check_candidate_alias_source_policy

EMPTY_MANIFEST="$TMP_ROOT/empty-migrations.json"
printf '%s\n' '{"version":1,"migrations":[]}' >"$EMPTY_MANIFEST"
USE_MIGRATION_GOVERNANCE=false

if has_complete_schema_migration_governance "$BASE_COMMIT"; then
	USE_MIGRATION_GOVERNANCE=true
	APPROVED_MANIFEST="$BASE_WORKTREE/$MIGRATION_MANIFEST_REL"
	CANDIDATE_MANIFEST="$CANDIDATE_WORKTREE/$MIGRATION_MANIFEST_REL"
else
	# One-time bootstrap for the governance PR itself. The old base checker owns
	# the decision and receives no flags it does not understand. With no base
	# approval authority, only the exact canonical empty candidate ledger is
	# admissible.
	if has_any_schema_migration_governance_artifact "$BASE_COMMIT"; then
		printf 'error: merge-base contains an incomplete Schema flag migration governance artifact set: %s\n' \
			"$BASE_REF" >&2
		exit 2
	fi
	BOOTSTRAP_MANIFEST="$CANDIDATE_WORKTREE/$MIGRATION_MANIFEST_REL"
	if ! cmp -s "$BOOTSTRAP_MANIFEST" "$EMPTY_MANIFEST"; then
		printf 'error: initial Schema flag migration governance requires the canonical empty manifest: %s\n' \
			"$BOOTSTRAP_MANIFEST" >&2
		exit 2
	fi
fi

BASE_BIN="$TMP_ROOT/base-dws"
CANDIDATE_BIN="$TMP_ROOT/candidate-dws"
CHECKER="$TMP_ROOT/schema-compat"
BASE_RAW="$TMP_ROOT/base-schema.json"
BASELINE="$TMP_ROOT/base-contract.json"
CANDIDATE_RAW="$TMP_ROOT/candidate-schema.json"

(
	cd "$BASE_WORKTREE"
	go build -o "$BASE_BIN" ./cmd
	go build -o "$CHECKER" ./scripts/policy/schema-compat
)
(
	cd "$CANDIDATE_WORKTREE"
	go build -o "$CANDIDATE_BIN" ./cmd
)

mkdir -p "$TMP_ROOT/base-home" "$TMP_ROOT/candidate-home"
HOME="$TMP_ROOT/base-home" DWS_LANG=zh \
	"$BASE_BIN" schema --all --format json >"$BASE_RAW"
HOME="$TMP_ROOT/candidate-home" DWS_LANG=zh \
	"$CANDIDATE_BIN" schema --all --format json >"$CANDIDATE_RAW"

"$CHECKER" --normalize "$BASE_RAW" >"$BASELINE"
printf 'checking complete Schema contract against PR merge-base %s and stable %s\n' \
	"$BASE_REF" "$STABLE_REF"

if [ "$USE_MIGRATION_GOVERNANCE" = false ]; then
	"$CHECKER" --check "$BASELINE" --current "$CANDIDATE_RAW"
	exit $?
fi

install_authority_interface_helper() {
	worktree="$1"
	for helper_path in cmd/interface-snapshot internal/interfacesnapshot; do
		rm -rf "$worktree/$helper_path"
		mkdir -p "$worktree/$helper_path"
		cp -R "$BASE_WORKTREE/$helper_path/." "$worktree/$helper_path/"
	done
	mkdir -p "$worktree/$(dirname "$ALIAS_CONTRACT_REL")"
	rm -f "$worktree/$ALIAS_CONTRACT_REL"
	cp "$BASE_WORKTREE/$ALIAS_CONTRACT_REL" "$worktree/$ALIAS_CONTRACT_REL"
}

# The candidate and historical stable trees provide only the command surface.
# The merge-base owns the snapshot generator, comparator, and alias contract.
install_authority_interface_helper "$CANDIDATE_WORKTREE"
install_authority_interface_helper "$STABLE_WORKTREE"

generate_interface_snapshot() {
	worktree="$1"
	output="$2"
	(
		cd "$worktree"
		go run ./cmd/interface-snapshot generate --output "$output"
	)
}

CURRENT_INTERFACE_SNAPSHOT="$TMP_ROOT/candidate-interface-snapshot.json"
BASE_INTERFACE_SNAPSHOT="$TMP_ROOT/base-interface-snapshot.json"
STABLE_INTERFACE_SNAPSHOT="$TMP_ROOT/stable-interface-snapshot.json"

generate_interface_snapshot "$CANDIDATE_WORKTREE" "$CURRENT_INTERFACE_SNAPSHOT"
generate_interface_snapshot "$BASE_WORKTREE" "$BASE_INTERFACE_SNAPSHOT"
generate_interface_snapshot "$STABLE_WORKTREE" "$STABLE_INTERFACE_SNAPSHOT"

"$CHECKER" \
	--check "$BASELINE" \
	--current "$CANDIDATE_RAW" \
	--approved-flag-migrations "$APPROVED_MANIFEST" \
	--candidate-flag-migrations "$CANDIDATE_MANIFEST" \
	--migration-current-snapshot "$CURRENT_INTERFACE_SNAPSHOT" \
	--migration-base-snapshot "$BASE_INTERFACE_SNAPSHOT" \
	--migration-stable-snapshot "$STABLE_INTERFACE_SNAPSHOT"
