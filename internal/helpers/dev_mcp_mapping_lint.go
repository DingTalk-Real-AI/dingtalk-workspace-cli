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
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cobracmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
)

// 映射引用静态校验（红线#13）：mapping rules 的 source/target 引用了未声明的
// 字段时，服务端照单全收、运行时也不报错，但管理台 UI 出参映射页标
// 「变量已失效」，且工具出参 schema 为空。tool update 是全量提交，因此
// mappings 引用的字段树必须与 mappings 同批出现，可以在本地静态互验。

const (
	devMCPSourceSystemNode       = "$.system_node"
	devMCPSourceNodeStart        = "$.node_start"
	devMCPSourceServiceActivator = "$.node_service_activator"
)

// devMCPLintMappingReferences cross-checks inputMappings/outputMappings against
// the field trees submitted in the same upsert request.
func devMCPLintMappingReferences(params map[string]any) error {
	apiInputs, _ := params["apiInputs"].(map[string]any)
	apiOutputs, _ := params["apiOutputs"].(map[string]any)
	toolInputs, _ := params["toolInputs"].([]any)
	toolOutputs, _ := params["toolOutputs"].([]any)
	if mappings, ok := params["inputMappings"].([]any); ok {
		if err := devMCPLintInputMappings(mappings, toolInputs, apiInputs); err != nil {
			return err
		}
	}
	if mappings, ok := params["outputMappings"].([]any); ok {
		if err := devMCPLintOutputMappings(mappings, apiOutputs, toolOutputs, toolInputs); err != nil {
			return err
		}
	}
	return nil
}

func devMCPLintInputMappings(mappings []any, toolInputs []any, apiInputs map[string]any) error {
	for i, raw := range mappings {
		mapping, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		path := fmt.Sprintf("--input-mappings[%d]", i)
		typ := stringValue(mapping["type"])
		source := stringValue(mapping["source"])
		target := stringValue(mapping["target"])

		if typ == "reference" {
			if err := devMCPLintVariableSource(path, source, toolInputs, "--tool-inputs"); err != nil {
				return err
			}
		}
		if err := devMCPLintInputTarget(path, target, apiInputs); err != nil {
			return err
		}
	}
	return nil
}

func devMCPLintOutputMappings(mappings []any, apiOutputs map[string]any, toolOutputs []any, toolInputs []any) error {
	for i, raw := range mappings {
		mapping, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		path := fmt.Sprintf("--output-mappings[%d]", i)
		typ := stringValue(mapping["type"])
		source := stringValue(mapping["source"])
		target := stringValue(mapping["target"])

		if typ == "reference" {
			switch {
			case devMCPPathHasPrefix(source, devMCPSourceSystemNode):
				// 系统变量注入，白名单放行。
			case devMCPPathHasPrefix(source, devMCPSourceNodeStart):
				if err := devMCPLintVariableSource(path, source, toolInputs, "--tool-inputs"); err != nil {
					return err
				}
			case devMCPPathHasPrefix(source, devMCPSourceServiceActivator):
				if err := devMCPLintActivatorSource(path, source, apiOutputs); err != nil {
					return err
				}
			default:
				return apperrors.NewValidation(fmt.Sprintf(
					"%s.source %q 不是可解析的变量路径：出参映射 source 只支持 $.node_service_activator.*（API 出参）/ $.node_start.*（工具入参）/ $.system_node.*（系统参数）；常量请用 type=fixed",
					path, source))
			}
		}
		if err := devMCPLintOutputTarget(path, target, toolOutputs); err != nil {
			return err
		}
	}
	return nil
}

