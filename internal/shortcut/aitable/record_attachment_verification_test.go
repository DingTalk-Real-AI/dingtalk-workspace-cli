// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func TestCrossPlatformCoverageRecordAttachmentReadback(t *testing.T) {
	const token = "synthetic-private-file-token"
	for _, command := range []string{"+record-update", "+record-upsert"} {
		for _, tc := range []struct {
			name, actual string
			verified     bool
		}{
			{"same token with metadata", `[{"fileToken":"` + token + `","filename":"report.pdf","resourceId":"resource"}]`, true},
			{"resource projection without token", `[{"filename":"report.pdf","resourceId":"resource"}]`, false},
			{"different token", `[{"fileToken":"another-private-token","filename":"report.pdf"}]`, false},
			{"missing attachment", `[]`, false},
		} {
			t.Run(command+"/"+tc.name, func(t *testing.T) {
				caller := &upsertByKeyCaller{callFn: func(_ int, _, tool string, _ map[string]any) (string, error) {
					switch tool {
					case "update_records", "record_upsert":
						return `{"data":{"updatedRecordIds":["r1"]}}`, nil
					case "query_records":
						return `{"data":{"records":[{"recordId":"r1","cells":{"attachment":` + tc.actual + `}}]}}`, nil
					case "get_fields":
						return `{"data":{"fields":[{"fieldId":"attachment","type":"attachment"}]}}`, nil
					}
					return "", fmt.Errorf("unexpected tool %s", tool)
				}}
				out, err := runAITableCompositeCLI(t, caller, command, "--base-id", "b", "--table-id", "t", "--records", `[{"recordId":"r1","cells":{"attachment":[{"fileToken":"`+token+`"}]}}]`, "--yes")
				writes := 0
				for _, call := range caller.calls {
					if call.tool == "update_records" || call.tool == "record_upsert" {
						writes++
					}
				}
				if writes != 1 {
					t.Fatalf("write replayed: %d writes", writes)
				}
				if tc.verified {
					if err != nil || !strings.Contains(out, `"status": "verified"`) {
						t.Fatalf("stable attachment identity was not verified: %v", err)
					}
					return
				}
				var typed *apperrors.Error
				if !errors.As(err, &typed) {
					t.Fatalf("missing structured unknown: %v", err)
				}
				result := typed.Details["result"].(compositeResult)
				if apperrors.ExitCode(err) != 1 || typed.Retryable || result.Retryable || result.Status != "unknown" || result.FailedCount != 0 || result.Verification["attachmentVerification"] != "unconfirmed" || result.Verification["code"] != "ATTACHMENT_WRITE_UNCONFIRMED" ||
					!strings.Contains(result.NextCommand, "+record-query --base-id b --table-id t --record-ids r1") || !strings.Contains(strings.Join(result.Warnings, " "), "禁止重放") {
					t.Errorf("attachment unknown lacks non-replay recovery: retryable=%v status=%s failed=%d verification=%v next=%s", typed.Retryable, result.Status, result.FailedCount, result.Verification, result.NextCommand)
				}
				raw, marshalErr := json.Marshal(typed)
				if marshalErr != nil || strings.Contains(string(raw), token) || strings.Contains(string(raw), "another-private-token") || strings.Contains(typed.Error(), token) {
					t.Error("attachment diagnostics exposed a file token")
				}
			})
		}
	}
}

