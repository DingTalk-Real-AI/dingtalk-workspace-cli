// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"errors"
	"reflect"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func TestCrossPlatformCoverageAitableAttachmentReceiptKeepsOnlySafeRecoveryFields(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		details := map[string]any{
			"attachmentVerification": "unconfirmed", "recordIds": []any{"r"}, "fieldIds": []any{"f"},
			"createdRecordIds": []any{"created"}, "updatedRecordIds": []any{"updated"},
			"baseId": "b", "tableId": "t", "clientToken": "token", "privateRaw": "must not leak",
			"reconcileTool": "get_record_write_result",
		}
		if invalid {
			details["reconcileTool"] = "untrusted_tool"
			details["recordIds"] = []any{"r", 1}
			details["fieldIds"] = make([]any, 101)
			details["createdRecordIds"] = []any{"created", false}
			details["updatedRecordIds"] = make([]any, 101)
			details["baseId"] = "b\nprivate"
		}
		body := map[string]any{"status": "error", "error": map[string]any{
			"code": "ATTACHMENT_WRITE_UNCONFIRMED", "message": "Query without replay", "details": details,
		}, "data": map[string]any{"cells": map[string]any{"secret": "must not leak"}}}
		err := ClassifyAitableRecordMutationError("aitable", "record_upsert", body)
		var typed *apperrors.Error
		if !errors.As(err, &typed) || typed.ExecutionStarted == nil || !*typed.ExecutionStarted || !typed.RetryableSet || typed.Retryable {
			t.Fatalf("unknown attachment receipt was not protected: %#v", err)
		}
		if _, leaked := typed.Details["privateRaw"]; leaked || typed.Details["cells"] != nil {
			t.Fatalf("unsafe upstream fields leaked: %#v", typed.Details)
		}
		if invalid {
			if typed.Details["reconcileTool"] != nil || typed.Details["recordIds"] != nil || typed.Details["fieldIds"] != nil || typed.Details["baseId"] != nil || typed.Details["createdRecordIds"] != nil || typed.Details["updatedRecordIds"] != nil {
				t.Fatalf("invalid recovery IDs leaked: %#v", typed.Details)
			}
		} else if typed.Details["reconcileTool"] != "get_record_write_result" || !reflect.DeepEqual(typed.Details["recordIds"], []string{"r"}) || !reflect.DeepEqual(typed.Details["fieldIds"], []string{"f"}) || typed.Details["clientToken"] != "token" || !reflect.DeepEqual(typed.Details["createdRecordIds"], []string{"created"}) || !reflect.DeepEqual(typed.Details["updatedRecordIds"], []string{"updated"}) {
			t.Fatalf("safe recovery IDs lost: %#v", typed.Details)
		}
	}
}
