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
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

func TestRootHelpHidesCompatibilityOnlyCommands(t *testing.T) {
	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("root help: %v\n%s", err, out.String())
	}
	help := out.String()
	if strings.Contains(help, "● conference") {
		t.Fatalf("root help should hide conference compatibility command:\n%s", help)
	}
	for _, want := range []string{
		"● dev",
		"• upgrade",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("root help missing %q:\n%s", want, help)
		}
	}
}

func TestRootKeepsMainBranchChatCompatibilityCommands(t *testing.T) {
	root := NewRootCommand()
	listDirect := mustFindCommand(t, root, "chat", "message", "list-direct")
	for _, flag := range []string{"user", "open-dingtalk-id", "time", "forward", "limit"} {
		if listDirect.Flags().Lookup(flag) == nil {
			t.Fatalf("chat message list-direct missing --%s", flag)
		}
	}

	mediaUpload := mustFindCommand(t, root, "chat", "media", "upload")
	for _, flag := range []string{"file", "type"} {
		if mediaUpload.Flags().Lookup(flag) == nil {
			t.Fatalf("chat media upload missing --%s", flag)
		}
	}

	mustFindCommand(t, root, "contact", "get")
	mustFindCommand(t, root, "contact", "search")
	mustFindCommand(t, root, "contact", "user", "list")
	mustFindCommand(t, root, "conference", "meeting", "reserve")
}

func TestRootKeepsContactWukongCompatibilityCommands(t *testing.T) {
	root := NewRootCommand()
	label := mustFindCommand(t, root, "contact", "label")
	if label.Hidden {
		t.Fatal("contact label should be visible as a real command group")
	}
	if !containsString(label.Aliases, "role") {
		t.Fatal("contact label missing role alias")
	}
	mustFindCommand(t, root, "contact", "label", "get")
	mustFindCommand(t, root, "contact", "label", "list")
	mustFindCommand(t, root, "contact", "label", "list-members")
	mustFindCommand(t, root, "contact", "label", "find")
	mustFindCommand(t, root, "contact", "label", "search")
	mustFindCommand(t, root, "contact", "label", "info")
	mustFindCommand(t, root, "contact", "label", "detail")
	mustFindCommand(t, root, "contact", "label", "list-all")

	getSelf := mustFindCommand(t, root, "contact", "user", "get-self")
	for _, alias := range []string{"self", "me", "whoami", "current"} {
		if !containsString(getSelf.Aliases, alias) {
			t.Fatalf("contact user get-self missing alias %q", alias)
		}
	}

	for _, tc := range []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "label list",
			args: []string{"--dry-run", "contact", "label", "list"},
			want: []string{"get_org_labels"},
		},
		{
			name: "label get",
			args: []string{"--dry-run", "contact", "label", "get", "--names", "admin,finance"},
			want: []string{"search_label_by_name", "labelNames", "admin", "finance"},
		},
		{
			name: "label members",
			args: []string{"--dry-run", "contact", "label", "list-members", "--id", "123"},
			want: []string{"get_label_members_by_labelId", "labelId", "123"},
		},
		{
			name: "role shim",
			args: []string{"--dry-run", "contact", "role", "list"},
			want: []string{"get_org_labels"},
		},
		{
			name: "label fuzzy shim",
			args: []string{"--dry-run", "contact", "label", "find", "--names", "admin"},
			want: []string{"search_label_by_name", "labelNames", "admin"},
		},
		{
			name: "label detail shim",
			args: []string{"--dry-run", "contact", "label", "detail", "--id", "123"},
			want: []string{"get_label_members_by_labelId", "labelId", "123"},
		},
		{
			name: "contact search shim",
			args: []string{"--dry-run", "contact", "search", "--query", "admin"},
			want: []string{"search_contact_by_key_word", "keyword", "admin"},
		},
		{
			name: "contact find shim",
			args: []string{"--dry-run", "contact", "find", "--query", "admin"},
			want: []string{"search_contact_by_key_word", "keyword", "admin"},
		},
		{
			name: "contact list defaults to label list",
			args: []string{"--dry-run", "contact", "list"},
			want: []string{"get_org_labels"},
		},
		{
			name: "contact list department members",
			args: []string{"--dry-run", "contact", "list", "--depts", "1"},
			want: []string{"get_dept_members_by_deptId", "deptIds", "1"},
		},
		{
			name: "contact get user details",
			args: []string{"--dry-run", "contact", "get", "--ids", "user1"},
			want: []string{"get_user_info_by_user_ids", "user_id_list", "user1"},
		},
		{
			name: "contact get label by name",
			args: []string{"--dry-run", "contact", "get", "--names", "admin"},
			want: []string{"search_label_by_name", "labelNames", "admin"},
		},
		{
			name: "contact self shim",
			args: []string{"--dry-run", "contact", "self"},
			want: []string{"get_current_user_profile"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := executeRootCaptureStdout(t, tc.args)
			if err != nil {
				t.Fatalf("Execute(%v) error = %v\n%s", tc.args, err, got)
			}
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Fatalf("Execute(%v) output missing %q:\n%s", tc.args, want, got)
				}
			}
		})
	}
}

