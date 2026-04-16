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
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// mockMCPHandler is a minimal MCP JSON-RPC handler for testing.
func mockMCPHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, _ := io.ReadAll(r.Body)
	var req struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      int             `json:"id"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params,omitempty"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSONResp(w, map[string]any{
			"jsonrpc": "2.0", "error": map[string]any{"code": -32700, "message": "parse error"},
		})
		return
	}

	switch req.Method {
	case "initialize":
		writeJSONResp(w, map[string]any{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result": map[string]any{
				"protocolVersion": "2025-03-26",
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "mock-server", "version": "0.0.1"},
			},
		})

	case "notifications/initialized":
		w.WriteHeader(http.StatusOK)
		writeJSONResp(w, map[string]any{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result":  map[string]any{},
		})

	case "tools/list":
		writeJSONResp(w, map[string]any{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result": map[string]any{
				"tools": []map[string]any{
					{
						"name":        "mock_hello",
						"description": "Say hello",
						"inputSchema": map[string]any{
							"type":       "object",
							"properties": map[string]any{"name": map[string]any{"type": "string"}},
							"required":   []string{"name"},
						},
					},
				},
			},
		})

	case "tools/call":
		var params struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		_ = json.Unmarshal(req.Params, &params)

		if params.Name == "mock_hello" {
			name, _ := params.Arguments["name"].(string)
			writeJSONResp(w, map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result": map[string]any{
					"content": []map[string]any{
						{"type": "text", "text": "Hello, " + name + "!"},
					},
				},
			})
		} else {
			writeJSONResp(w, map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"error":   map[string]any{"code": -32601, "message": "unknown tool"},
			})
		}

	default:
		writeJSONResp(w, map[string]any{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"error":   map[string]any{"code": -32601, "message": "method not found"},
		})
	}
}

func writeJSONResp(w http.ResponseWriter, resp any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func TestHTTPClientEndToEnd(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(mockMCPHandler))
	defer server.Close()

	client := NewClient(nil)
	endpoint := server.URL

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Initialize
	initResult, err := client.Initialize(ctx, endpoint)
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if initResult.ProtocolVersion != "2025-03-26" {
		t.Errorf("protocolVersion = %q, want 2025-03-26", initResult.ProtocolVersion)
	}

	// ListTools
	toolsResult, err := client.ListTools(ctx, endpoint)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(toolsResult.Tools) != 1 {
		t.Fatalf("ListTools: got %d tools, want 1", len(toolsResult.Tools))
	}
	if toolsResult.Tools[0].Name != "mock_hello" {
		t.Errorf("tool name = %q, want mock_hello", toolsResult.Tools[0].Name)
	}

	// CallTool
	callResult, err := client.CallTool(ctx, endpoint, "mock_hello", map[string]any{
		"name": "DWS",
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if callResult.IsError {
		t.Fatal("CallTool returned isError=true")
	}
	if len(callResult.Blocks) == 0 {
		t.Fatal("CallTool: no content blocks")
	}
	if callResult.Blocks[0].Text != "Hello, DWS!" {
		t.Errorf("CallTool text = %q, want %q", callResult.Blocks[0].Text, "Hello, DWS!")
	}

	// CallTool with unknown tool
	_, err = client.CallTool(ctx, endpoint, "nonexistent", nil)
	if err == nil {
		t.Error("CallTool with unknown tool should return error")
	}
}

func TestHTTPClientInitializeFailsWithBadEndpoint(t *testing.T) {
	client := NewClient(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := client.Initialize(ctx, "http://127.0.0.1:0/nonexistent")
	if err == nil {
		t.Error("Initialize with bad endpoint should fail")
	}
}
