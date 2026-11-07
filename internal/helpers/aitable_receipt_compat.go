// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"context"
	"fmt"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

// Additive projections only: existing consumers retain newRecordIds and the
// entire original MCP envelope. Never derive filtered counts from prose.
func callAitableCompatibleReceipt(ctx context.Context, tool string, args map[string]any) error {
	if deps.Caller.DryRun() {
		return callMCPToolContext(ctx, tool, args)
	}
	call := func(callCtx context.Context) (any, error) {
		return CallMCPToolDataOnServer(callCtx, "aitable", tool, args)
	}
	var receipt any
	var err error
	if isAitableReadRetryTool(tool) {
		receipt, err = callAitableReadWithRetry(ctx, tool, call)
	} else {
		// Adding a receipt alias must not enable replay of create_records.
		receipt, err = call(ctx)
	}
	if err != nil {
		return err
	}
	data := aitableReceiptData(receipt)
	switch tool {
	case "create_records":
		if ids, exists := data["newRecordIds"]; exists {
			if _, present := data["createdRecordIds"]; !present {
				data["createdRecordIds"] = ids
			}
		}
	case "list_bases":
		if bases, ok := data["bases"].([]any); ok {
			data["returnedCount"] = len(bases)
			cursor, present := data["nextCursor"]
			if cursor == nil {
				data["hasMore"] = false
			} else if value, ok := cursor.(string); ok {
				data["hasMore"] = strings.TrimSpace(value) != ""
			} else if present {
				return apperrors.NewAPI("list_bases returned a non-string nextCursor", apperrors.WithReason("invalid_pagination_receipt"))
			}
		}
	}
	return renderAitableReceipt(tool, receipt)
}

func validateAitableViewType(value string) error {
	allowed := []string{"Grid", "Kanban", "Gantt", "Calendar", "Gallery", "FormDesigner"}
	for _, candidate := range allowed {
		if value == candidate {
			return nil
		}
		if strings.EqualFold(value, candidate) {
			return apperrors.NewValidation(fmt.Sprintf("--view-type is case-sensitive: got %q; did you mean %q?", value, candidate))
		}
	}
	return apperrors.NewValidation("--view-type must be one of: " + strings.Join(allowed, ", "))
}
