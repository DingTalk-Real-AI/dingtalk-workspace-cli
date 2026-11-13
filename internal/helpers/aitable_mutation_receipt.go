// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

// classifyAitableMutationError preserves MCP's explicit pre-submission evidence
// and safe batch candidates without turning an error envelope into success.
// Only reviewed mutation protocols are handled here; arbitrary response/config
// fields must not become error diagnostics.
func classifyAitableMutationError(body map[string]any) error {
	if body["status"] != "error" {
		return nil
	}
	upstream, _ := body["error"].(map[string]any)
	code, _ := upstream["code"].(string)
	details, _ := upstream["details"].(map[string]any)
	known := (code == "FORBIDDEN" && details["mutationSubmitted"] == false) || code == "LAYOUT_CONFLICT" || code == "ATOMIC_LAYOUT_PRECONDITION_UNAVAILABLE"
	for _, prefix := range []string{"DASHBOARD_WRITE_", "DASHBOARD_STORAGE_", "BATCH_CREATE_CHART", "CHART_LAYOUT_", "CREATE_CHART", "UPDATE_CHART", "DELETE_CHART", "CREATE_DASHBOARD", "UPDATE_DASHBOARD", "DELETE_DASHBOARD", "ALIGN_DASHBOARD"} {
		known = known || strings.HasPrefix(code, prefix)
	}
	candidates := safeAitableChartCandidates(body["data"])
	unknown := details["deliveryStatus"] == "unknown" && (details["mutationSubmitted"] == true || candidates != nil)
	// Legacy services without explicit mutation evidence retain their existing
	// error representation; only the new structured evidence is projected.
	_, hasMutationEvidence := details["mutationSubmitted"].(bool)
	if !unknown && (!known || !hasMutationEvidence) {
		return nil
	}
	projected := map[string]any{}
	opts := []apperrors.Option{apperrors.WithOrigin("mcp"), apperrors.WithReason(code),
		apperrors.WithServerDiag(apperrors.ServerDiagnostics{ServerErrorCode: code})}
	submitted, confirmed := details["mutationSubmitted"].(bool)
	allocationEffect := details["licenseAllocationAttempted"] == true || details["licenseAllocationConfirmed"] == true
	if unknown {
		submitted, confirmed = true, true
		projected["deliveryStatus"] = "unknown"
		if candidates != nil {
			projected["result"] = candidates
		}
	}
	if confirmed {
		projected["mutationSubmitted"] = submitted
		// mutationSubmitted describes the layout only. A seat allocation attempt
		// is a separate real side effect, even when its result is unconfirmed.
		opts = append(opts, apperrors.WithExecutionStarted(submitted || allocationEffect))
	}
	// A retryable permission/read failure is safe only with explicit no-write
	// evidence. This preserves advice; it never retries a mutation in the CLI.
	opts = append(opts, apperrors.WithRetryable(confirmed && !submitted && !allocationEffect && upstream["retryable"] == true))
	if stage, ok := details["stage"].(string); ok {
		switch stage {
		case "document_edit", "advanced_permission", "document_role", "license":
			projected["stage"] = stage
		}
	}
	for _, key := range []string{"licenseAllocationAttempted", "licenseAllocationConfirmed"} {
		if value, ok := details[key].(bool); ok {
			projected[key] = value
		}
	}
	for _, key := range []string{"baseId", "dashboardId", "chartId"} {
		if value, ok := details[key].(string); ok && value != "" {
			projected[key] = value
		}
	}
	opts = append(opts, apperrors.WithDetails(projected))
	return apperrors.NewAPI(businessErrorDisplayMessage(body, "Dashboard/chart write was rejected; retain the receipt"), opts...)
}

// safeAitableChartCandidates allowlists only reconciliation identities and
// statuses. It deliberately excludes downstream messages, config and identities.
func safeAitableChartCandidates(value any) map[string]any {
	data, _ := value.(map[string]any)
	base, _ := data["baseId"].(string)
	dashboard, _ := data["dashboardId"].(string)
	rows, ok := data["results"].([]any)
	if base == "" || dashboard == "" || !ok || len(rows) == 0 || len(rows) > 20 {
		return nil
	}
	items := make([]any, 0, len(rows))
	for _, value := range rows {
		row, ok := value.(map[string]any)
		if !ok {
			return nil
		}
		item := map[string]any{}
		for _, key := range []string{"index", "chartId", "status", "success"} {
			if v, exists := row[key]; exists {
				switch v.(type) {
				case string, float64, bool:
					item[key] = v
				}
			}
		}
		items = append(items, item)
	}
	return map[string]any{"baseId": base, "dashboardId": dashboard, "results": items}
}
