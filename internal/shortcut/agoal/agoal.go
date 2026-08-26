// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

// Package agoal declares reviewed shortcuts for DingTalk Agoal. The public
// surface intentionally stays smaller than the atomic helper surface: only
// reads with strict response contracts and safe live proof are available.
package agoal

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

const productAgoal = "agoal"

var agoalLoadLocation = time.LoadLocation

func agoalResponseError(operation, reason, message string) error {
	return apperrors.NewAPI(message,
		apperrors.WithOperation(operation),
		apperrors.WithOrigin("mcp"),
		apperrors.WithFailureStage("response_validation"),
		apperrors.WithRetryable(false),
		apperrors.WithReason(reason),
	)
}

func requireAgoalSuccess(data map[string]any, operation string) (map[string]any, error) {
	if len(data) == 0 {
		return nil, agoalResponseError(operation, "empty_tool_response", "服务返回空响应，无法证明查询成功或结果确实为空")
	}
	rawSuccess, present := data["success"]
	if !present {
		return nil, agoalResponseError(operation, "missing_success", "响应缺少 success 布尔终态")
	}
	success, ok := rawSuccess.(bool)
	if !ok {
		return nil, agoalResponseError(operation, "malformed_success", "响应 success 字段不是布尔值")
	}
	if !success {
		message, _ := data["message"].(string)
		if strings.TrimSpace(message) == "" {
			message = "服务明确返回 success=false"
		}
		return nil, agoalResponseError(operation, "remote_failure", message)
	}
	for _, key := range []string{"error", "errorCode", "error_code"} {
		if value, found := data[key]; found && value != nil && strings.TrimSpace(fmt.Sprint(value)) != "" && fmt.Sprint(value) != "0" {
			return nil, agoalResponseError(operation, "conflicting_error", fmt.Sprintf("响应 success 与 %s 错误字段冲突", key))
		}
	}
	return data, nil
}

func requireAgoalList(data map[string]any, operation string, value any, idKeys ...string) ([]map[string]any, error) {
	items, ok := value.([]any)
	if !ok {
		return nil, agoalResponseError(operation, "malformed_collection", "成功响应的业务集合不是数组")
	}
	out := make([]map[string]any, 0, len(items))
	for index, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			return nil, agoalResponseError(operation, "malformed_item", fmt.Sprintf("业务集合第 %d 项不是对象", index))
		}
		if agoalFirstString(item, idKeys...) == "" {
			return nil, agoalResponseError(operation, "missing_item_id", fmt.Sprintf("业务集合第 %d 项缺少稳定身份字段", index))
		}
		out = append(out, item)
	}
	return out, nil
}

