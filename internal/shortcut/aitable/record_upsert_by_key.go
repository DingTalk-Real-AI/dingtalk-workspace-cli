// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// RecordUpsertByKey creates or updates exactly one record selected by a unique
// field value. It never writes when the key is ambiguous and always verifies
// the final record through a read-back query.
var RecordUpsertByKey = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+record-upsert-by-key",
	Product:     serverMain,
	Description: "按唯一字段值有则更新、无则创建记录，并读回验证",
	Intent:      "已知一个应当唯一的 fieldId/value、希望幂等同步一条记录时使用；先完整查询键值，0 条创建、1 条更新、2 条以上停止，写后再次查询并验证所有传入 cells。",
	Risk:        shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "user_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "aitable",
			Name:           "shortcut_record_upsert_by_key",
			CanonicalPath:  "aitable.shortcut_record_upsert_by_key",
			CLIPath:        "aitable +record-upsert-by-key",
			PrimaryCLIPath: "aitable +record-upsert-by-key",
		},
		Description: "按唯一字段值有则更新、无则创建记录，并读回验证。",
		Interface: &contract.InterfaceSpec{
			Mode:         contract.InterfaceModeComposite,
			Availability: contract.InterfaceAvailable,
			Reason:       "The command composes query_records with create_records/update_records and a final read-back; no single RPC owns its uniqueness and verification contract.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "按唯一字段值有则更新、无则创建记录，并读回验证",
			UseWhen:      []string{"已知一个应当唯一的 fieldId/value、希望幂等同步一条记录时使用；先完整查询键值，0 条创建、1 条更新、2 条以上停止，写后再次查询并验证所有传入 cells。"},
			AvoidWhen:    []string{"已经知道 recordId 时用 +record-update；一次处理多条不同键值时使用批量导入或分批调用"},
			Examples: []string{
				"dws aitable +record-upsert-by-key --base-id B --table-id T --key-field-id fldKey --key-value TASK-001 --cells '{\"fldStatus\":\"进行中\"}'",
			},
			ExampleDispositions: []contract.ExampleDisposition{{
				Index:      recordUpsertExampleIndex(),
				Mode:       contract.ExampleDispositionModeContractOnly,
				ReasonCode: contract.ExampleDispositionReasonStatefulPreflight,
				Reason:     "dry-run must query the live table to prove whether the unique key matches zero or one record; the isolated Agent example runner has no remote AITable fixture",
				Reviewed:   true,
			}},
		},
		DryRun: &contract.DryRunSpec{PreviewKind: "plan", RemoteReads: true},
	},
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "Table ID", Required: true},
		{Name: "key-field-id", Type: shortcut.FlagString, Desc: "具有唯一语义的字段 ID", Required: true},
		{Name: "key-value", Type: shortcut.FlagString, Desc: "字符串键值；与 --key-value-json 二选一"},
		{Name: "key-value-json", Type: shortcut.FlagString, Desc: "JSON 类型键值；与 --key-value 二选一"},
		{Name: "cells", Type: shortcut.FlagString, Desc: "要写入的 cells JSON 对象", Required: true},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintExactlyOne, Flags: []string{"key-value", "key-value-json"}, Description: "必须且只能提供一种键值表示"},
	},
	Tips: []string{
		`dws aitable +record-upsert-by-key --base-id B --table-id T --key-field-id fldKey --key-value TASK-001 --cells '{"fldStatus":"进行中"}'`,
	},
	Execute: executeRecordUpsertByKey,
}

func recordUpsertExampleIndex() *int {
	index := 0
	return &index
}

