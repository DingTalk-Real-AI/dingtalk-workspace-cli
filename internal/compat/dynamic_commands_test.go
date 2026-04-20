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
	"context"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/market"
	"github.com/spf13/cobra"
)

// captureRunner records the most recent invocation for assertion.
type captureRunner struct {
	lastProduct string
	lastTool    string
	lastParams  map[string]any
}

func (c *captureRunner) Run(_ context.Context, inv executor.Invocation) (executor.Result, error) {
	c.lastProduct = inv.CanonicalProduct
	c.lastTool = inv.Tool
	c.lastParams = inv.Params
	return executor.Result{Invocation: inv}, nil
}

// findChild returns the direct sub-command with the given name, or nil.
func findChild(parent *cobra.Command, name string) *cobra.Command {
	for _, c := range parent.Commands() {
		if c.Name() == name {
			return c
		}
	}
	return nil
}

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

// Phase 0 P1 schema extensions -----------------------------------------------

// TestBuildDynamicCommands_ShorthandFlag verifies the Shorthand field wires
// through to cobra's StringP short form.
func TestBuildDynamicCommands_ShorthandFlag(t *testing.T) {
	t.Parallel()

	servers := []market.ServerDescriptor{
		{
			Endpoint: "https://endpoint-chat",
			CLI: market.CLIOverlay{
				ID:      "chat",
				Command: "chat",
				ToolOverrides: map[string]market.CLIToolOverride{
					"send_message": {
						CLIName: "send",
						Flags: map[string]market.CLIFlagOverride{
							"conversationId": {
								Alias:     "conv",
								Shorthand: "c",
							},
						},
					},
				},
			},
		},
	}

	cmds := BuildDynamicCommands(servers, executor.EchoRunner{}, nil)
	if len(cmds) != 1 {
		t.Fatalf("expected 1 cmd, got %d", len(cmds))
	}
	send := findChild(cmds[0], "send")
	if send == nil {
		t.Fatal("send leaf not found")
	}
	f := send.Flags().Lookup("conv")
	if f == nil {
		t.Fatal("--conv flag missing")
	}
	if f.Shorthand != "c" {
		t.Fatalf("expected shorthand 'c', got %q", f.Shorthand)
	}
}

// TestBuildDynamicCommands_RequiredFlag verifies Required marks the flag via
// cobra.MarkFlagRequired (recorded under the BashCompletion annotation).
func TestBuildDynamicCommands_RequiredFlag(t *testing.T) {
	t.Parallel()

	servers := []market.ServerDescriptor{
		{
			Endpoint: "https://endpoint-chat",
			CLI: market.CLIOverlay{
				ID:      "chat",
				Command: "chat",
				ToolOverrides: map[string]market.CLIToolOverride{
					"send_message": {
						CLIName: "send",
						Flags: map[string]market.CLIFlagOverride{
							"conversationId": {
								Alias:    "conv",
								Required: true,
							},
						},
					},
				},
			},
		},
	}

	cmds := BuildDynamicCommands(servers, executor.EchoRunner{}, nil)
	send := findChild(cmds[0], "send")
	if send == nil {
		t.Fatal("send leaf not found")
	}
	f := send.Flags().Lookup("conv")
	if f == nil {
		t.Fatal("--conv flag missing")
	}
	if _, ok := f.Annotations[cobra.BashCompOneRequiredFlag]; !ok {
		t.Fatalf("expected --conv to be marked required, annotations=%v", f.Annotations)
	}
}

// TestBuildDynamicCommands_RequiredIgnoredWhenPositional verifies that
// Required is ignored when Positional is true (cobra arity handles it).
func TestBuildDynamicCommands_RequiredIgnoredWhenPositional(t *testing.T) {
	t.Parallel()

	servers := []market.ServerDescriptor{
		{
			Endpoint: "https://endpoint-chat",
			CLI: market.CLIOverlay{
				ID:      "chat",
				Command: "chat",
				ToolOverrides: map[string]market.CLIToolOverride{
					"send_message": {
						CLIName: "send",
						Flags: map[string]market.CLIFlagOverride{
							"text": {
								Positional:      true,
								PositionalIndex: 0,
								Required:        true,
							},
						},
					},
				},
			},
		},
	}

	cmds := BuildDynamicCommands(servers, executor.EchoRunner{}, nil)
	send := findChild(cmds[0], "send")
	if send == nil {
		t.Fatal("send leaf not found")
	}
	if f := send.Flags().Lookup("text"); f != nil {
		t.Fatalf("positional 'text' must not be registered as flag: %+v", f)
	}
}