func agoalFirstString(item map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := item[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func agoalNonNegativeInt(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, typed >= 0
	case int64:
		if typed < 0 || int64(int(typed)) != typed {
			return 0, false
		}
		return int(typed), true
	case float64:
		if typed < 0 || math.Trunc(typed) != typed || typed > float64(^uint(0)>>1) {
			return 0, false
		}
		return int(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil || parsed < 0 || int64(int(parsed)) != parsed {
			return 0, false
		}
		return int(parsed), true
	default:
		return 0, false
	}
}

func agoalReadSafety() contract.SafetySpec {
	return contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"}
}

func agoalUnavailableSafety(write bool) contract.SafetySpec {
	if write {
		return contract.SafetySpec{Effect: "destructive", Risk: "high", Confirmation: "user_required", Idempotency: "unknown"}
	}
	return agoalReadSafety()
}

func agoalListResult(collection, description string, paged bool, sensitivePaths ...string) *contract.ResultSpec {
	properties := map[string]any{
		"count": map[string]any{"type": "integer", "description": "当前结果中的业务记录数量"},
		collection: map[string]any{
			"type": "array", "description": description,
			"items": map[string]any{"type": "object", "description": "经稳定身份验证的 Agoal 业务记录", "additionalProperties": true},
		},
	}
	required := []string{"count", collection}
	if paged {
		properties["page"] = map[string]any{"type": "integer", "description": "服务端确认的当前页码"}
		properties["pageSize"] = map[string]any{"type": "integer", "description": "服务端确认的当前页容量"}
		properties["totalCount"] = map[string]any{"type": "integer", "description": "服务端确认的全部匹配记录数"}
		required = append(required, "page", "pageSize", "totalCount")
	}
	schema, _ := json.Marshal(map[string]any{
		"type": "object", "description": description, "additionalProperties": false,
		"properties": properties, "required": required,
	})
	return &contract.ResultSpec{
		Outcomes:       []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema:     schema,
		SensitivePaths: append([]string(nil), sensitivePaths...),
	}
}

var ReportStatisticsList = shortcut.Shortcut{
	Service:     productAgoal,
	Command:     "+report-statistics-list",
	Product:     productAgoal,
	Description: "查询周月报规则提交统计",
	Intent:      "当你要盘点周报或月报规则的按时、迟交、未提交汇总，或按规则关键词定位统计项时使用；返回严格验证的统计列表。",
	Risk:        shortcut.RiskRead,
	Safety:      agoalReadSafety(),
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID: productAgoal, Name: "shortcut_report_statistics_list",
			CanonicalPath: "agoal.shortcut_report_statistics_list", CLIPath: "agoal +report-statistics-list", PrimaryCLIPath: "agoal +report-statistics-list",
		},
		Description: "查询周月报规则提交统计",
		Interface: &contract.InterfaceSpec{
			Mode: "composite", Availability: "available",
			Reason: "Reviewed Agoal adapter validates the terminal success marker, collection type, every stable templateId, and the legal empty result before unified output.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "查询周月报规则提交统计",
			UseWhen:      []string{"当你要盘点周报或月报规则的按时、迟交、未提交汇总，或按规则关键词定位统计项时使用；返回严格验证的统计列表。"},
			AvoidWhen:    []string{"需要查看某一规则下的人员级提交详情时不要使用；该能力尚未完成隐私与分页闭环"},
			Examples:     []string{"dws agoal +report-statistics-list --keyword <KEYWORD>"},
		},
		Result: agoalListResult("statistics", "周月报规则提交统计记录", false,
			"statistics.title", "statistics.lastModifier", "statistics.content.name"),
	},
	Flags: []shortcut.Flag{{Name: "keyword", Type: shortcut.FlagString, Desc: "规则名称关键词；传入保证不匹配的关键词可得到合法空集合"}},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{}
		if rt.Changed("keyword") {
			params["keyword"] = rt.Str("keyword")
		}
		raw, err := rt.CallMCPData(productAgoal, "list_report_statistics", params)
		if err != nil {
			return err
		}
		if _, err := requireAgoalSuccess(raw, "agoal/list_report_statistics"); err != nil {
			return err
		}
		items, err := requireAgoalList(raw, "agoal/list_report_statistics", raw["content"], "templateId")
		if err != nil {
			return err
		}
		return rt.Output(map[string]any{"count": len(items), "statistics": items})
	},
}