func executeRecordUpsertByKey(rt *shortcut.RuntimeContext) error {
	keyValue, err := recordKeyValue(rt)
	if err != nil {
		return err
	}
	cells, err := parseJSONObject("cells", rt.Str("cells"))
	if err != nil {
		return err
	}
	keyFieldID := rt.Str("key-field-id")
	if existing, present := cells[keyFieldID]; present && !reflect.DeepEqual(existing, keyValue) {
		return apperrors.NewValidation("--cells 中的键字段值与 --key-value/--key-value-json 冲突",
			apperrors.WithReason("key_value_conflict"),
			apperrors.WithExecutionStarted(false),
		)
	}
	cells[keyFieldID] = keyValue

	baseID, tableID := rt.Str("base-id"), rt.Str("table-id")
	preflight, err := queryUniqueRecordByKey(rt, baseID, tableID, keyFieldID, keyValue)
	if err != nil {
		return err
	}
	action, tool := "create", "create_records"
	writeRecord := map[string]any{"cells": cells}
	if preflight != nil {
		action, tool = "update", "update_records"
		writeRecord["recordId"] = recordID(preflight)
	}
	params := map[string]any{
		"baseId": baseID, "tableId": tableID,
		"records": []any{writeRecord},
	}
	result := newCompositeResult("record_upsert_by_key")
	result.Resolved = map[string]any{
		"baseId": baseID, "tableId": tableID, "keyFieldId": keyFieldID,
		"keyValue": keyValue, "matchedCount": boolCount(preflight != nil), "action": action,
	}
	result.RequestedCount = 1
	result.Plan = []compositeStep{{Index: 1, Name: action + " record", Tool: tool, Status: "planned", Count: 1, Arguments: params}}
	if rt.DryRun() {
		result.Status = "planned"
		result.Executed = false
		return rt.Output(result)
	}

	writeData, writeErr := rt.CallMCPWriteDataStrict(serverMain, tool, params)
	writeStep := compositeStep{Index: 1, Name: action + " record", Tool: tool, Status: "completed", Count: 1, Result: writeData}
	if writeErr != nil {
		writeStep.Status = "unknown"
		writeStep.Error = writeErr.Error()
	}
	result.CompletedSteps = append(result.CompletedSteps, writeStep)

	verified, verifyErr := queryUniqueRecordByKey(rt, baseID, tableID, keyFieldID, keyValue)
	if verifyErr == nil && verified != nil {
		verifyErr = newRecordFieldTypeResolver(rt, baseID, tableID).verify(verified, cells)
	}
	if verifyErr != nil || verified == nil {
		result.Status = "unknown"
		result.FailedCount = 1
		// An update targets a known record ID and is safe to retry. A create whose
		// effect could not be read back must be resolved by the unique-key query
		// first; an immediate blind retry could duplicate the row under eventual
		// consistency.
		retryable := action == "update"
		result.Retryable = retryable
		if verifyErr == nil {
			verifyErr = fmt.Errorf("read-back found no record for the unique key")
		}
		result.Verification = map[string]any{"status": "failed", "error": verifyErr.Error()}
		if writeErr != nil {
			result.Warnings = append(result.Warnings, "write call also returned an error: "+writeErr.Error())
		}
		result.Checkpoint = map[string]any{
			"nextStep":   "query the unique key again and verify its cells before retrying",
			"keyFieldId": keyFieldID,
			"keyValue":   keyValue,
		}
		return compositeError(result, verifyErr, retryable)
	}

	verifiedID := recordID(verified)
	result.CompletedCount = 1
	result.KnownEffects = append(result.KnownEffects, map[string]any{"action": action, "recordId": verifiedID, "keyFieldId": keyFieldID, "keyValue": keyValue})
	result.Verification = map[string]any{
		"status": "verified", "recordId": verifiedID,
		"checkedFields": sortedMapKeys(cells),
	}
	result.Result = map[string]any{"action": action, "recordId": verifiedID, "record": verified}
	if writeErr != nil {
		result.Warnings = append(result.Warnings, "write response was an error, but the requested final state was proven by read-back")
		result.CompletedSteps[0].Status = "recovered"
	}
	return rt.Output(result)
}

func recordKeyValue(rt *shortcut.RuntimeContext) (any, error) {
	if rt.Changed("key-value") {
		// The command framework rejects an explicitly empty string before Execute.
		return rt.Str("key-value"), nil
	}
	decoder := json.NewDecoder(strings.NewReader(rt.Str("key-value-json")))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, apperrors.NewValidation("--key-value-json 不是合法 JSON: " + err.Error())
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, apperrors.NewValidation("--key-value-json 必须只包含一个 JSON 标量")
	}
	if value == nil || reflect.ValueOf(value).Kind() == reflect.Map || reflect.ValueOf(value).Kind() == reflect.Slice {
		return nil, apperrors.NewValidation("--key-value-json 只接受 string/number/bool 标量")
	}
	return value, nil
}

