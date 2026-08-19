// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

var TableBootstrap = shortcut.Shortcut{
	OutputRollout: output.RolloutUnifiedActive,
	Service:       "aitable",
	Command:       "+table-bootstrap",
	Product:       serverMain,
	Description:   "在已有 Base 中一次创建数据表和字段，自动分片并读回验证",
	Intent:        "当你已有 baseId、需要新增一张带完整字段结构的数据表时使用；替代 table create 后连续 field create 和手工验证。",
	Risk:          shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "non_idempotent",
	},
	Contract: aitableCompositeContractWithResult(
		"+table-bootstrap",
		"在已有 Base 中一次创建数据表和字段，自动分片并读回验证",
		"当你已有 baseId、需要新增一张带完整字段结构的数据表时使用；替代 table create 后连续 field create 和手工验证。",
		"需要同时新建 Base 用 +base-bootstrap；复制现有表用 +table-copy；只补字段用 field create",
		`dws aitable +table-bootstrap --base-id BASE_ID --name "任务" --fields '[{"fieldName":"标题","type":"text"}]'`,
		aitableTableBootstrapResultSpec(),
	),
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "目标 Base ID", Required: true},
		{Name: "name", Type: shortcut.FlagString, Desc: "新数据表名称", Required: true},
		{Name: "fields", Type: shortcut.FlagString, Desc: "字段结构 JSON 数组；字段对象使用 fieldName/type/config", Required: true},
	},
	Tips: []string{
		`dws aitable +table-bootstrap --base-id BASE_ID --name "任务" --fields '[{"fieldName":"标题","type":"text"}]'`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return executeTableBootstrap(rt)
	},
}

type createdTableStructure struct {
	TableID  string
	Fields   []map[string]any
	Warnings []string
}

func parseBootstrapFields(raw string) ([]any, error) {
	value, err := parseJSONAny("fields", raw)
	if err != nil {
		return nil, tableBootstrapValidation(fmt.Sprintf("--fields 不是合法 JSON 数组：%v", err))
	}
	fields, ok := value.([]any)
	if !ok {
		return nil, tableBootstrapValidation("--fields 必须是 JSON 数组")
	}
	if len(fields) > 100 {
		return nil, tableBootstrapValidation("--fields 最多接受 100 个字段")
	}
	seen := map[string]bool{}
	for index, rawField := range fields {
		field, ok := rawField.(map[string]any)
		name := strings.TrimSpace(stringValue(field, "fieldName", "name"))
		if !ok || name == "" || strings.TrimSpace(stringValue(field, "type")) == "" {
			return nil, tableBootstrapValidation(fmt.Sprintf("--fields[%d] 必须包含 fieldName 和 type", index))
		}
		if seen[name] {
			return nil, tableBootstrapValidation(fmt.Sprintf("--fields[%d].fieldName %q 不能重复", index, name))
		}
		seen[name] = true
		if config, exists := field["config"]; exists {
			if _, ok := config.(map[string]any); !ok {
				return nil, tableBootstrapValidation(fmt.Sprintf("--fields[%d].config 必须是 JSON 对象", index))
			}
		}
	}
	return fields, nil
}

func tableBootstrapValidation(message string) error {
	return apperrors.NewValidation(message,
		apperrors.WithHint("字段对象使用 fieldName/type/config；已知参数时直接执行，不需要先调用 --help"),
		apperrors.WithActions(`dws aitable +table-bootstrap --base-id BASE_ID --name "任务" --fields '[{"fieldName":"标题","type":"text"}]'`),
		apperrors.WithAvailableFlags("base-id", "name", "fields"),
	)
}

