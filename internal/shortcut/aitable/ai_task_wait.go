// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

var FieldWaitAI = shortcut.Shortcut{
	Service: "aitable", Command: "+field-wait-ai", Product: serverMain, Description: "按本轮 taskId 和固定记录集合只读等待终态，结果可读尚未核实", Risk: shortcut.RiskRead,
	Intent: "仅在已确认状态 MCP 入口部署，且已有本轮真实 taskId、fieldId 和固定 recordIds 时只读查询。terminalVerified 与 resultReadableVerified 是独立证据；旧窗口提供方过滤成功/取消行，缺失行保持 unknown；successTerminalVerifiable=false 或省略时不推断历史能力。提供方确认历史能力后可以返回固定 taskId 终态及写回版本，但写回版本不证明 Record/WAL 可读，即使取得终态也保持结果待核实。每次 MCP 只读调用最多 50 条，累计原目标的已获状态；部分失败保留证据。总时限结束保留原任务，下次仍对同一任务续验，不重新触发 AI。需要跨进程接续时显式使用 checkpoint，恢复携带原始完整 tasks 和 resume。",
	Safety: contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"}, OutputRollout: output.RolloutUnifiedActive,
	Contract: aitableCompositeContractWithResult("+field-wait-ai", "只读核验本轮 AI 行任务终态；旧窗口缺失行保持 unknown；新提供方的终态/写回版本仍不证明结果可读，保持 pending", "已有本轮真实 taskId、fieldId 与固定目标 recordIds，状态 MCP 入口已经确认部署，需要只读续验同一任务终态时", "没有真实 taskId、整列目标记录不固定或未确认状态入口部署时保留提交回执；terminalVerified 不等于 resultReadableVerified；单元格非空和固定等待均不能证明本轮结果可读", `dws aitable +field-wait-ai --base-id B --table-id T --tasks '[{"taskId":"TASK","fieldId":"F","recordIds":["R"]}]' --timeout 2m`, &contract.ResultSpec{Outcomes: []contract.ResultOutcome{contract.ResultOutcomePending, contract.ResultOutcomeFailure}, DataSchema: json.RawMessage(`{"type":"object","description":"本轮固定任务的只读等待证据；旧窗口缺失行保持 unknown；新提供方的精确终态和写回版本仍不证明结果可读","properties":{"baseId":{"type":"string","description":"所属 Base"},"tableId":{"type":"string","description":"所属数据表"},"tasks":{"type":"array","description":"精确绑定本轮 taskId/fieldId/recordIds 的任务状态；超时前未取得回执时保留原目标","items":{"type":"object","properties":{"taskId":{"type":"string","description":"原受理回执中的真实非空任务 ID"},"fieldId":{"type":"string","description":"本轮任务对应的真实字段 ID"},"recordIds":{"type":"array","description":"本轮固定目标记录 ID 集合，不由单元格内容推断","items":{"type":"string"}}},"required":["taskId","fieldId","recordIds"],"additionalProperties":true}},"queryComplete":{"type":"boolean","description":"本次目标查询是否完整，不代表任务完成"},"queryRetryable":{"type":"boolean","description":"查询不完整时允许按原 taskId 只读续查，不能重新运行 AI"},"queryBudgetExhausted":{"type":"boolean","description":"服务端状态查询预算是否耗尽"},"terminalVerified":{"type":"boolean","description":"提供方是否确认本轮全部固定目标具有真实终态；不等于结果可读，不能据此返回 success"},"successTerminalVerifiable":{"type":"boolean","description":"提供方是否能够验证成功终态；旧窗口提供方为 false，旧服务可能省略，缺失不代表支持历史查询"},"resultReadableVerified":{"type":"boolean","description":"本轮结果版本和可读性是否核实；当前兼容接口恒为 false，不由终态、旧单元格或等待时长推断"},"resultVersionAvailable":{"type":"boolean","description":"提供方是否返回本轮行的写回版本证据；有版本不证明 Record/WAL 或结果可读"},"polls":{"type":"integer","description":"本次实际状态查询次数"},"elapsedMs":{"type":"integer","description":"本次请求与退避总耗时"},"metrics":{"type":"object","description":"持久累计的 CLI 到 MCP 调用和等待统计，均已包含在累计时间内，不代表内部 HSF 调用数","additionalProperties":true}},"required":["baseId","tableId","tasks","terminalVerified","resultReadableVerified","resultVersionAvailable","polls","elapsedMs"]}`)}),
	Flags:    []shortcut.Flag{{Name: "base-id", Type: shortcut.FlagString, Desc: "本轮任务所属 Base ID", Required: true}, {Name: "table-id", Type: shortcut.FlagString, Desc: "本轮任务所属 Table ID", Required: true}, {Name: "tasks", Type: shortcut.FlagString, Desc: "本轮已受理的 1～10 项 JSON 数组；真实非空 taskId/fieldId 必填且字段不同，每项 recordIds 为 1～500 个不同非空 ID", Required: true}, {Name: "checkpoint", Type: shortcut.FlagString, Desc: "可选私有持久检查点文件；绑定原始全部 tasks、账号组织、网关及目标，禁止覆盖"}, {Name: "resume", Type: shortcut.FlagBool, Desc: "从 checkpoint 恢复同一固定任务证据，只读续查未确认行"}, {Name: "timeout", Type: shortcut.FlagString, Default: "2m", Desc: "状态请求与退避共同的 0s～30m 总时限，0s 仅查询一批最多 50 条；保留完整目标及未查询行 unknown"}},
	Execute:  executeFieldWaitAI,
}

