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

package corecmd

import (
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
)

func TestCrossPlatformCoverageNewCommandEmbedsFullContractDeclAsFinalSource(t *testing.T) {
	cmd := New(Spec{
		Use:   "create",
		Short: "short",
		Long:  "long",
		Flags: []FlagSpec{
			{
				Name: "mode", Usage: "mode usage", Bind: "mode",
				Enum: []string{"a", "b"}, Format: "token", Example: "a",
				RequiredWhen: "when x", SchemaDescription: "schema desc",
			},
		},
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "user_required", Idempotency: "retryable",
		},
		Contract: ContractDecl{
			Title:       "Create Title",
			Description: "Create Desc",
			Positionals: []contract.RuntimeSchemaPositional{{Name: "id", Required: true, Index: 0}},
			DryRun:      &contract.DryRunSpec{PreviewKind: "invocation", RemoteReads: true},
			Result: &contract.ResultSpec{
				Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess},
				DataSchema: []byte(`{"type":"object"}`),
			},
			Pagination: &contract.PaginationSpec{Kind: contract.PaginationKindCursor, CursorParameter: "cursor"},
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "dev", RPCName: "create_thing"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "summary",
				UseWhen:      []string{"when create"},
				AvoidWhen:    []string{"when read"},
				Examples:     []string{"dws create --mode a"},
			},
			Identity: contract.ToolIdentitySpec{
				ProductID: "dev", Name: "create_thing",
				CLIPath: "dev create", CanonicalPath: "dev.create_thing",
			},
		},
		Invoke: func(*Ctx, map[string]any) error { return nil },
	})

	if cmd.Annotations != nil {
		if _, ok := cmd.Annotations["dws.schema.final"]; ok {
			t.Fatal("framework must convert typed ContractDecl; must not write JSON dws.schema.final")
		}
	}
	final, ok := contractfinal.RuntimeContractFinal(cmd)
	if !ok {
		t.Fatal("expected typed ContractFinal registration")
	}
	if final.Title != "Create Title" || final.Description != "Create Desc" {
		t.Fatalf("title/desc = %q %q (payload stores declared Contract text; Catalog may prefer Cobra Long)", final.Title, final.Description)
	}
	if final.Safety == nil || final.Safety.Confirmation != "user_required" || final.Safety.Idempotency != "retryable" {
		t.Fatalf("safety = %#v", final.Safety)
	}
	if final.DryRun == nil || final.DryRun.PreviewKind != "invocation" || !final.DryRun.RemoteReads {
		t.Fatalf("dry_run = %#v", final.DryRun)
	}
	if final.Result == nil || len(final.Result.Outcomes) != 1 {
		t.Fatalf("result = %#v", final.Result)
	}
	if final.Pagination == nil || final.Pagination.CursorParameter != "cursor" || final.Pagination.MetaPath != contract.PaginationMetaPath {
		t.Fatalf("pagination = %#v", final.Pagination)
	}
	if final.Interface == nil || final.Interface.Mode != "mcp" || final.Interface.Ref == nil || final.Interface.Ref.RPCName != "create_thing" {
		t.Fatalf("interface = %#v", final.Interface)
	}
	if final.Selection == nil || final.Selection.AgentSummary != "summary" || len(final.Selection.UseWhen) != 1 {
		t.Fatalf("selection = %#v", final.Selection)
	}
	if final.Identity == nil || final.Identity.ProductID != "dev" || final.Identity.Name != "create_thing" {
		t.Fatalf("identity = %#v", final.Identity)
	}
	if len(final.Positionals) != 1 || final.Positionals[0].Name != "id" {
		t.Fatalf("positionals = %#v", final.Positionals)
	}

	flag := cmd.Flags().Lookup("mode")
	if flag == nil {
		t.Fatal("missing mode flag")
	}
	if got := flag.Annotations["dws.schema.description"]; len(got) == 0 || got[0] != "schema desc" {
		t.Fatalf("description annotation = %#v", flag.Annotations["dws.schema.description"])
	}
	if got := flag.Annotations["dws.schema.required_when"]; len(got) == 0 || got[0] != "when x" {
		t.Fatalf("required_when = %#v", flag.Annotations["dws.schema.required_when"])
	}
	if got := flag.Annotations["x-cli-format"]; len(got) == 0 || got[0] != "token" {
		t.Fatalf("format = %#v", flag.Annotations["x-cli-format"])
	}
	if got := flag.Annotations["x-cli-enum"]; len(got) != 2 {
		t.Fatalf("enum = %#v", flag.Annotations["x-cli-enum"])
	}
}

