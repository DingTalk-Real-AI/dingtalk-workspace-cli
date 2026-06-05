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

package transport

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func TestInitializeNegotiatesProtocolVersion(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		params := req["params"].(map[string]any)
		version := params["protocolVersion"].(string)
		if version == "2025-03-26" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      1,
				"error": map[string]any{
					"code":    -32600,
					"message": "unsupported",
				},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"result": map[string]any{
				"protocolVersion": version,
				"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
				"serverInfo":      map[string]any{"name": "demo", "version": "1.0.0"},
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.Client())
	result, err := client.Initialize(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if result.ProtocolVersion != "2024-11-05" {
		t.Fatalf("Initialize() selected %q, want 2024-11-05", result.ProtocolVersion)
	}
}

func TestInitializeShortCircuitsOnHTTPError(t *testing.T) {
	t.Parallel()

	// Track which protocol versions actually get sent. With the short-circuit
	// in place, a transport-layer (HTTP) failure should fail Initialize on the
	// FIRST version without iterating through every supported version. Without
	// the short-circuit, three round-trips would happen — needlessly tripling
	// every CLI startup when a plugin endpoint is broken (issue #119).
	var seenVersions []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		params := req["params"].(map[string]any)
		seenVersions = append(seenVersions, params["protocolVersion"].(string))
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.Client())
	client.MaxRetries = 0 // skip the HTTP retry loop — we only care about version iteration

	if _, err := client.Initialize(context.Background(), server.URL); err == nil {
		t.Fatal("Initialize() error = nil, want HTTP error")
	}

	if len(seenVersions) != 1 {
		t.Fatalf("Initialize() attempted %d protocol versions (%v), want 1 — HTTP failures must short-circuit",
			len(seenVersions), seenVersions)
	}
}

func TestInitializeShortCircuitsOnDialFailure(t *testing.T) {
	t.Parallel()

	// Bind to an ephemeral port, then close the listener. Subsequent connects
	// to that address fail with "connection refused" almost instantly. With
	// the short-circuit, three protocol versions would otherwise stack three
	// dial-error returns; we only want one — and we want Initialize to return
	// well under the per-dial budget.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	endpoint := "http://" + ln.Addr().String()
	_ = ln.Close()

	client := NewClient(nil)
	client.MaxRetries = 0

	start := time.Now()
	if _, err := client.Initialize(context.Background(), endpoint); err == nil {
		t.Fatal("Initialize() error = nil, want dial failure")
	}
	elapsed := time.Since(start)

	// Three dial attempts (one per supported version) on a refused-connection
	// path is still fast on loopback, so this assertion is a sanity bound, not
	// the primary signal — but if the short-circuit regresses, on a real
	// unreachable address this jumps from one dial timeout to three.
	if elapsed > 2*time.Second {
		t.Fatalf("Initialize() took %v, want <2s — dial failure should short-circuit", elapsed)
	}
}

func TestListToolsRetriesOnServerError(t *testing.T) {
	t.Parallel()

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      2,
			"result": map[string]any{
				"tools": []map[string]any{
					{
						"name":        "create_document",
						"title":       "创建文档",
						"description": "创建文档",
						"inputSchema": map[string]any{"type": "object"},
					},
				},
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.Client())
	result, err := client.ListTools(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if attempts != 2 {
		t.Fatalf("ListTools() attempts = %d, want 2", attempts)
	}
	if len(result.Tools) != 1 {
		t.Fatalf("ListTools() len = %d, want 1", len(result.Tools))
	}
}

func TestListToolsUsesExponentialBackoffWithCap(t *testing.T) {
	t.Parallel()

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts <= 3 {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      2,
			"result":  map[string]any{"tools": []map[string]any{}},
		})
	}))
	defer server.Close()

	var delays []time.Duration
	client := NewClient(server.Client())
	client.MaxRetries = 3
	client.RetryDelay = 10 * time.Millisecond
	client.RetryMaxDelay = 25 * time.Millisecond
	client.sleep = func(_ context.Context, delay time.Duration) error {
		delays = append(delays, delay)
		return nil
	}

	if _, err := client.ListTools(context.Background(), server.URL); err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}

	want := []time.Duration{10 * time.Millisecond, 20 * time.Millisecond, 25 * time.Millisecond}
	if !reflect.DeepEqual(delays, want) {
		t.Fatalf("retry delays = %#v, want %#v", delays, want)
	}
}

func TestListToolsHonorsRetryAfterHeader(t *testing.T) {
	t.Parallel()

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "3")
			http.Error(w, "rate limited", http.StatusTooManyRequests)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      2,
			"result":  map[string]any{"tools": []map[string]any{}},
		})
	}))
	defer server.Close()

	var delays []time.Duration
	client := NewClient(server.Client())
	client.MaxRetries = 1
	client.RetryDelay = 10 * time.Millisecond
	client.RetryMaxDelay = 5 * time.Second
	client.sleep = func(_ context.Context, delay time.Duration) error {
		delays = append(delays, delay)
		return nil
	}

	if _, err := client.ListTools(context.Background(), server.URL); err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}

	want := []time.Duration{3 * time.Second}
	if !reflect.DeepEqual(delays, want) {
		t.Fatalf("retry delays = %#v, want %#v", delays, want)
	}
}