func TestChatFileUploadDownlinedButMessageFileSendStays(t *testing.T) {
	root := NewRootCommand()
	fileCmd := mustFindCommand(t, root, "chat", "file")
	if !fileCmd.Hidden {
		t.Fatal("chat file should be hidden after upload_conversation_file_by_url downline")
	}
	upload := mustFindCommand(t, root, "chat", "file", "upload")
	if !upload.Hidden {
		t.Fatal("chat file upload should be hidden after downline")
	}
	for _, flag := range []string{"group", "url", "file", "file-name"} {
		if upload.Flags().Lookup(flag) == nil {
			t.Fatalf("chat file upload missing compatibility flag --%s", flag)
		}
	}

	send := mustFindCommand(t, root, "chat", "message", "send")
	for _, flag := range []string{"msg-type", "file-path"} {
		if send.Flags().Lookup(flag) == nil {
			t.Fatalf("chat message send missing --%s", flag)
		}
	}

	got, err := executeRootCaptureStdout(t, []string{
		"chat", "file", "upload",
		"--group", "cid",
		"--url", "https://example.com/report.pdf",
		"--file-name", "report.pdf",
	})
	if err == nil {
		t.Fatalf("chat file upload error = nil, want downline error\n%s", got)
	}
	got = got + "\n" + err.Error()
	for _, want := range []string{"已下线", "upload_conversation_file_by_url", "chat message send --msg-type file --file-path"} {
		if !strings.Contains(got, want) {
			t.Fatalf("chat file upload output missing %q:\n%s", want, got)
		}
	}
}

func TestCalendarEventListDryRunPreviewsOnly(t *testing.T) {
	got, err := executeRootCaptureStdout(t, []string{
		"--dry-run", "calendar", "event", "list",
		"--start", "2026-07-07T00:00:00+08:00",
		"--end", "2026-07-07T01:00:00+08:00",
	})
	if err != nil {
		t.Fatalf("calendar event list --dry-run error = %v\n%s", err, got)
	}
	for _, want := range []string{"list_calendar_events", "startTime", "endTime"} {
		if !strings.Contains(got, want) {
			t.Fatalf("calendar dry-run output missing %q:\n%s", want, got)
		}
	}
}

func TestRootKeepsSVIPChatCompatibilityFlags(t *testing.T) {
	root := NewRootCommand()

	listBySender := mustFindCommand(t, root, "chat", "message", "list-by-sender")
	if listBySender.Flags().Lookup("sender") == nil {
		t.Fatal("chat message list-by-sender missing hidden --sender alias")
	}

	searchAdvanced := mustFindCommand(t, root, "chat", "message", "search-advanced")
	for _, flag := range []string{"sender", "senders", "sender-ids"} {
		if searchAdvanced.Flags().Lookup(flag) == nil {
			t.Fatalf("chat message search-advanced missing --%s", flag)
		}
	}
}

func TestCacheRefreshCompatibilityStub(t *testing.T) {
	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"cache", "refresh", "--format", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("cache refresh compatibility stub: %v\n%s", err, out.String())
	}
	got := out.String()
	for _, want := range []string{`"status":"deprecated"`, `"command":"dws cache refresh"`, "服务发现已下线"} {
		if !strings.Contains(got, want) {
			t.Fatalf("cache refresh output missing %q:\n%s", want, got)
		}
	}
}

func TestInjectStaticServersMergesStaticAndSupplementServers(t *testing.T) {
	previous := edition.Get()
	defer edition.Override(previous)
	defer SetDynamicServers(nil)

	edition.Override(&edition.Hooks{
		Name: "test",
		StaticServers: func() []edition.ServerInfo {
			return []edition.ServerInfo{{
				ID:       "static-test",
				Name:     "Static Test",
				Endpoint: "https://static.example/server/static-test",
				Prefixes: []string{"static-alias"},
			}}
		},
		SupplementServers: func() []edition.ServerInfo {
			return []edition.ServerInfo{{
				ID:       "supplement-test",
				Name:     "Supplement Test",
				Endpoint: "https://supplement.example/server/supplement-test",
				Prefixes: []string{"supplement-alias"},
			}}
		},
	})

	injectStaticServers()

	for _, tc := range []struct {
		productID string
		endpoint  string
	}{
		{"static-test", "https://static.example/server/static-test"},
		{"static-alias", "https://static.example/server/static-test"},
		{"supplement-test", "https://supplement.example/server/supplement-test"},
		{"supplement-alias", "https://supplement.example/server/supplement-test"},
	} {
		got, ok := directRuntimeEndpoint(tc.productID, "")
		if !ok || got != tc.endpoint {
			t.Fatalf("directRuntimeEndpoint(%q) = %q, %v; want %q, true", tc.productID, got, ok, tc.endpoint)
		}
	}
}

func mustFindCommand(t *testing.T, root *cobra.Command, path ...string) *cobra.Command {
	t.Helper()
	cmd := root
	for _, name := range path {
		var next *cobra.Command
		for _, child := range cmd.Commands() {
			if child.Name() == name {
				next = child
				break
			}
		}
		if next == nil {
			t.Fatalf("missing command path %q under %q", strings.Join(path, " "), cmd.CommandPath())
		}
		cmd = next
	}
	return cmd
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func executeRootCaptureStdout(t *testing.T, args []string) (string, error) {
	t.Helper()

	oldStdout := os.Stdout
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe error = %v", err)
	}
	os.Stdout = writePipe

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	execErr := cmd.Execute()

	_ = writePipe.Close()
	os.Stdout = oldStdout
	captured, readErr := io.ReadAll(readPipe)
	if readErr != nil {
		t.Fatalf("read stdout pipe error = %v", readErr)
	}
	return out.String() + string(captured), execErr
}
