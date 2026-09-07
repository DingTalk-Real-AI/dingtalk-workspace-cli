// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

const aiFieldRunMaxRecords = 100

var aiFieldRunOutputTypes = map[string]bool{
	"text": true, "select": true, "multiSelect": true, "number": true,
	"currency": true, "image": true, "video": true,
}

// AIFieldRun triggers one DingTalk-native AI field for an explicitly selected
// record set. It is intentionally not an alias for Lark field extensions: DWS
// neither installs nor configures extensions and exposes no task-status API.
var AIFieldRun = shortcut.Shortcut{
	OutputRollout: output.RolloutUnifiedActive,
	Service:       "aitable",
	Command:       "+ai-field-run",
	Product:       serverMain,
	Description:   "为明确选择的记录触发一个钉钉 AI 字段任务",
	Intent:        "当你已有一个配置完成的钉钉 AI 字段，并要为 1–100 条明确 recordId 触发计算时使用。命令只证明任务已受理，不等待或声明计算完成。",
	Risk:          shortcut.RiskHighWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "high",
		Confirmation: "user_required", Idempotency: "non_idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "aitable",
			Name:           "shortcut_ai_field_run",
			CanonicalPath:  "aitable.shortcut_ai_field_run",
			CLIPath:        "aitable +ai-field-run",
			PrimaryCLIPath: "aitable +ai-field-run",
		},
		Description: "为明确选择的记录触发一个钉钉 AI 字段任务",
		Interface: &contract.InterfaceSpec{
			Mode:         contract.InterfaceModeComposite,
			Availability: contract.InterfaceAvailable,
			Reason:       "DWS first verifies the exact AI field identity and complete selected-record set, then calls a single run_ai_field operation and strictly projects its submission receipt.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "为明确选择的记录触发一个钉钉 AI 字段任务",
			UseWhen:      []string{"当你已有一个配置完成的钉钉 AI 字段，并要为 1–100 条明确 recordId 触发计算时使用。命令只证明任务已受理，不等待或声明计算完成。"},
			AvoidWhen: []string{
				"不要用于安装、更新或清除 Lark field extension；这是钉钉原生 AI 字段执行入口。",
				"不要用于整列或多字段执行；当前公开合同仅支持一个 fieldId 和显式 recordIds。",
				"当前没有 AI 字段任务状态查询接口；pending 回执不代表计算完成。",
			},
			Examples: []string{"dws aitable +ai-field-run --base-id <BASE_ID> --table-id <TABLE_ID> --field-id <FIELD_ID> --record-ids <R1,R2>"},
			ExampleDispositions: []contract.ExampleDisposition{{
				Index:      aiFieldRunExampleIndex(),
				Mode:       contract.ExampleDispositionModeContractOnly,
				ReasonCode: contract.ExampleDispositionReasonStatefulPreflight,
				Reason:     "dry-run must read the exact live AI field and selected records before producing a plan; the isolated Agent example runner has no remote AITable fixture",
				Reviewed:   true,
			}},
		},
		Parameters: []contract.ParamDecl{
			{Name: "base-id", Property: "baseId"},
			{Name: "table-id", Property: "tableId"},
			{Name: "field-id", Property: "fieldIds"},
			{Name: "record-ids", Property: "recordIds"},
		},
		DryRun: &contract.DryRunSpec{
			PreviewKind: contract.DryRunPreviewPlan,
			RemoteReads: true,
		},
		Result: &contract.ResultSpec{
			Outcomes: []contract.ResultOutcome{contract.ResultOutcomePending, contract.ResultOutcomeFailure},
			DataSchema: json.RawMessage(`{
				"type":"object",
				"description":"AI 字段任务的严格受理回执；不表示计算完成",
				"properties":{
					"scope":{"type":"string","description":"执行范围，固定为 selected_records","const":"selected_records"},
					"fieldId":{"type":"string","description":"已受理任务绑定的稳定 AI 字段 ID"},
					"taskId":{"type":"string","description":"下层返回的稳定任务 ID"},
					"total":{"type":"integer","description":"下层确认接收的记录数量"},
					"requestedRecordCount":{"type":"integer","description":"去重后请求的记录数量"},
					"documentUrl":{"type":"string","description":"可选的 HTTPS 文档链接"},
					"verified":{"type":"boolean","description":"固定 false；当前没有任务状态或结果读回接口","const":false},
					"completionStatus":{"type":"string","description":"固定 unknown；任务最终状态不可观测","const":"unknown"}
				},
				"required":["scope","fieldId","taskId","total","requestedRecordCount","verified","completionStatus"],
				"additionalProperties":false
			}`),
			SensitivePaths: []string{"documentUrl"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "目标 Base ID", Required: true},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "目标 Table ID", Required: true},
		{Name: "field-id", Type: shortcut.FlagString, Desc: "一个已配置 AI 功能的字段 ID；仅支持一个非空 AI fieldId", Required: true},
		{Name: "record-ids", Type: shortcut.FlagStringSlice, Desc: "明确选择的记录 ID；recordIds 去除空白并去重后必须为 1–100 条", Required: true},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintCustom, Flags: []string{"field-id"}, Description: "仅支持一个非空 AI fieldId"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"record-ids"}, Description: "recordIds 去除空白并去重后必须为 1–100 条"},
	},
	Tips: []string{"dws aitable +ai-field-run --base-id BASE --table-id TABLE --field-id FIELD --record-ids R1,R2"},
	Validate: func(rt *shortcut.RuntimeContext) error {
		_, _, err := aiFieldRunInputs(rt)
		return err
	},
	Execute: executeAIFieldRun,
}