// TestBuildDynamicCommands_PositionalArg verifies Positional params are NOT
// registered as flags and that cobra Args switches to MinimumNArgs.
func TestBuildDynamicCommands_PositionalArg(t *testing.T) {
	t.Parallel()

	servers := []market.ServerDescriptor{
		{
			Endpoint: "https://endpoint-chat",
			CLI: market.CLIOverlay{
				ID:      "chat",
				Command: "chat",
				ToolOverrides: map[string]market.CLIToolOverride{
					"send_message": {
						CLIName: "send",
						Flags: map[string]market.CLIFlagOverride{
							"text": {
								Positional:      true,
								PositionalIndex: 0,
							},
							"conversationId": {
								Alias: "conv",
							},
						},
					},
				},
			},
		},
	}

	cmds := BuildDynamicCommands(servers, executor.EchoRunner{}, nil)
	send := findChild(cmds[0], "send")
	if send == nil {
		t.Fatal("send leaf not found")
	}
	if f := send.Flags().Lookup("text"); f != nil {
		t.Fatalf("text should be positional, not flag, but got flag with usage=%q", f.Usage)
	}
	// Non-positional flag should still be present.
	if f := send.Flags().Lookup("conv"); f == nil {
		t.Fatal("--conv flag should still be registered")
	}
	// Validate arity: executing with zero args must fail with cobra's arity error.
	// Execute() walks up to the root, so set args on the root command.
	cmds[0].SetArgs([]string{"send"})
	cmds[0].SilenceUsage = true
	cmds[0].SilenceErrors = true
	send.SilenceUsage = true
	send.SilenceErrors = true
	err := cmds[0].Execute()
	if err == nil {
		t.Fatal("expected error when required positional is missing")
	}
	if !strings.Contains(err.Error(), "arg") {
		t.Fatalf("expected arity error, got %v", err)
	}
}

// TestBuildDynamicCommands_PositionalArgInjection verifies that positional
// args are injected into params[property] when the leaf is invoked.
func TestBuildDynamicCommands_PositionalArgInjection(t *testing.T) {
	t.Parallel()

	captured := &captureRunner{}
	servers := []market.ServerDescriptor{
		{
			Endpoint: "https://endpoint-chat",
			CLI: market.CLIOverlay{
				ID:      "chat",
				Command: "chat",
				ToolOverrides: map[string]market.CLIToolOverride{
					"send_message": {
						CLIName: "send",
						Flags: map[string]market.CLIFlagOverride{
							"text": {
								Positional:      true,
								PositionalIndex: 0,
							},
						},
					},
				},
			},
		},
	}

	cmds := BuildDynamicCommands(servers, captured, nil)
	send := findChild(cmds[0], "send")
	if send == nil {
		t.Fatal("send leaf not found")
	}
	if err := send.RunE(send, []string{"hello world"}); err != nil {
		t.Fatalf("runE: %v", err)
	}
	if captured.lastParams["text"] != "hello world" {
		t.Fatalf("expected params[text]='hello world', got %+v", captured.lastParams)
	}
}

// TestBuildDynamicCommands_ServerOverride verifies ServerOverride routes
// the tool invocation's CanonicalProduct to a different product.
func TestBuildDynamicCommands_ServerOverride(t *testing.T) {
	t.Parallel()

	captured := &captureRunner{}
	servers := []market.ServerDescriptor{
		{
			Endpoint: "https://endpoint-chat",
			CLI: market.CLIOverlay{
				ID:      "chat",
				Command: "chat",
				ToolOverrides: map[string]market.CLIToolOverride{
					"list_bots": {
						CLIName:        "bot-list",
						ServerOverride: "bot",
					},
				},
			},
		},
	}

	cmds := BuildDynamicCommands(servers, captured, nil)
	leaf := findChild(cmds[0], "bot-list")
	if leaf == nil {
		t.Fatal("bot-list leaf not found")
	}
	if err := leaf.RunE(leaf, nil); err != nil {
		t.Fatalf("runE: %v", err)
	}
	if captured.lastProduct != "bot" {
		t.Fatalf("expected CanonicalProduct=bot, got %q", captured.lastProduct)
	}
	if captured.lastTool != "list_bots" {
		t.Fatalf("expected tool=list_bots, got %q", captured.lastTool)
	}
}

