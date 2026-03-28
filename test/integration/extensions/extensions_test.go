package extensions_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/cache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/discovery"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/market"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/test/mock_mcp"
)

func TestDiscoveryPreservesCLIMetadataAndLifecycle(t *testing.T) {
	t.Parallel()

	srv := newRuntimeMarketServer(t, mockmcp.DefaultFixture())
	defer srv.Close()

	service := discovery.NewService(
		market.NewClient(srv.URL, srv.Client()),
		transport.NewClient(srv.Client()),
		cache.NewStore(t.TempDir()),
	)

	servers, err := service.DiscoverServers(context.Background())
	if err != nil {
		t.Fatalf("DiscoverServers() error = %v", err)
	}
	if len(servers) != 2 {
		t.Fatalf("DiscoverServers() len = %d, want 2", len(servers))
	}

	doc := findServerByName(t, servers, "钉钉文档")
	if !doc.HasCLIMeta {
		t.Fatalf("doc server should expose CLI metadata")
	}
	if doc.CLI.ID != "doc" || doc.CLI.Command != "doc" {
		t.Fatalf("doc CLI overlay = %#v, want doc command", doc.CLI)
	}
	if len(doc.CLI.Tools) != 2 {
		t.Fatalf("doc CLI tools len = %d, want 2", len(doc.CLI.Tools))
	}
	if doc.DetailLocator.MCPID != 9629 {
		t.Fatalf("doc detail locator mcpId = %d, want 9629", doc.DetailLocator.MCPID)
	}
	if doc.Lifecycle.DeprecatedBy != 0 {
		t.Fatalf("doc lifecycle = %#v, want zero-value lifecycle", doc.Lifecycle)
	}

	legacy := findServerByName(t, servers, "钉钉文档（旧）")
	if !legacy.HasCLIMeta {
		t.Fatalf("legacy server should also expose CLI metadata")
	}
	if legacy.Lifecycle.DeprecatedBy != 9629 {
		t.Fatalf("legacy lifecycle deprecatedBy = %d, want 9629", legacy.Lifecycle.DeprecatedBy)
	}
	if legacy.Lifecycle.MigrationURL == "" {
		t.Fatalf("legacy lifecycle should carry migration URL")
	}
}

func TestDiscoverySurfacesNewServerWithoutRuntimeChanges(t *testing.T) {
	t.Parallel()

	fixture := mockmcp.DefaultFixture()
	fixture.Servers = append(fixture.Servers, mockmcp.ServerFixture{
		Name:        "钉钉会议",
		Description: "会议能力通过市场元数据零代码出现",
		SchemaURI:   mockmcp.DefaultSchemaURI,
		RemotePath:  "/server/meeting",
		Registry: market.RegistryMetadata{
			IsLatest:    true,
			PublishedAt: "2026-03-19T00:00:00Z",
			UpdatedAt:   "2026-03-20T00:00:00Z",
			Status:      "active",
			MCPID:       9901,
			DetailURL:   "",
			Quality: market.QualityMetadata{
				HighQuality: true,
				Official:    true,
				DTBiz:       true,
			},
		},
		CLI: market.CLIOverlay{
			ID:          "meeting",
			Command:     "meeting",
			Description: "会议管理",
			Prefixes:    []string{"meeting"},
			Aliases:     []string{"钉钉会议"},
			Tools: []market.CLITool{
				{
					Name:        "create_meeting",
					CLIName:     "create-meeting",
					Title:       "创建会议",
					Description: "创建新的钉钉会议",
					Category:    "写入",
				},
			},
		},
	})

	srv := newRuntimeMarketServer(t, fixture)
	defer srv.Close()

	service := discovery.NewService(
		market.NewClient(srv.URL, srv.Client()),
		transport.NewClient(srv.Client()),
		cache.NewStore(t.TempDir()),
	)

	servers, err := service.DiscoverServers(context.Background())
	if err != nil {
		t.Fatalf("DiscoverServers() error = %v", err)
	}

	meeting := findServerByName(t, servers, "钉钉会议")
	if meeting.Endpoint == "" {
		t.Fatalf("meeting server missing endpoint: %#v", meeting)
	}
	if !meeting.HasCLIMeta {
		t.Fatalf("meeting server should expose CLI metadata")
	}
	if meeting.CLI.ID != "meeting" || meeting.CLI.Command != "meeting" {
		t.Fatalf("meeting CLI overlay = %#v, want meeting command", meeting.CLI)
	}
	if meeting.Source != "live_market" {
		t.Fatalf("meeting source = %q, want live_market", meeting.Source)
	}
}

