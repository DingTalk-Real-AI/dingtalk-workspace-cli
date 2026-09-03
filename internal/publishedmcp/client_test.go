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

package publishedmcp

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

func TestCrossPlatformCoverageClientInvokeValidatedUsesAuthenticatedDiscoveryAndCall(t *testing.T) {
	t.Setenv("DWS_ALLOW_HTTP_ENDPOINTS", "1")

	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.Header.Get("x-user-access-token"); got != "test-token" {
			t.Errorf("x-user-access-token = %q", got)
		}
		if got := r.Header.Get("x-identity-id"); got != "identity-1" {
			t.Errorf("x-identity-id = %q", got)
		}

		var request struct {
			ID     int            `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		methods = append(methods, request.Method)
		w.Header().Set("Content-Type", "application/json")

		switch request.Method {
		case "tools/list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"result": map[string]any{
					"tools": []map[string]any{{
						"name":        "search",
						"description": "Search records",
						"inputSchema": map[string]any{"type": "object"},
					}},
				},
			})
		case "tools/call":
			if request.Params["name"] != "search" {
				t.Errorf("tool name = %#v", request.Params["name"])
			}
			arguments, _ := request.Params["arguments"].(map[string]any)
			if arguments["query"] != "example" {
				t.Errorf("arguments = %#v", arguments)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"result": map[string]any{
					"content": []map[string]any{{"type": "text", "text": "ok"}},
				},
			})
		default:
			t.Errorf("unexpected method %q", request.Method)
			http.Error(w, "unexpected method", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	base := transport.NewClient(server.Client())
	base.TrustedDomains = []string{"127.0.0.1"}
	client := New(base, "test-token", map[string]string{"x-identity-id": "identity-1"})

	result, err := client.InvokeValidated(t.Context(), server.URL, "search", map[string]any{"query": "example"})
	if err != nil {
		t.Fatalf("InvokeValidated() error = %v", err)
	}
	if len(result.Result.Blocks) != 1 || result.Result.Blocks[0].Text != "ok" {
		t.Fatalf("result blocks = %#v", result.Result.Blocks)
	}
	if len(methods) != 2 || methods[0] != "tools/list" || methods[1] != "tools/call" {
		t.Fatalf("methods = %#v", methods)
	}
	if result.InputSchemaValidation != "fresh_core_subset_snapshot" || len(result.InputSchemaDigest) != 64 {
		t.Fatalf("validation evidence = %#v", result)
	}
}

func TestCrossPlatformCoverageNewCreatesDefaultTransport(t *testing.T) {
	client := New(nil, "", nil)
	if client == nil || client.transport == nil {
		t.Fatal("New(nil, ...) returned an incomplete client")
	}
}

func TestCrossPlatformCoverageClientRejectsUntrustedEndpointBeforeSendingHeaders(t *testing.T) {
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests++
	}))
	defer server.Close()

	client := New(transport.NewClient(server.Client()), "secret-token", map[string]string{"x-identity-secret": "secret"})
	if _, err := client.Tools(t.Context(), server.URL); err == nil {
		t.Fatal("untrusted endpoint accepted")
	}
	if requests != 0 {
		t.Fatalf("requests = %d, want zero", requests)
	}
}

func TestCrossPlatformCoverageClientToolsAggregatesPages(t *testing.T) {
	t.Setenv("DWS_ALLOW_HTTP_ENDPOINTS", "1")

	var cursors []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     int               `json:"id"`
			Params map[string]string `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		cursor := request.Params["cursor"]
		cursors = append(cursors, cursor)
		result := map[string]any{
			"tools":      []map[string]any{{"name": "first", "inputSchema": map[string]any{"type": "object"}}},
			"nextCursor": " page-2 ",
		}
		if cursor == " page-2 " {
			result = map[string]any{
				"tools": []map[string]any{{"name": "second", "inputSchema": map[string]any{"type": "object"}}},
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
	}))
	defer server.Close()

	base := transport.NewClient(server.Client())
	base.TrustedDomains = []string{"127.0.0.1"}
	result, err := New(base, "", nil).Tools(t.Context(), server.URL)
	if err != nil {
		t.Fatalf("Tools() error = %v", err)
	}
	if len(result.Tools) != 2 || result.Tools[0].Name != "first" || result.Tools[1].Name != "second" {
		t.Fatalf("Tools() = %#v", result.Tools)
	}
	if len(cursors) != 2 || cursors[0] != "" || cursors[1] != " page-2 " {
		t.Fatalf("tools/list cursors = %#v", cursors)
	}
}

