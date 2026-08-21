// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

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

func oaFirstPageOnly(rt *shortcut.RuntimeContext, tool, collection string, params map[string]any) error {
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
	page, err := oaHasMorePage(result, operation, 1)
	if err != nil {
		return err
	}
	if page.HasMore {
		return oaResponseError(operation, "unavailable_continuation", "兼容 Shortcut 没有页码参数且结果仍有后续页；请改用对应可分页命令")
	}
	return outputOACompleteCollection(rt, collection, instances)
}

var PendingApprovals = shortcut.Shortcut{
	Service: "oa", Command: "+pending", Product: "oa",
	Description:   "只读列出待我审批的审批任务并投影为可读列表（只看不批）",
	Intent:        "兼容入口：读取近三个月待处理审批的首个完整页；无法证明完整或缺少非空 fixture 时不进入 Agent 公开发现。",
	Risk:          shortcut.RiskRead,
	Safety:        oaReadSafety(),
	OutputRollout: output.RolloutUnifiedActive,
	Contract: oaContract(
		"+pending", "只读列出待我审批的审批任务并投影为可读列表（只看不批）",
		"兼容旧的待审批摘要入口；新调用优先使用支持显式时间和页码的 +list-pending。",
		true,
		oaCollectionResult("pending", "严格验证的待审批摘要"), nil,
		[]contract.ParamDecl{{Name: "limit", Property: "limit"}}, "dws oa +pending --limit 10",
	),
	Flags:       []shortcut.Flag{{Name: "limit", Type: shortcut.FlagInt, Desc: "最多列出多少条（可选）"}},
	Constraints: []shortcut.Constraint{{Kind: shortcut.ConstraintCustom, Flags: []string{"limit"}, Description: "显式 --limit 必须在 1-100"}},
	Tips:        []string{`dws oa +pending --limit 10`},
	Validate: func(rt *shortcut.RuntimeContext) error {
		if rt.Changed("limit") && (rt.Int("limit") <= 0 || rt.Int("limit") > 100) {
			return apperrors.NewValidation("--limit 必须在 1 到 100 之间")
		}
		return nil
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		now := time.Now()
		params := map[string]any{"starTime": float64(now.AddDate(0, 0, -90).UnixMilli()), "endTime": float64(now.UnixMilli())}
		if rt.Changed("limit") {
			params["pageSize"] = float64(rt.Int("limit"))
		}
		return oaFirstPageOnly(rt, "list_pending_approvals", "pending", params)
	},
}

var DoneApprovals = shortcut.Shortcut{
	Service: "oa", Command: "+done-approvals", Product: "oa",
	Description:   "只读列出我已处理过的审批任务（审批历史）并投影为可读列表",
	Intent:        "兼容入口：读取我已处理审批的首个完整页；新调用优先使用可分页的 +list-executed。",
	Risk:          shortcut.RiskRead,
	Safety:        oaReadSafety(),
	OutputRollout: output.RolloutUnifiedActive,
	Contract: oaContract(
		"+done-approvals", "只读列出我已处理过的审批任务（审批历史）并投影为可读列表",
		"兼容旧的已处理审批摘要入口；需要翻页或搜索时使用 +list-executed。",
		true,
		oaCollectionResult("done", "严格验证的已处理审批摘要"), nil,
		[]contract.ParamDecl{{Name: "limit", Property: "limit"}}, "dws oa +done-approvals --limit 10",
	),
	Flags:       []shortcut.Flag{{Name: "limit", Type: shortcut.FlagInt, Desc: "最多列出多少条（可选）"}},
	Constraints: []shortcut.Constraint{{Kind: shortcut.ConstraintCustom, Flags: []string{"limit"}, Description: "显式 --limit 必须在 1-100"}},
	Tips:        []string{`dws oa +done-approvals --limit 10`},
	Validate: func(rt *shortcut.RuntimeContext) error {
		if rt.Changed("limit") && (rt.Int("limit") <= 0 || rt.Int("limit") > 100) {
			return apperrors.NewValidation("--limit 必须在 1 到 100 之间")
		}
		return nil
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		limit := 20
		if rt.Changed("limit") {
			limit = rt.Int("limit")
		}
		return oaFirstPageOnly(rt, "get_done_tasks", "done", map[string]any{"pageNumber": float64(1), "pageSize": float64(limit)})
	},
}

