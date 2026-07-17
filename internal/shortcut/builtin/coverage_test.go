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

package builtin_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/builtin"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

// fakeCaller implements edition.ToolCaller. It records the (product, tool, args)
// of the last CallTool so the coverage test can assert what MCP call a shortcut
// assembled — WITHOUT any network I/O or side effects. This lets us exercise
// every shortcut, including write/delete commands, safely.
type fakeCaller struct {
	called  bool
	product string
	tool    string
	args    map[string]any
	dryRun  bool
	calls   []toolCall
	results map[string]string
}

type toolCall struct {
	product string
	tool    string
	args    map[string]any
}

func (f *fakeCaller) reset() {
	f.called, f.product, f.tool, f.args = false, "", "", nil
	f.calls = nil
}

func (f *fakeCaller) CallTool(_ context.Context, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	f.called, f.product, f.tool, f.args = true, product, tool, args
	f.calls = append(f.calls, toolCall{product: product, tool: tool, args: args})
	text := `{"ok":true}`
	if f.results != nil {
		if v, ok := f.results[product+"/"+tool]; ok {
			text = v
		} else if v, ok := f.results[tool]; ok {
			text = v
		}
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: text}}}, nil
}
func (f *fakeCaller) Format() string { return "json" }
func (f *fakeCaller) DryRun() bool   { return f.dryRun }
func (f *fakeCaller) Fields() string { return "" }
func (f *fakeCaller) JQ() string     { return "" }

// realToolSet scans the helper sources (the ground truth) and returns the set of
// snake_case identifiers found there. Every tool a shortcut invokes must appear
// in this set; anything else would be a hallucinated tool name.
func realToolSet(t *testing.T) map[string]bool {
	t.Helper()
	files, err := filepath.Glob("../../../internal/helpers/*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("cannot locate helper sources: %v (found %d)", err, len(files))
	}
	// Two oracles unioned:
	//   - every ASCII word token in the helper sources (guarantees any
	//     snake_case / camelCase tool name present anywhere is captured);
	//   - quoted literals containing CJK, to cover Chinese const tool names
	//     (e.g. minutes' "执行听记指令-发起AI听记录音").
	wordRe := regexp.MustCompile(`[A-Za-z][A-Za-z0-9_]{2,}`)
	cjkRe := regexp.MustCompile(`"([^"]*\p{Han}[^"]*)"`)
	set := map[string]bool{}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		s := string(data)
		for _, m := range wordRe.FindAllString(s, -1) {
			set[m] = true
		}
		for _, m := range cjkRe.FindAllStringSubmatch(s, -1) {
			set[m[1]] = true
		}
	}
	return set
}

// synthArgs builds a plausible argument vector for a shortcut: a value for every
// declared flag (enum flags get their first allowed value), plus --yes to skip
// confirmation on write/high-risk commands.
func synthArgs(s shortcut.Shortcut) []string {
	args := []string{s.Service, s.Command}
	for _, f := range s.Flags {
		switch {
		case len(f.Enum) > 0:
			args = append(args, "--"+f.Name, f.Enum[0])
		case f.Type == shortcut.FlagBool:
			args = append(args, "--"+f.Name)
		case f.Type == shortcut.FlagInt:
			args = append(args, "--"+f.Name, "1")
		default: // string / string_slice
			args = append(args, "--"+f.Name, "x")
		}
	}
	return args
}

