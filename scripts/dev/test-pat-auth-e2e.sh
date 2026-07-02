#!/usr/bin/env bash
# Isolated PAT/login authorization regression matrix for CI and release gates.
#
# The script never reads the user's real ~/.dws or system keychain. CI can
# provide the same test-account metadata used by the original CI/CD smoke via
# DWS_CI_TEST_* env vars; when they are absent, deterministic fake values keep
# the contract checks runnable for forks and local development.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RUN_GO_TESTS=1
VERBOSE=0
KEEP_WORKDIR=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --skip-go-tests)
      RUN_GO_TESTS=0
      shift
      ;;
    --verbose)
      VERBOSE=1
      shift
      ;;
    --keep-workdir)
      KEEP_WORKDIR=1
      shift
      ;;
    -h|--help)
      sed -n '1,14p' "$0"
      exit 0
      ;;
    *)
      echo "unknown option: $1" >&2
      exit 2
      ;;
  esac
done

mkdir -p "$ROOT/.tmp-bin"
WORKDIR="$(mktemp -d "$ROOT/.tmp-bin/pat-auth-e2e.XXXXXX")"
BIN="$WORKDIR/bin/dws"
HELPER_DIR="$WORKDIR/helper"
HELPER_BIN="$WORKDIR/bin/pat-auth-e2e-helper"
CONFIG_DIR="$WORKDIR/config"
KEYCHAIN_DIR="$WORKDIR/keychain"
CACHE_DIR="$WORKDIR/cache"
OUT_DIR="$WORKDIR/out"

cleanup() {
  if [[ "$KEEP_WORKDIR" -eq 1 ]]; then
    echo "[INFO] kept workdir: $WORKDIR"
  else
    rm -rf "$WORKDIR"
  fi
}
trap cleanup EXIT

export DWS_CONFIG_DIR="$CONFIG_DIR"
export DWS_KEYCHAIN_DIR="$KEYCHAIN_DIR"
export DWS_DISABLE_KEYCHAIN=1
export DWS_CACHE_DIR="$CACHE_DIR"
export DWS_PERF_REPORT=
export DWS_PERF_DEBUG=

unset DINGTALK_DWS_AGENTCODE
unset DWS_DINGTALK_AGENTCODE
unset DINGTALK_AGENT
unset DINGTALK_SESSION_ID
unset DWS_SESSION_ID
unset REWIND_SESSION_ID

TEST_CORP_ID="${DWS_CI_TEST_CORP_ID:-${DWS_PAT_E2E_CORP_ID:-ci-corp-open}}"
TEST_CORP_NAME="${DWS_CI_TEST_CORP_NAME:-${DWS_PAT_E2E_CORP_NAME:-DWS CI Test Org}}"
TEST_USER_ID="${DWS_CI_TEST_USER_ID:-${DWS_PAT_E2E_USER_ID:-ci-user-open}}"
TEST_USER_NAME="${DWS_CI_TEST_USER_NAME:-${DWS_PAT_E2E_USER_NAME:-DWS CI Bot}}"
TEST_ACCESS_TOKEN="${DWS_CI_TEST_ACCESS_TOKEN:-${DWS_PAT_E2E_ACCESS_TOKEN:-ci-access-token}}"
TEST_REFRESH_TOKEN="${DWS_CI_TEST_REFRESH_TOKEN:-${DWS_PAT_E2E_REFRESH_TOKEN:-ci-refresh-token}}"

mkdir -p "$HELPER_DIR" "$CONFIG_DIR" "$KEYCHAIN_DIR" "$CACHE_DIR" "$OUT_DIR" "$(dirname "$BIN")"

log() {
  printf '\n==> %s\n' "$*"
}

fail() {
  echo "[FAIL] $*" >&2
  exit 1
}

run() {
  if [[ "$VERBOSE" -eq 1 ]]; then
    "$@"
  else
    "$@" >/dev/null
  fi
}

