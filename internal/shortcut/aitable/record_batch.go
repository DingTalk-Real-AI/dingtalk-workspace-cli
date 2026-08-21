// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"fmt"
	"sort"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

const (
	recordBatchSize       = 100
	maxCompositeRecordRun = 10000
)

func parseRecordObjects(raw string, requireRecordID bool) ([]map[string]any, error) {
	value, err := parseJSONAny("records", raw)
	if err != nil {
		return nil, err
	}
	list, ok := value.([]any)
	if !ok || len(list) == 0 {
		return nil, apperrors.NewValidation("--records 必须是非空 JSON 数组")
	}
	if len(list) > maxCompositeRecordRun {
		return nil, apperrors.NewValidation(fmt.Sprintf("--records 最多接受 %d 条，got %d", maxCompositeRecordRun, len(list)))
	}
	out := make([]map[string]any, 0, len(list))
	for index, item := range list {
		record, ok := item.(map[string]any)
		if !ok {
			return nil, apperrors.NewValidation(fmt.Sprintf("--records[%d] 必须是 JSON 对象", index))
		}
		cells, ok := record["cells"].(map[string]any)
		if !ok || len(cells) == 0 {
			return nil, apperrors.NewValidation(fmt.Sprintf("--records[%d].cells 必须是非空 JSON 对象", index))
		}
		if requireRecordID && recordID(record) == "" {
			return nil, apperrors.NewValidation(fmt.Sprintf("--records[%d] 缺少 recordId", index))
		}
		out = append(out, record)
	}
	return out, nil
}

func executeRecordUpdateBatches(rt *shortcut.RuntimeContext) error {
	records, err := parseRecordObjects(rt.Str("records"), true)
	if err != nil {
		return err
	}
	return executeRecordBatches(rt, "record_update", "update_records", serverMain, records, verifyUpdateBatch)
}

func executeRecordUpsertBatches(rt *shortcut.RuntimeContext) error {
	records, err := parseRecordObjects(rt.Str("records"), false)
	if err != nil {
		return err
	}
	return executeRecordBatches(rt, "record_upsert", "record_upsert", serverHelper, records, verifyUpsertBatch)
}

