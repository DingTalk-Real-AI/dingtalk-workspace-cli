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
	"encoding/json"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
)

func runDevEventLintOnContent(t *testing.T, content string) (bool, []string) {
	t.Helper()
	path := writeDevEventFile(t, "event.json", content)
	var out bytes.Buffer
	cmd := newDevEventLintCommand(nil)
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs([]string{"--file", path})
	ctx, _ := output.WithResultStore(context.Background())
	cmd.SetContext(ctx)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("lint Execute() error = %v", err)
	}
	if _, _, err := output.EmitStoredResult(cmd); err != nil {
		t.Fatalf("emit stored result: %v", err)
	}
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Valid  bool     `json:"valid"`
			Issues []string `json:"issues"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("decode lint output: %v\n%s", err, out.String())
	}
	return envelope.Data.Valid, envelope.Data.Issues
}

func lintIssueText(issues []string) string {
	return strings.Join(issues, "\n")
}

func TestCrossPlatformCoverageDevEventLintValidDefinitionPasses(t *testing.T) {
	valid, issues := runDevEventLintOnContent(t, devEventTestDefinition)
	if !valid || len(issues) != 0 {
		t.Fatalf("valid definition => valid=%v issues=%s", valid, lintIssueText(issues))
	}
}

func TestCrossPlatformCoverageDevEventLintStructureRules(t *testing.T) {
	cases := []struct {
		name     string
		schema   string
		wantText []string
	}{
		{
			name:     "unsupported keyword",
			schema:   `{"type":"object","properties":{"a":{"type":"string","minLength":2}}}`,
			wantText: []string{`payloadSchema.properties.a: 不支持的关键字 "minLength"`},
		},
		{
			name:     "format invalid value",
			schema:   `{"type":"object","properties":{"a":{"type":"string","format":"date-time"}}}`,
			wantText: []string{`payloadSchema.properties.a: format 取值非法 "date-time"（受控枚举）`},
		},
		{
			name:     "format non-string value",
			schema:   `{"type":"object","properties":{"a":{"type":"string","format":false,"description":"员工唯一标识"}}}`,
			wantText: []string{`payloadSchema.properties.a: format 取值非法 false（受控枚举）`},
		},
		{
			name:     "format on non-string field",
			schema:   `{"type":"object","properties":{"a":{"type":"integer","format":"timestamp_ms","description":"毫秒时间戳"}}}`,
			wantText: []string{`payloadSchema.properties.a: format 仅允许声明在 type=string 的字段上`},
		},
		{
			name:     "bad type value",
			schema:   `{"type":"object","properties":{"a":{"type":"int"}}}`,
			wantText: []string{`payloadSchema.properties.a: type 取值非法 "int"`},
		},
		{
			name:     "missing type",
			schema:   `{"type":"object","properties":{"a":{"description":"no type"}}}`,
			wantText: []string{`payloadSchema.properties.a: 缺少必填关键字 "type"`},
		},
		{
			name:     "missing field description",
			schema:   `{"type":"object","properties":{"a":{"type":"string"}}}`,
			wantText: []string{`payloadSchema.properties.a: 字段必须提供非空字符串 description`},
		},
		{
			name:     "blank field description",
			schema:   `{"type":"object","properties":{"a":{"type":"string","description":"  "}}}`,
			wantText: []string{`payloadSchema.properties.a: 字段 description 必须是非空字符串`},
		},
		{
			name:     "non-string field description",
			schema:   `{"type":"object","properties":{"a":{"type":"string","description":false}}}`,
			wantText: []string{`payloadSchema.properties.a: 字段 description 必须是非空字符串`},
		},
		{
			name:     "placeholder field description",
			schema:   `{"type":"object","properties":{"a":{"type":"string","description":"名称"},"employeeId":{"type":"string","description":"employeeId"}}}`,
			wantText: []string{`payloadSchema.properties.a: 字段 description 不能是字段名或占位文本`, `payloadSchema.properties.employeeId: 字段 description 不能是字段名或占位文本`},
		},
		{
			name:     "dangling required",
			schema:   `{"type":"object","properties":{"a":{"type":"string"}},"required":["ghost"]}`,
			wantText: []string{`payloadSchema.required: 字段 "ghost" 未在 properties 中声明`},
		},
		{
			name:     "tuple items rejected",
			schema:   `{"type":"object","properties":{"a":{"type":"array","items":[{"type":"string"}]}}}`,
			wantText: []string{`payloadSchema.properties.a.items: 必须是对象（不支持元组形式）`},
		},
		{
			name:     "properties on non object",
			schema:   `{"type":"object","properties":{"a":{"type":"string","properties":{"b":{"type":"string"}}}}}`,
			wantText: []string{`payloadSchema.properties.a: 非 object 类型不允许声明 "properties"`},
		},
		{
			name:     "object missing properties",
			schema:   `{"type":"object","properties":{"a":{"type":"object"}}}`,
			wantText: []string{`payloadSchema.properties.a: 根/对象节点缺少 "properties"`},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			content := `{"eventKey":"app_user_hr_employee_joined","name":"员工入职","payloadSchema":` + tc.schema + `}`
			valid, issues := runDevEventLintOnContent(t, content)
			if valid {
				t.Fatalf("expected invalid, issues=%s", lintIssueText(issues))
			}
			for _, want := range tc.wantText {
				if !strings.Contains(lintIssueText(issues), want) {
					t.Fatalf("issues missing %q:\n%s", want, lintIssueText(issues))
				}
			}
		})
	}
}

func TestCrossPlatformCoverageDevEventLintRequiresNestedFieldDescriptions(t *testing.T) {
	content := `{"eventKey":"app_user_hr_employee_joined","name":"员工入职","payloadSchema":` +
		`{"type":"object","properties":{"profile":{"type":"object","description":"员工资料","properties":{"employeeId":{"type":"string"}}},` +
		`"items":{"type":"array","description":"员工条目列表","items":{"type":"object","properties":{"value":{"type":"string"}}}}}}}`
	valid, issues := runDevEventLintOnContent(t, content)
	if valid {
		t.Fatalf("expected invalid, issues=%s", lintIssueText(issues))
	}
	joined := lintIssueText(issues)
	for _, want := range []string{
		"payloadSchema.properties.profile.properties.employeeId: 字段必须提供非空字符串 description",
		"payloadSchema.properties.items.items.properties.value: 字段必须提供非空字符串 description",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("issues missing %q:\n%s", want, joined)
		}
	}
}

func TestCrossPlatformCoverageDevEventLintFileLevelIssues(t *testing.T) {
	// 受控键 + 命名规则 + 缺 name 一次全部暴露（收集而非快速失败）
	content := `{"eventKey":"app_user_bad","scope":"dws_x","payloadSchema":{"type":"object","properties":{"a":{"type":"string"}}}}`
	valid, issues := runDevEventLintOnContent(t, content)
	if valid {
		t.Fatalf("expected invalid")
	}
	joined := lintIssueText(issues)
	for _, want := range []string{
		"含不接受开发者自行指定的键：scope",
		"缺少必填键 \"name\"",
		"eventKey \"app_user_bad\" 不符合命名规则",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("issues missing %q:\n%s", want, joined)
		}
	}
}

func TestCrossPlatformCoverageDevEventLintSampleConformance(t *testing.T) {
	// 类型不符
	content := `{"eventKey":"app_user_hr_employee_joined","name":"员工入职",` +
		`"payloadSchema":{"type":"object","properties":{"amount":{"type":"number"}},"required":["amount"]},` +
		`"samplePayload":{"amount":"12.34"}}`
	valid, issues := runDevEventLintOnContent(t, content)
	if valid || !strings.Contains(lintIssueText(issues), "payloadSchema 与 samplePayload 不符") {
		t.Fatalf("type mismatch => valid=%v issues=%s", valid, lintIssueText(issues))
	}

	// additionalProperties:false 下样例多字段
	content = `{"eventKey":"app_user_hr_employee_joined","name":"员工入职",` +
		`"payloadSchema":{"type":"object","properties":{"a":{"type":"string"}},"additionalProperties":false},` +
		`"samplePayload":{"a":"x","b":"y"}}`
	valid, issues = runDevEventLintOnContent(t, content)
	if valid || !strings.Contains(lintIssueText(issues), "payloadSchema 与 samplePayload 不符") {
		t.Fatalf("extra field => valid=%v issues=%s", valid, lintIssueText(issues))
	}

	// enum + 嵌套对象样例通过
	content = `{"eventKey":"app_user_hr_employee_joined","name":"员工入职",` +
		`"payloadSchema":{"type":"object","properties":{"level":{"type":"string","description":"事件级别","enum":["low","high"]},` +
		`"detail":{"type":"object","description":"事件详情","properties":{"memo":{"type":"string","description":"补充说明"}},"required":["memo"]}},` +
		`"required":["level"]},"samplePayload":{"level":"high","detail":{"memo":"ok"}}}`
	valid, issues = runDevEventLintOnContent(t, content)
	if !valid || len(issues) != 0 {
		t.Fatalf("valid sample => valid=%v issues=%s", valid, lintIssueText(issues))
	}

	// 样例缺 required
	content = `{"eventKey":"app_user_hr_employee_joined","name":"员工入职",` +
		`"payloadSchema":{"type":"object","properties":{"level":{"type":"string"}},"required":["level"]},` +
		`"samplePayload":{}}`
	valid, issues = runDevEventLintOnContent(t, content)
	if valid || !strings.Contains(lintIssueText(issues), "payloadSchema 与 samplePayload 不符") {
		t.Fatalf("missing required => valid=%v issues=%s", valid, lintIssueText(issues))
	}
}

func TestCrossPlatformCoverageDevEventLintDepthLimit(t *testing.T) {
	// 构造 18 层嵌套对象，第 17 层触发深度上限
	inner := `{"type":"object","properties":{"leaf":{"type":"string"}}}`
	schema := inner
	for i := 0; i < 17; i++ {
		schema = `{"type":"object","properties":{"n":` + schema + `}}`
	}
	content := `{"eventKey":"app_user_hr_employee_joined","name":"员工入职","payloadSchema":` + schema + `}`
	valid, issues := runDevEventLintOnContent(t, content)
	if valid || !strings.Contains(lintIssueText(issues), "嵌套深度超出上限 16") {
		t.Fatalf("depth => valid=%v issues=%s", valid, lintIssueText(issues))
	}
}

func TestCrossPlatformCoverageDevEventLintMalformedStructures(t *testing.T) {
	cases := []struct {
		name    string
		def     string
		wantSub string
	}{
		{"root type", `{"eventKey":"app_user_demo","name":"n","description":"d","payloadSchema":{"type":"string"},"samplePayload":{}}`, "根类型必须是 object"},
		{"properties not object", `{"eventKey":"app_user_demo","name":"n","description":"d","payloadSchema":{"type":"object","properties":"x"},"samplePayload":{}}`, "properties: 必须是对象"},
		{"property not object", `{"eventKey":"app_user_demo","name":"n","description":"d","payloadSchema":{"type":"object","properties":{"a":"str"}},"samplePayload":{}}`, "字段定义必须是对象"},
		{"items on non-array", `{"eventKey":"app_user_demo","name":"n","description":"d","payloadSchema":{"type":"object","items":{"type":"string"}},"samplePayload":{}}`, "非 array 类型不允许声明"},
		{"required not array", `{"eventKey":"app_user_demo","name":"n","description":"d","payloadSchema":{"type":"object","required":"a"},"samplePayload":{}}`, "required: 必须是字符串数组"},
		{"required non-string", `{"eventKey":"app_user_demo","name":"n","description":"d","payloadSchema":{"type":"object","required":[1]},"samplePayload":{}}`, "required: 必须是字符串数组"},
		{"schema compile fails", `{"eventKey":"app_user_demo","name":"n","description":"d","payloadSchema":{"type":5},"samplePayload":{}}`, "无法编译"},
		{"dangling ref fails compile", `{"eventKey":"app_user_demo","name":"n","description":"d","payloadSchema":{"$ref":"dev-event-lint://missing.json#"},"samplePayload":{}}`, "无法编译"},
		{"payloadSchema missing", `{"eventKey":"app_user_demo","name":"n","description":"d","samplePayload":{}}`, "缺少必填键 \"payloadSchema\""},
		{"payloadSchema null", `{"eventKey":"app_user_demo","name":"n","description":"d","payloadSchema":null,"samplePayload":{}}`, "payloadSchema 是 null"},
		{"payloadSchema not object", `{"eventKey":"app_user_demo","name":"n","description":"d","payloadSchema":"str","samplePayload":{}}`, "payloadSchema 必须是 JSON 对象"},
		{"samplePayload null", `{"eventKey":"app_user_demo","name":"n","description":"d","payloadSchema":{"type":"object"},"samplePayload":null}`, "samplePayload 是 null"},
		{"samplePayload not object", `{"eventKey":"app_user_demo","name":"n","description":"d","payloadSchema":{"type":"object"},"samplePayload":"str"}`, "samplePayload 必须是 JSON 对象"},
		{"eventKey too long", `{"eventKey":"app_user_` + strings.Repeat("x", 130) + `","name":"n","description":"d","payloadSchema":{"type":"object"},"samplePayload":{}}`, "eventKey 超过"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			valid, issues := runDevEventLintOnContent(t, tc.def)
			if valid {
				t.Fatalf("expected invalid, issues = %v", issues)
			}
			if text := lintIssueText(issues); !strings.Contains(text, tc.wantSub) {
				t.Fatalf("issues = %q, want containing %q", text, tc.wantSub)
			}
		})
	}
}

func TestCrossPlatformCoverageDevEventLintRequiresFileFlag(t *testing.T) {
	cmd := newDevEventLintCommand(nil)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs([]string{})
	ctx, _ := output.WithResultStore(context.Background())
	cmd.SetContext(ctx)
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--file 为必填") {
		t.Fatalf("lint without --file error = %v", err)
	}
}

func TestCrossPlatformCoverageRunDevEventLintRequiresPathDirect(t *testing.T) {
	cmd := newDevEventLintCommand(nil)
	ctx, _ := output.WithResultStore(context.Background())
	cmd.SetContext(ctx)
	if err := runDevEventLint(cmd, map[string]any{}); err == nil || !strings.Contains(err.Error(), "--file 为必填") {
		t.Fatalf("runDevEventLint without path error = %v", err)
	}
}
