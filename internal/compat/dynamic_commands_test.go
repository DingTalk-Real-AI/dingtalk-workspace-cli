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

// TestBuildDynamicCommands_PositionalWithFlagAliases verifies that an
// envelope binding marked Positional + Alias + Aliases registers visible
// primary + hidden alias flags AND still accepts the value as a positional
// arg. Mirrors the dws devdoc article search shape:
//   { keyword: { alias: "query", aliases: ["keyword"], positional: true } }
func TestBuildDynamicCommands_PositionalWithFlagAliases(t *testing.T) {
	t.Parallel()

	build := func() (*cobra.Command, *captureRunner) {
		captured := &captureRunner{}
		servers := []market.ServerDescriptor{
			{
				Endpoint: "https://endpoint-devdoc",
				CLI: market.CLIOverlay{
					ID:      "devdoc",
					Command: "devdoc",
					Groups: map[string]market.CLIGroupDef{
						"article": {Description: "文档文章"},
					},
					ToolOverrides: map[string]market.CLIToolOverride{
						"search_open_platform_docs": {
							CLIName: "search",
							Group:   "article",
							Flags: map[string]market.CLIFlagOverride{
								"keyword": {
									Alias:           "query",
									Aliases:         []string{"keyword"},
									Required:        true,
									Positional:      true,
									PositionalIndex: 0,
									Description:     "搜索关键词 (必填)",
								},
							},
						},
					},
				},
			},
		}
		cmds := BuildDynamicCommands(servers, captured, nil)
		article := findChild(cmds[0], "article")
		if article == nil {
			t.Fatal("article group not found")
		}
		search := findChild(article, "search")
		if search == nil {
			t.Fatal("search leaf not found")
		}
		return search, captured
	}

	t.Run("flag registration: visible query + hidden keyword", func(t *testing.T) {
		t.Parallel()
		search, _ := build()
		queryFlag := search.Flags().Lookup("query")
		if queryFlag == nil {
			t.Fatal("--query flag should be registered for dual-mode positional")
		}
		if queryFlag.Hidden {
			t.Fatal("--query flag should be visible (not hidden)")
		}
		keywordFlag := search.Flags().Lookup("keyword")
		if keywordFlag == nil {
			t.Fatal("--keyword hidden alias flag should be registered")
		}
		if !keywordFlag.Hidden {
			t.Fatal("--keyword alias flag should be hidden")
		}
	})

	t.Run("invocation via positional", func(t *testing.T) {
		t.Parallel()
		search, captured := build()
		if err := search.RunE(search, []string{"MCP"}); err != nil {
			t.Fatalf("RunE positional: %v", err)
		}
		if captured.lastParams["keyword"] != "MCP" {
			t.Fatalf("positional: keyword = %v, want MCP", captured.lastParams["keyword"])
		}
	})

	t.Run("invocation via --query primary flag", func(t *testing.T) {
		t.Parallel()
		search, captured := build()
		if err := search.Flags().Set("query", "MCP"); err != nil {
			t.Fatalf("Set --query: %v", err)
		}
		if err := search.RunE(search, nil); err != nil {
			t.Fatalf("RunE --query: %v", err)
		}
		if captured.lastParams["keyword"] != "MCP" {
			t.Fatalf("--query: keyword = %v, want MCP", captured.lastParams["keyword"])
		}
	})

	t.Run("invocation via --keyword hidden alias", func(t *testing.T) {
		t.Parallel()
		search, captured := build()
		if err := search.Flags().Set("keyword", "MCP"); err != nil {
			t.Fatalf("Set --keyword: %v", err)
		}
		if err := search.RunE(search, nil); err != nil {
			t.Fatalf("RunE --keyword: %v", err)
		}
		if captured.lastParams["keyword"] != "MCP" {
			t.Fatalf("--keyword: keyword = %v, want MCP", captured.lastParams["keyword"])
		}
	})

	t.Run("flag wins over positional when both supplied", func(t *testing.T) {
		t.Parallel()
		search, captured := build()
		if err := search.Flags().Set("query", "FROM_FLAG"); err != nil {
			t.Fatalf("Set --query: %v", err)
		}
		if err := search.RunE(search, []string{"FROM_POSITIONAL"}); err != nil {
			t.Fatalf("RunE both: %v", err)
		}
		if captured.lastParams["keyword"] != "FROM_FLAG" {
			t.Fatalf("flag should win: keyword = %v, want FROM_FLAG", captured.lastParams["keyword"])
		}
	})

	t.Run("missing input returns validation error", func(t *testing.T) {
		t.Parallel()
		search, _ := build()
		err := search.RunE(search, nil)
		if err == nil {
			t.Fatal("expected validation error when neither flag nor positional was supplied")
		}
		msg := err.Error()
		if !strings.Contains(msg, "--query") || !strings.Contains(msg, "keyword") {
			t.Fatalf("error message %q should reference both --query and keyword", msg)
		}
	})

	t.Run("arity allows zero args (relaxed for dual-mode)", func(t *testing.T) {
		t.Parallel()
		search, _ := build()
		// Args validator should accept zero args because --query / --keyword
		// can satisfy the requirement; the validation step in RunE catches
		// the truly-missing case.
		if err := search.Args(search, []string{}); err != nil {
			t.Fatalf("Args([]) should not error for dual-mode positional: %v", err)
		}
		if err := search.Args(search, []string{"MCP"}); err != nil {
			t.Fatalf("Args([MCP]) should not error: %v", err)
		}
		if err := search.Args(search, []string{"MCP", "extra"}); err == nil {
			t.Fatal("Args should cap at totalMax=1, extra arg should error")
		}
	})
}

