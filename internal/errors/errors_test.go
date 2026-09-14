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

package errors

import (
	stderrors "errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

func TestCrossPlatformCoverageExitCodeByCategory(t *testing.T) {
	t.Parallel()

	cases := []struct {
		err  error
		want int
	}{
		{err: NewAPI("api"), want: 1},
		{err: NewAuth("auth"), want: 2},
		{err: NewValidation("validation"), want: 3},
		{err: NewDiscovery("discovery"), want: 6},
		{err: NewInternal("internal"), want: 5},
		{err: stderrors.New("plain"), want: 5},
	}

	for _, tc := range cases {
		if got := ExitCode(tc.err); got != tc.want {
			t.Fatalf("ExitCode(%v) = %d, want %d", tc.err, got, tc.want)
		}
	}
}

func TestCrossPlatformCoveragePrintJSON(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	if err := PrintJSON(&b, NewValidation(
		"bad flag",
		WithReason("missing_required_flag"),
		WithOrigin("client"),
		WithFailureStage("request_validation"),
		WithExecutionStarted(false),
		WithHint("Pass the required flag and retry."),
		WithRetryable(true),
		WithActions("dws schema doc.create_document", "retry command"),
		WithSnapshot("/tmp/dws-recovery/snapshot.json"),
		WithDetails(map[string]any{
			"type":  "resolution",
			"query": "项目群",
		}),
	)); err != nil {
		t.Fatalf("PrintJSON() error = %v", err)
	}

	got := b.String()
	if !strings.Contains(got, "\"category\": \"validation\"") {
		t.Fatalf("expected validation category in output, got %q", got)
	}
	if !strings.Contains(got, "\"message\": \"bad flag\"") {
		t.Fatalf("expected error message in output, got %q", got)
	}
	if !strings.Contains(got, "\"reason\": \"missing_required_flag\"") {
		t.Fatalf("expected reason in output, got %q", got)
	}
	if !strings.Contains(got, "\"origin\": \"client\"") ||
		!strings.Contains(got, "\"stage\": \"request_validation\"") ||
		!strings.Contains(got, "\"execution_started\": false") {
		t.Fatalf("expected failure provenance in output, got %q", got)
	}
	if !strings.Contains(got, "\"retryable\": true") {
		t.Fatalf("expected retryable in output, got %q", got)
	}
	if !strings.Contains(got, "\"hint\": \"Pass the required flag and retry.\"") {
		t.Fatalf("expected hint in output, got %q", got)
	}
	if !strings.Contains(got, "\"snapshot_path\": \"/tmp/dws-recovery/snapshot.json\"") {
		t.Fatalf("expected snapshot path in output, got %q", got)
	}
	if !strings.Contains(got, "\"type\": \"resolution\"") || !strings.Contains(got, "\"query\": \"项目群\"") {
		t.Fatalf("expected structured details in output, got %q", got)
	}
}

func TestCrossPlatformCoverageRetryabilityTriStateAndRetryTiming(t *testing.T) {
	t.Parallel()

	next := time.Date(2026, time.July, 30, 4, 5, 6, 0, time.FixedZone("CST", 8*60*60))
	tests := []struct {
		name            string
		err             error
		wantRetryable   string
		wantRetryAfter  bool
		wantNextRetryAt bool
	}{
		{
			name: "unknown is omitted",
			err:  NewAPI("unknown"),
		},
		{
			name:          "explicit false is preserved",
			err:           NewValidation("terminal", WithRetryable(false)),
			wantRetryable: `"retryable": false`,
		},
		{
			name:            "explicit true with timing",
			err:             NewAPI("transient", WithRetryable(true), WithRetryAfterSeconds(30), WithNextRetryAt(next)),
			wantRetryable:   `"retryable": true`,
			wantRetryAfter:  true,
			wantNextRetryAt: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var jsonOut strings.Builder
			if err := PrintJSON(&jsonOut, tt.err); err != nil {
				t.Fatalf("PrintJSON() error = %v", err)
			}
			gotJSON := jsonOut.String()
			if tt.wantRetryable == "" {
				if strings.Contains(gotJSON, `"retryable"`) {
					t.Fatalf("unknown retryability must be omitted: %s", gotJSON)
				}
			} else if !strings.Contains(gotJSON, tt.wantRetryable) {
				t.Fatalf("missing %s in %s", tt.wantRetryable, gotJSON)
			}
			if got := strings.Contains(gotJSON, `"retry_after_seconds": 30`); got != tt.wantRetryAfter {
				t.Fatalf("retry_after_seconds presence = %v, want %v: %s", got, tt.wantRetryAfter, gotJSON)
			}
			if got := strings.Contains(gotJSON, `"next_retry_at": "2026-07-29T20:05:06Z"`); got != tt.wantNextRetryAt {
				t.Fatalf("next_retry_at presence = %v, want %v: %s", got, tt.wantNextRetryAt, gotJSON)
			}

			var humanOut strings.Builder
			if err := PrintHuman(&humanOut, tt.err); err != nil {
				t.Fatalf("PrintHuman() error = %v", err)
			}
			gotHuman := humanOut.String()
			if tt.wantRetryable == "" {
				if strings.Contains(gotHuman, "Retryable:") {
					t.Fatalf("unknown retryability must be omitted: %s", gotHuman)
				}
			} else {
				want := "Retryable: true"
				if strings.Contains(tt.wantRetryable, "false") {
					want = "Retryable: false"
				}
				if !strings.Contains(gotHuman, want) {
					t.Fatalf("missing %q in %s", want, gotHuman)
				}
			}
			if tt.wantRetryAfter && !strings.Contains(gotHuman, "Retry After: 30s") {
				t.Fatalf("missing retry delay in %s", gotHuman)
			}
			if tt.wantNextRetryAt && !strings.Contains(gotHuman, "Next Retry At: 2026-07-29T20:05:06Z") {
				t.Fatalf("missing next retry time in %s", gotHuman)
			}
		})
	}
}

