// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"encoding/json"
	"fmt"
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
	if !local {
		return callMCPToolOnServer(deapAgentServerID, tool, args)
	}
	delete(args, "avatarUrl")
	if deps.Caller.DryRun() {
		args["avatarUrl"] = map[string]any{
			"localFile": filepath.Base(avatarURL), "upload": true, "redacted": true,
		}
		return deps.Out.PrintJSON(map[string]any{
			"dryRun": true, "action": "create_then_" + deapAgentAvatarUploadAction, "request": args,
		})
	}
	responseText, err := callMCPToolReturnTextOnServer(cmd.Context(), deapAgentServerID, tool, args)
	if err != nil {
		return err
	}
	agentUUID, err := deapAgentParseCreatedUUID(responseText)
	if err != nil {
		return &deapAgentAvatarStageError{Stage: "创建结果解析", Err: err}
	}
	fileURL, err := deapAgentAvatarFileUploader.Upload(cmd.Context(), agentUUID, avatarURL)
	if err != nil {
		return &deapAgentAvatarStageError{Stage: "上传", AgentUUID: agentUUID, Err: err}
	}
	return callMCPToolOnServer(deapAgentServerID, deapAgentSaveDraftTool, map[string]any{
		"agentUuid": agentUUID,
		"avatarUrl": fileURL,
	})
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
