// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ir

import (
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/discovery"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/market"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/transport"
)

func boolPtr(value bool) *bool {
	return &value
}

func stringPtr(value string) *string {
	return &value
}

func TestBuildCatalogUsesCliIDAndCanonicalPaths(t *testing.T) {
	t.Parallel()

	catalog := BuildCatalog([]discovery.RuntimeServer{
		{
			Server: market.ServerDescriptor{
				Key:         "doc-key",
				DisplayName: "文档",
				Endpoint:    "https://example.com/server/doc",
				CLI: market.CLIOverlay{
					ID: "doc",
				},
			},
			NegotiatedProtocolVersion: "2025-03-26",
			Tools: []transport.ToolDescriptor{
				{Name: "create_document", Title: "创建文档", Description: "创建文档", InputSchema: map[string]any{"type": "object"}},
				{Name: "list_documents", Description: "列出文档", InputSchema: map[string]any{"type": "object"}},
			},
			Source:   "live_runtime",
			Degraded: false,
		},
	})

	if len(catalog.Products) != 1 {
		t.Fatalf("BuildCatalog() products len = %d, want 1", len(catalog.Products))
	}

	product := catalog.Products[0]
	if product.ID != "doc" {
		t.Fatalf("BuildCatalog() product ID = %q, want doc", product.ID)
	}
	if product.Tools[0].CanonicalPath != "doc.create_document" {
		t.Fatalf("BuildCatalog() canonical path = %q, want doc.create_document", product.Tools[0].CanonicalPath)
	}
	if product.Tools[1].Title != "list_documents" {
		t.Fatalf("BuildCatalog() title fallback = %q, want list_documents", product.Tools[1].Title)
	}
}

func TestBuildCatalogSkipsServerWithoutCliID(t *testing.T) {
	t.Parallel()

	catalog := BuildCatalog([]discovery.RuntimeServer{
		{
			Server: market.ServerDescriptor{
				Key:         "no-cli-id-key",
				DisplayName: "无 CLI ID",
				Endpoint:    "https://example.com/server/missing",
			},
			Tools: []transport.ToolDescriptor{
				{Name: "some_tool", InputSchema: map[string]any{"type": "object"}},
			},
		},
	})

	if len(catalog.Products) != 0 {
		t.Fatalf("BuildCatalog() should skip server without CLI.ID, got %d products", len(catalog.Products))
	}
}

func TestBuildCatalogUsesCliIDDirectly(t *testing.T) {
	t.Parallel()

	catalog := BuildCatalog([]discovery.RuntimeServer{
		{
			Server: market.ServerDescriptor{
				Key:         "aitable-key",
				DisplayName: "多维表格",
				Endpoint:    "https://example.com/server/table",
				CLI: market.CLIOverlay{
					ID:      "aitable",
					Command: "table",
				},
			},
			Tools: []transport.ToolDescriptor{
				{Name: "list_tables", InputSchema: map[string]any{"type": "object"}},
			},
		},
	})

	if len(catalog.Products) != 1 {
		t.Fatalf("BuildCatalog() products len = %d, want 1", len(catalog.Products))
	}
	if got := catalog.Products[0].ID; got != "aitable" {
		t.Fatalf("BuildCatalog() product ID = %q, want aitable (from CLI.ID, not alias)", got)
	}
	if got := catalog.Products[0].Tools[0].CanonicalPath; got != "aitable.list_tables" {
		t.Fatalf("BuildCatalog() canonical path = %q, want aitable.list_tables", got)
	}
}

func TestBuildCatalogDisambiguatesCollision(t *testing.T) {
	t.Parallel()

	catalog := BuildCatalog([]discovery.RuntimeServer{
		{
			Server: market.ServerDescriptor{
				Key:         "first-key",
				DisplayName: "文档一",
				Endpoint:    "https://example.com/server/doc",
				CLI:         market.CLIOverlay{ID: "doc"},
			},
		},
		{
			Server: market.ServerDescriptor{
				Key:         "second-key",
				DisplayName: "文档二",
				Endpoint:    "https://example.com/another/doc",
				CLI:         market.CLIOverlay{ID: "doc"},
			},
		},
	})

	if len(catalog.Products) != 2 {
		t.Fatalf("BuildCatalog() products len = %d, want 2", len(catalog.Products))
	}
	if catalog.Products[0].ID != "doc" {
		t.Fatalf("BuildCatalog() first product ID = %q, want doc", catalog.Products[0].ID)
	}
	if catalog.Products[1].ID == "doc" {
		t.Fatalf("BuildCatalog() second product ID unexpectedly reused base ID")
	}
}

func TestFindToolRejectsMalformedPath(t *testing.T) {
	t.Parallel()

	catalog := Catalog{
		Products: []CanonicalProduct{
			{
				ID: "doc",
				Tools: []ToolDescriptor{
					{RPCName: "create_document"},
				},
			},
		},
	}

	if _, _, ok := catalog.FindTool("doc"); ok {
		t.Fatalf("FindTool() accepted malformed path")
	}
	if _, _, ok := catalog.FindTool("doc.create_document"); !ok {
		t.Fatalf("FindTool() rejected valid path")
	}
}

func TestBuildCatalogCarriesSensitiveFlagFromCLIMetadata(t *testing.T) {
	t.Parallel()

	catalog := BuildCatalog([]discovery.RuntimeServer{
		{
			Server: market.ServerDescriptor{
				Key:         "doc-key",
				DisplayName: "文档",
				Endpoint:    "https://example.com/server/doc",
				CLI: market.CLIOverlay{
					ID: "doc",
					Tools: []market.CLITool{
						{Name: "create_document", IsSensitive: boolPtr(true)},
						{Name: "search_documents", IsSensitive: boolPtr(false)},
					},
				},
			},
			Tools: []transport.ToolDescriptor{
				{Name: "create_document", Title: "创建文档", InputSchema: map[string]any{"type": "object"}},
				{Name: "search_documents", Title: "搜索文档", InputSchema: map[string]any{"type": "object"}},
			},
		},
	})

	product := catalog.Products[0]
	createTool, ok := product.FindTool("create_document")
	if !ok {
		t.Fatalf("FindTool(create_document) = not found")
	}
	if !createTool.Sensitive {
		t.Fatalf("create_document sensitive = false, want true")
	}

	searchTool, ok := product.FindTool("search_documents")
	if !ok {
		t.Fatalf("FindTool(search_documents) = not found")
	}
	if searchTool.Sensitive {
		t.Fatalf("search_documents sensitive = true, want false")
	}
}

