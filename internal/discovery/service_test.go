package discovery

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/market"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
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

func TestDiscoverAllRuntime_SkipsCLISkippedServersWithoutWritingCache(t *testing.T) {
	t.Parallel()

	srv := newTestMCPServer(t, true, true)
	defer srv.Close()

	svc := newTestService(t, srv.URL, srv)

	servers := []market.ServerDescriptor{
		{
			Key:      "active",
			Endpoint: srv.URL + "/mcp",
			CLI:      market.CLIOverlay{ID: "active", Command: "active"},
		},
		{
			Key:      "skipped",
			Endpoint: srv.URL + "/mcp",
			CLI:      market.CLIOverlay{ID: "legacy", Command: "legacy", Skip: true},
		},
	}

	results, failures := svc.DiscoverAllRuntime(context.Background(), servers)
	if len(failures) != 0 {
		t.Fatalf("failures count = %d, want 0", len(failures))
	}
	if len(results) != 1 {
		t.Fatalf("results count = %d, want 1", len(results))
	}
	if results[0].Server.Key != "active" {
		t.Fatalf("successful server key = %q, want active", results[0].Server.Key)
	}

	if _, _, err := svc.Cache.LoadTools("test-tenant/test-identity", "skipped"); err == nil {
		t.Fatal("LoadTools(skipped) error = nil, want skipped service to avoid cache writes")
	}
}

func TestCacheDetailSnapshotWritesServerKeyAndCLIID(t *testing.T) {
	t.Parallel()

	store := cache.NewStore(t.TempDir())
	server := market.ServerDescriptor{
		Key: "server-key",
		CLI: market.CLIOverlay{ID: "server-id"},
	}

	cacheDetailSnapshot(store, "test-tenant/test-identity", server, 42, market.DetailResponse{
		Success: true,
		Result: market.DetailResult{
			Tools: []market.DetailTool{{ToolName: "search", ToolTitle: "Search"}},
		},
	})

	if _, _, err := store.LoadDetail("test-tenant/test-identity", "server-key"); err != nil {
		t.Fatalf("LoadDetail(server-key) error = %v", err)
	}
	if _, _, err := store.LoadDetail("test-tenant/test-identity", "server-id"); err != nil {
		t.Fatalf("LoadDetail(server-id) error = %v", err)
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
	// "search" should be updated from detail.
	if merged[0].Title != "Search Title" {
		t.Fatalf("search title = %q, want 'Search Title'", merged[0].Title)
	}
	if merged[0].Description != "Updated desc" {
		t.Fatalf("search description = %q, want 'Updated desc'", merged[0].Description)
	}
	if !merged[0].Sensitive {
		t.Fatal("search should be marked sensitive")
	}
	// "create" should remain unchanged.
	if merged[1].Description != "create desc" {
		t.Fatalf("create description = %q, want original", merged[1].Description)
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

func TestDiscoverAllRuntime_TimeoutFallsBackToCache(t *testing.T) {
	t.Parallel()

	// done signals slow handlers to exit so srv.Close() can complete.
	done := make(chan struct{})
	// Server that blocks until signalled (simulates an unreachable MCP server).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-done:
		case <-time.After(10 * time.Second):
		}
		http.Error(w, "timeout", http.StatusServiceUnavailable)
	}))
	// LIFO: close(done) runs first so handlers exit, then srv.Close() completes.
	defer srv.Close()
	defer close(done)

	svc := newTestService(t, srv.URL, srv)
	// Override timeout so the test completes quickly.
	svc.PerServerTimeout = 80 * time.Millisecond

	server := market.ServerDescriptor{
		Key:      "slow-server",
		Endpoint: srv.URL + "/mcp",
	}

	// Pre-populate the cache so the fallback has data to return.
	partition := "test-tenant/test-identity"
	_ = svc.Cache.SaveTools(partition, server.Key, cache.ToolsSnapshot{
		ServerKey:       server.Key,
		ProtocolVersion: "2025-03-26",
		Tools: []transport.ToolDescriptor{
			{Name: "cached-tool", Description: "from cache"},
		},
	})

	results, failures := svc.DiscoverAllRuntime(context.Background(), []market.ServerDescriptor{server})
	if len(failures) != 0 {
		t.Fatalf("failures count = %d, want 0 (expected cache fallback)", len(failures))
	}
	if len(results) != 1 {
		t.Fatalf("results count = %d, want 1", len(results))
	}
	if !results[0].Degraded {
		t.Fatal("cache fallback result should be degraded")
	}
	if len(results[0].Tools) == 0 {
		t.Fatal("expected cached tools to be returned")
	}
	if results[0].Tools[0].Name != "cached-tool" {
		t.Fatalf("tool name = %q, want cached-tool", results[0].Tools[0].Name)
	}
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
