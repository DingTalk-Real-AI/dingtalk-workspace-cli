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

// Package devapp declares high-fidelity shortcuts for the DingTalk open-platform
// developer application ("devapp") MCP service, for the apps
// command surface. Tool names and parameter keys are copied verbatim from
// internal/helpers/devapp.go — do not invent tools or params here.
//
// Skipped tools (require async orchestration / polling, not a single MCP call):
//   - submit_robot_create_task  (异步提交机器人建号任务)
//   - query_robot_create_result (异步轮询建号结果)
//
// The robot-config i18n JSON-object params (i18nName/i18nBrief/i18nDescription)
// are also omitted: they need JSON-object parsing that a flat flag can't express.
package devapp

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

const productDevApp = "devapp"

// applyCursor forwards --cursor/--page-size into params, matching the helper's
// pass-through pagination (pageSize defaults to 20, cursor omitted on page 1).
func applyCursor(rt *shortcut.RuntimeContext, params map[string]any) {
	if rt.Changed("cursor") {
		if cur := rt.Str("cursor"); cur != "" {
			params["cursor"] = cur
		}
	}
	size := rt.Int("page-size")
	if size < 1 {
		size = 20
	}
	params["pageSize"] = size
}

var cursorFlags = []shortcut.Flag{
	{Name: "cursor", Type: shortcut.FlagString, Desc: "游标令牌：首次查询留空，续翻传上次 meta.pagination.next_token"},
	{Name: "page-size", Type: shortcut.FlagInt, Default: "20", Desc: "单页条数，默认 20"},
}

func devAppObjectResult(outcomes ...contract.ResultOutcome) *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes:   append([]contract.ResultOutcome(nil), outcomes...),
		DataSchema: json.RawMessage(`{"type":"object","description":"开放平台命令返回的业务对象；具体字段由对应操作定义","additionalProperties":true}`),
	}
}

func devAppVerifiedMutationResult() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(`{"type":"object","description":"经稳定应用 ID 精确读回验证的开放平台应用变更","properties":{"action":{"type":"string","description":"已执行并验证的变更动作"},"unifiedAppId":{"type":"string","description":"发生变更的稳定统一应用 ID"},"verified":{"type":"boolean","description":"是否已通过精确对象读回或删除后全量排除验证"},"resource":{"type":"object","description":"写后按同一稳定 ID 读回的业务对象；删除动作省略","additionalProperties":true}},"required":["action","unifiedAppId","verified"],"additionalProperties":false}`),
	}
}

func devAppCredentialResult() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes:       []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema:     json.RawMessage(`{"type":"object","description":"按稳定应用 ID 严格验证的开放平台应用凭证","properties":{"unifiedAppId":{"type":"string","description":"开放平台统一应用 ID"},"appKey":{"type":"string","description":"应用公开客户端标识"},"clientId":{"type":"string","description":"应用公开客户端标识兼容字段"},"appSecret":{"type":"string","description":"应用敏感客户端密钥"},"clientSecret":{"type":"string","description":"应用敏感客户端密钥兼容字段"},"secret":{"type":"string","description":"应用敏感密钥兼容字段"}},"additionalProperties":true}`),
		SensitivePaths: []string{"appSecret", "clientSecret", "secret"},
	}
}

func devAppMemberListResult() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes:       []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema:     json.RawMessage(`{"type":"object","description":"严格验证并可按稳定 userId 精确筛选的应用成员列表","properties":{"count":{"type":"integer","description":"匹配的成员数量"},"members":{"type":"array","description":"匹配的应用成员","items":{"type":"object","description":"带稳定 userId 的应用成员","additionalProperties":true}}},"required":["count","members"],"additionalProperties":false}`),
		SensitivePaths: []string{"members.name"},
	}
}

func devAppPaginatedProjectionResult(collection, description string) *contract.ResultSpec {
	schema, _ := json.Marshal(map[string]any{
		"type":                 "object",
		"description":          description,
		"additionalProperties": false,
		"properties": map[string]any{
			"count": map[string]any{
				"type":        "integer",
				"description": "当前页业务记录数量",
			},
			collection: map[string]any{
				"type":        "array",
				"description": "当前页业务记录；分页控制信息只读取 meta.pagination",
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": true,
				},
			},
		},
		"required": []string{"count", collection},
	})
	return &contract.ResultSpec{
		Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: schema,
	}
}

func devAppCursorPagination() *contract.PaginationSpec {
	return &contract.PaginationSpec{
		Kind:            contract.PaginationKindCursor,
		CursorParameter: "cursor",
	}
}

const devAppMaxReadbackPages = 20

func devAppResponseError(operation, reason, message string) error {
	return apperrors.NewAPI(message,
		apperrors.WithOperation(operation),
		apperrors.WithOrigin("mcp"),
		apperrors.WithFailureStage("response_validation"),
		apperrors.WithRetryable(false),
		apperrors.WithReason(reason),
	)
}

func devAppContainers(data map[string]any) []map[string]any {
	if data == nil {
		return nil
	}
	containers := []map[string]any{data}
	for index := 0; index < len(containers) && index < 12; index++ {
		for _, key := range []string{"content", "result", "data"} {
			if nested, ok := containers[index][key].(map[string]any); ok {
				containers = append(containers, nested)
			}
		}
	}
	return containers
}

func devAppErrorValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case bool:
		return typed
	case float64:
		return typed != 0 && typed != 200
	case string:
		normalized := strings.ToUpper(strings.TrimSpace(typed))
		return normalized != "" && normalized != "0" && normalized != "200" && normalized != "OK" && normalized != "SUCCESS"
	case map[string]any:
		return len(typed) > 0
	case []any:
		return len(typed) > 0
	default:
		return true
	}
}

func devAppFirstString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func requireDevAppSuccess(data map[string]any, operation string) (map[string]any, error) {
	if len(data) == 0 {
		return nil, devAppResponseError(operation, "empty_tool_response", "服务返回空响应，无法证明操作成功或结果确实为空")
	}
	foundSuccess := false
	for _, candidate := range devAppContainers(data) {
		if value, present := candidate["success"]; present {
			foundSuccess = true
			success, ok := value.(bool)
			if !ok {
				return nil, devAppResponseError(operation, "malformed_success", "响应 success 字段不是布尔值")
			}
			if !success {
				message := devAppFirstString(candidate, "errorMsg", "errorMessage", "message")
				if message == "" {
					message = "服务明确返回 success=false"
				}
				return nil, devAppResponseError(operation, "remote_failure", message)
			}
		}
		for _, key := range []string{"error", "errorCode", "error_code"} {
			if value, present := candidate[key]; present && devAppErrorValue(value) {
				return nil, devAppResponseError(operation, "conflicting_error", fmt.Sprintf("响应 success 与 %s 错误字段冲突", key))
			}
		}
	}
	if !foundSuccess {
		return nil, devAppResponseError(operation, "missing_success", "响应缺少 success 布尔终态，无法证明业务成功")
	}
	return data, nil
}

func devAppOnlyEnvelopeFields(candidate map[string]any) bool {
	for key := range candidate {
		switch key {
		case "success", "result", "data", "content", "message", "error", "errorCode", "error_code", "code":
		default:
			return false
		}
	}
	return true
}

func requireDevAppObject(data map[string]any, operation string) (map[string]any, error) {
	data, err := requireDevAppSuccess(data, operation)
	if err != nil {
		return nil, err
	}
	containers := devAppContainers(data)
	for index := len(containers) - 1; index >= 0; index-- {
		candidate := containers[index]
		if len(candidate) == 0 || devAppOnlyEnvelopeFields(candidate) {
			continue
		}
		return candidate, nil
	}
	return nil, devAppResponseError(operation, "missing_business_result", "响应没有可验证的非空业务对象")
}

func requireDevAppIdentity(object map[string]any, operation string, expected map[string]string) error {
	for key, want := range expected {
		if want == "" {
			continue
		}
		if got := devAppFirstString(object, key); got == "" {
			return devAppResponseError(operation, "readback_id_missing", fmt.Sprintf("读回对象缺少稳定字段 %s", key))
		} else if got != want {
			return devAppResponseError(operation, "readback_id_mismatch", fmt.Sprintf("读回对象字段 %s 与请求目标不一致", key))
		}
	}
	return nil
}

func requireDevAppFields(object map[string]any, operation string, expected map[string]any) error {
	for key, want := range expected {
		got, present := object[key]
		if !present {
			return devAppResponseError(operation, "readback_field_missing", fmt.Sprintf("读回对象缺少请求字段 %s", key))
		}
		if fmt.Sprint(got) != fmt.Sprint(want) {
			return devAppResponseError(operation, "readback_field_mismatch", fmt.Sprintf("读回对象字段 %s 与请求值不一致", key))
		}
	}
	return nil
}

func requireDevAppStringField(object map[string]any, operation, want string, keys ...string) error {
	if got := devAppFirstString(object, keys...); got == "" {
		return devAppResponseError(operation, "readback_field_missing", fmt.Sprintf("读回对象缺少请求字段 %s", strings.Join(keys, "/")))
	} else if got != want {
		return devAppResponseError(operation, "readback_field_mismatch", fmt.Sprintf("读回对象字段 %s 与请求值不一致", strings.Join(keys, "/")))
	}
	return nil
}

func requireDevAppCollection(data map[string]any, operation string, idKeys []string, keys ...string) ([]any, map[string]any, error) {
	data, err := requireDevAppSuccess(data, operation)
	if err != nil {
		return nil, nil, err
	}
	for _, candidate := range devAppContainers(data) {
		for _, key := range keys {
			value, present := candidate[key]
			if !present {
				continue
			}
			items, ok := value.([]any)
			if !ok {
				return nil, nil, devAppResponseError(operation, "malformed_collection", fmt.Sprintf("响应 %s 字段不是数组", key))
			}
			for index, raw := range items {
				item, ok := raw.(map[string]any)
				if !ok {
					return nil, nil, devAppResponseError(operation, "malformed_item", fmt.Sprintf("响应 %s[%d] 不是对象", key, index))
				}
				if devAppFirstString(item, idKeys...) == "" {
					return nil, nil, devAppResponseError(operation, "missing_item_id", fmt.Sprintf("响应 %s[%d] 缺少稳定身份字段", key, index))
				}
			}
			return items, candidate, nil
		}
	}
	return nil, nil, devAppResponseError(operation, "missing_collection", "成功响应缺少预期集合字段")
}

func readDevAppObject(rt *shortcut.RuntimeContext, tool string, params map[string]any, expected map[string]string) (map[string]any, map[string]any, error) {
	raw, err := rt.CallMCPData(productDevApp, tool, params)
	if err != nil {
		return nil, nil, err
	}
	object, err := requireDevAppObject(raw, productDevApp+"/"+tool)
	if err != nil {
		return nil, nil, err
	}
	if err := requireDevAppIdentity(object, productDevApp+"/"+tool, expected); err != nil {
		return nil, nil, err
	}
	return raw, object, nil
}

func outputDevAppObject(rt *shortcut.RuntimeContext, tool string, params map[string]any, expected map[string]string) error {
	raw, _, err := readDevAppObject(rt, tool, params, expected)
	if err != nil {
		return err
	}
	return rt.OutputForTool(tool, raw)
}

func devAppWritePreview(rt *shortcut.RuntimeContext, tool string, params map[string]any) (bool, error) {
	if !rt.DryRun() {
		return false, nil
	}
	return true, rt.Output(map[string]any{
		"dry_run": true, "executed": false, "tool": tool, "arguments": params,
	})
}

func callDevAppWrite(rt *shortcut.RuntimeContext, tool string, params map[string]any) (map[string]any, error) {
	raw, err := rt.CallMCPWriteDataStrict(productDevApp, tool, params)
	if err != nil {
		return nil, err
	}
	return requireDevAppSuccess(raw, productDevApp+"/"+tool)
}

func verifiedDevAppMutation(action, appID string, resource map[string]any) map[string]any {
	result := map[string]any{
		"action": action, "unifiedAppId": appID, "verified": true,
	}
	if resource != nil {
		result["resource"] = resource
	}
	return result
}

