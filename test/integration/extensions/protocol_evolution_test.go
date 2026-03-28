package extensions_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/market"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/test/mock_mcp"
	"github.com/spf13/cobra"
)

func TestCLIAdaptsToProtocolAddDeleteModifyAfterDiscoveryRefresh(t *testing.T) {
	t.Skip("doc product is no longer dynamically discovered via mcpSurface; needs fixture alignment")
	server := newEvolvingRuntimeMarketServer(t, protocolFixturePhase1(t))
	defer server.Close()

	app.SetDiscoveryBaseURL(server.URL())
	t.Cleanup(func() { app.SetDiscoveryBaseURL("") })
	t.Setenv("DWS_CACHE_DIR", t.TempDir())

	mustRunRoot(t, []string{"cache", "refresh"})
	assertProductTools(t, mcpSurface(t), "doc", []string{"create-document", "search-documents"})
	assertNoProduct(t, mcpSurface(t), "drive")

	server.SetFixture(t, protocolFixturePhase2(t))
	mustRunRoot(t, []string{"cache", "refresh"})
	addedSurface := mcpSurface(t)
	assertProductTools(t, addedSurface, "doc", []string{"archive-document", "create-document", "search-documents"})
	assertProductTools(t, addedSurface, "drive", []string{"list-files"})

	server.SetFixture(t, protocolFixturePhase3(t))
	mustRunRoot(t, []string{"cache", "refresh"})
	updatedSurface := mcpSurface(t)
	assertProductTools(t, updatedSurface, "doc", []string{"archive-document", "create-doc-v2"})
	assertNoProduct(t, updatedSurface, "drive")

	help := mustRunRoot(t, []string{"doc", "create-doc-v2", "--help"})
	if !strings.Contains(help, "--folder-id") {
		t.Fatalf("help output missing --folder-id:\n%s", help)
	}

	payload := mustRunRootJSON(t, []string{"-f", "json", "doc", "create-doc-v2", "--name", "协议升级文档", "--folder-id", "folder-a", "--dry-run"})
	invocation, ok := payload["invocation"].(map[string]any)
	if !ok {
		t.Fatalf("invocation payload missing: %#v", payload)
	}
	if invocation["tool"] != "create_document" {
		t.Fatalf("invocation.tool = %#v, want create_document", invocation["tool"])
	}
	params, ok := invocation["params"].(map[string]any)
	if !ok {
		t.Fatalf("invocation.params missing: %#v", invocation["params"])
	}
	if params["title"] != "协议升级文档" {
		t.Fatalf("params.title = %#v, want 协议升级文档", params["title"])
	}
	if params["folder_id"] != "folder-a" {
		t.Fatalf("params.folder_id = %#v, want folder-a", params["folder_id"])
	}

	legacyOut, legacyErr := runRoot(t, []string{"doc", "create-document", "--title", "legacy"})
	if legacyErr == nil {
		t.Fatal("expected legacy command invocation to fail after protocol update")
	}
	legacyText := strings.ToLower(strings.TrimSpace(legacyErr.Error() + "\n" + legacyOut))
	if !strings.Contains(legacyText, "unknown command") && !strings.Contains(legacyText, "unknown flag") {
		t.Fatalf("expected legacy command invocation to fail with unknown command/flag, err=%v output:\n%s", legacyErr, legacyOut)
	}
}

type evolvingRuntimeMarketServer struct {
	t       *testing.T
	mu      sync.RWMutex
	fixture mockmcp.Fixture
	server  *httptest.Server
	stats   runtimeRequestStats
}

type runtimeRequestStats struct {
	registryCalls int
	detailCalls   map[int]int
	mcpCalls      map[string]map[string]int
}

func newEvolvingRuntimeMarketServer(t *testing.T, fixture mockmcp.Fixture) *evolvingRuntimeMarketServer {
	t.Helper()

	s := &evolvingRuntimeMarketServer{t: t}
	s.SetFixture(t, fixture)

	mux := http.NewServeMux()
	mux.HandleFunc("/cli/discovery/apis", s.serveMarketServers)
	mux.HandleFunc("/mcp/market/detail", s.serveMarketDetail)
	mux.HandleFunc("/", s.serveMCP)
	s.server = httptest.NewServer(mux)
	return s
}

