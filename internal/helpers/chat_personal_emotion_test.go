package helpers

import (
	"context"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type personalEmotionCall struct {
	server string
	tool   string
	args   map[string]any
}

type personalEmotionCaller struct {
	calls []personalEmotionCall
}

func (c *personalEmotionCaller) CallTool(_ context.Context, server, tool string, args map[string]any) (*edition.ToolResult, error) {
	copied := make(map[string]any, len(args))
	for key, value := range args {
		copied[key] = value
	}
	c.calls = append(c.calls, personalEmotionCall{server: server, tool: tool, args: copied})
	if server == "contact" && tool == "get_user_info_by_user_ids" {
		return textToolResult(`{"result":[{"userId":"u1","openDingTalkId":"` + helperCurrentDOpenID2 + `"}]}`), nil
	}
	return textToolResult(`{"ok":true}`), nil
}

func (*personalEmotionCaller) Format() string { return "json" }
func (*personalEmotionCaller) DryRun() bool   { return false }
func (*personalEmotionCaller) Fields() string { return "" }
func (*personalEmotionCaller) JQ() string     { return "" }

func executePersonalEmotionCommand(t *testing.T, caller *personalEmotionCaller, args ...string) error {
	t.Helper()
	installHelpersCoreDeps(t, caller)
	deps.Out.w = io.Discard
	deps.Out.errW = io.Discard
	root := newChatCommand()
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	return root.ExecuteContext(context.Background())
}

func requirePersonalEmotionCall(t *testing.T, caller *personalEmotionCaller, tool string, want map[string]any) {
	t.Helper()
	if len(caller.calls) != 1 {
		t.Fatalf("calls = %d, want 1: %+v", len(caller.calls), caller.calls)
	}
	call := caller.calls[0]
	if call.server != "im" || call.tool != tool {
		t.Fatalf("tool call = %s/%s, want im/%s", call.server, call.tool, tool)
	}
	if !reflect.DeepEqual(call.args, want) {
		t.Fatalf("args = %#v, want %#v", call.args, want)
	}
}

func TestChatEmotionListCallsIMToolWithoutBusinessArgs(t *testing.T) {
	// TC-001: list 无业务参数，当前用户身份由 MCP server 注入。
	caller := &personalEmotionCaller{}
	if err := executePersonalEmotionCommand(t, caller, "emotion", "list"); err != nil {
		t.Fatalf("chat emotion list returned error: %v", err)
	}
	requirePersonalEmotionCall(t, caller, "list_personal_emotions", map[string]any{})
}

func TestChatEmotionSendMapsGroupTargetAndIdempotency(t *testing.T) {
	// TC-002: 群聊目标映射为 openConversationId，uuid 与表情字段按 MCP 字段透传。
	caller := &personalEmotionCaller{}
	err := executePersonalEmotionCommand(t, caller,
		"emotion", "send",
		"--media-id", "@media",
		"--emotion-id", "emotion123",
		"--group", "cid123",
		"--idempotency-key", "idem-001",
	)
	if err != nil {
		t.Fatalf("chat emotion send returned error: %v", err)
	}
	requirePersonalEmotionCall(t, caller, "send_personal_emotion", map[string]any{
		"mediaId":            "@media",
		"emotionId":          "emotion123",
		"openConversationId": "cid123",
		"uuid":               "idem-001",
	})
}

func TestChatEmotionSendMapsOpenDingTalkTarget(t *testing.T) {
	// TC-003: 已知 openDingTalkId 时直传 receiverOpenDingTalkId，不做外部解析。
	caller := &personalEmotionCaller{}
	err := executePersonalEmotionCommand(t, caller,
		"emotion", "send",
		"--media-id", "@media",
		"--open-dingtalk-id", helperCurrentDOpenID,
	)
	if err != nil {
		t.Fatalf("chat emotion send returned error: %v", err)
	}
	requirePersonalEmotionCall(t, caller, "send_personal_emotion", map[string]any{
		"mediaId":                "@media",
		"receiverOpenDingTalkId": helperCurrentDOpenID,
	})
}

func TestChatEmotionSendTreatsOpenDingTalkIDPassedAsUserAsResolvedTarget(t *testing.T) {
	// TC-004: --user 收到 openDingTalkId 形态时保持 chat message send 的兼容语义。
	caller := &personalEmotionCaller{}
	err := executePersonalEmotionCommand(t, caller,
		"emotion", "send",
		"--media-id", "@media",
		"--user", helperCurrentDOpenID,
	)
	if err != nil {
		t.Fatalf("chat emotion send returned error: %v", err)
	}
	requirePersonalEmotionCall(t, caller, "send_personal_emotion", map[string]any{
		"mediaId":                "@media",
		"receiverOpenDingTalkId": helperCurrentDOpenID,
	})
}

func TestChatEmotionSendResolvesUserIDTarget(t *testing.T) {
	// TC-004b: --user 收到普通 userId 时先解析为 openDingTalkId，再发送个人收藏表情。
	caller := &personalEmotionCaller{}
	err := executePersonalEmotionCommand(t, caller,
		"emotion", "send",
		"--media-id", "@media",
		"--user", "u1",
	)
	if err != nil {
		t.Fatalf("chat emotion send returned error: %v", err)
	}
	if len(caller.calls) != 2 {
		t.Fatalf("calls = %d, want contact resolve then send: %+v", len(caller.calls), caller.calls)
	}
	resolveCall := caller.calls[0]
	if resolveCall.server != "contact" || resolveCall.tool != "get_user_info_by_user_ids" {
		t.Fatalf("resolve call = %s/%s, want contact/get_user_info_by_user_ids", resolveCall.server, resolveCall.tool)
	}
	sendCall := caller.calls[1]
	if sendCall.server != "im" || sendCall.tool != "send_personal_emotion" {
		t.Fatalf("send call = %s/%s, want im/send_personal_emotion", sendCall.server, sendCall.tool)
	}
	want := map[string]any{
		"mediaId":                "@media",
		"receiverOpenDingTalkId": helperCurrentDOpenID2,
	}
	if !reflect.DeepEqual(sendCall.args, want) {
		t.Fatalf("send args = %#v, want %#v", sendCall.args, want)
	}
}

func TestChatEmotionSendRejectsInvalidTargets(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "missing target",
			args:    []string{"emotion", "send", "--media-id", "@media"},
			wantErr: "specify exactly one",
		},
		{
			name:    "multiple targets",
			args:    []string{"emotion", "send", "--media-id", "@media", "--group", "cid", "--open-dingtalk-id", helperCurrentDOpenID},
			wantErr: "specify exactly one",
		},
		{
			name:    "missing media",
			args:    []string{"emotion", "send", "--group", "cid"},
			wantErr: "--media-id is required",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			caller := &personalEmotionCaller{}
			err := executePersonalEmotionCommand(t, caller, tc.args...)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tc.wantErr)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("invalid command reached MCP: %+v", caller.calls)
			}
		})
	}
}

