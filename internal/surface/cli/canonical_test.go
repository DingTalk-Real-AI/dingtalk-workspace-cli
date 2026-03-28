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

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/discovery"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/ir"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/market"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/transport"
	"github.com/spf13/cobra"
)

func TestBuildFlagSpecsGeneratesOnlySupportedTopLevelFlags(t *testing.T) {
	t.Parallel()

	specs := BuildFlagSpecs(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"title": map[string]any{
				"type":        "string",
				"description": "Document title",
			},
			"notify": map[string]any{
				"type": "boolean",
			},
			"metadata": map[string]any{
				"type": "object",
			},
			"tags": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "string",
				},
			},
			"publish_at": map[string]any{
				"type": "integer",
			},
			"visibility": map[string]any{
				"type": "integer",
			},
		},
	}, map[string]ir.CLIFlagHint{
		"title": {
			Shorthand: "t",
			Alias:     "name",
		},
		"publish_at": {
			Transform: "iso8601_to_millis",
		},
		"visibility": {
			Transform: "enum_map",
		},
	})

	if len(specs) != 6 {
		t.Fatalf("BuildFlagSpecs() len = %d, want 6", len(specs))
	}
	if specs[0].PropertyName != "metadata" || specs[1].PropertyName != "notify" || specs[2].PropertyName != "publish_at" || specs[3].PropertyName != "tags" || specs[4].PropertyName != "title" || specs[5].PropertyName != "visibility" {
		t.Fatalf("BuildFlagSpecs() unexpected order = %#v", specs)
	}
	if specs[0].Kind != "json" {
		t.Fatalf("BuildFlagSpecs() metadata kind = %q, want json", specs[0].Kind)
	}
	if specs[2].Kind != flagString {
		t.Fatalf("BuildFlagSpecs() publish_at kind = %q, want string", specs[2].Kind)
	}
	if specs[4].FlagName != "name" || specs[4].Alias != "title" || specs[4].Shorthand != "t" {
		t.Fatalf("BuildFlagSpecs() title hints = %#v, want flag=name alias=title shorthand=t", specs[4])
	}
	if specs[5].Kind != flagString {
		t.Fatalf("BuildFlagSpecs() visibility kind = %q, want string", specs[5].Kind)
	}
}

func TestBuildFlagSpecsUsesPublicAliasAsVisibleFlagName(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "create"}
	specs := []FlagSpec{
		{
			PropertyName: "keyword",
			FlagName:     "query",
			Alias:        "keyword",
			Kind:         flagString,
			Description:  "搜索关键词",
		},
	}

	applyFlagSpecs(cmd, specs)

	flag := cmd.Flags().Lookup("query")
	if flag == nil {
		t.Fatal("Lookup(query) = nil")
	}
	if got := flag.Usage; got != "搜索关键词" {
		t.Fatalf("flag.Usage = %q, want public alias usage only", got)
	}
	if alias := cmd.Flags().Lookup("keyword"); alias == nil {
		t.Fatal("Lookup(keyword) = nil")
	} else if !alias.Hidden {
		t.Fatalf("keyword.Hidden = false, want true")
	}
}

func TestBuildFlagSpecsPreservesHiddenHints(t *testing.T) {
	t.Parallel()

	specs := BuildFlagSpecs(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"title": map[string]any{"type": "string"},
		},
	}, map[string]ir.CLIFlagHint{
		"title": {Hidden: true},
	})

	if len(specs) != 1 {
		t.Fatalf("BuildFlagSpecs() len = %d, want 1", len(specs))
	}
	if !specs[0].Hidden {
		t.Fatalf("specs[0].Hidden = false, want true")
	}
}

func TestApplyFlagSpecsHidesPrimaryFlagMarkedHidden(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "create"}
	specs := []FlagSpec{
		{
			PropertyName: "title",
			FlagName:     "title",
			Kind:         flagString,
			Description:  "Document title",
			Hidden:       true,
		},
	}

	applyFlagSpecs(cmd, specs)

	flag := cmd.Flags().Lookup("title")
	if flag == nil {
		t.Fatal("Lookup(title) = nil")
	}
	if !flag.Hidden {
		t.Fatalf("flag.Hidden = false, want true")
	}
}

func TestSchemaPayloadOmitsHiddenFlags(t *testing.T) {
	t.Parallel()

	payload, err := schemaPayload(ir.Catalog{
		Products: []ir.CanonicalProduct{
			{
				ID: "doc",
				Tools: []ir.ToolDescriptor{
					{
						RPCName:       "create_document",
						CanonicalPath: "doc.create_document",
						InputSchema: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"title": map[string]any{"type": "string"},
								"debug": map[string]any{"type": "string"},
							},
						},
						FlagHints: map[string]ir.CLIFlagHint{
							"debug": {Hidden: true},
						},
					},
				},
			},
		},
	}, []string{"doc.create_document"})
	if err != nil {
		t.Fatalf("schemaPayload() error = %v", err)
	}

	flags, ok := payload["flags"].([]FlagSpec)
	if !ok {
		t.Fatalf("payload[flags] = %#v, want []FlagSpec", payload["flags"])
	}
	for _, spec := range flags {
		if spec.PropertyName == "debug" {
			t.Fatalf("schemaPayload() unexpectedly exposed hidden flag: %#v", flags)
		}
	}
}

func TestBuildFlagSpecsFlattensNestedWrapperObjectFields(t *testing.T) {
	t.Parallel()

	specs := BuildFlagSpecs(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"OpenSearchRequest": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"cursor": map[string]any{"type": "string", "description": "分页游标"},
					"query":  map[string]any{"type": "string", "description": "搜索关键词"},
				},
				"required": []any{"query"},
			},
		},
	}, map[string]ir.CLIFlagHint{
		"OpenSearchRequest.cursor": {Alias: "cursor"},
		"OpenSearchRequest.query":  {Alias: "query"},
	})

	if len(specs) != 2 {
		t.Fatalf("BuildFlagSpecs() len = %d, want 2", len(specs))
	}
	if got := specs[0]; got.PropertyName != "OpenSearchRequest.cursor" || got.FlagName != "cursor" {
		t.Fatalf("specs[0] = %#v, want OpenSearchRequest.cursor -> cursor", got)
	}
	if got := specs[1]; got.PropertyName != "OpenSearchRequest.query" || got.FlagName != "query" {
		t.Fatalf("specs[1] = %#v, want OpenSearchRequest.query -> query", got)
	}
	for _, spec := range specs {
		if spec.PropertyName == "OpenSearchRequest" {
			t.Fatalf("BuildFlagSpecs() unexpectedly kept wrapper object flag: %#v", specs)
		}
	}
}

func TestBuildFlagSpecsFallsBackToJSONObjectForDynamicObjects(t *testing.T) {
	t.Parallel()

	specs := BuildFlagSpecs(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"SearchRequest": map[string]any{
				"type":                 "object",
				"additionalProperties": true,
				"properties": map[string]any{
					"query": map[string]any{"type": "string"},
				},
			},
		},
	}, nil)

	if len(specs) != 1 {
		t.Fatalf("BuildFlagSpecs() len = %d, want 1", len(specs))
	}
	if got := specs[0]; got.PropertyName != "SearchRequest" || got.Kind != flagJSON {
		t.Fatalf("specs[0] = %#v, want SearchRequest json fallback", got)
	}
}

