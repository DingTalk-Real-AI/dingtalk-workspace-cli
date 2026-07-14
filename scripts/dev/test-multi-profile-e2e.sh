#!/usr/bin/env bash
# End-to-end regression script for multi-profile / multi-organization login.
# It uses an isolated DWS_CONFIG_DIR and DWS_KEYCHAIN_DIR, seeds post-login
# token results through the production auth storage API, then verifies the real
# dws CLI command surface.
#
# Usage:
#   bash scripts/dev/test-multi-profile-e2e.sh
#   bash scripts/dev/test-multi-profile-e2e.sh --skip-go-tests --verbose

set -euo pipefail

echo "[FAIL] intentional CI pipeline probe: Multi Profile E2E must fail" >&2
exit 97

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
      sed -n '1,12p' "$0"
      exit 0
      ;;
    *)
      echo "unknown option: $1" >&2
      exit 2
      ;;
  esac
done

mkdir -p "$ROOT/.tmp-bin"
WORKDIR="$(mktemp -d "$ROOT/.tmp-bin/multi-profile-e2e.XXXXXX")"
BIN="$WORKDIR/bin/dws"
HELPER_DIR="$WORKDIR/helper"
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

expect_not_contains_line_command() {
  local file="$1"
  local command="$2"
  if grep -E "^[[:space:]]+$command([[:space:]]|$)" "$file" >/dev/null; then
    echo "----- $file -----" >&2
    cat "$file" >&2
    fail "did not expect command '$command' in $file"
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

cat >"$HELPER_DIR/main.go" <<'GOEOF'
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	auth "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
)

type profileListResponse struct {
	Success         bool          `json:"success"`
	PrimaryProfile  string        `json:"primaryProfile"`
	CurrentProfile  string        `json:"currentProfile"`
	PreviousProfile string        `json:"previousProfile"`
	Profiles        []profileView `json:"profiles"`
}

