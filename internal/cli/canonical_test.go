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
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/ir"
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
		},
	}, map[string]ir.CLIFlagHint{
		"title": {
			Shorthand: "t",
			Alias:     "name",
		},
	})

	if len(specs) != 4 {
		t.Fatalf("BuildFlagSpecs() len = %d, want 4", len(specs))
	}
	if specs[0].PropertyName != "metadata" || specs[1].PropertyName != "notify" || specs[2].PropertyName != "tags" || specs[3].PropertyName != "title" {
		t.Fatalf("BuildFlagSpecs() unexpected order = %#v", specs)
	}
	if specs[0].Kind != "json" {
		t.Fatalf("BuildFlagSpecs() metadata kind = %q, want json", specs[0].Kind)
	}
	if specs[3].Alias != "name" || specs[3].Shorthand != "t" {
		t.Fatalf("BuildFlagSpecs() title hints = %#v, want alias=name shorthand=t", specs[3])
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

func TestRuntimeSchemaPayloadFindsTool(t *testing.T) {
	t.Parallel()

	payload, err := runtimeSchemaPayload(buildRuntimeSchemaTestRoot(), []string{"doc.create_document"})
	if err != nil {
		t.Fatalf("runtimeSchemaPayload() error = %v", err)
	}
	if payload["path"] != "doc.create_document" {
		t.Fatalf("runtimeSchemaPayload() path = %#v, want doc.create_document", payload["path"])
	}
	if payload["product_id"] != "doc" {
		t.Fatalf("runtimeSchemaPayload() product_id = %#v, want doc", payload["product_id"])
	}
}

func TestRuntimeSchemaPayloadMarksNoParameterCommands(t *testing.T) {
	t.Parallel()

	root := buildRuntimeSchemaTestRoot()
	noop := &cobra.Command{Use: "noop", Short: "No params", Run: func(*cobra.Command, []string) {}}
	AttachRuntimeSchema(noop, "doc", "noop", "runtime:doc")
	doc, _, err := root.Find([]string{"doc"})
	if err != nil || doc == nil {
		t.Fatalf("find doc command: %v", err)
	}
	doc.AddCommand(noop)

	payload, err := runtimeSchemaPayload(root, []string{"doc.noop"})
	if err != nil {
		t.Fatalf("runtimeSchemaPayload() error = %v", err)
	}
	params, ok := payload["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("parameters type = %T", payload["parameters"])
	}
	if len(params) != 0 || payload["has_parameters"] != false || payload["parameter_count"] != 0 {
		t.Fatalf("no-param marker mismatch: parameters=%#v has=%#v count=%#v", params, payload["has_parameters"], payload["parameter_count"])
	}
}

func TestCompactToolEmitsExtendedFields(t *testing.T) {
	t.Parallel()

	destructive := true
	tool := ir.ToolDescriptor{
		RPCName:       "send_ding_message",
		CLIName:       "send",
		Group:         "message",
		CanonicalPath: "sample.send_ding_message",
		Title:         "发送DING消息",
		Description:   "desc",
		Sensitive:     true,
		InputSchema: map[string]any{
			"type":     "object",
			"required": []any{"robotCode"},
			"properties": map[string]any{
				"receiverUserIdList": map[string]any{"type": "array", "description": "接收人"},
				"robotCode":          map[string]any{"type": "string", "title": "机器人编码", "default": "robot-default"},
			},
		},
		OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"openDingId": map[string]any{"type": "string"},
			},
		},
		Annotations: &ir.ToolAnnotations{DestructiveHint: &destructive},
		Auth: &ir.ToolAuthMetadata{
			ProductCode:         "calendar",
			RequiredPermissions: []string{"Calendar.Event.Write"},
			GrantProductCodes:   []string{"calendar"},
			AuthMetaHash:        "sha256:test",
		},
		FlagOverlay: map[string]ir.FlagOverlay{
			"receiverUserIdList": {Alias: "users", Transform: "csv_to_array", Description: "CLI 接收人列表"},
		},
	}

	out := compactTool(tool)
	if out["name"] != "send_ding_message" {
		t.Errorf("name = %v", out["name"])
	}
	if out["cli_name"] != "send" {
		t.Errorf("cli_name = %v", out["cli_name"])
	}
	if out["canonical_path"] != "sample.send_ding_message" {
		t.Errorf("canonical_path = %v", out["canonical_path"])
	}
	if out["group"] != "message" {
		t.Errorf("group = %v", out["group"])
	}
	params, ok := out["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("parameters type = %T", out["parameters"])
	}
	robot, _ := params["robot-code"].(map[string]any)
	if robot == nil || robot["type"] != "string" || robot["required"] != true || robot["default"] != "robot-default" {
		t.Fatalf("robot-code = %#v, want flat string required default", robot)
	}
	if robot["description"] != "机器人编码" {
		t.Fatalf("robot-code description = %#v, want schema title fallback", robot["description"])
	}
	users, _ := params["users"].(map[string]any)
	if users == nil || users["type"] != "array" || users["required"] != false {
		t.Fatalf("users = %#v, want alias-backed array", users)
	}
	if users["description"] != "CLI 接收人列表" {
		t.Fatalf("users description = %#v, want overlay description", users["description"])
	}
	if _, ok := out["output_schema"]; !ok {
		t.Errorf("output_schema missing, keys = %v", keysOf(out))
	}
	if _, ok := out["annotations"]; !ok {
		t.Errorf("annotations missing, keys = %v", keysOf(out))
	}
	auth, ok := out["auth"].(*ir.ToolAuthMetadata)
	if !ok {
		t.Fatalf("auth type = %T", out["auth"])
	}
	if auth.RequiredPermissions[0] != "Calendar.Event.Write" {
		t.Errorf("auth required permissions = %#v", auth.RequiredPermissions)
	}
	overlay, ok := out["flag_overlay"].(map[string]ir.FlagOverlay)
	if !ok {
		t.Fatalf("flag_overlay type = %T", out["flag_overlay"])
	}
	if overlay["receiverUserIdList"].Alias != "users" {
		t.Errorf("overlay alias = %q", overlay["receiverUserIdList"].Alias)
	}
}

