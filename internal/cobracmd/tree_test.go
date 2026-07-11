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
	"testing"

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
		t.Fatal("Args should be set (cobra.NoArgs)")
	}
	// Verify Args rejects arguments.
	if err := cmd.Args(cmd, []string{"extra"}); err == nil {
		t.Fatal("expected Args to reject extra arguments")
	}
	// Verify RunE is set and returns help (no error for valid invocation).
	if cmd.RunE == nil {
		t.Fatal("RunE should not be nil")
	}
	// RunE calls cmd.Help() which should not error.
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("RunE returned unexpected error: %v", err)
	}
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
		dst := &cobra.Command{Use: "root"}
		dstChild := &cobra.Command{Use: "sub", Short: ""}
		dst.AddCommand(dstChild)

		src := &cobra.Command{Use: "root"}
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
		dst := &cobra.Command{Use: "root"}
		dstLeaf := &cobra.Command{Use: "leaf", Short: "old"}
		SetOverridePriority(dstLeaf, 1)
		dst.AddCommand(dstLeaf)

		src := &cobra.Command{Use: "root"}
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
		dst := &cobra.Command{Use: "root"}
		dst.AddCommand(&cobra.Command{Use: "existing"})

		src := &cobra.Command{Use: "root"}
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