func TestCrossPlatformCoverageRetryTimingOptionsIgnoreInvalidValues(t *testing.T) {
	t.Parallel()

	err := NewAPI(
		"invalid timing",
		WithRetryAfterSeconds(-1),
		WithNextRetryAt(time.Time{}),
	).(*Error)
	if err.RetryAfterSeconds != nil || err.NextRetryAt != nil {
		t.Fatalf("invalid retry timing was retained: %#v", err)
	}
}

func TestCrossPlatformCoveragePrintJSON_AvailableFlags(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	if err := PrintJSON(&b, NewValidation(
		"unknown flag: --foo",
		WithReason("unknown_flag"),
		WithHint("Did you mean --bar?"),
		WithAvailableFlags("bar", "baz"),
	)); err != nil {
		t.Fatalf("PrintJSON() error = %v", err)
	}
	got := b.String()
	if !strings.Contains(got, `"available_flags"`) {
		t.Fatalf("expected available_flags in output, got %q", got)
	}
	if !strings.Contains(got, `"bar"`) || !strings.Contains(got, `"baz"`) {
		t.Fatalf("expected flag names in output, got %q", got)
	}
}

func TestCrossPlatformCoveragePrintHuman(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	if err := PrintHumanAt(&b, NewValidation(
		"bad flag",
		WithReason("missing_required_flag"),
		WithOperation("calendar.list"),
		WithServerKey("calendar"),
		WithOrigin("client"),
		WithFailureStage("request_validation"),
		WithExecutionStarted(false),
		WithHint("Pass the required flag and retry."),
		WithRetryable(true),
		WithActions("retry command"),
		WithSnapshot("/tmp/dws-recovery/snapshot.json"),
	), VerbosityVerbose); err != nil {
		t.Fatalf("PrintHuman() error = %v", err)
	}

	got := b.String()
	if !strings.Contains(got, "Error: [VALIDATION] bad flag") {
		t.Fatalf("expected formatted header in output, got %q", got)
	}
	if !strings.Contains(got, "Reason: missing_required_flag") {
		t.Fatalf("expected reason in output, got %q", got)
	}
	if !strings.Contains(got, "Hint: Pass the required flag and retry.") {
		t.Fatalf("expected hint in output, got %q", got)
	}
	if !strings.Contains(got, "Action: retry command") {
		t.Fatalf("expected action in output, got %q", got)
	}
	if !strings.Contains(got, "Snapshot: /tmp/dws-recovery/snapshot.json") {
		t.Fatalf("expected snapshot in verbose output, got %q", got)
	}
	if !strings.Contains(got, "Retryable: true") {
		t.Fatalf("expected retryable marker in output, got %q", got)
	}
	for _, want := range []string{"Origin: client", "Stage: request_validation", "Execution Started: false"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in verbose output, got %q", want, got)
		}
	}

	withoutDetails := NewValidation("empty", WithDetails(nil)).(*Error)
	if withoutDetails.Details != nil {
		t.Fatalf("empty details were retained: %#v", withoutDetails.Details)
	}
}

