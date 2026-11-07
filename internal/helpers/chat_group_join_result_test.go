// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func TestCrossPlatformCoverageChatGroupJoinSubmittedRunnerError(t *testing.T) {
	const inviteURL = "https://qr.dingtalk.com/action/joingroup?code=TEST_CODE&origin=11"
	for _, tc := range []struct {
		command, tool string
		args          []string
	}{
		{"join-by-group-id", "join_group_by_group_id", []string{"--group-id", "123456"}},
		{"join-by-invite-url", "join_group_by_invite_url", []string{"--invite-url", inviteURL}},
	} {
		for _, reason := range []string{"business_error", "mcp_tool_error"} {
			t.Run(tc.command+"/"+reason, func(t *testing.T) {
				message := "成员邀请已发送"
				if reason == "mcp_tool_error" {
					message = "服务端业务错误"
				}
				original := apperrors.NewAPI(message,
					apperrors.WithServerKey("im"), apperrors.WithReason(reason),
					apperrors.WithHint("Check required parameters and values"),
					apperrors.WithRetryable(true),
					apperrors.WithServerDiag(apperrors.ServerDiagnostics{
						ServerErrorCode: "400001", TraceID: "join-trace", TechnicalDetail: "成员邀请已发送",
					})).(*apperrors.Error)
				after, next := int64(1), time.Now().Add(time.Second)
				original.RetryAfterSeconds, original.NextRetryAt = &after, &next
				original.Actions = []string{"dws chat group join-by-group-id --group-id 123456"}
				caller := &chatThreadCaller{errors: map[string]error{"im/" + tc.tool: original}}
				stdout, err := executeAtomicThreadCommandOutput(t, caller, append([]string{"group", tc.command}, tc.args...)...)
				var projected *apperrors.Error
				if !errors.As(err, &projected) || !errors.Is(err, original) || projected == original || len(stdout) != 0 {
					t.Fatalf("expected preserved typed failure, got stdout=%q err=%#v", stdout, err)
				}
				if projected.ExitCode() != 1 || projected.Category != original.Category || projected.Reason != "group_join_submitted" || !reflect.DeepEqual(projected.ServerDiag, original.ServerDiag) {
					t.Fatalf("classification/diagnostic lost: %#v", projected)
				}
				if projected.Retryable || !projected.RetryableSet || projected.RetryAfterSeconds != nil || projected.NextRetryAt != nil {
					t.Fatalf("submitted write must not advertise retries: %#v", projected)
				}
				for _, text := range []string{"已提交", "等待", "尚未确认入群", "不要重复提交", "只读"} {
					if !strings.Contains(projected.Hint, text) {
						t.Fatalf("hint %q missing %q", projected.Hint, text)
					}
				}
				if strings.Contains(projected.Hint, "Check required") || strings.Contains(projected.Hint, inviteURL) || strings.Contains(projected.Hint, "TEST_CODE") {
					t.Fatalf("generic hint or invitation leaked: %q", projected.Hint)
				}
				if tc.command == "join-by-group-id" && !reflect.DeepEqual(projected.Actions, []string{"dws chat group get-by-group-id --group-id 123456 --format json"}) {
					t.Fatalf("expected only a read recovery action: %#v", projected.Actions)
				}
				if tc.command == "join-by-invite-url" && len(projected.Actions) != 0 {
					t.Fatalf("no credential-bearing recovery action: %#v", projected.Actions)
				}
				if original.Hint != "Check required parameters and values" || !original.Retryable || len(caller.calls) != 1 {
					t.Fatalf("original error mutated or write replayed: %#v calls=%#v", original, caller.calls)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageChatGroupJoinUnrelatedErrorsPreserved(t *testing.T) {
	api := func(code, message, reason, server string) error {
		return apperrors.NewAPI(message, apperrors.WithServerKey(server), apperrors.WithReason(reason),
			apperrors.WithServerDiag(apperrors.ServerDiagnostics{ServerErrorCode: code}))
	}
	for _, tc := range []struct {
		name, tool string
		err        error
	}{
		{"unknown result", "join_group_by_group_id", api("RESULT_UNKNOWN", "结果未知", "business_error", "im")},
		{"different code", "join_group_by_group_id", api("16003", "成员邀请已发送", "business_error", "im")},
		{"different meaning", "join_group_by_group_id", api("400001", "参数无效", "business_error", "im")},
		{"different reason", "join_group_by_group_id", api("400001", "成员邀请已发送", "network_error", "im")},
		{"different server", "join_group_by_group_id", api("400001", "成员邀请已发送", "business_error", "other")},
		{"unverified applied error code", "join_group_by_group_id", api("APPLIED", "已提交", "business_error", "im")},
		{"read preview", "get_group_info_by_invite_url", api("400001", "成员邀请已发送", "business_error", "im")},
		{"untyped error", "join_group_by_group_id", errors.New("400001 成员邀请已发送")},
		{"authentication", "join_group_by_group_id", &apperrors.Error{Category: apperrors.CategoryAuth, Message: "成员邀请已发送", Reason: "business_error", ServerKey: "im", ServerDiag: apperrors.ServerDiagnostics{ServerErrorCode: "400001"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := chatGroupJoinCommandResult(tc.tool, nil, nil, tc.err)
			if result != nil || err != tc.err {
				t.Fatalf("unrelated failure was reclassified: result=%#v err=%#v", result, err)
			}
		})
	}
}

func TestCrossPlatformCoverageChatGroupJoinAppliedPending(t *testing.T) {
	const response = `{"success":true,"result":{"status":"APPLIED","openConversationId":"cidPending","isMember":false},"errorCode":null,"errorMsg":null,"arguments":[]}`
	for _, tc := range []struct {
		command, tool, next string
		args                []string
	}{
		{"join-by-group-id", "join_group_by_group_id", "dws chat group get-by-group-id --group-id 123456 --format json", []string{"--group-id", "123456"}},
		{"join-by-invite-url", "join_group_by_invite_url", "dws chat conversation-info --conversation-id cidPending --format json", []string{"--invite-url", "https://qr.dingtalk.com/action/joingroup?code=TEST_CODE"}},
	} {
		t.Run(tc.command, func(t *testing.T) {
			caller := &chatThreadCaller{responses: map[string]string{"im/" + tc.tool: response}}
			stdout, err := executeAtomicThreadCommandOutput(t, caller, append([]string{"group", tc.command}, tc.args...)...)
			if err != nil {
				t.Fatal(err)
			}
			var envelope, expected map[string]any
			if json.Unmarshal(stdout, &envelope) != nil || json.Unmarshal([]byte(response), &expected) != nil {
				t.Fatalf("invalid JSON: %s", stdout)
			}
			if envelope["ok"] != true || envelope["outcome"] != "pending" || !reflect.DeepEqual(envelope["data"], expected) || envelope["error"] != nil {
				t.Fatalf("pending must preserve the real data without inventing membership: %#v", envelope)
			}
			op := envelope["meta"].(map[string]any)["operation"].(map[string]any)
			if op["id"] != "cidPending" || op["state"] != "APPLIED" || op["next_command"] != tc.next || strings.Contains(string(stdout), "TEST_CODE") || len(caller.calls) != 1 {
				t.Fatalf("invalid recovery or extra request: output=%s calls=%#v", stdout, caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageChatGroupJoinAppliedWithoutSafeRecovery(t *testing.T) {
	for _, tc := range []struct {
		name, response, reason string
	}{
		{"missing CID", `{"success":true,"result":{"status":"APPLIED"}}`, "group_join_submitted"},
		{"blank CID", `{"success":true,"result":{"status":"APPLIED","openConversationId":" "}}`, "group_join_submitted"},
		{"unsafe CID", `{"success":true,"result":{"status":"APPLIED","openConversationId":"cid;echo side-effect"}}`, "group_join_submitted"},
		{"contradictory member", `{"success":true,"result":{"status":"APPLIED","openConversationId":"cidPending","isMember":true}}`, "group_join_result_unknown"},
		{"contradictory prior member", `{"success":true,"result":{"status":"APPLIED","openConversationId":"cidPending","alreadyMember":true}}`, "group_join_result_unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &chatThreadCaller{responses: map[string]string{"im/join_group_by_group_id": tc.response}}
			stdout, err := executeAtomicThreadCommandOutput(t, caller, "group", "join-by-group-id", "--group-id", "123456")
			var typed *apperrors.Error
			if !errors.As(err, &typed) || typed.Reason != tc.reason || typed.Retryable || !strings.Contains(typed.Hint, "不要重复提交") || len(stdout) != 0 || len(caller.calls) != 1 {
				t.Fatalf("must fail without fake pending metadata or replay: stdout=%q err=%#v calls=%#v", stdout, err, caller.calls)
			}
		})
	}
}
