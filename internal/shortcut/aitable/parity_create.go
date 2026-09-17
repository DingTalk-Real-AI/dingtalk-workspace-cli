// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0
package aitable

import (
	"encoding/json"
	"fmt"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"strings"
	"time"
)

func parityCompositeResultSpec() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(`{"type":"object","description":"执行进度、对象 ID、读回验证和失败恢复信息；失败结果位于 error.details.result","properties":{"contractVersion":{"type":"string","description":"业务回执版本"},"operation":{"type":"string","description":"本次操作名称"},"status":{"type":"string","description":"success/planned/partial_success/unknown/failed 等业务状态"},"executed":{"type":"boolean","description":"是否执行远端操作"},"retryable":{"type":"boolean","description":"是否可直接重试"},"requestedCount":{"type":"integer","description":"请求项目数"},"completedCount":{"type":"integer","description":"已完成并验证的项目数"},"failedCount":{"type":"integer","description":"未完成项目数"},"resolved":{"type":"object","description":"准确目标及新对象 ID"},"plan":{"type":"array","description":"有序执行步骤","items":{"type":"object"}},"completedSteps":{"type":"array","description":"已执行步骤与结果","items":{"type":"object"}},"verification":{"type":"object","description":"独立读回验证事实"},"checkpoint":{"type":"object","description":"中断后已知进度和安全恢复提示"},"nextCommand":{"type":"string","description":"下一步核实指令"},"knownSideEffects":{"type":"array","description":"已知远端副作用","items":{"type":"object"}},"warnings":{"type":"array","description":"明确能力边界","items":{"type":"string"}},"result":{"type":"object","description":"成功结果"}},"required":["operation","status","executed","retryable"]}`),
	}
}

var RecordBatchCreate = shortcut.Shortcut{
	Service: "aitable", Command: "+record-batch-create", Product: serverMain,
	Description: "批量新增记录，100 条分片并按返回 ID 独立验证；已有 recordId 明确拒绝",
	Intent:      "仅新增一批记录时使用；每条 records.cells 独立取值，返回已创建 ID 和分片恢复进度。",
	Risk:        shortcut.RiskWrite,
	Safety:      contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "non_idempotent"},
	Contract:    aitableCompositeContractWithResult("+record-batch-create", "批量新增记录并逐批读回验证", "仅新增记录且需要分片与准确 ID 读回时", "更新已有行用 +record-update；有增有改用 +record-upsert", `dws aitable +record-batch-create --base-id B --table-id T --records '[{"cells":{"fldTitle":"任务"}}]'`, parityCompositeResultSpec()),
	Flags:       []shortcut.Flag{{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true}, {Name: "table-id", Type: shortcut.FlagString, Desc: "Table ID", Required: true}, {Name: "records", Type: shortcut.FlagString, Desc: "非空 records JSON 数组，每项 cells；不允许 recordId，最多 10000 项", Required: true}},
	Execute: func(rt *shortcut.RuntimeContext) error {
		records, err := parseRecordObjects(rt.Str("records"), false)
		if err != nil {
			return err
		}
		for i, r := range records {
			if _, exists := r["recordId"]; exists {
				return apperrors.NewValidation(fmt.Sprintf("records[%d] 含 recordId；新增入口不接受更新目标", i))
			}
		}
		return executeRecordBatches(rt, "record_create", "create_records", serverMain, records, verifyUpsertBatch)
	},
}

var FieldCreate = shortcut.Shortcut{
	Service: "aitable", Command: "+field-create", Product: serverMain,
	Description: "批量新增字段，15 个分片并核对新字段 ID、类型与配置",
	Intent:      "已有数据表要新增明确结构的一批字段时使用；名称已存在或重复则停止，避免重试创建重复列。",
	Risk:        shortcut.RiskWrite,
	Safety:      contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "non_idempotent"},
	Contract:    aitableCompositeContractWithResult("+field-create", "批量新增字段并核对结构", "已有表新增一批不重名字段时", "已有字段修改用 +field-update；新建整张表用 +table-bootstrap", `dws aitable +field-create --base-id B --table-id T --fields '[{"fieldName":"标题","type":"text"}]'`, parityCompositeResultSpec()),
	Flags:       []shortcut.Flag{{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true}, {Name: "table-id", Type: shortcut.FlagString, Desc: "Table ID", Required: true}, {Name: "fields", Type: shortcut.FlagString, Desc: "非空字段结构数组，fieldName/type/config/description/aiConfig，最多 100 个", Required: true}, {Name: "resume-field-ids", Type: shortcut.FlagStringSlice, Desc: "提供 fields 开头已创建字段的 ID；核实名称与结构后按 fields 顺序返回，只创建剩余字段，全部提供时只核实"}},
	Execute:     executeParityFieldCreate,
}

