// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/audit"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func TestCrossPlatformCoverageAITableSFCapabilityPATRecovery(t *testing.T) {
	old := edition.Get()
	t.Cleanup(func() { edition.Override(old) })
	edition.Override(&edition.Hooks{})
	for _, tool := range []string{"list_field_groups", "list_builtin_ai_field_plugins"} {
		for _, rejection := range []struct {
			code, message string
			retry         bool
		}{
			{"insufficient_scope", "aitable:read", true}, {"missing_scope", "aitable:read", true},
			{"OTHER_FAILURE", "scope aitable:read required", false},
			{"OTHER_FAILURE", "application scope approval required", false},
		} {
			for _, isError := range []bool{false, true} {
				for _, scoped := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/%s/isError=%t/scoped=%t", tool, rejection.code, isError, scoped), func(t *testing.T) {
						t.Setenv("DWS_CONFIG_DIR", t.TempDir())
						calls, retries := 0, 0
						testseam.Swap(t, &runnerCallTool, func(*transport.Client, context.Context, string, string, map[string]any) (transport.ToolCallResult, error) {
							calls++
							return transport.ToolCallResult{IsError: isError, Content: map[string]any{"status": "error", "error": map[string]any{
								"code": rejection.code, "message": rejection.message,
							}}}, nil
						})
						sentinel := errors.New("authorization requested")
						testseam.Swap(t, &runnerRetryWithPatAuthRetry, func(_ context.Context, _ executor.Runner, inv executor.Invocation, scope *PatScopeError, _ string, _ io.Writer) (executor.Result, error) {
							retries++
							if inv.Tool != tool || scope.MissingScope != "aitable:read" || !strings.Contains(scope.Hint, "dws auth login --scope") {
								t.Fatalf("scope recovery lost: %#v %#v", inv, scope)
							}
							return executor.Result{}, sentinel
						})
						ctx := context.Background()
						if scoped {
							ctx = context.WithValue(ctx, scopedAuthTokenKey, "employee")
						}
						r := &runtimeRunner{transport: transport.NewClient(nil), globalFlags: &GlobalFlags{Token: "fixture-only"}, auditSink: audit.NopSink{}}
						_, err := r.executeInvocation(ctx, "https://fixture.test", executor.Invocation{CanonicalProduct: "aitable", Tool: tool})
						wantRetry := rejection.retry && !scoped
						if calls != 1 || (wantRetry && (retries != 1 || !errors.Is(err, sentinel))) {
							t.Fatalf("scope recovery calls=%d retries=%d scoped=%t err=%v", calls, retries, scoped, err)
						}
						if !wantRetry {
							var typed *apperrors.Error
							if retries != 0 || !errors.As(err, &typed) || typed.Category != apperrors.CategoryAPI || typed.Reason != rejection.code || typed.Message != rejection.message {
								t.Fatalf("business rejection entered PAT recovery or changed: retries=%d err=%v", retries, err)
							}
						}
					})
				}
			}
		}
	}
}

func TestCrossPlatformCoverageAITableSFCapabilityFailureLogs(t *testing.T) {
	for _, tc := range []struct{ tool, code string }{
		{"list_field_groups", "InvalidArgument"}, {"list_builtin_ai_field_plugins", "InvalidArgument"},
		{"create_field_group", "FIELD_GROUP_OUTCOME_UNKNOWN"}, {"update_field_group", "FIELD_GROUP_OUTCOME_UNKNOWN"},
		{"ungroup_field_group", "FIELD_GROUP_OUTCOME_UNKNOWN"}, {"move_field_group_fields", "FIELD_GROUP_OUTCOME_UNKNOWN"},
	} {
		for _, isError := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/isError=%t", tc.tool, isError), func(t *testing.T) {
				t.Setenv("DWS_CONFIG_DIR", t.TempDir())
				testseam.Swap(t, &runnerCallTool, func(*transport.Client, context.Context, string, string, map[string]any) (transport.ToolCallResult, error) {
					return transport.ToolCallResult{IsError: isError, Content: map[string]any{
						"status": "error", "requestId": "trace-fixture", "technicalDetail": "local-detail",
						"error": map[string]any{"code": tc.code, "message": "Safe failure", "retryable": false, "details": map[string]any{"token": "private-token"}},
						"data":  map[string]any{"uid": "private-uid"},
					}}, nil
				})
				var logs bytes.Buffer
				client := transport.NewClient(nil)
				client.FileLogger = slog.New(slog.NewJSONHandler(&logs, nil))
				r := &runtimeRunner{transport: client, globalFlags: &GlobalFlags{Token: "fixture-only"}, auditSink: audit.NopSink{}}
				_, err := r.executeInvocation(context.Background(), "https://fixture.test", executor.Invocation{CanonicalProduct: "aitable", Tool: tc.tool})
				var typed *apperrors.Error
				if !errors.As(err, &typed) || typed.ServerDiag.TraceID != "trace-fixture" || typed.ServerDiag.TechnicalDetail != "" || typed.Details != nil {
					t.Fatalf("unsafe or missing public diagnostics: %#v %v", typed, err)
				}
				wire, marshalErr := json.Marshal(errorInfoFromExecutionError(err))
				if marshalErr != nil || bytes.Contains(wire, []byte("local-detail")) || bytes.Contains(wire, []byte("private-")) {
					t.Fatalf("local diagnostics leaked: %s %v", wire, marshalErr)
				}
				count := 0
				for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
					var entry map[string]any
					if json.Unmarshal([]byte(line), &entry) != nil || entry["msg"] != "business_error" {
						continue
					}
					count++
					if entry["product"] != "aitable" || entry["tool"] != tc.tool || entry["server_error_code"] != tc.code || entry["trace_id"] != "trace-fixture" || entry["technical_detail"] != "local-detail" {
						t.Fatalf("local diagnostics lost: %#v", entry)
					}
				}
				if count != 1 || strings.Contains(logs.String(), "private-") {
					t.Fatalf("missing, duplicate or unsafe business log: %s", logs.String())
				}
			})
		}
	}
}
