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

package cmdutil

import (
	"testing"

	"github.com/spf13/cobra"
)

// newGroup returns a group command with the given children attached.
func newGroup(name string, children ...*cobra.Command) *cobra.Command {
	cmd := &cobra.Command{Use: name}
	cmd.AddCommand(children...)
	return cmd
}

// newLeaf returns a leaf command tagged via Short so tests can verify which
// variant (dynamic or hardcoded) wins without introducing a priority system.
func newLeaf(name, tag string) *cobra.Command {
	return &cobra.Command{Use: name, Short: tag}
}

func TestMergeHardcodedLeaves_NilInputs(t *testing.T) {
	t.Parallel()
	if got := MergeHardcodedLeaves(nil, nil); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
	dyn := newGroup("root")
	if got := MergeHardcodedLeaves(dyn, nil); got != dyn {
		t.Fatal("expected dyn to be returned unchanged")
	}
	hc := newGroup("root")
	if got := MergeHardcodedLeaves(nil, hc); got != nil {
		t.Fatal("expected nil when dynamicRoot is nil")
	}
}

func TestMergeHardcodedLeaves_GraftsUnknownLeaf(t *testing.T) {
	t.Parallel()
	dyn := newGroup("root", newLeaf("kept", "dynamic"))
	hc := newGroup("root", newLeaf("extra", "hardcoded"))

	MergeHardcodedLeaves(dyn, hc)

	got := findChildByName(dyn, "extra")
	if got == nil {
		t.Fatal("expected extra leaf to be grafted onto dyn")
	}
	if got.Short != "hardcoded" {
		t.Fatalf("extra.Short = %q, want %q", got.Short, "hardcoded")
	}
	if findChildByName(hc, "extra") != nil {
		t.Fatal("expected extra to be detached from hardcodedRoot")
	}
	if got.Parent() != dyn {
		t.Fatalf("grafted leaf parent = %v, want %v", got.Parent(), dyn)
	}
}

func TestMergeHardcodedLeaves_DynamicLeafWins(t *testing.T) {
	t.Parallel()
	dyn := newGroup("root", newLeaf("shared", "dynamic"))
	hc := newGroup("root", newLeaf("shared", "hardcoded"))

	MergeHardcodedLeaves(dyn, hc)

	got := findChildByName(dyn, "shared")
	if got == nil {
		t.Fatal("expected shared leaf to remain on dyn")
	}
	if got.Short != "dynamic" {
		t.Fatalf("shared.Short = %q, want %q (dynamic must win)", got.Short, "dynamic")
	}
	if findChildByName(hc, "shared") == nil {
		t.Fatal("expected hardcoded.shared to remain on hardcodedRoot (not grafted)")
	}
}

func TestMergeHardcodedLeaves_HigherPriorityHardcodedOverridesDynamic(t *testing.T) {
	t.Parallel()
	dynLeaf := newLeaf("shared", "dynamic")
	dyn := newGroup("root", dynLeaf)
	hcLeaf := newLeaf("shared", "hardcoded")
	SetOverridePriority(hcLeaf, 100)
	hc := newGroup("root", hcLeaf)

	MergeHardcodedLeaves(dyn, hc)

	got := findChildByName(dyn, "shared")
	if got == nil {
		t.Fatal("expected shared leaf on dyn after merge")
	}
	if got != hcLeaf {
		t.Fatalf("expected hardcoded leaf to replace dynamic; got Short=%q", got.Short)
	}
	if findChildByName(hc, "shared") != nil {
		t.Fatal("expected hardcoded.shared to be moved off hardcodedRoot after replacement")
	}
}

func TestMergeHardcodedLeaves_EqualPriorityKeepsDynamic(t *testing.T) {
	t.Parallel()
	dynLeaf := newLeaf("shared", "dynamic")
	SetOverridePriority(dynLeaf, 100)
	dyn := newGroup("root", dynLeaf)
	hcLeaf := newLeaf("shared", "hardcoded")
	SetOverridePriority(hcLeaf, 100)
	hc := newGroup("root", hcLeaf)

	MergeHardcodedLeaves(dyn, hc)

	got := findChildByName(dyn, "shared")
	if got != dynLeaf {
		t.Fatalf("equal priorities must keep dynamic; got Short=%q", got.Short)
	}
}