// TestBuildDynamicCommands_ServerOverrideFallback verifies ServerOverride
// falls back to cli.ID when left empty (backwards compat with existing configs).
func TestBuildDynamicCommands_ServerOverrideFallback(t *testing.T) {
	t.Parallel()

	captured := &captureRunner{}
	servers := []market.ServerDescriptor{
		{
			Endpoint: "https://endpoint-chat",
			CLI: market.CLIOverlay{
				ID:      "chat",
				Command: "chat",
				ToolOverrides: map[string]market.CLIToolOverride{
					"list_conversations": {CLIName: "list"},
				},
			},
		},
	}

	cmds := BuildDynamicCommands(servers, captured, nil)
	leaf := findChild(cmds[0], "list")
	if leaf == nil {
		t.Fatal("list leaf not found")
	}
	if err := leaf.RunE(leaf, nil); err != nil {
		t.Fatalf("runE: %v", err)
	}
	if captured.lastProduct != "chat" {
		t.Fatalf("expected CanonicalProduct=chat (fallback), got %q", captured.lastProduct)
	}
}

// TestBuildDynamicCommands_DescriptionOverridesUsage verifies that overlay
// Description wins over the default paramName usage text.
func TestBuildDynamicCommands_DescriptionOverridesUsage(t *testing.T) {
	t.Parallel()

	servers := []market.ServerDescriptor{
		{
			Endpoint: "https://endpoint-chat",
			CLI: market.CLIOverlay{
				ID:      "chat",
				Command: "chat",
				ToolOverrides: map[string]market.CLIToolOverride{
					"send_message": {
						CLIName: "send",
						Flags: map[string]market.CLIFlagOverride{
							"conversationId": {
								Alias:       "conv",
								Description: "Target conversation open ID",
							},
						},
					},
				},
			},
		},
	}

	cmds := BuildDynamicCommands(servers, executor.EchoRunner{}, nil)
	send := findChild(cmds[0], "send")
	if send == nil {
		t.Fatal("send leaf not found")
	}
	f := send.Flags().Lookup("conv")
	if f == nil {
		t.Fatal("--conv flag missing")
	}
	if f.Usage != "Target conversation open ID" {
		t.Fatalf("expected custom description, got %q", f.Usage)
	}
}

// TestBuildDynamicCommands_OverlayFlagWinsOverDetailSchema verifies that when
// the Detail API schema and overlay both define the same param, the overlay's
// Description survives enrichment.
func TestBuildDynamicCommands_OverlayFlagWinsOverDetailSchema(t *testing.T) {
	t.Parallel()

	servers := []market.ServerDescriptor{
		{
			Endpoint: "https://endpoint-chat",
			CLI: market.CLIOverlay{
				ID:      "chat",
				Command: "chat",
				ToolOverrides: map[string]market.CLIToolOverride{
					"send_message": {
						CLIName: "send",
						Flags: map[string]market.CLIFlagOverride{
							"conversationId": {
								Alias:       "conv",
								Description: "Overlay wins",
							},
						},
					},
				},
			},
		},
	}

	details := map[string][]market.DetailTool{
		"chat": {
			{
				ToolName: "send_message",
				ToolRequest: `{"properties":{` +
					`"conversationId":{"type":"string","description":"Schema description"}` +
					`},"required":["conversationId"]}`,
			},
		},
	}

	cmds := BuildDynamicCommands(servers, executor.EchoRunner{}, details)
	send := findChild(cmds[0], "send")
	if send == nil {
		t.Fatal("send leaf not found")
	}
	f := send.Flags().Lookup("conv")
	if f == nil {
		t.Fatal("--conv flag missing")
	}
	if f.Usage != "Overlay wins" {
		t.Fatalf("expected overlay description to win, got %q", f.Usage)
	}
}

