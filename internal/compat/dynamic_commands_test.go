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

// Regression: bindings whose schema type is array/object must be promoted
// from ValueString (the buildOverrideBindings default) to ValueJSON so the
// cli parses input as JSON instead of forwarding raw strings to MCP — the
// backend silently drops stringified payloads on params it expects as arrays.
// See PR #154.
func TestUpgradeBindingsFromSchema(t *testing.T) {
	t.Parallel()

	bindings := []FlagBinding{
		{FlagName: "values", Property: "values", Kind: ValueString},
		{FlagName: "node-id", Property: "nodeId", Kind: ValueString},
		{FlagName: "records", Property: "records", Kind: ValueString},
		{FlagName: "remind-type", Property: "remindType", Kind: ValueString},
		{FlagName: "criteria", Property: "criteria", Kind: ValueString},
		// Already non-string: must NOT be downgraded.
		{FlagName: "page", Property: "page", Kind: ValueInt},
	}

	schemaJSON := `{
		"properties": {
			"values":     {"type": "array", "items": {"type": "array", "items": {"type": "string"}}},
			"nodeId":     {"type": "string"},
			"records":    {"type": "array", "items": {"type": "object"}},
			"remindType": {"type": "number"},
			"criteria":   {"type": "object"},
			"page":       {"type": "integer"}
		}
	}`

	got := upgradeBindingsFromSchema(bindings, schemaJSON)
	want := map[string]ValueKind{
		"values":     ValueJSON,
		"nodeId":     ValueString,
		"records":    ValueJSON,
		"remindType": ValueString,
		"criteria":   ValueJSON,
		"page":       ValueInt,
	}
	for _, b := range got {
		if w, ok := want[b.Property]; ok && b.Kind != w {
			t.Errorf("%s kind = %q, want %q", b.Property, b.Kind, w)
		}
	}
}

func TestUpgradeBindingsFromSchema_GuardClauses(t *testing.T) {
	t.Parallel()

	bindings := []FlagBinding{{FlagName: "v", Property: "v", Kind: ValueString}}

	if got := upgradeBindingsFromSchema(nil, `{"properties":{"v":{"type":"array"}}}`); got != nil {
		t.Errorf("nil bindings should return nil, got %v", got)
	}
	if got := upgradeBindingsFromSchema(bindings, ""); got[0].Kind != ValueString {
		t.Errorf("empty schema should keep ValueString, got %q", got[0].Kind)
	}
	if got := upgradeBindingsFromSchema(bindings, "not-json"); got[0].Kind != ValueString {
		t.Errorf("invalid schema should keep ValueString, got %q", got[0].Kind)
	}
	if got := upgradeBindingsFromSchema(bindings, `{"properties":{"other":{"type":"array"}}}`); got[0].Kind != ValueString {
		t.Errorf("unknown property should keep ValueString, got %q", got[0].Kind)
	}
}