func TestCompactToolAppliesHardcodedSchemaHints(t *testing.T) {
	t.Parallel()

	tool := ir.ToolDescriptor{
		RPCName:       "send_ding_message",
		CLIName:       "send",
		CanonicalPath: "ding.send_ding_message",
		Description:   "remote description",
		InputSchema: map[string]any{
			"type":     "object",
			"required": []any{},
			"properties": map[string]any{
				"robotCode":          map[string]any{"type": "string", "description": "remote robot"},
				"receiverUserIdList": map[string]any{"type": "array", "description": "remote users"},
				"remindType":         map[string]any{"type": "integer", "description": "remote type"},
			},
		},
		FlagOverlay: map[string]ir.FlagOverlay{
			"receiverUserIdList": {Alias: "users", Transform: "csv_to_array"},
			"remindType":         {Alias: "type"},
		},
	}

	out := compactTool(tool)
	if out["description"] == "remote description" {
		t.Fatalf("description was not overridden by hardcoded hint")
	}
	params, ok := out["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("parameters type = %T", out["parameters"])
	}
	robot, _ := params["robot-code"].(map[string]any)
	if robot == nil || robot["description"] == "remote robot" || robot["required"] != true {
		t.Fatalf("robot-code hardcoded hint not applied: %#v", robot)
	}
	remind, _ := params["type"].(map[string]any)
	if remind == nil || remind["default"] != "app" {
		t.Fatalf("type hardcoded default not applied: %#v", remind)
	}
}

func TestCompactToolOmitsEmptyExtras(t *testing.T) {
	t.Parallel()

	tool := ir.ToolDescriptor{
		RPCName:       "list_documents",
		CLIName:       "list",
		CanonicalPath: "doc.list_documents",
		InputSchema:   map[string]any{"type": "object"},
	}
	out := compactTool(tool)
	for _, key := range []string{"output_schema", "annotations", "flag_overlay", "group"} {
		if _, has := out[key]; has {
			t.Errorf("key %q should be omitted when empty, got %#v", key, out[key])
		}
	}
	params, ok := out["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("parameters type = %T", out["parameters"])
	}
	if len(params) != 0 || out["has_parameters"] != false || out["parameter_count"] != 0 {
		t.Fatalf("no-param marker mismatch: parameters=%#v has=%#v count=%#v", params, out["has_parameters"], out["parameter_count"])
	}
}

func TestRuntimeSchemaPayloadResolvesCLIPath(t *testing.T) {
	t.Parallel()

	root := buildRuntimeSchemaTestRoot()

	cases := []struct {
		name    string
		input   string
		wantRPC string
		wantErr bool
	}{
		{"canonical rpc path", "ding.send_ding_message", "send_ding_message", false},
		{"dotted cli path", "ding.message.send", "send_ding_message", false},
		{"space cli path", "ding message send", "send_ding_message", false},
		{"slash cli path", "ding/message/recall", "recall_ding_message", false},
		{"unknown leaf", "ding message nope", "", true},
		{"unknown group", "ding random send", "", true},
		{"unknown product", "nope send", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload, err := runtimeSchemaPayload(root, []string{tc.input})
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if payload["name"] != tc.wantRPC {
				t.Errorf("tool name = %v, want %s", payload["name"], tc.wantRPC)
			}
		})
	}
}

func TestRuntimeSchemaPayloadBrowsesProductAndGroup(t *testing.T) {
	t.Parallel()

	root := buildRuntimeSchemaTestRoot()
	productPayload, err := runtimeSchemaPayload(root, []string{"ding"})
	if err != nil {
		t.Fatalf("product payload: %v", err)
	}
	if productPayload["level"] != "product" || productPayload["count"] != 2 {
		t.Fatalf("product payload = %#v", productPayload)
	}
	product, _ := productPayload["product"].(map[string]any)
	tools, _ := product["tools"].([]map[string]any)
	if product["id"] != "ding" || len(tools) != 2 {
		t.Fatalf("product = %#v, want ding with two tools", product)
	}

	groupPayload, err := runtimeSchemaPayload(root, []string{"ding.message"})
	if err != nil {
		t.Fatalf("group payload: %v", err)
	}
	if groupPayload["level"] != "group" || groupPayload["path"] != "ding message" || groupPayload["count"] != 2 {
		t.Fatalf("group payload = %#v", groupPayload)
	}
}

func TestSchemaCommandProgressiveDisclosure(t *testing.T) {
	t.Parallel()

	t.Run("default is compact product overview", func(t *testing.T) {
		cmd := buildSchemaCommandTestRoot()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"schema"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if payload["level"] != "products" || payload["tool_count"] != float64(3) {
			t.Fatalf("overview payload = %#v", payload)
		}
		products, _ := payload["products"].([]any)
		if len(products) != 2 {
			t.Fatalf("products len = %d, want 2", len(products))
		}
		for _, raw := range products {
			product, _ := raw.(map[string]any)
			if _, hasTools := product["tools"]; hasTools {
				t.Fatalf("compact product unexpectedly embeds tools: %#v", product)
			}
			if product["tool_count"] == nil || product["schema_path"] == nil {
				t.Fatalf("compact product missing drill-down metadata: %#v", product)
			}
		}
	})

	t.Run("list remains a compact root overview compatibility alias", func(t *testing.T) {
		cmd := buildSchemaCommandTestRoot()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"schema", "list"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if payload["level"] != "products" || payload["tool_count"] != float64(3) {
			t.Fatalf("list compatibility payload = %#v", payload)
		}
	})

	t.Run("all preserves complete catalog", func(t *testing.T) {
		cmd := buildSchemaCommandTestRoot()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"schema", "--all"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if payload["level"] != "catalog" || payload["tool_count"] != float64(3) {
			t.Fatalf("catalog payload = %#v", payload)
		}
		products, _ := payload["products"].([]any)
		first, _ := products[0].(map[string]any)
		if tools, _ := first["tools"].([]any); len(tools) == 0 {
			t.Fatalf("full catalog product has no tools: %#v", first)
		}
	})

	t.Run("all rejects path", func(t *testing.T) {
		cmd := buildSchemaCommandTestRoot()
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"schema", "--all", "ding"})
		if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "cannot be combined") {
			t.Fatalf("Execute() error = %v, want --all/path validation", err)
		}
	})
}

