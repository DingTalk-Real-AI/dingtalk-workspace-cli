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

func TestExtractServerDiagnosticsFromMapNestedDiagnosticEnvelope(t *testing.T) {
	t.Parallel()
	content := map[string]any{
		"result": map[string]any{
			"data": map[string]any{
				"requestId":       "request-nested",
				"technicalDetail": "nested detail",
				"serverRetryable": false,
				"reason":          "confirmation_required",
				"status":          "failure",
			},
		},
	}
	diag := ExtractServerDiagnosticsFromMap(content)
	if diag.TraceID != "request-nested" || diag.TechnicalDetail != "nested detail" {
		t.Fatalf("nested diagnostics = %#v", diag)
	}
	if diag.ServerRetryable == nil || *diag.ServerRetryable || diag.Reason != "" || diag.Outcome != "" {
		t.Fatalf("nested error semantics = %#v", diag)
	}
}

func TestExtractServerDiagnosticsFromMapExplicitErrorDetail(t *testing.T) {
	t.Parallel()
	content := map[string]any{
		"result": map[string]any{
			"data": map[string]any{
				"requestId": "request-explicit",
				"errorDetail": map[string]any{
					"errorType":        "auth",
					"reason":           "remote_outcome_unknown",
					"friendly_hint":    "先查询状态",
					"failureStage":     "upstream_execution",
					"actions":          []any{"query_status"},
					"details":          map[string]any{"checkpoint": "v1"},
					"serverRetryable":  false,
					"executionStarted": true,
					"outcome":          "unknown",
				},
			},
		},
	}
	diag := ExtractServerDiagnosticsFromMap(content)
	if diag.TraceID != "request-explicit" || diag.ErrorType != "auth" || diag.Reason != "remote_outcome_unknown" ||
		diag.FriendlyHint != "先查询状态" || diag.FailureStage != "upstream_execution" ||
		diag.Outcome != "unknown" {
		t.Fatalf("explicit diagnostics = %#v", diag)
	}
	if diag.ServerRetryable == nil || *diag.ServerRetryable || diag.ExecutionStarted == nil || !*diag.ExecutionStarted {
		t.Fatalf("explicit booleans = %#v", diag)
	}
	if len(diag.Actions) != 1 || diag.Actions[0] != "query_status" || diag.Details["checkpoint"] != "v1" {
		t.Fatalf("explicit recovery = %#v", diag)
	}
}

func TestExtractServerDiagnosticsFromMapNestedErrorDetail(t *testing.T) {
	t.Parallel()
	content := map[string]any{
		"error": map[string]any{
			"type": "text",
			"errorDetail": map[string]any{
				"errorType":     "auth",
				"reason":        "permission_denied",
				"friendly_hint": "请检查授权",
			},
		},
	}
	diag := ExtractServerDiagnosticsFromMap(content)
	if diag.ErrorType != "auth" || diag.Reason != "permission_denied" || diag.FriendlyHint != "请检查授权" {
		t.Fatalf("nested diagnostics = %#v", diag)
	}
}