func TestBuildFlagSpecsUsesPathFallbackForNestedNameCollisions(t *testing.T) {
	t.Parallel()

	specs := BuildFlagSpecs(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"A": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string"},
				},
			},
			"B": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string"},
				},
			},
		},
	}, nil)

	if len(specs) != 2 {
		t.Fatalf("BuildFlagSpecs() len = %d, want 2", len(specs))
	}
	got := map[string]string{}
	for _, spec := range specs {
		got[spec.PropertyName] = spec.FlagName
	}
	want := map[string]string{
		"A.query": "a-query",
		"B.query": "b-query",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("flag names = %#v, want %#v", got, want)
	}
}

func TestBuildFlagSpecsHelpRendersAliasInlineOnce(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{
		Use: "search",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	specs := []FlagSpec{
		{
			PropertyName: "keyword",
			FlagName:     "query",
			Alias:        "keyword",
			Kind:         flagString,
			Description:  "搜索关键词",
		},
	}

	applyFlagSpecs(cmd, specs)

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	help := out.String()
	if !strings.Contains(help, "--query string") {
		t.Fatalf("help missing public alias flag:\n%s", help)
	}
	if strings.Contains(help, "--keyword") {
		t.Fatalf("help unexpectedly exposed canonical fallback flag:\n%s", help)
	}
	if strings.Contains(help, "(alias: --keyword)") {
		t.Fatalf("help unexpectedly exposed inline alias marker:\n%s", help)
	}
}

func TestCommandHelpSuppressesLegacyAliasSection(t *testing.T) {
	t.Parallel()

	cmd := NewMCPCommand(StaticLoader{
		Catalog: ir.Catalog{
			Products: []ir.CanonicalProduct{
				{
					ID: "group-chat",
					CLI: &ir.ProductCLIMetadata{
						Command: "chat",
					},
					Tools: []ir.ToolDescriptor{
						{
							RPCName:       "search_groups_by_keyword",
							CLIName:       "search",
							CanonicalPath: "group-chat.search_groups_by_keyword",
							InputSchema:   map[string]any{"type": "object"},
						},
					},
				},
			},
		},
	}, executor.EchoRunner{})

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"chat", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute(chat --help) error = %v", err)
	}

	productHelp := out.String()
	if strings.Contains(productHelp, "Aliases:") {
		t.Fatalf("product help unexpectedly renders aliases section:\n%s", productHelp)
	}
	if strings.Contains(productHelp, "group-chat") {
		t.Fatalf("product help unexpectedly exposes canonical product route:\n%s", productHelp)
	}

	out.Reset()
	cmd.SetArgs([]string{"chat", "search", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute(chat search --help) error = %v", err)
	}

	toolHelp := out.String()
	if strings.Contains(toolHelp, "Aliases:") {
		t.Fatalf("tool help unexpectedly renders aliases section:\n%s", toolHelp)
	}
	if strings.Contains(toolHelp, "search_groups_by_keyword") {
		t.Fatalf("tool help unexpectedly exposes canonical tool route:\n%s", toolHelp)
	}
}

func TestToolHelpHidesJSONFallbackFlags(t *testing.T) {
	t.Parallel()

	cmd := NewMCPCommand(StaticLoader{
		Catalog: ir.Catalog{
			Products: []ir.CanonicalProduct{
				{
					ID: "doc",
					CLI: &ir.ProductCLIMetadata{
						Command: "documents",
					},
					Tools: []ir.ToolDescriptor{
						{
							RPCName:       "create_document",
							CLIName:       "create",
							CanonicalPath: "doc.create_document",
							InputSchema: map[string]any{
								"type": "object",
								"properties": map[string]any{
									"title": map[string]any{"type": "string"},
								},
							},
						},
					},
				},
			},
		},
	}, executor.EchoRunner{})

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"documents", "create", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute(documents create --help) error = %v", err)
	}

	help := out.String()
	if strings.Contains(help, "--json") {
		t.Fatalf("help unexpectedly exposed --json:\n%s", help)
	}
	if strings.Contains(help, "--params") {
		t.Fatalf("help unexpectedly exposed --params:\n%s", help)
	}

	create := commandByName(commandByName(cmd, "documents"), "create")
	if create == nil {
		t.Fatal("commandByName(documents create) = nil")
	}
	if create.Flags().Lookup("json") == nil {
		t.Fatal("Lookup(json) = nil, want hidden flag to remain registered")
	}
	if create.Flags().Lookup("params") == nil {
		t.Fatal("Lookup(params) = nil, want hidden flag to remain registered")
	}
}

func TestToolCommandFlattensNestedWrapperFlags(t *testing.T) {
	t.Parallel()

	runner := &captureRunner{}
	cmd := NewMCPCommand(StaticLoader{
		Catalog: ir.Catalog{
			Products: []ir.CanonicalProduct{
				{
					ID: "group-chat",
					CLI: &ir.ProductCLIMetadata{
						Command: "chat",
					},
					Tools: []ir.ToolDescriptor{
						{
							RPCName:       "search_groups_by_keyword",
							CLIName:       "search",
							CanonicalPath: "group-chat.search_groups_by_keyword",
							InputSchema: map[string]any{
								"type": "object",
								"properties": map[string]any{
									"OpenSearchRequest": map[string]any{
										"type": "object",
										"properties": map[string]any{
											"cursor": map[string]any{"type": "string"},
											"query":  map[string]any{"type": "string"},
										},
										"required": []any{"query"},
									},
								},
							},
							FlagHints: map[string]ir.CLIFlagHint{
								"OpenSearchRequest.cursor": {Alias: "cursor"},
								"OpenSearchRequest.query":  {Alias: "query"},
							},
						},
					},
				},
			},
		},
	}, runner)

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"chat", "search", "--query", "项目冲刺", "--cursor", "next-page"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	request, ok := runner.last.Params["OpenSearchRequest"].(map[string]any)
	if !ok {
		t.Fatalf("runner.last.Params[OpenSearchRequest] = %#v, want nested map", runner.last.Params["OpenSearchRequest"])
	}
	if got := request["query"]; got != "项目冲刺" {
		t.Fatalf("request[query] = %#v, want 项目冲刺", got)
	}
	if got := request["cursor"]; got != "next-page" {
		t.Fatalf("request[cursor] = %#v, want next-page", got)
	}
}

