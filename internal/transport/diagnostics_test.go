package transport

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestExtractServerDiagnostics_SnakeCase(t *testing.T) {
	t.Parallel()
	data := json.RawMessage(`{"trace_id":"abc123","code":"PARAM_ERROR","technical_detail":"field required","friendly_hint":"请开通消息搜索权益","action_url":"https://example.test/enable-search","retryable":false}`)
	diag := ExtractServerDiagnostics(data)
	if diag.TraceID != "abc123" {
		t.Fatalf("TraceID = %q, want abc123", diag.TraceID)
	}
	if diag.ServerErrorCode != "PARAM_ERROR" {
		t.Fatalf("ServerErrorCode = %q, want PARAM_ERROR", diag.ServerErrorCode)
	}
	if diag.TechnicalDetail != "field required" {
		t.Fatalf("TechnicalDetail = %q, want 'field required'", diag.TechnicalDetail)
	}
	if diag.FriendlyHint != "请开通消息搜索权益" {
		t.Fatalf("FriendlyHint = %q, want search entitlement guidance", diag.FriendlyHint)
	}
	if diag.ActionURL != "https://example.test/enable-search" {
		t.Fatalf("ActionURL = %q, want entitlement URL", diag.ActionURL)
	}
	if diag.ServerRetryable == nil || *diag.ServerRetryable != false {
		t.Fatalf("ServerRetryable = %v, want false", diag.ServerRetryable)
	}
}

func TestExtractServerDiagnostics_CamelCase(t *testing.T) {
	t.Parallel()
	data := json.RawMessage(`{"traceId":"xyz789","errorCode":"AUTH_ERROR","friendlyHint":"request access","actionUrl":"https://example.test/request"}`)
	diag := ExtractServerDiagnostics(data)
	if diag.TraceID != "xyz789" {
		t.Fatalf("TraceID = %q, want xyz789", diag.TraceID)
	}
	if diag.ServerErrorCode != "AUTH_ERROR" {
		t.Fatalf("ServerErrorCode = %q, want AUTH_ERROR", diag.ServerErrorCode)
	}
	if diag.FriendlyHint != "request access" || diag.ActionURL != "https://example.test/request" {
		t.Fatalf("guidance = (%q, %q), want camelCase values", diag.FriendlyHint, diag.ActionURL)
	}
}

func TestExtractServerDiagnostics_Empty(t *testing.T) {
	t.Parallel()
	diag := ExtractServerDiagnostics(nil)
	if !diag.IsEmpty() {
		t.Fatal("expected empty diagnostics for nil input")
	}
	diag = ExtractServerDiagnostics(json.RawMessage(`{}`))
	if !diag.IsEmpty() {
		t.Fatal("expected empty diagnostics for empty object")
	}
}

func TestExtractServerDiagnostics_Malformed(t *testing.T) {
	t.Parallel()
	diag := ExtractServerDiagnostics(json.RawMessage(`not json`))
	if !diag.IsEmpty() {
		t.Fatal("expected empty diagnostics for malformed JSON")
	}
}

func TestExtractServerDiagnosticsFromMap(t *testing.T) {
	t.Parallel()
	content := map[string]any{
		"trace_id":         "trace-001",
		"code":             "TIMEOUT_ERROR",
		"technical_detail": "deadline exceeded",
		"retryable":        true,
	}
	diag := ExtractServerDiagnosticsFromMap(content)
	if diag.TraceID != "trace-001" {
		t.Fatalf("TraceID = %q, want trace-001", diag.TraceID)
	}
	if diag.ServerErrorCode != "TIMEOUT_ERROR" {
		t.Fatalf("ServerErrorCode = %q, want TIMEOUT_ERROR", diag.ServerErrorCode)
	}
	if diag.ServerRetryable == nil || *diag.ServerRetryable != true {
		t.Fatal("expected retryable=true")
	}
}

func TestExtractServerDiagnosticsFromMapNestedBusinessCode(t *testing.T) {
	t.Parallel()
	content := map[string]any{
		"trace_id": "trace-002",
		"code":     "BUSINESS_ERROR",
		"result": map[string]any{
			"success":       false,
			"errorCode":     "ROBOT_NOT_FOUND",
			"errorMsg":      "robot info is not exist",
			"friendly_hint": "请检查机器人是否仍在群内",
			"action_url":    "https://example.test/robots",
		},
	}
	diag := ExtractServerDiagnosticsFromMap(content)
	if diag.TraceID != "trace-002" {
		t.Fatalf("TraceID = %q, want trace-002", diag.TraceID)
	}
	if diag.ServerErrorCode != "ROBOT_NOT_FOUND" {
		t.Fatalf("ServerErrorCode = %q, want ROBOT_NOT_FOUND", diag.ServerErrorCode)
	}
	if diag.FriendlyHint != "请检查机器人是否仍在群内" || diag.ActionURL != "https://example.test/robots" {
		t.Fatalf("nested guidance = (%q, %q), want nested values", diag.FriendlyHint, diag.ActionURL)
	}
}

func TestExtractServerDiagnosticsFromMap_Empty(t *testing.T) {
	t.Parallel()
	diag := ExtractServerDiagnosticsFromMap(nil)
	if !diag.IsEmpty() {
		t.Fatal("expected empty for nil map")
	}
}

func TestExtractTraceIDFromHeaders(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		headers http.Header
		want    string
	}{
		{"x-trace-id", http.Header{"X-Trace-Id": {"abc"}}, "abc"},
		{"x-request-id", http.Header{"X-Request-Id": {"def"}}, "def"},
		{"dingtalk", http.Header{"X-Dingtalk-Trace-Id": {"ghi"}}, "ghi"},
		{"priority", http.Header{
			"X-Trace-Id":          {"first"},
			"X-Dingtalk-Trace-Id": {"second"},
		}, "first"},
		{"empty", http.Header{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ExtractTraceIDFromHeaders(tt.headers); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCoalesceStr(t *testing.T) {
	t.Parallel()
	if got := coalesceStr("", "b"); got != "b" {
		t.Fatalf("got %q, want b", got)
	}
	if got := coalesceStr("a", "b"); got != "a" {
		t.Fatalf("got %q, want a", got)
	}
	if got := coalesceStr("", ""); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}