func verifyDeletedDevApp(rt *shortcut.RuntimeContext, appID, appKey string) error {
	if strings.TrimSpace(appKey) == "" {
		return devAppResponseError("devapp/delete_dev_app", "missing_readback_selector", "删除前读回缺少 appKey，无法在删除后绑定稳定排除查询")
	}
	cursor := ""
	seen := map[string]bool{}
	for page := 0; page < devAppMaxReadbackPages; page++ {
		params := map[string]any{"appKey": appKey, "pageSize": 20}
		if cursor != "" {
			params["cursor"] = cursor
		}
		raw, err := rt.CallMCPData(productDevApp, "list_dev_app", params)
		if err != nil {
			return err
		}
		apps, err := listAppProject(raw)
		if err != nil {
			return err
		}
		for _, app := range apps {
			if devAppFirstString(app, "unifiedAppId") == appID {
				return devAppResponseError("devapp/list_dev_app", "delete_readback_present", "删除后精确列表读回仍包含目标应用")
			}
		}
		projection, err := devAppListProjection(raw, "apps", apps, "devapp/list_dev_app")
		if err != nil {
			return err
		}
		more, _ := projection["hasMore"].(bool)
		if !more {
			return nil
		}
		next := devAppFirstString(projection, "nextCursor")
		if next == "" || next == cursor || seen[next] {
			return devAppResponseError("devapp/list_dev_app", "cursor_stall", "删除后排除查询的分页游标未推进")
		}
		seen[next] = true
		cursor = next
	}
	return devAppResponseError("devapp/list_dev_app", "page_limit", "删除后排除查询达到页数上限，无法证明目标已不存在")
}

func changeDevAppStatus(rt *shortcut.RuntimeContext, tool, action, expectedStatus string) error {
	appID := rt.Str("unified-app-id")
	params := map[string]any{"unifiedAppId": appID}
	if previewed, err := devAppWritePreview(rt, tool, params); previewed || err != nil {
		return err
	}
	if _, err := callDevAppWrite(rt, tool, params); err != nil {
		return err
	}
	_, resource, err := readDevAppObject(rt, "get_dev_app", params, map[string]string{"unifiedAppId": appID})
	if err != nil {
		return err
	}
	if err := requireDevAppStringField(resource, "devapp/get_dev_app", expectedStatus, "appStatus", "status"); err != nil {
		return err
	}
	return rt.Output(verifiedDevAppMutation(action, appID, resource))
}

// ---------------------------------------------------------------------------
// 应用主体
// ---------------------------------------------------------------------------

// ListApp maps helper `list_dev_app`.
var ListApp = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+list",
	Product:     productDevApp,
	Description: "查询开放平台企业内部应用列表",
	Intent:      "当你要在开发者后台盘点或定位某个企业内部应用（例如按应用名、appKey、创建人或机器人名搜索，拿到其 unifiedAppId 以便后续查看详情、配置或发布）时使用；支持关键词过滤、排序和分页，返回应用列表。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "devapp",
			Name:           "shortcut_list",
			CanonicalPath:  "devapp.shortcut_list",
			CLIPath:        "devapp +list",
			PrimaryCLIPath: "devapp +list",
		},
		Description: "查询开放平台企业内部应用列表",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "查询开放平台企业内部应用列表",
			UseWhen:      []string{"当你要在开发者后台盘点或定位某个企业内部应用（例如按应用名、appKey、创建人或机器人名搜索，拿到其 unifiedAppId 以便后续查看详情、配置或发布）时使用；支持关键词过滤、排序和分页，返回应用列表。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws devapp +list"},
		},
		Result:     devAppPaginatedProjectionResult("apps", "当前页开放平台应用查询结果"),
		Pagination: devAppCursorPagination(),
	},
	Flags: append([]shortcut.Flag{
		{Name: "name", Type: shortcut.FlagString, Desc: "应用名称关键词"},
		{Name: "app-key", Type: shortcut.FlagString, Desc: "按 appKey/clientId 过滤"},
		{Name: "app-group-id", Type: shortcut.FlagInt, Desc: "应用分组 ID"},
		{Name: "creator", Type: shortcut.FlagString, Desc: "创建人名称关键词"},
		{Name: "robot-name", Type: shortcut.FlagString, Desc: "机器人名称关键词"},
		{Name: "develop-type", Type: shortcut.FlagInt, Desc: "开发类型枚举；不确定时不要传"},
		{Name: "filter-cool-app", Type: shortcut.FlagInt, Desc: "酷应用过滤枚举；不确定时不要传"},
		{Name: "sort-type", Type: shortcut.FlagString, Desc: "排序字段，如 gmt_modified"},
		{Name: "sort-order", Type: shortcut.FlagString, Desc: "排序方向 asc 或 desc"},
	}, cursorFlags...),
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{}
		applyCursor(rt, params)
		if rt.Changed("name") {
			params["name"] = rt.Str("name")
		}
		if rt.Changed("app-key") {
			params["appKey"] = rt.Str("app-key")
		}
		if rt.Changed("app-group-id") {
			params["appGroupId"] = rt.Int("app-group-id")
		}
		if rt.Changed("creator") {
			params["creator"] = rt.Str("creator")
		}
		if rt.Changed("robot-name") {
			params["robotName"] = rt.Str("robot-name")
		}
		if rt.Changed("develop-type") {
			params["developType"] = rt.Int("develop-type")
		}
		if rt.Changed("filter-cool-app") {
			params["filterCoolApp"] = rt.Int("filter-cool-app")
		}
		if rt.Changed("sort-type") {
			params["sortType"] = rt.Str("sort-type")
		}
		if rt.Changed("sort-order") {
			params["sortOrder"] = rt.Str("sort-order")
		}
		data, err := rt.CallMCPData(productDevApp, "list_dev_app", params)
		if err != nil {
			return err
		}
		apps, err := listAppProject(data)
		if err != nil {
			return err
		}
		projection, err := devAppListProjection(data, "apps", apps, "devapp/list_dev_app")
		if err != nil {
			return err
		}
		return rt.Output(projection)
	},
}

// listAppProject reshapes list_dev_app into a clean app list
// ({unifiedAppId, name, appKey, agentId, status, gmtModified}) — output-projection
// clean output projection. The list container and every item are validated
// before projection so unknown response shapes fail closed.
func listAppProject(data map[string]any) ([]map[string]any, error) {
	raw, _, err := requireDevAppCollection(data, "devapp/list_dev_app",
		[]string{"unifiedAppId", "unified_app_id"}, "list", "items", "apps", "appList")
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m := item.(map[string]any)
		row := map[string]any{}
		if v, ok := listAppFirst(m, "unifiedAppId", "unified_app_id"); ok {
			row["unifiedAppId"] = v
		}
		if v, ok := listAppFirst(m, "name", "appName", "app_name"); ok {
			row["name"] = v
		}
		if v, ok := listAppFirst(m, "appKey", "clientId", "app_key", "client_id"); ok {
			row["appKey"] = v
		}
		if v, ok := listAppFirst(m, "agentId", "agent_id"); ok {
			row["agentId"] = v
		}
		if v, ok := listAppFirst(m, "status", "appStatus", "app_status"); ok {
			row["status"] = v
		}
		if v, ok := listAppFirst(m, "gmtModified", "gmt_modified", "modifyTime", "modified_time"); ok {
			row["gmtModified"] = v
		}
		out = append(out, row)
	}
	return out, nil
}

// listAppFirst returns the first present candidate key's value.
func listAppFirst(m map[string]any, keys ...string) (any, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v, true
		}
	}
	return nil, false
}

// GetApp maps helper `get_dev_app`.
var GetApp = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+get",
	Product:     productDevApp,
	Description: "查询开放平台企业内部应用详情",
	Intent:      "当你已知某应用的 unifiedAppId、需要查看它的完整配置信息（如名称、描述、图标、能力开关等）以便核对现状或作为修改前的依据时使用；输入 unifiedAppId，返回单个应用的详情。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "devapp",
			Name:           "shortcut_get",
			CanonicalPath:  "devapp.shortcut_get",
			CLIPath:        "devapp +get",
			PrimaryCLIPath: "devapp +get",
		},
		Description: "查询开放平台企业内部应用详情",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "查询开放平台企业内部应用详情",
			UseWhen:      []string{"当你已知某应用的 unifiedAppId、需要查看它的完整配置信息（如名称、描述、图标、能力开关等）以便核对现状或作为修改前的依据时使用；输入 unifiedAppId，返回单个应用的详情。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws devapp +get --unified-app-id <UNIFIED_APP_ID>"},
		},
		Result: devAppObjectResult(
			contract.ResultOutcomeSuccess,
			contract.ResultOutcomeFailure,
		),
	},
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		appID := rt.Str("unified-app-id")
		return outputDevAppObject(rt, "get_dev_app", map[string]any{"unifiedAppId": appID}, map[string]string{"unifiedAppId": appID})
	},
}

// CreateApp maps helper `create_dev_app`.
var CreateApp = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+create",
	Product:     productDevApp,
	Description: "创建开放平台企业内部应用",
	Intent:      "当你要在开放平台从零新建一个企业内部应用（H5/机器人等的载体）时使用；传入应用名称、可选描述与图标 mediaId，会实际创建出一个新应用并返回其 unifiedAppId 供后续配置。",
	Risk:        shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "user_required", Idempotency: "unknown",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "devapp",
			Name:           "shortcut_create",
			CanonicalPath:  "devapp.shortcut_create",
			CLIPath:        "devapp +create",
			PrimaryCLIPath: "devapp +create",
		},
		Description: "创建开放平台企业内部应用",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "创建开放平台企业内部应用",
			UseWhen:      []string{"当你要在开放平台从零新建一个企业内部应用（H5/机器人等的载体）时使用；传入应用名称、可选描述与图标 mediaId，会实际创建出一个新应用并返回其 unifiedAppId 供后续配置。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws devapp +create --name <NAME>"},
		},
		Result: devAppVerifiedMutationResult(),
	},
	Flags: []shortcut.Flag{
		{Name: "name", Type: shortcut.FlagString, Desc: "应用名称", Required: true},
		{Name: "desc", Type: shortcut.FlagString, Desc: "应用描述"},
		{Name: "icon-media-id", Type: shortcut.FlagString, Desc: "应用图标 mediaId"},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"name": rt.Str("name")}
		if rt.Changed("desc") {
			params["desc"] = rt.Str("desc")
		}
		if rt.Changed("icon-media-id") {
			params["iconMediaId"] = rt.Str("icon-media-id")
		}
		if previewed, err := devAppWritePreview(rt, "create_dev_app", params); previewed || err != nil {
			return err
		}
		receipt, err := callDevAppWrite(rt, "create_dev_app", params)
		if err != nil {
			return err
		}
		created, err := requireDevAppObject(receipt, "devapp/create_dev_app")
		if err != nil {
			return err
		}
		appID := devAppFirstString(created, "unifiedAppId", "unified_app_id")
		if appID == "" {
			return devAppResponseError("devapp/create_dev_app", "missing_resource_id", "创建回执缺少 unifiedAppId，无法绑定精确读回")
		}
		_, resource, err := readDevAppObject(rt, "get_dev_app", map[string]any{"unifiedAppId": appID}, map[string]string{"unifiedAppId": appID})
		if err != nil {
			return err
		}
		if err := requireDevAppStringField(resource, "devapp/get_dev_app", rt.Str("name"), "name", "appName"); err != nil {
			return err
		}
		if rt.Changed("desc") {
			if err := requireDevAppStringField(resource, "devapp/get_dev_app", rt.Str("desc"), "desc", "description"); err != nil {
				return err
			}
		}
		if rt.Changed("icon-media-id") {
			if err := requireDevAppStringField(resource, "devapp/get_dev_app", rt.Str("icon-media-id"), "iconMediaId", "icon"); err != nil {
				return err
			}
		}
		return rt.Output(verifiedDevAppMutation("create", appID, resource))
	},
}

