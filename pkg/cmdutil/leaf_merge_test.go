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

func newGroup(name string, children ...*cobra.Command) *cobra.Command {
	cmd := &cobra.Command{Use: name}
	cmd.AddCommand(children...)
	return cmd
}

func newLeaf(name, tag string) *cobra.Command {
	return &cobra.Command{Use: name, Short: tag}
}

func TestMergeHardcodedLeavesNilInputs(t *testing.T) {
	t.Parallel()
	if got := MergeHardcodedLeaves(nil, nil); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
	dyn := newGroup("root")
	if got := MergeHardcodedLeaves(dyn, nil); got != dyn {
		t.Fatal("expected dynamic root to be returned unchanged")
	}
	hc := newGroup("root")
	if got := MergeHardcodedLeaves(nil, hc); got != nil {
		t.Fatal("expected nil when dynamic root is nil")
	}
}

func TestMergeHardcodedLeavesGraftsUnknownLeaf(t *testing.T) {
	t.Parallel()
	dyn := newGroup("root", newLeaf("kept", "dynamic"))
	hc := newGroup("root", newLeaf("extra", "hardcoded"))

	MergeHardcodedLeaves(dyn, hc)

	got := findChildByName(dyn, "extra")
	if got == nil {
		t.Fatal("expected extra leaf to be grafted")
	}
	if got.Short != "hardcoded" {
		t.Fatalf("extra.Short = %q, want %q", got.Short, "hardcoded")
	}
	if findChildByName(hc, "extra") != nil {
		t.Fatal("expected extra leaf to be detached from hardcoded root")
	}
	if got.Parent() != dyn {
		t.Fatalf("grafted leaf parent = %v, want %v", got.Parent(), dyn)
	}
}

func TestMergeHardcodedLeavesDynamicLeafWinsByDefault(t *testing.T) {
	t.Parallel()
	dyn := newGroup("root", newLeaf("shared", "dynamic"))
	hc := newGroup("root", newLeaf("shared", "hardcoded"))

	MergeHardcodedLeaves(dyn, hc)

	got := findChildByName(dyn, "shared")
	if got == nil {
		t.Fatal("expected shared leaf to remain")
	}
	if got.Short != "dynamic" {
		t.Fatalf("shared.Short = %q, want dynamic", got.Short)
	}
	if findChildByName(hc, "shared") == nil {
		t.Fatal("expected hardcoded shared leaf to remain on donor root")
	}
}

func TestMergeHardcodedLeavesHigherPriorityHardcodedLeafWins(t *testing.T) {
	t.Parallel()
	dynLeaf := newLeaf("shared", "dynamic")
	dyn := newGroup("root", dynLeaf)
	hcLeaf := newLeaf("shared", "hardcoded")
	SetOverridePriority(hcLeaf, 100)
	hc := newGroup("root", hcLeaf)

	MergeHardcodedLeaves(dyn, hc)

	got := findChildByName(dyn, "shared")
	if got != hcLeaf {
		t.Fatalf("expected hardcoded leaf to replace dynamic leaf, got %+v", got)
	}
	if findChildByName(hc, "shared") != nil {
		t.Fatal("expected hardcoded leaf to be detached from donor root")
	}
}

func TestMergeHardcodedLeavesEqualPriorityKeepsDynamic(t *testing.T) {
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
		t.Fatalf("equal priorities should keep dynamic leaf, got %+v", got)
	}
}

func TestMergeHardcodedLeavesRecurseGroups(t *testing.T) {
	t.Parallel()
	dyn := newGroup("root",
		newGroup("space",
			newLeaf("list", "dynamic"),
		),
	)
	hc := newGroup("root",
		newGroup("space",
			newLeaf("list", "hardcoded"),
			newLeaf("create", "hardcoded"),
		),
		newLeaf("ping", "hardcoded"),
	)

	MergeHardcodedLeaves(dyn, hc)

	space := findChildByName(dyn, "space")
	if space == nil {
		t.Fatal("expected space group")
	}
	if list := findChildByName(space, "list"); list == nil || list.Short != "dynamic" {
		t.Fatalf("space.list should remain dynamic, got %+v", list)
	}
	if create := findChildByName(space, "create"); create == nil || create.Short != "hardcoded" {
		t.Fatalf("space.create should be grafted, got %+v", create)
	}
	if ping := findChildByName(dyn, "ping"); ping == nil || ping.Short != "hardcoded" {
		t.Fatalf("ping should be grafted, got %+v", ping)
	}
}

func TestMergeHardcodedLeavesHigherPriorityGroupReplacesDynamicGroup(t *testing.T) {
	t.Parallel()
	dynGroup := newGroup("export", newLeaf("get", "dynamic"))
	dyn := newGroup("root", dynGroup)
	hcGroup := newGroup("export", newLeaf("get", "hardcoded"))
	SetOverridePriority(hcGroup, 100)
	hc := newGroup("root", hcGroup)

	MergeHardcodedLeaves(dyn, hc)

	got := findChildByName(dyn, "export")
	if got != hcGroup {
		t.Fatal("expected higher-priority hardcoded group to replace dynamic group")
	}
	if get := findChildByName(got, "get"); get == nil || get.Short != "hardcoded" {
		t.Fatalf("expected hardcoded export.get after replacement, got %+v", get)
	}
	if findChildByName(hc, "export") != nil {
		t.Fatal("expected hardcoded group to be detached from donor root")
	}
}

func TestMergeHardcodedLeavesShapeMismatchKeepsDynamic(t *testing.T) {
	t.Parallel()
	dyn := newGroup("root",
		newGroup("cmd", newLeaf("sub", "dynamic")),
	)
	hc := newGroup("root", newLeaf("cmd", "hardcoded"))

	MergeHardcodedLeaves(dyn, hc)

	cmd := findChildByName(dyn, "cmd")
	if cmd == nil {
		t.Fatal("expected dynamic cmd to remain")
	}
	if IsLeafCmd(cmd) {
		t.Fatal("expected dynamic cmd to remain a group")
	}
	if findChildByName(cmd, "sub") == nil {
		t.Fatal("expected cmd.sub to remain")
	}
}

func TestMergeHardcodedLeavesHigherPriorityGroupReplacesDynamicLeaf(t *testing.T) {
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
	if got != hcGroup {
		t.Fatal("expected higher-priority hardcoded group to replace dynamic leaf")
	}
	if findChildByName(got, "list") == nil {
		t.Fatal("expected hardcoded subtree leaf list to be reachable")
	}
	if findChildByName(got, "add") == nil {
		t.Fatal("expected hardcoded subtree leaf add to be reachable")
	}
	if findChildByName(hc, "shared") != nil {
		t.Fatal("expected hardcoded group to be detached from donor root")
	}
}

func TestMergeHardcodedLeavesEqualPriorityShapeMismatchKeepsDynamic(t *testing.T) {
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
		t.Fatalf("equal priorities should keep dynamic leaf, got %+v", got)
	}
}

func TestIsLeafCmd(t *testing.T) {
	t.Parallel()
	leaf := newLeaf("x", "")
	group := newGroup("x", newLeaf("child", ""))
	if !IsLeafCmd(leaf) {
		t.Fatal("expected leaf to be leaf")
	}
	if IsLeafCmd(group) {
		t.Fatal("expected group to not be leaf")
	}
	if IsLeafCmd(nil) {
		t.Fatal("expected nil to not be leaf")
	}
}