capture() {
  local file="$1"
  shift
  if [[ "$VERBOSE" -eq 1 ]]; then
    echo "+ $*" >&2
  fi
  "$@" >"$file" 2>"$file.stderr"
}

expect_contains() {
  local file="$1"
  local needle="$2"
  if ! grep -F -- "$needle" "$file" >/dev/null; then
    echo "----- $file -----" >&2
    cat "$file" >&2
    fail "expected $file to contain: $needle"
  fi
}

expect_fail() {
  local needle="$1"
  shift
  local output
  set +e
  output="$("$@" 2>&1)"
  local code=$?
  set -e
  if [[ "$code" -eq 0 ]]; then
    echo "$output" >&2
    fail "expected command to fail: $*"
  fi
  if ! grep -F -- "$needle" <<<"$output" >/dev/null; then
    echo "$output" >&2
    fail "expected failure output to contain: $needle"
  fi
}

assert_no_secret_leak() {
  local secret="$1"
  local label="$2"
  if [[ -z "$secret" ]]; then
    return
  fi
  if grep -R -q -- "$secret" "$OUT_DIR"; then
    fail "$label leaked into captured CLI output"
  fi
}

cat >"$HELPER_DIR/main.go" <<'GOEOF'
package main

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"os"
	"strings"
	"time"

	auth "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pat"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type authStatusResponse struct {
	Success           bool   `json:"success"`
	Authenticated     bool   `json:"authenticated"`
	TokenValid        bool   `json:"token_valid"`
	RefreshTokenValid bool   `json:"refresh_token_valid"`
	CorpID            string `json:"corp_id"`
	CorpName          string `json:"corp_name"`
	UserID            string `json:"user_id"`
	UserName          string `json:"user_name"`
}

type authLoginResponse struct {
	Success           bool   `json:"success"`
	TokenValid        bool   `json:"token_valid"`
	RefreshTokenValid bool   `json:"refresh_token_valid"`
	CorpID            string `json:"corp_id"`
	UserID            string `json:"user_id"`
}

type profileListResponse struct {
	Success        bool          `json:"success"`
	PrimaryProfile string        `json:"primaryProfile"`
	CurrentProfile string        `json:"currentProfile"`
	Profiles       []profileView `json:"profiles"`
}

type profileView struct {
	CorpID    string `json:"corpId"`
	CorpName  string `json:"corpName"`
	UserID    string `json:"userId"`
	UserName  string `json:"userName"`
	Status    string `json:"status"`
	IsPrimary bool   `json:"isPrimary"`
	IsCurrent bool   `json:"isCurrent"`
}

type recordedToolCall struct {
	tool string
	args map[string]any
}

type sequenceToolCaller struct {
	calls     []recordedToolCall
	responses []string
}

func (s *sequenceToolCaller) CallTool(_ context.Context, _ string, toolName string, args map[string]any) (*edition.ToolResult, error) {
	copied := make(map[string]any, len(args))
	for k, v := range args {
		copied[k] = v
	}
	s.calls = append(s.calls, recordedToolCall{tool: toolName, args: copied})
	response := `{"success":true,"data":{}}`
	if len(s.responses) >= len(s.calls) {
		response = s.responses[len(s.calls)-1]
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: response}}}, nil
}

func (s *sequenceToolCaller) Format() string { return "json" }
func (s *sequenceToolCaller) DryRun() bool   { return false }