// UpdateApp maps helper `update_dev_app`.
var UpdateApp = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+update",
	Product:     productDevApp,
	Description: "修改开放平台企业内部应用基础信息",
	Intent:      "当你要改动一个已存在应用的基础信息（更名、改描述或换图标）时使用；指定 unifiedAppId 及要更新的字段，会实际写回并覆盖对应的应用基础资料。",
	Risk:        shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "user_required", Idempotency: "unknown",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "devapp",
			Name:           "shortcut_update",
			CanonicalPath:  "devapp.shortcut_update",
			CLIPath:        "devapp +update",
			PrimaryCLIPath: "devapp +update",
		},
		Description: "修改开放平台企业内部应用基础信息",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "修改开放平台企业内部应用基础信息",
			UseWhen:      []string{"当你要改动一个已存在应用的基础信息（更名、改描述或换图标）时使用；指定 unifiedAppId 及要更新的字段，会实际写回并覆盖对应的应用基础资料。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws devapp +update --unified-app-id <UNIFIED_APP_ID>"},
		},
		Result: devAppVerifiedMutationResult(),
	},
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
		{Name: "name", Type: shortcut.FlagString, Desc: "新的应用名称；至少提供一项非空的应用基础信息更新"},
		{Name: "desc", Type: shortcut.FlagString, Desc: "新的应用描述；至少提供一项非空的应用基础信息更新"},
		{Name: "icon-media-id", Type: shortcut.FlagString, Desc: "新的应用图标 mediaId；至少提供一项非空的应用基础信息更新"},
	},
	Constraints: []shortcut.Constraint{{
		Kind:        shortcut.ConstraintCustom,
		Flags:       []string{"name", "desc", "icon-media-id"},
		Description: "至少提供一项非空的应用基础信息更新",
	}},
	Validate: func(rt *shortcut.RuntimeContext) error {
		for _, name := range []string{"name", "desc", "icon-media-id"} {
			if rt.Changed(name) && rt.Str(name) != "" {
				return nil
			}
		}
		return apperrors.NewValidation("至少提供一项待更新字段：--name、--desc 或 --icon-media-id")
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		appID := rt.Str("unified-app-id")
		params := map[string]any{"unifiedAppId": appID}
		if rt.Changed("name") {
			params["name"] = rt.Str("name")
		}
		if rt.Changed("desc") {
			params["desc"] = rt.Str("desc")
		}
		if rt.Changed("icon-media-id") {
			params["iconMediaId"] = rt.Str("icon-media-id")
		}
		if previewed, err := devAppWritePreview(rt, "update_dev_app", params); previewed || err != nil {
			return err
		}
		if _, err := callDevAppWrite(rt, "update_dev_app", params); err != nil {
			return err
		}
		_, resource, err := readDevAppObject(rt, "get_dev_app", map[string]any{"unifiedAppId": appID}, map[string]string{"unifiedAppId": appID})
		if err != nil {
			return err
		}
		for flag, keys := range map[string][]string{
			"name": {"name", "appName"}, "desc": {"desc", "description"}, "icon-media-id": {"iconMediaId", "icon"},
		} {
			if rt.Changed(flag) {
				if err := requireDevAppStringField(resource, "devapp/get_dev_app", rt.Str(flag), keys...); err != nil {
					return err
				}
			}
		}
		return rt.Output(verifiedDevAppMutation("update", appID, resource))
	},
}

// DeleteApp maps helper `delete_dev_app`.
var DeleteApp = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+delete",
	Product:     productDevApp,
	Description: "删除开放平台企业内部应用（不可逆）",
	Intent:      "当你确认要彻底废弃某个企业内部应用时使用；传入 unifiedAppId 会真实且不可逆地删除该应用及其配置，执行前务必确认无误。",
	Risk:        shortcut.RiskHighWrite,
	Safety: contract.SafetySpec{
		Effect: "destructive", Risk: "high",
		Confirmation: "user_required", Idempotency: "unknown",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "devapp",
			Name:           "shortcut_delete",
			CanonicalPath:  "devapp.shortcut_delete",
			CLIPath:        "devapp +delete",
			PrimaryCLIPath: "devapp +delete",
		},
		Description: "删除开放平台企业内部应用（不可逆）",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "删除开放平台企业内部应用（不可逆）",
			UseWhen:      []string{"当你确认要彻底废弃某个企业内部应用时使用；传入 unifiedAppId 会真实且不可逆地删除该应用及其配置，执行前务必确认无误。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws devapp +delete --unified-app-id <UNIFIED_APP_ID>"},
		},
		Result: devAppVerifiedMutationResult(),
	},
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		appID := rt.Str("unified-app-id")
		params := map[string]any{"unifiedAppId": appID}
		if previewed, err := devAppWritePreview(rt, "delete_dev_app", params); previewed || err != nil {
			return err
		}
		_, existing, err := readDevAppObject(rt, "get_dev_app", params, map[string]string{"unifiedAppId": appID})
		if err != nil {
			return err
		}
		appKey := devAppFirstString(existing, "appKey", "clientId")
		if appKey == "" {
			return devAppResponseError("devapp/get_dev_app", "missing_readback_selector", "删除前读回缺少 appKey，拒绝进入不可逆写入")
		}
		if _, err := callDevAppWrite(rt, "delete_dev_app", params); err != nil {
			return err
		}
		if err := verifyDeletedDevApp(rt, appID, appKey); err != nil {
			return err
		}
		return rt.Output(verifiedDevAppMutation("delete", appID, nil))
	},
}

// EnableApp maps helper `enable_dev_app`.
var EnableApp = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+enable",
	Product:     productDevApp,
	Description: "启用开放平台企业内部应用",
	Intent:      "当某个应用处于停用状态、你要让它重新生效可用时使用；传入 unifiedAppId 会实际将应用状态切换为启用。",
	Risk:        shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "user_required", Idempotency: "unknown",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "devapp",
			Name:           "shortcut_enable",
			CanonicalPath:  "devapp.shortcut_enable",
			CLIPath:        "devapp +enable",
			PrimaryCLIPath: "devapp +enable",
		},
		Description: "启用开放平台企业内部应用",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "启用开放平台企业内部应用",
			UseWhen:      []string{"当某个应用处于停用状态、你要让它重新生效可用时使用；传入 unifiedAppId 会实际将应用状态切换为启用。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws devapp +enable --unified-app-id <UNIFIED_APP_ID>"},
		},
		Result: devAppVerifiedMutationResult(),
	},
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return changeDevAppStatus(rt, "enable_dev_app", "enable", "normal")
	},
}

// DisableApp maps helper `disable_dev_app`.
var DisableApp = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+disable",
	Product:     productDevApp,
	Description: "停用开放平台企业内部应用",
	Intent:      "当你要临时下线某个应用、让它对用户不可用又不删除时使用；传入 unifiedAppId 会实际将应用状态切换为停用，可日后再启用恢复。",
	Risk:        shortcut.RiskHighWrite,
	Safety: contract.SafetySpec{
		Effect: "destructive", Risk: "high",
		Confirmation: "user_required", Idempotency: "unknown",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "devapp",
			Name:           "shortcut_disable",
			CanonicalPath:  "devapp.shortcut_disable",
			CLIPath:        "devapp +disable",
			PrimaryCLIPath: "devapp +disable",
		},
		Description: "停用开放平台企业内部应用",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "停用开放平台企业内部应用",
			UseWhen:      []string{"当你要临时下线某个应用、让它对用户不可用又不删除时使用；传入 unifiedAppId 会实际将应用状态切换为停用，可日后再启用恢复。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws devapp +disable --unified-app-id <UNIFIED_APP_ID>"},
		},
		Result: devAppVerifiedMutationResult(),
	},
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return changeDevAppStatus(rt, "disable_dev_app", "disable", "disabled")
	},
}

// GetCredentials maps helper `get_dev_app_credentials`.
var GetCredentials = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+credentials-get",
	Product:     productDevApp,
	Description: "读取开放平台应用凭证",
	Intent:      "当你需要拿到某应用的鉴权凭证（如 clientId/AppKey、clientSecret/AppSecret）以便在代码或调试中调用开放平台接口时使用；输入 unifiedAppId，返回该应用的凭证信息。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "medium",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "devapp",
			Name:           "shortcut_credentials_get",
			CanonicalPath:  "devapp.shortcut_credentials_get",
			CLIPath:        "devapp +credentials-get",
			PrimaryCLIPath: "devapp +credentials-get",
		},
		Description: "读取开放平台应用凭证",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed adapter requires an explicit success marker, a non-empty credential object, exact unifiedAppId, a public client identifier, and a non-empty secret before unified output; secret fields are declared sensitive.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "读取开放平台应用凭证",
			UseWhen:      []string{"当你需要拿到某应用的鉴权凭证（如 clientId/AppKey、clientSecret/AppSecret）以便在代码或调试中调用开放平台接口时使用；输入 unifiedAppId，返回该应用的凭证信息。"},
			AvoidWhen:    []string{"只需应用元数据时使用 +get；不要把返回密钥写入文档、日志、示例或聊天正文"},
			Examples:     []string{"dws devapp +credentials-get --unified-app-id <UNIFIED_APP_ID>"},
		},
		Result: devAppCredentialResult(),
	},
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		appID := rt.Str("unified-app-id")
		_, credentials, err := readDevAppObject(rt, "get_dev_app_credentials", map[string]any{"unifiedAppId": appID}, map[string]string{"unifiedAppId": appID})
		if err != nil {
			return err
		}
		if devAppFirstString(credentials, "appKey", "clientId") == "" {
			return devAppResponseError("devapp/get_dev_app_credentials", "missing_client_id", "凭证响应缺少非空 appKey/clientId")
		}
		if devAppFirstString(credentials, "appSecret", "clientSecret", "secret") == "" {
			return devAppResponseError("devapp/get_dev_app_credentials", "missing_client_secret", "凭证响应缺少非空 appSecret/clientSecret")
		}
		return rt.Output(credentials)
	},
}

// ---------------------------------------------------------------------------
// 网页应用配置
// ---------------------------------------------------------------------------

// WebappGet maps helper `get_extension_webapp_config`.
var WebappGet = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+webapp-get",
	Product:     productDevApp,
	Description: "查询网页应用配置",
	Intent:      "当你要查看某应用的网页（H5）能力现状，如移动端/PC 首页地址、管理后台地址等，以便核对或作为改配置前的参考时使用；输入 unifiedAppId，返回当前网页应用配置。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "devapp",
			Name:           "shortcut_webapp_get",
			CanonicalPath:  "devapp.shortcut_webapp_get",
			CLIPath:        "devapp +webapp-get",
			PrimaryCLIPath: "devapp +webapp-get",
		},
		Description: "查询网页应用配置",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "查询网页应用配置",
			UseWhen:      []string{"当你要查看某应用的网页（H5）能力现状，如移动端/PC 首页地址、管理后台地址等，以便核对或作为改配置前的参考时使用；输入 unifiedAppId，返回当前网页应用配置。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws devapp +webapp-get --unified-app-id <UNIFIED_APP_ID>"},
		},
		Result: devAppObjectResult(
			contract.ResultOutcomeSuccess,
			contract.ResultOutcomeFailure,
		),
	},
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		appID := rt.Str("unified-app-id")
		return outputDevAppObject(rt, "get_extension_webapp_config", map[string]any{"unifiedAppId": appID}, map[string]string{"unifiedAppId": appID})
	},
}