func TestCrossPlatformCoveragePrintHuman_NormalMode(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	PrintHuman(&b, NewValidation(
		"bad flag",
		WithHint("fix it"),
		WithRetryable(true),
		WithActions("retry"),
		WithServerDiag(ServerDiagnostics{TraceID: "trace-abc", ServerErrorCode: "PARAM_ERROR"}),
	))

	got := b.String()
	if !strings.Contains(got, "Error: [VALIDATION] bad flag") {
		t.Fatalf("expected header, got %q", got)
	}
	if !strings.Contains(got, "Trace ID: trace-abc") {
		t.Fatalf("expected trace id in normal output, got %q", got)
	}
	if !strings.Contains(got, "Server Code: PARAM_ERROR") {
		t.Fatalf("expected server code in normal output, got %q", got)
	}
}

func TestCrossPlatformCoveragePrintJSONIncludesServerDiag(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	if err := PrintJSON(&b, NewAPI(
		"server error",
		WithServerDiag(ServerDiagnostics{
			TraceID:         "trace-xyz",
			ServerErrorCode: "TIMEOUT_ERROR",
			TechnicalDetail: "deadline exceeded",
			FriendlyHint:    "请开通消息搜索权益",
			ActionURL:       "https://example.test/enable-search",
		}),
	)); err != nil {
		t.Fatalf("PrintJSON() error = %v", err)
	}

	got := b.String()
	if !strings.Contains(got, `"trace_id": "trace-xyz"`) {
		t.Fatalf("expected trace_id in output, got %q", got)
	}
	if !strings.Contains(got, `"server_error_code": "TIMEOUT_ERROR"`) {
		t.Fatalf("expected server_error_code in output, got %q", got)
	}
	if !strings.Contains(got, `"technical_detail": "deadline exceeded"`) {
		t.Fatalf("expected technical_detail in output, got %q", got)
	}
	if !strings.Contains(got, `"friendly_hint": "请开通消息搜索权益"`) {
		t.Fatalf("expected server friendly_hint in output, got %q", got)
	}
	if !strings.Contains(got, `"action_url": "https://example.test/enable-search"`) {
		t.Fatalf("expected server action_url in output, got %q", got)
	}
}

func TestCrossPlatformCoveragePrintHumanIncludesServerGuidance(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	if err := PrintHuman(&b, NewAPI(
		"search entitlement required",
		WithServerDiag(ServerDiagnostics{
			ServerErrorCode: "SEARCH_ENTITLEMENT_REQUIRED",
			FriendlyHint:    "请联系管理员开通消息搜索权益",
			ActionURL:       "https://example.test/enable-search",
		}),
	)); err != nil {
		t.Fatalf("PrintHuman() error = %v", err)
	}

	got := b.String()
	if !strings.Contains(got, "Hint: 请联系管理员开通消息搜索权益") {
		t.Fatalf("expected server guidance in output, got %q", got)
	}
	if !strings.Contains(got, "Action: 处理入口: https://example.test/enable-search") {
		t.Fatalf("expected server action URL in output, got %q", got)
	}
}

func TestCrossPlatformCoverageServerGuidanceAdapter(t *testing.T) {
	t.Parallel()

	hint, action := ServerGuidance(ServerDiagnostics{
		FriendlyHint: "follow the recovery action",
		ActionURL:    "https://example.test/recover",
	})
	if hint != "follow the recovery action" || action != "https://example.test/recover" {
		t.Fatalf("ServerGuidance() = (%q, %q)", hint, action)
	}
}

func TestCrossPlatformCoverageServerGuidanceSuppressesUnsafeActionURL(t *testing.T) {
	t.Parallel()
	for _, actionURL := range []string{
		"http://example.test/help",
		"javascript:alert(1)",
		"https://user:secret@example.test/help",
		"not a url",
	} {
		var human strings.Builder
		err := NewAPI("server error", WithServerDiag(ServerDiagnostics{
			FriendlyHint: "保留 Trace ID 后排查",
			ActionURL:    actionURL,
		}))
		if printErr := PrintHuman(&human, err); printErr != nil {
			t.Fatal(printErr)
		}
		if strings.Contains(human.String(), actionURL) || strings.Contains(human.String(), "处理入口") {
			t.Fatalf("unsafe action URL %q leaked to human output: %q", actionURL, human.String())
		}
		var jsonOutput strings.Builder
		if printErr := PrintJSON(&jsonOutput, err); printErr != nil {
			t.Fatal(printErr)
		}
		if strings.Contains(jsonOutput.String(), `"action_url"`) {
			t.Fatalf("unsafe action URL %q leaked to JSON output: %q", actionURL, jsonOutput.String())
		}
	}
}

