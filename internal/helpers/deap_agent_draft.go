// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"fmt"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
)

const deapAgentLocalDefaultPrompt = "你是由本地 Agent 驱动的数字员工。请依据用户明确授权的任务与本地 Agent 配置提供帮助，遵守所在组织的安全和权限要求；信息不足时先澄清，不编造执行结果。"

var deapAgentPublishOperatorUserID = func(ctx context.Context) (string, error) {
	_, token, err := currentSupervisorProfile(ctx, deapConnectConfigDir())
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(token.UserID), nil
}

func deapAgentValidateDraftText(cmd *cobra.Command) error {
	for _, name := range []string{"name", "description", "dept-id", "prompt", "avatar-url", "supervisor-user-id", "type", "main-program-type", "response-mode"} {
		if !cmd.Flags().Changed(name) {
			continue
		}
		value, _ := cmd.Flags().GetString(name)
		if strings.TrimSpace(value) == "" {
			return apperrors.NewValidation(fmt.Sprintf("参数 --%s 不能是空字符串或纯空白；不修改时请省略该参数", name))
		}
	}
	return nil
}

func deapAgentCallPublish(cmd *cobra.Command, tool string, args map[string]any) error {
	// Retain parsing compatibility without sending the retired field upstream.
	delete(args, "allowJoinGroup")
	if commandDryRun(cmd) || deps.Caller.DryRun() {
		return deps.Out.PrintJSON(map[string]any{
			"dry_run": true, "executed": false, "tool": tool, "arguments": args,
			"steps": []string{"read_draft", "check_draft_editor_if_available", "publish"},
		})
	}

	agentUUID := stringArgument(args, "agentUuid")
	value, err := callDeapJSON(cmd.Context(), deapAgentDetailTool, map[string]any{
		"agentUuid": agentUUID, "snapshot": "draft",
	}, false)
	if err != nil {
		return fmt.Errorf("读取发布前草稿失败: %w", err)
	}
	success, _ := value["success"].(bool)
	draft, ok := value["data"].(map[string]any)
	if !ok {
		draft, ok = value["result"].(map[string]any)
	}
	if !success || !ok || stringArgument(draft, "agentUuid") != agentUUID {
		return apperrors.NewInternal("数字员工草稿响应无法确认目标和配置，已停止发布")
	}
	if rawEditor, present := draft["updateUserId"]; present && rawEditor != nil {
		editor, ok := rawEditor.(string)
		if !ok {
			return apperrors.NewInternal("数字员工草稿更新人格式无效，已停止发布")
		}
		if editor = strings.TrimSpace(editor); editor != "" {
			operator, err := deapAgentPublishOperatorUserID(cmd.Context())
			if err != nil {
				return fmt.Errorf("无法确认当前操作人身份，已停止发布: %w", err)
			}
			if operator = strings.TrimSpace(operator); operator == "" {
				return apperrors.NewValidation("当前操作人 Profile 缺少 userId，已停止发布")
			}
			if editor != operator {
				return apperrors.NewValidation(fmt.Sprintf("草稿最近更新人为 %s，当前操作人为 %s；请由更新人发布，或先由本人检查并保存草稿后重试", editor, operator))
			}
		}
	}
	return CallMCPToolOnServerContext(cmd.Context(), deapAgentServerID, tool, args)
}