func TestBuildCatalogFallsBackToRuntimeSensitiveMetadata(t *testing.T) {
	t.Parallel()

	catalog := BuildCatalog([]discovery.RuntimeServer{
		{
			Server: market.ServerDescriptor{
				Key:         "doc-key",
				DisplayName: "文档",
				Endpoint:    "https://example.com/server/doc",
				CLI:         market.CLIOverlay{ID: "doc"},
			},
			Tools: []transport.ToolDescriptor{
				{
					Name:         "create_document",
					Title:        "创建文档",
					Sensitive:    true,
					InputSchema:  map[string]any{"type": "object"},
					OutputSchema: map[string]any{"type": "object"},
				},
			},
		},
	})

	product := catalog.Products[0]
	tool, ok := product.FindTool("create_document")
	if !ok {
		t.Fatalf("FindTool(create_document) = not found")
	}
	if !tool.Sensitive {
		t.Fatalf("create_document sensitive = false, want true")
	}
	if tool.OutputSchema == nil || tool.OutputSchema["type"] != "object" {
		t.Fatalf("create_document output_schema = %#v, want object schema", tool.OutputSchema)
	}
}

func TestBuildCatalogConsumesCLIRouteMetadata(t *testing.T) {
	t.Parallel()

	catalog := BuildCatalog([]discovery.RuntimeServer{
		{
			Server: market.ServerDescriptor{
				Key:         "doc-key",
				DisplayName: "文档",
				Endpoint:    "https://example.com/server/doc",
				CLI: market.CLIOverlay{
					ID:          "documents",
					Command:     "documents",
					Group:       "office",
					Hidden:      true,
					Skip:        false,
					Description: "文档命令",
					Tools: []market.CLITool{
						{
							Name:        "create_document",
							CLIName:     "create",
							Title:       "创建文档（CLI）",
							Description: "CLI 覆盖描述",
							Hidden:      boolPtr(true),
							Flags: map[string]market.CLIFlagHint{
								"title": {Alias: "name", Shorthand: "t"},
							},
						},
					},
				},
			},
			Tools: []transport.ToolDescriptor{
				{Name: "create_document", Title: "创建文档", InputSchema: map[string]any{"type": "object"}},
			},
		},
	})

	product := catalog.Products[0]
	if product.ID != "documents" {
		t.Fatalf("product.ID = %q, want documents", product.ID)
	}
	if product.CLI == nil || !product.CLI.Hidden || product.CLI.Group != "office" {
		t.Fatalf("product.CLI = %#v, want hidden office group", product.CLI)
	}
	if product.Description != "文档命令" {
		t.Fatalf("product.Description = %q, want 文档命令", product.Description)
	}
	tool, ok := product.FindTool("create_document")
	if !ok {
		t.Fatalf("FindTool(create_document) = not found")
	}
	if tool.CLIName != "create" {
		t.Fatalf("tool.CLIName = %q, want create", tool.CLIName)
	}
	if tool.Title != "创建文档（CLI）" {
		t.Fatalf("tool.Title = %q, want 创建文档（CLI）", tool.Title)
	}
	if tool.Description != "CLI 覆盖描述" {
		t.Fatalf("tool.Description = %q, want CLI 覆盖描述", tool.Description)
	}
	if !tool.Hidden {
		t.Fatalf("tool.Hidden = false, want true")
	}
	hint := tool.FlagHints["title"]
	if hint.Alias != "name" || hint.Shorthand != "t" {
		t.Fatalf("tool.FlagHints[title] = %#v, want alias=name shorthand=t", hint)
	}
}

func TestBuildCatalogKeepsRuntimeSchemaWhenCLIToolFlagsArePartialHints(t *testing.T) {
	t.Parallel()

	catalog := BuildCatalog([]discovery.RuntimeServer{
		{
			Server: market.ServerDescriptor{
				Key:         "doc-key",
				DisplayName: "文档",
				Endpoint:    "https://example.com/server/doc",
				CLI: market.CLIOverlay{
					ID:      "doc",
					Command: "doc",
					Tools: []market.CLITool{
						{
							Name:    "create_document",
							CLIName: "create-doc-v2",
							Flags: map[string]market.CLIFlagHint{
								"title": {Alias: "name", Shorthand: "n"},
							},
						},
					},
				},
			},
			Tools: []transport.ToolDescriptor{
				{
					Name: "create_document",
					InputSchema: map[string]any{
						"type":     "object",
						"required": []any{"title"},
						"properties": map[string]any{
							"title":     map[string]any{"type": "string"},
							"folder_id": map[string]any{"type": "string"},
						},
					},
				},
			},
		},
	})

	product, ok := catalog.FindProduct("doc")
	if !ok {
		t.Fatalf("FindProduct(doc) = not found")
	}
	tool, ok := product.FindTool("create_document")
	if !ok {
		t.Fatalf("FindTool(create_document) = not found")
	}
	properties, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("tool.InputSchema.properties = %#v, want map", tool.InputSchema["properties"])
	}
	if _, exists := properties["folder_id"]; !exists {
		t.Fatalf("runtime schema unexpectedly lost folder_id: %#v", properties)
	}
	if got := tool.FlagHints["title"].Alias; got != "name" {
		t.Fatalf("tool.FlagHints[title].Alias = %q, want name", got)
	}
}

func TestBuildCatalogReconcilesSingleOrphanTopLevelFlagHint(t *testing.T) {
	t.Parallel()

	catalog := BuildCatalog([]discovery.RuntimeServer{
		{
			Server: market.ServerDescriptor{
				Key:         "todo-key",
				DisplayName: "待办",
				Endpoint:    "https://example.com/server/todo",
				CLI: market.CLIOverlay{
					ID:      "todo",
					Command: "todo",
					Tools: []market.CLITool{
						{
							Name:    "get_user_todos_in_current_org",
							CLIName: "list",
							Group:   "task",
							Flags: map[string]market.CLIFlagHint{
								"pageNum":  {Alias: "page"},
								"pageSize": {Alias: "size"},
								"isDone":   {Alias: "status"},
							},
						},
					},
				},
			},
			Tools: []transport.ToolDescriptor{
				{
					Name:  "get_user_todos_in_current_org",
					Title: "查询个人待办",
					InputSchema: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"pageNum":    map[string]any{"type": "string"},
							"pageSize":   map[string]any{"type": "string"},
							"todoStatus": map[string]any{"type": "string"},
						},
						"required": []any{"pageNum", "pageSize"},
					},
				},
			},
		},
	})

	tool, ok := catalog.Products[0].FindTool("get_user_todos_in_current_org")
	if !ok {
		t.Fatalf("FindTool(get_user_todos_in_current_org) = not found")
	}
	if got := tool.FlagHints["todoStatus"].Alias; got != "status" {
		t.Fatalf("tool.FlagHints[todoStatus].Alias = %q, want status", got)
	}
	if _, exists := tool.FlagHints["isDone"]; exists {
		t.Fatalf("tool.FlagHints unexpectedly kept orphan key isDone: %#v", tool.FlagHints["isDone"])
	}
}

