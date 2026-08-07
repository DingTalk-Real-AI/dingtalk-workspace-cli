#!/bin/sh

# Shared library for the Phase I unified-output-contract CI scan prototypes
# (B161~B167): check-stdout-json.sh / check-string-bool.sh /
# check-envelope-keys.sh. Sourced by the three check scripts; not executable
# and not a policy gate by itself.
#
# Positioning: Phase I prototypes. They are NOT wired into `make policy`; the
# wiring design draft lives in scripts/policy/README.md (B167).
#
# Contract anchors:
#   - AC-02  ok/success-style booleans are always JSON booleans; string
#            booleans ("true"/"false") are violations.
#   - AC-11  json-mode stdout carries zero log bytes and exactly one primary
#            result document for success, pending, partial, or failure.
#   - Envelope top-level key set is fixed: ok/outcome/identity/dry_run/data/
#            meta/error/_notice (snake_case); historical variants such as
#            errcode/error_code/errorCode/success are violations.
#
# Scan scope (--scope, B166): dev (default) covers only dev-domain commands
# that are deterministic offline (isolated fresh HOME, no login, no network,
# no side effects). all additionally covers auth-dependent dev-domain reads
# and legacy non-envelope json commands (legacy class is exempt from the
# envelope-shape checks; see README.md).

output_contract_init() {
	OC_ROOT="$1"
	OC_BIN="${DWS_BIN:-$OC_ROOT/dws}"
	OC_TESTDATA="$OC_ROOT/scripts/policy/testdata"
	SCOPE="dev"
	SELF_TEST=0
	OC_FAILURES=0
}

output_contract_parse_args() {
	while [ $# -gt 0 ]; do
		case "$1" in
		--scope)
			if [ $# -lt 2 ]; then
				output_contract_die_usage "--scope requires a value (dev|all)"
			fi
			shift
			SCOPE="$1"
			;;
		--scope=*)
			SCOPE="${1#--scope=}"
			;;
		--self-test)
			SELF_TEST=1
			;;
		-h | --help)
			output_contract_usage
			exit 0
			;;
		*)
			output_contract_die_usage "unknown argument: $1"
			;;
		esac
		shift
	done
	case "$SCOPE" in
	dev | all) ;;
	*)
		output_contract_die_usage "invalid --scope: $SCOPE (expected dev|all)"
		;;
	esac
}

output_contract_die_usage() {
	printf 'error: %s\n' "$1" >&2
	output_contract_usage >&2
	exit 2
}

# Samples: <class>\t<label>\t<argv>
# class=envelope: output must be a contract envelope (all checks apply).
# class=legacy:   pre-migration non-envelope json output; parseability and
#                 string-bool checks apply, envelope-shape checks do not.
# The dev-domain probe id is a throwaway identifier: no real connector/daemon
# can match it, so status/stop are side-effect-free no-ops.
output_contract_samples() {
	printf 'envelope\tdev-connect-list\tdev connect list --format json\n'
	printf 'envelope\tdev-connect-status\tdev connect status --unified-app-id dws-policy-scan-probe --format json\n'
	printf 'envelope\tdev-connect-stop-preview\tdev connect stop --unified-app-id dws-policy-scan-probe --dry-run --format json\n'
	if [ "$SCOPE" = "all" ]; then
		printf 'envelope\tdev-app-list\tdev app list --format json\n'
		printf 'legacy\tschema-list\tschema list -f json\n'
		printf 'legacy\tauth-status\tauth status --format json\n'
		printf 'legacy\tversion\tversion --format json\n'
	fi
}

output_contract_require_jq() {
	if ! command -v jq >/dev/null 2>&1; then
		printf 'error: jq is required by the output-contract scan prototypes\n' >&2
		exit 2
	fi
}

# output_contract_run_self_test <scan_fn>
# <scan_fn> <class> <label> <file> prints violation lines (empty output means
# pass). Fixtures and expectations come from the caller-defined
# self_test_cases() emitting "<fixture>|<class>|pass" / "<fixture>|<class>|fail"
# lines (class is passed through so legacy-exemption rules are testable).
output_contract_run_self_test() {
	self_test_scan_fn="$1"
	output_contract_require_jq
	if [ ! -d "$OC_TESTDATA" ]; then
		printf 'error: testdata directory missing at %s\n' "$OC_TESTDATA" >&2
		exit 2
	fi
	self_test_tmp="$(mktemp -d)"
	trap 'rm -rf "$self_test_tmp"' EXIT HUP INT TERM
	self_test_cases >"$self_test_tmp/cases"
	self_test_fail=0
	while IFS='|' read -r self_test_fixture self_test_class self_test_expect; do
		if [ -z "$self_test_fixture" ]; then
			continue
		fi
		self_test_path="$OC_TESTDATA/$self_test_fixture"
		if [ ! -e "$self_test_path" ]; then
			printf 'self-test: missing fixture %s\n' "$self_test_path" >&2
			self_test_fail=1
			continue
		fi
		self_test_violations="$("$self_test_scan_fn" "$self_test_class" "$self_test_fixture" "$self_test_path")" || true
		if [ -n "$self_test_violations" ]; then
			self_test_got="fail"
		else
			self_test_got="pass"
		fi
		if [ "$self_test_got" != "$self_test_expect" ]; then
			self_test_fail=1
			printf 'self-test MISMATCH: %s expected=%s got=%s\n' \
				"$self_test_fixture" "$self_test_expect" "$self_test_got" >&2
			if [ -n "$self_test_violations" ]; then
				printf '%s\n' "$self_test_violations" >&2
			fi
		else
			printf 'self-test ok: %s (%s)\n' "$self_test_fixture" "$self_test_expect"
		fi
	done <"$self_test_tmp/cases"
	if [ "$self_test_fail" -ne 0 ]; then
		printf '%s self-test: FAILED\n' "$OC_SCRIPT_NAME" >&2
		exit 1
	fi
	printf '%s self-test: ok\n' "$OC_SCRIPT_NAME"
	exit 0
}

