// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0
package aitable

import (
	"encoding/json"
	"fmt"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

var FieldRunAI = shortcut.Shortcut{Service: "aitable", Command: "+field-run-ai", Product: serverMain, Description: "提交 AI 字段运行请求；缺少可靠受理证据时非零退出并保留回执", Intent: "已有 AI 字段需要运行整列或指定记录时；不把受理或重复运行冲突当成计算完成。", Risk: shortcut.RiskWrite, Safety: contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "unknown"}, Contract: aitableCompositeContractWithResult("+field-run-ai", "提交 AI 字段运行请求并保留逐字段原始回执", "触发已有 AI 字段运行时", "读取计算值用 +record-query；配置 AI 用 +field-update --ai-config；通用扩展安装不走本入口", `dws aitable +field-run-ai --base-id B --table-id T --field-ids F --record-ids R`, &contract.ResultSpec{Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure}, DataSchema: json.RawMessage(`{"type":"object","properties":{"status":{"type":"string","description":"submitted_unverified：请求已返回，不能据此认为所有任务已受理或计算完成"},"completed":{"type":"boolean","description":"始终 false，服务不等待计算完成"},"fieldIds":{"type":"array","description":"提交的字段 ID","items":{"type":"string"}},"receipt":{"type":"object","description":"下游原始回执，保留每个字段的冲突或错误"}},"required":["status","completed","fieldIds","receipt"]}`)}), Flags: []shortcut.Flag{{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true}, {Name: "table-id", Type: shortcut.FlagString, Desc: "Table ID", Required: true}, {Name: "field-ids", Type: shortcut.FlagStringSlice, Desc: "1-10 个 AI 字段 ID", Required: true}, {Name: "record-ids", Type: shortcut.FlagStringSlice, Desc: "1-500 个记录 ID；省略整列运行"}}, Execute: func(rt *shortcut.RuntimeContext) error {
	fields, err := parseRecordIDs(rt.StrSlice("field-ids"))
	if err != nil || len(fields) > 10 {
		return apperrors.NewValidation("field-ids 必须为 1-10 个不同 ID")
	}
	args := map[string]any{"baseId": rt.Str("base-id"), "tableId": rt.Str("table-id"), "fieldIds": fields}
	if rt.Changed("record-ids") {
		ids, e := parseRecordIDs(rt.StrSlice("record-ids"))
		if e != nil || len(ids) > 500 {
			return apperrors.NewValidation("record-ids 必须为 1-500 个不同 ID")
		}
		args["recordIds"] = ids
	}
	if rt.DryRun() {
		return rt.Output(map[string]any{"executed": false, "arguments": args})
	}
	raw, err := rt.CallMCPData(serverMain, "get_fields", map[string]any{"baseId": args["baseId"], "tableId": args["tableId"], "fieldIds": fields})
	if err != nil {
		return err
	}
	actual, ok := findNamedObjectList(raw, "fields")
	if !ok {
		return fmt.Errorf("get_fields lacks fields")
	}
	for _, id := range fields {
		matches := 0
		for _, f := range actual {
			if stringValue(f, "fieldId") == id {
				matches++

			}
		}
		if matches != 1 {
			return apperrors.NewValidation("AI 字段不存在或不唯一")
		}
	}
	receipt, err := rt.CallMCPWriteDataStrict(serverMain, "run_ai_field", args)
	if err != nil {
		return err
	}
	return apperrors.NewAPI("AI 运行请求已返回，但下游没有可核实的逐字段受理或任务状态；请核查回执与目标记录，不要直接重试",
		apperrors.WithReason("aitable_ai_submission_unverified"),
		apperrors.WithExecutionStarted(true), apperrors.WithRetryable(false),
		apperrors.WithDetails(map[string]any{"result": map[string]any{"status": "submitted_unverified", "completed": false, "fieldIds": fields, "receipt": receipt}}),
	)
}}

func init() { shortcut.Register(withAITableParityAliases(FieldRunAI)) }