func TestBuildCatalogReconcilesFlattenedDottedFlagHints(t *testing.T) {
	t.Parallel()

	catalog := BuildCatalog([]discovery.RuntimeServer{
		{
			Server: market.ServerDescriptor{
				Key:         "bot-key",
				DisplayName: "机器人",
				Endpoint:    "https://example.com/server/bot",
				CLI: market.CLIOverlay{
					ID:      "bot",
					Command: "chat",
					ToolOverrides: map[string]market.CLIToolOverride{
						"search_groups_by_keyword": {
							CLIName: "search",
							Flags: map[string]market.CLIFlagOverride{
								"OpenSearchRequest.cursor": {Alias: "cursor"},
								"OpenSearchRequest.query":  {Alias: "query"},
							},
						},
					},
				},
			},
			Tools: []transport.ToolDescriptor{
				{
					Name:  "search_groups_by_keyword",
					Title: "搜索群聊",
					InputSchema: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"cursor":  map[string]any{"type": "string"},
							"keyword": map[string]any{"type": "string"},
						},
						"required": []any{"keyword"},
					},
				},
			},
		},
	})

	tool, ok := catalog.Products[0].FindTool("search_groups_by_keyword")
	if !ok {
		t.Fatalf("FindTool(search_groups_by_keyword) = not found")
	}
	if got := tool.FlagHints["cursor"].Alias; got != "cursor" {
		t.Fatalf("tool.FlagHints[cursor].Alias = %q, want cursor", got)
	}
	if got := tool.FlagHints["keyword"].Alias; got != "query" {
		t.Fatalf("tool.FlagHints[keyword].Alias = %q, want query", got)
	}
	if !tool.FlagHints["keyword"].Required {
		t.Fatalf("tool.FlagHints[keyword].Required = false, want true")
	}
	if _, exists := tool.FlagHints["OpenSearchRequest.query"]; exists {
		t.Fatalf("tool.FlagHints unexpectedly kept orphan key OpenSearchRequest.query: %#v", tool.FlagHints["OpenSearchRequest.query"])
	}
}

func TestBuildCatalogKeepsWrapperObjectFlagHintsNested(t *testing.T) {
	t.Parallel()

	catalog := BuildCatalog([]discovery.RuntimeServer{
		{
			Server: market.ServerDescriptor{
				Key:         "todo-key",
				DisplayName: "待办",
				Endpoint:    "https://example.com/server/todo",
				CLI: market.CLIOverlay{
					ID:      "todo",
					Command: "todo",
					ToolOverrides: map[string]market.CLIToolOverride{
						"create_personal_todo": {
							CLIName: "create",
							Group:   "task",
							Flags: map[string]market.CLIFlagOverride{
								"PersonalTodoCreateVO.subject":     {Alias: "title"},
								"PersonalTodoCreateVO.executorIds": {Alias: "executors", Transform: "csv_to_array"},
							},
						},
					},
				},
			},
			Tools: []transport.ToolDescriptor{
				{
					Name:  "create_personal_todo",
					Title: "创建待办",
					InputSchema: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"PersonalTodoCreateVO": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"subject":     map[string]any{"type": "string"},
									"executorIds": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
								},
							},
						},
						"required": []any{"PersonalTodoCreateVO"},
					},
				},
			},
		},
	})

	tool, ok := catalog.Products[0].FindTool("create_personal_todo")
	if !ok {
		t.Fatalf("FindTool(create_personal_todo) = not found")
	}
	if _, exists := tool.FlagHints["subject"]; exists {
		t.Fatalf("tool.FlagHints unexpectedly flattened subject: %#v", tool.FlagHints["subject"])
	}
	if got := tool.FlagHints["PersonalTodoCreateVO.subject"].Alias; got != "title" {
		t.Fatalf("tool.FlagHints[PersonalTodoCreateVO.subject].Alias = %q, want title", got)
	}
	if got := tool.FlagHints["PersonalTodoCreateVO.executorIds"].Alias; got != "executors" {
		t.Fatalf("tool.FlagHints[PersonalTodoCreateVO.executorIds].Alias = %q, want executors", got)
	}
}

func TestBuildCatalogPreservesLiveProbeChatSurface(t *testing.T) {
	t.Parallel()

	catalog := BuildCatalog([]discovery.RuntimeServer{
		{
			Server: market.ServerDescriptor{
				Key:         "bot-key",
				DisplayName: "机器人消息",
				Endpoint:    "https://example.com/server/bot",
				CLI: market.CLIOverlay{
					ID:      "bot",
					Command: "bot",
					ToolOverrides: map[string]market.CLIToolOverride{
						"search_my_robots": {
							CLIName: "search",
							Group:   "bot",
							Flags: map[string]market.CLIFlagOverride{
								"currentPage": {Alias: "page"},
								"pageSize":    {Alias: "size"},
								"robotName":   {Alias: "name"},
							},
						},
						"add_robot_to_group": {
							CLIName: "add-bot",
							Group:   "group.members",
							Flags: map[string]market.CLIFlagOverride{
								"openConversationId": {Alias: "id"},
								"robotCode":          {Alias: "robot-code"},
							},
						},
					},
				},
			},
			Tools: []transport.ToolDescriptor{
				{Name: "search_my_robots", Title: "搜索机器人", InputSchema: map[string]any{"type": "object"}},
				{Name: "add_robot_to_group", Title: "添加机器人", InputSchema: map[string]any{"type": "object"}},
			},
		},
		{
			Server: market.ServerDescriptor{
				Key:         "group-chat-key",
				DisplayName: "群聊",
				Endpoint:    "https://example.com/server/group-chat",
				CLI: market.CLIOverlay{
					ID:      "group-chat",
					Command: "chat",
					ToolOverrides: map[string]market.CLIToolOverride{
						"get_group_members": {
							CLIName: "list",
							Group:   "group.members",
							Flags: map[string]market.CLIFlagOverride{
								"cursor":              {Alias: "cursor"},
								"openconversation_id": {Alias: "id"},
							},
						},
					},
				},
			},
			Tools: []transport.ToolDescriptor{
				{
					Name:  "get_group_members",
					Title: "查群成员列表",
					InputSchema: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"openconversation_id": map[string]any{"type": "string"},
							"cursor":              map[string]any{"type": "string"},
						},
					},
				},
			},
		},
	})

	bot, ok := catalog.FindProduct("bot")
	if !ok {
		t.Fatal("FindProduct(bot) = not found")
	}
	if bot.CLI == nil || bot.CLI.Command != "bot" {
		t.Fatalf("bot.CLI.Command = %#v, want bot", bot.CLI)
	}
	searchBot, ok := bot.FindTool("search_my_robots")
	if !ok {
		t.Fatal("FindTool(search_my_robots) = not found")
	}
	if searchBot.Group != "bot" {
		t.Fatalf("search_my_robots group = %q, want bot", searchBot.Group)
	}
	addBot, ok := bot.FindTool("add_robot_to_group")
	if !ok {
		t.Fatal("FindTool(add_robot_to_group) = not found")
	}
	if addBot.CLIName != "add-bot" {
		t.Fatalf("add_robot_to_group CLIName = %q, want add-bot", addBot.CLIName)
	}
	if addBot.Group != "group.members" {
		t.Fatalf("add_robot_to_group group = %q, want group.members", addBot.Group)
	}

	groupChat, ok := catalog.FindProduct("group-chat")
	if !ok {
		t.Fatal("FindProduct(group-chat) = not found")
	}
	members, ok := groupChat.FindTool("get_group_members")
	if !ok {
		t.Fatal("FindTool(get_group_members) = not found")
	}
	if members.CLIName != "list" {
		t.Fatalf("get_group_members CLIName = %q, want list", members.CLIName)
	}
	if members.Group != "group.members" {
		t.Fatalf("get_group_members group = %q, want group.members", members.Group)
	}
}