func TestDiscoverDetailFallsBackToCachedSnapshot(t *testing.T) {
	t.Parallel()

	fixture := mockmcp.DefaultFixture()
	for idx := range fixture.Servers {
		if fixture.Servers[idx].Name == "钉钉文档" {
			fixture.Servers[idx].Detail = nil
		}
	}

	srv := newRuntimeMarketServer(t, fixture)
	defer srv.Close()

	store := cache.NewStore(t.TempDir())
	service := discovery.NewService(
		market.NewClient(srv.URL, srv.Client()),
		transport.NewClient(srv.Client()),
		store,
	)

	servers, err := service.DiscoverServers(context.Background())
	if err != nil {
		t.Fatalf("DiscoverServers() error = %v", err)
	}
	doc := findServerByName(t, servers, "钉钉文档")

	cached := market.DetailResponse{
		Success: true,
		Result: market.DetailResult{
			MCPID:       doc.DetailLocator.MCPID,
			Name:        doc.DisplayName,
			Description: doc.Description,
			Tools: []market.DetailTool{
				{
					ToolName:      "search_documents",
					ToolTitle:     "搜索文档",
					ToolDesc:      "缓存回退文档搜索",
					IsSensitive:   false,
					ToolRequest:   `{"type":"object","properties":{"keyword":{"type":"string"}}}`,
					ToolResponse:  `{"type":"object"}`,
					ActionVersion: "cached-version",
				},
			},
		},
	}
	raw, err := json.Marshal(cached)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := store.SaveDetail("default/default", doc.Key, cache.DetailSnapshot{
		MCPID:   doc.DetailLocator.MCPID,
		Payload: raw,
	}); err != nil {
		t.Fatalf("SaveDetail() error = %v", err)
	}

	detail, err := service.DiscoverDetail(context.Background(), doc)
	if err != nil {
		t.Fatalf("DiscoverDetail() error = %v", err)
	}
	if !detail.Success {
		t.Fatalf("DiscoverDetail() success = false, want cached success")
	}
	if detail.Result.Tools[0].ActionVersion != "cached-version" {
		t.Fatalf("DiscoverDetail() returned %q, want cached-version", detail.Result.Tools[0].ActionVersion)
	}
}

func TestToolsCallUsesMockTransportPath(t *testing.T) {
	t.Parallel()

	srv := mockmcp.DefaultServer()
	defer srv.Close()

	client := transport.NewClient(srv.Client())
	result, err := client.CallTool(context.Background(), srv.RemoteURL("/server/doc"), "create_document", map[string]any{
		"title": "Quarterly Report",
	})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if got := result.Content["documentId"]; got != "doc-123" {
		t.Fatalf("CallTool() documentId = %#v, want doc-123", got)
	}
}

func findServerByName(t *testing.T, servers []market.ServerDescriptor, name string) market.ServerDescriptor {
	t.Helper()

	for _, server := range servers {
		if server.DisplayName == name {
			return server
		}
	}
	t.Fatalf("server %q not found in %#v", name, servers)
	return market.ServerDescriptor{}
}

