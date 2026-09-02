package helpers

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestDevMCPCommandTree(t *testing.T) {
	dev := devHandler{}.Command(&captureRunner{})
	mcp, _, err := dev.Find([]string{"mcp"})
	if err != nil {
		t.Fatalf("Find(mcp) error = %v", err)
	}
	if mcp == nil || mcp.Name() != "mcp" {
		t.Fatalf("mcp command not registered: %#v", mcp)
	}
	for _, path := range [][]string{
		{"mcp", "service", "list"},
		{"mcp", "service", "get"},
		{"mcp", "service", "create"},
		{"mcp", "service", "update"},
		{"mcp", "service", "delete"},
		{"mcp", "tool", "list"},
		{"mcp", "tool", "get"},
		{"mcp", "tool", "create"},
		{"mcp", "tool", "update"},
		{"mcp", "tool", "debug"},
		{"mcp", "tool", "publish"},
		{"mcp", "tool", "delete"},
		{"mcp", "tool", "versions"},
		{"mcp", "tool", "create-hsf"},
		{"mcp", "tool", "update-hsf"},
		{"mcp", "url", "get"},
		{"mcp", "auth", "get"},
		{"mcp", "auth", "save"},
		{"mcp", "credential", "list"},
		{"mcp", "credential", "get"},
		{"mcp", "credential", "save"},
		{"mcp", "credential", "debug"},
		{"mcp", "credential", "bind"},
		{"mcp", "credential", "unbind"},
		{"mcp", "credential", "delete"},
		{"mcp", "member", "list"},
		{"mcp", "member", "add"},
		{"mcp", "member", "remove"},
		{"mcp", "hsf", "method-list"},
	} {
		cmd, _, err := dev.Find(path)
		if err != nil {
			t.Fatalf("Find(%v) error = %v", path, err)
		}
		if cmd == nil || cmd.Annotations["mcp-source"] != devMCPProduct || cmd.Annotations["mcp-tool"] == "" {
			t.Fatalf("Find(%v) missing MCP annotations: %#v", path, cmd)
		}
	}

	serviceCreate, _, err := dev.Find([]string{"mcp", "service", "create"})
	if err != nil {
		t.Fatalf("Find(service create) error = %v", err)
	}
	if !strings.Contains(serviceCreate.Example, "--server-name customer-info") {
		t.Fatalf("service create example must teach the semantic server name: %q", serviceCreate.Example)
	}
}

func TestDevMCPRenamedBackendTools(t *testing.T) {
	dev := devHandler{}.Command(&captureRunner{})
	for _, tc := range []struct {
		path []string
		tool string
	}{
		{path: []string{"mcp", "url", "get"}, tool: "mcp_server_url_get"},
		{path: []string{"mcp", "tool", "create"}, tool: "mcp_tool_create_http"},
		{path: []string{"mcp", "tool", "update"}, tool: "mcp_tool_update_http"},
	} {
		cmd, _, err := dev.Find(tc.path)
		if err != nil {
			t.Fatalf("Find(%v) error = %v", tc.path, err)
		}
		if got := cmd.Annotations["mcp-tool"]; got != tc.tool {
			t.Fatalf("Find(%v) mcp-tool = %q, want %q", tc.path, got, tc.tool)
		}
	}
}

