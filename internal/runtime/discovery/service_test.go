package discovery

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/cache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/market"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/transport"
)

// newTestMCPServer returns an httptest.Server that handles both market registry
// and MCP JSON-RPC endpoints. marketOK controls whether /cli/discovery/apis
// succeeds, and mcpOK controls whether initialize+tools/list succeed.
func newTestMCPServer(t *testing.T, marketOK, mcpOK bool) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/cli/discovery/apis", func(w http.ResponseWriter, r *http.Request) {
		if !marketOK {
			http.Error(w, "market unavailable", http.StatusInternalServerError)
			return
		}
		// Return one server whose MCP endpoint is this same test server.
		resp := map[string]any{
			"metadata": map[string]any{"count": 1},
			"servers": []map[string]any{{
				"server": map[string]any{
					"name":        "test-server",
					"description": "Test server",
					"remotes":     []map[string]any{{"type": "streamable-http", "url": "PLACEHOLDER"}},
				},
				"_meta": map[string]any{
					"com.dingtalk.mcp.registry/metadata": map[string]any{"mcpId": 0, "status": "active"},
					"com.dingtalk.mcp.registry/cli":      map[string]any{"id": "test", "command": "test", "description": "Test CLI"},
				},
			}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		if !mcpOK {
			http.Error(w, "mcp unavailable", http.StatusInternalServerError)
			return
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		method, _ := req["method"].(string)
		id := req["id"]

		switch method {
		case "initialize":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]any{
					"protocolVersion": "2025-03-26",
					"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
					"serverInfo":      map[string]any{"name": "test", "version": "1.0"},
				},
			})
		case "notifications/initialized":
			// Notification — no response needed, but respond 200 with empty.
			w.WriteHeader(http.StatusOK)
		case "tools/list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]any{
					"tools": []map[string]any{{
						"name":        "search",
						"description": "Search documents",
						"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
					}},
				},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"error":   map[string]any{"code": -32601, "message": "method not found"},
			})
		}
	})

	return httptest.NewServer(mux)
}

func newTestService(t *testing.T, baseURL string, mcpServer *httptest.Server) *Service {
	t.Helper()
	cacheDir := t.TempDir()
	return &Service{
		MarketClient: market.NewClient(baseURL, mcpServer.Client()),
		Transport:    transport.NewClient(mcpServer.Client()),
		Cache:        cache.NewStore(cacheDir),
		Tenant:       "test-tenant",
		AuthIdentity: "test-identity",
	}
}

func TestDiscoverServers_LiveSuccess(t *testing.T) {
	t.Parallel()

	srv := newTestMCPServer(t, true, true)
	defer srv.Close()

	svc := newTestService(t, srv.URL, srv)
	servers, err := svc.DiscoverServers(context.Background())
	if err != nil {
		t.Fatalf("DiscoverServers() error = %v", err)
	}
	if len(servers) == 0 {
		t.Fatal("DiscoverServers() returned no servers")
	}
	if servers[0].Source != "live_market" {
		t.Fatalf("Source = %q, want live_market", servers[0].Source)
	}
}

func TestDiscoverServers_FallbackToCache(t *testing.T) {
	t.Parallel()

	srv := newTestMCPServer(t, true, true)
	defer srv.Close()

	svc := newTestService(t, srv.URL, srv)

	// Populate cache with a live fetch first.
	_, err := svc.DiscoverServers(context.Background())
	if err != nil {
		t.Fatalf("initial DiscoverServers() error = %v", err)
	}

	// Now point market at a broken URL so the next fetch fails.
	svc.MarketClient = market.NewClient("http://127.0.0.1:1/broken", nil)
	servers, err := svc.DiscoverServers(context.Background())
	if err != nil {
		t.Fatalf("DiscoverServers() with cache fallback error = %v", err)
	}
	if len(servers) == 0 {
		t.Fatal("DiscoverServers() returned no servers from cache")
	}
	if !servers[0].Degraded {
		t.Fatal("cached servers should be marked Degraded")
	}
}

func TestDiscoverServers_NoMarketNoCache(t *testing.T) {
	t.Parallel()

	srv := newTestMCPServer(t, false, false)
	defer srv.Close()

	svc := newTestService(t, srv.URL, srv)
	_, err := svc.DiscoverServers(context.Background())
	if err == nil {
		t.Fatal("DiscoverServers() error = nil, want error when both market and cache fail")
	}
}