func TestBuildCatalogPreservesProbeAliasesAndSchemas(t *testing.T) {
	t.Parallel()

	catalog := BuildCatalog([]discovery.RuntimeServer{
		{
			Server: market.ServerDescriptor{
				Key:         "contact-key",
				DisplayName: "通讯录",
				Endpoint:    "https://example.com/server/contact",
				CLI: market.CLIOverlay{
					ID:      "contact",
					Command: "contact",
					ToolOverrides: map[string]market.CLIToolOverride{
						"search_dept_by_keyword": {
							CLIName: "search",
							Group:   "dept",
							Flags: map[string]market.CLIFlagOverride{
								"query": {Alias: "keyword"},
							},
						},
						"search_user_by_key_word": {
							CLIName: "search",
							Group:   "user",
							Flags: map[string]market.CLIFlagOverride{
								"keyWord": {Alias: "keyword"},
							},
						},
					},
				},
			},
			Tools: []transport.ToolDescriptor{
				{
					Name:  "search_dept_by_keyword",
					Title: "搜索部门",
					InputSchema: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"query": map[string]any{"type": "string"},
						},
					},
				},
				{
					Name:  "search_user_by_key_word",
					Title: "搜索用户",
					InputSchema: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"keyWord": map[string]any{"type": "string"},
						},
					},
				},
			},
		},
		{
			Server: market.ServerDescriptor{
				Key:         "devdoc-key",
				DisplayName: "开放平台文档",
				Endpoint:    "https://example.com/server/devdoc",
				CLI: market.CLIOverlay{
					ID:      "devdoc",
					Command: "devdoc",
					ToolOverrides: map[string]market.CLIToolOverride{
						"search_open_platform_docs": {
							CLIName: "search",
							Group:   "article",
							Flags: map[string]market.CLIFlagOverride{
								"keyword": {Alias: "keyword"},
								"page":    {Alias: "page"},
								"size":    {Alias: "size"},
							},
						},
					},
				},
			},
			Tools: []transport.ToolDescriptor{
				{
					Name:  "search_open_platform_docs",
					Title: "搜索文档",
					InputSchema: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"keyword": map[string]any{"type": "string"},
							"page":    map[string]any{"type": "number"},
							"size":    map[string]any{"type": "number"},
						},
					},
				},
			},
		},
		{
			Server: market.ServerDescriptor{
				Key:         "aitable-key",
				DisplayName: "AI表格",
				Endpoint:    "https://example.com/server/aitable",
				CLI: market.CLIOverlay{
					ID:      "aitable",
					Command: "aitable",
					ToolOverrides: map[string]market.CLIToolOverride{
						"query_records": {
							CLIName: "query",
							Group:   "record",
							Flags: map[string]market.CLIFlagOverride{
								"keyword": {Alias: "keyword"},
							},
						},
					},
				},
			},
			Tools: []transport.ToolDescriptor{
				{
					Name:  "query_records",
					Title: "查询记录",
					InputSchema: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"keyword": map[string]any{"type": "string"},
						},
					},
				},
			},
		},
		{
			Server: market.ServerDescriptor{
				Key:         "todo-key",
				DisplayName: "待办",
				Endpoint:    "https://example.com/server/todo",
				CLI: market.CLIOverlay{
					ID:      "todo",
					Command: "todo",
					ToolOverrides: map[string]market.CLIToolOverride{
						"create_personal_todo": {
							CLIName: "create",
							Group:   "task",
							Flags: map[string]market.CLIFlagOverride{
								"PersonalTodoCreateVO.subject":     {Alias: "title"},
								"PersonalTodoCreateVO.executorIds": {Alias: "executors", Transform: "csv_to_array"},
							},
						},
					},
				},
			},
			Tools: []transport.ToolDescriptor{
				{
					Name:  "create_personal_todo",
					Title: "创建待办",
					InputSchema: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"PersonalTodoCreateVO": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"subject": map[string]any{"type": "string"},
								},
							},
						},
					},
				},
			},
		},
		{
			Server: market.ServerDescriptor{
				Key:         "group-chat-key",
				DisplayName: "群聊",
				Endpoint:    "https://example.com/server/group-chat",
				CLI: market.CLIOverlay{
					ID:      "group-chat",
					Command: "chat",
					ToolOverrides: map[string]market.CLIToolOverride{
						"list_topic_replies": {
							CLIName: "list-topic-replies",
							Group:   "message",
							Flags: map[string]market.CLIFlagOverride{
								"pageSize": {Alias: "limit"},
							},
						},
					},
				},
			},
			Tools: []transport.ToolDescriptor{
				{
					Name:  "list_topic_replies",
					Title: "话题回复",
					InputSchema: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"pageSize": map[string]any{"type": "number"},
						},
					},
				},
			},
		},
	})

	contact, _ := catalog.FindProduct("contact")
	deptSearch, _ := contact.FindTool("search_dept_by_keyword")
	if got := deptSearch.FlagHints["query"].Alias; got != "keyword" {
		t.Fatalf("search_dept_by_keyword query alias = %q, want keyword", got)
	}
	userSearch, _ := contact.FindTool("search_user_by_key_word")
	if got := userSearch.FlagHints["keyWord"].Alias; got != "keyword" {
		t.Fatalf("search_user_by_key_word keyWord alias = %q, want keyword", got)
	}

	devdoc, _ := catalog.FindProduct("devdoc")
	docSearch, _ := devdoc.FindTool("search_open_platform_docs")
	if got := docSearch.FlagHints["keyword"].Alias; got != "keyword" {
		t.Fatalf("search_open_platform_docs keyword alias = %q, want keyword", got)
	}

	aitable, _ := catalog.FindProduct("aitable")
	recordQuery, _ := aitable.FindTool("query_records")
	if got := recordQuery.FlagHints["keyword"].Alias; got != "keyword" {
		t.Fatalf("query_records keyword alias = %q, want keyword", got)
	}

	todo, _ := catalog.FindProduct("todo")
	createTodo, _ := todo.FindTool("create_personal_todo")
	if _, ok := createTodo.FlagHints["PersonalTodoCreateVO.recurrence"]; ok {
		t.Fatalf("create_personal_todo unexpectedly synthesized recurrence hint: %#v", createTodo.FlagHints["PersonalTodoCreateVO.recurrence"])
	}
	if got := schemaPropertyType(createTodo.InputSchema, "PersonalTodoCreateVO.recurrence"); got != "" {
		t.Fatalf("create_personal_todo recurrence type = %q, want empty", got)
	}

	groupChat, _ := catalog.FindProduct("group-chat")
	topicReplies, _ := groupChat.FindTool("list_topic_replies")
	if got := schemaPropertyType(topicReplies.InputSchema, "pageSize"); got != "number" {
		t.Fatalf("list_topic_replies pageSize type = %q, want number", got)
	}
}

