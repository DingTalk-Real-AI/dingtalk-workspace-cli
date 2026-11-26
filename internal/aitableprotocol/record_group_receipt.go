// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitableprotocol

import (
	"strings"
	"unicode"
)

// ProjectAitableRecordGroupOutcomes projects the two reviewed upsert groups. It
// keeps requested update targets separate from accepted IDs and excludes cells,
// messages, internal identities and arbitrary downstream diagnostics.
func ProjectAitableRecordGroupOutcomes(value any) (map[string]any, bool) {
	raw, _ := value.(map[string]any)
	groups := map[string]any{}
	recovery := false
	for _, name := range []string{"create", "update"} {
		group, _ := raw[name].(map[string]any)
		requested, valid := group["requested"].(bool)
		if len(group) == 0 {
			continue
		}
		projected := map[string]any{}
		if valid {
			projected["requested"] = requested
		}
		if !valid || requested {
			for _, key := range []string{"requestedRecordIds", "acceptedRecordIds"} {
				if ids := safeAitableRecordIDs(group[key]); ids != nil {
					projected[key] = ids
				}
			}
			if upstream, ok := group["error"].(map[string]any); ok {
				details, _ := upstream["details"].(map[string]any)
				safe := safeAitableRecordRecoveryDetails(details)
				groupError := map[string]any{"retryable": false, "details": safe}
				code, _ := upstream["code"].(string)
				code = safeAitableRecordErrorCode(code)
				groupError["code"] = code
				outcome, _ := safe["writeOutcome"].(string)
				if code == "DUPLICATE_CLIENT_TOKEN" || (outcome != "REJECTED" && outcome != "NOT_ATTEMPTED") {
					recovery = recovery || !valid || code == "UPDATE_RECORDS_RESULT_UNKNOWN" || code == "ATTACHMENT_WRITE_UNCONFIRMED" ||
						code == "DUPLICATE_CLIENT_TOKEN" || strings.HasSuffix(code, "_OUTCOME_UNKNOWN") ||
						outcome == "OUTCOME_UNKNOWN" || outcome == "COMMITTED_RESULT_UNAVAILABLE" || outcome == "PARTIALLY_COMPLETED" ||
						safe["mutationSubmitted"] == true || (safe["clientToken"] != nil && safe["reconcileTool"] == "get_record_write_result")
				}
				projected["error"] = groupError
			} else if !valid && (projected["requestedRecordIds"] != nil || projected["acceptedRecordIds"] != nil) {
				recovery = true
			}
		}
		groups[name] = projected
	}
	return groups, recovery
}

// safeAitableRecordRecoveryDetails allows only bounded public recovery targets
// and explicit write stages; no unreviewed field becomes CLI error metadata.
func safeAitableRecordRecoveryDetails(details map[string]any) map[string]any {
	safe := map[string]any{}
	for _, key := range []string{"baseId", "tableId", "clientToken"} {
		if value, ok := details[key].(string); ok && safeAitableRecoveryString(value) {
			safe[key] = value
		}
	}
	for _, key := range []string{"recordIds", "fieldIds", "createdRecordIds", "updatedRecordIds"} {
		if ids := safeAitableRecordIDs(details[key]); ids != nil {
			safe[key] = ids
		}
	}
	if submitted, ok := details["mutationSubmitted"].(bool); ok {
		safe["mutationSubmitted"] = submitted
	}
	switch details["writeOutcome"] {
	case "NOT_ATTEMPTED", "REJECTED", "OUTCOME_UNKNOWN", "COMMITTED_RESULT_UNAVAILABLE", "PARTIALLY_COMPLETED":
		safe["writeOutcome"] = details["writeOutcome"]
	}
	if details["attachmentVerification"] == "unconfirmed" {
		safe["attachmentVerification"] = "unconfirmed"
	}
	if details["reconcileTool"] == "get_record_write_result" {
		safe["reconcileTool"] = "get_record_write_result"
	}
	return safe
}

// safeAitableRecordIDs rejects the whole malformed list rather than truncating a
// reconciliation scope and silently omitting targets that still need read-back.
func safeAitableRecordIDs(value any) []string {
	items, ok := value.([]any)
	if !ok || len(items) > 100 {
		return nil
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		id, ok := item.(string)
		if !ok || !safeAitableRecoveryString(id) {
			return nil
		}
		ids = append(ids, id)
	}
	return ids
}

// safeAitableRecoveryString bounds protocol identifiers and rejects log controls.
func safeAitableRecoveryString(value string) bool {
	return strings.TrimSpace(value) != "" && len(value) <= 256 && strings.IndexFunc(value, unicode.IsControl) < 0
}

func safeAitableRecordErrorCode(code string) string {
	if code != "" && len(code) <= 256 && strings.IndexFunc(code, func(r rune) bool {
		return !(r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_')
	}) < 0 {
		return code
	}
	return "RECORD_WRITE_REQUIRES_RECONCILIATION"
}