func executeParityFieldCreate(rt *shortcut.RuntimeContext) error {
	fields, err := parseBootstrapFields(rt.Str("fields"))
	if err != nil {
		return err
	}
	if len(fields) == 0 || len(fields) > 100 {
		return apperrors.NewValidation("--fields 必须包含 1-100 个字段")
	}
	base, table := rt.Str("base-id"), rt.Str("table-id")
	params := map[string]any{"baseId": base, "tableId": table}
	result := newCompositeResult("field_create")
	result.RequestedCount = len(fields)
	result.Resolved = map[string]any{"baseId": base, "tableId": table}
	var resumeIDs []string
	if rt.Changed("resume-field-ids") {
		ids, e := parseRecordIDs(rt.StrSlice("resume-field-ids"))
		if e != nil || len(ids) > len(fields) {
			return apperrors.NewValidation("resume-field-ids 必须对应 fields 开头已创建的字段，数量不能超过 fields")
		}
		resumeIDs = ids
		result.Resolved["fieldIds"] = ids
		result.Plan = []compositeStep{{Index: 1, Name: "verify previously returned field IDs before continuing", Tool: "get_fields", Status: "planned"}}
	}
	for start := len(resumeIDs); start < len(fields); start += 15 {
		result.Plan = append(result.Plan, compositeStep{Index: len(result.Plan) + 1, Name: "create fields", Tool: "create_fields", Status: "planned", Offset: start, Count: minInt(15, len(fields)-start)})
	}
	if rt.DryRun() {
		result.Status = "planned"
		result.Executed = false
		return rt.Output(result)
	}
	if len(resumeIDs) > 0 {
		actual, e := readParityCreatedFields(rt, base, table, resumeIDs)
		if e == nil {
			e = verifyDeclaredFieldStructures(actual, fields[:len(resumeIDs)])
		}
		if e != nil {
			return compositeError(result, e, false)
		}
		resumeIDs = verifiedFieldIDsInDeclarationOrder(actual, fields[:len(resumeIDs)])
		result.Resolved["fieldIds"] = append([]string{}, resumeIDs...)
		result.CompletedCount = len(resumeIDs)
		if len(resumeIDs) == len(fields) {
			result.Verification = map[string]any{"status": "verified", "createdThisRun": false}
			return rt.Output(result)
		}
	}
	existing, err := rt.CallMCPData(serverMain, "get_fields", params)
	if err != nil {
		return err
	}
	before, found := findNamedObjectList(existing, "fields", "fieldList")
	if !found {
		return fmt.Errorf("get_fields preflight lacks fields")
	}
	names := map[string]bool{}
	oldIDs := map[string]bool{}
	for _, f := range before {
		id := stringValue(f, "fieldId", "id")
		name := stringValue(f, "fieldName", "name")
		if id == "" || name == "" {
			return fmt.Errorf("get_fields preflight lacks field identity")
		}
		names[name] = true
		oldIDs[id] = true
	}
	for _, raw := range fields[len(resumeIDs):] {
		f := raw.(map[string]any)
		if names[stringValue(f, "fieldName")] {
			return apperrors.NewValidation("待新增字段名称已存在；请更新原字段或更换名称")
		}
	}
	allIDs := append([]string{}, resumeIDs...)
	for offset := len(resumeIDs); offset < len(fields); offset += 15 {
		end := minInt(offset+15, len(fields))
		batch := fields[offset:end]
		receipt, writeErr := rt.CallMCPWriteDataStrict(serverMain, "create_fields", map[string]any{"baseId": base, "tableId": table, "fields": batch})
		result.CompletedSteps = append(result.CompletedSteps, compositeStep{Index: len(result.CompletedSteps) + 1, Name: "create fields", Tool: "create_fields", Status: "unknown", Offset: offset, Count: len(batch), Result: receipt})

		returnedIDs, receiptErr := parityCreatedFieldIDs(receipt, batch)
		if len(returnedIDs) > 0 {
			result.KnownEffects = append(result.KnownEffects, map[string]any{"tool": "create_fields", "fieldIds": returnedIDs})
		}
		if writeErr == nil {
			writeErr = receiptErr
		}
		if writeErr != nil {
			return compositeError(result, writeErr, false)
		}
		var actual []map[string]any
		var readErr error
		// Visibility can lag an acknowledged write. Retry reads only; never
		// replay create_fields after an unknown or delayed result.
		for attempt := 0; attempt < 6; attempt++ {
			if attempt > 0 {
				if e := recordReadbackWait(rt.Command().Context(), time.Duration(1<<(attempt-1))*time.Second); e != nil {
					return compositeError(result, e, false)
				}
			}
			actual, readErr = readParityCreatedFields(rt, base, table, returnedIDs)

			if readErr == nil {
				readErr = verifyDeclaredFieldStructures(actual, batch)
			}
			if readErr == nil {
				break
			}
		}

		ids := []string{}
		if readErr == nil {
			ids = verifiedFieldIDsInDeclarationOrder(actual, batch)
			for _, id := range ids {
				if oldIDs[id] {
					readErr = fmt.Errorf("created field identity was not new")
				}
			}
		}
		if readErr != nil {
			result.Status = "unknown"
			if offset > 0 {
				result.Status = "partial_success"
			}
			result.Checkpoint = map[string]any{"nextOffset": offset, "inspectBeforeRetry": true}
			return compositeError(result, readErr, false)
		}
		step := &result.CompletedSteps[len(result.CompletedSteps)-1]
		step.Status = "verified"
		step.Result = map[string]any{"fieldIds": ids}
		result.CompletedCount = end
		allIDs = append(allIDs, ids...)
		result.Resolved["fieldIds"] = append([]string{}, allIDs...)
		for _, id := range ids {
			oldIDs[id] = true
		}
	}
	result.Verification = map[string]any{"status": "verified", "newFields": len(fields) - len(resumeIDs)}
	return rt.Output(result)
}

