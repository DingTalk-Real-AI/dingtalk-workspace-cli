// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0
package aitable

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/aitableprotocol"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/google/uuid"
)

// createRecordsReconciled sends one mutation with a stable per-batch token.
// An uncertain receipt is followed by one read, never a second mutation. The
// caller must independently verify IDs and cells before advancing its offset.
func createRecordsReconciled(rt *shortcut.RuntimeContext, base, table string, records []any) (map[string]any, string, error) {
	token := uuid.NewString()
	data, writeErr := rt.CallMCPWriteDataStrict(serverMain, "create_records", map[string]any{
		"baseId": base, "tableId": table, "records": records, "clientToken": token,
	})
	ids := createdRecordIDs(data)
	if len(ids) == 0 {
		if returned, found := findRecords(data); found {
			ids = recordIDs(returned)
		}
	}
	if writeErr == nil && validUniqueRecordIDs(ids, len(records)) {
		return data, token, nil
	}
	if isRecordWriteInputRejection(writeErr) {
		return data, token, writeErr
	}
	read, readErr := rt.CallMCPData(serverMain, "get_record_write_result", map[string]any{
		"baseId": base, "tableId": table, "clientToken": token,
	})
	if readErr == nil {
		body := parityResponseObject(read)
		ids, err := reconciledRecordIDs(body, base, table, token)
		if err == nil {
			// These IDs are a set, not a positional mapping to the input batch.
			wireIDs := make([]any, len(ids))
			for i, id := range ids {
				wireIDs[i] = id
			}
			data = map[string]any{"newRecordIds": wireIDs, "clientToken": token, "reconciled": true}
			if len(ids) == len(records) {
				return data, token, nil
			}
			return data, token, fmt.Errorf("write reconciliation found %d of %d records; inspect IDs before continuing", len(ids), len(records))
		}
		readErr = err
	}
	return data, token, fmt.Errorf("create_records outcome is unknown (clientToken %s); reconcile without repeating the write: %v; original receipt: %v", token, readErr, writeErr)
}

// reconciledRecordIDs binds a receipt to all three original selectors and
// rejects duplicate, empty or malformed IDs. Unknown never means not written.
func reconciledRecordIDs(body map[string]any, base, table, token string) ([]string, error) {
	if body["state"] != "applied" || body["baseId"] != base || body["tableId"] != table || body["clientToken"] != token {
		return nil, fmt.Errorf("write reconciliation is unknown or identifies a different request")
	}
	values, ok := body["recordIds"].([]any)
	if !ok || len(values) < 1 || len(values) > recordBatchSize {
		return nil, fmt.Errorf("write reconciliation lacks a bounded recordIds collection")
	}
	ids := make([]string, 0, len(values))
	for _, value := range values {
		id, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("write reconciliation has a non-string record ID")
		}
		ids = append(ids, id)
	}
	if !validUniqueRecordIDs(ids, len(ids)) {
		return nil, fmt.Errorf("write reconciliation has missing or duplicate IDs")
	}
	return ids, nil
}

func validUniqueRecordIDs(ids []string, expected int) bool {
	seen := map[string]bool{}
	for _, id := range ids {
		if strings.TrimSpace(id) == "" || id != strings.TrimSpace(id) || seen[id] {
			return false
		}
		seen[id] = true
	}
	return expected > 0 && len(ids) == expected
}

const recordWriteResultSchema = `{"oneOf":[
  {
    "type":"object",
    "properties":{
      "baseId":{"type":"string","minLength":1,"description":"原请求 Base ID"},
      "tableId":{"type":"string","minLength":1,"description":"原请求 Table ID"},
      "clientToken":{"type":"string","minLength":1,"description":"原请求核对键"},
      "state":{"type":"string","const":"applied","description":"成功结果已确认请求至少部分应用；unknown 会返回失败而非成功数据"},
      "recordIds":{"type":"array","minItems":1,"maxItems":100,"uniqueItems":true,"description":"实际已应用的 ID 集合，不保证输入顺序，也不保证整批完整；未返回 ID 的记录仍未核实，不能按数量差额或输入位置补写","items":{"type":"string","minLength":1}}
    },
    "required":["baseId","tableId","clientToken","state","recordIds"],
    "additionalProperties":true
  },
  {
    "type":"object",
    "description":"未执行远端查询的 dry-run 请求预览，不代表已确认写入结果",
    "properties":{
      "executed":{"type":"boolean","const":false,"description":"远端查询未执行，恒为 false"},
      "arguments":{"type":"object","description":"计划用于只读核对的请求参数","additionalProperties":true}
    },
    "required":["executed","arguments"],
    "additionalProperties":false
  }
]}`

var RecordWriteResult = shortcut.Shortcut{
	OutputRollout: output.RolloutUnifiedActive,
	Service:       "aitable", Command: "+record-write-result", Product: serverMain,
	Description: "按原 clientToken 只读核对已落库记录；未知结果不能作为重新创建依据",
	Intent:      "批量写入失败或超时且持有原 clientToken 时使用；只读核对实际已应用的记录 ID，不补写或重放，未知结果不代表未写入。unknown 或 ID 不完整时停止后续写入，下一步仍使用本命令和原 baseId/tableId/clientToken 只读对账，不能用全表查询替代；完整 ID 集合确认后才逐 ID 核对实际值。返回部分 ID 只证明这些记录已应用，不证明其余记录未写入，不能按数量差额补写或更换 token。用户评审恢复方案并要求下一步命令时，先给本命令使用原三个参数的完整只读命令并注明未执行，再解释判断；不要先展开 Schema 字段表。",
	Risk:        shortcut.RiskRead,
	Safety:      contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
	Contract: aitableCompositeContractWithResult("+record-write-result", "只读核对一次记录写入的实际结果",
		"批量写入失败或超时后，持有原 clientToken 需要确定已写入 ID 时", "普通查询用 +record-query；本命令不会补写或重放数据",
		`dws aitable +record-write-result --base-id B --table-id T --client-token 123e4567-e89b-42d3-a456-426614174000`,
		&contract.ResultSpec{Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure}, DataSchema: json.RawMessage(recordWriteResultSchema)}),
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "原写入 Base ID", Required: true},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "原写入 Table ID", Required: true},
		{Name: "client-token", Type: shortcut.FlagString, Desc: "原写入使用的 UUID v4", Required: true},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		if err := aitableprotocol.ValidateClientToken(rt.Str("client-token")); err != nil {
			return err
		}
		args := map[string]any{"baseId": rt.Str("base-id"), "tableId": rt.Str("table-id"), "clientToken": rt.Str("client-token")}
		if rt.DryRun() {
			return rt.Output(map[string]any{"executed": false, "arguments": args})
		}
		data, err := rt.CallMCPData(serverMain, "get_record_write_result", args)
		if err != nil {
			return err
		}
		body := parityResponseObject(data)
		if _, err := reconciledRecordIDs(body, rt.Str("base-id"), rt.Str("table-id"), rt.Str("client-token")); err != nil {
			return err
		}
		return rt.Output(body)
	},
}

func init() { shortcut.Register(RecordWriteResult) }
