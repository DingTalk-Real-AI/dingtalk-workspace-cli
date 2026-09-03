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

package helpers

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
)

const lintTestHTTPInfo = `{"method":"GET","url":"https://example.com","auth":{"type":"NO_AUTH"}}`

func lintTestCreateArgs(extra ...string) []string {
	args := []string{
		"dev", "mcp", "tool", "create",
		"--mcp-id", "10487",
		"--name", "get_weather",
		"--http-info", lintTestHTTPInfo,
		"--dry-run",
	}
	return append(args, extra...)
}

func TestDevMCPMappingReferenceLintRejects(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			// 0720 第六刀：出参三件套必填，裸草稿不再是合法中间态。
			name:    "bare draft rejected since output trio required",
			args:    lintTestCreateArgs(),
			wantErr: "出参三件套",
		},
		{
			// 0717 opencode 实测形态：整体透传 rule + 不传 apiOutputs，
			// 发布后 UI 标「变量已失效」。
			name: "passthrough without api outputs",
			args: lintTestCreateArgs(
				"--output-mappings", `[{"target":"$","type":"reference","source":"$.node_service_activator.Body"}]`,
			),
			wantErr: "整体透传也必须声明",
		},
		{
			name: "passthrough with empty api outputs body",
			args: lintTestCreateArgs(
				"--api-outputs", `{"body":[]}`,
				"--output-mappings", `[{"target":"$","type":"reference","source":"$.node_service_activator.Body"}]`,
			),
			wantErr: "为空——整体透传也必须按真实响应声明字段",
		},
		{
			name: "output source references undeclared field",
			args: lintTestCreateArgs(
				"--api-outputs", `{"body":[{"key":"errcode","type":"number"}]}`,
				"--output-mappings", `[{"target":"$","type":"reference","source":"$.node_service_activator.Body.result.dept_id_list"}]`,
			),
			wantErr: "缺少字段 \"result.dept_id_list\"",
		},
		{
			name: "field level target without tool outputs",
			args: lintTestCreateArgs(
				"--api-outputs", `{"body":[{"key":"errcode","type":"number"}]}`,
				"--output-mappings", `[{"target":"$.errcode","type":"reference","source":"$.node_service_activator.Body.errcode"}]`,
			),
			wantErr: "必须同批提供 --tool-outputs",
		},
		{
			name: "output source with unknown node",
			args: lintTestCreateArgs(
				"--api-outputs", `{"body":[{"key":"errcode","type":"number"}]}`,
				"--output-mappings", `[{"target":"$","type":"reference","source":"$.node_service.Body"}]`,
			),
			wantErr: "不是可解析的变量路径",
		},
		{
			name: "input source references undeclared tool input",
			args: lintTestCreateArgs(
				"--api-inputs", `{"query":[{"key":"name","type":"string"}]}`,
				"--tool-inputs", `[{"key":"city_name","type":"string","description":"城市名，如：杭州"}]`,
				"--input-mappings", `[{"target":"$.Query.name","type":"reference","source":"$.node_start.city"}]`,
			),
			wantErr: "缺少字段 \"city\"",
		},
		{
			name: "input source without tool inputs",
			args: lintTestCreateArgs(
				"--api-inputs", `{"query":[{"key":"name","type":"string"}]}`,
				"--input-mappings", `[{"target":"$.Query.name","type":"reference","source":"$.node_start.city_name"}]`,
			),
			wantErr: "必须同批提交",
		},
		{
			name: "input target lowercase position",
			args: lintTestCreateArgs(
				"--api-inputs", `{"query":[{"key":"name","type":"string"}]}`,
				"--tool-inputs", `[{"key":"city_name","type":"string","description":"城市名，如：杭州"}]`,
				"--input-mappings", `[{"target":"$.query.name","type":"reference","source":"$.node_start.city_name"}]`,
			),
			wantErr: "Pascal 位置名",
		},
		{
			name: "fixed target references undeclared api input",
			args: lintTestCreateArgs(
				"--api-inputs", `{"query":[{"key":"name","type":"string"}]}`,
				"--tool-inputs", `[{"key":"city_name","type":"string","description":"城市名，如：杭州"}]`,
				"--input-mappings", `[{"target":"$.Query.name","type":"reference","source":"$.node_start.city_name"},{"target":"$.Query.language","type":"fixed","source":"zh"}]`,
			),
			wantErr: "缺少字段 \"language\"",
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

func TestDevMCPMappingReferenceLintAccepts(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{
			name: "passthrough with declared api outputs",
			args: lintTestCreateArgs(
				"--api-outputs", `{"body":[{"key":"errcode","type":"number"},{"key":"result","type":"object","children":[{"key":"dept_id_list","type":"array","children":[{"key":"items","type":"number"}]}]}]}`,
				"--tool-outputs", `[]`,
				"--output-mappings", `[{"target":"$","type":"reference","source":"$.node_service_activator.Body"}]`,
			),
		},
		{
			name: "field level refine with array projection and system node",
			args: lintTestCreateArgs(
				"--api-outputs", `{"body":[{"key":"result","type":"object","children":[{"key":"list","type":"array","children":[{"key":"items","type":"object","children":[{"key":"staff_id","type":"string"}]}]}]}]}`,
				"--tool-outputs", `[{"key":"members","type":"array","description":"成员列表","children":[{"key":"items","type":"object","description":"成员","children":[{"key":"userId","type":"string","description":"用户ID"},{"key":"corpId","type":"string","description":"组织ID"}]}]}]`,
				"--output-mappings", `[{"target":"$.members[*].userId","type":"reference","source":"$.node_service_activator.Body.result.list[*].staff_id"},{"target":"$.members[*].corpId","type":"reference","source":"$.system_node.ddDataCorpId"}]`,
			),
		},
		{
			name: "input mappings with fixed constant declared in api inputs",
			args: lintTestCreateArgs(
				"--api-inputs", `{"query":[{"key":"name","type":"string"},{"key":"language","type":"string"}]}`,
				"--tool-inputs", `[{"key":"city_name","type":"string","description":"城市名，如：杭州"}]`,
				"--input-mappings", `[{"target":"$.Query.name","type":"reference","source":"$.node_start.city_name"},{"target":"$.Query.language","type":"fixed","source":"zh"}]`,
				"--api-outputs", `{"body":[{"key":"results","type":"array","children":[{"key":"items","type":"object"}]}]}`,
				"--tool-outputs", `[]`,
				"--output-mappings", `[{"target":"$","type":"reference","source":"$.node_service_activator.Body"}]`,
			),
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
		})
	}
}

