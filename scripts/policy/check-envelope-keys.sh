#!/bin/sh
set -eu

# B163 Phase I prototype: non-standard envelope key scan (G1).
#
# Contract anchor: the envelope top-level key set is fixed —
# contract_version / ok / outcome / identity / dry_run / data / meta / error / _notice
# (snake_case). This scan flags historical variants on envelope-class output:
#   - legacy status keys at the top level: success / errcode / error_code /
#     errorCode / err_code / isSuccess / is_success
#   - retired camelCase wire keys anywhere in the document: timedOut /
#     nextCommand / endpointExhausted / nextToken (contract bans camelCase
#     wire forms; 轮4裁决④/⑤ in the shared execution log)
#
# legacy-class samples (pre-migration non-envelope json, e.g. schema list /
# auth status) are exempt from the envelope-shape scan — that is their known
# pre-migration shape, not a regression. See README.md.
#
# This is a PROTOTYPE scan, not a wired policy gate. Positioning, sample
# selection, the B164 false-positive verification record, --scope semantics
# (B166) and the `make policy` hook design draft (B167) live in
# scripts/policy/README.md. Samples run under an isolated fresh HOME
# (DWS_SCAN_HOME overrides), so the default dev scope needs no login.
#
# Usage:
#   check-envelope-keys.sh [--scope dev|all]   (default: dev)
#   check-envelope-keys.sh --self-test         scan scripts/policy/testdata fixtures

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
. "$ROOT/scripts/policy/output-contract-lib.sh"

OC_SCRIPT_NAME="envelope-keys"

output_contract_usage() {
	printf 'usage: %s [--scope dev|all] | --self-test | --help\n' "$0"
}

envelope_keys_scan() {
	envelope_keys_class="$1"
	envelope_keys_label="$2"
	envelope_keys_out="$3"
	envelope_keys_violations=""

	if [ ! -s "$envelope_keys_out" ]; then
		envelope_keys_violations="stdout is empty; nothing to scan"
		output_contract_report_violations "$envelope_keys_label" "$envelope_keys_violations"
		return 0
	fi

	if [ "$envelope_keys_class" = "legacy" ]; then
		# Known pre-migration non-envelope shape; envelope-key scan not applicable.
		return 0
	fi

	if ! jq empty <"$envelope_keys_out" >/dev/null 2>&1; then
		envelope_keys_violations="stdout is not parseable JSON; envelope-key scan fails closed (parseability gate: check-stdout-json.sh)"
		output_contract_report_violations "$envelope_keys_label" "$envelope_keys_violations"
		return 0
	fi

	# Top-level key set: only the fixed envelope keys are allowed.
	envelope_keys_extra="$(jq -r '
		["contract_version", "ok", "outcome", "identity", "dry_run", "data", "meta", "error", "_notice"] as $allowed |
		if type == "object" then
			keys_unsorted[] | select(. as $k | $allowed | index($k) | not)
		else empty end' <"$envelope_keys_out")" || envelope_keys_extra=""
	if [ -n "$envelope_keys_extra" ]; then
		envelope_keys_violations="non-envelope top-level key(s): $(printf '%s' "$envelope_keys_extra" | sort -u | tr '\n' ' ')"
	fi

	# Historical status/error key forms and retired camelCase wire keys are
	# scanned only on envelope structure (top level via the allowed-set check
	# above, plus meta/error subtrees here). data is business payload and may
	# legitimately carry camelCase or legacy-named business fields.
	envelope_keys_struct="$(jq -r '
		["success", "errcode", "err_code", "errCode", "error_code", "errorCode",
		 "isSuccess", "is_success", "timedOut", "nextCommand",
		 "endpointExhausted", "nextToken"] as $banned |
		[
			(if (.meta? | type) == "object" then
				(.meta | keys_unsorted[] | select(. as $k | $banned | index($k)) | "meta." + .)
			 else empty end),
			(if (.error? | type) == "object" then
				(.error | keys_unsorted[] | select(. as $k | $banned | index($k)) | "error." + .)
			 else empty end)
		] | .[]' <"$envelope_keys_out")" || envelope_keys_struct=""
	if [ -n "$envelope_keys_struct" ]; then
		envelope_keys_struct_msg="historical/retired key form(s) in envelope structure: $(printf '%s' "$envelope_keys_struct" | sort -u | tr '\n' ' ')"
		envelope_keys_violations="${envelope_keys_violations:+$envelope_keys_violations; }$envelope_keys_struct_msg"
	fi

	output_contract_report_violations "$envelope_keys_label" "$envelope_keys_violations"
}

output_contract_init "$ROOT"
output_contract_parse_args "$@"

self_test_cases() {
	printf 'envelope_v2_legal_success.json|envelope|pass\n'
	printf 'envelope_legal_success.json|envelope|pass\n'
	printf 'envelope_legal_pending_dry_run.json|envelope|pass\n'
	printf 'envelope_ok_full_meta.json|envelope|pass\n'
	printf 'envelope_ok_with_legacy_payload.json|envelope|pass\n'
	printf 'legacy_ok.json|legacy|pass\n'
	printf 'string_bool_ok.json|envelope|pass\n'
	printf 'envelope_legacy_keys.json|envelope|fail\n'
	printf 'envelope_camel_keys.json|envelope|fail\n'
	printf 'legacy_envelope_keys.json|envelope|fail\n'
	printf 'legacy_ok.json|envelope|fail\n'
	printf 'stdout_log_polluted.json|envelope|fail\n'
}

if [ "$SELF_TEST" -eq 1 ]; then
	output_contract_run_self_test envelope_keys_scan
fi

output_contract_scan_samples envelope_keys_scan