func TestRuntimeSchemaPayloadMarksCanonicalAliases(t *testing.T) {
	t.Parallel()

	root := &cobra.Command{Use: "dws"}
	list := &cobra.Command{Use: "list", Short: "List records", Run: func(*cobra.Command, []string) {}}
	AttachRuntimeSchema(list, "aitable", "query_records", "hardcoded:aitable")
	query := &cobra.Command{Use: "query", Short: "Query records", Run: func(*cobra.Command, []string) {}}
	AttachRuntimeSchema(query, "aitable", "query_records", "hardcoded:aitable")
	record := &cobra.Command{Use: "record", Short: "Records"}
	record.AddCommand(list, query)
	aitable := &cobra.Command{Use: "aitable", Short: "AI 表格"}
	aitable.AddCommand(record)
	root.AddCommand(aitable)

	entries := collectRuntimeSchemaEntries(root)
	listing := runtimeSchemaListPayload(entries)
	products, _ := listing["products"].([]map[string]any)
	if len(products) != 1 {
		t.Fatalf("products len = %d, want 1", len(products))
	}
	tools, _ := products[0]["tools"].([]map[string]any)
	if len(tools) != 1 {
		t.Fatalf("tools len = %d, want one primary entry; tools=%#v", len(tools), tools)
	}
	if tools[0]["cli_path"] != "aitable record query" || tools[0]["primary_cli_path"] != "aitable record query" {
		t.Fatalf("primary tool = %#v, want query primary", tools[0])
	}
	aliases, _ := tools[0]["aliases"].([]string)
	if len(aliases) != 1 || aliases[0] != "aitable record list" {
		t.Fatalf("aliases = %#v, want record list", tools[0]["aliases"])
	}

	canonical, err := runtimeSchemaPayload(root, []string{"aitable.query_records"})
	if err != nil {
		t.Fatalf("canonical payload: %v", err)
	}
	if canonical["cli_path"] != "aitable record query" || canonical["is_alias"] != false {
		t.Fatalf("canonical payload = %#v, want primary query", canonical)
	}

	alias, err := runtimeSchemaPayload(root, []string{"aitable record list"})
	if err != nil {
		t.Fatalf("alias payload: %v", err)
	}
	if alias["cli_path"] != "aitable record list" || alias["primary_cli_path"] != "aitable record query" || alias["is_alias"] != true {
		t.Fatalf("alias payload = %#v, want list alias to query", alias)
	}
}

func TestSchemaCommandCLIPathFlag(t *testing.T) {
	t.Parallel()

	t.Run("resolves via --cli-path", func(t *testing.T) {
		cmd := buildSchemaCommandTestRoot()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"schema", "--cli-path", "ding message send"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if payload["name"] != "send_ding_message" {
			t.Errorf("tool name = %v", payload["name"])
		}
	})

	t.Run("rejects positional + flag collision", func(t *testing.T) {
		cmd := buildSchemaCommandTestRoot()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"schema", "--cli-path", "ding message send", "ding.send_ding_message"})
		err := cmd.Execute()
		if err == nil {
			t.Fatalf("expected mutual-exclusion error, got nil")
		}
		if !strings.Contains(err.Error(), "mutually exclusive") {
			t.Errorf("err = %v, want mutual-exclusion message", err)
		}
	})
}

func buildRuntimeSchemaTestRoot() *cobra.Command {
	root := &cobra.Command{Use: "dws"}

	create := &cobra.Command{Use: "create", Short: "Create document", Run: func(*cobra.Command, []string) {}}
	create.Flags().String("title", "", "Document title")
	AttachRuntimeSchema(create, "doc", "create_document", "runtime:doc")
	AnnotateRuntimeFlag(create, "title", "title", "string", true, "")
	doc := &cobra.Command{Use: "doc", Short: "Docs"}
	doc.AddCommand(create)

	send := &cobra.Command{Use: "send", Short: "Send ding", Run: func(*cobra.Command, []string) {}}
	send.Flags().String("robot-code", "", "Robot code")
	AttachRuntimeSchema(send, "ding", "send_ding_message", "runtime:ding")
	AnnotateRuntimeFlag(send, "robot-code", "robotCode", "string", true, "")
	recall := &cobra.Command{Use: "recall", Short: "Recall ding", Run: func(*cobra.Command, []string) {}}
	recall.Flags().String("id", "", "DING message ID")
	AttachRuntimeSchema(recall, "ding", "recall_ding_message", "runtime:ding")
	AnnotateRuntimeFlag(recall, "id", "openDingId", "string", true, "")
	message := &cobra.Command{Use: "message", Short: "DING messages"}
	message.AddCommand(send, recall)
	ding := &cobra.Command{Use: "ding", Short: "DING"}
	ding.AddCommand(message)

	root.AddCommand(doc, ding)
	return root
}

func buildSchemaCommandTestRoot() *cobra.Command {
	root := buildRuntimeSchemaTestRoot()
	root.AddCommand(NewSchemaCommandWithRootProvider(func() *cobra.Command { return root }))
	return root
}

