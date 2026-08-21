// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package todo

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

const todoCompositeReason = "Reviewed Todo Shortcut composite: the executable CLI owns strict response validation, pagination, write/read-back verification, output projection, and confirmation; no single MCP interface represents the complete command contract."

func todoContract(command, description, useWhen string, result *contract.ResultSpec, examples ...string) corecmd.ContractDecl {
	name := "shortcut_" + strings.ReplaceAll(strings.TrimPrefix(command, "+"), "-", "_")
	cliPath := "todo " + command
	return corecmd.ContractDecl{
		Description: description,
		Result:      result,
		Identity: contract.ToolIdentitySpec{
			ProductID: "todo", Name: name, CanonicalPath: "todo." + name,
			CLIPath: cliPath, PrimaryCLIPath: cliPath,
		},
		Interface: &contract.InterfaceSpec{Mode: contract.InterfaceModeComposite, Availability: contract.InterfaceAvailable, Reason: todoCompositeReason},
		Selection: contract.SelectionSpec{
			AgentSummary: description,
			UseWhen:      []string{useWhen},
			AvoidWhen:    []string{"需要底层未公开参数或原始响应时改用对应原子命令"},
			Examples:     examples,
		},
	}
}

func todoWriteSafety(idempotency string) contract.SafetySpec {
	return contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: idempotency}
}

func todoReadSafety() contract.SafetySpec {
	return contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"}
}

func parseTodoMillis(flag, value string) (int64, error) {
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	if err != nil {
		return 0, apperrors.NewValidation(fmt.Sprintf("--%s 必须是 ISO8601 时间", flag))
	}
	return parsed.UnixMilli(), nil
}

func createTodo(rt *shortcut.RuntimeContext) error {
	title := rt.Str("title")
	executors := nonEmptyStrings(rt.StrSlice("executors"))
	if title == "" || len(executors) == 0 {
		return apperrors.NewValidation("--title 与至少一个 --executors 均为必填")
	}
	vo := map[string]any{"subject": title, "executorIds": executors}
	if rt.Changed("due") {
		millis, err := parseTodoMillis("due", rt.Str("due"))
		if err != nil {
			return err
		}
		vo["dueTime"] = millis
	}
	if rt.Changed("priority") {
		priority := rt.Int("priority")
		if priority != 10 && priority != 20 && priority != 30 && priority != 40 {
			return apperrors.NewValidation("--priority 仅接受 10/20/30/40")
		}
		vo["priority"] = priority
	}
	if rt.DryRun() {
		return rt.Output(map[string]any{"dryRun": true, "executed": false, "operation": "create", "subject": title})
	}
	data, err := rt.CallMCPWriteDataStrict("todo", "create_personal_todo", map[string]any{"PersonalTodoCreateVO": vo})
	if err != nil {
		return err
	}
	if err := requireTodoWriteReceipt(data, "todo/create_personal_todo"); err != nil {
		return err
	}
	taskID := todoCreatedTaskID(data)
	if taskID == "" {
		return todoResponseError("todo/create_personal_todo", "missing_stable_id", "创建响应缺少稳定 taskId；远端效果未知")
	}
	detail, err := readTodoDetail(rt, taskID)
	if err != nil {
		return err
	}
	if subject, _ := detail["subject"].(string); subject != title {
		return todoResponseError("todo/create_personal_todo", "verification_mismatch", "创建后读回的标题不一致")
	}
	return rt.Output(map[string]any{"taskId": taskID, "verified": true, "todo": detail})
}

var Create = shortcut.Shortcut{
	OutputRollout: output.RolloutUnifiedActive,
	Service:       "todo", Command: "+create", Product: "todo",
	Description: "创建待办并读回验证", Intent: "创建一条个人待办；只有取得稳定 taskId 且详情读回一致才报告成功。",
	Risk: shortcut.RiskWrite, Safety: todoWriteSafety("non_idempotent"),
	Contract: todoContract("+create", "创建待办并读回验证", "标题和执行人已确定，需要创建待办时使用", todoObjectResult("已验证的新建待办"), `dws todo +create --title "提交报告" --executors <USER_ID>`),
	Flags: []shortcut.Flag{
		{Name: "title", Type: shortcut.FlagString, Desc: "待办标题", Required: true},
		{Name: "executors", Type: shortcut.FlagStringSlice, Desc: "执行人 userId", Required: true},
		{Name: "due", Type: shortcut.FlagString, Desc: "截止时间（ISO8601）"},
		{Name: "priority", Type: shortcut.FlagInt, Desc: "优先级 10/20/30/40"},
	},
	Execute: createTodo,
}

