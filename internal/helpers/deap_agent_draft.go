// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"fmt"
	"log/slog"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
)

const deapAgentLocalDefaultPrompt = "你是由本地 Agent 驱动的数字员工。请依据用户明确授权的任务与本地 Agent 配置提供帮助，遵守所在组织的安全和权限要求；信息不足时先澄清，不编造执行结果。"

func deapAgentValidateDraftText(cmd *cobra.Command) error {
	for _, name := range []string{"name", "description", "dept-id", "prompt", "avatar-url", "supervisor-user-id", "type", "response-mode"} {
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
	if commandDryRun(cmd) || deps.Caller.DryRun() {
		return deps.Out.PrintJSON(map[string]any{
			"dry_run": true, "executed": false, "tool": tool, "arguments": args,
			"steps": []string{"read_draft", "default_prompt_if_local_agent_and_missing", "publish"},
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
	agentType := stringArgument(draft, "type")
	if agentType == "" {
		agentType = stringArgument(draft, "mainProgramType")
	}
	if agentType == "" {
		if profile, ok := draft["digitalTagEmployeeProfile"].(map[string]any); ok {
			agentType = stringArgument(profile, "mainProgramType")
			if agentType == "" {
				agentType = stringArgument(profile, "type")
			}
		}
	}
	if agentType == "" {
		return apperrors.NewInternal("数字员工草稿响应缺少主程序类型，已停止发布")
	}
	rawPrompt := draft["prompt"]
	if rawPrompt == nil {
		if config, ok := draft["promptConfig"].(map[string]any); ok {
			rawPrompt = config["prompt"]
		}
	}
	prompt, validPrompt := rawPrompt.(string)
	if rawPrompt != nil && !validPrompt {
		return apperrors.NewInternal("数字员工草稿人设响应格式无效，已停止发布")
	}
	defaulted := agentType == deapAgentMainProgramTypeLocalAgent && strings.TrimSpace(prompt) == ""
	if defaulted {
		saved, err := callDeapJSON(cmd.Context(), deapAgentSaveDraftTool, map[string]any{
			"agentUuid": agentUUID, "prompt": deapAgentLocalDefaultPrompt,
		}, false)
		if err != nil {
			return fmt.Errorf("补齐本地数字员工默认人设失败，尚未发布: %w", err)
		}
		if success, _ := saved["success"].(bool); !success {
			return apperrors.NewInternal("默认人设保存结果无法确认，已停止发布；请先查询草稿")
		}
		slog.DebugContext(cmd.Context(), "dingtalk_tag_local_prompt_defaulted", "agentUuid", agentUUID)
	}
	if err := CallMCPToolOnServerContext(cmd.Context(), deapAgentServerID, tool, args); err != nil {
		if defaulted {
			return fmt.Errorf("默认人设已保存，但发布失败；重试前可查询草稿确认: %w", err)
		}
		return err
	}
	return nil
}
