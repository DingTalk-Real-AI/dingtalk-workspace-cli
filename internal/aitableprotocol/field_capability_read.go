// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitableprotocol

import (
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

// FieldCapabilityReadError preserves the MCP read receipt at both the runner
// and helper boundaries. Scope is explicit: neither writes nor other products
// inherit retry advice from a similarly shaped error envelope.
func FieldCapabilityReadError(product, tool string, body map[string]any) error {
	if product != "aitable" || (tool != "list_field_groups" && tool != "list_builtin_ai_field_plugins") || body["status"] != "error" {
		return nil
	}
	upstream, _ := body["error"].(map[string]any)
	code, _ := upstream["code"].(string)
	code = strings.TrimSpace(code)
	if code == "" {
		return nil
	}
	message, _ := upstream["message"].(string)
	if strings.TrimSpace(message) == "" {
		message = "AI table read failed; inspect the error code before retrying"
	}
	// Copy only the public code, trace identifier, safe message and boolean hint.
	// Do not infer retryability from prose, retain raw data/details, or retry here.
	diag := transport.ExtractServerDiagnosticsFromMap(body)
	return apperrors.NewAPI(message, apperrors.WithOrigin("mcp"),
		apperrors.WithOperation("tools/call"), apperrors.WithServerKey(product),
		apperrors.WithReason(code), apperrors.WithServerDiag(apperrors.ServerDiagnostics{ServerErrorCode: code, TraceID: diag.TraceID}),
		apperrors.WithRetryable(upstream["retryable"] == true))
}