func aiFieldRunExampleIndex() *int {
	index := 0
	return &index
}

func aiFieldRunInputs(rt *shortcut.RuntimeContext) (string, []string, error) {
	if rt.Str("base-id") == "" {
		return "", nil, apperrors.NewValidation("--base-id 去除首尾空白后不能为空")
	}
	if rt.Str("table-id") == "" {
		return "", nil, apperrors.NewValidation("--table-id 去除首尾空白后不能为空")
	}
	fieldID := strings.TrimSpace(rt.Str("field-id"))
	if fieldID == "" {
		return "", nil, apperrors.NewValidation("--field-id 去除首尾空白后不能为空")
	}
	recordIDs, err := normalizeAIFieldRunRecordIDs(rt.StrSlice("record-ids"))
	if err != nil {
		return "", nil, err
	}
	return fieldID, recordIDs, nil
}

func normalizeAIFieldRunRecordIDs(values []string) ([]string, error) {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, apperrors.NewValidation("--record-ids 不能包含空记录 ID")
		}
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	if len(out) == 0 || len(out) > aiFieldRunMaxRecords {
		return nil, apperrors.NewValidation(fmt.Sprintf("--record-ids 去重后必须为 1–%d 条", aiFieldRunMaxRecords))
	}
	return out, nil
}

func executeAIFieldRun(rt *shortcut.RuntimeContext) error {
	fieldID, recordIDs, err := aiFieldRunInputs(rt)
	if err != nil {
		return err
	}
	baseID, tableID := rt.Str("base-id"), rt.Str("table-id")
	if err := preflightAIField(rt, baseID, tableID, fieldID); err != nil {
		return err
	}
	if err := preflightAIFieldRecords(rt, baseID, tableID, fieldID, recordIDs); err != nil {
		return err
	}
	params := map[string]any{
		"baseId":    baseID,
		"tableId":   tableID,
		"fieldIds":  []string{fieldID},
		"recordIds": recordIDs,
	}
	if rt.DryRun() {
		return rt.Output(map[string]any{
			"scope":                "selected_records",
			"preview_kind":         contract.DryRunPreviewPlan,
			"tool":                 "run_ai_field",
			"arguments":            params,
			"requestedRecordCount": len(recordIDs),
			"preflight": map[string]any{
				"fieldVerified":   true,
				"recordsVerified": true,
			},
			"executed": false,
			"verified": false,
		})
	}
	receipt, err := rt.CallMCPWriteDataStrict(serverMain, "run_ai_field", params)
	if err != nil {
		return err
	}
	projected, err := projectAIFieldRunReceipt(receipt, fieldID, len(recordIDs))
	if err != nil {
		return err
	}
	nextCommand := aitableRecoveryCommand("dws", "aitable", "+record-query",
		"--base-id", baseID, "--table-id", tableID,
		"--record-ids", strings.Join(recordIDs, ","), "--field-ids", fieldID, "--format", "json")
	return output.StoreResult(rt.Command().Context(), output.Pending(projected, &output.OperationInfo{
		ID:          projected["taskId"].(string),
		State:       "submitted",
		NextCommand: nextCommand,
	}))
}