func TestRuntimeSchemaUsesMCPMetadataAnnotations(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	search := &cobra.Command{Use: "search", Short: "fallback title", Long: "fallback description", Run: func(*cobra.Command, []string) {}}
	search.Flags().String("start", "", "fallback start")
	search.Flags().String("status", "", "fallback status")
	_ = search.Flags().SetAnnotation("start", "x-cli-format", []string{"date-time"})
	_ = search.Flags().SetAnnotation("status", "x-cli-enum", []string{"confirmed", "cancelled"})
	AttachRuntimeSchema(search, "calendar", "search_events", "runtime:calendar")
	AnnotateRuntimeToolMetadata(search, "MCP title", "MCP description", "mcp-detail")
	AnnotateRuntimeFlag(search, "start", "startTime", "string", true, "")
	AnnotateRuntimeFlag(search, "status", "status", "string", false, "")
	calendar := &cobra.Command{Use: "calendar", Short: "Calendar"}
	calendar.AddCommand(search)
	root.AddCommand(calendar)

	payload, err := runtimeSchemaPayload(root, []string{"calendar search"})
	if err != nil {
		t.Fatalf("runtimeSchemaPayload() error = %v", err)
	}
	if payload["title"] != "MCP title" {
		t.Fatalf("title = %v, want MCP title", payload["title"])
	}
	if payload["description"] != "MCP description" {
		t.Fatalf("description = %v, want MCP description", payload["description"])
	}
	if payload["metadata_source"] != "mcp-detail" {
		t.Fatalf("metadata_source = %v, want mcp-detail", payload["metadata_source"])
	}
	params, _ := payload["parameters"].(map[string]any)
	start, _ := params["start"].(map[string]any)
	if start["format"] != "date-time" {
		t.Fatalf("start format = %v, want date-time", start["format"])
	}
	status, _ := params["status"].(map[string]any)
	enum, _ := status["enum"].([]string)
	if !equalStrings(enum, []string{"confirmed", "cancelled"}) {
		t.Fatalf("status enum = %#v", status["enum"])
	}
}

func TestRuntimeSchemaEmitsCrossParameterConstraintsAndPositionals(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	send := &cobra.Command{Use: "send", RunE: func(*cobra.Command, []string) error { return nil }}
	send.Flags().String("group", "", "群聊必填")
	send.Flags().String("users", "", "单聊必填")
	AttachRuntimeSchema(send, "chat", "send_robot_message", "hardcoded:chat")
	AnnotateRuntimeConstraints(send, RuntimeSchemaConstraints{
		MutuallyExclusive: [][]string{{"group", "users"}},
		RequireOneOf:      [][]string{{"group", "users"}},
	})
	AnnotateRuntimePositionals(send, RuntimeSchemaPositional{
		Name: "scope", Type: "array", Variadic: true, Index: 0,
	})
	chat := &cobra.Command{Use: "chat"}
	chat.AddCommand(send)
	root.AddCommand(chat)

	payload, err := runtimeSchemaPayload(root, []string{"chat.send_robot_message"})
	if err != nil {
		t.Fatalf("runtimeSchemaPayload() error = %v", err)
	}
	params, _ := payload["parameters"].(map[string]any)
	for _, name := range []string{"group", "users"} {
		param, _ := params[name].(map[string]any)
		if param["required"] != false {
			t.Fatalf("%s required = %#v, want conditional false", name, param["required"])
		}
	}
	constraints, _ := payload["constraints"].(map[string]any)
	requireOneOf, _ := constraints["require_one_of"].([][]string)
	if len(requireOneOf) != 1 || !equalStrings(requireOneOf[0], []string{"group", "users"}) {
		t.Fatalf("require_one_of = %#v", constraints["require_one_of"])
	}
	positionals, _ := payload["positionals"].([]RuntimeSchemaPositional)
	if len(positionals) != 1 || positionals[0].Name != "scope" || !positionals[0].Variadic {
		t.Fatalf("positionals = %#v", payload["positionals"])
	}
}

