// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

// Package oa registers strict declarative shortcuts for DingTalk OA approval.
package oa

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

const oaApprovalListDateLayout = "2006-01-02"

type oaApprovalListOptions struct {
	includeCreateBefore bool
	includeLegacyRange  bool
	includeUnreadOnly   bool
	includeStatus       bool
}

var oaApprovalListStringProperties = []struct {
	flag     string
	property string
}{
	{flag: "query", property: "query"},
	{flag: "process-code", property: "processCode"},
	{flag: "originator-user-id", property: "originatorUserId"},
	{flag: "create-time-from", property: "createTimeFrom"},
	{flag: "create-time-to", property: "createTimeTo"},
	{flag: "finish-time-from", property: "finishTimeFrom"},
	{flag: "finish-time-to", property: "finishTimeTo"},
}

func oaApprovalListParamDecls(options oaApprovalListOptions) []contract.ParamDecl {
	params := []contract.ParamDecl{
		{Name: "page"},
		{Name: "limit"},
	}
	for _, binding := range oaApprovalListStringProperties {
		params = append(params, contract.ParamDecl{Name: binding.flag, Property: binding.property})
	}
	if options.includeStatus {
		params = append(params, contract.ParamDecl{Name: "process-instance-status", Property: "processInstanceStatus"})
	}
	if options.includeCreateBefore {
		params = append(params, contract.ParamDecl{Name: "create-before", Property: "createBefore"})
	}
	if options.includeLegacyRange {
		params = append(params,
			contract.ParamDecl{Name: "start"},
			contract.ParamDecl{Name: "end"},
		)
	}
	if options.includeUnreadOnly {
		params = append(params, contract.ParamDecl{Name: "unread-only", Property: "unreadOnly"})
	}
	return params
}

func oaApprovalListFlags(options oaApprovalListOptions) []shortcut.Flag {
	pageDefault, limitDefault := "1", "20"
	if options.includeLegacyRange {
		pageDefault, limitDefault = "", ""
	}
	flags := []shortcut.Flag{
		{Name: "page", Type: shortcut.FlagString, Default: pageDefault, Desc: "分页页码；必须大于 0"},
		{Name: "limit", Type: shortcut.FlagString, Default: limitDefault, Desc: "每页大小；必须在 1-100"},
		{Name: "query", Type: shortcut.FlagString, Desc: "关键字搜索"},
		{Name: "process-code", Type: shortcut.FlagString, Desc: "审批模板 code"},
		{Name: "originator-user-id", Type: shortcut.FlagString, Desc: "审批单发起人 userId"},
		{Name: "create-time-from", Type: shortcut.FlagString, Desc: "发起时间起始，格式 yyyy-MM-dd"},
		{Name: "create-time-to", Type: shortcut.FlagString, Desc: "发起时间截止，格式 yyyy-MM-dd（含当日）"},
		{Name: "finish-time-from", Type: shortcut.FlagString, Desc: "审批完成时间起始，格式 yyyy-MM-dd"},
		{Name: "finish-time-to", Type: shortcut.FlagString, Desc: "审批完成时间截止，格式 yyyy-MM-dd（含当日）"},
	}
	if options.includeStatus {
		flags = append(flags, shortcut.Flag{Name: "process-instance-status", Type: shortcut.FlagString, Desc: "审批状态，如 NEW/RUNNING/COMPLETED/TERMINATED"})
	}
	if options.includeCreateBefore {
		flags = append(flags, shortcut.Flag{Name: "create-before", Type: shortcut.FlagString, Desc: "创建时间"})
	}
	if options.includeLegacyRange {
		flags = append(flags,
			shortcut.Flag{Name: "start", Type: shortcut.FlagInt, Desc: "兼容参数：发起时间起始（epoch 毫秒）", Required: true},
			shortcut.Flag{Name: "end", Type: shortcut.FlagInt, Desc: "兼容参数：发起时间截止（epoch 毫秒）", Required: true},
		)
	}
	if options.includeUnreadOnly {
		flags = append(flags, shortcut.Flag{Name: "unread-only", Type: shortcut.FlagBool, Desc: "仅查询未读抄送审批"})
	}
	return flags
}