func main() {
	if len(os.Args) < 2 {
		die("missing helper command")
	}
	switch os.Args[1] {
	case "seed":
		requireArgs(8)
		seedToken(os.Args[2], os.Args[3], os.Args[4], os.Args[5], os.Args[6], os.Args[7])
	case "assert-auth-status":
		requireArgs(6)
		assertAuthStatus(os.Args[2], parseBool(os.Args[3]), os.Args[4], os.Args[5])
	case "assert-login-json":
		requireArgs(6)
		assertLoginJSON(os.Args[2], parseBool(os.Args[3]), os.Args[4], os.Args[5])
	case "assert-profile-list":
		requireArgs(5)
		assertProfileList(os.Args[2], os.Args[3], os.Args[4])
	case "assert-empty-auth":
		requireArgs(2)
		assertEmptyAuth()
	case "assert-auth-error-contract":
		requireArgs(2)
		assertAuthErrorContract()
	case "assert-pat-contract":
		requireArgs(3)
		assertPATContract(os.Args[2])
	case "assert-login-recommend":
		requireArgs(2)
		assertLoginRecommend()
	default:
		die("unknown helper command %q", os.Args[1])
	}
}

func seedToken(corpID, corpName, userID, userName, accessToken, refreshToken string) {
	configDir := configDir()
	data := &auth.TokenData{
		AccessToken:    accessToken,
		RefreshToken:   refreshToken,
		PersistentCode: "persistent-" + corpID,
		ExpiresAt:      time.Now().Add(2 * time.Hour),
		RefreshExpAt:   time.Now().Add(30 * 24 * time.Hour),
		CorpID:         corpID,
		CorpName:       corpName,
		UserID:         userID,
		UserName:       userName,
		Source:         "pat-auth-e2e",
	}
	must(auth.SaveTokenData(configDir, data))
}

func assertAuthStatus(path string, wantAuthenticated bool, wantCorpID, wantUserID string) {
	var resp authStatusResponse
	readJSON(path, &resp)
	if !resp.Success {
		die("auth status success=false: %#v", resp)
	}
	if resp.Authenticated != wantAuthenticated {
		die("auth status authenticated=%v, want %v: %#v", resp.Authenticated, wantAuthenticated, resp)
	}
	if !wantAuthenticated {
		return
	}
	if !resp.TokenValid {
		die("auth status token_valid=false: %#v", resp)
	}
	if wantCorpID != "" && resp.CorpID != wantCorpID {
		die("auth status corp_id=%q, want %q", resp.CorpID, wantCorpID)
	}
	if wantUserID != "" && resp.UserID != wantUserID {
		die("auth status user_id=%q, want %q", resp.UserID, wantUserID)
	}
}

func assertLoginJSON(path string, wantTokenValid bool, wantCorpID, wantUserID string) {
	var resp authLoginResponse
	readJSON(path, &resp)
	if !resp.Success {
		die("auth login success=false: %#v", resp)
	}
	if resp.TokenValid != wantTokenValid {
		die("auth login token_valid=%v, want %v: %#v", resp.TokenValid, wantTokenValid, resp)
	}
	if wantCorpID != "" && resp.CorpID != wantCorpID {
		die("auth login corp_id=%q, want %q", resp.CorpID, wantCorpID)
	}
	if wantUserID != "" && resp.UserID != wantUserID {
		die("auth login user_id=%q, want %q", resp.UserID, wantUserID)
	}
}

func assertProfileList(path, wantCorpID, wantUserID string) {
	var resp profileListResponse
	readJSON(path, &resp)
	if !resp.Success {
		die("profile list success=false: %#v", resp)
	}
	if len(resp.Profiles) != 1 {
		die("profile count=%d, want 1: %#v", len(resp.Profiles), resp.Profiles)
	}
	p := resp.Profiles[0]
	if p.CorpID != wantCorpID || p.UserID != wantUserID {
		die("profile identity=(%q,%q), want (%q,%q)", p.CorpID, p.UserID, wantCorpID, wantUserID)
	}
	if resp.PrimaryProfile != wantCorpID || resp.CurrentProfile != wantCorpID || !p.IsPrimary || !p.IsCurrent {
		die("profile primary/current mismatch: %#v item=%#v", resp, p)
	}
	if p.Status != auth.ProfileStatusActive {
		die("profile status=%q, want active", p.Status)
	}
}

