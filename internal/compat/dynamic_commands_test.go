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

package compat

import (
	"testing"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/market"
)

func TestBuildDynamicCommands_ParentNesting(t *testing.T) {
	t.Parallel()

	servers := []market.ServerDescriptor{
		{
			Endpoint: "https://endpoint-chat",
			CLI: market.CLIOverlay{
				ID:      "group-chat",
				Command: "chat",
				ToolOverrides: map[string]market.CLIToolOverride{
					"list_conversations": {CLIName: "list"},
				},
			},
		},
		{
			Endpoint: "https://endpoint-bot",
			CLI: market.CLIOverlay{
				ID:      "bot",
				Command: "bot",
				Parent:  "chat",
				ToolOverrides: map[string]market.CLIToolOverride{
					"send_robot_message": {CLIName: "send"},
				},
			},
		},
	}

	cmds := BuildDynamicCommands(servers, executor.EchoRunner{}, nil)

	// Should produce only one top-level command: "chat"
	if len(cmds) != 1 {
		names := make([]string, len(cmds))
		for i, c := range cmds {
			names[i] = c.Name()
		}
		t.Fatalf("expected 1 top-level command, got %d: %v", len(cmds), names)
	}
	if cmds[0].Name() != "chat" {
		t.Fatalf("expected top-level command 'chat', got %q", cmds[0].Name())
	}

	// "bot" should be a sub-command of "chat"
	found := false
	for _, sub := range cmds[0].Commands() {
		if sub.Name() == "bot" {
			found = true
			// "bot" should have its own sub-command "send"
			hasSend := false
			for _, leaf := range sub.Commands() {
				if leaf.Name() == "send" {
					hasSend = true
				}
			}
			if !hasSend {
				t.Fatal("expected 'bot' to have sub-command 'send'")
			}
		}
	}
	if !found {
		t.Fatal("expected 'bot' as sub-command of 'chat'")
	}
}

func TestBuildDynamicCommands_ParentNotFound(t *testing.T) {
	t.Parallel()

	servers := []market.ServerDescriptor{
		{
			Endpoint: "https://endpoint-orphan",
			CLI: market.CLIOverlay{
				ID:      "orphan",
				Command: "orphan",
				Parent:  "nonexistent",
				ToolOverrides: map[string]market.CLIToolOverride{
					"do_something": {CLIName: "do"},
				},
			},
		},
	}

	cmds := BuildDynamicCommands(servers, executor.EchoRunner{}, nil)

	// Parent not found, should fall back to top-level
	if len(cmds) != 1 {
		t.Fatalf("expected 1 top-level command, got %d", len(cmds))
	}
	if cmds[0].Name() != "orphan" {
		t.Fatalf("expected top-level command 'orphan', got %q", cmds[0].Name())
	}
}

func TestBuildDynamicCommands_NoParent(t *testing.T) {
	t.Parallel()

	servers := []market.ServerDescriptor{
		{
			Endpoint: "https://endpoint-a",
			CLI: market.CLIOverlay{
				ID:      "svc-a",
				Command: "alpha",
				ToolOverrides: map[string]market.CLIToolOverride{
					"tool_a": {CLIName: "run"},
				},
			},
		},
		{
			Endpoint: "https://endpoint-b",
			CLI: market.CLIOverlay{
				ID:      "svc-b",
				Command: "beta",
				ToolOverrides: map[string]market.CLIToolOverride{
					"tool_b": {CLIName: "exec"},
				},
			},
		},
	}

	cmds := BuildDynamicCommands(servers, executor.EchoRunner{}, nil)

	if len(cmds) != 2 {
		t.Fatalf("expected 2 top-level commands, got %d", len(cmds))
	}
}

// TestBuildDynamicCommands_ParentMergeSameName covers the case where two
// servers share the same cli.command + cli.parent. Instead of producing two
// sibling subcommands with the same Name under the parent (which cobra allows
// but `--help` renders as duplicate rows), the compat layer merges them into
// a single subcommand whose children are the union of both sides.
func TestBuildDynamicCommands_ParentMergeSameName(t *testing.T) {
	t.Parallel()

	servers := []market.ServerDescriptor{
		{
			Endpoint: "https://endpoint-chat",
			CLI: market.CLIOverlay{
				ID:      "group-chat",
				Command: "chat",
				Groups: map[string]market.CLIGroupDef{
					"message": {Description: "会话消息管理"},
				},
				ToolOverrides: map[string]market.CLIToolOverride{
					"send_message_as_user":         {CLIName: "send", Group: "message"},
					"list_conversation_message_v2": {CLIName: "list", Group: "message"},
				},
			},
		},
		{
			// Second server contributes more leaves into the same "message"
			// namespace via command="message" + parent="chat".
			Endpoint: "https://endpoint-bot",
			CLI: market.CLIOverlay{
				ID:      "bot-message",
				Command: "message",
				Parent:  "chat",
				ToolOverrides: map[string]market.CLIToolOverride{
					"send_robot_group_message":     {CLIName: "send-by-bot"},
					"send_message_by_custom_robot": {CLIName: "send-by-webhook"},
				},
			},
		},
	}

	cmds := BuildDynamicCommands(servers, executor.EchoRunner{}, nil)
	if len(cmds) != 1 || cmds[0].Name() != "chat" {
		t.Fatalf("expected single top-level 'chat', got %d cmds", len(cmds))
	}

	// There must be exactly one child named "message" under chat, not two.
	var messageCmds []*cobra.Command
	for _, sub := range cmds[0].Commands() {
		if sub.Name() == "message" {
			messageCmds = append(messageCmds, sub)
		}
	}
	if len(messageCmds) != 1 {
		names := make([]string, 0, len(cmds[0].Commands()))
		for _, c := range cmds[0].Commands() {
			names = append(names, c.Name())
		}
		t.Fatalf("expected exactly one 'message' under chat, got %d — chat children: %v", len(messageCmds), names)
	}

	// The merged message subcommand must contain all four leaves.
	want := map[string]bool{"send": false, "list": false, "send-by-bot": false, "send-by-webhook": false}
	for _, leaf := range messageCmds[0].Commands() {
		if _, ok := want[leaf.Name()]; ok {
			want[leaf.Name()] = true
		}
	}
	for leaf, seen := range want {
		if !seen {
			t.Errorf("expected 'chat message %s' after merge, missing", leaf)
		}
	}
}