func TestCrossPlatformCoveragePrintJSONCLIOrgNotAuthorizedUsesInternationalActionURL(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", dir)
	if err := os.WriteFile(filepath.Join(dir, "mcp_url"), []byte("https://mcp.dingtalk.io\n"), config.FilePerm); err != nil {
		t.Fatalf("WriteFile(mcp_url) error = %v", err)
	}

	var b strings.Builder
	if err := PrintJSON(&b, NewAPI(
		"business error",
		WithServerDiag(ServerDiagnostics{ServerErrorCode: "CLI_ORG_NOT_AUTHORIZED"}),
	)); err != nil {
		t.Fatalf("PrintJSON() error = %v", err)
	}

	want := `"action_url": "https://open-dev.dingtalk.io/fe/old#/developerSettings"`
	if got := b.String(); !strings.Contains(got, want) {
		t.Fatalf("expected international action_url %q, got %q", want, got)
	}
}

func TestCrossPlatformCoveragePrintJSONIncludesRPCCodeAndData(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	if err := PrintJSON(&b, NewAPI(
		"JSON-RPC tools/call failed with code -32602: invalid arguments",
		WithReason("tools_call_jsonrpc_invalid_params"),
		WithRPCCode(-32602),
		WithRPCData([]byte(`{"field":"base_id","error":"required"}`)),
	)); err != nil {
		t.Fatalf("PrintJSON() error = %v", err)
	}

	got := b.String()
	if !strings.Contains(got, `"rpc_code": -32602`) {
		t.Fatalf("expected rpc_code in output, got %q", got)
	}
	if !strings.Contains(got, `"field"`) || !strings.Contains(got, `"base_id"`) {
		t.Fatalf("expected rpc_data content in output, got %q", got)
	}
}

func TestCrossPlatformCoveragePrintHumanIncludesRPCCode_Debug(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	if err := PrintHumanAt(&b, NewValidation(
		"invalid params",
		WithRPCCode(-32602),
		WithRPCData([]byte(`"missing field"`)),
	), VerbosityDebug); err != nil {
		t.Fatalf("PrintHuman() error = %v", err)
	}

	got := b.String()
	if !strings.Contains(got, "RPC Code: -32602") {
		t.Fatalf("expected RPC Code in debug output, got %q", got)
	}
	if !strings.Contains(got, "RPC Data:") {
		t.Fatalf("expected RPC Data in debug output, got %q", got)
	}
}

func TestCrossPlatformCoveragePrintHumanHidesRPCCode_Normal(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	PrintHuman(&b, NewValidation(
		"invalid params",
		WithRPCCode(-32602),
	))

	got := b.String()
	if strings.Contains(got, "RPC Code:") {
		t.Fatalf("normal mode should not show RPC Code, got %q", got)
	}
}

func TestCrossPlatformCoverageIsConfirmationRequired(t *testing.T) {
	t.Parallel()

	if IsConfirmationRequired(nil) {
		t.Fatal("nil error must not report confirmation_required")
	}
	if IsConfirmationRequired(NewValidation("missing required flag")) {
		t.Fatal("plain validation error must not report confirmation_required")
	}
	plain := stderrors.New("需要用户确认")
	if IsConfirmationRequired(plain) {
		t.Fatal("message text alone must not report confirmation_required")
	}
	confirmation := NewValidation(
		"blocked",
		WithReason("confirmation_required"),
	)
	if !IsConfirmationRequired(confirmation) {
		t.Fatal("typed confirmation error must report confirmation_required")
	}
	// 包装链（fmt.Errorf %w）必须能穿透到 typed 原因。
	wrapped := fmt.Errorf("call tool: %w", confirmation)
	if !IsConfirmationRequired(wrapped) {
		t.Fatal("wrapped confirmation error must report confirmation_required")
	}
	otherReason := NewValidation("rate limited", WithReason("rate_limit"))
	if IsConfirmationRequired(otherReason) {
		t.Fatal("other reasons must not report confirmation_required")
	}
}
