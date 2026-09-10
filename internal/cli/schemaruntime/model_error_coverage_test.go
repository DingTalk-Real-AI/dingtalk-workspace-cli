// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package schemaruntime

import (
	"encoding/json"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
)

func TestCrossPlatformCoverageModelIndexValidateAndPayloadErrors(t *testing.T) {
	valid := func(product, name, cli string) ToolSpec {
		return ToolSpec{Identity: contract.ToolIdentitySpec{
			ProductID: product, Name: name, CanonicalPath: product + "." + name,
			CLIPath: cli, PrimaryCLIPath: cli,
		}}
	}

	if _, ok := ToolProvenanceValue(valid("p", "n", "p n"), "title"); !ok {
		t.Fatal("tool provenance wrapper")
	}
	if _, ok := ParameterProvenanceValue(ParameterSpec{Name: "f"}, "name"); !ok {
		t.Fatal("param provenance wrapper")
	}
	if _, ok := ProductProvenanceValue(ProductSpec{Selection: contract.SelectionSpec{AgentSummary: "s"}}, "agent_summary"); !ok {
		t.Fatal("product provenance wrapper")
	}

	if _, err := (SchemaRegistry{Products: []ProductSpec{{ID: ""}}}).Index(); err == nil {
		t.Fatal("empty product id")
	}
	if _, err := (SchemaRegistry{Products: []ProductSpec{{ID: "a", Tools: []ToolSpec{valid("a", "t", "a t")}}, {ID: "a", Tools: []ToolSpec{valid("a", "u", "a u")}}}}).Index(); err == nil {
		t.Fatal("duplicate product")
	}
	if _, err := (SchemaRegistry{Products: []ProductSpec{{ID: "a", Tools: []ToolSpec{{Identity: contract.ToolIdentitySpec{ProductID: "a", CanonicalPath: "a.x", CLIPath: "a x", PrimaryCLIPath: "a x"}}}}}}).Index(); err == nil {
		t.Fatal("validate empty name")
	}
	aliased := valid("a", "t", "a t")
	aliased.Identity.IsAlias = true
	if _, err := (SchemaRegistry{Products: []ProductSpec{{ID: "a", Tools: []ToolSpec{aliased}}}}).Index(); err == nil {
		t.Fatal("canonical alias accepted")
	}
	mismatchCLI := valid("a", "t", "a t")
	mismatchCLI.Identity.PrimaryCLIPath = "a other"
	if _, err := (SchemaRegistry{Products: []ProductSpec{{ID: "a", Tools: []ToolSpec{mismatchCLI}}}}).Index(); err == nil {
		t.Fatal("cli/primary mismatch accepted")
	}
	foreign := valid("other", "run", "other run")
	if _, err := (SchemaRegistry{Products: []ProductSpec{{ID: "sample", Tools: []ToolSpec{foreign}}}}).Index(); err == nil {
		t.Fatal("foreign product tool accepted")
	}
	dupCanon := valid("a", "t", "a t")
	if _, err := (SchemaRegistry{Products: []ProductSpec{{ID: "a", Tools: []ToolSpec{dupCanon, dupCanon}}}}).Index(); err == nil {
		t.Fatal("duplicate canonical accepted")
	}
	early := valid("a", "aaa", "a aaa")
	early.Identity.Path = "a.zzz"
	late := valid("a", "zzz", "a zzz")
	if _, err := (SchemaRegistry{Products: []ProductSpec{{ID: "a", Tools: []ToolSpec{early, late}}}}).Index(); err == nil {
		t.Fatal("canonical/contract path conflict accepted")
	}
	left := valid("a", "one", "a one")
	left.Identity.Path = "shared.path"
	right := valid("a", "two", "a two")
	right.Identity.Path = "shared.path"
	if _, err := (SchemaRegistry{Products: []ProductSpec{{ID: "a", Tools: []ToolSpec{left, right}}}}).Index(); err == nil {
		t.Fatal("shared contract path accepted")
	}
	blocker := valid("a", "aaa", "a aaa")
	collider := valid("a", "bbb", "a bbb")
	collider.Identity.Path = "a.aaa"
	if _, err := (SchemaRegistry{Products: []ProductSpec{{ID: "a", Tools: []ToolSpec{blocker, collider}}}}).Index(); err == nil {
		t.Fatal("path colliding with canonical accepted")
	}
	cliLeft := valid("a", "one", "same cli")
	cliRight := valid("a", "two", "same cli")
	if _, err := (SchemaRegistry{Products: []ProductSpec{{ID: "a", Tools: []ToolSpec{cliLeft, cliRight}}}}).Index(); err == nil {
		t.Fatal("shared CLI path accepted")
	}

	emptyID := valid("a", "t", "a t")
	emptyID.Identity.ProductID = ""
	emptyID.Identity.CanonicalPath = ".t"
	if err := emptyID.Validate(); err == nil {
		t.Fatal("empty product id validated")
	}
	if err := (ToolSpec{Identity: contract.ToolIdentitySpec{ProductID: "a", Name: "t", CanonicalPath: "nope", CLIPath: "a t"}}).Validate(); err == nil {
		t.Fatal("canonical mismatch validated")
	}
	if err := (ToolSpec{Identity: contract.ToolIdentitySpec{ProductID: "a", Name: "t", CanonicalPath: "a.t"}}).Validate(); err == nil {
		t.Fatal("empty cli validated")
	}
	if err := valid("a", "t", "a t").Validate(); err != nil {
		t.Fatal(err)
	}
	dupParam := valid("a", "t", "a t")
	dupParam.Parameters = []ParameterSpec{{Name: "f"}, {Name: "f"}}
	if err := dupParam.Validate(); err == nil {
		t.Fatal("duplicate param validated")
	}
	emptyParam := valid("a", "t", "a t")
	emptyParam.Parameters = []ParameterSpec{{Name: ""}}
	if err := emptyParam.Validate(); err == nil {
		t.Fatal("empty param validated")
	}
	badJSON := valid("a", "t", "a t")
	badJSON.Parameters = []ParameterSpec{{Name: "f", Default: json.RawMessage("nope")}}
	if err := badJSON.Validate(); err == nil {
		t.Fatal("invalid default validated")
	}
	badIfaceDef := valid("a", "t", "a t")
	badIfaceDef.Parameters = []ParameterSpec{{Name: "f", InterfaceDefault: json.RawMessage("nope")}}
	if err := badIfaceDef.Validate(); err == nil {
		t.Fatal("invalid interface default validated")
	}
	badExample := valid("a", "t", "a t")
	badExample.Parameters = []ParameterSpec{{Name: "f", Example: json.RawMessage("nope")}}
	if err := badExample.Validate(); err == nil {
		t.Fatal("invalid example validated")
	}
	incompleteRef := valid("a", "t", "a t")
	incompleteRef.Interface.Ref = &contract.InterfaceRefSpec{ProductID: " "}
	if err := incompleteRef.Validate(); err == nil {
		t.Fatal("incomplete interface ref validated")
	}
	badDry := valid("a", "t", "a t")
	badDry.DryRun = &contract.DryRunSpec{}
	if err := badDry.Validate(); err == nil {
		t.Fatal("empty dry-run validated")
	}
	badResult := valid("a", "t", "a t")
	badResult.Result = &contract.ResultSpec{}
	if err := badResult.Validate(); err == nil {
		t.Fatal("empty result validated")
	}
	badPage := valid("a", "t", "a t")
	badPage.Parameters = []ParameterSpec{{Name: "flag"}}
	badPage.Pagination = &contract.PaginationSpec{Kind: contract.PaginationKindCursor, CursorParameter: "missing"}
	if err := badPage.Validate(); err == nil {
		t.Fatal("unknown cursor validated")
	}
	badIface := valid("a", "t", "a t")
	badIface.Interface.Mode = "nope"
	if err := badIface.Validate(); err == nil {
		t.Fatal("unknown interface mode validated")
	}
	badProv := valid("a", "t", "a t")
	badProv.FieldProvenance = map[string]contract.FieldProvenance{"title": {Source: "s"}}
	if err := badProv.Validate(); err == nil {
		t.Fatal("incomplete provenance validated")
	}
	badParamProv := valid("a", "t", "a t")
	badParamProv.Parameters = []ParameterSpec{{Name: "f", Type: "string", FieldProvenance: map[string]contract.FieldProvenance{"type": {Source: "s"}}}}
	if err := badParamProv.Validate(); err == nil {
		t.Fatal("incomplete param provenance validated")
	}
	if err := ValidateFinalFieldProvenance("o", "f", contract.FieldProvenance{}, make(chan int)); err == nil {
		t.Fatal("unmarshalable provenance accepted")
	}
	selected := true
	if err := ValidateFinalFieldProvenance("o", "f", contract.FieldProvenance{
		Value: json.RawMessage("1"), Source: "s", Precedence: "1", Resolution: "r",
		Candidates:           []contract.FieldCandidateProvenance{{Value: json.RawMessage("1"), Source: "s", Precedence: "1", Selected: &selected}},
		OverriddenCandidates: []contract.FieldCandidateProvenance{{Value: json.RawMessage("nope")}},
	}, 1); err == nil {
		t.Fatal("invalid overridden candidate accepted")
	}

	if _, err := (SchemaRegistry{Products: []ProductSpec{{ID: ""}}}).ToPayload(); err == nil {
		t.Fatal("registry payload index error")
	}
	if _, err := (SchemaRegistry{Products: []ProductSpec{{ID: ""}}}).ToOverviewPayload(); err == nil {
		t.Fatal("overview index error")
	}
	if _, err := (SchemaRegistry{Products: []ProductSpec{{ID: ""}}}).ToSnapshotPayload(); err == nil {
		t.Fatal("snapshot index error")
	}
	okReg := SchemaRegistry{Products: []ProductSpec{{ID: "a", Tools: []ToolSpec{valid("a", "t", "a t")}}}}
	if _, err := ToSnapshotPayloadWithProjectors(okReg, SnapshotProjectors{
		ToolSummary: func(ToolSpec) (map[string]any, error) { return nil, errQuery },
	}); err == nil {
		t.Fatal("forced tool summary")
	}
	if _, err := ToSnapshotPayloadWithProjectors(okReg, SnapshotProjectors{
		ToolPayload: func(ToolSpec) (map[string]any, error) { return nil, errQuery },
	}); err == nil {
		t.Fatal("forced tool payload")
	}
	if _, err := ToSnapshotPayloadWithProjectors(okReg, SnapshotProjectors{
		ProductSummary: func(ProductSpec) (map[string]any, error) { return nil, errQuery },
	}); err == nil {
		t.Fatal("forced product summary")
	}
	if _, err := (ProductSpec{ID: "a", Tools: []ToolSpec{emptyID}}).ToPayload(); err == nil {
		t.Fatal("product envelope tool error")
	}
	if _, err := emptyID.ToPayload(); err == nil {
		t.Fatal("tool payload validate error")
	}
	anyOfTool := valid("a", "t", "a t")
	anyOfTool.Parameters = []ParameterSpec{{Name: "f", Type: "int", AnyOf: []contract.FormatAlternative{{Format: "uri"}}}}
	if _, err := anyOfTool.ToPayload(); err == nil {
		t.Fatal("tool parameter anyOf error")
	}
	garbageProv := valid("a", "t", "a t")
	garbageProv.FieldProvenance = map[string]contract.FieldProvenance{"not-a-field": {Value: json.RawMessage("not-json")}}
	if _, err := garbageProv.ToPayload(); err == nil {
		t.Fatal("tool provenance marshal error")
	}
	garbageParamProv := valid("a", "t", "a t")
	garbageParamProv.Parameters = []ParameterSpec{{
		Name: "f", Type: "string",
		FieldProvenance: map[string]contract.FieldProvenance{"not-a-field": {Value: json.RawMessage("not-json")}},
	}}
	if _, err := garbageParamProv.ToPayload(); err == nil {
		t.Fatal("param provenance marshal error")
	}
	if _, err := emptyID.ToSummaryPayload(); err == nil {
		t.Fatal("tool summary validate error")
	}
	if _, err := (ParameterSpec{Name: "f", InterfaceDefault: json.RawMessage("nope")}).ToPayload(); err == nil {
		t.Fatal("interface default payload")
	}
	if _, err := (ParameterSpec{Name: "f", Example: json.RawMessage("nope")}).ToPayload(); err == nil {
		t.Fatal("example payload")
	}
	anyOf, err := (ParameterSpec{Name: "u", Type: "string", AnyOf: []contract.FormatAlternative{{Format: "uri"}}}).ToPayload()
	if err != nil || anyOf["anyOf"] == nil {
		t.Fatalf("anyOf success = %#v %v", anyOf, err)
	}
	useWhen := SchemaRegistry{Products: []ProductSpec{{
		ID: "a", Tools: []ToolSpec{valid("a", "t", "a t")},
		Selection: contract.SelectionSpec{UseWhen: []string{"when"}},
	}}}
	if payload, err := useWhen.ToOverviewPayload(); err != nil || payload == nil {
		t.Fatal(err)
	}

	_ = NormalizeToolSpec(ToolSpec{Identity: contract.ToolIdentitySpec{ProductID: "a", Name: "t", CLIPath: "z", PrimaryCLIPath: "a"}})
	_ = NormalizeParameterSpec(ParameterSpec{Name: " f ", Enum: []string{"b", "a", "b"}})
	if SortedUniqueStrings([]string{"b", "a", "b"})[0] != "a" {
		t.Fatal("sorted unique")
	}
	if CloneOptionalStrings(nil) != nil || len(CloneOptionalStrings([]string{})) != 0 {
		t.Fatal("clone optional")
	}
	if _, err := RawJSONValue(json.RawMessage("1")); err != nil {
		t.Fatal(err)
	}
	if _, err := TypedJSONValue(map[string]int{"n": 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := TypedJSONValue(make(chan int)); err == nil {
		t.Fatal("typed json channel")
	}
	if EqualJSONValues([]byte("1"), []byte("1")) != true || EqualJSONValues([]byte("x"), []byte("y")) {
		t.Fatal("equal json")
	}
	sameCanon := ProductSpec{ID: "a", Tools: []ToolSpec{
		{Identity: contract.ToolIdentitySpec{ProductID: "a", Name: "t", CanonicalPath: "a.t", CLIPath: "b", PrimaryCLIPath: "b"}},
		{Identity: contract.ToolIdentitySpec{ProductID: "a", Name: "t", CanonicalPath: "a.t", CLIPath: "a", PrimaryCLIPath: "a"}},
	}}
	_ = sameCanon.normalized()
	twoPos := valid("a", "t", "a t")
	twoPos.Positionals = []contract.RuntimeSchemaPositional{{Name: "z", Index: 1}, {Name: "a", Index: 0}, {Name: "b", Index: 1}}
	_ = NormalizeToolSpec(twoPos)
	if AliasView(valid("a", "t", "a t"), "not-an-alias").Identity.IsAlias {
		t.Fatal("unmatched alias view")
	}
}
