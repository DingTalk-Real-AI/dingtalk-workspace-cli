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

// devevent_lint.go 是 `dws dev event lint` 的本地全量校验：文件预检 +
// Payload Schema 结构规则 + 样例符合性，全部在本地完成，不落库、不联网。
//
// 规则同步纪律：结构规则逐条镜像服务端 AppUserEventSchemaValidator
// （ability-meta）的 checkSubSchema——关键字白名单、类型集合、深度上限与
// 错误措辞保持一致；服务端规则演进（如 format 语义标签批次）落地时必须
// 同步修改本文件，并以服务端单测用例导出的夹具做镜像回归。样例符合性
// 使用内嵌 Draft 2020-12 校验器（jsonschema/v6），受控白名单恰好排除了
// 所有跨实现方言灰区关键字，语义等价由标准保证。
//
// 本地校验的判定方向：只报"确定错"。服务端仍是权威（validate/publish），
// 本地通过不代表服务端一定通过（白名单可能滞后于服务端新放行的关键字）。

import (
	"fmt"
	"sort"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/spf13/cobra"
)

// devEventLintAllowedKeywords 镜像服务端 ALLOWED_KEYWORDS（含 format 语义标签）。
var devEventLintAllowedKeywords = map[string]bool{
	"$schema": true, "type": true, "properties": true, "required": true,
	"additionalProperties": true, "items": true, "enum": true,
	"description": true, "title": true, "format": true,
}

// devEventLintAllowedFormats 镜像服务端 ALLOWED_FORMATS：自定义语义标签
// 受控枚举，区分同类型不同语义字段（消费 Agent 据此决定反查/转换动作）。
var devEventLintAllowedFormats = map[string]bool{
	"open_dingtalk_id": true, "staff_id": true, "conversation_id": true,
	"unified_app_id": true, "timestamp_ms": true, "url": true, "email": true,
}

// devEventLintAllowedTypes 镜像服务端 ALLOWED_TYPES：Draft 2020-12 类型全集。
var devEventLintAllowedTypes = map[string]bool{
	"object": true, "array": true, "string": true, "number": true,
	"integer": true, "boolean": true, "null": true,
}

var devEventLintPlaceholderDescriptions = map[string]bool{
	"name": true, "名称": true, "内容": true, "字段值": true, "暂无": true,
	"string": true, "number": true, "integer": true,
	"boolean": true, "object": true, "array": true, "null": true,
	"字符串": true, "数字": true, "整数": true, "布尔值": true, "对象": true, "数组": true,
}

// devEventLintMaxDepth 镜像服务端 MAX_SCHEMA_DEPTH。
const devEventLintMaxDepth = 16

// devEventLintTool 是本地 lint 叶子的身份名。lint 本地执行不经 MCP 派发；
// 名称与服务端后续注册的 lint_event_definition 工具对齐，供未来 --check 复用。
const devEventLintTool = "lint_event_definition"

// devEventSortedKeys 返回 map 键的稳定排序，保证 issues 输出顺序确定。
func devEventSortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// devEventLintFieldDescription 是本地 lint 的硬门禁：properties 下声明的每个
// 业务字段都必须提供非空 description，避免定义可以创建但调用方无法理解。
func devEventLintFieldDescription(path string, fieldName string, field map[string]any, issues []string) []string {
	description, present := field["description"]
	if !present {
		return append(issues, fmt.Sprintf("%s: 字段必须提供非空字符串 description", path))
	}
	text, ok := description.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return append(issues, fmt.Sprintf("%s: 字段 description 必须是非空字符串", path))
	}
	normalized := strings.ToLower(strings.TrimSpace(text))
	if devEventLintPlaceholderDescriptions[normalized] || normalized == strings.ToLower(strings.TrimSpace(fieldName)) {
		return append(issues, fmt.Sprintf("%s: 字段 description 不能是字段名或占位文本", path))
	}
	return issues
}

func devEventLintFormatValue(value any) string {
	if text, ok := value.(string); ok {
		return fmt.Sprintf("%q", text)
	}
	return fmt.Sprintf("%v", value)
}

