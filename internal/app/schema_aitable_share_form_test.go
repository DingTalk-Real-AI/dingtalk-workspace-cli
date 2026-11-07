// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
)

const (
	aitableShareUsageAnswerRule    = "用户仅询问用法时，最终回答必须先给出完整命令；缺少必填 ID 时则给出带明确占位符的完整命令模板，禁止猜测。"
	aitableShareUsageDiscoveryGate = "发现门禁：即使 Skill 或参考文档已提供完整示例，回答前也必须实际执行一次且仅执行一次目标 leaf 的安全 help/schema 查询；不得仅依据 Skill 或参考文档直接作答。"
	aitableShareShortcutSpelling   = "Shortcut 名称开头的 `+` 是命令名不可省略的一部分；不得改写、试探其他拼法或改用 `--help`/`-h`。"
	aitableSharePlaceholderRule    = "缺少的值必须保留为 `<BASE_ID>`、`<TABLE_ID>`、`<VIEW_ID>` 等明确占位符。"
	aitableShareUsageEvidence      = "请将 <BASE_ID>、<TABLE_ID>、<VIEW_ID> 替换为真实值；未传入的分享配置保持原值。本次仅查询 help/schema，未执行写操作。"
	aitableShareUsageTemplate      = "dws aitable form share update --base-id <BASE_ID> --table-id <TABLE_ID> --view-id <VIEW_ID> --enabled true"
	aitableShareNoCompensationRule = "不自行调用第二个 View 更新命令补偿 CP"
)

var aitableShareFinalStateFields = []string{"shareFormUuid", "status", "formCover", "cpSynced"}

func aitableShareSchemaObject(t testing.TB, value any, label string) map[string]any {
	t.Helper()
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("%s=%#v, want JSON object", label, value)
	}
	return object
}

func assertAITableShareFinalStateFields(t testing.TB, body, label string) {
	t.Helper()
	if !strings.Contains(body, "诊断不能新增写入") {
		t.Errorf("%s must not suggest repeating form share update to diagnose malformed success", label)
	}
	if !strings.Contains(body, "get 不返回 cpSynced") && !strings.Contains(body, "get 只诊断分享配置，不返回 `cpSynced`") {
		t.Errorf("%s must not suggest verifying CP via get", label)
	}
	for _, field := range aitableShareFinalStateFields {
		if !strings.Contains(body, field) {
			t.Errorf("%s missing final-state field %s", label, field)
		}
	}
}

