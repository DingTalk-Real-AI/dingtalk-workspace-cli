// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"reflect"
	"sort"
	"strconv"
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
	expectedCells := make(map[string]any, len(cells)+1)
	for fieldID, value := range cells {
		expectedCells[fieldID] = value
	}
	expectedCells[keyFieldID] = keyValue

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
	} else {
		// A create must persist the unique key. An update sends only the cells
		// the caller asked to modify; the key is selection state, not a patch.
		writeRecord["cells"] = expectedCells
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
		verifyErr = verifyRecordCells(verified, expectedCells)
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
		filters := map[string]any{"operator": "and", "operands": []any{map[string]any{"operator": "eq", "operands": []any{keyFieldID, keyValue}}}}
		if encoded, marshalErr := json.Marshal(filters); marshalErr == nil {
			result.NextCommand = aitableRecoveryCommand("dws", "aitable", "+record-query", "--base-id", baseID, "--table-id", tableID, "--filters", string(encoded), "--format", "json")
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
	value, _ := responseHasMoreKnown(data)
	return value
}

func responseHasMoreKnown(data map[string]any) (bool, bool) {
	if data == nil {
		return false, false
	}
	if value, ok := data["hasMore"].(bool); ok {
		return value, true
	}
	for _, key := range []string{"data", "result", "pagination", "page"} {
		if nested, ok := data[key].(map[string]any); ok {
			if value, known := responseHasMoreKnown(nested); known {
				return value, true
			}
		}
	}
	for _, key := range []string{"nextCursor", "cursor"} {
		if value, ok := data[key].(string); ok && strings.TrimSpace(value) != "" {
			return true, true
		}
	}
	return false, false
}

func recordID(record map[string]any) string {
	for _, key := range []string{"recordId", "record_id", "id"} {
		if value, ok := record[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func verifyRecordCells(record map[string]any, expected map[string]any) error {
	actual, ok := record["cells"].(map[string]any)
	if !ok {
		return fmt.Errorf("read-back record %q is missing cells", recordID(record))
	}
	for fieldID, want := range expected {
		got, exists := actual[fieldID]
		if !exists || !recordCellEquivalent(got, want) {
			return fmt.Errorf("read-back mismatch for field %s: got %#v, want %#v", fieldID, got, want)
		}
	}
	return nil
}

type recordSelectionOption struct{ aliases map[string]struct{} }

// recordCellEquivalent preserves exact comparison for arbitrary payloads but
// treats select values according to their business meaning. The write API
// accepts option names while read-back commonly expands them to {id,name}
// objects; multiple-select order is not semantically significant.
func recordCellEquivalent(actual, expected any) bool {
	if reflect.DeepEqual(actual, expected) {
		return true
	}
	if recordNumericEquivalent(actual, expected) {
		return true
	}
	actualSelection, actualMultiple, actualOK := normalizeRecordSelection(actual)
	expectedSelection, expectedMultiple, expectedOK := normalizeRecordSelection(expected)
	if !actualOK || !expectedOK || actualMultiple != expectedMultiple || len(actualSelection) != len(expectedSelection) {
		return false
	}
	matched := make([]bool, len(expectedSelection))
	for _, got := range actualSelection {
		found := false
		for index, want := range expectedSelection {
			if !matched[index] && recordSelectionAliasesOverlap(got, want) {
				matched[index] = true
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func recordNumericEquivalent(actual, expected any) bool {
	// Two strings remain text. Decimal-string equivalence is enabled only when
	// the other side is a JSON numeric value, as happens for number/currency
	// write versus read-back representations.
	if _, actualString := actual.(string); actualString {
		if _, expectedString := expected.(string); expectedString {
			return false
		}
	}
	actualNumber, actualOK := recordNumber(actual)
	expectedNumber, expectedOK := recordNumber(expected)
	return actualOK && expectedOK && actualNumber.Cmp(expectedNumber) == 0
}

func recordNumber(value any) (*big.Rat, bool) {
	var raw string
	switch number := value.(type) {
	case json.Number:
		raw = number.String()
	case float64:
		raw = strconv.FormatFloat(number, 'g', -1, 64)
	case float32:
		raw = strconv.FormatFloat(float64(number), 'g', -1, 32)
	case int:
		raw = strconv.Itoa(number)
	case int64:
		raw = strconv.FormatInt(number, 10)
	case int32:
		raw = strconv.FormatInt(int64(number), 10)
	case uint:
		raw = strconv.FormatUint(uint64(number), 10)
	case uint64:
		raw = strconv.FormatUint(number, 10)
	case uint32:
		raw = strconv.FormatUint(uint64(number), 10)
	case string:
		raw = strings.TrimSpace(number)
	default:
		return nil, false
	}
	parsed, ok := new(big.Rat).SetString(raw)
	return parsed, ok
}

func normalizeRecordSelection(value any) ([]recordSelectionOption, bool, bool) {
	if option, ok := recordSelectionValue(value); ok {
		return []recordSelectionOption{option}, false, true
	}

	var values []any
	switch typed := value.(type) {
	case []any:
		values = typed
	case []string:
		values = make([]any, 0, len(typed))
		for _, item := range typed {
			values = append(values, item)
		}
	default:
		return nil, false, false
	}
	options := make([]recordSelectionOption, 0, len(values))
	for _, item := range values {
		option, ok := recordSelectionValue(item)
		if !ok {
			return nil, false, false
		}
		options = append(options, option)
	}
	return options, true, true
}

func recordSelectionValue(value any) (recordSelectionOption, bool) {
	if token, ok := value.(string); ok {
		token = strings.TrimSpace(token)
		return recordSelectionOption{aliases: map[string]struct{}{token: {}}}, token != ""
	}
	object, ok := value.(map[string]any)
	if !ok {
		return recordSelectionOption{}, false
	}
	for key := range object {
		switch key {
		case "id", "optionId", "option_id", "name":
		default:
			return recordSelectionOption{}, false
		}
	}
	aliases := map[string]struct{}{}
	for _, key := range []string{"id", "optionId", "option_id", "name"} {
		if token, ok := object[key].(string); ok && strings.TrimSpace(token) != "" {
			aliases[strings.TrimSpace(token)] = struct{}{}
		}
	}
	return recordSelectionOption{aliases: aliases}, len(aliases) > 0
}

func recordSelectionAliasesOverlap(left, right recordSelectionOption) bool {
	for alias := range left.aliases {
		if _, ok := right.aliases[alias]; ok {
			return true
		}
	}
	return false
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