func TestCrossPlatformCoverageRecordAttachmentCreatedIDsRemainReadable(t *testing.T) {
	for _, command := range []string{"+record-batch-create", "+record-upsert"} {
		t.Run(command, func(t *testing.T) {
			writes, reads := 0, 0
			var token string
			caller := &upsertByKeyCaller{callFn: func(_ int, _, tool string, args map[string]any) (string, error) {
				switch tool {
				case "create_records", "record_upsert":
					writes++
					token, _ = args["clientToken"].(string)
					return `{"data":{"createdRecordIds":["new"]}}`, nil
				case "query_records":
					reads++
					return "", fmt.Errorf("read-back unavailable")
				}
				return "", fmt.Errorf("unexpected tool %s", tool)
			}}
			_, err := runAITableCompositeCLI(t, caller, command, "--base-id", "b", "--table-id", "t", "--records", `[{"cells":{"attachment":[{"fileToken":"private-created-token"}]}}]`, "--yes")
			var typed *apperrors.Error
			if !errors.As(err, &typed) || writes != 1 || reads != 1 || len(caller.calls) != 2 {
				t.Fatalf("creation must not replay or query recovery automatically: err=%v writes=%d reads=%d", err, writes, reads)
			}
			result := typed.Details["result"].(compositeResult)
			query := aitableRecoveryCommand("dws", "aitable", "+record-query", "--base-id", "b", "--table-id", "t", "--record-ids", "new")
			if typed.Retryable || result.Retryable || result.Status != "unknown" || result.FailedCount != 0 || result.CompletedCount != 0 ||
				result.Verification["attachmentVerification"] != "unconfirmed" || result.Checkpoint["attachmentReadbackCommand"] != query || result.Checkpoint["updateReadbackCommand"] != nil ||
				token == "" || result.Checkpoint["clientToken"] != token || !strings.Contains(result.NextCommand, "+record-write-result") || !strings.Contains(result.NextCommand, "--client-token "+token) {
				t.Fatalf("creation lost separate attachment read-back or original reconciliation: %#v", result)
			}
			raw, _ := json.Marshal(typed)
			if strings.Contains(string(raw), "private-created-token") {
				t.Fatal("recovery exposed attachment credentials")
			}
		})
	}
}

func TestCrossPlatformCoverageRecordAttachmentRecoveryTargetTrust(t *testing.T) {
	const token = "123e4567-e89b-42d3-a456-426614174000"
	for _, tc := range []struct {
		name, sent, received string
		ids                  []string
		wantQuery            bool
	}{
		{"no token", "", "", []string{"new", "new"}, true},
		{"invalid token", "", "invalid", []string{"new"}, true},
		{"missing IDs", token, token, nil, false},
		{"different token", token, "11111111-1111-4111-8111-111111111111", []string{"untrusted"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &upsertByKeyCaller{}
			rt := boundaryRuntime(t, RecordUpsert, map[string]string{"base-id": "b", "table-id": "t"}, nil, caller)
			result := newCompositeResult("record_upsert")
			result.Checkpoint = map[string]any{}
			failure := recordWriteFailure{}
			failure.Error.Details.ClientToken = tc.received
			failure.Error.Details.AttachmentVerification = "unconfirmed"
			failure.Error.Details.AttachmentRecordIDs = tc.ids
			err := recordUpsertReconciliationError(rt, result, compositeStep{}, []map[string]any{{"cells": map[string]any{}}}, 0, tc.sent, fmt.Errorf("unconfirmed"), failure)
			var typed *apperrors.Error
			if !errors.As(err, &typed) || typed.Retryable || len(caller.calls) != 0 {
				t.Fatalf("recovery dispatched a call or allowed replay: %v", err)
			}
			result = typed.Details["result"].(compositeResult)
			query, present := result.Checkpoint["attachmentReadbackCommand"]
			if present != tc.wantQuery {
				t.Fatalf("attachment target trust changed: %#v", result)
			}
			if tc.wantQuery {
				want := aitableRecoveryCommand("dws", "aitable", "+record-query", "--base-id", "b", "--table-id", "t", "--record-ids", "new")
				if query != want || result.NextCommand != want {
					t.Fatalf("known IDs must remain readable without a usable token: %#v", result)
				}
			} else if !strings.Contains(result.NextCommand, "--client-token "+token) || strings.Contains(result.NextCommand, "untrusted") {
				t.Fatalf("missing or mismatched IDs replaced original reconciliation: %#v", result)
			}
		})
	}
}