func updateTodo(rt *shortcut.RuntimeContext) error {
	if !rt.Changed("title") && !rt.Changed("due") && !rt.Changed("priority") {
		return apperrors.NewValidation("至少指定 --title、--due、--priority 之一")
	}
	taskID := rt.Str("task-id")
	request := map[string]any{"taskId": taskID}
	if rt.Changed("title") {
		if rt.Str("title") == "" {
			return apperrors.NewValidation("--title 不能为空")
		}
		request["subject"] = rt.Str("title")
	}
	if rt.Changed("due") {
		millis, err := parseTodoMillis("due", rt.Str("due"))
		if err != nil {
			return err
		}
		request["dueTime"] = millis
	}
	if rt.Changed("priority") {
		priority := rt.Int("priority")
		if priority != 10 && priority != 20 && priority != 30 && priority != 40 {
			return apperrors.NewValidation("--priority 仅接受 10/20/30/40")
		}
		request["priority"] = priority
	}
	if rt.DryRun() {
		return rt.Output(map[string]any{"dryRun": true, "executed": false, "operation": "update", "taskId": taskID})
	}
	data, err := rt.CallMCPWriteDataStrict("todo", "update_todo_task", map[string]any{"TodoUpdateRequest": request})
	if err != nil {
		return err
	}
	if err := requireTodoWriteReceipt(data, "todo/update_todo_task"); err != nil {
		return err
	}
	detail, err := readTodoDetail(rt, taskID)
	if err != nil {
		return err
	}
	for key, expected := range request {
		if key == "taskId" {
			continue
		}
		if !todoUpdateFieldMatches(key, detail[key], expected) {
			return todoResponseError("todo/update_todo_task", "verification_mismatch", "更新后读回字段 "+key+" 不一致")
		}
	}
	return rt.Output(map[string]any{"taskId": taskID, "verified": true, "todo": detail})
}

func todoUpdateFieldMatches(key string, actual, expected any) bool {
	switch key {
	case "dueTime", "priority":
		actualNumber, actualOK := todoExactInteger(actual)
		expectedNumber, expectedOK := todoExactInteger(expected)
		return actualOK && expectedOK && actualNumber == expectedNumber
	case "subject":
		actualText, actualOK := actual.(string)
		expectedText, expectedOK := expected.(string)
		return actualOK && expectedOK && actualText == expectedText
	default:
		return false
	}
}

func todoExactInteger(value any) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int64:
		return typed, true
	case float64:
		return todoExactFloat(typed)
	case json.Number:
		parsed, err := typed.Int64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func todoExactFloat(value float64) (int64, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value ||
		value < float64(math.MinInt64) || value >= float64(math.MaxInt64) {
		return 0, false
	}
	return int64(value), true
}

var Update = shortcut.Shortcut{
	OutputRollout: output.RolloutUnifiedActive,
	Service:       "todo", Command: "+update", Product: "todo",
	Description: "更新待办并读回验证", Intent: "按 taskId 修改标题、截止时间或优先级，并严格读回核验。",
	Risk: shortcut.RiskWrite, Safety: todoWriteSafety("idempotent"),
	Contract: todoContract("+update", "更新待办并读回验证", "已知 taskId 且需要修改待办字段时使用", todoObjectResult("已验证的更新结果"), `dws todo +update --task-id <TASK_ID> --title "新标题"`),
	Flags: []shortcut.Flag{
		{Name: "task-id", Type: shortcut.FlagString, Desc: "待办 taskId", Required: true},
		{Name: "title", Type: shortcut.FlagString, Desc: "新标题"},
		{Name: "due", Type: shortcut.FlagString, Desc: "新截止时间（ISO8601）"},
		{Name: "priority", Type: shortcut.FlagInt, Desc: "新优先级 10/20/30/40"},
	},
	Execute: updateTodo,
}

