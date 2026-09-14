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

package cobracmd

import (
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/spf13/cobra"
)

func TestChildByName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		parent   *cobra.Command
		child    string
		wantNil  bool
		wantName string
	}{
		{
			name:    "nil parent returns nil",
			parent:  nil,
			child:   "anything",
			wantNil: true,
		},
		{
			name: "no matching child returns nil",
			parent: func() *cobra.Command {
				p := &cobra.Command{Use: "root"}
				p.AddCommand(&cobra.Command{Use: "alpha"})
				return p
			}(),
			child:   "beta",
			wantNil: true,
		},
		{
			name: "matching child is returned",
			parent: func() *cobra.Command {
				p := &cobra.Command{Use: "root"}
				p.AddCommand(&cobra.Command{Use: "alpha"})
				p.AddCommand(&cobra.Command{Use: "beta"})
				return p
			}(),
			child:    "beta",
			wantNil:  false,
			wantName: "beta",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ChildByName(tc.parent, tc.child)
			if tc.wantNil {
				if got != nil {
					t.Fatalf("expected nil, got %v", got.Name())
				}
				return
			}
			if got == nil {
				t.Fatal("expected non-nil command")
			}
			if got.Name() != tc.wantName {
				t.Fatalf("expected name %q, got %q", tc.wantName, got.Name())
			}
		})
	}
}

