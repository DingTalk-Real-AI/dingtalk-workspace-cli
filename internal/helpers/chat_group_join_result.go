// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"errors"
	"fmt"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
)

// Group-join writes are never replayed. Keep the ServiceResult as data, and
// distinguish an accepted application from completed group membership.
func chatGroupJoinCommandResult(tool string, args map[string]any, data any, err error) (output.CommandResult, error) {
	if tool != "join_group_by_group_id" && tool != "join_group_by_invite_url" {
		if err != nil {
			return nil, err
		}
		return output.Success(data), nil
	}
	if err != nil {
		var typed *apperrors.Error
		if errors.As(err, &typed) && typed.Category == apperrors.CategoryAPI && typed.ServerKey == "im" &&
			(typed.Reason == "business_error" || typed.Reason == "mcp_tool_error") &&
			typed.ServerDiag.ServerErrorCode == "400001" &&
			(strings.TrimSpace(typed.Message) == "成员邀请已发送" || strings.TrimSpace(typed.ServerDiag.TechnicalDetail) == "成员邀请已发送") {
			// The runner's diagnostic contains no application ID or raw result.
			// Do not invent operation metadata merely to emit pending. Preserve
			// its classified error and provide the explicit non-retry guidance.
			copy := *typed
			copy.Reason = "group_join_submitted"
			copy.Hint = chatGroupJoinSubmittedHint(tool, args)
			copy.Retryable, copy.RetryableSet = false, true
			copy.RetryAfterSeconds, copy.NextRetryAt = nil, nil
			copy.Actions = nil
			if tool == "join_group_by_group_id" {
				copy.Actions = []string{chatGroupJoinGroupIDReadCommand(args)}
			}
			copy.Cause = err
			return nil, &copy
		}
		return nil, err
	}
	response, _ := data.(map[string]any)
	result, _ := response["result"].(map[string]any)
	if response["success"] == true && result["status"] == "APPLIED" {
		cid, _ := result["openConversationId"].(string)
		if result["isMember"] == true || result["alreadyMember"] == true {
			return nil, apperrors.NewAPI("服务端入群状态不一致，尚未确认申请结果",
				apperrors.WithReason("group_join_result_unknown"), apperrors.WithRetryable(false),
				apperrors.WithHint("先只读回查目标群的成员状态，不要重复提交入群申请。"))
		}
		// A server-provided target CID is the resource being observed, not a
		// fabricated application/task ID. Only emit a copyable command whose
		// identifier is safe in all supported shells; never embed invite codes.
		if !shellValueIsBare(cid) {
			return nil, apperrors.NewAPI("入群申请已提交，等待审批，尚未确认入群",
				apperrors.WithReason("group_join_submitted"), apperrors.WithRetryable(false),
				apperrors.WithHint(chatGroupJoinSubmittedHint(tool, args)))
		}
		next := "dws chat conversation-info --conversation-id " + cid + " --format json"
		if tool == "join_group_by_group_id" {
			next = chatGroupJoinGroupIDReadCommand(args)
		}
		return output.Pending(data, &output.OperationInfo{ID: cid, State: "APPLIED", NextCommand: next}), nil
	}
	return output.Success(data), nil
}

func chatGroupJoinGroupIDReadCommand(args map[string]any) string {
	return "dws chat group get-by-group-id --group-id " + fmt.Sprint(args["groupId"]) + " --format json"
}

func chatGroupJoinSubmittedHint(tool string, args map[string]any) string {
	hint := "入群申请/邀请已提交，等待群主或管理员审批处理；尚未确认入群，不要重复提交。"
	if tool == "join_group_by_group_id" {
		return hint + "先只读回查群状态：" + chatGroupJoinGroupIDReadCommand(args)
	}
	return hint + "先使用 chat group get-info-by-invite-url 按原邀请链接只读回查群状态；成员状态未明确时继续核实，不要猜测。"
}