func TestDevMCPCommandsBuildToolParams(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantTool   string
		wantParams map[string]any
	}{
		{
			name:     "service list",
			args:     []string{"dev", "mcp", "service", "list", "--keyword", "脚手架", "--cursor", "2", "--page-size", "20"},
			wantTool: devMCPServiceListTool,
			wantParams: map[string]any{
				"keyword":  "脚手架",
				"cursor":   2,
				"pageSize": 20,
			},
		},
		{
			name:     "url get",
			args:     []string{"dev", "mcp", "url", "get", "--mcp-id", "10487", "--source", "MARKET"},
			wantTool: devMCPServerURLGetTool,
			wantParams: map[string]any{
				"mcpId":  10487,
				"source": "MARKET",
			},
		},
		{
			name: "service create with server name",
			args: []string{
				"dev", "mcp", "service", "create",
				"--name", "客户信息服务",
				"--description", "查询客户信息",
				"--server-name", "crm-assets",
				"--dry-run",
			},
			wantTool: devMCPServiceCreateTool,
			wantParams: map[string]any{
				"name":        "客户信息服务",
				"description": "查询客户信息",
				"serverName":  "crm-assets",
			},
		},
		{
			name: "tool create parses json",
			args: []string{
				"dev", "mcp", "tool", "create",
				"--mcp-id", "10487",
				"--name", "get_weather",
				"--http-info", `{"method":"GET","url":"https://example.com","auth":{"type":"NO_AUTH"}}`,
				"--tool-inputs", `[{"key":"city","type":"string","description":"City name, for example: Hangzhou."}]`,
				"--api-outputs", `{"body":[{"key":"temperature","type":"number","description":"Temperature."}]}`,
				"--tool-outputs", `[]`,
				"--output-mappings", `[{"target":"$","type":"reference","source":"$.node_service_activator.Body"}]`,
				"--dry-run",
			},
			wantTool: devMCPToolCreateHTTPTool,
			wantParams: map[string]any{
				"mcpId": 10487,
				"name":  "get_weather",
				"httpInfo": map[string]any{
					"method": "GET",
					"url":    "https://example.com",
					"auth": map[string]any{
						"type": "NO_AUTH",
					},
				},
				"toolInputs": []any{
					map[string]any{"key": "city", "type": "string", "description": "City name, for example: Hangzhou."},
				},
				"apiOutputs": map[string]any{
					"body": []any{
						map[string]any{"key": "temperature", "type": "number", "description": "Temperature."},
					},
				},
				"toolOutputs": []any{},
				"outputMappings": []any{
					map[string]any{"target": "$", "type": "reference", "source": "$.node_service_activator.Body"},
				},
			},
		},
		{
			name: "tool update sends toolId",
			args: []string{
				"dev", "mcp", "tool", "update",
				"--mcp-id", "10487",
				"--tool-id", "G-ACT-1",
				"--name", "get_weather",
				"--http-info", `{"method":"GET","url":"https://example.com","auth":{"type":"NO_AUTH"}}`,
				"--api-outputs", `{"body":[{"key":"temperature","type":"number","description":"Temperature."}]}`,
				"--tool-outputs", `[]`,
				"--output-mappings", `[{"target":"$","type":"reference","source":"$.node_service_activator.Body"}]`,
				"--dry-run",
			},
			wantTool: devMCPToolUpdateHTTPTool,
			wantParams: map[string]any{
				"mcpId":  10487,
				"toolId": "G-ACT-1",
				"name":   "get_weather",
				"httpInfo": map[string]any{
					"method": "GET",
					"url":    "https://example.com",
					"auth": map[string]any{
						"type": "NO_AUTH",
					},
				},
				"apiOutputs": map[string]any{
					"body": []any{
						map[string]any{"key": "temperature", "type": "number", "description": "Temperature."},
					},
				},
				"toolOutputs": []any{},
				"outputMappings": []any{
					map[string]any{"target": "$", "type": "reference", "source": "$.node_service_activator.Body"},
				},
			},
		},
		{
			name: "tool debug with credential",
			args: []string{
				"dev", "mcp", "tool", "debug",
				"--mcp-id", "10487",
				"--tool-id", "G-ACT-1",
				"--value", `{"city":"杭州"}`,
				"--credential-id", "10518",
				"--dry-run",
			},
			wantTool: devMCPToolDebugTool,
			wantParams: map[string]any{
				"mcpId":        10487,
				"toolId":       "G-ACT-1",
				"value":        map[string]any{"city": "杭州"},
				"credentialId": 10518,
			},
		},
		{
			name: "tool debug no credential explicit",
			args: []string{
				"dev", "mcp", "tool", "debug",
				"--mcp-id", "10487",
				"--tool-id", "G-ACT-1",
				"--value", `{"city":"杭州"}`,
				"--no-credential",
				"--dry-run",
			},
			wantTool: devMCPToolDebugTool,
			wantParams: map[string]any{
				"mcpId":  10487,
				"toolId": "G-ACT-1",
				"value":  map[string]any{"city": "杭州"},
			},
		},
		{
			name: "tool create passes empty output mappings through",
			args: []string{
				"dev", "mcp", "tool", "create",
				"--mcp-id", "10487",
				"--name", "get_weather",
				"--http-info", `{"method":"GET","url":"https://example.com","auth":{"type":"NO_AUTH"}}`,
				"--api-outputs", `{"body":[{"key":"temperature","type":"number","description":"Temperature."}]}`,
				"--tool-outputs", `[]`,
				"--output-mappings", `[]`,
				"--dry-run",
			},
			wantTool: devMCPToolCreateHTTPTool,
			wantParams: map[string]any{
				"mcpId": 10487,
				"name":  "get_weather",
				"httpInfo": map[string]any{
					"method": "GET",
					"url":    "https://example.com",
					"auth": map[string]any{
						"type": "NO_AUTH",
					},
				},
				"apiOutputs": map[string]any{
					"body": []any{
						map[string]any{"key": "temperature", "type": "number", "description": "Temperature."},
					},
				},
				"toolOutputs":    []any{},
				"outputMappings": []any{},
			},
		},
		{
			name:     "tool versions",
			args:     []string{"dev", "mcp", "tool", "versions", "--mcp-id", "10487", "--tool-id", "G-ACT-1", "--page-size", "10"},
			wantTool: devMCPToolVersionsTool,
			wantParams: map[string]any{
				"mcpId":    10487,
				"toolId":   "G-ACT-1",
				"pageSize": 10,
			},
		},
		{
			name: "tool create hsf assembles params",
			args: []string{
				"dev", "mcp", "tool", "create-hsf",
				"--mcp-id", "10520",
				"--name", "search_mcp_services",
				"--title", "搜索MCP服务",
				"--description", "按名称关键词搜索当前用户有权限的 MCP 服务",
				"--hsf-info", `{"interfaceName":"com.dingtalk.open.connect.workbench.api.service.hsf.MCPHsfService","methodName":"searchMCPs","version":"1.0.0"}`,
				"--tool-inputs", `[{"key":"keyword","type":"string","description":"可选。按服务名关键词过滤。示例：验收"}]`,
				"--input-mappings", `[{"target":"$.SearchMCPRequest.name","type":"reference","source":"$.node_start.keyword"},{"target":"$.SearchMCPRequest.corpId","type":"reference","source":"$.system_node.ddDataCorpId"},{"target":"$.SearchMCPRequest.userId","type":"reference","source":"$.system_node.operateUserId"}]`,
				"--tool-outputs", `[]`,
				"--output-mappings", `[{"target":"$.success","type":"reference","source":"$.node_service_activator.success"}]`,
				"--timeout", "30",
				"--only-original-keys",
				"--dry-run",
			},
			wantTool: devMCPToolCreateHsfTool,
			wantParams: map[string]any{
				"mcpId":       10520,
				"name":        "search_mcp_services",
				"title":       "搜索MCP服务",
				"description": "按名称关键词搜索当前用户有权限的 MCP 服务",
				"hsfInfo": map[string]any{
					"interfaceName": "com.dingtalk.open.connect.workbench.api.service.hsf.MCPHsfService",
					"methodName":    "searchMCPs",
					"version":       "1.0.0",
				},
				"toolInputs": []any{
					map[string]any{"key": "keyword", "type": "string", "description": "可选。按服务名关键词过滤。示例：验收"},
				},
				"inputMappings": []any{
					map[string]any{"target": "$.SearchMCPRequest.name", "type": "reference", "source": "$.node_start.keyword"},
					map[string]any{"target": "$.SearchMCPRequest.corpId", "type": "reference", "source": "$.system_node.ddDataCorpId"},
					map[string]any{"target": "$.SearchMCPRequest.userId", "type": "reference", "source": "$.system_node.operateUserId"},
				},
				"toolOutputs": []any{},
				"outputMappings": []any{
					map[string]any{"target": "$.success", "type": "reference", "source": "$.node_service_activator.success"},
				},
				"timeout":          30,
				"onlyOriginalKeys": true,
			},
		},
		{
			name: "tool update hsf partial semantics",
			args: []string{
				"dev", "mcp", "tool", "update-hsf",
				"--mcp-id", "10520",
				"--tool-id", "G-ACT-1",
				"--description", "更准确的新描述",
				"--dry-run",
			},
			wantTool: devMCPToolUpdateHsfTool,
			wantParams: map[string]any{
				"mcpId":       10520,
				"toolId":      "G-ACT-1",
				"description": "更准确的新描述",
			},
		},
		{
			name:     "hsf method list",
			args:     []string{"dev", "mcp", "hsf", "method-list", "--interface-name", "com.dingtalk.open.connect.workbench.api.service.hsf.MCPHsfService"},
			wantTool: devMCPHsfMethodListTool,
			wantParams: map[string]any{
				"interfaceName": "com.dingtalk.open.connect.workbench.api.service.hsf.MCPHsfService",
			},
		},
		{
			name:     "credential unbind",
			args:     []string{"dev", "mcp", "credential", "unbind", "--mcp-id", "10520", "--dry-run"},
			wantTool: devMCPCredentialUnbindTool,
			wantParams: map[string]any{
				"mcpId": 10520,
			},
		},
		{
			name: "auth config save",
			args: []string{
				"dev", "mcp", "auth", "save",
				"--mcp-id", "10520",
				"--auth-type", "token",
				"--token-auth-config", `{"refreshToken":true,"fetchTokenRequest":{"method":"GET","url":"https://example.com/token"}}`,
				"--dry-run",
			},
			wantTool: devMCPAuthConfigSaveTool,
			wantParams: map[string]any{
				"mcpId":    10520,
				"authType": "TOKEN",
				"tokenAuthConfig": map[string]any{
					"refreshToken": true,
					"fetchTokenRequest": map[string]any{
						"method": "GET",
						"url":    "https://example.com/token",
					},
				},
			},
		},
		{
			name: "credential save",
			args: []string{
				"dev", "mcp", "credential", "save",
				"--mcp-id", "10520",
				"--name", "生产账号",
				"--content", `{"appKey":"key","appSecret":"secret"}`,
				"--yes",
			},
			wantTool: devMCPCredentialSaveTool,
			wantParams: map[string]any{
				"mcpId": 10520,
				"name":  "生产账号",
				"content": map[string]any{
					"appKey":    "key",
					"appSecret": "secret",
				},
			},
		},
		{
			name:     "credential list",
			args:     []string{"dev", "mcp", "credential", "list", "--mcp-id", "10520", "--cursor", "2", "--page-size", "20"},
			wantTool: devMCPCredentialListTool,
			wantParams: map[string]any{
				"mcpId":    10520,
				"cursor":   2,
				"pageSize": 20,
			},
		},
		{
			name:     "member add",
			args:     []string{"dev", "mcp", "member", "add", "--mcp-id", "10520", "--user-ids", "staff001, staff002", "--dry-run"},
			wantTool: devMCPMemberAddTool,
			wantParams: map[string]any{
				"mcpId":         10520,
				"memberUserIds": []string{"staff001", "staff002"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &captureRunner{}
			root := newDevAppTestRoot(runner)
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&out)
			root.SetArgs(tc.args)

			if err := root.Execute(); err != nil {
				t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
			}

			if got := runner.last.CanonicalProduct; got != devMCPProduct {
				t.Fatalf("CanonicalProduct = %q, want %q", got, devMCPProduct)
			}
			if got := runner.last.Tool; got != tc.wantTool {
				t.Fatalf("Tool = %q, want %q", got, tc.wantTool)
			}
			if !reflect.DeepEqual(runner.last.Params, tc.wantParams) {
				t.Fatalf("Params = %#v, want %#v", runner.last.Params, tc.wantParams)
			}
		})
	}
}