// devMCPLintVariableSource resolves a $.node_start.* source against the tool
// input field tree submitted in the same request.
func devMCPLintVariableSource(path, source string, toolInputs []any, flag string) error {
	if !devMCPPathHasPrefix(source, devMCPSourceNodeStart) {
		if devMCPPathHasPrefix(source, devMCPSourceSystemNode) {
			return nil
		}
		return apperrors.NewValidation(fmt.Sprintf(
			"%s.source %q 不是可解析的变量路径：入参映射 source 只支持 $.node_start.*（工具入参）/ $.system_node.*（系统参数）；常量请用 type=fixed",
			path, source))
	}
	rest := devMCPPathRest(source, devMCPSourceNodeStart)
	if len(toolInputs) == 0 {
		return apperrors.NewValidation(fmt.Sprintf(
			"%s.source %q 引用了工具入参，但本次请求未提供 %s——tool create/update 为全量提交，mappings 与其引用的字段树必须同批提交",
			path, source, flag))
	}
	if rest == "" {
		return nil
	}
	if !devMCPResolveFieldPath(toolInputs, rest) {
		return apperrors.NewValidation(fmt.Sprintf(
			"%s.source %q 在 %s 中不可解析（缺少字段 %q）——引用未声明字段会静默失效，请先在 %s 里声明",
			path, source, flag, rest, flag))
	}
	return nil
}

// devMCPLintActivatorSource resolves a $.node_service_activator.* source
// against the declared API output groups（红线#13：整体透传引用 Body 根节点
// 同样要求 apiOutputs 已声明，否则 UI 标「变量已失效」）。
func devMCPLintActivatorSource(path, source string, apiOutputs map[string]any) error {
	if apiOutputs == nil {
		return apperrors.NewValidation(fmt.Sprintf(
			"%s.source %q 引用了 API 出参，但本次请求未提供 --api-outputs。整体透传也必须声明 API 出参（红线#13）：不声明则发布后 UI 出参映射标「变量已失效」、工具出参 schema 为空。结构未知时先按材料尽力声明 --api-outputs → tool debug 取样 → tool update 修正补全（0720 起出参三件套必填，不可裸建）",
			path, source))
	}
	rest := devMCPPathRest(source, devMCPSourceServiceActivator)
	if rest == "" {
		return apperrors.NewValidation(fmt.Sprintf(
			"%s.source 至少要引用到 $.node_service_activator.Body（HTTP 出参挂在 Body 下）", path))
	}
	segments := strings.SplitN(rest, ".", 2)
	group, groupErr := devMCPOutputGroupFields(apiOutputs, segments[0])
	if groupErr != "" {
		return apperrors.NewValidation(fmt.Sprintf("%s.source %q %s", path, source, groupErr))
	}
	if len(segments) == 1 {
		// 整体透传（引用组根节点）：要求该组声明了至少一个字段。
		if len(group) == 0 {
			return apperrors.NewValidation(fmt.Sprintf(
				"%s.source %q 引用的出参组在 --api-outputs 中为空——整体透传也必须按真实响应声明字段（红线#13），建议 tool debug 取样后如实声明",
				path, source))
		}
		return nil
	}
	if !devMCPResolveFieldPath(group, segments[1]) {
		return apperrors.NewValidation(fmt.Sprintf(
			"%s.source %q 在 --api-outputs 中不可解析（缺少字段 %q）——apiOutputs 必须如实声明到被映射的最深层级（红线#13，否则 UI 标「变量已失效」）",
			path, source, segments[1]))
	}
	return nil
}

func devMCPLintInputTarget(path, target string, apiInputs map[string]any) error {
	if target == "" {
		return nil
	}
	position, rest, ok := devMCPSplitTargetPosition(target)
	if !ok {
		return apperrors.NewValidation(fmt.Sprintf(
			"%s.target %q 必须以 Pascal 位置名开头：$.Body./$.Query./$.Head./$.Path.（全小写/全大写会静默失效不报错）",
			path, target))
	}
	if apiInputs == nil {
		return apperrors.NewValidation(fmt.Sprintf(
			"%s.target %q 指向 API 入参，但本次请求未提供 --api-inputs——tool create/update 为全量提交，mappings 与其引用的字段树必须同批提交",
			path, target))
	}
	group, groupErr := devMCPInputGroupFields(apiInputs, position)
	if groupErr != "" {
		return apperrors.NewValidation(fmt.Sprintf("%s.target %q %s", path, target, groupErr))
	}
	if rest == "" {
		return nil
	}
	if !devMCPResolveFieldPath(group, rest) {
		return apperrors.NewValidation(fmt.Sprintf(
			"%s.target %q 在 --api-inputs 中不可解析（缺少字段 %q）——映射目标字段（含 fixed 写死的）也必须在 apiInputs 里声明",
			path, target, rest))
	}
	return nil
}

