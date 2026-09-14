// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"fmt"
	"strings"
	"unicode"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func executeBaseCopy(rt *shortcut.RuntimeContext) error {
	baseID := strings.TrimSpace(rt.Str("base-id"))
	targetFolderID := strings.TrimSpace(rt.Str("target-folder-id"))
	newName := strings.TrimSpace(rt.Str("new-name"))
	if !validCompositeOpaqueID(baseID) {
		return apperrors.NewValidation("--base-id 不是合法的 Base ID")
	}
	if !validCompositeOpaqueID(targetFolderID) || pureNumericID(targetFolderID) {
		return apperrors.NewValidation("--target-folder-id 必须是文件夹 nodeId，不能是 URL、路径、纯数字 dentryId 或空值")
	}
	if rt.Changed("new-name") && (newName == "" || len([]rune(newName)) > 50) {
		return apperrors.NewValidation("--new-name 必须包含 1-50 个字符")
	}
	params := map[string]any{
		"baseId":         baseID,
		"targetFolderId": targetFolderID,
		"onlyCopyMeta":   rt.Bool("only-struct"),
	}
	result := newCompositeResult("base_copy")
	result.Resolved = map[string]any{"sourceBaseId": baseID, "targetFolderId": targetFolderID}
	if newName != "" {
		result.Resolved["newName"] = newName
	}
	result.Plan = []compositeStep{
		{Index: 1, Name: "verify target folder", Tool: "drive/get_file_info", Status: "planned", Arguments: map[string]any{"fileId": targetFolderID}},
		{Index: 2, Name: "copy base", Tool: "copy_base", Status: "planned", Arguments: params},
	}
	if newName != "" {
		result.Plan = append(result.Plan, compositeStep{Index: 3, Name: "rename copied base", Tool: "update_base", Status: "planned", Arguments: map[string]any{"baseId": "<newBaseId>", "newBaseName": newName}})
	}
	if rt.DryRun() {
		result.Status = "planned"
		result.Executed = false
		return rt.Output(result)
	}

	targetData, err := rt.CallMCPData("drive", "get_file_info", map[string]any{"fileId": targetFolderID})
	if err != nil {
		return err
	}
	actualTargetID := findStringByKeys(targetData, "fileId", "nodeId", "dentryUuid")
	if actualTargetID == "" || actualTargetID != targetFolderID {
		return apperrors.NewAPI("drive/get_file_info did not prove the exact target folder nodeId",
			apperrors.WithOperation("drive/get_file_info"), apperrors.WithReason("target_not_supported"),
			apperrors.WithFailureStage("target_resolution"), apperrors.WithExecutionStarted(false), apperrors.WithRetryable(false))
	}
	targetType := findStringByKeys(targetData, "type", "dentryType", "fileType")
	if targetType == "" {
		return apperrors.NewAPI("drive/get_file_info response is missing the target node type",
			apperrors.WithOperation("drive/get_file_info"), apperrors.WithReason("target_not_supported"),
			apperrors.WithFailureStage("response_validation"), apperrors.WithExecutionStarted(false), apperrors.WithRetryable(false))
	}
	if !strings.EqualFold(targetType, "folder") {
		return apperrors.NewAPI(fmt.Sprintf("target %s is %s, not a folder", targetFolderID, targetType),
			apperrors.WithOperation("drive/get_file_info"), apperrors.WithReason("target_wrong_type"),
			apperrors.WithFailureStage("target_resolution"), apperrors.WithExecutionStarted(false), apperrors.WithRetryable(false))
	}
	result.CompletedSteps = append(result.CompletedSteps, compositeStep{Index: 1, Name: "verify target folder", Tool: "drive/get_file_info", Status: "completed", Result: map[string]any{"fileId": actualTargetID, "type": targetType}})

	writeData, writeErr := rt.CallMCPWriteDataStrict(serverMain, "copy_base", params)
	newBaseID := copiedBaseID(writeData, baseID)
	if newBaseID == "" && copyBaseRejectedTarget(writeErr) {
		result.Status = "blocked"
		result.Retryable = false
		result.Verification = map[string]any{
			"status":     "terminal",
			"reason":     "target_not_supported",
			"nextAction": "report_unsupported",
		}
		return apperrors.NewAPI("copy_base rejected a target that Drive proved is the exact folder nodeId",
			apperrors.WithOperation("aitable/copy_base"), apperrors.WithOrigin("mcp"),
			apperrors.WithReason("target_not_supported"), apperrors.WithFailureStage("copy_base"),
			apperrors.WithExecutionStarted(true), apperrors.WithRetryable(false), apperrors.WithCause(writeErr),
			apperrors.WithHint("the current copy_base service does not accept this folder nodeId; stop and report unsupported instead of retrying, loading Help, creating test folders, or substituting another ID type"),
			apperrors.WithDetails(map[string]any{
				"result":                  result,
				"terminal":                true,
				"nextAction":              "report_unsupported",
				"prohibitedSubstitutions": []string{"folderId", "workspaceId", "spaceId", "numeric dentryId", "rootFolderId"},
			}))
	}
	if newBaseID == "" || !validCompositeOpaqueID(newBaseID) {
		cause := fmt.Errorf("copy_base response is missing a valid copied Base ID (newBaseId or data.baseId)")
		if writeErr != nil {
			cause = fmt.Errorf("copy_base response error: %w; newBaseId is unavailable", writeErr)
		}
		result.Status = "unknown"
		return compositeError(result, cause, false)
	}
	result.KnownEffects = append(result.KnownEffects, map[string]any{"tool": "copy_base", "newBaseId": newBaseID})
	result.CompletedSteps = append(result.CompletedSteps, compositeStep{Index: 2, Name: "copy base", Tool: "copy_base", Status: "completed", Result: map[string]any{"newBaseId": newBaseID}})

	var renameErr error
	if newName != "" {
		renameData, err := rt.CallMCPWriteDataStrict(serverMain, "update_base", map[string]any{"baseId": newBaseID, "newBaseName": newName})
		renameErr = err
		renameStep := compositeStep{Index: 3, Name: "rename copied base", Tool: "update_base", Status: "completed", Result: renameData}
		if renameErr != nil {
			renameStep.Status = "unknown"
			renameStep.Error = renameErr.Error()
		}
		result.CompletedSteps = append(result.CompletedSteps, renameStep)
	}

	readBack, verifyErr := rt.CallMCPData(serverMain, "get_base", map[string]any{"baseId": newBaseID})
	if verifyErr == nil {
		actualID := findStringByKeys(readBack, "baseId")
		if actualID == "" {
			verifyErr = fmt.Errorf("get_base read-back is missing baseId")
		} else if actualID != newBaseID {
			verifyErr = fmt.Errorf("get_base read-back identity mismatch: got %q, want %q", actualID, newBaseID)
		}
		if verifyErr == nil && newName != "" {
			actualName := findStringByKeys(readBack, "baseName", "name", "title")
			if actualName == "" {
				verifyErr = fmt.Errorf("get_base read-back is missing the copied Base name")
			} else if actualName != newName {
				verifyErr = fmt.Errorf("get_base read-back name mismatch: got %q, want %q", actualName, newName)
			}
		}
	}
	if verifyErr != nil {
		result.Status = "partial_success"
		result.Verification = map[string]any{"status": "failed", "newBaseId": newBaseID, "error": verifyErr.Error()}
		if renameErr != nil {
			result.Warnings = append(result.Warnings, "rename call also returned an error: "+renameErr.Error())
		}
		return compositeError(result, verifyErr, false)
	}

	if renameErr != nil {
		result.CompletedSteps[len(result.CompletedSteps)-1].Status = "recovered"
		result.Warnings = append(result.Warnings, "rename response was an error, but the requested name was proven by read-back")
	}
	result.CompletedCount = len(result.Plan)
	result.Verification = map[string]any{"status": "verified", "newBaseId": newBaseID}
	if newName != "" {
		result.Verification["baseName"] = newName
		result.KnownEffects = append(result.KnownEffects, map[string]any{"tool": "update_base", "newBaseId": newBaseID, "baseName": newName})
	}
	result.Result = map[string]any{"newBaseId": newBaseID, "base": readBack}
	return rt.Output(result)
}

func copiedBaseID(value map[string]any, sourceBaseID string) string {
	if explicit := findStringByKeys(value, "newBaseId"); explicit != "" && explicit != sourceBaseID && validCompositeOpaqueID(explicit) {
		return explicit
	}
	if data, ok := value["data"].(map[string]any); ok {
		if id := strings.TrimSpace(stringValue(data, "baseId")); id != "" && id != sourceBaseID && validCompositeOpaqueID(id) {
			return id
		}
	}
	if id := strings.TrimSpace(stringValue(value, "baseId")); id != "" && id != sourceBaseID && validCompositeOpaqueID(id) {
		return id
	}
	return ""
}

func copyBaseRejectedTarget(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "invalid target folder") ||
		strings.Contains(message, "invalid targetfolder")
}

func validCompositeOpaqueID(value string) bool {
	if value == "" || len(value) > 512 || strings.ContainsAny(value, "/?#") {
		return false
	}
	for _, r := range value {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func pureNumericID(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