func parseOAApprovalListPage(rt *shortcut.RuntimeContext, name string, fallback int) (int, error) {
	raw := strings.TrimSpace(rt.Str(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, apperrors.NewValidation(fmt.Sprintf("--%s 必须是整数", name))
	}
	return value, nil
}

func validateOAApprovalList(rt *shortcut.RuntimeContext, options oaApprovalListOptions) error {
	page, err := parseOAApprovalListPage(rt, "page", 1)
	if err != nil {
		return err
	}
	limit, err := parseOAApprovalListPage(rt, "limit", 20)
	if err != nil {
		return err
	}
	if err := validateOAPage(page, limit); err != nil {
		return err
	}
	if options.includeLegacyRange && (rt.Int("start") <= 0 || rt.Int("end") <= rt.Int("start")) {
		return apperrors.NewValidation("--start/--end 必须是递增的正整数 epoch 毫秒范围")
	}
	for _, flag := range []string{"create-time-from", "create-time-to", "finish-time-from", "finish-time-to"} {
		value := strings.TrimSpace(rt.Str(flag))
		if value == "" {
			continue
		}
		if _, err := time.Parse(oaApprovalListDateLayout, value); err != nil {
			return apperrors.NewValidation("--" + flag + " 必须是 yyyy-MM-dd 格式")
		}
	}
	for _, pair := range [][2]string{{"create-time-from", "create-time-to"}, {"finish-time-from", "finish-time-to"}} {
		fromRaw := strings.TrimSpace(rt.Str(pair[0]))
		toRaw := strings.TrimSpace(rt.Str(pair[1]))
		if fromRaw == "" || toRaw == "" {
			continue
		}
		from, _ := time.Parse(oaApprovalListDateLayout, fromRaw)
		to, _ := time.Parse(oaApprovalListDateLayout, toRaw)
		if from.After(to) {
			return apperrors.NewValidation("--" + pair[0] + " 不能晚于 --" + pair[1])
		}
	}
	return nil
}

func oaApprovalListParams(rt *shortcut.RuntimeContext, options oaApprovalListOptions) map[string]any {
	page, _ := parseOAApprovalListPage(rt, "page", 1)
	limit, _ := parseOAApprovalListPage(rt, "limit", 20)
	params := map[string]any{"pageNumber": page, "pageSize": limit}
	for _, binding := range oaApprovalListStringProperties {
		if value := strings.TrimSpace(rt.Str(binding.flag)); value != "" {
			params[binding.property] = value
		}
	}
	if options.includeStatus {
		if value := strings.TrimSpace(rt.Str("process-instance-status")); value != "" {
			params["processInstanceStatus"] = value
		}
	}
	if options.includeCreateBefore {
		if value := strings.TrimSpace(rt.Str("create-before")); value != "" {
			params["createBefore"] = value
		}
	}
	if options.includeLegacyRange {
		legacyZone := time.FixedZone("Asia/Shanghai", 8*3600)
		for _, binding := range []struct {
			flag     string
			property string
		}{{flag: "start", property: "createTimeFrom"}, {flag: "end", property: "createTimeTo"}} {
			if _, exists := params[binding.property]; exists {
				continue
			}
			params[binding.property] = time.UnixMilli(int64(rt.Int(binding.flag))).In(legacyZone).Format(oaApprovalListDateLayout)
		}
	}
	if options.includeUnreadOnly && rt.Changed("unread-only") {
		params["unreadOnly"] = rt.Bool("unread-only")
	}
	return params
}

func oaInstancePage(rt *shortcut.RuntimeContext, tool string, params map[string]any, page int) error {
	operation := "oa/" + tool
	data, err := rt.CallMCPData("oa", tool, params)
	if err != nil {
		return err
	}
	instances, err := oaProjectInstances(data, operation, "result.values")
	if err != nil {
		return err
	}
	result, _ := data["result"].(map[string]any)
	evidence, err := oaHasMorePage(result, operation, page)
	if err != nil {
		return err
	}
	return outputOAPage(rt, "instances", instances, evidence)
}

var ListPending = shortcut.Shortcut{
	Service: "oa", Command: "+list-pending", Product: "oa",
	Description:   "查询当前登录用户待处理的审批任务列表",
	Intent:        "按页码、日期、模板和发起人等条件查询待处理审批；只有显式成功、严格实例数组与可续页证据齐全时才返回结果。",
	Risk:          shortcut.RiskRead,
	Safety:        oaReadSafety(),
	OutputRollout: output.RolloutUnifiedActive,
	Contract: oaContract(
		"+list-pending",
		"查询当前登录用户待处理的审批任务列表",
		"需要按日期、模板或发起人读取待我审批的实例，并取得稳定 processInstanceId 时使用；没有安全非空待办 fixture 前不会进入公开发现。",
		true,
		oaCollectionResult("instances", "严格验证的待处理审批实例页"),
		oaPagePagination("page"),
		oaApprovalListParamDecls(oaApprovalListOptions{includeCreateBefore: true, includeLegacyRange: true}),
		"dws oa +list-pending --start 1785513600000 --end 1788191999000 --page 1 --limit 20",
	),
	Flags:       oaApprovalListFlags(oaApprovalListOptions{includeCreateBefore: true, includeLegacyRange: true}),
	Constraints: []shortcut.Constraint{{Kind: shortcut.ConstraintCustom, Flags: []string{"start", "end", "page", "limit", "create-time-from", "create-time-to", "finish-time-from", "finish-time-to"}, Description: "--start/--end 必须是递增的正整数 epoch 毫秒；--page 必须大于 0；--limit 必须在 1-100；日期筛选必须为 yyyy-MM-dd 且起始不晚于截止"}},
	Tips:        []string{`dws oa +list-pending --start 1785513600000 --end 1788191999000 --page 1 --limit 20`},
	Validate: func(rt *shortcut.RuntimeContext) error {
		return validateOAApprovalList(rt, oaApprovalListOptions{includeCreateBefore: true, includeLegacyRange: true})
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		page, _ := parseOAApprovalListPage(rt, "page", 1)
		return oaInstancePage(rt, "get_todo_tasks", oaApprovalListParams(rt, oaApprovalListOptions{includeCreateBefore: true, includeLegacyRange: true}), page)
	},
}

var ListForms = shortcut.Shortcut{
	Service: "oa", Command: "+list-forms", Product: "oa",
	Description:   "获取当前用户可见的审批表单列表",
	Intent:        "按服务端游标读取当前用户可发起的审批表单；缺少 hasMore/nextCursor 或游标不前进时失败，不把重复首页宣称为完整列表。",
	Risk:          shortcut.RiskRead,
	Safety:        oaReadSafety(),
	OutputRollout: output.RolloutUnifiedActive,
	Contract: oaContract(
		"+list-forms", "获取当前用户可见的审批表单列表",
		"需要枚举可发起审批定义并取得稳定 processCode 时使用；当前下游不返回可验证 continuation，故不进入 Agent 公开发现。",
		true,
		oaCollectionResult("forms", "严格验证的可见审批表单页"), oaPagePagination("cursor"),
		[]contract.ParamDecl{{Name: "cursor", Property: "cursor"}, {Name: "limit", Property: "limit"}},
		"dws oa +list-forms --cursor 0 --limit 100",
	),
	Flags: []shortcut.Flag{
		{Name: "cursor", Type: shortcut.FlagInt, Default: "0", Desc: "分页游标，首次传 0"},
		{Name: "limit", Type: shortcut.FlagInt, Default: "100", Desc: "每页大小，最大 100"},
	},
	Constraints: []shortcut.Constraint{{Kind: shortcut.ConstraintCustom, Flags: []string{"cursor", "limit"}, Description: "--cursor 不能小于 0；--limit 必须在 1-100"}},
	Tips:        []string{`dws oa +list-forms --cursor 0 --limit 100`},
	Validate: func(rt *shortcut.RuntimeContext) error {
		if rt.Int("cursor") < 0 {
			return apperrors.NewValidation("--cursor 不能小于 0")
		}
		if rt.Int("limit") <= 0 || rt.Int("limit") > 100 {
			return apperrors.NewValidation("--limit 必须在 1 到 100 之间")
		}
		return nil
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		const operation = "oa/list_user_visible_process"
		data, err := rt.CallMCPData("oa", "list_user_visible_process", map[string]any{"cursor": rt.Int("cursor"), "pageSize": rt.Int("limit")})
		if err != nil {
			return err
		}
		forms, err := oaProjectForms(data, operation, "result.processCodeList")
		if err != nil {
			return err
		}
		result, _ := data["result"].(map[string]any)
		page, err := oaCursorPage(result, operation, rt.Int("cursor"))
		if err != nil {
			return err
		}
		return outputOAPage(rt, "forms", forms, page)
	},
}

var SearchForms = shortcut.Shortcut{
	Service: "oa", Command: "+search-forms", Product: "oa",
	Description:   "按关键字模糊搜索当前用户可见的审批表单",
	Intent:        "已知审批定义关键字，需要取得一个或多个稳定 processCode 时使用；要无条件遍历全部定义不要使用本命令。",
	Risk:          shortcut.RiskRead,
	Safety:        oaReadSafety(),
	OutputRollout: output.RolloutUnifiedActive,
	Contract: oaContract(
		"+search-forms", "按关键字模糊搜索当前用户可见的审批表单",
		"已知审批定义关键字，需要取得一个或多个稳定 processCode 时使用；要无条件遍历全部定义不要使用本命令。",
		true,
		oaCollectionResult("forms", "严格验证的审批表单搜索结果"), nil,
		[]contract.ParamDecl{{Name: "query", Property: "query"}},
		"dws oa +search-forms --query 报销",
	),
	Flags:       []shortcut.Flag{{Name: "query", Type: shortcut.FlagString, Desc: "关键字（匹配 processCode 或表单名称）；去除空白后不能为空", Required: true}},
	Constraints: []shortcut.Constraint{{Kind: shortcut.ConstraintCustom, Flags: []string{"query"}, Description: "--query 去除空白后不能为空"}},
	Tips:        []string{`dws oa +search-forms --query 报销`},
	Validate: func(rt *shortcut.RuntimeContext) error {
		if strings.TrimSpace(rt.Str("query")) == "" {
			return apperrors.NewValidation("--query 不能为空")
		}
		return nil
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		const operation = "oa/search_form"
		data, err := rt.CallMCPData("oa", "search_form", map[string]any{"query": strings.TrimSpace(rt.Str("query"))})
		if err != nil {
			return err
		}
		forms, err := oaProjectForms(data, operation, "result")
		if err != nil {
			return err
		}
		return outputOACompleteCollection(rt, "forms", forms)
	},
}

func oaNumberedInstanceShortcut(command, tool, description, intent string, options oaApprovalListOptions) shortcut.Shortcut {
	declaration := shortcut.Shortcut{
		Service: "oa", Command: command, Product: "oa",
		Description: description, Intent: intent, Risk: shortcut.RiskRead,
		Safety: oaReadSafety(), OutputRollout: output.RolloutUnifiedActive,
		Contract: oaContract(command, description, intent, true,
			oaCollectionResult("instances", description), oaPagePagination("page"),
			oaApprovalListParamDecls(options),
			"dws oa "+command+" --page 1 --limit 20"),
		Flags: oaApprovalListFlags(options),
		Constraints: []shortcut.Constraint{
			{Kind: shortcut.ConstraintCustom, Flags: []string{"page"}, Description: "--page 必须大于 0"},
			{Kind: shortcut.ConstraintCustom, Flags: []string{"limit"}, Description: "--limit 必须在 1-100"},
		},
		Tips: []string{"dws oa " + command + " --page 1 --limit 20"},
	}
	declaration.Validate = func(rt *shortcut.RuntimeContext) error {
		return validateOAApprovalList(rt, options)
	}
	declaration.Execute = func(rt *shortcut.RuntimeContext) error {
		page, _ := parseOAApprovalListPage(rt, "page", 1)
		return oaInstancePage(rt, tool, oaApprovalListParams(rt, options), page)
	}
	return declaration
}

var ListExecuted = oaNumberedInstanceShortcut(
	"+list-executed", "get_done_tasks", "获取当前用户已经处理过的审批单列表",
	"需要回顾当前用户已同意或拒绝过的审批实例时使用；与待办、已发起和抄送列表分开。",
	oaApprovalListOptions{includeStatus: true},
)

var ListSubmitted = oaNumberedInstanceShortcut(
	"+list-submitted", "get_submitted_instances", "获取当前用户已发起的审批单列表",
	"需要查看当前用户发起的审批实例和当前状态时使用；返回稳定 processInstanceId 与可续页证据。",
	oaApprovalListOptions{includeStatus: true},
)

var ListCc = oaNumberedInstanceShortcut(
	"+list-cc", "get_noticed_instances", "获取抄送当前用户的审批单列表",
	"需要查看抄送给当前用户的审批实例时使用；没有安全非空抄送 fixture 前不会进入公开发现。",
	oaApprovalListOptions{includeUnreadOnly: true},
)

func init() {
	shortcut.Register(ListPending, ListForms, SearchForms, ListExecuted, ListSubmitted, ListCc)
}