func TestDevMCPLintStoredOutputMappings(t *testing.T) {
	passthroughConfig := `{"mappingType":"custom","modelVersionId":"","rules":"[{\"source\":\"$.node_service_activator.Body\",\"target\":\"$\",\"type\":\"reference\"}]","customSchemaMapping":"{}"}`

	// 0717 实测的坏形态：声明为空 + 透传 rule。
	if broken := devMCPLintStoredOutputMappings("{}", passthroughConfig); len(broken) != 1 {
		t.Fatalf("empty declaration want 1 broken rule, got %v", broken)
	}
	declared := `{"BODY":{"type":"object","properties":{"errcode":{"type":"number","properties":{}},"result":{"type":"object","properties":{"dept_id_list":{"type":"array","items":{"type":"number"}}}}}}}`
	if broken := devMCPLintStoredOutputMappings(declared, passthroughConfig); len(broken) != 0 {
		t.Fatalf("declared body want 0 broken rules, got %v", broken)
	}

	deepConfig := `{"rules":"[{\"source\":\"$.node_service_activator.Body.result.dept_id_list\",\"target\":\"$.dept_id_list\",\"type\":\"reference\"},{\"source\":\"$.system_node.ddDataCorpId\",\"target\":\"$.corpId\",\"type\":\"reference\"}]"}`
	if broken := devMCPLintStoredOutputMappings(declared, deepConfig); len(broken) != 0 {
		t.Fatalf("deep path want 0 broken rules, got %v", broken)
	}
	missingConfig := `{"rules":"[{\"source\":\"$.node_service_activator.Body.result.name_list\",\"target\":\"$.names\",\"type\":\"reference\"}]"}`
	if broken := devMCPLintStoredOutputMappings(declared, missingConfig); len(broken) != 1 {
		t.Fatalf("missing path want 1 broken rule, got %v", broken)
	}
	if broken := devMCPLintStoredOutputMappings(declared, ""); len(broken) != 0 {
		t.Fatalf("empty config want 0 broken rules, got %v", broken)
	}
}

