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

package smart

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

var errStubTool = errors.New("stub tool error")

// TestSearchMailPickMailbox covers every list_user_mailboxes shape: objects with
// an email field and bare string arrays, each at the top level or one level
// under a result/data wrapper. The nested bare-string case regressed silently
// (searchMailToMaps drops strings), making all smart mail shortcuts wrongly
// demand an explicit --email.
func TestSearchMailPickMailbox(t *testing.T) {
	cases := []struct {
		name, raw, want string
	}{
		{"object top-level", `{"emailAccounts":[{"email":"a@x.com"}]}`, "a@x.com"},
		{"object under result", `{"result":{"emailAccounts":[{"email":"b@x.com"}]}}`, "b@x.com"},
		{"bare string top-level", `{"emailAccounts":["c@x.com"]}`, "c@x.com"},
		{"bare string under result", `{"result":{"emailAccounts":["d@x.com"]}}`, "d@x.com"},
		{"empty", `{"result":{"emailAccounts":[]}}`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var data map[string]any
			if err := json.Unmarshal([]byte(tc.raw), &data); err != nil {
				t.Fatal(err)
			}
			if got := searchMailPickMailbox(data); got != tc.want {
				t.Fatalf("searchMailPickMailbox(%s) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

// TestResolveSpaceItemsWikiSpaces guards that search_wikiSpaces' result.wikiSpaces
// container is probed — otherwise +resolve-space silently finds no candidates.
func TestResolveSpaceItemsWikiSpaces(t *testing.T) {
	const raw = `{"result":{"wikiSpaces":[
		{"spaceId":"s1","name":"R&D wiki"},
		{"spaceId":"s2","name":"product wiki"}
	]}}`
	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatal(err)
	}
	if got := resolveSpaceItems(data); len(got) != 2 {
		t.Fatalf("lower/upper mismatch: result.wikiSpaces has 2 entries, resolver returned %d", len(got))
	}
}

// --- fake MCP caller for end-to-end shortcut projection tests ---

type stubMailboxCaller struct {
	byTool  map[string]string
	errTool string // tool name that should return a transport error
}

func (f *stubMailboxCaller) CallTool(_ context.Context, _, tool string, _ map[string]any) (*edition.ToolResult, error) {
	if tool == f.errTool {
		return nil, errStubTool
	}
	text := f.byTool[tool]
	if text == "" {
		text = `{"result":{}}`
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: text}}}, nil
}
func (f *stubMailboxCaller) Format() string { return "json" }
func (f *stubMailboxCaller) DryRun() bool   { return false }
func (f *stubMailboxCaller) Fields() string { return "" }
func (f *stubMailboxCaller) JQ() string     { return "" }

func runShortcut(t *testing.T, fake *stubMailboxCaller, argv ...string) string {
	t.Helper()
	helpers.InitDeps(fake)
	root := &cobra.Command{Use: "dws", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	root.AddCommand(shortcut.Commands()...)
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(io.Discard)
	root.SetArgs(argv)
	if err := root.Execute(); err != nil {
		t.Fatalf("execute %v: %v", argv, err)
	}
	return buf.String()
}

func runShortcutErr(t *testing.T, fake *stubMailboxCaller, argv ...string) error {
	t.Helper()
	helpers.InitDeps(fake)
	root := &cobra.Command{Use: "dws", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	root.AddCommand(shortcut.Commands()...)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(argv)
	return root.Execute()
}

// TestSmartSearchMailNoMailbox covers the empty-mailbox path: when
// list_user_mailboxes yields no address, +search-mail must surface a clear
// "provide --email" validation error rather than silently proceeding.
func TestSmartSearchMailNoMailbox(t *testing.T) {
	fake := &stubMailboxCaller{byTool: map[string]string{
		"list_user_mailboxes": `{"result":{"emailAccounts":[]}}`,
	}}
	err := runShortcutErr(t, fake, "mail", "+search-mail", "--query", "x", "--format", "json")
	if err == nil || !strings.Contains(err.Error(), "未找到可用邮箱") {
		t.Fatalf("want no-mailbox validation error, got %v", err)
	}
}

// TestSmartSearchMailBackendError covers the transport-error branch of the
// mailbox resolver.
func TestSmartSearchMailBackendError(t *testing.T) {
	fake := &stubMailboxCaller{byTool: map[string]string{}, errTool: "list_user_mailboxes"}
	if err := runShortcutErr(t, fake, "mail", "+search-mail", "--query", "x", "--format", "json"); err == nil {
		t.Fatal("want backend error propagated, got nil")
	}
}

// TestCreatedTodosBackendError covers the error branch of +created-todos when the
// paged todo fetch fails.
func TestCreatedTodosBackendError(t *testing.T) {
	fake := &stubMailboxCaller{byTool: map[string]string{}, errTool: "get_user_todos_in_current_org"}
	if err := runShortcutErr(t, fake, "todo", "+created-todos", "--format", "json"); err == nil {
		t.Fatal("want backend error propagated, got nil")
	}
}

// TestCreatedTodosProjectsCards runs +created-todos end to end through a fake MCP
// caller. get_user_todos_in_current_org nests cards under result.todoCards; the
// shared pager (pageSize=20, not the old 50 that silently returned empty) must
// surface them into the projected "created" list.
func TestCreatedTodosProjectsCards(t *testing.T) {
	fake := &stubMailboxCaller{byTool: map[string]string{
		"get_user_todos_in_current_org": `{"result":{"todoCards":[
			{"subject":"todoA","taskId":"111","dueTime":1784686500000},
			{"subject":"todoB","taskId":"222"}
		]}}`,
	}}
	out := runShortcut(t, fake, "todo", "+created-todos", "--format", "json")
	var d map[string]any
	if err := json.Unmarshal([]byte(out), &d); err != nil {
		t.Fatalf("output not json: %q", out)
	}
	created, _ := d["created"].([]any)
	if len(created) != 2 {
		t.Fatalf("lower/upper mismatch: 2 todos in backend, projection created=%d (%s)", len(created), out)
	}
	if !strings.Contains(out, "111") || !strings.Contains(out, "222") {
		t.Fatalf("projected rows missing taskId: %s", out)
	}
}

// TestSmartSearchMailAutoResolvesMailbox runs +search-mail without --email so the
// shortcut must auto-resolve the mailbox from list_user_mailboxes
// (result.emailAccounts) before searching. Before the fix this returned
// the no-mailbox validation error because emailAccounts was not probed.
func TestSmartSearchMailAutoResolvesMailbox(t *testing.T) {
	fake := &stubMailboxCaller{byTool: map[string]string{
		"list_user_mailboxes": `{"emailAccounts":[{"email":"me@example.com"}]}`,
		"search_emails":       `{"result":{"messages":[{"subject":"weekly report","from":"a@x.com"}]}}`,
	}}
	out := runShortcut(t, fake, "mail", "+search-mail", "--query", "subject:weekly report", "--format", "json")
	if strings.Contains(out, "未找到可用邮箱") || out == "" {
		t.Fatalf("mailbox auto-resolve failed: %s", out)
	}
	if !strings.Contains(out, "weekly report") {
		t.Fatalf("search result not projected: %s", out)
	}
}