// WebappConfig maps helper `set_extension_webapp_config`.
var WebappConfig = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+webapp-config",
	Product:     productDevApp,
	Description: "配置网页应用能力",
	Intent:      "当你要为应用开通或调整网页（H5）入口，如设置移动端/PC 端首页 URL、管理后台地址或页面类型时使用；指定 unifiedAppId 及相应地址，会实际写入该应用的网页应用配置。",
	Risk:        shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "user_required", Idempotency: "unknown",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "devapp",
			Name:           "shortcut_webapp_config",
			CanonicalPath:  "devapp.shortcut_webapp_config",
			CLIPath:        "devapp +webapp-config",
			PrimaryCLIPath: "devapp +webapp-config",
		},
		Description: "配置网页应用能力",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "配置网页应用能力",
			UseWhen:      []string{"当你要为应用开通或调整网页（H5）入口，如设置移动端/PC 端首页 URL、管理后台地址或页面类型时使用；指定 unifiedAppId 及相应地址，会实际写入该应用的网页应用配置。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws devapp +webapp-config --unified-app-id <UNIFIED_APP_ID>"},
		},
		Result: devAppVerifiedMutationResult(),
	},
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
		{Name: "h5-page-type", Type: shortcut.FlagString, Desc: "网页应用生效端/页面类型；至少提供一项非空的网页应用配置"},
		{Name: "homepage-url", Type: shortcut.FlagString, Desc: "移动端首页地址；至少提供一项非空的网页应用配置"},
		{Name: "pc-homepage-url", Type: shortcut.FlagString, Desc: "PC 端首页地址；至少提供一项非空的网页应用配置"},
		{Name: "omp-url", Type: shortcut.FlagString, Desc: "管理后台地址；至少提供一项非空的网页应用配置"},
	},
	Constraints: []shortcut.Constraint{{
		Kind:        shortcut.ConstraintCustom,
		Flags:       []string{"h5-page-type", "homepage-url", "pc-homepage-url", "omp-url"},
		Description: "至少提供一项非空的网页应用配置",
	}},
	Validate: func(rt *shortcut.RuntimeContext) error {
		for _, name := range []string{"h5-page-type", "homepage-url", "pc-homepage-url", "omp-url"} {
			if rt.Changed(name) && rt.Str(name) != "" {
				return nil
			}
		}
		return apperrors.NewValidation("至少提供一项网页应用配置：--h5-page-type、--homepage-url、--pc-homepage-url 或 --omp-url")
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		appID := rt.Str("unified-app-id")
		params := map[string]any{"unifiedAppId": appID}
		if rt.Changed("h5-page-type") {
			params["h5PageType"] = rt.Str("h5-page-type")
		}
		if rt.Changed("homepage-url") {
			params["homepageUrl"] = rt.Str("homepage-url")
		}
		if rt.Changed("pc-homepage-url") {
			params["pcHomepageUrl"] = rt.Str("pc-homepage-url")
		}
		if rt.Changed("omp-url") {
			params["ompUrl"] = rt.Str("omp-url")
		}
		if previewed, err := devAppWritePreview(rt, "set_extension_webapp_config", params); previewed || err != nil {
			return err
		}
		if _, err := callDevAppWrite(rt, "set_extension_webapp_config", params); err != nil {
			return err
		}
		_, resource, err := readDevAppObject(rt, "get_extension_webapp_config", map[string]any{"unifiedAppId": appID}, map[string]string{"unifiedAppId": appID})
		if err != nil {
			return err
		}
		for flag, keys := range map[string][]string{
			"h5-page-type": {"h5PageType"}, "homepage-url": {"homepageUrl"},
			"pc-homepage-url": {"pcHomepageUrl"}, "omp-url": {"ompUrl"},
		} {
			if rt.Changed(flag) {
				if err := requireDevAppStringField(resource, "devapp/get_extension_webapp_config", rt.Str(flag), keys...); err != nil {
					return err
				}
			}
		}
		return rt.Output(verifiedDevAppMutation("webapp_config", appID, resource))
	},
}

// ---------------------------------------------------------------------------
// 权限
// ---------------------------------------------------------------------------

// PermissionList maps helper `list_dev_app_permissions`.
var PermissionList = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+permission-list",
	Product:     productDevApp,
	Description: "查询开放平台应用权限列表",
	Intent:      "当你要查看某应用已申请/可申请的 API 权限点及其授权状态（用于排查接口报权限错、或确认某 scopeValue 是否已开通）时使用；可按关键词、scopeValue、授权状态等过滤，返回权限点列表。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "devapp",
			Name:           "shortcut_permission_list",
			CanonicalPath:  "devapp.shortcut_permission_list",
			CLIPath:        "devapp +permission-list",
			PrimaryCLIPath: "devapp +permission-list",
		},
		Description: "查询开放平台应用权限列表",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "查询开放平台应用权限列表",
			UseWhen:      []string{"当你要查看某应用已申请/可申请的 API 权限点及其授权状态（用于排查接口报权限错、或确认某 scopeValue 是否已开通）时使用；可按关键词、scopeValue、授权状态等过滤，返回权限点列表。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws devapp +permission-list --unified-app-id <UNIFIED_APP_ID>"},
		},
		Result:     devAppPaginatedProjectionResult("permissions", "当前页开放平台应用权限查询结果"),
		Pagination: devAppCursorPagination(),
	},
	Flags: append([]shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
		{Name: "keyword", Type: shortcut.FlagString, Desc: "权限名、权限点、接口名关键词"},
		{Name: "scope-value", Type: shortcut.FlagString, Desc: "精确权限点 scopeValue"},
		{Name: "auth-status", Type: shortcut.FlagString, Default: "ALL", Desc: "权限状态：ALL、AUTHED、UNAUTHED"},
		{Name: "scope-type", Type: shortcut.FlagString, Desc: "权限一级类型：APP 或 SNS"},
		{Name: "api-status", Type: shortcut.FlagString, Desc: "开发者后台 apiStatus 过滤"},
	}, cursorFlags...),
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"unifiedAppId": rt.Str("unified-app-id")}
		if rt.Changed("keyword") {
			params["keyword"] = rt.Str("keyword")
		}
		if rt.Changed("scope-value") {
			params["scopeValue"] = rt.Str("scope-value")
		}
		if v := strings.ToUpper(rt.Str("auth-status")); v != "" {
			params["authStatus"] = v
		}
		if rt.Changed("scope-type") {
			params["scopeType"] = strings.ToUpper(rt.Str("scope-type"))
		}
		if rt.Changed("api-status") {
			params["apiStatus"] = rt.Str("api-status")
		}
		applyCursor(rt, params)
		data, err := rt.CallMCPData(productDevApp, "list_dev_app_permissions", params)
		if err != nil {
			return err
		}
		permissions, err := permissionListProject(data)
		if err != nil {
			return err
		}
		projection, err := devAppListProjection(data, "permissions", permissions, "devapp/list_dev_app_permissions")
		if err != nil {
			return err
		}
		return rt.Output(projection)
	},
}

// permissionListProject reshapes list_dev_app_permissions into a clean
// permission-point list ({scopeValue, scopeName, apiName, authStatus, scopeType})
// — clean output projection. The list container and per-item field
// names are probed across reviewed candidate keys, while missing, mistyped, or
// malformed collections fail closed.
func permissionListProject(data map[string]any) ([]map[string]any, error) {
	raw, _, err := requireDevAppCollection(data, "devapp/list_dev_app_permissions",
		[]string{"scopeValue", "scope_value", "permissionCode", "code"},
		"list", "items", "permissions", "permissionList", "scopes")
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m := item.(map[string]any)
		row := map[string]any{}
		if v, ok := permissionListFirst(m, "scopeValue", "scope_value", "permissionCode", "code"); ok {
			row["scopeValue"] = v
		}
		if v, ok := permissionListFirst(m, "scopeName", "scope_name", "permissionName", "name"); ok {
			row["scopeName"] = v
		}
		if v, ok := permissionListFirst(m, "apiName", "api_name", "interfaceName"); ok {
			row["apiName"] = v
		}
		if v, ok := permissionListFirst(m, "authStatus", "auth_status", "status"); ok {
			row["authStatus"] = v
		}
		if v, ok := permissionListFirst(m, "scopeType", "scope_type"); ok {
			row["scopeType"] = v
		}
		out = append(out, row)
	}
	return out, nil
}

// permissionListFirst returns the first present candidate key's value.
func permissionListFirst(m map[string]any, keys ...string) (any, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v, true
		}
	}
	return nil, false
}

// PermissionAdd maps helper `apply_dev_app_permissions`.
var PermissionAdd = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+permission-add",
	Product:     productDevApp,
	Description: "申请开放平台应用权限点",
	Intent:      "当应用调用某接口报缺少权限、你要为它开通一批 API 权限点时使用；传入 unifiedAppId 和 scopeValue 列表，会实际为该应用申请/授予这些权限。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
		{Name: "scope-values", Type: shortcut.FlagStringSlice, Desc: "权限点 scopeValue 列表", Required: true},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"unifiedAppId": rt.Str("unified-app-id"),
			"scopeValues":  rt.StrSlice("scope-values"),
		}
		return rt.CallMCP("apply_dev_app_permissions", params)
	},
}

// PermissionRemove maps helper `remove_dev_app_permissions`.
var PermissionRemove = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+permission-remove",
	Product:     productDevApp,
	Description: "取消开放平台应用权限点",
	Intent:      "当你要收回应用已开通的某些 API 权限（如安全收敛、下线不再使用的接口）时使用；传入 unifiedAppId 和待取消的 scopeValue 列表，会实际移除这些权限授权。",
	Risk:        shortcut.RiskHighWrite,
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
		{Name: "scope-values", Type: shortcut.FlagStringSlice, Desc: "待取消权限点 scopeValue 列表", Required: true},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"unifiedAppId": rt.Str("unified-app-id"),
			"scopeValues":  rt.StrSlice("scope-values"),
		}
		return rt.CallMCP("remove_dev_app_permissions", params)
	},
}

// ---------------------------------------------------------------------------
// 成员
// ---------------------------------------------------------------------------

// MemberList maps helper `list_dev_app_members`.
var MemberList = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+member-list",
	Product:     productDevApp,
	Description: "查询开放平台应用成员",
	Intent:      "当你要查看某应用有哪些成员及其角色（如谁是开发者/管理员），用于核对协作人员或权限归属时使用；输入 unifiedAppId，返回成员列表。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "devapp",
			Name:           "shortcut_member_list",
			CanonicalPath:  "devapp.shortcut_member_list",
			CLIPath:        "devapp +member-list",
			PrimaryCLIPath: "devapp +member-list",
		},
		Description: "查询开放平台应用成员",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "查询开放平台应用成员",
			UseWhen:      []string{"当你要查看某应用有哪些成员及其角色（如谁是开发者/管理员），用于核对协作人员或权限归属时使用；输入 unifiedAppId，返回成员列表。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws devapp +member-list --unified-app-id <UNIFIED_APP_ID> --user-id <USER_ID>"},
		},
		Result: devAppMemberListResult(),
	},
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
		{Name: "user-id", Type: shortcut.FlagString, Desc: "可选稳定 userId；由 Shortcut 在严格验证完整成员数组后做精确等值筛选"},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		raw, err := rt.CallMCPData(productDevApp, "list_dev_app_members", map[string]any{"unifiedAppId": rt.Str("unified-app-id")})
		if err != nil {
			return err
		}
		members, _, err := requireDevAppCollection(raw, "devapp/list_dev_app_members", []string{"userId", "user_id"}, "members", "items", "list")
		if err != nil {
			return err
		}
		wantUser := strings.TrimSpace(rt.Str("user-id"))
		out := projectDevAppMembers(members, wantUser)
		return rt.Output(map[string]any{"count": len(out), "members": out})
	},
}

func projectDevAppMembers(members []any, wantUser string) []map[string]any {
	out := make([]map[string]any, 0, len(members))
	for _, value := range members {
		member := value.(map[string]any)
		if wantUser != "" && devAppFirstString(member, "userId", "user_id") != wantUser {
			continue
		}
		out = append(out, member)
	}
	return out
}

func readDevAppMembers(rt *shortcut.RuntimeContext, appID string) ([]any, error) {
	raw, err := rt.CallMCPData(productDevApp, "list_dev_app_members", map[string]any{"unifiedAppId": appID})
	if err != nil {
		return nil, err
	}
	members, _, err := requireDevAppCollection(raw, "devapp/list_dev_app_members",
		[]string{"userId", "user_id"}, "members", "items", "list")
	return members, err
}

func verifyDevAppMembers(rt *shortcut.RuntimeContext, appID string, userIDs []string, memberType string, present bool) error {
	members, err := readDevAppMembers(rt, appID)
	if err != nil {
		return err
	}
	want := make(map[string]bool, len(userIDs))
	for _, userID := range userIDs {
		want[userID] = true
	}
	found := make(map[string]int, len(userIDs))
	for _, value := range members {
		member := value.(map[string]any)
		userID := devAppFirstString(member, "userId", "user_id")
		if !want[userID] {
			continue
		}
		found[userID]++
		if found[userID] > 1 {
			return devAppResponseError("devapp/list_dev_app_members", "duplicate_readback_identity", "成员读回包含重复稳定 userId，无法证明唯一终态")
		}
		if present {
			if err := requireDevAppStringField(member, "devapp/list_dev_app_members", memberType, "memberType", "member_type"); err != nil {
				return err
			}
		}
	}
	for _, userID := range userIDs {
		if present && found[userID] != 1 {
			return devAppResponseError("devapp/list_dev_app_members", "member_readback_missing", "添加后成员列表未精确包含全部目标 userId")
		}
		if !present && found[userID] != 0 {
			return devAppResponseError("devapp/list_dev_app_members", "member_remove_readback_present", "移除后成员列表仍包含目标 userId")
		}
	}
	return nil
}

