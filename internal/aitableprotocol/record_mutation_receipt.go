// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitableprotocol

import apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"

// ClassifyAitableRecordMutationError retains the pure record projection for product callers.
func ClassifyAitableRecordMutationError(product, tool string, body map[string]any) error {
	if projected := classifyAitableRecordMutationError(product, tool, body); projected != nil {
		return projected
	}
	return nil
}

// classifyAitableRecordMutationError projects reviewed record recovery receipts
// before either a text response or MCP isError handling can echo raw diagnostics.
// Context is required: error-code collisions in other products must preserve
// their original classification, and reads never acquire write semantics.
func classifyAitableRecordMutationError(serverID, toolName string, body map[string]any) *apperrors.Error {
	if serverID != "aitable" && serverID != "aitable-helper" {
		return nil
	}
	switch toolName {
	case "create_records", "update_records", "record_upsert":
	default:
		return nil
	}
	if body["status"] != "error" {
		return nil
	}
	upstream, _ := body["error"].(map[string]any)
	code, _ := upstream["code"].(string)
	details, _ := upstream["details"].(map[string]any)
	mixedRecovery := (code == "CREATE_RECORDS_OUTCOME_UNKNOWN" || code == "DUPLICATE_CLIENT_TOKEN") &&
		details["reconcileTool"] == "get_record_write_result"
	if details["attachmentVerification"] == "unconfirmed" && (code == "ATTACHMENT_WRITE_UNCONFIRMED" || mixedRecovery) {
		return classifyAitableAttachmentReceipt(code, details)
	}
	if groups, recovery := ProjectAitableRecordGroupOutcomes(details["groupOutcomes"]); recovery {
		projected := safeAitableRecordRecoveryDetails(details)
		projected["groupOutcomes"] = groups
		return aitableRecordRecoveryError(code, projected)
	}
	// A single create/update has the same recovery obligation as an upsert group.
	// Require both a reviewed record code and explicit evidence; other products
	// and legacy errors without a write stage retain their existing projection.
	projected := safeAitableRecordRecoveryDetails(details)
	outcome, _ := projected["writeOutcome"].(string)
	clientToken, _ := projected["clientToken"].(string)
	reconcileTool, _ := projected["reconcileTool"].(string)
	if AitableRecordWriteNeedsReconciliation(code, outcome, clientToken, reconcileTool) {
		return aitableRecordRecoveryError(code, projected)
	}
	return nil
}

// AitableRecordWriteNeedsReconciliation is shared with record Shortcuts so a
// reviewed single-group write stage cannot lose its non-replay semantics after
// classification. A stage alone never grants an unrelated error this protocol.
func AitableRecordWriteNeedsReconciliation(code, outcome, clientToken, reconcileTool string) bool {
	// A duplicate token describes this attempt, not the original token's write.
	if (outcome == "NOT_ATTEMPTED" || outcome == "REJECTED") && code != "DUPLICATE_CLIENT_TOKEN" {
		return false
	}
	switch code {
	case "ATTACHMENT_PREPARATION_UNAVAILABLE", "CREATE_RECORDS_OUTCOME_UNKNOWN", "UPDATE_RECORDS_RESULT_UNKNOWN", "DUPLICATE_CLIENT_TOKEN":
		return outcome == "OUTCOME_UNKNOWN" || outcome == "COMMITTED_RESULT_UNAVAILABLE" || outcome == "PARTIALLY_COMPLETED" ||
			(safeAitableRecoveryString(clientToken) && reconcileTool == "get_record_write_result")
	}
	return false
}

// classifyAitableAttachmentReceipt retains reconciliation IDs for the exact reviewed attachment protocol.
// It never copies cell contents or arbitrary upstream diagnostics into the CLI error.
func classifyAitableAttachmentReceipt(code string, details map[string]any) *apperrors.Error {
	projected := safeAitableRecordRecoveryDetails(details)
	if groups, _ := ProjectAitableRecordGroupOutcomes(details["groupOutcomes"]); len(groups) > 0 {
		projected["groupOutcomes"] = groups
	}
	return aitableRecordRecoveryError(code, projected)
}

// aitableRecordRecoveryError keeps each submitted group available for read-back;
// the CLI never automatically replays either group of an uncertain upsert.
func aitableRecordRecoveryError(code string, projected map[string]any) *apperrors.Error {
	code = safeAitableRecordErrorCode(code)
	// Unreviewed upstream prose/log IDs can contain upload credentials. Keep
	// machine-readable codes and safe recovery targets, never their raw message.
	return apperrors.NewAPI("Record writes require reconciliation; query original targets without replaying the write",
		apperrors.WithOrigin("mcp"), apperrors.WithReason(code), apperrors.WithDetails(projected),
		apperrors.WithExecutionStarted(true), apperrors.WithRetryable(false),
		apperrors.WithServerDiag(apperrors.ServerDiagnostics{ServerErrorCode: code})).(*apperrors.Error)
}
