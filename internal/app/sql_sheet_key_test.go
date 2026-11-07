// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/audit"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/safety"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type keySnapshotTrap struct{ t *testing.T }

func (s keySnapshotTrap) RecordJSONRPC(string, string, []byte, []byte) string {
	s.t.Fatal("credential reached snapshot")
	return ""
}

func TestCrossPlatformCoverageSQLSheetKeyRunnerNoReplay(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	testseam.Swap(t, &runnerResolveAuthSnapshot, func(*runtimeRunner, context.Context) (AccessTokenSnapshot, error) {
		return AccessTokenSnapshot{AccessToken: "synthetic-token"}, nil
	})
	previousHooks := edition.Get()
	hooks := *previousHooks
	hooks.OnAuthError = func(string, error) error { t.Fatal("credential request entered auth replay"); return nil }
	hooks.ClassifyToolResult = func(map[string]any) error { t.Fatal("credential response entered edition replay"); return nil }
	edition.Override(&hooks)
	t.Cleanup(func() { edition.Override(previousHooks) })
	testseam.Swap(t, &runnerForceRefreshRejectedAccessToken, func(context.Context, string, string) (string, error) {
		t.Fatal("credential request triggered token refresh")
		return "", nil
	})
	for _, tool := range []string{"create_sql_sheet_api_key", "list_sql_sheet_api_keys", "revoke_sql_sheet_api_key"} {
		for _, mode := range []string{"success", "gateway-empty-error", "gateway-empty-error-text", "http-auth", "timeout", "rpc-error", "business-auth", "is-error-auth", "already-exists", "ambiguous-text"} {
			t.Run(tool+mode, func(t *testing.T) {
				wantSuccess := mode == "success" || strings.HasPrefix(mode, "gateway-empty-error")
				calls := 0
				base := transport.NewClient(nil)
				base.FileLogger = slog.New(slog.NewTextHandler(io.Discard, nil))
				base.SnapshotRecorder = keySnapshotTrap{t}
				sink := &auditCoverageSink{}
				r := &runtimeRunner{transport: base, globalFlags: &GlobalFlags{}, auditSink: sink}
				testseam.Swap(t, &runnerCallTool, func(c *transport.Client, _ context.Context, _, _ string, _ map[string]any) (transport.ToolCallResult, error) {
					calls++
					if c.MaxRetries != 0 || c.FileLogger != nil || c.SnapshotRecorder != nil {
						t.Fatal("unsafe credential transport")
					}
					switch mode {
					case "gateway-empty-error", "gateway-empty-error-text":
						// Match the gateway's text and structuredContent success envelopes.
						data := map[string]any{"keys": []any{}}
						if tool == "create_sql_sheet_api_key" {
							data = map[string]any{"keyId": "00000000-0000-4000-8000-000000000001", "status": "ACTIVE", "createdAt": 123, "apiKey": "TEST_SENTINEL_KEY"}
						} else if tool == "revoke_sql_sheet_api_key" {
							data = map[string]any{"keyId": "00000000-0000-4000-8000-000000000001", "revoked": true}
						}
						body := map[string]any{"status": "success", "data": data, "error": map[string]any{}, "meta": map[string]any{}}
						text, err := json.Marshal(body)
						if err != nil {
							t.Fatal(err)
						}
						wire := map[string]any{"isError": false, "content": []any{map[string]any{"type": "text", "text": string(text)}}}
						if mode == "gateway-empty-error" {
							wire["structuredContent"] = body
						}
						raw, err := json.Marshal(wire)
						if err != nil {
							t.Fatal(err)
						}
						var result transport.ToolCallResult
						if err := json.Unmarshal(raw, &result); err != nil {
							t.Fatal(err)
						}
						return result, nil
					case "http-auth":
						return transport.ToolCallResult{}, apperrors.NewAuth("TEST_SENTINEL_KEY", apperrors.WithReason("http_401"))
					case "timeout":
						return transport.ToolCallResult{}, context.DeadlineExceeded
					case "rpc-error":
						return transport.ToolCallResult{IsError: true, Content: map[string]any{"apiKey": "TEST_SENTINEL_KEY"}}, nil
					case "business-auth":
						return transport.ToolCallResult{Content: map[string]any{"status": "error", "error": map[string]any{"code": "403", "message": "TEST_SENTINEL_KEY"}}}, nil
					case "is-error-auth":
						return transport.ToolCallResult{IsError: true, Content: map[string]any{"status": "error", "error": map[string]any{"code": "403", "message": "TEST_SENTINEL_KEY"}}}, nil
					case "already-exists":
						return transport.ToolCallResult{Content: map[string]any{"status": "error", "error": map[string]any{"code": "SQL_SHEET_API_KEY_ALREADY_EXISTS"}}}, nil
					case "ambiguous-text":
						var result transport.ToolCallResult
						err := json.Unmarshal([]byte(`{"content":[{"type":"text","text":"{\"status\":\"error\",\"status\":\"success\",\"data\":{\"apiKey\":\"TEST_SENTINEL_KEY\"}}"}]}`), &result)
						if err != nil {
							t.Fatalf("decode transport fixture: %v", err)
						}
						return result, nil
					default:
						return transport.ToolCallResult{Content: map[string]any{"status": "success", "data": map[string]any{"apiKey": "TEST_SENTINEL_KEY"}}}, nil
					}
				})
				inv := executor.NewHelperInvocation("alias", "aitable", tool, nil)
				_, err := r.executeInvocation(t.Context(), "https://mcp-gw.dingtalk.com/test", inv)
				if calls != 1 {
					t.Fatalf("calls=%d", calls)
				}
				if err != nil && strings.Contains(err.Error(), "TEST_SENTINEL_KEY") {
					t.Fatal("error leaked credential")
				}
				if !wantSuccess && err == nil {
					t.Fatal("expected safe failure")
				}
				if wantSuccess && err != nil {
					t.Fatalf("successful response failed: %v", err)
				}
				if len(sink.events) != 1 || (sink.events[0].Result == "error") != !wantSuccess {
					t.Fatalf("incorrect audit outcome: %#v", sink.events)
				}
				if mode == "business-auth" || mode == "is-error-auth" || mode == "already-exists" {
					wantCode := "403"
					if mode == "already-exists" {
						wantCode = "SQL_SHEET_API_KEY_ALREADY_EXISTS"
					}
					var typed *apperrors.Error
					if !errors.As(err, &typed) || typed.Reason != wantCode || sink.events[0].ErrReason != wantCode {
						t.Fatalf("business error code was lost: %v, audit=%#v", err, sink.events[0])
					}
				}
				if base.FileLogger == nil || base.SnapshotRecorder == nil {
					t.Fatal("mutated shared client")
				}
			})
		}
	}
}

