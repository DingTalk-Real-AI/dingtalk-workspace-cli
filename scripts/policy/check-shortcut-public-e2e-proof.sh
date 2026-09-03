#!/usr/bin/env bash

set -euo pipefail

ROOT="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
MANIFEST="$ROOT/scripts/policy/shortcut-public-e2e-proof.json"
BINARY="${DWS_SHORTCUT_PROOF_BINARY:-$ROOT/dws}"
GO_BINARY="${GO:-go}"
PROOF_ROOT=""

fail() {
  printf 'shortcut public E2E proof failed: %s\n' "$*" >&2
  exit 1
}

cleanup() {
  if [ -n "$PROOF_ROOT" ] && [ -d "$PROOF_ROOT" ]; then
    find "$PROOF_ROOT" -type f -delete 2>/dev/null || true
    find "$PROOF_ROOT" -depth -type d -exec rmdir -- {} \; 2>/dev/null || true
  fi
}

trap cleanup EXIT HUP INT TERM

command -v jq >/dev/null 2>&1 || fail "jq is required"
command -v "$GO_BINARY" >/dev/null 2>&1 || fail "Go is required"
[ -x "$BINARY" ] || fail "current source binary is missing; run make build"
[ -f "$MANIFEST" ] || fail "reviewed proof manifest is missing"

if [ "${DWS_SHORTCUT_PROOF_ALLOW_DIRTY:-0}" != "1" ] &&
  [ -n "$(git -C "$ROOT" status --porcelain --untracked-files=all)" ]; then
  fail "worktree must be clean so evidence is bound to current HEAD"
fi

HEAD_SHA="$(git -C "$ROOT" rev-parse HEAD)"
[ -n "$HEAD_SHA" ] || fail "cannot resolve current HEAD"

jq -e '
  .version == 1 and
  (.products | type == "array" and length > 0) and
  (.public_shortcuts | type == "array") and
  (.products | group_by(.service) | all(.[]; length == 1)) and
  (.public_shortcuts | group_by([.service, .command]) | all(.[]; length == 1)) and
  (.products | all(.[];
    (.service | type == "string" and length > 0) and
    (.source_total | type == "number") and
    (.public | type == "number") and
    (.unavailable | type == "number") and
    (.source_total == (.public + .unavailable)))) and
  (.public_shortcuts | all(.[];
    (.service | type == "string" and length > 0) and
    (.command | type == "string" and startswith("+")) and
    (.owning_atomic | type == "string" and length > 0) and
    (.proof_class | IN("list", "search", "read", "write")) and
    (.proof_case | type == "string" and length > 0) and
    (.stable_target | type == "string" and length > 0) and
    (.pass_labels | type == "array" and length > 0 and all(.[]; type == "string" and endswith("-PASS")))))
' "$MANIFEST" >/dev/null || fail "reviewed proof manifest is malformed"

SOURCE_TOTAL=0
PUBLIC_TOTAL=0
UNAVAILABLE_TOTAL=0
SEMANTIC_PUBLIC=""
RUNTIME_PUBLIC=""

