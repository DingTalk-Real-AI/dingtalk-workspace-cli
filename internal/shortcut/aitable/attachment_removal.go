// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"fmt"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

type attachmentRemovalPlan struct {
	tool        string
	resourceIDs []string
	remaining   []map[string]any
	replacement []any
	removed     int
}

// planAttachmentRemoval prefers server-side removal by stable resource ID so
// the client need not reconstruct retained attachments. The server still has a
// best-effort read/write window, not CAS. Legacy rows without target IDs may use
// the existing replacement path only when every retained file has a token.
func planAttachmentRemoval(existing []map[string]any, name string, clearAll bool) (attachmentRemovalPlan, error) {
	plan := attachmentRemovalPlan{tool: "remove_attachments"}
	if clearAll {
		plan.removed = len(existing)
		return plan, nil
	}
	ids := make(map[string]bool)
	allIDs := true
	for _, item := range existing {
		if attachmentName(item) != name {
			plan.remaining = append(plan.remaining, item)
			continue
		}
		plan.removed++
		id := attachmentResourceID(item)
		if id == "" {
			allIDs = false
		} else if !ids[id] {
			ids[id] = true
			plan.resourceIDs = append(plan.resourceIDs, id)
		}
	}
	if plan.removed == 0 {
		return plan, nil
	}
	if allIDs {
		for _, item := range plan.remaining {
			if ids[attachmentResourceID(item)] {
				return plan, apperrors.NewValidation("目标附件 resourceId 同时出现在不同文件名下，无法按名称唯一确定删除范围",
					apperrors.WithReason("attachment_identity_ambiguous"), apperrors.WithExecutionStarted(false))
			}
		}
		return plan, nil
	}
	plan.tool = "update_records"
	plan.resourceIDs = nil
	plan.replacement = make([]any, 0, len(plan.remaining))
	for index, item := range plan.remaining {
		token := attachmentToken(item)
		if token == "" {
			return plan, apperrors.NewValidation(fmt.Sprintf("待删除附件缺少 resourceId，且剩余附件[%d]缺少 fileToken，当前命令无法可靠保留后删除；取得真实 resourceId 后可用 aitable attachment remove --resource-ids", index),
				apperrors.WithReason("attachment_tokens_unavailable"), apperrors.WithExecutionStarted(false))
		}
		plan.replacement = append(plan.replacement, map[string]any{"fileToken": token})
	}
	return plan, nil
}

func attachmentResourceID(item map[string]any) string {
	return strings.TrimSpace(stringValue(item, "resourceId"))
}

// verifyAttachmentRemoval checks target absence and retained stable identities,
// rather than treating an unchanged count with different files as success.
func verifyAttachmentRemoval(actual []map[string]any, plan attachmentRemovalPlan, name string) error {
	if len(actual) != len(plan.remaining) {
		return fmt.Errorf("attachment read-back count is %d, want %d", len(actual), len(plan.remaining))
	}
	counts := make(map[string]int)
	for _, item := range actual {
		if name != "" && attachmentName(item) == name {
			return fmt.Errorf("attachment %q is still present after removal", name)
		}
		if id := attachmentResourceID(item); id != "" {
			counts[id]++
		}
	}
	for _, id := range plan.resourceIDs {
		if counts[id] > 0 {
			return fmt.Errorf("removed attachment resourceId %s is still present", id)
		}
	}
	used := make([]bool, len(actual))
	for _, expected := range plan.remaining {
		id, token := attachmentResourceID(expected), attachmentToken(expected)
		matched := false
		for index, item := range actual {
			if used[index] {
				continue
			}
			// A legacy token is evidence only if it also survives read-back.
			// Filename/count alone cannot prove that the retained file survived.
			if (id != "" && attachmentResourceID(item) == id) ||
				(id == "" && token != "" && attachmentToken(item) == token) {
				used[index], matched = true, true
				break
			}
		}
		if !matched {
			return fmt.Errorf("retained attachment %q identity could not be verified", attachmentName(expected))
		}
	}
	return nil
}
