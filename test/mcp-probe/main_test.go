package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/test/mcp-probe/runner"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/test/mcp-probe/schema"
)

func TestBuildVariantsCoversFlattenedNestedJSONForms(t *testing.T) {
	t.Parallel()

	tool := schema.ToolSchema{
		CLIPath: []string{"chat", "search"},
		Tool: schema.Tool{
			RPCName:       "search_groups_by_keyword",
			CLIName:       "search",
			CanonicalPath: "group-chat.search_groups_by_keyword",
		},
		Flags: []schema.Flag{
			{PropertyName: "OpenSearchRequest.cursor", FlagName: "cursor", Kind: "string"},
			{PropertyName: "OpenSearchRequest.query", FlagName: "query", Kind: "string"},
		},
		FlagHints: map[string]schema.FlagHint{
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

	variants, err := buildVariants(tool)
	if err != nil {
		t.Fatalf("buildVariants() error = %v", err)
	}

	if len(variants) != 3 {
		t.Fatalf("buildVariants() len = %d, want 3", len(variants))
	}

	wantLabels := []string{
		"group-chat.search_groups_by_keyword [flags]",
		"group-chat.search_groups_by_keyword [json-flat]",
		"group-chat.search_groups_by_keyword [json]",
	}
	gotLabels := []string{variants[0].label, variants[1].label, variants[2].label}
	if !reflect.DeepEqual(gotLabels, wantLabels) {
		t.Fatalf("buildVariants() labels = %#v, want %#v", gotLabels, wantLabels)
	}
}

func TestRunTruthSurfaceParityRespectsFilterAndReportsMismatch(t *testing.T) {
	t.Parallel()

	truthBinary := writeFakeProbeHelpBinary(t, map[string]string{
		"--help": `DWS CLI

Usage:
  dws [command]

Available Commands:
  chat        群聊 / 会话 / 群组管理
  credit      企业信用查询
`,
		"chat --help": `群聊 / 会话 / 群组管理

Usage:
  dws chat [flags]
  dws chat [command]

Available Commands:
  search      根据名称搜索会话列表

Flags:
  -h, --help   help for chat
`,
		"credit --help": `企业信用查询

Usage:
  dws credit [flags]
`,
		"chat search --help": `根据名称搜索会话列表

Usage:
  dws chat search [flags]

Flags:
      --cursor string   分页游标 (首页留空)
  -h, --help            help for search
      --query string    搜索关键词 (必填)
`,
	})
	candidateBinary := writeFakeProbeHelpBinary(t, map[string]string{
		"--help": `DWS CLI

Usage:
  dws [command]

Available Commands:
  chat        群聊 / 会话 / 群组管理
`,
		"chat --help": `群聊 / 会话 / 群组管理

Usage:
  dws chat [flags]
  dws chat [command]

Available Commands:
  search      根据名称搜索会话列表

Flags:
  -h, --help   help for chat
`,
		"chat search --help": `根据群名称关键词，搜索符合条件的群，返回群的openconversion_id、群名称等信息

Usage:
  dws chat search [flags]

Flags:
      --cursor string   cursor
  -h, --help            help for search
      --json string     Base JSON object payload for this tool invocation
      --params string   Additional JSON object payload merged after --json
      --query string    query
`,
	})

	results, err := runTruthSurfaceParity(
		context.Background(),
		&runner.Runner{DWSBinary: truthBinary, ExtraEnv: []string{"PATH=/usr/bin:/bin"}, Timeout: 2 * time.Second},
		&runner.Runner{DWSBinary: candidateBinary, ExtraEnv: []string{"PATH=/usr/bin:/bin"}, Timeout: 2 * time.Second},
		map[string]struct{}{"chat": {}},
		"chat search",
		false,
	)
	if err != nil {
		t.Fatalf("runTruthSurfaceParity() error = %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("runTruthSurfaceParity() len = %d, want 1", len(results))
	}
	if results[0].label != "chat search [truth-surface]" {
		t.Fatalf("label = %q, want chat search [truth-surface]", results[0].label)
	}
	if results[0].passed {
		t.Fatalf("passed = true, want false")
	}
	if results[0].diff == "" {
		t.Fatalf("diff = empty, want mismatch details")
	}
	if results[0].diff == "summary mismatch" {
		t.Fatalf("diff should not compare summary: %q", results[0].diff)
	}
}

func TestRunTruthSurfaceParityPassesForSupportedPagesAndSkipsRoot(t *testing.T) {
	t.Parallel()

	truthBinary := writeFakeProbeHelpBinary(t, map[string]string{
		"--help": `DWS CLI

Usage:
  dws [command]

Available Commands:
  aitable     AI 表格操作
  credit      企业信用查询
`,
		"aitable --help": `AI 表格操作

Usage:
  dws aitable [flags]

Flags:
  -h, --help   help for aitable
`,
		"credit --help": `企业信用查询

Usage:
  dws credit [flags]
`,
	})
	candidateBinary := writeFakeProbeHelpBinary(t, map[string]string{
		"--help": `Discovered MCP Services:

Usage:
  dws <service> [command] [flags]

Available Commands:
  aitable     这是不同的描述
`,
		"aitable --help": `这是不同的摘要

Usage:
  dws aitable [flags]

Flags:
  -h, --help            help for aitable
`,
	})

	results, err := runTruthSurfaceParity(
		context.Background(),
		&runner.Runner{DWSBinary: truthBinary, ExtraEnv: []string{"PATH=/usr/bin:/bin"}, Timeout: 2 * time.Second},
		&runner.Runner{DWSBinary: candidateBinary, ExtraEnv: []string{"PATH=/usr/bin:/bin"}, Timeout: 2 * time.Second},
		map[string]struct{}{"aitable": {}},
		"",
		false,
	)
	if err != nil {
		t.Fatalf("runTruthSurfaceParity() error = %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("runTruthSurfaceParity() len = %d, want 1", len(results))
	}
	if results[0].label != "aitable [truth-surface]" {
		t.Fatalf("label = %q, want aitable [truth-surface]", results[0].label)
	}
	if !results[0].passed {
		t.Fatalf("result %#v, want passed", results[0])
	}
}

func TestFetchSchemaUsesDefaultJSONOutput(t *testing.T) {
	t.Parallel()

	fakeBinary := writeFakeProbeHelpBinary(t, map[string]string{
		"schema":                  `{"kind":"schema","scope":"catalog","products":[],"count":0}`,
		"schema bot.create_robot": `{"kind":"schema","scope":"tool","path":"bot.create_robot","tool":{"rpc_name":"create_robot"},"input_schema":{"type":"object"},"flags":[],"required":[],"cli_path":["chat","create_robot"]}`,
	})

	catalogJSON, err := fetchSchema(context.Background(), fakeBinary, "")
	if err != nil {
		t.Fatalf("fetchSchema(catalog) error = %v", err)
	}
	var catalog map[string]any
	if err := json.Unmarshal(catalogJSON, &catalog); err != nil {
		t.Fatalf("fetchSchema(catalog) returned non-JSON output: %v\n%s", err, string(catalogJSON))
	}
	if got := catalog["scope"]; got != "catalog" {
		t.Fatalf("catalog scope = %#v, want catalog", got)
	}

	toolJSON, err := fetchSchema(context.Background(), fakeBinary, "bot.create_robot")
	if err != nil {
		t.Fatalf("fetchSchema(tool) error = %v", err)
	}
	var tool map[string]any
	if err := json.Unmarshal(toolJSON, &tool); err != nil {
		t.Fatalf("fetchSchema(tool) returned non-JSON output: %v\n%s", err, string(toolJSON))
	}
	if got := tool["path"]; got != "bot.create_robot" {
		t.Fatalf("tool path = %#v, want bot.create_robot", got)
	}
}

func writeFakeProbeHelpBinary(t *testing.T, responses map[string]string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "fake-dws")
	content := "#!/bin/sh\ncase \"$*\" in\n"
	for args, output := range responses {
		content += "  " + shellQuote(args) + ")\n"
		content += "    cat <<'EOF'\n" + output + "\nEOF\n"
		content += "    ;;\n"
	}
	content += "  *)\n"
	content += "    echo \"unexpected args: $*\" >&2\n"
	content += "    exit 1\n"
	content += "    ;;\n"
	content += "esac\n"

	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

func shellQuote(input string) string {
	return "'" + input + "'"
}