func queryUniqueRecordByKey(rt *shortcut.RuntimeContext, baseID, tableID, fieldID string, value any) (map[string]any, error) {
	filters := map[string]any{
		"operator": "and",
		"operands": []any{map[string]any{
			"operator": "eq", "operands": []any{fieldID, value},
		}},
	}
	data, err := rt.CallMCPData(serverMain, "query_records", map[string]any{
		"baseId": baseID, "tableId": tableID, "filters": filters, "limit": 2,
	})
	if err != nil {
		return nil, err
	}
	records, found := findRecords(data)
	if !found {
		return nil, apperrors.NewAPI("query_records response is missing the records collection",
			apperrors.WithOperation("aitable/query_records"),
			apperrors.WithReason("target_invalid_response"),
			apperrors.WithFailureStage("response_validation"),
			apperrors.WithExecutionStarted(false),
		)
	}
	if responseHasMore(data) {
		return nil, apperrors.NewAPI("unique-key query is incomplete and cannot prove uniqueness",
			apperrors.WithOperation("aitable/query_records"),
			apperrors.WithReason("target_incomplete"),
			apperrors.WithFailureStage("target_resolution"),
			apperrors.WithExecutionStarted(false),
			apperrors.WithDetails(map[string]any{"records": records}),
		)
	}
	switch len(records) {
	case 0:
		return nil, nil
	case 1:
		if recordID(records[0]) == "" {
			return nil, apperrors.NewAPI("query_records returned a record without recordId",
				apperrors.WithOperation("aitable/query_records"),
				apperrors.WithReason("target_invalid_response"),
				apperrors.WithExecutionStarted(false),
			)
		}
		return records[0], nil
	default:
		return nil, apperrors.NewValidation("唯一键匹配到多条记录，已在写入前停止",
			apperrors.WithReason("target_ambiguous"),
			apperrors.WithExecutionStarted(false),
			apperrors.WithDetails(map[string]any{"records": records}),
		)
	}
}

func findRecords(data map[string]any) ([]map[string]any, bool) {
	if data == nil {
		return nil, false
	}
	if raw, exists := data["records"]; exists {
		list, ok := raw.([]any)
		if !ok {
			return nil, false
		}
		out := make([]map[string]any, 0, len(list))
		for _, item := range list {
			record, ok := item.(map[string]any)
			if !ok {
				return nil, false
			}
			out = append(out, record)
		}
		return out, true
	}
	for _, key := range []string{"data", "result"} {
		if nested, ok := data[key].(map[string]any); ok {
			if records, found := findRecords(nested); found {
				return records, true
			}
		}
	}
	return nil, false
}

func responseHasMore(data map[string]any) bool {
	if data == nil {
		return false
	}
	if value, ok := data["hasMore"].(bool); ok && value {
		return true
	}
	for _, key := range []string{"nextCursor", "cursor"} {
		if value, ok := data[key].(string); ok && strings.TrimSpace(value) != "" {
			return true
		}
	}
	for _, key := range []string{"data", "result", "pagination", "page"} {
		if nested, ok := data[key].(map[string]any); ok && responseHasMore(nested) {
			return true
		}
	}
	return false
}

