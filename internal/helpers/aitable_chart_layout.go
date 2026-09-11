// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/aitableprotocol"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

// validateAitableChartLayout performs the read-before-write protocol check for
// Chart layout mutations. isAppMode is read-only caller context and is never
// forwarded into Dashboard or Chart persistent payloads.
func validateAitableChartLayout(
	ctx context.Context,
	baseID, dashboardID string,
	layout map[string]any,
	isAppMode bool,
) error {
	raw, err := callAitableReadToolTextContext(ctx, "get_dashboard", map[string]any{
		"baseId":      baseID,
		"dashboardId": dashboardID,
	})
	if err != nil {
		return err
	}
	if strings.TrimSpace(raw) == "" {
		return newAitableDashboardPreflightResponseError(
			"empty_tool_response", "get_dashboard 返回空响应，未执行 Chart 布局写入")
	}
	var dashboard map[string]any
	if err := json.Unmarshal([]byte(raw), &dashboard); err != nil {
		return newAitableDashboardPreflightResponseError(
			"invalid_tool_response",
			fmt.Sprintf("get_dashboard 返回不是合法 JSON 对象，未执行 Chart 布局写入: %v", err))
	}
	totalColumns, err := aitableprotocol.ResolveDashboardRootColumns(
		dashboard, baseID, dashboardID, isAppMode)
	if err != nil {
		return newAitableDashboardPreflightResponseError(
			"dashboard_protocol_evidence_invalid", err.Error()+"；未执行 Chart 布局写入")
	}
	if err := aitableprotocol.ValidateRootChartLayout(layout, totalColumns); err != nil {
		return apperrors.NewValidation(
			err.Error()+"；未执行 Chart 布局写入",
			apperrors.WithReason("invalid_chart_layout"),
			apperrors.WithFailureStage("request_validation"),
			apperrors.WithExecutionStarted(false),
			apperrors.WithRetryable(false),
		)
	}
	return nil
}

func newAitableDashboardPreflightResponseError(reason, message string) error {
	return apperrors.NewAPI(
		message,
		apperrors.WithOperation("aitable/get_dashboard"),
		apperrors.WithOrigin("mcp"),
		apperrors.WithFailureStage("response_validation"),
		apperrors.WithExecutionStarted(false),
		apperrors.WithRetryable(false),
		apperrors.WithReason(reason),
	)
}