var MyInitiated = shortcut.Shortcut{
	Service: "oa", Command: "+my-initiated", Product: "oa",
	Description:   "列出我发起（提交）的审批单据",
	Intent:        "需要兼容旧的 initiated 输出字段时使用；一般列表与分页可直接使用 +list-submitted。",
	Risk:          shortcut.RiskRead,
	Safety:        oaReadSafety(),
	OutputRollout: output.RolloutUnifiedActive,
	Contract: oaContract(
		"+my-initiated", "列出我发起（提交）的审批单据",
		"需要兼容旧的 initiated 输出字段时使用；一般列表与分页可直接使用 +list-submitted。",
		true,
		oaCollectionResult("initiated", "严格验证的已发起审批实例页"), oaPagePagination("page"),
		[]contract.ParamDecl{{Name: "query", Property: "query"}, {Name: "page", Property: "page"}, {Name: "limit", Property: "limit"}},
		"dws oa +my-initiated --page 1 --limit 20", "dws oa +my-initiated --query 报销",
	),
	Flags: []shortcut.Flag{
		{Name: "query", Type: shortcut.FlagString, Desc: "关键字搜索（可选）"},
		{Name: "page", Type: shortcut.FlagInt, Desc: "分页页码（可选，默认 1）；--page 必须大于 0", Default: "1"},
		{Name: "limit", Type: shortcut.FlagInt, Desc: "每页大小（可选，默认 20）；--limit 必须在 1-100", Default: "20"},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintCustom, Flags: []string{"page"}, Description: "--page 必须大于 0"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"limit"}, Description: "--limit 必须在 1-100"},
	},
	Tips:     []string{`dws oa +my-initiated`, `dws oa +my-initiated --query 报销`, `dws oa +my-initiated --page 2 --limit 50`},
	Validate: func(rt *shortcut.RuntimeContext) error { return validateOAPage(rt.Int("page"), rt.Int("limit")) },
	Execute: func(rt *shortcut.RuntimeContext) error {
		const operation = "oa/get_submitted_instances"
		params := map[string]any{"pageNumber": float64(rt.Int("page")), "pageSize": float64(rt.Int("limit"))}
		if query := rt.Str("query"); query != "" {
			params["query"] = query
		}
		data, err := rt.CallMCPData("oa", "get_submitted_instances", params)
		if err != nil {
			return err
		}
		items, err := oaProjectInstances(data, operation, "result.values")
		if err != nil {
			return err
		}
		result, _ := data["result"].(map[string]any)
		page, err := oaHasMorePage(result, operation, rt.Int("page"))
		if err != nil {
			return err
		}
		return outputOAPage(rt, "initiated", items, page)
	},
}

type oaApprovalMatch struct {
	id    string
	title string
}

func oaMatchApprovals(items []map[string]any, keyword string) []oaApprovalMatch {
	needle := strings.ToLower(strings.TrimSpace(keyword))
	matches := make([]oaApprovalMatch, 0)
	for _, item := range items {
		id := oaIdentity(item, "processInstanceId")
		title := oaFirstString(item, "title", "processInstanceTitle")
		businessID := oaIdentity(item, "businessId")
		if strings.Contains(strings.ToLower(title), needle) || strings.Contains(strings.ToLower(businessID), needle) || strings.EqualFold(id, needle) {
			matches = append(matches, oaApprovalMatch{id: id, title: title})
		}
	}
	return matches
}