func TestRuntimeSchemaAnnotationsDoNotChangeCobraValidation(t *testing.T) {
	cmd := &cobra.Command{Use: "read", RunE: func(*cobra.Command, []string) error { return nil }}
	cmd.Flags().String("node", "", "node ID")
	cmd.Flags().String("url", "", "node URL alias")
	AnnotateRuntimeRequiredFlags(cmd, "node")
	cmd.SetArgs([]string{"--url", "https://example.com/doc"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("schema-only required annotation rejected alias invocation: %v", err)
	}
}

func TestRuntimeSchemaExcludesHumanOnlyHints(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	product := &cobra.Command{Use: "sheet"}
	real := &cobra.Command{Use: "range-read", RunE: func(*cobra.Command, []string) error { return nil }}
	hint := &cobra.Command{Use: "read", RunE: func(*cobra.Command, []string) error { return nil }}
	AttachRuntimeSchema(real, "sheet", "range_read", "runtime:sheet")
	AttachRuntimeSchema(hint, "sheet", "read", "runtime:sheet")
	ExcludeFromRuntimeSchema(hint)
	product.AddCommand(real, hint)
	root.AddCommand(product)

	entries := collectRuntimeSchemaEntries(root)
	if len(entries) != 1 || entries[0].CLIPath != "sheet range-read" {
		t.Fatalf("entries = %#v, want only executable leaf", entries)
	}
}

func TestRuntimeSchemaHidesInternalSourceProductsAndKeepsCanonicalHelpers(t *testing.T) {
	root := &cobra.Command{Use: "dws"}

	internalLeaf := &cobra.Command{Use: "list-comments", RunE: func(*cobra.Command, []string) error { return nil }}
	AttachRuntimeSchema(internalLeaf, "doc-comment", "list_comments", "runtime:doc-comment")
	internalRoot := &cobra.Command{Use: "doc-comment"}
	internalRoot.AddCommand(internalLeaf)

	publicLeaf := &cobra.Command{Use: "list", RunE: func(*cobra.Command, []string) error { return nil }}
	AttachRuntimeSchema(publicLeaf, "doc-comment", "list_comments", "hardcoded:doc")
	comment := &cobra.Command{Use: "comment"}
	comment.AddCommand(publicLeaf)
	publicRoot := &cobra.Command{Use: "doc"}
	publicRoot.AddCommand(comment)
	root.AddCommand(internalRoot, publicRoot)

	entries := collectRuntimeSchemaEntries(root)
	if len(entries) != 1 {
		t.Fatalf("entries = %#v, want only the public canonical helper", entries)
	}
	entry := entries[0]
	if entry.ProductID != "doc" || entry.SourceProductID != "doc-comment" || entry.CLIPath != "doc comment list" {
		t.Fatalf("entry = %#v", entry)
	}
}

func TestRuntimeSchemaUsesCuratedRequiredSemantics(t *testing.T) {
	root := &cobra.Command{Use: "dws"}

	create := &cobra.Command{Use: "create", Run: func(*cobra.Command, []string) {}}
	for _, flag := range []struct {
		name     string
		property string
		required bool
	}{
		{name: "title", property: "summary"},
		{name: "start", property: "startDateTime"},
		{name: "end", property: "endDateTime"},
		{name: "recurrence-type", property: "recurrence.pattern.type", required: true},
		{name: "recurrence-interval", property: "recurrence.pattern.interval", required: true},
		{name: "recurrence-count", property: "recurrence.range.numberOfOccurrences", required: true},
	} {
		create.Flags().String(flag.name, "", flag.property)
		AnnotateRuntimeFlag(create, flag.name, flag.property, "string", flag.required, "")
	}
	AttachRuntimeSchema(create, "calendar", "create_calendar_event", "runtime:calendar")
	event := &cobra.Command{Use: "event"}
	event.AddCommand(create)
	calendar := &cobra.Command{Use: "calendar"}
	calendar.AddCommand(event)

	summary := &cobra.Command{Use: "summary", Run: func(*cobra.Command, []string) {}}
	summary.Flags().String("id", "", "task UUID")
	AnnotateRuntimeFlag(summary, "id", "taskUuid", "string", false, "")
	AttachRuntimeSchema(summary, "minutes", "get_minutes_ai_summary", "runtime:minutes")
	get := &cobra.Command{Use: "get"}
	get.AddCommand(summary)
	minutes := &cobra.Command{Use: "minutes"}
	minutes.AddCommand(get)
	root.AddCommand(calendar, minutes)

	calendarPayload, err := runtimeSchemaPayload(root, []string{"calendar event create"})
	if err != nil {
		t.Fatalf("calendar payload: %v", err)
	}
	calendarParams, _ := calendarPayload["parameters"].(map[string]any)
	for _, name := range []string{"title", "start", "end"} {
		param, _ := calendarParams[name].(map[string]any)
		if param["required"] != true {
			t.Fatalf("%s required = %#v, want true", name, param["required"])
		}
	}
	for _, check := range []struct {
		name string
		when string
	}{
		{name: "recurrence-type", when: "any recurrence-* flag is provided"},
		{name: "recurrence-interval", when: "any recurrence-* flag is provided"},
		{name: "recurrence-count", when: "recurrence-range-type is numbered"},
	} {
		param, _ := calendarParams[check.name].(map[string]any)
		if param["required"] != false || param["required_when"] != check.when {
			t.Fatalf("%s = %#v, want conditional requirement %q", check.name, param, check.when)
		}
	}

	minutesPayload, err := runtimeSchemaPayload(root, []string{"minutes get summary"})
	if err != nil {
		t.Fatalf("minutes payload: %v", err)
	}
	minutesParams, _ := minutesPayload["parameters"].(map[string]any)
	id, _ := minutesParams["id"].(map[string]any)
	if id["required"] != true {
		t.Fatalf("minutes --id required = %#v, want true", id["required"])
	}
}

func TestRuntimeSchemaUsesEmbeddedMCPMetadataFallback(t *testing.T) {
	prev := runtimeEmbeddedMCPMetadata
	required := true
	runtimeEmbeddedMCPMetadata = embeddedMCPMetadata{
		Tools: map[string]embeddedMCPToolMetadata{
			"calendar.search_events": {
				Title:       "Embedded title",
				Description: "Embedded description",
				InterfaceRef: &embeddedMCPInterfaceRef{
					ProductID: "calendar",
					RPCName:   "search_calendar_events",
				},
				Parameters: map[string]embeddedMCPParamMeta{
					"startTime": {
						Description: "Embedded start time",
						Type:        "number",
						Format:      "date-time",
						Required:    &required,
					},
				},
			},
		},
	}
	t.Cleanup(func() { runtimeEmbeddedMCPMetadata = prev })

	root := &cobra.Command{Use: "dws"}
	search := &cobra.Command{Use: "search", Short: "fallback title", Long: "fallback description", Run: func(*cobra.Command, []string) {}}
	search.Flags().String("start", "", "fallback start")
	AttachRuntimeSchema(search, "calendar", "search_events", "runtime:calendar")
	AnnotateRuntimeFlag(search, "start", "startTime", "string", false, "")
	calendar := &cobra.Command{Use: "calendar", Short: "Calendar"}
	calendar.AddCommand(search)
	root.AddCommand(calendar)

	payload, err := runtimeSchemaPayload(root, []string{"calendar search"})
	if err != nil {
		t.Fatalf("runtimeSchemaPayload() error = %v", err)
	}
	if payload["title"] != "Embedded title" || payload["description"] != "Embedded description" {
		t.Fatalf("payload metadata = %#v", payload)
	}
	if payload["metadata_source"] != "embedded-mcp-metadata" {
		t.Fatalf("metadata_source = %v", payload["metadata_source"])
	}
	interfaceRef, _ := payload["interface_ref"].(map[string]any)
	if interfaceRef["product_id"] != "calendar" || interfaceRef["rpc_name"] != "search_calendar_events" {
		t.Fatalf("interface_ref = %#v", payload["interface_ref"])
	}
	params, _ := payload["parameters"].(map[string]any)
	start, _ := params["start"].(map[string]any)
	if start["description"] != "fallback start" || start["interface_description"] != "Embedded start time" || start["format"] != "date-time" || start["required"] != false || start["type"] != "string" || start["interface_type"] != "number" {
		t.Fatalf("start param = %#v", start)
	}
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestNewMCPCommandReturnsLoaderErrorForInvocations(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("fixture missing")
	cmd := NewMCPCommand(context.Background(), errorLoader{err: wantErr}, executor.EchoRunner{}, nil)
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

	cmd := NewMCPCommand(context.Background(), StaticLoader{
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
	}, executor.EchoRunner{}, nil)

	if got := cmd.Commands(); len(got) != 1 || got[0].Name() != "drive" {
		t.Fatalf("mcp commands = %#v, want only drive", got)
	}
}

func TestProductCommandUsesCLICommandAlias(t *testing.T) {
	t.Parallel()

	runner := &captureRunner{}
	cmd := NewMCPCommand(context.Background(), StaticLoader{
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
	}, runner, nil)

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
	cmd := NewMCPCommand(context.Background(), StaticLoader{
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
	}, runner, nil)

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
	cmd := NewMCPCommand(context.Background(), StaticLoader{
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
	}, runner, nil)
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

func TestToolCommandValidatesInputSchemaBeforeRun(t *testing.T) {
	t.Parallel()

	runner := &captureRunner{}
	cmd := NewMCPCommand(context.Background(), StaticLoader{
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
	}, runner, nil)

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
	cmd := NewMCPCommand(context.Background(), StaticLoader{
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
	}, runner, nil)

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

	cmd := NewMCPCommand(context.Background(), StaticLoader{
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
	}, executor.EchoRunner{}, nil)

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"legacy-doc", "search_documents"})
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

	cmd := NewMCPCommand(context.Background(), StaticLoader{
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
	}, executor.EchoRunner{}, nil)

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

	cmd := NewMCPCommand(context.Background(), StaticLoader{
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
	}, executor.EchoRunner{}, nil)

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

	cmd := NewMCPCommand(context.Background(), StaticLoader{
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
	}, executor.EchoRunner{}, nil)

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"legacy-candidate", "search_documents"})
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