func validatedDevAppMemberInputs(rt *shortcut.RuntimeContext) ([]string, string, error) {
	userIDs, err := validatedDevAppValues(rt.StrSlice("user-ids"), "--user-ids")
	if err != nil {
		return nil, "", err
	}
	memberType, err := validatedDevAppMemberType(rt.Str("member-type"))
	return userIDs, memberType, err
}

func validatedDevAppMemberType(value string) (string, error) {
	memberType := strings.TrimSpace(value)
	if memberType == "" {
		return "", apperrors.NewValidation("--member-type 不能为空")
	}
	return memberType, nil
}

// MemberAdd maps helper `add_dev_app_members`.
var MemberAdd = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+member-add",
	Product:     productDevApp,
	Description: "添加开放平台应用成员",
	Intent:      "当你要给某应用增加协作人员并要求按稳定 userId 逐项确认成员角色已写回时使用；传入 unifiedAppId、userId 列表和成员类型（如 DEVELOPER）。",
	Risk:        shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "user_required", Idempotency: "unknown",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "devapp",
			Name:           "shortcut_member_add",
			CanonicalPath:  "devapp.shortcut_member_add",
			CLIPath:        "devapp +member-add",
			PrimaryCLIPath: "devapp +member-add",
		},
		Description: "添加开放平台应用成员",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "添加开放平台应用成员",
			UseWhen:      []string{"当你要给某应用增加协作人员并要求按稳定 userId 逐项确认成员角色已写回时使用；传入 unifiedAppId、userId 列表和成员类型（如 DEVELOPER）。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws devapp +member-add --unified-app-id <UNIFIED_APP_ID> --user-ids <VALUES> --member-type <MEMBER_TYPE>"},
		},
		Result: devAppVerifiedMutationResult(),
	},
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
		{Name: "user-ids", Type: shortcut.FlagStringSlice, Desc: "成员 userId 列表，不能为空且不能重复", Required: true},
		{Name: "member-type", Type: shortcut.FlagString, Desc: "成员类型，如 DEVELOPER，不能为空", Required: true},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintCustom, Flags: []string{"user-ids"}, Description: "userId 列表不能为空且不能重复"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"member-type"}, Description: "成员类型不能为空"},
	},
	Validate: func(rt *shortcut.RuntimeContext) error {
		_, _, err := validatedDevAppMemberInputs(rt)
		return err
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		userIDs, memberType, _ := validatedDevAppMemberInputs(rt)
		appID := rt.Str("unified-app-id")
		params := map[string]any{
			"unifiedAppId": appID,
			"userIds":      userIDs,
			"memberType":   memberType,
		}
		if previewed, err := devAppWritePreview(rt, "add_dev_app_members", params); previewed || err != nil {
			return err
		}
		if _, err := callDevAppWrite(rt, "add_dev_app_members", params); err != nil {
			return err
		}
		if err := verifyDevAppMembers(rt, appID, userIDs, memberType, true); err != nil {
			return err
		}
		return rt.Output(verifiedDevAppMutation("member_add", appID, map[string]any{
			"count": len(userIDs), "memberType": memberType,
		}))
	},
}

// MemberRemove maps helper `remove_dev_app_members`.
var MemberRemove = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+member-remove",
	Product:     productDevApp,
	Description: "移除开放平台应用成员",
	Intent:      "当某人不再参与、你要取消其应用成员身份，并要求按稳定 userId 逐项确认目标已不在成员列表中时使用；传入 unifiedAppId、userId 列表和成员类型。",
	Risk:        shortcut.RiskHighWrite,
	Safety: contract.SafetySpec{
		Effect: "destructive", Risk: "high",
		Confirmation: "user_required", Idempotency: "unknown",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "devapp",
			Name:           "shortcut_member_remove",
			CanonicalPath:  "devapp.shortcut_member_remove",
			CLIPath:        "devapp +member-remove",
			PrimaryCLIPath: "devapp +member-remove",
		},
		Description: "移除开放平台应用成员",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "移除开放平台应用成员",
			UseWhen:      []string{"当某人不再参与、你要取消其应用成员身份，并要求按稳定 userId 逐项确认目标已不在成员列表中时使用；传入 unifiedAppId、userId 列表和成员类型。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws devapp +member-remove --unified-app-id <UNIFIED_APP_ID> --user-ids <VALUES> --member-type <MEMBER_TYPE>"},
		},
		Result: devAppVerifiedMutationResult(),
	},
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
		{Name: "user-ids", Type: shortcut.FlagStringSlice, Desc: "成员 userId 列表，不能为空且不能重复", Required: true},
		{Name: "member-type", Type: shortcut.FlagString, Desc: "成员类型，如 DEVELOPER，不能为空", Required: true},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintCustom, Flags: []string{"user-ids"}, Description: "userId 列表不能为空且不能重复"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"member-type"}, Description: "成员类型不能为空"},
	},
	Validate: func(rt *shortcut.RuntimeContext) error {
		_, _, err := validatedDevAppMemberInputs(rt)
		return err
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		userIDs, memberType, _ := validatedDevAppMemberInputs(rt)
		appID := rt.Str("unified-app-id")
		params := map[string]any{
			"unifiedAppId": appID,
			"userIds":      userIDs,
			"memberType":   memberType,
		}
		if previewed, err := devAppWritePreview(rt, "remove_dev_app_members", params); previewed || err != nil {
			return err
		}
		if _, err := callDevAppWrite(rt, "remove_dev_app_members", params); err != nil {
			return err
		}
		if err := verifyDevAppMembers(rt, appID, userIDs, memberType, false); err != nil {
			return err
		}
		return rt.Output(verifiedDevAppMutation("member_remove", appID, map[string]any{
			"count": len(userIDs), "memberType": memberType,
		}))
	},
}

// ---------------------------------------------------------------------------
// 安全配置
// ---------------------------------------------------------------------------

// SecurityConfig maps helper `update_dev_app_security_config`.
var SecurityConfig = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+security-config",
	Product:     productDevApp,
	Description: "更新开放平台应用安全配置（整组覆盖，非追加）",
	Intent:      "当你要设置应用的安全策略，如出口 IP 白名单、登录重定向 URL、端内免登地址时使用；注意每项传入的是整组值会全量覆盖旧配置（非追加），所以要一次性传全，常用于配置 OAuth 回调或加固网络白名单。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
		{Name: "ip-whitelist", Type: shortcut.FlagStringSlice, Desc: "出口 IP 白名单（整组覆盖）"},
		{Name: "redirect-urls", Type: shortcut.FlagStringSlice, Desc: "登录重定向 URL（整组覆盖）"},
		{Name: "sso-urls", Type: shortcut.FlagStringSlice, Desc: "端内免登地址（整组覆盖）"},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"unifiedAppId": rt.Str("unified-app-id")}
		if rt.Changed("ip-whitelist") {
			params["ipWhitelist"] = rt.StrSlice("ip-whitelist")
		}
		if rt.Changed("redirect-urls") {
			params["redirectUrls"] = rt.StrSlice("redirect-urls")
		}
		if rt.Changed("sso-urls") {
			params["ssoUrls"] = rt.StrSlice("sso-urls")
		}
		return rt.CallMCP("update_dev_app_security_config", params)
	},
}

// ---------------------------------------------------------------------------
// 机器人能力
// ---------------------------------------------------------------------------

// RobotGet maps helper `get_extension_robot_config`.
var RobotGet = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+robot-get",
	Product:     productDevApp,
	Description: "查询现有应用的机器人配置",
	Intent:      "当你要查看某应用已有的机器人配置（名称、回调地址、模式 HTTPS/STREAM/AISKILL、技能等）以核对现状或作为改配置前的依据时使用；输入 unifiedAppId，返回当前机器人配置。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "devapp",
			Name:           "shortcut_robot_get",
			CanonicalPath:  "devapp.shortcut_robot_get",
			CLIPath:        "devapp +robot-get",
			PrimaryCLIPath: "devapp +robot-get",
		},
		Description: "查询现有应用的机器人配置",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "查询现有应用的机器人配置",
			UseWhen:      []string{"当你要查看某应用已有的机器人配置（名称、回调地址、模式 HTTPS/STREAM/AISKILL、技能等）以核对现状或作为改配置前的依据时使用；输入 unifiedAppId，返回当前机器人配置。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws devapp +robot-get --unified-app-id <UNIFIED_APP_ID>"},
		},
		Result: devAppObjectResult(
			contract.ResultOutcomeSuccess,
			contract.ResultOutcomeFailure,
		),
	},
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		appID := rt.Str("unified-app-id")
		return outputDevAppObject(rt, "get_extension_robot_config", map[string]any{"unifiedAppId": appID}, map[string]string{"unifiedAppId": appID})
	},
}