func assertEmptyAuth() {
	cfg, err := auth.LoadProfiles(configDir())
	must(err)
	if cfg.PrimaryProfile != "" || cfg.CurrentProfile != "" || cfg.PreviousProfile != "" || len(cfg.Profiles) != 0 {
		die("expected empty profiles after reset, got %#v", cfg)
	}
	if auth.TokenDataExistsKeychain() {
		die("legacy auth-token still exists after reset")
	}
}

func assertAuthErrorContract() {
	assertStructuredAuth(`{"error":"Missing service_id or access_key in request headers"}`, "not_configured")
	assertStructuredAuth(`{"success":false,"code":"DWS_SERVICE_UNAUTHORIZED","message":"expired"}`, "gateway_auth_expired")
}

func assertStructuredAuth(raw, wantReason string) {
	err := apperrors.ClassifyMCPResponseText(raw)
	if err == nil {
		die("ClassifyMCPResponseText(%s) returned nil", raw)
	}
	var typed *apperrors.Error
	if !stderrors.As(err, &typed) {
		die("ClassifyMCPResponseText(%s) returned %T, want *errors.Error", raw, err)
	}
	if typed.Category != apperrors.CategoryAuth || typed.Reason != wantReason {
		die("auth error=(%s,%s), want (%s,%s)", typed.Category, typed.Reason, apperrors.CategoryAuth, wantReason)
	}
	var patErr *apperrors.PATError
	if stderrors.As(err, &patErr) {
		die("auth error %s must not be classified as PATError", raw)
	}
}