type profileUseResponse struct {
	Success bool        `json:"success"`
	Profile profileView `json:"profile"`
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

type multiProfileResponse struct {
	Success      bool                 `json:"success"`
	MultiProfile bool                 `json:"multiProfile"`
	Summary      multiProfileSummary  `json:"summary"`
	Profiles     []multiProfileResult `json:"profiles"`
}

type multiProfileSummary struct {
	Total     int `json:"total"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
}

type multiProfileResult struct {
	Selector string         `json:"selector"`
	CorpID   string         `json:"corpId"`
	CorpName string         `json:"corpName"`
	OK       bool           `json:"ok"`
	Result   map[string]any `json:"result"`
}

func main() {
	if len(os.Args) < 2 {
		die("missing helper command")
	}
	configDir := os.Getenv("DWS_CONFIG_DIR")
	if strings.TrimSpace(configDir) == "" {
		die("DWS_CONFIG_DIR is required")
	}
	switch os.Args[1] {
	case "seed":
		needArgs(7)
		data := token(os.Args[2], os.Args[3], os.Args[4], os.Args[5], os.Args[6])
		must(auth.SaveTokenData(configDir, data))
	case "seed-legacy":
		needArgs(7)
		data := token(os.Args[2], os.Args[3], os.Args[4], os.Args[5], os.Args[6])
		must(auth.SaveTokenDataKeychain(data))
		must(auth.WriteTokenMarker(configDir))
	case "write-app-config":
		needArgs(4)
		must(auth.SaveAppConfig(configDir, &auth.AppConfig{
			ClientID:     os.Args[2],
			ClientSecret: auth.PlainSecret(os.Args[3]),
		}))
	case "assert-app-config":
		needArgs(3)
		cfg, err := auth.LoadAppConfig(configDir)
		must(err)
		switch os.Args[2] {
		case "exists":
			if cfg == nil || strings.TrimSpace(cfg.ClientID) == "" {
				die("expected app config to exist")
			}
		case "absent":
			if cfg != nil {
				die("expected app config to be absent, got clientID=%q", cfg.ClientID)
			}
		default:
			die("unknown app config expectation %q", os.Args[2])
		}
	case "assert-profiles":
		needArgs(6)
		cfg, err := auth.LoadProfiles(configDir)
		must(err)
		wantCount := atoi(os.Args[2])
		if len(cfg.Profiles) != wantCount {
			die("profiles len=%d, want %d: %#v", len(cfg.Profiles), wantCount, cfg.Profiles)
		}
		assertEqual("primaryProfile", cfg.PrimaryProfile, emptySentinel(os.Args[3]))
		assertEqual("currentProfile", cfg.CurrentProfile, emptySentinel(os.Args[4]))
		assertEqual("previousProfile", cfg.PreviousProfile, emptySentinel(os.Args[5]))
		assertNoSecrets(configDir)
		assertProfileMetadata(cfg)
	case "assert-list-json":
		needArgs(7)
		var resp profileListResponse
		raw := readJSON(os.Args[2], &resp)
		if strings.Contains(string(raw), `"name"`) {
			die("profile list JSON must not expose local name: %s", string(raw))
		}
		if !resp.Success {
			die("profile list success=false")
		}
		wantCount := atoi(os.Args[3])
		if len(resp.Profiles) != wantCount {
			die("list profiles len=%d, want %d: %#v", len(resp.Profiles), wantCount, resp.Profiles)
		}
		assertEqual("list primaryProfile", resp.PrimaryProfile, emptySentinel(os.Args[4]))
		assertEqual("list currentProfile", resp.CurrentProfile, emptySentinel(os.Args[5]))
		assertEqual("list previousProfile", resp.PreviousProfile, emptySentinel(os.Args[6]))
		for _, p := range resp.Profiles {
			if strings.TrimSpace(p.CorpID) == "" || strings.TrimSpace(p.CorpName) == "" {
				die("profile list item missing corp identity: %#v", p)
			}
			if p.CorpID == resp.PrimaryProfile && !p.IsPrimary {
				die("profile %s should be primary", p.CorpID)
			}
			if p.CorpID == resp.CurrentProfile && !p.IsCurrent {
				die("profile %s should be current", p.CorpID)
			}
		}
	case "assert-switch-json":
		needArgs(5)
		var resp profileUseResponse
		readJSON(os.Args[2], &resp)
		if !resp.Success {
			die("switch JSON success=false")
		}
		assertEqual("switch corpId", resp.Profile.CorpID, os.Args[3])
		assertEqual("switch corpName", resp.Profile.CorpName, os.Args[4])
		if !resp.Profile.IsCurrent {
			die("switch profile isCurrent=false")
		}
	case "assert-status-json":
		needArgs(6)
		var resp authStatusResponse
		readJSON(os.Args[2], &resp)
		if !resp.Success || !resp.Authenticated || !resp.TokenValid || !resp.RefreshTokenValid {
			die("bad auth status response: %#v", resp)
		}
		assertEqual("status corpId", resp.CorpID, os.Args[3])
		assertEqual("status corpName", resp.CorpName, os.Args[4])
		assertEqual("status userId", resp.UserID, os.Args[5])
	case "assert-multi-profile-json":
		needArgs(5)
		var resp multiProfileResponse
		readJSON(os.Args[2], &resp)
		if !resp.Success || !resp.MultiProfile {
			die("bad multi-profile response: %#v", resp)
		}
		wantCount := atoi(os.Args[3])
		if len(resp.Profiles) != wantCount {
			die("multi-profile len=%d, want %d: %#v", len(resp.Profiles), wantCount, resp.Profiles)
		}
		if resp.Summary.Total != wantCount || resp.Summary.Succeeded != wantCount || resp.Summary.Failed != 0 {
			die("bad multi-profile summary: %#v", resp.Summary)
		}
		wantCorpIDs := strings.Split(os.Args[4], ",")
		if len(wantCorpIDs) != wantCount {
			die("want corpId count=%d, want %d", len(wantCorpIDs), wantCount)
		}
		for i, want := range wantCorpIDs {
			want = strings.TrimSpace(want)
			got := resp.Profiles[i]
			if !got.OK {
				die("profile %d ok=false: %#v", i, got)
			}
			assertEqual(fmt.Sprintf("multi-profile corpId[%d]", i), got.CorpID, want)
			if got.Result["_mock"] != true {
				die("profile %s result is not mock payload: %#v", got.CorpID, got.Result)
			}
		}
	case "assert-token":
		needArgs(5)
		data, err := loadToken(configDir, os.Args[2])
		must(err)
		assertEqual("token corpId", data.CorpID, os.Args[3])
		assertEqual("token access", data.AccessToken, os.Args[4])
	case "assert-empty-auth":
		needArgs(2)
		cfg, err := auth.LoadProfiles(configDir)
		must(err)
		if cfg.PrimaryProfile != "" || cfg.CurrentProfile != "" || cfg.PreviousProfile != "" || len(cfg.Profiles) != 0 {
			die("expected empty profiles after reset, got %#v", cfg)
		}
		if auth.TokenDataExistsKeychain() {
			die("legacy auth-token still exists")
		}
	case "assert-duplicate-name-fallback":
		needArgs(4)
		cfg, err := auth.LoadProfiles(configDir)
		must(err)
		p := findProfile(cfg, os.Args[2])
		if p == nil {
			die("profile %q not found", os.Args[2])
		}
		if p.CorpName != os.Args[3] {
			die("profile %s corpName=%q, want %q", p.CorpID, p.CorpName, os.Args[3])
		}
		if p.Name == os.Args[3] || !strings.HasPrefix(p.Name, os.Args[3]+"-") {
			die("profile %s name=%q, want stable fallback prefix %q", p.CorpID, p.Name, os.Args[3]+"-")
		}
	default:
		die("unknown helper command %q", os.Args[1])
	}
}

func token(corpID, corpName, userID, userName, access string) *auth.TokenData {
	return &auth.TokenData{
		AccessToken:    access,
		RefreshToken:   "refresh-" + corpID,
		PersistentCode: "persistent-" + corpID,
		ExpiresAt:      time.Now().Add(2 * time.Hour),
		RefreshExpAt:   time.Now().Add(720 * time.Hour),
		CorpID:         corpID,
		CorpName:       corpName,
		UserID:         userID,
		UserName:       userName,
		ClientID:       "client-" + corpID,
		Source:         "multi-profile-e2e",
	}
}

func needArgs(n int) {
	if len(os.Args) != n {
		die("%s: got %d args, want %d", os.Args[1], len(os.Args)-2, n-2)
	}
}

func loadToken(configDir, selector string) (*auth.TokenData, error) {
	if selector == "default" {
		return auth.LoadTokenData(configDir)
	}
	return auth.LoadTokenDataForProfile(configDir, selector)
}

func readJSON(path string, dst any) []byte {
	data, err := os.ReadFile(path)
	must(err)
	if err := json.Unmarshal(data, dst); err != nil {
		die("parse %s: %v\n%s", path, err, string(data))
	}
	return data
}

func assertProfileMetadata(cfg *auth.ProfilesConfig) {
	names := map[string]string{}
	for _, p := range cfg.Profiles {
		if strings.TrimSpace(p.CorpID) == "" || strings.TrimSpace(p.CorpName) == "" {
			die("profile missing corp metadata: %#v", p)
		}
		if prev, ok := names[p.Name]; ok {
			die("duplicate profile local name %q for %s and %s", p.Name, prev, p.CorpID)
		}
		names[p.Name] = p.CorpID
	}
}

func assertNoSecrets(configDir string) {
	data, err := os.ReadFile(filepath.Join(configDir, "profiles.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		must(err)
	}
	for _, forbidden := range []string{"access_token", "refresh_token", "persistent_code", "client_secret"} {
		if strings.Contains(string(data), forbidden) {
			die("profiles.json contains secret field %q", forbidden)
		}
	}
}

func findProfile(cfg *auth.ProfilesConfig, corpID string) *auth.Profile {
	for i := range cfg.Profiles {
		if cfg.Profiles[i].CorpID == corpID {
			return &cfg.Profiles[i]
		}
	}
	return nil
}

func atoi(raw string) int {
	var n int
	if _, err := fmt.Sscanf(raw, "%d", &n); err != nil {
		die("invalid integer %q", raw)
	}
	return n
}

func emptySentinel(s string) string {
	if s == "_" {
		return ""
	}
	return s
}

func assertEqual(label, got, want string) {
	if got != want {
		die("%s=%q, want %q", label, got, want)
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

if [[ "$RUN_GO_TESTS" -eq 1 ]]; then
  log "running multi-profile Go regressions"
  go test -timeout 180s -count=1 ./internal/auth ./internal/app ./test/cli
fi

log "building dws"
run go build -o "$BIN" ./cmd

helper() {
  go run "$HELPER_DIR" "$@"
}

log "checking command surface"
capture "$OUT_DIR/root-help.txt" "$BIN" --help
expect_contains "$OUT_DIR/root-help.txt" "--profile"
expect_contains "$OUT_DIR/root-help.txt" "--yes"
expect_contains "$OUT_DIR/root-help.txt" "--dry-run"
expect_contains "$OUT_DIR/root-help.txt" "profile"
capture "$OUT_DIR/profile-help.txt" "$BIN" profile --help
expect_contains "$OUT_DIR/profile-help.txt" "list"
expect_contains "$OUT_DIR/profile-help.txt" "switch"
expect_contains "$OUT_DIR/profile-help.txt" "use"
expect_contains "$OUT_DIR/profile-help.txt" "--profile"
capture "$OUT_DIR/auth-login-help.txt" "$BIN" auth login --help
expect_contains "$OUT_DIR/auth-login-help.txt" "--device"
expect_contains "$OUT_DIR/auth-login-help.txt" "--token"
expect_contains "$OUT_DIR/auth-login-help.txt" "--recommend"
expect_contains "$OUT_DIR/auth-login-help.txt" "--yes"
capture "$OUT_DIR/skill-setup-help.txt" "$BIN" skill setup --help
expect_contains "$OUT_DIR/skill-setup-help.txt" "--mode"
expect_contains "$OUT_DIR/skill-setup-help.txt" "--target"
expect_contains "$OUT_DIR/skill-setup-help.txt" "--yes"
expect_contains "$OUT_DIR/skill-setup-help.txt" "--skill"
expect_contains "$OUT_DIR/skill-setup-help.txt" "--exclude"
capture "$OUT_DIR/upgrade-help.txt" "$BIN" upgrade --help
expect_contains "$OUT_DIR/upgrade-help.txt" "--dry-run"
expect_contains "$OUT_DIR/upgrade-help.txt" "--yes"
capture "$OUT_DIR/dev-connect-help.txt" "$BIN" dev connect --help
expect_contains "$OUT_DIR/dev-connect-help.txt" "--robot-client-id"
expect_contains "$OUT_DIR/dev-connect-help.txt" "--robot-client-secret"
expect_contains "$OUT_DIR/dev-connect-help.txt" "--unified-app-id"
expect_contains "$OUT_DIR/dev-connect-help.txt" "--agent-cmd"
expect_contains "$OUT_DIR/dev-connect-help.txt" "--daemon"
capture "$OUT_DIR/doc-delete-help.txt" "$BIN" doc delete --help
expect_contains "$OUT_DIR/doc-delete-help.txt" "--yes"
capture "$OUT_DIR/aitable-base-delete-help.txt" "$BIN" aitable base delete --help
expect_contains "$OUT_DIR/aitable-base-delete-help.txt" "--yes"
capture "$OUT_DIR/auth-help.txt" "$BIN" auth --help
expect_not_contains_line_command "$OUT_DIR/auth-help.txt" "switch"

log "verifying empty profile list"
capture "$OUT_DIR/list-empty.json" "$BIN" profile list --format json
helper assert-list-json "$OUT_DIR/list-empty.json" 0 _ _ _

log "seeding first organization profile"
helper seed corp_alpha "Alpha Org" user_alpha "Alice Alpha" access-alpha-v1
capture "$OUT_DIR/list-alpha.json" "$BIN" profile list --format json
helper assert-list-json "$OUT_DIR/list-alpha.json" 1 corp_alpha corp_alpha _
helper assert-profiles 1 corp_alpha corp_alpha _
helper assert-token default corp_alpha access-alpha-v1
helper assert-token corp_alpha corp_alpha access-alpha-v1
capture "$OUT_DIR/status-alpha-default.json" "$BIN" auth status --format json
helper assert-status-json "$OUT_DIR/status-alpha-default.json" corp_alpha "Alpha Org" user_alpha

log "seeding second organization profile"
helper seed corp_beta "Beta Org" user_beta "Bob Beta" access-beta-v1
capture "$OUT_DIR/list-alpha-beta.json" "$BIN" profile list --format json
helper assert-list-json "$OUT_DIR/list-alpha-beta.json" 2 corp_alpha corp_beta corp_alpha
helper assert-profiles 2 corp_alpha corp_beta corp_alpha
helper assert-token default corp_beta access-beta-v1
helper assert-token corp_alpha corp_alpha access-alpha-v1
helper assert-token corp_beta corp_beta access-beta-v1

log "refreshing existing organization without duplicating profile"
helper seed corp_beta "Beta Org" user_beta "Bob Beta" access-beta-v2
capture "$OUT_DIR/list-beta-refresh.json" "$BIN" profile list --format json
helper assert-list-json "$OUT_DIR/list-beta-refresh.json" 2 corp_alpha corp_beta corp_alpha
helper assert-profiles 2 corp_alpha corp_beta corp_alpha
helper assert-token corp_beta corp_beta access-beta-v2

log "seeding duplicate organization name and checking stable fallback"
helper seed corp_gamma "Beta Org" user_gamma "Gina Gamma" access-gamma-v1
capture "$OUT_DIR/list-duplicate-name.json" "$BIN" profile list --format json
helper assert-list-json "$OUT_DIR/list-duplicate-name.json" 3 corp_alpha corp_gamma corp_beta
helper assert-profiles 3 corp_alpha corp_gamma corp_beta
helper assert-duplicate-name-fallback corp_gamma "Beta Org"

log "switching profiles and verifying legacy mirror"
capture "$OUT_DIR/switch-alpha.json" "$BIN" profile switch corp_alpha --format json
helper assert-switch-json "$OUT_DIR/switch-alpha.json" corp_alpha "Alpha Org"
helper assert-profiles 3 corp_alpha corp_alpha corp_gamma
helper assert-token default corp_alpha access-alpha-v1
capture "$OUT_DIR/switch-beta.txt" "$BIN" profile switch corp_beta --format table
expect_contains "$OUT_DIR/switch-beta.txt" "Beta Org"
expect_contains "$OUT_DIR/switch-beta.txt" "corp_beta"
helper assert-profiles 3 corp_alpha corp_beta corp_alpha
helper assert-token default corp_beta access-beta-v2
capture "$OUT_DIR/switch-previous.json" "$BIN" profile switch - --format json
helper assert-switch-json "$OUT_DIR/switch-previous.json" corp_alpha "Alpha Org"
helper assert-profiles 3 corp_alpha corp_alpha corp_beta
capture "$OUT_DIR/use-gamma.json" "$BIN" profile use corp_gamma --format json
helper assert-switch-json "$OUT_DIR/use-gamma.json" corp_gamma "Beta Org"
helper assert-profiles 3 corp_alpha corp_gamma corp_alpha

log "checking profile switch validation"
expect_fail "profile selector required" "$BIN" profile switch
expect_fail "只能指定一个组织选择器" "$BIN" profile switch corp_alpha --corpId corp_beta
expect_fail "missing_org" "$BIN" profile switch missing_org

log "checking one-shot profile override without changing current profile"
capture "$OUT_DIR/status-root-profile-alpha.json" "$BIN" --profile corp_alpha auth status --format json
helper assert-status-json "$OUT_DIR/status-root-profile-alpha.json" corp_alpha "Alpha Org" user_alpha
helper assert-profiles 3 corp_alpha corp_gamma corp_alpha
capture "$OUT_DIR/status-local-profile-beta.json" "$BIN" auth status --profile corp_beta --format json
helper assert-status-json "$OUT_DIR/status-local-profile-beta.json" corp_beta "Beta Org" user_beta
helper assert-profiles 3 corp_alpha corp_gamma corp_alpha
capture "$OUT_DIR/status-current-gamma.json" "$BIN" auth status --format json
helper assert-status-json "$OUT_DIR/status-current-gamma.json" corp_gamma "Beta Org" user_gamma
capture "$OUT_DIR/contact-multi-profile.json" "$BIN" --mock --profile corp_alpha, corp_beta contact user get-self --format json
helper assert-multi-profile-json "$OUT_DIR/contact-multi-profile.json" 2 corp_alpha,corp_beta
helper assert-profiles 3 corp_alpha corp_gamma corp_alpha
capture "$OUT_DIR/contact-multi-profile-leaf-profile.json" "$BIN" --mock contact user get-self --profile corp_alpha, corp_beta --format json
helper assert-multi-profile-json "$OUT_DIR/contact-multi-profile-leaf-profile.json" 2 corp_alpha,corp_beta
helper assert-profiles 3 corp_alpha corp_gamma corp_alpha

log "checking auth reset cleanup"
helper write-app-config client-reset secret-reset
helper assert-app-config exists
capture "$OUT_DIR/auth-reset.txt" "$BIN" auth reset
expect_contains "$OUT_DIR/auth-reset.txt" "[OK]"
helper assert-empty-auth
helper assert-app-config absent

log "checking legacy single-slot migration"
helper seed-legacy corp_legacy "Legacy Org" user_legacy "Lena Legacy" access-legacy-v1
helper assert-profiles 0 _ _ _
capture "$OUT_DIR/list-legacy-migrated.json" "$BIN" profile list --format json
helper assert-list-json "$OUT_DIR/list-legacy-migrated.json" 1 corp_legacy corp_legacy _
helper assert-profiles 1 corp_legacy corp_legacy _
helper assert-token default corp_legacy access-legacy-v1
helper assert-token corp_legacy corp_legacy access-legacy-v1

log "multi-profile e2e passed"
echo "[PASS] isolated multi-profile chain completed"
