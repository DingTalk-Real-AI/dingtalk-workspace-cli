// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

// Package aisearch implements strict, task-oriented Shortcut projections over
// DingTalk's enterprise search APIs.
package aisearch

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/responsecheck"
)

const (
	aisearchCompositeReason = "Reviewed AiSearch Shortcut adapter: the executable CLI owns strict success, collection, item, stable-identity, and output validation; the task contract is stronger than a direct MCP passthrough."
	aisearchZeroGapReason   = "Live exact-leaf probes with a random zero-match marker still returned non-empty business objects, so the Shortcut cannot prove query relevance or a legitimate zero result."
)

var aisearchReadSafety = contract.SafetySpec{
	Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent",
}

func aisearchResult(description string) *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(fmt.Sprintf(
			`{"type":"object","description":%q,"properties":{"count":{"type":"integer","description":"当前响应中通过严格校验的命中数量"},"matches":{"type":"array","description":"通过 success、集合、元素和稳定身份校验的搜索命中；该接口不发布分页或全局完整性承诺","items":{"type":"object","description":"带稳定身份和来源类型的搜索命中","additionalProperties":true}}},"required":["count","matches"],"additionalProperties":false}`,
			description,
		)),
	}
}

func aisearchContract(command, description, intent string, available bool, flags []contract.ParamDecl, examples ...string) corecmd.ContractDecl {
	name := "shortcut_" + strings.ReplaceAll(strings.TrimPrefix(command, "+"), "-", "_")
	availability := contract.InterfaceAvailable
	reason := aisearchCompositeReason
	if !available {
		availability = contract.InterfaceUnavailable
		reason = aisearchZeroGapReason
	}
	return corecmd.ContractDecl{
		Description: description,
		Result:      aisearchResult(description),
		Identity: contract.ToolIdentitySpec{
			ProductID: "aisearch", Name: name, CanonicalPath: "aisearch." + name,
			CLIPath: "aisearch " + command, PrimaryCLIPath: "aisearch " + command,
		},
		Interface: &contract.InterfaceSpec{
			Mode: contract.InterfaceModeComposite, Availability: availability, Reason: reason,
		},
		Selection: contract.SelectionSpec{
			AgentSummary: description,
			UseWhen:      []string{intent},
			AvoidWhen: []string{
				"已有稳定资源 ID 时使用对应产品详情命令；不要把搜索命中当作资源写入回执",
				"需要全局完结或可续页枚举时不要使用；下层未发布分页与终止证据",
			},
			Examples: examples,
		},
		Parameters: flags,
	}
}

// SearchPerson is the only public AiSearch Shortcut. Exact live evidence
// proves both a known non-empty user fixture and an explicit random zero hit.
var SearchPerson = shortcut.Shortcut{
	OutputRollout: output.RolloutUnifiedActive,
	Service:       "aisearch", Command: "+search-person", Product: "aisearch",
	Description: "按姓名、部门、职责或组织关系语义搜索企业人员",
	Intent:      "只知道人名、部门、职责、手机号、工号或上下级关系，需要获得可用于后续任务的人员稳定身份时使用。",
	Risk:        shortcut.RiskRead,
	Safety:      aisearchReadSafety,
	Contract: aisearchContract(
		"+search-person",
		"按姓名、部门、职责或组织关系语义搜索企业人员",
		"只知道人名、部门、职责、手机号、工号或上下级关系，需要获得可用于后续任务的人员稳定身份时使用。",
		true,
		[]contract.ParamDecl{{Name: "query", Property: "keyword"}, {Name: "dimensions", Property: "dimension"}},
		`dws aisearch +search-person --query "张三" --dimensions name --format json`,
		`dws aisearch +search-person --query "产品" --dimensions duty,department --format json`,
	),
	Flags: []shortcut.Flag{
		{Name: "query", Type: shortcut.FlagString, Required: true, Desc: "人员搜索关键词"},
		{Name: "dimensions", Type: shortcut.FlagStringSlice, Default: "all", Desc: "搜索维度；--dimensions 只能包含 all/name/department/position/duty/supervisor/subordinate/phone/jobNumber，all 不能与其他维度并用"},
	},
	Constraints: []shortcut.Constraint{{
		Kind: shortcut.ConstraintCustom, Flags: []string{"dimensions"},
		Description: "--dimensions 只能包含 all/name/department/position/duty/supervisor/subordinate/phone/jobNumber，all 不能与其他维度并用",
	}},
	Validate: validatePerson,
	Execute: func(rt *shortcut.RuntimeContext) error {
		return executeSearch(rt, "enterprise_person_search", map[string]any{
			"keyword": rt.Str("query"), "dimension": rt.StrSlice("dimensions"),
		}, []string{"userId", "openDingTalkId", "url"})
	},
}