func assertPATContract(mode string) {
	hostMode := mode == "host"
	if !hostMode && mode != "cli" {
		die("mode must be cli or host, got %q", mode)
	}
	_ = os.Unsetenv(auth.AgentCodeEnv)
	if hostMode {
		_ = os.Setenv(auth.AgentCodeEnv, "agt-ci-host")
	}
	apperrors.SetHostControlProvider(func() string {
		if auth.HostOwnsPATFlow() {
			return "openClaw"
		}
		return ""
	})
	apperrors.SetPATOpenBrowserProvider(func() bool { return false })

	if auth.HostOwnsPATFlow() != hostMode {
		die("HostOwnsPATFlow()=%v, want %v", auth.HostOwnsPATFlow(), hostMode)
	}

	rawLegacyURI := "https://open-dev.dingtalk.com/fe/old#%2FpersonalAuthorization%3FflowId%3Dflow-ci%26userCode%3D123456"
	cases := []struct {
		code       string
		body       map[string]any
		checkField string
	}{
		{
			code: "PAT_NO_PERMISSION",
			body: map[string]any{"success": false, "code": "PAT_NO_PERMISSION", "data": map[string]any{
				"requiredScopes": []any{"aitable.record:read"},
				"displayName":    "read records",
			}},
			checkField: "requiredScopes",
		},
		{
			code: "PAT_LOW_RISK_NO_PERMISSION",
			body: map[string]any{"success": false, "code": "PAT_LOW_RISK_NO_PERMISSION", "data": map[string]any{
				"requiredScopes": []any{"contact.user:read"},
			}},
			checkField: "requiredScopes",
		},
		{
			code: "PAT_MEDIUM_RISK_NO_PERMISSION",
			body: map[string]any{"success": false, "code": "PAT_MEDIUM_RISK_NO_PERMISSION", "data": map[string]any{
				"requiredScopes": []any{"chat.message:send"},
			}},
			checkField: "requiredScopes",
		},
		{
			code: "PAT_HIGH_RISK_NO_PERMISSION",
			body: map[string]any{"success": false, "code": "PAT_HIGH_RISK_NO_PERMISSION", "data": map[string]any{
				"requiredScopes": []any{"finance.invoice:write"},
				"authRequestId":  "auth-request-ci",
			}},
			checkField: "requiredScopes",
		},
		{
			code: "PAT_SCOPE_AUTH_REQUIRED",
			body: map[string]any{"success": false, "code": "PAT_SCOPE_AUTH_REQUIRED", "data": map[string]any{
				"missingScope": "Contact.User.Read",
			}},
			checkField: "missingScope",
		},
		{
			code: "PAT_BATCH_AUTH_PENDING",
			body: map[string]any{"success": false, "code": "PAT_BATCH_AUTH_PENDING", "data": map[string]any{
				"flowId":    "flow-ci",
				"uri":       rawLegacyURI,
				"callbacks": map[string]any{"poll": "legacy-host-callback"},
			}},
			checkField: "uri",
		},
		{
			code: "AGENT_CODE_NOT_EXISTS",
			body: map[string]any{"success": false, "code": "AGENT_CODE_NOT_EXISTS", "data": map[string]any{
				"agentCode": "agt-missing",
			}},
			checkField: "agentCode",
		},
	}

	for _, tc := range cases {
		raw, err := json.Marshal(tc.body)
		must(err)
		classified := apperrors.ClassifyMCPResponseText(string(raw))
		var patErr *apperrors.PATError
		if !stderrors.As(classified, &patErr) {
			die("%s classified as %T, want PATError", tc.code, classified)
		}
		if patErr.ExitCode() != apperrors.ExitCodePermission {
			die("%s exit=%d, want %d", tc.code, patErr.ExitCode(), apperrors.ExitCodePermission)
		}
		if strings.ContainsAny(patErr.RawStderr(), "\n\r") {
			die("%s RawStderr is not single-line: %q", tc.code, patErr.RawStderr())
		}
		var parsed map[string]any
		if err := json.Unmarshal([]byte(patErr.RawStderr()), &parsed); err != nil {
			die("%s RawStderr is not JSON: %v raw=%s", tc.code, err, patErr.RawStderr())
		}
		if success, _ := parsed["success"].(bool); success {
			die("%s success=true, want false", tc.code)
		}
		if code, _ := parsed["code"].(string); code != tc.code {
			die("code=%q, want %q", code, tc.code)
		}
		data, _ := parsed["data"].(map[string]any)
		if data == nil {
			die("%s data missing", tc.code)
		}
		if _, ok := data[tc.checkField]; !ok {
			die("%s data.%s missing: %#v", tc.code, tc.checkField, data)
		}
		if openBrowser, ok := data["openBrowser"].(bool); !ok || openBrowser {
			die("%s openBrowser=%#v, want false", tc.code, data["openBrowser"])
		}
		if tc.code == "PAT_BATCH_AUTH_PENDING" {
			uri, _ := data["uri"].(string)
			if !strings.Contains(uri, "hash=") || !strings.Contains(uri, "flow-ci") {
				die("normalized PAT uri missing hash/flowId: %s", uri)
			}
		}
		hostControl, hasHostControl := data["hostControl"].(map[string]any)
		if hostMode {
			if !hasHostControl {
				die("%s hostControl missing in host mode: %#v", tc.code, data)
			}
			if hostControl["mode"] != "host" || hostControl["clawType"] != "openClaw" {
				die("%s bad hostControl: %#v", tc.code, hostControl)
			}
			if _, ok := data["callbacks"]; ok {
				die("%s legacy callbacks must be stripped in host mode", tc.code)
			}
		} else if hasHostControl {
			die("%s hostControl present in CLI-owned mode: %#v", tc.code, hostControl)
		}
	}
}