func TestFlagChanged(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		setup    func() *cobra.Command
		flagName string
		want     bool
	}{
		{
			name: "flag exists and changed",
			setup: func() *cobra.Command {
				cmd := &cobra.Command{Use: "test"}
				cmd.Flags().String("output", "", "output format")
				_ = cmd.Flags().Set("output", "json")
				return cmd
			},
			flagName: "output",
			want:     true,
		},
		{
			name: "flag exists but not changed",
			setup: func() *cobra.Command {
				cmd := &cobra.Command{Use: "test"}
				cmd.Flags().String("output", "table", "output format")
				return cmd
			},
			flagName: "output",
			want:     false,
		},
		{
			name: "flag does not exist",
			setup: func() *cobra.Command {
				return &cobra.Command{Use: "test"}
			},
			flagName: "nonexistent",
			want:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cmd := tc.setup()
			got := FlagChanged(cmd, tc.flagName)
			if got != tc.want {
				t.Fatalf("FlagChanged() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNewGroupCommand(t *testing.T) {
	t.Parallel()

	cmd := NewGroupCommand("mygroup", "my group description")

	if cmd.Use != "mygroup" {
		t.Fatalf("Use = %q, want %q", cmd.Use, "mygroup")
	}
	if cmd.Short != "my group description" {
		t.Fatalf("Short = %q, want %q", cmd.Short, "my group description")
	}
	if cmd.Args == nil {
		t.Fatal("Args should be set (cobra.ArbitraryArgs)")
	}
	// Args must reach the shared resolver instead of Cobra's generic arg error.
	if err := cmd.Args(cmd, []string{"extra"}); err != nil {
		t.Fatalf("Args intercepted command resolution: %v", err)
	}
	// Verify RunE is set and returns help (no error for valid invocation).
	if cmd.RunE == nil {
		t.Fatal("RunE should not be nil")
	}
	policy, ok, err := corecmd.GroupPolicyFor(cmd)
	if err != nil || !ok || policy != (corecmd.GroupPolicy{
		Mode:        corecmd.GroupNavigationOnly,
		Positionals: corecmd.PositionalsReject,
		Recovery:    corecmd.RecoverySibling,
	}) {
		t.Fatalf("GroupPolicyFor() = %+v, %v, %v", policy, ok, err)
	}
	// RunE calls cmd.Help() which should not error.
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("RunE returned unexpected error: %v", err)
	}
	if err := cmd.RunE(cmd, []string{"extra"}); err == nil || !strings.Contains(err.Error(), "unknown subcommand") {
		t.Fatalf("RunE typo error = %v", err)
	}
}

func TestCrossPlatformCoverageValidateGroupTree(t *testing.T) {
	t.Run("valid final tree", func(t *testing.T) {
		root := NewGroupCommand("dws", "root")
		nested := NewGroupCommand("nested", "nested")
		nested.AddCommand(&cobra.Command{Use: "leaf", RunE: func(*cobra.Command, []string) error { return nil }})
		root.AddCommand(nested)
		if err := ValidateGroupTree(root); err != nil {
			t.Fatalf("ValidateGroupTree() = %v", err)
		}
	})

	t.Run("nil tree", func(t *testing.T) {
		if err := ValidateGroupTree(nil); err == nil || !strings.Contains(err.Error(), "nil") {
			t.Fatalf("ValidateGroupTree(nil) = %v", err)
		}
	})

	t.Run("children require declaration", func(t *testing.T) {
		root := &cobra.Command{Use: "dws"}
		root.AddCommand(&cobra.Command{Use: "leaf", RunE: func(*cobra.Command, []string) error { return nil }})
		if err := ValidateGroupTree(root); err == nil || !strings.Contains(err.Error(), "no GroupPolicy") {
			t.Fatalf("ValidateGroupTree() = %v", err)
		}
	})

	t.Run("leaf rejects stale declaration", func(t *testing.T) {
		leaf := NewGroupCommand("stale", "stale")
		if err := ValidateGroupTree(leaf); err == nil || !strings.Contains(err.Error(), "retains GroupPolicy") {
			t.Fatalf("ValidateGroupTree() = %v", err)
		}
	})

	t.Run("deep policy on a leaf is still a stale group declaration", func(t *testing.T) {
		leaf := &cobra.Command{Use: "stale-deep"}
		corecmd.ApplyGroupPolicy(leaf, corecmd.GroupPolicy{
			Mode: corecmd.GroupNavigationOnly, Positionals: corecmd.PositionalsReject, Recovery: corecmd.RecoveryDeep,
		})
		if err := ValidateGroupTree(leaf); err == nil || !strings.Contains(err.Error(), "retains GroupPolicy") {
			t.Fatalf("ValidateGroupTree() = %v", err)
		}
	})

	t.Run("declared group must stay runnable", func(t *testing.T) {
		root := NewGroupCommand("dws", "root")
		root.AddCommand(&cobra.Command{Use: "leaf", RunE: func(*cobra.Command, []string) error { return nil }})
		root.RunE = nil
		root.Run = nil
		if err := ValidateGroupTree(root); err == nil || !strings.Contains(err.Error(), "not runnable") {
			t.Fatalf("ValidateGroupTree() = %v", err)
		}
	})

	t.Run("rejected positionals require compiled Args behavior", func(t *testing.T) {
		root := NewGroupCommand("dws", "root")
		root.AddCommand(&cobra.Command{Use: "leaf", RunE: func(*cobra.Command, []string) error { return nil }})
		root.Args = nil
		if err := ValidateGroupTree(root); err == nil || !strings.Contains(err.Error(), "compiled Args") {
			t.Fatalf("ValidateGroupTree() = %v", err)
		}
	})

	t.Run("allowed positionals require explicit Args contract", func(t *testing.T) {
		root := &cobra.Command{Use: "dws", RunE: func(*cobra.Command, []string) error { return nil }}
		corecmd.ApplyGroupPolicy(root, corecmd.GroupPolicy{
			Mode: corecmd.GroupHybrid, Positionals: corecmd.PositionalsAllow, Recovery: corecmd.RecoveryDisabled,
		})
		root.AddCommand(&cobra.Command{Use: "leaf", RunE: func(*cobra.Command, []string) error { return nil }})
		if err := ValidateGroupTree(root); err == nil || !strings.Contains(err.Error(), "explicit Args") {
			t.Fatalf("ValidateGroupTree() = %v", err)
		}
	})

	t.Run("validation does not execute positional contracts", func(t *testing.T) {
		calls := 0
		root := &cobra.Command{
			Use: "dws",
			Args: func(*cobra.Command, []string) error {
				calls++
				return nil
			},
			RunE: func(*cobra.Command, []string) error { return nil },
		}
		corecmd.ApplyGroupPolicy(root, corecmd.GroupPolicy{
			Mode: corecmd.GroupHybrid, Positionals: corecmd.PositionalsAllow, Recovery: corecmd.RecoveryDisabled,
		})
		root.AddCommand(&cobra.Command{Use: "leaf", RunE: func(*cobra.Command, []string) error { return nil }})
		if err := ValidateGroupTree(root); err != nil {
			t.Fatalf("ValidateGroupTree() = %v", err)
		}
		if calls != 0 {
			t.Fatalf("ValidateGroupTree executed Args %d times", calls)
		}
	})

	t.Run("deep recovery requires available descendants", func(t *testing.T) {
		root := &cobra.Command{Use: "dws"}
		corecmd.ApplyGroupPolicy(root, corecmd.GroupPolicy{
			Mode: corecmd.GroupNavigationOnly, Positionals: corecmd.PositionalsReject, Recovery: corecmd.RecoveryDeep,
		})
		root.AddCommand(&cobra.Command{Use: "hidden", Hidden: true, RunE: func(*cobra.Command, []string) error { return nil }})
		if err := ValidateGroupTree(root); err == nil || !strings.Contains(err.Error(), "available descendant") {
			t.Fatalf("ValidateGroupTree() = %v", err)
		}
	})
}

func TestCrossPlatformCoverageValidateGroupTreeFailsClosedOnCorruption(t *testing.T) {
	t.Run("malformed policy metadata", func(t *testing.T) {
		root := &cobra.Command{
			Use: "dws",
			Annotations: map[string]string{
				"dws.internal.corecmd.group_policy.v1": "malformed",
			},
		}
		if err := ValidateGroupTree(root); err == nil || !strings.Contains(err.Error(), "invalid GroupPolicy metadata") {
			t.Fatalf("ValidateGroupTree() = %v", err)
		}
	})

	t.Run("navigation-only execution hook changed", func(t *testing.T) {
		root := NewGroupCommand("dws", "root")
		root.AddCommand(&cobra.Command{Use: "leaf", RunE: func(*cobra.Command, []string) error { return nil }})
		root.RunE = nil
		root.Run = func(*cobra.Command, []string) {}
		if err := ValidateGroupTree(root); err == nil || !strings.Contains(err.Error(), "does not retain framework help execution") {
			t.Fatalf("ValidateGroupTree() = %v", err)
		}
	})

	t.Run("hybrid business execution hook removed", func(t *testing.T) {
		root := &cobra.Command{Use: "dws", RunE: func(*cobra.Command, []string) error { return nil }}
		corecmd.ApplyGroupPolicy(root, corecmd.GroupPolicy{
			Mode: corecmd.GroupHybrid, Positionals: corecmd.PositionalsReject, Recovery: corecmd.RecoveryDisabled,
		})
		root.AddCommand(&cobra.Command{Use: "leaf", RunE: func(*cobra.Command, []string) error { return nil }})
		root.RunE = nil
		root.Run = func(*cobra.Command, []string) {}
		if err := ValidateGroupTree(root); err == nil || !strings.Contains(err.Error(), "lost its business RunE") {
			t.Fatalf("ValidateGroupTree() = %v", err)
		}
	})

	t.Run("nested validation error is propagated", func(t *testing.T) {
		root := NewGroupCommand("dws", "root")
		root.AddCommand(NewGroupCommand("stale", "stale"))
		if err := ValidateGroupTree(root); err == nil || !strings.Contains(err.Error(), `leaf command "dws stale" retains GroupPolicy`) {
			t.Fatalf("ValidateGroupTree() = %v", err)
		}
	})

	t.Run("deep recovery accepts an available descendant", func(t *testing.T) {
		root := &cobra.Command{Use: "dws"}
		corecmd.ApplyGroupPolicy(root, corecmd.GroupPolicy{
			Mode: corecmd.GroupNavigationOnly, Positionals: corecmd.PositionalsReject, Recovery: corecmd.RecoveryDeep,
		})
		root.AddCommand(&cobra.Command{Use: "leaf", RunE: func(*cobra.Command, []string) error { return nil }})
		if err := ValidateGroupTree(root); err != nil {
			t.Fatalf("ValidateGroupTree() = %v", err)
		}
	})
}

func TestCrossPlatformCoverageMergeCommandTreeGroupPolicy(t *testing.T) {
	t.Run("copies source declaration", func(t *testing.T) {
		dst := &cobra.Command{Use: "root"}
		src := NewGroupCommand("root", "source")
		MergeCommandTree(dst, src)
		policy, ok, err := corecmd.GroupPolicyFor(dst)
		if err != nil || !ok || policy.Recovery != corecmd.RecoverySibling {
			t.Fatalf("merged policy = %+v, %v, %v", policy, ok, err)
		}
	})

	t.Run("typed destination accepts metadata-only source", func(t *testing.T) {
		dst := NewGroupCommand("root", "destination")
		src := &cobra.Command{Use: "root", Long: "source details"}
		MergeCommandTree(dst, src)
		if dst.Long != "source details" {
			t.Fatalf("Long = %q", dst.Long)
		}
	})

	t.Run("accepts identical declarations", func(t *testing.T) {
		dst := NewGroupCommand("root", "destination")
		src := NewGroupCommand("root", "source")
		MergeCommandTree(dst, src)
	})

	t.Run("neutral scaffold preserves owning hybrid deep policy", func(t *testing.T) {
		businessCalled := false
		dst := &cobra.Command{
			Use: "root",
			RunE: func(*cobra.Command, []string) error {
				businessCalled = true
				return nil
			},
		}
		want := corecmd.GroupPolicy{
			Mode: corecmd.GroupHybrid, Positionals: corecmd.PositionalsReject, Recovery: corecmd.RecoveryDeep,
		}
		corecmd.ApplyGroupPolicy(dst, want)
		dst.AddCommand(&cobra.Command{Use: "native", RunE: func(*cobra.Command, []string) error { return nil }})

		src := NewGroupCommand("root", "neutral scaffold")
		src.AddCommand(&cobra.Command{Use: "overlay", RunE: func(*cobra.Command, []string) error { return nil }})
		MergeCommandTree(dst, src)

		got, ok, err := corecmd.GroupPolicyFor(dst)
		if err != nil || !ok || got != want {
			t.Fatalf("merged owning policy = %+v, %v, %v; want %+v", got, ok, err, want)
		}
		if ChildByName(dst, "overlay") == nil {
			t.Fatal("neutral scaffold child was not merged")
		}
		if err := dst.RunE(dst, nil); err != nil || !businessCalled {
			t.Fatalf("owning Hybrid RunE was not preserved: called=%v err=%v", businessCalled, err)
		}
	})

	t.Run("rejects conflicting declarations", func(t *testing.T) {
		dst := NewGroupCommand("root", "destination")
		src := &cobra.Command{Use: "root"}
		corecmd.ApplyGroupPolicy(src, corecmd.GroupPolicy{
			Mode: corecmd.GroupNavigationOnly, Positionals: corecmd.PositionalsReject, Recovery: corecmd.RecoveryDisabled,
		})
		defer func() {
			got := recover()
			if got == nil || !strings.Contains(got.(string), "conflicting GroupPolicy") {
				t.Fatalf("MergeCommandTree panic = %v", got)
			}
		}()
		MergeCommandTree(dst, src)
	})

	t.Run("hybrid deep target rejects non-neutral source", func(t *testing.T) {
		dst := &cobra.Command{Use: "root", RunE: func(*cobra.Command, []string) error { return nil }}
		corecmd.ApplyGroupPolicy(dst, corecmd.GroupPolicy{
			Mode: corecmd.GroupHybrid, Positionals: corecmd.PositionalsReject, Recovery: corecmd.RecoveryDeep,
		})
		src := &cobra.Command{Use: "root"}
		corecmd.ApplyGroupPolicy(src, corecmd.GroupPolicy{
			Mode: corecmd.GroupNavigationOnly, Positionals: corecmd.PositionalsReject, Recovery: corecmd.RecoveryDeep,
		})
		defer func() {
			got := recover()
			if got == nil || !strings.Contains(got.(string), "conflicting GroupPolicy") {
				t.Fatalf("MergeCommandTree panic = %v", got)
			}
		}()
		MergeCommandTree(dst, src)
	})

	t.Run("does not overwrite undeclared runnable destination", func(t *testing.T) {
		dst := &cobra.Command{Use: "root", RunE: func(*cobra.Command, []string) error { return nil }}
		src := NewGroupCommand("root", "source")
		defer func() {
			got := recover()
			if got == nil || !strings.Contains(got.(string), "behavior-bearing leaf") {
				t.Fatalf("MergeCommandTree panic = %v", got)
			}
		}()
		MergeCommandTree(dst, src)
	})

	t.Run("does not swallow runnable leaf source into group destination", func(t *testing.T) {
		dst := NewGroupCommand("root", "destination")
		dst.AddCommand(&cobra.Command{Use: "native", RunE: func(*cobra.Command, []string) error { return nil }})
		src := &cobra.Command{Use: "root", RunE: func(*cobra.Command, []string) error { return nil }}
		src.Flags().String("source-only", "", "must not be silently discarded")
		defer func() {
			got := recover()
			if got == nil || !strings.Contains(got.(string), "behavior-bearing leaf") {
				t.Fatalf("MergeCommandTree panic = %v", got)
			}
		}()
		MergeCommandTree(dst, src)
	})

	t.Run("does not swallow parse behavior from source into group destination", func(t *testing.T) {
		dst := NewGroupCommand("root", "destination")
		src := &cobra.Command{Use: "root", Args: cobra.NoArgs}
		defer func() {
			got := recover()
			if got == nil || !strings.Contains(got.(string), "behavior-bearing leaf") {
				t.Fatalf("MergeCommandTree panic = %v", got)
			}
		}()
		MergeCommandTree(dst, src)
	})

	t.Run("rejects undeclared destination group", func(t *testing.T) {
		dst := &cobra.Command{Use: "root"}
		dst.AddCommand(&cobra.Command{Use: "child"})
		defer func() {
			got := recover()
			if got == nil || !strings.Contains(got.(string), "destination command") || !strings.Contains(got.(string), "no GroupPolicy") {
				t.Fatalf("MergeCommandTree panic = %v", got)
			}
		}()
		MergeCommandTree(dst, &cobra.Command{Use: "root"})
	})

	t.Run("rejects undeclared source group", func(t *testing.T) {
		src := &cobra.Command{Use: "root"}
		src.AddCommand(&cobra.Command{Use: "child"})
		defer func() {
			got := recover()
			if got == nil || !strings.Contains(got.(string), "source command") || !strings.Contains(got.(string), "no GroupPolicy") {
				t.Fatalf("MergeCommandTree panic = %v", got)
			}
		}()
		MergeCommandTree(&cobra.Command{Use: "root"}, src)
	})
}

func TestCrossPlatformCoverageMergeCommandTreeFailsClosedOnCorruption(t *testing.T) {
	mustPanic := func(t *testing.T, want string, fn func()) {
		t.Helper()
		defer func() {
			got := recover()
			message, ok := got.(string)
			if !ok || !strings.Contains(message, want) {
				t.Fatalf("panic = %v, want substring %q", got, want)
			}
		}()
		fn()
	}

	malformed := func() *cobra.Command {
		return &cobra.Command{
			Use: "root",
			Annotations: map[string]string{
				"dws.internal.corecmd.group_policy.v1": "malformed",
			},
		}
	}

	t.Run("malformed destination policy", func(t *testing.T) {
		mustPanic(t, "destination command", func() {
			MergeCommandTree(malformed(), &cobra.Command{Use: "root"})
		})
	})

	t.Run("malformed source policy", func(t *testing.T) {
		mustPanic(t, "source command", func() {
			MergeCommandTree(&cobra.Command{Use: "root"}, malformed())
		})
	})

	t.Run("local flag prevents placeholder merge", func(t *testing.T) {
		dst := NewGroupCommand("root", "destination")
		src := &cobra.Command{Use: "root"}
		src.Flags().String("local", "", "local parse behavior")
		mustPanic(t, "behavior-bearing leaf", func() {
			MergeCommandTree(dst, src)
		})
	})

	t.Run("persistent flag prevents placeholder merge", func(t *testing.T) {
		dst := NewGroupCommand("root", "destination")
		src := &cobra.Command{Use: "root"}
		src.PersistentFlags().String("persistent", "", "inherited parse behavior")
		mustPanic(t, "behavior-bearing leaf", func() {
			MergeCommandTree(dst, src)
		})
	})
}

func TestNewHiddenGroupCommand(t *testing.T) {
	t.Parallel()

	cmd := NewHiddenGroupCommand("secret", "hidden group")

	if !cmd.Hidden {
		t.Fatal("expected Hidden to be true")
	}
	if cmd.Use != "secret" {
		t.Fatalf("Use = %q, want %q", cmd.Use, "secret")
	}
	if cmd.Short != "hidden group" {
		t.Fatalf("Short = %q, want %q", cmd.Short, "hidden group")
	}
}

func TestNewPlaceholderParent(t *testing.T) {
	t.Parallel()

	child1 := &cobra.Command{Use: "child1"}
	child2 := &cobra.Command{Use: "child2"}

	cmd := NewPlaceholderParent("parent", "parent desc", child1, child2)

	if cmd.Use != "parent" {
		t.Fatalf("Use = %q, want %q", cmd.Use, "parent")
	}
	if len(cmd.Commands()) != 2 {
		t.Fatalf("expected 2 children, got %d", len(cmd.Commands()))
	}
	if ChildByName(cmd, "child1") == nil {
		t.Fatal("child1 not found")
	}
	if ChildByName(cmd, "child2") == nil {
		t.Fatal("child2 not found")
	}
}

func TestIsGenericOverlayShort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "Generated compatibility overlay prefix",
			input: "Generated compatibility overlay for foo",
			want:  true,
		},
		{
			name:  "Generated raw tool overlay prefix",
			input: "Generated raw tool overlay for bar",
			want:  true,
		},
		{
			name:  "Fallback-only prefix",
			input: "Fallback-only command",
			want:  true,
		},
		{
			name:  "non-matching string",
			input: "A real description of a command",
			want:  false,
		},
		{
			name:  "empty string",
			input: "",
			want:  false,
		},
		{
			name:  "partial match not at prefix",
			input: "This is a Generated compatibility overlay",
			want:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := IsGenericOverlayShort(tc.input)
			if got != tc.want {
				t.Fatalf("IsGenericOverlayShort(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestMergeCommandTree(t *testing.T) {
	t.Parallel()

	t.Run("nil inputs are safe", func(t *testing.T) {
		t.Parallel()
		MergeCommandTree(nil, nil)
		MergeCommandTree(&cobra.Command{Use: "a"}, nil)
		MergeCommandTree(nil, &cobra.Command{Use: "b"})
	})

	t.Run("short override from generic to real", func(t *testing.T) {
		t.Parallel()
		dst := &cobra.Command{Use: "root", Short: "Generated compatibility overlay for root"}
		src := &cobra.Command{Use: "root", Short: "Real description"}
		MergeCommandTree(dst, src)
		if dst.Short != "Real description" {
			t.Fatalf("Short = %q, want %q", dst.Short, "Real description")
		}
	})

	t.Run("short not overridden when dst is real", func(t *testing.T) {
		t.Parallel()
		dst := &cobra.Command{Use: "root", Short: "Real description"}
		src := &cobra.Command{Use: "root", Short: "Another description"}
		MergeCommandTree(dst, src)
		if dst.Short != "Real description" {
			t.Fatalf("Short = %q, want %q", dst.Short, "Real description")
		}
	})

	t.Run("short override from empty", func(t *testing.T) {
		t.Parallel()
		dst := &cobra.Command{Use: "root", Short: ""}
		src := &cobra.Command{Use: "root", Short: "New description"}
		MergeCommandTree(dst, src)
		if dst.Short != "New description" {
			t.Fatalf("Short = %q, want %q", dst.Short, "New description")
		}
	})

	t.Run("long override from empty", func(t *testing.T) {
		t.Parallel()
		dst := &cobra.Command{Use: "root", Long: ""}
		src := &cobra.Command{Use: "root", Long: "Detailed description"}
		MergeCommandTree(dst, src)
		if dst.Long != "Detailed description" {
			t.Fatalf("Long = %q, want %q", dst.Long, "Detailed description")
		}
	})

	t.Run("long not overridden when dst is set", func(t *testing.T) {
		t.Parallel()
		dst := &cobra.Command{Use: "root", Long: "Already set"}
		src := &cobra.Command{Use: "root", Long: "Other long"}
		MergeCommandTree(dst, src)
		if dst.Long != "Already set" {
			t.Fatalf("Long = %q, want %q", dst.Long, "Already set")
		}
	})

	t.Run("hidden override from true to false", func(t *testing.T) {
		t.Parallel()
		dst := &cobra.Command{Use: "root", Hidden: true}
		src := &cobra.Command{Use: "root", Hidden: false}
		MergeCommandTree(dst, src)
		if dst.Hidden {
			t.Fatal("expected Hidden to become false")
		}
	})

	t.Run("lower priority source cannot unhide destination", func(t *testing.T) {
		t.Parallel()
		dst := &cobra.Command{Use: "root", Hidden: true}
		src := &cobra.Command{Use: "root", Hidden: false}
		SetOverridePriority(src, -100)
		MergeCommandTree(dst, src)
		if !dst.Hidden {
			t.Fatal("lower priority fallback must preserve an explicit hidden command")
		}
	})

	t.Run("hidden stays false when both false", func(t *testing.T) {
		t.Parallel()
		dst := &cobra.Command{Use: "root", Hidden: false}
		src := &cobra.Command{Use: "root", Hidden: true}
		MergeCommandTree(dst, src)
		if dst.Hidden {
			t.Fatal("expected Hidden to remain false")
		}
	})

	t.Run("child merge recursive", func(t *testing.T) {
		t.Parallel()
		dst := NewGroupCommand("root", "destination")
		dstChild := &cobra.Command{Use: "sub", Short: ""}
		dst.AddCommand(dstChild)

		src := NewGroupCommand("root", "source")
		srcChild := &cobra.Command{Use: "sub", Short: "Merged short"}
		src.AddCommand(srcChild)

		MergeCommandTree(dst, src)
		found := ChildByName(dst, "sub")
		if found == nil {
			t.Fatal("expected child 'sub' to exist")
		}
		if found.Short != "Merged short" {
			t.Fatalf("child Short = %q, want %q", found.Short, "Merged short")
		}
	})

	t.Run("leaf replacement by higher priority", func(t *testing.T) {
		t.Parallel()
		dst := NewGroupCommand("root", "destination")
		dstLeaf := &cobra.Command{Use: "leaf", Short: "old"}
		SetOverridePriority(dstLeaf, 1)
		dst.AddCommand(dstLeaf)

		src := NewGroupCommand("root", "source")
		srcLeaf := &cobra.Command{Use: "leaf", Short: "new"}
		SetOverridePriority(srcLeaf, 5)
		src.AddCommand(srcLeaf)

		MergeCommandTree(dst, src)
		found := ChildByName(dst, "leaf")
		if found == nil {
			t.Fatal("expected child 'leaf' to exist")
		}
		if found.Short != "new" {
			t.Fatalf("leaf Short = %q, want %q", found.Short, "new")
		}
	})

	t.Run("new child addition", func(t *testing.T) {
		t.Parallel()
		dst := NewGroupCommand("root", "destination")
		dst.AddCommand(&cobra.Command{Use: "existing"})

		src := NewGroupCommand("root", "source")
		src.AddCommand(&cobra.Command{Use: "brand-new", Short: "added"})

		MergeCommandTree(dst, src)
		found := ChildByName(dst, "brand-new")
		if found == nil {
			t.Fatal("expected new child 'brand-new' to be added")
		}
		if found.Short != "added" {
			t.Fatalf("Short = %q, want %q", found.Short, "added")
		}
	})
}

func TestReplaceChild(t *testing.T) {
	t.Parallel()

	t.Run("nil inputs are safe", func(t *testing.T) {
		t.Parallel()
		ReplaceChild(nil, nil, nil)
		ReplaceChild(&cobra.Command{Use: "p"}, nil, &cobra.Command{Use: "n"})
		ReplaceChild(&cobra.Command{Use: "p"}, &cobra.Command{Use: "o"}, nil)
	})

	t.Run("normal replacement", func(t *testing.T) {
		t.Parallel()
		parent := &cobra.Command{Use: "root"}
		old := &cobra.Command{Use: "child", Short: "old"}
		parent.AddCommand(old)

		replacement := &cobra.Command{Use: "child", Short: "new"}
		ReplaceChild(parent, old, replacement)

		found := ChildByName(parent, "child")
		if found == nil {
			t.Fatal("expected child to exist")
		}
		if found.Short != "new" {
			t.Fatalf("Short = %q, want %q", found.Short, "new")
		}
	})
}

func TestLocalFlagCount(t *testing.T) {
	t.Parallel()

	t.Run("nil cmd returns 0", func(t *testing.T) {
		t.Parallel()
		if got := LocalFlagCount(nil); got != 0 {
			t.Fatalf("LocalFlagCount(nil) = %d, want 0", got)
		}
	})

	t.Run("hidden flags are excluded", func(t *testing.T) {
		t.Parallel()
		cmd := &cobra.Command{Use: "test"}
		cmd.Flags().String("visible", "", "visible flag")
		cmd.Flags().String("secret", "", "hidden flag")
		_ = cmd.Flags().MarkHidden("secret")

		if got := LocalFlagCount(cmd); got != 1 {
			t.Fatalf("LocalFlagCount() = %d, want 1", got)
		}
	})

	t.Run("visible flags are counted", func(t *testing.T) {
		t.Parallel()
		cmd := &cobra.Command{Use: "test"}
		cmd.Flags().String("a", "", "flag a")
		cmd.Flags().String("b", "", "flag b")
		cmd.Flags().Int("c", 0, "flag c")

		if got := LocalFlagCount(cmd); got != 3 {
			t.Fatalf("LocalFlagCount() = %d, want 3", got)
		}
	})
}

func TestLegacyCommandPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cmd  func() *cobra.Command
		want string
	}{
		{
			name: "normal path strips dws prefix",
			cmd: func() *cobra.Command {
				root := &cobra.Command{Use: "dws"}
				sub := &cobra.Command{Use: "product"}
				leaf := &cobra.Command{Use: "action"}
				sub.AddCommand(leaf)
				root.AddCommand(sub)
				return leaf
			},
			want: "product action",
		},
		{
			name: "root only returns dws unchanged",
			cmd: func() *cobra.Command {
				return &cobra.Command{Use: "dws"}
			},
			want: "dws",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := LegacyCommandPath(tc.cmd())
			if got != tc.want {
				t.Fatalf("LegacyCommandPath() = %q, want %q", got, tc.want)
			}
		})
	}
}