func TestDiscoverServerRuntime_LiveSuccess(t *testing.T) {
	t.Parallel()

	srv := newTestMCPServer(t, true, true)
	defer srv.Close()

	svc := newTestService(t, srv.URL, srv)
	server := market.ServerDescriptor{
		Key:      "test-key",
		Endpoint: srv.URL + "/mcp",
	}
	result, err := svc.DiscoverServerRuntime(context.Background(), server)
	if err != nil {
		t.Fatalf("DiscoverServerRuntime() error = %v", err)
	}
	if result.Source != "live_runtime" {
		t.Fatalf("Source = %q, want live_runtime", result.Source)
	}
	if result.Degraded {
		t.Fatal("live result should not be degraded")
	}
	if len(result.Tools) == 0 {
		t.Fatal("expected at least one tool")
	}
	if result.Tools[0].Name != "search" {
		t.Fatalf("tool name = %q, want search", result.Tools[0].Name)
	}
	if result.NegotiatedProtocolVersion != "2025-03-26" {
		t.Fatalf("protocol = %q, want 2025-03-26", result.NegotiatedProtocolVersion)
	}
}

func TestDiscoverServerRuntime_FallbackToCache(t *testing.T) {
	t.Parallel()

	srv := newTestMCPServer(t, true, true)
	defer srv.Close()

	svc := newTestService(t, srv.URL, srv)
	server := market.ServerDescriptor{
		Key:      "cache-test",
		Endpoint: srv.URL + "/mcp",
	}

	// First call populates cache.
	_, err := svc.DiscoverServerRuntime(context.Background(), server)
	if err != nil {
		t.Fatalf("initial DiscoverServerRuntime() error = %v", err)
	}

	// Now use a broken MCP endpoint so initialize fails.
	server.Endpoint = "http://127.0.0.1:1/broken"
	result, err := svc.DiscoverServerRuntime(context.Background(), server)
	if err != nil {
		t.Fatalf("DiscoverServerRuntime() with cache fallback error = %v", err)
	}
	if !result.Degraded {
		t.Fatal("cached result should be degraded")
	}
	if len(result.Tools) == 0 {
		t.Fatal("cached tools should be present")
	}
}

func TestDiscoverServerRuntime_NoCacheNoLive(t *testing.T) {
	t.Parallel()

	srv := newTestMCPServer(t, true, false)
	defer srv.Close()

	svc := newTestService(t, srv.URL, srv)
	server := market.ServerDescriptor{
		Key:      "fail-key",
		Endpoint: srv.URL + "/mcp",
	}
	_, err := svc.DiscoverServerRuntime(context.Background(), server)
	if err == nil {
		t.Fatal("DiscoverServerRuntime() error = nil, want error")
	}
	// Error should contain server key for debugging.
	if got := err.Error(); !contains(got, "fail-key") {
		t.Fatalf("error %q should contain server key 'fail-key'", got)
	}
}

func TestDiscoverServerRuntime_ContextCanceled(t *testing.T) {
	t.Parallel()

	srv := newTestMCPServer(t, true, true)
	defer srv.Close()

	svc := newTestService(t, srv.URL, srv)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	server := market.ServerDescriptor{
		Key:      "cancel-key",
		Endpoint: srv.URL + "/mcp",
	}
	_, err := svc.DiscoverServerRuntime(ctx, server)
	if err == nil {
		t.Fatal("DiscoverServerRuntime() error = nil, want context canceled")
	}
}

func TestDiscoverAllRuntime_PartialFailure(t *testing.T) {
	t.Parallel()

	srv := newTestMCPServer(t, true, true)
	defer srv.Close()

	svc := newTestService(t, srv.URL, srv)

	servers := []market.ServerDescriptor{
		{Key: "good", Endpoint: srv.URL + "/mcp"},
		{Key: "bad", Endpoint: "http://127.0.0.1:1/broken"},
	}

	results, failures := svc.DiscoverAllRuntime(context.Background(), servers)
	if len(results) != 1 {
		t.Fatalf("results count = %d, want 1", len(results))
	}
	if results[0].Server.Key != "good" {
		t.Fatalf("successful server key = %q, want good", results[0].Server.Key)
	}
	if len(failures) != 1 {
		t.Fatalf("failures count = %d, want 1", len(failures))
	}
	if failures[0].ServerKey != "bad" {
		t.Fatalf("failed server key = %q, want bad", failures[0].ServerKey)
	}
}

func TestMergeRuntimeToolsWithDetail(t *testing.T) {
	t.Parallel()

	tools := []transport.ToolDescriptor{
		{Name: "search", Description: "original desc"},
		{Name: "create", Description: "create desc"},
	}
	detail := market.DetailResponse{
		Success: true,
		Result: market.DetailResult{
			Tools: []market.DetailTool{
				{ToolName: "search", ToolTitle: "Search Title", ToolDesc: "Updated desc", IsSensitive: true},
			},
		},
	}

	merged := mergeRuntimeToolsWithDetail(tools, detail)
	if len(merged) != 2 {
		t.Fatalf("merged count = %d, want 2", len(merged))
	}
	// "search" should pick up the missing title, but keep its non-empty runtime description.
	if merged[0].Title != "Search Title" {
		t.Fatalf("search title = %q, want 'Search Title'", merged[0].Title)
	}
	if merged[0].Description != "original desc" {
		t.Fatalf("search description = %q, want 'original desc'", merged[0].Description)
	}
	if !merged[0].Sensitive {
		t.Fatal("search should be marked sensitive")
	}
	// "create" should remain unchanged.
	if merged[1].Description != "create desc" {
		t.Fatalf("create description = %q, want original", merged[1].Description)
	}
}