func setTodoDone(rt *shortcut.RuntimeContext, target bool) error {
	taskID := rt.Str("task-id")
	if rt.DryRun() {
		return rt.Output(map[string]any{"dryRun": true, "executed": false, "taskId": taskID, "isDone": target})
	}
	before, err := readTodoDetail(rt, taskID)
	if err != nil {
		return err
	}
	current, ok := before["isDone"].(bool)
	if !ok {
		return todoResponseError("todo/get_todo_detail", "malformed_done_status", "待办详情缺少布尔 isDone，拒绝盲目写状态")
	}
	if current == target {
		return rt.Output(map[string]any{"taskId": taskID, "isDone": target, "verified": true, "alreadyInTargetState": true})
	}
	data, err := rt.CallMCPWriteDataStrict("todo", "update_todo_done_status", map[string]any{"taskId": taskID, "isDone": strconv.FormatBool(target)})
	if err != nil {
		return err
	}
	if err := requireTodoWriteReceipt(data, "todo/update_todo_done_status"); err != nil {
		return err
	}
	detail, err := readTodoDetail(rt, taskID)
	if err != nil {
		return err
	}
	done, ok := detail["isDone"].(bool)
	if !ok || done != target {
		return todoResponseError("todo/update_todo_done_status", "verification_mismatch", "完成状态读回不一致或缺失")
	}
	return rt.Output(map[string]any{"taskId": taskID, "isDone": target, "verified": true, "alreadyInTargetState": false})
}

func doneShortcut(command string, target bool) shortcut.Shortcut {
	description := "完成待办并读回验证"
	if !target {
		description = "重新打开待办并读回验证"
	}
	return shortcut.Shortcut{
		OutputRollout: output.RolloutUnifiedActive,
		Service:       "todo", Command: command, Product: "todo", Description: description, Intent: description,
		Risk: shortcut.RiskWrite, Safety: todoWriteSafety("idempotent"),
		Contract: todoContract(command, description, "已知 taskId 且需要改变完成状态时使用", todoObjectResult("已验证的待办状态"), "dws todo "+command+" --task-id <TASK_ID>"),
		Flags:    []shortcut.Flag{{Name: "task-id", Type: shortcut.FlagString, Desc: "待办 taskId", Required: true}},
		Execute:  func(rt *shortcut.RuntimeContext) error { return setTodoDone(rt, target) },
	}
}

var Complete = doneShortcut("+complete", true)
var Reopen = doneShortcut("+reopen", false)

var Search = shortcut.Shortcut{
	OutputRollout: output.RolloutUnifiedActive,
	Service:       "todo", Command: "+search", Product: "todo",
	Description: "搜索与我相关的全部待办", Intent: "分页遍历当前组织下与我相关的待办并按标题关键词匹配。",
	Risk: shortcut.RiskRead, Safety: todoReadSafety(),
	Contract: todoContract("+search", "搜索与我相关的全部待办", "需要按标题关键词跨全部分页搜索待办时使用", todoCollectionResult("todos", "待办搜索结果"), `dws todo +search --query "周报"`),
	Flags: []shortcut.Flag{
		{Name: "query", Type: shortcut.FlagString, Desc: "标题关键词", Required: true},
		{Name: "status", Type: shortcut.FlagString, Enum: []string{"true", "false"}, Desc: "完成状态"},
		{Name: "max-pages", Type: shortcut.FlagInt, Default: "40", Desc: "最大遍历页数（1-40）"},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		query := strings.ToLower(rt.Str("query"))
		if query == "" {
			return apperrors.NewValidation("--query 不能为空")
		}
		maxPages := rt.Int("max-pages")
		if maxPages < 1 || maxPages > todoMaxPages {
			return apperrors.NewValidation("--max-pages 必须是 1 到 40")
		}
		base := map[string]any{"roleTypes": []string{"creator", "executor", "participant"}}
		if rt.Changed("status") {
			base["todoStatus"] = rt.Str("status")
		}
		cards, err := listAllTodoCards(rt, base, maxPages)
		if err != nil {
			return err
		}
		matches := make([]map[string]any, 0)
		for _, card := range cards {
			subject, ok := card["subject"].(string)
			if !ok {
				return todoResponseError("todo/+search", "malformed_subject", "待办条目缺少字符串 subject")
			}
			if strings.Contains(strings.ToLower(subject), query) {
				matches = append(matches, card)
			}
		}
		return rt.Output(map[string]any{"count": len(matches), "todos": matches, "complete": true})
	},
}

