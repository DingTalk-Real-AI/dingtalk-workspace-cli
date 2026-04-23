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
	"strings"
	"testing"
)

func TestExitCodeByCategory(t *testing.T) {
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

func TestPrintJSON(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	if err := PrintJSON(&b, NewValidation(
		"bad flag",
		WithReason("missing_required_flag"),
		WithHint("Pass the required flag and retry."),
		WithRetryable(true),
		WithActions("dws schema doc.create_document", "retry command"),
		WithSnapshot("/tmp/dws-recovery/snapshot.json"),
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
	if !strings.Contains(got, "\"retryable\": true") {
		t.Fatalf("expected retryable in output, got %q", got)
	}
	if !strings.Contains(got, "\"hint\": \"Pass the required flag and retry.\"") {
		t.Fatalf("expected hint in output, got %q", got)
	}
	if !strings.Contains(got, "\"snapshot_path\": \"/tmp/dws-recovery/snapshot.json\"") {
		t.Fatalf("expected snapshot path in output, got %q", got)
	}
}

func TestPrintHuman(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	if err := PrintHumanAt(&b, NewValidation(
		"bad flag",
		WithReason("missing_required_flag"),
		WithOperation("calendar.list"),
		WithServerKey("calendar"),
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
}

func TestPrintHuman_NormalMode(t *testing.T) {
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

func TestPrintJSONIncludesServerDiag(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	if err := PrintJSON(&b, NewAPI(
		"server error",
		WithServerDiag(ServerDiagnostics{
			TraceID:         "trace-xyz",
			ServerErrorCode: "TIMEOUT_ERROR",
			TechnicalDetail: "deadline exceeded",
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
}

func TestPrintJSONIncludesRPCCodeAndData(t *testing.T) {
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

func TestPrintHumanIncludesRPCCode_Debug(t *testing.T) {
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

func TestPrintHumanHidesRPCCode_Normal(t *testing.T) {
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