func preflightAIField(rt *shortcut.RuntimeContext, baseID, tableID, fieldID string) error {
	data, err := rt.CallMCPReadData(serverMain, "get_fields", map[string]any{
		"baseId": baseID, "tableId": tableID, "fieldIds": []string{fieldID},
	})
	if err != nil {
		return err
	}
	items, err := uniqueAIFieldPreflightList(data, "fields")
	if err != nil || len(items) != 1 {
		return aiFieldPreflightError("field_identity_mismatch", "get_fields 必须返回唯一目标字段", map[string]any{"fieldId": fieldID})
	}
	field, ok := items[0].(map[string]any)
	if !ok {
		return aiFieldPreflightError("malformed_field", "get_fields 字段条目必须是对象", nil)
	}
	returnedID, _ := field["fieldId"].(string)
	if strings.TrimSpace(returnedID) != fieldID {
		return aiFieldPreflightError("field_identity_mismatch", "get_fields 返回了不同的 fieldId", map[string]any{"fieldId": fieldID})
	}
	aiConfig, ok := field["aiConfig"].(map[string]any)
	if !ok || validateAIFieldConfig(aiConfig) != nil {
		return aiFieldPreflightError("field_not_ai", "目标字段没有可执行的 aiConfig", map[string]any{"fieldId": fieldID})
	}
	return nil
}

func preflightAIFieldRecords(rt *shortcut.RuntimeContext, baseID, tableID, fieldID string, recordIDs []string) error {
	data, err := rt.CallMCPReadData(serverMain, "query_records", map[string]any{
		"baseId": baseID, "tableId": tableID, "recordIds": recordIDs, "fieldIds": []string{fieldID},
	})
	if err != nil {
		return err
	}
	items, err := uniqueAIFieldPreflightList(data, "records")
	if err != nil {
		return aiFieldPreflightError("malformed_records", err.Error(), nil)
	}
	wanted := make(map[string]bool, len(recordIDs))
	for _, id := range recordIDs {
		wanted[id] = true
	}
	seen := make(map[string]bool, len(items))
	for index, item := range items {
		record, ok := item.(map[string]any)
		if !ok {
			return aiFieldPreflightError("malformed_records", fmt.Sprintf("query_records records[%d] 必须是对象", index), nil)
		}
		id, ok := record["recordId"].(string)
		id = strings.TrimSpace(id)
		if !ok || id == "" || !wanted[id] || seen[id] {
			return aiFieldPreflightError("record_identity_mismatch", "query_records 返回陌生、重复或畸形 recordId", map[string]any{"recordId": id})
		}
		seen[id] = true
	}
	if len(seen) != len(wanted) {
		return aiFieldPreflightError("record_identity_mismatch", "query_records 未返回全部请求记录", map[string]any{"requested": len(wanted), "returned": len(seen)})
	}
	return nil
}

func uniqueAIFieldPreflightList(response map[string]any, key string) ([]any, error) {
	if response == nil {
		return nil, fmt.Errorf("response is nil")
	}
	type candidate struct {
		name  string
		items []any
	}
	candidates := make([]candidate, 0, 3)
	if value, exists := response[key]; exists {
		items, ok := value.([]any)
		if !ok {
			return nil, fmt.Errorf("top.%s must be an array", key)
		}
		candidates = append(candidates, candidate{name: "top", items: items})
	}
	for _, envelopeKey := range []string{"data", "result"} {
		value, exists := response[envelopeKey]
		if !exists {
			continue
		}
		envelope, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s envelope must be an object", envelopeKey)
		}
		value, exists = envelope[key]
		if !exists {
			continue
		}
		items, ok := value.([]any)
		if !ok {
			return nil, fmt.Errorf("%s.%s must be an array", envelopeKey, key)
		}
		candidates = append(candidates, candidate{name: envelopeKey, items: items})
	}
	if len(candidates) != 1 {
		names := make([]string, 0, len(candidates))
		for _, item := range candidates {
			names = append(names, item.name)
		}
		return nil, fmt.Errorf("response must contain exactly one %s array; found %v", key, names)
	}
	return candidates[0].items, nil
}

