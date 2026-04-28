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
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cobracmd"
	"github.com/spf13/cobra"
)

// TestPickCommands_DynamicWinsLeafConflicts verifies that when the discovery
// envelope produces a dynamic leaf and a helper registers the same-named leaf,
// the dynamic one wins — envelopes remain the runtime authority for behaviour
// they declare. The helper subtree must not slip in via
// mergeTopLevelCommands's LocalFlagCount-based arbitration.
func TestPickCommands_DynamicWinsLeafConflicts(t *testing.T) {
	dynTask := &cobra.Command{Use: "task", Short: "dynamic-task", Run: func(*cobra.Command, []string) {}}
	dyn := &cobra.Command{Use: "todo", Short: "dynamic"}
	dyn.AddCommand(dynTask)
	dynamic := []*cobra.Command{dyn}

	hlpTask := &cobra.Command{Use: "task", Short: "helper-task", Run: func(*cobra.Command, []string) {}}
	hlp := &cobra.Command{Use: "todo", Short: "helper"}
	hlp.AddCommand(hlpTask)
	helpers := []*cobra.Command{hlp}

	got := pickCommands(dynamic, helpers)

	if len(got) != 1 || got[0] != dyn {
		t.Fatalf("pickCommands returned %v, want [dyn]", got)
	}
	// The dynamic leaf must still be the one we find under the top-level name.
	var found *cobra.Command
	for _, c := range got[0].Commands() {
		if c.Name() == "task" {
			found = c
		}
	}
	if found != dynTask {
		t.Fatalf("leaf conflict resolved to helper; want dynamic to win")
	}
}

// TestPickCommands_HelperOnlyLeavesAreGrafted verifies that when a helper
// registers siblings the discovery envelope did NOT declare (e.g.
// `chat message send-by-bot`, `chat message recall-by-bot` next to the
// envelope's `chat message send`), those helper-only leaves are grafted into
// the dynamic subtree instead of being dropped. This is a regression guard:
// prior to this fix, pickCommands silently dropped the entire helper subtree
// whenever the top-level product name collided, which disappeared every
// helper-only leaf the envelope didn't cover.
func TestPickCommands_HelperOnlyLeavesAreGrafted(t *testing.T) {
	dynMessage := &cobra.Command{Use: "message"}
	dynMessage.AddCommand(&cobra.Command{Use: "send", Run: func(*cobra.Command, []string) {}})
	dyn := &cobra.Command{Use: "chat"}
	dyn.AddCommand(dynMessage)
	dynamic := []*cobra.Command{dyn}

	helperOnlyLeaf := &cobra.Command{Use: "send-by-bot", Run: func(*cobra.Command, []string) {}}
	hlpMessage := &cobra.Command{Use: "message"}
	hlpMessage.AddCommand(helperOnlyLeaf)
	hlp := &cobra.Command{Use: "chat"}
	hlp.AddCommand(hlpMessage)
	helpers := []*cobra.Command{hlp}

	got := pickCommands(dynamic, helpers)

	if len(got) != 1 || got[0] != dyn {
		t.Fatalf("pickCommands returned %v, want [dyn]", got)
	}
	var grafted *cobra.Command
	for _, child := range dynMessage.Commands() {
		if child.Name() == "send-by-bot" {
			grafted = child
		}
	}
	if grafted == nil {
		t.Fatalf("helper-only leaf send-by-bot was not grafted into dynamic.chat.message")
	}
	if grafted != helperOnlyLeaf {
		t.Fatalf("grafted leaf identity differs from helper-registered leaf")
	}
}

// TestPickCommands_HelpersFillUncoveredProducts verifies that helpers whose
// names are NOT in the dynamic set are preserved — the dynamic overlay only
// shadows products it actually covers.
func TestPickCommands_HelpersFillUncoveredProducts(t *testing.T) {
	dyn := &cobra.Command{Use: "todo"}
	dynamic := []*cobra.Command{dyn}

	todoHelper := &cobra.Command{Use: "todo"}
	attendanceHelper := &cobra.Command{Use: "attendance"}
	chatHelper := &cobra.Command{Use: "chat"}
	helpers := []*cobra.Command{todoHelper, attendanceHelper, chatHelper}

	got := pickCommands(dynamic, helpers)

	names := make(map[string]*cobra.Command, len(got))
	for _, c := range got {
		names[c.Name()] = c
	}
	if names["todo"] != dyn {
		t.Fatalf("todo = %v, want dynamic", names["todo"])
	}
	if names["attendance"] != attendanceHelper {
		t.Fatalf("attendance not preserved from helpers")
	}
	if names["chat"] != chatHelper {
		t.Fatalf("chat not preserved from helpers")
	}
	if len(got) != 3 {
		t.Fatalf("got %d commands, want 3 (todo+attendance+chat)", len(got))
	}
}