func init() {
	shortcut.Register(withAITableParityAliases(RecordBatchCreate), withAITableParityAliases(FieldCreate))
}

func parityCreatedFieldIDs(raw map[string]any, expected []any) ([]string, error) {
	body := parityResponseObject(raw)
	rows, ok := body["results"].([]any)
	if !ok {
		return nil, fmt.Errorf("create_fields lacks per-field results")
	}
	names := map[string]bool{}
	for _, f := range expected {
		names[f.(map[string]any)["fieldName"].(string)] = true
	}
	ids := []string{}
	seen := map[string]bool{}
	var failure error
	for _, v := range rows {
		m, ok := v.(map[string]any)
		if !ok {
			failure = fmt.Errorf("invalid create_fields result")
			continue
		}
		name := stringValue(m, "fieldName")
		if !names[name] {
			failure = fmt.Errorf("create_fields returned unexpected or repeated field name")
			continue
		}
		delete(names, name)
		if m["success"] != true {
			failure = fmt.Errorf("create_fields rejected field %q: %s", name, stringValue(m, "errorMessage", "reason", "errorCode"))
			continue
		}
		id := stringValue(m, "fieldId")
		if id == "" || seen[id] {
			failure = fmt.Errorf("create_fields returned missing/duplicate field ID")
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(names) > 0 {
		failure = fmt.Errorf("create_fields omitted one or more field results")
	}
	return ids, failure
}
func readParityCreatedFields(rt *shortcut.RuntimeContext, base, table string, ids []string) ([]map[string]any, error) {
	all := []map[string]any{}
	for start := 0; start < len(ids); start += 10 {
		end := minInt(start+10, len(ids))
		raw, err := rt.CallMCPData(serverMain, "get_fields", map[string]any{"baseId": base, "tableId": table, "fieldIds": ids[start:end]})
		if err != nil {
			return nil, err
		}
		rows, ok := findNamedObjectList(raw, "fields")
		if !ok {
			return nil, fmt.Errorf("get_fields lacks fields")
		}
		wanted := map[string]bool{}
		for _, id := range ids[start:end] {
			wanted[id] = true
		}
		for _, f := range rows {
			id := stringValue(f, "fieldId")
			if !wanted[id] {
				return nil, fmt.Errorf("get_fields returned unexpected or repeated ID")
			}
			delete(wanted, id)
		}
		if len(wanted) > 0 {
			return nil, fmt.Errorf("created field IDs not yet visible")
		}
		all = append(all, rows...)
	}
	return all, nil
}

// Call only after exact-ID and verifyDeclaredFieldStructures checks succeed:
// each declaration then has exactly one matching field with a validated ID.
func verifiedFieldIDsInDeclarationOrder(actual []map[string]any, fields []any) []string {
	byName := make(map[string]string, len(actual))
	for _, f := range actual {
		byName[strings.TrimSpace(stringValue(f, "fieldName", "name"))] = stringValue(f, "fieldId")
	}
	ids := make([]string, 0, len(fields))
	for _, f := range fields {
		ids = append(ids, byName[strings.TrimSpace(stringValue(f.(map[string]any), "fieldName"))])
	}
	return ids
}
