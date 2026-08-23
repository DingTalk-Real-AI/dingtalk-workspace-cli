// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

// Package oa registers strict declarative shortcuts for DingTalk OA approval.
package oa

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func parseOAStringPage(rt *shortcut.RuntimeContext, name string, fallback int) (int, error) {
	raw := rt.Str(name)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, apperrors.NewValidation(fmt.Sprintf("--%s 必须是整数", name))
	}
	return value, nil
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
	Description:   "查询待我处理的审批（时间范围为 epoch 毫秒）",
	Intent:        "按 epoch 毫秒时间范围查询当前用户待处理审批；只有显式成功、严格实例数组与可续页证据齐全时才返回结果。",
	Risk:          shortcut.RiskRead,
	Safety:        oaReadSafety(),
	OutputRollout: output.RolloutUnifiedActive,
	Contract: oaContract(
		"+list-pending",
		"查询待我处理的审批（时间范围为 epoch 毫秒）",
		"需要按时间范围读取待我审批的实例，并取得稳定 processInstanceId 时使用；没有安全非空待办 fixture 前不会进入公开发现。",
		true,
		oaCollectionResult("instances", "严格验证的待处理审批实例页"),
		oaPagePagination("page"),
		[]contract.ParamDecl{
			{Name: "start", Property: "start"},
			{Name: "end", Property: "end"},
			{Name: "page", Property: "page"},
			{Name: "limit", Property: "limit"},
			{Name: "query", Property: "query"},
		},
		"dws oa +list-pending --start 1741536000000 --end 1741622399000 --page 1 --limit 20",
	),
	Flags: []shortcut.Flag{
		{Name: "start", Type: shortcut.FlagInt, Desc: "开始时间（epoch 毫秒）", Required: true},
		{Name: "end", Type: shortcut.FlagInt, Desc: "结束时间（epoch 毫秒）", Required: true},
		{Name: "page", Type: shortcut.FlagString, Desc: "分页页码"},
		{Name: "limit", Type: shortcut.FlagString, Desc: "每页大小"},
		{Name: "query", Type: shortcut.FlagString, Desc: "关键字搜索"},
	},
	Constraints: []shortcut.Constraint{{Kind: shortcut.ConstraintCustom, Flags: []string{"start", "end", "page", "limit"}, Description: "--start/--end 必须是递增的正整数 epoch 毫秒；--page 必须大于 0；--limit 必须在 1-100"}},
	Tips:        []string{`dws oa +list-pending --start 1741536000000 --end 1741622399000 --page 1 --limit 20`},
	Validate: func(rt *shortcut.RuntimeContext) error {
		if rt.Int("start") <= 0 || rt.Int("end") <= rt.Int("start") {
			return apperrors.NewValidation("--start/--end 必须是递增的正整数 epoch 毫秒范围")
		}
		page, err := parseOAStringPage(rt, "page", 1)
		if err != nil {
			return err
		}
		limit, err := parseOAStringPage(rt, "limit", 20)
		if err != nil {
			return err
		}
		return validateOAPage(page, limit)
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		page, _ := parseOAStringPage(rt, "page", 1)
		params := map[string]any{"starTime": rt.Int("start"), "endTime": rt.Int("end")}
		if rt.Changed("page") {
			params["pageNum"] = rt.Str("page")
		}
		if rt.Changed("limit") {
			params["pageSize"] = rt.Str("limit")
		}
		if rt.Changed("query") {
			params["query"] = rt.Str("query")
		}
		return oaInstancePage(rt, "list_pending_approvals", params, page)
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

func oaNumberedInstanceShortcut(command, tool, description, intent string) shortcut.Shortcut {
	declaration := shortcut.Shortcut{
		Service: "oa", Command: command, Product: "oa",
		Description: description, Intent: intent, Risk: shortcut.RiskRead,
		Safety: oaReadSafety(), OutputRollout: output.RolloutUnifiedActive,
		Contract: oaContract(command, description, intent, true,
			oaCollectionResult("instances", description), oaPagePagination("page"),
			[]contract.ParamDecl{{Name: "page", Property: "page"}, {Name: "limit", Property: "limit"}, {Name: "query", Property: "query"}},
			"dws oa "+command+" --page 1 --limit 20"),
		Flags: []shortcut.Flag{
			{Name: "page", Type: shortcut.FlagString, Default: "1", Desc: "分页页码；--page 必须大于 0"},
			{Name: "limit", Type: shortcut.FlagString, Default: "20", Desc: "每页大小；--limit 必须在 1-100"},
			{Name: "query", Type: shortcut.FlagString, Desc: "关键字搜索"},
		},
		Constraints: []shortcut.Constraint{
			{Kind: shortcut.ConstraintCustom, Flags: []string{"page"}, Description: "--page 必须大于 0"},
			{Kind: shortcut.ConstraintCustom, Flags: []string{"limit"}, Description: "--limit 必须在 1-100"},
		},
		Tips: []string{"dws oa " + command + " --page 1 --limit 20"},
	}
	declaration.Validate = func(rt *shortcut.RuntimeContext) error {
		page, err := parseOAStringPage(rt, "page", 1)
		if err != nil {
			return err
		}
		limit, err := parseOAStringPage(rt, "limit", 20)
		if err != nil {
			return err
		}
		return validateOAPage(page, limit)
	}
	declaration.Execute = func(rt *shortcut.RuntimeContext) error {
		page, _ := parseOAStringPage(rt, "page", 1)
		params := map[string]any{"pageNumber": rt.Str("page"), "pageSize": rt.Str("limit")}
		if query := strings.TrimSpace(rt.Str("query")); query != "" {
			params["query"] = query
		}
		return oaInstancePage(rt, tool, params, page)
	}
	return declaration
}

var ListExecuted = oaNumberedInstanceShortcut(
	"+list-executed", "get_done_tasks", "获取当前用户已经处理过的审批单列表",
	"需要回顾当前用户已同意或拒绝过的审批实例时使用；与待办、已发起和抄送列表分开。",
)

var ListSubmitted = oaNumberedInstanceShortcut(
	"+list-submitted", "get_submitted_instances", "获取当前用户已发起的审批单列表",
	"需要查看当前用户发起的审批实例和当前状态时使用；返回稳定 processInstanceId 与可续页证据。",
)

var ListCc = oaNumberedInstanceShortcut(
	"+list-cc", "get_noticed_instances", "获取抄送当前用户的审批单列表",
	"需要查看抄送给当前用户的审批实例时使用；没有安全非空抄送 fixture 前不会进入公开发现。",
)

func init() {
	shortcut.Register(ListPending, ListForms, SearchForms, ListExecuted, ListSubmitted, ListCc)
}
