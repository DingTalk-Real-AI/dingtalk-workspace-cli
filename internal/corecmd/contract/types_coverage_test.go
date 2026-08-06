// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package contract

import (
	"strings"
	"testing"
)

func TestCrossPlatformCoverageDryRunSpecValidate(t *testing.T) {
	for _, kind := range []string{DryRunPreviewInvocation, DryRunPreviewRequest, DryRunPreviewPlan, DryRunPreviewDiff} {
		if err := (DryRunSpec{PreviewKind: kind}).Validate("sample.run"); err != nil {
			t.Fatalf("preview_kind %q: %v", kind, err)
		}
	}
	if err := (DryRunSpec{}).Validate("sample.run"); err == nil || !strings.Contains(err.Error(), "no preview_kind") {
		t.Fatalf("empty preview_kind error = %v", err)
	}
	if err := (DryRunSpec{PreviewKind: "bogus"}).Validate("sample.run"); err == nil || !strings.Contains(err.Error(), "unknown preview_kind") {
		t.Fatalf("unknown preview_kind error = %v", err)
	}
	if err := (DryRunSpec{PreviewKind: DryRunPreviewPlan}).Validate(""); err != nil {
		t.Fatalf("default canonical error = %v", err)
	}
}

func TestCrossPlatformCoverageInterfaceSpecAgentExecutableAndValidate(t *testing.T) {
	ref := &InterfaceRefSpec{ProductID: "im", RPCName: "send"}
	cases := []struct {
		name string
		spec InterfaceSpec
		want bool
	}{
		{"mcp available", InterfaceSpec{Mode: InterfaceModeMCP, Availability: InterfaceAvailable, Ref: ref}, true},
		{"mcp missing ref", InterfaceSpec{Mode: InterfaceModeMCP, Availability: InterfaceAvailable}, false},
		{"local available", InterfaceSpec{Mode: InterfaceModeLocal, Availability: InterfaceAvailable}, true},
		{"local with ref", InterfaceSpec{Mode: InterfaceModeLocal, Availability: InterfaceAvailable, Ref: ref}, false},
		{"composite with reason", InterfaceSpec{Mode: InterfaceModeComposite, Availability: InterfaceAvailable, Reason: "orchestrated"}, true},
		{"composite missing reason", InterfaceSpec{Mode: InterfaceModeComposite, Availability: InterfaceAvailable}, false},
		{"unavailable", InterfaceSpec{Mode: InterfaceModeMCP, Availability: InterfaceUnavailable, Reason: "blocked"}, false},
		{"unknown mode", InterfaceSpec{Mode: "rpc", Availability: InterfaceAvailable}, false},
	}
	for _, tc := range cases {
		if got := tc.spec.AgentExecutable(); got != tc.want {
			t.Fatalf("%s AgentExecutable = %v, want %v", tc.name, got, tc.want)
		}
	}

	validateErr := func(spec InterfaceSpec, want string) {
		t.Helper()
		if err := spec.Validate("sample.run"); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("spec %#v error = %v, want %q", spec, err, want)
		}
	}
	validateErr(InterfaceSpec{Mode: InterfaceUnavailable}, "legacy interface_mode=unavailable")
	validateErr(InterfaceSpec{}, "no interface mode")
	validateErr(InterfaceSpec{Mode: "bogus", Availability: InterfaceAvailable}, "unknown interface mode")
	validateErr(InterfaceSpec{Mode: InterfaceModeMCP, Availability: ""}, "no interface availability")
	validateErr(InterfaceSpec{Mode: InterfaceModeMCP, Availability: "blocked"}, "unknown interface availability")
	validateErr(InterfaceSpec{Mode: InterfaceModeMCP, Availability: InterfaceUnavailable, Ref: ref}, "must not declare interface_ref")
	validateErr(InterfaceSpec{Mode: InterfaceModeMCP, Availability: InterfaceUnavailable}, "must declare interface_reason")
	validateErr(InterfaceSpec{Mode: InterfaceModeMCP, Availability: InterfaceAvailable}, "has no interface_ref")
	validateErr(InterfaceSpec{Mode: InterfaceModeLocal, Availability: InterfaceAvailable, Ref: ref}, "must not declare interface_ref")
	validateErr(InterfaceSpec{Mode: InterfaceModeComposite, Availability: InterfaceAvailable, Ref: ref}, "must not declare a single interface_ref")
	validateErr(InterfaceSpec{Mode: InterfaceModeComposite, Availability: InterfaceAvailable}, "must declare interface_reason")
	if err := (InterfaceSpec{Mode: InterfaceModeMCP, Availability: InterfaceAvailable, Ref: ref}).Validate("sample.run"); err != nil {
		t.Fatalf("valid mcp spec: %v", err)
	}
	if err := (InterfaceSpec{
		Mode: InterfaceModeMCP, Availability: InterfaceUnavailable, Reason: "blocked by policy",
	}).Validate("sample.run"); err != nil {
		t.Fatalf("valid unavailable spec: %v", err)
	}
}