func TestFrameworkContractDeclResultMarksNonEmptyAndRejectsInvalidSchema(t *testing.T) {
	if (ContractDecl{Result: &contract.ResultSpec{}}).Empty() {
		t.Fatal("Result declaration was treated as empty")
	}
	defer func() {
		if recovered := recover(); recovered == nil || !strings.Contains(recovered.(string), "invalid Contract.Result") {
			t.Fatalf("panic=%v", recovered)
		}
	}()
	New(Spec{
		Use:    "bad-result",
		Safety: contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
		Contract: ContractDecl{
			Title: "Bad", Description: "bad result",
			Result:    &contract.ResultSpec{Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess}},
			Interface: &contract.InterfaceSpec{Mode: "local", Availability: "available"},
			Selection: contract.SelectionSpec{AgentSummary: "bad", UseWhen: []string{"bad"}, AvoidWhen: []string{"good"}, Examples: []string{"dws bad-result"}},
			Identity:  contract.ToolIdentitySpec{ProductID: "sample", Name: "bad", CanonicalPath: "sample.bad", CLIPath: "bad-result", PrimaryCLIPath: "bad-result"},
		},
		Invoke: func(*Ctx, map[string]any) error { return nil },
	})
}

func TestFrameworkContractDeclPaginationMarksNonEmptyAndRejectsInvalidSpec(t *testing.T) {
	if (ContractDecl{Pagination: &contract.PaginationSpec{}}).Empty() {
		t.Fatal("Pagination declaration was treated as empty")
	}
	defer func() {
		recovered := recover()
		if recovered == nil || !strings.Contains(recovered.(string), "invalid Contract.Pagination") {
			t.Fatalf("panic=%v", recovered)
		}
	}()
	New(Spec{
		Use:    "bad-pagination",
		Safety: contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
		Contract: ContractDecl{
			Title:       "Bad pagination",
			Description: "bad pagination",
			Pagination:  &contract.PaginationSpec{Kind: "offset", CursorParameter: "cursor"},
			Interface:   &contract.InterfaceSpec{Mode: "local", Availability: "available"},
			Selection:   contract.SelectionSpec{AgentSummary: "bad", UseWhen: []string{"bad"}, AvoidWhen: []string{"good"}, Examples: []string{"dws bad-pagination"}},
			Identity:    contract.ToolIdentitySpec{ProductID: "sample", Name: "bad_pagination", CanonicalPath: "sample.bad_pagination", CLIPath: "bad-pagination", PrimaryCLIPath: "bad-pagination"},
		},
		Invoke: func(*Ctx, map[string]any) error { return nil },
	})
}

func TestNewCommandFallsBackToDeclaredDescriptionWithoutLong(t *testing.T) {
	// Long wins when authored; without one the mandatory declaration supplies it.
	cmd := New(Spec{
		Use:   "create",
		Short: "short",
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: ContractDecl{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "dev",
				Name:           "create_thing",
				CanonicalPath:  "dev.create_thing",
				CLIPath:        "dev create",
				PrimaryCLIPath: "dev create",
			},

			Title:       "Create Title",
			Description: "Create Desc",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "dev", RPCName: "create_thing"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "summary",
				UseWhen:      []string{"when create"},
				AvoidWhen:    []string{"when read"},
				Examples:     []string{"dws create"},
			},
		},
		Invoke: func(*Ctx, map[string]any) error { return nil },
	})
	final, ok := contractfinal.RuntimeContractFinal(cmd)
	if !ok {
		t.Fatal("expected typed ContractFinal registration")
	}
	if final.Description != "Create Desc" {
		t.Fatalf("description = %q, want the declared fallback", final.Description)
	}
}