var ObjectTemplateList = shortcut.Shortcut{
	Service:     productAgoal,
	Command:     "+obj-template-list",
	Product:     productAgoal,
	Description: "分页查询 Agoal 目标模板",
	Intent:      "当你要盘点或按关键词查找 Agoal 目标模板，并查看服务端确认的页码、页容量和总数时使用；不会修改模板。",
	Risk:        shortcut.RiskRead,
	Safety:      agoalReadSafety(),
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID: productAgoal, Name: "shortcut_obj_template_list",
			CanonicalPath: "agoal.shortcut_obj_template_list", CLIPath: "agoal +obj-template-list", PrimaryCLIPath: "agoal +obj-template-list",
		},
		Description: "分页查询 Agoal 目标模板",
		Interface: &contract.InterfaceSpec{
			Mode: "composite", Availability: "available",
			Reason: "Reviewed Agoal adapter validates success, result array, stable template IDs, and explicit page/pageSize/totalCount facts without inventing cursor pagination.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "分页查询 Agoal 目标模板",
			UseWhen:      []string{"当你要盘点或按关键词查找 Agoal 目标模板，并查看服务端确认的页码、页容量和总数时使用；不会修改模板。"},
			AvoidWhen:    []string{"需要新增或覆盖更新模板时不要使用；写能力尚未具备安全回滚合同"},
			Examples:     []string{"dws agoal +obj-template-list --keyword <KEYWORD> --page 1 --page-size 20"},
		},
		Result: agoalListResult("templates", "Agoal 目标模板查询结果", true,
			"templates.title", "templates.creator", "templates.dimensions.title"),
	},
	Flags: []shortcut.Flag{
		{Name: "keyword", Type: shortcut.FlagString, Desc: "模板关键词；传入保证不匹配的关键词可得到合法空集合"},
		{Name: "page", Type: shortcut.FlagInt, Default: "1", Desc: "页码，必须大于等于 1"},
		{Name: "page-size", Type: shortcut.FlagInt, Default: "20", Desc: "每页数量，必须在 1 到 100 之间"},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintCustom, Flags: []string{"page"}, Description: "页码，必须大于等于 1"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"page-size"}, Description: "每页数量，必须在 1 到 100 之间"},
	},
	Validate: func(rt *shortcut.RuntimeContext) error {
		if rt.Int("page") < 1 {
			return apperrors.NewValidation("--page 必须大于等于 1")
		}
		if size := rt.Int("page-size"); size < 1 || size > 100 {
			return apperrors.NewValidation("--page-size 必须在 1 到 100 之间")
		}
		return nil
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		wantPage := rt.Int("page")
		wantSize := rt.Int("page-size")
		params := map[string]any{"page": wantPage, "pageSize": wantSize}
		if rt.Changed("keyword") {
			params["keyword"] = rt.Str("keyword")
		}
		raw, err := rt.CallMCPData(productAgoal, "list_obj_template", params)
		if err != nil {
			return err
		}
		if _, err := requireAgoalSuccess(raw, "agoal/list_obj_template"); err != nil {
			return err
		}
		content, ok := raw["content"].(map[string]any)
		if !ok {
			return agoalResponseError("agoal/list_obj_template", "malformed_business_result", "成功响应 content 不是分页对象")
		}
		items, err := requireAgoalList(raw, "agoal/list_obj_template", content["result"], "id", "templateId")
		if err != nil {
			return err
		}
		page, pageOK := agoalNonNegativeInt(content["page"])
		pageSize, pageSizeOK := agoalNonNegativeInt(content["pageSize"])
		total, totalOK := agoalNonNegativeInt(content["totalCount"])
		if !pageOK || !pageSizeOK || !totalOK {
			return agoalResponseError("agoal/list_obj_template", "malformed_pagination", "分页响应缺少非负整数 page/pageSize/totalCount")
		}
		if page != wantPage || pageSize != wantSize {
			return agoalResponseError("agoal/list_obj_template", "pagination_mismatch", "服务端分页回显与请求不一致")
		}
		if total < len(items) {
			return agoalResponseError("agoal/list_obj_template", "impossible_total", "totalCount 小于当前页记录数")
		}
		return rt.Output(map[string]any{
			"count": len(items), "templates": items,
			"page": page, "pageSize": pageSize, "totalCount": total,
		})
	},
}

var ContractFields = shortcut.Shortcut{
	Service: productAgoal, Command: "+contract-fields", Product: productAgoal,
	Description: "查询经营合约字段配置",
	Intent:      "当你要查看当前组织可用的经营合约字段，或按字段 ID、编码、标题、分类、类型精确定位配置时使用；不会修改合约。",
	Risk:        shortcut.RiskRead,
	Safety:      agoalReadSafety(),
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID: productAgoal, Name: "shortcut_contract_fields",
			CanonicalPath: "agoal.shortcut_contract_fields", CLIPath: "agoal +contract-fields", PrimaryCLIPath: "agoal +contract-fields",
		},
		Description: "查询并严格筛选经营合约字段配置",
		Interface: &contract.InterfaceSpec{
			Mode: "composite", Availability: "available",
			Reason: "Reviewed adapter requires success, an explicit content array, and a stable id on every field; the optional keyword is applied locally across reviewed scalar field metadata so known and guaranteed-zero cases are provable.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "查询经营合约字段配置",
			UseWhen:      []string{"当你要查看当前组织可用的经营合约字段，或按字段 ID、编码、标题、分类、类型精确定位配置时使用；不会修改合约。"},
			AvoidWhen:    []string{"查询具体经营合约不要使用；当前合约列表和详情缺少非空安全 fixture"},
			Examples:     []string{"dws agoal +contract-fields --keyword <KEYWORD>"},
		},
		Result: agoalListResult("fields", "经营合约字段配置", false),
	},
	Flags: []shortcut.Flag{{Name: "keyword", Type: shortcut.FlagString, Desc: "字段 ID、编码、标题、分类或类型关键词；本地大小写不敏感筛选"}},
	Execute: func(rt *shortcut.RuntimeContext) error {
		raw, err := rt.CallMCPData(productAgoal, "list_op_contract_fields", map[string]any{})
		if err != nil {
			return err
		}
		if _, err := requireAgoalSuccess(raw, "agoal/list_op_contract_fields"); err != nil {
			return err
		}
		fields, err := requireAgoalList(raw, "agoal/list_op_contract_fields", raw["content"], "id")
		if err != nil {
			return err
		}
		fields = filterAgoalItems(fields, rt.Str("keyword"), "id", "code", "title", "category", "type")
		return rt.Output(map[string]any{"count": len(fields), "fields": fields})
	},
}