func TestFixtureLoaderLoadsCatalog(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	fixturePath := filepath.Join(dir, "catalog.json")
	data := []byte(`{"products":[{"id":"doc","display_name":"文档","server_key":"doc-key","endpoint":"https://example.com/server/doc","tools":[{"rpc_name":"create_document","canonical_path":"doc.create_document"}]}]}`)
	if err := os.WriteFile(fixturePath, data, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	catalog, err := FixtureLoader{Path: fixturePath}.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(catalog.Products) != 1 || catalog.Products[0].ID != "doc" {
		t.Fatalf("Load() catalog = %#v, want doc product", catalog)
	}
}

func TestSchemaPayloadFindsTool(t *testing.T) {
	t.Parallel()

	payload, err := schemaPayload(ir.Catalog{
		Products: []ir.CanonicalProduct{
			{
				ID: "doc",
				Tools: []ir.ToolDescriptor{
					{
						RPCName:       "create_document",
						CanonicalPath: "doc.create_document",
						InputSchema: map[string]any{
							"type": "object",
							"required": []any{
								"title",
							},
						},
					},
				},
			},
		},
	}, []string{"doc.create_document"})
	if err != nil {
		t.Fatalf("schemaPayload() error = %v", err)
	}
	if payload["kind"] != "schema" {
		t.Fatalf("schemaPayload() kind = %#v, want schema", payload["kind"])
	}
}

func TestSchemaPayloadReturnsFocusedToolSchema(t *testing.T) {
	t.Parallel()

	payload, err := schemaPayload(ir.Catalog{
		Products: []ir.CanonicalProduct{
			{
				ID:          "bot",
				DisplayName: "机器人消息",
				CLI: &ir.ProductCLIMetadata{
					Command: "chat",
				},
				Tools: []ir.ToolDescriptor{
					{
						RPCName:       "create_robot",
						CLIName:       "create_robot",
						Title:         "创建企业机器人",
						Description:   "创建企业机器人",
						CanonicalPath: "bot.create_robot",
						InputSchema: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"robot_name": map[string]any{"type": "string", "description": "机器人名称"},
							},
							"required": []any{"robot_name"},
						},
						OutputSchema: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"robotCode": map[string]any{"type": "string"},
							},
						},
						FlagHints: map[string]ir.CLIFlagHint{
							"robot_name": {Alias: "robot-name"},
						},
					},
					{
						RPCName:       "search_my_robots",
						CanonicalPath: "bot.search_my_robots",
					},
				},
			},
		},
	}, []string{"bot.create_robot"})
	if err != nil {
		t.Fatalf("schemaPayload() error = %v", err)
	}

	if got := payload["scope"]; got != "tool" {
		t.Fatalf("schemaPayload() scope = %#v, want tool", got)
	}
	if got := payload["path"]; got != "bot.create_robot" {
		t.Fatalf("schemaPayload() path = %#v, want bot.create_robot", got)
	}
	if got := payload["cli_path"]; !reflect.DeepEqual(got, []string{"chat", "create_robot"}) {
		t.Fatalf("schemaPayload() cli_path = %#v, want [chat create_robot]", got)
	}
	if _, ok := payload["required"].([]string); !ok {
		t.Fatalf("schemaPayload() required = %#v, want []string", payload["required"])
	}
	if got := payload["input_schema"].(map[string]any)["type"]; got != "object" {
		t.Fatalf("schemaPayload() input_schema.type = %#v, want object", got)
	}
	if got := payload["output_schema"].(map[string]any)["type"]; got != "object" {
		t.Fatalf("schemaPayload() output_schema.type = %#v, want object", got)
	}
	flags, ok := payload["flags"].([]FlagSpec)
	if !ok {
		t.Fatalf("schemaPayload() flags = %#v, want []FlagSpec", payload["flags"])
	}
	if len(flags) != 1 || flags[0].FlagName != "robot-name" {
		t.Fatalf("schemaPayload() flags = %#v, want robot-name flag", flags)
	}
	flagHints, ok := payload["flag_hints"].(map[string]ir.CLIFlagHint)
	if !ok {
		t.Fatalf("schemaPayload() flag_hints = %#v, want map[string]ir.CLIFlagHint", payload["flag_hints"])
	}
	if got := flagHints["robot_name"].Alias; got != "robot-name" {
		t.Fatalf("schemaPayload() flag_hints[robot_name].Alias = %#v, want robot-name", got)
	}
	if _, ok := payload["products"]; ok {
		t.Fatalf("schemaPayload() unexpectedly exposed full catalog payload: %#v", payload)
	}
}

func TestSchemaPayloadIncludesGroupedCLIPath(t *testing.T) {
	t.Parallel()

	payload, err := schemaPayload(ir.Catalog{
		Products: []ir.CanonicalProduct{
			{
				ID: "todo",
				Tools: []ir.ToolDescriptor{
					{
						RPCName:       "get_user_todos_in_current_org",
						CLIName:       "list",
						Group:         "task",
						CanonicalPath: "todo.get_user_todos_in_current_org",
						InputSchema:   map[string]any{"type": "object"},
					},
				},
			},
		},
	}, []string{"todo.get_user_todos_in_current_org"})
	if err != nil {
		t.Fatalf("schemaPayload() error = %v", err)
	}

	if got := payload["cli_path"]; !reflect.DeepEqual(got, []string{"todo", "task", "list"}) {
		t.Fatalf("schemaPayload() cli_path = %#v, want [todo task list]", got)
	}
}

func TestSchemaPayloadResolvesGroupedCLIPathQuery(t *testing.T) {
	t.Parallel()

	payload, err := schemaPayload(ir.Catalog{
		Products: []ir.CanonicalProduct{
			{
				ID: "todo",
				Tools: []ir.ToolDescriptor{
					{
						RPCName:       "get_user_todos_in_current_org",
						CLIName:       "list",
						Group:         "task",
						CanonicalPath: "todo.get_user_todos_in_current_org",
						InputSchema:   map[string]any{"type": "object"},
					},
				},
			},
		},
	}, []string{"todo.task.list"})
	if err != nil {
		t.Fatalf("schemaPayload() error = %v", err)
	}

	if got := payload["path"]; got != "todo.get_user_todos_in_current_org" {
		t.Fatalf("schemaPayload() path = %#v, want todo.get_user_todos_in_current_org", got)
	}
	if got := payload["cli_path"]; !reflect.DeepEqual(got, []string{"todo", "task", "list"}) {
		t.Fatalf("schemaPayload() cli_path = %#v, want [todo task list]", got)
	}
}

func TestSchemaPayloadResolvesCLICommandAlias(t *testing.T) {
	t.Parallel()

	payload, err := schemaPayload(ir.Catalog{
		Products: []ir.CanonicalProduct{
			{
				ID: "bot",
				CLI: &ir.ProductCLIMetadata{
					Command: "chat",
				},
				Tools: []ir.ToolDescriptor{
					{
						RPCName:       "create_robot",
						CanonicalPath: "bot.create_robot",
						InputSchema:   map[string]any{"type": "object"},
					},
				},
			},
		},
	}, []string{"chat.create_robot"})
	if err != nil {
		t.Fatalf("schemaPayload() error = %v", err)
	}

	if got := payload["path"]; got != "bot.create_robot" {
		t.Fatalf("schemaPayload() path = %#v, want canonical path bot.create_robot", got)
	}
}

func TestSchemaPayloadResolvesPublicRouteTokensToCanonicalToolPath(t *testing.T) {
	t.Parallel()

	payload, err := schemaPayload(ir.Catalog{
		Products: []ir.CanonicalProduct{
			{
				ID: "doc",
				CLI: &ir.ProductCLIMetadata{
					Command: "documents",
				},
				Tools: []ir.ToolDescriptor{
					{
						RPCName:       "create_document",
						CLIName:       "create",
						CanonicalPath: "doc.create_document",
						InputSchema:   map[string]any{"type": "object"},
					},
				},
			},
		},
	}, []string{"documents.create"})
	if err != nil {
		t.Fatalf("schemaPayload() error = %v", err)
	}

	if got := payload["path"]; got != "doc.create_document" {
		t.Fatalf("schemaPayload() path = %#v, want canonical path doc.create_document", got)
	}
	tool, ok := payload["tool"].(map[string]any)
	if !ok {
		t.Fatalf("schemaPayload() tool = %#v, want tool summary", payload["tool"])
	}
	if got := tool["rpc_name"]; got != "create_document" {
		t.Fatalf("schemaPayload() tool.rpc_name = %#v, want create_document", got)
	}
}