func executeRecordDeleteBatches(rt *shortcut.RuntimeContext) error {
	ids, err := parseRecordIDs(rt.StrSlice("record-ids"))
	if err != nil {
		return err
	}
	baseID, tableID := rt.Str("base-id"), rt.Str("table-id")
	result := newCompositeResult("record_delete")
	result.RequestedCount = len(ids)
	for offset := 0; offset < len(ids); offset += recordBatchSize {
		end := minInt(offset+recordBatchSize, len(ids))
		result.Plan = append(result.Plan, compositeStep{
			Index: len(result.Plan) + 1, Name: "delete record batch", Tool: "delete_records",
			Status: "planned", Offset: offset, Count: end - offset,
		})
	}
	if rt.DryRun() {
		result.Status = "planned"
		result.Executed = false
		return rt.Output(result)
	}

	existingIDs, err := queryExistingRecordIDs(rt, baseID, tableID, ids)
	if err != nil {
		return err
	}
	existingSet := make(map[string]bool, len(existingIDs))
	for _, id := range existingIDs {
		existingSet[id] = true
	}
	missingIDs := make([]string, 0, len(ids)-len(existingIDs))
	for _, id := range ids {
		if !existingSet[id] {
			missingIDs = append(missingIDs, id)
		}
	}
	result.Resolved = map[string]any{
		"existingRecordIds": existingIDs,
		"missingRecordIds":  missingIDs,
	}
	result.Plan = nil
	for offset := 0; offset < len(existingIDs); offset += recordBatchSize {
		end := minInt(offset+recordBatchSize, len(existingIDs))
		result.Plan = append(result.Plan, compositeStep{
			Index: len(result.Plan) + 1, Name: "delete existing record batch", Tool: "delete_records",
			Status: "planned", Offset: offset, Count: end - offset,
		})
	}
	if len(existingIDs) == 0 {
		result.Status = "unchanged"
		result.Executed = false
		result.Verification = map[string]any{"status": "verified_absent_before_write", "missingCount": len(missingIDs)}
		result.Result = map[string]any{"deletedCount": 0, "missingCount": len(missingIDs), "batchCount": 0}
		return rt.Output(result)
	}

	for offset := 0; offset < len(existingIDs); offset += recordBatchSize {
		end := minInt(offset+recordBatchSize, len(existingIDs))
		batch := existingIDs[offset:end]
		writeData, writeErr := rt.CallMCPWriteDataStrict(serverMain, "delete_records", map[string]any{
			"baseId": baseID, "tableId": tableID, "recordIds": batch,
		})
		step := compositeStep{
			Index: len(result.CompletedSteps) + 1, Name: "delete record batch", Tool: "delete_records",
			Status: "completed", Offset: offset, Count: len(batch), Result: writeData,
		}
		if writeErr != nil {
			step.Status = "unknown"
			step.Error = writeErr.Error()
		}
		remaining, verifyErr := queryDeletedRecordsByIDs(rt, baseID, tableID, batch)
		if verifyErr == nil && len(remaining) > 0 {
			verifyErr = fmt.Errorf("read-back still contains deleted record IDs: %s", strings.Join(recordIDs(remaining), ","))
		}
		if verifyErr != nil {
			if step.Error == "" {
				step.Error = verifyErr.Error()
			}
			step.Status = "unknown"
			result.CompletedSteps = append(result.CompletedSteps, step)
			result.CompletedCount = offset
			result.FailedCount = len(existingIDs) - offset
			result.Status = "unknown"
			if offset > 0 {
				result.Status = "partial_success"
			}
			result.Verification = map[string]any{"status": "failed", "error": verifyErr.Error(), "batchOffset": offset}
			result.Checkpoint = map[string]any{"nextOffset": offset, "batchSize": recordBatchSize}
			if writeErr != nil {
				result.Warnings = append(result.Warnings, "write response error: "+writeErr.Error())
			}
			return compositeError(result, verifyErr, true)
		}
		if writeErr != nil {
			step.Status = "recovered"
			result.Warnings = append(result.Warnings, fmt.Sprintf("batch at offset %d had a write-response error but deletion was proven by read-back", offset))
		}
		step.Result = map[string]any{"status": "verified_absent", "recordIds": batch}
		result.CompletedSteps = append(result.CompletedSteps, step)
		result.CompletedCount = end
		result.KnownEffects = append(result.KnownEffects, map[string]any{"tool": "delete_records", "offset": offset, "recordIds": batch})
	}
	result.Verification = map[string]any{
		"status": "verified_absent", "verifiedCount": len(existingIDs),
		"missingBeforeWriteCount": len(missingIDs),
	}
	result.Result = map[string]any{
		"deletedCount": len(existingIDs), "missingCount": len(missingIDs),
		"batchCount": len(result.CompletedSteps),
	}
	return rt.Output(result)
}

func queryExistingRecordIDs(rt *shortcut.RuntimeContext, baseID, tableID string, ids []string) ([]string, error) {
	records, err := queryDeletedRecordsByIDs(rt, baseID, tableID, ids)
	if err != nil {
		return nil, err
	}
	wanted := make(map[string]bool, len(ids))
	for _, id := range ids {
		wanted[id] = true
	}
	present := make(map[string]bool, len(records))
	for index, record := range records {
		id := recordID(record)
		if id == "" {
			return nil, fmt.Errorf("delete preflight record %d is missing recordId", index)
		}
		if !wanted[id] {
			return nil, fmt.Errorf("delete preflight returned unexpected recordId %q", id)
		}
		if present[id] {
			return nil, fmt.Errorf("delete preflight returned duplicate recordId %q", id)
		}
		present[id] = true
	}
	existing := make([]string, 0, len(present))
	for _, id := range ids {
		if present[id] {
			existing = append(existing, id)
		}
	}
	return existing, nil
}