func (s *evolvingRuntimeMarketServer) Close() {
	if s.server != nil {
		s.server.Close()
	}
}

func (s *evolvingRuntimeMarketServer) URL() string {
	if s.server == nil {
		return ""
	}
	return s.server.URL
}

func (s *evolvingRuntimeMarketServer) SetFixture(t *testing.T, fixture mockmcp.Fixture) {
	t.Helper()
	if err := fixture.Validate(); err != nil {
		t.Fatalf("fixture.Validate() error = %v", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fixture = cloneFixture(t, fixture)
}

func (s *evolvingRuntimeMarketServer) ResetStats() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stats = runtimeRequestStats{
		detailCalls: make(map[int]int),
		mcpCalls:    make(map[string]map[string]int),
	}
}

func (s *evolvingRuntimeMarketServer) RegistryCalls() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stats.registryCalls
}

func (s *evolvingRuntimeMarketServer) DetailCalls(mcpID int) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stats.detailCalls[mcpID]
}

func (s *evolvingRuntimeMarketServer) MCPCalls(remotePath, method string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stats.mcpCalls[remotePath][method]
}

func (s *evolvingRuntimeMarketServer) snapshotFixture(t *testing.T) mockmcp.Fixture {
	t.Helper()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneFixture(t, s.fixture)
}

func (s *evolvingRuntimeMarketServer) serveMarketServers(w http.ResponseWriter, r *http.Request) {
	fixture := s.snapshotFixture(s.t)
	s.mu.Lock()
	s.stats.registryCalls++
	s.mu.Unlock()
	response := market.ListResponse{}
	for _, server := range fixture.Servers {
		response.Servers = append(response.Servers, market.ServerEnvelope{
			Server: market.RegistryServer{
				SchemaURI:   server.SchemaURI,
				Name:        server.Name,
				Description: server.Description,
				Remotes: []market.RegistryRemote{
					{Type: "streamable-http", URL: s.URL() + server.RemotePath},
				},
			},
			Meta: market.EnvelopeMeta{
				Registry: server.RegistryWithDetailURL(s.URL() + "/mcp/market/detail?mcpId=" + strconv.Itoa(server.Registry.MCPID)),
				CLI:      server.CLI,
			},
		})
	}
	response.Metadata.Count = len(response.Servers)
	_ = json.NewEncoder(w).Encode(response)
}

func (s *evolvingRuntimeMarketServer) serveMarketDetail(w http.ResponseWriter, r *http.Request) {
	fixture := s.snapshotFixture(s.t)

	mcpID, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("mcpId")))
	if err != nil {
		http.Error(w, "invalid mcpId", http.StatusBadRequest)
		return
	}
	server, ok := fixture.ServerByMCPID(mcpID)
	if !ok || server.Detail == nil {
		http.Error(w, "detail fixture missing", http.StatusNotFound)
		return
	}
	s.mu.Lock()
	if s.stats.detailCalls == nil {
		s.stats.detailCalls = make(map[int]int)
	}
	s.stats.detailCalls[mcpID]++
	s.mu.Unlock()
	_ = json.NewEncoder(w).Encode(server.Detail.Response)
}

func (s *evolvingRuntimeMarketServer) serveMCP(w http.ResponseWriter, r *http.Request) {
	fixture := s.snapshotFixture(s.t)
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
	s.mu.Lock()
	if s.stats.mcpCalls == nil {
		s.stats.mcpCalls = make(map[string]map[string]int)
	}
	if s.stats.mcpCalls[r.URL.Path] == nil {
		s.stats.mcpCalls[r.URL.Path] = make(map[string]int)
	}
	s.stats.mcpCalls[r.URL.Path][method]++
	s.mu.Unlock()
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
		writeJSONRPC(w, req, map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities":    capabilities,
			"serverInfo":      serverInfo,
		}, nil)
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
}

func cloneFixture(t *testing.T, fixture mockmcp.Fixture) mockmcp.Fixture {
	t.Helper()

	data, err := json.Marshal(fixture)
	if err != nil {
		t.Fatalf("json.Marshal(fixture) error = %v", err)
	}
	var out mockmcp.Fixture
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("json.Unmarshal(fixture) error = %v", err)
	}
	return out
}