// TestAllShortcutsAssemble drives every registered shortcut end-to-end through
// the cobra tree with synthesized inputs and asserts each one is healthy:
//   - it either assembles an MCP call with a real (non-hallucinated) tool name,
//   - or it is rejected by its own validation (proving the plumbing ran),
//   - and it never panics or silently no-ops.
func TestAllShortcutsAssemble(t *testing.T) {
	fake := &fakeCaller{}
	helpers.InitDeps(fake)

	root := &cobra.Command{Use: "dws", SilenceUsage: true, SilenceErrors: true}
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	root.AddCommand(builtin.Commands()...)

	real := realToolSet(t)
	all := shortcut.All()
	if len(all) == 0 {
		t.Fatal("no shortcuts registered")
	}

	var assembled, validated, failed int
	var validatedNames []string
	for _, s := range all {
		name := s.Service + " " + s.Command
		args := append(synthArgs(s), "--yes")

		fake.reset()
		err := func() (e error) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("PANIC in %s: %v", name, r)
					e = nil
				}
			}()
			root.SetArgs(args)
			return root.Execute()
		}()

		switch {
		case fake.called:
			assembled++
			if fake.tool == "" {
				t.Errorf("%s: assembled call with EMPTY tool name", name)
			} else if !real[fake.tool] {
				t.Errorf("%s: tool %q not found in any helper (hallucinated?)", name, fake.tool)
			}
			if fake.product == "" {
				t.Errorf("%s: assembled call with EMPTY product", name)
			}
		case err != nil:
			// Rejected by required/enum/Validate before dispatch — plumbing OK.
			validated++
			validatedNames = append(validatedNames, s.Service+s.Command)
		default:
			failed++
			t.Errorf("%s: returned nil error but never assembled an MCP call (dead command)", name)
		}
	}

	t.Logf("shortcuts=%d  assembled(真实MCP)=%d  validated(自校验拦截)=%d  failed=%d",
		len(all), assembled, validated, failed)
	t.Logf("validated(自校验拦截) 明细: %s", strings.Join(validatedNames, " "))
	if assembled == 0 {
		t.Fatal("no shortcut assembled an MCP call — harness likely broken")
	}
}