func TestCrossPlatformCoverageSelectionSpecNormalizedAndProvenanceHelpers(t *testing.T) {
	exampleIndex := 0
	normalized := (SelectionSpec{
		UseWhen:   []string{" one ", "one", ""},
		AvoidWhen: []string{"avoid"},
		ExampleDispositions: []ExampleDisposition{{
			Index: &exampleIndex, Mode: ExampleDispositionModeContractOnly,
			ReasonCode: ExampleDispositionReasonLocalState, Reason: "local file", Reviewed: true,
		}},
		SourceRefs: []string{"b", "a", "b"},
	}).Normalized()
	if len(normalized.UseWhen) != 1 || normalized.UseWhen[0] != "one" {
		t.Fatalf("UseWhen = %#v", normalized.UseWhen)
	}
	if normalized.SourceRefs[0] != "a" || normalized.SourceRefs[1] != "b" {
		t.Fatalf("SourceRefs = %#v", normalized.SourceRefs)
	}
	if len(normalized.ExampleDispositions) != 1 || normalized.ExampleDispositions[0].Index == nil || *normalized.ExampleDispositions[0].Index != 0 {
		t.Fatalf("ExampleDispositions = %#v", normalized.ExampleDispositions)
	}
	exampleIndex = 1
	if *normalized.ExampleDispositions[0].Index != 0 {
		t.Fatal("ExampleDispositions index was not cloned")
	}
	if got := cloneExampleDispositions(nil); got != nil {
		t.Fatalf("cloneExampleDispositions(nil) = %#v", got)
	}
	if got := stableUniqueStrings(nil); got != nil {
		t.Fatalf("stableUniqueStrings(nil) = %#v", got)
	}
	if got := sortedUniqueStrings(nil); got != nil {
		t.Fatalf("sortedUniqueStrings(nil) = %#v", got)
	}
	if defaultString("  x ", "fallback") != "x" || defaultString(" ", "fallback") != "fallback" {
		t.Fatal("defaultString changed")
	}
	prov := ResolvedFieldProvenance(func() {}, "src", "ref", "prec", "res", "reason")
	if string(prov.Value) != "null" {
		t.Fatalf("marshal fallback value = %s", prov.Value)
	}
}

func TestCrossPlatformCoverageStoreProductDeclRawForTest(t *testing.T) {
	const id = "coverage-raw-product"
	t.Cleanup(func() { ClearProductDeclForTest(id) })
	StoreProductDeclRawForTest(" ", "ignored")
	StoreProductDeclRawForTest(id, "not-a-decl")
	if _, ok := LookupProductDecl(id); ok {
		t.Fatal("raw non-decl store must not surface via LookupProductDecl")
	}
	ClearProductDeclForTest(" ")
	ClearProductDeclForTest(id)
}
