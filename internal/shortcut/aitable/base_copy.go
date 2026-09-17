// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"fmt"
	"strings"
	"unicode"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/aitabletarget"
)

func executeBaseCopy(rt *shortcut.RuntimeContext) error {
	baseID := strings.TrimSpace(rt.Str("base-id"))
	// Keep the wire input unchanged, but compare stable IDs before any rename.
	sourceBaseID := baseID
	if strings.Contains(baseID, "://") {
		target, err := aitabletarget.ParseURL(baseID)
		if err != nil {
			return err
		}
		sourceBaseID = target.BaseID
	}
	targetFolderID := strings.TrimSpace(rt.Str("target-folder-id"))
	newName := strings.TrimSpace(rt.Str("new-name"))
	if rt.Changed("new-name") && (newName == "" || len([]rune(newName)) > 50) {
		return apperrors.NewValidation("--new-name 必须包含 1-50 个字符")
	}
	params := map[string]any{
		"baseId":       baseID,
		"onlyCopyMeta": rt.Bool("only-struct"),
	}
	if targetFolderID != "" {
		params["targetFolderId"] = targetFolderID
	}
	result := newCompositeResult("base_copy")
	result.Resolved = map[string]any{"sourceBaseId": baseID}
	if targetFolderID == "" {
		result.Resolved["target"] = "source_workspace_root"
	} else {
		result.Resolved["targetFolderId"] = targetFolderID
	}
	if newName != "" {
		result.Resolved["newName"] = newName
	}
	result.Plan = []compositeStep{
		{Index: 1, Name: "copy base", Tool: "copy_base", Status: "planned", Arguments: params},
	}
	if newName != "" {
		result.Plan = append(result.Plan, compositeStep{Index: 2, Name: "rename copied base", Tool: "update_base", Status: "planned", Arguments: map[string]any{"baseId": "<newBaseId>", "newBaseName": newName}})
	}
	if rt.DryRun() {
		result.Status = "planned"
		result.Executed = false
		return rt.Output(result)
	}

	writeData, writeErr := rt.CallMCPWriteDataStrict(serverMain, "copy_base", params)
	newBaseID := copiedBaseID(writeData, sourceBaseID)
	if newBaseID == "" || !validCompositeOpaqueID(newBaseID) {
		cause := fmt.Errorf("copy_base response is missing a valid copied Base ID (newBaseId or data.baseId)")
		if writeErr != nil {
			cause = fmt.Errorf("copy_base response error: %w; newBaseId is unavailable", writeErr)
		}
		result.Status = "unknown"
		return compositeError(result, cause, false)
	}
	result.KnownEffects = append(result.KnownEffects, map[string]any{"tool": "copy_base", "newBaseId": newBaseID})
	result.CompletedSteps = append(result.CompletedSteps, compositeStep{Index: 1, Name: "copy base", Tool: "copy_base", Status: "completed", Result: map[string]any{"newBaseId": newBaseID}})

	var renameErr error
	if newName != "" {
		renameData, err := rt.CallMCPWriteDataStrict(serverMain, "update_base", map[string]any{"baseId": newBaseID, "newBaseName": newName})
		renameErr = err
		renameStep := compositeStep{Index: 2, Name: "rename copied base", Tool: "update_base", Status: "completed", Result: renameData}
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