var UserRules = shortcut.Shortcut{
	Service: productAgoal, Command: "+user-rules", Product: productAgoal,
	Description: "查询用户 Agoal 规则周期",
	Intent:      "当你要查看本人或指定用户可见的 Agoal 规则及周期，或按稳定 ruleId 精确确认某项规则时使用。",
	Risk:        shortcut.RiskRead,
	Safety:      agoalReadSafety(),
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID: productAgoal, Name: "shortcut_user_rules",
			CanonicalPath: "agoal.shortcut_user_rules", CLIPath: "agoal +user-rules", PrimaryCLIPath: "agoal +user-rules",
		},
		Description: "查询用户规则周期并可按稳定 ruleId 精确筛选",
		Interface: &contract.InterfaceSpec{
			Mode: "composite", Availability: "available",
			Reason: "Reviewed adapter requires success, a content object, an explicit rules array, and stable rule IDs; the optional rule-id filter is local exact equality and provides a guaranteed-zero proof path.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "查询用户 Agoal 规则周期",
			UseWhen:      []string{"当你要查看本人或指定用户可见的 Agoal 规则及周期，或按稳定 ruleId 精确确认某项规则时使用。"},
			AvoidWhen:    []string{"需要目标正文时使用 +user-objectives；该目标列表当前缺少非空安全 fixture"},
			Examples:     []string{"dws agoal +user-rules --rule-id <RULE_ID>"},
		},
		Result: agoalListResult("rules", "用户可见的 Agoal 规则与周期", false,
			"rules.ruleName", "rules.periodFilter.currentPeriods.nameCn", "rules.periodFilter.historyPeriods.nameCn"),
	},
	Flags: []shortcut.Flag{
		{Name: "user-id", Type: shortcut.FlagString, Desc: "可选用户稳定 ID；省略时查询本人"},
		{Name: "rule-id", Type: shortcut.FlagString, Desc: "可选稳定 ruleId；Shortcut 在严格验证完整数组后做精确等值筛选"},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{}
		if rt.Changed("user-id") {
			params["dingUserId"] = rt.Str("user-id")
		}
		raw, err := rt.CallMCPData(productAgoal, "get_user_rules", params)
		if err != nil {
			return err
		}
		if _, err := requireAgoalSuccess(raw, "agoal/get_user_rules"); err != nil {
			return err
		}
		content, ok := raw["content"].(map[string]any)
		if !ok {
			return agoalResponseError("agoal/get_user_rules", "malformed_business_result", "成功响应 content 不是规则对象")
		}
		rules, err := requireAgoalList(raw, "agoal/get_user_rules", content["rules"], "id")
		if err != nil {
			return err
		}
		wantRule := strings.TrimSpace(rt.Str("rule-id"))
		if wantRule != "" {
			filtered := make([]map[string]any, 0, 1)
			for _, rule := range rules {
				if agoalFirstString(rule, "id") == wantRule {
					filtered = append(filtered, rule)
				}
			}
			rules = filtered
		}
		return rt.Output(map[string]any{"count": len(rules), "rules": rules})
	},
}