// Phase 5 P2 schema extensions ----------------------------------------------

// TestBuildDynamicCommands_BodyWrapper verifies that bodyWrapper wraps all
// user-facing params under the named key while keeping internal control
// keys (prefix '_') at the top level.
func TestBuildDynamicCommands_BodyWrapper(t *testing.T) {
	t.Parallel()

	runner := &captureRunner{}
	servers := []market.ServerDescriptor{
		{
			Endpoint: "https://endpoint-todo",
			CLI: market.CLIOverlay{
				ID:      "todo",
				Command: "todo",
				ToolOverrides: map[string]market.CLIToolOverride{
					"create_todo": {
						CLIName:     "create",
						BodyWrapper: "PersonalTodoCreateVO",
						Flags: map[string]market.CLIFlagOverride{
							"subject": {Alias: "subject", Required: true},
							"dueTime": {Alias: "due", Transform: "iso8601_to_millis"},
						},
					},
				},
			},
		},
	}

	cmds := BuildDynamicCommands(servers, runner, nil)
	create := findChild(cmds[0], "create")
	if create == nil {
		t.Fatal("create leaf not found")
	}

	cmds[0].SetArgs([]string{"create", "--subject", "buy milk", "--due", "2026-05-01"})
	cmds[0].SilenceErrors = true
	cmds[0].SilenceUsage = true
	if err := cmds[0].Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	wrap, ok := runner.lastParams["PersonalTodoCreateVO"].(map[string]any)
	if !ok {
		t.Fatalf("expected params wrapped under PersonalTodoCreateVO, got %+v", runner.lastParams)
	}
	if wrap["subject"] != "buy milk" {
		t.Fatalf("wrap[subject]=%v, want 'buy milk'", wrap["subject"])
	}
	if due, ok := wrap["dueTime"].(int64); !ok || due == 0 {
		t.Fatalf("wrap[dueTime] must be int64 millis (iso8601_to_millis), got %T %v", wrap["dueTime"], wrap["dueTime"])
	}
	if _, exists := runner.lastParams["subject"]; exists {
		t.Fatalf("bodyWrapper must move 'subject' off the top level, got %+v", runner.lastParams)
	}
}

// TestBuildDynamicCommands_BodyWrapperPreservesInternalKeys verifies that
// keys starting with '_' stay at the top level so sensitive / _blocked /
// _yes style confirmation plumbing keeps working.
func TestBuildDynamicCommands_BodyWrapperPreservesInternalKeys(t *testing.T) {
	t.Parallel()

	params := map[string]any{
		"subject":  "x",
		"_blocked": true,
		"_yes":     false,
	}
	wrapParamsIntoBody(params, "Body")
	body, ok := params["Body"].(map[string]any)
	if !ok {
		t.Fatalf("expected Body wrapper, got %+v", params)
	}
	if body["subject"] != "x" {
		t.Fatalf("body[subject]=%v, want 'x'", body["subject"])
	}
	if _, has := body["_blocked"]; has {
		t.Fatal("internal _blocked must not be moved into body")
	}
	if _, has := params["_blocked"]; !has {
		t.Fatal("_blocked must remain at top level")
	}
}

