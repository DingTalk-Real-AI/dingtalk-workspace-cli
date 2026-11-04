// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package helpers

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/jsonutil"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/publishedmcp"
)

// Preserve bounded, named service diagnostics, never the full remote payload or
// arbitrary text blocks: those can contain template files and business values.
func aicardTemplateServiceError(service, tool string, invocation publishedmcp.ValidatedInvocationResult, payload map[string]any) error {
	return apperrors.NewAPI("The template service rejected the operation",
		apperrors.WithReason("aicard_template_rejected"), apperrors.WithRetryable(false),
		apperrors.WithDetails(aicardTemplateServiceDetails(service, tool, invocation, payload)))
}

func aicardTemplateServiceDetails(service, tool string, invocation publishedmcp.ValidatedInvocationResult, payload map[string]any) map[string]any {
	if payload == nil {
		payload = invocation.Result.StructuredContent
		if payload == nil && len(invocation.Result.Blocks) == 0 {
			payload = invocation.Result.Content
		}
		if payload == nil {
			for _, block := range invocation.Result.Blocks {
				if block.Type != "text" || jsonutil.RejectDuplicateObjectKeys([]byte(block.Text)) != nil {
					continue
				}
				if json.Unmarshal([]byte(block.Text), &payload) == nil && payload != nil {
					break
				}
			}
		}
	}
	code := aicardTemplateDiagnostic(payload, "errorCode", "code")
	message := aicardTemplateDiagnostic(payload, "errorMessage", "errorMsg", "message")
	if nested, ok := payload["error"].(map[string]any); ok {
		if code == "" {
			code = aicardTemplateDiagnostic(nested, "errorCode", "code")
		}
		if message == "" {
			message = aicardTemplateDiagnostic(nested, "errorMessage", "errorMsg", "message")
		}
	}
	details := map[string]any{"mcpId": service, "operation": tool, "retryAutomatically": false}
	if code != "" {
		details["serviceErrorCode"] = code
	}
	if message != "" {
		details["serviceMessage"] = message
	}
	return details
}

func aicardTemplateDiagnostic(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		value, _ := payload[key].(string)
		if key == "code" || key == "errorCode" {
			switch payload[key].(type) {
			case json.Number, float64, int, int64:
				value = fmt.Sprint(payload[key])
			}
		}
		if strings.TrimSpace(value) == "" {
			continue
		}
		var result strings.Builder
		for _, r := range value {
			if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
				r = ' '
			}
			if result.Len()+utf8.RuneLen(r) > 1024 {
				break
			}
			result.WriteRune(r)
		}
		return strings.TrimSpace(result.String())
	}
	return ""
}