// devEventLintSchema 递归执行 Payload Schema 的结构校验，问题逐条追加进
// issues（措辞对齐服务端 checkSchemaDocument，便于与服务端 validate 对照）。
// 每次从 properties 进入字段节点时，都会执行 description 硬门禁。
func devEventLintSchema(node map[string]any, path string, depth int, issues []string) []string {
	if depth > devEventLintMaxDepth {
		return append(issues, fmt.Sprintf("%s: 嵌套深度超出上限 %d", path, devEventLintMaxDepth))
	}
	for key := range node {
		if !devEventLintAllowedKeywords[key] {
			issues = append(issues, fmt.Sprintf("%s: 不支持的关键字 %q", path, key))
		}
	}
	typ, hasType := node["type"].(string)
	if node["type"] == nil {
		issues = append(issues, fmt.Sprintf("%s: 缺少必填关键字 \"type\"", path))
	} else if !devEventLintAllowedTypes[typ] {
		return append(issues, fmt.Sprintf("%s: type 取值非法 %q", path, node["type"]))
	}
	if depth == 0 && hasType && typ != "object" {
		return append(issues, fmt.Sprintf("%s: 根类型必须是 object", path))
	}
	if format, present := node["format"]; present {
		formatStr, ok := format.(string)
		if !ok || !devEventLintAllowedFormats[formatStr] {
			issues = append(issues, fmt.Sprintf("%s: format 取值非法 %s（受控枚举）", path, devEventLintFormatValue(format)))
		} else if typ != "string" {
			issues = append(issues, fmt.Sprintf("%s: format 仅允许声明在 type=string 的字段上", path))
		}
	}
	properties, propertiesDeclared := node["properties"]
	if typ == "object" || (!hasType && propertiesDeclared) {
		if !propertiesDeclared {
			issues = append(issues, fmt.Sprintf("%s: 根/对象节点缺少 \"properties\"", path))
		} else if props, ok := properties.(map[string]any); !ok {
			issues = append(issues, fmt.Sprintf("%s.properties: 必须是对象", path))
		} else {
			for _, field := range devEventSortedKeys(props) {
				switch sub := props[field].(type) {
				case map[string]any:
					fieldPath := path + ".properties." + field
					issues = devEventLintFieldDescription(fieldPath, field, sub, issues)
					issues = devEventLintSchema(sub, fieldPath, depth+1, issues)
				default:
					issues = append(issues, fmt.Sprintf("%s.properties.%s: 字段定义必须是对象", path, field))
				}
			}
		}
	} else if propertiesDeclared {
		issues = append(issues, fmt.Sprintf("%s: 非 object 类型不允许声明 \"properties\"", path))
	}
	if required, present := node["required"]; present {
		issues = devEventLintRequired(path, node, required, issues)
	}
	if items, present := node["items"]; present {
		if typ == "array" {
			if sub, ok := items.(map[string]any); ok {
				issues = devEventLintSchema(sub, path+".items", depth+1, issues)
			} else {
				issues = append(issues, fmt.Sprintf("%s.items: 必须是对象（不支持元组形式）", path))
			}
		} else {
			issues = append(issues, fmt.Sprintf("%s: 非 array 类型不允许声明 \"items\"", path))
		}
	}
	return issues
}

// devEventLintRequired 校验 required 形状与悬空引用（措辞对齐服务端：
// 引用未在 properties 中声明的字段直接报错）。
func devEventLintRequired(path string, node map[string]any, required any, issues []string) []string {
	list, ok := required.([]any)
	if !ok {
		return append(issues, fmt.Sprintf("%s.required: 必须是字符串数组", path))
	}
	props, _ := node["properties"].(map[string]any)
	for _, item := range list {
		field, isString := item.(string)
		if !isString {
			return append(issues, fmt.Sprintf("%s.required: 必须是字符串数组", path))
		}
		if props != nil {
			if _, declared := props[field]; !declared {
				issues = append(issues, fmt.Sprintf("%s.required: 字段 %q 未在 properties 中声明", path, field))
			}
		}
	}
	return issues
}

// devEventLintSample 按 Draft 2020-12 校验 samplePayload 是否符合 payloadSchema。
// 受控白名单内的关键字语义由标准保证，内嵌校验器与服务端 networknt 行为一致。
func devEventLintSample(schema map[string]any, sample map[string]any) []string {
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("dev-event-lint://payload-schema.json", schema); err != nil {
		return []string{"payloadSchema 无法编译: " + err.Error()}
	}
	compiled, err := compiler.Compile("dev-event-lint://payload-schema.json")
	if err != nil {
		return []string{"payloadSchema 无法编译: " + err.Error()}
	}
	if err := compiled.Validate(sample); err != nil {
		return []string{"payloadSchema 与 samplePayload 不符: " + devEventLintErrorText(err)}
	}
	return nil
}

// devEventLintErrorText 把 jsonschema/v6 的校验错误压成单行可读文本。
func devEventLintErrorText(err error) string {
	text := strings.TrimSpace(err.Error())
	if idx := strings.IndexByte(text, '\n'); idx >= 0 {
		text = text[:idx] + " …"
	}
	return text
}