func TestSchemaPayloadFindsProductByIDOrCLICommand(t *testing.T) {
	t.Parallel()

	catalog := ir.Catalog{
		Products: []ir.CanonicalProduct{
			{
				ID:          "bot",
				DisplayName: "机器人消息",
				CLI: &ir.ProductCLIMetadata{
					Command: "chat",
				},
				Tools: []ir.ToolDescriptor{
					{RPCName: "create_robot", CanonicalPath: "bot.create_robot", InputSchema: map[string]any{"type": "object"}},
					{RPCName: "search_my_robots", CanonicalPath: "bot.search_my_robots", InputSchema: map[string]any{"type": "object"}},
				},
			},
		},
	}

	for _, query := range []string{"bot", "chat"} {
		payload, err := schemaPayload(catalog, []string{query})
		if err != nil {
			t.Fatalf("schemaPayload(%q) error = %v", query, err)
		}
		if got := payload["scope"]; got != "product" {
			t.Fatalf("schemaPayload(%q) scope = %#v, want product", query, got)
		}
		if got := payload["path"]; got != "bot" {
			t.Fatalf("schemaPayload(%q) path = %#v, want bot", query, got)
		}
		tools, ok := payload["tools"].([]map[string]any)
		if !ok {
			t.Fatalf("schemaPayload(%q) tools = %#v, want []map[string]any", query, payload["tools"])
		}
		if len(tools) != 2 {
			t.Fatalf("schemaPayload(%q) tools len = %d, want 2", query, len(tools))
		}
	}
}

func TestCanonicalCommandsPreserveOverlayRouting(t *testing.T) {
	t.Parallel()

	catalog := ir.BuildCatalog([]discovery.RuntimeServer{
		{
			Server: market.ServerDescriptor{
				Key:         "doc-key",
				DisplayName: "文档",
				Endpoint:    "https://example.com/server/doc",
				Source:      "live_runtime",
				HasCLIMeta:  true,
				CLI: market.CLIOverlay{
					ID:      "doc",
					Command: "documents",
					Groups: map[string]market.CLIGroupDef{
						"chat":       {Description: "消息"},
						"chat.group": {Description: "子组"},
					},
					ToolOverrides: map[string]market.CLIToolOverride{
						"create_document": {
							CLIName: "create",
							Group:   "chat.group",
						},
						"archive_document": {
							CLIName: "archive",
							Group:   "chat.group",
							Hidden:  boolPtr(true),
						},
					},
				},
			},
			Tools: []transport.ToolDescriptor{
				{
					Name:        "create_document",
					Title:       "创建文档",
					Description: "创建文档",
					InputSchema: map[string]any{
						"type": "object",
					},
				},
				{
					Name:        "archive_document",
					Title:       "归档文档",
					Description: "归档文档",
					InputSchema: map[string]any{
						"type": "object",
					},
				},
			},
			Source:   "live_runtime",
			Degraded: false,
		},
	})

	root := &cobra.Command{Use: "dws"}
	if err := AddCanonicalProducts(root, StaticLoader{Catalog: catalog}, executor.EchoRunner{}); err != nil {
		t.Fatalf("AddCanonicalProducts() error = %v", err)
	}

	product := commandByName(root, "documents")
	if product == nil {
		t.Fatalf("commandByName(documents) = nil, want product command routed by CLI.Command")
	}
	if got := product.Name(); got != "documents" {
		t.Fatalf("product command name = %q, want documents", got)
	}

	chat := commandByName(product, "chat")
	if chat == nil {
		t.Fatalf("commandByName(documents chat) = nil, want nested group")
	}
	group := commandByName(chat, "group")
	if group == nil {
		t.Fatalf("commandByName(documents chat group) = nil, want nested group leaf")
	}

	create := commandByName(group, "create")
	if create == nil {
		t.Fatalf("commandByName(documents chat group create) = nil, want tool command routed by CLIName")
	}
	if got := create.Name(); got != "create" {
		t.Fatalf("tool command name = %q, want create", got)
	}

	var out bytes.Buffer
	product.SetOut(&out)
	product.SetErr(&out)
	if err := product.Help(); err != nil {
		t.Fatalf("product help execute error = %v", err)
	}
	if strings.Contains(out.String(), "archive") {
		t.Fatalf("hidden tool rendered in help:\n%s", out.String())
	}

	out.Reset()
	group.SetOut(&out)
	group.SetErr(&out)
	if err := group.Help(); err != nil {
		t.Fatalf("group help execute error = %v", err)
	}
	if strings.Contains(out.String(), "archive") {
		t.Fatalf("hidden nested tool rendered in group help:\n%s", out.String())
	}
}

func TestCanonicalInvocationStaysStableAcrossPublicRouteAliases(t *testing.T) {
	t.Parallel()

	runner := &captureRunner{}
	cmd := NewMCPCommand(StaticLoader{
		Catalog: ir.Catalog{
			Products: []ir.CanonicalProduct{
				{
					ID: "doc",
					CLI: &ir.ProductCLIMetadata{
						Command: "documents",
					},
					Tools: []ir.ToolDescriptor{
						{
							RPCName:       "create_document",
							CLIName:       "create",
							CanonicalPath: "doc.create_document",
							InputSchema: map[string]any{
								"type": "object",
							},
						},
					},
				},
			},
		},
	}, runner)

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"documents", "create"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if got := runner.last.CanonicalProduct; got != "doc" {
		t.Fatalf("runner.last.CanonicalProduct = %q, want doc", got)
	}
	if got := runner.last.Tool; got != "create_document" {
		t.Fatalf("runner.last.Tool = %q, want create_document", got)
	}
	if got := runner.last.CanonicalPath; got != "doc.create_document" {
		t.Fatalf("runner.last.CanonicalPath = %q, want doc.create_document", got)
	}
}

func TestNewSchemaCommandDefaultsToJSON(t *testing.T) {
	t.Parallel()

	loader := StaticLoader{
		Catalog: ir.Catalog{
			Products: []ir.CanonicalProduct{
				{
					ID: "bot",
					CLI: &ir.ProductCLIMetadata{
						Command: "chat",
					},
					Tools: []ir.ToolDescriptor{
						{
							RPCName:       "create_robot",
							CLIName:       "create_robot",
							CanonicalPath: "bot.create_robot",
							Description:   "创建企业机器人",
							InputSchema: map[string]any{
								"type": "object",
								"properties": map[string]any{
									"robot_name": map[string]any{"type": "string"},
								},
								"required": []any{"robot_name"},
							},
							FlagHints: map[string]ir.CLIFlagHint{
								"robot_name": {Alias: "robot-name"},
							},
						},
					},
				},
			},
		},
	}

	run := func(args ...string) map[string]any {
		cmd := NewSchemaCommand(loader)
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute(%v) error = %v", args, err)
		}
		var payload map[string]any
		if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
			t.Fatalf("Execute(%v) output is not JSON: %v\n%s", args, err, out.String())
		}
		return payload
	}

	defaultPayload := run("bot.create_robot")
	jsonPayload := run("bot.create_robot", "--json")

	if got := defaultPayload["scope"]; got != "tool" {
		t.Fatalf("default payload scope = %#v, want tool", got)
	}
	if got := defaultPayload["path"]; got != "bot.create_robot" {
		t.Fatalf("default payload path = %#v, want bot.create_robot", got)
	}
	if !reflect.DeepEqual(defaultPayload, jsonPayload) {
		t.Fatalf("default payload != --json payload\ndefault=%#v\njson=%#v", defaultPayload, jsonPayload)
	}
}