func TestBuildCatalogFallsBackToToolOverrides(t *testing.T) {
	t.Parallel()

	catalog := BuildCatalog([]discovery.RuntimeServer{
		{
			Server: market.ServerDescriptor{
				Key:         "bot-key",
				DisplayName: "机器人",
				Endpoint:    "https://example.com/server/bot",
				CLI: market.CLIOverlay{
					ID:      "bot",
					Command: "bot",
					// Tools is nil — simulates current production data.
					ToolOverrides: map[string]market.CLIToolOverride{
						"add_robot_to_group": {
							CLIName:     "add-bot",
							Hidden:      boolPtr(false),
							IsSensitive: boolPtr(true),
							Flags: map[string]market.CLIFlagOverride{
								"robot_code": {Alias: "code"},
								"group_id":   {Alias: "group"},
							},
						},
						"search_my_robots": {
							CLIName: "search",
							Hidden:  boolPtr(true),
						},
					},
				},
			},
			Tools: []transport.ToolDescriptor{
				{Name: "add_robot_to_group", Title: "添加机器人", InputSchema: map[string]any{"type": "object"}},
				{Name: "search_my_robots", Title: "搜索机器人", InputSchema: map[string]any{"type": "object"}},
			},
		},
	})

	if len(catalog.Products) != 1 {
		t.Fatalf("products len = %d, want 1", len(catalog.Products))
	}

	addTool, ok := catalog.Products[0].FindTool("add_robot_to_group")
	if !ok {
		t.Fatalf("FindTool(add_robot_to_group) = not found")
	}
	if addTool.CLIName != "add-bot" {
		t.Fatalf("CLIName = %q, want add-bot", addTool.CLIName)
	}
	if !addTool.Sensitive {
		t.Fatalf("Sensitive = false, want true")
	}
	if addTool.FlagHints["robot_code"].Alias != "code" {
		t.Fatalf("FlagHints[robot_code].Alias = %q, want code", addTool.FlagHints["robot_code"].Alias)
	}
	if addTool.FlagHints["group_id"].Alias != "group" {
		t.Fatalf("FlagHints[group_id].Alias = %q, want group", addTool.FlagHints["group_id"].Alias)
	}

	searchTool, ok := catalog.Products[0].FindTool("search_my_robots")
	if !ok {
		t.Fatalf("FindTool(search_my_robots) = not found")
	}
	if searchTool.CLIName != "search" {
		t.Fatalf("CLIName = %q, want search", searchTool.CLIName)
	}
	if !searchTool.Hidden {
		t.Fatalf("Hidden = false, want true")
	}
}

func schemaPropertyType(schema map[string]any, path string) string {
	current := schema
	for _, part := range strings.Split(path, ".") {
		properties, _ := current["properties"].(map[string]any)
		if properties == nil {
			return ""
		}
		next, _ := properties[part].(map[string]any)
		if next == nil {
			return ""
		}
		current = next
	}
	kind, _ := current["type"].(string)
	return kind
}

func TestBuildCatalogToolsOverridesTakesPrecedenceOverToolOverrides(t *testing.T) {
	t.Parallel()

	catalog := BuildCatalog([]discovery.RuntimeServer{
		{
			Server: market.ServerDescriptor{
				Key:         "doc-key",
				DisplayName: "文档",
				Endpoint:    "https://example.com/server/doc",
				CLI: market.CLIOverlay{
					ID: "doc",
					Tools: []market.CLITool{
						{Name: "create_document", CLIName: "create-from-tools", Hidden: boolPtr(true)},
					},
					ToolOverrides: map[string]market.CLIToolOverride{
						"create_document": {CLIName: "create-from-overrides", Hidden: boolPtr(false)},
					},
				},
			},
			Tools: []transport.ToolDescriptor{
				{Name: "create_document", Title: "创建文档", InputSchema: map[string]any{"type": "object"}},
			},
		},
	})

	tool, ok := catalog.Products[0].FindTool("create_document")
	if !ok {
		t.Fatalf("FindTool(create_document) = not found")
	}
	if tool.CLIName != "create-from-tools" {
		t.Fatalf("CLIName = %q, want create-from-tools (tools[] should take precedence)", tool.CLIName)
	}
	if !tool.Hidden {
		t.Fatalf("Hidden = false, want true (tools[] should take precedence)")
	}
}

func TestBuildCatalogPreservesSensitiveAndHiddenWhenOverrideOnlyAddsMetadata(t *testing.T) {
	t.Parallel()

	catalog := BuildCatalog([]discovery.RuntimeServer{
		{
			Server: market.ServerDescriptor{
				Key:         "doc-key",
				DisplayName: "文档",
				Endpoint:    "https://example.com/server/doc",
				CLI: market.CLIOverlay{
					ID: "doc",
					Tools: []market.CLITool{
						{Name: "create_document", Hidden: boolPtr(true)},
					},
					ToolOverrides: map[string]market.CLIToolOverride{
						"create_document": {
							CLIName: "create",
							Group:   "authoring",
							Flags: map[string]market.CLIFlagOverride{
								"title": {Alias: "name"},
							},
						},
					},
				},
			},
			Tools: []transport.ToolDescriptor{
				{
					Name:        "create_document",
					Title:       "创建文档",
					Sensitive:   true,
					InputSchema: map[string]any{"type": "object"},
				},
			},
		},
	})

	tool, ok := catalog.Products[0].FindTool("create_document")
	if !ok {
		t.Fatalf("FindTool(create_document) = not found")
	}
	if !tool.Sensitive {
		t.Fatalf("tool.Sensitive = false, want runtime sensitive preserved")
	}
	if !tool.Hidden {
		t.Fatalf("tool.Hidden = false, want prior hidden preserved")
	}
	if tool.CLIName != "create" {
		t.Fatalf("tool.CLIName = %q, want create", tool.CLIName)
	}
	if tool.Group != "authoring" {
		t.Fatalf("tool.Group = %q, want authoring", tool.Group)
	}
	if tool.FlagHints["title"].Alias != "name" {
		t.Fatalf("tool.FlagHints[title].Alias = %q, want name", tool.FlagHints["title"].Alias)
	}
}

