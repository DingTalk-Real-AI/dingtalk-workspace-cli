// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

// 临时凭证的验证结果独立于本地 OAuth 状态，不构造或保存 TokenData。
func runAuthStatusExplicitToken(cmd *cobra.Command, token string, caller edition.ToolCaller) error {
	response := struct {
		authStatusResponse
		TokenSource  string `json:"token_source"`
		Verification string `json:"verification"`
	}{authStatusResponse: authStatusResponse{Success: true}, TokenSource: "explicit", Verification: "not_checked"}
	readonly, _ := cmd.Flags().GetBool("readonly")
	dryrun, _ := cmd.Root().PersistentFlags().GetBool("dry-run")
	response.Message = "已注入临时凭证，未联网验证"
	response.Reason = "explicit_token_not_checked"
	if !readonly && !dryrun {
		response.Verification = "unknown"
		response.Message = "无法确认临时凭证是否有效"
		response.Reason = "explicit_token_verification_failed"
		if scoped, ok := caller.(tokenOverrideToolCaller); ok {
			ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
			result, err := scoped.CallToolWithToken(ctx, token, "contact", "get_current_user_profile", nil)
			cancel()
			// 只认明确的认证拒绝；权限不足和网络失败均不能判定 token 无效。
			if isRefreshableTransportAuthError(err) {
				response.Verification = "invalid"
				response.Message = "临时凭证已被认证服务拒绝"
				response.Reason = "explicit_token_rejected"
			} else if err == nil && authStatusTokenIdentityValid(result) {
				response.Verification = "valid"
				response.Authenticated = true
				response.TokenValid = true
				response.Message = "临时凭证验证成功"
				response.Reason = ""
			}
		}
	}
	check := commandVersionCheck(cmd, false)
	response.VersionCheck = &check
	response.Notice = noticeForVersion(check, false)
	format, _ := cmd.Root().PersistentFlags().GetString("format")
	if strings.EqualFold(strings.TrimSpace(format), "json") {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(response)
	}
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "来源: explicit\n验证: %s\n状态: %s\n", response.Verification, response.Message)
	if err != nil {
		return err
	}
	return writeVersionCheck(cmd.ErrOrStderr(), check)
}

func authStatusTokenIdentityValid(result *edition.ToolResult) bool {
	if result == nil {
		return false
	}
	valid := false
	for _, block := range result.Content {
		if block.Type != "text" {
			continue
		}
		var payload map[string]json.RawMessage
		if json.Unmarshal([]byte(block.Text), &payload) != nil {
			return false
		}
		if raw, ok := payload["success"]; ok && string(raw) != "true" {
			return false
		}
		if raw, ok := payload["dingOpenErrcode"]; ok && string(raw) != "0" {
			return false
		}
		identity, ok := authpkg.ContactProfileIdentityFromJSON([]byte(block.Text))
		valid = valid || (ok && identity.UserID != "" && identity.CorpID != "")
	}
	return valid
}