func TestMergeHardcodedLeaves_RecurseGroups(t *testing.T) {
	t.Parallel()
	dyn := newGroup("root",
		newGroup("space",
			newLeaf("list", "dynamic"),
		),
	)
	hc := newGroup("root",
		newGroup("space",
			newLeaf("list", "hardcoded"),   // dynamic wins
			newLeaf("create", "hardcoded"), // grafted
		),
		newLeaf("ping", "hardcoded"), // grafted top-level
	)

	MergeHardcodedLeaves(dyn, hc)

	space := findChildByName(dyn, "space")
	if space == nil {
		t.Fatal("expected space group on dyn")
	}
	if l := findChildByName(space, "list"); l == nil || l.Short != "dynamic" {
		t.Fatalf("space.list should remain dynamic, got %+v", l)
	}
	if c := findChildByName(space, "create"); c == nil || c.Short != "hardcoded" {
		t.Fatalf("space.create should be grafted from hardcoded, got %+v", c)
	}
	if p := findChildByName(dyn, "ping"); p == nil || p.Short != "hardcoded" {
		t.Fatalf("ping should be grafted from hardcoded, got %+v", p)
	}
}

func TestMergeHardcodedLeaves_ShapeMismatch_KeepsDynamic(t *testing.T) {
	t.Parallel()
	// Dynamic declares `cmd` as a group; hardcoded declares `cmd` as a leaf.
	// Shape mismatch — dynamic wins, warning logged, hardcoded leaf discarded.
	dyn := newGroup("root",
		newGroup("cmd", newLeaf("sub", "dynamic")),
	)
	hc := newGroup("root", newLeaf("cmd", "hardcoded"))

	MergeHardcodedLeaves(dyn, hc)

	cmd := findChildByName(dyn, "cmd")
	if cmd == nil {
		t.Fatal("expected cmd to remain on dyn")
	}
	if IsLeafCmd(cmd) {
		t.Fatal("expected dynamic cmd to remain a group")
	}
	if findChildByName(cmd, "sub") == nil {
		t.Fatal("expected cmd.sub to remain")
	}
}

// TestMergeHardcodedLeaves_HigherPriorityHardcodedGroupOverridesDynamicLeaf
// covers the leaf↔group shape-mismatch promotion path added for issue #164:
// when the envelope publishes a single tool at a CLI path (leaf) but the
// helper restructures it into a richer subcommand group, the helper group
// must win when it carries OverridePriority strictly higher than the
// dynamic leaf — otherwise the helper subtree (e.g. `chat group members
// list / add / remove / add-bot`) is silently dropped and the regression
// the priority annotation is meant to prevent re-emerges in shape-mismatch
// form.
func TestMergeHardcodedLeaves_HigherPriorityHardcodedGroupOverridesDynamicLeaf(t *testing.T) {
	t.Parallel()
	dynLeaf := newLeaf("shared", "dynamic")
	dyn := newGroup("root", dynLeaf)
	hcGroup := newGroup("shared",
		newLeaf("list", "hardcoded"),
		newLeaf("add", "hardcoded"),
	)
	SetOverridePriority(hcGroup, 100)
	hc := newGroup("root", hcGroup)

	MergeHardcodedLeaves(dyn, hc)

	got := findChildByName(dyn, "shared")
	if got == nil {
		t.Fatal("expected `shared` on dyn after merge")
	}
	if got != hcGroup {
		t.Fatal("expected hardcoded group to replace dynamic leaf")
	}
	if findChildByName(got, "list") == nil {
		t.Fatal("expected hardcoded subtree leaf `list` to be reachable")
	}
	if findChildByName(got, "add") == nil {
		t.Fatal("expected hardcoded subtree leaf `add` to be reachable")
	}
	if findChildByName(hc, "shared") != nil {
		t.Fatal("expected hardcoded `shared` to be detached from hc after replacement")
	}
}

// TestMergeHardcodedLeaves_EqualPriorityShapeMismatchKeepsDynamic guards the
// boundary: only a strictly-higher priority promotes the helper group.
// Equal priority must still keep the envelope as authority (warn case).
func TestMergeHardcodedLeaves_EqualPriorityShapeMismatchKeepsDynamic(t *testing.T) {
	t.Parallel()
	dynLeaf := newLeaf("shared", "dynamic")
	SetOverridePriority(dynLeaf, 100)
	dyn := newGroup("root", dynLeaf)
	hcGroup := newGroup("shared", newLeaf("list", "hardcoded"))
	SetOverridePriority(hcGroup, 100)
	hc := newGroup("root", hcGroup)

	MergeHardcodedLeaves(dyn, hc)

	got := findChildByName(dyn, "shared")
	if got != dynLeaf {
		t.Fatalf("equal priorities + shape mismatch must keep dynamic leaf; got %+v", got)
	}
}

func TestIsLeafCmd(t *testing.T) {
	t.Parallel()
	leaf := newLeaf("x", "")
	group := newGroup("x", newLeaf("child", ""))
	if !IsLeafCmd(leaf) {
		t.Fatal("expected leaf to be a leaf")
	}
	if IsLeafCmd(group) {
		t.Fatal("expected group to not be a leaf")
	}
	if IsLeafCmd(nil) {
		t.Fatal("expected nil to not be a leaf")
	}
}