# output_contract_scan_samples <process_fn>
# <process_fn> <class> <label> <stdout_file> <stderr_file> inspects one
# captured sample and increments OC_FAILURES on violations.
output_contract_scan_samples() {
	scan_process_fn="$1"
	output_contract_require_jq
	if [ ! -x "$OC_BIN" ]; then
		printf 'error: dws binary not found at %s (run make build first)\n' "$OC_BIN" >&2
		exit 2
	fi
	scan_tmp="$(mktemp -d)"
	trap 'rm -rf "$scan_tmp"' EXIT HUP INT TERM
	# HOME selection:
	#   - DWS_SCAN_HOME override wins (operator-controlled environment).
	#   - scope=dev (default): isolated fresh HOME + DWS_DISABLE_KEYCHAIN=1 so
	#     the scan is deterministic, login-free, and side-effect-free.
	#   - scope=all: inherits the real HOME because auth-dependent samples
	#     (dev app list) need a logged-in session.
	scan_disable_keychain=0
	if [ -n "${DWS_SCAN_HOME:-}" ]; then
		scan_home="$DWS_SCAN_HOME"
	elif [ "$SCOPE" = "dev" ]; then
		scan_home="$(mktemp -d "$scan_tmp/scan-home.XXXXXX")"
		scan_disable_keychain=1
	else
		scan_home="${HOME:-$scan_tmp/home}"
	fi
	scan_samples="$scan_tmp/samples"
	output_contract_samples >"$scan_samples"
	scan_tab="$(printf '\t')"
	scan_total=0
	scan_verified=0
	scan_skipped=0
	while IFS="$scan_tab" read -r scan_class scan_label scan_argv; do
		if [ -z "$scan_label" ]; then
			continue
		fi
		scan_total=$((scan_total + 1))
		scan_out="$scan_tmp/$scan_label.stdout"
		scan_err="$scan_tmp/$scan_label.stderr"
		scan_rc=0
		scan_attempt=1
		# Hard discipline: transient in-flight edits of the shared binary can
		# cause sporadic failures; retry once before drawing conclusions.
		while [ "$scan_attempt" -le 2 ]; do
			scan_rc=0
			if [ "$scan_disable_keychain" -eq 1 ]; then
				HOME="$scan_home" DWS_DISABLE_KEYCHAIN=1 "$OC_BIN" $scan_argv \
					>"$scan_out" 2>"$scan_err" </dev/null || scan_rc=$?
			else
				HOME="$scan_home" "$OC_BIN" $scan_argv \
					>"$scan_out" 2>"$scan_err" </dev/null || scan_rc=$?
			fi
			if [ "$scan_rc" -eq 0 ]; then
				break
			fi
			scan_attempt=$((scan_attempt + 1))
		done
		if [ "$scan_rc" -ne 0 ]; then
			scan_skipped=$((scan_skipped + 1))
			printf '  [skip] %s: exited rc=%s after retry; stderr tail: %s\n' \
				"$scan_label" "$scan_rc" "$(tail -c 160 "$scan_err" 2>/dev/null | tr '\n' ' ')"
			continue
		fi
		scan_verified=$((scan_verified + 1))
		"$scan_process_fn" "$scan_class" "$scan_label" "$scan_out" "$scan_err"
	done <"$scan_samples"
	if [ "$scan_verified" -eq 0 ]; then
		printf 'error: %s verified no sample successfully (scope=%s); refusing to pass vacuously\n' \
			"$OC_SCRIPT_NAME" "$SCOPE" >&2
		exit 1
	fi
	if [ "$OC_FAILURES" -gt 0 ]; then
		printf '%s check: FAILED (%s violation(s), scope=%s, verified=%s/%s, skipped=%s)\n' \
			"$OC_SCRIPT_NAME" "$OC_FAILURES" "$SCOPE" "$scan_verified" "$scan_total" "$scan_skipped" >&2
		exit 1
	fi
	printf '%s check: ok (scope=%s, verified=%s/%s, skipped=%s)\n' \
		"$OC_SCRIPT_NAME" "$SCOPE" "$scan_verified" "$scan_total" "$scan_skipped"
}

# output_contract_report_violations <label> <violations>
output_contract_report_violations() {
	report_label="$1"
	report_violations="$2"
	if [ -z "$report_violations" ]; then
		return 0
	fi
	OC_FAILURES=$((OC_FAILURES + 1))
	printf '%s\n' "$report_violations" | while IFS= read -r report_line; do
		printf '  [%s] %s\n' "$report_label" "$report_line"
	done
}
