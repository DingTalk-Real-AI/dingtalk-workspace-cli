// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

var TableCopy = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+table-copy",
	Product:     serverMain,
	Description: "跨 Base 同步复制一张表的可创建字段结构，并可同步复制全部记录",
	Intent:      "当服务端没有 table copy/task 接口、但你需要在目标 Base 重建一张表时使用；本地编排字段分片、fieldId 映射、记录分片和读回验证。",
	Risk:        shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "non_idempotent",
	},
	Contract: aitableCompositeContract(
		"+table-copy",
		"跨 Base 同步复制一张表的可创建字段结构，并可同步复制全部记录",
		"当服务端没有 table copy/task 接口、但你需要在目标 Base 重建一张表时使用；本地编排字段分片、fieldId 映射、记录分片和读回验证。",
		"复制整个 Base 用 base copy；需要复制公式、查找引用、关联字段或所有视图时先用 schema snapshot 手工处理这些无法安全重映射的依赖",
		`dws aitable +table-copy --source-base-id B1 --source-table-id T1 --target-base-id B2 --new-name "任务副本"`,
	),
	Flags: []shortcut.Flag{
		{Name: "source-base-id", Type: shortcut.FlagString, Desc: "源 Base ID", Required: true},
		{Name: "source-table-id", Type: shortcut.FlagString, Desc: "源 Table ID", Required: true},
		{Name: "target-base-id", Type: shortcut.FlagString, Desc: "目标 Base ID", Required: true},
		{Name: "new-name", Type: shortcut.FlagString, Desc: "目标表名", Required: true},
		{Name: "include-records", Type: shortcut.FlagBool, Desc: "复制全部记录；默认只复制可安全重建的字段结构"},
		{Name: "max-records", Type: shortcut.FlagInt, Default: "10000", Desc: "复制记录的写前上限，1-10000"},
	},
	Tips: []string{`dws aitable +table-copy --source-base-id B1 --source-table-id T1 --target-base-id B2 --new-name "任务副本"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return executeTableCopy(rt)
	},
}

var tableCopyUnsupportedTypes = map[string]string{
	"creator": "system field", "lastModifier": "system field", "createdTime": "system field", "lastModifiedTime": "system field",
	"formula":            "formula references field names and may be unsupported by the service",
	"filterUp":           "lookup configuration contains source field/table IDs",
	"lookup":             "lookup configuration contains source field IDs",
	"unidirectionalLink": "link configuration contains a source table ID",
	"bidirectionalLink":  "link configuration contains a source table ID and creates a reverse field",
}

func executeTableCopy(rt *shortcut.RuntimeContext) error {
	sourceBase, sourceTable := rt.Str("source-base-id"), rt.Str("source-table-id")
	targetBase := rt.Str("target-base-id")
	maxRecords := rt.Int("max-records")
	if maxRecords < 1 || maxRecords > maxCompositeRecordRun {
		return apperrors.NewValidation(fmt.Sprintf("--max-records 必须在 1..%d", maxCompositeRecordRun))
	}
	sourceFieldsData, err := rt.CallMCPData(serverMain, "get_fields", map[string]any{"baseId": sourceBase, "tableId": sourceTable})
	if err != nil {
		return err
	}
	sourceFields, found := findNamedObjectList(sourceFieldsData, "fields", "fieldList")
	if !found {
		return fmt.Errorf("source get_fields response is missing the fields collection")
	}
	createFields := make([]any, 0, len(sourceFields))
	copiedSourceFields := make([]map[string]any, 0, len(sourceFields))
	warnings := make([]string, 0)
	for _, field := range sourceFields {
		fieldID := stringValue(field, "fieldId", "id")
		name := stringValue(field, "fieldName", "name")
		typeName := stringValue(field, "type", "fieldType")
		if fieldID == "" || name == "" || typeName == "" {
			return fmt.Errorf("source field is missing fieldId, fieldName, or type")
		}
		if reason := tableCopyUnsupportedTypes[normalizedFieldType(typeName)]; reason != "" {
			warnings = append(warnings, fmt.Sprintf("skipped field %q (%s): %s", name, typeName, reason))
			continue
		}
		declaration := map[string]any{"fieldName": name, "type": typeName}
		for _, key := range []string{"config", "aiConfig"} {
			if value, exists := field[key]; exists {
				declaration[key] = value
			}
		}
		createFields = append(createFields, declaration)
		copiedSourceFields = append(copiedSourceFields, field)
	}
	var sourceRecords []map[string]any
	if rt.Bool("include-records") {
		sourceRecords, err = queryAllRecords(rt, map[string]any{"baseId": sourceBase, "tableId": sourceTable}, maxRecords)
		if err != nil {
			return err
		}
	}
	result := newCompositeResult("table_copy")
	result.RequestedCount = len(sourceRecords)
	result.Resolved = map[string]any{"sourceBaseId": sourceBase, "sourceTableId": sourceTable, "targetBaseId": targetBase}
	result.Warnings = warnings
	result.Plan = []compositeStep{{Index: 1, Name: "create target table", Tool: "create_table", Status: "planned", Count: len(createFields)}}
	if len(sourceRecords) > 0 {
		result.Plan = append(result.Plan, compositeStep{Index: 2, Name: "copy records", Tool: "create_records", Status: "planned", Count: len(sourceRecords)})
	}
	if rt.DryRun() {
		result.Status = "planned"
		result.Executed = false
		result.Result = map[string]any{"fieldCount": len(createFields), "recordCount": len(sourceRecords)}
		return rt.Output(result)
	}

	initialEnd := minInt(15, len(createFields))
	createData, err := rt.CallMCPWriteDataStrict(serverMain, "create_table", map[string]any{
		"baseId": targetBase, "tableName": rt.Str("new-name"), "fields": createFields[:initialEnd],
	})
	targetTable := findStringByKeys(createData, "tableId", "sheetId")
	if err != nil || targetTable == "" {
		if err == nil {
			err = fmt.Errorf("create_table response is missing target tableId")
		}
		result.Status = "unknown"
		result.Checkpoint = map[string]any{"nextStep": "locate the target table by exact name before retrying"}
		return compositeError(result, err, false)
	}
	result.Resolved["targetTableId"] = targetTable
	result.KnownEffects = append(result.KnownEffects, map[string]any{"tool": "create_table", "baseId": targetBase, "tableId": targetTable})
	for offset := initialEnd; offset < len(createFields); offset += 15 {
		end := minInt(offset+15, len(createFields))
		_, fieldErr := rt.CallMCPWriteDataStrict(serverMain, "create_fields", map[string]any{"baseId": targetBase, "tableId": targetTable, "fields": createFields[offset:end]})
		if fieldErr != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("create_fields offset %d returned an error; final schema verification decides success: %v", offset, fieldErr))
		}
	}
	targetFieldsData, err := rt.CallMCPData(serverMain, "get_fields", map[string]any{"baseId": targetBase, "tableId": targetTable})
	targetFields, found := findNamedObjectList(targetFieldsData, "fields", "fieldList")
	if err == nil && !found {
		err = fmt.Errorf("target field read-back is missing the fields collection")
	}
	if err == nil {
		err = verifyDeclaredFieldStructures(targetFields, createFields)
	}
	if err != nil {
		result.Status = "partial_success"
		result.Checkpoint = map[string]any{"targetTableId": targetTable, "step": "repair target fields"}
		return compositeError(result, err, false)
	}
	result.CompletedSteps = append(result.CompletedSteps, compositeStep{Index: 1, Name: "create target table", Tool: "create_table", Status: "completed", Count: len(createFields), Result: map[string]any{"tableId": targetTable}})

	fieldMap, err := mapCopiedFieldIDs(copiedSourceFields, targetFields)
	if err != nil {
		result.Status = "partial_success"
		result.Checkpoint = map[string]any{"targetTableId": targetTable, "step": "resolve field mapping"}
		return compositeError(result, err, false)
	}
	createdCount := 0
	for offset := 0; offset < len(sourceRecords); offset += recordBatchSize {
		end := minInt(offset+recordBatchSize, len(sourceRecords))
		batch := make([]map[string]any, 0, end-offset)
		wire := make([]any, 0, end-offset)
		for _, sourceRecord := range sourceRecords[offset:end] {
			sourceCells, ok := sourceRecord["cells"].(map[string]any)
			if !ok {
				return compositeError(result, fmt.Errorf("source record %s is missing cells", recordID(sourceRecord)), false)
			}
			targetCells := map[string]any{}
			for sourceFieldID, value := range sourceCells {
				if targetFieldID := fieldMap[sourceFieldID]; targetFieldID != "" {
					targetCells[targetFieldID] = value
				}
			}
			record := map[string]any{"cells": targetCells}
			batch = append(batch, record)
			wire = append(wire, record)
		}
		writeData, writeErr := rt.CallMCPWriteDataStrict(serverMain, "create_records", map[string]any{"baseId": targetBase, "tableId": targetTable, "records": wire})
		createdIDs := createdRecordIDs(writeData)
		if len(createdIDs) == 0 {
			if returned, found := findRecords(writeData); found {
				createdIDs = recordIDs(returned)
			}
		}
		if len(createdIDs) != len(batch) {
			if writeErr == nil {
				writeErr = fmt.Errorf("create_records returned %d record IDs for %d copied records", len(createdIDs), len(batch))
			}
			result.Status = "partial_success"
			result.CompletedCount = createdCount
			result.FailedCount = len(sourceRecords) - createdCount
			result.Checkpoint = map[string]any{"targetTableId": targetTable, "nextRecordOffset": offset}
			return compositeError(result, writeErr, false)
		}
		actual, verifyErr := queryRecordsByIDs(rt, targetBase, targetTable, createdIDs)
		if verifyErr == nil {
			verifyErr = matchCreatedCells(batch, actual)
		}
		if verifyErr != nil {
			result.Status = "partial_success"
			result.CompletedCount = createdCount
			result.FailedCount = len(sourceRecords) - createdCount
			result.Checkpoint = map[string]any{"targetTableId": targetTable, "nextRecordOffset": offset}
			return compositeError(result, verifyErr, false)
		}
		createdCount = end
		result.KnownEffects = append(result.KnownEffects, map[string]any{"tool": "create_records", "offset": offset, "recordIds": createdIDs})
	}
	result.CompletedCount = createdCount
	result.Verification = map[string]any{"status": "verified", "fieldCount": len(createFields), "recordCount": createdCount}
	result.Result = map[string]any{"targetBaseId": targetBase, "targetTableId": targetTable, "fieldCount": len(createFields), "recordCount": createdCount}
	return rt.Output(result)
}

func mapCopiedFieldIDs(source, target []map[string]any) (map[string]string, error) {
	targetByName := map[string]string{}
	for _, field := range target {
		name := stringValue(field, "fieldName", "name")
		id := stringValue(field, "fieldId", "id")
		if name != "" && id != "" {
			if targetByName[name] != "" {
				return nil, fmt.Errorf("target has duplicate field name %q", name)
			}
			targetByName[name] = id
		}
	}
	out := map[string]string{}
	for _, field := range source {
		name := stringValue(field, "fieldName", "name")
		sourceID := stringValue(field, "fieldId", "id")
		if targetID := targetByName[name]; targetID != "" {
			out[sourceID] = targetID
		} else {
			return nil, fmt.Errorf("target is missing copied field %q", name)
		}
	}
	return out, nil
}

func normalizedFieldType(value string) string {
	for key := range tableCopyUnsupportedTypes {
		if strings.EqualFold(key, value) {
			return key
		}
	}
	return value
}