func newRuntimeMarketServer(t *testing.T, fixture mockmcp.Fixture) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)

	mux.HandleFunc("/cli/discovery/apis", func(w http.ResponseWriter, r *http.Request) {
		response := market.ListResponse{}
		for _, server := range fixture.Servers {
			response.Servers = append(response.Servers, market.ServerEnvelope{
				Server: market.RegistryServer{
					SchemaURI:   server.SchemaURI,
					Name:        server.Name,
					Description: server.Description,
					Remotes: []market.RegistryRemote{
						{Type: "streamable-http", URL: srv.URL + server.RemotePath},
					},
				},
				Meta: market.EnvelopeMeta{
					Registry: server.RegistryWithDetailURL(srv.URL + "/mcp/market/detail?mcpId=" + strconv.Itoa(server.Registry.MCPID)),
					CLI:      server.CLI,
				},
			})
		}
		response.Metadata.Count = len(response.Servers)
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Fatalf("encode market response: %v", err)
		}
	})

	mux.HandleFunc("/mcp/market/detail", func(w http.ResponseWriter, r *http.Request) {
		raw := r.URL.Query().Get("mcpId")
		mcpID, err := strconv.Atoi(raw)
		if err != nil {
			http.Error(w, "invalid mcpId", http.StatusBadRequest)
			return
		}

		server, ok := fixture.ServerByMCPID(mcpID)
		if !ok || server.Detail == nil {
			http.Error(w, "detail fixture missing", http.StatusNotFound)
			return
		}
		if err := json.NewEncoder(w).Encode(server.Detail.Response); err != nil {
			t.Fatalf("encode detail response: %v", err)
		}
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		server, ok := fixture.ServerByRemotePath(r.URL.Path)
		if !ok {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json-rpc request", http.StatusBadRequest)
			return
		}
		method, _ := req["method"].(string)
		switch method {
		case "initialize":
			protocolVersion := mockmcp.DefaultProtocolVer
			capabilities := map[string]any{"tools": map[string]any{"listChanged": false}}
			serverInfo := map[string]any{"name": server.Name, "version": "0.0.0"}
			if server.MCP != nil {
				if server.MCP.ProtocolVersion != "" {
					protocolVersion = server.MCP.ProtocolVersion
				}
				if len(server.MCP.Capabilities) > 0 {
					capabilities = server.MCP.Capabilities
				}
				if len(server.MCP.ServerInfo) > 0 {
					serverInfo = server.MCP.ServerInfo
				}
			}
			payload := map[string]any{
				"protocolVersion": protocolVersion,
				"capabilities":    capabilities,
				"serverInfo":      serverInfo,
			}
			writeJSONRPC(w, req, payload, nil)
		case "notifications/initialized":
			w.WriteHeader(http.StatusNoContent)
		case "tools/list":
			tools := []any(nil)
			if server.MCP != nil && len(server.MCP.Tools) > 0 {
				tools = make([]any, 0, len(server.MCP.Tools))
				for _, tool := range server.MCP.Tools {
					tools = append(tools, tool)
				}
			}
			writeJSONRPC(w, req, map[string]any{"tools": tools}, nil)
		case "tools/call":
			toolName := ""
			if params, ok := req["params"].(map[string]any); ok {
				if name, ok := params["name"].(string); ok {
					toolName = name
				}
			}
			call := mockmcp.ToolCallFixture{}
			if server.MCP != nil && server.MCP.Calls != nil {
				if fixtureCall, ok := server.MCP.Calls[toolName]; ok {
					call = fixtureCall
				}
			}
			if call.Error != nil {
				writeJSONRPC(w, req, nil, call.Error)
				return
			}
			if call.Status != 0 {
				w.WriteHeader(call.Status)
			}
			writeJSONRPC(w, req, call.ResultOrDefault(toolName), nil)
		default:
			http.Error(w, "unexpected json-rpc method", http.StatusBadRequest)
		}
	})

	return srv
}

func writeJSONRPC(w http.ResponseWriter, req map[string]any, result any, rpcErr *transport.RPCError) {
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      requestID(req),
	}
	if rpcErr != nil {
		payload["error"] = rpcErr
	} else {
		payload["result"] = result
	}
	_ = json.NewEncoder(w).Encode(payload)
}

func requestID(req map[string]any) any {
	if id, ok := req["id"]; ok {
		return id
	}
	return 0
}
