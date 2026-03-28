package unit_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/cache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/discovery"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/market"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/transport"
)

func TestDiscoverServersFallsBackToCachedRegistry(t *testing.T) {
	t.Parallel()

	store := cache.NewStore(t.TempDir())
	store.Now = func() time.Time { return time.Date(2026, 3, 21, 0, 0, 0, 0, time.UTC) }
	if err := store.SaveRegistry("default/default", cache.RegistrySnapshot{
		Servers: []market.ServerDescriptor{
			{Key: "doc", DisplayName: "文档", Endpoint: "https://example.com/server/doc"},
		},
	}); err != nil {
		t.Fatalf("SaveRegistry() error = %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	service := discovery.NewService(market.NewClient(server.URL, server.Client()), transport.NewClient(server.Client()), store)
	servers, err := service.DiscoverServers(context.Background())
	if err != nil {
		t.Fatalf("DiscoverServers() error = %v", err)
	}
	if len(servers) != 1 {
		t.Fatalf("DiscoverServers() len = %d, want 1", len(servers))
	}
	if !servers[0].Degraded {
		t.Fatalf("DiscoverServers() expected degraded cache fallback")
	}
}

func TestNewServiceUsesTenantAndAuthIdentityFromEnvironment(t *testing.T) {
	t.Setenv("DWS_TENANT", "corp-a")
	t.Setenv("DWS_AUTH_IDENTITY", "user-001")

	service := discovery.NewService(nil, nil, cache.NewStore(t.TempDir()))
	if service.Tenant != "corp-a" {
		t.Fatalf("service.Tenant = %q, want corp-a", service.Tenant)
	}
	if service.AuthIdentity != "user-001" {
		t.Fatalf("service.AuthIdentity = %q, want user-001", service.AuthIdentity)
	}
}

func TestDiscoverServerRuntimeFallsBackToCachedTools(t *testing.T) {
	t.Parallel()

	store := cache.NewStore(t.TempDir())
	store.Now = func() time.Time { return time.Date(2026, 3, 21, 0, 0, 0, 0, time.UTC) }
	if err := store.SaveTools("default/default", "doc", cache.ToolsSnapshot{
		ServerKey:       "doc",
		ProtocolVersion: "2025-03-26",
		Tools: []transport.ToolDescriptor{
			{Name: "create_document", Title: "创建文档"},
		},
	}); err != nil {
		t.Fatalf("SaveTools() error = %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	service := discovery.NewService(market.NewClient(server.URL, server.Client()), transport.NewClient(server.Client()), store)
	runtimeServer, err := service.DiscoverServerRuntime(context.Background(), market.ServerDescriptor{
		Key:      "doc",
		Endpoint: server.URL,
	})
	if err != nil {
		t.Fatalf("DiscoverServerRuntime() error = %v", err)
	}
	if !runtimeServer.Degraded {
		t.Fatalf("DiscoverServerRuntime() expected degraded cache fallback")
	}
	if len(runtimeServer.Tools) != 1 {
		t.Fatalf("DiscoverServerRuntime() len = %d, want 1", len(runtimeServer.Tools))
	}
}

func TestDiscoverServersReturnsLiveResultsWhenRegistryCacheSaveFails(t *testing.T) {
	t.Parallel()

	cacheRoot := filepath.Join(t.TempDir(), "cache-root")
	if err := os.WriteFile(cacheRoot, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload := market.ListResponse{
			Metadata: market.ListMetadata{Count: 1},
			Servers: []market.ServerEnvelope{
				{
					Server: market.RegistryServer{
						Name: "文档",
						Remotes: []market.RegistryRemote{
							{Type: "streamable-http", URL: "https://example.com/server/doc"},
						},
					},
					Meta: market.EnvelopeMeta{Registry: market.RegistryMetadata{Status: "active"}},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer server.Close()

	service := discovery.NewService(
		market.NewClient(server.URL, server.Client()),
		transport.NewClient(server.Client()),
		cache.NewStore(cacheRoot),
	)

	servers, err := service.DiscoverServers(context.Background())
	if err != nil {
		t.Fatalf("DiscoverServers() error = %v", err)
	}
	if len(servers) != 1 {
		t.Fatalf("DiscoverServers() len = %d, want 1", len(servers))
	}
	if servers[0].Degraded {
		t.Fatalf("DiscoverServers() returned degraded live result")
	}
}

func TestDiscoverServerRuntimeReturnsLiveToolsWhenCacheSaveFails(t *testing.T) {
	t.Parallel()

	cacheRoot := filepath.Join(t.TempDir(), "cache-root")
	if err := os.WriteFile(cacheRoot, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		method := req["method"].(string)
		switch method {
		case "initialize":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      1,
				"result": map[string]any{
					"protocolVersion": "2025-03-26",
					"capabilities":    map[string]any{},
					"serverInfo":      map[string]any{"name": "doc", "version": "1.0.0"},
				},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      2,
				"result": map[string]any{
					"tools": []map[string]any{
						{"name": "create_document", "title": "创建文档", "description": "创建文档", "inputSchema": map[string]any{"type": "object"}},
					},
				},
			})
		default:
			t.Fatalf("unexpected method %q", method)
		}
	}))
	defer server.Close()

	service := discovery.NewService(
		market.NewClient(server.URL, server.Client()),
		transport.NewClient(server.Client()),
		cache.NewStore(cacheRoot),
	)

	runtimeServer, err := service.DiscoverServerRuntime(context.Background(), market.ServerDescriptor{
		Key:      "doc",
		Endpoint: server.URL,
	})
	if err != nil {
		t.Fatalf("DiscoverServerRuntime() error = %v", err)
	}
	if runtimeServer.Degraded {
		t.Fatalf("DiscoverServerRuntime() returned degraded live result")
	}
	if len(runtimeServer.Tools) != 1 {
		t.Fatalf("DiscoverServerRuntime() len = %d, want 1", len(runtimeServer.Tools))
	}
}

func TestDiscoverAllRuntimeKeepsWorkingServersWhenOneFails(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	mux.HandleFunc("/cli/discovery/apis", func(w http.ResponseWriter, r *http.Request) {
		payload := market.ListResponse{
			Metadata: market.ListMetadata{Count: 2},
			Servers: []market.ServerEnvelope{
				{
					Server: market.RegistryServer{
						Name: "成功服务",
						Remotes: []market.RegistryRemote{
							{Type: "streamable-http", URL: server.URL + "/server/success"},
						},
					},
					Meta: market.EnvelopeMeta{Registry: market.RegistryMetadata{Status: "active", UpdatedAt: "2026-03-21T00:00:00Z"}},
				},
				{
					Server: market.RegistryServer{
						Name: "失败服务",
						Remotes: []market.RegistryRemote{
							{Type: "streamable-http", URL: server.URL + "/server/failure"},
						},
					},
					Meta: market.EnvelopeMeta{Registry: market.RegistryMetadata{Status: "active", UpdatedAt: "2026-03-21T00:00:00Z"}},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(payload)
	})

	mux.HandleFunc("/server/success", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		method := req["method"].(string)
		switch method {
		case "initialize":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      1,
				"result": map[string]any{
					"protocolVersion": "2025-03-26",
					"capabilities":    map[string]any{},
					"serverInfo":      map[string]any{"name": "success", "version": "1.0.0"},
				},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      2,
				"result": map[string]any{
					"tools": []map[string]any{
						{"name": "list_docs", "title": "列出文档", "description": "列出文档", "inputSchema": map[string]any{"type": "object"}},
					},
				},
			})
		}
	})

	mux.HandleFunc("/server/failure", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})

	store := cache.NewStore(t.TempDir())
	service := discovery.NewService(market.NewClient(server.URL, server.Client()), transport.NewClient(server.Client()), store)
	servers, err := service.DiscoverServers(context.Background())
	if err != nil {
		t.Fatalf("DiscoverServers() error = %v", err)
	}
	results, failures := service.DiscoverAllRuntime(context.Background(), servers)
	if len(results) != 1 {
		t.Fatalf("DiscoverAllRuntime() results = %d, want 1", len(results))
	}
	if len(failures) != 1 {
		t.Fatalf("DiscoverAllRuntime() failures = %d, want 1", len(failures))
	}
}

func TestDiscoverServerRuntimeEnrichesToolMetadataFromDetail(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	mux.HandleFunc("/mcp/market/detail", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"result": map[string]any{
				"mcpId":       9629,
				"name":        "钉钉文档",
				"description": "钉钉文档",
				"tools": []map[string]any{
					{
						"toolName":      "create_document",
						"toolTitle":     "创建文档",
						"toolDesc":      "在指定位置创建钉钉文档",
						"isSensitive":   true,
						"toolRequest":   `{"type":"object","required":["title"],"properties":{"title":{"type":"string"}}}`,
						"toolResponse":  `{"type":"object","properties":{"documentId":{"type":"string"}}}`,
						"actionVersion": "G-ACT-VER-101",
					},
				},
			},
		})
	})

	mux.HandleFunc("/server/doc", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		method := req["method"].(string)
		switch method {
		case "initialize":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      1,
				"result": map[string]any{
					"protocolVersion": "2025-03-26",
					"capabilities":    map[string]any{},
					"serverInfo":      map[string]any{"name": "doc", "version": "1.0.0"},
				},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      2,
				"result": map[string]any{
					"tools": []map[string]any{
						{"name": "create_document", "title": "create_document", "description": "legacy", "inputSchema": map[string]any{"type": "object"}},
					},
				},
			})
		default:
			t.Fatalf("unexpected method %q", method)
		}
	})

	service := discovery.NewService(
		market.NewClient(server.URL, server.Client()),
		transport.NewClient(server.Client()),
		cache.NewStore(t.TempDir()),
	)
	service.AllowLiveDetailFetch = true
	runtimeServer, err := service.DiscoverServerRuntime(context.Background(), market.ServerDescriptor{
		Key:      "doc",
		Endpoint: server.URL + "/server/doc",
		DetailLocator: market.DetailLocator{
			MCPID: 9629,
		},
	})
	if err != nil {
		t.Fatalf("DiscoverServerRuntime() error = %v", err)
	}
	if len(runtimeServer.Tools) != 1 {
		t.Fatalf("DiscoverServerRuntime() tools len = %d, want 1", len(runtimeServer.Tools))
	}
	tool := runtimeServer.Tools[0]
	if tool.Title != "创建文档" {
		t.Fatalf("tool.Title = %q, want 创建文档", tool.Title)
	}
	if tool.Description != "legacy" {
		t.Fatalf("tool.Description = %q, want runtime description to be preserved", tool.Description)
	}
	if !tool.Sensitive {
		t.Fatalf("tool.Sensitive = false, want true")
	}
	if tool.InputSchema == nil || tool.InputSchema["type"] != "object" {
		t.Fatalf("tool.InputSchema = %#v, want parsed detail schema", tool.InputSchema)
	}
	if tool.OutputSchema == nil || tool.OutputSchema["type"] != "object" {
		t.Fatalf("tool.OutputSchema = %#v, want parsed detail response schema", tool.OutputSchema)
	}
}

