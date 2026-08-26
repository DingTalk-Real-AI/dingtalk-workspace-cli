#!/usr/bin/env bash
set -euo pipefail

# Opt-in, real-data release evidence gate for the Dev -> DevApp -> Agoal
# Shortcut delivery group. The current worktree binary supplies both exact
# `+shortcut` leaves and their owning atomic command trees. Raw responses and
# stable IDs stay in a repository-external temporary directory; stdout contains
# only PII-free PASS labels and aggregate facts.

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
dws_bin=${DWS_SHORTCUT_GATE_BIN:-"$repo_root/dws"}
selected_product=${1:-all}

case "$selected_product" in
  all | dev | devapp | agoal) ;;
  *)
    echo "usage: $0 [all|dev|devapp|agoal]" >&2
    exit 2
    ;;
esac

if [[ ${DWS_SHORTCUT_LIVE_GATE:-} != 1 ]]; then
  echo "set DWS_SHORTCUT_LIVE_GATE=1 to authorize the real-data dual-layer gate" >&2
  exit 2
fi
if [[ $selected_product == all || $selected_product == devapp ]]; then
  if [[ ! -t 0 ]]; then
    echo "the DevApp live gate requires an interactive terminal for explicit write and cleanup confirmation" >&2
    exit 2
  fi
  read -r -p "This gate creates, changes, and deletes isolated DevApp fixtures. Type RUN to authorize the writes and cleanup: " live_confirmation
  if [[ $live_confirmation != RUN ]]; then
    echo "live gate authorization declined" >&2
    exit 2
  fi
fi
if [[ ! -x $dws_bin ]]; then
  echo "current worktree binary is missing; run make build first" >&2
  exit 2
fi
command -v jq >/dev/null 2>&1 || {
  echo "jq is required" >&2
  exit 2
}

member_user_id=""
member_type=${DWS_DEVAPP_E2E_MEMBER_TYPE:-DEVELOPER}
if [[ $selected_product == all || $selected_product == devapp ]]; then
  member_user_id_file=${DWS_DEVAPP_E2E_MEMBER_USER_ID_FILE:-}
  if [[ -z $member_user_id_file || ! -f $member_user_id_file ]]; then
    echo "set DWS_DEVAPP_E2E_MEMBER_USER_ID_FILE to a private file containing one safe test-account userId" >&2
    exit 2
  fi
  IFS= read -r member_user_id <"$member_user_id_file" || true
  [[ -n $member_user_id ]] || {
    echo "the private DevApp member fixture is empty" >&2
    exit 2
  }
  [[ -n $member_type ]] || {
    echo "DWS_DEVAPP_E2E_MEMBER_TYPE must not be empty" >&2
    exit 2
  }
fi

evidence_dir=$(mktemp -d "${TMPDIR:-/tmp}/dws-shortcut-dual-live.XXXXXX")
exact_app_id=""
exact_app_name=""
raw_app_id=""
raw_app_name=""

fail() {
  echo "FAIL $1 (secure response retained only until trap cleanup)" >&2
  exit 1
}

run_json() {
  local label=$1
  local output=$2
  shift 2
  if ! "$@" --format json >"$output" 2>"$output.stderr"; then
    fail "$label"
  fi
  jq -e . "$output" >/dev/null 2>&1 || fail "$label: non-JSON response"
}

assert_jq() {
  local label=$1
  local expression=$2
  shift 2
  jq -e "$expression" "$@" >/dev/null 2>&1 || fail "$label"
}

pass() {
  printf 'PASS %s\n' "$1"
}

assert_public_surface() {
  local product=$1
  local expected=$2
  local actual
  actual=$(jq -r --arg product "$product" '.results[] | select(.service == $product) | .command' "$repo_root/docs/shortcut-public-catalog.json" | LC_ALL=C sort)
  expected=$(printf '%s\n' "$expected" | sed '/^$/d' | LC_ALL=C sort)
  [[ $actual == "$expected" ]] || fail "$product public catalog differs from the exhaustive live matrix"
}

cleanup() {
  set +e
  if [[ -n $exact_app_id ]]; then
    "$dws_bin" devapp +delete --unified-app-id "$exact_app_id" --yes --format json >/dev/null 2>&1
  fi
  if [[ -n $raw_app_id ]]; then
    "$dws_bin" dev app delete --unified-app-id "$raw_app_id" --confirm-name "$raw_app_name" --yes --format json >/dev/null 2>&1
  fi
  rm -rf "$evidence_dir"
}
trap cleanup EXIT INT TERM

run_dev_gate() {
  # Dev owns routing/documentation only and registers no Shortcut leaves.
  assert_public_surface dev ""
  pass "DEV-INVENTORY public=0 fully-unlocked=0 hardened-blocked=0 routed=4 platform-unavailable=1"
}

expect_confirmation() {
  local label=$1
  local output=$2
  shift 2
  local status=0
  # The outer gate intentionally requires a TTY for one explicit RUN
  # authorization. The command under test must not inherit that TTY here:
  # otherwise its own confirmation prompt blocks instead of returning the
  # machine-readable confirmation_required result that proves zero writes.
  "$@" --format json </dev/null >"$output" 2>"$output.stderr" || status=$?
  [[ $status -ne 0 ]] || fail "$label: unconfirmed command unexpectedly succeeded"
  jq -e '.ok == false and .error.type == "validation" and .error.subtype == "confirmation_required"' "$output" >/dev/null 2>&1 || fail "$label: missing confirmation_required"
}