func TestCrossPlatformCoverageDiagnosticsContainerEdges(t *testing.T) {
	direct := ExtractServerDiagnosticsFromMap(map[string]any{
		"errorDetail": map[string]any{
			"errorType": "auth",
			"reason":    "confirmation_required",
		},
	})
	if direct.ErrorType != "auth" || direct.Reason != "" {
		t.Fatalf("direct diagnostics = %#v", direct)
	}
	arrayNested := ExtractServerDiagnosticsFromMap(map[string]any{
		"error": []any{
			"skip",
			map[string]any{"errorDetail": map[string]any{"errorType": "internal"}},
		},
	})
	if arrayNested.ErrorType != "internal" {
		t.Fatalf("array nested diagnostics = %#v", arrayNested)
	}
	nested := ExtractServerDiagnosticsFromMap(map[string]any{
		"result": []any{
			"skip",
			map[string]any{"data": map[string]any{
				"errorDetail": map[string]any{"errorType": "validation", "reason": "bad_input"},
			}},
		},
	})
	if nested.ErrorType != "validation" || nested.Reason != "bad_input" {
		t.Fatalf("nested diagnostics = %#v", nested)
	}
	if got := ExtractServerDiagnosticsFromMap(map[string]any{
		"type": "text", "errorDetail": map[string]any{"type": "text"},
	}); got.ErrorType != "" {
		t.Fatalf("content type became error type = %q", got.ErrorType)
	}
	if got := ExtractServerDiagnosticsFromMap(map[string]any{
		"errorDetail": map[string]any{"type": "validation"},
	}); got.ErrorType != "validation" {
		t.Fatalf("known error type = %q", got.ErrorType)
	}
	if explicitErrorDiagnosticsContainer(nil, 0) != nil || explicitErrorDiagnosticsContainer(map[string]any{}, 9) != nil {
		t.Fatal("explicit diagnostics depth boundary failed")
	}
	if !IsFrameworkReservedReason(" confirmation_required ") || IsFrameworkReservedReason("business_error") {
		t.Fatal("framework reason boundary failed")
	}
	if stringSliceFromMapRecursive(map[string]any{}, 9, "actions") != nil || mapFromMapRecursive(map[string]any{}, 9, "details") != nil {
		t.Fatal("recursive diagnostic boundaries failed")
	}
}

