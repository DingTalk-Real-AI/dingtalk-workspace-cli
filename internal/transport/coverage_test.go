package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// ─── MCP JSON-RPC mock server ──────────────────────────────────────────

func newMockMCPServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func jsonRPCResponse(id int, result any) []byte {
	data, _ := json.Marshal(result)
	return []byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"result":%s}`, id, data))
}

func jsonRPCError(id, code int, msg string) []byte {
	return []byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"error":{"code":%d,"message":"%s"}}`, id, code, msg))
}

// ─── NotifyInitialized ─────────────────────────────────────────────────

func TestNotifyInitialized(t *testing.T) {
	srv := newMockMCPServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	c := NewClient(srv.Client())
	c.TrustedDomains = []string{"*"}
	err := c.NotifyInitialized(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("NotifyInitialized error: %v", err)
	}
}

// ─── ListTools ─────────────────────────────────────────────────────────

func TestListTools_Success(t *testing.T) {
	srv := newMockMCPServer(t, func(w http.ResponseWriter, r *http.Request) {
		result := ToolsListResult{Tools: []ToolDescriptor{{Name: "test-tool", Description: "A test tool"}}}
		w.Write(jsonRPCResponse(2, result))
	})
	c := NewClient(srv.Client())
	c.TrustedDomains = []string{"*"}
	result, err := c.ListTools(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("ListTools error: %v", err)
	}
	if len(result.Tools) != 1 || result.Tools[0].Name != "test-tool" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestListTools_RPCError(t *testing.T) {
	srv := newMockMCPServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(jsonRPCError(2, -32601, "method not found"))
	})
	c := NewClient(srv.Client())
	c.TrustedDomains = []string{"*"}
	_, err := c.ListTools(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected error for RPC error response")
	}
}

// ─── CallTool ──────────────────────────────────────────────────────────

func TestCallTool_Success(t *testing.T) {
	srv := newMockMCPServer(t, func(w http.ResponseWriter, r *http.Request) {
		result := map[string]any{
			"content": []any{map[string]any{"type": "text", "text": `{"result":"ok"}`}},
		}
		w.Write(jsonRPCResponse(3, result))
	})
	c := NewClient(srv.Client())
	c.TrustedDomains = []string{"*"}
	result, err := c.CallTool(context.Background(), srv.URL, "test-tool", map[string]any{"key": "val"})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result.Content == nil {
		t.Fatal("expected non-nil content")
	}
}

func TestCallTool_InvalidParams(t *testing.T) {
	srv := newMockMCPServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(jsonRPCError(3, -32602, "invalid params"))
	})
	c := NewClient(srv.Client())
	c.TrustedDomains = []string{"*"}
	_, err := c.CallTool(context.Background(), srv.URL, "test-tool", nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

// ─── parseRetryAfter ───────────────────────────────────────────────────

func TestParseRetryAfter(t *testing.T) {
	t.Parallel()
	tests := []struct {
		raw     string
		wantOK  bool
		wantPos bool // expect positive duration
	}{
		{"", false, false},
		{"5", true, true},
		{"0", false, false},
		{"not-a-number", false, false},
	}
	for _, tt := range tests {
		d, ok := parseRetryAfter(tt.raw)
		if ok != tt.wantOK {
			t.Errorf("parseRetryAfter(%q) ok=%v, want %v", tt.raw, ok, tt.wantOK)
		}
		if tt.wantPos && d <= 0 {
			t.Errorf("parseRetryAfter(%q) duration=%v, expected positive", tt.raw, d)
		}
	}
}

// ─── jsonrpcCodeLabel ──────────────────────────────────────────────────

func TestJsonrpcCodeLabel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		code int
		want string
	}{
		{-32700, "parse_error"},
		{-32600, "invalid_request"},
		{-32601, "method_not_found"},
		{-32602, "invalid_params"},
		{-32603, "internal_error"},
		{-32000, "server_error_32000"},
		{500, "error_500"},
	}
	for _, tt := range tests {
		if got := jsonrpcCodeLabel(tt.code); got != tt.want {
			t.Errorf("jsonrpcCodeLabel(%d) = %s, want %s", tt.code, got, tt.want)
		}
	}
}

