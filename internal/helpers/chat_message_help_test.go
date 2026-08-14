// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package helpers

import (
	"bytes"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageChatMessageHelpDocumentsPostSendIDChain(t *testing.T) {
	tests := []struct {
		name       string
		command    string
		contains   []string
		notContain string
	}{
		{
			name:    "send returns task ID",
			command: "send",
			contains: []string{
				"openTaskId",
				"query-send-status --open-task-id <openTaskId>",
			},
		},
		{
			name:    "query returns message and conversation IDs",
			command: "query-send-status",
			contains: []string{
				"openTaskId",
				"openMessageId",
				"openConversationId",
				"chat message edit",
				"chat message recall",
			},
		},
		{
			name:    "edit includes post-send workflow",
			command: "edit",
			contains: []string{
				"send -> query-send-status -> edit",
				"query-send-status --open-task-id <上一步返回的openTaskId>",
				"edit --conversation-id <上一步返回的openConversationId> --msg-id <上一步返回的openMessageId>",
			},
			notContain: "chat message list",
		},
		{
			name:    "recall includes post-send workflow",
			command: "recall",
			contains: []string{
				"send -> query-send-status -> recall",
				"query-send-status --open-task-id <上一步返回的openTaskId>",
				"recall --conversation-id <上一步返回的openConversationId> --msg-id <上一步返回的openMessageId>",
			},
			notContain: "chat message list",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd := newChatCommand()
			var output bytes.Buffer
			cmd.SetOut(&output)
			cmd.SetErr(&output)
			cmd.SetArgs([]string{"message", test.command, "--help"})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("chat message %s --help: %v\n%s", test.command, err, output.String())
			}

			help := output.String()
			for _, want := range test.contains {
				if !strings.Contains(help, want) {
					t.Errorf("chat message %s help missing %q:\n%s", test.command, want, help)
				}
			}
			if test.notContain != "" && strings.Contains(help, test.notContain) {
				t.Errorf("chat message %s help still contains %q:\n%s", test.command, test.notContain, help)
			}
		})
	}
}

func TestCrossPlatformCoverageChatMessageHelpDocumentsOptionalTimeDefaults(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		contains []string
		absent   []string
	}{
		{
			name: "list",
			args: []string{"message", "list", "--help"},
			contains: []string{
				"--time 可选，不传时默认上海时间当前时间并向旧消息拉取",
				"默认上海时间当前时间",
				"未传 --time 时默认 older",
			},
			absent: []string{"开始时间，格式: yyyy-MM-dd HH:mm:ss (必填)"},
		},
		{
			name: "search",
			args: []string{"message", "search", "--help"},
			contains: []string{
				"可选，不传时默认最近 7 天到当前时间",
				"默认当前时间前 7 天",
				"默认当前时间",
			},
			absent: []string{"开始时间，ISO-8601 格式 (必填)", "结束时间，ISO-8601 格式 (必填)"},
		},
		{
			name: "list-by-sender",
			args: []string{"message", "list-by-sender", "--help"},
			contains: []string{
				"--start 和 --end 可选，不传时默认最近 7 天到当前时间",
				"默认当前时间前 7 天",
				"默认当前时间",
			},
			absent: []string{"开始时间，ISO-8601 格式 (必填)", "结束时间，ISO-8601 格式 (必填)"},
		},
		{
			name: "list-mentions",
			args: []string{"message", "list-mentions", "--help"},
			contains: []string{
				"--start 和 --end 可选，不传时默认最近 7 天到当前时间",
				"默认当前时间前 7 天",
				"默认当前时间",
			},
			absent: []string{"开始时间，ISO-8601 格式 (必填)", "结束时间，ISO-8601 格式 (必填)"},
		},
		{
			name: "list-all",
			args: []string{"message", "list-all", "--help"},
			contains: []string{
				"--start 和 --end 可选，不传时默认最近 1 天到当前时间",
				"默认当前时间前 1 天",
				"默认当前时间",
			},
			absent: []string{"起始时间，格式: yyyy-MM-dd HH:mm:ss (必填)", "结束时间，格式: yyyy-MM-dd HH:mm:ss (必填)"},
		},
		{
			name: "download-media",
			args: []string{"message", "download-media", "--help"},
			contains: []string{
				"只支持聊天消息 mediaId 下载，不支持钉盘 fileId",
				"fileId 下载请使用钉盘/drive 下载命令",
				"仅支持聊天消息 mediaId，不支持钉盘 fileId",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd := newChatCommand()
			var output bytes.Buffer
			cmd.SetOut(&output)
			cmd.SetErr(&output)
			cmd.SetArgs(test.args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("dws chat %s: %v\n%s", strings.Join(test.args, " "), err, output.String())
			}
			help := output.String()
			for _, want := range test.contains {
				if !strings.Contains(help, want) {
					t.Errorf("chat message %s help missing %q:\n%s", test.name, want, help)
				}
			}
			for _, unwanted := range test.absent {
				if strings.Contains(help, unwanted) {
					t.Errorf("chat message %s help still contains %q:\n%s", test.name, unwanted, help)
				}
			}
		})
	}
}