run_devapp_gate() {
  assert_public_surface devapp "+create
+credentials-get
+delete
+disable
+enable
+event-list
+event-subscribe
+get
+list
+member-add
+member-list
+member-remove
+permission-list
+robot-config
+robot-disable
+robot-enable
+robot-get
+update
+version-check-approval
+version-create
+version-get
+version-list
+version-status
+webapp-config
+webapp-get"
  local nonce="DWS-GUARANTEED-ZERO-8F3D2C4A"
  local suffix="$(date +%s)-$$"
  exact_app_name="DWS-E2E-EXACT-$suffix"
  raw_app_name="DWS-E2E-RAW-$suffix"
  local exact_updated_name="DWS-E2E-UPDATED-$suffix"
  local raw_updated_name="DWS-E2E-RAW-UPDATED-$suffix"
  local bot_raw="DWS-E2E-BOT-RAW-$suffix"
  local bot_exact="DWS-E2E-BOT-EXACT-$suffix"
  local event_raw="bpms_task_change"
  local event_exact="$event_raw"

  run_json "devapp create guard before exact query" "$evidence_dir/devapp-create-guard-before-exact.json" \
    "$dws_bin" devapp +list --name "$exact_app_name" --page-size 20
  run_json "devapp create guard before raw query" "$evidence_dir/devapp-create-guard-before-raw.json" \
    "$dws_bin" dev app list --name "$exact_app_name" --page-size 20
  expect_confirmation "devapp +create confirmation" "$evidence_dir/devapp-create-confirm.json" \
    "$dws_bin" devapp +create --name "$exact_app_name"
  run_json "devapp create guard after exact query" "$evidence_dir/devapp-create-guard-after-exact.json" \
    "$dws_bin" devapp +list --name "$exact_app_name" --page-size 20
  run_json "devapp create guard after raw query" "$evidence_dir/devapp-create-guard-after-raw.json" \
    "$dws_bin" dev app list --name "$exact_app_name" --page-size 20
  assert_jq "devapp +create confirmation zero-call state" '
    (input) as $br | (input) as $ae | (input) as $ar |
    .data.count == 0 and ($br.data.items | length) == 0 and
    $ae.data.count == 0 and ($ar.data.items | length) == 0
  ' "$evidence_dir/devapp-create-guard-before-exact.json" "$evidence_dir/devapp-create-guard-before-raw.json" \
    "$evidence_dir/devapp-create-guard-after-exact.json" "$evidence_dir/devapp-create-guard-after-raw.json"

  run_json "devapp exact create" "$evidence_dir/devapp-create-exact.json" \
    "$dws_bin" devapp +create --name "$exact_app_name" --desc "dual-live-exact" --yes
  exact_app_id=$(jq -er '.data.unifiedAppId | select(type == "string" and length > 0)' "$evidence_dir/devapp-create-exact.json") || fail "devapp exact create stable ID"
  assert_jq "devapp exact create terminal receipt" '.ok == true and .outcome == "success" and .data.verified == true and .data.action == "create"' "$evidence_dir/devapp-create-exact.json"
  run_json "devapp exact create raw readback" "$evidence_dir/devapp-create-exact-raw-readback.json" \
    "$dws_bin" dev app get --unified-app-id "$exact_app_id"
  jq -e --arg id "$exact_app_id" --arg name "$exact_app_name" '.ok == true and .outcome == "success" and .data.unifiedAppId == $id and .data.name == $name' "$evidence_dir/devapp-create-exact-raw-readback.json" >/dev/null || fail "devapp exact create raw readback"

  run_json "devapp raw create" "$evidence_dir/devapp-create-raw.json" \
    "$dws_bin" dev app create --name "$raw_app_name" --desc "dual-live-raw" --yes
  raw_app_id=$(jq -er '.data.unifiedAppId | select(type == "string" and length > 0)' "$evidence_dir/devapp-create-raw.json") || fail "devapp raw create stable ID"
  assert_jq "devapp raw create terminal receipt" '.ok == true and .outcome == "success"' "$evidence_dir/devapp-create-raw.json"
  run_json "devapp raw create exact readback" "$evidence_dir/devapp-create-raw-exact-readback.json" \
    "$dws_bin" devapp +get --unified-app-id "$raw_app_id"
  jq -e --arg id "$raw_app_id" --arg name "$raw_app_name" '.ok == true and .outcome == "success" and .data.unifiedAppId == $id and .data.name == $name' "$evidence_dir/devapp-create-raw-exact-readback.json" >/dev/null || fail "devapp raw create exact readback"
  pass "DEVAPP +create confirm0/exact+raw terminal-receipt/stable-ID readback"

  run_json "devapp guard snapshot app" "$evidence_dir/devapp-guard-app-before.json" \
    "$dws_bin" dev app get --unified-app-id "$exact_app_id"
  run_json "devapp guard snapshot webapp" "$evidence_dir/devapp-guard-webapp-before.json" \
    "$dws_bin" dev app webapp get --unified-app-id "$exact_app_id"
  run_json "devapp guard snapshot robot" "$evidence_dir/devapp-guard-robot-before.json" \
    "$dws_bin" dev app robot get --unified-app-id "$exact_app_id"
  run_json "devapp guard snapshot events" "$evidence_dir/devapp-guard-events-before.json" \
    "$dws_bin" dev app event list --unified-app-id "$exact_app_id" --page-size 100
  run_json "devapp guard snapshot versions" "$evidence_dir/devapp-guard-versions-before.json" \
    "$dws_bin" dev app version list --unified-app-id "$exact_app_id" --page-size 100

  expect_confirmation "devapp +update confirmation" "$evidence_dir/devapp-update-confirm.json" \
    "$dws_bin" devapp +update --unified-app-id "$exact_app_id" --name "guard-must-not-write"
  expect_confirmation "devapp +delete confirmation" "$evidence_dir/devapp-delete-confirm.json" \
    "$dws_bin" devapp +delete --unified-app-id "$exact_app_id"
  expect_confirmation "devapp +enable confirmation" "$evidence_dir/devapp-enable-confirm.json" \
    "$dws_bin" devapp +enable --unified-app-id "$exact_app_id"
  expect_confirmation "devapp +disable confirmation" "$evidence_dir/devapp-disable-confirm.json" \
    "$dws_bin" devapp +disable --unified-app-id "$exact_app_id"
  expect_confirmation "devapp +webapp-config confirmation" "$evidence_dir/devapp-webapp-confirm.json" \
    "$dws_bin" devapp +webapp-config --unified-app-id "$exact_app_id" --homepage-url "https://example.invalid/guard"
  expect_confirmation "devapp +robot-config confirmation" "$evidence_dir/devapp-robot-config-confirm.json" \
    "$dws_bin" devapp +robot-config --unified-app-id "$exact_app_id" --name "guard-must-not-write" --mode STREAM
  expect_confirmation "devapp +robot-enable confirmation" "$evidence_dir/devapp-robot-enable-confirm.json" \
    "$dws_bin" devapp +robot-enable --unified-app-id "$exact_app_id"
  expect_confirmation "devapp +robot-disable confirmation" "$evidence_dir/devapp-robot-disable-confirm.json" \
    "$dws_bin" devapp +robot-disable --unified-app-id "$exact_app_id"
  expect_confirmation "devapp +event-subscribe confirmation" "$evidence_dir/devapp-event-confirm.json" \
    "$dws_bin" devapp +event-subscribe --unified-app-id "$exact_app_id" --event-codes "$event_exact"
  expect_confirmation "devapp +version-create confirmation" "$evidence_dir/devapp-version-create-confirm.json" \
    "$dws_bin" devapp +version-create --unified-app-id "$exact_app_id" --desc "guard-must-not-write"

  run_json "devapp guard snapshot app after" "$evidence_dir/devapp-guard-app-after.json" \
    "$dws_bin" dev app get --unified-app-id "$exact_app_id"
  run_json "devapp guard snapshot webapp after" "$evidence_dir/devapp-guard-webapp-after.json" \
    "$dws_bin" dev app webapp get --unified-app-id "$exact_app_id"
  run_json "devapp guard snapshot robot after" "$evidence_dir/devapp-guard-robot-after.json" \
    "$dws_bin" dev app robot get --unified-app-id "$exact_app_id"
  run_json "devapp guard snapshot events after" "$evidence_dir/devapp-guard-events-after.json" \
    "$dws_bin" dev app event list --unified-app-id "$exact_app_id" --page-size 100
  run_json "devapp guard snapshot versions after" "$evidence_dir/devapp-guard-versions-after.json" \
    "$dws_bin" dev app version list --unified-app-id "$exact_app_id" --page-size 100
  assert_jq "devapp all public write confirmations preserve raw state" '
    (input) as $aa | (input) as $wb | (input) as $wa | (input) as $rb | (input) as $ra |
    (input) as $eb | (input) as $ea | (input) as $vb | (input) as $va |
    [.data.unifiedAppId,.data.name,.data.desc,.data.appStatus] == [$aa.data.unifiedAppId,$aa.data.name,$aa.data.desc,$aa.data.appStatus] and
    $wb.data == $wa.data and $rb.data == $ra.data and
    ([$eb.data.events[].eventCode] | sort) == ([$ea.data.events[].eventCode] | sort) and
    ([$vb.data.items[].versionId] | sort) == ([$va.data.items[].versionId] | sort)
  ' "$evidence_dir/devapp-guard-app-before.json" "$evidence_dir/devapp-guard-app-after.json" \
    "$evidence_dir/devapp-guard-webapp-before.json" "$evidence_dir/devapp-guard-webapp-after.json" \
    "$evidence_dir/devapp-guard-robot-before.json" "$evidence_dir/devapp-guard-robot-after.json" \
    "$evidence_dir/devapp-guard-events-before.json" "$evidence_dir/devapp-guard-events-after.json" \
    "$evidence_dir/devapp-guard-versions-before.json" "$evidence_dir/devapp-guard-versions-after.json"

  run_json "devapp exact list known" "$evidence_dir/devapp-list-known-exact.json" \
    "$dws_bin" devapp +list --name "$exact_app_name" --page-size 20
  run_json "devapp raw list known" "$evidence_dir/devapp-list-known-raw.json" \
    "$dws_bin" dev app list --name "$exact_app_name" --page-size 20
  run_json "devapp exact list zero" "$evidence_dir/devapp-list-zero-exact.json" \
    "$dws_bin" devapp +list --name "$nonce" --page-size 20
  run_json "devapp raw list zero" "$evidence_dir/devapp-list-zero-raw.json" \
    "$dws_bin" dev app list --name "$nonce" --page-size 20
  jq -e --arg id "$exact_app_id" '
    (input) as $raw | (input) as $ze | (input) as $zr |
    .ok == true and .outcome == "success" and .data.count == 1 and .data.apps[0].unifiedAppId == $id and
    $raw.ok == true and $raw.outcome == "success" and ($raw.data.items | length) == 1 and $raw.data.items[0].unifiedAppId == $id and
    $ze.ok == true and $ze.data.count == 0 and ($ze.data.apps | length) == 0 and
    $zr.ok == true and ($zr.data.items | length) == 0 and
    .meta.pagination.endpoint_exhausted == true and $raw.meta.pagination.endpoint_exhausted == true and
    $ze.meta.pagination.endpoint_exhausted == true and $zr.meta.pagination.endpoint_exhausted == true
  ' "$evidence_dir/devapp-list-known-exact.json" "$evidence_dir/devapp-list-known-raw.json" \
    "$evidence_dir/devapp-list-zero-exact.json" "$evidence_dir/devapp-list-zero-raw.json" >/dev/null || fail "devapp +list dual known/zero alignment"
  pass "DEVAPP +list exact+raw known-nonempty/guaranteed-zero stable-ID/cursor termination"

  run_json "devapp exact get" "$evidence_dir/devapp-get-exact.json" \
    "$dws_bin" devapp +get --unified-app-id "$exact_app_id"
  run_json "devapp raw get" "$evidence_dir/devapp-get-raw.json" \
    "$dws_bin" dev app get --unified-app-id "$exact_app_id"
  jq -e --arg id "$exact_app_id" '(input) as $raw | .ok == true and .data.unifiedAppId == $id and $raw.ok == true and $raw.data.unifiedAppId == $id and .data.appKey == $raw.data.appKey' "$evidence_dir/devapp-get-exact.json" "$evidence_dir/devapp-get-raw.json" >/dev/null || fail "devapp +get dual object read"
  pass "DEVAPP +get exact+raw stable-application-ID object alignment"

  run_json "devapp raw update" "$evidence_dir/devapp-update-raw.json" \
    "$dws_bin" dev app update --unified-app-id "$exact_app_id" --name "$raw_updated_name" --desc "raw-update" --yes
  assert_jq "devapp raw update terminal receipt" '.ok == true and .outcome == "success"' "$evidence_dir/devapp-update-raw.json"
  run_json "devapp raw update exact readback" "$evidence_dir/devapp-update-raw-exact-readback.json" \
    "$dws_bin" devapp +get --unified-app-id "$exact_app_id"
  jq -e --arg id "$exact_app_id" --arg name "$raw_updated_name" '.ok == true and .data.unifiedAppId == $id and .data.name == $name and .data.desc == "raw-update"' "$evidence_dir/devapp-update-raw-exact-readback.json" >/dev/null || fail "devapp raw update exact readback"
  run_json "devapp exact update" "$evidence_dir/devapp-update-exact.json" \
    "$dws_bin" devapp +update --unified-app-id "$exact_app_id" --name "$exact_updated_name" --desc "exact-update" --yes
  assert_jq "devapp exact update terminal receipt" '.ok == true and .outcome == "success" and .data.verified == true and .data.action == "update"' "$evidence_dir/devapp-update-exact.json"
  run_json "devapp exact update raw readback" "$evidence_dir/devapp-update-exact-raw-readback.json" \
    "$dws_bin" dev app get --unified-app-id "$exact_app_id"
  jq -e --arg id "$exact_app_id" --arg name "$exact_updated_name" '.ok == true and .data.unifiedAppId == $id and .data.name == $name and .data.desc == "exact-update"' "$evidence_dir/devapp-update-exact-raw-readback.json" >/dev/null || fail "devapp exact update raw readback"
  pass "DEVAPP +update confirm0/exact+raw terminal-receipt/key-field readback"

  run_json "devapp raw disable" "$evidence_dir/devapp-disable-raw.json" \
    "$dws_bin" dev app disable --unified-app-id "$exact_app_id" --yes
  run_json "devapp raw disable exact readback" "$evidence_dir/devapp-disable-raw-exact-readback.json" \
    "$dws_bin" devapp +get --unified-app-id "$exact_app_id"
  assert_jq "devapp raw disable terminal" '.ok == true and .outcome == "success"' "$evidence_dir/devapp-disable-raw.json"
  assert_jq "devapp raw disable exact state" '(.data.appStatus | ascii_downcase) == "disabled"' "$evidence_dir/devapp-disable-raw-exact-readback.json"
  run_json "devapp exact enable" "$evidence_dir/devapp-enable-exact.json" \
    "$dws_bin" devapp +enable --unified-app-id "$exact_app_id" --yes
  run_json "devapp exact enable raw readback" "$evidence_dir/devapp-enable-exact-raw-readback.json" \
    "$dws_bin" dev app get --unified-app-id "$exact_app_id"
  assert_jq "devapp exact enable terminal" '.ok == true and .outcome == "success" and .data.verified == true' "$evidence_dir/devapp-enable-exact.json"
  assert_jq "devapp exact enable raw state" '(.data.appStatus | ascii_downcase) == "normal"' "$evidence_dir/devapp-enable-exact-raw-readback.json"
  run_json "devapp exact disable" "$evidence_dir/devapp-disable-exact.json" \
    "$dws_bin" devapp +disable --unified-app-id "$exact_app_id" --yes
  run_json "devapp exact disable raw readback" "$evidence_dir/devapp-disable-exact-raw-readback.json" \
    "$dws_bin" dev app get --unified-app-id "$exact_app_id"
  assert_jq "devapp exact disable terminal" '.ok == true and .outcome == "success" and .data.verified == true' "$evidence_dir/devapp-disable-exact.json"
  assert_jq "devapp exact disable raw state" '(.data.appStatus | ascii_downcase) == "disabled"' "$evidence_dir/devapp-disable-exact-raw-readback.json"
  run_json "devapp raw enable" "$evidence_dir/devapp-enable-raw.json" \
    "$dws_bin" dev app enable --unified-app-id "$exact_app_id" --yes
  run_json "devapp raw enable exact readback" "$evidence_dir/devapp-enable-raw-exact-readback.json" \
    "$dws_bin" devapp +get --unified-app-id "$exact_app_id"
  assert_jq "devapp raw enable terminal" '.ok == true and .outcome == "success"' "$evidence_dir/devapp-enable-raw.json"
  assert_jq "devapp raw enable exact state" '(.data.appStatus | ascii_downcase) == "normal"' "$evidence_dir/devapp-enable-raw-exact-readback.json"
  pass "DEVAPP +disable confirm0/exact+raw terminal-receipt/disabled readback/restored"
  pass "DEVAPP +enable confirm0/exact+raw terminal-receipt/normal readback"

  run_json "devapp exact credentials" "$evidence_dir/devapp-credentials-exact.json" \
    "$dws_bin" devapp +credentials-get --unified-app-id "$exact_app_id"
  run_json "devapp raw credentials" "$evidence_dir/devapp-credentials-raw.json" \
    "$dws_bin" dev app credentials get --unified-app-id "$exact_app_id"
  jq -e --arg id "$exact_app_id" '
    (input) as $raw |
    .ok == true and .outcome == "success" and .data.unifiedAppId == $id and
    (.data.appKey | type) == "string" and (.data.appKey | length) > 0 and
    (.data.appSecret | type) == "string" and (.data.appSecret | length) > 0 and
    $raw.ok == true and $raw.data.unifiedAppId == $id and
    .data.appKey == $raw.data.appKey and .data.appSecret == $raw.data.appSecret
  ' "$evidence_dir/devapp-credentials-exact.json" "$evidence_dir/devapp-credentials-raw.json" >/dev/null || fail "devapp +credentials-get dual object read"
  pass "DEVAPP +credentials-get exact+raw stable-app/client/one-nonempty-secret alignment"

  run_json "devapp raw webapp config" "$evidence_dir/devapp-webapp-config-raw.json" \
    "$dws_bin" dev app webapp config --unified-app-id "$exact_app_id" --homepage-url "https://example.invalid/raw" --yes
  run_json "devapp raw webapp exact readback" "$evidence_dir/devapp-webapp-config-raw-exact-readback.json" \
    "$dws_bin" devapp +webapp-get --unified-app-id "$exact_app_id"
  assert_jq "devapp raw webapp terminal" '.ok == true and .outcome == "success"' "$evidence_dir/devapp-webapp-config-raw.json"
  assert_jq "devapp raw webapp exact readback" '.data.homepageUrl == "https://example.invalid/raw"' "$evidence_dir/devapp-webapp-config-raw-exact-readback.json"
  run_json "devapp exact webapp config" "$evidence_dir/devapp-webapp-config-exact.json" \
    "$dws_bin" devapp +webapp-config --unified-app-id "$exact_app_id" --homepage-url "https://example.invalid/exact" --yes
  run_json "devapp exact webapp raw readback" "$evidence_dir/devapp-webapp-config-exact-raw-readback.json" \
    "$dws_bin" dev app webapp get --unified-app-id "$exact_app_id"
  assert_jq "devapp exact webapp terminal" '.ok == true and .outcome == "success" and .data.verified == true' "$evidence_dir/devapp-webapp-config-exact.json"
  assert_jq "devapp exact webapp raw readback" '.data.homepageUrl == "https://example.invalid/exact"' "$evidence_dir/devapp-webapp-config-exact-raw-readback.json"
  pass "DEVAPP +webapp-config confirm0/exact+raw terminal-receipt/URL readback"
  run_json "devapp exact webapp get final" "$evidence_dir/devapp-webapp-get-exact.json" \
    "$dws_bin" devapp +webapp-get --unified-app-id "$exact_app_id"
  run_json "devapp raw webapp get final" "$evidence_dir/devapp-webapp-get-raw.json" \
    "$dws_bin" dev app webapp get --unified-app-id "$exact_app_id"
  jq -e --arg id "$exact_app_id" '(input) as $raw | .data.unifiedAppId == $id and $raw.data.unifiedAppId == $id and .data.homepageUrl == $raw.data.homepageUrl' "$evidence_dir/devapp-webapp-get-exact.json" "$evidence_dir/devapp-webapp-get-raw.json" >/dev/null || fail "devapp +webapp-get dual object read"
  pass "DEVAPP +webapp-get exact+raw stable-app/key-field object alignment"

  run_json "devapp member raw known" "$evidence_dir/devapp-member-known-raw.json" \
    "$dws_bin" dev app member list --unified-app-id "$exact_app_id"
  run_json "devapp member exact known" "$evidence_dir/devapp-member-known-exact.json" \
    "$dws_bin" devapp +member-list --unified-app-id "$exact_app_id"
  run_json "devapp member exact zero" "$evidence_dir/devapp-member-zero-exact.json" \
    "$dws_bin" devapp +member-list --unified-app-id "$exact_app_id" --user-id "$nonce"
  jq -e --arg nonce "$nonce" '
    (input) as $raw | (input) as $zero |
    .ok == true and .data.count > 0 and $raw.ok == true and ($raw.data.members | type) == "array" and
    ([.data.members[].userId] | sort) == ([$raw.data.members[].userId] | sort) and
    $zero.ok == true and $zero.data.count == 0 and ($zero.data.members | length) == 0 and
    ([$raw.data.members[].userId] | index($nonce)) == null
  ' "$evidence_dir/devapp-member-known-exact.json" "$evidence_dir/devapp-member-known-raw.json" "$evidence_dir/devapp-member-zero-exact.json" >/dev/null || fail "devapp +member-list adapter alignment"
  pass "DEVAPP +member-list exact local-ID-filter+raw full-set known/guaranteed-zero stable-member alignment"

  run_json "devapp member fixture exact before" "$evidence_dir/devapp-member-fixture-before-exact.json" \
    "$dws_bin" devapp +member-list --unified-app-id "$exact_app_id" --user-id "$member_user_id"
  run_json "devapp member fixture raw before" "$evidence_dir/devapp-member-fixture-before-raw.json" \
    "$dws_bin" dev app member list --unified-app-id "$exact_app_id"
  jq -e --arg user "$member_user_id" '(input) as $raw | .data.count == 0 and (.data.members | length) == 0 and ([$raw.data.members[].userId] | index($user)) == null' \
    "$evidence_dir/devapp-member-fixture-before-exact.json" "$evidence_dir/devapp-member-fixture-before-raw.json" >/dev/null || fail "devapp member fixture initially absent"

  expect_confirmation "devapp +member-add confirmation" "$evidence_dir/devapp-member-add-confirm.json" \
    "$dws_bin" devapp +member-add --unified-app-id "$exact_app_id" --user-ids "$member_user_id" --member-type "$member_type"
  expect_confirmation "devapp +member-remove confirmation" "$evidence_dir/devapp-member-remove-confirm.json" \
    "$dws_bin" devapp +member-remove --unified-app-id "$exact_app_id" --user-ids "$member_user_id" --member-type "$member_type"
  run_json "devapp member fixture exact after guards" "$evidence_dir/devapp-member-fixture-after-guards-exact.json" \
    "$dws_bin" devapp +member-list --unified-app-id "$exact_app_id" --user-id "$member_user_id"
  run_json "devapp member fixture raw after guards" "$evidence_dir/devapp-member-fixture-after-guards-raw.json" \
    "$dws_bin" dev app member list --unified-app-id "$exact_app_id"
  jq -e --arg user "$member_user_id" '(input) as $raw | .data.count == 0 and (.data.members | length) == 0 and ([$raw.data.members[].userId] | index($user)) == null' \
    "$evidence_dir/devapp-member-fixture-after-guards-exact.json" "$evidence_dir/devapp-member-fixture-after-guards-raw.json" >/dev/null || fail "devapp member confirmations made no writes"

  run_json "devapp member exact add" "$evidence_dir/devapp-member-add-exact.json" \
    "$dws_bin" devapp +member-add --unified-app-id "$exact_app_id" --user-ids "$member_user_id" --member-type "$member_type" --yes
  run_json "devapp member exact add raw readback" "$evidence_dir/devapp-member-add-exact-raw-readback.json" \
    "$dws_bin" dev app member list --unified-app-id "$exact_app_id"
  jq -e --arg type "$member_type" '.ok == true and .outcome == "success" and .data.verified == true and .data.action == "member_add" and .data.resource.count == 1 and .data.resource.memberType == $type' \
    "$evidence_dir/devapp-member-add-exact.json" >/dev/null || fail "devapp member exact add terminal"
  jq -e --arg user "$member_user_id" --arg type "$member_type" '([.data.members[] | select(.userId == $user and .memberType == $type)] | length) == 1' \
    "$evidence_dir/devapp-member-add-exact-raw-readback.json" >/dev/null || fail "devapp member exact add raw stable-ID role readback"

  run_json "devapp member raw remove" "$evidence_dir/devapp-member-remove-raw.json" \
    "$dws_bin" dev app member remove --unified-app-id "$exact_app_id" --user-ids "$member_user_id" --member-type "$member_type" --yes
  run_json "devapp member raw remove exact readback" "$evidence_dir/devapp-member-remove-raw-exact-readback.json" \
    "$dws_bin" devapp +member-list --unified-app-id "$exact_app_id" --user-id "$member_user_id"
  assert_jq "devapp member raw remove terminal" '.ok == true and .outcome == "success"' "$evidence_dir/devapp-member-remove-raw.json"
  assert_jq "devapp member raw remove exact absence" '.ok == true and .data.count == 0 and (.data.members | length) == 0' "$evidence_dir/devapp-member-remove-raw-exact-readback.json"

  run_json "devapp member raw add" "$evidence_dir/devapp-member-add-raw.json" \
    "$dws_bin" dev app member add --unified-app-id "$exact_app_id" --user-ids "$member_user_id" --member-type "$member_type" --yes
  run_json "devapp member raw add exact readback" "$evidence_dir/devapp-member-add-raw-exact-readback.json" \
    "$dws_bin" devapp +member-list --unified-app-id "$exact_app_id" --user-id "$member_user_id"
  assert_jq "devapp member raw add terminal" '.ok == true and .outcome == "success"' "$evidence_dir/devapp-member-add-raw.json"
  jq -e --arg user "$member_user_id" --arg type "$member_type" \
    '.ok == true and .data.count == 1 and .data.members[0].userId == $user and .data.members[0].memberType == $type' \
    "$evidence_dir/devapp-member-add-raw-exact-readback.json" >/dev/null || fail "devapp member raw add exact stable-ID role readback"

  run_json "devapp member exact remove" "$evidence_dir/devapp-member-remove-exact.json" \
    "$dws_bin" devapp +member-remove --unified-app-id "$exact_app_id" --user-ids "$member_user_id" --member-type "$member_type" --yes
  run_json "devapp member exact remove raw readback" "$evidence_dir/devapp-member-remove-exact-raw-readback.json" \
    "$dws_bin" dev app member list --unified-app-id "$exact_app_id"
  assert_jq "devapp member exact remove terminal" '.ok == true and .outcome == "success" and .data.verified == true and .data.action == "member_remove"' \
    "$evidence_dir/devapp-member-remove-exact.json"
  jq -e --arg user "$member_user_id" '([.data.members[].userId] | index($user)) == null' \
    "$evidence_dir/devapp-member-remove-exact-raw-readback.json" >/dev/null || fail "devapp member exact remove raw absence"
  pass "DEVAPP +member-add/+member-remove confirm0/exact+raw terminal-receipt/stable-ID role/absence readback"

  run_json "devapp permission exact known" "$evidence_dir/devapp-permission-known-exact.json" \
    "$dws_bin" devapp +permission-list --unified-app-id "$exact_app_id" --page-size 20
  run_json "devapp permission raw known" "$evidence_dir/devapp-permission-known-raw.json" \
    "$dws_bin" dev app permission list --unified-app-id "$exact_app_id" --page-size 20
  run_json "devapp permission exact zero" "$evidence_dir/devapp-permission-zero-exact.json" \
    "$dws_bin" devapp +permission-list --unified-app-id "$exact_app_id" --keyword "$nonce" --page-size 20
  run_json "devapp permission raw zero" "$evidence_dir/devapp-permission-zero-raw.json" \
    "$dws_bin" dev app permission list --unified-app-id "$exact_app_id" --keyword "$nonce" --page-size 20
  assert_jq "devapp permission dual known/zero alignment" '
    (input) as $raw | (input) as $ze | (input) as $zr |
    .ok == true and .data.count > 0 and $raw.ok == true and ($raw.data.items | length) > 0 and
    ([.data.permissions[].scopeValue] | sort) == ([$raw.data.items[].scopeValue] | sort) and
    $ze.ok == true and $ze.data.count == 0 and ($ze.data.permissions | length) == 0 and
    $zr.ok == true and ($zr.data.items | length) == 0 and
    .meta.pagination.endpoint_exhausted == $raw.meta.pagination.endpoint_exhausted and
    $ze.meta.pagination.endpoint_exhausted == true and $zr.meta.pagination.endpoint_exhausted == true
  ' "$evidence_dir/devapp-permission-known-exact.json" "$evidence_dir/devapp-permission-known-raw.json" \
    "$evidence_dir/devapp-permission-zero-exact.json" "$evidence_dir/devapp-permission-zero-raw.json"
  pass "DEVAPP +permission-list exact+raw known-nonempty/guaranteed-zero stable-scope/cursor alignment"

  run_json "devapp robot exact initial" "$evidence_dir/devapp-robot-get-exact.json" \
    "$dws_bin" devapp +robot-get --unified-app-id "$exact_app_id"
  run_json "devapp robot raw initial" "$evidence_dir/devapp-robot-get-raw.json" \
    "$dws_bin" dev app robot get --unified-app-id "$exact_app_id"
  jq -e --arg id "$exact_app_id" '(input) as $raw | .data.unifiedAppId == $id and $raw.data.unifiedAppId == $id and .data.robotStatus == $raw.data.robotStatus' "$evidence_dir/devapp-robot-get-exact.json" "$evidence_dir/devapp-robot-get-raw.json" >/dev/null || fail "devapp +robot-get initial dual object"
  pass "DEVAPP +robot-get exact+raw stable-app/status object alignment"

  run_json "devapp robot raw config" "$evidence_dir/devapp-robot-config-raw.json" \
    "$dws_bin" dev app robot config --unified-app-id "$exact_app_id" --name "$bot_raw" --brief "dual-live-raw" --desc "dual-live-raw" --mode STREAM --add-scope --yes
  run_json "devapp robot raw config exact readback" "$evidence_dir/devapp-robot-config-raw-exact-readback.json" \
    "$dws_bin" devapp +robot-get --unified-app-id "$exact_app_id"
  assert_jq "devapp robot raw config terminal" '.ok == true and .outcome == "success"' "$evidence_dir/devapp-robot-config-raw.json"
  jq -e --arg name "$bot_raw" '.data.name == $name and .data.mode == "STREAM"' "$evidence_dir/devapp-robot-config-raw-exact-readback.json" >/dev/null || fail "devapp robot raw config exact readback"
  run_json "devapp robot exact config" "$evidence_dir/devapp-robot-config-exact.json" \
    "$dws_bin" devapp +robot-config --unified-app-id "$exact_app_id" --name "$bot_exact" --brief "dual-live-exact" --desc "dual-live-exact" --mode STREAM --yes
  run_json "devapp robot exact config raw readback" "$evidence_dir/devapp-robot-config-exact-raw-readback.json" \
    "$dws_bin" dev app robot get --unified-app-id "$exact_app_id"
  assert_jq "devapp robot exact config terminal" '.ok == true and .outcome == "success" and .data.verified == true' "$evidence_dir/devapp-robot-config-exact.json"
  jq -e --arg name "$bot_exact" '.data.name == $name and .data.mode == "STREAM"' "$evidence_dir/devapp-robot-config-exact-raw-readback.json" >/dev/null || fail "devapp robot exact config raw readback"
  pass "DEVAPP +robot-config confirm0/exact+raw terminal-receipt/config-field readback"

  run_json "devapp robot raw enable" "$evidence_dir/devapp-robot-enable-raw.json" \
    "$dws_bin" dev app robot enable --unified-app-id "$exact_app_id" --yes
  run_json "devapp robot raw enable exact readback" "$evidence_dir/devapp-robot-enable-raw-exact-readback.json" \
    "$dws_bin" devapp +robot-get --unified-app-id "$exact_app_id"
  assert_jq "devapp robot raw enable terminal" '.ok == true and .outcome == "success"' "$evidence_dir/devapp-robot-enable-raw.json"
  assert_jq "devapp robot raw enable exact state" '.data.robotStatus == "ONLINE"' "$evidence_dir/devapp-robot-enable-raw-exact-readback.json"
  run_json "devapp robot exact disable" "$evidence_dir/devapp-robot-disable-exact.json" \
    "$dws_bin" devapp +robot-disable --unified-app-id "$exact_app_id" --yes
  run_json "devapp robot exact disable raw readback" "$evidence_dir/devapp-robot-disable-exact-raw-readback.json" \
    "$dws_bin" dev app robot get --unified-app-id "$exact_app_id"
  assert_jq "devapp robot exact disable terminal" '.ok == true and .outcome == "success" and .data.verified == true' "$evidence_dir/devapp-robot-disable-exact.json"
  assert_jq "devapp robot exact disable raw state" '.data.robotStatus == "UNCONFIGURED"' "$evidence_dir/devapp-robot-disable-exact-raw-readback.json"
  run_json "devapp robot raw reconfigure" "$evidence_dir/devapp-robot-reconfigure-raw.json" \
    "$dws_bin" dev app robot config --unified-app-id "$exact_app_id" --name "$bot_raw" --brief "dual-live-raw" --mode STREAM --yes
  run_json "devapp robot exact enable" "$evidence_dir/devapp-robot-enable-exact.json" \
    "$dws_bin" devapp +robot-enable --unified-app-id "$exact_app_id" --yes
  run_json "devapp robot exact enable raw readback" "$evidence_dir/devapp-robot-enable-exact-raw-readback.json" \
    "$dws_bin" dev app robot get --unified-app-id "$exact_app_id"
  assert_jq "devapp robot exact enable terminal" '.ok == true and .outcome == "success" and .data.verified == true' "$evidence_dir/devapp-robot-enable-exact.json"
  assert_jq "devapp robot exact enable raw state" '.data.robotStatus == "ONLINE"' "$evidence_dir/devapp-robot-enable-exact-raw-readback.json"
  run_json "devapp robot raw disable" "$evidence_dir/devapp-robot-disable-raw.json" \
    "$dws_bin" dev app robot disable --unified-app-id "$exact_app_id" --yes
  run_json "devapp robot raw disable exact readback" "$evidence_dir/devapp-robot-disable-raw-exact-readback.json" \
    "$dws_bin" devapp +robot-get --unified-app-id "$exact_app_id"
  assert_jq "devapp robot raw disable terminal" '.ok == true and .outcome == "success"' "$evidence_dir/devapp-robot-disable-raw.json"
  assert_jq "devapp robot raw disable exact state" '.data.robotStatus == "UNCONFIGURED"' "$evidence_dir/devapp-robot-disable-raw-exact-readback.json"
  pass "DEVAPP +robot-enable confirm0/exact+raw terminal-receipt/ONLINE readback"
  pass "DEVAPP +robot-disable confirm0/exact+raw terminal-receipt/UNCONFIGURED readback"

  run_json "devapp event raw subscribe" "$evidence_dir/devapp-event-subscribe-raw.json" \
    "$dws_bin" dev app event subscribe --unified-app-id "$exact_app_id" --event-codes "$event_raw" --yes
  run_json "devapp event raw subscribe exact replay" "$evidence_dir/devapp-event-subscribe-raw-exact-replay.json" \
    "$dws_bin" devapp +event-subscribe --unified-app-id "$exact_app_id" --event-codes "$event_raw" --yes
  run_json "devapp event raw subscribe raw readback" "$evidence_dir/devapp-event-subscribe-raw-readback.json" \
    "$dws_bin" dev app event list --unified-app-id "$exact_app_id" --keyword "$event_raw" --page-size 100
  assert_jq "devapp event raw subscribe terminal" '.ok == true and .outcome == "success"' "$evidence_dir/devapp-event-subscribe-raw.json"
  assert_jq "devapp event raw subscribe exact replay" '.ok == true and .outcome == "success" and .data.verified == true' "$evidence_dir/devapp-event-subscribe-raw-exact-replay.json"
  jq -e --arg code "$event_raw" '(.data.events | length) == 1 and .data.events[0].eventCode == $code' "$evidence_dir/devapp-event-subscribe-raw-readback.json" >/dev/null || fail "devapp event raw subscribe readback"
  run_json "devapp event exact subscribe" "$evidence_dir/devapp-event-subscribe-exact.json" \
    "$dws_bin" devapp +event-subscribe --unified-app-id "$exact_app_id" --event-codes "$event_exact" --yes
  run_json "devapp event exact subscribe raw readback" "$evidence_dir/devapp-event-subscribe-exact-raw-readback.json" \
    "$dws_bin" dev app event list --unified-app-id "$exact_app_id" --keyword "$event_exact" --page-size 100
  assert_jq "devapp event exact subscribe terminal" '.ok == true and .outcome == "success" and .data.verified == true' "$evidence_dir/devapp-event-subscribe-exact.json"
  jq -e --arg code "$event_exact" '(.data.events | length) == 1 and .data.events[0].eventCode == $code' "$evidence_dir/devapp-event-subscribe-exact-raw-readback.json" >/dev/null || fail "devapp event exact subscribe raw readback"
  pass "DEVAPP +event-subscribe confirm0/exact+raw terminal-receipt/eventCode readback"

  run_json "devapp event exact known" "$evidence_dir/devapp-event-list-known-exact.json" \
    "$dws_bin" devapp +event-list --unified-app-id "$exact_app_id" --keyword "$event_exact" --page-size 100
  run_json "devapp event raw known" "$evidence_dir/devapp-event-list-known-raw.json" \
    "$dws_bin" dev app event list --unified-app-id "$exact_app_id" --keyword "$event_exact" --page-size 100
  run_json "devapp event exact zero" "$evidence_dir/devapp-event-list-zero-exact.json" \
    "$dws_bin" devapp +event-list --unified-app-id "$exact_app_id" --keyword "$nonce" --page-size 100
  run_json "devapp event raw zero" "$evidence_dir/devapp-event-list-zero-raw.json" \
    "$dws_bin" dev app event list --unified-app-id "$exact_app_id" --keyword "$nonce" --page-size 100
  jq -e --arg code "$event_exact" '(input) as $raw | (input) as $ze | (input) as $zr |
    .ok == true and .data.count == 1 and .data.events[0].eventCode == $code and
    $raw.ok == true and ($raw.data.events | length) == 1 and $raw.data.events[0].eventCode == $code and
    .meta.pagination.endpoint_exhausted == true and $raw.meta.pagination.endpoint_exhausted == true and
    $ze.ok == true and $ze.data.count == 0 and ($ze.data.events | length) == 0 and
    $zr.ok == true and ($zr.data.events | length) == 0 and
    $ze.meta.pagination.endpoint_exhausted == true and $zr.meta.pagination.endpoint_exhausted == true' \
    "$evidence_dir/devapp-event-list-known-exact.json" "$evidence_dir/devapp-event-list-known-raw.json" \
    "$evidence_dir/devapp-event-list-zero-exact.json" "$evidence_dir/devapp-event-list-zero-raw.json" >/dev/null || fail "devapp +event-list dual known/zero alignment"
  pass "DEVAPP +event-list exact+raw known-nonempty/guaranteed-zero stable-event/cursor alignment"

  run_json "devapp version exact zero before" "$evidence_dir/devapp-version-zero-exact.json" \
    "$dws_bin" devapp +version-list --unified-app-id "$exact_app_id" --page-size 100
  run_json "devapp version raw zero before" "$evidence_dir/devapp-version-zero-raw.json" \
    "$dws_bin" dev app version list --unified-app-id "$exact_app_id" --page-size 100
  assert_jq "devapp version guaranteed zero fixture" '(input) as $raw | .data.count == 0 and (.data.versions | length) == 0 and ($raw.data.items | length) == 0 and .meta.pagination.endpoint_exhausted == true and $raw.meta.pagination.endpoint_exhausted == true' "$evidence_dir/devapp-version-zero-exact.json" "$evidence_dir/devapp-version-zero-raw.json"
  run_json "devapp version raw create" "$evidence_dir/devapp-version-create-raw.json" \
    "$dws_bin" dev app version create --unified-app-id "$exact_app_id" --desc "dual-live-raw" --yes
  local raw_version_id
  raw_version_id=$(jq -er '.data.versionId | select(type == "string" and length > 0)' "$evidence_dir/devapp-version-create-raw.json") || fail "devapp raw version create stable ID"
  assert_jq "devapp raw version create terminal" '.ok == true and .outcome == "success"' "$evidence_dir/devapp-version-create-raw.json"
  run_json "devapp raw version exact readback" "$evidence_dir/devapp-version-create-raw-exact-readback.json" \
    "$dws_bin" devapp +version-get --unified-app-id "$exact_app_id" --version-id "$raw_version_id"
  jq -e --arg app "$exact_app_id" --arg version "$raw_version_id" '.ok == true and .data.unifiedAppId == $app and .data.versionId == $version' "$evidence_dir/devapp-version-create-raw-exact-readback.json" >/dev/null || fail "devapp raw version exact readback"
  run_json "devapp version exact create" "$evidence_dir/devapp-version-create-exact.json" \
    "$dws_bin" devapp +version-create --unified-app-id "$exact_app_id" --desc "dual-live-exact" --yes
  local exact_version_id
  exact_version_id=$(jq -er '.data.resource.versionId | select(type == "string" and length > 0)' "$evidence_dir/devapp-version-create-exact.json") || fail "devapp exact version create stable ID"
  assert_jq "devapp exact version create terminal" '.ok == true and .outcome == "success" and .data.verified == true' "$evidence_dir/devapp-version-create-exact.json"
  run_json "devapp exact version raw readback" "$evidence_dir/devapp-version-create-exact-raw-readback.json" \
    "$dws_bin" dev app version get --unified-app-id "$exact_app_id" --version-id "$exact_version_id"
  jq -e --arg app "$exact_app_id" --arg version "$exact_version_id" '.ok == true and .data.unifiedAppId == $app and .data.versionId == $version' "$evidence_dir/devapp-version-create-exact-raw-readback.json" >/dev/null || fail "devapp exact version raw readback"
  pass "DEVAPP +version-create confirm0/exact+raw terminal-receipt/app+version-ID readback"

  local version_aligned=false
  for _ in 1 2 3 4 5; do
    run_json "devapp version exact known" "$evidence_dir/devapp-version-known-exact.json" \
      "$dws_bin" devapp +version-list --unified-app-id "$exact_app_id" --page-size 100
    run_json "devapp version raw known" "$evidence_dir/devapp-version-known-raw.json" \
      "$dws_bin" dev app version list --unified-app-id "$exact_app_id" --page-size 100
    if jq -e --arg raw_version "$raw_version_id" --arg exact_version "$exact_version_id" '
      (input) as $raw |
      .data.count >= 1 and
      ([.data.versions[].versionId] | index($raw_version)) != null and
      ([.data.versions[].versionId] | index($exact_version)) != null and
      ([.data.versions[].versionId] | sort) == ([$raw.data.items[].versionId] | sort) and
      .meta.pagination.endpoint_exhausted == true and $raw.meta.pagination.endpoint_exhausted == true
    ' "$evidence_dir/devapp-version-known-exact.json" "$evidence_dir/devapp-version-known-raw.json" >/dev/null 2>&1; then
      version_aligned=true
      break
    fi
    sleep 2
  done
  if [[ $version_aligned != true ]]; then
    jq -n -c --arg raw_version "$raw_version_id" --arg exact_version "$exact_version_id" \
      --slurpfile exact "$evidence_dir/devapp-version-known-exact.json" \
      --slurpfile raw "$evidence_dir/devapp-version-known-raw.json" '{
        diagnostic:"DEVAPP-VERSION-LIST",
        exact_count:$exact[0].data.count,
        exact_items:($exact[0].data.versions|length),
        raw_items:($raw[0].data.items|length),
        ids_equal:(([$exact[0].data.versions[].versionId]|sort)==([$raw[0].data.items[].versionId]|sort)),
        raw_version_visible:(([$exact[0].data.versions[].versionId]|index($raw_version))!=null),
        exact_version_visible:(([$exact[0].data.versions[].versionId]|index($exact_version))!=null),
        exact_pagination_type:($exact[0].meta.pagination|type),
        raw_pagination_type:($raw[0].meta.pagination|type),
        exact_exhausted:$exact[0].meta.pagination.endpoint_exhausted,
        raw_exhausted:$raw[0].meta.pagination.endpoint_exhausted
      }' >&2
    fail "devapp version known alignment"
  fi
  pass "DEVAPP +version-list exact+raw known-nonempty/empty-fixture-zero stable-version/cursor alignment"

  run_json "devapp version exact get" "$evidence_dir/devapp-version-get-exact.json" \
    "$dws_bin" devapp +version-get --unified-app-id "$exact_app_id" --version-id "$exact_version_id"
  run_json "devapp version raw get" "$evidence_dir/devapp-version-get-raw.json" \
    "$dws_bin" dev app version get --unified-app-id "$exact_app_id" --version-id "$exact_version_id"
  jq -e --arg app "$exact_app_id" --arg version "$exact_version_id" '(input) as $raw | .data.unifiedAppId == $app and .data.versionId == $version and $raw.data.unifiedAppId == $app and $raw.data.versionId == $version' "$evidence_dir/devapp-version-get-exact.json" "$evidence_dir/devapp-version-get-raw.json" >/dev/null || fail "devapp +version-get dual object"
  pass "DEVAPP +version-get exact+raw stable-app+version-ID object alignment"

  run_json "devapp version exact approval" "$evidence_dir/devapp-version-approval-exact.json" \
    "$dws_bin" devapp +version-check-approval --unified-app-id "$exact_app_id" --version-id "$exact_version_id"
  run_json "devapp version raw approval" "$evidence_dir/devapp-version-approval-raw.json" \
    "$dws_bin" dev app version check-approval --unified-app-id "$exact_app_id" --version-id "$exact_version_id"
  jq -e --arg app "$exact_app_id" --arg version "$exact_version_id" '(input) as $raw | (.outcome == "success" or .outcome == "pending") and .data.unifiedAppId == $app and .data.versionId == $version and ($raw.outcome == "success" or $raw.outcome == "pending") and $raw.data.unifiedAppId == $app and $raw.data.versionId == $version' "$evidence_dir/devapp-version-approval-exact.json" "$evidence_dir/devapp-version-approval-raw.json" >/dev/null || fail "devapp +version-check-approval dual object"
  pass "DEVAPP +version-check-approval exact+raw stable-app+version-ID pending/terminal alignment"

  run_json "devapp version exact status" "$evidence_dir/devapp-version-status-exact.json" \
    "$dws_bin" devapp +version-status --unified-app-id "$exact_app_id" --version-id "$exact_version_id"
  run_json "devapp version raw status" "$evidence_dir/devapp-version-status-raw.json" \
    "$dws_bin" dev app version status --unified-app-id "$exact_app_id" --version-id "$exact_version_id"
  jq -e --arg app "$exact_app_id" --arg version "$exact_version_id" '(input) as $raw | (.outcome == "success" or .outcome == "pending") and .data.unifiedAppId == $app and .data.versionId == $version and ($raw.outcome == "success" or $raw.outcome == "pending") and $raw.data.unifiedAppId == $app and $raw.data.versionId == $version' "$evidence_dir/devapp-version-status-exact.json" "$evidence_dir/devapp-version-status-raw.json" >/dev/null || fail "devapp +version-status dual object"
  pass "DEVAPP +version-status exact+raw stable-app+version-ID status alignment"

  run_json "devapp raw delete" "$evidence_dir/devapp-delete-raw.json" \
    "$dws_bin" dev app delete --unified-app-id "$raw_app_id" --confirm-name "$raw_app_name" --yes
  assert_jq "devapp raw delete terminal" '.ok == true and .outcome == "success"' "$evidence_dir/devapp-delete-raw.json"
  run_json "devapp raw delete exact absence" "$evidence_dir/devapp-delete-raw-exact-absence.json" \
    "$dws_bin" devapp +list --name "$raw_app_name" --page-size 20
  run_json "devapp raw delete raw absence" "$evidence_dir/devapp-delete-raw-raw-absence.json" \
    "$dws_bin" dev app list --name "$raw_app_name" --page-size 20
  assert_jq "devapp raw delete dual absence" '(input) as $raw | .data.count == 0 and (.data.apps | length) == 0 and ($raw.data.items | length) == 0' "$evidence_dir/devapp-delete-raw-exact-absence.json" "$evidence_dir/devapp-delete-raw-raw-absence.json"
  raw_app_id=""

  run_json "devapp exact delete" "$evidence_dir/devapp-delete-exact.json" \
    "$dws_bin" devapp +delete --unified-app-id "$exact_app_id" --yes
  assert_jq "devapp exact delete terminal" '.ok == true and .outcome == "success" and .data.verified == true and .data.action == "delete"' "$evidence_dir/devapp-delete-exact.json"
  run_json "devapp exact delete exact absence" "$evidence_dir/devapp-delete-exact-exact-absence.json" \
    "$dws_bin" devapp +list --name "$exact_updated_name" --page-size 20
  run_json "devapp exact delete raw absence" "$evidence_dir/devapp-delete-exact-raw-absence.json" \
    "$dws_bin" dev app list --name "$exact_updated_name" --page-size 20
  assert_jq "devapp exact delete dual absence" '(input) as $raw | .data.count == 0 and (.data.apps | length) == 0 and ($raw.data.items | length) == 0' "$evidence_dir/devapp-delete-exact-exact-absence.json" "$evidence_dir/devapp-delete-exact-raw-absence.json"
  exact_app_id=""
  pass "DEVAPP +delete confirm0/exact+raw terminal-receipt/dual absence cleanup"

  pass "DEVAPP-INVENTORY public=25 dual-layer-pass=25 unavailable=5 cleanup=zero-residual"
}