// TestBuildDynamicCommands_MutuallyExclusive verifies that cobra refuses to
// run when two flags from a mutually-exclusive group are set together.
func TestBuildDynamicCommands_MutuallyExclusive(t *testing.T) {
	t.Parallel()

	servers := []market.ServerDescriptor{
		{
			Endpoint: "https://endpoint-chat",
			CLI: market.CLIOverlay{
				ID:      "chat",
				Command: "chat",
				ToolOverrides: map[string]market.CLIToolOverride{
					"list_messages": {
						CLIName: "list",
						Flags: map[string]market.CLIFlagOverride{
							"groupId":         {Alias: "group"},
							"userId":          {Alias: "user"},
							"openDingtalkId":  {Alias: "open-dingtalk-id"},
						},
						MutuallyExclusive: [][]string{
							{"group", "user", "open-dingtalk-id"},
						},
					},
				},
			},
		},
	}

	cmds := BuildDynamicCommands(servers, executor.EchoRunner{}, nil)
	cmds[0].SetArgs([]string{"list", "--group", "g1", "--user", "u1"})
	cmds[0].SilenceErrors = true
	cmds[0].SilenceUsage = true
	err := cmds[0].Execute()
	if err == nil {
		t.Fatal("expected mutually-exclusive error, got nil")
	}
	// cobra 1.10+ prints "if any flags in the group [...] are set none of the others can be";
	// earlier versions used "mutually exclusive". Accept either wording.
	msg := err.Error()
	if !strings.Contains(msg, "none of the others") && !strings.Contains(msg, "mutually") && !strings.Contains(msg, "exclusive") {
		t.Fatalf("expected mutually-exclusive error, got %v", err)
	}
}

// TestBuildDynamicCommands_RequireOneOf verifies that cobra refuses to run
// when none of the required-one-of flags are set.
func TestBuildDynamicCommands_RequireOneOf(t *testing.T) {
	t.Parallel()

	servers := []market.ServerDescriptor{
		{
			Endpoint: "https://endpoint-chat",
			CLI: market.CLIOverlay{
				ID:      "chat",
				Command: "chat",
				ToolOverrides: map[string]market.CLIToolOverride{
					"list_messages": {
						CLIName: "list",
						Flags: map[string]market.CLIFlagOverride{
							"groupId": {Alias: "group"},
							"userId":  {Alias: "user"},
						},
						RequireOneOf: [][]string{
							{"group", "user"},
						},
					},
				},
			},
		},
	}

	cmds := BuildDynamicCommands(servers, executor.EchoRunner{}, nil)
	cmds[0].SetArgs([]string{"list"})
	cmds[0].SilenceErrors = true
	cmds[0].SilenceUsage = true
	err := cmds[0].Execute()
	if err == nil {
		t.Fatal("expected require-one-of error, got nil")
	}
	if !strings.Contains(err.Error(), "required") && !strings.Contains(err.Error(), "one") {
		t.Fatalf("expected required-one-of error, got %v", err)
	}
}

// TestBuildDynamicCommands_RequireOneOfSatisfied verifies that setting one
// of the required flags lets the command run normally.
func TestBuildDynamicCommands_RequireOneOfSatisfied(t *testing.T) {
	t.Parallel()

	runner := &captureRunner{}
	servers := []market.ServerDescriptor{
		{
			Endpoint: "https://endpoint-chat",
			CLI: market.CLIOverlay{
				ID:      "chat",
				Command: "chat",
				ToolOverrides: map[string]market.CLIToolOverride{
					"list_messages": {
						CLIName: "list",
						Flags: map[string]market.CLIFlagOverride{
							"groupId": {Alias: "group"},
							"userId":  {Alias: "user"},
						},
						RequireOneOf: [][]string{{"group", "user"}},
					},
				},
			},
		},
	}

	cmds := BuildDynamicCommands(servers, runner, nil)
	cmds[0].SetArgs([]string{"list", "--group", "g1"})
	cmds[0].SilenceErrors = true
	cmds[0].SilenceUsage = true
	if err := cmds[0].Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if runner.lastParams["groupId"] != "g1" {
		t.Fatalf("expected groupId=g1, got %+v", runner.lastParams)
	}
}

