// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0
package aitable

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/aitableprotocol"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
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

// errRecordWriteUnknown identifies a validated receipt for the original write
// which is not confirmed yet; it never permits resending that write.
var errRecordWriteUnknown = errors.New("write reconciliation state is unknown")

// reconciledRecordIDs binds a receipt to all three original selectors and
// rejects duplicate, empty or malformed IDs. Unknown never means not written.
func reconciledRecordIDs(body map[string]any, base, table, token string) ([]string, error) {
	if body["baseId"] != base || body["tableId"] != table || body["clientToken"] != token {
		return nil, fmt.Errorf("write reconciliation identifies a different request")
	}
	switch body["state"] {
	case "applied":
	case "unknown":
		// Unknown can omit IDs or carry an empty collection, but must not
		// smuggle malformed or partially confirmed IDs into a pending result.
		if raw, present := body["recordIds"]; present {
			ids, ok := raw.([]any)
			if !ok || len(ids) != 0 {
				return nil, fmt.Errorf("unknown write reconciliation has invalid record IDs")
			}
		}
		return nil, errRecordWriteUnknown
	default:
		return nil, fmt.Errorf("write reconciliation has an unsupported state")
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
      "state":{"type":"string","const":"applied","description":"已确认请求至少部分应用；仍需独立核对数量和值"},
      "recordIds":{"type":"array","minItems":1,"maxItems":100,"uniqueItems":true,"description":"原 clientToken 对应创建分组已应用的 ID 集合，只覆盖输入中的创建行，不包含更新分组的原 recordId；不保证输入顺序，也不保证整批完整（整批仅指创建分组）。未返回 ID 的记录仍未核实（仅指创建行），不能按数量差额或输入位置补写；更新分组须按原 recordId 独立回读","items":{"type":"string","minLength":1}}
    },
    "required":["baseId","tableId","clientToken","state","recordIds"],
    "additionalProperties":true
  },
  {
    "type":"object",
    "description":"合法但尚未确认的原写入，outcome 为 pending；只能沿原身份只读对账，不能重放写入",
    "properties":{
      "baseId":{"type":"string","minLength":1,"description":"原请求 Base ID"},
      "tableId":{"type":"string","minLength":1,"description":"原请求 Table ID"},
      "clientToken":{"type":"string","minLength":1,"description":"原请求核对键"},
      "state":{"type":"string","const":"unknown","description":"原写入尚未确认，不代表未写入"},
      "recordIds":{"type":"array","maxItems":0,"description":"可省略或为空；未知状态不承诺已写入 ID","items":{"type":"string"}}
    },
    "required":["baseId","tableId","clientToken","state"],
    "additionalProperties":true
  },
  {
    "type":"object",
    "description":"未执行远端查询的 dry-run 请求预览，不代表已确认写入结果",
    "properties":{
      "executed":{"type":"boolean","const":false,"description":"远端查询未执行，恒为 false"},
      "tool":{"type":"string","const":"get_record_write_result","description":"计划调用的只读核对工具"},
      "arguments":{"type":"object","description":"计划用于只读核对的请求参数","additionalProperties":true}
    },
    "required":["executed","tool","arguments"],
    "additionalProperties":false
  }
]}`

func recordWriteResultContract() corecmd.ContractDecl {
	declaration := aitableCompositeContractWithResult("+record-write-result", "只读核对一次记录写入的实际结果",
		"批量创建或含创建的 upsert 失败、超时后，持有原 clientToken 需要核对创建分组 ID 时", "普通查询和 upsert 更新分组按原 recordId 使用 +record-query；本命令只核对创建分组，不会补写或重放，也不能证明更新分组或子记录父子链接完成",
		`dws aitable +record-write-result --base-id B --table-id T --client-token 123e4567-e89b-42d3-a456-426614174000`,
		&contract.ResultSpec{Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomePending, contract.ResultOutcomeFailure}, DataSchema: json.RawMessage(recordWriteResultSchema)})
	declaration.Parameters = []contract.ParamDecl{
		{Name: "base-id", Property: "baseId", Required: boolPtr(true)},
		{Name: "table-id", Property: "tableId", Required: boolPtr(true)},
		{Name: "client-token", Property: "clientToken", Required: boolPtr(true)},
		{Name: "write-intent", Property: "writeIntent"},
	}
	declaration.DryRun = &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest}
	return declaration
}

const recordWriteIntentRecoveryHelp = `每次含创建的 upsert 调用使用独立 UUID 文件名保存恢复文件，不覆盖此前文件；恢复文件仅证明准备过请求，不含 cell 内容，也不证明写入成功。文件名与 clientToken 是独立标识，应分开判断。原身份、端点、Base/Table 固定时，创建去重按 clientToken 核对，恢复文件名不参与创建去重。
保持原 token、只换文件名：创建分组继续沿用原 clientToken 的同一去重绑定和覆盖范围，文件名变化仅涉及本地恢复文件定位；去重是否实际命中须按原 token 只读核对，不能据此断言创建去重失效或成功。即使创建已去重，重复更新仍可能覆盖并发修改。
换成新 token（无论文件名保持还是更换）：可能失去原创建请求的去重绑定，增加重复创建风险；原写入可能尚未生效，所以风险不等于必然重复创建。不能把“换 token 或换文件”合并成同一结论，也不能推断重跑会覆盖原恢复文件。
unknown 或创建 ID 不完整时，本恢复流程必须继续停止写入，不能以新 token、新文件或补写确认将未核实范围当作未写入；另一次写入授权不构成原写入已核实，也不是本恢复流程的补建步骤。
只有显式传入 --write-intent 才校验该文件；确实没有原恢复文件时可省略，省略本身不会因缺文件返回 validation，但不保证远端查询成功。已有原恢复文件时，首次查询必须携带。纯更新不能由 +record-write-result 证明生效，因为该入口只对账创建分组，与是否缺少恢复文件无关。
+record-write-result 的 --write-intent 校验失败在查询写入结果之前返回 validation，不会发出 get_record_write_result；不能据此判断原写入不存在或写入失败。`

var RecordWriteResult = shortcut.Shortcut{
	OutputRollout: output.RolloutUnifiedActive,
	Service:       "aitable", Command: "+record-write-result", Product: serverMain,
	Description: "按原 clientToken 只读核对创建分组；未知结果不能作为重新创建依据",
	Intent:      "批量创建或含创建的 upsert 失败、超时后，按原 clientToken 只读核对创建分组，不补写或重放。返回的 recordIds 只覆盖输入中的创建行，不包含更新分组的原 recordId；创建 ID 集合的完整性也只相对于创建行判断，不能要求它包含更新记录。混合 upsert 的更新分组须按原 Base/Table/recordId 独立回读，不能等待创建组对账完成才核对更新。合法 unknown 返回 pending，不代表未写入；unknown 或创建 ID 不完整时停止后续写入，下一步仍使用本命令和原 baseId/tableId/clientToken 只读对账创建分组，不能用全表查询替代。返回部分 ID 不证明其余创建行未写入，不能按数量差额补写或更换 token。checkpoint 已提供 writeIntentFiles 或原恢复文件路径时，从首次查询就必须带 --write-intent <原文件>，不称其为可选、不先查询再补；手工核对文件内容不能替代该参数触发的身份、端点和目标校验。只有没有原恢复文件时才可省略该参数；身份或端点不匹配时不能删参数或修改文件绕过。" + recordWriteIntentRecoveryHelp + "\n\n" + aitableprotocol.RecordRecoveryReviewHelp,
	Risk:        shortcut.RiskRead,
	Safety:      contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
	Contract:    recordWriteResultContract(),
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "原写入 Base ID", Required: true},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "原写入 Table ID", Required: true},
		{Name: "client-token", Type: shortcut.FlagString, Desc: "原写入使用的 UUID v4", Required: true},
		{Name: "write-intent", Type: shortcut.FlagString, Desc: "原 upsert 写前保存的恢复文件，查询前校验原账号、端点与目标；checkpoint 已提供文件时首次查询必须带上，不得省略或查询后再补；没有原文件时才可省略"},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		token := strings.TrimSpace(rt.Str("client-token"))
		if err := aitableprotocol.ValidateClientToken(token); err != nil {
			return apperrors.NewValidation("--client-token: " + err.Error())
		}
		intentPath := rt.Str("write-intent")
		args := map[string]any{"baseId": rt.Str("base-id"), "tableId": rt.Str("table-id"), "clientToken": token}
		if rt.DryRun() {
			return rt.Output(map[string]any{
				"executed":  false,
				"tool":      "get_record_write_result",
				"arguments": args,
			})
		}
		// 预览不查询外部身份，也不证明恢复文件已通过执行校验。
		if intentPath != "" {
			if err := validateRecordWriteIntent(rt, intentPath, token); err != nil {
				// 身份查询的认证/PAT/网络分类必须留给原有执行器处理。
				if apperrors.DeclaresClassification(err) {
					return err
				}
				return apperrors.NewValidation("--write-intent: " + err.Error())
			}
		}
		data, err := rt.CallMCPReadData(serverMain, "get_record_write_result", args)
		if err != nil {
			return err
		}
		body := parityResponseObject(data)
		if _, err := reconciledRecordIDs(body, rt.Str("base-id"), rt.Str("table-id"), token); err != nil {
			if errors.Is(err, errRecordWriteUnknown) {
				// A valid unknown is a successful read of an unresolved write,
				// not a bad receipt and not permission to issue another mutation.
				return output.StoreResult(rt.Command().Context(), output.Pending(body, &output.OperationInfo{
					ID: token, State: "unknown",
					NextCommand: recordWriteRecoveryCommand(rt.Str("base-id"), rt.Str("table-id"), token, intentPath),
				}))
			}
			return invalidRecordWriteResult(err)
		}
		return rt.Output(body)
	},
}

func init() { shortcut.Register(RecordWriteResult) }