func TestCrossPlatformCoverageRecordAttachmentIdentityComparison(t *testing.T) {
	for _, tc := range []struct {
		name, actual, expected string
		equal                  bool
	}{
		{"resource metadata", `[{"resourceId":"r","filename":"a"}]`, `[{"resourceId":"r"}]`, true},
		{"reordered", `[{"resourceId":"b"},{"fileToken":"a","filename":"name"}]`, `[{"fileToken":"a"},{"resourceId":"b"}]`, true},
		{"clear null", `null`, `[]`, true},
		{"clear empty", `[]`, `[]`, true},
		{"extra item", `[{"fileToken":"a"},{"fileToken":"b"}]`, `[{"fileToken":"a"}]`, false},
		{"non list", `{}`, `[]`, false},
		{"non expected list", `[]`, `{}`, false},
		{"invalid expected item", `[{}]`, `[1]`, false},
		{"invalid actual item", `[1]`, `[{"fileToken":"a"}]`, false},
		{"reused identity", `[{"fileToken":"a"},{"fileToken":"b"}]`, `[{"fileToken":"a"},{"fileToken":"a"}]`, false},
		{"conflicting identity", `[{"fileToken":"a","resourceId":"wrong"}]`, `[{"fileToken":"a","resourceId":"r"}]`, false},
		{"missing requested identity", `[{"fileToken":"a"}]`, `[{"fileToken":"a","resourceId":"r"}]`, false},
		{"names alone", `[{"filename":"same"}]`, `[{"filename":"same"}]`, false},
		{"url alone", `[{"url":"https://example.test/file"}]`, `[{"url":"https://example.test/file"}]`, false},
		{"blank identity", `[{"fileToken":" "}]`, `[{"fileToken":" "}]`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var actual, expected any
			if err := json.Unmarshal([]byte(tc.actual), &actual); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(tc.expected), &expected); err != nil {
				t.Fatal(err)
			}
			if got := recordCellValueEqual(actual, expected, "attachment"); got != tc.equal {
				t.Fatalf("identity comparison=%v, want %v", got, tc.equal)
			}
		})
	}
	if err := verifyRecordCells(map[string]any{"cells": map[string]any{}}, map[string]any{"attachment": []any{}}, map[string]string{"attachment": "attachment"}); err != nil {
		t.Fatalf("omitted cleared attachment was not verified: %v", err)
	}
}

func TestCrossPlatformCoverageRecordAttachmentReadFailureAndUniqueKey(t *testing.T) {
	for _, command := range []string{"+record-update", "+record-upsert", "+record-upsert-by-key"} {
		for _, existing := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/existing=%v", command, existing), func(t *testing.T) {
				caller := &upsertByKeyCaller{callFn: func(index int, _, tool string, _ map[string]any) (string, error) {
					if command == "+record-upsert-by-key" && index == 0 {
						if existing {
							return `{"data":{"records":[{"recordId":"r1"}]}}`, nil
						}
						return `{"data":{"records":[]}}`, nil
					}
					if tool == "update_records" || tool == "record_upsert" || tool == "create_records" {
						return `{"data":{"recordIds":["r1"]}}`, nil
					}
					return "", fmt.Errorf("read service unavailable")
				}}
				var err error
				if command == "+record-upsert-by-key" {
					_, err = runUpsertByKeyCLI(t, caller, "--cells", `{"attachment":[{"fileToken":"private-canary"}]}`)
				} else {
					_, err = runAITableCompositeCLI(t, caller, command, "--base-id", "b", "--table-id", "t", "--records", `[{"recordId":"r1","cells":{"attachment":[{"fileToken":"private-canary"}]}}]`, "--yes")
				}
				var typed *apperrors.Error
				if !errors.As(err, &typed) || typed.Retryable {
					t.Fatalf("read failure suggested replay: %v", err)
				}
				result := typed.Details["result"].(compositeResult)
				if result.Status != "unknown" || result.FailedCount != 0 || result.NextCommand == "" || result.Verification["attachmentVerification"] != "unconfirmed" {
					t.Fatalf("missing attachment recovery: status=%s failed=%d next=%s", result.Status, result.FailedCount, result.NextCommand)
				}
				raw, _ := json.Marshal(typed)
				if strings.Contains(string(raw), "private-canary") {
					t.Fatal("error retained private input in plan or cause")
				}
			})
		}
	}
}

