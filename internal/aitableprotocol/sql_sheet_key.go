// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package aitableprotocol

import (
	"errors"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

// IsSQLSheetKeyOperation identifies credential-management calls by product and tool.
func IsSQLSheetKeyOperation(product, tool string) bool {
	if product != "aitable" {
		return false
	}
	switch tool {
	case "create_sql_sheet_api_key", "list_sql_sheet_api_keys", "revoke_sql_sheet_api_key":
		return true
	}
	return false
}

// SQLSheetKeyFailure projects known error codes to messages safe for output and audit.
func SQLSheetKeyFailure(tool, code string) error {
	var message string
	switch code {
	case "403":
		message = "需要文档 MANAGER 权限"
	case "404":
		message = "目标资源不存在"
	case "SQL_SHEET_API_KEY_ALREADY_EXISTS":
		message = "已有有效 Key；使用 list 核对，不自动覆盖"
	case "600", "INVALID_PARAM", "INVALID_SQL_SHEET_API_KEY_REQUEST":
		message = "请求参数无效"
	case "RATE_LIMIT_EXCEEDED":
		message = "API Key 管理请求已被限流；本次不自动重试"
	case "INTERNAL_ERROR", "INVALID_SQL_SHEET_API_KEY_RESPONSE", "SQL_SHEET_API_KEY_OPERATION_FAILED":
		message = "调用失败或结果未确认；先用 list 核对，不自动重试、撤销或重新创建"
	default:
		code = "SQL_SHEET_API_KEY_OPERATION_FAILED"
		message = "调用失败或结果未确认；先用 list 核对，不自动重试、撤销或重新创建"
	}
	return apperrors.NewAPI(message, apperrors.WithOperation("aitable/"+tool), apperrors.WithReason(code), apperrors.WithRetryable(false))
}

// SQLSheetKeyCallError preserves known local failures and safe business codes.
func SQLSheetKeyCallError(tool string, err error) error {
	var typed *apperrors.Error
	if errors.As(err, &typed) {
		if typed.Category == apperrors.CategoryValidation && typed.Origin == "client" && typed.ExecutionStarted != nil && !*typed.ExecutionStarted {
			return apperrors.NewValidation("调用前检查失败；请检查当前配置和请求参数",
				apperrors.WithReason("sql_sheet_api_key_setup_failed"), apperrors.WithOrigin("client"),
				apperrors.WithFailureStage("invocation_setup"), apperrors.WithExecutionStarted(false), apperrors.WithRetryable(false))
		}
		switch typed.Reason {
		case "sql_sheet_api_key_single_profile_required":
			return SQLSheetKeyProfileError()
		case "not_authenticated":
			return apperrors.NewAuth("未登录，请先执行 dws auth login", apperrors.WithReason("not_authenticated"))
		default:
			return SQLSheetKeyFailure(tool, typed.Reason)
		}
	}
	return SQLSheetKeyFailure(tool, "")
}

// SQLSheetKeyProfileError reports the single-identity command contract.
func SQLSheetKeyProfileError() error {
	return apperrors.NewValidation("API Key 管理仅支持单一 profile", apperrors.WithReason("sql_sheet_api_key_single_profile_required"))
}

// SQLSheetKeyResponseError classifies the business envelope before runtime audit is emitted.
// Successful gateway responses may represent an absent business error as null or {}.
func SQLSheetKeyResponseError(tool string, body map[string]any, isError bool) error {
	detail, isObject := body["error"].(map[string]any)
	// Only a zero-member object is equivalent to no error; empty error fields are not.
	if !isError && body["status"] == "success" && (body["error"] == nil || (isObject && len(detail) == 0)) {
		return nil
	}
	code, _ := detail["code"].(string)
	return SQLSheetKeyFailure(tool, code)
}
