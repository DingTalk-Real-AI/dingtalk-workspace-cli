// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package wiki

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func validateWikiCopySourceID(rt *shortcut.RuntimeContext) error {
	value := strings.ToLower(strings.TrimSpace(rt.Str("node")))
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return apperrors.NewValidation("--node 不接受 http(s) URL；请先通过 wiki +node-get 取得真实 nodeId",
			apperrors.WithReason("stable_node_id_required"))
	}
	return nil
}

// verifyWikiNodeWrite reuses the existing readback; an omitted folder does not
// assert that an independently identified root folder was verified.
func verifyWikiNodeWrite(rt *shortcut.RuntimeContext, written map[string]any, operation, sourceID string) (string, map[string]any, error) {
	id := nestedWikiString(written, "nodeId", "fileId", "id")
	fail := func(cause error) (string, map[string]any, error) {
		return "", nil, wikiNodeWriteError(rt, id, sourceID, cause)
	}
	if _, err := requireWikiWrite(written, operation); err != nil {
		return fail(err)
	}
	if id == "" {
		return fail(wikiResponseError(operation, "missing_created_id", "写响应缺少新 nodeId；远端效果未知"))
	}
	if sourceID != "" && id == sourceID {
		return fail(wikiResponseError(operation, "copy_id_not_new", "副本 nodeId 与源节点相同"))
	}
	readback, err := rt.CallMCPData("doc", "get_document_info", map[string]any{"nodeId": id})
	if err != nil {
		return fail(err)
	}
	readback, err = requireWikiObject(readback, "doc/get_document_info")
	if err != nil {
		return fail(err)
	}
	if _, err = requireWikiResponse(readback, "doc/get_document_info"); err != nil {
		return fail(err)
	}
	if firstWikiString(readback, "nodeId", "id", "fileId") != id {
		return fail(wikiResponseError(operation, "readback_id_mismatch", "写后读回节点 ID 缺失或不一致"))
	}
	if firstWikiString(readback, "workspaceId", "spaceId") != rt.Str("workspace") {
		return fail(wikiResponseError(operation, "workspace_readback_mismatch", "写后读回知识库缺失或不一致"))
	}
	if folderID := firstWikiString(readback, "folderId", "parentId"); rt.Changed("folder") && (folderID == "" || folderID != rt.Str("folder")) {
		return fail(wikiResponseError(operation, "folder_readback_mismatch", "写后读回父目录缺失或不一致"))
	}
	return id, readback, nil
}

func wikiNodeWriteError(rt *shortcut.RuntimeContext, id, sourceID string, cause error) error {
	// PAT/control errors have a host-owned raw wire that must remain untouched.
	var raw apperrors.RawStderrError
	if errors.As(cause, &raw) {
		return cause
	}
	receipt := map[string]any{"expectedWorkspaceId": rt.Str("workspace"), "newResourceConfirmed": false}
	if id != "" {
		receipt["returnedNodeId"] = id
	}
	if sourceID != "" {
		receipt["sourceNodeId"] = sourceID
	}
	if rt.Changed("folder") {
		receipt["expectedFolderId"] = rt.Str("folder")
	}
	var typed *apperrors.Error
	if errors.As(cause, &typed) {
		enriched := *typed
		details := make(map[string]any, len(typed.Details)+1)
		for key, value := range typed.Details {
			details[key] = value
		}
		details["writeReceipt"] = receipt
		apperrors.WithDetails(details)(&enriched)
		apperrors.WithExecutionStarted(true)(&enriched)
		apperrors.WithRetryable(false)(&enriched)
		apperrors.WithFailureStage("write_verification")(&enriched)
		apperrors.WithCause(cause)(&enriched)
		enriched.Message = "节点写请求已提交，但核验未完成：" + typed.Message
		enriched.Hint = "先在同一 profile 下按 error.details.writeReceipt 核对；返回 ID 不证明本轮资源归属，禁止直接重试或自动删除。"
		return &enriched
	}
	// Keep legacy error types/exit codes discoverable instead of coercing them
	// into an API error. The added receipt contains only known IDs and targets.
	encoded, _ := json.Marshal(receipt)
	var legacy *helpers.CLIError
	if errors.As(cause, &legacy) {
		enriched := *legacy
		enriched.Message = fmt.Sprintf("节点写请求已提交，回执=%s；核验未完成：%s", encoded, legacy.Message)
		enriched.Suggestion = "先在同一 profile 下按回执核对；返回 ID 不证明本轮资源归属，禁止直接重试或自动删除。"
		enriched.Cause = cause
		return &enriched
	}
	return fmt.Errorf("节点写请求已提交，回执=%s；请先核对，禁止直接重试：%w", encoded, cause)
}
