// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package helpers

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageDingConversionReceiptFailureMatrix(t *testing.T) {
	for _, tc := range []struct {
		name, response, reason, message string
	}{
		{"invalid JSON", `{`, "malformed_conversion_receipt", ""},
		{"null envelope", `null`, "malformed_conversion_receipt", ""},
		{"missing status", `{}`, "conversion_failed", ""},
		{"false status", `{"success":false}`, "conversion_failed", ""},
		{"wrong status type", `{"success":"true"}`, "conversion_failed", ""},
		{"primary error", `{"success":false,"errorMsg":" refused ","message":"other"}`, "conversion_failed", "refused"},
		{"fallback error", `{"success":false,"errorMsg":" ","errorMessage":" rejected "}`, "conversion_failed", "rejected"},
		{"message error", `{"success":false,"errorMsg":1,"message":" denied "}`, "conversion_failed", "denied"},
		{"missing result", `{"success":true}`, "missing_conversion_receipt", ""},
		{"null result", `{"success":true,"result":null}`, "missing_conversion_receipt", ""},
		{"wrong result type", `{"success":true,"result":[]}`, "missing_conversion_receipt", ""},
		{"missing identity", `{"success":true,"result":{}}`, "missing_ding_identity", ""},
		{"wrong identity type", `{"success":true,"result":{"openDingId":123}}`, "missing_ding_identity", ""},
		{"blank identity", `{"success":true,"result":{"openDingId":" \t"}}`, "missing_ding_identity", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := enrichDingConversionReceipt(tc.response, "conversation", "source-message")
			var failure *apperrors.Error
			if got != "" || !errors.As(err, &failure) || failure.Reason != tc.reason || failure.Operation != "im/send_ding_by_message" || failure.FailureStage != "response_validation" {
				t.Fatalf("unverified conversion returned receipt or lost failure: output=%q err=%#v", got, err)
			}
			if tc.message != "" && failure.Message != tc.message {
				t.Fatalf("lost server error: %q", failure.Message)
			}
			if tc.reason != "conversion_failed" && (!failure.RetryableSet || failure.Retryable) {
				t.Fatalf("unverified receipt permits replay: %#v", failure)
			}
		})
	}
}

func TestCrossPlatformCoverageDingConversionCommandSingleCall(t *testing.T) {
	const tool = "send_ding_by_message"
	const valid = `{"success":true,"result":{"openDingId":"msgOpaque+/==","extra":{"kept":true}}}`
	for _, tc := range []struct {
		name, response string
		transport      error
		dryRun, fail   bool
	}{
		{"success", valid, nil, false, false},
		{"dry run", valid, nil, true, false},
		{"transport failure", "", errors.New("transport failed"), false, true},
		{"business rejection", `{"success":false,"message":"rejected"}`, nil, false, true},
		{"missing receipt", `{"success":true,"result":{}}`, nil, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &imReadResultCaller{
				responses: map[string]string{tool: tc.response}, errors: map[string]error{tool: tc.transport}, dryRun: tc.dryRun,
			}
			got, err := executeIMReadCommand(t, caller, []string{"dws", "ding"}, newDingCommand,
				"message", "send-by-message", "--group", "conversation", "--message-id", "source-message", "--users", "user1,user2", "--type", "call", "--uuid", "stable-uuid")
			if tc.dryRun {
				if err != nil || len(caller.calls) != 0 {
					t.Fatalf("dry run crossed RPC boundary: err=%v calls=%v", err, caller.calls)
				}
				return
			}
			if (err != nil) != tc.fail || len(caller.calls) != 1 || caller.calls[0].toolName != tool || caller.calls[0].productID != "im" {
				t.Fatalf("conversion failed or replayed: err=%v calls=%v", err, caller.calls)
			}
			wantArgs := map[string]any{
				"openConversationId": "conversation", "openMessageId": "source-message", "receiverOpenDingTalkIds": []string{"user1", "user2"}, "remindType": "PHONE", "uuid": "stable-uuid",
			}
			if !reflect.DeepEqual(caller.args, wantArgs) {
				t.Fatalf("changed message identity or payload: %#v", caller.args)
			}
			if tc.fail {
				if got != "" {
					t.Fatalf("failure emitted success payload: %s", got)
				}
				return
			}
			if !tc.dryRun {
				var envelope struct {
					Result map[string]any `json:"result"`
				}
				if err := json.Unmarshal([]byte(got), &envelope); err != nil {
					t.Fatal(err)
				}
				result := envelope.Result
				if result["sourceMessageId"] != "source-message" || result["sourceConversationId"] != "conversation" || result["openDingId"] != "msgOpaque+/==" || result["extra"].(map[string]any)["kept"] != true || result["recallTarget"].(map[string]any)["openDingId"] != "msgOpaque+/==" {
					t.Fatalf("conversion lost business fields or confused identities: %s", got)
				}
			}
		})
	}
}

func TestCrossPlatformCoverageDingConversionReceiptEncodingFailure(t *testing.T) {
	testseam.Swap(t, &dingReceiptMarshal, func(any) ([]byte, error) {
		return nil, errors.New("receipt encoder failed")
	})
	got, err := enrichDingConversionReceipt(`{"success":true,"result":{"openDingId":"ding-id"}}`, "conversation", "source-message")
	var failure *apperrors.Error
	if got != "" || !errors.As(err, &failure) || failure.Category != apperrors.CategoryInternal {
		t.Fatalf("encoding failure was hidden or emitted partial receipt: got=%q err=%#v", got, err)
	}
}
