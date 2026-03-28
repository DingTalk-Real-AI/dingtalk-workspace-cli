package mockmcp

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/market"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/transport"
)

const (
	DefaultSchemaURI     = "https://static.modelcontextprotocol.io/schemas/2025-12-11/server.schema.json"
	DefaultProtocolVer   = "2025-03-26"
	DefaultMarketSource  = "mock_market"
	DefaultRuntimeSource = "mock_runtime"
)

type Fixture struct {
	Servers []ServerFixture `json:"servers"`
}

type ServerFixture struct {
	Name        string                  `json:"name"`
	Description string                  `json:"description,omitempty"`
	SchemaURI   string                  `json:"schema_uri,omitempty"`
	RemotePath  string                  `json:"remote_path"`
	Registry    market.RegistryMetadata `json:"registry"`
	CLI         market.CLIOverlay       `json:"cli"`
	Detail      *DetailFixture          `json:"detail,omitempty"`
	MCP         *MCPFixture             `json:"mcp,omitempty"`
}

type DetailFixture struct {
	Response market.DetailResponse `json:"response"`
}

type MCPFixture struct {
	ProtocolVersion string                     `json:"protocol_version,omitempty"`
	Capabilities    map[string]any             `json:"capabilities,omitempty"`
	ServerInfo      map[string]any             `json:"server_info,omitempty"`
	Tools           []transport.ToolDescriptor `json:"tools,omitempty"`
	Calls           map[string]ToolCallFixture `json:"calls,omitempty"`
}

type ToolCallFixture struct {
	Result map[string]any      `json:"result,omitempty"`
	Error  *transport.RPCError `json:"error,omitempty"`
	Status int                 `json:"status,omitempty"`
}

func DefaultFixture() Fixture {
	return Fixture{
		Servers: []ServerFixture{
			docServerFixture(),
			docLegacyServerFixture(),
		},
	}
}

func docServerFixture() ServerFixture {
	return ServerFixture{
		Name:        "钉钉文档",
		Description: "钉钉文档MCP支持查找、创建文档",
		SchemaURI:   DefaultSchemaURI,
		RemotePath:  "/server/doc",
		Registry: market.RegistryMetadata{
			IsLatest:    true,
			PublishedAt: "2026-03-13T06:03:05Z",
			UpdatedAt:   "2026-03-18T09:15:00Z",
			Status:      "active",
			MCPID:       9629,
			DetailURL:   "",
			Quality: market.QualityMetadata{
				HighQuality: true,
				Official:    true,
				DTBiz:       true,
			},
		},
		CLI: market.CLIOverlay{
			ID:          "doc",
			Command:     "doc",
			Description: "钉钉文档管理",
			Prefixes:    []string{"doc"},
			Aliases:     []string{"钉钉文档"},
			Group:       "",
			Skip:        false,
			Hidden:      false,
			Tools: []market.CLITool{
				{
					Name:        "search_documents",
					CLIName:     "search-documents",
					Title:       "搜索文档",
					Description: "根据关键词搜索员工有权限访问的文档",
					IsSensitive: boolPtr(false),
					Category:    "查询",
					Hidden:      boolPtr(false),
					Flags: map[string]market.CLIFlagHint{
						"keyword":    {Shorthand: "k", Alias: "query"},
						"maxResults": {Alias: "limit"},
					},
				},
				{
					Name:        "create_document",
					CLIName:     "create-document",
					Title:       "创建文档",
					Description: "在指定位置创建钉钉文档",
					IsSensitive: boolPtr(true),
					Category:    "写入",
					Hidden:      boolPtr(false),
				},
			},
		},
		Detail: &DetailFixture{
			Response: market.DetailResponse{
				Success: true,
				Result: market.DetailResult{
					MCPID:       9629,
					Name:        "钉钉文档",
					Description: "钉钉文档MCP支持查找、创建文档",
					Tools: []market.DetailTool{
						{
							ToolName:      "search_documents",
							ToolTitle:     "搜索文档",
							ToolDesc:      "根据关键词搜索员工有权限访问的文档",
							IsSensitive:   false,
							ToolRequest:   mustJSONString(map[string]any{"type": "object", "properties": map[string]any{"keyword": map[string]any{"type": "string"}, "maxResults": map[string]any{"type": "integer"}}}),
							ToolResponse:  mustJSONString(map[string]any{"type": "object"}),
							ActionVersion: "G-ACT-VER-100",
						},
						{
							ToolName:      "create_document",
							ToolTitle:     "创建文档",
							ToolDesc:      "在指定位置创建钉钉文档",
							IsSensitive:   true,
							ToolRequest:   mustJSONString(map[string]any{"type": "object", "required": []any{"title"}, "properties": map[string]any{"title": map[string]any{"type": "string"}, "content": map[string]any{"type": "string"}}}),
							ToolResponse:  mustJSONString(map[string]any{"type": "object"}),
							ActionVersion: "G-ACT-VER-101",
						},
					},
				},
			},
		},
		MCP: &MCPFixture{
			ProtocolVersion: DefaultProtocolVer,
			Capabilities: map[string]any{
				"tools": map[string]any{"listChanged": false},
			},
			ServerInfo: map[string]any{
				"name":    "doc",
				"version": "1.0.0",
			},
			Tools: []transport.ToolDescriptor{
				{
					Name:        "search_documents",
					Title:       "搜索文档",
					Description: "根据关键词搜索员工有权限访问的文档",
					InputSchema: map[string]any{"type": "object", "properties": map[string]any{"keyword": map[string]any{"type": "string"}}},
				},
				{
					Name:        "create_document",
					Title:       "创建文档",
					Description: "在指定位置创建钉钉文档",
					InputSchema: map[string]any{"type": "object", "required": []any{"title"}},
				},
			},
			Calls: map[string]ToolCallFixture{
				"search_documents": {
					Result: map[string]any{
						"content": map[string]any{
							"items": []any{
								map[string]any{"title": "Project Plan", "id": "doc-001"},
							},
						},
					},
				},
				"create_document": {
					Result: map[string]any{
						"content": map[string]any{
							"documentId": "doc-123",
						},
					},
				},
			},
		},
	}
}