func TestNewMCPCommandReturnsLoaderErrorForInvocations(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("fixture missing")
	cmd := NewMCPCommand(errorLoader{err: wantErr}, executor.EchoRunner{})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"doc", "create_document"})

	err := cmd.Execute()
	if !errors.Is(err, wantErr) {
		t.Fatalf("Execute() error = %v, want %v", err, wantErr)
	}
}

func TestNewMCPCommandSkipsProductsMarkedSkip(t *testing.T) {
	t.Parallel()

	cmd := NewMCPCommand(StaticLoader{
		Catalog: ir.Catalog{
			Products: []ir.CanonicalProduct{
				{
					ID: "doc",
					CLI: &ir.ProductCLIMetadata{
						Skip: true,
					},
				},
				{
					ID: "drive",
				},
			},
		},
	}, executor.EchoRunner{})

	if got := cmd.Commands(); len(got) != 1 || got[0].Name() != "drive" {
		t.Fatalf("mcp commands = %#v, want only drive", got)
	}
}

func TestProductCommandUsesCLICommandAlias(t *testing.T) {
	t.Parallel()

	runner := &captureRunner{}
	cmd := NewMCPCommand(StaticLoader{
		Catalog: ir.Catalog{
			Products: []ir.CanonicalProduct{
				{
					ID: "doc",
					CLI: &ir.ProductCLIMetadata{
						Command: "documents",
					},
					Tools: []ir.ToolDescriptor{
						{RPCName: "create_document"},
					},
				},
			},
		},
	}, runner)

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"documents", "create_document"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if runner.last.CanonicalProduct != "doc" {
		t.Fatalf("runner.last.CanonicalProduct = %q, want doc", runner.last.CanonicalProduct)
	}
}

func TestNewMCPCommandAddsGroupedRoutesFromCLIMetadata(t *testing.T) {
	t.Parallel()

	runner := &captureRunner{}
	cmd := NewMCPCommand(StaticLoader{
		Catalog: ir.Catalog{
			Products: []ir.CanonicalProduct{
				{
					ID: "doc",
					CLI: &ir.ProductCLIMetadata{
						Command: "documents",
						Group:   "office/collab",
					},
					Tools: []ir.ToolDescriptor{
						{RPCName: "create_document"},
					},
				},
			},
		},
	}, runner)

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"office", "collab", "documents", "create_document"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if runner.last.CanonicalProduct != "doc" {
		t.Fatalf("runner.last.CanonicalProduct = %q, want doc", runner.last.CanonicalProduct)
	}
}

func TestToolCommandUsesCLINameAndFlagHints(t *testing.T) {
	t.Parallel()

	runner := &captureRunner{}
	cmd := NewMCPCommand(StaticLoader{
		Catalog: ir.Catalog{
			Products: []ir.CanonicalProduct{
				{
					ID: "doc",
					Tools: []ir.ToolDescriptor{
						{
							RPCName: "create_document",
							CLIName: "create",
							InputSchema: map[string]any{
								"type": "object",
								"properties": map[string]any{
									"title": map[string]any{"type": "string"},
								},
							},
							FlagHints: map[string]ir.CLIFlagHint{
								"title": {
									Alias:     "name",
									Shorthand: "t",
								},
							},
						},
					},
				},
			},
		},
	}, runner)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"doc", "create", "--name", "hello"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if runner.last.Tool != "create_document" {
		t.Fatalf("runner.last.Tool = %q, want create_document", runner.last.Tool)
	}
	if runner.last.Params["title"] != "hello" {
		t.Fatalf("runner.last.Params[title] = %#v, want hello", runner.last.Params["title"])
	}
}

func TestToolCommandReconcilesFlattenedAliasFlags(t *testing.T) {
	t.Parallel()

	runner := &captureRunner{}
	cmd := NewMCPCommand(StaticLoader{
		Catalog: ir.Catalog{
			Products: []ir.CanonicalProduct{
				{
					ID: "bot",
					CLI: &ir.ProductCLIMetadata{
						Command: "chat",
					},
					Tools: []ir.ToolDescriptor{
						{
							RPCName:       "search_groups_by_keyword",
							CLIName:       "search",
							CanonicalPath: "bot.search_groups_by_keyword",
							InputSchema: map[string]any{
								"type": "object",
								"properties": map[string]any{
									"cursor":  map[string]any{"type": "string"},
									"keyword": map[string]any{"type": "string"},
								},
							},
							FlagHints: map[string]ir.CLIFlagHint{
								"cursor":  {Alias: "cursor"},
								"keyword": {Alias: "query"},
							},
						},
					},
				},
			},
		},
	}, runner)

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"chat", "search", "--query", "项目冲刺"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if got := runner.last.Params["keyword"]; got != "项目冲刺" {
		t.Fatalf("runner.last.Params[keyword] = %#v, want 项目冲刺", got)
	}
}

func TestToolCommandRetainsCanonicalFallbackAliases(t *testing.T) {
	t.Parallel()

	runner := &captureRunner{}
	cmd := NewMCPCommand(StaticLoader{
		Catalog: ir.Catalog{
			Products: []ir.CanonicalProduct{
				{
					ID: "doc",
					CLI: &ir.ProductCLIMetadata{
						Command: "documents",
					},
					Tools: []ir.ToolDescriptor{
						{
							RPCName:       "create_document",
							CLIName:       "create",
							CanonicalPath: "doc.create_document",
							InputSchema:   map[string]any{"type": "object"},
						},
					},
				},
			},
		},
	}, runner)

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"doc", "create_document"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if got := runner.last.CanonicalPath; got != "doc.create_document" {
		t.Fatalf("runner.last.CanonicalPath = %q, want doc.create_document", got)
	}
}

func TestSchemaPayloadDisambiguatesDuplicateToolRoutes(t *testing.T) {
	t.Parallel()

	catalog := ir.Catalog{
		Products: []ir.CanonicalProduct{
			{
				ID: "bot",
				CLI: &ir.ProductCLIMetadata{
					Command: "chat",
				},
				Tools: []ir.ToolDescriptor{
					{
						RPCName:       "batch_send_robot_msg_to_users",
						CLIName:       "send-by-bot",
						Group:         "message",
						CanonicalPath: "bot.batch_send_robot_msg_to_users",
						InputSchema:   map[string]any{"type": "object"},
					},
					{
						RPCName:       "send_robot_group_message",
						CLIName:       "send-by-bot",
						Group:         "message",
						CanonicalPath: "bot.send_robot_group_message",
						InputSchema:   map[string]any{"type": "object"},
					},
				},
			},
		},
	}

	payload, err := schemaPayload(catalog, []string{"bot.send_robot_group_message"})
	if err != nil {
		t.Fatalf("schemaPayload() error = %v", err)
	}
	if got := payload["cli_path"]; !reflect.DeepEqual(got, []string{"chat", "message", "send_robot_group_message"}) {
		t.Fatalf("schemaPayload() cli_path = %#v, want [chat message send_robot_group_message]", got)
	}
}