func TestCrossPlatformCoverageInvokeValidatedFindsLaterPageAndDigestsValidatedSnapshot(t *testing.T) {
	t.Setenv("DWS_ALLOW_HTTP_ENDPOINTS", "1")
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     int            `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		methods = append(methods, request.Method)
		var result any
		switch request.Method {
		case "tools/list":
			if request.Params["cursor"] == "later" {
				result = map[string]any{"tools": []map[string]any{{
					"name": "target",
					"inputSchema": map[string]any{
						"type": "object", "required": []any{"query"},
						"properties": map[string]any{"query": map[string]any{"type": "string"}},
					},
				}}}
			} else {
				result = map[string]any{
					"tools":      []map[string]any{{"name": "first", "inputSchema": map[string]any{"type": "object"}}},
					"nextCursor": "later",
				}
			}
		case "tools/call":
			result = map[string]any{"content": map[string]any{"ok": true}}
		default:
			t.Fatalf("unexpected method %q", request.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
	}))
	defer server.Close()

	base := transport.NewClient(server.Client())
	base.TrustedDomains = []string{"127.0.0.1"}
	result, err := New(base, "", nil).InvokeValidated(t.Context(), server.URL, "target", map[string]any{"query": "value"})
	if err != nil {
		t.Fatalf("InvokeValidated() error = %v", err)
	}
	wantDigest := fmt.Sprintf("%x", sha256.Sum256([]byte(`{"properties":{"query":{"type":"string"}},"required":["query"],"type":"object"}`)))
	if result.InputSchemaDigest != wantDigest {
		t.Fatalf("InputSchemaDigest = %q, want %q", result.InputSchemaDigest, wantDigest)
	}
	if got := strings.Join(methods, ","); got != "tools/list,tools/list,tools/call" {
		t.Fatalf("methods = %q", got)
	}
}

func TestCrossPlatformCoverageInvokeValidatedRejectsUnsafeDiscoveryWithoutCall(t *testing.T) {
	t.Setenv("DWS_ALLOW_HTTP_ENDPOINTS", "1")
	tests := []struct {
		name       string
		tools      []map[string]any
		tool       string
		wantReason string
	}{
		{name: "missing tool", tools: []map[string]any{{"name": "other", "inputSchema": map[string]any{"type": "object"}}}, tool: "target", wantReason: "published_mcp_tool_not_found"},
		{name: "near name is not exact", tools: []map[string]any{{"name": " target ", "inputSchema": map[string]any{"type": "object"}}}, tool: "target", wantReason: "published_mcp_tool_not_found"},
		{name: "duplicate tool", tools: []map[string]any{{"name": "target", "inputSchema": map[string]any{"type": "object"}}, {"name": "target", "inputSchema": map[string]any{"type": "object"}}}, tool: "target", wantReason: "published_mcp_tool_ambiguous"},
		{name: "missing schema", tools: []map[string]any{{"name": "target"}}, tool: "target", wantReason: "published_mcp_input_schema_unavailable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callRequests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					ID     int    `json:"id"`
					Method string `json:"method"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Fatalf("decode request: %v", err)
				}
				if request.Method == "tools/call" {
					callRequests++
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"jsonrpc": "2.0", "id": request.ID, "result": map[string]any{"tools": tt.tools},
				})
			}))
			defer server.Close()

			_, err := New(transport.NewClient(server.Client()), "", nil).InvokeValidated(t.Context(), server.URL, tt.tool, map[string]any{})
			var typed *apperrors.Error
			if !errors.As(err, &typed) || typed.Reason != tt.wantReason {
				t.Fatalf("error = %#v, want reason %q", err, tt.wantReason)
			}
			if callRequests != 0 {
				t.Fatalf("tools/call requests = %d, want zero", callRequests)
			}
		})
	}
}