run_agoal_gate() {
  assert_public_surface agoal "+contract-fields
+obj-template-list
+report-statistics-list
+report-submit-detail
+user-rules"
  local nonce="DWS-GUARANTEED-ZERO-8F3D2C4A"
  local future_date="2099-12-31"

  run_json "agoal report statistics exact known" "$evidence_dir/agoal-report-known-exact.json" \
    "$dws_bin" agoal +report-statistics-list
  run_json "agoal report statistics raw known" "$evidence_dir/agoal-report-known-raw.json" \
    "$dws_bin" agoal report list-statistics
  assert_jq "agoal report statistics known alignment" '
    (input) as $raw |
    .ok == true and .outcome == "success" and .data.count > 0 and
    $raw.success == true and ($raw.content | type) == "array" and
    (.data.count == ($raw.content | length)) and
    ([.data.statistics[].templateId] | sort) == ([$raw.content[].templateId] | sort)
  ' "$evidence_dir/agoal-report-known-exact.json" "$evidence_dir/agoal-report-known-raw.json"
  run_json "agoal report statistics exact zero" "$evidence_dir/agoal-report-zero-exact.json" \
    "$dws_bin" agoal +report-statistics-list --keyword "$nonce"
  run_json "agoal report statistics raw zero" "$evidence_dir/agoal-report-zero-raw.json" \
    "$dws_bin" agoal report list-statistics --keyword "$nonce"
  assert_jq "agoal report statistics zero alignment" '
    (input) as $raw |
    .ok == true and .outcome == "success" and .data.count == 0 and
    (.data.statistics | type) == "array" and (.data.statistics | length) == 0 and
    $raw.success == true and ($raw.content | type) == "array" and ($raw.content | length) == 0
  ' "$evidence_dir/agoal-report-zero-exact.json" "$evidence_dir/agoal-report-zero-raw.json"
  pass "AGOAL +report-statistics-list exact+raw known-nonempty/guaranteed-zero stable-template alignment"

  run_json "agoal template exact known" "$evidence_dir/agoal-template-known-exact.json" \
    "$dws_bin" agoal +obj-template-list --page 1 --page-size 20
  run_json "agoal template raw known" "$evidence_dir/agoal-template-known-raw.json" \
    "$dws_bin" agoal obj-template list --page 1 --page-size 20
  assert_jq "agoal template known alignment" '
    (input) as $raw |
    .ok == true and .outcome == "success" and .data.count > 0 and
    $raw.success == true and ($raw.content.result | type) == "array" and
    .data.page == $raw.content.page and .data.pageSize == $raw.content.pageSize and
    .data.totalCount == $raw.content.totalCount and
    ([.data.templates[] | (.id // .templateId)] | sort) ==
      ([$raw.content.result[] | (.id // .templateId)] | sort)
  ' "$evidence_dir/agoal-template-known-exact.json" "$evidence_dir/agoal-template-known-raw.json"
  run_json "agoal template exact zero" "$evidence_dir/agoal-template-zero-exact.json" \
    "$dws_bin" agoal +obj-template-list --keyword "$nonce" --page 1 --page-size 20
  run_json "agoal template raw zero" "$evidence_dir/agoal-template-zero-raw.json" \
    "$dws_bin" agoal obj-template list --keyword "$nonce" --page 1 --page-size 20
  assert_jq "agoal template zero alignment" '
    (input) as $raw |
    .ok == true and .outcome == "success" and .data.count == 0 and .data.totalCount == 0 and
    (.data.templates | type) == "array" and (.data.templates | length) == 0 and
    $raw.success == true and ($raw.content.result | type) == "array" and
    ($raw.content.result | length) == 0 and $raw.content.totalCount == 0 and
    .data.page == $raw.content.page and .data.pageSize == $raw.content.pageSize
  ' "$evidence_dir/agoal-template-zero-exact.json" "$evidence_dir/agoal-template-zero-raw.json"
  pass "AGOAL +obj-template-list exact+raw known-nonempty/guaranteed-zero stable-ID/page alignment"

  run_json "agoal contract fields raw" "$evidence_dir/agoal-fields-raw.json" \
    "$dws_bin" agoal contract fields
  local field_id
  field_id=$(jq -er '.content[0].id | select(type == "string" and length > 0)' "$evidence_dir/agoal-fields-raw.json") || fail "agoal contract fields raw fixture"
  run_json "agoal contract fields exact known" "$evidence_dir/agoal-fields-known-exact.json" \
    "$dws_bin" agoal +contract-fields --keyword "$field_id"
  run_json "agoal contract fields exact zero" "$evidence_dir/agoal-fields-zero-exact.json" \
    "$dws_bin" agoal +contract-fields --keyword "$nonce"
  jq -e --arg field_id "$field_id" --arg nonce "$nonce" '
    (input) as $zero | (input) as $raw |
    .ok == true and .outcome == "success" and .data.count == 1 and
    .data.fields[0].id == $field_id and
    $zero.ok == true and $zero.outcome == "success" and $zero.data.count == 0 and
    ($zero.data.fields | length) == 0 and
    $raw.success == true and ($raw.content | type) == "array" and ($raw.content | length) > 0 and
    ([$raw.content[].id] | index($field_id)) != null and
    ([$raw.content[] | [.id,.code,.title,.category,.type] | map(select(type == "string")) | join(" ") | ascii_downcase] |
      map(contains($nonce | ascii_downcase)) | any) == false
  ' "$evidence_dir/agoal-fields-known-exact.json" "$evidence_dir/agoal-fields-zero-exact.json" "$evidence_dir/agoal-fields-raw.json" >/dev/null 2>&1 || fail "agoal contract fields adapter alignment"
  pass "AGOAL +contract-fields exact local-filter+raw full-set known/guaranteed-zero stable-ID alignment"

  run_json "agoal user rules raw" "$evidence_dir/agoal-rules-raw.json" \
    "$dws_bin" agoal user rules
  local rule_id
  rule_id=$(jq -er '.content.rules[0].id | select(type == "string" and length > 0)' "$evidence_dir/agoal-rules-raw.json") || fail "agoal user rules raw fixture"
  run_json "agoal user rules exact known" "$evidence_dir/agoal-rules-known-exact.json" \
    "$dws_bin" agoal +user-rules --rule-id "$rule_id"
  run_json "agoal user rules exact zero" "$evidence_dir/agoal-rules-zero-exact.json" \
    "$dws_bin" agoal +user-rules --rule-id "$nonce"
  jq -e --arg rule_id "$rule_id" --arg nonce "$nonce" '
    (input) as $zero | (input) as $raw |
    .ok == true and .outcome == "success" and .data.count == 1 and .data.rules[0].id == $rule_id and
    $zero.ok == true and $zero.outcome == "success" and $zero.data.count == 0 and
    ($zero.data.rules | length) == 0 and
    $raw.success == true and ($raw.content.rules | type) == "array" and ($raw.content.rules | length) > 0 and
    ([$raw.content.rules[].id] | index($rule_id)) != null and
    ([$raw.content.rules[].id] | index($nonce)) == null
  ' "$evidence_dir/agoal-rules-known-exact.json" "$evidence_dir/agoal-rules-zero-exact.json" "$evidence_dir/agoal-rules-raw.json" >/dev/null 2>&1 || fail "agoal user rules adapter alignment"
  pass "AGOAL +user-rules exact local-filter+raw full-set known/guaranteed-zero stable-rule alignment"

  local template_id=""
  local submit_state=""
  while IFS= read -r candidate_template; do
    for candidate_state in ON_TIME LATE NOT_SUBMITTED; do
      run_json "agoal submission raw fixture scan" "$evidence_dir/agoal-submit-scan.json" \
        "$dws_bin" agoal report submit-detail --template-id "$candidate_template" --submit-state "$candidate_state" --page 1 --page-size 20 --timeout 60
      if jq -e '.success == true and (.content.result | type) == "array" and (.content.result | length) > 0' "$evidence_dir/agoal-submit-scan.json" >/dev/null; then
        template_id=$candidate_template
        submit_state=$candidate_state
        cp "$evidence_dir/agoal-submit-scan.json" "$evidence_dir/agoal-submit-known-raw.json"
        break 2
      fi
    done
  done < <(jq -r '.content[].templateId' "$evidence_dir/agoal-report-known-raw.json")
  [[ -n $template_id && -n $submit_state ]] || fail "agoal submission known-nonempty fixture"
  run_json "agoal submission exact known" "$evidence_dir/agoal-submit-known-exact.json" \
    "$dws_bin" agoal +report-submit-detail --template-id "$template_id" --submit-state "$submit_state" --page 1 --page-size 20 --timeout 60
  assert_jq "agoal submission known alignment" '
    (input) as $raw |
    def stable_id: (.reportId // .user.dingUserId // .user.id);
    .ok == true and .outcome == "success" and .data.count > 0 and
    .data.page == $raw.content.page and .data.pageSize == $raw.content.pageSize and
    .data.totalCount == $raw.content.totalCount and
    ([.data.submissions[] | stable_id] | sort) == ([$raw.content.result[] | stable_id] | sort)
  ' "$evidence_dir/agoal-submit-known-exact.json" "$evidence_dir/agoal-submit-known-raw.json"
  run_json "agoal submission exact guaranteed zero" "$evidence_dir/agoal-submit-zero-exact.json" \
    "$dws_bin" agoal +report-submit-detail --template-id "$template_id" --submit-state ON_TIME --query-date "$future_date" --page 1 --page-size 20 --timeout 60
  run_json "agoal submission raw guaranteed zero" "$evidence_dir/agoal-submit-zero-raw.json" \
    "$dws_bin" agoal report submit-detail --template-id "$template_id" --submit-state ON_TIME --query-date "$future_date" --page 1 --page-size 20 --timeout 60
  assert_jq "agoal submission guaranteed zero alignment" '
    (input) as $raw |
    .ok == true and .outcome == "success" and .data.count == 0 and .data.totalCount == 0 and
    (.data.submissions | type) == "array" and (.data.submissions | length) == 0 and
    $raw.success == true and ($raw.content.result | type) == "array" and
    ($raw.content.result | length) == 0 and $raw.content.totalCount == 0 and
    .data.page == $raw.content.page and .data.pageSize == $raw.content.pageSize
  ' "$evidence_dir/agoal-submit-zero-exact.json" "$evidence_dir/agoal-submit-zero-raw.json"
  pass "AGOAL +report-submit-detail exact+raw known-nonempty/future guaranteed-zero stable-user/page alignment"

  pass "AGOAL-INVENTORY public=5 dual-layer-pass=5 unavailable=11"
}

if [[ $selected_product == all || $selected_product == dev ]]; then
  run_dev_gate
fi
if [[ $selected_product == all || $selected_product == devapp ]]; then
  # Added below after the Agoal proof block so product execution remains
  # Dev -> DevApp -> Agoal when `all` is selected.
  run_devapp_gate
fi
if [[ $selected_product == all || $selected_product == agoal ]]; then
  run_agoal_gate
fi