func TestDiscoverServerRuntimeIgnoresDetailFailure(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	mux.HandleFunc("/mcp/market/detail", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})

	mux.HandleFunc("/server/doc", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		method := req["method"].(string)
		switch method {
		case "initialize":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      1,
				"result": map[string]any{
					"protocolVersion": "2025-03-26",
					"capabilities":    map[string]any{},
					"serverInfo":      map[string]any{"name": "doc", "version": "1.0.0"},
				},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      2,
				"result": map[string]any{
					"tools": []map[string]any{
						{"name": "create_document", "title": "fallback-title", "description": "fallback-description", "inputSchema": map[string]any{"type": "object"}},
					},
				},
			})
		default:
			t.Fatalf("unexpected method %q", method)
		}
	})

	service := discovery.NewService(
		market.NewClient(server.URL, server.Client()),
		transport.NewClient(server.Client()),
		cache.NewStore(t.TempDir()),
	)
	runtimeServer, err := service.DiscoverServerRuntime(context.Background(), market.ServerDescriptor{
		Key:      "doc",
		Endpoint: server.URL + "/server/doc",
		DetailLocator: market.DetailLocator{
			MCPID: 9629,
		},
	})
	if err != nil {
		t.Fatalf("DiscoverServerRuntime() error = %v", err)
	}
	if runtimeServer.Degraded {
		t.Fatalf("runtime server unexpectedly marked degraded")
	}
	if len(runtimeServer.Tools) != 1 {
		t.Fatalf("DiscoverServerRuntime() tools len = %d, want 1", len(runtimeServer.Tools))
	}
	tool := runtimeServer.Tools[0]
	if tool.Title != "fallback-title" {
		t.Fatalf("tool.Title = %q, want fallback-title", tool.Title)
	}
	if tool.Sensitive {
		t.Fatalf("tool.Sensitive = true, want false without detail override")
	}
}

