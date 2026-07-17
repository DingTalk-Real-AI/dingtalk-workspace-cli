// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
package cli

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func contractCoverageTool(product, name, cliPath string) ToolSpec {
	canonical := product + "." + name
	return ToolSpec{Identity: ToolIdentitySpec{
		ProductID: product, Name: name, CLIName: name, CanonicalPath: canonical,
		Path: canonical, CLIPath: cliPath, PrimaryCLIPath: cliPath,
	}}
}

func TestCrossPlatformCoverageContractModelProvenanceValueAndDryRunEdges(t *testing.T) {
	if err := (DryRunSpec{}).Validate(""); err == nil || !strings.Contains(err.Error(), "no preview_kind") {
		t.Fatalf("empty dry-run error = %v", err)
	}
	tool := contractCoverageTool("sample", "run", "sample run")
	tool.Safety.EffectSource = "source"
	if value, ok := tool.provenanceValue("effect_source"); !ok || value != "source" {
		t.Fatalf("effect_source provenance = %#v %v", value, ok)
	}
	if _, ok := tool.provenanceValue("unknown"); ok {
		t.Fatal("unknown tool provenance field accepted")
	}
	parameter := ParameterSpec{Default: json.RawMessage(`1`), InterfaceDefault: json.RawMessage(`2`), InterfaceDescription: "description"}
	for _, field := range []string{"name", "default", "interface_default", "interface_description"} {
		if _, ok := parameter.provenanceValue(field); !ok {
			t.Errorf("parameter provenance field %q missing", field)
		}
	}
	if _, ok := parameter.provenanceValue("unknown"); ok {
		t.Fatal("unknown parameter provenance field accepted")
	}
	if _, ok := (ProductSpec{}).provenanceValue("unknown"); ok {
		t.Fatal("unknown product provenance field accepted")
	}
}