func TestCrossPlatformCoverageAITableShareFormResultContracts(t *testing.T) {
	tests := []struct {
		paths    []string
		outcomes []any
		required []any
		fields   []string
	}{
		{
			paths:    []string{"aitable form share get", "aitable +form-share-get"},
			outcomes: []any{"success", "failure"},
			required: []any{"baseId", "tableId", "viewId", "enabled", "status"},
			fields:   []string{"enabled", "status", "shareFormUuid", "formCover"},
		},
		{
			paths:    []string{"aitable form share update", "aitable +form-share-update"},
			outcomes: []any{"success", "partial_failure", "failure"},
			required: []any{"baseId", "tableId", "viewId", "enabled", "status", "cpSynced", "verified"},
			fields:   []string{"enabled", "status", "shareFormUuid", "formCover", "cpSynced", "verified", "unverified"},
		},
	}
	for _, tc := range tests {
		for _, path := range tc.paths {
			t.Run(path, func(t *testing.T) {
				full := executeShortcutSchemaQuery(t, "--cli-path", path)
				compact := executeShortcutSchemaQuery(t, "--cli-path", path, "--compact")
				if !reflect.DeepEqual(full["result"], compact["result"]) {
					t.Fatal("compact result differs from full result")
				}
				result := aitableShareSchemaObject(t, full["result"], "result")
				if !schemaContractJSONEqual(result["outcomes"], tc.outcomes) {
					t.Fatalf("outcomes=%#v", result["outcomes"])
				}
				dataSchema := aitableShareSchemaObject(t, result["data_schema"], "result.data_schema")
				variants := dataSchema["oneOf"].([]any)
				wantVariants := 2 // server state and non-executed preview
				if strings.Contains(path, "update") {
					wantVariants++ // remote response with unverified terminal state
				}
				if len(variants) != wantVariants {
					t.Fatalf("missing result alternatives: %#v", dataSchema)
				}
				dataSchema = aitableShareSchemaObject(t, variants[0], "server state")
				if strings.Contains(path, "update") {
					if !strings.Contains(dataSchema["description"].(string), "baseId/tableId/viewId 必须逐项与本次请求完全一致") {
						t.Fatal("success schema must bind terminal state to the requested target")
					}
					if !strings.Contains(dataSchema["description"].(string), "本次显式请求的 enabled/formName/formDesc 必须与响应回读值一致") {
						t.Fatal("success schema must bind terminal state to the requested values")
					}
					if !strings.Contains(dataSchema["description"].(string), "unverified") {
						t.Fatal("success schema must explain requested fields the response never echoes")
					}
					partial := aitableShareSchemaObject(t, variants[1], "partial")
					if !strings.Contains(partial["description"].(string), "响应 baseId/tableId/viewId 与本次请求不一致") {
						t.Fatal("partial schema must explain mismatched response targets")
					}
					if !strings.Contains(partial["description"].(string), "本次显式请求的 enabled/formName/formDesc 与响应回读值不一致") {
						t.Fatal("partial schema must explain mismatched requested values")
					}
					if !strings.Contains(partial["description"].(string), "execution_started=true") {
						t.Fatal("partial schema must explain remote execution")
					}
					if dataSchema["properties"].(map[string]any)["cpSynced"].(map[string]any)["const"] != true {
						t.Fatal("success requires cpSynced=true")
					}
				}
				if !schemaContractJSONEqual(dataSchema["required"], tc.required) {
					t.Fatalf("required=%#v", dataSchema["required"])
				}
				properties := aitableShareSchemaObject(t, dataSchema["properties"], "result.data_schema.properties")
				fields := append(append([]string{}, tc.fields...), "shareFormUrl", "authTypeCode", "authData", "submitTimesLimit", "submitTimesUserLimit", "formStartTime", "formEndTime", "anonymousSubmit", "loadLastSubmit", "submitRecordViewMode", "replyNotice", "shareUidList", "warnings")
				for _, field := range fields {
					property := aitableShareSchemaObject(t, properties[field], "result field "+field)
					if schemaContractString(property["description"]) == "" {
						t.Errorf("missing described result field %s", field)
					}
				}
				cover, exists := schemaContractMap(full["parameters"])["form-cover"]
				if strings.Contains(path, "update") {
					if !exists {
						t.Fatal("update must expose the optional form-cover input")
					}
					param := aitableShareSchemaObject(t, cover, "form-cover")
					if param["required"] == true || param["cli_required"] == true {
						t.Fatal("form-cover must not become a required DWS input")
					}
				} else if exists {
					t.Fatal("get must not expose a form-cover input")
				}
			})
		}
	}
}

func TestCrossPlatformCoverageAITableShareFormUpdateConstraints(t *testing.T) {
	want := map[string][][]string{"require_one_of": {{
		"enabled", "auth-type-code", "auth-data", "submit-times-limit", "submit-times-user-limit",
		"form-start-time", "form-end-time", "form-name", "form-desc", "anonymous-submit",
		"form-start-at", "form-end-at", "load-last-submit", "reply-notice", "share-uid-list",
		"submit-record-view-mode", "form-cover", "form-cover-file", "form-cover-resource-id",
	}}, "mutually_exclusive": {{"form-cover", "form-cover-file", "form-cover-resource-id"},
		{"form-start-time", "form-start-at"}, {"form-end-time", "form-end-at"}}}
	for _, path := range []string{"aitable form share update", "aitable +form-share-update"} {
		for _, compact := range []bool{false, true} {
			args := []string{"--cli-path", path}
			if compact {
				args = append(args, "--compact")
			}
			tool := executeShortcutSchemaQuery(t, args...)
			{
				file := tool["parameters"].(map[string]any)["form-cover-file"].(map[string]any)
				if _, present := file["property"]; present {
					t.Fatalf("local cover file must not publish a direct RPC property: %#v", file)
				}
				if !compact {
					provenance := file["field_provenance"].(map[string]any)["property"].(map[string]any)
					if provenance["source"] != "reviewed_mapping_exclusion" {
						t.Fatalf("local cover file mapping exclusion was not delivered: %#v", provenance)
					}
				}
			}
			for name, description := range map[string]string{
				"form-cover":             helpers.FormCoverURLDescription,
				"form-cover-resource-id": helpers.FormCoverResourceDescription,
			} {
				param := tool["parameters"].(map[string]any)[name].(map[string]any)
				if param["description"] != description {
					t.Fatalf("%s compact=%v %s description=%#v", path, compact, name, param["description"])
				}
			}
			if !schemaContractJSONEqual(tool["constraints"], want) {
				t.Fatalf("%s compact=%v constraints=%#v", path, compact, tool["constraints"])
			}
			enabled := tool["parameters"].(map[string]any)["enabled"].(map[string]any)
			if enabled["type"] != "string" || enabled["required"] != false {
				t.Fatalf("%s compact=%v enabled=%#v", path, compact, enabled)
			}
			if _, published := enabled["interface_type"]; published {
				t.Fatalf("%s compact=%v must preserve the historical omitted enabled interface_type", path, compact)
			}
		}
	}
}

