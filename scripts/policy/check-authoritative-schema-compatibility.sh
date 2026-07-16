#!/bin/sh
set -eu

# Compare the candidate's complete Schema contract with the Git-owned PR
# merge-base (or previous main SHA on push). The candidate branch cannot
# weaken this authority by editing a checked-in fixture.

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
BIN="${DWS_BIN:-$ROOT/dws}"
BASE_REF=""

usage() {
	printf '%s\n' "usage: $0 --base-ref <ref>" >&2
}

while [ "$#" -gt 0 ]; do
	case "$1" in
	--base-ref)
		[ "$#" -ge 2 ] || { usage; exit 2; }
		BASE_REF="$2"
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
policy_prepare_runtime "$ROOT"

git rev-parse --verify --quiet "${BASE_REF}^{commit}" >/dev/null || {
	printf 'error: Schema authority is not available locally: %s\n' "$BASE_REF" >&2
	exit 2
}
if [ ! -x "$BIN" ]; then
	printf 'error: dws binary not found at %s (run make build first)\n' "$BIN" >&2
	exit 2
fi

TMP_ROOT="$(policy_runtime_mktemp_dir dws-schema-authority)"
BASE_WORKTREE="$TMP_ROOT/base-worktree"
BASE_BIN="$TMP_ROOT/base-dws"
BASE_RAW="$TMP_ROOT/base-schema.json"
BASELINE="$TMP_ROOT/base-contract.json"
CANDIDATE_RAW="$TMP_ROOT/candidate-schema.json"
CHECKER="$TMP_ROOT/schema-compat"

cleanup() {
	git -C "$ROOT" worktree remove --force "$BASE_WORKTREE" >/dev/null 2>&1 || true
	rm -rf "$TMP_ROOT"
}
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

git -C "$ROOT" worktree add --detach "$BASE_WORKTREE" "$BASE_REF" >/dev/null
(
	cd "$BASE_WORKTREE"
	go build -o "$BASE_BIN" ./cmd
)
go build -o "$CHECKER" ./scripts/policy/schema-compat

mkdir -p "$TMP_ROOT/base-home" "$TMP_ROOT/candidate-home"
HOME="$TMP_ROOT/base-home" DWS_LANG=zh \
	"$BASE_BIN" schema --all --format json >"$BASE_RAW"
HOME="$TMP_ROOT/candidate-home" DWS_LANG=zh \
	"$BIN" schema --all --format json >"$CANDIDATE_RAW"

"$CHECKER" --normalize "$BASE_RAW" >"$BASELINE"
printf 'checking complete Schema contract against PR merge-base %s\n' "$BASE_REF"
"$CHECKER" --check "$BASELINE" --current "$CANDIDATE_RAW"
