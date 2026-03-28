package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/cache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/discovery"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/market"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/transport"
)

func TestEndToEndDiscoveryFlow(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	mux.HandleFunc("/cli/discovery/apis", func(w http.ResponseWriter, r *http.Request) {
		payload := market.ListResponse{
			Metadata: market.ListMetadata{Count: 1},
			Servers: []market.ServerEnvelope{
				{
					Server: market.RegistryServer{
						Name:        "文档",
						Description: "文档服务",
						SchemaURI:   "https://static.modelcontextprotocol.io/schemas/2025-12-11/server.schema.json",
						Remotes: []market.RegistryRemote{
							{Type: "streamable-http", URL: server.URL + "/server/doc"},
						},
					},
					Meta: market.EnvelopeMeta{Registry: market.RegistryMetadata{Status: "active", UpdatedAt: "2026-03-21T00:00:00Z"}},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(payload)
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
					"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
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
	})

	service := discovery.NewService(
		market.NewClient(server.URL, server.Client()),
		transport.NewClient(server.Client()),
		cache.NewStore(t.TempDir()),
	)

	servers, err := service.DiscoverServers(context.Background())
	if err != nil {
		t.Fatalf("DiscoverServers() error = %v", err)
	}
	if len(servers) != 1 {
		t.Fatalf("DiscoverServers() len = %d, want 1", len(servers))
	}

	runtimeServer, err := service.DiscoverServerRuntime(context.Background(), servers[0])
	if err != nil {
		t.Fatalf("DiscoverServerRuntime() error = %v", err)
	}
	if runtimeServer.NegotiatedProtocolVersion != "2025-03-26" {
		t.Fatalf("DiscoverServerRuntime() protocol = %q, want 2025-03-26", runtimeServer.NegotiatedProtocolVersion)
	}
	if len(runtimeServer.Tools) != 1 {
		t.Fatalf("DiscoverServerRuntime() tools len = %d, want 1", len(runtimeServer.Tools))
	}
}