var ReportSubmitDetail = shortcut.Shortcut{
	Service: productAgoal, Command: "+report-submit-detail", Product: productAgoal,
	Description: "分页查询周月报人员提交详情",
	Intent:      "当你要按周月报规则、提交状态、日期或人员关键词查看具体人员提交详情时使用；返回严格验证的人员身份和页码事实。",
	Risk:        shortcut.RiskRead,
	Safety:      agoalReadSafety(),
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID: productAgoal, Name: "shortcut_report_submit_detail",
			CanonicalPath: "agoal.shortcut_report_submit_detail", CLIPath: "agoal +report-submit-detail", PrimaryCLIPath: "agoal +report-submit-detail",
		},
		Description: "分页查询周月报人员提交详情",
		Interface: &contract.InterfaceSpec{
			Mode: "composite", Availability: "available",
			Reason: "Reviewed adapter requires success, a paged content object, an explicit result array, stable nested user identity on every row, exact page/pageSize echo, and a valid totalCount. Because the lower keyword is ignored, keyword queries use one bounded full traversal with total consistency, duplicate, stall, and page-limit guards before local filtering; only reviewed personnel fields are projected and declared sensitive.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "分页查询周月报人员提交详情",
			UseWhen:      []string{"当你要按周月报规则、提交状态、日期或人员关键词查看具体人员提交详情时使用；返回严格验证的人员身份和页码事实。"},
			AvoidWhen:    []string{"只需规则级汇总时使用 +report-statistics-list；不要把人员级原始结果写入审计文档"},
			Examples:     []string{"dws agoal +report-submit-detail --template-id <TEMPLATE_ID> --submit-state NOT_SUBMITTED --page 1 --page-size 20"},
		},
		Result: agoalListResult("submissions", "周月报人员提交详情", true,
			"submissions.reportId", "submissions.user.dingUserId", "submissions.user.id", "submissions.user.name", "submissions.user.workNo"),
	},
	Flags: []shortcut.Flag{
		{Name: "template-id", Type: shortcut.FlagString, Desc: "周月报规则模板稳定 ID", Required: true},
		{Name: "submit-state", Type: shortcut.FlagString, Desc: "提交状态", Required: true, Enum: []string{"ON_TIME", "LATE", "NOT_SUBMITTED"}},
		{Name: "query-date", Type: shortcut.FlagString, Desc: "可选 ISO-8601 日期或时间"},
		{Name: "keyword", Type: shortcut.FlagString, Desc: "可选人员名称关键词；可用于保证零命中"},
		{Name: "page", Type: shortcut.FlagInt, Default: "1", Desc: "页码，必须大于等于 1"},
		{Name: "page-size", Type: shortcut.FlagInt, Default: "20", Desc: "每页数量，必须在 1 到 100 之间"},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintCustom, Flags: []string{"page"}, Description: "页码，必须大于等于 1"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"page-size"}, Description: "每页数量，必须在 1 到 100 之间"},
	},
	Validate: func(rt *shortcut.RuntimeContext) error {
		if rt.Int("page") < 1 {
			return apperrors.NewValidation("--page 必须大于等于 1")
		}
		if size := rt.Int("page-size"); size < 1 || size > 100 {
			return apperrors.NewValidation("--page-size 必须在 1 到 100 之间")
		}
		if rt.Changed("query-date") {
			if _, err := normalizeAgoalQueryDate(rt.Str("query-date")); err != nil {
				return err
			}
		}
		return nil
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		wantPage, wantSize := rt.Int("page"), rt.Int("page-size")
		baseParams := map[string]any{
			"templateId": rt.Str("template-id"), "submitState": strings.ToUpper(rt.Str("submit-state")),
		}
		if rt.Changed("query-date") {
			// Validate has already accepted this exact immutable Cobra value.
			queryDate, _ := normalizeAgoalQueryDate(rt.Str("query-date"))
			baseParams["queryDate"] = queryDate
		}
		keyword := strings.TrimSpace(rt.Str("keyword"))
		if keyword == "" {
			items, total, err := readAgoalSubmissionPage(rt, baseParams, wantPage, wantSize)
			if err != nil {
				return err
			}
			return rt.Output(map[string]any{"count": len(items), "submissions": items, "page": wantPage, "pageSize": wantSize, "totalCount": total})
		}
		all, err := readAllAgoalSubmissions(rt, baseParams)
		if err != nil {
			return err
		}
		filtered := filterAgoalSubmissions(all, keyword)
		start := len(filtered)
		pageOffset := wantPage - 1
		if pageOffset <= len(filtered)/wantSize {
			start = pageOffset * wantSize
		}
		end := start + wantSize
		if end > len(filtered) {
			end = len(filtered)
		}
		items := filtered[start:end]
		return rt.Output(map[string]any{"count": len(items), "submissions": items, "page": wantPage, "pageSize": wantSize, "totalCount": len(filtered)})
	},
}

const (
	agoalSubmissionReadPageSize = 100
	agoalSubmissionMaxPages     = 20
)

func readAgoalSubmissionPage(rt *shortcut.RuntimeContext, baseParams map[string]any, page, pageSize int) ([]map[string]any, int, error) {
	params := make(map[string]any, len(baseParams)+2)
	for key, value := range baseParams {
		params[key] = value
	}
	params["page"] = page
	params["pageSize"] = pageSize
	raw, err := rt.CallMCPData(productAgoal, "get_submit_detail", params)
	if err != nil {
		return nil, 0, err
	}
	if _, err := requireAgoalSuccess(raw, "agoal/get_submit_detail"); err != nil {
		return nil, 0, err
	}
	content, ok := raw["content"].(map[string]any)
	if !ok {
		return nil, 0, agoalResponseError("agoal/get_submit_detail", "malformed_business_result", "成功响应 content 不是分页对象")
	}
	items, err := requireAgoalSubmissionList(content["result"])
	if err != nil {
		return nil, 0, err
	}
	gotPage, pageOK := agoalNonNegativeInt(content["page"])
	gotSize, sizeOK := agoalNonNegativeInt(content["pageSize"])
	total, totalOK := agoalNonNegativeInt(content["totalCount"])
	if !pageOK || !sizeOK || !totalOK {
		return nil, 0, agoalResponseError("agoal/get_submit_detail", "malformed_pagination", "分页响应缺少非负整数 page/pageSize/totalCount")
	}
	if gotPage != page || gotSize != pageSize {
		return nil, 0, agoalResponseError("agoal/get_submit_detail", "pagination_mismatch", "服务端分页回显与请求不一致")
	}
	if total < len(items) {
		return nil, 0, agoalResponseError("agoal/get_submit_detail", "impossible_total", "totalCount 小于当前页记录数")
	}
	return items, total, nil
}