func TestMergeRuntimeToolsWithDetailFillsSchemaGapsWithoutClearingRuntimeTruth(t *testing.T) {
	t.Parallel()

	tools := []transport.ToolDescriptor{
		{
			Name:        "search",
			Title:       "Runtime Title",
			Description: "runtime desc",
			Sensitive:   true,
			InputSchema: map[string]any{
				"type": "object",
				"required": []any{
					"folder_id",
				},
				"properties": map[string]any{
					"title": map[string]any{
						"type":        "string",
						"description": "runtime title desc",
					},
					"folder_id": map[string]any{
						"type": "string",
					},
				},
			},
		},
	}
	detail := market.DetailResponse{
		Success: true,
		Result: market.DetailResult{
			Tools: []market.DetailTool{
				{
					ToolName:  "search",
					ToolTitle: "Detail Title",
					ToolDesc:  "Detail desc",
					ToolRequest: `{"type":"object","required":["title"],"properties":{
						"title":{"type":"string","description":"detail title desc"},
						"folder_id":{"type":"string","description":"Folder ID"},
						"extra":{"type":"string","description":"Extra field"}
					}}`,
				},
			},
		},
	}

	merged := mergeRuntimeToolsWithDetail(tools, detail)
	if len(merged) != 1 {
		t.Fatalf("merged count = %d, want 1", len(merged))
	}
	if got := merged[0].Title; got != "Runtime Title" {
		t.Fatalf("merged title = %q, want Runtime Title", got)
	}
	if got := merged[0].Description; got != "runtime desc" {
		t.Fatalf("merged description = %q, want runtime desc", got)
	}
	if !merged[0].Sensitive {
		t.Fatal("merged sensitive = false, want true")
	}
	properties, ok := merged[0].InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("merged input schema missing properties: %#v", merged[0].InputSchema)
	}
	title, _ := properties["title"].(map[string]any)
	if got := title["description"]; got != "runtime title desc" {
		t.Fatalf("title.description = %#v, want runtime title desc", got)
	}
	folder, _ := properties["folder_id"].(map[string]any)
	if got := folder["description"]; got != "Folder ID" {
		t.Fatalf("folder_id.description = %#v, want Folder ID", got)
	}
	if _, ok := properties["extra"]; !ok {
		t.Fatalf("merged properties missing extra: %#v", properties)
	}
	required := requiredFieldSet(merged[0].InputSchema)
	if _, ok := required["folder_id"]; !ok {
		t.Fatalf("required missing folder_id: %#v", merged[0].InputSchema["required"])
	}
	if _, ok := required["title"]; ok {
		t.Fatalf("required unexpectedly widened with title: %#v", merged[0].InputSchema["required"])
	}
}

func TestMergeRuntimeToolsWithDetail_EmptyDetail(t *testing.T) {
	t.Parallel()

	tools := []transport.ToolDescriptor{{Name: "t1"}}
	empty := market.DetailResponse{Success: false}
	result := mergeRuntimeToolsWithDetail(tools, empty)
	if len(result) != 1 || result[0].Name != "t1" {
		t.Fatal("empty detail should return original tools unchanged")
	}
}

func TestParseDetailSchema(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantNil bool
	}{
		{"valid object", `{"type":"object"}`, false},
		{"empty string", "", true},
		{"invalid json", "not-json", true},
		{"json array", `[1,2,3]`, true},
		{"whitespace only", "   ", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseDetailSchema(tt.input)
			if tt.wantNil && result != nil {
				t.Fatalf("parseDetailSchema(%q) = %v, want nil", tt.input, result)
			}
			if !tt.wantNil && result == nil {
				t.Fatalf("parseDetailSchema(%q) = nil, want non-nil", tt.input)
			}
		})
	}
}

func requiredFieldSet(schema map[string]any) map[string]struct{} {
	out := make(map[string]struct{})
	for _, entry := range requiredSlice(schema) {
		out[entry] = struct{}{}
	}
	return out
}

func requiredSlice(schema map[string]any) []string {
	raw, _ := schema["required"].([]any)
	fields := make([]string, 0, len(raw))
	for _, entry := range raw {
		value, _ := entry.(string)
		if value != "" {
			fields = append(fields, value)
		}
	}
	return fields
}

func TestPartition(t *testing.T) {
	t.Parallel()
	svc := &Service{Tenant: "corp1", AuthIdentity: "user1"}
	got := svc.partition()
	if got != "corp1/user1" {
		t.Fatalf("partition() = %q, want corp1/user1", got)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && searchString(s, substr)))
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
