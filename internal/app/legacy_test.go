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

	"github.com/spf13/cobra"
)

// TestPickCommands_DynamicShadowsSameNamedHelpers verifies that when the
// discovery envelope produces a dynamic command with the same top-level name
// as a helpers-registered hardcoded command, the helper is dropped and the
// dynamic command is kept verbatim. This prevents mergeTopLevelCommands from
// mixing helper leaves into the dynamic subtree via MergeCommandTree's
// LocalFlagCount-based arbitration.
func TestPickCommands_DynamicShadowsSameNamedHelpers(t *testing.T) {
	dyn := &cobra.Command{Use: "todo", Short: "dynamic"}
	dyn.AddCommand(&cobra.Command{Use: "task"})
	dynamic := []*cobra.Command{dyn}

	hlp := &cobra.Command{Use: "todo", Short: "helper"}
	hlp.AddCommand(&cobra.Command{Use: "task"})
	helpers := []*cobra.Command{hlp}

	got := pickCommands(dynamic, helpers)

	if len(got) != 1 {
		t.Fatalf("pickCommands returned %d commands, want 1", len(got))
	}
	if got[0] != dyn {
		t.Fatalf("pickCommands returned helpers command, want dynamic")
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
