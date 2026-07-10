// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/market"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

func TestMetadataFromRegistrySanitizesAndMapsOverrides(t *testing.T) {
	snapshot := cache.RegistrySnapshot{
		SavedAt: time.Now(),
		Servers: []market.ServerDescriptor{{
			Endpoint: "https://secret.example/server/calendar",
			CLI: market.CLIOverlay{
				ID: "calendar",
				ToolOverrides: map[string]market.CLIToolOverride{
					"list_calendar_events": {
						Description: "查询日程",
						Flags: map[string]market.CLIFlagOverride{
							"startTime": {Type: "string", Description: "开始时间"},
							"limit":     {Type: "int", Default: "20", Required: true},
						},
					},
					"search_my_robots": {
						ServerOverride: "bot",
						Description:    "查询机器人",
					},
					"legacy": {RedirectTo: "calendar event list", Description: "旧入口"},
				},
			},
		}},
	}

	metadata := metadataFromRegistry(snapshot)
	list := metadata.Tools["calendar.list_calendar_events"]
	if list.Description != "查询日程" || list.Parameters["startTime"].Type != "string" {
		t.Fatalf("calendar metadata = %#v", list)
	}
	limit := list.Parameters["limit"]
	if limit.Type != "integer" || limit.Default != "20" || limit.Required == nil || !*limit.Required {
		t.Fatalf("limit metadata = %#v", limit)
	}
	if got := metadata.Tools["bot.search_my_robots"].Description; got != "查询机器人" {
		t.Fatalf("serverOverride metadata = %q", got)
	}
	if _, exists := metadata.Tools["calendar.legacy"]; exists {
		t.Fatal("redirect metadata must be omitted")
	}

	encoded, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if strings.Contains(string(encoded), "secret.example") || strings.Contains(string(encoded), "saved_at") {
		t.Fatalf("generated metadata leaked registry transport data: %s", encoded)
	}
}

func TestMetadataFromRegistryAndToolsPrefersMCPInterfaceDescriptions(t *testing.T) {
	snapshot := cache.RegistrySnapshot{
		Servers: []market.ServerDescriptor{{
			Key: "calendar-key",
			CLI: market.CLIOverlay{
				ID: "calendar",
				ToolOverrides: map[string]market.CLIToolOverride{
					"get_calendar_detail": {
						Description: "CLI overlay description",
						Flags: map[string]market.CLIFlagOverride{
							"eventId": {Alias: "id", Required: true, Description: "CLI event ID"},
						},
					},
					"registry_only": {Description: "Registry-only tool"},
				},
			},
		}},
	}
	tools := []cache.ToolsSnapshot{{
		ServerKey: "calendar-key",
		Tools: []transport.ToolDescriptor{{
			Name:        "get_calendar_detail",
			Title:       "Get calendar detail",
			Description: "MCP tools/list description",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"eventId":    map[string]any{"type": "string", "description": "MCP event ID"},
					"calendarId": map[string]any{"type": "string", "description": "MCP calendar ID"},
				},
				"required": []any{"eventId"},
			},
		}},
	}}

	metadata := metadataFromRegistryAndTools(snapshot, tools)
	detail := metadata.Tools["calendar.get_calendar_detail"]
	if detail.Description != "MCP tools/list description" || detail.Title != "Get calendar detail" {
		t.Fatalf("tool metadata = %#v", detail)
	}
	if detail.Parameters["eventId"].Description != "MCP event ID" || detail.Parameters["calendarId"].Description != "MCP calendar ID" {
		t.Fatalf("MCP parameter metadata = %#v", detail.Parameters)
	}
	if detail.Parameters["eventId"].Required == nil || !*detail.Parameters["eventId"].Required {
		t.Fatalf("eventId required = %#v", detail.Parameters["eventId"])
	}
	if got := metadata.Tools["calendar.registry_only"].Description; got != "Registry-only tool" {
		t.Fatalf("registry fallback description = %q", got)
	}
}

func TestProjectMetadataToSurfaceUsesRuntimeAndHintInterfaceRefs(t *testing.T) {
	source := metadataFile{
		Coverage: metadataCoverage{SourceTools: 3},
		Tools: map[string]toolMetadata{
			"im.add_reaction": {
				Description: "Add a reaction",
			},
			"aitable.create_base": {
				Description: "Create a base",
			},
			"unused.remote_tool": {
				Description: "Must not enter public Schema",
			},
		},
	}
	surface := interfaceSurfaceSnapshot{
		Version: 1,
		Products: []interfaceSurfaceProduct{
			{ID: "chat", Tools: []interfaceSurfaceTool{
				{CanonicalPath: "chat.add_reaction", SourceProductID: "im"},
			}},
			{ID: "aitable", Tools: []interfaceSurfaceTool{
				{CanonicalPath: "aitable.base_create"},
				{CanonicalPath: "aitable.local_only"},
			}},
		},
	}
	refs := map[string]interfaceRef{
		"aitable.base_create": {ProductID: "aitable", RPCName: "create_base"},
	}

	projected := projectMetadataToSurface(source, surface, refs)
	if len(projected.Tools) != 2 {
		t.Fatalf("projected tools = %#v", projected.Tools)
	}
	chat := projected.Tools["chat.add_reaction"]
	if chat.InterfaceRef == nil || chat.InterfaceRef.ProductID != "im" || chat.InterfaceRef.RPCName != "add_reaction" {
		t.Fatalf("chat interface ref = %#v", chat.InterfaceRef)
	}
	base := projected.Tools["aitable.base_create"]
	if base.Description != "Create a base" || base.InterfaceRef == nil || base.InterfaceRef.RPCName != "create_base" {
		t.Fatalf("base metadata = %#v", base)
	}
	if _, ok := projected.Tools["unused.remote_tool"]; ok {
		t.Fatal("metadata outside the public command surface must be omitted")
	}
	coverage := projected.Coverage
	if coverage.SourceTools != 3 || coverage.SurfaceTools != 3 || coverage.MatchedTools != 2 || coverage.AliasedTools != 2 || coverage.UnmatchedTools != 1 {
		t.Fatalf("coverage = %#v", coverage)
	}
}

func TestRegistryCoverageReportsMissingStaticServerSnapshots(t *testing.T) {
	snapshot := cache.RegistrySnapshot{Servers: []market.ServerDescriptor{
		{Key: "calendar"},
		{Key: "notify"},
	}}
	coverage := registryCoverage(snapshot, []cache.ToolsSnapshot{{ServerKey: "calendar"}})
	if coverage.SourceServices != 2 || coverage.SnapshotServices != 1 || len(coverage.MissingServices) != 1 || coverage.MissingServices[0] != "notify" {
		t.Fatalf("coverage = %#v", coverage)
	}
}
