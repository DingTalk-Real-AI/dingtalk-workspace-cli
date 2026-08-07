#!/bin/sh
set -eu

# B162 Phase I prototype: string-boolean scan (AC-02).
#
# Contract anchor AC-02: booleans carried by the unified output contract are
# always JSON booleans. This scan flags string-encoded booleans on
# boolean-contract keys:
#   top-level envelope keys   ok / success / dry_run / retryable / timed_out
#   meta subtree keys         supervised / endpoint_exhausted / timed_out
#   error subtree keys        retryable
# Matches inside data payloads are NOT flagged: data is business payload and
# a business field named "success" with string content is not an envelope
# violation (prototype stance, see README.md).
#
# This is a PROTOTYPE scan, not a wired policy gate. Positioning, sample
# selection, the B164 false-positive verification record, --scope semantics
# (B166) and the `make policy` hook design draft (B167) live in
# scripts/policy/README.md. Samples run under an isolated fresh HOME
# (DWS_SCAN_HOME overrides), so the default dev scope needs no login.
#
# Usage:
#   check-string-bool.sh [--scope dev|all]   (default: dev)
#   check-string-bool.sh --self-test         scan scripts/policy/testdata fixtures

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
. "$ROOT/scripts/policy/output-contract-lib.sh"

OC_SCRIPT_NAME="string-bool"

output_contract_usage() {
	printf 'usage: %s [--scope dev|all] | --self-test | --help\n' "$0"
}

string_bool_scan() {
	string_bool_class="$1"
	string_bool_label="$2"
	string_bool_out="$3"
	string_bool_violations=""

	if [ ! -s "$string_bool_out" ]; then
		string_bool_violations="stdout is empty; nothing to scan"
		output_contract_report_violations "$string_bool_label" "$string_bool_violations"
		return 0
	fi

	if ! jq empty <"$string_bool_out" >/dev/null 2>&1; then
		string_bool_violations="stdout is not parseable JSON; string-bool scan fails closed (parseability gate: check-stdout-json.sh)"
		output_contract_report_violations "$string_bool_label" "$string_bool_violations"
		return 0
	fi

	# Top-level boolean-contract keys must be JSON booleans, not "true"/"false".
	string_bool_top="$(jq -r '
		["ok", "success", "dry_run", "retryable", "timed_out", "supervised",
		 "endpoint_exhausted"] as $bool_keys |
		if type == "object" then
			to_entries[]
			| select(.key as $k | $bool_keys | index($k))
			| select((.value | type) == "string" and (.value == "true" or .value == "false"))
			| "top-level \"" + .key + "\" is string boolean \"" + .value + "\" (must be JSON boolean)"
		else empty end' <"$string_bool_out")" || string_bool_top=""
	if [ -n "$string_bool_top" ]; then
		string_bool_violations="$string_bool_top"
	fi

	# meta / error subtrees are envelope structure: boolean-contract keys there
	# must also be JSON booleans. They are shallow, so a text scan of the
	# re-serialized subtrees is exact enough.
	string_bool_subtrees="$(jq -c '[.meta // {}, .error // {}]' <"$string_bool_out" 2>/dev/null)" || string_bool_subtrees="[]"
	string_bool_nested="$(printf '%s\n' "$string_bool_subtrees" |
		grep -noE '"(ok|success|dry_run|retryable|timed_out|supervised|endpoint_exhausted)"[[:space:]]*:[[:space:]]*"(true|false)"' || true)"
	if [ -n "$string_bool_nested" ]; then
		string_bool_nested_msg="string boolean in meta/error subtree: $(printf '%s' "$string_bool_nested" | head -3 | tr '\n' '|')"
		string_bool_violations="${string_bool_violations:+$string_bool_violations; }$string_bool_nested_msg"
	fi

	output_contract_report_violations "$string_bool_label" "$string_bool_violations"
}

output_contract_init "$ROOT"
output_contract_parse_args "$@"

self_test_cases() {
	printf 'envelope_legal_success.json|envelope|pass\n'
	printf 'envelope_legal_pending_dry_run.json|envelope|pass\n'
	printf 'envelope_legal_partial_failure.json|envelope|pass\n'
	printf 'envelope_legal_failure.json|envelope|pass\n'
	printf 'envelope_ok_full_meta.json|envelope|pass\n'
	printf 'envelope_ok_with_legacy_payload.json|envelope|pass\n'
	printf 'string_bool_ok.json|envelope|pass\n'
	printf 'legacy_ok.json|legacy|pass\n'
	printf 'legacy_envelope_keys.json|envelope|pass\n'
	printf 'string_bool_violation.json|envelope|fail\n'
	printf 'stdout_log_polluted.json|envelope|fail\n'
}

if [ "$SELF_TEST" -eq 1 ]; then
	output_contract_run_self_test string_bool_scan
fi

output_contract_scan_samples string_bool_scan
