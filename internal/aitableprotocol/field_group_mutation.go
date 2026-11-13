// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitableprotocol

import (
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

// FieldGroupMutationError projects only reviewed stage codes. Retry guidance
// requires both a known pre-submission stage and an explicit boolean hint;
// neither raw diagnostics nor unknown outcomes authorize another write.
func FieldGroupMutationError(product, tool string, body map[string]any) error {
	if product != "aitable" {
		return nil
	}
	switch tool {
	case "create_field_group", "update_field_group", "ungroup_field_group", "move_field_group_fields":
	default:
		return nil
	}
	if body["status"] != "error" {
		return nil
	}
	upstream, _ := body["error"].(map[string]any)
	code, _ := upstream["code"].(string)
	preSubmissionRetry := false
	switch code {
	case "FIELD_GROUP_SERVICE_UNAVAILABLE", "FIELD_GROUP_PRECHECK_FAILED", "FIELD_GROUP_DATA_NOT_READY":
		preSubmissionRetry = true
	case "FIELD_GROUP_CREDENTIAL_REJECTED", "FIELD_GROUP_PERMISSION_REJECTED", "FIELD_GROUP_INPUT_REJECTED", "FIELD_GROUP_OUTCOME_UNKNOWN":
	default:
		return nil
	}
	message, _ := upstream["message"].(string)
	if strings.TrimSpace(message) == "" {
		message = "AI table mutation failed; inspect the stage code before another write"
	}
	diag := transport.ExtractServerDiagnosticsFromMap(body)
	return apperrors.NewAPI(message, apperrors.WithOrigin("mcp"), apperrors.WithReason(code),
		apperrors.WithOperation("tools/call"), apperrors.WithServerKey(product),
		apperrors.WithServerDiag(apperrors.ServerDiagnostics{ServerErrorCode: code, TraceID: diag.TraceID}),
		apperrors.WithRetryable(preSubmissionRetry && upstream["retryable"] == true))
}