func TestCrossPlatformCoverageInvokeValidatedRejectsInvalidSchemaOrArgumentsWithoutCall(t *testing.T) {
	t.Setenv("DWS_ALLOW_HTTP_ENDPOINTS", "1")
	tests := []struct {
		name      string
		schema    map[string]any
		arguments map[string]any
		want      string
	}{
		{name: "unsupported schema", schema: map[string]any{"type": "object", "oneOf": []any{}}, arguments: map[string]any{}, want: `unsupported JSON Schema keyword "oneOf"`},
		{name: "invalid arguments", schema: map[string]any{"type": "object", "required": []any{"id"}}, arguments: map[string]any{}, want: "$.id is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callRequests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					ID     int    `json:"id"`
					Method string `json:"method"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Fatalf("decode request: %v", err)
				}
				if request.Method == "tools/call" {
					callRequests++
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"jsonrpc": "2.0", "id": request.ID,
					"result": map[string]any{"tools": []map[string]any{{"name": "target", "inputSchema": tt.schema}}},
				})
			}))
			defer server.Close()
			_, err := New(transport.NewClient(server.Client()), "", nil).InvokeValidated(t.Context(), server.URL, "target", tt.arguments)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("InvokeValidated() error = %v, want %q", err, tt.want)
			}
			if callRequests != 0 {
				t.Fatalf("tools/call requests = %d, want zero", callRequests)
			}
		})
	}
}

func TestCrossPlatformCoverageAppendToolPageRejectsAggregateLimit(t *testing.T) {
	aggregate := transport.ToolsListResult{}
	page := transport.ToolsListResult{
		Tools:          []transport.ToolDescriptor{{Name: "search", Description: "large"}},
		RawResultBytes: 2,
	}

	if _, err := appendToolPage(&aggregate, page, 0, 1, 1024, 10); err == nil || !strings.Contains(err.Error(), "aggregate safety limit") {
		t.Fatalf("appendToolPage() error = %v, want aggregate safety limit", err)
	}
	if len(aggregate.Tools) != 0 {
		t.Fatalf("appendToolPage() retained tools after rejection: %#v", aggregate.Tools)
	}
}

func TestCrossPlatformCoverageAppendToolPageRejectsMissingRawMeasurement(t *testing.T) {
	aggregate := transport.ToolsListResult{}
	page := transport.ToolsListResult{Tools: []transport.ToolDescriptor{{Name: "search"}}}

	if _, err := appendToolPage(&aggregate, page, 0, 1024, 1024, 10); err == nil || !strings.Contains(err.Error(), "raw byte measurement") {
		t.Fatalf("appendToolPage() error = %v, want raw byte measurement error", err)
	}
}

func TestCrossPlatformCoverageAppendToolPageAccountsFullResponseBytes(t *testing.T) {
	page := transport.ToolsListResult{
		Tools:            []transport.ToolDescriptor{{Name: "search"}},
		RawResultBytes:   1,
		RawResponseBytes: 3,
	}
	if _, err := appendToolPage(&transport.ToolsListResult{}, page, 0, 2, 1024, 10); err == nil || !strings.Contains(err.Error(), "aggregate safety limit") {
		t.Fatalf("appendToolPage() error = %v, want full-response byte limit", err)
	}
}

func TestCrossPlatformCoverageClientToolsRejectsPageAndAggregateLimits(t *testing.T) {
	t.Setenv("DWS_ALLOW_HTTP_ENDPOINTS", "1")

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      2,
			"result": map[string]any{
				"tools":      []map[string]any{},
				"nextCursor": fmt.Sprintf("cursor-%d", requests),
			},
		})
	}))
	defer server.Close()

	base := transport.NewClient(server.Client())
	base.TrustedDomains = []string{"127.0.0.1"}
	client := New(base, "", nil)
	if _, err := client.tools(t.Context(), server.URL, 1, 1024); err == nil || !strings.Contains(err.Error(), "1-page safety limit") {
		t.Fatalf("tools() error = %v, want page safety limit", err)
	}
	if _, err := client.tools(t.Context(), server.URL, 2, 1); err == nil || !strings.Contains(err.Error(), "aggregate safety limit") {
		t.Fatalf("tools() error = %v, want aggregate safety limit", err)
	}
}

func TestCrossPlatformCoverageClientToolsReturnsTransportFailure(t *testing.T) {
	client := New(transport.NewClient(nil), "", nil)
	if _, err := client.tools(t.Context(), "://invalid", 1, 1024); err == nil {
		t.Fatal("tools() error = nil, want transport failure")
	}
}

func TestCrossPlatformCoverageClientToolsRejectsRepeatedCursor(t *testing.T) {
	t.Setenv("DWS_ALLOW_HTTP_ENDPOINTS", "1")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID int `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      request.ID,
			"result": map[string]any{
				"tools":      []map[string]any{},
				"nextCursor": "same",
			},
		})
	}))
	defer server.Close()

	base := transport.NewClient(server.Client())
	base.TrustedDomains = []string{"127.0.0.1"}
	_, err := New(base, "", nil).Tools(t.Context(), server.URL)
	if err == nil || !strings.Contains(err.Error(), "repeated cursor") {
		t.Fatalf("Tools() error = %v, want repeated cursor error", err)
	}
}