func TestReplaceBatchDryRunDoesNotCallTool(t *testing.T) {
	fake := &fakeCaller{dryRun: true}
	helpers.InitDeps(fake)

	root := &cobra.Command{Use: "dws", SilenceUsage: true, SilenceErrors: true}
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	root.AddCommand(builtin.Commands()...)
	root.SetArgs([]string{
		"minutes", "+replace-batch", "--id", "task-1",
		"--pair", "Q2=>第二季度", "--dry-run", "--yes",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if fake.called {
		t.Fatalf("dry-run called real tool %s/%s with %#v", fake.product, fake.tool, fake.args)
	}
}

func TestDMAddsAIClawTypeByDefault(t *testing.T) {
	fake := &fakeCaller{results: map[string]string{
		"contact/search_contact_by_key_word": `{"result":[{"userId":"u1","name":"张三","openDingTalkId":"open1"}]}`,
	}}
	helpers.InitDeps(fake)

	root := &cobra.Command{Use: "dws", SilenceUsage: true, SilenceErrors: true}
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	root.AddCommand(builtin.Commands()...)
	root.SetArgs([]string{"chat", "+dm", "--to", "张三", "--text", "你好", "--yes"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) < 2 {
		t.Fatalf("calls = %d, want contact lookup then message send", len(fake.calls))
	}
	send := fake.calls[len(fake.calls)-1]
	if send.product != "chat" || send.tool != "send_personal_message" {
		t.Fatalf("last call = %s/%s, want chat/send_personal_message", send.product, send.tool)
	}
	if got := send.args["clawType"]; got != edition.ClawType() {
		t.Fatalf("clawType = %#v, want %q", got, edition.ClawType())
	}
}

func TestDMCanDisableAIClawType(t *testing.T) {
	fake := &fakeCaller{results: map[string]string{
		"contact/search_contact_by_key_word": `{"result":[{"userId":"u1","name":"张三","openDingTalkId":"open1"}]}`,
	}}
	helpers.InitDeps(fake)

	root := &cobra.Command{Use: "dws", SilenceUsage: true, SilenceErrors: true}
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	root.AddCommand(builtin.Commands()...)
	root.SetArgs([]string{"chat", "+dm", "--to", "张三", "--text", "你好", "--ai-tag=false", "--yes"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	send := fake.calls[len(fake.calls)-1]
	if _, ok := send.args["clawType"]; ok {
		t.Fatalf("clawType unexpectedly present when --ai-tag=false: %#v", send.args["clawType"])
	}
}

func TestChatSearchAcceptsKeywordAlias(t *testing.T) {
	fake := &fakeCaller{}
	helpers.InitDeps(fake)

	root := &cobra.Command{Use: "dws", SilenceUsage: true, SilenceErrors: true}
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	root.AddCommand(builtin.Commands()...)
	root.SetArgs([]string{"chat", "+chat-search", "--keyword", "树莓派", "--size", "5", "--yes"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if fake.product != "im" || fake.tool != "search_groups" {
		t.Fatalf("call = %s/%s, want im/search_groups", fake.product, fake.tool)
	}
	if got := fake.args["keyword"]; got != "树莓派" {
		t.Fatalf("keyword = %#v, want 树莓派", got)
	}
	if got := fake.args["limit"]; got != 5 {
		t.Fatalf("limit = %#v, want 5", got)
	}
}

func TestBotFindAcceptsKeywordAlias(t *testing.T) {
	fake := &fakeCaller{}
	helpers.InitDeps(fake)

	root := &cobra.Command{Use: "dws", SilenceUsage: true, SilenceErrors: true}
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	root.AddCommand(builtin.Commands()...)
	root.SetArgs([]string{"chat", "+bot-find", "--keyword", "日报", "--yes"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if fake.product != "bot" || fake.tool != "search_bots" {
		t.Fatalf("call = %s/%s, want bot/search_bots", fake.product, fake.tool)
	}
	if got := fake.args["keyword"]; got != "日报" {
		t.Fatalf("keyword = %#v, want 日报", got)
	}
}

func TestMessagesListAcceptsConversationIDAndSizeAliases(t *testing.T) {
	fake := &fakeCaller{}
	helpers.InitDeps(fake)

	root := &cobra.Command{Use: "dws", SilenceUsage: true, SilenceErrors: true}
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	root.AddCommand(builtin.Commands()...)
	root.SetArgs([]string{
		"chat", "+messages-list",
		"--conversation-id", "cid-1",
		"--time", "2026-07-17 10:00:00",
		"--size", "7",
		"--yes",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if fake.product != "chat" || fake.tool != "list_conversation_message_v2" {
		t.Fatalf("call = %s/%s, want chat/list_conversation_message_v2", fake.product, fake.tool)
	}
	if got := fake.args["openCid"]; got != "cid-1" {
		t.Fatalf("openCid = %#v, want cid-1", got)
	}
	if got := fake.args["limit"]; got != 7 {
		t.Fatalf("limit = %#v, want 7", got)
	}
}

func TestMessagesReadStatusAcceptsIDAlias(t *testing.T) {
	fake := &fakeCaller{}
	helpers.InitDeps(fake)

	root := &cobra.Command{Use: "dws", SilenceUsage: true, SilenceErrors: true}
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	root.AddCommand(builtin.Commands()...)
	root.SetArgs([]string{
		"chat", "+messages-read-status",
		"--id", "cid-1",
		"--message-id", "msg-1",
		"--yes",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if fake.product != "im" || fake.tool != "query_msg_read_status" {
		t.Fatalf("call = %s/%s, want im/query_msg_read_status", fake.product, fake.tool)
	}
	if got := fake.args["openConversationId"]; got != "cid-1" {
		t.Fatalf("openConversationId = %#v, want cid-1", got)
	}
}

func TestSearchMsgAcceptsIDAndKeywordAliases(t *testing.T) {
	fake := &fakeCaller{}
	helpers.InitDeps(fake)

	root := &cobra.Command{Use: "dws", SilenceUsage: true, SilenceErrors: true}
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	root.AddCommand(builtin.Commands()...)
	root.SetArgs([]string{"chat", "+search-msg", "--id", "cid-1", "--keyword", "树莓派", "--yes"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if fake.product != "chat" || fake.tool != "search_messages_by_keyword" {
		t.Fatalf("call = %s/%s, want chat/search_messages_by_keyword", fake.product, fake.tool)
	}
	if got := fake.args["openConversationId"]; got != "cid-1" {
		t.Fatalf("openConversationId = %#v, want cid-1", got)
	}
	if got := fake.args["keyword"]; got != "树莓派" {
		t.Fatalf("keyword = %#v, want 树莓派", got)
	}
}

func TestChatMessagesAcceptsIDAndSizeAliases(t *testing.T) {
	fake := &fakeCaller{}
	helpers.InitDeps(fake)

	root := &cobra.Command{Use: "dws", SilenceUsage: true, SilenceErrors: true}
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	root.AddCommand(builtin.Commands()...)
	root.SetArgs([]string{"chat", "+chat-messages", "--id", "cid-1", "--size", "9", "--yes"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if fake.product != "chat" || fake.tool != "list_conversation_message_v2" {
		t.Fatalf("call = %s/%s, want chat/list_conversation_message_v2", fake.product, fake.tool)
	}
	if got := fake.args["openCid"]; got != "cid-1" {
		t.Fatalf("openCid = %#v, want cid-1", got)
	}
	if got := fake.args["limit"]; got != 9 {
		t.Fatalf("limit = %#v, want 9", got)
	}
}

// TestAllToolLiteralsAreReal statically scans every shortcut package source for
// CallMCP("tool", ...) literals and asserts each tool exists in the helper
// ground truth. This covers the ~55 shortcuts whose own validation blocks the
// runtime assemble path, so no shortcut's tool name goes unverified.
func TestAllToolLiteralsAreReal(t *testing.T) {
	real := realToolSet(t)
	srcs, err := filepath.Glob("../*/*.go")
	if err != nil || len(srcs) == 0 {
		t.Fatalf("cannot locate shortcut sources: %v", err)
	}
	// CallMCP("tool", ...) — 1:1 wrappers; the tool is the 1st arg.
	callRe := regexp.MustCompile(`CallMCP\("([^"]+)"`)
	// CallMCPData("product", "tool", ...) — multi-step (smart) shortcuts; the
	// tool is the 2nd arg.
	dataRe := regexp.MustCompile(`CallMCPData\("[^"]+",\s*"([^"]+)"`)
	var checked, bad int
	check := func(f, tool string) {
		checked++
		if !real[tool] {
			bad++
			t.Errorf("%s: tool %q not found in any helper (hallucinated?)", filepath.Base(f), tool)
		}
	}
	for _, f := range srcs {
		if strings.HasSuffix(f, "_test.go") || strings.Contains(f, "/builtin/") {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		src := string(data)
		for _, m := range callRe.FindAllStringSubmatch(src, -1) {
			check(f, m[1])
		}
		for _, m := range dataRe.FindAllStringSubmatch(src, -1) {
			check(f, m[1])
		}
	}
	t.Logf("tool literals checked=%d hallucinated=%d", checked, bad)
}

// TestAllHaveIntent enforces that every shortcut carries a natural-language
// Intent (a fuller "what/when to use" description for discovery and AI-agent
// matching), not just the terse one-line Description.
func TestAllHaveIntent(t *testing.T) {
	var missing []string
	for _, s := range shortcut.All() {
		if strings.TrimSpace(s.Intent) == "" {
			missing = append(missing, s.Service+" "+s.Command)
		}
	}
	if len(missing) > 0 {
		t.Errorf("%d shortcut(s) missing Intent (natural-language description):\n  %s",
			len(missing), strings.Join(missing, "\n  "))
	}
}

// TestNoDuplicateCommands guards against two shortcuts colliding on the same
// service+command path.
func TestNoDuplicateCommands(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range shortcut.All() {
		key := s.Service + " " + s.Command
		if seen[key] {
			t.Errorf("duplicate shortcut: %s", key)
		}
		seen[key] = true
		if !strings.HasPrefix(s.Command, "+") {
			t.Errorf("%s: command must start with '+'", key)
		}
	}
}
