package schema

import (
	"reflect"
	"testing"
	"time"
)

func TestParseToolSchemaIncludesCLIPathAndFlags(t *testing.T) {
	t.Parallel()

	tool, err := ParseTool([]byte(`{
	  "path": "todo.get_user_todos_in_current_org",
	  "cli_path": ["todo", "task", "list"],
	  "required": ["pageSize", "pageNum"],
	  "flag_hints": {
	    "pageNum": {"alias":"page","transform":"enum_map","transform_args":{"one":1},"required":true}
	  },
	  "flags": [
	    {"property_name":"pageNum","flag_name":"pageNum","alias":"page","kind":"string"},
	    {"property_name":"pageSize","flag_name":"pageSize","alias":"size","kind":"string"},
	    {"property_name":"todoStatus","flag_name":"todoStatus","kind":"string"}
	  ],
	  "input_schema": {
	    "type": "object",
	    "properties": {
	      "pageNum": {"type":"string"},
	      "pageSize": {"type":"string"},
	      "todoStatus": {"type":"string"}
	    },
	    "required": ["pageSize", "pageNum"]
	  },
	  "tool": {
	    "rpc_name": "get_user_todos_in_current_org",
	    "cli_name": "list",
	    "canonical_path": "todo.get_user_todos_in_current_org"
	  }
	}`))
	if err != nil {
		t.Fatalf("ParseTool() error = %v", err)
	}

	if tool.Tool.RPCName != "get_user_todos_in_current_org" {
		t.Fatalf("RPCName = %q, want get_user_todos_in_current_org", tool.Tool.RPCName)
	}
	if !reflect.DeepEqual(tool.CLIPath, []string{"todo", "task", "list"}) {
		t.Fatalf("CLIPath = %#v, want [todo task list]", tool.CLIPath)
	}
	if len(tool.Flags) != 3 || tool.Flags[0].PropertyName != "pageNum" {
		t.Fatalf("Flags = %#v, want schema flags preserved", tool.Flags)
	}
	if tool.FlagHints["pageNum"].Transform != "enum_map" || !tool.FlagHints["pageNum"].Required {
		t.Fatalf("FlagHints = %#v, want transform+required preserved", tool.FlagHints)
	}
}

func TestToolSchemaBuildProbeInputsAndFlagArgs(t *testing.T) {
	t.Parallel()

	tool := ToolSchema{
		CLIPath: []string{"todo", "task", "list"},
		Flags: []Flag{
			{PropertyName: "pageNum", FlagName: "pageNum", Alias: "page", Kind: "string"},
			{PropertyName: "pageSize", FlagName: "pageSize", Alias: "size", Kind: "integer"},
			{PropertyName: "includeDone", FlagName: "includeDone", Kind: "boolean"},
			{PropertyName: "labels", FlagName: "labels", Kind: "string_array"},
			{PropertyName: "filters", FlagName: "filters", Kind: "json"},
		},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pageNum":     map[string]any{"type": "string"},
				"pageSize":    map[string]any{"type": "integer"},
				"includeDone": map[string]any{"type": "boolean"},
				"labels": map[string]any{
					"type":  "array",
					"items": map[string]any{"type": "string"},
				},
				"filters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"owner": map[string]any{"type": "string"},
					},
				},
			},
		},
	}

	args, err := tool.GenerateArguments()
	if err != nil {
		t.Fatalf("GenerateArguments() error = %v", err)
	}

	wantArgs := map[string]any{
		"pageNum":     "probe-pageNum",
		"pageSize":    int64(7),
		"includeDone": true,
		"labels":      []any{"probe-labels-1", "probe-labels-2"},
		"filters":     map[string]any{"owner": "probe-owner"},
	}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("GenerateArguments() = %#v, want %#v", args, wantArgs)
	}

	flagArgs, err := BuildFlagArgs(tool, args, false)
	if err != nil {
		t.Fatalf("BuildFlagArgs(primary) error = %v", err)
	}
	wantPrimary := []string{
		"todo", "task", "list",
		"--pageNum", "probe-pageNum",
		"--pageSize", "7",
		"--includeDone=true",
		"--labels", "probe-labels-1,probe-labels-2",
		"--filters", `{"owner":"probe-owner"}`,
	}
	if !reflect.DeepEqual(flagArgs, wantPrimary) {
		t.Fatalf("BuildFlagArgs(primary) = %#v, want %#v", flagArgs, wantPrimary)
	}

	aliasArgs, err := BuildFlagArgs(tool, args, true)
	if err != nil {
		t.Fatalf("BuildFlagArgs(alias) error = %v", err)
	}
	wantAlias := []string{
		"todo", "task", "list",
		"--page", "probe-pageNum",
		"--size", "7",
		"--includeDone=true",
		"--labels", "probe-labels-1,probe-labels-2",
		"--filters", `{"owner":"probe-owner"}`,
	}
	if !reflect.DeepEqual(aliasArgs, wantAlias) {
		t.Fatalf("BuildFlagArgs(alias) = %#v, want %#v", aliasArgs, wantAlias)
	}
}