func parseRecordIDs(values []string) ([]string, error) {
	seen := map[string]bool{}
	ids := make([]string, 0, len(values))
	for _, value := range values {
		id := strings.TrimSpace(value)
		if id == "" {
			continue
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, apperrors.NewValidation("--record-ids 必须至少包含一个非空 recordId")
	}
	if len(ids) > maxCompositeRecordRun {
		return nil, apperrors.NewValidation(fmt.Sprintf("--record-ids 最多接受 %d 个唯一 ID，got %d", maxCompositeRecordRun, len(ids)))
	}
	// Stable ordering makes checkpoints and retries deterministic even when the
	// flag parser or a caller supplied IDs through more than one occurrence.
	sort.Strings(ids)
	return ids, nil
}

type recordBatchVerifier func(*shortcut.RuntimeContext, string, string, []map[string]any, map[string]any) (map[string]any, error)

func executeRecordBatches(
	rt *shortcut.RuntimeContext,
	operation, tool, product string,
	records []map[string]any,
	verify recordBatchVerifier,
) error {
	baseID, tableID := rt.Str("base-id"), rt.Str("table-id")
	result := newCompositeResult(operation)
	result.RequestedCount = len(records)
	for offset := 0; offset < len(records); offset += recordBatchSize {
		end := minInt(offset+recordBatchSize, len(records))
		result.Plan = append(result.Plan, compositeStep{
			Index: len(result.Plan) + 1, Name: "write record batch", Tool: tool,
			Status: "planned", Offset: offset, Count: end - offset,
		})
	}
	if rt.DryRun() {
		result.Status = "planned"
		result.Executed = false
		return rt.Output(result)
	}

	for offset := 0; offset < len(records); offset += recordBatchSize {
		end := minInt(offset+recordBatchSize, len(records))
		batch := records[offset:end]
		wireRecords := make([]any, 0, len(batch))
		for _, record := range batch {
			wireRecords = append(wireRecords, record)
		}
		params := map[string]any{"baseId": baseID, "tableId": tableID, "records": wireRecords}
		writeData, writeErr := rt.CallMCPWriteDataStrict(product, tool, params)
		step := compositeStep{
			Index: len(result.CompletedSteps) + 1, Name: "write record batch", Tool: tool,
			Status: "completed", Offset: offset, Count: len(batch), Result: writeData,
		}
		if writeErr != nil {
			step.Status = "unknown"
			step.Error = writeErr.Error()
		}
		verification, verifyErr := verify(rt, baseID, tableID, batch, writeData)
		if verifyErr != nil {
			step.Status = "unknown"
			if step.Error == "" {
				step.Error = verifyErr.Error()
			}
			result.CompletedSteps = append(result.CompletedSteps, step)
			result.CompletedCount = offset
			result.FailedCount = len(records) - offset
			result.Status = "unknown"
			if offset > 0 {
				result.Status = "partial_success"
			}
			result.Verification = map[string]any{"status": "failed", "error": verifyErr.Error(), "batchOffset": offset}
			result.Checkpoint = map[string]any{"nextOffset": offset, "batchSize": recordBatchSize}
			if writeErr != nil {
				result.Warnings = append(result.Warnings, "write response error: "+writeErr.Error())
			}
			retryable := true
			if tool == "record_upsert" {
				for _, record := range batch {
					if recordID(record) == "" {
						retryable = false
						break
					}
				}
			}
			return compositeError(result, verifyErr, retryable)
		}
		if writeErr != nil {
			step.Status = "recovered"
			result.Warnings = append(result.Warnings, fmt.Sprintf("batch at offset %d had a write-response error but its final state was proven by read-back", offset))
		}
		step.Result = verification
		result.CompletedSteps = append(result.CompletedSteps, step)
		result.CompletedCount = end
		result.KnownEffects = append(result.KnownEffects, map[string]any{"tool": tool, "offset": offset, "count": len(batch)})
	}
	result.Verification = map[string]any{"status": "verified", "verifiedCount": len(records)}
	result.Result = map[string]any{"processedCount": len(records), "batchCount": len(result.CompletedSteps)}
	return rt.Output(result)
}

func verifyUpdateBatch(rt *shortcut.RuntimeContext, baseID, tableID string, batch []map[string]any, _ map[string]any) (map[string]any, error) {
	ids := recordIDs(batch)
	actual, err := queryRecordsByIDs(rt, baseID, tableID, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]map[string]any, len(actual))
	for _, record := range actual {
		byID[recordID(record)] = record
	}
	for _, expected := range batch {
		id := recordID(expected)
		got := byID[id]
		if got == nil {
			return nil, fmt.Errorf("read-back is missing updated record %s", id)
		}
		if err := verifyRecordCells(got, expected["cells"].(map[string]any)); err != nil {
			return nil, err
		}
	}
	return map[string]any{"status": "verified", "recordIds": ids}, nil
}