func assertLoginRecommend() {
	alreadyGranted := &sequenceToolCaller{responses: []string{
		`{"success":true,"data":{"allGranted":true,"selectedScopes":[]}}`,
	}}
	var out strings.Builder
	if err := pat.RunLoginRecommendAuthorizationWithOptions(context.Background(), alreadyGranted, &out, pat.LoginRecommendOptions{Confirmed: true}); err != nil {
		die("already-granted recommend error: %v", err)
	}
	if len(alreadyGranted.calls) != 1 || alreadyGranted.calls[0].tool != "pat.batch_plan" {
		die("already-granted calls=%#v, want one pat.batch_plan", alreadyGranted.calls)
	}
	if !strings.Contains(out.String(), "推荐权限已全部授权或没有可授权项") {
		die("already-granted output missing skip message: %q", out.String())
	}

	unconfirmed := &sequenceToolCaller{responses: []string{
		`{"success":true,"data":{"items":[{"scope":"calendar.event:read","productCode":"calendar","productName":"calendar"}],"selectedScopes":["calendar.event:read"]}}`,
		`{"success":true,"data":{"flowId":"flow-ci","userCode":"ABCD-EFGH","uri":"https://example.com/auth"}}`,
	}}
	if err := pat.RunLoginRecommendAuthorizationWithOptions(context.Background(), unconfirmed, nil, pat.LoginRecommendOptions{}); err != nil {
		die("unconfirmed recommend error: %v", err)
	}
	if len(unconfirmed.calls) != 2 || unconfirmed.calls[0].tool != "pat.batch_plan" || unconfirmed.calls[1].tool != "pat.batch_grant" {
		die("unconfirmed calls=%#v, want plan+grant", unconfirmed.calls)
	}
	if unconfirmed.calls[1].args["startFlow"] != true || unconfirmed.calls[1].args["noWait"] != true {
		die("unconfirmed grant args missing startFlow/noWait: %#v", unconfirmed.calls[1].args)
	}
	if unconfirmed.calls[1].args["caller"] != "auth_login_recommend" {
		die("unconfirmed caller=%#v, want auth_login_recommend", unconfirmed.calls[1].args["caller"])
	}

	confirmed := &sequenceToolCaller{responses: []string{
		`{"success":true,"data":{"items":[{"scope":"calendar.event:read","productCode":"calendar","productName":"calendar"}],"selectedScopes":["calendar.event:read"]}}`,
		`{"success":true,"data":{"grantedScopes":["calendar.event:read"]}}`,
	}}
	if err := pat.RunLoginRecommendAuthorizationWithOptions(context.Background(), confirmed, nil, pat.LoginRecommendOptions{Confirmed: true}); err != nil {
		die("confirmed recommend error: %v", err)
	}
	if _, ok := confirmed.calls[1].args["startFlow"]; ok {
		die("confirmed grant must not set startFlow: %#v", confirmed.calls[1].args)
	}
	if _, ok := confirmed.calls[1].args["noWait"]; ok {
		die("confirmed grant must not set noWait: %#v", confirmed.calls[1].args)
	}
}

func readJSON(path string, v any) {
	data, err := os.ReadFile(path)
	must(err)
	if err := json.Unmarshal(data, v); err != nil {
		die("parse %s: %v\nraw=%s", path, err, string(data))
	}
}

func configDir() string {
	dir := strings.TrimSpace(os.Getenv("DWS_CONFIG_DIR"))
	if dir == "" {
		die("DWS_CONFIG_DIR is required")
	}
	return dir
}

func parseBool(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true", "1", "yes":
		return true
	case "false", "0", "no":
		return false
	default:
		die("invalid bool %q", raw)
		return false
	}
}

func requireArgs(n int) {
	if len(os.Args) != n {
		die("%s expects %d argv items, got %d: %v", os.Args[1], n, len(os.Args), os.Args)
	}
}