func TestToolSchemaGenerateArgumentsRespectsTransforms(t *testing.T) {
	t.Parallel()

	tool := ToolSchema{
		CLIPath: []string{"ding", "message", "send"},
		Flags: []Flag{
			{PropertyName: "fromDateTime", FlagName: "fromDateTime", Kind: "string"},
			{PropertyName: "deptIds", FlagName: "deptIds", Kind: "string_array"},
			{PropertyName: "remindType", FlagName: "remindType", Kind: "string"},
		},
		FlagHints: map[string]FlagHint{
			"fromDateTime": {Transform: "iso8601_to_millis"},
			"deptIds":      {Transform: "csv_to_array"},
			"remindType": {
				Transform: "enum_map",
				TransformArgs: map[string]any{
					"app":      1,
					"sms":      2,
					"_default": 9,
				},
			},
		},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"fromDateTime": map[string]any{"type": "number"},
				"deptIds": map[string]any{
					"type":  "array",
					"items": map[string]any{"type": "integer"},
				},
				"remindType": map[string]any{"type": "number"},
			},
		},
	}

	rawArgs, err := tool.GenerateArguments()
	if err != nil {
		t.Fatalf("GenerateArguments() error = %v", err)
	}

	wantRaw := map[string]any{
		"fromDateTime": "2026-03-27T08:09:10Z",
		"deptIds":      []any{int64(7), int64(8)},
		"remindType":   "app",
	}
	if !reflect.DeepEqual(rawArgs, wantRaw) {
		t.Fatalf("GenerateArguments() = %#v, want %#v", rawArgs, wantRaw)
	}

	normalized, err := tool.NormalizeArguments(rawArgs)
	if err != nil {
		t.Fatalf("NormalizeArguments() error = %v", err)
	}
	wantNormalized := map[string]any{
		"fromDateTime": time.Date(2026, time.March, 27, 8, 9, 10, 0, time.UTC).UnixMilli(),
		"deptIds":      []any{int64(7), int64(8)},
		"remindType":   1,
	}
	if !reflect.DeepEqual(normalized, wantNormalized) {
		t.Fatalf("NormalizeArguments() = %#v, want %#v", normalized, wantNormalized)
	}
}

func TestBuildFlagArgsEncodesNumericCSVArrayAsJSONArray(t *testing.T) {
	t.Parallel()

	tool := ToolSchema{
		CLIPath: []string{"contact", "dept", "list-members"},
		Flags: []Flag{
			{PropertyName: "deptIds", FlagName: "deptIds", Alias: "ids", Kind: "string_array"},
		},
		FlagHints: map[string]FlagHint{
			"deptIds": {Transform: "csv_to_array"},
		},
	}
	args := map[string]any{
		"deptIds": []any{int64(7), int64(8)},
	}

	flagArgs, err := BuildFlagArgs(tool, args, false)
	if err != nil {
		t.Fatalf("BuildFlagArgs() error = %v", err)
	}
	want := []string{
		"contact", "dept", "list-members",
		"--deptIds", `[7,8]`,
	}
	if !reflect.DeepEqual(flagArgs, want) {
		t.Fatalf("BuildFlagArgs() = %#v, want %#v", flagArgs, want)
	}
}