func docLegacyServerFixture() ServerFixture {
	return ServerFixture{
		Name:        "钉钉文档（旧）",
		Description: "旧版钉钉文档服务",
		SchemaURI:   DefaultSchemaURI,
		RemotePath:  "/server/doc-legacy",
		Registry: market.RegistryMetadata{
			IsLatest:    false,
			PublishedAt: "2026-03-10T06:03:05Z",
			UpdatedAt:   "2026-03-12T09:15:00Z",
			Status:      "active",
			MCPID:       1047,
			DetailURL:   "",
			Quality: market.QualityMetadata{
				HighQuality: false,
				Official:    true,
				DTBiz:       true,
			},
			Lifecycle: market.LifecycleInfo{
				DeprecatedBy:    9629,
				DeprecationDate: "2026-04-01T00:00:00Z",
				MigrationURL:    "https://example.com/migration/doc",
			},
		},
		CLI: market.CLIOverlay{
			ID:          "doc-legacy",
			Command:     "doc-legacy",
			Description: "旧版钉钉文档",
			Prefixes:    []string{"doc"},
			Aliases:     []string{"钉钉文档（旧）"},
			Group:       "",
			Skip:        true,
			Hidden:      true,
			Tools: []market.CLITool{
				{
					Name:        "search_documents",
					CLIName:     "search-documents",
					Title:       "搜索文档",
					Description: "旧版文档搜索",
					IsSensitive: boolPtr(false),
					Category:    "查询",
					Hidden:      boolPtr(true),
				},
			},
		},
		Detail: &DetailFixture{
			Response: market.DetailResponse{
				Success: true,
				Result: market.DetailResult{
					MCPID:       1047,
					Name:        "钉钉文档（旧）",
					Description: "旧版钉钉文档服务",
					Tools: []market.DetailTool{
						{
							ToolName:      "search_documents",
							ToolTitle:     "搜索文档",
							ToolDesc:      "旧版文档搜索",
							IsSensitive:   false,
							ToolRequest:   mustJSONString(map[string]any{"type": "object"}),
							ToolResponse:  mustJSONString(map[string]any{"type": "object"}),
							ActionVersion: "G-ACT-VER-099",
						},
					},
				},
			},
		},
		MCP: &MCPFixture{
			ProtocolVersion: DefaultProtocolVer,
			Capabilities: map[string]any{
				"tools": map[string]any{"listChanged": false},
			},
			ServerInfo: map[string]any{
				"name":    "doc-legacy",
				"version": "0.9.0",
			},
			Tools: []transport.ToolDescriptor{
				{
					Name:        "search_documents",
					Title:       "搜索文档",
					Description: "旧版文档搜索",
					InputSchema: map[string]any{"type": "object"},
				},
			},
			Calls: map[string]ToolCallFixture{
				"search_documents": {
					Result: map[string]any{
						"content": map[string]any{
							"items": []any{
								map[string]any{"title": "Legacy Doc", "id": "doc-legacy-001"},
							},
						},
					},
				},
			},
		},
	}
}

func (f Fixture) Validate() error {
	seenPaths := make(map[string]struct{}, len(f.Servers))
	seenMCPIDs := make(map[int]struct{}, len(f.Servers))
	for _, server := range f.Servers {
		if strings.TrimSpace(server.RemotePath) == "" {
			return fmt.Errorf("mock server requires remote_path")
		}
		if !strings.HasPrefix(server.RemotePath, "/") {
			return fmt.Errorf("mock server remote_path must start with /: %s", server.RemotePath)
		}
		if server.Registry.MCPID <= 0 {
			return fmt.Errorf("mock server %s requires a positive mcp_id", server.Name)
		}
		if _, exists := seenPaths[server.RemotePath]; exists {
			return fmt.Errorf("duplicate mock server remote_path: %s", server.RemotePath)
		}
		seenPaths[server.RemotePath] = struct{}{}
		if _, exists := seenMCPIDs[server.Registry.MCPID]; exists {
			return fmt.Errorf("duplicate mock server mcp_id: %d", server.Registry.MCPID)
		}
		seenMCPIDs[server.Registry.MCPID] = struct{}{}
	}
	return nil
}

func (f Fixture) ServerByMCPID(mcpID int) (ServerFixture, bool) {
	for _, server := range f.Servers {
		if server.Registry.MCPID == mcpID {
			return server, true
		}
	}
	return ServerFixture{}, false
}

func (f Fixture) ServerByRemotePath(path string) (ServerFixture, bool) {
	for _, server := range f.Servers {
		if server.RemotePath == path {
			return server, true
		}
	}
	return ServerFixture{}, false
}

func (s ServerFixture) RegistryWithDetailURL(detailURL string) market.RegistryMetadata {
	meta := s.Registry
	meta.DetailURL = detailURL
	return meta
}

func (c ToolCallFixture) ResultOrDefault(toolName string) map[string]any {
	if len(c.Result) > 0 {
		return c.Result
	}
	return map[string]any{
		"content": map[string]any{
			"name": toolName,
		},
	}
}

func mustJSONString(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(data)
}

func boolPtr(value bool) *bool {
	return &value
}