func TestCallToolUsesJSONRPCMethod(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req["method"] != "tools/call" {
			t.Fatalf("method = %#v, want tools/call", req["method"])
		}
		params := req["params"].(map[string]any)
		if params["name"] != "create_document" {
			t.Fatalf("tool name = %#v, want create_document", params["name"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      3,
			"result": map[string]any{
				"content": map[string]any{
					"documentId": "doc-123",
				},
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.Client())
	result, err := client.CallTool(context.Background(), server.URL, "create_document", map[string]any{"title": "Quarterly Report"})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if result.Content["documentId"] != "doc-123" {
		t.Fatalf("CallTool() content = %#v", result.Content)
	}
}

func TestCallToolInjectsAuthHeaders(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("Authorization header = %q, want Bearer test-token", got)
		}
		if got := r.Header.Get("x-user-access-token"); got != "test-token" {
			t.Fatalf("x-user-access-token header = %q, want test-token", got)
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Fatalf("Accept header = %q, want application/json", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      3,
			"result": map[string]any{
				"content": map[string]any{
					"ok": true,
				},
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.Client())
	client.AuthToken = "test-token"
	client.TrustedDomains = []string{"127.0.0.1"}

	result, err := client.CallTool(context.Background(), server.URL, "create_document", map[string]any{"title": "Quarterly Report"})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if result.Content["ok"] != true {
		t.Fatalf("CallTool() content = %#v, want ok=true", result.Content)
	}
}

func TestCallToolAcceptsStructuredContentResults(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      3,
			"result": map[string]any{
				"content": []map[string]any{
					{
						"type": "text",
						"text": `{"ignored":true}`,
					},
				},
				"structuredContent": map[string]any{
					"success": true,
					"result": map[string]any{
						"documentId": "doc-structured",
					},
				},
				"isError": false,
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.Client())
	result, err := client.CallTool(context.Background(), server.URL, "create_document", map[string]any{"title": "Quarterly Report"})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if result.Content["success"] != true {
		t.Fatalf("success = %#v, want true", result.Content["success"])
	}
	payload, ok := result.Content["result"].(map[string]any)
	if !ok {
		t.Fatalf("result.Content[result] = %#v, want object", result.Content["result"])
	}
	if payload["documentId"] != "doc-structured" {
		t.Fatalf("documentId = %#v, want doc-structured", payload["documentId"])
	}
}

func TestCallToolClassifiesUnauthorizedHTTPAsAuthError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()

	root := t.TempDir()
	client := NewClient(server.Client())
	client.MaxRetries = 0
	client.SnapshotRecorder = testSnapshotRecorder{root: root}

	_, err := client.CallTool(context.Background(), server.URL, "create_document", map[string]any{"title": "Quarterly Report"})
	if err == nil {
		t.Fatal("CallTool() error = nil, want auth error")
	}

	var typed *apperrors.Error
	if !stderrors.As(err, &typed) {
		t.Fatalf("CallTool() error = %T, want *errors.Error", err)
	}
	if typed.Category != apperrors.CategoryAuth {
		t.Fatalf("category = %q, want auth", typed.Category)
	}
	if typed.Reason != "http_401" {
		t.Fatalf("reason = %q, want http_401", typed.Reason)
	}
	if typed.Snapshot == "" {
		t.Fatal("snapshot path should not be empty")
	}
}

func TestCallToolClassifiesForbiddenHTTPAsAuthError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer server.Close()

	client := NewClient(server.Client())
	client.MaxRetries = 0

	_, err := client.CallTool(context.Background(), server.URL, "create_document", map[string]any{"title": "Quarterly Report"})
	if err == nil {
		t.Fatal("CallTool() error = nil, want auth error")
	}

	var typed *apperrors.Error
	if !stderrors.As(err, &typed) {
		t.Fatalf("CallTool() error = %T, want *errors.Error", err)
	}
	if typed.Category != apperrors.CategoryAuth {
		t.Fatalf("category = %q, want auth", typed.Category)
	}
	if typed.Reason != "http_403" {
		t.Fatalf("reason = %q, want http_403", typed.Reason)
	}
}

func TestCallToolClassifiesJSONRPCEnvelopeErrorsAsAPIErrors(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      3,
			"error": map[string]any{
				"code":    -32603,
				"message": "upstream timeout",
			},
		})
	}))
	defer server.Close()

	root := t.TempDir()
	client := NewClient(server.Client())
	client.SnapshotRecorder = testSnapshotRecorder{root: root}

	_, err := client.CallTool(context.Background(), server.URL, "create_document", map[string]any{"title": "Quarterly Report"})
	if err == nil {
		t.Fatal("CallTool() error = nil, want api error")
	}

	var typed *apperrors.Error
	if !stderrors.As(err, &typed) {
		t.Fatalf("CallTool() error = %T, want *errors.Error", err)
	}
	if typed.Category != apperrors.CategoryAPI {
		t.Fatalf("category = %q, want api", typed.Category)
	}
	if typed.Reason != "tools_call_jsonrpc_internal_error" {
		t.Fatalf("reason = %q, want tools_call_jsonrpc_internal_error", typed.Reason)
	}
	if typed.Snapshot == "" {
		t.Fatal("snapshot path should not be empty")
	}
}