func TestCrossPlatformCoverageRecordAttachmentUniqueKeyVerified(t *testing.T) {
	for _, existing := range []bool{false, true} {
		for _, verified := range []bool{false, true} {
			preflight, actual := `[]`, `[{"resourceId":"resource","filename":"report.pdf"}]`
			if existing {
				preflight = `[{"recordId":"r1"}]`
			}
			if verified {
				actual = `[{"fileToken":"same","resourceId":"resource"}]`
			}
			caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
				{text: `{"data":{"records":` + preflight + `}}`},
				{text: `{"data":{"recordIds":["r1"]}}`},
				{text: `{"data":{"records":[{"recordId":"r1","cells":{"fldKey":"TASK-1","attachment":` + actual + `}}]}}`},
				{text: `{"data":{"fields":[{"fieldId":"attachment","type":"attachment"},{"fieldId":"fldKey","type":"text"}]}}`},
			}}
			out, err := runUpsertByKeyCLI(t, caller, "--cells", `{"attachment":[{"fileToken":"same"}]}`)
			if verified {
				if err != nil || len(caller.calls) != 4 || !strings.Contains(out, `"attachmentVerification": "verified"`) {
					t.Fatalf("unique-key stable attachment identity was not verified: %v", err)
				}
				continue
			}
			var typed *apperrors.Error
			if !errors.As(err, &typed) || typed.Retryable {
				t.Fatalf("unconfirmed unique-key attachment lost protection: %v", err)
			}
			result := typed.Details["result"].(compositeResult)
			if result.Checkpoint["recordId"] != "r1" || result.Verification["attachmentVerification"] != "unconfirmed" || result.FailedCount != 0 || len(caller.calls) != 4 {
				t.Fatalf("unique-key attachment lost read-back ID or made extra calls: %#v", result)
			}
		}
	}
}

func TestCrossPlatformCoverageRecordAttachmentMixedRecoveryAndRejection(t *testing.T) {
	const token = "123e4567-e89b-42d3-a456-426614174000"
	for _, rejected := range []bool{false, true} {
		caller := &upsertByKeyCaller{callFn: func(index int, _, tool string, _ map[string]any) (string, error) {
			if index == 0 {
				if rejected {
					return `{"status":"error","error":{"type":"INPUT_ERROR","code":"INVALID_ATTACHMENT","retryable":false}}`, nil
				}
				return `{"data":{"createdRecordIds":["new"],"updatedRecordIds":["r1"]}}`, nil
			}
			return "", fmt.Errorf("read-back unavailable")
		}}
		_, err := runAITableCompositeCLI(t, caller, "+record-upsert", "--base-id", "b", "--table-id", "t", "--records",
			`[{"recordId":"r1","cells":{"attachment":[{"fileToken":"private"}]}},{"cells":{"attachment":[{"fileToken":"private-new"}]}}]`, "--client-token", token, "--yes")
		var typed *apperrors.Error
		if !errors.As(err, &typed) || typed.Retryable {
			t.Fatalf("attachment recovery suggested replay: %v", err)
		}
		result := typed.Details["result"].(compositeResult)
		if rejected {
			if len(caller.calls) != 1 || result.Status != "failed" || result.FailedCount != 2 {
				t.Fatalf("definite rejection was recovered: status=%s failed=%d calls=%d", result.Status, result.FailedCount, len(caller.calls))
			}
			continue
		}
		if len(caller.calls) != 2 || result.Status != "unknown" || result.FailedCount != 0 || result.Checkpoint["clientToken"] != token ||
			!strings.Contains(result.NextCommand, "+record-write-result") || !strings.Contains(result.NextCommand, token) ||
			!strings.Contains(result.Checkpoint["updateReadbackCommand"].(string), "--record-ids r1") ||
			result.Checkpoint["attachmentReadbackCommand"] != aitableRecoveryCommand("dws", "aitable", "+record-query", "--base-id", "b", "--table-id", "t", "--record-ids", "r1,new") {
			t.Fatalf("mixed attachment recovery lost original targets: %#v", result)
		}
	}
	resolver := resolvedRecordFieldTypeResolver([]map[string]any{{"fieldId": "f", "type": "text"}})
	cells := map[string]any{"f": []any{map[string]any{"fileToken": "same"}}}
	if err := resolver.verify(map[string]any{"cells": cells}, cells); err == nil {
		t.Fatal("an unproven attachment field was marked verified")
	}
}