func TestCrossPlatformCoverageBoolDiagnosticsRecursiveEdges(t *testing.T) {
	t.Parallel()
	if value, ok := boolFromMapRecursive(nil, 0, "retryable"); ok || value {
		t.Fatalf("nil recursive bool = (%v, %v)", value, ok)
	}
	if value, ok := boolFromMapRecursive(map[string]any{"retryable": true}, 9, "retryable"); ok || value {
		t.Fatalf("over-depth recursive bool = (%v, %v)", value, ok)
	}
	content := map[string]any{
		"result": []any{
			"ignore",
			map[string]any{"data": map[string]any{"serverRetryable": true}},
		},
	}
	value, ok := boolFromMapRecursive(content, 0, "serverRetryable")
	if !ok || !value {
		t.Fatalf("nested array recursive bool = (%v, %v)", value, ok)
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

func TestCrossPlatformCoverageDiagnosticsNestedCollections(t *testing.T) {
	content := map[string]any{
		"result": []any{
			"skip",
			map[string]any{
				"code": "BUSINESS_ERROR",
				"data": map[string]any{
					"code":             "NESTED_CODE",
					"actions":          []string{"direct-action"},
					"details":          map[string]any{"nested": "value"},
					"executionStarted": true,
					"unknown":          true,
				},
			},
		},
		"error": []any{
			"skip",
			map[string]any{
				"actions": []any{"nested-action"},
				"details": map[string]any{"nested-detail": "value"},
				"unknown": true,
			},
		},
	}
	if got := serverErrorCodeFromMap(content, 0); got != "NESTED_CODE" {
		t.Fatalf("nested server code = %q", got)
	}
	diag := ExtractServerDiagnosticsFromMap(content)
	if diag.Unknown == nil || !*diag.Unknown {
		t.Fatalf("unknown diagnostic = %#v", diag.Unknown)
	}
	if stringSliceFromMapRecursive(nil, 0, "actions") != nil || stringSliceFromMapRecursive(map[string]any{}, 9, "actions") != nil {
		t.Fatal("string slice recursive boundaries failed")
	}
	if actions := stringSliceFromMapRecursive(content, 0, "actions"); len(actions) != 1 || actions[0] != "direct-action" {
		t.Fatalf("direct actions = %#v", actions)
	}
	if details := mapFromMapRecursive(content, 0, "details"); details["nested"] != "value" {
		t.Fatalf("nested details = %#v", details)
	}
	if details := mapFromMapRecursive(map[string]any{"data": []any{"skip", map[string]any{"details": map[string]any{"array": true}}}}, 0, "details"); details["array"] != true {
		t.Fatalf("array details = %#v", details)
	}
	if scalarStringFromMap(map[string]any{"value": true}, "value") != "" || scalarStringFromMap(map[string]any{"value": map[string]any{}}, "value") != "" || scalarStringFromMap(map[string]any{"value": []any{}}, "value") != "" || scalarStringFromMap(map[string]any{"value": 42}, "value") != "42" {
		t.Fatal("scalar string edge cases failed")
	}
	if mapFromMapRecursive(nil, 0, "details") != nil || mapFromMapRecursive(map[string]any{}, 9, "details") != nil {
		t.Fatal("map recursive boundary failed")
	}
}

func TestExtractServerDiagnosticsFromMapStructuredErrorDetail(t *testing.T) {
	diag := ExtractServerDiagnosticsFromMap(map[string]any{
		"success": false,
		"errorDetail": map[string]any{
			"code":             "PUBLISH_UNKNOWN",
			"reason":           "remote_outcome_unknown",
			"hint":             "先查询状态",
			"retryable":        false,
			"executionStarted": true,
			"nextStep":         "查询 versionId",
			"actions":          []any{"query_status"},
			"outcome":          "unknown",
			"contractVersion":  "opendev.appdev.v1",
			"details":          map[string]any{"checkpoint": map[string]any{"versionId": "v1"}},
		},
	})
	if diag.ServerErrorCode != "PUBLISH_UNKNOWN" || diag.Reason != "remote_outcome_unknown" ||
		diag.FriendlyHint != "先查询状态" || diag.NextCommand != "查询 versionId" ||
		diag.Outcome != "unknown" || diag.ContractVersion != "opendev.appdev.v1" {
		t.Fatalf("structured diagnostics=%#v", diag)
	}
	if diag.ServerRetryable == nil || *diag.ServerRetryable || diag.ExecutionStarted == nil || !*diag.ExecutionStarted {
		t.Fatalf("structured booleans=%#v", diag)
	}
	if len(diag.Actions) != 1 || diag.Actions[0] != "query_status" || diag.Details["checkpoint"] == nil {
		t.Fatalf("structured recovery=%#v", diag)
	}
}

func TestExtractServerDiagnosticsFromMapBusinessCategoryIsNotErrorType(t *testing.T) {
	t.Parallel()
	business := ExtractServerDiagnosticsFromMap(map[string]any{
		"errorDetail": map[string]any{
			"category": "application_scope",
			"message":  "business failure",
		},
	})
	if business.ErrorType != "" {
		t.Fatalf("business category became error type = %q", business.ErrorType)
	}
	standard := ExtractServerDiagnosticsFromMap(map[string]any{
		"errorDetail": map[string]any{
			"errorType": "auth",
		},
	})
	if standard.ErrorType != "auth" {
		t.Fatalf("explicit errorType = %q, want auth", standard.ErrorType)
	}
	contentType := ExtractServerDiagnosticsFromMap(map[string]any{
		"errorDetail": map[string]any{
			"type": "text",
		},
	})
	if contentType.ErrorType != "" {
		t.Fatalf("content type became error type = %q", contentType.ErrorType)
	}
}

func TestCrossPlatformCoverageBusinessDiagnosticsPreserveMCPFields(t *testing.T) {
	for _, category := range []string{"VALIDATION", "DOWNSTREAM", "TIMEOUT"} {
		for _, wrapped := range []bool{false, true} {
			data := map[string]any{"logId": "trace", "errorStage": "processContent", "errorCategory": category}
			if wrapped {
				data = map[string]any{"requestId": "legacy-request-trace", "result": data}
			}
			diag := ExtractServerDiagnosticsFromMap(data)
			wantTrace := "trace"
			if wrapped {
				wantTrace = "legacy-request-trace"
			}
			if diag.TraceID != wantTrace || diag.ErrorStage != "processContent" || diag.ErrorCategory != category || diag.ServerRetryable != nil {
				t.Fatalf("lost diagnostics: %#v", diag)
			}
		}
	}
	for _, category := range []string{"", "java.lang.Exception"} {
		diag := ExtractServerDiagnosticsFromMap(map[string]any{"requestId": "fallback", "errorCategory": category})
		if diag.TraceID != "fallback" || diag.ErrorCategory != "" {
			t.Fatalf("invalid diagnostics: %#v", diag)
		}
	}
}

func TestCrossPlatformCoverageDiagnosticsPreserveBusinessAndRecoveryFields(t *testing.T) {
	t.Parallel()
	diag := ExtractServerDiagnosticsFromMap(map[string]any{
		"requestId": "legacy-request-trace",
		"result": map[string]any{
			"logId":         "server-trace",
			"errorStage":    "processContent",
			"errorCategory": "DOWNSTREAM",
			"errorDetail": map[string]any{
				"code":             "PUBLISH_UNKNOWN",
				"errorType":        "api",
				"reason":           "remote_outcome_unknown",
				"stage":            "commit",
				"retryable":        false,
				"unknown":          true,
				"executionStarted": true,
				"nextCommand":      "query status",
				"actions":          []any{"query_status"},
				"details":          map[string]any{"versionId": "v1"},
			},
		},
	})
	if diag.TraceID != "legacy-request-trace" || diag.ErrorStage != "processContent" || diag.ErrorCategory != "DOWNSTREAM" {
		t.Fatalf("lost business diagnostics: %#v", diag)
	}
	if diag.ServerErrorCode != "PUBLISH_UNKNOWN" || diag.ErrorType != "api" ||
		diag.Reason != "remote_outcome_unknown" || diag.FailureStage != "commit" || diag.NextCommand != "query status" {
		t.Fatalf("lost recovery diagnostics: %#v", diag)
	}
	if diag.ServerRetryable == nil || *diag.ServerRetryable || diag.Unknown == nil || !*diag.Unknown ||
		diag.ExecutionStarted == nil || !*diag.ExecutionStarted {
		t.Fatalf("lost execution or retry semantics: %#v", diag)
	}
	if len(diag.Actions) != 1 || diag.Actions[0] != "query_status" || diag.Details["versionId"] != "v1" {
		t.Fatalf("lost recovery actions or details: %#v", diag)
	}
}

// Preserve both public entry points and the pre-mind wrapper traversal order.
func TestCrossPlatformCoverageDiagnosticsTraceIDCompatibility(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, payload, want string }{
		{"snake request before log", `{"request_id":"request","logId":"log"}`, "request"},
		{"camel request before log", `{"requestId":"request","log_id":"log"}`, "request"},
		{"trace before request", `{"traceId":"trace","request_id":"request","logId":"log"}`, "trace"},
		{"snake trace first", `{"trace_id":"snake","traceId":"camel","requestId":"request","logId":"log"}`, "snake"},
		{"outer request before nested trace", `{"requestId":"outer","result":{"traceId":"nested","logId":"log"}}`, "outer"},
		{"nested request before outer log", `{"logId":"log","result":{"request_id":"nested"}}`, "nested"},
		{"array request before log", `{"logId":"log","result":[{"requestId":"nested"}]}`, "nested"},
		{"log fallback", `{"logId":"log"}`, "log"},
		{"nested snake log fallback", `{"result":{"log_id":"log"}}`, "log"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var content map[string]any
			if err := json.Unmarshal([]byte(tc.payload), &content); err != nil {
				t.Fatal(err)
			}
			for _, diag := range []string{ExtractServerDiagnostics(json.RawMessage(tc.payload)).TraceID, ExtractServerDiagnosticsFromMap(content).TraceID} {
				if diag != tc.want {
					t.Fatalf("TraceID = %q, want %q", diag, tc.want)
				}
			}
		})
	}
}