func TestToolSchemaGenerateArgumentsUsesFlattenedNestedFlags(t *testing.T) {
	t.Parallel()

	tool := ToolSchema{
		CLIPath: []string{"chat", "search"},
		Flags: []Flag{
			{PropertyName: "OpenSearchRequest.cursor", FlagName: "cursor", Kind: "string"},
			{PropertyName: "OpenSearchRequest.query", FlagName: "query", Kind: "string"},
		},
		FlagHints: map[string]FlagHint{
			"OpenSearchRequest.cursor": {Alias: "cursor"},
			"OpenSearchRequest.query":  {Alias: "query", Required: true},
		},
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
	}

	args, err := tool.GenerateArguments()
	if err != nil {
		t.Fatalf("GenerateArguments() error = %v", err)
	}
	wantArgs := map[string]any{
		"OpenSearchRequest.cursor": "probe-OpenSearchRequest.cursor",
		"OpenSearchRequest.query":  "probe-OpenSearchRequest.query",
	}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("GenerateArguments() = %#v, want %#v", args, wantArgs)
	}

	flagArgs, err := BuildFlagArgs(tool, args, false)
	if err != nil {
		t.Fatalf("BuildFlagArgs() error = %v", err)
	}
	wantFlags := []string{
		"chat", "search",
		"--cursor", "probe-OpenSearchRequest.cursor",
		"--query", "probe-OpenSearchRequest.query",
	}
	if !reflect.DeepEqual(flagArgs, wantFlags) {
		t.Fatalf("BuildFlagArgs() = %#v, want %#v", flagArgs, wantFlags)
	}
}

func TestBuildJSONArgsNestsDottedArguments(t *testing.T) {
	t.Parallel()

	tool := ToolSchema{
		CLIPath: []string{"chat", "search"},
	}
	args := map[string]any{
		"OpenSearchRequest.cursor": "probe-cursor",
		"OpenSearchRequest.query":  "probe-query",
	}

	jsonArgs, err := BuildJSONArgs(tool, args, "--json")
	if err != nil {
		t.Fatalf("BuildJSONArgs() error = %v", err)
	}
	want := []string{
		"chat", "search",
		"--json", `{"OpenSearchRequest":{"cursor":"probe-cursor","query":"probe-query"}}`,
	}
	if !reflect.DeepEqual(jsonArgs, want) {
		t.Fatalf("BuildJSONArgs() = %#v, want %#v", jsonArgs, want)
	}
}

func TestBuildPublicJSONArgsUsesFlattenedFlagNames(t *testing.T) {
	t.Parallel()

	tool := ToolSchema{
		CLIPath: []string{"chat", "search"},
		Flags: []Flag{
			{PropertyName: "OpenSearchRequest.cursor", FlagName: "cursor", Kind: "string"},
			{PropertyName: "OpenSearchRequest.query", FlagName: "query", Kind: "string"},
		},
	}
	args := map[string]any{
		"OpenSearchRequest.cursor": "probe-cursor",
		"OpenSearchRequest.query":  "probe-query",
	}

	jsonArgs, err := BuildPublicJSONArgs(tool, args, "--json", false)
	if err != nil {
		t.Fatalf("BuildPublicJSONArgs() error = %v", err)
	}
	want := []string{
		"chat", "search",
		"--json", `{"cursor":"probe-cursor","query":"probe-query"}`,
	}
	if !reflect.DeepEqual(jsonArgs, want) {
		t.Fatalf("BuildPublicJSONArgs() = %#v, want %#v", jsonArgs, want)
	}
}