// ─── looksAuthRelated ──────────────────────────────────────────────────

func TestLooksAuthRelated(t *testing.T) {
	t.Parallel()
	tests := []struct {
		msg  string
		want bool
	}{
		{"Unauthorized", true},
		{"access denied", true},
		{"permission error", true},
		{"token expired", true},
		{"normal error", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := looksAuthRelated(tt.msg); got != tt.want {
			t.Errorf("looksAuthRelated(%q) = %v, want %v", tt.msg, got, tt.want)
		}
	}
}

// ─── looksAuthRPCError ─────────────────────────────────────────────────

func TestLooksAuthRPCError(t *testing.T) {
	t.Parallel()
	if looksAuthRPCError(nil) {
		t.Fatal("nil should not be auth error")
	}
	if !looksAuthRPCError(&RPCError{Code: 401, Message: "auth"}) {
		t.Fatal("401 should be auth error")
	}
	if !looksAuthRPCError(&RPCError{Code: 403, Message: "forbidden"}) {
		t.Fatal("403 should be auth error")
	}
	if !looksAuthRPCError(&RPCError{Code: -32000, Message: "token expired"}) {
		t.Fatal("message with 'token' should be auth error")
	}
	if looksAuthRPCError(&RPCError{Code: -32000, Message: "timeout"}) {
		t.Fatal("generic error should not be auth error")
	}
}

// ─── jsonrpcEnvelopeError ──────────────────────────────────────────────

func TestJsonrpcEnvelopeError_InvalidParams(t *testing.T) {
	t.Parallel()
	err := jsonrpcEnvelopeError("tools/call", &RPCError{Code: -32602, Message: "invalid params"}, "", "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestJsonrpcEnvelopeError_AuthError(t *testing.T) {
	t.Parallel()
	err := jsonrpcEnvelopeError("tools/call", &RPCError{Code: 401, Message: "Unauthorized"}, "", "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestJsonrpcEnvelopeError_MethodNotFound(t *testing.T) {
	t.Parallel()
	err := jsonrpcEnvelopeError("tools/call", &RPCError{Code: -32601, Message: "method not found"}, "/tmp/snap", "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestJsonrpcEnvelopeError_GenericToolError(t *testing.T) {
	t.Parallel()
	err := jsonrpcEnvelopeError("tools/call", &RPCError{Code: -32000, Message: "server error"}, "", "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestJsonrpcEnvelopeError_DiscoveryMethod(t *testing.T) {
	t.Parallel()
	err := jsonrpcEnvelopeError("initialize", &RPCError{Code: -32000, Message: "failed"}, "", "")
	if err == nil {
		t.Fatal("expected error")
	}
}

// ─── reasonForMethod ───────────────────────────────────────────────────

func TestReasonForMethod(t *testing.T) {
	t.Parallel()
	tests := []struct {
		method, suffix, want string
	}{
		{"tools/call", "timeout", "tools_call_timeout"},
		{"", "error", "jsonrpc_error"},
		{"my-method", "fail", "my_method_fail"},
	}
	for _, tt := range tests {
		if got := reasonForMethod(tt.method, tt.suffix); got != tt.want {
			t.Errorf("reasonForMethod(%q, %q) = %q, want %q", tt.method, tt.suffix, got, tt.want)
		}
	}
}

// ─── doWithRetry (via CallTool) ────────────────────────────────────────

func TestCallTool_DoesNotRetryOn502(t *testing.T) {
	attempts := 0
	srv := newMockMCPServer(t, func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadGateway)
	})
	c := NewClient(srv.Client())
	c.TrustedDomains = []string{"*"}
	c.MaxRetries = 3
	c.RetryDelay = time.Millisecond
	c.RetryMaxDelay = 10 * time.Millisecond
	c.sleep = func(ctx context.Context, d time.Duration) error { return nil }

	_, err := c.CallTool(context.Background(), srv.URL, "test", nil)
	if err == nil {
		t.Fatal("expected 502 error")
	}
	if attempts != 1 {
		t.Fatalf("tools/call attempts=%d, want 1", attempts)
	}
}

// ─── httpStatusError ───────────────────────────────────────────────────

func TestHttpStatusError_AllCodes(t *testing.T) {
	t.Parallel()
	for _, code := range []int{400, 401, 403, 404, 429, 500, 502, 503} {
		err := httpStatusError("tools/call", "https://api.example.com", code, "", "")
		if err == nil {
			t.Fatalf("expected error for status %d", code)
		}
	}
}

func TestHttpStatusError_WithSnapshot(t *testing.T) {
	t.Parallel()
	err := httpStatusError("initialize", "https://api.example.com", 500, "/tmp/snap.json", "")
	if err == nil {
		t.Fatal("expected error")
	}
}

// ─── parseRetryAfter 钳制上限参数化（B197） ─────────────────────────────

// TestParseRetryAfterWithMaxClamps 验证 parseRetryAfterWithMax 的新增上限参数：
// 服务端 Retry-After 超过 maxDelay 时被钳制到 maxDelay；maxDelay<=0 时保持
// 原值（默认行为零变化，与 parseRetryAfter 一致）。
func TestParseRetryAfterWithMaxClamps(t *testing.T) {
	t.Parallel()

	t.Run("seconds clamps to max", func(t *testing.T) {
		d, ok := parseRetryAfterWithMax("30", 5*time.Second)
		if !ok || d != 5*time.Second {
			t.Fatalf("parseRetryAfterWithMax(30s, 5s) = %v/%v, want 5s/true", d, ok)
		}
	})
	t.Run("seconds below max preserved", func(t *testing.T) {
		d, ok := parseRetryAfterWithMax("3", 5*time.Second)
		if !ok || d != 3*time.Second {
			t.Fatalf("parseRetryAfterWithMax(3s, 5s) = %v/%v, want 3s/true", d, ok)
		}
	})
	t.Run("zero max preserves original value", func(t *testing.T) {
		// maxDelay=0 表示不钳制：与 parseRetryAfter 行为一致（B197 默认值零变化）。
		d, ok := parseRetryAfterWithMax("120", 0)
		if !ok || d != 120*time.Second {
			t.Fatalf("parseRetryAfterWithMax(120s, 0) = %v/%v, want 120s/true", d, ok)
		}
	})
	t.Run("http date clamps to max", func(t *testing.T) {
		future := time.Now().Add(10 * time.Minute).UTC().Format(http.TimeFormat)
		d, ok := parseRetryAfterWithMax(future, 2*time.Second)
		if !ok || d != 2*time.Second {
			t.Fatalf("parseRetryAfterWithMax(in 10m, 2s) = %v/%v, want 2s/true", d, ok)
		}
	})
	t.Run("invalid raw rejected", func(t *testing.T) {
		if _, ok := parseRetryAfterWithMax("not-a-number", 5*time.Second); ok {
			t.Fatal("invalid raw must be rejected")
		}
	})
}

// TestParseRetryAfterLegacyUnchanged 验证 parseRetryAfter 原签名行为零变化
// （B197 默认值不变）：委托 parseRetryAfterWithMax(raw, 0) 不引入钳制。
func TestParseRetryAfterLegacyUnchanged(t *testing.T) {
	t.Parallel()
	d, ok := parseRetryAfter("120")
	if !ok || d != 120*time.Second {
		t.Fatalf("parseRetryAfter(120) = %v/%v, want 120s/true", d, ok)
	}
}