func parseAITaskTargets(raw string) ([]map[string]any, error) {
	var tasks []map[string]any
	if err := json.Unmarshal([]byte(raw), &tasks); err != nil || len(tasks) == 0 || len(tasks) > 10 {
		return nil, apperrors.NewValidation("tasks 必须为 1～10 项 JSON 数组")
	}
	fields := map[string]bool{}
	for _, task := range tasks {
		id, field := stringValue(task, "taskId"), stringValue(task, "fieldId")
		ids, ok := task["recordIds"].([]any)
		if id == "" || field == "" || fields[field] || !ok || len(ids) == 0 || len(ids) > 500 {
			return nil, apperrors.NewValidation("每任务 taskId/fieldId 非空且字段不同，recordIds 为 1～500 个不同 ID")
		}
		fields[field] = true
		seen := map[string]bool{}
		for _, v := range ids {
			id, ok := v.(string)
			if !ok || id == "" || seen[id] {
				return nil, apperrors.NewValidation("recordIds 必须为不同的非空字符串")
			}
			seen[id] = true
		}
	}
	return tasks, nil
}

func executeFieldWaitAI(rt *shortcut.RuntimeContext) (retErr error) {
	if rt.DryRun() {
		return apperrors.NewValidation("任务状态等待需要真实只读查询；dry-run 不能确认本轮终态")
	}
	tasks, err := parseAITaskTargets(rt.Str("tasks"))
	if err != nil {
		return err
	}
	budget, err := time.ParseDuration(rt.Str("timeout"))
	if err != nil || budget < 0 || budget > 30*time.Minute {
		return apperrors.NewValidation("timeout 必须为 0s～30m 的时长")
	}
	b, t := rt.Str("base-id"), rt.Str("table-id")
	checkpoint, save, closeCheckpoint, err := openAIWaitCheckpoint(rt, tasks)
	if err != nil {
		return apperrors.NewValidation(err.Error())
	}
	defer closeCheckpoint()
	progress := newAITaskPoll(b, t, tasks)
	if rt.Bool("resume") {
		progress, err = restoreAITaskPoll(b, t, tasks, checkpoint.Progress)
		if err != nil {
			return apperrors.NewValidation(err.Error())
		}
	}
	checkpoint.Metrics.Invocations++
	checkpoint.InvocationComplete = false
	started := time.Now()
	previousElapsed := checkpoint.ElapsedMS
	persist := func() error {
		checkpoint.Progress = progress.evidence()
		checkpoint.ElapsedMS = previousElapsed + time.Since(started).Milliseconds()
		return save()
	}
	defer func() {
		checkpoint.InvocationComplete = true
		if err := persist(); err != nil {
			retErr = errors.Join(retErr, err)
		}
	}()
	if err := persist(); err != nil {
		return err
	}
	ctx := rt.Command().Context()
	cancel := func() {}
	if budget > 0 {
		ctx, cancel = context.WithTimeout(ctx, budget)
	}
	defer cancel()
	previous := rt.Command().Context()
	rt.Command().SetContext(ctx)
	defer rt.Command().SetContext(previous)
	last := progress.body
	next := aitableRecoveryCommand("dws", "aitable", "+field-wait-ai", "--base-id", b, "--table-id", t, "--tasks", rt.Str("tasks"), "--timeout", rt.Str("timeout"), "--format", "json")
	finish := func(polls int, timedOut bool) error {
		if pending := progress.batches(tasks); len(pending) > 0 {
			var remaining []map[string]any
			byField := map[string]map[string]any{}
			// A field may appear in both round phases. Merge by identity,
			// rather than adjacency, to keep continuation tasks unique.
			for _, batch := range pending {
				task := batch[0]
				field := stringValue(task, "fieldId")
				if previous, ok := byField[field]; ok {
					previous["recordIds"] = append(previous["recordIds"].([]any), task["recordIds"].([]any)...)
				} else {
					byField[field] = task
					remaining = append(remaining, task)
				}
			}
			sort.SliceStable(remaining, func(i, j int) bool {
				return !progress.queriedField(stringValue(remaining[i], "fieldId")) && progress.queriedField(stringValue(remaining[j], "fieldId"))
			})
			raw, _ := json.Marshal(remaining)
			next = aitableRecoveryCommand("dws", "aitable", "+field-wait-ai", "--base-id", b, "--table-id", t, "--tasks", string(raw), "--timeout", rt.Str("timeout"), "--format", "json")
		}
		last["polls"] = polls
		last["elapsedMs"] = time.Since(started).Milliseconds()
		last["metrics"] = checkpoint.Metrics
		if rt.Str("checkpoint") != "" {
			next = aitableRecoveryCommand("dws", "aitable", "+field-wait-ai", "--base-id", b, "--table-id", t, "--tasks", rt.Str("tasks"), "--timeout", rt.Str("timeout"), "--checkpoint", rt.Str("checkpoint"), "--resume", "--format", "json")
		}
		// Task terminal evidence and result-readability evidence are independent.
		// A write-back version alone does not establish Record/WAL readability.
		state := "pending"
		if last["terminalVerified"] == true {
			state = "terminal_results_unverified"
		}
		return output.StoreResult(rt.Command().Context(), output.Pending(last, &output.OperationInfo{ID: stringValue(tasks[0], "taskId"), State: state, TimedOut: timedOut, NextCommand: next}))
	}

	polls, timedOut, err := pollAITasks(rt, b, t, tasks, progress, budget == 0, &checkpoint.Metrics, persist)
	if err != nil && !timedOut {
		return err
	}
	return finish(polls, timedOut)
}