func readAllAgoalSubmissions(rt *shortcut.RuntimeContext, baseParams map[string]any) ([]map[string]any, error) {
	all := make([]map[string]any, 0)
	seen := map[string]bool{}
	wantTotal := -1
	for page := 1; page <= agoalSubmissionMaxPages; page++ {
		items, total, err := readAgoalSubmissionPage(rt, baseParams, page, agoalSubmissionReadPageSize)
		if err != nil {
			return nil, err
		}
		if wantTotal < 0 {
			wantTotal = total
		} else if total != wantTotal {
			return nil, agoalResponseError("agoal/get_submit_detail", "pagination_total_changed", "分页遍历期间 totalCount 发生变化")
		}
		for _, item := range items {
			identity := agoalSubmissionIdentity(item)
			if seen[identity] {
				return nil, agoalResponseError("agoal/get_submit_detail", "pagination_duplicate", "分页遍历出现重复人员提交身份")
			}
			seen[identity] = true
			all = append(all, item)
		}
		if len(all) == wantTotal {
			return all, nil
		}
		if len(items) == 0 {
			return nil, agoalResponseError("agoal/get_submit_detail", "pagination_stall", "尚有记录时服务返回空页，分页无法推进")
		}
	}
	return nil, agoalResponseError("agoal/get_submit_detail", "readback_page_limit", "人员提交详情超过有界分页上限")
}

func agoalSubmissionIdentity(item map[string]any) string {
	if reportID := agoalFirstString(item, "reportId"); reportID != "" {
		return "report:" + reportID
	}
	user, _ := item["user"].(map[string]any)
	return "user:" + agoalFirstString(user, "dingUserId", "id")
}

func filterAgoalSubmissions(items []map[string]any, keyword string) []map[string]any {
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	if keyword == "" {
		return items
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		user, _ := item["user"].(map[string]any)
		if strings.Contains(strings.ToLower(agoalFirstString(user, "name")), keyword) {
			out = append(out, item)
		}
	}
	return out
}

func filterAgoalItems(items []map[string]any, keyword string, keys ...string) []map[string]any {
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	if keyword == "" {
		return items
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		for _, key := range keys {
			value, ok := item[key].(string)
			if ok && strings.Contains(strings.ToLower(value), keyword) {
				out = append(out, item)
				break
			}
		}
	}
	return out
}

func requireAgoalSubmissionList(value any) ([]map[string]any, error) {
	rawItems, ok := value.([]any)
	if !ok {
		return nil, agoalResponseError("agoal/get_submit_detail", "malformed_collection", "成功响应 result 不是数组")
	}
	out := make([]map[string]any, 0, len(rawItems))
	for index, raw := range rawItems {
		item, ok := raw.(map[string]any)
		if !ok {
			return nil, agoalResponseError("agoal/get_submit_detail", "malformed_item", fmt.Sprintf("人员提交详情第 %d 项不是对象", index))
		}
		user, ok := item["user"].(map[string]any)
		if !ok || agoalFirstString(user, "dingUserId", "id") == "" {
			return nil, agoalResponseError("agoal/get_submit_detail", "missing_item_id", fmt.Sprintf("人员提交详情第 %d 项缺少稳定用户身份", index))
		}
		projectedUser := map[string]any{}
		for _, key := range []string{"dingUserId", "id", "name", "workNo"} {
			if value, found := user[key]; found {
				projectedUser[key] = value
			}
		}
		projected := map[string]any{"user": projectedUser}
		for _, key := range []string{"reportId", "publishTime"} {
			if value, found := item[key]; found {
				projected[key] = value
			}
		}
		out = append(out, projected)
	}
	return out, nil
}

func normalizeAgoalQueryDate(value string) (string, error) {
	value = strings.TrimSpace(value)
	if parsed, err := time.Parse("2006-01-02", value); err == nil {
		return parsed.Format("2006-01-02"), nil
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		location, loadErr := agoalLoadLocation("Asia/Shanghai")
		if loadErr != nil {
			location = time.FixedZone("CST", 8*60*60)
		}
		return parsed.In(location).Format("2006-01-02"), nil
	}
	return "", apperrors.NewValidation("--query-date 必须是 YYYY-MM-DD 或 RFC3339 时间")
}

