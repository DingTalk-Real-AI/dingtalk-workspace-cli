// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
)

const (
	deapAgentAvatarUploadPath   = "/v1.0/assistant/digital-employees/avatar/upload"
	deapAgentAvatarMaxFileSize  = 10 * 1024 * 1024
	deapAgentAvatarUploadAction = "upload_avatar_then_save_draft"
)

var deapAgentAvatarExtensions = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true,
}

type deapAgentAvatarUploader interface {
	Upload(context.Context, string, string) (string, error)
}

type deapAgentOpenAPIAvatarUploader struct {
	delegate deapAgentOpenAPISkillUploader
}

func (u deapAgentOpenAPIAvatarUploader) Upload(
	ctx context.Context, agentUUID, filePath string,
) (string, error) {
	return u.delegate.uploadFile(ctx, agentUUID, filePath, deapAgentAvatarUploadPath)
}

var deapAgentAvatarFileUploader deapAgentAvatarUploader = deapAgentOpenAPIAvatarUploader{}

type deapAgentCreatedEnvelope struct {
	AgentUUID string                    `json:"agentUuid"`
	Data      *deapAgentCreatedEnvelope `json:"data"`
	Result    *deapAgentCreatedEnvelope `json:"result"`
	Content   *deapAgentCreatedEnvelope `json:"content"`
}

type deapAgentAvatarStageError struct {
	Stage     string
	AgentUUID string
	Err       error
}

func (e *deapAgentAvatarStageError) Error() string {
	if e == nil {
		return "头像处理失败"
	}
	detail := ""
	if e.Err != nil {
		detail = e.Err.Error()
	}
	if e.AgentUUID != "" {
		return fmt.Sprintf("头像%s失败；数字员工草稿已创建，agentUuid=%s，请勿重复创建: %s",
			e.Stage, e.AgentUUID, detail)
	}
	return fmt.Sprintf("头像%s失败: %s", e.Stage, detail)
}

func (e *deapAgentAvatarStageError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func deapAgentValidateAvatarURLInput(raw string) (string, bool, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", false, nil
	}
	parsed, err := url.Parse(value)
	if err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") {
		if parsed.Host == "" {
			return "", false, apperrors.NewValidation("参数 --avatar-url 必须是完整的 HTTP(S) 地址")
		}
		return value, false, nil
	}
	resolved, err := apperrors.SafeInputPath(value)
	if err != nil {
		return "", true, apperrors.NewValidation(fmt.Sprintf("参数 --avatar-url 路径不安全: %v", err))
	}
	ext := strings.ToLower(filepath.Ext(resolved))
	if !deapAgentAvatarExtensions[ext] {
		return "", true, apperrors.NewValidation(
			"参数 --avatar-url 本地文件只支持 jpg、jpeg、png、gif 或 webp")
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", true, apperrors.NewValidation("参数 --avatar-url 本地文件不可读")
	}
	if !info.Mode().IsRegular() {
		return "", true, apperrors.NewValidation("参数 --avatar-url 必须是普通文件")
	}
	if info.Size() > deapAgentAvatarMaxFileSize {
		return "", true, apperrors.NewValidation("参数 --avatar-url 头像不能超过 10 MiB")
	}
	return resolved, true, nil
}

func deapAgentValidateAvatarURLFlag(cmd *cobra.Command) error {
	raw, _ := cmd.Flags().GetString("avatar-url")
	_, _, err := deapAgentValidateAvatarURLInput(raw)
	return err
}