var Approve = shortcut.Shortcut{
	Service: "oa", Command: "+approve-by", Product: "oa",
	Description:   "按关键词把我的一条待审批单据一键通过（自动定位实例与任务 ID）",
	Intent:        "高风险兼容编排：完整读取并唯一匹配待办、唯一解析 taskId、确认后同意，再精确读回该 taskId 已不在待处理集合；没有安全 fixture 前不进入 Agent 公开发现。",
	Risk:          shortcut.RiskHighWrite,
	Safety:        oaWriteSafety(),
	OutputRollout: output.RolloutUnifiedActive,
	Contract: oaContract(
		"+approve-by", "按关键词把我的一条待审批单据一键通过（自动定位实例与任务 ID）",
		"仅在用户明确确认同意、关键词唯一定位实例且任务读回可验证时使用；任何歧义、分页不完整或读回失败都会阻止成功。",
		true,
		oaWriteResult("同意审批并通过精确任务读回验证"), nil,
		[]contract.ParamDecl{{Name: "keyword", Property: "query"}, {Name: "comment", Property: "remark"}},
		"dws oa +approve-by --keyword 报销",
	),
	Flags: []shortcut.Flag{
		{Name: "keyword", Type: shortcut.FlagString, Desc: "待审批单据的单号或标题关键词", Required: true},
		{Name: "comment", Type: shortcut.FlagString, Desc: "审批意见（可选）"},
	},
	Constraints: []shortcut.Constraint{{Kind: shortcut.ConstraintCustom, Flags: []string{"keyword"}, Description: "--keyword 去除空白后不能为空，且必须唯一匹配完整待办集合中的一条实例"}},
	Tips:        []string{`dws oa +approve-by --keyword 报销`, `dws oa +approve-by --keyword 出差单 --comment "同意"`},
	Validate: func(rt *shortcut.RuntimeContext) error {
		keyword := strings.TrimSpace(rt.Str("keyword"))
		if keyword == "" {
			return apperrors.NewValidation("--keyword 不能为空")
		}
		return nil
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		keyword := strings.TrimSpace(rt.Str("keyword"))
		now := time.Now()
		pending, err := rt.CallMCPData("oa", "list_pending_approvals", map[string]any{
			"starTime": float64(now.AddDate(0, 0, -90).UnixMilli()), "endTime": float64(now.UnixMilli()), "query": keyword,
		})
		if err != nil {
			return err
		}
		items, err := oaProjectInstances(pending, "oa/list_pending_approvals", "result.values")
		if err != nil {
			return err
		}
		result, _ := pending["result"].(map[string]any)
		page, err := oaHasMorePage(result, "oa/list_pending_approvals", 1)
		if err != nil {
			return err
		}
		if page.HasMore {
			return oaResponseError("oa/list_pending_approvals", "ambiguous_incomplete_search", "待办搜索仍有后续页，无法证明关键词唯一")
		}
		matches := oaMatchApprovals(items, keyword)
		if len(matches) != 1 {
			return apperrors.NewValidation(fmt.Sprintf("关键词必须唯一匹配一条待审批实例，当前匹配 %d 条", len(matches)))
		}
		instanceID := matches[0].id
		tasksData, err := rt.CallMCPData("oa", "list_pending_tasks", map[string]any{"processInstanceId": instanceID})
		if err != nil {
			return err
		}
		tasks, err := oaProjectTasks(tasksData, "oa/list_pending_tasks")
		if err != nil {
			return err
		}
		if len(tasks) != 1 {
			return apperrors.NewValidation(fmt.Sprintf("审批实例必须唯一对应一条待处理任务，当前为 %d 条", len(tasks)))
		}
		taskID := tasks[0]["taskId"].(string)
		numericTaskID, err := strconv.ParseFloat(taskID, 64)
		if err != nil {
			return oaResponseError("oa/list_pending_tasks", "malformed_task_identity", "taskId 不是可写入审批接口的数字")
		}
		writeArgs := map[string]any{"processInstanceId": instanceID, "taskId": numericTaskID}
		if comment := rt.Str("comment"); comment != "" {
			writeArgs["remark"] = comment
		}
		receipt, err := rt.CallMCPWriteDataStrict("oa", "approve_processInstance", writeArgs)
		if err != nil {
			return err
		}
		if err := oaRequireSuccess(receipt, "oa/approve_processInstance"); err != nil {
			return err
		}
		readback, err := rt.CallMCPData("oa", "list_pending_tasks", map[string]any{"processInstanceId": instanceID})
		if err != nil {
			return oaPostWriteError("oa/list_pending_tasks", "readback_failed", "审批写入后无法读取任务状态；远端效果未知")
		}
		remaining, err := oaProjectTasks(readback, "oa/list_pending_tasks")
		if err != nil {
			return oaPostWriteError("oa/list_pending_tasks", "readback_malformed", "审批写入后的任务读回无法验证；远端效果未知")
		}
		for _, task := range remaining {
			if task["taskId"] == taskID {
				return oaPostWriteError("oa/list_pending_tasks", "write_not_observed", "审批任务写后读回仍处于待处理集合")
			}
		}
		return rt.Output(map[string]any{"processInstanceId": instanceID, "taskId": taskID, "verified": true})
	},
}

func init() {
	shortcut.Register(PendingApprovals, DoneApprovals, Approve, MyInitiated)
}