func TestCrossPlatformCoverageClientToolsCountsIgnoredResultFieldsAgainstByteLimit(t *testing.T) {
	t.Setenv("DWS_ALLOW_HTTP_ENDPOINTS", "1")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      2,
			"result": map[string]any{
				"tools":    []map[string]any{},
				"metadata": strings.Repeat("x", 2048),
			},
		})
	}))
	defer server.Close()

	client := New(transport.NewClient(server.Client()), "", nil)
	if _, err := client.tools(t.Context(), server.URL, 1, 1024); err == nil || !strings.Contains(err.Error(), "aggregate safety limit") {
		t.Fatalf("tools() error = %v, want ignored metadata to consume aggregate budget", err)
	}
}

func TestCrossPlatformCoverageClientToolsRejectsLargeCursorBeforeFollowingIt(t *testing.T) {
	t.Setenv("DWS_ALLOW_HTTP_ENDPOINTS", "1")

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      2,
			"result": map[string]any{
				"tools":      []map[string]any{},
				"nextCursor": strings.Repeat("c", maxToolListCursorBytes+1),
			},
		})
	}))
	defer server.Close()

	_, err := New(transport.NewClient(server.Client()), "", nil).Tools(t.Context(), server.URL)
	if err == nil || !strings.Contains(err.Error(), "cursor exceeded") {
		t.Fatalf("Tools() error = %v, want cursor limit", err)
	}
	if requests != 1 {
		t.Fatalf("tools/list requests = %d, want oversized cursor rejected before follow-up", requests)
	}
}

func TestCrossPlatformCoverageClientToolsRejectsAggregateToolCount(t *testing.T) {
	t.Setenv("DWS_ALLOW_HTTP_ENDPOINTS", "1")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      2,
			"result": map[string]any{
				"tools": []map[string]any{{"name": "one"}, {"name": "two"}},
			},
		})
	}))
	defer server.Close()

	client := New(transport.NewClient(server.Client()), "", nil)
	_, err := client.toolsWithLimits(t.Context(), server.URL, 1, 1024, 1024, 1)
	if err == nil || !strings.Contains(err.Error(), "1-tool aggregate safety limit") {
		t.Fatalf("toolsWithLimits() error = %v, want tool count limit", err)
	}
}

func TestCrossPlatformCoverageClientToolsRejectsMalformedTerminalAndIntermediatePages(t *testing.T) {
	t.Setenv("DWS_ALLOW_HTTP_ENDPOINTS", "1")

	tests := []struct {
		name    string
		results []string
	}{
		{name: "terminal missing tools", results: []string{`{"metadata":{}}`}},
		{name: "terminal null tools", results: []string{`{"tools":null}`}},
		{name: "terminal malformed tools", results: []string{`{"tools":{}}`}},
		{name: "intermediate missing tools", results: []string{`{"tools":[],"nextCursor":"next"}`, `{"metadata":{}}`}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				result := tt.results[requests]
				requests++
				_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":2,"result":%s}`, result)
			}))
			defer server.Close()

			_, err := New(transport.NewClient(server.Client()), "", nil).Tools(t.Context(), server.URL)
			if err == nil {
				t.Fatal("Tools() error = nil, want malformed page rejection")
			}
			if requests != len(tt.results) {
				t.Fatalf("tools/list requests = %d, want %d", requests, len(tt.results))
			}
		})
	}
}