func TestNewMCPCommandDisambiguatesDuplicateToolRoutes(t *testing.T) {
	t.Parallel()

	runner := &captureRunner{}
	cmd := NewMCPCommand(StaticLoader{
		Catalog: ir.Catalog{
			Products: []ir.CanonicalProduct{
				{
					ID: "bot",
					CLI: &ir.ProductCLIMetadata{
						Command: "chat",
					},
					Tools: []ir.ToolDescriptor{
						{
							RPCName:       "batch_send_robot_msg_to_users",
							CLIName:       "send-by-bot",
							Group:         "message",
							CanonicalPath: "bot.batch_send_robot_msg_to_users",
							InputSchema: map[string]any{
								"type": "object",
								"properties": map[string]any{
									"userIds": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
								},
							},
						},
						{
							RPCName:       "send_robot_group_message",
							CLIName:       "send-by-bot",
							Group:         "message",
							CanonicalPath: "bot.send_robot_group_message",
							InputSchema: map[string]any{
								"type": "object",
								"properties": map[string]any{
									"openConversationId": map[string]any{"type": "string"},
								},
							},
						},
					},
				},
			},
		},
	}, runner)

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	chat := commandByName(cmd, "chat")
	if chat == nil {
		t.Fatal("commandByName(chat) = nil, want product command")
	}
	message := commandByName(chat, "message")
	if message == nil {
		t.Fatal("commandByName(chat message) = nil, want group command")
	}
	if got := commandByName(message, "send_robot_group_message"); got == nil {
		t.Fatal("commandByName(chat message send_robot_group_message) = nil, want disambiguated child command")
	}

	cmd.SetArgs([]string{"chat", "message", "send_robot_group_message", "--openConversationId", "cid-1"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := runner.last.Tool; got != "send_robot_group_message" {
		t.Fatalf("runner.last.Tool = %q, want send_robot_group_message", got)
	}
	if got := runner.last.Params["openConversationId"]; got != "cid-1" {
		t.Fatalf("runner.last.Params[openConversationId] = %#v, want cid-1", got)
	}
}

func TestNewMCPCommandPreservesOutputFormatFlagWhenToolHasFormatParam(t *testing.T) {
	t.Parallel()

	runner := &captureRunner{}
	cmd := NewMCPCommand(StaticLoader{
		Catalog: ir.Catalog{
			Products: []ir.CanonicalProduct{
				{
					ID: "aitable",
					Tools: []ir.ToolDescriptor{
						{
							RPCName:       "export_data",
							CLIName:       "export_data",
							Group:         "table",
							CanonicalPath: "aitable.export_data",
							InputSchema: map[string]any{
								"type":     "object",
								"required": []any{"baseId"},
								"properties": map[string]any{
									"baseId": map[string]any{"type": "string"},
									"format": map[string]any{"type": "string"},
								},
							},
						},
					},
				},
			},
		},
	}, runner)

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"aitable", "table", "export_data", "-f", "raw", "--baseId", "base-1", "--format", "excel"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}
	if got := runner.last.Params["format"]; got != "excel" {
		t.Fatalf("runner.last.Params[format] = %#v, want excel", got)
	}
	if got := runner.last.Params["baseId"]; got != "base-1" {
		t.Fatalf("runner.last.Params[baseId] = %#v, want base-1", got)
	}
	if text := strings.TrimSpace(out.String()); !strings.HasPrefix(text, "{\"invocation\":") {
		t.Fatalf("output = %q, want raw JSON payload", text)
	}
}

func TestNewMCPCommandGroupsUngroupedAITableTransferCommandsUnderTable(t *testing.T) {
	t.Parallel()

	catalog := ir.BuildCatalog([]discovery.RuntimeServer{
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
						"attachment": {Description: "附件管理"},
						"base":       {Description: "Base 管理"},
						"field":      {Description: "字段管理"},
						"record":     {Description: "记录管理"},
						"table":      {Description: "数据表管理"},
						"template":   {Description: "模板搜索"},
					},
					ToolOverrides: map[string]market.CLIToolOverride{
						"create_base": {CLIName: "create", Group: "base"},
					},
				},
			},
			Tools: []transport.ToolDescriptor{
				{Name: "create_base", Title: "创建 Base", Description: "创建 Base", InputSchema: map[string]any{"type": "object"}},
				{Name: "create_view", Title: "创建视图", Description: "创建视图", InputSchema: map[string]any{"type": "object"}},
				{Name: "get_views", Title: "获取视图", Description: "获取视图", InputSchema: map[string]any{"type": "object"}},
				{Name: "import_data", Title: "导入数据", Description: "导入数据", InputSchema: map[string]any{"type": "object"}},
				{Name: "prepare_import_upload", Title: "准备导入上传", Description: "准备导入上传", InputSchema: map[string]any{"type": "object"}},
				{Name: "export_data", Title: "导出数据", Description: "导出数据", InputSchema: map[string]any{"type": "object"}},
			},
		},
	})

	cmd := NewMCPCommand(StaticLoader{Catalog: catalog}, executor.EchoRunner{})
	aitable := commandByName(cmd, "aitable")
	if aitable == nil {
		t.Fatal("commandByName(aitable) = nil")
	}
	if got := commandByName(aitable, "create_view"); got != nil {
		t.Fatalf("commandByName(aitable create_view) = %q, want nil", got.CommandPath())
	}
	table := commandByName(aitable, "table")
	if table == nil {
		t.Fatal("commandByName(aitable table) = nil")
	}
	for _, name := range []string{"create_view", "get_views", "import_data", "prepare_import_upload", "export_data"} {
		if got := commandByName(table, name); got == nil {
			t.Fatalf("commandByName(aitable table %s) = nil", name)
		}
	}
}