func protocolFixturePhase1(t *testing.T) mockmcp.Fixture {
	t.Helper()
	fixture := mockmcp.DefaultFixture()
	fixture.Servers = []mockmcp.ServerFixture{fixture.Servers[0]}
	return fixture
}

func protocolFixturePhase2(t *testing.T) mockmcp.Fixture {
	t.Helper()

	fixture := protocolFixturePhase1(t)
	doc := &fixture.Servers[0]
	doc.Registry.UpdatedAt = "2026-03-24T09:00:00Z"

	doc.CLI.Tools = append(doc.CLI.Tools, market.CLITool{
		Name:        "archive_document",
		CLIName:     "archive-document",
		Title:       "归档文档",
		Description: "归档指定文档",
		IsSensitive: boolPtr(false),
		Category:    "写入",
		Hidden:      boolPtr(false),
		Flags: map[string]market.CLIFlagHint{
			"document_id": {Alias: "doc-id"},
		},
	})

	doc.MCP.Tools = append(doc.MCP.Tools, transport.ToolDescriptor{
		Name:        "archive_document",
		Title:       "归档文档",
		Description: "归档指定文档",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"document_id": map[string]any{"type": "string"},
			},
		},
	})
	doc.MCP.Calls["archive_document"] = mockmcp.ToolCallFixture{
		Result: map[string]any{
			"content": map[string]any{"archived": true},
		},
	}

	doc.Detail.Response.Result.Tools = append(doc.Detail.Response.Result.Tools, market.DetailTool{
		ToolName:      "archive_document",
		ToolTitle:     "归档文档",
		ToolDesc:      "归档指定文档",
		IsSensitive:   false,
		ToolRequest:   mustJSONString(map[string]any{"type": "object", "properties": map[string]any{"document_id": map[string]any{"type": "string"}}}),
		ToolResponse:  mustJSONString(map[string]any{"type": "object"}),
		ActionVersion: "G-ACT-VER-102",
	})

	fixture.Servers = append(fixture.Servers, driveServerFixture())
	return fixture
}

func protocolFixturePhase3(t *testing.T) mockmcp.Fixture {
	t.Helper()

	fixture := protocolFixturePhase2(t)
	doc := &fixture.Servers[0]
	doc.Registry.UpdatedAt = "2026-03-25T09:00:00Z"

	doc.MCP.Tools = filterMCPToolsByName(doc.MCP.Tools, "search_documents")
	doc.Detail.Response.Result.Tools = filterDetailToolsByName(doc.Detail.Response.Result.Tools, "search_documents")
	doc.CLI.Tools = filterCLIToolsByName(doc.CLI.Tools, "search_documents")

	for idx := range doc.MCP.Tools {
		if doc.MCP.Tools[idx].Name != "create_document" {
			continue
		}
		doc.MCP.Tools[idx].InputSchema = map[string]any{
			"type":     "object",
			"required": []any{"title"},
			"properties": map[string]any{
				"title":     map[string]any{"type": "string"},
				"folder_id": map[string]any{"type": "string"},
			},
		}
	}

	for idx := range doc.Detail.Response.Result.Tools {
		if doc.Detail.Response.Result.Tools[idx].ToolName != "create_document" {
			continue
		}
		doc.Detail.Response.Result.Tools[idx].IsSensitive = false
		doc.Detail.Response.Result.Tools[idx].ToolTitle = "创建文档V2"
		doc.Detail.Response.Result.Tools[idx].ToolDesc = "创建文档并可指定目录"
		doc.Detail.Response.Result.Tools[idx].ToolRequest = mustJSONString(map[string]any{
			"type":     "object",
			"required": []any{"title"},
			"properties": map[string]any{
				"title":     map[string]any{"type": "string"},
				"folder_id": map[string]any{"type": "string"},
			},
		})
	}

	for idx := range doc.CLI.Tools {
		if doc.CLI.Tools[idx].Name != "create_document" {
			continue
		}
		doc.CLI.Tools[idx].CLIName = "create-doc-v2"
		doc.CLI.Tools[idx].Title = "创建文档V2"
		doc.CLI.Tools[idx].Description = "创建文档并可指定目录"
		doc.CLI.Tools[idx].IsSensitive = boolPtr(false)
		doc.CLI.Tools[idx].Flags = map[string]market.CLIFlagHint{
			"title": {Alias: "name", Shorthand: "n"},
		}
	}

	fixture.Servers = filterServersByCommand(fixture.Servers, "drive")
	return fixture
}

