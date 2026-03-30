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