// TestBuildDynamicCommands_PositionalArityMixed verifies that mixing pure
// positional (required) with dual-mode positional (with flag aliases) yields
// a RangeArgs validator: pure positional enforces the minimum, dual-mode
// extends the maximum.
func TestBuildDynamicCommands_PositionalArityMixed(t *testing.T) {
	t.Parallel()

	servers := []market.ServerDescriptor{
		{
			Endpoint: "https://endpoint-mixed",
			CLI: market.CLIOverlay{
				ID:      "mixed",
				Command: "mixed",
				ToolOverrides: map[string]market.CLIToolOverride{
					"do_thing": {
						CLIName: "do",
						Flags: map[string]market.CLIFlagOverride{
							// pure positional, required: enforces MinimumNArgs(1)
							"target": {
								Positional:      true,
								PositionalIndex: 0,
								Required:        true,
							},
							// dual-mode positional at slot 1: extends totalMax to 2
							"label": {
								Alias:           "label",
								Aliases:         []string{"name"},
								Positional:      true,
								PositionalIndex: 1,
								Required:        true,
							},
						},
					},
				},
			},
		},
	}

	cmds := BuildDynamicCommands(servers, executor.EchoRunner{}, nil)
	leaf := findChild(cmds[0], "do")
	if leaf == nil {
		t.Fatal("do leaf not found")
	}

	// 0 args → arity error (pure positional 'target' missing)
	if err := leaf.Args(leaf, []string{}); err == nil {
		t.Fatal("Args([]) should error: pure positional 'target' is required")
	}
	// 1 arg → fills target, label can come from --label flag
	if err := leaf.Args(leaf, []string{"T"}); err != nil {
		t.Fatalf("Args([T]) should succeed: %v", err)
	}
	// 2 args → both positionals satisfied
	if err := leaf.Args(leaf, []string{"T", "L"}); err != nil {
		t.Fatalf("Args([T, L]) should succeed: %v", err)
	}
	// 3 args → exceeds totalMax=2
	if err := leaf.Args(leaf, []string{"T", "L", "extra"}); err == nil {
		t.Fatal("Args([T, L, extra]) should error: totalMax=2")
	}

	// 1 arg + --label flag should satisfy the validation step in RunE.
	if err := leaf.Flags().Set("label", "L_FROM_FLAG"); err != nil {
		t.Fatalf("Set --label: %v", err)
	}
	if err := leaf.RunE(leaf, []string{"T"}); err != nil {
		t.Fatalf("RunE([T]) with --label flag: %v", err)
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
							"groupId":        {Alias: "group"},
							"userId":         {Alias: "user"},
							"openDingtalkId": {Alias: "open-dingtalk-id"},
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

// TestBuildDynamicCommands_MultipleAliases_PrimarySet verifies the primary
// flag still works when a binding declares extra hidden aliases.
func TestBuildDynamicCommands_MultipleAliases_PrimarySet(t *testing.T) {
	t.Parallel()

	runner := &captureRunner{}
	servers := []market.ServerDescriptor{
		{
			Endpoint: "https://endpoint-contact",
			CLI: market.CLIOverlay{
				ID:      "contact",
				Command: "contact",
				ToolOverrides: map[string]market.CLIToolOverride{
					"search_contact_by_key_word": {
						CLIName: "search",
						Flags: map[string]market.CLIFlagOverride{
							"keyword": {
								Alias:    "query",
								Aliases:  []string{"keyword"},
								Required: true,
							},
						},
					},
				},
			},
		},
	}

	cmds := BuildDynamicCommands(servers, runner, nil)
	cmds[0].SetArgs([]string{"search", "--query", "hello"})
	cmds[0].SilenceErrors = true
	cmds[0].SilenceUsage = true
	if err := cmds[0].Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if runner.lastParams["keyword"] != "hello" {
		t.Fatalf("expected keyword=hello, got %+v", runner.lastParams)
	}

	// The hidden alias must exist on the leaf command but be marked hidden.
	leaf := findChild(cmds[0], "search")
	if leaf == nil {
		t.Fatal("search leaf missing")
	}
	alias := leaf.Flags().Lookup("keyword")
	if alias == nil {
		t.Fatal("hidden alias --keyword not registered")
	}
	if !alias.Hidden {
		t.Fatalf("--keyword must be hidden (got Hidden=false)")
	}
	if primary := leaf.Flags().Lookup("query"); primary == nil || primary.Hidden {
		t.Fatalf("--query must be registered and visible (got %+v)", primary)
	}
}

// TestBuildDynamicCommands_MultipleAliases_OnlyAliasSet verifies that
// passing only a hidden alias satisfies Required and routes the value to
// params[Property]. Regression: pre-fix envelope rejected --keyword with
// "unknown flag".
func TestBuildDynamicCommands_MultipleAliases_OnlyAliasSet(t *testing.T) {
	t.Parallel()

	runner := &captureRunner{}
	servers := []market.ServerDescriptor{
		{
			Endpoint: "https://endpoint-contact",
			CLI: market.CLIOverlay{
				ID:      "contact",
				Command: "contact",
				ToolOverrides: map[string]market.CLIToolOverride{
					"search_contact_by_key_word": {
						CLIName: "search",
						Flags: map[string]market.CLIFlagOverride{
							"keyword": {
								Alias:    "query",
								Aliases:  []string{"keyword"},
								Required: true,
							},
						},
					},
				},
			},
		},
	}

	cmds := BuildDynamicCommands(servers, runner, nil)
	cmds[0].SetArgs([]string{"search", "--keyword", "hi"})
	cmds[0].SilenceErrors = true
	cmds[0].SilenceUsage = true
	if err := cmds[0].Execute(); err != nil {
		t.Fatalf("execute with hidden alias: %v", err)
	}
	if runner.lastParams["keyword"] != "hi" {
		t.Fatalf("expected keyword=hi, got %+v", runner.lastParams)
	}
}

// TestBuildDynamicCommands_MultipleAliases_RequiredErrorWhenNoneSet verifies
// the self-check fallback: when Required binding has aliases but none are
// supplied, CollectBindings emits "--<primary> is required".
func TestBuildDynamicCommands_MultipleAliases_RequiredErrorWhenNoneSet(t *testing.T) {
	t.Parallel()

	runner := &captureRunner{}
	servers := []market.ServerDescriptor{
		{
			Endpoint: "https://endpoint-contact",
			CLI: market.CLIOverlay{
				ID:      "contact",
				Command: "contact",
				ToolOverrides: map[string]market.CLIToolOverride{
					"search_contact_by_key_word": {
						CLIName: "search",
						Flags: map[string]market.CLIFlagOverride{
							"keyword": {
								Alias:    "query",
								Aliases:  []string{"keyword"},
								Required: true,
							},
						},
					},
				},
			},
		},
	}

	cmds := BuildDynamicCommands(servers, runner, nil)
	cmds[0].SetArgs([]string{"search"})
	cmds[0].SilenceErrors = true
	cmds[0].SilenceUsage = true
	err := cmds[0].Execute()
	if err == nil {
		t.Fatal("expected required error, got nil")
	}
	if !strings.Contains(err.Error(), "--query is required") {
		t.Fatalf("expected --query is required, got %v", err)
	}
}

// TestBuildDynamicCommands_MultipleAliases_PrimaryWinsWhenBothSet verifies
// that when both the primary and an alias are provided on the CLI, the
// primary wins (matches cmdutil.FlagOrFallback precedence: primary first).
func TestBuildDynamicCommands_MultipleAliases_PrimaryWinsWhenBothSet(t *testing.T) {
	t.Parallel()

	runner := &captureRunner{}
	servers := []market.ServerDescriptor{
		{
			Endpoint: "https://endpoint-contact",
			CLI: market.CLIOverlay{
				ID:      "contact",
				Command: "contact",
				ToolOverrides: map[string]market.CLIToolOverride{
					"search_contact_by_key_word": {
						CLIName: "search",
						Flags: map[string]market.CLIFlagOverride{
							"keyword": {
								Alias:   "query",
								Aliases: []string{"keyword"},
							},
						},
					},
				},
			},
		},
	}

	cmds := BuildDynamicCommands(servers, runner, nil)
	cmds[0].SetArgs([]string{"search", "--query", "primary", "--keyword", "fallback"})
	cmds[0].SilenceErrors = true
	cmds[0].SilenceUsage = true
	if err := cmds[0].Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if runner.lastParams["keyword"] != "primary" {
		t.Fatalf("expected primary to win (keyword=primary), got %+v", runner.lastParams)
	}
}

// TestBuildDynamicCommands_MultipleAliases_MultiAliasChain verifies a chain
// of 3+ aliases resolves the value from the first one that is set.
func TestBuildDynamicCommands_MultipleAliases_MultiAliasChain(t *testing.T) {
	t.Parallel()

	runner := &captureRunner{}
	servers := []market.ServerDescriptor{
		{
			Endpoint: "https://endpoint-contact",
			CLI: market.CLIOverlay{
				ID:      "contact",
				Command: "contact",
				ToolOverrides: map[string]market.CLIToolOverride{
					"get_user_info_by_user_ids": {
						CLIName: "get",
						Flags: map[string]market.CLIFlagOverride{
							"user_id_list": {
								Alias:     "ids",
								Aliases:   []string{"user-id", "user-ids"},
								Required:  true,
								Transform: "csv_to_array",
							},
						},
					},
				},
			},
		},
	}

	cmds := BuildDynamicCommands(servers, runner, nil)
	cmds[0].SetArgs([]string{"get", "--user-ids", "u1,u2"})
	cmds[0].SilenceErrors = true
	cmds[0].SilenceUsage = true
	if err := cmds[0].Execute(); err != nil {
		t.Fatalf("execute with --user-ids: %v", err)
	}
	got, ok := runner.lastParams["user_id_list"].([]any)
	if !ok {
		t.Fatalf("expected []any for user_id_list after csv_to_array, got %T (%+v)", runner.lastParams["user_id_list"], runner.lastParams)
	}
	if len(got) != 2 || got[0] != "u1" || got[1] != "u2" {
		t.Fatalf("expected [u1 u2], got %+v", got)
	}

	// All three hidden aliases must be registered and hidden, primary visible.
	leaf := findChild(cmds[0], "get")
	if leaf == nil {
		t.Fatal("get leaf missing")
	}
	for _, name := range []string{"user-id", "user-ids"} {
		f := leaf.Flags().Lookup(name)
		if f == nil {
			t.Fatalf("alias --%s not registered", name)
		}
		if !f.Hidden {
			t.Fatalf("alias --%s must be hidden", name)
		}
	}
	if p := leaf.Flags().Lookup("ids"); p == nil || p.Hidden {
		t.Fatalf("primary --ids must be visible")
	}
}

// TestBuildDynamicCommands_MultipleAliases_Dedup verifies that reserved
// names ("json", "params"), duplicates of the primary, duplicates of the
// single Alias, and intra-slice duplicates are all silently skipped so
// cobra never double-registers a flag.
func TestBuildDynamicCommands_MultipleAliases_Dedup(t *testing.T) {
	t.Parallel()

	runner := &captureRunner{}
	servers := []market.ServerDescriptor{
		{
			Endpoint: "https://endpoint-contact",
			CLI: market.CLIOverlay{
				ID:      "contact",
				Command: "contact",
				ToolOverrides: map[string]market.CLIToolOverride{
					"search_contact_by_key_word": {
						CLIName: "search",
						Flags: map[string]market.CLIFlagOverride{
							"keyword": {
								Alias: "query",
								// Conflicts: query dup with alias, json/params are
								// reserved, keyword appears twice.
								Aliases: []string{"query", "json", "params", "keyword", "keyword"},
							},
						},
					},
				},
			},
		},
	}

	// If ApplyBindings panics (duplicate pflag) we fail. Otherwise the cmd
	// should build and execute fine.
	cmds := BuildDynamicCommands(servers, runner, nil)
	cmds[0].SetArgs([]string{"search", "--keyword", "ok"})
	cmds[0].SilenceErrors = true
	cmds[0].SilenceUsage = true
	if err := cmds[0].Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if runner.lastParams["keyword"] != "ok" {
		t.Fatalf("expected keyword=ok, got %+v", runner.lastParams)
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

// envelope-help-alignment ----------------------------------------------------
//
// The next four tests cover the envelope-driven --help upgrades so dws-wukong
// can ship hardcoded-equivalent --help text purely through Diamond config.

// TestBuildDynamicCommands_ExampleField verifies CLIToolOverride.Example flows
// to cobra.Command.Example, surfacing the "Examples:" section in --help.
func TestBuildDynamicCommands_ExampleField(t *testing.T) {
	t.Parallel()

	const want = "  dws oa approval list-forms --cursor 0 --size 100"

	servers := []market.ServerDescriptor{
		{
			Endpoint: "https://endpoint-oa",
			CLI: market.CLIOverlay{
				ID:      "oa",
				Command: "oa",
				ToolOverrides: map[string]market.CLIToolOverride{
					"list_user_visible_process": {
						CLIName: "list-forms",
						Group:   "approval",
						Example: "  dws oa approval list-forms --cursor 0 --size 100",
					},
				},
			},
		},
	}

	cmds := BuildDynamicCommands(servers, executor.EchoRunner{}, nil)
	approval := findChild(cmds[0], "approval")
	if approval == nil {
		t.Fatal("approval group not found")
	}
	leaf := findChild(approval, "list-forms")
	if leaf == nil {
		t.Fatal("list-forms leaf not found")
	}
	if leaf.Example != want {
		t.Fatalf("Example mismatch:\n want: %q\n got:  %q", want, leaf.Example)
	}
	if !strings.Contains(leaf.UsageString(), "Examples:") {
		t.Fatalf("expected 'Examples:' section in --help; usage:\n%s", leaf.UsageString())
	}
}

// TestApplyBindings_VisibleFlagDefault_String verifies that
// CLIFlagOverride.Default flows to cobra String flags so --help shows
// (default "0"). Hidden-only behavior is unchanged; this is the path that was
// previously dead code for visible flags.
func TestApplyBindings_VisibleFlagDefault_String(t *testing.T) {
	t.Parallel()

	servers := []market.ServerDescriptor{
		{
			Endpoint: "https://endpoint-oa",
			CLI: market.CLIOverlay{
				ID:      "oa",
				Command: "oa",
				ToolOverrides: map[string]market.CLIToolOverride{
					"list_user_visible_process": {
						CLIName: "list-forms",
						Flags: map[string]market.CLIFlagOverride{
							"cursor": {
								Alias:   "cursor",
								Type:    "string",
								Default: "0",
							},
						},
					},
				},
			},
		},
	}

	cmds := BuildDynamicCommands(servers, executor.EchoRunner{}, nil)
	leaf := findChild(cmds[0], "list-forms")
	if leaf == nil {
		t.Fatal("list-forms leaf not found")
	}
	f := leaf.Flags().Lookup("cursor")
	if f == nil {
		t.Fatal("--cursor flag missing")
	}
	if f.DefValue != "0" {
		t.Fatalf("expected DefValue=\"0\", got %q", f.DefValue)
	}
	usage := leaf.UsageString()
	if !strings.Contains(usage, `(default "0")`) {
		t.Fatalf("expected --help to contain (default \"0\"); got:\n%s", usage)
	}
}

// TestApplyBindings_VisibleFlagDefault_Int verifies the Int kind path: cobra
// renders int defaults without quotes, so we look for "(default 100)".
func TestApplyBindings_VisibleFlagDefault_Int(t *testing.T) {
	t.Parallel()

	servers := []market.ServerDescriptor{
		{
			Endpoint: "https://endpoint-oa",
			CLI: market.CLIOverlay{
				ID:      "oa",
				Command: "oa",
				ToolOverrides: map[string]market.CLIToolOverride{
					"list_user_visible_process": {
						CLIName: "list-forms",
						Flags: map[string]market.CLIFlagOverride{
							"pageSize": {
								Alias:   "size",
								Type:    "int",
								Default: "100",
							},
						},
					},
				},
			},
		},
	}

	cmds := BuildDynamicCommands(servers, executor.EchoRunner{}, nil)
	leaf := findChild(cmds[0], "list-forms")
	if leaf == nil {
		t.Fatal("list-forms leaf not found")
	}
	f := leaf.Flags().Lookup("size")
	if f == nil {
		t.Fatal("--size flag missing")
	}
	if f.DefValue != "100" {
		t.Fatalf("expected DefValue=\"100\", got %q", f.DefValue)
	}
	if !strings.Contains(leaf.UsageString(), "(default 100)") {
		t.Fatalf("expected --help to contain (default 100); got:\n%s", leaf.UsageString())
	}
}

// TestCollectBindings_DefaultDoesNotImplyChanged guards the lower half of
// the v3.2 contract: CollectBindings itself must not infer "user changed
// this flag" from the presence of an envelope Default. Default-driven MCP
// body injection lives in buildOverrideBindings' normalizer (verified by the
// TestNormalizer_* tests below) so user-vs-default attribution stays clean
// — `firstChangedFlag` is the single source of truth for "user typed it".
func TestCollectBindings_DefaultDoesNotImplyChanged(t *testing.T) {
	t.Parallel()

	bindings := []FlagBinding{
		{
			FlagName: "cursor",
			Property: "cursor",
			Kind:     ValueString,
			Default:  "0",
			Usage:    "page cursor",
		},
	}
	cmd := &cobra.Command{Use: "list-forms"}
	ApplyBindings(cmd, bindings)

	// Sanity: cobra received the default and would render it in --help.
	if got := cmd.Flags().Lookup("cursor").DefValue; got != "0" {
		t.Fatalf("expected DefValue=\"0\" wired to cobra, got %q", got)
	}

	// Don't call cmd.SetArgs / cmd.Execute — i.e. user typed nothing.
	params, err := CollectBindings(cmd, bindings, nil)
	if err != nil {
		t.Fatalf("CollectBindings returned error: %v", err)
	}
	if _, ok := params["cursor"]; ok {
		t.Fatalf("default-only flag must not appear in MCP params; got %v", params)
	}
}

// TestNormalizer_VisibleFlagDefault_InjectsWhenOmitted is the v3.2 mirror of
// the test above: CollectBindings still skips, but the normalizer returned
// by buildOverrideBindings must inject the envelope default for visible
// flags so the MCP body matches the hardcoded helper command's behavior
// (`mustGetFlag(cobra default) → body`). This is the regression that broke
// `dws oa approval list-forms` when the user omitted --cursor / --size.
func TestNormalizer_VisibleFlagDefault_InjectsWhenOmitted(t *testing.T) {
	t.Parallel()

	override := market.CLIToolOverride{
		CLIName: "list-forms",
		Flags: map[string]market.CLIFlagOverride{
			"cursor": {
				Alias:   "cursor",
				Type:    "int",
				Default: "0",
			},
			"pageSize": {
				Alias:   "size",
				Type:    "int",
				Default: "100",
			},
		},
	}

	bindings, normalizer := buildOverrideBindings(override)
	if normalizer == nil {
		t.Fatal("expected non-nil normalizer when defaults are present")
	}
	cmd := &cobra.Command{Use: "list-forms"}
	ApplyBindings(cmd, bindings)

	params, err := CollectBindings(cmd, bindings, nil)
	if err != nil {
		t.Fatalf("CollectBindings returned error: %v", err)
	}
	if err := normalizer(cmd, params); err != nil {
		t.Fatalf("normalizer returned error: %v", err)
	}

	if got, ok := params["cursor"].(int); !ok || got != 0 {
		t.Fatalf("expected params[cursor] = int(0), got %T(%v)", params["cursor"], params["cursor"])
	}
	if got, ok := params["pageSize"].(int); !ok || got != 100 {
		t.Fatalf("expected params[pageSize] = int(100), got %T(%v)", params["pageSize"], params["pageSize"])
	}
}

// TestNormalizer_DefaultCoercedByKind covers every ValueKind the envelope
// type-name dictionary (kindFromTypeName) can actually produce today —
// string / int / bool / string_slice — so a number-typed schema receives a
// number (not the raw envelope string) and slice kinds get a []string.
// Catches regressions in the parseFlagDefault dispatch.
//
// ValueFloat and the *_slice numeric kinds aren't reachable from envelope
// `type` strings yet, but the normalizer still handles them for forward
// compatibility (asserted indirectly by the build).
func TestNormalizer_DefaultCoercedByKind(t *testing.T) {
	t.Parallel()

	override := market.CLIToolOverride{
		CLIName: "demo",
		Flags: map[string]market.CLIFlagOverride{
			"name":    {Alias: "name", Type: "string", Default: "alice"},
			"page":    {Alias: "page", Type: "int", Default: "7"},
			"verbose": {Alias: "verbose", Type: "bool", Default: "true"},
			"tags":    {Alias: "tags", Type: "string_slice", Default: "a,b,c"},
		},
	}

	bindings, normalizer := buildOverrideBindings(override)
	if normalizer == nil {
		t.Fatal("expected non-nil normalizer when defaults are present")
	}
	cmd := &cobra.Command{Use: "demo"}
	ApplyBindings(cmd, bindings)

	params, err := CollectBindings(cmd, bindings, nil)
	if err != nil {
		t.Fatalf("CollectBindings returned error: %v", err)
	}
	if err := normalizer(cmd, params); err != nil {
		t.Fatalf("normalizer returned error: %v", err)
	}

	if got, ok := params["name"].(string); !ok || got != "alice" {
		t.Fatalf("name: expected string \"alice\", got %T(%v)", params["name"], params["name"])
	}
	if got, ok := params["page"].(int); !ok || got != 7 {
		t.Fatalf("page: expected int 7, got %T(%v)", params["page"], params["page"])
	}
	if got, ok := params["verbose"].(bool); !ok || !got {
		t.Fatalf("verbose: expected bool true, got %T(%v)", params["verbose"], params["verbose"])
	}
	got, ok := params["tags"].([]string)
	if !ok {
		t.Fatalf("tags: expected []string, got %T(%v)", params["tags"], params["tags"])
	}
	if strings.Join(got, ",") != "a,b,c" {
		t.Fatalf("tags: expected [a b c], got %v", got)
	}
}

// TestNormalizer_DefaultDoesNotOverrideUserValue locks in the
// `if exists continue` guard so user input always beats the envelope default,
// matching CollectBindings' user-changed-flag wins contract.
func TestNormalizer_DefaultDoesNotOverrideUserValue(t *testing.T) {
	t.Parallel()

	override := market.CLIToolOverride{
		CLIName: "list-forms",
		Flags: map[string]market.CLIFlagOverride{
			"cursor": {
				Alias:   "cursor",
				Type:    "int",
				Default: "0",
			},
		},
	}

	bindings, normalizer := buildOverrideBindings(override)
	if normalizer == nil {
		t.Fatal("expected non-nil normalizer when defaults are present")
	}
	cmd := &cobra.Command{Use: "list-forms"}
	ApplyBindings(cmd, bindings)

	if err := cmd.ParseFlags([]string{"--cursor", "5"}); err != nil {
		t.Fatalf("ParseFlags returned error: %v", err)
	}

	params, err := CollectBindings(cmd, bindings, nil)
	if err != nil {
		t.Fatalf("CollectBindings returned error: %v", err)
	}
	if err := normalizer(cmd, params); err != nil {
		t.Fatalf("normalizer returned error: %v", err)
	}

	if got, ok := params["cursor"].(int); !ok || got != 5 {
		t.Fatalf("expected params[cursor] = int(5), got %T(%v)", params["cursor"], params["cursor"])
	}
}