func TestCrossPlatformCoverageClientInvokeDoesNotRetryUnknownIdempotency(t *testing.T) {
	t.Setenv("DWS_ALLOW_HTTP_ENDPOINTS", "1")

	callRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if request.Method == "tools/list" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": request.ID,
				"result": map[string]any{"tools": []map[string]any{{"name": "write_once", "inputSchema": map[string]any{"type": "object"}}}},
			})
			return
		}
		callRequests++
		http.Error(w, "uncertain outcome", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	base := transport.NewClient(server.Client())
	base.MaxRetries = 3
	base.RetryDelay = time.Nanosecond
	base.RetryMaxDelay = time.Nanosecond
	client := New(base, "", nil)

	_, err := client.InvokeValidated(t.Context(), server.URL, "write_once", map[string]any{"value": "x"})
	if err == nil {
		t.Fatal("InvokeValidated() error = nil, want HTTP 503 error")
	}
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Reason != "published_mcp_execution_unknown" || !typed.RetryableSet || typed.Retryable || typed.ExecutionStarted != nil {
		t.Fatalf("InvokeValidated() error metadata = %#v, %v", typed, err)
	}
	if callRequests != 1 {
		t.Fatalf("tools/call requests = %d, want exactly one", callRequests)
	}
	if base.MaxRetries != 3 || client.transport.MaxRetries != 3 || client.invokeTransport.MaxRetries != 0 {
		t.Fatalf("retry policies mutated: base=%d list=%d invoke=%d", base.MaxRetries, client.transport.MaxRetries, client.invokeTransport.MaxRetries)
	}
}

func TestCrossPlatformCoverageClientInvokeDoesNotFollowWriteRedirect(t *testing.T) {
	t.Setenv("DWS_ALLOW_HTTP_ENDPOINTS", "1")
	targetCalls := 0
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		targetCalls++
	}))
	defer target.Close()

	sourceCalls := 0
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if request.Method == "tools/list" {
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":2,"result":{"tools":[{"name":"write_once","inputSchema":{"type":"object"}}]}}`))
			return
		}
		sourceCalls++
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()

	_, err := New(transport.NewClient(source.Client()), "", nil).InvokeValidated(t.Context(), source.URL, "write_once", map[string]any{})
	var typed *apperrors.Error
	if err == nil || !errors.As(err, &typed) || typed.Reason != "published_mcp_execution_unknown" {
		t.Fatalf("InvokeValidated() error = %#v, %v", typed, err)
	}
	if sourceCalls != 1 || targetCalls != 0 {
		t.Fatalf("tools/call source=%d target=%d, want 1 and 0", sourceCalls, targetCalls)
	}
}

func TestCrossPlatformCoverageClientDiscoveryDoesNotCrossRedirectBoundary(t *testing.T) {
	t.Setenv("DWS_ALLOW_HTTP_ENDPOINTS", "1")
	targetCalls := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetCalls++
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":2,"result":{"tools":[]}}`))
	}))
	defer target.Close()
	sourceCalls := 0
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sourceCalls++
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()

	if _, err := New(transport.NewClient(source.Client()), "", nil).Tools(t.Context(), source.URL); err == nil {
		t.Fatal("Tools() followed redirect")
	}
	if sourceCalls != 1 || targetCalls != 0 {
		t.Fatalf("tools/list source=%d target=%d, want 1 and 0", sourceCalls, targetCalls)
	}
}

