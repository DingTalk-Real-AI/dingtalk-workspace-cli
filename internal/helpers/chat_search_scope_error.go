// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package helpers

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

// NormalizeSearchConversationScopeError maps only errors that prove the
// requested conversation does not exist or that openConversationId itself is
// invalid. Unknown MCP tool failures must remain unchanged: the legacy
// CodeMCPToolError bucket also carries permission, throttling, and transient
// backend failures, none of which proves that the caller supplied a bad CID.
func NormalizeSearchConversationScopeError(conversationID string, err error) error {
	if err == nil || !isDefinitiveInvalidSearchConversationError(err) {
		return err
	}
	return apperrors.NewValidation(
		fmt.Sprintf("无法验证会话 CID %q；已停止搜索，避免过滤失效后返回其他会话消息", conversationID),
		apperrors.WithReason("search_conversation_scope_invalid"),
		apperrors.WithDetails(map[string]any{"conversationId": conversationID}),
		apperrors.WithRetryable(false),
		apperrors.WithHint("确认 openConversationId 存在且当前账号可访问后重试"),
		apperrors.WithCause(err),
	)
}

func isDefinitiveInvalidSearchConversationError(err error) bool {
	var cliErr *CLIError
	if errors.As(err, &cliErr) {
		switch cliErr.Code {
		case CodeResourceNotFound, CodeInvalidParam:
			return true
		}
		code, message := searchConversationErrorFacts(cliErr.Message)
		if isExplicitInvalidConversationCode(code) || isConversationParameterError(code, message) {
			return true
		}
	}

	// Transport errors can survive below a legacy CLIError in the cause chain.
	// Inspect their structured diagnostics, but require conversation-specific
	// evidence before treating a generic PARAM_ERROR as an invalid CID.
	var appErr *apperrors.Error
	if errors.As(err, &appErr) {
		code := strings.TrimSpace(appErr.ServerDiag.ServerErrorCode)
		message := strings.Join([]string{
			appErr.Message,
			appErr.ServerDiag.TechnicalDetail,
			appErr.Reason,
			appErr.FailureStage,
		}, " ")
		return isExplicitInvalidConversationCode(code) || isConversationParameterError(code, message)
	}
	return false
}

func searchConversationErrorFacts(raw string) (string, string) {
	var body map[string]any
	if json.Unmarshal([]byte(raw), &body) != nil {
		return "", raw
	}
	code := firstSearchConversationErrorString(body, "errorCode", "error_code", "code")
	message := firstSearchConversationErrorString(body, "errorMsg", "error_msg", "message", "error")
	return code, message
}

func firstSearchConversationErrorString(body map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := body[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func isExplicitInvalidConversationCode(code string) bool {
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case "INVALID_OPEN_CONVERSATION_ID", "OPEN_CONVERSATION_NOT_FOUND", "CONVERSATION_NOT_FOUND":
		return true
	default:
		return false
	}
}

func isConversationParameterError(code, message string) bool {
	normalizedCode := strings.ToUpper(strings.TrimSpace(code))
	if normalizedCode != "PARAM_ERROR" && normalizedCode != "PARAMETER_ERROR" && normalizedCode != "INVALID_ARGUMENT" {
		return false
	}
	normalizedMessage := strings.ToLower(strings.TrimSpace(message))
	mentionsConversationID := strings.Contains(normalizedMessage, "openconversationid") ||
		strings.Contains(normalizedMessage, "open conversation id") ||
		strings.Contains(normalizedMessage, "conversation id") ||
		strings.Contains(normalizedMessage, "cid")
	if !mentionsConversationID {
		return false
	}
	for _, marker := range []string{
		"invalid", "illegal", "malformed", "required", "missing", "not found", "不存在", "无效", "非法", "缺少", "必填",
	} {
		if strings.Contains(normalizedMessage, marker) {
			return true
		}
	}
	return false
}