// RobotConfig maps helper `set_extension_robot_config` (upsert). The i18n
// JSON-object params are intentionally omitted (see package doc).
var RobotConfig = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+robot-config",
	Product:     productDevApp,
	Description: "创建或更新现有应用的机器人配置（upsert）",
	Intent:      "当你要为应用开通机器人或调整其机器人设置（改名称/简介/图标、设消息与事件回调地址、切换 HTTPS/STREAM/AISKILL 模式、配技能、是否自动加权限或关 SSL 校验）时使用；按 unifiedAppId 以 upsert 方式写入机器人配置，会实际生效。",
	Risk:        shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "user_required", Idempotency: "unknown",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID: "devapp", Name: "shortcut_robot_config",
			CanonicalPath: "devapp.shortcut_robot_config", CLIPath: "devapp +robot-config", PrimaryCLIPath: "devapp +robot-config",
		},
		Description: "创建或更新现有应用的机器人配置并精确读回",
		Interface: &contract.InterfaceSpec{
			Mode: "composite", Availability: "available",
			Reason: "Reviewed adapter rejects an empty update, requires terminal success, then reads the same unifiedAppId and compares every changed scalar field before returning verified output.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "创建或更新现有应用的机器人配置（upsert）",
			UseWhen:      []string{"当你要为应用开通机器人或调整其机器人设置（改名称/简介/图标、设消息与事件回调地址、切换 HTTPS/STREAM/AISKILL 模式、配技能、是否自动加权限或关 SSL 校验）时使用；按 unifiedAppId 以 upsert 方式写入机器人配置，会实际生效。"},
			AvoidWhen:    []string{"只需查看现状使用 +robot-get；仅切换已配置机器人的在线状态使用 +robot-enable 或 +robot-disable"},
			Examples:     []string{"dws devapp +robot-config --unified-app-id <UNIFIED_APP_ID> --name <BOT_NAME> --mode STREAM"},
		},
		Result: devAppVerifiedMutationResult(),
	},
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
		{Name: "name", Type: shortcut.FlagString, Desc: "机器人名称；至少提供一项非空机器人配置；布尔开关显式传入 false 也算配置"},
		{Name: "brief", Type: shortcut.FlagString, Desc: "机器人简介；至少提供一项非空机器人配置；布尔开关显式传入 false 也算配置"},
		{Name: "desc", Type: shortcut.FlagString, Desc: "机器人描述；至少提供一项非空机器人配置；布尔开关显式传入 false 也算配置"},
		{Name: "icon-media-id", Type: shortcut.FlagString, Desc: "机器人图标 mediaId；至少提供一项非空机器人配置；布尔开关显式传入 false 也算配置"},
		{Name: "outgoing-url", Type: shortcut.FlagString, Desc: "消息回调地址；至少提供一项非空机器人配置；布尔开关显式传入 false 也算配置"},
		{Name: "event-callback-url", Type: shortcut.FlagString, Desc: "事件回调地址；至少提供一项非空机器人配置；布尔开关显式传入 false 也算配置"},
		{Name: "mode", Type: shortcut.FlagString, Enum: []string{"HTTPS", "STREAM", "AISKILL"}, Desc: "机器人模式：HTTPS / STREAM / AISKILL；至少提供一项非空机器人配置；布尔开关显式传入 false 也算配置"},
		{Name: "skills", Type: shortcut.FlagStringSlice, Desc: "技能列表；至少提供一项非空机器人配置；布尔开关显式传入 false 也算配置"},
		{Name: "add-scope", Type: shortcut.FlagBool, Desc: "是否自动添加机器人相关权限；至少提供一项非空机器人配置；布尔开关显式传入 false 也算配置"},
		{Name: "disable-ssl-verify", Type: shortcut.FlagBool, Desc: "回调地址是否关闭 SSL 校验；至少提供一项非空机器人配置；布尔开关显式传入 false 也算配置"},
	},
	Constraints: []shortcut.Constraint{{
		Kind:        shortcut.ConstraintCustom,
		Flags:       []string{"name", "brief", "desc", "icon-media-id", "outgoing-url", "event-callback-url", "mode", "skills", "add-scope", "disable-ssl-verify"},
		Description: "至少提供一项非空机器人配置；布尔开关显式传入 false 也算配置",
	}},
	Validate: func(rt *shortcut.RuntimeContext) error {
		for _, name := range []string{"name", "brief", "desc", "icon-media-id", "outgoing-url", "event-callback-url", "mode"} {
			if rt.Changed(name) && strings.TrimSpace(rt.Str(name)) != "" {
				return nil
			}
		}
		if rt.Changed("skills") && len(rt.StrSlice("skills")) > 0 {
			return nil
		}
		if rt.Changed("add-scope") || rt.Changed("disable-ssl-verify") {
			return nil
		}
		return apperrors.NewValidation("至少提供一项非空机器人配置")
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		appID := rt.Str("unified-app-id")
		params := map[string]any{"unifiedAppId": appID}
		if rt.Changed("name") {
			params["name"] = rt.Str("name")
		}
		if rt.Changed("brief") {
			params["brief"] = rt.Str("brief")
		}
		if rt.Changed("desc") {
			params["desc"] = rt.Str("desc")
		}
		if rt.Changed("icon-media-id") {
			params["iconMediaId"] = rt.Str("icon-media-id")
		}
		if rt.Changed("outgoing-url") {
			params["outgoingUrl"] = rt.Str("outgoing-url")
		}
		if rt.Changed("event-callback-url") {
			params["eventCallbackUrl"] = rt.Str("event-callback-url")
		}
		if rt.Changed("mode") {
			params["mode"] = strings.ToUpper(rt.Str("mode"))
		}
		if rt.Changed("skills") {
			params["skills"] = rt.StrSlice("skills")
		}
		if rt.Changed("add-scope") {
			params["addScope"] = rt.Bool("add-scope")
		}
		if rt.Changed("disable-ssl-verify") {
			params["disableSSLVerify"] = rt.Bool("disable-ssl-verify")
		}
		if previewed, err := devAppWritePreview(rt, "set_extension_robot_config", params); previewed || err != nil {
			return err
		}
		if _, err := callDevAppWrite(rt, "set_extension_robot_config", params); err != nil {
			return err
		}
		_, resource, err := readDevAppObject(rt, "get_extension_robot_config", map[string]any{"unifiedAppId": appID}, map[string]string{"unifiedAppId": appID})
		if err != nil {
			return err
		}
		for flag, keys := range map[string][]string{
			"name": {"name"}, "brief": {"brief"}, "desc": {"desc", "description"},
			"icon-media-id": {"iconMediaId"}, "outgoing-url": {"outgoingUrl"},
			"event-callback-url": {"eventCallbackUrl"}, "mode": {"mode"},
		} {
			if !rt.Changed(flag) {
				continue
			}
			want := rt.Str(flag)
			if flag == "mode" {
				want = strings.ToUpper(want)
			}
			if err := requireDevAppStringField(resource, "devapp/get_extension_robot_config", want, keys...); err != nil {
				return err
			}
		}
		for flag, key := range map[string]string{"add-scope": "addScope", "disable-ssl-verify": "disableSSLVerify"} {
			if rt.Changed(flag) {
				if err := requireDevAppFields(resource, "devapp/get_extension_robot_config", map[string]any{key: rt.Bool(flag)}); err != nil {
					return err
				}
			}
		}
		if rt.Changed("skills") {
			if err := requireDevAppFields(resource, "devapp/get_extension_robot_config", map[string]any{"skills": rt.StrSlice("skills")}); err != nil {
				return err
			}
		}
		return rt.Output(verifiedDevAppMutation("robot_config", appID, resource))
	},
}

// RobotEnable maps helper `enable_dev_app_robot`.
var RobotEnable = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+robot-enable",
	Product:     productDevApp,
	Description: "启用现有应用机器人能力（纯启用，无需配置字段）",
	Intent:      "当应用已经具有可启用的机器人配置、你要把它切换到 ONLINE 时使用；写后会读取同一 unifiedAppId 并只在 robotStatus=ONLINE 时成功。",
	Risk:        shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "user_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID: "devapp", Name: "shortcut_robot_enable",
			CanonicalPath: "devapp.shortcut_robot_enable", CLIPath: "devapp +robot-enable", PrimaryCLIPath: "devapp +robot-enable",
		},
		Description: "启用已配置的应用机器人并读回 ONLINE 终态",
		Interface:   &contract.InterfaceSpec{Mode: "composite", Availability: "available", Reason: "Reviewed adapter requires terminal success and exact get_extension_robot_config readback for the same unifiedAppId with robotStatus=ONLINE."},
		Selection: contract.SelectionSpec{
			AgentSummary: "启用现有应用机器人能力（纯启用，无需配置字段）",
			UseWhen:      []string{"当应用已经具有可启用的机器人配置、你要把它切换到 ONLINE 时使用；写后会读取同一 unifiedAppId 并只在 robotStatus=ONLINE 时成功。"},
			AvoidWhen:    []string{"需要修改机器人字段时使用 +robot-config；只查看状态使用 +robot-get"},
			Examples:     []string{"dws devapp +robot-enable --unified-app-id <UNIFIED_APP_ID>"},
		},
		Result: devAppVerifiedMutationResult(),
	},
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return changeDevAppRobotStatus(rt, "enable_dev_app_robot", "robot_enable", "ONLINE")
	},
}

// RobotDisable maps helper `disable_dev_app_robot`.
var RobotDisable = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+robot-disable",
	Product:     productDevApp,
	Description: "停用现有应用的机器人能力",
	Intent:      "当你要下线某应用的机器人、不再收发机器人消息时使用；当前平台读回终态为 UNCONFIGURED，因此重新上线前可能需要先用 +robot-config 恢复配置。",
	Risk:        shortcut.RiskHighWrite,
	Safety: contract.SafetySpec{
		Effect: "destructive", Risk: "high",
		Confirmation: "user_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID: "devapp", Name: "shortcut_robot_disable",
			CanonicalPath: "devapp.shortcut_robot_disable", CLIPath: "devapp +robot-disable", PrimaryCLIPath: "devapp +robot-disable",
		},
		Description: "下线应用机器人并读回 UNCONFIGURED 终态",
		Interface:   &contract.InterfaceSpec{Mode: "composite", Availability: "available", Reason: "Reviewed adapter requires terminal success and exact get_extension_robot_config readback for the same unifiedAppId with robotStatus=UNCONFIGURED; it does not promise that configuration remains reusable."},
		Selection: contract.SelectionSpec{
			AgentSummary: "停用现有应用的机器人能力",
			UseWhen:      []string{"当你要下线某应用的机器人、不再收发机器人消息时使用；当前平台读回终态为 UNCONFIGURED，因此重新上线前可能需要先用 +robot-config 恢复配置。"},
			AvoidWhen:    []string{"只需临时停用但必须保证配置可原样恢复时不要使用；当前下游没有该保留语义"},
			Examples:     []string{"dws devapp +robot-disable --unified-app-id <UNIFIED_APP_ID>"},
		},
		Result: devAppVerifiedMutationResult(),
	},
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return changeDevAppRobotStatus(rt, "disable_dev_app_robot", "robot_disable", "UNCONFIGURED")
	},
}

func changeDevAppRobotStatus(rt *shortcut.RuntimeContext, tool, action, wantStatus string) error {
	appID := rt.Str("unified-app-id")
	params := map[string]any{"unifiedAppId": appID}
	if previewed, err := devAppWritePreview(rt, tool, params); previewed || err != nil {
		return err
	}
	if _, err := callDevAppWrite(rt, tool, params); err != nil {
		return err
	}
	_, resource, err := readDevAppObject(rt, "get_extension_robot_config", params, map[string]string{"unifiedAppId": appID})
	if err != nil {
		return err
	}
	gotStatus := strings.ToUpper(devAppFirstString(resource, "robotStatus", "robot_status", "status"))
	if gotStatus == "" {
		return devAppResponseError("devapp/get_extension_robot_config", "readback_status_missing", "机器人状态读回缺少 robotStatus")
	}
	if gotStatus != wantStatus {
		return devAppResponseError("devapp/get_extension_robot_config", "readback_status_mismatch", fmt.Sprintf("机器人状态读回为 %s，预期 %s", gotStatus, wantStatus))
	}
	return rt.Output(verifiedDevAppMutation(action, appID, resource))
}

// ---------------------------------------------------------------------------
// 事件订阅
// ---------------------------------------------------------------------------

// EventList maps helper `list_dev_app_events`.
var EventList = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+event-list",
	Product:     productDevApp,
	Description: "查询应用可用事件目录与订阅状态",
	Intent:      "当你要查某应用可用的事件码、事件名称及当前订阅状态（用于选择订阅项、排查漏收事件或退订前核对）时使用；输入 unifiedAppId，可按关键词过滤并游标分页。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "devapp",
			Name:           "shortcut_event_list",
			CanonicalPath:  "devapp.shortcut_event_list",
			CLIPath:        "devapp +event-list",
			PrimaryCLIPath: "devapp +event-list",
		},
		Description: "查询应用可用事件目录与订阅状态",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "查询应用可用事件目录与订阅状态",
			UseWhen:      []string{"当你要查某应用可用的事件码、事件名称及当前订阅状态（用于选择订阅项、排查漏收事件或退订前核对）时使用；输入 unifiedAppId，可按关键词过滤并游标分页。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws devapp +event-list --unified-app-id <UNIFIED_APP_ID>"},
		},
		Result:     devAppPaginatedProjectionResult("events", "当前页应用订阅事件查询结果"),
		Pagination: devAppCursorPagination(),
	},
	Flags: append([]shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
		{Name: "keyword", Type: shortcut.FlagString, Desc: "事件搜索关键词，支持按事件码或事件名称模糊匹配"},
	}, cursorFlags...),
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"unifiedAppId": rt.Str("unified-app-id")}
		if rt.Changed("keyword") {
			params["keyword"] = rt.Str("keyword")
		}
		applyCursor(rt, params)
		data, err := rt.CallMCPData(productDevApp, "list_dev_app_events", params)
		if err != nil {
			return err
		}
		events, err := eventListProject(data)
		if err != nil {
			return err
		}
		projection, err := devAppListProjection(data, "events", events, "devapp/list_dev_app_events")
		if err != nil {
			return err
		}
		return rt.Output(projection)
	},
}

// eventListProject reshapes list_dev_app_events into a clean subscribed-event
// list ({eventCode, eventName, status, gmtModified}) — output-projection
// clean output projection. The list container and every item are validated
// before projection so unknown response shapes fail closed.
func eventListProject(data map[string]any) ([]map[string]any, error) {
	raw, _, err := requireDevAppCollection(data, "devapp/list_dev_app_events",
		[]string{"eventCode", "event_code", "code"}, "list", "items", "events", "eventList")
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m := item.(map[string]any)
		row := map[string]any{}
		if v, ok := eventListFirst(m, "eventCode", "event_code", "code"); ok {
			row["eventCode"] = v
		}
		if v, ok := eventListFirst(m, "eventName", "event_name", "name"); ok {
			row["eventName"] = v
		}
		if v, ok := eventListFirst(m, "status", "subscribeStatus", "subscribe_status"); ok {
			row["status"] = v
		}
		if v, ok := eventListFirst(m, "gmtModified", "gmt_modified", "modifyTime", "modified_time"); ok {
			row["gmtModified"] = v
		}
		out = append(out, row)
	}
	return out, nil
}