func TestToolSchemaNormalizeArgumentsKeepsISOStringsWhenSchemaExpectsString(t *testing.T) {
	t.Parallel()

	tool := ToolSchema{
		FlagHints: map[string]FlagHint{
			"startTime": {Transform: "iso8601_to_millis"},
			"endTime":   {Transform: "iso8601_to_millis"},
		},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"startTime": map[string]any{"type": "string"},
				"endTime":   map[string]any{"type": "string"},
			},
		},
	}

	normalized, err := tool.NormalizeArguments(map[string]any{
		"startTime": "2026-03-27T08:09:10Z",
		"endTime":   "2026-03-27T09:09:10Z",
	})
	if err != nil {
		t.Fatalf("NormalizeArguments() error = %v", err)
	}

	want := map[string]any{
		"startTime": "2026-03-27T08:09:10Z",
		"endTime":   "2026-03-27T09:09:10Z",
	}
	if !reflect.DeepEqual(normalized, want) {
		t.Fatalf("NormalizeArguments() = %#v, want %#v", normalized, want)
	}
}

func TestBuildFlagArgsKeepsPrimaryWhenAliasCollides(t *testing.T) {
	t.Parallel()

	tool := ToolSchema{
		CLIPath: []string{"todo", "task", "create"},
		Flags: []Flag{
			{PropertyName: "title", FlagName: "title", Alias: "name", Kind: "string"},
			{PropertyName: "name", FlagName: "name", Kind: "string"},
			{PropertyName: "category", FlagName: "category", Alias: "cat", Kind: "string"},
		},
	}
	args := map[string]any{
		"title":    "probe-title",
		"name":     "probe-name",
		"category": "probe-category",
	}

	aliasArgs, err := BuildFlagArgs(tool, args, true)
	if err != nil {
		t.Fatalf("BuildFlagArgs(alias) error = %v", err)
	}

	want := []string{
		"todo", "task", "create",
		"--title", "probe-title",
		"--name", "probe-name",
		"--cat", "probe-category",
	}
	if !reflect.DeepEqual(aliasArgs, want) {
		t.Fatalf("BuildFlagArgs(alias) = %#v, want %#v", aliasArgs, want)
	}
}

