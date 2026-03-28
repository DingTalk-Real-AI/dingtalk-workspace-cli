package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/transport"
)

type discoveryRuntimeFixture struct {
	ID            string
	Command       string
	CLICommand    string
	EndpointPath  string
	Description   string
	Groups        map[string]any
	ToolOverrides map[string]any
	Tools         []transport.ToolDescriptor
}

func newDiscoveryRuntimeServer(t *testing.T, fixtures ...discoveryRuntimeFixture) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	for _, fixture := range fixtures {
		fixture := fixture
		endpointPath := fixture.EndpointPath
		if endpointPath == "" {
			endpointPath = fixture.Command
		}
		mux.HandleFunc("/"+endpointPath, func(w http.ResponseWriter, r *http.Request) {
			serveAppRuntimeToolList(t, w, r, endpointPath, fixture.Tools...)
		})
	}
	mux.HandleFunc("/cli/discovery/apis", func(w http.ResponseWriter, r *http.Request) {
		baseURL := "http://" + r.Host
		response := map[string]any{
			"metadata": map[string]any{"count": len(fixtures), "nextCursor": ""},
			"servers":  make([]any, 0, len(fixtures)),
		}
		servers := response["servers"].([]any)
		for _, fixture := range fixtures {
			servers = append(servers, discoveryRuntimeServerEntry(baseURL, fixture))
		}
		response["servers"] = servers
		_ = json.NewEncoder(w).Encode(response)
	})

	return httptest.NewServer(mux)
}

func discoveryRuntimeServerEntry(baseURL string, fixture discoveryRuntimeFixture) map[string]any {
	cliID := fixture.ID
	if cliID == "" {
		cliID = fixture.Command
	}
	cliCommand := fixture.CLICommand
	if cliCommand == "" {
		cliCommand = fixture.Command
	}
	endpointPath := fixture.EndpointPath
	if endpointPath == "" {
		endpointPath = fixture.Command
	}
	cliMeta := map[string]any{
		"id":            cliID,
		"command":       cliCommand,
		"description":   fixture.Description,
		"toolOverrides": fixture.ToolOverrides,
	}
	if len(fixture.Groups) > 0 {
		cliMeta["groups"] = fixture.Groups
	}

	return map[string]any{
		"server": map[string]any{
			"name":        cliID,
			"description": fixture.Description,
			"remotes": []any{
				map[string]any{
					"type": "streamable-http",
					"url":  baseURL + "/" + endpointPath,
				},
			},
		},
		"_meta": map[string]any{
			"com.dingtalk.mcp.registry/metadata": map[string]any{
				"status":   "active",
				"isLatest": true,
			},
			"com.dingtalk.mcp.registry/cli": cliMeta,
		},
	}
}

func serveAppRuntimeToolList(t *testing.T, w http.ResponseWriter, r *http.Request, serverName string, tools ...transport.ToolDescriptor) {
	t.Helper()

	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		t.Fatalf("Decode(JSON-RPC request) error = %v", err)
	}

	method, _ := req["method"].(string)
	switch method {
	case "initialize":
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"result": map[string]any{
				"protocolVersion": "2025-03-26",
				"capabilities":    map[string]any{},
				"serverInfo":      map[string]any{"name": serverName, "version": "1.0.0"},
			},
		})
	case "notifications/initialized":
		w.WriteHeader(http.StatusNoContent)
	case "tools/list":
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      2,
			"result": map[string]any{
				"tools": tools,
			},
		})
	default:
		t.Fatalf("unexpected JSON-RPC method %q", method)
	}
}
