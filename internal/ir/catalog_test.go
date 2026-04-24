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
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/discovery"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/market"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

func TestBuildCatalogUsesEndpointSlugAndCanonicalPaths(t *testing.T) {
	t.Parallel()

	catalog := BuildCatalog([]discovery.RuntimeServer{
		{
			Server: market.ServerDescriptor{
				Key:         "doc-key",
				DisplayName: "文档",
				Endpoint:    "https://example.com/server/doc",
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

func TestBuildCatalogAppliesCanonicalProductAlias(t *testing.T) {
	t.Parallel()

	catalog := BuildCatalog([]discovery.RuntimeServer{
		{
			Server: market.ServerDescriptor{
				Key:         "aitable-key",
				DisplayName: "多维表格",
				Endpoint:    "https://example.com/server/table",
				CLI: market.CLIOverlay{
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
		t.Fatalf("BuildCatalog() product ID = %q, want aitable", got)
	}
	if got := catalog.Products[0].Tools[0].CanonicalPath; got != "aitable.list_tables" {
		t.Fatalf("BuildCatalog() canonical path = %q, want aitable.list_tables", got)
	}
}

func TestBuildCatalogFallsBackToHashOnSlugCollision(t *testing.T) {
	t.Parallel()

	catalog := BuildCatalog([]discovery.RuntimeServer{
		{
			Server: market.ServerDescriptor{
				Key:         "first-key",
				DisplayName: "文档一",
				Endpoint:    "https://example.com/server/doc",
			},
		},
		{
			Server: market.ServerDescriptor{
				Key:         "second-key",
				DisplayName: "文档二",
				Endpoint:    "https://example.com/another/doc",
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
		t.Fatalf("BuildCatalog() second product ID unexpectedly reused base slug")
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
					Tools: []market.CLITool{
						{Name: "create_document", IsSensitive: true},
						{Name: "search_documents", IsSensitive: false},
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

func TestBuildCatalogCarriesToolOverrideGroupAndFlagOverlay(t *testing.T) {
	t.Parallel()

	catalog := BuildCatalog([]discovery.RuntimeServer{
		{
			Server: market.ServerDescriptor{
				Key:         "ding-key",
				DisplayName: "DING",
				Endpoint:    "https://example.com/server/ding",
				CLI: market.CLIOverlay{
					Command: "ding",
					ToolOverrides: map[string]market.CLIToolOverride{
						"send_ding_message": {
							CLIName:     "send",
							Group:       "message",
							IsSensitive: true,
							Flags: map[string]market.CLIFlagOverride{
								"receiverUserIdList": {
									Alias:     "users",
									Transform: "csv_to_array",
								},
								"robotCode": {
									Alias:      "robot-code",
									EnvDefault: "DINGTALK_DING_ROBOT_CODE",
								},
								"remindType": {
									Alias:     "type",
									Transform: "enum_map",
									TransformArgs: map[string]any{
										"_default": float64(1),
										"app":      float64(1),
									},
								},
							},
						},
					},
				},
			},
			Tools: []transport.ToolDescriptor{
				{Name: "send_ding_message", Title: "发送DING消息", InputSchema: map[string]any{"type": "object"}},
			},
		},
	})

	if len(catalog.Products) != 1 {
		t.Fatalf("products len = %d, want 1", len(catalog.Products))
	}
	product := catalog.Products[0]
	if product.ID != "ding" {
		t.Fatalf("product ID = %q, want ding", product.ID)
	}
	tool, ok := product.FindTool("send_ding_message")
	if !ok {
		t.Fatalf("tool not found")
	}
	if tool.CLIName != "send" {
		t.Fatalf("CLIName = %q, want send", tool.CLIName)
	}
	if tool.Group != "message" {
		t.Fatalf("Group = %q, want message", tool.Group)
	}
	if !tool.Sensitive {
		t.Fatalf("Sensitive = false, want true (from overlay)")
	}
	if tool.Annotations == nil || tool.Annotations.DestructiveHint == nil || !*tool.Annotations.DestructiveHint {
		t.Fatalf("DestructiveHint not propagated, got %#v", tool.Annotations)
	}
	if tool.Annotations.ReadOnlyHint != nil {
		t.Fatalf("ReadOnlyHint should be nil when source is unknown, got %#v", *tool.Annotations.ReadOnlyHint)
	}
	users := tool.FlagOverlay["receiverUserIdList"]
	if users.Alias != "users" || users.Transform != "csv_to_array" {
		t.Fatalf("receiverUserIdList overlay = %#v", users)
	}
	robot := tool.FlagOverlay["robotCode"]
	if robot.Alias != "robot-code" || robot.EnvDefault != "DINGTALK_DING_ROBOT_CODE" {
		t.Fatalf("robotCode overlay = %#v", robot)
	}
	remind := tool.FlagOverlay["remindType"]
	if remind.Transform != "enum_map" {
		t.Fatalf("remindType.Transform = %q, want enum_map", remind.Transform)
	}
	if v, _ := remind.TransformArgs["app"].(float64); v != 1 {
		t.Fatalf("remindType.TransformArgs[app] = %v, want 1", remind.TransformArgs["app"])
	}
}

func TestBuildCatalogLeavesAnnotationsNilForNonSensitive(t *testing.T) {
	t.Parallel()

	catalog := BuildCatalog([]discovery.RuntimeServer{
		{
			Server: market.ServerDescriptor{
				Key:         "doc-key",
				DisplayName: "文档",
				Endpoint:    "https://example.com/server/doc",
			},
			Tools: []transport.ToolDescriptor{
				{Name: "list_documents", InputSchema: map[string]any{"type": "object"}},
			},
		},
	})
	tool := catalog.Products[0].Tools[0]
	if tool.Annotations != nil {
		t.Fatalf("Annotations = %#v, want nil for non-sensitive tool", tool.Annotations)
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
							Hidden:      true,
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