func TestCrossPlatformCoverageClientPreservesDefinitePreSendValidationError(t *testing.T) {
	t.Setenv("DWS_ALLOW_HTTP_ENDPOINTS", "1")
	callRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Method == "tools/list" {
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":2,"result":{"tools":[{"name":"write_once","inputSchema":{"type":"object"}}]}}`))
			return
		}
		callRequests++
	}))
	defer server.Close()

	_, err := New(transport.NewClient(server.Client()), "", nil).InvokeValidated(t.Context(), server.URL, "write_once", map[string]any{"bad": "x\x00"})
	var typed *apperrors.Error
	if err == nil || !errors.As(err, &typed) || typed.Category != apperrors.CategoryValidation || typed.Reason == "published_mcp_execution_unknown" {
		t.Fatalf("InvokeValidated() error = %#v, %v", typed, err)
	}
	if callRequests != 0 {
		t.Fatalf("tools/call requests = %d, want zero", callRequests)
	}
}

func TestCrossPlatformCoverageClientToolsRetainsTransportRetries(t *testing.T) {
	t.Setenv("DWS_ALLOW_HTTP_ENDPOINTS", "1")

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			http.Error(w, "temporary", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":2,"result":{"tools":[]}}`))
	}))
	defer server.Close()

	base := transport.NewClient(server.Client())
	base.RetryDelay = time.Nanosecond
	base.RetryMaxDelay = time.Nanosecond
	if _, err := New(base, "", nil).Tools(t.Context(), server.URL); err != nil {
		t.Fatalf("Tools() error = %v", err)
	}
	if requests != 2 {
		t.Fatalf("tools/list requests = %d, want one retry", requests)
	}
}

func TestCrossPlatformCoverageInvokeValidatedPropagatesDiscoveryAndDigestFailuresWithoutCall(t *testing.T) {
	t.Run("discovery", func(t *testing.T) {
		client := New(transport.NewClient(nil), "", nil)
		_, err := client.InvokeValidated(t.Context(), "://invalid", "target", map[string]any{})
		if err == nil || !strings.Contains(err.Error(), "发现已发布 MCP 工具") {
			t.Fatalf("InvokeValidated() error = %v, want discovery context", err)
		}
	})

	t.Run("digest", func(t *testing.T) {
		t.Setenv("DWS_ALLOW_HTTP_ENDPOINTS", "1")
		callRequests := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var request struct {
				ID     int    `json:"id"`
				Method string `json:"method"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatal(err)
			}
			if request.Method == "tools/call" {
				callRequests++
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": request.ID,
				"result": map[string]any{"tools": []map[string]any{{"name": "target", "inputSchema": map[string]any{"type": "object"}}}},
			})
		}))
		defer server.Close()

		wantErr := errors.New("digest failed")
		testseam.Swap(t, &digestInputSchema, func(map[string]any) (string, error) { return "", wantErr })
		_, err := New(transport.NewClient(server.Client()), "", nil).InvokeValidated(t.Context(), server.URL, "target", map[string]any{})
		if !errors.Is(err, wantErr) {
			t.Fatalf("InvokeValidated() error = %v, want injected digest failure", err)
		}
		if callRequests != 0 {
			t.Fatalf("tools/call requests = %d, want zero", callRequests)
		}
	})
}

func TestCrossPlatformCoveragePublishedMCPDefiniteRPCFailuresAreNotReplayable(t *testing.T) {
	for _, code := range []int{-32700, -32600, -32601, -32602} {
		err := apperrors.NewAPI("definite RPC rejection", apperrors.WithRPCCode(code), apperrors.WithRetryable(true), apperrors.WithActions("retry"))
		got := publishedMCPInvocationError(err)
		if got != err {
			t.Fatalf("RPC %d returned a replacement error", code)
		}
		var typed *apperrors.Error
		if !errors.As(got, &typed) || typed.ExecutionStarted == nil || *typed.ExecutionStarted || !typed.RetryableSet || typed.Retryable || typed.Actions != nil {
			t.Fatalf("RPC %d metadata = %#v", code, typed)
		}
	}
	validation := apperrors.NewValidation("invalid params", apperrors.WithRPCCode(-32602), apperrors.WithRetryable(true), apperrors.WithActions("retry"))
	got := publishedMCPInvocationError(validation)
	var typed *apperrors.Error
	if got != validation || !errors.As(got, &typed) || typed.ExecutionStarted == nil || *typed.ExecutionStarted || typed.Retryable || typed.Actions != nil {
		t.Fatalf("validation RPC -32602 metadata = %#v", typed)
	}
}