func TestCrossPlatformCoverageNewCommandPanicsOnPartialContractDecl(t *testing.T) {
	completeIface := &contract.InterfaceSpec{Mode: "mcp", Availability: "available", Ref: &contract.InterfaceRefSpec{ProductID: "dev", RPCName: "get_thing"}}
	completeSel := contract.SelectionSpec{AgentSummary: "s", UseWhen: []string{"u"}, AvoidWhen: []string{"a"}, Examples: []string{"dws x"}}
	for _, tc := range []struct {
		name     string
		contract ContractDecl
		wantSub  string
	}{
		{"missing description", ContractDecl{
			Selection: contract.SelectionSpec{AgentSummary: "s", UseWhen: []string{"u"}, AvoidWhen: []string{"a"}, Examples: []string{"dws x"}},
		}, "Contract.Description"},
		{"missing selection", ContractDecl{
			Description: "d",
		}, "Contract.Selection.AgentSummary"},
		{"missing examples", ContractDecl{
			Description: "d",
			Interface:   completeIface,
			Selection:   contract.SelectionSpec{AgentSummary: "s", UseWhen: []string{"u"}, AvoidWhen: []string{"a"}},
		}, "Contract.Selection.Examples"},
		{"missing interface", ContractDecl{
			Description: "d",
			Selection:   completeSel,
		}, "Contract.Interface"},
		{"composite without reason", ContractDecl{
			Description: "d",
			Interface:   &contract.InterfaceSpec{Mode: "composite", Availability: "available"},
			Selection:   completeSel,
		}, "Contract.Interface.Reason"},
		{"missing identity", ContractDecl{
			Description: "d",
			Interface:   completeIface,
			Selection:   completeSel,
		}, "Contract.Identity"},
		{"canonical path mismatch", ContractDecl{
			Description: "d",
			Interface:   completeIface,
			Selection:   completeSel,
			Identity: contract.ToolIdentitySpec{
				ProductID: "dev", Name: "create_thing", CanonicalPath: "dev.other_thing",
				CLIPath: "dev create",
			},
		}, "must equal ProductID.Name"},
		{"cli and primary disagree", ContractDecl{
			Description: "d",
			Interface:   completeIface,
			Selection:   completeSel,
			Identity: contract.ToolIdentitySpec{
				ProductID: "dev", Name: "create_thing", CanonicalPath: "dev.create_thing",
				CLIPath: "dev create", PrimaryCLIPath: "dev other",
			},
		}, "must agree on the primary leaf path"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				recovered := recover()
				if recovered == nil {
					t.Fatal("partial ContractDecl must panic at construction")
				}
				if msg, _ := recovered.(string); !strings.Contains(msg, tc.wantSub) {
					t.Fatalf("panic = %v, want mention %s", recovered, tc.wantSub)
				}
			}()
			New(Spec{
				Use:      "x",
				Short:    "x",
				Contract: tc.contract,
				Invoke:   func(*Ctx, map[string]any) error { return nil },
			})
		})
	}
}

func TestNewCommandDerivesHelpExampleFromDeclaredSelection(t *testing.T) {
	decl := ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID: "dev", Name: "create_thing", CanonicalPath: "dev.create_thing",
			CLIPath: "dev create", PrimaryCLIPath: "dev create",
		},
		Description: "desc",
		Interface:   &contract.InterfaceSpec{Mode: "mcp", Availability: "available", Ref: &contract.InterfaceRefSpec{ProductID: "dev", RPCName: "create_thing"}},
		Selection: contract.SelectionSpec{
			AgentSummary: "summary",
			UseWhen:      []string{"when"},
			AvoidWhen:    []string{"avoid"},
			Examples:     []string{"dws create --mode a", "dws create --mode b --dry-run"},
		},
	}
	cmd := New(Spec{
		Use:      "create",
		Short:    "short",
		Safety:   testWriteSafety(),
		Contract: decl,
		Invoke:   func(*Ctx, map[string]any) error { return nil },
	})
	want := "  dws create --mode a\n  dws create --mode b --dry-run"
	if cmd.Example != want {
		t.Fatalf("derived Example = %q, want %q", cmd.Example, want)
	}

	decl.Selection.Examples = []string{"dws create --mode a"}
	explicit := New(Spec{
		Use:      "create",
		Short:    "short",
		Example:  "  dws create --custom",
		Safety:   testWriteSafety(),
		Contract: decl,
		Invoke:   func(*Ctx, map[string]any) error { return nil },
	})
	if explicit.Example != "  dws create --custom" {
		t.Fatalf("authored Example must win over derivation, got %q", explicit.Example)
	}
}