func deapAgentCallCreateWithAvatar(cmd *cobra.Command, tool string, args map[string]any) error {
	deapAgentPrepareProfile(args)
	avatarURL, local, err := deapAgentValidateAvatarURLInput(stringArgument(args, "avatarUrl"))
	if err != nil {
		return err
	}
	prompt := stringArgument(args, "prompt")
	delete(args, "prompt") // The create API has no prompt field; persist it in the new draft.
	profile, _ := args["digitalTagEmployeeProfile"].(map[string]any)
	agentType := stringArgument(profile, "type")
	defaulted := prompt == "" && agentType == deapAgentMainProgramTypeLocalAgent
	if defaulted {
		prompt = deapAgentLocalDefaultPrompt
	}
	if prompt == "" {
		deps.Out.PrintWarning("未提供 --prompt；本次仅创建草稿，请在发布前使用 manage save-draft --agent-uuid <agentUuid> --prompt 配置人设。")
	}
	if !local && prompt == "" {
		return CallMCPToolOnServerContext(cmd.Context(), deapAgentServerID, tool, args)
	}
	patch := map[string]any{}
	if prompt != "" {
		patch["prompt"] = prompt
	}
	action := "create_then_save_draft"
	if local {
		delete(args, "avatarUrl")
		action = "create_then_" + deapAgentAvatarUploadAction
	}
	if commandDryRun(cmd) || deps.Caller.DryRun() {
		if local {
			args["avatarUrl"] = map[string]any{"localFile": filepath.Base(avatarURL), "upload": true, "redacted": true}
		}
		return deps.Out.PrintJSON(map[string]any{
			"dry_run": true, "dryRun": true, "executed": false, "action": action, "tool": tool, "arguments": args, "request": args, "draftPatch": patch,
		})
	}
	responseText, err := callMCPToolReturnTextOnServer(cmd.Context(), deapAgentServerID, tool, args)
	if err != nil {
		return err
	}
	agentUUID, err := deapAgentParseCreatedUUID(responseText)
	if err != nil {
		return fmt.Errorf("创建结果解析失败，无法确认 agentUuid；请先查询草稿，勿重复创建: %w", err)
	}
	patch["agentUuid"] = agentUUID
	slog.DebugContext(cmd.Context(), "dingtalk_tag_create_draft_created", "agentUuid", agentUUID, "hasPrompt", prompt != "", "promptDefaulted", defaulted, "localAvatar", local)
	if local {
		fileURL, err := deapAgentAvatarFileUploader.Upload(cmd.Context(), agentUUID, avatarURL)
		if err != nil {
			return &deapAgentAvatarStageError{Stage: "上传", AgentUUID: agentUUID, Err: err}
		}
		patch["avatarUrl"] = fileURL
	}
	if err := CallMCPToolOnServerContext(cmd.Context(), deapAgentServerID, deapAgentSaveDraftTool, patch); err != nil {
		slog.DebugContext(cmd.Context(), "dingtalk_tag_create_draft_patch_failed", "agentUuid", agentUUID, "stage", "save_draft")
		return fmt.Errorf("数字员工草稿已创建，agentUuid=%s，但保存人设/头像失败；请通过 manage detail / save-draft 恢复，勿重复创建: %w", agentUUID, err)
	}
	slog.DebugContext(cmd.Context(), "dingtalk_tag_create_draft_initialized", "agentUuid", agentUUID, "promptDefaulted", defaulted)
	return nil
}

func deapAgentCallSaveWithAvatar(cmd *cobra.Command, tool string, args map[string]any) error {
	deapAgentPrepareProfile(args)
	avatarURL, local, err := deapAgentValidateAvatarURLInput(stringArgument(args, "avatarUrl"))
	if err != nil {
		return err
	}
	if !local {
		return callMCPToolOnServer(deapAgentServerID, tool, args)
	}
	agentUUID := stringArgument(args, "agentUuid")
	if deps.Caller.DryRun() {
		args["avatarUrl"] = map[string]any{
			"localFile": filepath.Base(avatarURL), "upload": true, "redacted": true,
		}
		return deps.Out.PrintJSON(map[string]any{
			"dryRun": true, "action": deapAgentAvatarUploadAction, "request": args,
		})
	}
	fileURL, err := deapAgentAvatarFileUploader.Upload(cmd.Context(), agentUUID, avatarURL)
	if err != nil {
		return &deapAgentAvatarStageError{Stage: "上传", Err: err}
	}
	args["avatarUrl"] = fileURL
	return callMCPToolOnServer(deapAgentServerID, tool, args)
}

func deapAgentParseCreatedUUID(responseText string) (string, error) {
	var envelope deapAgentCreatedEnvelope
	if err := json.Unmarshal([]byte(responseText), &envelope); err != nil {
		return "", fmt.Errorf("创建响应格式非法")
	}
	for candidate := &envelope; candidate != nil; {
		if strings.TrimSpace(candidate.AgentUUID) != "" {
			return strings.TrimSpace(candidate.AgentUUID), nil
		}
		switch {
		case candidate.Data != nil:
			candidate = candidate.Data
		case candidate.Result != nil:
			candidate = candidate.Result
		default:
			candidate = candidate.Content
		}
	}
	return "", fmt.Errorf("创建响应缺少 agentUuid")
}

func stringArgument(args map[string]any, key string) string {
	value, _ := args[key].(string)
	return strings.TrimSpace(value)
}
