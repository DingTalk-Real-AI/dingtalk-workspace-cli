package mock_mcp_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// Exercise the production CLI process, HTTP MCP transport and business-error
// adapter together. All identities, emails and credentials are synthetic.
func TestMockMCPSmoke_MailEmployeeLookups(t *testing.T) {
	const employee = `{"uid":9223372036854775806,"orgId":456,"staffId":"mock-staff","name":"Mock Employee","orgEmail":"a@example.com"}`
	for _, tc := range []struct {
		name        string
		args        []string
		batchReply  string
		failedReply string
		wantTools   []string
		wantArgs    []map[string]any
		wantCode    int
		wantUnknown int
	}{
		{
			name:      "single",
			args:      []string{"get", "--org-email", "a@example.com"},
			wantTools: []string{"get_user_by_org_email"},
			wantArgs:  []map[string]any{{"orgEmail": "a@example.com"}},
		},
		{
			name:       "batch found and not found",
			args:       []string{"batch-get", "--org-emails", "a@example.com,missing@example.com"},
			batchReply: `{"success":true,"result":{"users":[` + employee + `],"notFoundOrgEmails":["missing@example.com"]}}`,
			wantTools:  []string{"batch_get_users_by_org_emails"},
			wantArgs:   []map[string]any{{"orgEmails": []any{"a@example.com", "missing@example.com"}}},
		},
		{
			name:       "invalid address preserves valid results",
			args:       []string{"batch-get", "--org-emails", "a@example.com,invalid,missing@example.com"},
			batchReply: `{"success":false,"errorCode":"SYSTEM_ERROR","errorMsg":"orgEmails[1]必须是完整有效的企业邮箱地址"}`,
			wantTools:  []string{"batch_get_users_by_org_emails", "get_user_by_org_email", "get_user_by_org_email", "get_user_by_org_email"},
			wantArgs: []map[string]any{
				{"orgEmails": []any{"a@example.com", "invalid", "missing@example.com"}},
				{"orgEmail": "a@example.com"}, {"orgEmail": "invalid"}, {"orgEmail": "missing@example.com"},
			},
			wantCode: 7,
		},
		{
			name:        "permission failure stops fallback without losing results",
			args:        []string{"batch-get", "--org-emails", "a@example.com,invalid,missing@example.com"},
			batchReply:  `{"success":false,"errorCode":"SYSTEM_ERROR","errorMsg":"orgEmails[1]必须是完整有效的企业邮箱地址"}`,
			failedReply: `{"success":false,"errorCode":"noPermission","errorMsg":"Not a member"}`,
			wantTools:   []string{"batch_get_users_by_org_emails", "get_user_by_org_email", "get_user_by_org_email"},
			wantArgs: []map[string]any{
				{"orgEmails": []any{"a@example.com", "invalid", "missing@example.com"}},
				{"orgEmail": "a@example.com"}, {"orgEmail": "invalid"},
			},
			wantCode: 7, wantUnknown: 1,
		},
		{
			name:       "mapping configuration error must not trigger fallback",
			args:       []string{"batch-get", "--org-emails", "a@example.com,missing@example.com"},
			batchReply: `{"success":false,"errorCode":"PARAM_ERROR","errorMsg":"Expected ',' in expression"}`,
			wantTools:  []string{"batch_get_users_by_org_emails"},
			wantArgs:   []map[string]any{{"orgEmails": []any{"a@example.com", "missing@example.com"}}},
			wantCode:   1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			var calls []recordedToolCall
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					ID     json.RawMessage `json:"id"`
					Method string          `json:"method"`
					Params struct {
						Name      string         `json:"name"`
						Arguments map[string]any `json:"arguments"`
					} `json:"params"`
				}
				err := json.NewDecoder(r.Body).Decode(&request)
				mu.Lock()
				calls = append(calls, recordedToolCall{path: r.URL.Path, method: request.Method,
					authorization: r.Header.Get("Authorization"), tool: request.Params.Name, arguments: request.Params.Arguments, err: err})
				mu.Unlock()
				response := `{"success":true,"result":` + employee + `}`
				if request.Params.Name == "batch_get_users_by_org_emails" {
					response = tc.batchReply
				} else if request.Params.Arguments["orgEmail"] == "missing@example.com" {
					response = `{"success":true,"result":null}`
				} else if request.Params.Arguments["orgEmail"] == "invalid" {
					response = `{"success":false,"errorCode":"SYSTEM_ERROR","errorMsg":"orgEmail必须是完整有效的企业邮箱地址"}`
					if tc.failedReply != "" {
						response = tc.failedReply
					}
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"jsonrpc": "2.0", "id": request.ID,
					"result": map[string]any{"content": []map[string]any{{"type": "text", "text": response}}},
				})
			}))
			defer server.Close()
			env := isolatedCLIEnv(t, map[string]string{"DINGTALK_MAIL_MCP_URL": server.URL + "/mcp/mail"})
			args := append([]string{"--token", "ci-smoke-token", "--format", "json", "mail", "user"}, tc.args...)
			stdout, stderr, err := runCLI(t, env, args...)
			code := 0
			if err != nil {
				exit, ok := err.(*exec.ExitError)
				if !ok {
					t.Fatalf("CLI execution: %v", err)
				}
				code = exit.ExitCode()
			}
			if code != tc.wantCode {
				t.Fatalf("exit=%d want=%d\nstdout=%s\nstderr=%s", code, tc.wantCode, stdout, stderr)
			}
			mu.Lock()
			recorded := append([]recordedToolCall(nil), calls...)
			mu.Unlock()
			if len(recorded) != len(tc.wantTools) {
				t.Fatalf("calls=%#v, want tools=%v", recorded, tc.wantTools)
			}
			for i, call := range recorded {
				if call.err != nil || call.method != "tools/call" || call.path != "/mcp/mail" || call.authorization != "Bearer ci-smoke-token" || call.tool != tc.wantTools[i] || !reflect.DeepEqual(call.arguments, tc.wantArgs[i]) {
					t.Fatalf("unexpected MCP call %d: %#v", i, call)
				}
			}
			if tc.wantCode != 1 && !strings.Contains(stdout, "9223372036854775806") {
				t.Fatalf("large UID precision lost: %s", stdout)
			}
			var envelope struct {
				OK      bool   `json:"ok"`
				Outcome string `json:"outcome"`
				Error   struct {
					UpstreamCode string `json:"upstream_code"`
				} `json:"error"`
				Data struct {
					Result    json.RawMessage `json:"result"`
					Succeeded []struct {
						ID    string `json:"id"`
						Found bool   `json:"found"`
					} `json:"succeeded"`
					Failed []struct {
						ID string `json:"id"`
					} `json:"failed"`
					Unknown []struct {
						ID string `json:"id"`
					} `json:"unknown"`
				} `json:"data"`
			}
			if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
				t.Fatal(err)
			}
			if tc.wantCode == 7 {
				if envelope.OK || envelope.Outcome != "partial_failure" || len(envelope.Data.Succeeded) != 2-tc.wantUnknown || !envelope.Data.Succeeded[0].Found || envelope.Data.Succeeded[0].ID != "a@example.com" || len(envelope.Data.Failed) != 1 || envelope.Data.Failed[0].ID != "invalid" || len(envelope.Data.Unknown) != tc.wantUnknown {
					t.Fatalf("partial results lost or misclassified: %s", stdout)
				}
				if tc.wantUnknown == 1 {
					if envelope.Data.Unknown[0].ID != "missing@example.com" {
						t.Fatalf("unattempted lookup not marked unknown: %s", stdout)
					}
				} else if envelope.Data.Succeeded[1].Found || envelope.Data.Succeeded[1].ID != "missing@example.com" {
					t.Fatalf("confirmed unmatched address lost: %s", stdout)
				}
			} else if tc.wantCode == 1 {
				if envelope.OK || envelope.Outcome != "failure" || envelope.Error.UpstreamCode != "PARAM_ERROR" {
					t.Fatalf("mapping error misclassified: %s", stdout)
				}
			} else if !envelope.OK || envelope.Outcome != "success" || len(envelope.Data.Result) == 0 {
				t.Fatalf("invalid success result: %s", stdout)
			}
			if tc.name == "batch found and not found" {
				var result struct {
					NotFoundOrgEmails []string `json:"notFoundOrgEmails"`
				}
				if err := json.Unmarshal(envelope.Data.Result, &result); err != nil || !reflect.DeepEqual(result.NotFoundOrgEmails, []string{"missing@example.com"}) {
					t.Fatalf("unmatched address lost: %s", stdout)
				}
			}
		})
	}
}