// ---------------------------------------------------------------------------
// Input source resolution: @file for string flags
// ---------------------------------------------------------------------------

func TestToolCommandResolvesAtFileForStringFlag(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	filePath := filepath.Join(dir, "msg.md")
	if err := os.WriteFile(filePath, []byte("Hello from file"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	runner := &captureRunner{}
	cmd := newTestMCPCommand(t, ir.Catalog{
		Products: []ir.CanonicalProduct{
			{
				ID: "chat",
				Tools: []ir.ToolDescriptor{
					{
						RPCName: "send_message",
						InputSchema: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"text":    map[string]any{"type": "string"},
								"user_id": map[string]any{"type": "string"},
							},
						},
					},
				},
			},
		},
	}, runner)

	cmd.SetArgs([]string{"chat", "send_message", "--text", "@" + filePath, "--user-id", "u001"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if runner.last.Params["text"] != "Hello from file" {
		t.Errorf("params[text] = %q, want %q", runner.last.Params["text"], "Hello from file")
	}
	if runner.last.Params["user_id"] != "u001" {
		t.Errorf("params[user_id] = %q, want %q", runner.last.Params["user_id"], "u001")
	}
}

func TestToolCommandResolvesAtFileForJsonFlag(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	filePath := filepath.Join(dir, "payload.json")
	payload := `{"text":"from json file","user_id":"u002"}`
	if err := os.WriteFile(filePath, []byte(payload), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	runner := &captureRunner{}
	cmd := newTestMCPCommand(t, ir.Catalog{
		Products: []ir.CanonicalProduct{
			{
				ID: "chat",
				Tools: []ir.ToolDescriptor{
					{
						RPCName: "send_message",
						InputSchema: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"text":    map[string]any{"type": "string"},
								"user_id": map[string]any{"type": "string"},
							},
						},
					},
				},
			},
		},
	}, runner)

	cmd.SetArgs([]string{"chat", "send_message", "--json", "@" + filePath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if runner.last.Params["text"] != "from json file" {
		t.Errorf("params[text] = %q, want %q", runner.last.Params["text"], "from json file")
	}
	if runner.last.Params["user_id"] != "u002" {
		t.Errorf("params[user_id] = %q, want %q", runner.last.Params["user_id"], "u002")
	}
}

func TestToolCommandMultipleAtFileFlags(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	titlePath := filepath.Join(dir, "title.txt")
	bodyPath := filepath.Join(dir, "body.md")
	if err := os.WriteFile(titlePath, []byte("My Title"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.WriteFile(bodyPath, []byte("# Body\n\nContent here"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	runner := &captureRunner{}
	cmd := newTestMCPCommand(t, ir.Catalog{
		Products: []ir.CanonicalProduct{
			{
				ID: "doc",
				Tools: []ir.ToolDescriptor{
					{
						RPCName: "create_document",
						InputSchema: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"title": map[string]any{"type": "string"},
								"body":  map[string]any{"type": "string"},
							},
						},
					},
				},
			},
		},
	}, runner)

	cmd.SetArgs([]string{"doc", "create_document", "--title", "@" + titlePath, "--body", "@" + bodyPath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if runner.last.Params["title"] != "My Title" {
		t.Errorf("params[title] = %q, want %q", runner.last.Params["title"], "My Title")
	}
	if runner.last.Params["body"] != "# Body\n\nContent here" {
		t.Errorf("params[body] = %q, want %q", runner.last.Params["body"], "# Body\n\nContent here")
	}
}

func TestToolCommandAtFileMissingReturnsError(t *testing.T) {
	t.Parallel()

	runner := &captureRunner{}
	cmd := newTestMCPCommand(t, ir.Catalog{
		Products: []ir.CanonicalProduct{
			{
				ID: "chat",
				Tools: []ir.ToolDescriptor{
					{
						RPCName: "send_message",
						InputSchema: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"text": map[string]any{"type": "string"},
							},
						},
					},
				},
			},
		},
	}, runner)

	cmd.SetArgs([]string{"chat", "send_message", "--text", "@/nonexistent/file.txt"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() should fail for missing @file")
	}
	if !strings.Contains(err.Error(), "--text") {
		t.Errorf("error should mention flag name, got: %v", err)
	}
	if runner.called != 0 {
		t.Error("runner should not be called on @file error")
	}
}

func TestToolCommandAtFileForJsonMissingReturnsError(t *testing.T) {
	t.Parallel()

	runner := &captureRunner{}
	cmd := newTestMCPCommand(t, ir.Catalog{
		Products: []ir.CanonicalProduct{
			{
				ID: "chat",
				Tools: []ir.ToolDescriptor{
					{
						RPCName: "send_message",
						InputSchema: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"text": map[string]any{"type": "string"},
							},
						},
					},
				},
			},
		},
	}, runner)

	cmd.SetArgs([]string{"chat", "send_message", "--json", "@/nonexistent/payload.json"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() should fail for missing @file on --json")
	}
	if !strings.Contains(err.Error(), "--json") {
		t.Errorf("error should mention --json, got: %v", err)
	}
}

func TestToolCommandAtFileUTF8ContentPreserved(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	filePath := filepath.Join(dir, "chinese.txt")
	content := "你好世界 🌍\n第二行"
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	runner := &captureRunner{}
	cmd := newTestMCPCommand(t, ir.Catalog{
		Products: []ir.CanonicalProduct{
			{
				ID: "chat",
				Tools: []ir.ToolDescriptor{
					{
						RPCName: "send_message",
						InputSchema: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"text": map[string]any{"type": "string"},
							},
						},
					},
				},
			},
		},
	}, runner)

	cmd.SetArgs([]string{"chat", "send_message", "--text", "@" + filePath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if runner.last.Params["text"] != content {
		t.Errorf("params[text] = %q, want %q", runner.last.Params["text"], content)
	}
}

func TestToolCommandPlainAtValueNotResolvedForNonStringFlags(t *testing.T) {
	t.Parallel()

	// Integer and boolean flags should NOT resolve @file syntax.
	runner := &captureRunner{}
	cmd := newTestMCPCommand(t, ir.Catalog{
		Products: []ir.CanonicalProduct{
			{
				ID: "todo",
				Tools: []ir.ToolDescriptor{
					{
						RPCName: "create_task",
						InputSchema: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"title":    map[string]any{"type": "string"},
								"priority": map[string]any{"type": "integer"},
							},
						},
					},
				},
			},
		},
	}, runner)

	cmd.SetArgs([]string{"todo", "create_task", "--title", "test", "--priority", "3"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if runner.last.Params["priority"] != 3 {
		t.Errorf("params[priority] = %v, want 3", runner.last.Params["priority"])
	}
}

// ---------------------------------------------------------------------------
// Input source resolution: --json @file override priority
// ---------------------------------------------------------------------------

func TestToolCommandJsonFlagOverridesOverrideFlags(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	filePath := filepath.Join(dir, "base.json")
	if err := os.WriteFile(filePath, []byte(`{"text":"from-json","user_id":"json-user"}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	runner := &captureRunner{}
	cmd := newTestMCPCommand(t, ir.Catalog{
		Products: []ir.CanonicalProduct{
			{
				ID: "chat",
				Tools: []ir.ToolDescriptor{
					{
						RPCName: "send_message",
						InputSchema: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"text":    map[string]any{"type": "string"},
								"user_id": map[string]any{"type": "string"},
							},
						},
					},
				},
			},
		},
	}, runner)

	// --text override should win over --json base payload.
	cmd.SetArgs([]string{"chat", "send_message", "--json", "@" + filePath, "--text", "override"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if runner.last.Params["text"] != "override" {
		t.Errorf("params[text] = %q, want %q (override should win)", runner.last.Params["text"], "override")
	}
	if runner.last.Params["user_id"] != "json-user" {
		t.Errorf("params[user_id] = %q, want %q (from json base)", runner.last.Params["user_id"], "json-user")
	}
}

// ---------------------------------------------------------------------------
// Sensitive tool + stdin guard interaction
// ---------------------------------------------------------------------------

func TestSensitiveToolWithStdinClaimedRequiresYes(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	filePath := filepath.Join(dir, "msg.txt")
	if err := os.WriteFile(filePath, []byte("content"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Sensitive tool + @file (does NOT claim stdin) → should still prompt.
	// We provide "yes" on stdin to pass confirmation.
	runner := &captureRunner{}
	cmd := newTestMCPCommand(t, ir.Catalog{
		Products: []ir.CanonicalProduct{
			{
				ID: "doc",
				Tools: []ir.ToolDescriptor{
					{
						RPCName:   "delete_document",
						Sensitive: true,
						InputSchema: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"doc_id": map[string]any{"type": "string"},
							},
						},
					},
				},
			},
		},
	}, runner)

	cmd.SetIn(strings.NewReader("yes\n"))
	cmd.SetArgs([]string{"doc", "delete_document", "--doc-id", "DOC001"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if runner.called != 1 {
		t.Errorf("runner called = %d, want 1", runner.called)
	}
}

func TestSensitiveToolDeniedOnStdinWithNoYesFlag(t *testing.T) {
	t.Parallel()

	runner := &captureRunner{}
	cmd := newTestMCPCommand(t, ir.Catalog{
		Products: []ir.CanonicalProduct{
			{
				ID: "doc",
				Tools: []ir.ToolDescriptor{
					{
						RPCName:   "delete_document",
						Sensitive: true,
						InputSchema: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"doc_id": map[string]any{"type": "string"},
							},
						},
					},
				},
			},
		},
	}, runner)

	cmd.SetIn(strings.NewReader("no\n"))
	cmd.SetArgs([]string{"doc", "delete_document", "--doc-id", "DOC001"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() should fail when user denies confirmation")
	}
	if !strings.Contains(err.Error(), "cancelled") {
		t.Errorf("error should mention cancellation, got: %v", err)
	}
	if runner.called != 0 {
		t.Error("runner should not be called when confirmation denied")
	}
}

func TestSensitiveToolWithYesFlagSkipsConfirmation(t *testing.T) {
	t.Parallel()

	runner := &captureRunner{}
	cmd := newTestMCPCommand(t, ir.Catalog{
		Products: []ir.CanonicalProduct{
			{
				ID: "doc",
				Tools: []ir.ToolDescriptor{
					{
						RPCName:   "delete_document",
						Sensitive: true,
						InputSchema: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"doc_id": map[string]any{"type": "string"},
							},
						},
					},
				},
			},
		},
	}, runner)

	cmd.PersistentFlags().Bool("yes", false, "Skip confirmation")
	cmd.SetArgs([]string{"doc", "delete_document", "--doc-id", "DOC001", "--yes"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if runner.called != 1 {
		t.Errorf("runner called = %d, want 1", runner.called)
	}
}

// ---------------------------------------------------------------------------
// collectOverrides: @file does not affect non-string flag types
// ---------------------------------------------------------------------------

func TestCollectOverridesResolvesAtFileOnlyForStringKind(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	filePath := filepath.Join(dir, "name.txt")
	if err := os.WriteFile(filePath, []byte("resolved name"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	runner := &captureRunner{}
	cmd := newTestMCPCommand(t, ir.Catalog{
		Products: []ir.CanonicalProduct{
			{
				ID: "contact",
				Tools: []ir.ToolDescriptor{
					{
						RPCName: "search_user",
						InputSchema: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"keyword": map[string]any{"type": "string"},
								"active":  map[string]any{"type": "boolean"},
								"limit":   map[string]any{"type": "integer"},
							},
						},
					},
				},
			},
		},
	}, runner)

	cmd.SetArgs([]string{"contact", "search_user",
		"--keyword", "@" + filePath,
		"--active=true",
		"--limit", "10",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if runner.last.Params["keyword"] != "resolved name" {
		t.Errorf("params[keyword] = %q, want %q", runner.last.Params["keyword"], "resolved name")
	}
	if runner.last.Params["active"] != true {
		t.Errorf("params[active] = %v, want true", runner.last.Params["active"])
	}
	if runner.last.Params["limit"] != 10 {
		t.Errorf("params[limit] = %v, want 10", runner.last.Params["limit"])
	}
}

// ---------------------------------------------------------------------------
// Test helper
// ---------------------------------------------------------------------------

func newTestMCPCommand(t *testing.T, catalog ir.Catalog, runner executor.Runner) *cobra.Command {
	t.Helper()
	cmd := NewMCPCommand(context.Background(), StaticLoader{Catalog: catalog}, runner, nil)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	return cmd
}

func TestSchemaCommandDoesNotNeedDiscovery(t *testing.T) {
	t.Parallel()

	cmd := NewSchemaCommand()

	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v\noutput:\n%s", err, out.String())
	}
	if payload["count"] != float64(21) {
		t.Fatalf("payload[count] = %v, want embedded product count 21", payload["count"])
	}
	if errOut.String() != "" {
		t.Fatalf("stderr = %q, want empty", errOut.String())
	}
}

func TestRuntimeSchemaRootIncludesVisibleCommandsAndSkipsHiddenCommands(t *testing.T) {
	t.Parallel()

	root := &cobra.Command{Use: "dws"}
	patRoot := &cobra.Command{Use: "pat", Short: "行为授权管理", RunE: func(*cobra.Command, []string) error { return nil }}
	patRoot.AddCommand(
		&cobra.Command{Use: "chmod", Short: "授权", RunE: func(*cobra.Command, []string) error { return nil }},
		&cobra.Command{Use: "browser-policy", Short: "本地策略", RunE: func(*cobra.Command, []string) error { return nil }},
		&cobra.Command{Use: "hidden", Hidden: true, RunE: func(*cobra.Command, []string) error { return nil }},
	)
	root.AddCommand(patRoot)

	payload, err := runtimeSchemaPayload(root, nil)
	if err != nil {
		t.Fatalf("runtimeSchemaPayload() error = %v", err)
	}
	products, ok := payload["products"].([]map[string]any)
	if !ok {
		t.Fatalf("products type = %T, want []map[string]any", payload["products"])
	}
	var paths []string
	for _, product := range products {
		if product["id"] != "pat" {
			continue
		}
		tools, ok := product["tools"].([]map[string]any)
		if !ok {
			t.Fatalf("tools type = %T, want []map[string]any", product["tools"])
		}
		for _, tool := range tools {
			paths = append(paths, tool["cli_path"].(string))
		}
	}
	got := map[string]bool{}
	for _, path := range paths {
		got[path] = true
	}
	for _, want := range []string{"pat chmod", "pat browser-policy"} {
		if !got[want] {
			t.Fatalf("pat schema cli paths = %q, missing %q", strings.Join(paths, ","), want)
		}
	}
	if got["pat hidden"] {
		t.Fatalf("pat schema cli paths = %q, hidden command must not be included", strings.Join(paths, ","))
	}
}

func TestRuntimeSchemaRootFallbackDoesNotDuplicateAnnotatedCLIPath(t *testing.T) {
	t.Parallel()

	root := &cobra.Command{Use: "dws"}
	attendance := &cobra.Command{Use: "attendance", Short: "考勤"}
	adjustment := &cobra.Command{Use: "adjustment"}
	get := &cobra.Command{Use: "get", Short: "查询补卡规则", RunE: func(*cobra.Command, []string) error { return nil }}
	AttachRuntimeSchema(get, "hrmregister", "get_adjustment_rule_detail", "runtime:hrmregister")
	adjustment.AddCommand(get)
	attendance.AddCommand(adjustment)
	root.AddCommand(attendance)

	entries := collectRuntimeSchemaEntries(root)
	matched := []runtimeSchemaEntry{}
	for _, entry := range entries {
		if entry.CLIPath == "attendance adjustment get" {
			matched = append(matched, entry)
		}
	}
	if len(matched) != 1 {
		t.Fatalf("matching entries = %#v, want exactly one", matched)
	}
	if matched[0].ToolName != "get_adjustment_rule_detail" || matched[0].SourceProductID != "hrmregister" {
		t.Fatalf("annotated entry was not preserved: %#v", matched[0])
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