func executeTableBootstrap(rt *shortcut.RuntimeContext) error {
	fields, err := parseBootstrapFields(rt.Str("fields"))
	if err != nil {
		return err
	}
	baseID, tableName := rt.Str("base-id"), rt.Str("name")
	result := newCompositeResult("table_bootstrap")
	result.RequestedCount = len(fields)
	result.Resolved = map[string]any{"baseId": baseID, "tableName": tableName}
	result.Plan = []compositeStep{{Index: 1, Name: "create and verify table", Tool: "create_table", Status: "planned", Count: len(fields)}}
	if rt.DryRun() {
		result.Status = "planned"
		result.Executed = false
		return rt.Output(result)
	}

	created, err := createAndVerifyTableStructure(rt, baseID, tableName, fields)
	result.Warnings = append(result.Warnings, created.Warnings...)
	if created.TableID != "" {
		result.Resolved["tableId"] = created.TableID
		result.KnownEffects = append(result.KnownEffects, map[string]any{"tool": "create_table", "baseId": baseID, "tableId": created.TableID, "name": tableName})
		result.NextCommand = aitableRecoveryCommand("dws", "aitable", "+table-get", "--base-id", baseID, "--table-id", created.TableID, "--format", "json")
	}
	if err != nil {
		if created.TableID == "" {
			result.Status = "unknown"
			result.Checkpoint = map[string]any{"step": "resolve table by exact name before retrying", "baseId": baseID, "tableName": tableName}
		} else {
			result.Status = "partial_success"
			result.Checkpoint = map[string]any{"step": "verify or repair table fields", "baseId": baseID, "tableId": created.TableID}
		}
		return compositeError(result, err, false)
	}

	result.CompletedCount = len(fields)
	result.CompletedSteps = []compositeStep{{Index: 1, Name: "create and verify table", Tool: "create_table", Status: "completed", Count: len(fields), Result: map[string]any{"tableId": created.TableID, "fieldCount": len(created.Fields)}}}
	result.Verification = map[string]any{"status": "verified", "baseId": baseID, "tableId": created.TableID, "fieldCount": len(created.Fields)}
	result.Result = map[string]any{"baseId": baseID, "tableId": created.TableID, "tableName": tableName, "fields": created.Fields}
	result.NextCommand = ""
	return rt.Output(result)
}

func createAndVerifyTableStructure(rt *shortcut.RuntimeContext, baseID, tableName string, fields []any) (createdTableStructure, error) {
	created := createdTableStructure{}
	initialEnd := minInt(15, len(fields))
	createData, err := rt.CallMCPWriteDataStrict(serverMain, "create_table", map[string]any{
		"baseId": baseID, "tableName": tableName, "fields": fields[:initialEnd],
	})
	created.TableID = findStringByKeys(createData, "tableId", "sheetId")
	if err != nil || created.TableID == "" {
		if err == nil {
			err = fmt.Errorf("create_table response is missing tableId")
		}
		return created, err
	}
	for offset := initialEnd; offset < len(fields); offset += 15 {
		end := minInt(offset+15, len(fields))
		if _, fieldErr := rt.CallMCPWriteDataStrict(serverMain, "create_fields", map[string]any{
			"baseId": baseID, "tableId": created.TableID, "fields": fields[offset:end],
		}); fieldErr != nil {
			created.Warnings = append(created.Warnings, fmt.Sprintf("create_fields offset %d returned an error; final read-back decides success: %v", offset, fieldErr))
		}
	}
	detail, err := rt.CallMCPData(serverMain, "get_tables", map[string]any{"baseId": baseID, "tableIds": []string{created.TableID}})
	if err != nil || !deepContainsString(detail, created.TableID) {
		if err == nil {
			err = fmt.Errorf("get_tables does not identify created tableId %s", created.TableID)
		}
		return created, err
	}
	fieldsData, err := rt.CallMCPData(serverMain, "get_fields", map[string]any{"baseId": baseID, "tableId": created.TableID})
	created.Fields, _ = findNamedObjectList(fieldsData, "fields", "fieldList")
	if err == nil && created.Fields == nil {
		err = fmt.Errorf("field read-back for table %s is missing the fields collection", created.TableID)
	}
	if err == nil {
		err = verifyDeclaredFieldStructures(created.Fields, fields)
	}
	if err != nil {
		return created, err
	}
	return created, nil
}