// eventListFirst returns the first present candidate key's value.
func eventListFirst(m map[string]any, keys ...string) (any, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v, true
		}
	}
	return nil, false
}

// EventSubscribe maps helper `subscribe_dev_app_events`.
var EventSubscribe = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+event-subscribe",
	Product:     productDevApp,
	Description: "订阅应用事件回调",
	Intent:      "当你要让应用开始接收某些事件的回调推送（如通讯录变更、审批事件等）时使用；传入 unifiedAppId 和事件码列表，会实际为该应用登记这些事件订阅。",
	Risk:        shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "user_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID: "devapp", Name: "shortcut_event_subscribe",
			CanonicalPath: "devapp.shortcut_event_subscribe", CLIPath: "devapp +event-subscribe", PrimaryCLIPath: "devapp +event-subscribe",
		},
		Description: "订阅应用事件并逐项精确读回",
		Interface:   &contract.InterfaceSpec{Mode: "composite", Availability: "available", Reason: "Reviewed adapter rejects empty or duplicate event codes, requires terminal success, then traverses the bounded event cursor and verifies every requested eventCode under the same unifiedAppId."},
		Selection: contract.SelectionSpec{
			AgentSummary: "订阅应用事件回调",
			UseWhen:      []string{"当你要让应用开始接收某些事件的回调推送（如通讯录变更、审批事件等）时使用；传入 unifiedAppId 和事件码列表，会实际为该应用登记这些事件订阅。"},
			AvoidWhen:    []string{"只需查看现有订阅时暂用 dev app event list 原子命令；+event-list 缺少零结果分页终止事实，退订也无法证明实际移除"},
			Examples:     []string{"dws devapp +event-subscribe --unified-app-id <UNIFIED_APP_ID> --event-codes <EVENT_CODES>"},
		},
		Result: devAppVerifiedMutationResult(),
	},
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
		{Name: "event-codes", Type: shortcut.FlagStringSlice, Desc: "事件码列表至少包含一项非空且互不重复的 eventCode", Required: true},
	},
	Constraints: []shortcut.Constraint{{
		Kind:        shortcut.ConstraintCustom,
		Flags:       []string{"event-codes"},
		Description: "事件码列表至少包含一项非空且互不重复的 eventCode",
	}},
	Validate: func(rt *shortcut.RuntimeContext) error {
		_, err := validatedDevAppValues(rt.StrSlice("event-codes"), "--event-codes")
		return err
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		// Validate already rejects empty and duplicate values before Execute.
		codes, _ := validatedDevAppValues(rt.StrSlice("event-codes"), "--event-codes")
		appID := rt.Str("unified-app-id")
		params := map[string]any{
			"unifiedAppId": appID,
			"eventCodes":   codes,
		}
		if previewed, err := devAppWritePreview(rt, "subscribe_dev_app_events", params); previewed || err != nil {
			return err
		}
		if _, err := callDevAppWrite(rt, "subscribe_dev_app_events", params); err != nil {
			return err
		}
		if err := verifyDevAppEventCodes(rt, appID, codes); err != nil {
			return err
		}
		return rt.Output(verifiedDevAppMutation("event_subscribe", appID, map[string]any{"eventCodes": codes}))
	},
}

func validatedDevAppValues(values []string, flag string) ([]string, error) {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			return nil, apperrors.NewValidation(flag + " 不能包含空值")
		}
		if seen[value] {
			return nil, apperrors.NewValidation(flag + " 不能包含重复值")
		}
		seen[value] = true
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil, apperrors.NewValidation(flag + " 至少需要一个非空值")
	}
	return out, nil
}

func verifyDevAppEventCodes(rt *shortcut.RuntimeContext, appID string, wantCodes []string) error {
	want := make(map[string]bool, len(wantCodes))
	for _, code := range wantCodes {
		want[code] = true
	}
	found := map[string]bool{}
	cursor := ""
	seenCursors := map[string]bool{}
	for page := 0; page < devAppMaxReadbackPages; page++ {
		params := map[string]any{"unifiedAppId": appID, "pageSize": 100}
		if cursor != "" {
			params["cursor"] = cursor
		}
		raw, err := rt.CallMCPData(productDevApp, "list_dev_app_events", params)
		if err != nil {
			return err
		}
		events, err := eventListProject(raw)
		if err != nil {
			return err
		}
		for _, event := range events {
			code := devAppFirstString(event, "eventCode")
			if want[code] {
				found[code] = true
			}
		}
		if len(found) == len(want) {
			return nil
		}
		projection, err := devAppListProjection(raw, "events", events, "devapp/list_dev_app_events")
		if err != nil {
			return err
		}
		more, _ := projection["hasMore"].(bool)
		if !more {
			return devAppResponseError("devapp/list_dev_app_events", "subscription_readback_missing", "事件订阅终态读回缺少请求的 eventCode")
		}
		next := devAppFirstString(projection, "nextCursor")
		if next == "" || next == cursor || seenCursors[next] {
			return devAppResponseError("devapp/list_dev_app_events", "cursor_stall", "事件订阅读回游标未推进")
		}
		seenCursors[next] = true
		cursor = next
	}
	return devAppResponseError("devapp/list_dev_app_events", "readback_page_limit", "事件订阅读回超过有界页数，无法证明全部事件已订阅")
}

// EventUnsubscribe maps helper `unsubscribe_dev_app_events`.
var EventUnsubscribe = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+event-unsubscribe",
	Product:     productDevApp,
	Description: "取消订阅应用事件",
	Intent:      "当你不再需要某些事件的回调推送、要停止接收它们时使用；传入 unifiedAppId 和事件码列表，会实际取消这些事件的订阅。",
	Risk:        shortcut.RiskHighWrite,
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
		{Name: "event-codes", Type: shortcut.FlagStringSlice, Desc: "事件码列表", Required: true},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"unifiedAppId": rt.Str("unified-app-id"),
			"eventCodes":   rt.StrSlice("event-codes"),
		}
		return rt.CallMCP("unsubscribe_dev_app_events", params)
	},
}

// ---------------------------------------------------------------------------
// 版本发布
// ---------------------------------------------------------------------------

// VersionCreate maps helper `create_dev_app_version`.
var VersionCreate = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+version-create",
	Product:     productDevApp,
	Description: "基于当前配置创建应用新版本",
	Intent:      "当你改完应用配置、准备走发布流程前需要先打一个版本快照时使用；传入 unifiedAppId（可选显式版本号与描述，默认服务端自动递增），会实际创建一个新版本并返回 versionId 供后续预检和发布。",
	Risk:        shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "user_required", Idempotency: "non_idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID: "devapp", Name: "shortcut_version_create",
			CanonicalPath: "devapp.shortcut_version_create", CLIPath: "devapp +version-create", PrimaryCLIPath: "devapp +version-create",
		},
		Description: "创建不可变版本快照并按应用与版本双 ID 精确读回",
		Interface:   &contract.InterfaceSpec{Mode: "composite", Availability: "available", Reason: "Reviewed adapter requires terminal success and a non-empty versionId, then reads get_dev_app_version_detail with the same unifiedAppId/versionId and compares supplied version metadata before verified output."},
		Selection: contract.SelectionSpec{
			AgentSummary: "基于当前配置创建应用新版本",
			UseWhen:      []string{"当你改完应用配置、准备走发布流程前需要先打一个版本快照时使用；传入 unifiedAppId（可选显式版本号与描述，默认服务端自动递增），会实际创建一个新版本并返回 versionId 供后续预检和发布。"},
			AvoidWhen:    []string{"只需查看历史版本使用 +version-list；正式发布使用前先运行 +version-check-approval，发布能力当前仍不可用"},
			Examples:     []string{"dws devapp +version-create --unified-app-id <UNIFIED_APP_ID> --desc <DESCRIPTION>"},
		},
		Result: devAppVerifiedMutationResult(),
	},
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
		{Name: "version", Type: shortcut.FlagString, Desc: "高级可选：显式版本号，如 1.0.1；默认由服务端自动递增"},
		{Name: "desc", Type: shortcut.FlagString, Desc: "版本描述"},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		appID := rt.Str("unified-app-id")
		params := map[string]any{"unifiedAppId": appID}
		if rt.Changed("version") {
			params["version"] = rt.Str("version")
		}
		if rt.Changed("desc") {
			params["desc"] = rt.Str("desc")
		}
		if previewed, err := devAppWritePreview(rt, "create_dev_app_version", params); previewed || err != nil {
			return err
		}
		created, err := callDevAppWrite(rt, "create_dev_app_version", params)
		if err != nil {
			return err
		}
		createdObject, err := requireDevAppObject(created, "devapp/create_dev_app_version")
		if err != nil {
			return err
		}
		versionID := devAppFirstString(createdObject, "versionId", "version_id", "id")
		if versionID == "" {
			return devAppResponseError("devapp/create_dev_app_version", "missing_version_id", "创建版本回执缺少稳定 versionId")
		}
		_, resource, err := readDevAppObject(rt, "get_dev_app_version_detail", map[string]any{
			"unifiedAppId": appID,
			"versionId":    versionID,
		}, map[string]string{"unifiedAppId": appID, "versionId": versionID})
		if err != nil {
			return err
		}
		if rt.Changed("version") {
			if err := requireDevAppStringField(resource, "devapp/get_dev_app_version_detail", rt.Str("version"), "version", "versionName"); err != nil {
				return err
			}
		}
		if rt.Changed("desc") {
			if err := requireDevAppStringField(resource, "devapp/get_dev_app_version_detail", rt.Str("desc"), "desc", "description", "remark"); err != nil {
				return err
			}
		}
		return rt.Output(verifiedDevAppMutation("version_create", appID, resource))
	},
}

// VersionList maps helper `list_dev_app_versions`.
var VersionList = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+version-list",
	Product:     productDevApp,
	Description: "分页查询应用版本列表",
	Intent:      "当你要查看某应用的历史版本（找某个 versionId、看各版本发布状态或回顾迭代记录）时使用；输入 unifiedAppId 并分页，返回版本列表。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "devapp",
			Name:           "shortcut_version_list",
			CanonicalPath:  "devapp.shortcut_version_list",
			CLIPath:        "devapp +version-list",
			PrimaryCLIPath: "devapp +version-list",
		},
		Description: "分页查询应用版本列表",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "分页查询应用版本列表",
			UseWhen:      []string{"当你要查看某应用的历史版本（找某个 versionId、看各版本发布状态或回顾迭代记录）时使用；输入 unifiedAppId 并分页，返回版本列表。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws devapp +version-list --unified-app-id <UNIFIED_APP_ID>"},
		},
		Result:     devAppPaginatedProjectionResult("versions", "当前页开放平台应用版本查询结果"),
		Pagination: devAppCursorPagination(),
	},
	Flags: append([]shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
	}, cursorFlags...),
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"unifiedAppId": rt.Str("unified-app-id")}
		applyCursor(rt, params)
		data, err := rt.CallMCPData(productDevApp, "list_dev_app_versions", params)
		if err != nil {
			return err
		}
		versions, err := versionListProject(data)
		if err != nil {
			return err
		}
		projection, err := devAppListProjection(data, "versions", versions, "devapp/list_dev_app_versions")
		if err != nil {
			return err
		}
		return rt.Output(projection)
	},
}

func devAppListProjection(data map[string]any, key string, items []map[string]any, operation string) (map[string]any, error) {
	if _, err := requireDevAppSuccess(data, operation); err != nil {
		return nil, err
	}
	out := map[string]any{"count": len(items), key: items}
	for _, candidate := range devAppPaginationCandidates(data) {
		rawHasMore, hasMore := candidate["hasMore"]
		rawCursor, hasCursor := candidate["nextCursor"]
		if !hasMore && !hasCursor {
			continue
		}
		if !hasMore {
			return nil, devAppResponseError(operation, "missing_has_more", "分页响应只有 nextCursor，缺少 hasMore 终止证据")
		}
		more, ok := rawHasMore.(bool)
		if !ok {
			return nil, devAppResponseError(operation, "malformed_has_more", "分页响应 hasMore 不是布尔值")
		}
		out["hasMore"] = more
		if hasCursor {
			cursor, ok := rawCursor.(string)
			if !ok {
				return nil, devAppResponseError(operation, "malformed_cursor", "分页响应 nextCursor 不是字符串")
			}
			if strings.TrimSpace(cursor) != "" {
				out["nextCursor"] = strings.TrimSpace(cursor)
			}
		}
		if more && devAppFirstString(out, "nextCursor") == "" {
			return nil, devAppResponseError(operation, "missing_cursor", "分页响应 hasMore=true 但缺少可推进的 nextCursor")
		}
		return out, nil
	}
	return nil, devAppResponseError(operation, "missing_pagination", "分页响应缺少 hasMore 终止证据")
}

