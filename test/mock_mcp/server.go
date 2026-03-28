package mockmcp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/market"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/transport"
)

type Server struct {
	*httptest.Server
	Fixture Fixture
}

func NewServer(fixture Fixture) (*Server, error) {
	if err := fixture.Validate(); err != nil {
		return nil, err
	}

	s := &Server{Fixture: fixture}
	mux := http.NewServeMux()
	mux.HandleFunc("/cli/discovery/apis", s.serveMarketServers)
	mux.HandleFunc("/mcp/market/detail", s.serveMarketDetail)
	mux.HandleFunc("/", s.serveMCP)

	s.Server = httptest.NewServer(mux)
	return s, nil
}

func MustNewServer(fixture Fixture) *Server {
	server, err := NewServer(fixture)
	if err != nil {
		panic(err)
	}
	return server
}

func DefaultServer() *Server {
	return MustNewServer(DefaultFixture())
}

func (s *Server) MarketURL() string {
	return s.URL + "/cli/discovery/apis"
}

func (s *Server) DetailURL(mcpID int) string {
	return fmt.Sprintf("%s/mcp/market/detail?mcpId=%d", s.URL, mcpID)
}

func (s *Server) RemoteURL(path string) string {
	return s.URL + path
}

func (s *Server) serveMarketServers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	response := struct {
		Metadata struct {
			Count      int    `json:"count"`
			NextCursor string `json:"nextCursor"`
		} `json:"metadata"`
		Servers []marketServerEnvelope `json:"servers"`
	}{}

	for _, server := range s.Fixture.Servers {
		response.Servers = append(response.Servers, marketServerEnvelope{
			Server: marketServer{
				SchemaURI:   server.SchemaURI,
				Name:        server.Name,
				Description: server.Description,
				Remotes: []marketRemote{
					{Type: "streamable-http", URL: s.RemoteURL(server.RemotePath)},
				},
			},
			Meta: marketEnvelopeMeta{
				Registry: server.RegistryWithDetailURL(s.DetailURL(server.Registry.MCPID)),
				CLI:      server.CLI,
			},
		})
	}
	response.Metadata.Count = len(response.Servers)

	_ = json.NewEncoder(w).Encode(response)
}

func (s *Server) serveMarketDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	mcpID, err := readMCPID(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	for _, server := range s.Fixture.Servers {
		if server.Registry.MCPID != mcpID {
			continue
		}
		if server.Detail == nil {
			http.Error(w, "detail fixture missing", http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(server.Detail.Response)
		return
	}

	http.Error(w, "detail fixture missing", http.StatusNotFound)
}

func (s *Server) serveMCP(w http.ResponseWriter, r *http.Request) {
	server, ok := s.serverForPath(r.URL.Path)
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
		result := map[string]any{
			"protocolVersion": server.ProtocolVersionOrDefault(),
			"capabilities":    server.CapabilitiesOrDefault(),
			"serverInfo":      server.ServerInfoOrDefault(),
		}
		s.writeJSONRPC(w, req, result, nil)
	case "notifications/initialized":
		w.WriteHeader(http.StatusNoContent)
	case "tools/list":
		result := map[string]any{
			"tools": server.ToolsOrDefault(),
		}
		s.writeJSONRPC(w, req, result, nil)
	case "tools/call":
		name := toolNameFromRequest(req)
		call := server.CallFixture(name)
		if call.Error != nil {
			s.writeJSONRPC(w, req, nil, call.Error)
			return
		}
		if call.Status != 0 {
			w.WriteHeader(call.Status)
			s.writeJSONRPC(w, req, call.ResultOrDefault(name), nil)
			return
		}
		s.writeJSONRPC(w, req, call.ResultOrDefault(name), nil)
	default:
		http.Error(w, "unexpected json-rpc method", http.StatusBadRequest)
	}
}

func (s *Server) writeJSONRPC(w http.ResponseWriter, req map[string]any, result any, rpcErr *transport.RPCError) {
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

func (s *Server) serverForPath(path string) (serverFixtureRuntime, bool) {
	for _, fixture := range s.Fixture.Servers {
		if fixture.RemotePath == path {
			return serverFixtureRuntime{ServerFixture: fixture}, true
		}
	}
	return serverFixtureRuntime{}, false
}

type serverFixtureRuntime struct {
	ServerFixture
}

func (s serverFixtureRuntime) ProtocolVersionOrDefault() string {
	if s.MCP == nil {
		return DefaultProtocolVer
	}
	if strings.TrimSpace(s.MCP.ProtocolVersion) != "" {
		return s.MCP.ProtocolVersion
	}
	return DefaultProtocolVer
}

func (s serverFixtureRuntime) CapabilitiesOrDefault() map[string]any {
	if s.MCP == nil {
		return map[string]any{"tools": map[string]any{"listChanged": false}}
	}
	if len(s.MCP.Capabilities) > 0 {
		return s.MCP.Capabilities
	}
	return map[string]any{"tools": map[string]any{"listChanged": false}}
}

func (s serverFixtureRuntime) ServerInfoOrDefault() map[string]any {
	if s.MCP == nil {
		return map[string]any{"name": s.Name, "version": "0.0.0"}
	}
	if len(s.MCP.ServerInfo) > 0 {
		return s.MCP.ServerInfo
	}
	return map[string]any{"name": s.Name, "version": "0.0.0"}
}

func (s serverFixtureRuntime) ToolsOrDefault() []any {
	if s.MCP == nil {
		return nil
	}
	if len(s.MCP.Tools) > 0 {
		out := make([]any, 0, len(s.MCP.Tools))
		for _, tool := range s.MCP.Tools {
			out = append(out, tool)
		}
		return out
	}
	return nil
}

func (s serverFixtureRuntime) CallFixture(name string) ToolCallFixture {
	if s.MCP == nil || s.MCP.Calls == nil {
		return ToolCallFixture{}
	}
	if call, ok := s.MCP.Calls[name]; ok {
		return call
	}
	return ToolCallFixture{}
}

type marketServerEnvelope struct {
	Server marketServer       `json:"server"`
	Meta   marketEnvelopeMeta `json:"_meta"`
}

type marketServer struct {
	SchemaURI   string         `json:"$schema"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Remotes     []marketRemote `json:"remotes"`
}

type marketRemote struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

type marketEnvelopeMeta struct {
	Registry market.RegistryMetadata `json:"metadata"`
	CLI      market.CLIOverlay       `json:"cli"`
}

func readMCPID(r *http.Request) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("mcpId"))
	if raw == "" {
		return 0, fmt.Errorf("missing mcpId")
	}
	return strconv.Atoi(raw)
}

func requestID(req map[string]any) any {
	if id, ok := req["id"]; ok {
		return id
	}
	return 0
}

func toolNameFromRequest(req map[string]any) string {
	params, _ := req["params"].(map[string]any)
	if params == nil {
		return ""
	}
	if name, ok := params["name"].(string); ok {
		return name
	}
	return ""
}