// devEventLintDefinitionIssues 是文件级预检的"收集版"：与 create 的
// devEventReadDefinitionFile 同一套规则，但逐条追加为 issue 而不是快速失败，
// 让一次 lint 看到全部问题。
func devEventLintDefinitionIssues(path string, definition map[string]any, issues []string) []string {
	if rejected := devEventRejectedControlledKeys(definition); len(rejected) > 0 {
		issues = append(issues, fmt.Sprintf("%s 含不接受开发者自行指定的键：%s", path, strings.Join(rejected, ", ")))
	}
	for _, field := range []string{"eventKey", "name"} {
		if strings.TrimSpace(devEventParamString(definition, field)) == "" {
			issues = append(issues, fmt.Sprintf("%s 缺少必填键 %q", path, field))
		}
	}
	if payloadSchema, present := definition["payloadSchema"]; !present {
		issues = append(issues, fmt.Sprintf("%s 缺少必填键 \"payloadSchema\"", path))
	} else if payloadSchema == nil {
		issues = append(issues, fmt.Sprintf("%s 的 payloadSchema 是 null（省略键与写 null 语义不同，null 会被原样下发）", path))
	} else if _, ok := payloadSchema.(map[string]any); !ok {
		issues = append(issues, fmt.Sprintf("%s 的 payloadSchema 必须是 JSON 对象", path))
	}
	if samplePayload, present := definition["samplePayload"]; present {
		if samplePayload == nil {
			issues = append(issues, fmt.Sprintf("%s 的 samplePayload 是 null（省略请删除该键，写 null 会被原样下发）", path))
		} else if _, ok := samplePayload.(map[string]any); !ok {
			issues = append(issues, fmt.Sprintf("%s 的 samplePayload 必须是 JSON 对象（省略请删除该键，不要写 null）", path))
		}
	}
	eventKey := strings.TrimSpace(devEventParamString(definition, "eventKey"))
	if eventKey != "" {
		if len(eventKey) > devEventKeyMaxLength {
			issues = append(issues, fmt.Sprintf("%s 的 eventKey 超过 %d 字符（当前 %d）", path, devEventKeyMaxLength, len(eventKey)))
		}
		if !devEventKeyPattern.MatchString(eventKey) {
			issues = append(issues, fmt.Sprintf("%s 的 eventKey %q 不符合命名规则", path, eventKey))
		}
	}
	return issues
}

// runDevEventLint 是 lint 的执行体：读文件（不可读/超限/非 JSON 仍快速失败），
// 其余全部问题收集为 issues，valid=issues 为空。lint 本身恒为成功结果——
// "文件不合法"是一次成功的校验（data.valid=false），不是命令失败。
func runDevEventLint(cmd *cobra.Command, params map[string]any) error {
	path, _ := params["file"].(string)
	path = strings.TrimSpace(path)
	delete(params, "file")
	if path == "" {
		return apperrors.NewValidation("--file 为必填", apperrors.WithHint("指向一份事件定义 JSON 文件，一份文件定义一个事件。"))
	}
	definition, err := devEventReadJSONObject(path, "--file")
	if err != nil {
		return err
	}
	issues := devEventLintDefinitionIssues(path, definition, nil)
	if schema, ok := definition["payloadSchema"].(map[string]any); ok {
		issues = devEventLintSchema(schema, "payloadSchema", 0, issues)
		if sample, hasSample := definition["samplePayload"].(map[string]any); hasSample {
			issues = append(issues, devEventLintSample(schema, sample)...)
		}
	}
	// Result 契约声明 issues 为数组且必填：通过时输出 []，不能是 null。
	if issues == nil {
		issues = []string{}
	}
	result := map[string]any{"valid": len(issues) == 0, "issues": issues}
	env := &output.Envelope{OK: true, Outcome: output.OutcomeSuccess, Data: result}
	return writeDevRolloutResult(cmd, output.Success(result), env, output.FormatJSON)
}

func newDevEventLintCommand(_ executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:           "lint",
		Short:         "本地全量校验事件定义文件（不落库不联网）",
		Example:       "  dws dev event lint --file ./event.json --format json",
		Tool:          devEventLintTool,
		Safety:        devEventSafetyRead(),
		OutputRollout: output.RolloutUnifiedActive,
		Flags:         []LeafFlag{devEventDefinitionFileFlag()},
		Contract: LeafContract{
			Identity:    devEventIdentity(devEventLintTool, "dev event lint"),
			Description: "本地全量校验事件定义文件：文件预检 + Payload Schema 结构规则 + 样例符合性，不落库不联网",
			Result: devEventResultSpec("本地 lint 校验结论", map[string]any{
				"valid":  map[string]any{"type": "boolean", "description": "文件是否通过全部本地校验"},
				"issues": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "全部问题清单（文件级、结构规则、样例符合性），为空表示通过"},
			}, "valid", "issues"),
			Interface: &contract.InterfaceSpec{
				Mode:         "local",
				Availability: "available",
				Reason:       "本地校验：文件预检与结构规则为服务端校验器的 Go 直译，样例校验用内嵌 Draft 2020-12 校验器；不绑定 MCP RPC",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "在 create 之前于本地拿到文件预检、Schema 结构规则与样例符合性的全量校验结论",
				UseWhen:      []string{"写完定义文件想立即校验，或离线/CI 场景需要在提交前拦截问题"},
				AvoidWhen: devEventAvoidWhen(
					"已创建的草稿用 dev event validate（含与生效快照的兼容性检查）；本地通过不代表服务端一定通过，权威判定仍是服务端"),
				Examples: []string{
					"dws dev event lint --file ./event.json",
				},
			},
		},
		Call: func(cmd *cobra.Command, _ string, params map[string]any) error {
			return runDevEventLint(cmd, params)
		},
		PostMount: devAppMeta(devEventLintTool),
	})
}