// TestPickCommands_EmptyDynamicPreservesHelpers verifies the degenerate case:
// when discovery returns nothing, helpers are the sole source of truth — the
// behaviour must be identical to the pre-refactor append-all code path.
func TestPickCommands_EmptyDynamicPreservesHelpers(t *testing.T) {
	todoHelper := &cobra.Command{Use: "todo"}
	chatHelper := &cobra.Command{Use: "chat"}
	helpers := []*cobra.Command{todoHelper, chatHelper}

	got := pickCommands(nil, helpers)

	if len(got) != 2 {
		t.Fatalf("got %d commands, want 2", len(got))
	}
	if got[0] != todoHelper || got[1] != chatHelper {
		t.Fatalf("pickCommands changed helpers order or identity")
	}
}

// TestPickCommands_HelperGroupShadowsDynamicLeaf simulates the issue #164
// shape mismatch: the discovery envelope publishes `chat group members` as
// a LEAF (the get_group_members tool exposed at that CLI path), while the
// hardcoded helper has restructured `members` into a GROUP container with
// `list / add / remove / add-bot` subcommands. The helper group carries the
// preferLegacyLeaf priority annotation, so it must replace the dynamic leaf
// and surface its subtree — otherwise `dws chat group members list` is
// unreachable and the user-visible regression in #164 stays.
func TestPickCommands_HelperGroupShadowsDynamicLeaf(t *testing.T) {
	dynMembers := &cobra.Command{Use: "members", Run: func(*cobra.Command, []string) {}}
	dynMembers.Flags().String("id", "", "")
	dynGroup := &cobra.Command{Use: "group"}
	dynGroup.AddCommand(dynMembers)
	dyn := &cobra.Command{Use: "chat"}
	dyn.AddCommand(dynGroup)

	hlpList := &cobra.Command{Use: "list", Run: func(*cobra.Command, []string) {}}
	hlpList.Flags().String("id", "", "")
	hlpAdd := &cobra.Command{Use: "add", Run: func(*cobra.Command, []string) {}}
	hlpRemove := &cobra.Command{Use: "remove", Run: func(*cobra.Command, []string) {}}
	hlpMembers := &cobra.Command{Use: "members"}
	hlpMembers.AddCommand(hlpList, hlpAdd, hlpRemove)
	cobracmd.SetOverridePriority(hlpMembers, 100)
	hlpGroup := &cobra.Command{Use: "group"}
	hlpGroup.AddCommand(hlpMembers)
	hlp := &cobra.Command{Use: "chat"}
	hlp.AddCommand(hlpGroup)

	got := pickCommands([]*cobra.Command{dyn}, []*cobra.Command{hlp})
	if len(got) != 1 || got[0] != dyn {
		t.Fatalf("got %v, want [dyn]", got)
	}

	// Locate the (potentially replaced) members node under chat.group.
	var members *cobra.Command
	for _, c := range dynGroup.Commands() {
		if c.Name() == "members" {
			members = c
			break
		}
	}
	if members == nil {
		t.Fatalf("members node missing under dyn.chat.group after merge")
	}

	want := map[string]bool{"list": false, "add": false, "remove": false}
	for _, sub := range members.Commands() {
		if _, ok := want[sub.Name()]; ok {
			want[sub.Name()] = true
		}
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("expected `chat group members %s` after merge, missing", name)
		}
	}
}

// TestPickCommands_NilsAreSkipped guards against nil entries sneaking in from
// a misbehaving factory.
func TestPickCommands_NilsAreSkipped(t *testing.T) {
	dyn := &cobra.Command{Use: "todo"}
	hlp := &cobra.Command{Use: "chat"}

	got := pickCommands([]*cobra.Command{nil, dyn}, []*cobra.Command{nil, hlp})

	if len(got) != 2 {
		t.Fatalf("got %d commands, want 2 (nils filtered)", len(got))
	}
	if got[0] != dyn || got[1] != hlp {
		t.Fatalf("unexpected ordering or identity after nil filter")
	}
}
