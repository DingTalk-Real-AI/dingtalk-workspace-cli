// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"fmt"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/aitableprotocol"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// preflightBoundPlanChart checks the actual target grid after dependency binding,
// before started is persisted or any create call is made. It also works with
// servers that predate the new chart mutation bounds validation.
func preflightBoundPlanChart(rt *shortcut.RuntimeContext, state *buildPlanState, step string, args map[string]any) error {
	dashboard, ok := args["dashboardId"].(string)
	if !ok || dashboard == "" {
		return apperrors.NewValidation("计划图表 dashboardId 必须绑定真实 ID")
	}
	appMode := false
	if value, exists := args["isAppMode"]; exists {
		appMode, ok = value.(bool)
		if !ok {
			return apperrors.NewValidation("isAppMode 必须为布尔值")
		}
	}
	raw, err := buildPlanCallTool(rt, state, step, "get_dashboard", "prepare", map[string]any{"baseId": state.BaseID, "dashboardId": dashboard}, false)
	if err != nil {
		return err
	}
	columns, err := aitableprotocol.ResolveDashboardRootColumns(raw, state.BaseID, dashboard, appMode)
	if err != nil {
		return err
	}
	layout, ok := args["layout"].(map[string]any)
	if !ok {
		return fmt.Errorf("计划图表缺少完整布局")
	}
	if err := aitableprotocol.ValidateRootChartLayout(layout, columns); err != nil {
		return apperrors.NewValidation(err.Error())
	}
	args["isAppMode"] = appMode
	return nil
}