func verifyUpsertBatch(rt *shortcut.RuntimeContext, baseID, tableID string, batch []map[string]any, writeData map[string]any) (map[string]any, error) {
	updates := make([]map[string]any, 0)
	creates := make([]map[string]any, 0)
	for _, record := range batch {
		if recordID(record) == "" {
			creates = append(creates, record)
		} else {
			updates = append(updates, record)
		}
	}
	verifiedIDs := make([]string, 0, len(batch))
	if len(updates) > 0 {
		verified, err := verifyUpdateBatch(rt, baseID, tableID, updates, writeData)
		if err != nil {
			return nil, err
		}
		verifiedIDs = append(verifiedIDs, stringSlice(verified["recordIds"])...)
	}
	if len(creates) > 0 {
		createdIDs := createdRecordIDs(writeData)
		if len(createdIDs) != len(creates) {
			return nil, fmt.Errorf("record_upsert returned %d created record IDs for %d creates", len(createdIDs), len(creates))
		}
		actual, err := queryRecordsByIDs(rt, baseID, tableID, createdIDs)
		if err != nil {
			return nil, err
		}
		if err := matchCreatedCells(creates, actual); err != nil {
			return nil, err
		}
		verifiedIDs = append(verifiedIDs, createdIDs...)
	}
	return map[string]any{"status": "verified", "recordIds": verifiedIDs}, nil
}

func queryRecordsByIDs(rt *shortcut.RuntimeContext, baseID, tableID string, ids []string) ([]map[string]any, error) {
	window, err := queryRecordWindow(rt, map[string]any{
		"baseId": baseID, "tableId": tableID, "recordIds": ids,
	}, len(ids))
	if err != nil {
		return nil, err
	}
	// Exact-ID verification below compares every requested ID with the returned
	// records. The service can publish a continuation even after all requested
	// IDs are present, so hasMore is not evidence that this bounded read-back is
	// incomplete.
	return window.Records, nil
}

func queryDeletedRecordsByIDs(rt *shortcut.RuntimeContext, baseID, tableID string, ids []string) ([]map[string]any, error) {
	remaining := make([]map[string]any, 0)
	for offset := 0; offset < len(ids); offset += recordQueryServicePageSize {
		end := minInt(offset+recordQueryServicePageSize, len(ids))
		chunk := ids[offset:end]
		window, err := queryRecordWindow(rt, map[string]any{
			"baseId": baseID, "tableId": tableID, "recordIds": chunk,
		}, len(chunk))
		if err != nil {
			return nil, err
		}
		remaining = append(remaining, window.Records...)
	}
	return remaining, nil
}

func recordIDs(records []map[string]any) []string {
	out := make([]string, 0, len(records))
	seen := map[string]bool{}
	for _, record := range records {
		id := recordID(record)
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func createdRecordIDs(data map[string]any) []string {
	seen := map[string]bool{}
	out := make([]string, 0)
	var walk func(any, bool)
	walk = func(value any, createdContext bool) {
		switch typed := value.(type) {
		case map[string]any:
			if createdContext {
				if id := recordID(typed); id != "" && !seen[id] {
					seen[id] = true
					out = append(out, id)
				}
			}
			for key, child := range typed {
				lower := strings.ToLower(key)
				nextContext := createdContext || strings.Contains(lower, "created")
				walk(child, nextContext)
			}
		case []any:
			for _, child := range typed {
				if createdContext {
					if id, ok := child.(string); ok {
						id = strings.TrimSpace(id)
						if id != "" && !seen[id] {
							seen[id] = true
							out = append(out, id)
							continue
						}
					}
				}
				walk(child, createdContext)
			}
		}
	}
	walk(data, false)
	return out
}

func matchCreatedCells(expected, actual []map[string]any) error {
	if len(actual) != len(expected) {
		return fmt.Errorf("read-back returned %d created records, want %d", len(actual), len(expected))
	}
	used := make([]bool, len(actual))
	for _, wantRecord := range expected {
		wantCells := wantRecord["cells"].(map[string]any)
		matched := false
		for index, gotRecord := range actual {
			if used[index] || verifyRecordCells(gotRecord, wantCells) != nil {
				continue
			}
			used[index] = true
			matched = true
			break
		}
		if !matched {
			return fmt.Errorf("no created read-back record matches cells %#v", wantCells)
		}
	}
	return nil
}

func stringSlice(value any) []string {
	values, _ := value.([]string)
	return values
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