func must(err error) {
	if err != nil {
		die("%v", err)
	}
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
GOEOF

cd "$ROOT"

log "Build dws and PAT helper"
run gofmt -w "$HELPER_DIR/main.go"
run go build -o "$BIN" ./cmd
run go build -o "$HELPER_BIN" "$HELPER_DIR"

if [[ "$RUN_GO_TESTS" -eq 1 ]]; then
  log "Run targeted PAT/auth unit contracts"
  run go test -count=1 ./internal/auth ./internal/errors ./internal/pat ./internal/app ./test/unit
fi

log "Unauthenticated login state"
capture "$OUT_DIR/00-auth-status-empty.json" "$BIN" --format json auth status
"$HELPER_BIN" assert-auth-status "$OUT_DIR/00-auth-status-empty.json" false "" ""
"$HELPER_BIN" assert-auth-error-contract

log "Direct token login path"
capture "$OUT_DIR/10-auth-login-token.json" "$BIN" --yes --format json auth login --token "$TEST_ACCESS_TOKEN"
"$HELPER_BIN" assert-login-json "$OUT_DIR/10-auth-login-token.json" true "" ""
capture "$OUT_DIR/11-auth-status-token.json" "$BIN" --format json auth status
"$HELPER_BIN" assert-auth-status "$OUT_DIR/11-auth-status-token.json" true "" ""
capture "$OUT_DIR/12-auth-reset.out" "$BIN" --yes auth reset
"$HELPER_BIN" assert-empty-auth

log "Seed isolated CI test account profile"
"$HELPER_BIN" seed "$TEST_CORP_ID" "$TEST_CORP_NAME" "$TEST_USER_ID" "$TEST_USER_NAME" "$TEST_ACCESS_TOKEN" "$TEST_REFRESH_TOKEN"
capture "$OUT_DIR/20-auth-status-profile.json" "$BIN" --format json auth status
"$HELPER_BIN" assert-auth-status "$OUT_DIR/20-auth-status-profile.json" true "$TEST_CORP_ID" "$TEST_USER_ID"
capture "$OUT_DIR/21-profile-list.json" "$BIN" --format json profile list
"$HELPER_BIN" assert-profile-list "$OUT_DIR/21-profile-list.json" "$TEST_CORP_ID" "$TEST_USER_ID"

log "PAT CLI argument and authorization gates"
capture "$OUT_DIR/30-pat-dry-run-session.out" env DWS_SESSION_ID=ci-session-001 "$BIN" --dry-run --format table pat chmod aitable.record:read --grant-type session
expect_contains "$OUT_DIR/30-pat-dry-run-session.out" "[DRY-RUN]"
expect_contains "$OUT_DIR/30-pat-dry-run-session.out" "SessionID:"
expect_contains "$OUT_DIR/30-pat-dry-run-session.out" "ci-session-001"

capture "$OUT_DIR/31-pat-dry-run-agent.out" env DINGTALK_DWS_AGENTCODE=agt_ci_host DWS_SESSION_ID=ci-session-002 "$BIN" --dry-run --format table pat chmod aitable.record:read --grant-type session
expect_contains "$OUT_DIR/31-pat-dry-run-agent.out" "AgentCode:"
expect_contains "$OUT_DIR/31-pat-dry-run-agent.out" "agt_ci_host"

expect_fail "--session-id is required" "$BIN" --dry-run --format json pat chmod aitable.record:read --grant-type session
expect_fail "invalid agentCode" env DINGTALK_DWS_AGENTCODE='bad agent code' "$BIN" --dry-run --format json pat chmod aitable.record:read --grant-type once
expect_fail "batch PAT authorization blocked" env DINGTALK_DWS_AGENTCODE=agt_ci_host "$BIN" --format json pat chmod aitable.record:read aitable.record:write --grant-type once

log "PAT stderr JSON and host-owned contracts"
"$HELPER_BIN" assert-pat-contract cli
"$HELPER_BIN" assert-pat-contract host
"$HELPER_BIN" assert-login-recommend

log "Secret redaction check"
assert_no_secret_leak "$TEST_ACCESS_TOKEN" "access token"
assert_no_secret_leak "$TEST_REFRESH_TOKEN" "refresh token"

echo
echo "[OK] PAT/login authorization CI/CD matrix passed"