func driveServerFixture() mockmcp.ServerFixture {
	return mockmcp.ServerFixture{
		Name:        "钉钉钉盘",
		Description: "钉盘文件管理",
		SchemaURI:   mockmcp.DefaultSchemaURI,
		RemotePath:  "/server/drive",
		Registry: market.RegistryMetadata{
			IsLatest:    true,
			PublishedAt: "2026-03-21T01:00:00Z",
			UpdatedAt:   "2026-03-21T02:00:00Z",
			Status:      "active",
			MCPID:       9701,
			Quality: market.QualityMetadata{
				HighQuality: true,
				Official:    true,
				DTBiz:       true,
			},
		},
		CLI: market.CLIOverlay{
			ID:          "drive",
			Command:     "drive",
			Description: "钉盘文件管理",
			Prefixes:    []string{"drive"},
			Tools: []market.CLITool{
				{
					Name:        "list_files",
					CLIName:     "list-files",
					Title:       "列出文件",
					Description: "列出钉盘文件",
				},
			},
		},
		Detail: &mockmcp.DetailFixture{
			Response: market.DetailResponse{
				Success: true,
				Result: market.DetailResult{
					MCPID:       9701,
					Name:        "钉钉钉盘",
					Description: "钉盘文件管理",
					Tools: []market.DetailTool{
						{
							ToolName:      "list_files",
							ToolTitle:     "列出文件",
							ToolDesc:      "列出钉盘文件",
							IsSensitive:   false,
							ToolRequest:   mustJSONString(map[string]any{"type": "object", "properties": map[string]any{"space_id": map[string]any{"type": "string"}}}),
							ToolResponse:  mustJSONString(map[string]any{"type": "object"}),
							ActionVersion: "DRV-ACT-1",
						},
					},
				},
			},
		},
		MCP: &mockmcp.MCPFixture{
			ProtocolVersion: mockmcp.DefaultProtocolVer,
			Capabilities: map[string]any{
				"tools": map[string]any{"listChanged": false},
			},
			ServerInfo: map[string]any{
				"name":    "drive",
				"version": "1.0.0",
			},
			Tools: []transport.ToolDescriptor{
				{
					Name:        "list_files",
					Title:       "列出文件",
					Description: "列出钉盘文件",
					InputSchema: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"space_id": map[string]any{"type": "string"},
						},
					},
				},
			},
			Calls: map[string]mockmcp.ToolCallFixture{
				"list_files": {
					Result: map[string]any{
						"content": map[string]any{"items": []any{}},
					},
				},
			},
		},
	}
}

func filterCLIToolsByName(tools []market.CLITool, name string) []market.CLITool {
	out := make([]market.CLITool, 0, len(tools))
	for _, tool := range tools {
		if tool.Name == name {
			continue
		}
		out = append(out, tool)
	}
	return out
}

func filterMCPToolsByName(tools []transport.ToolDescriptor, name string) []transport.ToolDescriptor {
	out := make([]transport.ToolDescriptor, 0, len(tools))
	for _, tool := range tools {
		if tool.Name == name {
			continue
		}
		out = append(out, tool)
	}
	return out
}

func filterDetailToolsByName(tools []market.DetailTool, name string) []market.DetailTool {
	out := make([]market.DetailTool, 0, len(tools))
	for _, tool := range tools {
		if tool.ToolName == name {
			continue
		}
		out = append(out, tool)
	}
	return out
}

func filterServersByCommand(servers []mockmcp.ServerFixture, command string) []mockmcp.ServerFixture {
	out := make([]mockmcp.ServerFixture, 0, len(servers))
	for _, server := range servers {
		if strings.TrimSpace(server.CLI.Command) == command {
			continue
		}
		out = append(out, server)
	}
	return out
}

func mcpSurface(t *testing.T) map[string][]string {
	t.Helper()

	root := app.NewRootCommand()

	return commandSurface(root)
}

