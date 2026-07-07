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