// aiTaskReadFailure attaches prior evidence without changing an explicitly
// classified auth/API error or cancellation into a different failure category.
func aiTaskReadFailure(err error, result map[string]any) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var typed *apperrors.Error
	if errors.As(err, &typed) {
		copy := *typed
		copy.Details = make(map[string]any, len(typed.Details)+1)
		for key, value := range typed.Details {
			copy.Details[key] = value
		}
		copy.Details["result"] = result
		return &copy
	}
	var legacy *helpers.CLIError
	if errors.As(err, &legacy) {
		copy := *legacy
		copy.Details = make(map[string]any, len(legacy.Details)+1)
		for key, value := range legacy.Details {
			copy.Details[key] = value
		}
		copy.Details["result"] = result
		return &copy
	}
	return apperrors.NewAPI("AI 状态只读查询中断；保留已取得状态，按原 taskId 续查", apperrors.WithCause(err),
		apperrors.WithDetails(map[string]any{"result": result}), apperrors.WithRetryable(false))
}

// Aggregate flags alone cannot establish this run's terminal state. Require one
// uniquely bound row per target and consistent state/terminal evidence. A writeback
// version is provider evidence only, so readability still requires verification.
func validateAITaskStatusReceipt(body map[string]any, targets []map[string]any) error {
	// A new provider may disclose its success-terminal limitation. Preserve
	// the optional boolean without inventing support for older missing fields.
	if value, present := body["successTerminalVerifiable"]; present {
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("AI 成功终态能力标记必须为布尔值")
		}
	}
	for _, key := range []string{"queryComplete", "queryRetryable", "queryBudgetExhausted"} {
		if value, present := body[key]; present {
			if _, ok := value.(bool); !ok {
				return fmt.Errorf("AI 查询完整性标记必须为布尔值")
			}
		}
	}
	if body["queryComplete"] == true && (body["queryRetryable"] == true || body["queryBudgetExhausted"] == true) {
		return fmt.Errorf("AI 查询完整性与续查标记矛盾")
	}
	returned, ok := findNamedObjectList(body, "tasks")
	if !ok || len(returned) != len(targets) {
		return fmt.Errorf("AI 状态回执缺少完整任务集合")
	}
	if body["resultReadableVerified"] != false {
		return fmt.Errorf("AI 状态查询尚未核验记录可读性，不接受结果可读声明")
	}
	versionAvailable, ok := body["resultVersionAvailable"].(bool)
	if !ok {
		return fmt.Errorf("AI 写回版本能力标记必须为布尔值")
	}
	anyResultVersion := false
	allTerminal := true
	for i, target := range targets {
		task := returned[i]
		if task["taskId"] != target["taskId"] || task["fieldId"] != target["fieldId"] || !declaredValueMatches(task["recordIds"], target["recordIds"]) {
			return fmt.Errorf("AI 状态回执不是本轮固定目标")
		}
		ids := target["recordIds"].([]any)
		rows, ok := findNamedObjectList(task, "rows")
		if !ok || len(rows) != len(ids) {
			return fmt.Errorf("AI 状态回执缺少本轮完整行集合")
		}
		allowed, seen := map[string]bool{}, map[string]bool{}
		for _, id := range ids {
			allowed[id.(string)] = true
		}
		terminal := true
		for _, row := range rows {
			id := stringValue(row, "recordId")
			if !allowed[id] || seen[id] || row["taskId"] != target["taskId"] || row["fieldId"] != target["fieldId"] {
				return fmt.Errorf("AI 行状态身份缺失、重复或不属于本轮任务")
			}
			seen[id] = true
			finished := false
			switch row["state"] {
			case "succeeded", "failed", "timed_out", "cancelled":
				finished = true
			case "queued", "running", "writing", "unknown":
			default:
				return fmt.Errorf("AI 行状态不属于已核准协议")
			}
			// New history responses bind non-negative numeric versions to succeeded
			// rows. Older responses may omit this capability and version evidence.
			if value, present := row["resultVersion"]; present {
				version, numeric := value.(float64)
				if !numeric || version < 0 || math.IsInf(version, 0) || math.IsNaN(version) || math.Trunc(version) != version || row["state"] != "succeeded" {
					return fmt.Errorf("AI 行写回版本缺少有效成功行证据")
				}
				anyResultVersion = true
			}
			if row["state"] == "succeeded" && row["resultVersion"] == nil {
				return fmt.Errorf("AI 成功行缺少本轮写回版本证据")
			}

			if row["terminal"] != finished {
				return fmt.Errorf("AI 行状态与终态标识不一致")
			}
			terminal = terminal && finished
		}
		if task["terminalVerified"] != terminal {
			return fmt.Errorf("AI 字段终态标识与完整行状态不一致")
		}
		allTerminal = allTerminal && terminal
	}
	if versionAvailable != anyResultVersion {
		return fmt.Errorf("AI 写回版本标记与本轮行证据不一致")
	}
	if body["terminalVerified"] != allTerminal {
		return fmt.Errorf("AI 聚合终态标识与本轮完整行状态不一致")
	}
	return nil
}

func init() { shortcut.Register(FieldWaitAI) }
