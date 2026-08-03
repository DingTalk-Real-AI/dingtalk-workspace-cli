// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package app

import (
	stderrors "errors"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func TestValidateChatWorkbookRawArgs(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{
			name: "members group flag",
			args: []string{"chat", "group", "members", "list", "--group", "cid-demo", "--format", "json"},
			want: "群成员列表命令路径或群参数不正确",
		},
		{
			name: "rename group flag",
			args: []string{"chat", "group", "rename", "--group=cid-demo", "--name", "新群名"},
			want: "群重命名命令不支持 --group",
		},
		{
			name: "image local path",
			args: []string{"chat", "message", "send", "--group", "cid-demo", "--msg-type", "image", "--file-path", "/tmp/x.png"},
			want: "image 消息不能直接使用 --file-path",
		},
		{
			name: "unsupported message type",
			args: []string{"chat", "message", "send", "--group", "cid-demo", "--msg-type=sticker"},
			want: "不支持指定的 --msg-type：sticker",
		},
		{
			name: "numeric group id required",
			args: []string{"chat", "group", "get-by-group-id", "--group-id", "cid-demo"},
			want: "--group-id 必须是数字群号",
		},
		{
			name: "file media id conflict",
			args: []string{"chat", "message", "send", "--group", "cid-demo", "--msg-type", "file", "--media-id", "media"},
			want: "文件消息不能使用 --media-id",
		},
		{
			name: "silent text media conflict",
			args: []string{"chat", "message", "send", "--group", "cid-demo", "--media-id", "media", "--text", "file.pdf"},
			want: "检测到 --media-id，但没有指定媒体消息类型",
		},
		{
			name: "dismiss numeric group id",
			args: []string{"chat", "group", "dismiss", "--group", "12345678"},
			want: "解散群命令需要 openConversationId，不是数字群号",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateChatWorkbookRawArgs(tc.args)
			var typed *apperrors.Error
			if !stderrors.As(err, &typed) {
				t.Fatalf("error = %T, want *errors.Error", err)
			}
			if typed.Message != tc.want || len(typed.Actions) == 0 || len(typed.Examples) == 0 {
				t.Fatalf("guidance = %#v", typed)
			}
		})
	}

	if err := validateChatWorkbookRawArgs([]string{"chat", "group", "rename", "--id", "cid-demo"}); err != nil {
		t.Fatalf("canonical rename args rejected: %v", err)
	}
}

func TestChatWorkbookHelpGuidanceCoverage(t *testing.T) {
	t.Parallel()

	for _, path := range []string{
		"chat group members",
		"chat group members add",
		"chat group members remove",
		"chat group members add-bot",
		"chat group members remove-bot",
		"chat group members list-by-ids",
		"chat group create",
		"chat group rename",
		"chat message list",
		"chat message search",
		"chat message search-advanced",
		"chat message list-all",
		"chat message list-by-sender",
	} {
		guide, ok := chatWorkbookHelpGuidance[path]
		if !ok || guide.reason == "" || guide.action == "" || guide.example == "" {
			t.Fatalf("incomplete help guidance for %q: %#v", path, guide)
		}
	}
}

func TestRawArgsFlagValue(t *testing.T) {
	t.Parallel()

	if got := rawArgsFlagValue([]string{"--msg-type", "image"}, "msg-type"); got != "image" {
		t.Fatalf("separate value = %q", got)
	}
	if got := rawArgsFlagValue([]string{"--msg-type=file"}, "msg-type"); got != "file" {
		t.Fatalf("equals value = %q", got)
	}
	if got := rawArgsFlagValue([]string{"--text", "hello"}, "msg-type"); got != "" {
		t.Fatalf("missing value = %q", got)
	}
}

func TestRawArgsRequestJSON(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{
		{"chat", "search", "--format", "json"},
		{"chat", "search", "--format=json"},
		{"chat", "search", "-f", "JSON"},
		{"chat", "search", "-f=json"},
	} {
		if !rawArgsRequestJSON(args) {
			t.Fatalf("rawArgsRequestJSON(%v) = false", args)
		}
	}
}

func TestSuppressJSONDeprecationPreamble(t *testing.T) {
	t.Parallel()

	root := NewRootCommand()
	cmd := mustFindCommand(t, root, "chat", "media", "upload")
	if cmd.Deprecated == "" {
		t.Fatal("fixture command is not deprecated")
	}
	suppressJSONDeprecationPreamble(root, []string{"chat", "media", "upload", "--format", "json"})
	if cmd.Deprecated != "" {
		t.Fatalf("JSON execution kept deprecation preamble: %q", cmd.Deprecated)
	}

	plainRoot := NewRootCommand()
	plain := mustFindCommand(t, plainRoot, "chat", "media", "upload")
	suppressJSONDeprecationPreamble(plainRoot, []string{"chat", "media", "upload"})
	if plain.Deprecated == "" {
		t.Fatal("human execution unexpectedly removed deprecation metadata")
	}
}
