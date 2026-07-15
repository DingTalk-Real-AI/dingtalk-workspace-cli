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

package helpers

import (
	"reflect"
	"testing"

	"github.com/spf13/cobra"
)

func requireWukongSyncCommand(t *testing.T, root *cobra.Command, path ...string) *cobra.Command {
	t.Helper()
	cmd, remaining, err := root.Find(path)
	if err != nil {
		t.Fatalf("%s: command not found: %v", path, err)
	}
	if len(remaining) != 0 {
		t.Fatalf("%s: unresolved path suffix %v (resolved to %q)", path, remaining, cmd.CommandPath())
	}
	return cmd
}

func requireWukongSyncFlags(t *testing.T, cmd *cobra.Command, names ...string) {
	t.Helper()
	for _, name := range names {
		if cmd.Flags().Lookup(name) == nil {
			t.Fatalf("%s: missing flag --%s", cmd.CommandPath(), name)
		}
	}
}

func TestWukongSyncMailCommands(t *testing.T) {
	root := newMailCommand()
	cases := []struct {
		path  []string
		flags []string
	}{
		{[]string{"mailbox", "profile"}, []string{"email"}},
		{[]string{"message", "batch-get"}, []string{"email", "ids"}},
		{[]string{"sent-message", "recall"}, []string{"email", "id", "subject", "yes"}},
		{[]string{"sent-message", "recall-detail"}, []string{"email", "id"}},
		{[]string{"auto-reply", "update"}, []string{"email", "enabled", "start", "end", "scope", "content"}},
		{[]string{"allow-list", "list"}, []string{"email"}},
		{[]string{"allow-list", "add"}, []string{"email", "entries"}},
		{[]string{"allow-list", "remove"}, []string{"email", "entries"}},
		{[]string{"block-list", "list"}, []string{"email"}},
		{[]string{"block-list", "add"}, []string{"email", "entries"}},
		{[]string{"block-list", "remove"}, []string{"email", "entries"}},
	}
	for _, tc := range cases {
		cmd := requireWukongSyncCommand(t, root, tc.path...)
		requireWukongSyncFlags(t, cmd, tc.flags...)
	}
}

func TestWukongSyncChatCommands(t *testing.T) {
	root := newChatCommand()
	cases := []struct {
		path  []string
		flags []string
	}{
		{[]string{"group", "notice", "create"}, []string{"group", "content", "sticky", "send-ding", "run-at"}},
		{[]string{"group", "notice", "edit"}, []string{"group", "notice-id", "content", "sticky", "send-ding"}},
		{[]string{"group", "notice", "get"}, []string{"group", "notice-id"}},
		{[]string{"group", "notice", "list"}, []string{"group", "limit", "cursor", "scheduled"}},
		{[]string{"group", "share-invite"}, []string{"source", "target", "receiver", "expires-seconds", "uuid"}},
		{[]string{"text", "translate"}, []string{"query", "to"}},
		{[]string{"category", "create-smart"}, []string{"name", "keywords", "members"}},
		{[]string{"message", "list-emotion-replies"}, []string{"msg-ids"}},
	}
	for _, tc := range cases {
		cmd := requireWukongSyncCommand(t, root, tc.path...)
		requireWukongSyncFlags(t, cmd, tc.flags...)
	}
}

func TestWukongSyncDocCommands(t *testing.T) {
	root := newDocCommand()

	importCmd := requireWukongSyncCommand(t, root, "import")
	requireWukongSyncFlags(t, importCmd, "file", "folder", "workspace", "name", "folder-id", "workspace-id")

	importGetCmd := requireWukongSyncCommand(t, root, "import", "get")
	requireWukongSyncFlags(t, importGetCmd, "task-id")
}