func devMCPLintOutputTarget(path, target string, toolOutputs []any) error {
	if target == "" || target == "$" {
		// 整体透传目标。
		return nil
	}
	if !strings.HasPrefix(target, "$.") {
		return apperrors.NewValidation(fmt.Sprintf(
			"%s.target %q 必须是 $（整体透传）或 $.<toolOutputs字段路径>（字段级精修）", path, target))
	}
	rest := strings.TrimPrefix(target, "$.")
	if len(toolOutputs) == 0 {
		return apperrors.NewValidation(fmt.Sprintf(
			"%s.target %q 做字段级出参精修，必须同批提供 --tool-outputs 声明对外字段树（tool create/update 为全量提交）",
			path, target))
	}
	if !devMCPResolveFieldPath(toolOutputs, rest) {
		return apperrors.NewValidation(fmt.Sprintf(
			"%s.target %q 在 --tool-outputs 中不可解析（缺少字段 %q）——对外出参字段必须先在 toolOutputs 里声明",
			path, target, rest))
	}
	return nil
}

// devMCPSplitTargetPosition splits an input-mapping target like
// "$.Body.dept_id" into its Pascal position name and the remaining field path.
func devMCPSplitTargetPosition(target string) (position, rest string, ok bool) {
	if !strings.HasPrefix(target, "$.") {
		return "", "", false
	}
	trimmed := strings.TrimPrefix(target, "$.")
	segments := strings.SplitN(trimmed, ".", 2)
	switch segments[0] {
	case "Body", "Query", "Head", "Path":
	default:
		return "", "", false
	}
	if len(segments) == 2 {
		rest = segments[1]
	}
	return segments[0], rest, true
}

func devMCPInputGroupFields(apiInputs map[string]any, position string) ([]any, string) {
	keys := map[string][]string{
		"Body":  {"body"},
		"Query": {"query"},
		"Head":  {"headers", "head"},
		"Path":  {"path"},
	}[position]
	for _, key := range keys {
		if fields, ok := apiInputs[key].([]any); ok {
			return fields, ""
		}
	}
	return nil, fmt.Sprintf("指向 %s 位置，但 --api-inputs 未声明 %s 组", position, strings.Join(keys, "/"))
}

func devMCPOutputGroupFields(apiOutputs map[string]any, segment string) ([]any, string) {
	segment = devMCPStripIndexSuffix(segment)
	var keys []string
	switch segment {
	case "Body":
		keys = []string{"body"}
	case "Head", "Headers":
		keys = []string{"headers", "head"}
	default:
		return nil, fmt.Sprintf("的第一段 %q 不是合法出参位置——HTTP 出参挂在 $.node_service_activator.Body（或 .Head）下", segment)
	}
	for _, key := range keys {
		if fields, ok := apiOutputs[key].([]any); ok {
			return fields, ""
		}
	}
	return nil, fmt.Sprintf("指向 %s 位置，但 --api-outputs 未声明 %s 组", segment, strings.Join(keys, "/"))
}

// devMCPResolveFieldPath reports whether a dotted field path (segments may
// carry [*]/[0] suffixes) resolves inside a {key,type,children} field list.
// Array fields descend through their single key=items child.
func devMCPResolveFieldPath(fields []any, path string) bool {
	current := fields
	for _, seg := range strings.Split(path, ".") {
		seg = devMCPStripIndexSuffix(seg)
		if seg == "" {
			return false
		}
		var match map[string]any
		for _, raw := range current {
			field, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if stringValue(field["key"]) == seg {
				match = field
				break
			}
		}
		if match == nil {
			return false
		}
		children, _ := match["children"].([]any)
		if stringValue(match["type"]) == "array" && len(children) == 1 {
			if item, ok := children[0].(map[string]any); ok {
				children, _ = item["children"].([]any)
			}
		}
		current = children
	}
	return true
}

func devMCPStripIndexSuffix(segment string) string {
	if idx := strings.IndexByte(segment, '['); idx >= 0 {
		return segment[:idx]
	}
	return segment
}

func devMCPPathHasPrefix(path, prefix string) bool {
	return path == prefix || strings.HasPrefix(path, prefix+".")
}

func devMCPPathRest(path, prefix string) string {
	return strings.TrimPrefix(strings.TrimPrefix(path, prefix), ".")
}

