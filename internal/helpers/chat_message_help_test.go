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