var Comment = shortcut.Shortcut{
	OutputRollout: output.RolloutUnifiedActive,
	Service:       "todo", Command: "+comment", Product: "todo",
	Description: "添加待办评论并读回验证", Intent: "向指定待办添加评论；取得稳定 commentId 并从评论列表读回才报告成功。",
	Risk: shortcut.RiskWrite, Safety: todoWriteSafety("non_idempotent"),
	Contract: todoContract("+comment", "添加待办评论并读回验证", "已知 taskId 且需要发表明确评论时使用", todoObjectResult("已验证的新评论"), `dws todo +comment --task-id <TASK_ID> --content "已处理"`),
	Flags: []shortcut.Flag{
		{Name: "task-id", Type: shortcut.FlagString, Desc: "待办 taskId", Required: true},
		{Name: "content", Type: shortcut.FlagString, Desc: "评论内容", Required: true},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		taskID, content := rt.Str("task-id"), rt.Str("content")
		if content == "" {
			return apperrors.NewValidation("--content 不能为空")
		}
		if rt.DryRun() {
			return rt.Output(map[string]any{"dryRun": true, "executed": false, "taskId": taskID})
		}
		before, err := listAllTodoComments(rt, taskID)
		if err != nil {
			return err
		}
		beforeIDs := make(map[string]bool, len(before))
		for _, comment := range before {
			beforeIDs[todoStableString(comment, "commentId", "id")] = true
		}
		data, err := rt.CallMCPWriteDataStrict("todo", "add_todo_comment", map[string]any{"taskId": taskID, "content": content})
		if err != nil {
			return err
		}
		if err := requireTodoWriteReceipt(data, "todo/add_todo_comment"); err != nil {
			return err
		}
		after, err := listAllTodoComments(rt, taskID)
		if err != nil {
			return err
		}
		matches := make([]map[string]any, 0, 1)
		for _, comment := range after {
			id := todoStableString(comment, "commentId", "id")
			text, _ := comment["content"].(string)
			if id != "" && !beforeIDs[id] && text == content {
				matches = append(matches, comment)
			}
		}
		if len(matches) != 1 {
			return todoResponseError("todo/add_todo_comment", "verification_ambiguous", "评论写入后无法在完整列表中唯一识别新增评论")
		}
		commentID := todoStableString(matches[0], "commentId", "id")
		return rt.Output(map[string]any{"taskId": taskID, "commentId": commentID, "content": content, "verified": true})
	},
}

func listAllTodoComments(rt *shortcut.RuntimeContext, taskID string) ([]map[string]any, error) {
	all := make([]map[string]any, 0)
	for page := 1; page <= todoMaxPages; page++ {
		data, err := rt.CallMCPReadData("todo", "list_todo_comment", map[string]any{
			"taskId": taskID, "page": strconv.Itoa(page), "pageSize": strconv.Itoa(todoPageSize),
		})
		if err != nil {
			return nil, err
		}
		comments, err := listCommentProjectStrict(data)
		if err != nil {
			return nil, err
		}
		hasMore, err := todoHasMore(data)
		if err != nil {
			return nil, err
		}
		all = append(all, comments...)
		if !hasMore {
			return all, nil
		}
	}
	return nil, todoResponseError("todo/list_todo_comment", "pagination_limit_reached", "评论列表达到 40 页仍未耗尽，拒绝把不完整列表用于写后验证")
}