func TestConnectorMCPToolUpsertMetadataValidation(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name: "rejects non snake case tool name",
			args: []string{
				"dev", "mcp", "tool", "create",
				"--mcp-id", "10487",
				"--name", "GetWeather",
				"--http-info", `{"method":"GET","url":"https://example.com","auth":{"type":"NO_AUTH"}}`,
				"--dry-run",
			},
			wantErr: "--name 必须是 snake_case",
		},
		{
			name: "rejects zero timeout",
			args: []string{
				"dev", "mcp", "tool", "update-hsf",
				"--mcp-id", "10520",
				"--tool-id", "G-ACT-1",
				"--timeout", "0",
				"--dry-run",
			},
			wantErr: "--timeout 取值范围 1-180",
		},
		{
			name: "rejects hsf create without quartet",
			args: []string{
				"dev", "mcp", "tool", "create-hsf",
				"--mcp-id", "10520",
				"--name", "search_mcp_services",
				"--hsf-info", `{"interfaceName":"a.b.C","methodName":"m"}`,
				"--dry-run",
			},
			wantErr: "hsf 建造四件套",
		},
		{
			name: "rejects hsf update without changes",
			args: []string{
				"dev", "mcp", "tool", "update-hsf",
				"--mcp-id", "10520",
				"--tool-id", "G-ACT-1",
				"--dry-run",
			},
			wantErr: "至少提供一项待更新字段",
		},
		{
			name: "rejects invalid server name",
			args: []string{
				"dev", "mcp", "service", "create",
				"--name", "客户服务",
				"--description", "查询客户",
				"--server-name", "crm_assets",
				"--dry-run",
			},
			wantErr: "--server-name 必须是 kebab-case",
		},
		{
			name: "rejects exposed input without description",
			args: []string{
				"dev", "mcp", "tool", "create",
				"--mcp-id", "10487",
				"--name", "get_weather",
				"--http-info", `{"method":"GET","url":"https://example.com","auth":{"type":"NO_AUTH"}}`,
				"--tool-inputs", `[{"key":"city","type":"string"}]`,
				"--dry-run",
			},
			wantErr: "--tool-inputs[0].description 为必填",
		},
		{
			name: "rejects array without items child",
			args: []string{
				"dev", "mcp", "tool", "create",
				"--mcp-id", "10487",
				"--name", "get_weather",
				"--http-info", `{"method":"GET","url":"https://example.com","auth":{"type":"NO_AUTH"}}`,
				"--tool-inputs", `[{"key":"cities","type":"array","description":"City names, for example: Hangzhou."}]`,
				"--dry-run",
			},
			wantErr: "children 必须且只能包含一个 key=items",
		},
		{
			name: "rejects mapping without target",
			args: []string{
				"dev", "mcp", "tool", "create",
				"--mcp-id", "10487",
				"--name", "get_weather",
				"--http-info", `{"method":"GET","url":"https://example.com","auth":{"type":"NO_AUTH"}}`,
				"--input-mappings", `[{"type":"reference","source":"$.node_start.city"}]`,
				"--dry-run",
			},
			wantErr: "--input-mappings[0].target 为必填",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &captureRunner{}
			root := newDevAppTestRoot(runner)
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&out)
			root.SetArgs(tc.args)

			err := root.Execute()
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Execute() error = %v, want %q\noutput:\n%s", err, tc.wantErr, out.String())
			}
		})
	}
}

