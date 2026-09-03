package helpers

import (
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func executeChatPortsCoverageCommand(t *testing.T, caller edition.ToolCaller, args ...string) error {
	t.Helper()
	previousDeps := deps
	t.Cleanup(func() { deps = previousDeps })
	InitDeps(caller)
	deps.Out.w = io.Discard
	deps.Out.errW = io.Discard
	root := newChatCommand()
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetArgs(args)
	return root.Execute()
}

func TestCrossPlatformCoverageChatUpdateTextEmotion(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want guardedMutationCall
	}{
		{
			name: "conversation-id flag",
			args: []string{
				"message", "update-text-emotion",
				"--conversation-id", "conv-1",
				"--msg-id", "msg-1",
				"--old-emotion-id", "old-1",
				"--emotion-id", "new-1",
				"--emotion-name", "like",
				"--text", "nice",
				"--background-id", "im_bg_5",
			},
			want: guardedMutationCall{
				productID: "im",
				toolName:  "update_text_emotion",
				args: map[string]any{
					"openConversationId": "conv-1",
					"openMsgId":          "msg-1",
					"oldEmotionId":       "old-1",
					"emotionId":          "new-1",
					"emotionName":        "like",
					"text":               "nice",
					"backgroundId":       "im_bg_5",
				},
			},
		},
		{
			name: "group alias for conversation-id",
			args: []string{
				"message", "update-text-emotion",
				"--group", "conv-2",
				"--msg-id", "msg-2",
				"--old-emotion-id", "old-2",
				"--emotion-id", "new-2",
				"--emotion-name", "heart",
				"--text", "great",
				"--background-id", "im_bg_1",
			},
			want: guardedMutationCall{
				productID: "im",
				toolName:  "update_text_emotion",
				args: map[string]any{
					"openConversationId": "conv-2",
					"openMsgId":          "msg-2",
					"oldEmotionId":       "old-2",
					"emotionId":          "new-2",
					"emotionName":        "heart",
					"text":               "great",
					"backgroundId":       "im_bg_1",
				},
			},
		},
		{
			name: "open-conversation-id alias",
			args: []string{
				"message", "update-text-emotion",
				"--open-conversation-id", "conv-3",
				"--message-id", "msg-3",
				"--old-emotion-id", "old-3",
				"--emotion-id", "new-3",
				"--emotion-name", "smile",
				"--text", "done",
				"--background-id", "im_bg_2",
			},
			want: guardedMutationCall{
				productID: "im",
				toolName:  "update_text_emotion",
				args: map[string]any{
					"openConversationId": "conv-3",
					"openMsgId":          "msg-3",
					"oldEmotionId":       "old-3",
					"emotionId":          "new-3",
					"emotionName":        "smile",
					"text":               "done",
					"backgroundId":       "im_bg_2",
				},
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			caller := &guardedMutationCaller{}
			err := executeGuardedMutationCommand(t, caller, newChatCommand, test.args...)
			if err != nil {
				t.Fatalf("chat %s returned error: %v", strings.Join(test.args, " "), err)
			}
			if len(caller.calls) != 1 || !reflect.DeepEqual(caller.calls[0], test.want) {
				t.Fatalf("tool calls = %#v, want %#v", caller.calls, test.want)
			}
		})
	}
}

func TestCrossPlatformCoverageChatUpdateTextEmotionRequiredFlags(t *testing.T) {
	fullArgs := []string{
		"message", "update-text-emotion",
		"--conversation-id", "conv-1",
		"--msg-id", "msg-1",
		"--old-emotion-id", "old-1",
		"--emotion-id", "new-1",
		"--emotion-name", "like",
		"--text", "nice",
		"--background-id", "im_bg_5",
	}
	dropFlag := func(name string) []string {
		out := make([]string, 0, len(fullArgs))
		for i := 0; i < len(fullArgs); i++ {
			if fullArgs[i] == name {
				i++ // skip the flag value too
				continue
			}
			out = append(out, fullArgs[i])
		}
		return out
	}
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "missing conversation-id and aliases",
			args:    dropFlag("--conversation-id"),
			wantErr: "missing required flag: --conversation-id (or --group / --id / --chat / --open-conversation-id)",
		},
		{
			name:    "missing old-emotion-id",
			args:    dropFlag("--old-emotion-id"),
			wantErr: `required flag(s) "old-emotion-id" not set`,
		},
		{
			name: "missing message-id and background-id",
			args: []string{
				"message", "update-text-emotion",
				"--conversation-id", "conv-1",
				"--old-emotion-id", "old-1",
				"--emotion-id", "new-1",
				"--emotion-name", "like",
				"--text", "nice",
			},
			wantErr: "missing required flag: --message-id (or --msg-id / --open-message-id)",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			caller := &guardedMutationCaller{}
			err := executeGuardedMutationCommand(t, caller, newChatCommand, test.args...)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("err = %v, want message containing %q", err, test.wantErr)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("tool calls = %#v, want none", caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageChatGroupGetMuteConfig(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want guardedMutationCall
	}{
		{
			name: "legacy group alias",
			args: []string{"group", "get-mute-config", "--group", "conv-1"},
			want: guardedMutationCall{
				productID: "im",
				toolName:  "get_group_mute_config",
				args:      map[string]any{"openConversationId": "conv-1"},
			},
		},
		{
			name: "canonical conversation id",
			args: []string{"group", "get-mute-config", "--conversation-id", "conv-2"},
			want: guardedMutationCall{
				productID: "im",
				toolName:  "get_group_mute_config",
				args:      map[string]any{"openConversationId": "conv-2"},
			},
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			caller := &guardedMutationCaller{}
			err := executeGuardedMutationCommand(t, caller, newChatCommand, test.args...)
			if err != nil {
				t.Fatalf("get-mute-config returned error: %v", err)
			}
			if len(caller.calls) != 1 || !reflect.DeepEqual(caller.calls[0], test.want) {
				t.Fatalf("tool calls = %#v, want %#v", caller.calls, test.want)
			}
		})
	}
}

func TestCrossPlatformCoverageChatGroupGetMuteConfigRecordsRawArgs(t *testing.T) {
	caller := &depthArgsRecordingCaller{steps: []scriptedToolStep{
		{text: `{"muteBlackList":[],"muteWhiteList":[]}`},
	}}
	err := executeChatPortsCoverageCommand(t, caller,
		"group", "get-mute-config", "--group", "conv-9")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("calls = %#v, want 1", caller.calls)
	}
	if caller.calls[0]["openConversationId"] != "conv-9" {
		t.Fatalf("raw args = %#v", caller.calls[0])
	}
}

func TestCrossPlatformCoverageChatGroupGetMuteConfigRequiresConversationID(t *testing.T) {
	caller := &guardedMutationCaller{}
	err := executeGuardedMutationCommand(t, caller, newChatCommand,
		"group", "get-mute-config")
	if err == nil || !strings.Contains(err.Error(), "--conversation-id") {
		t.Fatalf("err = %v, want message containing --conversation-id", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("tool calls = %#v, want none", caller.calls)
	}
}