func TestCLISynchronouslyRevalidatesRegistryWhenRegistryTTLExpires(t *testing.T) {
	t.Skip("doc product is no longer dynamically discovered via mcpSurface; needs fixture alignment")
	server := newEvolvingRuntimeMarketServer(t, protocolFixturePhase2(t))
	defer server.Close()

	app.SetDiscoveryBaseURL(server.URL())
	t.Cleanup(func() { app.SetDiscoveryBaseURL("") })

	cacheDir := t.TempDir()
	t.Setenv("DWS_CACHE_DIR", cacheDir)

	mustRunRoot(t, []string{"cache", "refresh"})

	server.ResetStats()
	ageCacheSnapshots(t, cacheDir, time.Now().UTC().Add(-25*time.Hour))

	root := app.NewRootCommand()
	assertProductToolsFromCommand(t, root, "doc", []string{"archive-document", "create-document", "search-documents"})
	assertProductToolsFromCommand(t, root, "drive", []string{"list-files"})

	if got := server.RegistryCalls(); got != 1 {
		t.Fatalf("registry calls = %d, want 1", got)
	}
	if got := server.MCPCalls("/server/doc", "initialize"); got != 0 {
		t.Fatalf("doc initialize calls = %d, want 0", got)
	}
	if got := server.MCPCalls("/server/doc", "tools/list"); got != 0 {
		t.Fatalf("doc tools/list calls = %d, want 0", got)
	}
	if got := server.DetailCalls(9629); got != 0 {
		t.Fatalf("doc detail calls = %d, want 0", got)
	}
	if got := server.MCPCalls("/server/drive", "initialize"); got != 0 {
		t.Fatalf("drive initialize calls = %d, want 0", got)
	}
	if got := server.MCPCalls("/server/drive", "tools/list"); got != 0 {
		t.Fatalf("drive tools/list calls = %d, want 0", got)
	}
	if got := server.DetailCalls(9701); got != 0 {
		t.Fatalf("drive detail calls = %d, want 0", got)
	}
}

func commandSurface(parent *cobra.Command) map[string][]string {
	surface := make(map[string][]string)
	for _, product := range parent.Commands() {
		if product.Name() == "help" {
			continue
		}
		tools := make([]string, 0)
		for _, tool := range product.Commands() {
			if tool.Name() == "help" {
				continue
			}
			tools = append(tools, tool.Name())
		}
		sort.Strings(tools)
		surface[product.Name()] = tools
	}
	return surface
}

func assertProductTools(t *testing.T, surface map[string][]string, product string, want []string) {
	t.Helper()

	got, ok := surface[product]
	if !ok {
		t.Fatalf("product %q not found in mcp surface: %#v", product, surface)
	}
	sortedWant := append([]string(nil), want...)
	sort.Strings(sortedWant)
	if strings.Join(got, ",") != strings.Join(sortedWant, ",") {
		t.Fatalf("product %q tools = %#v, want %#v", product, got, sortedWant)
	}
}

func assertNoProduct(t *testing.T, surface map[string][]string, product string) {
	t.Helper()
	if _, ok := surface[product]; ok {
		t.Fatalf("product %q should not exist, surface=%#v", product, surface)
	}
}

func assertProductToolsFromCommand(t *testing.T, parent *cobra.Command, product string, want []string) {
	t.Helper()
	assertProductTools(t, commandSurface(parent), product, want)
}

func mustRunRoot(t *testing.T, args []string) string {
	t.Helper()
	out, err := runRoot(t, args)
	if err != nil {
		t.Fatalf("Execute(%v) error = %v\noutput:\n%s", args, err, out)
	}
	return out
}

func mustRunRootJSON(t *testing.T, args []string) map[string]any {
	t.Helper()
	out := mustRunRoot(t, args)
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("json.Unmarshal(%v) error = %v\noutput:\n%s", args, err, out)
	}
	return payload
}

func runRoot(t *testing.T, args []string) (string, error) {
	t.Helper()
	cmd := app.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader("yes\n"))
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func ageCacheSnapshots(t *testing.T, root string, savedAt time.Time) {
	t.Helper()

	walkErr := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".json") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		var payload map[string]any
		if err := json.Unmarshal(data, &payload); err != nil {
			return nil
		}
		if _, ok := payload["saved_at"]; !ok {
			return nil
		}
		payload["saved_at"] = savedAt.Format(time.RFC3339Nano)

		rewritten, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return err
		}
		return os.WriteFile(path, rewritten, 0o644)
	})
	if walkErr != nil {
		t.Fatalf("ageCacheSnapshots() error = %v", walkErr)
	}
}

func mustJSONString(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(data)
}