// TestBuildDynamicCommands_ParentMergeRecursive covers a multi-level merge:
// chat already has `group.members` (with add/remove), and a separate
// bot-group server contributes `dws chat group members add-bot` via
// command=group + parent=chat + groups.members. The "group" and "members"
// nodes must each be merged, not duplicated.
func TestBuildDynamicCommands_ParentMergeRecursive(t *testing.T) {
	t.Parallel()

	servers := []market.ServerDescriptor{
		{
			Endpoint: "https://endpoint-chat",
			CLI: market.CLIOverlay{
				ID:      "group-chat",
				Command: "chat",
				Groups: map[string]market.CLIGroupDef{
					"group":         {Description: "群组管理"},
					"group.members": {Description: "群成员管理"},
				},
				ToolOverrides: map[string]market.CLIToolOverride{
					"add_group_member":    {CLIName: "add", Group: "group.members"},
					"remove_group_member": {CLIName: "remove", Group: "group.members"},
				},
			},
		},
		{
			Endpoint: "https://endpoint-bot",
			CLI: market.CLIOverlay{
				ID:      "bot-group",
				Command: "group",
				Parent:  "chat",
				Groups: map[string]market.CLIGroupDef{
					"members": {Description: "机器人群成员"},
				},
				ToolOverrides: map[string]market.CLIToolOverride{
					"add_robot_to_group": {CLIName: "add-bot", Group: "members"},
				},
			},
		},
	}

	cmds := BuildDynamicCommands(servers, executor.EchoRunner{}, nil)
	if len(cmds) != 1 || cmds[0].Name() != "chat" {
		t.Fatalf("expected single top-level 'chat', got %d", len(cmds))
	}

	// One 'group' under chat.
	var groupCmds []*cobra.Command
	for _, sub := range cmds[0].Commands() {
		if sub.Name() == "group" {
			groupCmds = append(groupCmds, sub)
		}
	}
	if len(groupCmds) != 1 {
		t.Fatalf("expected single 'group' under chat, got %d", len(groupCmds))
	}

	// One 'members' under chat.group.
	var membersCmds []*cobra.Command
	for _, sub := range groupCmds[0].Commands() {
		if sub.Name() == "members" {
			membersCmds = append(membersCmds, sub)
		}
	}
	if len(membersCmds) != 1 {
		t.Fatalf("expected single 'members' under chat.group, got %d", len(membersCmds))
	}

	// The merged members subcommand must contain add, remove, add-bot.
	want := map[string]bool{"add": false, "remove": false, "add-bot": false}
	for _, leaf := range membersCmds[0].Commands() {
		if _, ok := want[leaf.Name()]; ok {
			want[leaf.Name()] = true
		}
	}
	for leaf, seen := range want {
		if !seen {
			t.Errorf("expected 'chat group members %s', missing", leaf)
		}
	}
}

// TestBuildDynamicCommands_ParentMergeLeafCollision verifies that when two
// servers both produce the same leaf path (e.g. both try to register
// `chat message send`), the first one wins and the second is silently
// dropped rather than producing a duplicate cobra command.
func TestBuildDynamicCommands_ParentMergeLeafCollision(t *testing.T) {
	t.Parallel()

	servers := []market.ServerDescriptor{
		{
			Endpoint: "https://endpoint-chat",
			CLI: market.CLIOverlay{
				ID:      "group-chat",
				Command: "chat",
				Groups:  map[string]market.CLIGroupDef{"message": {Description: "消息"}},
				ToolOverrides: map[string]market.CLIToolOverride{
					"send_message_as_user": {CLIName: "send", Group: "message"},
				},
			},
		},
		{
			Endpoint: "https://endpoint-bot",
			CLI: market.CLIOverlay{
				ID:      "bot-message",
				Command: "message",
				Parent:  "chat",
				ToolOverrides: map[string]market.CLIToolOverride{
					// Intentional collision: same leaf name.
					"send_robot_group_message": {CLIName: "send"},
				},
			},
		},
	}

	cmds := BuildDynamicCommands(servers, executor.EchoRunner{}, nil)
	if len(cmds) != 1 {
		t.Fatalf("expected 1 top-level, got %d", len(cmds))
	}
	messageCmd := findSubcommand(cmds[0], "message")
	if messageCmd == nil {
		t.Fatal("expected 'message' subcommand under chat")
	}
	// Exactly one 'send' leaf.
	var sendCount int
	for _, leaf := range messageCmd.Commands() {
		if leaf.Name() == "send" {
			sendCount++
		}
	}
	if sendCount != 1 {
		t.Fatalf("expected exactly one 'send' leaf, got %d", sendCount)
	}
}
