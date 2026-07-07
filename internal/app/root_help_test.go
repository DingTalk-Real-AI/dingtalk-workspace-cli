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

	"github.com/spf13/cobra"
)

func TestRootHelpKeepsOpenCompatibilityCommandsVisible(t *testing.T) {
	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("root help: %v\n%s", err, out.String())
	}
	help := out.String()
	for _, want := range []string{
		"● conference",
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