func TestBuildCatalogPreservesExplicitEmptyStringOverrideDefault(t *testing.T) {
	t.Parallel()

	catalog := BuildCatalog([]discovery.RuntimeServer{
		{
			Server: market.ServerDescriptor{
				Key:         "doc-key",
				DisplayName: "文档",
				Endpoint:    "https://example.com/server/doc",
				CLI: market.CLIOverlay{
					ID: "doc",
					ToolOverrides: map[string]market.CLIToolOverride{
						"create_document": {
							Flags: map[string]market.CLIFlagOverride{
								"title": {Default: stringPtr("")},
							},
						},
					},
				},
			},
			Tools: []transport.ToolDescriptor{
				{
					Name: "create_document",
					InputSchema: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"title": map[string]any{
								"type":    "string",
								"default": "Untitled",
							},
						},
					},
				},
			},
		},
	})

	tool, ok := catalog.Products[0].FindTool("create_document")
	if !ok {
		t.Fatalf("FindTool(create_document) = not found")
	}
	if got := tool.FlagHints["title"].Default; got != "" {
		t.Fatalf("tool.FlagHints[title].Default = %#v, want explicit empty string", got)
	}
}

func TestBuildCatalogMergesCompatOnlyCLIMetadataWithDeterministicPrecedence(t *testing.T) {
	t.Parallel()

	catalog := BuildCatalog([]discovery.RuntimeServer{
		{
			Server: market.ServerDescriptor{
				Key:         "doc-key",
				DisplayName: "文档",
				Description: "运行时产品描述",
				Endpoint:    "https://example.com/server/doc",
				CLI: market.CLIOverlay{
					ID:          "doc",
					Command:     "documents",
					Aliases:     []string{"legacy-doc"},
					Group:       "office",
					Hidden:      true,
					Description: "CLI 产品描述",
					Tools: []market.CLITool{
						{
							Name:        "create_document",
							CLIName:     "create",
							Title:       "创建文档（CLI）",
							Description: "CLI 工具描述",
							IsSensitive: boolPtr(true),
							Hidden:      boolPtr(true),
							Group:       "authoring",
							Flags: map[string]market.CLIFlagHint{
								"title": {Alias: "name", Shorthand: "t"},
								"tags":  {Alias: "tag", Shorthand: "g"},
							},
						},
					},
					ToolOverrides: map[string]market.CLIToolOverride{
						"create_document": {
							CLIName:     "create-legacy",
							Group:       "legacy",
							IsSensitive: boolPtr(false),
							Hidden:      boolPtr(false),
							Flags: map[string]market.CLIFlagOverride{
								"title": {
									Alias:         "legacy-name",
									Transform:     "trim_space",
									TransformArgs: map[string]any{"side": "both"},
									Hidden:        boolPtr(true),
									Default:       stringPtr("Untitled"),
								},
								"tags": {
									Transform:     "csv_to_array",
									TransformArgs: map[string]any{"separator": ";"},
								},
							},
						},
					},
				},
			},
			Tools: []transport.ToolDescriptor{
				{
					Name:        "create_document",
					Title:       "运行时标题",
					Description: "来自 detail 的描述",
					InputSchema: map[string]any{
						"type":     "object",
						"required": []any{"title"},
						"properties": map[string]any{
							"title": map[string]any{"type": "string"},
							"tags":  map[string]any{"type": "array"},
						},
					},
					OutputSchema: map[string]any{"type": "object"},
					Sensitive:    false,
				},
			},
		},
	})

	product := catalog.Products[0]
	if product.ID != "doc" {
		t.Fatalf("product.ID = %q, want doc", product.ID)
	}
	if product.Description != "CLI 产品描述" {
		t.Fatalf("product.Description = %q, want CLI 产品描述", product.Description)
	}
	if product.ServiceDescription != "运行时产品描述" {
		t.Fatalf("product.ServiceDescription = %q, want 运行时产品描述", product.ServiceDescription)
	}
	if product.CLI == nil {
		t.Fatal("product.CLI = nil, want metadata")
	}
	if product.CLI.Command != "documents" {
		t.Fatalf("product.CLI.Command = %q, want documents", product.CLI.Command)
	}
	if !reflect.DeepEqual(product.CLI.Aliases, []string{"doc", "legacy-doc"}) {
		t.Fatalf("product.CLI.Aliases = %#v, want [doc legacy-doc]", product.CLI.Aliases)
	}

	tool, ok := product.FindTool("create_document")
	if !ok {
		t.Fatalf("FindTool(create_document) = not found")
	}
	if tool.CLIName != "create" {
		t.Fatalf("tool.CLIName = %q, want create", tool.CLIName)
	}
	if !reflect.DeepEqual(tool.Aliases, []string{"create_document", "create-legacy"}) {
		t.Fatalf("tool.Aliases = %#v, want [create_document create-legacy]", tool.Aliases)
	}
	if tool.CanonicalPath != "doc.create_document" {
		t.Fatalf("tool.CanonicalPath = %q, want doc.create_document", tool.CanonicalPath)
	}
	if tool.Title != "创建文档（CLI）" {
		t.Fatalf("tool.Title = %q, want 创建文档（CLI）", tool.Title)
	}
	if tool.Description != "CLI 工具描述" {
		t.Fatalf("tool.Description = %q, want CLI 工具描述", tool.Description)
	}
	if !tool.Hidden {
		t.Fatalf("tool.Hidden = false, want true")
	}
	if !tool.Sensitive {
		t.Fatalf("tool.Sensitive = false, want true")
	}
	if tool.Group != "authoring" {
		t.Fatalf("tool.Group = %q, want authoring", tool.Group)
	}
	if got := tool.OutputSchema["type"]; got != "object" {
		t.Fatalf("tool.OutputSchema[type] = %#v, want object", got)
	}

	titleHint := tool.FlagHints["title"]
	if titleHint.Alias != "name" || titleHint.Shorthand != "t" {
		t.Fatalf("tool.FlagHints[title] alias/shorthand = %#v, want alias=name shorthand=t", titleHint)
	}
	if titleHint.Transform != "trim_space" {
		t.Fatalf("tool.FlagHints[title].Transform = %q, want trim_space", titleHint.Transform)
	}
	if !reflect.DeepEqual(titleHint.TransformArgs, map[string]any{"side": "both"}) {
		t.Fatalf("tool.FlagHints[title].TransformArgs = %#v, want side=both", titleHint.TransformArgs)
	}
	if titleHint.Default != "Untitled" {
		t.Fatalf("tool.FlagHints[title].Default = %q, want Untitled", titleHint.Default)
	}
	if !titleHint.Hidden {
		t.Fatalf("tool.FlagHints[title].Hidden = false, want true")
	}
	if !titleHint.Required {
		t.Fatalf("tool.FlagHints[title].Required = false, want true")
	}

	tagsHint := tool.FlagHints["tags"]
	if tagsHint.Alias != "tag" || tagsHint.Shorthand != "g" {
		t.Fatalf("tool.FlagHints[tags] alias/shorthand = %#v, want alias=tag shorthand=g", tagsHint)
	}
	if tagsHint.Transform != "csv_to_array" {
		t.Fatalf("tool.FlagHints[tags].Transform = %q, want csv_to_array", tagsHint.Transform)
	}
	if !reflect.DeepEqual(tagsHint.TransformArgs, map[string]any{"separator": ";"}) {
		t.Fatalf("tool.FlagHints[tags].TransformArgs = %#v, want separator=;", tagsHint.TransformArgs)
	}
	if tagsHint.Required {
		t.Fatalf("tool.FlagHints[tags].Required = true, want false")
	}
}