// ---------------------------------------------------------------------------
// publish 读回硬闸：发布前把草稿详情读回来，静态复验出参 rules 的 source 在
// 已落库的声明 schema（outputSchemaMappingJson）里可解析。声明缺失的工具发布
// 后 UI 标「变量已失效」且工具出参 schema 为空，因此直接拒发并给修复指引。

// devMCPPublishPreflight reads the draft back via mcp_tool_get and statically
// re-validates every stored output-mapping source before publishing.
func devMCPPublishPreflight(runner executor.Runner, cmd *cobra.Command, locator map[string]any) error {
	inv := executor.NewHelperInvocation(cobracmd.LegacyCommandPath(cmd), devMCPProduct, devMCPToolGetTool, map[string]any{
		"mcpId":  locator["mcpId"],
		"toolId": locator["toolId"],
	})
	res, err := runner.Run(cmd.Context(), inv)
	if err != nil {
		return fmt.Errorf("发布前读回工具详情失败（mcp_tool_get）：%w", err)
	}
	tool := devMCPExtractToolDetail(res.Response)
	if tool == nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "警告：发布前读回的工具详情不含出参映射字段，跳过静态复验（如为脚手架契约变更请同步升级 dws）。")
		return nil
	}
	broken := devMCPLintStoredOutputMappings(
		stringValue(tool["outputSchemaMappingJson"]),
		stringValue(tool["outputMappingConfigJson"]),
	)
	if len(broken) == 0 {
		return nil
	}
	return apperrors.NewValidation(fmt.Sprintf(
		"发布被拦截：以下出参映射 source 在已声明的 API 出参 schema 中不可解析，发布后管理台 UI 将标「变量已失效」、工具出参 schema 为空/失真（红线#13）：\n  - %s\n修复：dws dev mcp tool update 按 tool debug 真实响应把 --api-outputs 声明到被映射的最深层级（整体透传也必须声明），与 --output-mappings 同批提交，再重新 publish。",
		strings.Join(broken, "\n  - ")))
}

// runDevMCPURLGet runs mcp_server_url_get and, when the call returns a success
// response that carries no接入地址, prints a hint. The most common cause of an
// empty-success is a DRAFT instance: for an auth-configured service the
// credential must be bound BEFORE the tool is published; binding afterwards does
// not recover it. Surfacing this stops callers reading the bare success as OK.
func runDevMCPURLGet(runner executor.Runner, cmd *cobra.Command, params map[string]any) error {
	inv := executor.NewHelperInvocation(cobracmd.LegacyCommandPath(cmd), devMCPProduct, devMCPServerURLGetTool, params)
	inv.DryRun = commandDryRun(cmd)
	res, err := runner.Run(cmd.Context(), inv)
	if err != nil {
		return err
	}
	if !commandDryRun(cmd) && devMCPResponseLacksURL(res.Response) {
		fmt.Fprintln(cmd.ErrOrStderr(), "提示：本次返回 success 但无接入地址（mcpUrl）。常见原因：服务处于草稿态——带鉴权的服务须在「发布工具之前」先 credential bind，先发布后 bind 会卡草稿态且事后 bind 也不生效（只能重建工具按正确顺序重发布）。请确认凭证已在发布前绑定；无鉴权服务不受此影响。")
	}
	return writeDevAppEnvelope(cmd, res)
}

// devMCPResponseLacksURL reports whether a mcp_server_url_get response came back
// without a usable mcpUrl (the empty-success that masks a DRAFT instance). It
// tolerates both the flat response and content/result envelopes.
func devMCPResponseLacksURL(response map[string]any) bool {
	for _, m := range []map[string]any{response, devAppConnectUnwrap(response)} {
		if m == nil {
			continue
		}
		if s, _ := m["mcpUrl"].(string); strings.TrimSpace(s) != "" {
			return false
		}
	}
	return true
}

// devMCPExtractToolDetail digs the tool object out of a mcp_tool_get response,
// tolerating content/result envelopes and both wrapped/flat layouts.
func devMCPExtractToolDetail(response map[string]any) map[string]any {
	payload := devAppConnectUnwrap(response)
	if payload == nil {
		return nil
	}
	if tool, ok := payload["tool"].(map[string]any); ok {
		payload = tool
	}
	if _, ok := payload["outputMappingConfigJson"]; ok {
		return payload
	}
	if _, ok := payload["outputSchemaMappingJson"]; ok {
		return payload
	}
	return nil
}