func TestCrossPlatformCoverageRecordAttachmentClearReadFailure(t *testing.T) {
	for _, command := range []string{"+record-update", "+record-upsert", "+record-upsert-by-key"} {
		for _, fieldType := range []string{"attachment", "multipleSelect", "", "unavailable", "unresolved"} {
			for _, selector := range []string{"attachment", "附件"} {
				t.Run(command+"/"+fieldType+"/"+selector, func(t *testing.T) {
					reads, writes, metadata := 0, 0, 0
					caller := &upsertByKeyCaller{callFn: func(_ int, _, tool string, _ map[string]any) (string, error) {
						switch tool {
						case "query_records":
							reads++
							if command == "+record-upsert-by-key" && writes == 0 {
								return `{"data":{"records":[{"recordId":"r1"}]}}`, nil
							}
							return "", fmt.Errorf("record read unavailable")
						case "update_records", "record_upsert":
							writes++
							return `{"data":{"updatedRecordIds":["r1"]}}`, nil
						case "get_fields":
							metadata++
							if fieldType == "unavailable" {
								return "", fmt.Errorf("field metadata unavailable")
							}
							if fieldType == "unresolved" {
								return `{"data":{"fields":[{"fieldId":"other","fieldName":"other","type":"text"}]}}`, nil
							}
							return fmt.Sprintf(`{"data":{"fields":[{"fieldId":"attachment","fieldName":"附件","type":%q}]}}`, fieldType), nil
						}
						return "", fmt.Errorf("unexpected tool %s", tool)
					}}
					cells := map[string]any{selector: []any{}}
					var err error
					if command == "+record-upsert-by-key" {
						raw, _ := json.Marshal(cells)
						_, err = runUpsertByKeyCLI(t, caller, "--cells", string(raw))
					} else {
						raw, _ := json.Marshal([]map[string]any{{"recordId": "r1", "cells": cells}})
						_, err = runAITableCompositeCLI(t, caller, command, "--base-id", "b", "--table-id", "t", "--records", string(raw), "--yes")
					}
					var typed *apperrors.Error
					if !errors.As(err, &typed) || writes != 1 {
						t.Fatalf("missing error or replayed clear: err=%v writes=%d", err, writes)
					}
					result := typed.Details["result"].(compositeResult)
					if fieldType == "multipleSelect" {
						if !typed.Retryable || result.Verification["attachmentVerification"] != nil {
							t.Fatal("a proven multi-select clear was classified as an attachment clear")
						}
						return
					}
					if typed.Retryable || result.Retryable || result.Status != "unknown" || result.FailedCount != 0 || result.Verification["attachmentVerification"] != "unconfirmed" ||
						result.Verification["code"] != "ATTACHMENT_WRITE_UNCONFIRMED" || !strings.Contains(result.NextCommand, "+record-query") || !strings.Contains(result.NextCommand, "--record-ids r1") {
						t.Fatalf("unconfirmed clear lost non-replay recovery: retryable=%v status=%s failed=%d", typed.Retryable, result.Status, result.FailedCount)
					}
					if metadata != 1 || reads != 1+boolCount(command == "+record-upsert-by-key") {
						t.Fatalf("clear verification calls: reads=%d metadata=%d", reads, metadata)
					}
					encoded, _ := json.Marshal(result)
					var wire map[string]any
					if json.Unmarshal(encoded, &wire) != nil {
						t.Fatal("invalid composite JSON")
					}
					if _, exists := wire["failedCount"]; exists {
						t.Fatal("zero failedCount must retain its existing omitempty wire contract")
					}
				})
			}
		}
	}
}