func TestCrossPlatformCoverageSchemaIndexRemainingCollisionEdges(t *testing.T) {
	base := contractCoverageTool("sample", "run", "sample run")
	alias := base
	alias.Identity.IsAlias = true
	earlierPath := contractCoverageTool("a", "one", "a one")
	earlierPath.Identity.Path = "z.two"
	sharedOne := contractCoverageTool("sample", "one", "sample one")
	sharedOne.Identity.Path = "legacy.same"
	sharedTwo := contractCoverageTool("sample", "two", "sample two")
	sharedTwo.Identity.Path = "legacy.same"
	canonicalPathConflict := contractCoverageTool("z", "two", "z two")
	canonicalPathConflict.Identity.Path = "a.one"
	cases := []struct {
		name     string
		registry SchemaRegistry
		want     string
	}{
		{name: "duplicate product", registry: SchemaRegistry{Products: []ProductSpec{{ID: "sample"}, {ID: "sample"}}}, want: "duplicate schema product"},
		{name: "invalid tool", registry: SchemaRegistry{Products: []ProductSpec{{ID: "sample", Tools: []ToolSpec{{}}}}}, want: "tool product_id is empty"},
		{name: "alias identity", registry: SchemaRegistry{Products: []ProductSpec{{ID: "sample", Tools: []ToolSpec{alias}}}}, want: "is_alias=false"},
		{name: "wrong product", registry: SchemaRegistry{Products: []ProductSpec{{ID: "other", Tools: []ToolSpec{base}}}}, want: "not containing product"},
		{name: "duplicate canonical", registry: SchemaRegistry{Products: []ProductSpec{{ID: "sample", Tools: []ToolSpec{base, base}}}}, want: "duplicate schema tool"},
		{name: "canonical conflicts with earlier path", registry: SchemaRegistry{Products: []ProductSpec{
			{ID: "a", Tools: []ToolSpec{earlierPath}},
			{ID: "z", Tools: []ToolSpec{contractCoverageTool("z", "two", "z two")}},
		}}, want: "conflicts with contract path"},
		{name: "shared contract path", registry: SchemaRegistry{Products: []ProductSpec{{ID: "sample", Tools: []ToolSpec{sharedOne, sharedTwo}}}}, want: "resolves to both"},
		{name: "path conflicts with canonical", registry: SchemaRegistry{Products: []ProductSpec{
			{ID: "a", Tools: []ToolSpec{contractCoverageTool("a", "one", "a one")}},
			{ID: "z", Tools: []ToolSpec{canonicalPathConflict}},
		}}, want: "conflicts with a canonical path"},
		{name: "shared cli path", registry: SchemaRegistry{Products: []ProductSpec{{ID: "sample", Tools: []ToolSpec{
			contractCoverageTool("sample", "one", "sample same"), contractCoverageTool("sample", "two", "sample same"),
		}}}}, want: "CLI path"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.registry.Index()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Index() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestCrossPlatformCoverageSchemaSnapshotRendererDependencyEdges(t *testing.T) {
	oldToolSummary := snapshotToolSummary
	oldToolPayload := snapshotToolPayload
	oldProductSummary := snapshotProductSummary
	t.Cleanup(func() {
		snapshotToolSummary = oldToolSummary
		snapshotToolPayload = oldToolPayload
		snapshotProductSummary = oldProductSummary
	})
	registry := SchemaRegistry{Products: []ProductSpec{{ID: "sample", Tools: []ToolSpec{contractCoverageTool("sample", "run", "sample run")}}}}
	snapshotToolSummary = func(ToolSpec) (map[string]any, error) { return nil, errors.New("summary failed") }
	if _, err := registry.ToSnapshotPayload(); err == nil || !strings.Contains(err.Error(), "summary failed") {
		t.Fatalf("summary error = %v", err)
	}
	snapshotToolSummary = oldToolSummary
	snapshotToolPayload = func(ToolSpec) (map[string]any, error) { return nil, errors.New("tool failed") }
	if _, err := registry.ToSnapshotPayload(); err == nil || !strings.Contains(err.Error(), "tool failed") {
		t.Fatalf("tool error = %v", err)
	}
	snapshotToolPayload = oldToolPayload
	snapshotProductSummary = func(ProductSpec) (map[string]any, error) { return nil, errors.New("product failed") }
	if _, err := registry.ToSnapshotPayload(); err == nil || !strings.Contains(err.Error(), "product failed") {
		t.Fatalf("product error = %v", err)
	}
	snapshotProductSummary = oldProductSummary

	for name, candidate := range map[string]SchemaRegistry{
		"extensions":         {Extensions: map[string]json.RawMessage{"bad": json.RawMessage(`{`)}},
		"interface metadata": {InterfaceMetadata: json.RawMessage(`{`)},
		"agent metadata":     {AgentMetadata: json.RawMessage(`{`)},
	} {
		if _, err := candidate.ToSnapshotPayload(); err == nil {
			t.Errorf("%s error was nil", name)
		}
	}
}

func TestCrossPlatformCoverageToolSpecValidationRemainingEdges(t *testing.T) {
	base := contractCoverageTool("sample", "run", "sample run")
	cases := []struct {
		name   string
		mutate func(*ToolSpec)
		want   string
	}{
		{name: "name", mutate: func(v *ToolSpec) { v.Identity.Name = "" }, want: "name is empty"},
		{name: "cli path", mutate: func(v *ToolSpec) { v.Identity.CLIPath = "" }, want: "cli_path is empty"},
		{name: "empty parameter", mutate: func(v *ToolSpec) { v.Parameters = []ParameterSpec{{}} }, want: "empty name"},
		{name: "duplicate parameter", mutate: func(v *ToolSpec) { v.Parameters = []ParameterSpec{{Name: "id"}, {Name: "id"}} }, want: "duplicate parameter"},
		{name: "default json", mutate: func(v *ToolSpec) { v.Parameters = []ParameterSpec{{Name: "id", Default: json.RawMessage(`{`)}} }, want: "invalid JSON default"},
		{name: "interface default json", mutate: func(v *ToolSpec) {
			v.Parameters = []ParameterSpec{{Name: "id", InterfaceDefault: json.RawMessage(`{`)}}
		}, want: "invalid JSON interface_default"},
		{name: "example json", mutate: func(v *ToolSpec) { v.Parameters = []ParameterSpec{{Name: "id", Example: json.RawMessage(`{`)}} }, want: "invalid JSON example"},
		{name: "interface ref", mutate: func(v *ToolSpec) { v.Interface.Ref = &InterfaceRefSpec{} }, want: "incomplete interface_ref"},
		{name: "dry run", mutate: func(v *ToolSpec) { v.DryRun = &DryRunSpec{} }, want: "preview_kind"},
		{name: "interface", mutate: func(v *ToolSpec) { v.Interface.Mode = "unknown" }, want: "unknown interface mode"},
		{name: "tool provenance", mutate: func(v *ToolSpec) {
			v.FieldProvenance = map[string]FieldProvenance{"title": {Value: json.RawMessage(`"wrong"`)}}
		}, want: "winner does not equal"},
		{name: "parameter provenance", mutate: func(v *ToolSpec) {
			v.Parameters = []ParameterSpec{{Name: "id", FieldProvenance: map[string]FieldProvenance{"required": {Value: json.RawMessage(`true`)}}}}
		}, want: "winner does not equal"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			candidate := base
			tc.mutate(&candidate)
			if err := candidate.Validate(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestCrossPlatformCoverageFinalFieldProvenanceRemainingEdges(t *testing.T) {
	if err := validateFinalFieldProvenance("owner", "field", FieldProvenance{}, func() {}); err == nil || !strings.Contains(err.Error(), "cannot encode") {
		t.Fatalf("final encode error = %v", err)
	}
	selected := true
	base := FieldProvenance{
		Value:      json.RawMessage(`"value"`),
		Source:     "source",
		Precedence: "high",
		Resolution: "selected",
		Candidates: []FieldCandidateProvenance{{Value: json.RawMessage(`"value"`), Source: "source", Precedence: "high", Selected: &selected}},
	}
	disagrees := base
	disagrees.Candidates = []FieldCandidateProvenance{{Value: json.RawMessage(`"value"`), Source: "other", Precedence: "high", Selected: &selected}}
	if err := validateFinalFieldProvenance("owner", "field", disagrees, "value"); err == nil || !strings.Contains(err.Error(), "disagrees with winner") {
		t.Fatalf("candidate disagreement = %v", err)
	}
	overridden := base
	overridden.OverriddenCandidates = []FieldCandidateProvenance{{Value: json.RawMessage(`{`)}}
	if err := validateFinalFieldProvenance("owner", "field", overridden, "value"); err == nil || !strings.Contains(err.Error(), "overridden provenance candidate with invalid") {
		t.Fatalf("overridden invalid error = %v", err)
	}
	if equalJSONValues([]byte(`{`), []byte(`}`)) {
		t.Fatal("invalid JSON values compared equal")
	}
}

func TestCrossPlatformCoverageContractModelNormalizationSortEdges(t *testing.T) {
	registry := SchemaRegistry{Products: []ProductSpec{{
		ID: " sample ",
		Tools: []ToolSpec{
			{Identity: ToolIdentitySpec{ProductID: "sample", Name: "same", CLIPath: "sample z"}},
			{Identity: ToolIdentitySpec{ProductID: "sample", Name: "same", CLIPath: "sample a"}},
		},
	}}}
	sorted := registry.Sorted()
	if sorted.Products[0].Tools[0].Identity.CLIPath != "sample a" {
		t.Fatalf("tool sort = %#v", sorted.Products[0].Tools)
	}
	tool := contractCoverageTool("sample", "run", "sample run")
	tool.Positionals = []RuntimeSchemaPositional{{Index: 0, Name: "z"}, {Index: 0, Name: "a"}, {Index: 1, Name: "x"}}
	normalized := tool.normalized()
	if normalized.Positionals[0].Name != "a" || normalized.Positionals[2].Index != 1 {
		t.Fatalf("positionals = %#v", normalized.Positionals)
	}
	if got := sortedUniqueStrings([]string{" ", ""}); got != nil {
		t.Fatalf("empty unique strings = %#v", got)
	}
}

func TestCrossPlatformCoverageContractModelPayloadErrorEdges(t *testing.T) {
	invalidRegistry := SchemaRegistry{Products: []ProductSpec{{ID: " "}}}
	if _, err := invalidRegistry.ToPayload(); err == nil {
		t.Fatal("invalid registry ToPayload succeeded")
	}
	if _, err := invalidRegistry.ToOverviewPayload(); err == nil {
		t.Fatal("invalid registry overview succeeded")
	}
	overview, err := (SchemaRegistry{Products: []ProductSpec{{ID: "sample", Selection: SelectionSpec{UseWhen: []string{"use it"}}}}}).ToOverviewPayload()
	if err != nil || schemaMapSlice(overview["products"])[0]["use_when"] == nil {
		t.Fatalf("use_when overview = %#v, %v", overview, err)
	}
	badRaw := json.RawMessage(`{`)
	for name, registry := range map[string]SchemaRegistry{
		"product":            {Products: []ProductSpec{{ID: "sample", Extensions: map[string]json.RawMessage{"bad": badRaw}}}},
		"registry extension": {Extensions: map[string]json.RawMessage{"bad": badRaw}},
		"interface metadata": {InterfaceMetadata: badRaw},
		"agent metadata":     {AgentMetadata: badRaw},
	} {
		if _, err := registry.ToPayload(); err == nil {
			t.Errorf("registry %s ToPayload error was nil", name)
		}
	}
	for name, registry := range map[string]SchemaRegistry{
		"interface metadata": {InterfaceMetadata: badRaw},
		"agent metadata":     {AgentMetadata: badRaw},
	} {
		if _, err := registry.ToOverviewPayload(); err == nil {
			t.Errorf("overview %s error was nil", name)
		}
	}

	badTool := ToolSpec{}
	if _, err := (ProductSpec{Tools: []ToolSpec{badTool}}).ToPayload(); err == nil {
		t.Fatal("product with invalid tool rendered")
	}
	if _, err := (ProductSpec{Tools: []ToolSpec{badTool}}).ToSummaryPayload(); err == nil {
		t.Fatal("product summary with invalid tool rendered")
	}
	if _, err := (ProductSpec{Extensions: map[string]json.RawMessage{"bad": badRaw}}).ToPayload(); err == nil {
		t.Fatal("product invalid extension rendered")
	}
	if _, err := (ProductSpec{Extensions: map[string]json.RawMessage{"bad": badRaw}}).ToSummaryPayload(); err == nil {
		t.Fatal("product summary invalid extension rendered")
	}
	badProvenance := map[string]FieldProvenance{"agent_summary": {Value: badRaw}}
	if _, err := (ProductSpec{FieldProvenance: badProvenance}).ToPayload(); err == nil {
		t.Fatal("product invalid provenance rendered")
	}
	if _, err := (ProductSpec{FieldProvenance: badProvenance}).ToSummaryPayload(); err == nil {
		t.Fatal("product summary invalid provenance rendered")
	}

	validTool := contractCoverageTool("sample", "run", "sample run")
	if _, err := badTool.ToPayload(); err == nil {
		t.Fatal("invalid tool rendered")
	}
	withExtension := validTool
	withExtension.Extensions = map[string]json.RawMessage{"bad": badRaw}
	if _, err := withExtension.ToPayload(); err == nil {
		t.Fatal("tool invalid extension rendered")
	}
	withParameter := validTool
	withParameter.Parameters = []ParameterSpec{{Name: "id", Extensions: map[string]json.RawMessage{"bad": badRaw}}}
	if _, err := withParameter.ToPayload(); err == nil || !strings.Contains(err.Error(), "render sample.run parameter id") {
		t.Fatalf("tool parameter render error = %v", err)
	}
	withDetails := validTool
	withDetails.Constraints = RuntimeSchemaConstraints{RequireOneOf: [][]string{{"id"}}}
	withDetails.Positionals = []RuntimeSchemaPositional{{Index: 0, Name: "id"}}
	withDetails.DryRun = &DryRunSpec{PreviewKind: DryRunPreviewPlan}
	if _, err := withDetails.ToPayload(); err != nil {
		t.Fatalf("typed details render error = %v", err)
	}
	withProvenance := validTool
	withProvenance.FieldProvenance = map[string]FieldProvenance{"unknown": {Value: badRaw}}
	if _, err := withProvenance.ToPayload(); err == nil {
		t.Fatal("tool invalid provenance rendered")
	}
	if _, err := badTool.ToSummaryPayload(); err == nil {
		t.Fatal("invalid tool summary rendered")
	}

	for name, parameter := range map[string]ParameterSpec{
		"extension":         {Extensions: map[string]json.RawMessage{"bad": badRaw}},
		"default":           {Default: badRaw},
		"interface default": {InterfaceDefault: badRaw},
		"example":           {Example: badRaw},
		"provenance":        {FieldProvenance: map[string]FieldProvenance{"required": {Value: badRaw}}},
	} {
		if _, err := parameter.ToPayload(); err == nil {
			t.Errorf("parameter %s error was nil", name)
		}
	}
	if _, err := typedJSONValue(func() {}); err == nil {
		t.Fatal("typedJSONValue accepted function")
	}
	if _, err := rawJSONValue(badRaw); err == nil {
		t.Fatal("rawJSONValue accepted invalid JSON")
	}
}