type devMCPStoredMappingRule struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"`
}

// devMCPLintStoredOutputMappings validates the stored draft form returned by
// mcp_tool_get: outputSchemaMappingJson（声明 schema，JSON-schema 形态）与
// outputMappingConfigJson.rules（映射规则）。返回全部不可解析的 rule 描述。
func devMCPLintStoredOutputMappings(outputSchemaMappingJSON, outputMappingConfigJSON string) []string {
	rules := devMCPParseStoredRules(outputMappingConfigJSON)
	if len(rules) == 0 {
		return nil
	}
	declared := map[string]any{}
	if strings.TrimSpace(outputSchemaMappingJSON) != "" {
		_ = json.Unmarshal([]byte(outputSchemaMappingJSON), &declared)
	}
	var broken []string
	for i, rule := range rules {
		if rule.Type != "reference" && rule.Type != "" {
			continue
		}
		source := strings.TrimSpace(rule.Source)
		if !devMCPPathHasPrefix(source, devMCPSourceServiceActivator) {
			// node_start/system_node 及未知节点在 create/update 侧把关。
			continue
		}
		rest := devMCPPathRest(source, devMCPSourceServiceActivator)
		if rest == "" {
			broken = append(broken, fmt.Sprintf("rules[%d].source %q（未落到 Body/Head 位置）", i, source))
			continue
		}
		if !devMCPResolveSchemaGroupPath(declared, rest) {
			broken = append(broken, fmt.Sprintf("rules[%d].source %q", i, source))
		}
	}
	return broken
}

func devMCPParseStoredRules(outputMappingConfigJSON string) []devMCPStoredMappingRule {
	trimmed := strings.TrimSpace(outputMappingConfigJSON)
	if trimmed == "" {
		return nil
	}
	var config struct {
		Rules string `json:"rules"`
	}
	if err := json.Unmarshal([]byte(trimmed), &config); err != nil {
		return nil
	}
	if strings.TrimSpace(config.Rules) == "" {
		return nil
	}
	var rules []devMCPStoredMappingRule
	if err := json.Unmarshal([]byte(config.Rules), &rules); err != nil {
		return nil
	}
	return rules
}

// devMCPResolveSchemaGroupPath resolves "Body.result.xxx" style paths against
// the stored declaration form: {"BODY":{JSON schema},"HEAD":{JSON schema}}.
func devMCPResolveSchemaGroupPath(declared map[string]any, rest string) bool {
	segments := strings.SplitN(rest, ".", 2)
	group := devMCPStripIndexSuffix(segments[0])
	var keys []string
	switch group {
	case "Body":
		keys = []string{"BODY", "Body", "body"}
	case "Head", "Headers":
		keys = []string{"HEAD", "Head", "head", "HEADERS", "headers"}
	default:
		// 未知位置（如 HSF 直挂形态）不在本闸校验范围，放行避免误拦。
		return true
	}
	var node map[string]any
	for _, key := range keys {
		if candidate, ok := declared[key].(map[string]any); ok {
			node = candidate
			break
		}
	}
	if node == nil {
		return false
	}
	if len(segments) == 1 {
		// 整体透传引用组根节点：要求声明了至少一个字段。
		props, _ := node["properties"].(map[string]any)
		return len(props) > 0
	}
	return devMCPResolveSchemaPath(node, segments[1])
}

// devMCPResolveSchemaPath walks a dotted path through a stored JSON-schema
// node (properties/items form). Unknown array shapes resolve leniently to
// avoid false-blocking publishes on forms this linter does not understand.
func devMCPResolveSchemaPath(node map[string]any, path string) bool {
	current := node
	for _, seg := range strings.Split(path, ".") {
		seg = devMCPStripIndexSuffix(seg)
		if seg == "" {
			return false
		}
		for stringValue(current["type"]) == "array" {
			items, ok := current["items"].(map[string]any)
			if !ok {
				return true
			}
			current = items
		}
		props, ok := current["properties"].(map[string]any)
		if !ok {
			return false
		}
		next, ok := props[seg].(map[string]any)
		if !ok {
			return false
		}
		current = next
	}
	return true
}