func recordID(record map[string]any) string {
	for _, key := range []string{"recordId", "record_id", "id"} {
		if value, ok := record[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func verifyRecordCells(record map[string]any, expected map[string]any, fieldTypes map[string]string) error {
	actual, ok := record["cells"].(map[string]any)
	if !ok {
		return fmt.Errorf("read-back record %q is missing cells", recordID(record))
	}
	for fieldID, want := range expected {
		got, exists := actual[fieldID]
		if !exists || !recordCellValueEqual(got, want, fieldTypes[fieldID]) {
			return fmt.Errorf("read-back mismatch for field %s: got %#v, want %#v", fieldID, got, want)
		}
	}
	return nil
}

// recordCellValueEqual accepts typed projections only for fields proven to be
// selects. Comparison is intentionally asymmetric: a scalar write may read
// back as {id,name}, but an object write must never be verified by a scalar
// response that dropped the requested ID or other identity fields.
func recordCellValueEqual(got, want any, fieldType string) bool {
	if reflect.DeepEqual(got, want) {
		return true
	}
	if strings.EqualFold(fieldType, "singleSelect") {
		return selectionObjectMatchesScalar(got, want)
	}
	if !strings.EqualFold(fieldType, "multipleSelect") && !strings.EqualFold(fieldType, "multiSelect") {
		return false
	}
	gotList, gotOK := got.([]any)
	wantList, wantOK := want.([]any)
	if !gotOK || !wantOK || len(gotList) != len(wantList) || !selectionLikeList(gotList) || !selectionLikeList(wantList) {
		return false
	}
	used := make([]bool, len(gotList))
	for _, wantItem := range wantList {
		matched := false
		for index, gotItem := range gotList {
			if used[index] || (!reflect.DeepEqual(gotItem, wantItem) && !selectionObjectMatchesScalar(gotItem, wantItem)) {
				continue
			}
			used[index] = true
			matched = true
			break
		}
		if !matched {
			return false
		}
	}
	return true
}

func recordCellsMayNeedSelectionTypes(record map[string]any, expected map[string]any) bool {
	actual, ok := record["cells"].(map[string]any)
	if !ok {
		return false
	}
	for fieldID, want := range expected {
		got, exists := actual[fieldID]
		if exists && !reflect.DeepEqual(got, want) && selectionProjectionShapes(got, want) {
			return true
		}
	}
	return false
}

func selectionProjectionShapes(got, want any) bool {
	if _, gotObject := got.(map[string]any); gotObject {
		_, wantScalar := want.(string)
		return wantScalar
	}
	if _, wantObject := want.(map[string]any); wantObject {
		_, gotScalar := got.(string)
		return gotScalar
	}
	gotList, gotOK := got.([]any)
	wantList, wantOK := want.([]any)
	return gotOK && wantOK && selectionLikeList(gotList) && selectionLikeList(wantList)
}

type recordFieldTypeResolver struct {
	rt               *shortcut.RuntimeContext
	baseID, tableID  string
	loaded           bool
	fieldTypes       map[string]string
	fieldTypeLoadErr error
}

func newRecordFieldTypeResolver(rt *shortcut.RuntimeContext, baseID, tableID string) *recordFieldTypeResolver {
	return &recordFieldTypeResolver{rt: rt, baseID: baseID, tableID: tableID}
}

func resolvedRecordFieldTypeResolver(fields []map[string]any) *recordFieldTypeResolver {
	return &recordFieldTypeResolver{loaded: true, fieldTypes: recordFieldTypesFromFields(fields)}
}

func (r *recordFieldTypeResolver) verify(record map[string]any, expected map[string]any) error {
	exactErr := verifyRecordCells(record, expected, nil)
	if exactErr == nil || !recordCellsMayNeedSelectionTypes(record, expected) {
		return exactErr
	}
	fieldTypes, err := r.resolve()
	if err != nil {
		return fmt.Errorf("%v; cannot load field types for select verification: %w", exactErr, err)
	}
	return verifyRecordCells(record, expected, fieldTypes)
}

func (r *recordFieldTypeResolver) resolve() (map[string]string, error) {
	if !r.loaded {
		r.loaded = true
		r.fieldTypes, r.fieldTypeLoadErr = loadRecordFieldTypes(r.rt, r.baseID, r.tableID)
	}
	return r.fieldTypes, r.fieldTypeLoadErr
}

func loadRecordFieldTypes(rt *shortcut.RuntimeContext, baseID, tableID string) (map[string]string, error) {
	data, err := rt.CallMCPData(serverMain, "get_fields", map[string]any{"baseId": baseID, "tableId": tableID})
	if err != nil {
		return nil, err
	}
	fields, found := findNamedObjectList(data, "fields", "fieldList")
	if !found {
		return nil, fmt.Errorf("get_fields response is missing the fields collection")
	}
	fieldTypes := recordFieldTypesFromFields(fields)
	if len(fieldTypes) == 0 {
		return nil, fmt.Errorf("get_fields response has no fields with both fieldId and type")
	}
	return fieldTypes, nil
}

func recordFieldTypesFromFields(fields []map[string]any) map[string]string {
	fieldTypes := make(map[string]string, len(fields))
	for _, field := range fields {
		fieldID := strings.TrimSpace(stringValue(field, "fieldId", "id"))
		fieldType := strings.TrimSpace(stringValue(field, "type", "fieldType"))
		if fieldID != "" && fieldType != "" {
			fieldTypes[fieldID] = fieldType
		}
	}
	return fieldTypes
}

func selectionObjectMatchesScalar(objectValue, scalarValue any) bool {
	object, ok := objectValue.(map[string]any)
	if !ok {
		return false
	}
	scalar, ok := scalarValue.(string)
	if !ok {
		return false
	}
	for _, key := range []string{"id", "name", "optionId", "optionName"} {
		if value, exists := object[key].(string); exists && value == scalar {
			return true
		}
	}
	return false
}

func selectionLikeList(values []any) bool {
	for _, value := range values {
		switch typed := value.(type) {
		case string:
		case map[string]any:
			if _, hasID := typed["id"]; !hasID {
				if _, hasName := typed["name"]; !hasName {
					if _, hasOptionID := typed["optionId"]; !hasOptionID {
						if _, hasOptionName := typed["optionName"]; !hasOptionName {
							return false
						}
					}
				}
			}
		default:
			return false
		}
	}
	return true
}

func sortedMapKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}