// TestBuildDynamicCommands_RedirectTo verifies redirectTo replaces the leaf
// with a stub that only prints the redirect target.
func TestBuildDynamicCommands_RedirectTo(t *testing.T) {
	t.Parallel()

	runner := &captureRunner{}
	servers := []market.ServerDescriptor{
		{
			Endpoint: "https://endpoint-chat",
			CLI: market.CLIOverlay{
				ID:      "chat",
				Command: "chat",
				ToolOverrides: map[string]market.CLIToolOverride{
					"old_history": {
						CLIName:    "history",
						RedirectTo: "dws chat message list",
					},
				},
			},
		},
	}

	cmds := BuildDynamicCommands(servers, runner, nil)
	history := findChild(cmds[0], "history")
	if history == nil {
		t.Fatal("history stub not found")
	}
	out := &strings.Builder{}
	history.SetOut(out)
	history.SetErr(out)
	if err := history.RunE(history, nil); err != nil {
		t.Fatalf("runE: %v", err)
	}
	if !strings.Contains(out.String(), "dws chat message list") {
		t.Fatalf("redirect output missing target, got %q", out.String())
	}
	if runner.lastTool != "" {
		t.Fatalf("redirect must not call a tool, got %q", runner.lastTool)
	}
	// Redirect stubs disable flag parsing, so arbitrary args must not error.
	if history.Flags().Lookup("json") != nil {
		t.Fatal("redirect stub must not register --json / --params flags")
	}
}

// TestBuildDynamicCommands_Hints verifies cli.hintCommands creates a stub
// sub-command under the overlay root (or under a named group) that prints
// the canonical path instead of invoking a tool.
func TestBuildDynamicCommands_Hints(t *testing.T) {
	t.Parallel()

	servers := []market.ServerDescriptor{
		{
			Endpoint: "https://endpoint-chat",
			CLI: market.CLIOverlay{
				ID:      "chat",
				Command: "chat",
				Groups: map[string]market.CLIGroupDef{
					"message": {Description: "会话消息"},
				},
				ToolOverrides: map[string]market.CLIToolOverride{
					"list_conversations": {CLIName: "list", Group: "message"},
				},
				Hints: map[string]market.CLIHintDef{
					"history": {
						Target:      "dws chat message list",
						Description: "migrated to `message list`",
					},
					"purge": {
						Target: "dws chat message delete-all",
						Group:  "message",
					},
				},
			},
		},
	}

	cmds := BuildDynamicCommands(servers, executor.EchoRunner{}, nil)
	root := cmds[0]

	// history hint attached directly under root.
	history := findChild(root, "history")
	if history == nil {
		t.Fatal("hint 'history' missing under chat root")
	}
	if history.Short != "migrated to `message list`" {
		t.Fatalf("hint description not applied, got %q", history.Short)
	}

	// purge hint nested under message group.
	msg := findChild(root, "message")
	if msg == nil {
		t.Fatal("message group missing")
	}
	purge := findChild(msg, "purge")
	if purge == nil {
		t.Fatal("hint 'purge' missing under message group")
	}

	out := &strings.Builder{}
	purge.SetOut(out)
	if err := purge.RunE(purge, nil); err != nil {
		t.Fatalf("runE: %v", err)
	}
	if !strings.Contains(out.String(), "dws chat message delete-all") {
		t.Fatalf("hint output missing target, got %q", out.String())
	}
}

// TestBuildDynamicCommands_UnknownFlagConstraintSkipped verifies that a
// stale / malformed mutuallyExclusive referencing an unknown flag is logged
// and skipped rather than blocking command tree construction.
func TestBuildDynamicCommands_UnknownFlagConstraintSkipped(t *testing.T) {
	t.Parallel()

	servers := []market.ServerDescriptor{
		{
			Endpoint: "https://endpoint-chat",
			CLI: market.CLIOverlay{
				ID:      "chat",
				Command: "chat",
				ToolOverrides: map[string]market.CLIToolOverride{
					"list_messages": {
						CLIName: "list",
						Flags: map[string]market.CLIFlagOverride{
							"groupId": {Alias: "group"},
						},
						MutuallyExclusive: [][]string{
							{"group", "does-not-exist"},
						},
					},
				},
			},
		},
	}

	cmds := BuildDynamicCommands(servers, executor.EchoRunner{}, nil)
	list := findChild(cmds[0], "list")
	if list == nil {
		t.Fatal("list leaf not found (constraint validation must not abort build)")
	}
	cmds[0].SetArgs([]string{"list", "--group", "g1"})
	cmds[0].SilenceErrors = true
	cmds[0].SilenceUsage = true
	if err := cmds[0].Execute(); err != nil {
		t.Fatalf("command should run despite skipped constraint, got %v", err)
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