func TestBuildCatalogRuntimeDefinesToolExistenceOutsideDegradedSynthesis(t *testing.T) {
	t.Parallel()

	catalog := BuildCatalog([]discovery.RuntimeServer{
		{
			Server: market.ServerDescriptor{
				Key:         "doc-key",
				DisplayName: "文档",
				Endpoint:    "https://example.com/server/doc",
				CLI: market.CLIOverlay{
					ID:      "doc",
					Command: "documents",
					Tools: []market.CLITool{
						{Name: "create_document", CLIName: "create"},
						{Name: "archive_document", CLIName: "archive"},
					},
					ToolOverrides: map[string]market.CLIToolOverride{
						"archive_document": {CLIName: "archive-legacy"},
					},
				},
			},
			Tools: []transport.ToolDescriptor{
				{Name: "create_document", InputSchema: map[string]any{"type": "object"}},
			},
			Degraded: false,
		},
	})

	product := catalog.Products[0]
	if len(product.Tools) != 1 {
		t.Fatalf("len(product.Tools) = %d, want 1", len(product.Tools))
	}
	if product.Tools[0].RPCName != "create_document" {
		t.Fatalf("product.Tools[0].RPCName = %q, want create_document", product.Tools[0].RPCName)
	}
}

func TestBuildCatalogSynthesizesToolsFromRegistryMetadataInDegradedMode(t *testing.T) {
	t.Parallel()

	catalog := BuildCatalog([]discovery.RuntimeServer{
		{
			Server: market.ServerDescriptor{
				Key:         "bot-key",
				DisplayName: "机器人",
				Endpoint:    "https://example.com/server/bot",
				CLI: market.CLIOverlay{
					ID:      "bot",
					Command: "bot",
					Tools: []market.CLITool{
						{
							Name:        "add_robot_to_group",
							CLIName:     "add",
							Title:       "添加机器人",
							Description: "通过注册表合成",
							Group:       "group",
							Flags: map[string]market.CLIFlagHint{
								"robot_code": {Alias: "code", Shorthand: "c"},
							},
						},
					},
					ToolOverrides: map[string]market.CLIToolOverride{
						"add_robot_to_group": {
							Flags: map[string]market.CLIFlagOverride{
								"robot_code": {
									Transform:     "trim_space",
									TransformArgs: map[string]any{"side": "both"},
								},
							},
						},
						"search_my_robots": {
							CLIName: "search",
							Group:   "query",
							Flags: map[string]market.CLIFlagOverride{
								"keyword": {Alias: "q"},
							},
						},
					},
				},
			},
			Tools:    nil,
			Degraded: true,
			Source:   "stale_cache",
		},
	})

	product := catalog.Products[0]
	if len(product.Tools) != 2 {
		t.Fatalf("len(product.Tools) = %d, want 2 synthesized tools", len(product.Tools))
	}

	addTool, ok := product.FindTool("add_robot_to_group")
	if !ok {
		t.Fatalf("FindTool(add_robot_to_group) = not found")
	}
	if addTool.CLIName != "add" {
		t.Fatalf("addTool.CLIName = %q, want add", addTool.CLIName)
	}
	if addTool.CanonicalPath != "bot.add_robot_to_group" {
		t.Fatalf("addTool.CanonicalPath = %q, want bot.add_robot_to_group", addTool.CanonicalPath)
	}
	if !reflect.DeepEqual(addTool.Aliases, []string{"add_robot_to_group"}) {
		t.Fatalf("addTool.Aliases = %#v, want [add_robot_to_group]", addTool.Aliases)
	}
	if addTool.InputSchema != nil {
		t.Fatalf("addTool.InputSchema = %#v, want nil synthesized schema", addTool.InputSchema)
	}
	if got := addTool.FlagHints["robot_code"].Transform; got != "trim_space" {
		t.Fatalf("addTool.FlagHints[robot_code].Transform = %q, want trim_space", got)
	}

	searchTool, ok := product.FindTool("search_my_robots")
	if !ok {
		t.Fatalf("FindTool(search_my_robots) = not found")
	}
	if searchTool.CLIName != "search" {
		t.Fatalf("searchTool.CLIName = %q, want search", searchTool.CLIName)
	}
	if !reflect.DeepEqual(searchTool.Aliases, []string{"search_my_robots"}) {
		t.Fatalf("searchTool.Aliases = %#v, want [search_my_robots]", searchTool.Aliases)
	}
	if searchTool.Group != "query" {
		t.Fatalf("searchTool.Group = %q, want query", searchTool.Group)
	}
	if searchTool.FlagHints["keyword"].Alias != "q" {
		t.Fatalf("searchTool.FlagHints[keyword].Alias = %q, want q", searchTool.FlagHints["keyword"].Alias)
	}
}

func TestBuildCatalogSynthesizesAITableTableGroupForUngroupedViewAndTransferTools(t *testing.T) {
	t.Parallel()

	catalog := BuildCatalog([]discovery.RuntimeServer{
		{
			Server: market.ServerDescriptor{
				Key:         "aitable-key",
				DisplayName: "AI 表格",
				Endpoint:    "https://example.com/server/aitable",
				CLI: market.CLIOverlay{
					ID:       "aitable",
					Command:  "aitable",
					Prefixes: []string{"table", "record", "field", "base", "attachment"},
					Groups: map[string]market.CLIGroupDef{
						"table": {Description: "数据表管理"},
					},
				},
			},
			Tools: []transport.ToolDescriptor{
				{Name: "create_view", InputSchema: map[string]any{"type": "object"}},
				{Name: "delete_view", InputSchema: map[string]any{"type": "object"}},
				{Name: "get_views", InputSchema: map[string]any{"type": "object"}},
				{Name: "update_view", InputSchema: map[string]any{"type": "object"}},
				{Name: "export_data", InputSchema: map[string]any{"type": "object"}},
				{Name: "import_data", InputSchema: map[string]any{"type": "object"}},
				{Name: "prepare_import_upload", InputSchema: map[string]any{"type": "object"}},
			},
		},
	})

	product, ok := catalog.FindProduct("aitable")
	if !ok {
		t.Fatalf("FindProduct(aitable) = not found")
	}

	for _, name := range []string{
		"create_view",
		"delete_view",
		"get_views",
		"update_view",
		"export_data",
		"import_data",
		"prepare_import_upload",
	} {
		tool, ok := product.FindTool(name)
		if !ok {
			t.Fatalf("FindTool(%s) = not found", name)
		}
		if tool.Group != "table" {
			t.Fatalf("%s group = %q, want table", name, tool.Group)
		}
	}
}