func TestBuildFixtureCatalogPreservesCLIShape(t *testing.T) {
	t.Parallel()

	catalog := Catalog{
		Products: []Product{
			{
				ID:          "todo",
				Command:     "todo",
				DisplayName: "Todo",
				Endpoint:    "http://127.0.0.1:8789/mcp",
			},
		},
	}
	tools := []ToolSchema{
		{
			Product: Product{
				ID:          "todo",
				Command:     "todo",
				DisplayName: "Todo",
				Endpoint:    "http://127.0.0.1:8789/mcp",
			},
			Tool: Tool{
				RPCName:       "get_user_todos_in_current_org",
				CLIName:       "list",
				CanonicalPath: "todo.get_user_todos_in_current_org",
			},
			Flags: []Flag{
				{PropertyName: "pageNum", FlagName: "pageNum", Alias: "page", Kind: "string"},
			},
			FlagHints: map[string]FlagHint{
				"pageNum": {
					Alias:     "page",
					Shorthand: "p",
					Transform: "enum_map",
					TransformArgs: map[string]any{
						"one": 1,
					},
					EnvDefault: "PAGE_NUM",
					Default:    "1",
					Hidden:     true,
					Required:   true,
				},
			},
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pageNum": map[string]any{"type": "string"},
				},
			},
			CLIPath: []string{"todo", "task", "list"},
		},
	}

	fixture := BuildFixtureCatalog(catalog, tools)
	if len(fixture.Products) != 1 {
		t.Fatalf("BuildFixtureCatalog() products = %d, want 1", len(fixture.Products))
	}
	product := fixture.Products[0]
	if product.CLI == nil || product.CLI.Command != "todo" {
		t.Fatalf("fixture product CLI = %#v, want command todo", product.CLI)
	}
	if len(product.Tools) != 1 {
		t.Fatalf("fixture product tools = %#v, want one tool", product.Tools)
	}
	tool := product.Tools[0]
	if tool.Group != "task" {
		t.Fatalf("fixture tool group = %q, want task", tool.Group)
	}
	if tool.FlagHints["pageNum"].Alias != "page" {
		t.Fatalf("fixture flag hint = %#v, want alias page", tool.FlagHints["pageNum"])
	}
	if tool.FlagHints["pageNum"].Transform != "enum_map" {
		t.Fatalf("fixture flag hint transform = %#v, want enum_map", tool.FlagHints["pageNum"])
	}
	if tool.FlagHints["pageNum"].EnvDefault != "PAGE_NUM" {
		t.Fatalf("fixture flag hint env default = %#v, want PAGE_NUM", tool.FlagHints["pageNum"])
	}
	if tool.FlagHints["pageNum"].Default != "1" {
		t.Fatalf("fixture flag hint default = %#v, want 1", tool.FlagHints["pageNum"])
	}
	if !tool.FlagHints["pageNum"].Hidden || !tool.FlagHints["pageNum"].Required {
		t.Fatalf("fixture flag hint = %#v, want hidden+required preserved", tool.FlagHints["pageNum"])
	}
	if got := tool.FlagHints["pageNum"].TransformArgs["one"]; got != 1 {
		t.Fatalf("fixture flag hint transform args = %#v, want one=1", tool.FlagHints["pageNum"].TransformArgs)
	}
	if tool.CanonicalPath != "todo.get_user_todos_in_current_org" {
		t.Fatalf("fixture canonical path = %q, want todo.get_user_todos_in_current_org", tool.CanonicalPath)
	}
}

func TestBuildFixtureCatalogProjectsPublicInputSchema(t *testing.T) {
	t.Parallel()

	catalog := Catalog{
		Products: []Product{{
			ID:          "aitable",
			Command:     "aitable",
			DisplayName: "AITable",
			Endpoint:    "http://127.0.0.1:8789/mcp",
		}},
	}
	tools := []ToolSchema{{
		Product: catalog.Products[0],
		Tool: Tool{
			RPCName:       "update_field",
			CLIName:       "update",
			CanonicalPath: "aitable.update_field",
		},
		Flags: []Flag{
			{PropertyName: "baseId", FlagName: "base-id", Kind: "string"},
			{PropertyName: "config", FlagName: "config", Kind: "json"},
		},
		FlagHints: map[string]FlagHint{
			"baseId": {Alias: "base-id", Required: true},
			"config": {Alias: "config", Transform: "json_parse"},
		},
		Required: []string{"baseId"},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"baseId": map[string]any{"type": "string"},
				"config": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"options": map[string]any{"type": "array"},
					},
				},
				"aiConfig": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"outputType": map[string]any{"type": "string"},
						"imageConfig": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"aiGeneratedWatermark": map[string]any{"type": "boolean"},
							},
							"required": []any{"aiGeneratedWatermark"},
						},
					},
					"required": []any{"outputType"},
				},
			},
		},
		CLIPath: []string{"aitable", "field", "update"},
	}}

	fixture := BuildFixtureCatalog(catalog, tools)
	schema := fixture.Products[0].Tools[0].InputSchema
	properties, _ := schema["properties"].(map[string]any)
	if _, ok := properties["aiConfig"]; ok {
		t.Fatalf("fixture schema unexpectedly retained hidden branch: %#v", schema)
	}
	if _, ok := properties["config"]; !ok {
		t.Fatalf("fixture schema missing public config property: %#v", schema)
	}
	required, _ := schema["required"].([]any)
	if !reflect.DeepEqual(required, []any{"baseId"}) {
		t.Fatalf("fixture required = %#v, want [baseId]", required)
	}
}