type scriptedRunner struct {
	responses map[string]map[string]any
	calls     []executor.Invocation
}

func (r *scriptedRunner) Run(_ context.Context, invocation executor.Invocation) (executor.Result, error) {
	r.calls = append(r.calls, invocation)
	return executor.Result{Invocation: invocation, Response: r.responses[invocation.Tool]}, nil
}

func (r *scriptedRunner) called(tool string) bool {
	for _, call := range r.calls {
		if call.Tool == tool {
			return true
		}
	}
	return false
}

func TestConnectorMCPToolPublishPreflight(t *testing.T) {
	brokenDetail := map[string]any{
		"tool": map[string]any{
			"toolId":                  "G-ACT-1",
			"outputSchemaMappingJson": "{}",
			"outputMappingConfigJson": `{"mappingType":"custom","rules":"[{\"source\":\"$.node_service_activator.Body\",\"target\":\"$\",\"type\":\"reference\"}]"}`,
		},
		"success": true,
	}
	healthyDetail := map[string]any{
		"tool": map[string]any{
			"toolId":                  "G-ACT-1",
			"outputSchemaMappingJson": `{"BODY":{"type":"object","properties":{"errcode":{"type":"number"}}}}`,
			"outputMappingConfigJson": `{"mappingType":"custom","rules":"[{\"source\":\"$.node_service_activator.Body\",\"target\":\"$\",\"type\":\"reference\"}]"}`,
		},
		"success": true,
	}
	publishArgs := []string{
		"dev", "mcp", "tool", "publish",
		"--mcp-id", "10487",
		"--tool-id", "G-ACT-1",
		"--yes",
	}

	t.Run("blocks publish when stored sources are unresolvable", func(t *testing.T) {
		runner := &scriptedRunner{responses: map[string]map[string]any{
			devMCPToolGetTool: brokenDetail,
		}}
		root := newDevAppTestRoot(runner)
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs(publishArgs)

		err := root.Execute()
		if err == nil || !strings.Contains(err.Error(), "发布被拦截") {
			t.Fatalf("Execute() error = %v, want 发布被拦截\noutput:\n%s", err, out.String())
		}
		if runner.called(devMCPToolPublishTool) {
			t.Fatalf("publish should not run when preflight fails, calls: %#v", runner.calls)
		}
	})

	t.Run("publishes when stored sources resolve", func(t *testing.T) {
		runner := &scriptedRunner{responses: map[string]map[string]any{
			devMCPToolGetTool: healthyDetail,
		}}
		root := newDevAppTestRoot(runner)
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs(publishArgs)

		if err := root.Execute(); err != nil {
			t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
		}
		if !runner.called(devMCPToolPublishTool) {
			t.Fatalf("publish should run after preflight passes, calls: %#v", runner.calls)
		}
	})
}

func TestDevMCPResponseLacksURL(t *testing.T) {
	cases := []struct {
		name     string
		response map[string]any
		want     bool
	}{
		{"flat with url", map[string]any{"success": true, "mcpUrl": "https://pre-mcp-gw/server/x?key=y"}, false},
		{"flat empty success (DRAFT)", map[string]any{"success": true}, true},
		{"flat blank url", map[string]any{"success": true, "mcpUrl": "  "}, true},
		{"wrapped result envelope with url", map[string]any{"result": map[string]any{"mcpUrl": "https://pre-mcp-gw/server/x?key=y"}}, false},
		{"nil response", nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := devMCPResponseLacksURL(tc.response); got != tc.want {
				t.Fatalf("devMCPResponseLacksURL(%#v) = %v, want %v", tc.response, got, tc.want)
			}
		})
	}
}