func TestNewCommandSafetySpecPassThrough(t *testing.T) {
	decl := func() ContractDecl {
		return ContractDecl{
			Identity: contract.ToolIdentitySpec{
				ProductID: "dev", Name: "create_thing", CanonicalPath: "dev.create_thing",
				CLIPath: "dev create", PrimaryCLIPath: "dev create",
			},
			Description: "desc",
			Interface:   &contract.InterfaceSpec{Mode: "mcp", Availability: "available", Ref: &contract.InterfaceRefSpec{ProductID: "dev", RPCName: "op"}},
			Selection: contract.SelectionSpec{
				AgentSummary: "s", UseWhen: []string{"u"}, AvoidWhen: []string{"a"}, Examples: []string{"dws x"},
			},
		}
	}
	build := func(spec Spec) *contract.SafetySpec {
		cmd := New(spec)
		final, ok := contractfinal.RuntimeContractFinal(cmd)
		if !ok || final.Safety == nil {
			t.Fatalf("expected declared safety, final=%#v ok=%v", final, ok)
		}
		return final.Safety
	}

	declared := contract.SafetySpec{
		Effect: "write", Risk: "low",
		Confirmation: "not_required", Idempotency: "non_idempotent",
	}
	if got := build(Spec{Use: "w", Short: "w", Safety: declared, Contract: decl(),
		Invoke: func(*Ctx, map[string]any) error { return nil }}); got.Effect != declared.Effect ||
		got.Risk != declared.Risk || got.Confirmation != declared.Confirmation ||
		got.Idempotency != declared.Idempotency {
		t.Fatalf("SafetySpec must pass through without cross-field inference: %#v", got)
	}
	// A wholly empty declaration preserves the historical read-only default.
	if got := build(Spec{Use: "r", Short: "r", Contract: decl(),
		Invoke: func(*Ctx, map[string]any) error { return nil }}); got.Effect != "read" || got.Risk != "low" ||
		got.Confirmation != "not_required" || got.Idempotency != "idempotent" {
		t.Fatalf("empty Safety must use read default, = %#v", got)
	}
}

func TestContractDeclEmptySkipsFinal(t *testing.T) {
	cmd := New(Spec{
		Use:    "get",
		Short:  "g",
		Flags:  []FlagSpec{{Name: "id", Usage: "id"}},
		Safety: testWriteSafety(),
		Invoke: func(*Ctx, map[string]any) error { return nil },
	})
	if contractfinal.HasRuntimeContractFinal(cmd) {
		t.Fatal("Safety without Contract must not register Final (keep runtime write light)")
	}
	if _, ok := cmd.Annotations["dws.schema.risk"]; ok {
		t.Fatal("Safety must not use the removed dws.schema.risk annotation")
	}
}

func TestCrossPlatformCoverageNewCommandRejectsPartialSafetySpec(t *testing.T) {
	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("partial SafetySpec must panic at construction")
		}
		if msg, _ := recovered.(string); !strings.Contains(msg, "Safety.Confirmation") {
			t.Fatalf("panic = %v, want missing Safety.Confirmation", recovered)
		}
	}()
	New(Spec{
		Use:    "partial",
		Safety: contract.SafetySpec{Effect: "write", Risk: "medium", Idempotency: "unknown"},
		Invoke: func(*Ctx, map[string]any) error { return nil },
	})
}

func TestCrossPlatformCoverageContractDeclEmptyReportsEveryAuthoredSection(t *testing.T) {
	if !(ContractDecl{}).Empty() {
		t.Fatal("zero ContractDecl must be empty")
	}

	authored := map[string]ContractDecl{
		"title":               {Title: "T"},
		"description":         {Description: "D"},
		"positionals":         {Positionals: []contract.RuntimeSchemaPositional{{Name: "id"}}},
		"parameters":          {Parameters: []contract.ParamDecl{{Name: "mode"}}},
		"dry_run":             {DryRun: &contract.DryRunSpec{PreviewKind: "request"}},
		"interface":           {Interface: &contract.InterfaceSpec{Mode: "mcp"}},
		"selection":           {Selection: contract.SelectionSpec{Tips: []string{"tip"}}},
		"identity.name":       {Identity: contract.ToolIdentitySpec{Name: "tool"}},
		"identity.group":      {Identity: contract.ToolIdentitySpec{Group: "ops"}},
		"identity.source":     {Identity: contract.ToolIdentitySpec{Source: "native"}},
		"identity.cli_name":   {Identity: contract.ToolIdentitySpec{CLIName: "create"}},
		"identity.source_pid": {Identity: contract.ToolIdentitySpec{SourceProductID: "doc"}},
	}
	for name, decl := range authored {
		if decl.Empty() {
			t.Fatalf("%s: authored section must make ContractDecl non-empty", name)
		}
	}

	// Pointers without any authored payload still count as empty: a bare
	// &contract.DryRunSpec{} / &contract.InterfaceSpec{} carries no final Schema fact.
	unauthored := map[string]ContractDecl{
		"dry_run pointer":   {DryRun: &contract.DryRunSpec{RemoteReads: true}},
		"interface pointer": {Interface: &contract.InterfaceSpec{}},
	}
	for name, decl := range unauthored {
		if !decl.Empty() {
			t.Fatalf("%s: payload-free pointer must stay empty", name)
		}
	}
}