func TestBuildCatalogHidesUndeclaredRuntimeToolsWhenCLIContractExists(t *testing.T) {
	t.Parallel()

	catalog := BuildCatalog([]discovery.RuntimeServer{
		{
			Server: market.ServerDescriptor{
				Key:         "aitable-key",
				DisplayName: "AI 表格",
				Endpoint:    "https://example.com/server/aitable",
				CLI: market.CLIOverlay{
					ID: "aitable",
					Tools: []market.CLITool{
						{Name: "list_tables", CLIName: "get"},
					},
				},
			},
			Tools: []transport.ToolDescriptor{
				{Name: "list_tables", InputSchema: map[string]any{"type": "object"}},
				{Name: "create_view", InputSchema: map[string]any{"type": "object"}},
			},
		},
	})

	product, ok := catalog.FindProduct("aitable")
	if !ok {
		t.Fatalf("FindProduct(aitable) = not found")
	}

	listTool, ok := product.FindTool("list_tables")
	if !ok {
		t.Fatalf("FindTool(list_tables) = not found")
	}
	if listTool.Hidden {
		t.Fatalf("list_tables hidden = true, want false")
	}

	viewTool, ok := product.FindTool("create_view")
	if !ok {
		t.Fatalf("FindTool(create_view) = not found")
	}
	if !viewTool.Hidden {
		t.Fatalf("create_view hidden = false, want true")
	}
}

func TestBuildCatalogProjectsDeclaredFlagContractOntoRuntimeSchema(t *testing.T) {
	t.Parallel()

	catalog := BuildCatalog([]discovery.RuntimeServer{
		{
			Server: market.ServerDescriptor{
				Key:         "attendance-key",
				DisplayName: "考勤",
				Endpoint:    "https://example.com/server/attendance",
				CLI: market.CLIOverlay{
					ID: "attendance",
					ToolOverrides: map[string]market.CLIToolOverride{
						"get_attendance_summary": {
							CLIName: "summary",
							Flags: map[string]market.CLIFlagOverride{
								"userList":                    {Alias: "user"},
								"corpId":                      {Hidden: boolPtr(true)},
								"opUserId":                    {Hidden: boolPtr(true)},
								"QueryUserAttendVO.workDate":  {Alias: "date"},
								"QueryUserAttendVO.statsType": {Hidden: boolPtr(true)},
							},
						},
					},
				},
			},
			Tools: []transport.ToolDescriptor{
				{
					Name: "get_attendance_summary",
					InputSchema: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"userList": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
							"corpId":   map[string]any{"type": "string"},
							"opUserId": map[string]any{"type": "string"},
							"QueryUserAttendVO": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"queryDate": map[string]any{"type": "string"},
									"statsType": map[string]any{"type": "string"},
									"tagName":   map[string]any{"type": "string"},
								},
							},
						},
					},
				},
			},
		},
	})

	product, ok := catalog.FindProduct("attendance")
	if !ok {
		t.Fatalf("FindProduct(attendance) = not found")
	}
	tool, ok := product.FindTool("get_attendance_summary")
	if !ok {
		t.Fatalf("FindTool(get_attendance_summary) = not found")
	}

	if _, exists := tool.FlagHints["QueryUserAttendVO.workDate"]; exists {
		t.Fatalf("FlagHints unexpectedly kept unresolved workDate key: %#v", tool.FlagHints)
	}
	if got := tool.FlagHints["QueryUserAttendVO.queryDate"].Alias; got != "date" {
		t.Fatalf("FlagHints[QueryUserAttendVO.queryDate].Alias = %q, want date", got)
	}
	if !tool.FlagHints["corpId"].Hidden {
		t.Fatalf("FlagHints[corpId].Hidden = false, want true")
	}

	properties, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("tool.InputSchema.properties = %#v, want map", tool.InputSchema["properties"])
	}
	wrapper, ok := properties["QueryUserAttendVO"].(map[string]any)
	if !ok {
		t.Fatalf("properties[QueryUserAttendVO] = %#v, want map", properties["QueryUserAttendVO"])
	}
	wrapperProps, ok := wrapper["properties"].(map[string]any)
	if !ok {
		t.Fatalf("QueryUserAttendVO.properties = %#v, want map", wrapper["properties"])
	}
	if _, exists := wrapperProps["queryDate"]; !exists {
		t.Fatalf("QueryUserAttendVO.properties missing queryDate: %#v", wrapperProps)
	}
	if _, exists := wrapperProps["tagName"]; exists {
		t.Fatalf("QueryUserAttendVO.properties unexpectedly kept tagName: %#v", wrapperProps)
	}
}

func TestBuildCatalogSynthesizesMissingDeclaredFlagFromCLIContract(t *testing.T) {
	t.Parallel()

	catalog := BuildCatalog([]discovery.RuntimeServer{
		{
			Server: market.ServerDescriptor{
				Key:         "calendar-key",
				DisplayName: "日历",
				Endpoint:    "https://example.com/server/calendar",
				CLI: market.CLIOverlay{
					ID: "calendar",
					ToolOverrides: map[string]market.CLIToolOverride{
						"query_available_meeting_room": {
							CLIName: "search",
							Group:   "room",
							Flags: map[string]market.CLIFlagOverride{
								"startTime":     {Alias: "start"},
								"endTime":       {Alias: "end"},
								"needAvailable": {Alias: "available"},
							},
						},
					},
				},
			},
			Tools: []transport.ToolDescriptor{
				{
					Name: "query_available_meeting_room",
					InputSchema: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"startTime": map[string]any{"type": "string"},
							"endTime":   map[string]any{"type": "string"},
							"groupId":   map[string]any{"type": "string"},
						},
					},
				},
			},
		},
	})

	product, ok := catalog.FindProduct("calendar")
	if !ok {
		t.Fatalf("FindProduct(calendar) = not found")
	}
	tool, ok := product.FindTool("query_available_meeting_room")
	if !ok {
		t.Fatalf("FindTool(query_available_meeting_room) = not found")
	}

	if got := tool.FlagHints["needAvailable"].Alias; got != "available" {
		t.Fatalf("FlagHints[needAvailable].Alias = %q, want available", got)
	}

	properties, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("tool.InputSchema.properties = %#v, want map", tool.InputSchema["properties"])
	}
	if _, exists := properties["needAvailable"]; !exists {
		t.Fatalf("projected schema missing needAvailable: %#v", properties)
	}
	if _, exists := properties["groupId"]; exists {
		t.Fatalf("projected schema unexpectedly kept undeclared groupId: %#v", properties)
	}
}

func TestMergeToolFlagHintsPreservesExistingShorthandWhenIncomingHintLeavesItBlank(t *testing.T) {
	t.Parallel()

	got := mergeToolFlagHints(map[string]CLIFlagHint{
		"title": {
			Alias:     "legacy-name",
			Shorthand: "n",
		},
	}, map[string]market.CLIFlagHint{
		"title": {
			Alias: "name",
		},
	})

	if got["title"].Shorthand != "n" {
		t.Fatalf("mergeToolFlagHints(...)[title].Shorthand = %q, want n preserved", got["title"].Shorthand)
	}
	if got["title"].Alias != "name" {
		t.Fatalf("mergeToolFlagHints(...)[title].Alias = %q, want name", got["title"].Alias)
	}
}