func TestCrossPlatformCoverageSQLSheetKeyRejectMultiProfile(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	testseam.Swap(t, &runnerResolveMultiProfileSelections, func(string, string) ([]multiProfileSelection, bool, error) {
		return []multiProfileSelection{{Selector: "first"}, {Selector: "second"}}, true, nil
	})
	calls := 0
	testseam.Swap(t, &runnerCallTool, func(*transport.Client, context.Context, string, string, map[string]any) (transport.ToolCallResult, error) {
		calls++
		return transport.ToolCallResult{}, nil
	})
	r := &runtimeRunner{transport: transport.NewClient(nil), globalFlags: &GlobalFlags{}, auditSink: audit.NopSink{}}
	for _, tool := range []string{"create_sql_sheet_api_key", "list_sql_sheet_api_keys", "revoke_sql_sheet_api_key"} {
		_, err := r.Run(t.Context(), executor.NewHelperInvocation("alias", "aitable", tool, nil))
		var typed *apperrors.Error
		if !errors.As(err, &typed) || typed.Reason != "sql_sheet_api_key_single_profile_required" {
			t.Fatalf("%s multi profile guard: %v", tool, err)
		}
	}
	if calls != 0 {
		t.Fatalf("multi-profile guard sent %d requests", calls)
	}
}

func TestCrossPlatformCoverageSQLSheetKeyContentScan(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	testseam.Swap(t, &runnerResolveAuthSnapshot, func(*runtimeRunner, context.Context) (AccessTokenSnapshot, error) {
		return AccessTokenSnapshot{AccessToken: "synthetic-token"}, nil
	})
	calls := 0
	testseam.Swap(t, &runnerCallTool, func(*transport.Client, context.Context, string, string, map[string]any) (transport.ToolCallResult, error) {
		calls++
		return transport.ToolCallResult{Content: map[string]any{"status": "success", "data": map[string]any{"apiKey": "TEST_SENTINEL_KEY"}}}, nil
	})
	for _, enforced := range []bool{false, true} {
		r := &runtimeRunner{
			transport: transport.NewClient(nil), globalFlags: &GlobalFlags{}, auditSink: audit.NopSink{},
			scanner:            coverageScanner{report: safety.Report{Scanned: true, Findings: []safety.Finding{{Path: "$.data.apiKey", Pattern: "test", Severity: "high", Snippet: "TEST_SENTINEL_KEY"}}}},
			enforceContentScan: enforced, includeScanReport: true,
		}
		result, err := r.executeInvocation(t.Context(), "https://example.test", executor.NewHelperInvocation("alias", "aitable", "create_sql_sheet_api_key", nil))
		if enforced {
			if err == nil || !strings.Contains(err.Error(), "content safety scan") {
				t.Fatalf("scan enforcement was bypassed: %v", err)
			}
		} else {
			if err != nil {
				t.Fatal(err)
			}
			report := result.Response["safety"].(safety.Report)
			if !report.Scanned || len(report.Findings) != 1 || report.Findings[0].Snippet != "" || report.Findings[0].Severity != "high" {
				t.Fatalf("incorrect safe scan report: %#v", report)
			}
		}
	}
	if calls != 2 {
		t.Fatalf("calls=%d, want one per scan scenario", calls)
	}
}

func TestCrossPlatformCoverageSQLSheetKeyRejectsNonText(t *testing.T) {
	for _, wire := range []string{
		`{"content":[{"type":"image","data":"TEST_SENTINEL_KEY"}]}`,
		`{"content":[{"type":"text","text":"{}"},{"type":"text","text":"{}"}]}`,
	} {
		var result transport.ToolCallResult
		if err := json.Unmarshal([]byte(wire), &result); err != nil {
			t.Fatal(err)
		}
		if err := sqlSheetKeyToolResultError("create_sql_sheet_api_key", result); err == nil {
			t.Fatal("invalid content accepted")
		}
	}
}
