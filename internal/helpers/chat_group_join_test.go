package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
)

// TC-001: 三命令只调用一次对应 IM 工具，并保留参数类型及邀请链接原文。
func TestCrossPlatformCoverageChatGroupJoinRoutes(t *testing.T) {
	const inviteURL = "https://qr.dingtalk.com/action/joingroup?code=CODE1"
	for _, tc := range []struct {
		command, flag, tool string
		want                map[string]any
	}{
		{"join-by-group-id", "--group-id=123456", "join_group_by_group_id", map[string]any{"groupId": int64(123456)}},
		{"get-info-by-invite-url", "--invite-url=" + inviteURL, "get_group_info_by_invite_url", map[string]any{"inviteUrl": inviteURL}},
		{"join-by-invite-url", "--invite-url=" + inviteURL, "join_group_by_invite_url", map[string]any{"inviteUrl": inviteURL}},
	} {
		t.Run(tc.command, func(t *testing.T) {
			caller := &scriptedToolCaller{}
			if err := runChatCoverageCommand(t, caller, "group", tc.command, tc.flag); err != nil {
				t.Fatal(err)
			}
			if caller.calls != 1 || caller.server != "im" || caller.tool != tc.tool || !reflect.DeepEqual(caller.args, tc.want) {
				t.Fatalf("call = count:%d server:%q tool:%q args:%#v, want %#v", caller.calls, caller.server, caller.tool, caller.args, tc.want)
			}
		})
	}
}

// TC-002: 缺失、空值、非正整数及溢出群号均为 validation/3，且不调用服务端。
func TestCrossPlatformCoverageChatGroupJoinByGroupIdInvalidInputMakesNoCall(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "missing group-id", args: nil},
		{name: "empty group-id", args: []string{"--group-id="}},
		{name: "non numeric group-id", args: []string{"--group-id=abc"}},
		{name: "zero group-id", args: []string{"--group-id=0"}},
		{name: "negative group-id", args: []string{"--group-id=-7"}},
		{name: "overflow group-id", args: []string{"--group-id=9223372036854775808"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &scriptedToolCaller{}
			err := runChatCoverageCommand(t, caller, append([]string{"group", "join-by-group-id"}, tc.args...)...)
			var typed *apperrors.Error
			if !errors.As(err, &typed) || typed.Category != apperrors.CategoryValidation || typed.ExitCode() != 3 {
				t.Fatalf("args %v error = %v, want validation/3", tc.args, err)
			}
			if caller.calls != 0 {
				t.Fatalf("args %v made %d calls", tc.args, caller.calls)
			}
		})
	}
}

// TC-003: 两个邀请链接命令拒绝缺参及显式空值，且不调用服务端。
func TestCrossPlatformCoverageChatGroupInviteUrlCommandsMissingUrlMakeNoCall(t *testing.T) {
	for _, command := range []string{
		"get-info-by-invite-url", "get-info-by-invite-url --invite-url=",
		"join-by-invite-url", "join-by-invite-url --invite-url=",
	} {
		t.Run(command, func(t *testing.T) {
			caller := &scriptedToolCaller{}
			path := append([]string{"group"}, strings.Fields(command)...)
			err := runChatCoverageCommand(t, caller, path...)
			if got := apperrors.ExitCode(err); got != 3 {
				t.Fatalf("%v exit = %d, want validation/3: %v", path, got, err)
			}
			if caller.calls != 0 {
				t.Fatalf("%v made %d calls", path, caller.calls)
			}
		})
	}
}

// TC-004: 会话 ID 的选路必须指向接受字符串 ID 的命令，且真实执行仍透传该 ID。
func TestCrossPlatformCoverageChatGroupInviteSelectionRoutesConversationID(t *testing.T) {
	root := newChatCommand()
	leaf, _, err := root.Find([]string{"group", "get-info-by-invite-url"})
	if err != nil {
		t.Fatal(err)
	}
	final, ok := contractfinal.RuntimeContractFinal(leaf)
	if !ok || final.Selection == nil {
		t.Fatal("missing invite query selection")
	}
	avoid := strings.Join(final.Selection.AvoidWhen, "\n")
	if !strings.Contains(avoid, "chat conversation-info --conversation-id") || strings.Contains(avoid, "chat group get-by-group-id") {
		t.Fatalf("conversation ID selection points to an incompatible command: %s", avoid)
	}
	const conversationID = "cidConversationForTest"
	caller := &scriptedToolCaller{}
	if err := runChatCoverageCommand(t, caller, "conversation-info", "--conversation-id", conversationID); err != nil {
		t.Fatal(err)
	}
	if caller.calls != 1 || caller.args["openConversationId"] != conversationID {
		t.Fatalf("conversation query = calls:%d args:%#v", caller.calls, caller.args)
	}
}

