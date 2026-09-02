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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

func TestClientToolsAndInvoke(t *testing.T) {
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

	tools, err := client.Tools(t.Context(), server.URL)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(tools.Tools) != 1 || tools.Tools[0].Name != "search" {
		t.Fatalf("tools = %#v", tools.Tools)
	}

	result, err := client.Invoke(t.Context(), server.URL, "search", map[string]any{"query": "example"})
	if err != nil {
		t.Fatalf("invoke tool: %v", err)
	}
	if len(result.Blocks) != 1 || result.Blocks[0].Text != "ok" {
		t.Fatalf("result blocks = %#v", result.Blocks)
	}
	if len(methods) != 2 || methods[0] != "tools/list" || methods[1] != "tools/call" {
		t.Fatalf("methods = %#v", methods)
	}
}

func TestNewCreatesDefaultTransport(t *testing.T) {
	client := New(nil, "", nil)
	if client == nil || client.transport == nil {
		t.Fatal("New(nil, ...) returned an incomplete client")
	}
}

func TestClientToolsAggregatesPages(t *testing.T) {
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

func TestAppendToolPageRejectsAggregateLimit(t *testing.T) {
	aggregate := transport.ToolsListResult{}
	tools := []transport.ToolDescriptor{{Name: "search", Description: "large"}}

	if _, err := appendToolPage(&aggregate, tools, 0, 1); err == nil || !strings.Contains(err.Error(), "aggregate safety limit") {
		t.Fatalf("appendToolPage() error = %v, want aggregate safety limit", err)
	}
	if len(aggregate.Tools) != 0 {
		t.Fatalf("appendToolPage() retained tools after rejection: %#v", aggregate.Tools)
	}
}

func TestClientToolsRejectsRepeatedCursor(t *testing.T) {
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