func TestWukongSyncSheetCommands(t *testing.T) {
	root := newSheetCommand()
	importCmd := requireWukongSyncCommand(t, root, "import")
	requireWukongSyncFlags(t, importCmd, "file", "folder-token", "workspace", "name", "folder")

	importGetCmd := requireWukongSyncCommand(t, root, "import", "get")
	requireWukongSyncFlags(t, importGetCmd, "task-id")

	tableGetCmd := requireWukongSyncCommand(t, root, "table-get")
	requireWukongSyncFlags(t, tableGetCmd, "node", "sheet-id", "range", "no-header")

	tablePutCmd := requireWukongSyncCommand(t, root, "table-put")
	requireWukongSyncFlags(t, tablePutCmd, "node", "sheets")

	groupCmd := requireWukongSyncCommand(t, root, "group-dimension")
	requireWukongSyncFlags(t, groupCmd, "node", "sheet-id", "range", "group-state")

	ungroupCmd := requireWukongSyncCommand(t, root, "ungroup-dimension")
	requireWukongSyncFlags(t, ungroupCmd, "node", "sheet-id", "range")
}

func TestWukongSyncSheetBatchDimensionGroupMapping(t *testing.T) {
	group, err := translateBatchOp(map[string]any{
		"toolName": "group-dimension",
		"input": map[string]any{
			"sheet-id":    "Sheet1",
			"range":       "3:7",
			"group-state": "fold",
		},
	})
	if err != nil {
		t.Fatalf("group-dimension mapping returned error: %v", err)
	}
	wantGroup := map[string]any{
		"toolName": "group_dimension",
		"input": map[string]any{
			"sheetId":    "Sheet1",
			"range":      "3:7",
			"groupState": "fold",
		},
	}
	if !reflect.DeepEqual(group, wantGroup) {
		t.Fatalf("group-dimension mapping mismatch:\n got: %#v\nwant: %#v", group, wantGroup)
	}

	ungroup, err := translateBatchOp(map[string]any{
		"toolName": "ungroup-dimension",
		"input": map[string]any{
			"sheet-id": "Sheet1",
			"range":    "C:F",
		},
	})
	if err != nil {
		t.Fatalf("ungroup-dimension mapping returned error: %v", err)
	}
	wantUngroup := map[string]any{
		"toolName": "ungroup_dimension",
		"input": map[string]any{
			"sheetId": "Sheet1",
			"range":   "C:F",
		},
	}
	if !reflect.DeepEqual(ungroup, wantUngroup) {
		t.Fatalf("ungroup-dimension mapping mismatch:\n got: %#v\nwant: %#v", ungroup, wantUngroup)
	}
}

func TestWukongSyncAgoalCommands(t *testing.T) {
	root := newAgoalCommand()
	cases := []struct {
		path  []string
		flags []string
	}{
		{[]string{"strategy", "list"}, []string{"scope-type", "scope-id", "request-id"}},
		{[]string{"strategy", "detail"}, []string{"profile-id", "request-id"}},
		{[]string{"strategy", "update"}, []string{"profile-id", "content", "request-id"}},
		{[]string{"contract", "list"}, []string{"scope-type", "scope-id", "request-id"}},
		{[]string{"contract", "fields"}, []string{"request-id"}},
		{[]string{"contract", "detail"}, []string{"contract-id", "request-id"}},
		{[]string{"contract", "update"}, []string{"contract-id", "dimensions", "audit-config", "objective-template", "request-id"}},
		{[]string{"scorecard", "detail"}, []string{"selected-time", "dept-id", "request-id"}},
		{[]string{"scorecard", "entity-detail"}, []string{"sc-id", "entity-id", "request-id"}},
		{[]string{"scorecard", "update"}, []string{"dept-id", "selected-time", "id", "tracking-period-type", "content", "request-id"}},
		{[]string{"user", "rules"}, []string{"user-id", "request-id"}},
		{[]string{"user", "objectives"}, []string{"user-id", "rule-id", "period-ids", "request-id"}},
		{[]string{"report", "list-statistics"}, []string{"keyword", "request-id"}},
		{[]string{"report", "submit-detail"}, []string{"template-id", "submit-state", "query-date", "page", "page-size", "keyword", "request-id"}},
		{[]string{"obj-template", "list"}, []string{"keyword", "page", "page-size", "request-id"}},
		{[]string{"obj-template", "create-or-update"}, []string{"template-id", "title", "objective-weight", "dimension-weight", "compute-by-weight", "dimensions", "request-id"}},
	}
	for _, tc := range cases {
		cmd := requireWukongSyncCommand(t, root, tc.path...)
		requireWukongSyncFlags(t, cmd, tc.flags...)
	}
}