var Reminder = shortcut.Shortcut{
	OutputRollout: output.RolloutUnifiedActive,
	Service:       "todo", Command: "+reminder", Product: "todo",
	Description: "设置或清除待办提醒（仅终端回执）", Intent: "设置或清除提醒；上游无提醒读接口，结果固定 verified=false。",
	Risk: shortcut.RiskWrite, Safety: todoWriteSafety("unknown"),
	Contract: todoContract("+reminder", "设置或清除待办提醒（仅终端回执）", "接受无法读回核验且需要设置/清除提醒时使用", &contract.ResultSpec{
		Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess},
		DataSchema: json.RawMessage(`{"type":"object","description":"提醒写入终端回执","properties":{"taskId":{"type":"string","description":"待办 taskId"},"action":{"type":"string","description":"set 或 clear"},"terminalReceipt":{"type":"boolean","description":"是否取得终端成功回执"},"verified":{"type":"boolean","description":"固定 false；上游无法读回提醒规则"}},"required":["taskId","action","terminalReceipt","verified"],"additionalProperties":false}`),
	}, `dws todo +reminder --task-id <TASK_ID> --base-time dueTime --due-date-offset -30`),
	Flags: []shortcut.Flag{
		{Name: "task-id", Type: shortcut.FlagString, Desc: "待办 taskId", Required: true},
		{Name: "clear", Type: shortcut.FlagBool, Desc: "清除全部提醒规则"},
		{Name: "base-time", Type: shortcut.FlagString, Enum: []string{"dueTime", "customTime"}, Desc: "提醒基准"},
		{Name: "due-date-offset", Type: shortcut.FlagInt, Desc: "相对截止时间的分钟偏移"},
		{Name: "at", Type: shortcut.FlagString, Desc: "customTime 的 ISO8601 时间"},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		taskID := rt.Str("task-id")
		clear := rt.Bool("clear")
		baseTime := rt.Str("base-time")
		if clear == (baseTime != "") {
			return apperrors.NewValidation("必须且只能选择 --clear 或 --base-time")
		}
		tool := "reset_todo_reminder"
		params := map[string]any{"todoReminderUpdateRequest": map[string]any{"taskId": taskID, "reminderRules": []any{}}}
		action := "clear"
		if !clear {
			request := map[string]any{"taskId": taskID, "baseTime": baseTime}
			if baseTime == "dueTime" {
				if !rt.Changed("due-date-offset") {
					return apperrors.NewValidation("--base-time=dueTime 要求 --due-date-offset")
				}
				request["dueDateOffset"] = strconv.Itoa(rt.Int("due-date-offset"))
			} else {
				if !rt.Changed("at") {
					return apperrors.NewValidation("--base-time=customTime 要求 --at")
				}
				millis, err := parseTodoMillis("at", rt.Str("at"))
				if err != nil {
					return err
				}
				request["reminderTimeStamp"] = millis
			}
			tool = "add_todo_reminder"
			params = map[string]any{"todoReminderAddRequest": request}
			action = "set"
		}
		if rt.DryRun() {
			return rt.Output(map[string]any{"taskId": taskID, "action": action, "terminalReceipt": false, "verified": false, "dryRun": true})
		}
		data, err := rt.CallMCPWriteDataStrict("todo", tool, params)
		if err != nil {
			return err
		}
		if err := requireTodoWriteReceipt(data, "todo/"+tool); err != nil {
			return err
		}
		return rt.Output(map[string]any{"taskId": taskID, "action": action, "terminalReceipt": true, "verified": false})
	},
}

var UploadAttachment = shortcut.Shortcut{
	OutputRollout: output.RolloutUnifiedActive,
	Service:       "todo",
	Command:       "+upload-attachment",
	Product:       "todo",
	Description:   "上传待办附件（当前请使用原子命令）",
	Intent:        "当前 Shortcut 运行时不能安全复用原子命令的带请求头上传事务，因此明确路由到 todo task add-attachment。",
	Risk:          shortcut.RiskWrite,
	Safety:        todoWriteSafety("unknown"),
	Contract: todoContract(
		"+upload-attachment",
		"上传待办附件（当前请使用原子命令）",
		"需要上传本地文件为待办附件时，使用公开的原子命令完成完整上传事务",
		todoObjectResult("附件上传结果"),
		`dws todo +upload-attachment --task-id <TASK_ID> --file-path ./report.pdf`,
	),
	Flags: []shortcut.Flag{
		{Name: "task-id", Type: shortcut.FlagString, Desc: "待办 taskId", Required: true},
		{Name: "file-path", Type: shortcut.FlagString, Desc: "本地文件路径", Required: true},
	},
	Execute: func(*shortcut.RuntimeContext) error {
		return apperrors.NewValidation("+upload-attachment 暂不复制原子上传事务；请使用 dws todo task add-attachment --task-id <TASK_ID> --file <PATH>")
	},
}

func nonEmptyStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}

func init() {
	items := []*shortcut.Shortcut{&Create, &Update, &Complete, &Reopen, &Search, &Comment, &UploadAttachment, &Reminder}
	for _, item := range items {
		item.Contract.Selection.AgentSummary = item.Description
		item.Contract.Selection.UseWhen = []string{item.Intent}
	}
	shortcut.Register(Create, Update, Complete, Reopen, Search, Comment, UploadAttachment, Reminder)
}