func TestConnectorMCPLegacyFlagRenameHints(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name: "tool debug requires credential choice",
			args: []string{
				"dev", "mcp", "tool", "debug",
				"--mcp-id", "10487",
				"--tool-id", "G-ACT-1",
				"--value", `{"city":"杭州"}`,
				"--dry-run",
			},
			wantErr: "调试需指定本次运行时凭证，二选一",
		},
		{
			name:    "tool get rejects action-id",
			args:    []string{"dev", "mcp", "tool", "get", "--mcp-id", "10487", "--action-id", "G-ACT-1"},
			wantErr: "--action-id 已更名为 --tool-id",
		},
		{
			name: "tool update rejects action-id",
			args: []string{
				"dev", "mcp", "tool", "update",
				"--mcp-id", "10487",
				"--action-id", "G-ACT-1",
				"--name", "get_weather",
				"--http-info", `{"method":"GET","url":"https://example.com","auth":{"type":"NO_AUTH"}}`,
				"--dry-run",
			},
			wantErr: "--action-id 已更名为 --tool-id",
		},
		{
			name: "tool create rejects legacy http flag",
			args: []string{
				"dev", "mcp", "tool", "create",
				"--mcp-id", "10487",
				"--name", "get_weather",
				"--http", `{"method":"GET","url":"https://example.com","auth":{"type":"NO_AUTH"}}`,
				"--dry-run",
			},
			wantErr: "--http 已更名为 --http-info",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &captureRunner{}
			root := newDevAppTestRoot(runner)
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&out)
			root.SetArgs(tc.args)

			err := root.Execute()
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Execute() error = %v, want %q\noutput:\n%s", err, tc.wantErr, out.String())
			}
		})
	}
}

