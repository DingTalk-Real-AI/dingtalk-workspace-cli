package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type oidcDiagnosticAuthorizer struct{}

func (oidcDiagnosticAuthorizer) Authorize(_ context.Context, request authpkg.OIDCAuthorizationRequest) (authpkg.OIDCAuthorizationResult, error) {
	return authpkg.OIDCAuthorizationResult{Code: "code-canary", State: request.Parameters["state"]}, nil
}

func TestCrossPlatformCoverageOIDCCommandErrorOutput(t *testing.T) {
	for _, entry := range []string{"exchange-prepare", "token-exchange", "provider-prepare", "provider-exchange"} {
		for _, format := range []string{"json", "table"} {
			for _, action := range []string{"stop", "restart_login"} {
				t.Run(entry+"/"+format+"/"+action, func(t *testing.T) {
					dir := t.TempDir()
					t.Setenv("DWS_CONFIG_DIR", dir)
					calls := 0
					code := "oidc_identity_invalid"
					status := http.StatusForbidden
					if action == "stop" {
						code = "oidc_configuration_invalid"
						status = http.StatusInternalServerError
					}
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls++
						if entry == "provider-exchange" && r.URL.Path == "/cli/auth/oidc/transactions" {
							w.WriteHeader(201)
							_ = json.NewEncoder(w).Encode(map[string]any{"provider": "ant", "expires_in": 600, "authorization_request": map[string]any{
								"authorization_endpoint": "https://idp.example/authorize", "parameters": map[string]string{
									"client_id": "idp-app", "response_type": "code", "scope": "openid", "redirect_uri": "ant-sdk://oidc/callback",
									"state": splitOIDCState, "nonce": strings.Repeat("n", 43), "code_challenge": strings.Repeat("c", 43), "code_challenge_method": "S256",
								},
							}})
							return
						}
						w.Header().Set("X-Dws-Trace-Id", "trace-cli-123")
						w.WriteHeader(status)
						_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "action": action, "message": "code-canary"}})
					}))
					defer server.Close()
					if err := os.WriteFile(filepath.Join(dir, "mcp_url"), []byte(server.URL), 0600); err != nil {
						t.Fatal(err)
					}
					root, _ := splitOIDCRoot(t, nil)
					args := []string{"auth", entry, "--provider", "ant", "--format", format}
					if strings.HasPrefix(entry, "provider-") {
						root, _, _ = providerLoginRoot()
						args = []string{"auth", "provider-login", "ant", "--format", format}
						testseam.Swap(t, &authProviderLogin, func(ctx context.Context, _, provider, base string, _ edition.ToolCaller, _ *authpkg.OIDCExchangeCredentials) (*authpkg.TokenData, error) {
							return authpkg.NewOIDCLoginProvider(provider, base, oidcDiagnosticAuthorizer{}).Login(ctx)
						})
					} else if entry == "token-exchange" {
						args = append(args, "--code", "code-canary", "--state", splitOIDCState)
					}
					var stdout, stderr bytes.Buffer
					root.SetOut(&stdout)
					root.SetErr(&stderr)
					root.SetArgs(args)
					command, err := corecmd.ExecuteCForTest(root)
					if err == nil || apperrors.ExitCode(err) != 2 {
						t.Fatalf("expected auth failure: %v", err)
					}
					unified := output.UsesUnifiedResult(command)
					if unified {
						if status, emitErr := emitFailureWithVersionNotice(command, err, nil); emitErr != nil || status != 2 {
							t.Fatalf("failure rendering: status=%d error=%v", status, emitErr)
						}
					} else if printErr := printExecutionError(command, &stdout, &stderr, err); printErr != nil {
						t.Fatal(printErr)
					}
					got := stdout.String() + stderr.String()
					if !strings.Contains(got, code) || !strings.Contains(got, "trace-cli-123") || strings.Contains(got, "canary") {
						t.Fatalf("diagnostics missing or credentials leaked: %s", got)
					}
					if action == "stop" && (strings.Contains(got, "Run dws auth exchange-prepare") || !strings.Contains(got, "administrator")) {
						t.Fatalf("wrong stop recovery: %s", got)
					}
					if format == "json" {
						var envelope struct {
							Error map[string]any `json:"error"`
						}
						if json.Unmarshal([]byte(got), &envelope) != nil || envelope.Error["trace_id"] != "trace-cli-123" {
							t.Fatalf("invalid JSON diagnostics: %s", got)
						}
						codeField := "server_error_code"
						if unified {
							codeField = "upstream_code"
						}
						if envelope.Error[codeField] != code {
							t.Fatalf("lost backend error code: %s", got)
						}
					}
					wantCalls := 1
					if entry == "provider-exchange" {
						wantCalls = 2
					}
					if calls != wantCalls {
						t.Fatalf("unexpected retry: calls=%d want=%d", calls, wantCalls)
					}
					if format != "json" && stdout.Len() != 0 {
						t.Fatal("human failure polluted stdout")
					}
				})
			}
		}
	}
}