func TestCallToolClassifiesJSONRPCInvalidParamsAsValidationError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      3,
			"error": map[string]any{
				"code":    -32602,
				"message": "invalid arguments",
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.Client())

	_, err := client.CallTool(context.Background(), server.URL, "create_document", map[string]any{"title": "Quarterly Report"})
	if err == nil {
		t.Fatal("CallTool() error = nil, want validation error")
	}

	var typed *apperrors.Error
	if !stderrors.As(err, &typed) {
		t.Fatalf("CallTool() error = %T, want *errors.Error", err)
	}
	if typed.Category != apperrors.CategoryValidation {
		t.Fatalf("category = %q, want validation", typed.Category)
	}
	if typed.Reason != "tools_call_jsonrpc_invalid_params" {
		t.Fatalf("reason = %q, want tools_call_jsonrpc_invalid_params", typed.Reason)
	}
}

func TestSanitizeJSONRPCEndpointPreservesDingTalkMCPGatewayQuery(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		endpoint string
		want     string
	}{
		{
			name:     "prepub gateway",
			endpoint: "https://pre-mcp-gw.dingtalk.com/server/demo?key=secret#frag",
			want:     "https://pre-mcp-gw.dingtalk.com/server/demo?key=secret",
		},
		{
			name:     "prod gateway",
			endpoint: "https://mcp-gw.dingtalk.com/server/demo?key=secret#frag",
			want:     "https://mcp-gw.dingtalk.com/server/demo?key=secret",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := sanitizeJSONRPCEndpoint(tt.endpoint); got != tt.want {
				t.Fatalf("sanitizeJSONRPCEndpoint() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSanitizeJSONRPCEndpointStripsQueryForOtherHosts(t *testing.T) {
	t.Parallel()

	got := sanitizeJSONRPCEndpoint("https://example.com/server/demo?admin=true#frag")
	want := "https://example.com/server/demo"
	if got != want {
		t.Fatalf("sanitizeJSONRPCEndpoint() = %q, want %q", got, want)
	}
}

func TestSanitizeJSONRPCEndpointStripsQueryForHTTPGateway(t *testing.T) {
	t.Parallel()

	got := sanitizeJSONRPCEndpoint("http://pre-mcp-gw.dingtalk.com/server/demo?key=secret#frag")
	want := "http://pre-mcp-gw.dingtalk.com/server/demo"
	if got != want {
		t.Fatalf("sanitizeJSONRPCEndpoint() = %q, want %q", got, want)
	}
}

type testSnapshotRecorder struct {
	root string
}

func (r testSnapshotRecorder) RecordJSONRPC(method, endpoint string, requestBody, responseBody []byte) string {
	path := filepath.Join(r.root, "snapshot.json")
	_ = os.WriteFile(path, responseBody, 0o644)
	return path
}

func TestCallToolPreservesJSONRPCErrorData(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      3,
			"error": map[string]any{
				"code":    -32602,
				"message": "invalid arguments",
				"data": map[string]any{
					"field": "base_id",
					"error": "required",
				},
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.Client())

	_, err := client.CallTool(context.Background(), server.URL, "create_document", map[string]any{"title": "Quarterly Report"})
	if err == nil {
		t.Fatal("CallTool() error = nil, want validation error")
	}

	var typed *apperrors.Error
	if !stderrors.As(err, &typed) {
		t.Fatalf("CallTool() error = %T, want *errors.Error", err)
	}
	if typed.RPCCode != -32602 {
		t.Fatalf("RPCCode = %d, want -32602", typed.RPCCode)
	}
	if len(typed.RPCData) == 0 {
		t.Fatal("RPCData should not be empty")
	}
	var data map[string]any
	if jsonErr := json.Unmarshal(typed.RPCData, &data); jsonErr != nil {
		t.Fatalf("json.Unmarshal(RPCData) error = %v", jsonErr)
	}
	if data["field"] != "base_id" {
		t.Fatalf("RPCData.field = %#v, want base_id", data["field"])
	}
}