func TestCrossPlatformCoverageAITableShareFormNotifyIdentity(t *testing.T) {
	for _, compact := range []bool{false, true} {
		args := []string{"--cli-path", "aitable form share notify"}
		if compact {
			args = append(args, "--compact")
		}
		tool := executeShortcutSchemaQuery(t, args...)
		parameter := aitableShareSchemaObject(t, tool["parameters"], "parameters")["recipients"].(map[string]any)
		for label, description := range map[string]string{"command": tool["description"].(string), "recipients": parameter["description"].(string)} {
			if !strings.Contains(description, "当前组织") || !strings.Contains(description, "userId") || !strings.Contains(description, "1-100") {
				t.Fatalf("compact=%t %s obscures recipient identity: %s", compact, label, description)
			}
		}
		for _, example := range tool["examples"].([]any) {
			if !strings.Contains(example.(string), "USER_ID") || strings.Contains(example.(string), "INTERNAL_UID") {
				t.Fatalf("notify example uses the wrong identity: %s", example)
			}
		}
	}
}

func TestCrossPlatformCoverageAITableShareFormUsageAnswerContract(t *testing.T) {
	root := NewRootCommand()
	paths := []string{"aitable form share update", "aitable +form-share-update"}
	for _, path := range paths {
		t.Run(path+" help", func(t *testing.T) {
			leaf, _, err := root.Find(strings.Fields(path))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(leaf.Long, aitableShareUsageAnswerRule) {
				t.Fatalf("%s help missing usage-only answer contract:\n%s", path, leaf.Long)
			}
			if !strings.Contains(leaf.Long, aitableShareUsageDiscoveryGate) {
				t.Fatalf("%s help missing mandatory discovery gate:\n%s", path, leaf.Long)
			}
			assertAITableShareFinalStateFields(t, leaf.Long, path+" help")
			if !strings.Contains(leaf.Long, aitableShareNoCompensationRule) {
				t.Fatalf("%s help missing no-compensation boundary:\n%s", path, leaf.Long)
			}
		})

		t.Run(path+" compact schema", func(t *testing.T) {
			tool := executeShortcutSchemaQuery(t, "--cli-path", path, "--compact")
			description, _ := tool["description"].(string)
			if !strings.Contains(description, aitableShareUsageAnswerRule) {
				t.Fatalf("%s compact description missing usage-only answer contract:\n%s", path, description)
			}
			if !strings.Contains(description, aitableShareUsageDiscoveryGate) {
				t.Fatalf("%s compact description missing mandatory discovery gate:\n%s", path, description)
			}
			assertAITableShareFinalStateFields(t, description, path+" compact description")
			if !strings.Contains(description, aitableShareNoCompensationRule) {
				t.Fatalf("%s compact description missing no-compensation boundary:\n%s", path, description)
			}
		})
	}

	for _, path := range []string{
		"../../skills/multi/dingtalk-aitable/SKILL.md",
		"../../skills/multi/dingtalk-aitable/references/aitable/aitable-form.md",
		"../../skills/mono/references/products/aitable/aitable-form.md",
	} {
		t.Run(path, func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			// Git may check Markdown out with CRLF on Windows.
			body := strings.ReplaceAll(string(raw), "\r\n", "\n")
			if strings.HasSuffix(path, "/SKILL.md") {
				frontmatter, markdown, found := strings.Cut(strings.TrimPrefix(body, "---\n"), "\n---\n")
				if !strings.HasPrefix(body, "---\n") || !found {
					t.Fatal("Skill must separate discovery metadata from execution guidance")
				}
				var description string
				for _, line := range strings.Split(frontmatter, "\n") {
					if value, ok := strings.CutPrefix(line, "description:"); ok {
						description = strings.TrimSpace(value)
					}
				}
				for _, rule := range []string{"CLI 契约评审", "只评审 aitable 合成 JSON 回执也必须加载本 Skill", "“不要执行线上业务”不等于免除本机离线契约核对", "不能按通用 JSON 经验直接作答", "不是仓库源码审查", "不从源码搜索开始", "先加载正文确定入口与规则", "不能仅凭摘要执行", "以用户原文指定的核对入口为准", "Skill 参数不得扩大范围"} {
					if !strings.Contains(description, rule) {
						t.Errorf("Skill discovery metadata missing review routing boundary %q", rule)
					}
				}
				// Concrete invocations belong in the loaded body: an isolated
				// Shortcut example in discovery metadata can misroute atomic reviews.
				if strings.Contains(description, "dws ") || strings.Contains(description, "--cli-path") {
					t.Error("Skill discovery metadata must not substitute command examples for loading the body")
				}
				priority, _, found := strings.Cut(markdown, "<!-- DWS_RUNTIME_CONTRACT_START -->")
				if !found {
					t.Fatal("Skill must retain the shared runtime contract after review routing")
				}
				for _, rule := range []string{
					"Agent 自行摘要或传入 Skill 的参数不是新增请求",
					"用法询问与返回值评审不能共用发现路径",
					"背景里提到的原创建命令不是额外发现目标",
					"先取得一次该入口的 compact Schema",
					"用户明确要求比较多个命令时，才分别查询各自契约",
					"Help/Schema 是本机离线查询",
					"用户明确禁止任何命令时则不查询",
					"先用简短结论回答用户的全部安全问题",
					"不能把关键禁止动作留到分节解释或末尾总结",
					`dws schema --cli-path "aitable <原入口>" --compact --format json`,
					"保留 `+`，不改查 Help，也不切换原子/Shortcut",
					"`dws schema --cli-path \"aitable form share update\" --compact --format json`",
				} {
					if !strings.Contains(priority, rule) {
						t.Errorf("Skill must publish review routing %q before generic runtime navigation", rule)
					}
				}
			}
			for _, entry := range []string{"form share get", "form share update", "+form-share-get", "+form-share-update"} {
				route := "| `" + entry + "` | `dws schema --cli-path \"aitable " + entry + "\" --compact --format json` |"
				if !strings.Contains(body, route) {
					t.Errorf("%s must preserve exact result-review entry mapping %q", path, entry)
				}
			}
			for _, rule := range []string{"## 返回值评审专用查询", "data.executed=false", "字段是否必填只读所选分支的 `required`", "不能把 Schema 中的字段补写成样本已有事实"} {
				if !strings.Contains(body, rule) {
					t.Errorf("%s missing result branch interpretation boundary %q", path, rule)
				}
			}
			for _, rule := range []string{"用法询问与返回值评审不能共用发现路径", "返回值评审专用查询（不适用于命令用法询问）", "不证明外部用户必定无法访问", "不把 0/1 自行翻译成未发布/已发布", "仅加载 Skill 或看到合成样本不满足此条件", "两种入口都必须完整保留用户指定的配置值", "Help/Schema 是离线契约查询，不调用线上业务", "只有用户明确禁止任何命令或本机查询时才不执行"} {
				if !strings.Contains(body, rule) {
					t.Errorf("%s missing discovery/interpretation boundary %q", path, rule)
				}
			}
			for _, rule := range []string{"所有 ID 已知时", "--format json", "不得用搜索源码或 reference 代替本机契约查询", "data.succeeded[0].response", "data.failed[0].error", "get 的成功不证明 CP 同步", "原子/Shortcut 入口必须原样保留", "不能建议“通过 get 回读 cpSynced 后确认闭环”"} {
				if !strings.Contains(body, rule) {
					t.Errorf("%s missing evaluated form-share rule %q", path, rule)
				}
			}
			if !strings.Contains(body, aitableShareUsageAnswerRule) {
				t.Fatalf("%s missing usage-only answer contract", path)
			}
			if !strings.Contains(body, aitableShareUsageDiscoveryGate) {
				t.Fatalf("%s missing mandatory discovery gate", path)
			}
			if !strings.Contains(body, aitableShareShortcutSpelling) {
				t.Fatalf("%s missing shortcut spelling gate", path)
			}
			if !strings.Contains(body, aitableSharePlaceholderRule) {
				t.Fatalf("%s missing no-guess placeholder rule", path)
			}
			assertAITableShareFinalStateFields(t, body, path)
			if !strings.Contains(body, aitableShareNoCompensationRule) {
				t.Fatalf("%s missing no-compensation boundary", path)
			}
			for _, discoveryCommand := range []string{
				`dws schema --cli-path "aitable form share update" --compact --format json`,
				`dws schema --cli-path "aitable +form-share-update" --compact --format json`,
			} {
				if !strings.Contains(body, discoveryCommand) {
					t.Fatalf("%s missing mandatory discovery command %q", path, discoveryCommand)
				}
			}
			commandIndex := strings.Index(body, aitableShareUsageTemplate)
			evidenceIndex := strings.Index(body, aitableShareUsageEvidence)
			if commandIndex < 0 || evidenceIndex < commandIndex {
				t.Fatalf("%s must place the placeholder-safe command template before its replacement and non-execution evidence", path)
			}
			if strings.Contains(body, "可执行完整命令") || strings.Contains(body, "<完整命令>") {
				t.Fatalf("%s still forces an executable command when required IDs are absent", path)
			}
		})
	}
}

