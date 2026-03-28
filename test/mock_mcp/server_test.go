package mockmcp

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/market"
)

func TestDefaultFixtureValidate(t *testing.T) {
	t.Parallel()

	if err := DefaultFixture().Validate(); err != nil {
		t.Fatalf("default fixture should validate: %v", err)
	}
}

func TestMarketServersEndpoint(t *testing.T) {
	t.Parallel()

	srv := DefaultServer()
	defer srv.Close()

	resp := mustDoRequest(t, http.MethodGet, srv.MarketURL(), nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}

	var payload struct {
		Metadata struct {
			Count      int    `json:"count"`
			NextCursor string `json:"nextCursor"`
		} `json:"metadata"`
		Servers []struct {
			Server struct {
				SchemaURI   string `json:"$schema"`
				Name        string `json:"name"`
				Description string `json:"description"`
				Remotes     []struct {
					Type string `json:"type"`
					URL  string `json:"url"`
				} `json:"remotes"`
			} `json:"server"`
			Meta struct {
				Metadata market.RegistryMetadata `json:"metadata"`
				CLI      market.CLIOverlay       `json:"cli"`
			} `json:"_meta"`
		} `json:"servers"`
	}
	decodeJSON(t, resp.Body, &payload)

	if payload.Metadata.Count != 2 {
		t.Fatalf("expected 2 servers, got %d", payload.Metadata.Count)
	}
	if len(payload.Servers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(payload.Servers))
	}

	doc := payload.Servers[0]
	if doc.Meta.Metadata.MCPID != 9629 {
		t.Fatalf("expected doc mcp id 9629, got %d", doc.Meta.Metadata.MCPID)
	}
	if got, want := doc.Meta.Metadata.DetailURL, srv.DetailURL(9629); got != want {
		t.Fatalf("unexpected detail url: got %q want %q", got, want)
	}
	if doc.Meta.CLI.ID != "doc" || doc.Meta.CLI.Hidden {
		t.Fatalf("expected visible doc cli overlay, got %+v", doc.Meta.CLI)
	}
	if doc.Server.Remotes[0].URL != srv.RemoteURL("/server/doc") {
		t.Fatalf("unexpected remote url: %q", doc.Server.Remotes[0].URL)
	}

	legacy := payload.Servers[1]
	if legacy.Meta.Metadata.Lifecycle.DeprecatedBy != 9629 {
		t.Fatalf("expected legacy lifecycle deprecatedBy 9629, got %d", legacy.Meta.Metadata.Lifecycle.DeprecatedBy)
	}
	if !legacy.Meta.CLI.Hidden {
		t.Fatalf("expected legacy CLI overlay to be hidden")
	}
}

func TestMarketDetailEndpoint(t *testing.T) {
	t.Parallel()

	srv := DefaultServer()
	defer srv.Close()

	resp := mustDoRequest(t, http.MethodGet, srv.DetailURL(9629), nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}

	var payload market.DetailResponse
	decodeJSON(t, resp.Body, &payload)

	if !payload.Success {
		t.Fatalf("expected success detail response")
	}
	if payload.Result.MCPID != 9629 {
		t.Fatalf("unexpected mcp id: %d", payload.Result.MCPID)
	}
	if len(payload.Result.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(payload.Result.Tools))
	}
	if payload.Result.Tools[0].ActionVersion != "G-ACT-VER-100" {
		t.Fatalf("unexpected action version: %q", payload.Result.Tools[0].ActionVersion)
	}
	if !strings.Contains(payload.Result.Tools[0].ToolRequest, "\"keyword\"") {
		t.Fatalf("expected tool request schema to include keyword")
	}
}

func TestJSONRPCLifecycle(t *testing.T) {
	t.Parallel()

	srv := DefaultServer()
	defer srv.Close()

	initResp := mustDoRequest(t, http.MethodPost, srv.RemoteURL("/server/doc"), strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"dws","version":"0.0.0-dev"}}}`))
	defer initResp.Body.Close()

	if initResp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected initialize status: %d", initResp.StatusCode)
	}

	var initPayload struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int    `json:"id"`
		Result  struct {
			ProtocolVersion string         `json:"protocolVersion"`
			Capabilities    map[string]any `json:"capabilities"`
			ServerInfo      map[string]any `json:"serverInfo"`
		} `json:"result"`
	}
	decodeJSON(t, initResp.Body, &initPayload)

	if initPayload.Result.ProtocolVersion != DefaultProtocolVer {
		t.Fatalf("unexpected protocol version: %q", initPayload.Result.ProtocolVersion)
	}
	if got := initPayload.Result.ServerInfo["name"]; got != "doc" {
		t.Fatalf("unexpected server info name: %v", got)
	}

	notifyResp := mustDoRequest(t, http.MethodPost, srv.RemoteURL("/server/doc"), strings.NewReader(`{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}`))
	defer notifyResp.Body.Close()
	if notifyResp.StatusCode != http.StatusNoContent {
		t.Fatalf("unexpected notify status: %d", notifyResp.StatusCode)
	}

	listResp := mustDoRequest(t, http.MethodPost, srv.RemoteURL("/server/doc"), strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`))
	defer listResp.Body.Close()

	var listPayload struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	decodeJSON(t, listResp.Body, &listPayload)
	if len(listPayload.Result.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(listPayload.Result.Tools))
	}
	if listPayload.Result.Tools[0].Name != "search_documents" {
		t.Fatalf("unexpected first tool name: %q", listPayload.Result.Tools[0].Name)
	}

	callResp := mustDoRequest(t, http.MethodPost, srv.RemoteURL("/server/doc"), strings.NewReader(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"search_documents","arguments":{"keyword":"plan"}}}`))
	defer callResp.Body.Close()

	var callPayload struct {
		Result struct {
			Content map[string]any `json:"content"`
		} `json:"result"`
	}
	decodeJSON(t, callResp.Body, &callPayload)
	if items, ok := callPayload.Result.Content["items"].([]any); !ok || len(items) != 1 {
		t.Fatalf("unexpected call result content: %#v", callPayload.Result.Content)
	}

	createResp := mustDoRequest(t, http.MethodPost, srv.RemoteURL("/server/doc"), strings.NewReader(`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"create_document","arguments":{"title":"hello"}}}`))
	defer createResp.Body.Close()

	var createPayload struct {
		Result struct {
			Content map[string]any `json:"content"`
		} `json:"result"`
	}
	decodeJSON(t, createResp.Body, &createPayload)
	if got := createPayload.Result.Content["documentId"]; got != "doc-123" {
		t.Fatalf("unexpected create result: %#v", createPayload.Result.Content)
	}
}

func mustDoRequest(t *testing.T, method, url string, body io.Reader) *http.Response {
	t.Helper()

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

func decodeJSON(t *testing.T, r io.Reader, out any) {
	t.Helper()

	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read json: %v", err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatalf("unmarshal json: %v\npayload: %s", err, string(data))
	}
}