// TC-005: 三命令在统一输出下产出 ok/outcome/data 信封，data 为服务端业务负载原文。
func TestCrossPlatformCoverageChatGroupJoinUnifiedEnvelopeDelivery(t *testing.T) {
	const inviteURL = "https://qr.dingtalk.com/action/joingroup?code=CODE1"
	for _, tc := range []struct {
		name     string
		tool     string
		response string
		args     []string
		wantData map[string]any
	}{
		{
			name:     "join-by-group-id",
			tool:     "join_group_by_group_id",
			response: `{"success":true,"result":{"openConversationId":"cidGroup1","status":"JOINED","isMember":true,"alreadyMember":false},"errorCode":null,"errorMsg":null,"arguments":[]}`,
			args:     []string{"group", "join-by-group-id", "--group-id", "123456"},
			wantData: map[string]any{"success": true, "result": map[string]any{"openConversationId": "cidGroup1", "status": "JOINED", "isMember": true, "alreadyMember": false}, "errorCode": nil, "errorMsg": nil, "arguments": []any{}},
		},
		{
			name:     "get-info-by-invite-url",
			tool:     "get_group_info_by_invite_url",
			response: `{"success":true,"result":{"groupInfo":{"openConversationId":"cidGroup1","title":"群名","memberCount":12},"isMember":null},"errorCode":null,"errorMsg":null,"arguments":[]}`,
			args:     []string{"group", "get-info-by-invite-url", "--invite-url", inviteURL},
			wantData: map[string]any{"success": true, "result": map[string]any{"groupInfo": map[string]any{"openConversationId": "cidGroup1", "title": "群名", "memberCount": float64(12)}, "isMember": nil}, "errorCode": nil, "errorMsg": nil, "arguments": []any{}},
		},
		{
			name:     "join-by-invite-url",
			tool:     "join_group_by_invite_url",
			response: `{"success":true,"result":{"openConversationId":"cidGroup1","status":"ALREADY_MEMBER"},"errorCode":null,"errorMsg":null,"arguments":[]}`,
			args:     []string{"group", "join-by-invite-url", "--invite-url", inviteURL},
			wantData: map[string]any{"success": true, "result": map[string]any{"openConversationId": "cidGroup1", "status": "ALREADY_MEMBER"}, "errorCode": nil, "errorMsg": nil, "arguments": []any{}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &chatThreadCaller{responses: map[string]string{"im/" + tc.tool: tc.response}}
			stdout, err := executeAtomicThreadCommandOutput(t, caller, tc.args...)
			if err != nil {
				t.Fatal(err)
			}
			var envelope map[string]any
			if err := json.Unmarshal(stdout, &envelope); err != nil {
				t.Fatalf("output is not one JSON envelope: %q: %v", stdout, err)
			}
			if envelope["ok"] != true || envelope["outcome"] != "success" {
				t.Fatalf("envelope = %#v", envelope)
			}
			if !reflect.DeepEqual(envelope["data"], tc.wantData) {
				t.Fatalf("data = %#v, want %#v", envelope["data"], tc.wantData)
			}
			if len(caller.calls) != 1 || caller.calls[0].product != "im" || caller.calls[0].tool != tc.tool {
				t.Fatalf("calls = %#v", caller.calls)
			}
		})
	}
}

// TC-006: 生产 runner 的 RESULT_UNKNOWN 类型化错误保留分类及诊断，
// 不得根据文本 parser 的行为误判为成功或丢失原始服务端码。
func TestCrossPlatformCoverageChatGroupJoinBusinessErrorPassthrough(t *testing.T) {
	original := apperrors.NewAPI("写入后结果暂未确认",
		apperrors.WithServerKey("im"), apperrors.WithReason("business_error"),
		apperrors.WithServerDiag(apperrors.ServerDiagnostics{ServerErrorCode: "RESULT_UNKNOWN", TraceID: "join-trace"}))
	caller := &chatThreadCaller{errors: map[string]error{
		"im/join_group_by_group_id": original,
	}}
	_, err := executeAtomicThreadCommandOutput(t, caller, "group", "join-by-group-id", "--group-id", "123456")
	var typed *apperrors.Error
	if err != original || !errors.As(err, &typed) || typed.ServerDiag.ServerErrorCode != "RESULT_UNKNOWN" || typed.ServerDiag.TraceID != "join-trace" || typed.Reason != "business_error" {
		t.Fatalf("production business error must be preserved: %#v", err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("unknown write result must never be replayed: %#v", caller.calls)
	}
}

// TC-007: 输出契约回滚到 legacy 时，三命令仍走直读打印路径并各调用一次对应 IM 工具。
func TestCrossPlatformCoverageChatGroupJoinLegacyRolloutPrintPath(t *testing.T) {
	const inviteURL = "https://qr.dingtalk.com/action/joingroup?code=CODE1"
	for _, tc := range []struct {
		command, flag, tool string
	}{
		{"join-by-group-id", "--group-id=123456", "join_group_by_group_id"},
		{"get-info-by-invite-url", "--invite-url=" + inviteURL, "get_group_info_by_invite_url"},
		{"join-by-invite-url", "--invite-url=" + inviteURL, "join_group_by_invite_url"},
	} {
		t.Run(tc.command, func(t *testing.T) {
			caller := &scriptedToolCaller{}
			InitDeps(caller)
			deps.Out.w = io.Discard
			deps.Out.errW = io.Discard
			root := newChatCommand()
			installExampleGlobalFlags(root)
			root.PersistentFlags().Bool("debug", false, "")
			root.PersistentFlags().Bool("verbose", false, "")
			root.SilenceErrors = true
			root.SilenceUsage = true
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			root.SetArgs([]string{"group", tc.command, tc.flag, "--yes"})
			leaf, _, err := root.Find([]string{"group", tc.command})
			if err != nil {
				t.Fatal(err)
			}
			output.SetCommandRollout(leaf, output.RolloutLegacyOnly)
			ctx, _ := output.WithResultStore(context.Background())
			if _, err := corecmd.ExecuteContextCForTest(root, ctx); err != nil {
				t.Fatal(err)
			}
			if caller.calls != 1 || caller.server != "im" || caller.tool != tc.tool {
				t.Fatalf("legacy call = count:%d server:%q tool:%q", caller.calls, caller.server, caller.tool)
			}
		})
	}
}