type unavailableAgoalSpec struct {
	command     string
	description string
	intent      string
	tool        string
	flags       []shortcut.Flag
	write       bool
	reason      string
}

func agoalStringFlag(name, description string, required bool) shortcut.Flag {
	return shortcut.Flag{Name: name, Type: shortcut.FlagString, Desc: description, Required: required}
}

func unavailableAgoalShortcut(spec unavailableAgoalSpec) shortcut.Shortcut {
	flagNames := make([]string, 0, len(spec.flags))
	for _, flag := range spec.flags {
		flagNames = append(flagNames, flag.Name)
	}
	unavailable := func(*shortcut.RuntimeContext) error {
		return apperrors.NewValidation("该 Agoal Shortcut 当前 unavailable：" + spec.reason)
	}
	return shortcut.Shortcut{
		Service: productAgoal, Command: spec.command, Product: productAgoal,
		Description: spec.description, Intent: spec.intent,
		Risk: func() shortcut.Risk {
			if spec.write {
				return shortcut.RiskHighWrite
			}
			return shortcut.RiskRead
		}(),
		Safety:       agoalUnavailableSafety(spec.write),
		Flags:        spec.flags,
		Hidden:       true,
		Availability: shortcut.AvailabilityUnavailable,
		Constraints: []shortcut.Constraint{{
			Kind: shortcut.ConstraintCustom, Flags: flagNames,
			Description: "该命令保持 unavailable，直到文档化的安全验证前置条件满足",
		}},
		SemanticDelta: "Atomic route " + spec.tool + " remains unavailable: " + spec.reason,
		Validate:      unavailable,
		Execute:       unavailable,
	}
}