func devAppPaginationCandidates(data map[string]any) []map[string]any {
	// Pagination evidence can occur at any of the reviewed envelope layers.
	// Reuse the same bounded breadth-first walk as response validation so a
	// valid content.result (or deeper content/result/data combination) cannot
	// be mistaken for a response without a pagination terminus.
	return devAppContainers(data)
}

// versionListProject reshapes list_dev_app_versions into a clean version list
// ({versionId, version, status, desc, gmtCreate}) — output-projection fidelity
// for clean output. The list container and per-item field names are probed defensively
// across reviewed candidate keys, while missing, mistyped, or malformed
// collections fail closed.
func versionListProject(data map[string]any) ([]map[string]any, error) {
	raw, _, err := requireDevAppCollection(data, "devapp/list_dev_app_versions",
		[]string{"versionId", "version_id", "id"}, "list", "items", "versions", "versionList")
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m := item.(map[string]any)
		row := map[string]any{}
		if v, ok := versionListFirst(m, "versionId", "version_id", "id"); ok {
			row["versionId"] = v
		}
		if v, ok := versionListFirst(m, "version", "versionName", "version_name"); ok {
			row["version"] = v
		}
		if v, ok := versionListFirst(m, "status", "publishStatus", "publish_status", "versionStatus"); ok {
			row["status"] = v
		}
		if v, ok := versionListFirst(m, "desc", "description", "remark"); ok {
			row["desc"] = v
		}
		if v, ok := versionListFirst(m, "gmtCreate", "gmt_create", "createTime", "create_time"); ok {
			row["gmtCreate"] = v
		}
		out = append(out, row)
	}
	return out, nil
}

// versionListFirst returns the first present candidate key's value.
func versionListFirst(m map[string]any, keys ...string) (any, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v, true
		}
	}
	return nil, false
}

// VersionGet maps helper `get_dev_app_version_detail`.
var VersionGet = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+version-get",
	Product:     productDevApp,
	Description: "查询指定版本详情",
	Intent:      "当你已知某个 versionId、要查看该版本的具体内容（版本号、描述、包含的配置等）以核对发布内容时使用；输入 unifiedAppId 和 versionId，返回单个版本的详情。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "devapp",
			Name:           "shortcut_version_get",
			CanonicalPath:  "devapp.shortcut_version_get",
			CLIPath:        "devapp +version-get",
			PrimaryCLIPath: "devapp +version-get",
		},
		Description: "查询指定版本详情",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "查询指定版本详情",
			UseWhen:      []string{"当你已知某个 versionId、要查看该版本的具体内容（版本号、描述、包含的配置等）以核对发布内容时使用；输入 unifiedAppId 和 versionId，返回单个版本的详情。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws devapp +version-get --unified-app-id <UNIFIED_APP_ID> --version-id <VERSION_ID>"},
		},
		Result: devAppObjectResult(
			contract.ResultOutcomeSuccess,
			contract.ResultOutcomeFailure,
		),
	},
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
		{Name: "version-id", Type: shortcut.FlagString, Desc: "版本 ID", Required: true},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		appID := rt.Str("unified-app-id")
		versionID := rt.Str("version-id")
		params := map[string]any{
			"unifiedAppId": appID,
			"versionId":    versionID,
		}
		return outputDevAppObject(rt, "get_dev_app_version_detail", params, map[string]string{
			"unifiedAppId": appID, "versionId": versionID,
		})
	},
}

// VersionCheckApproval maps helper `publish_dev_app_version` in precheck mode
// (precheckOnly=true): it only returns approval requirements, does not publish.
var VersionCheckApproval = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+version-check-approval",
	Product:     productDevApp,
	Description: "预检版本发布是否需要审批（不实际发布）",
	Intent:      "当你在正式发布某版本前想先确认它是否会触发审批、是否含高敏权限等发布前置要求时使用；传入 unifiedAppId 和 versionId，仅做预检返回审批要求，不会真正发布。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "devapp",
			Name:           "shortcut_version_check_approval",
			CanonicalPath:  "devapp.shortcut_version_check_approval",
			CLIPath:        "devapp +version-check-approval",
			PrimaryCLIPath: "devapp +version-check-approval",
		},
		Description: "预检版本发布是否需要审批（不实际发布）",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "预检版本发布是否需要审批（不实际发布）",
			UseWhen:      []string{"当你在正式发布某版本前想先确认它是否会触发审批、是否含高敏权限等发布前置要求时使用；传入 unifiedAppId 和 versionId，仅做预检返回审批要求，不会真正发布。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws devapp +version-check-approval --unified-app-id <UNIFIED_APP_ID> --version-id <VERSION_ID>"},
		},
		Result: devAppObjectResult(
			contract.ResultOutcomeSuccess,
			contract.ResultOutcomePending,
			contract.ResultOutcomeFailure,
		),
	},
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
		{Name: "version-id", Type: shortcut.FlagString, Desc: "版本 ID", Required: true},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		appID := rt.Str("unified-app-id")
		versionID := rt.Str("version-id")
		params := map[string]any{
			"unifiedAppId": appID,
			"versionId":    versionID,
			"precheckOnly": true,
		}
		return outputDevAppObject(rt, "publish_dev_app_version", params, map[string]string{
			"unifiedAppId": appID, "versionId": versionID,
		})
	},
}

// VersionPublish maps helper `publish_dev_app_version` (real publish,
// precheckOnly=false).
var VersionPublish = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+version-publish",
	Product:     productDevApp,
	Description: "发布指定版本（含高敏权限需 --confirmed-sensitive）",
	Intent:      "当你要把某个已创建的版本正式上线到线上环境时使用；传入 unifiedAppId 和 versionId，会实际触发发布（含高敏权限需加 --confirmed-sensitive 确认，灰度选人可指定审批人），可能进入审批流。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
		{Name: "version-id", Type: shortcut.FlagString, Desc: "版本 ID", Required: true},
		{Name: "confirmed-sensitive", Type: shortcut.FlagBool, Desc: "确认发布包含高敏权限的版本"},
		{Name: "approver-user-id", Type: shortcut.FlagString, Desc: "灰度选人模式下指定审批人 userId"},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"unifiedAppId": rt.Str("unified-app-id"),
			"versionId":    rt.Str("version-id"),
			"precheckOnly": false,
		}
		if rt.Changed("confirmed-sensitive") {
			params["confirmedSensitive"] = rt.Bool("confirmed-sensitive")
		}
		if rt.Changed("approver-user-id") {
			params["approverUserId"] = rt.Str("approver-user-id")
		}
		return rt.CallMCP("publish_dev_app_version", params)
	},
}

// VersionStatus maps helper `get_dev_app_version_status`.
var VersionStatus = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+version-status",
	Product:     productDevApp,
	Description: "查询版本发布/审批状态",
	Intent:      "当你已提交发布、想跟进某版本当前处于什么阶段（审批中、已发布、被驳回等）时使用；输入 unifiedAppId 和 versionId，返回该版本的发布/审批状态。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "devapp",
			Name:           "shortcut_version_status",
			CanonicalPath:  "devapp.shortcut_version_status",
			CLIPath:        "devapp +version-status",
			PrimaryCLIPath: "devapp +version-status",
		},
		Description: "查询版本发布/审批状态",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "查询版本发布/审批状态",
			UseWhen:      []string{"当你已提交发布、想跟进某版本当前处于什么阶段（审批中、已发布、被驳回等）时使用；输入 unifiedAppId 和 versionId，返回该版本的发布/审批状态。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws devapp +version-status --unified-app-id <UNIFIED_APP_ID> --version-id <VERSION_ID>"},
		},
		Result: devAppObjectResult(
			contract.ResultOutcomeSuccess,
			contract.ResultOutcomePending,
			contract.ResultOutcomeFailure,
		),
	},
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
		{Name: "version-id", Type: shortcut.FlagString, Desc: "版本 ID", Required: true},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		appID := rt.Str("unified-app-id")
		versionID := rt.Str("version-id")
		params := map[string]any{
			"unifiedAppId": appID,
			"versionId":    versionID,
		}
		return outputDevAppObject(rt, "get_dev_app_version_status", params, map[string]string{
			"unifiedAppId": appID, "versionId": versionID,
		})
	},
}

var devAppUnavailableReasons = map[string]string{
	"+permission-add":    "权限申请缺少逐项终态回执与按 scopeValue 精确读回验证",
	"+permission-remove": "权限移除缺少逐项终态回执与按 scopeValue 精确读回验证",
	"+security-config":   "整组覆盖写入缺少读取当前值、精确写回和恢复原配置的安全闭环",
	"+event-unsubscribe": "事件退订缺少可恢复的稳定订阅 fixture 与逐项读回",
	"+version-publish":   "版本发布可能进入审批或线上生效，缺少安全测试租户与回滚合同",
}

func unavailableDevAppShortcut(item shortcut.Shortcut, reason string) shortcut.Shortcut {
	item.Hidden = true
	item.Availability = shortcut.AvailabilityUnavailable
	item.OutputRollout = output.RolloutDualValidate
	item.Contract = corecmd.ContractDecl{}
	constraintFlags := make([]string, 0, len(item.Flags))
	for _, flag := range item.Flags {
		constraintFlags = append(constraintFlags, flag.Name)
	}
	item.Constraints = append(item.Constraints, shortcut.Constraint{
		Kind:        shortcut.ConstraintCustom,
		Flags:       constraintFlags,
		Description: "该命令保持 unavailable，直到文档化的安全验证前置条件满足",
	})
	unavailable := func(*shortcut.RuntimeContext) error {
		return apperrors.NewValidation("该 DevApp Shortcut 当前 unavailable：" + reason)
	}
	item.Validate = unavailable
	item.Execute = unavailable
	return item
}

func init() {
	items := []shortcut.Shortcut{
		frameworkUnified(ListApp),
		frameworkUnified(GetApp),
		frameworkUnified(CreateApp),
		frameworkUnified(UpdateApp),
		frameworkUnified(DeleteApp),
		frameworkUnified(EnableApp),
		frameworkUnified(DisableApp),
		frameworkUnified(GetCredentials),
		frameworkUnified(WebappGet),
		frameworkUnified(WebappConfig),
		frameworkUnified(PermissionList),
		frameworkDualValidate(PermissionAdd),
		frameworkDualValidate(PermissionRemove),
		frameworkUnified(MemberList),
		frameworkUnified(MemberAdd),
		frameworkUnified(MemberRemove),
		frameworkDualValidate(SecurityConfig),
		frameworkUnified(RobotGet),
		frameworkUnified(RobotConfig),
		frameworkUnified(RobotEnable),
		frameworkUnified(RobotDisable),
		frameworkUnified(EventList),
		frameworkUnified(EventSubscribe),
		frameworkDualValidate(EventUnsubscribe),
		frameworkUnified(VersionCreate),
		frameworkUnified(VersionList),
		frameworkUnified(VersionGet),
		frameworkUnified(VersionCheckApproval),
		frameworkDualValidate(VersionPublish),
		frameworkUnified(VersionStatus),
	}
	for index := range items {
		if reason, unavailable := devAppUnavailableReasons[items[index].Command]; unavailable {
			items[index] = unavailableDevAppShortcut(items[index], reason)
		}
	}
	shortcut.Register(items...)
}

func frameworkUnified(item shortcut.Shortcut) shortcut.Shortcut {
	item.OutputRollout = output.RolloutUnifiedActive
	return item
}

func frameworkDualValidate(item shortcut.Shortcut) shortcut.Shortcut {
	item.OutputRollout = output.RolloutDualValidate
	return item
}