func TestAddCanonicalProductsMergesProductsSharingCommandRoute(t *testing.T) {
	t.Parallel()

	runner := &captureRunner{}
	root := &cobra.Command{Use: "dws"}
	catalog := ir.Catalog{
		Products: []ir.CanonicalProduct{
			{
				ID:          "bot",
				DisplayName: "机器人消息",
				CLI: &ir.ProductCLIMetadata{
					Command: "chat",
					Groups: map[string]ir.CLIGroupDef{
						"bot":           {Description: "机器人管理"},
						"group":         {Description: "群组管理"},
						"group.members": {Description: "群成员管理"},
					},
				},
				Tools: []ir.ToolDescriptor{
					{
						RPCName:     "search_my_robots",
						CLIName:     "search",
						Group:       "bot",
						Title:       "搜索机器人",
						Description: "搜索机器人",
						InputSchema: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"robotName": map[string]any{"type": "string"},
							},
						},
						FlagHints: map[string]ir.CLIFlagHint{
							"robotName": {Alias: "name"},
						},
					},
					{
						RPCName:     "add_robot_to_group",
						CLIName:     "add-bot",
						Group:       "group.members",
						Title:       "添加机器人",
						Description: "添加机器人",
						InputSchema: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"openConversationId": map[string]any{"type": "string"},
								"robotCode":          map[string]any{"type": "string"},
							},
						},
						FlagHints: map[string]ir.CLIFlagHint{
							"openConversationId": {Alias: "id"},
							"robotCode":          {Alias: "robot-code"},
						},
					},
				},
			},
			{
				ID:          "group-chat",
				DisplayName: "钉钉群聊",
				CLI: &ir.ProductCLIMetadata{
					Command: "chat",
					Groups: map[string]ir.CLIGroupDef{
						"group":         {Description: "群组管理"},
						"group.members": {Description: "群成员管理"},
					},
				},
				Tools: []ir.ToolDescriptor{
					{
						RPCName:     "search_groups_by_keyword",
						CLIName:     "search",
						Title:       "搜索会话",
						Description: "搜索会话",
						InputSchema: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"OpenSearchRequest": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"query":  map[string]any{"type": "string"},
										"cursor": map[string]any{"type": "string"},
									},
								},
							},
						},
						FlagHints: map[string]ir.CLIFlagHint{
							"OpenSearchRequest.query":  {Alias: "query"},
							"OpenSearchRequest.cursor": {Alias: "cursor"},
						},
					},
					{
						RPCName:     "get_group_members",
						CLIName:     "members",
						Group:       "group",
						Title:       "群成员",
						Description: "群成员",
						InputSchema: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"openconversation_id": map[string]any{"type": "string"},
								"cursor":              map[string]any{"type": "string"},
							},
						},
						FlagHints: map[string]ir.CLIFlagHint{
							"openconversation_id": {Alias: "id"},
							"cursor":              {Alias: "cursor"},
						},
					},
					{
						RPCName:     "add_group_member",
						CLIName:     "add",
						Group:       "group.members",
						Title:       "添加成员",
						Description: "添加成员",
						InputSchema: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"openconversation_id": map[string]any{"type": "string"},
								"userId":              map[string]any{"type": "string"},
							},
						},
						FlagHints: map[string]ir.CLIFlagHint{
							"openconversation_id": {Alias: "id"},
							"userId":              {Alias: "users"},
						},
					},
					{
						RPCName:     "remove_group_member",
						CLIName:     "remove",
						Group:       "group.members",
						Title:       "移除成员",
						Description: "移除成员",
						InputSchema: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"openconversationId": map[string]any{"type": "string"},
								"userIdList":         map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
							},
						},
						FlagHints: map[string]ir.CLIFlagHint{
							"openconversationId": {Alias: "id"},
							"userIdList":         {Alias: "users", Transform: "csv_to_array"},
						},
					},
				},
			},
		},
	}

	if err := AddCanonicalProducts(root, StaticLoader{Catalog: catalog}, runner); err != nil {
		t.Fatalf("AddCanonicalProducts() error = %v", err)
	}

	var chatCommands []*cobra.Command
	for _, child := range root.Commands() {
		if child.Name() == "chat" {
			chatCommands = append(chatCommands, child)
		}
	}
	if len(chatCommands) != 1 {
		t.Fatalf("root chat command count = %d, want 1", len(chatCommands))
	}

	chat := chatCommands[0]
	if got := commandByName(chat, "bot"); got == nil {
		t.Fatal("commandByName(chat bot) = nil")
	}
	group := commandByName(chat, "group")
	if group == nil {
		t.Fatal("commandByName(chat group) = nil")
	}
	members := commandByName(group, "members")
	if members == nil {
		t.Fatal("commandByName(chat group members) = nil")
	}
	for _, want := range []string{"add", "add-bot", "remove"} {
		if got := commandByName(members, want); got == nil {
			t.Fatalf("commandByName(chat group members %s) = nil", want)
		}
	}

	root.SetArgs([]string{"chat", "bot", "search", "--name", "日报机器人"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(chat bot search) error = %v", err)
	}
	if got := runner.last.Tool; got != "search_my_robots" {
		t.Fatalf("runner.last.Tool = %q, want search_my_robots", got)
	}

	root.SetArgs([]string{"chat", "group", "members", "--id", "cid-1", "--cursor", "page-2"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(chat group members) error = %v", err)
	}
	if got := runner.last.Tool; got != "get_group_members" {
		t.Fatalf("runner.last.Tool = %q, want get_group_members", got)
	}
	if got := runner.last.Params["openconversation_id"]; got != "cid-1" {
		t.Fatalf("runner.last.Params[openconversation_id] = %#v, want cid-1", got)
	}
	if got := runner.last.Params["cursor"]; got != "page-2" {
		t.Fatalf("runner.last.Params[cursor] = %#v, want page-2", got)
	}
}

func TestBuildFlagSpecsDoesNotExpandHiddenObjectSubfields(t *testing.T) {
	t.Parallel()

	specs := BuildFlagSpecs(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"aiConfig": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"outputType": map[string]any{"type": "string"},
					"imageConfig": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"aiGeneratedWatermark": map[string]any{"type": "boolean"},
						},
					},
				},
			},
		},
	}, map[string]ir.CLIFlagHint{
		"aiConfig": {Hidden: true},
	})

	if len(specs) != 1 {
		t.Fatalf("BuildFlagSpecs() len = %d, want 1", len(specs))
	}
	if got := specs[0].PropertyName; got != "aiConfig" {
		t.Fatalf("specs[0].PropertyName = %q, want aiConfig", got)
	}
	if !specs[0].Hidden {
		t.Fatalf("specs[0].Hidden = false, want true")
	}
}

func TestCanonicalCommandExecutionNormalizesTransforms(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		property string
		schema   map[string]any
		hints    map[string]ir.CLIFlagHint
		args     []string
		want     any
	}{
		{
			name:     "csv_to_array",
			property: "tags",
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"tags": map[string]any{
						"type":  "array",
						"items": map[string]any{"type": "string"},
					},
				},
			},
			hints: map[string]ir.CLIFlagHint{
				"tags": {Transform: "csv_to_array"},
			},
			args: []string{"documents", "create", "--params", `{"tags":"alpha, beta,gamma"}`},
			want: []any{"alpha", "beta", "gamma"},
		},
		{
			name:     "json_parse",
			property: "config",
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"config": map[string]any{"type": "object"},
				},
			},
			hints: map[string]ir.CLIFlagHint{
				"config": {Transform: "json_parse"},
			},
			args: []string{"documents", "create", "--params", `{"config":"{\"enabled\":true,\"count\":2}"}`},
			want: map[string]any{"enabled": true, "count": float64(2)},
		},
		{
			name:     "iso8601_to_millis",
			property: "publish_at",
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"publish_at": map[string]any{"type": "integer"},
				},
			},
			hints: map[string]ir.CLIFlagHint{
				"publish_at": {Transform: "iso8601_to_millis"},
			},
			args: []string{"documents", "create", "--params", `{"publish_at":"2026-03-27T08:09:10Z"}`},
			want: time.Date(2026, time.March, 27, 8, 9, 10, 0, time.UTC).UnixMilli(),
		},
		{
			name:     "enum_map",
			property: "visibility",
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"visibility": map[string]any{"type": "integer"},
				},
			},
			hints: map[string]ir.CLIFlagHint{
				"visibility": {
					Transform: "enum_map",
					TransformArgs: map[string]any{
						"private":  2,
						"_default": 9,
					},
				},
			},
			args: []string{"documents", "create", "--params", `{"visibility":"private"}`},
			want: 2,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runner := &captureRunner{}
			cmd := NewMCPCommand(StaticLoader{
				Catalog: ir.Catalog{
					Products: []ir.CanonicalProduct{
						{
							ID: "doc",
							CLI: &ir.ProductCLIMetadata{
								Command: "documents",
							},
							Tools: []ir.ToolDescriptor{
								{
									RPCName:       "create_document",
									CLIName:       "create",
									CanonicalPath: "doc.create_document",
									InputSchema:   tt.schema,
									FlagHints:     tt.hints,
								},
							},
						},
					},
				},
			}, runner)

			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs(tt.args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}

			if got := runner.last.Params[tt.property]; !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("runner.last.Params[%s] = %#v, want %#v", tt.property, got, tt.want)
			}
		})
	}
}