var unavailableAgoalSpecs = []unavailableAgoalSpec{
	{command: "+strategy-list", description: "按范围查询战略解码", intent: "按部门或个人范围查询战略解码列表。", tool: "list_strategy_decodings", flags: []shortcut.Flag{
		{Name: "scope-type", Type: shortcut.FlagString, Desc: "范围类型", Required: true, Enum: []string{"DEPT", "PERSONAL"}},
		agoalStringFlag("scope-id", "部门或人员稳定 ID", true),
	}, reason: "exact+raw 在当前可访问 PERSONAL 与 DEPT 范围均仅返回显式空数组，缺少已知非空 fixture"},
	{command: "+strategy-get", description: "查询战略解码详情", intent: "按 profileId 查询单个战略解码。", tool: "get_strategy_decoding_detail", flags: []shortcut.Flag{
		agoalStringFlag("profile-id", "战略解码稳定 ID", true),
	}, reason: "raw 对不存在 profileId 返回 null 且无 success 终态，列表又无非空 fixture，无法绑定稳定身份"},
	{command: "+strategy-update", description: "覆盖更新战略解码", intent: "基于旧数据覆盖更新战略解码。", tool: "update_strategy_decoding", write: true, flags: []shortcut.Flag{
		agoalStringFlag("profile-id", "战略解码稳定 ID", true), agoalStringFlag("content", "完整实体 JSON 数组", true),
	}, reason: "raw 仅完成 dry-run payload；没有可读旧值 fixture、全字段精确读回与恢复闭环"},
	{command: "+contract-list", description: "按范围查询经营合约", intent: "按部门或个人范围查询经营合约列表。", tool: "list_op_contracts", flags: []shortcut.Flag{
		{Name: "scope-type", Type: shortcut.FlagString, Desc: "范围类型", Required: true, Enum: []string{"DEPT", "PERSONAL"}},
		agoalStringFlag("scope-id", "部门或人员稳定 ID", true),
	}, reason: "exact+raw 在当前可访问 PERSONAL 与 DEPT 范围均仅返回显式空数组，缺少已知非空 fixture"},
	{command: "+contract-get", description: "查询经营合约详情", intent: "按 contractId 查询单个经营合约。", tool: "get_op_contract_detail", flags: []shortcut.Flag{
		agoalStringFlag("contract-id", "经营合约稳定 ID", true),
	}, reason: "raw 对不存在 contractId 返回 null 且无 success 终态，列表又无非空 fixture，无法绑定稳定身份"},
	{command: "+contract-update", description: "覆盖更新经营合约", intent: "基于旧数据覆盖经营合约全部维度。", tool: "update_op_contract", write: true, flags: []shortcut.Flag{
		agoalStringFlag("contract-id", "经营合约稳定 ID", true), agoalStringFlag("dimensions", "完整维度 JSON 数组", true),
	}, reason: "raw 仅完成 dry-run payload；没有可读旧值 fixture、全字段精确读回与恢复闭环"},
	{command: "+scorecard-get", description: "查询计分卡详情", intent: "按部门与周期查询计分卡。", tool: "get_score_card_detail", flags: []shortcut.Flag{
		agoalStringFlag("selected-time", "ISO-8601 周期起始时间", true), agoalStringFlag("dept-id", "部门稳定 ID", true),
	}, reason: "exact+raw 对可访问部门和多个受控周期均返回 null 且无 success 终态，缺少非空计分卡 fixture"},
	{command: "+scorecard-entity-get", description: "查询计分卡实体详情", intent: "按计分卡与实体 ID 查询详情。", tool: "get_score_card_entity_detail", flags: []shortcut.Flag{
		agoalStringFlag("sc-id", "计分卡稳定 ID", true), agoalStringFlag("entity-id", "实体稳定 ID", true),
	}, reason: "raw 对不存在计分卡/实体双 ID 返回 null 且无 success 终态，无法取得可安全读取的双 ID fixture"},
	{command: "+scorecard-update", description: "覆盖更新计分卡", intent: "基于旧数据覆盖更新计分卡内容。", tool: "update_score_card", write: true, flags: []shortcut.Flag{
		agoalStringFlag("dept-id", "部门稳定 ID", true), agoalStringFlag("selected-time", "ISO-8601 周期起始时间", true), agoalStringFlag("id", "计分卡稳定 ID", true),
		{Name: "tracking-period-type", Type: shortcut.FlagString, Desc: "跟踪周期", Required: true, Enum: []string{"MONTHLY", "QUARTERLY"}},
		agoalStringFlag("content", "完整计分卡 JSON 数组", true),
	}, reason: "raw 仅完成 dry-run payload；没有可读计分卡 fixture、全字段精确读回与恢复闭环"},
	{command: "+user-objectives", description: "查询用户目标", intent: "按用户、规则与周期查询目标。", tool: "list_user_objectives", flags: []shortcut.Flag{
		agoalStringFlag("user-id", "人员稳定 ID", true), agoalStringFlag("rule-id", "规则稳定 ID", true), agoalStringFlag("period-ids", "周期 ID CSV", true),
	}, reason: "exact+raw 使用当前用户及全部可见规则/周期均只返回显式空数组，缺少已知非空目标 fixture"},
	{command: "+obj-template-upsert", description: "新增或覆盖更新目标模板", intent: "新增目标模板或基于旧数据覆盖更新模板。", tool: "create_or_update_obj_template", write: true, flags: []shortcut.Flag{
		agoalStringFlag("template-id", "更新时的模板稳定 ID", false), agoalStringFlag("title", "新增时的模板标题", false), agoalStringFlag("dimensions", "完整维度 JSON", true),
		{Name: "objective-weight", Type: shortcut.FlagBool, Desc: "是否启用目标权重"},
		{Name: "dimension-weight", Type: shortcut.FlagBool, Desc: "是否启用维度权重"},
		{Name: "compute-by-weight", Type: shortcut.FlagBool, Desc: "维度是否参与计算"},
	}, reason: "raw 仅完成 dry-run payload；原子面无模板删除或版本恢复，无法验证 create/update 终态与零残留"},
}

func init() {
	contract.RegisterProductDecl(contract.ProductDecl{
		ID: productAgoal,
		Selection: contract.ProductSelectionDecl{
			AgentSummary: "查询钉钉 Agoal 的周月报统计、人员提交详情、合约字段、用户规则与目标模板；其余能力按安全证据逐步开放",
			UseWhen:      []string{"用户明确处理钉钉 Agoal、战略解码、经营合约、计分卡、周月报统计或目标模板时使用"},
			AvoidWhen:    []string{"用户处理 Lark OKR 的 Objective、Key Result、Progress 或 Alignment 时不要使用；两者数据模型和平台不同"},
		},
	})
	items := []shortcut.Shortcut{ReportStatisticsList, ObjectTemplateList, ContractFields, UserRules, ReportSubmitDetail}
	for _, spec := range unavailableAgoalSpecs {
		items = append(items, unavailableAgoalShortcut(spec))
	}
	for index := range items {
		if items[index].Availability != shortcut.AvailabilityUnavailable {
			items[index].OutputRollout = output.RolloutUnifiedActive
		}
	}
	shortcut.Register(items...)
}