// SearchEnterprise exposes only the IM slice. Exact live evidence proves that
// this narrowed interface returns stable IM identities for a known fixture and
// an explicit empty collection for a guaranteed-random marker. Other content
// types remain routed to the atomic command until their relevance contract is
// equally trustworthy.
var SearchEnterprise = shortcut.Shortcut{
	OutputRollout: output.RolloutUnifiedActive,
	Service:       "aisearch", Command: "+search-enterprise", Product: "aisearch",
	Description: "按主题搜索企业知识、消息、邮件与协作内容",
	Intent:      "需要按关键词搜索企业即时消息时使用；当前只公开已通过已知非空与保证零命中验证的 IM 类型。",
	Risk:        shortcut.RiskRead,
	Safety:      aisearchReadSafety,
	Contract: aisearchContract(
		"+search-enterprise",
		"按主题搜索企业知识、消息、邮件与协作内容",
		"需要按关键词搜索企业即时消息时使用；当前只公开已通过已知非空与保证零命中验证的 IM 类型。",
		true,
		[]contract.ParamDecl{{Name: "queries"}, {Name: "types", Property: "searchTypes"}, {Name: "time-range", Property: "timeRange"}},
		`dws aisearch +search-enterprise --queries "发布方案" --types im --format json`,
	),
	Flags: []shortcut.Flag{
		{Name: "queries", Type: shortcut.FlagStringSlice, Required: true, Desc: "即时消息关键词列表"},
		{Name: "types", Type: shortcut.FlagStringSlice, Default: "im", Desc: "已审核内容类型；--types 当前必须且只能为 im"},
		{Name: "time-range", Type: shortcut.FlagString, Desc: "服务端自然语言时间范围，如今天、本周或过去一月"},
	},
	Constraints: []shortcut.Constraint{{
		Kind: shortcut.ConstraintCustom, Flags: []string{"types"},
		Description: "--types 当前必须且只能为 im",
	}},
	Validate: validateEnterprise,
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"queries": rt.StrSlice("queries"), "searchTypes": []string{"im"},
		}
		if timeRange := strings.TrimSpace(rt.Str("time-range")); timeRange != "" {
			params["timeRange"] = timeRange
		}
		return executeSearchForSource(rt, "search_enterprise", params, []string{"openConversationId", "url"}, "im")
	},
}

// SearchBehavior remains registered but unavailable for the same zero-match
// relevance gap as SearchEnterprise.
var SearchBehavior = shortcut.Shortcut{
	OutputRollout: output.RolloutUnifiedActive,
	Service:       "aisearch", Command: "+search-behavior", Product: "aisearch",
	Description: "搜索发送、创建、分享、编辑或接收等企业行为记录",
	Intent:      "用户明确询问谁发送、创建、分享、编辑或接收过什么时使用；当前因零命中相关性合同不足保持不可用。",
	Risk:        shortcut.RiskRead,
	Safety:      aisearchReadSafety,
	Contract: aisearchContract(
		"+search-behavior",
		"搜索发送、创建、分享、编辑或接收等企业行为记录",
		"用户明确询问谁发送、创建、分享、编辑或接收过什么时使用；当前因零命中相关性合同不足保持不可用。",
		false,
		[]contract.ParamDecl{
			{Name: "queries"}, {Name: "types", Property: "searchTypes"},
			{Name: "behavior", Property: "behaviorType"}, {Name: "chat-scope", Property: "chatScope"},
			{Name: "time-range", Property: "timeRange"}, {Name: "direction"},
		},
		`dws aisearch +search-behavior --types mail --behavior send --direction "我->同事" --format json`,
	),
	Flags: []shortcut.Flag{
		{Name: "queries", Type: shortcut.FlagStringSlice, Desc: "内容关键词列表；汇总场景可留空"},
		{Name: "types", Type: shortcut.FlagStringSlice, Default: "all", Desc: "内容类型：all/document/im/calendar/todo/minute/report/image/link/notable/baike/mail"},
		{Name: "behavior", Type: shortcut.FlagString, Default: "all", Enum: []string{"all", "send", "create", "share", "edit", "receive"}, Desc: "行为类型"},
		{Name: "chat-scope", Type: shortcut.FlagString, Desc: "仅 IM 搜索时的会话或群范围"},
		{Name: "time-range", Type: shortcut.FlagString, Desc: "服务端自然语言时间范围"},
		{Name: "direction", Type: shortcut.FlagString, Desc: "交互方向，如我->同事"},
	},
	Constraints: []shortcut.Constraint{{
		Kind: shortcut.ConstraintCustom, Flags: []string{"types"},
		Description: "--types 只能包含已审核内容类型，all 不能与其他类型并用；--chat-scope 仅能与 im 类型组合",
	}},
	Validate: validateBehavior,
	Execute: func(rt *shortcut.RuntimeContext) error {
		return unavailableSearch("aisearch/search_enterprise_behavior")
	},
}

func unavailableSearch(operation string) error {
	return responsecheck.Error(operation, "capability_unavailable", aisearchZeroGapReason)
}

func validatePerson(rt *shortcut.RuntimeContext) error {
	return validateValues("--dimensions", rt.StrSlice("dimensions"), map[string]bool{
		"all": true, "name": true, "department": true, "position": true, "duty": true,
		"supervisor": true, "subordinate": true, "phone": true, "jobNumber": true,
	})
}

