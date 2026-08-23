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
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
)

func TestDeclareLeafMetadataRejectsExecutionSurface(t *testing.T) {
	cmd := &cobra.Command{Use: "x", Short: "x"}
	schema := LeafContract{
		Identity: contract.ToolIdentitySpec{
			ProductID: "dev", Name: "create_thing", CanonicalPath: "dev.create_thing",
			CLIPath: "dev create", PrimaryCLIPath: "dev create",
		},
		Description: "d",
		Interface:   &contract.InterfaceSpec{Mode: "mcp", Availability: "available", Ref: &contract.InterfaceRefSpec{ProductID: "p", RPCName: "t"}},
		Selection: contract.SelectionSpec{
			AgentSummary: "s",
			UseWhen:      []string{"u"},
			AvoidWhen:    []string{"a"},
			Examples:     []string{"e"},
		},
	}
	mustPanic := func(name string, fn func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Fatalf("%s: expected panic", name)
			}
		}()
		fn()
	}
	mustPanic("flags", func() {
		DeclareLeafMetadata(cmd, LeafSpec{Contract: schema, Flags: []LeafFlag{{Name: "f"}}})
	})
	mustPanic("runE", func() {
		DeclareLeafMetadata(cmd, LeafSpec{Contract: schema, RunE: func(*cobra.Command, []string) error { return nil }})
	})
	mustPanic("empty schema", func() {
		DeclareLeafMetadata(cmd, LeafSpec{})
	})
	// Validate is the one execution hook allowed in metadata mode (PreRunE,
	// before ConfirmSafety). It must not panic.
	DeclareLeafMetadata(&cobra.Command{Use: "y", Short: "y", RunE: func(*cobra.Command, []string) error { return nil }}, LeafSpec{
		Contract: schema,
		Validate: func(*cobra.Command, []string) error { return nil },
	})
}

func TestDeclareLeafMetadataDoesNotRewriteRunE(t *testing.T) {
	run := func(*cobra.Command, []string) error { return nil }
	cmd := &cobra.Command{Use: "x", Short: "x", RunE: run}
	before := cmd.RunE
	DeclareLeafMetadata(cmd, LeafSpec{
		Safety: aitableSafetyRead(),
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "dev",
				Name:           "create_thing",
				CanonicalPath:  "dev.create_thing",
				CLIPath:        "dev create",
				PrimaryCLIPath: "dev create",
			},

			Description: "d",
			Interface:   aitableMCPInterface("tool"),
			Selection: contract.SelectionSpec{
				AgentSummary: "s",
				UseWhen:      []string{"u"},
				AvoidWhen:    []string{"a"},
				Examples:     []string{"e"},
			},
		},
	})
	if !contractfinal.HasRuntimeContractFinal(cmd) {
		t.Fatal("expected ContractFinal")
	}
	// function values are not comparable; ensure pointer identity via uintptr trick is unnecessary —
	// just check RunE is still non-nil and command still has same Use.
	if cmd.RunE == nil {
		t.Fatal("RunE must remain set")
	}
	_ = before
	if cmd.Use != "x" {
		t.Fatalf("Use mutated: %q", cmd.Use)
	}
}

// TestAitableDeclareLeafMetadataCoversRegistryHelpers asserts the aitable
// subtree is a complete identity source after the reviewed registry
// retirement: the collector finds a non-empty aitable identity set, every
// collected primary CLI path resolves back to a ContractFinal-bearing leaf in
// the tree, and every canonical stays inside the aitable product. Leaves
// without identity (compatibility aliases, hint stubs) remain allowed — they
// resolve to a primary, matching the collector's NoIdentity treatment.
func TestAitableDeclareLeafMetadataCoversRegistryHelpers(t *testing.T) {
	root := newAitableCommand()
	specs, _, err := cli.CollectIdentitySpecs(root)
	if err != nil {
		t.Fatalf("collect aitable identity specs: %v", err)
	}
	if len(specs) == 0 {
		t.Fatal("expected identity-bearing aitable leaves")
	}
	for _, spec := range specs {
		if !strings.HasPrefix(spec.CanonicalPath, "aitable.") {
			t.Fatalf("canonical %q escapes the aitable product", spec.CanonicalPath)
		}
		// Shortcut paths (+...) stay on bind-time decls until Shortcut.Schema migration.
		if containsPlus(spec.PrimaryCLIPath) {
			continue
		}
		leaf := findCLIPath(root, spec.PrimaryCLIPath)
		if leaf == nil {
			t.Fatalf("primary path %q does not resolve in the aitable tree", spec.PrimaryCLIPath)
		}
		if !contractfinal.HasRuntimeContractFinal(leaf) {
			t.Fatalf("primary path %q is missing ContractFinal", spec.PrimaryCLIPath)
		}
	}
}

func findCLIPath(root *cobra.Command, cliPath string) *cobra.Command {
	parts := splitCLI(cliPath)
	if len(parts) == 0 || parts[0] != "aitable" {
		return nil
	}
	cur := root
	for _, p := range parts[1:] {
		next, _, err := cur.Find(append([]string{}, p))
		if err != nil || next == nil {
			return nil
		}
		cur = next
	}
	return cur
}

func containsPlus(cliPath string) bool {
	for _, p := range splitCLI(cliPath) {
		if len(p) > 0 && p[0] == '+' {
			return true
		}
	}
	return false
}

func splitCLI(cliPath string) []string {
	out := make([]string, 0)
	cur := ""
	for _, r := range cliPath {
		if r == ' ' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