for service in $(jq -r '.products[].service' "$MANIFEST"); do
  catalog="$ROOT/internal/shortcut/semantic_catalog_${service}.json"
  [ -f "$catalog" ] || fail "semantic catalog is missing for $service"

  jq -e --arg service "$service" '
    . as $catalog |
    .service == $service and
    .default_availability == "unavailable" and
    (.shortcuts | type == "object") and
    (.shortcuts | to_entries | all(.[];
      (.key | type == "string" and startswith("+")) and
      (.value | type == "object") and
      (.value.reviewed == true) and
      ((.value.semantic_delta | type) == "string" and (.value.semantic_delta | length) > 0) and
      (if (.value.public // false)
       then ((.value.availability // $catalog.default_availability) == "available")
       else ((.value.availability // $catalog.default_availability) == "unavailable")
       end)))
  ' "$catalog" >/dev/null || fail "$service semantic catalog is not truthfully reviewed"

  source_count="$(jq '.shortcuts | length' "$catalog")"
  public_count="$(jq '[. as $catalog | .shortcuts | to_entries[] | select((.value.public // false) and ((.value.availability // $catalog.default_availability) == "available"))] | length' "$catalog")"
  unavailable_count=$((source_count - public_count))
  expected_source="$(jq -r --arg service "$service" '.products[] | select(.service == $service) | .source_total' "$MANIFEST")"
  expected_public="$(jq -r --arg service "$service" '.products[] | select(.service == $service) | .public' "$MANIFEST")"
  expected_unavailable="$(jq -r --arg service "$service" '.products[] | select(.service == $service) | .unavailable' "$MANIFEST")"
  [ "$source_count" -eq "$expected_source" ] || fail "$service source count drifted"
  [ "$public_count" -eq "$expected_public" ] || fail "$service public count drifted"
  [ "$unavailable_count" -eq "$expected_unavailable" ] || fail "$service unavailable count drifted"

  semantic_lines="$(jq -r --arg service "$service" '. as $catalog | .shortcuts | to_entries[] | select((.value.public // false) and ((.value.availability // $catalog.default_availability) == "available")) | [$service, .key] | @tsv' "$catalog")"
  if [ -n "$semantic_lines" ]; then
    SEMANTIC_PUBLIC="${SEMANTIC_PUBLIC}${semantic_lines}"$'\n'
  fi

  runtime_output="$("$BINARY" shortcut list --service "$service" --format json)"
  printf '%s' "$runtime_output" | jq -e --arg service "$service" '
    type == "object" and
    (.count | type == "number") and
    (.shortcuts | type == "array") and
    (.count == (.shortcuts | length)) and
    (.shortcuts | all(.[];
      type == "object" and
      .service == $service and
      (.command | type == "string" and startswith("+")) and
      .public == true and
      .reviewed == true and
      .availability == "available"))
  ' >/dev/null || fail "$service runtime public inventory is malformed"
  runtime_count="$(printf '%s' "$runtime_output" | jq '.shortcuts | length')"
  [ "$runtime_count" -eq "$public_count" ] || fail "$service runtime public count does not match its semantic catalog"
  runtime_lines="$(printf '%s' "$runtime_output" | jq -r '.shortcuts[] | [.service, .command] | @tsv')"
  if [ -n "$runtime_lines" ]; then
    RUNTIME_PUBLIC="${RUNTIME_PUBLIC}${runtime_lines}"$'\n'
  fi

  SOURCE_TOTAL=$((SOURCE_TOTAL + source_count))
  PUBLIC_TOTAL=$((PUBLIC_TOTAL + public_count))
  UNAVAILABLE_TOTAL=$((UNAVAILABLE_TOTAL + unavailable_count))
done

SEMANTIC_PUBLIC="$(printf '%s' "$SEMANTIC_PUBLIC" | sed '/^$/d' | LC_ALL=C sort)"
RUNTIME_PUBLIC="$(printf '%s' "$RUNTIME_PUBLIC" | sed '/^$/d' | LC_ALL=C sort)"
EXPECTED_PUBLIC="$(jq -r '.public_shortcuts[] | [.service, .command] | @tsv' "$MANIFEST" | LC_ALL=C sort)"
[ "$SEMANTIC_PUBLIC" = "$RUNTIME_PUBLIC" ] || fail "semantic and runtime public surfaces differ"
[ "$RUNTIME_PUBLIC" = "$EXPECTED_PUBLIC" ] || fail "a public shortcut lacks a reviewed exact plus raw proof case"

printf 'CURRENT-HEAD-PUBLIC-SURFACE-PASS head=%s source=%d public=%d unavailable=%d\n' "$HEAD_SHA" "$SOURCE_TOTAL" "$PUBLIC_TOTAL" "$UNAVAILABLE_TOTAL"

(cd "$ROOT" && DWS_PACKAGE_VERSION="${DWS_PACKAGE_VERSION:-0.0.0-test}" "$GO_BINARY" test -count=1 ./internal/shortcut/pat -run '^TestCrossPlatformCoveragePATBrowserPolicyConfirmationDryRunReadbackAndCleanup$' >/dev/null) ||
  fail "PAT exact/raw zero-call structural proof failed"

run_pat_browser_policy_default() {
  schema_output="$("$BINARY" schema --cli-path 'pat +browser-policy' --compact --format json)"
  printf '%s' "$schema_output" | jq -e '
    type == "object" and
    .cli_path == "pat +browser-policy" and
    .effect == "write" and
    .risk == "medium" and
    .confirmation == "user_required" and
    .idempotency == "idempotent" and
    .availability == "available" and
    .interface_mode == "composite" and
    (.result | type == "object") and
    (.result.outcomes | type == "array" and index("success") != null and index("failure") != null) and
    (.result.data_schema.required | sort == ["executed", "openBrowser", "scope", "verified"])
  ' >/dev/null || fail "pat +browser-policy compact contract is incomplete"

  PROOF_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/dws-pat-public-proof.XXXXXX")"
  exact_dir="$PROOF_ROOT/exact"
  raw_dir="$PROOF_ROOT/raw"
  mkdir -p "$exact_dir" "$raw_dir"
  exact_path="$exact_dir/pat_policy.json"
  raw_path="$raw_dir/pat_policy.json"

  if env -u DINGTALK_DWS_AGENTCODE -u DINGTALK_DWS_SESSIONID -u DINGTALK_DWS_ORGID -u DINGTALK_DWS_UID \
    DWS_CONFIG_DIR="$exact_dir" HTTPS_PROXY=http://127.0.0.1:1 HTTP_PROXY=http://127.0.0.1:1 \
    "$BINARY" pat +browser-policy --enabled=false --format json </dev/null >/dev/null 2>&1; then
    fail "pat +browser-policy wrote without confirmation"
  fi
  [ ! -e "$exact_path" ] || fail "pat +browser-policy left state before confirmation"
  printf '%s\n' 'PAT-BROWSER-POLICY-PRECONFIRM-ZERO-CALL-PASS'

  exact_dry_run="$(env -u DINGTALK_DWS_AGENTCODE -u DINGTALK_DWS_SESSIONID -u DINGTALK_DWS_ORGID -u DINGTALK_DWS_UID \
    DWS_CONFIG_DIR="$exact_dir" HTTPS_PROXY=http://127.0.0.1:1 HTTP_PROXY=http://127.0.0.1:1 \
    "$BINARY" pat +browser-policy --enabled=false --dry-run --format json)"
  printf '%s' "$exact_dry_run" | jq -e '
    type == "object" and .ok == true and .outcome == "success" and
    (.data | type == "object") and .data.scope == "default" and
    .data.openBrowser == false and .data.executed == false and .data.verified == false
  ' >/dev/null || fail "pat +browser-policy dry-run result is malformed"
  [ ! -e "$exact_path" ] || fail "pat +browser-policy dry-run wrote policy state"
  printf '%s\n' 'PAT-BROWSER-POLICY-DRY-RUN-ZERO-WRITE-PASS'

  exact_output="$(env -u DINGTALK_DWS_AGENTCODE -u DINGTALK_DWS_SESSIONID -u DINGTALK_DWS_ORGID -u DINGTALK_DWS_UID \
    DWS_CONFIG_DIR="$exact_dir" HTTPS_PROXY=http://127.0.0.1:1 HTTP_PROXY=http://127.0.0.1:1 \
    "$BINARY" pat +browser-policy --enabled=false --yes --format json)"
  printf '%s' "$exact_output" | jq -e '
    type == "object" and .ok == true and .outcome == "success" and
    (.data | type == "object") and .data.scope == "default" and
    .data.openBrowser == false and .data.executed == true and .data.verified == true
  ' >/dev/null || fail "pat +browser-policy terminal receipt is malformed"
  printf '%s\n' 'PAT-BROWSER-POLICY-EXACT-TERMINAL-RECEIPT-PASS'

  jq -e '
    type == "object" and (.default | type == "object") and
    .default.openBrowser == false and ((has("agents") | not) or .agents == {})
  ' "$exact_path" >/dev/null || fail "pat +browser-policy exact target readback failed"
  printf '%s\n' 'PAT-BROWSER-POLICY-EXACT-READBACK-PASS'

  raw_output="$(env -u DINGTALK_DWS_AGENTCODE -u DINGTALK_DWS_SESSIONID -u DINGTALK_DWS_ORGID -u DINGTALK_DWS_UID \
    DWS_CONFIG_DIR="$raw_dir" HTTPS_PROXY=http://127.0.0.1:1 HTTP_PROXY=http://127.0.0.1:1 \
    "$BINARY" pat browser-policy --enabled=false --format json)"
  printf '%s' "$raw_output" | jq -e '
    type == "object" and .scope == "default" and .source == "default" and
    .openBrowser == false and (has("agentCode") | not)
  ' >/dev/null || fail "pat browser-policy terminal receipt is malformed"
  printf '%s\n' 'PAT-BROWSER-POLICY-RAW-TERMINAL-RECEIPT-PASS'

  jq -e '
    type == "object" and (.default | type == "object") and
    .default.openBrowser == false and ((has("agents") | not) or .agents == {})
  ' "$raw_path" >/dev/null || fail "pat browser-policy raw target readback failed"
  printf '%s\n' 'PAT-BROWSER-POLICY-RAW-READBACK-PASS'

  unlink "$exact_path"
  unlink "$raw_path"
  [ ! -e "$exact_path" ] || fail "pat +browser-policy cleanup left exact residue"
  [ ! -e "$raw_path" ] || fail "pat browser-policy cleanup left raw residue"
  find "$exact_dir" "$raw_dir" -type f -delete
  [ -z "$(find "$PROOF_ROOT" -type f -print -quit)" ] || fail "PAT browser policy proof left file residue"
  find "$PROOF_ROOT" -depth -type d -exec rmdir -- {} \;
  [ ! -e "$PROOF_ROOT" ] || fail "PAT browser policy proof left directory residue"
  PROOF_ROOT=""
  printf '%s\n' 'PAT-BROWSER-POLICY-CLEANUP-ABSENCE-PASS'
}

while IFS=$'\t' read -r service command proof_case; do
  [ -n "$service" ] || continue
  case "$service $command $proof_case" in
    'pat +browser-policy pat_browser_policy_default')
      proof_output="$(run_pat_browser_policy_default)"
      ;;
    *)
      fail "$service $command has no executable proof implementation"
      ;;
  esac
  actual_labels="$(printf '%s\n' "$proof_output" | sed '/^$/d' | LC_ALL=C sort)"
  expected_labels="$(jq -r --arg service "$service" --arg command "$command" '.public_shortcuts[] | select(.service == $service and .command == $command) | .pass_labels[]' "$MANIFEST" | LC_ALL=C sort)"
  [ "$actual_labels" = "$expected_labels" ] || fail "$service $command did not emit every reviewed PASS label"
  printf '%s\n' "$proof_output"
done < <(jq -r '.public_shortcuts[] | [.service, .command, .proof_case] | @tsv' "$MANIFEST")

printf 'EVERY-PUBLIC-SHORTCUT-DOUBLE-LAYER-E2E-PASS total=%d\n' "$PUBLIC_TOTAL"