func TestDevMCPExamplesDoNotBypassConfirmation(t *testing.T) {
	dev := devHandler{}.Command(&captureRunner{})
	mcp, _, err := dev.Find([]string{"mcp"})
	if err != nil {
		t.Fatalf("Find(mcp) error = %v", err)
	}

	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		if cmd.Runnable() && strings.Contains(cmd.Example, "--yes") {
			t.Errorf("%q example contains --yes confirmation bypass:\n%s", cmd.CommandPath(), cmd.Example)
		}
		for _, child := range cmd.Commands() {
			walk(child)
		}
	}
	walk(mcp)
}

func TestConnectorMCPWriteGuard(t *testing.T) {
	runner := &captureRunner{}
	root := newDevAppTestRoot(runner)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"dev", "mcp", "service", "create", "--name", "客户服务", "--description", "查询客户"})

	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("Execute() error = %v, want --yes guard\noutput:\n%s", err, out.String())
	}
}

func TestConnectorMCPInvalidJSON(t *testing.T) {
	runner := &captureRunner{}
	root := newDevAppTestRoot(runner)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{
		"dev", "mcp", "tool", "create",
		"--mcp-id", "10487",
		"--name", "get_weather",
		"--http-info", "[]",
		"--dry-run",
	})

	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "--http-info 必须是 JSON 对象") {
		t.Fatalf("Execute() error = %v, want JSON object validation\noutput:\n%s", err, out.String())
	}
}

func TestConnectorMCPCredentialDryRunRedactsContent(t *testing.T) {
	runner := &captureRunner{}
	root := newDevAppTestRoot(runner)
	root.SetArgs([]string{
		"dev", "mcp", "credential", "save",
		"--mcp-id", "10520",
		"--name", "生产账号",
		"--content", `{"appKey":"key","appSecret":"secret"}`,
		"--dry-run",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := map[string]any{"redacted": true}
	if got := runner.last.Params["content"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("content = %#v, want %#v", got, want)
	}
}

func TestConnectorMCPCredentialContentFromStdin(t *testing.T) {
	runner := &captureRunner{}
	root := newDevAppTestRoot(runner)
	root.SetIn(strings.NewReader(`{"username":"api-user","password":"secret"}`))
	root.SetArgs([]string{
		"dev", "mcp", "credential", "save",
		"--mcp-id", "10520",
		"--name", "生产账号",
		"--content-file", "-",
		"--yes",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := map[string]any{"username": "api-user", "password": "secret"}
	if got := runner.last.Params["content"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("content = %#v, want %#v", got, want)
	}
}
