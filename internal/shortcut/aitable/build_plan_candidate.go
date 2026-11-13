// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"errors"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

// buildPlanCreationCandidate retains only explicitly unknown creation identities
// bound to the original Base and parent. It never upgrades a pre-write refusal,
// parses IDs from prose, or treats a candidate as accepted/verified evidence.
func buildPlanCreationCandidate(tool, base string, args map[string]any, cause error) map[string]any {
	if tool != "create_dashboard" && tool != "create_chart" {
		return nil
	}
	var typed *apperrors.Error
	if !errors.As(cause, &typed) || typed.ExecutionStarted == nil || !*typed.ExecutionStarted ||
		typed.Details["mutationSubmitted"] != true || typed.Details["deliveryStatus"] != "unknown" || typed.Details["baseId"] != base {
		return nil
	}
	dashboard, ok := typed.Details["dashboardId"].(string)
	if !ok || dashboard == "" || dashboard != strings.TrimSpace(dashboard) {
		return nil
	}
	candidate := map[string]any{"baseId": base, "dashboardId": dashboard, "mutationSubmitted": true, "deliveryStatus": "unknown"}
	if tool == "create_chart" {
		chart, ok := typed.Details["chartId"].(string)
		if !ok || chart == "" || chart != strings.TrimSpace(chart) || args["dashboardId"] != dashboard {
			return nil
		}
		candidate["chartId"] = chart
	}
	return candidate
}