func TestChatEmotionFavoriteMapsOptionalSourcePair(t *testing.T) {
	// TC-005: 收藏来源字段成对出现时透传为 sourceConversationId/sourceMessageId。
	caller := &personalEmotionCaller{}
	err := executePersonalEmotionCommand(t, caller,
		"emotion", "favorite",
		"--media-id", "@media",
		"--name", "赞",
		"--source-conversation-id", "cid123",
		"--source-message-id", "msg123",
	)
	if err != nil {
		t.Fatalf("chat emotion favorite returned error: %v", err)
	}
	requirePersonalEmotionCall(t, caller, "favorite_personal_emotion", map[string]any{
		"mediaId":              "@media",
		"name":                 "赞",
		"sourceConversationId": "cid123",
		"sourceMessageId":      "msg123",
	})
}

func TestChatEmotionFavoriteRejectsMissingRequiredOrUnpairedSource(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "missing media",
			args:    []string{"emotion", "favorite", "--name", "赞"},
			wantErr: "--media-id is required",
		},
		{
			name:    "source conversation only",
			args:    []string{"emotion", "favorite", "--media-id", "@media", "--source-conversation-id", "cid123"},
			wantErr: "must be specified together",
		},
		{
			name:    "source message only",
			args:    []string{"emotion", "favorite", "--media-id", "@media", "--source-message-id", "msg123"},
			wantErr: "must be specified together",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			caller := &personalEmotionCaller{}
			err := executePersonalEmotionCommand(t, caller, tc.args...)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tc.wantErr)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("invalid command reached MCP: %+v", caller.calls)
			}
		})
	}
}