func validateEnterprise(rt *shortcut.RuntimeContext) error {
	queries := rt.StrSlice("queries")
	if len(queries) == 0 {
		return responsecheck.Error("aisearch/search_enterprise", "empty_query", "--queries 至少需要一个非空即时消息关键词")
	}
	for _, query := range queries {
		if strings.TrimSpace(query) == "" {
			return responsecheck.Error("aisearch/search_enterprise", "empty_query", "--queries 不能包含空关键词")
		}
	}
	types := rt.StrSlice("types")
	if len(types) != 1 || strings.TrimSpace(types[0]) != "im" {
		return responsecheck.Error("aisearch/search_enterprise", "unsupported_search_type", "--types 当前必须且只能为 im；其他类型请使用 aisearch enterprise 原子命令")
	}
	return nil
}

func validateBehavior(rt *shortcut.RuntimeContext) error {
	types := rt.StrSlice("types")
	if err := validateSearchTypes(types); err != nil {
		return err
	}
	if rt.Str("chat-scope") != "" && !contains(types, "im") {
		return responsecheck.Error("aisearch/search_enterprise_behavior", "invalid_chat_scope", "--chat-scope 仅能与 --types im 组合")
	}
	return nil
}

func validateSearchTypes(values []string) error {
	return validateValues("--types", values, map[string]bool{
		"all": true, "document": true, "im": true, "calendar": true, "todo": true,
		"minute": true, "report": true, "image": true, "link": true, "notable": true,
		"baike": true, "mail": true,
	})
}

func validateValues(flag string, values []string, allowed map[string]bool) error {
	if len(values) == 0 {
		return responsecheck.Error("aisearch/validate", "empty_selector", flag+" 不能为空")
	}
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || !allowed[value] {
			return responsecheck.Error("aisearch/validate", "invalid_selector", fmt.Sprintf("%s 包含未支持值 %q", flag, value))
		}
		if seen[value] {
			return responsecheck.Error("aisearch/validate", "duplicate_selector", fmt.Sprintf("%s 包含重复值 %q", flag, value))
		}
		seen[value] = true
	}
	if len(values) > 1 && seen["all"] {
		return responsecheck.Error("aisearch/validate", "conflicting_selector", flag+" 中 all 不能与其他值并用")
	}
	return nil
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == expected {
			return true
		}
	}
	return false
}

func executeSearch(rt *shortcut.RuntimeContext, tool string, params map[string]any, identityKeys []string) error {
	operation := "aisearch/" + tool
	data, err := rt.CallMCPReadData("aisearch", tool, params)
	if err != nil {
		return err
	}
	matches, err := projectSearch(data, operation, identityKeys)
	if err != nil {
		return err
	}
	return rt.Output(map[string]any{"count": len(matches), "matches": matches})
}

func executeSearchForSource(rt *shortcut.RuntimeContext, tool string, params map[string]any, identityKeys []string, expectedSource string) error {
	operation := "aisearch/" + tool
	data, err := rt.CallMCPReadData("aisearch", tool, params)
	if err != nil {
		return err
	}
	matches, err := projectSearch(data, operation, identityKeys)
	if err != nil {
		return err
	}
	for index, item := range matches {
		if source := firstNonEmptyString(item, "sourceType"); source != expectedSource {
			return responsecheck.Error(operation, "unexpected_item_source", fmt.Sprintf("响应 result[%d] 来源 %q 与已审核类型 %q 不一致", index, source, expectedSource))
		}
	}
	return rt.Output(map[string]any{"count": len(matches), "matches": matches})
}

func projectSearch(data map[string]any, operation string, identityKeys []string) ([]map[string]any, error) {
	if hasConflictingError(data) {
		return nil, responsecheck.Error(operation, "conflicting_error_fields", "响应同时声明 success=true 和非空错误字段")
	}
	items, err := responsecheck.RequireObjectCollection(data, operation, "result")
	if err != nil {
		return nil, err
	}
	for index, item := range items {
		if firstNonEmptyString(item, "sourceType") == "" {
			return nil, responsecheck.Error(operation, "missing_item_source", fmt.Sprintf("响应 result[%d] 缺少非空 sourceType", index))
		}
		if firstNonEmptyString(item, identityKeys...) == "" {
			return nil, responsecheck.Error(operation, "missing_item_identity", fmt.Sprintf("响应 result[%d] 缺少稳定身份", index))
		}
	}
	return items, nil
}

func hasConflictingError(data map[string]any) bool {
	envelope := data
	if content, ok := data["content"].(map[string]any); ok {
		envelope = content
	}
	success, ok := envelope["success"].(bool)
	if !ok || !success {
		return false
	}
	if message := firstNonEmptyString(envelope, "errorMsg", "errorMessage"); message != "" {
		return true
	}
	if code, present := envelope["errorCode"]; present && code != nil {
		switch typed := code.(type) {
		case string:
			return strings.TrimSpace(typed) != "" && strings.TrimSpace(typed) != "0"
		case float64:
			return typed != 0
		default:
			return true
		}
	}
	return false
}

func firstNonEmptyString(object map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := object[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func init() {
	shortcut.Register(SearchPerson, SearchEnterprise, SearchBehavior)
}