func TestDiscoverDetailUsesDetailURLBeforeMCPID(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	// When detailURL is rejected by SSRF guard (HTTP, private IP), it falls
	// back to fetching by mcpID via the trusted BaseURL path.
	mux.HandleFunc("/custom-detail", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"result": map[string]any{
				"mcpId":       9527,
				"name":        "detail-url-doc",
				"description": "detail-url",
				"tools": []map[string]any{
					{
						"toolName":      "create_document",
						"toolTitle":     "来自detailUrl",
						"toolDesc":      "优先使用 detailUrl",
						"isSensitive":   false,
						"toolRequest":   `{"type":"object"}`,
						"toolResponse":  `{"type":"object"}`,
						"actionVersion": "url-priority",
					},
				},
			},
		})
	})
	mux.HandleFunc("/mcp/market/detail", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"result": map[string]any{
				"mcpId":       9527,
				"name":        "mcpid-doc",
				"description": "mcpid fallback",
				"tools": []map[string]any{
					{
						"toolName":      "create_document",
						"toolTitle":     "来自mcpId",
						"toolDesc":      "通过 mcpId 获取",
						"isSensitive":   false,
						"toolRequest":   `{"type":"object"}`,
						"toolResponse":  `{"type":"object"}`,
						"actionVersion": "mcpid-fallback",
					},
				},
			},
		})
	})

	service := discovery.NewService(
		market.NewClient(server.URL, server.Client()),
		transport.NewClient(server.Client()),
		cache.NewStore(t.TempDir()),
	)
	// The detailURL uses HTTP + private IP, so SSRF guard in FetchDetailByURL
	// will reject it. The service falls back to FetchDetail with mcpID.
	detail, err := service.DiscoverDetail(context.Background(), market.ServerDescriptor{
		Key: "doc",
		DetailLocator: market.DetailLocator{
			MCPID:     9527,
			DetailURL: server.URL + "/custom-detail",
		},
	})
	if err != nil {
		t.Fatalf("DiscoverDetail() error = %v", err)
	}
	if !detail.Success {
		t.Fatalf("DiscoverDetail() success = false")
	}
	// Falls back to mcpID path since detailURL is rejected by SSRF guard
	if detail.Result.Tools[0].ActionVersion != "mcpid-fallback" {
		t.Fatalf("DiscoverDetail() actionVersion = %q, want mcpid-fallback (SSRF guard rejected detailURL, fell back to mcpID)", detail.Result.Tools[0].ActionVersion)
	}
}