// Query the shipped projections, not only ContractFinal: legacy-only leaves
// may declare Result internally while intentionally omitting it from Schema.
func TestCrossPlatformCoverageAITableShareFormURLResultSchema(t *testing.T) {
	full := executeShortcutSchemaQuery(t, "--cli-path", "aitable form share url")
	compact := executeShortcutSchemaQuery(t, "--cli-path", "aitable form share url", "--compact")
	result, ok := full["result"].(map[string]any)
	if !ok {
		t.Fatal("form share url must publish result in final full Schema")
	}
	if !schemaContractJSONEqual(result, compact["result"]) {
		t.Fatalf("compact result differs from full result: full=%#v compact=%#v", result, compact["result"])
	}
	if !schemaContractJSONEqual(result["outcomes"], []string{"success", "failure"}) {
		t.Fatalf("unexpected outcomes: %#v", result["outcomes"])
	}
	schema := result["data_schema"].(map[string]any)
	branches := schema["oneOf"].([]any)
	if len(branches) != 2 {
		t.Fatalf("expected business and dry-run branches: %#v", branches)
	}
	business := branches[0].(map[string]any)
	properties := business["properties"].(map[string]any)
	for _, name := range []string{"shareFormUuid", "shareUrl", "copyableUrl", "qrFile"} {
		property, ok := properties[name].(map[string]any)
		if !ok || property["type"] != "string" || property["description"] == "" || property["description"] == nil {
			t.Fatalf("missing or invalid published property %s: %#v", name, properties[name])
		}
	}
	if !schemaContractJSONEqual(business["required"], []string{"shareFormUuid", "shareUrl", "copyableUrl"}) {
		t.Fatalf("qrFile must remain optional: %#v", business["required"])
	}
	preview := branches[1].(map[string]any)
	if !schemaContractJSONEqual(preview["required"], []string{"tool", "arguments", "executed"}) {
		t.Fatalf("unexpected dry-run required fields: %#v", preview)
	}
	executed := preview["properties"].(map[string]any)["executed"].(map[string]any)
	if executed["const"] != false {
		t.Fatalf("dry-run must declare executed=false: %#v", executed)
	}
}

func TestCrossPlatformCoverageAITableShareFormDiscoveryRoutesStayAligned(t *testing.T) {
	const atomicRoute = `dws schema --cli-path "aitable form share update" --compact --format json`
	const shortcutRoute = `dws schema --cli-path "aitable +form-share-update" --compact --format json`
	paths := []string{
		"../../skills/multi/dingtalk-aitable/SKILL.md",
		"../../skills/multi/dingtalk-aitable/references/aitable/aitable-form.md",
		"../../skills/mono/references/products/aitable/aitable-form.md",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			text := string(body)
			for _, route := range []string{atomicRoute, shortcutRoute} {
				if !strings.Contains(text, route) {
					t.Errorf("missing canonical form-share discovery route %q", route)
				}
			}
			if strings.Contains(text, "dws aitable form share update --help") {
				t.Error("form-share usage discovery must use Schema consistently, not leaf Help")
			}
		})
	}
}