func validateAIFieldConfig(config map[string]any) error {
	outputType, ok := config["outputType"].(string)
	outputType = strings.TrimSpace(outputType)
	if !ok || !aiFieldRunOutputTypes[outputType] {
		return fmt.Errorf("aiConfig.outputType is unsupported")
	}
	prompt, ok := config["prompt"].([]any)
	if !ok || len(prompt) == 0 {
		return fmt.Errorf("aiConfig.prompt must be a non-empty array")
	}
	hasFieldRef := false
	for index, raw := range prompt {
		item, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("aiConfig.prompt[%d] must be an object", index)
		}
		typeName, _ := item["type"].(string)
		switch strings.TrimSpace(typeName) {
		case "text":
			value, ok := item["value"].(string)
			if !ok || strings.TrimSpace(value) == "" {
				return fmt.Errorf("aiConfig.prompt[%d].value must be non-empty", index)
			}
		case "fieldRef":
			fieldID, ok := item["fieldId"].(string)
			if !ok || strings.TrimSpace(fieldID) == "" {
				return fmt.Errorf("aiConfig.prompt[%d].fieldId must be non-empty", index)
			}
			hasFieldRef = true
		default:
			return fmt.Errorf("aiConfig.prompt[%d].type is unsupported", index)
		}
	}
	if !hasFieldRef {
		return fmt.Errorf("aiConfig.prompt must contain at least one fieldRef")
	}
	return nil
}

func projectAIFieldRunReceipt(receipt map[string]any, fieldID string, requestedCount int) (map[string]any, error) {
	data, ok := receipt["data"].(map[string]any)
	if !ok {
		return nil, aiFieldReceiptError("malformed_data", "run_ai_field receipt.data 必须是对象")
	}
	rawTasks, ok := data["tasks"].([]any)
	if !ok || len(rawTasks) != 1 {
		return nil, aiFieldReceiptError("malformed_tasks", "run_ai_field data.tasks 必须恰好包含一项")
	}
	task, ok := rawTasks[0].(map[string]any)
	if !ok {
		return nil, aiFieldReceiptError("malformed_tasks", "run_ai_field task 必须是对象")
	}
	returnedFieldID, _ := task["fieldId"].(string)
	if strings.TrimSpace(returnedFieldID) != fieldID {
		return nil, aiFieldReceiptError("field_identity_mismatch", "run_ai_field task.fieldId 与请求不一致")
	}
	status, _ := task["status"].(string)
	if status != "submitted" {
		return nil, aiFieldReceiptError("status_mismatch", "run_ai_field task.status 必须是 submitted")
	}
	taskID, _ := task["taskId"].(string)
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil, aiFieldReceiptError("missing_task_id", "run_ai_field task.taskId 不能为空")
	}
	total, ok := strictJSONInteger(task["total"])
	if !ok || total != requestedCount {
		return nil, aiFieldReceiptError("total_mismatch", "run_ai_field task.total 必须等于请求记录数")
	}
	out := map[string]any{
		"scope":                "selected_records",
		"fieldId":              fieldID,
		"taskId":               taskID,
		"total":                total,
		"requestedRecordCount": requestedCount,
		"verified":             false,
		"completionStatus":     "unknown",
	}
	if rawURL, exists := data["documentUrl"]; exists {
		documentURL, ok := rawURL.(string)
		documentURL = strings.TrimSpace(documentURL)
		parsed, parseErr := url.Parse(documentURL)
		if !ok || parseErr != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
			return nil, aiFieldReceiptError("invalid_document_url", "run_ai_field data.documentUrl 必须是无凭据 HTTPS URL")
		}
		out["documentUrl"] = documentURL
	}
	return out, nil
}

func strictJSONInteger(value any) (int, bool) {
	return strictJSONIntegerForBits(value, strconv.IntSize)
}

func strictJSONIntegerForBits(value any, intBits int) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		if intBits <= 0 || intBits > 64 {
			return 0, false
		}
		if intBits < 64 {
			limit := int64(1) << (intBits - 1)
			if typed < -limit || typed >= limit {
				return 0, false
			}
		}
		return int(typed), true
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) || math.Trunc(typed) != typed || typed < 0 || typed > float64(^uint(0)>>1) {
			return 0, false
		}
		return int(typed), true
	default:
		return 0, false
	}
}

func aiFieldPreflightError(reason, message string, details map[string]any) error {
	return apperrors.NewAPI(message,
		apperrors.WithOperation("aitable/ai-field-run/preflight"),
		apperrors.WithOrigin("mcp"),
		apperrors.WithFailureStage("preflight"),
		apperrors.WithExecutionStarted(false),
		apperrors.WithRetryable(false),
		apperrors.WithReason(reason),
		apperrors.WithDetails(details),
	)
}

func aiFieldReceiptError(reason, message string) error {
	return apperrors.NewAPI(message,
		apperrors.WithOperation("aitable/run_ai_field"),
		apperrors.WithOrigin("mcp"),
		apperrors.WithFailureStage("response_validation"),
		apperrors.WithExecutionStarted(true),
		apperrors.WithRetryable(false),
		apperrors.WithReason(reason),
	)
}