func TestToolCommandValidatesInputSchemaBeforeRun(t *testing.T) {
	t.Parallel()

	runner := &captureRunner{}
	cmd := NewMCPCommand(StaticLoader{
		Catalog: ir.Catalog{
			Products: []ir.CanonicalProduct{
				{
					ID: "doc",
					Tools: []ir.ToolDescriptor{
						{
							RPCName: "create_document",
							InputSchema: map[string]any{
								"type": "object",
								"required": []any{
									"title",
								},
								"properties": map[string]any{
									"title": map[string]any{"type": "string"},
								},
							},
						},
					},
				},
			},
		},
	}, runner)

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"doc", "create_document", "--params", `{"title":"ok","unknown":"x"}`})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want schema validation error")
	}
	if !strings.Contains(err.Error(), "$.unknown is not allowed") {
		t.Fatalf("Execute() error = %v, want unknown-property validation", err)
	}
	if runner.called != 0 {
		t.Fatalf("runner called = %d, want 0", runner.called)
	}
}

func TestToolCommandSupportsDryRunWithoutSensitiveConfirmation(t *testing.T) {
	t.Parallel()

	runner := &captureRunner{}
	cmd := NewMCPCommand(StaticLoader{
		Catalog: ir.Catalog{
			Products: []ir.CanonicalProduct{
				{
					ID: "doc",
					Tools: []ir.ToolDescriptor{
						{
							RPCName:   "create_document",
							Sensitive: true,
							InputSchema: map[string]any{
								"type": "object",
							},
						},
					},
				},
			},
		},
	}, runner)

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.PersistentFlags().Bool("dry-run", false, "Preview the operation without executing it")
	cmd.SetArgs([]string{"doc", "create_document", "--dry-run"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !runner.last.DryRun {
		t.Fatalf("runner.last.DryRun = %t, want true", runner.last.DryRun)
	}
	if runner.called != 1 {
		t.Fatalf("runner called = %d, want 1", runner.called)
	}
}

func TestDeprecatedLifecycleAddsWarningToResult(t *testing.T) {
	t.Parallel()

	cmd := NewMCPCommand(StaticLoader{
		Catalog: ir.Catalog{
			Products: []ir.CanonicalProduct{
				{
					ID: "legacy-doc",
					Lifecycle: &ir.LifecycleInfo{
						DeprecatedBy:    9527,
						DeprecationDate: "2026-04-01T00:00:00Z",
						MigrationURL:    "https://example.com/migration",
					},
					Tools: []ir.ToolDescriptor{
						{
							RPCName: "search_documents",
						},
					},
				},
			},
		},
	}, executor.EchoRunner{})

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"-f", "json", "legacy-doc", "search_documents"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var payload struct {
		Response map[string]any `json:"response"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v\noutput:\n%s", err, out.String())
	}
	if payload.Response["warning"] == "" {
		t.Fatalf("warning is empty, payload=%#v", payload.Response)
	}
	warning, _ := payload.Response["warning"].(string)
	if !strings.Contains(warning, "deprecated_by_mcpId=9527") {
		t.Fatalf("warning = %q, want deprecated_by_mcpId=9527", warning)
	}
}

func TestDeprecatedLifecyclePrintsWarningToStderr(t *testing.T) {
	t.Parallel()

	cmd := NewMCPCommand(StaticLoader{
		Catalog: ir.Catalog{
			Products: []ir.CanonicalProduct{
				{
					ID: "legacy-doc",
					Lifecycle: &ir.LifecycleInfo{
						DeprecatedBy: 9527,
						MigrationURL: "https://example.com/migration",
					},
					Tools: []ir.ToolDescriptor{
						{RPCName: "search_documents"},
					},
				},
			},
		},
	}, executor.EchoRunner{})

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"legacy-doc", "search_documents"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	stderr := errOut.String()
	if !strings.Contains(stderr, "warning: product legacy-doc is deprecated") {
		t.Fatalf("stderr = %q, want deprecation warning", stderr)
	}
	if !strings.Contains(stderr, "migration=https://example.com/migration") {
		t.Fatalf("stderr = %q, want migration hint", stderr)
	}
}

func TestSensitiveToolConfirmationWorksWithoutYesFlag(t *testing.T) {
	t.Parallel()

	cmd := NewMCPCommand(StaticLoader{
		Catalog: ir.Catalog{
			Products: []ir.CanonicalProduct{
				{
					ID: "doc",
					Tools: []ir.ToolDescriptor{
						{
							RPCName:   "create_document",
							CLIName:   "create-document",
							Sensitive: true,
						},
					},
				},
			},
		},
	}, executor.EchoRunner{})

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader("yes\n"))
	cmd.SetArgs([]string{"doc", "create-document"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestLegacyCandidateLifecycleAddsWarningToResult(t *testing.T) {
	t.Parallel()

	cmd := NewMCPCommand(StaticLoader{
		Catalog: ir.Catalog{
			Products: []ir.CanonicalProduct{
				{
					ID: "legacy-candidate",
					Lifecycle: &ir.LifecycleInfo{
						DeprecatedCandidate: true,
					},
					Tools: []ir.ToolDescriptor{
						{RPCName: "search_documents"},
					},
				},
			},
		},
	}, executor.EchoRunner{})

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"-f", "json", "legacy-candidate", "search_documents"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var payload struct {
		Response map[string]any `json:"response"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v\noutput:\n%s", err, out.String())
	}
	warning, _ := payload.Response["warning"].(string)
	if !strings.Contains(warning, "legacy candidate") {
		t.Fatalf("warning = %q, want legacy candidate marker", warning)
	}
}

type errorLoader struct {
	err error
}

func (l errorLoader) Load(context.Context) (ir.Catalog, error) {
	return ir.Catalog{}, l.err
}

type captureRunner struct {
	last   executor.Invocation
	called int
}

func (r *captureRunner) Run(_ context.Context, invocation executor.Invocation) (executor.Result, error) {
	r.last = invocation
	r.called++
	return executor.Result{Invocation: invocation}, nil
}

func boolPtr(v bool) *bool {
	return &v
}
