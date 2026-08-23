// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/runtimeannotate"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestCrossPlatformCoverageRuntimeSchemaLoaderAndAnnotationEdges(t *testing.T) {
	if got := emptyPinnedMCPMetadata(); got.Tools == nil || len(got.Tools) != 0 {
		t.Fatalf("empty pinned metadata = %#v", got)
	}

	cmd := &cobra.Command{Use: "run", Short: "short", Long: "long"}
	AnnotateRuntimePositionals(cmd, contract.RuntimeSchemaPositional{Name: " ", Index: -1})
	if _, ok := cmd.Annotations[runtimeSchemaArgsAnnotation]; ok {
		t.Fatal("invalid positional should not be annotated")
	}
}

func TestCrossPlatformCoverageCollectRuntimeSchemaEntriesErrorsAndOrdering(t *testing.T) {
	testseam.Swap(t, &bindValidateParameterBindings, func() error { return errors.New("bindings failed") })
	if _, err := collectRuntimeSchemaEntries(&cobra.Command{Use: "dws"}); err == nil || !strings.Contains(err.Error(), "bindings failed") {
		t.Fatalf("validation error = %v", err)
	}

	testseam.Swap(t, &bindValidateParameterBindings, func() error { return nil })
	prevGroups := reviewedRuntimeSchemaExclusionGroups
	t.Cleanup(func() { reviewedRuntimeSchemaExclusionGroups = prevGroups })
	reviewedRuntimeSchemaExclusionGroups = []runtimeSchemaExclusionGroup{{
		ID: "", Reason: "x", Reviewed: true, Commands: []string{"x"},
	}}
	if _, err := collectRuntimeSchemaEntries(&cobra.Command{Use: "dws"}); err == nil {
		t.Fatal("invalid reviewed exclusions should fail identity collection")
	}
	reviewedRuntimeSchemaExclusionGroups = prevGroups

	if _, err := collectRuntimeSchemaEntriesFromBound(BoundCommandRegistry{Commands: []BoundCommandSpec{{
		CommandSpec: CommandSpec{CanonicalPath: "invalid", Visibility: SchemaVisibilityPublic},
	}}}); err == nil {
		t.Fatal("invalid canonical path should fail")
	}
	leaf := &cobra.Command{Use: "run", Run: func(*cobra.Command, []string) {}}
	entries, err := collectRuntimeSchemaEntriesFromBound(BoundCommandRegistry{Commands: []BoundCommandSpec{
		{CommandSpec: CommandSpec{CanonicalPath: "sample.run", PrimaryCLIPath: "z run", Visibility: SchemaVisibilityPublic}, PrimaryCommand: leaf},
		{CommandSpec: CommandSpec{CanonicalPath: "sample.run", PrimaryCLIPath: "a run", Visibility: SchemaVisibilityPublic}, PrimaryCommand: leaf},
	}})
	if err != nil || len(entries) != 2 || entries[0].CLIPath != "a run" {
		t.Fatalf("ordered entries = %#v, err = %v", entries, err)
	}
}

func TestCrossPlatformCoverageRuntimeSchemaMetadataLookupEdges(t *testing.T) {
	for _, test := range []struct {
		value any
		want  int
	}{
		{value: map[string]any{"tool_count": 1}, want: 1},
		{value: map[string]any{"tool_count": int64(2)}, want: 2},
		{value: map[string]any{"tool_count": float64(3)}, want: 3},
		{value: map[string]any{"tools": []any{1, 2, 3, 4}}, want: 4},
		{value: map[string]any{}, want: 0},
	} {
		if got := schemaProductToolCount(test.value.(map[string]any)); got != test.want {
			t.Fatalf("tool count = %d, want %d", got, test.want)
		}
	}
}

func TestCrossPlatformCoverageRuntimeSchemaCandidateAndProvenanceEdges(t *testing.T) {
	left := runtimeSchemaStringCandidateAtPriority("same", true, "same", 1, "z")
	right := runtimeSchemaStringCandidateAtPriority("same", true, "same", 1, "a")
	winner, err := resolveRuntimeSchemaCandidate("ordering", left, right)
	if err != nil || winner.Precedence != "a" {
		t.Fatalf("precedence tie ordering winner = %#v, err = %v", winner, err)
	}
	if got := runtimeSchemaFieldProvenance(runtimeSchemaFieldCandidate{}); !reflect.DeepEqual(got, contract.FieldProvenance{}) {
		t.Fatalf("absent provenance = %#v", got)
	}
	bad := runtimeSchemaCandidate(func() {}, true, "custom")
	bad.Compared = []runtimeSchemaFieldCandidate{bad}
	provenance := runtimeSchemaFieldProvenance(bad)
	if string(provenance.Value) != "null" || string(provenance.Candidates[0].Value) != "null" {
		t.Fatalf("invalid value provenance = %#v", provenance)
	}
	if rank, precedence := runtimeSchemaSourcePriority("custom"); rank != runtimeSchemaRankDerived || precedence != "source_order" {
		t.Fatalf("custom source = %d/%q", rank, precedence)
	}

	flags := pflag.NewFlagSet("annotations", pflag.ContinueOnError)
	flags.Bool("enabled", false, "")
	flag := flags.Lookup("enabled")
	setFlagAnnotation(flag, runtimeSchemaFlagRequiredAnnotation, "not-bool")
	if candidate := runtimeSchemaAnnotatedBoolCandidate(flag, runtimeSchemaFlagRequiredAnnotation, "native_annotation"); candidate.Present {
		t.Fatalf("invalid bool candidate = %#v", candidate)
	}

	originalResolver := resolveRuntimeSchemaField
	t.Cleanup(func() { resolveRuntimeSchemaField = originalResolver })
	resolveRuntimeSchemaField = func(string, ...runtimeSchemaFieldCandidate) (runtimeSchemaFieldCandidate, error) {
		return runtimeSchemaFieldCandidate{}, errors.New("required failed")
	}
	if _, err := resolveRequiredProjection(false); err == nil {
		t.Fatal("required resolver error was not returned")
	}
}

func TestCrossPlatformCoverageRuntimeCommandParameterErrorEdges(t *testing.T) {
	cmd := &cobra.Command{Use: "run"}
	cmd.Flags().String("value", "", "value")
	flag := cmd.Flags().Lookup("value")

	if specs, err := runtimeCommandParameterSpecs(nil, "sample.run", RuntimeSchemaConstraints{}); err != nil || specs != nil {
		t.Fatalf("nil command specs = %#v, err = %v", specs, err)
	}
	testseam.Swap(t, &schemaParameterBindingData, func() (schemaParameterBindingSnapshot, error) {
		return schemaParameterBindingSnapshot{}, errors.New("load failed")
	})
	if _, err := runtimeCommandParameterSpecs(cmd, "sample.run", RuntimeSchemaConstraints{}); err == nil || !strings.Contains(err.Error(), "load failed") {
		t.Fatalf("binding load error = %v", err)
	}
	testseam.Swap(t, &schemaParameterBindingData, func() (schemaParameterBindingSnapshot, error) {
		return schemaParameterBindingSnapshot{Bindings: map[string]map[string]string{}, MappingExclusions: map[string]string{}}, nil
	})
	if _, _, err := runtimeSchemaParameterMappingCandidates(schemaParameterBindingSnapshot{
		Bindings:          map[string]map[string]string{},
		MappingExclusions: map[string]string{"sample.run --value": " "},
	}, "sample.run", "value"); err == nil {
		t.Fatal("empty mapping exclusion reason should fail")
	}
	testseam.Swap(t, &schemaParameterBindingData, func() (schemaParameterBindingSnapshot, error) {
		return schemaParameterBindingSnapshot{
			Bindings:          map[string]map[string]string{},
			MappingExclusions: map[string]string{"sample.run --value": " "},
		}, nil
	})
	if _, err := runtimeCommandParameterSpecs(cmd, "sample.run", RuntimeSchemaConstraints{}); err == nil || !strings.Contains(err.Error(), "mapping exclusion") {
		t.Fatalf("mapping exclusion error = %v", err)
	}
	testseam.Swap(t, &schemaParameterBindingData, func() (schemaParameterBindingSnapshot, error) {
		return schemaParameterBindingSnapshot{Bindings: map[string]map[string]string{}, MappingExclusions: map[string]string{}}, nil
	})

	realResolver := resolveRuntimeSchemaField
	for _, target := range []string{"property", "interface_type", "description", "required", "required_when", "format", "enum", "example"} {
		testseam.Swap(t, &resolveRuntimeSchemaField, func(field string, candidates ...runtimeSchemaFieldCandidate) (runtimeSchemaFieldCandidate, error) {
			if field == target {
				return runtimeSchemaFieldCandidate{}, errors.New("forced " + target)
			}
			return resolveRuntimeSchemaCandidate(field, candidates...)
		})
		if _, err := runtimeCommandParameterSpecs(cmd, "sample.run", RuntimeSchemaConstraints{}); err == nil || !strings.Contains(err.Error(), target) {
			t.Fatalf("%s resolution error = %v", target, err)
		}
	}
	resolveRuntimeSchemaField = realResolver

	if specs, err := runtimeCommandParameterSpecs(&cobra.Command{Use: "empty"}, "sample.empty", RuntimeSchemaConstraints{}); err != nil || specs != nil {
		t.Fatalf("empty specs = %#v, err = %v", specs, err)
	}
	if payload, err := runtimeCommandParameters(nil, "", RuntimeSchemaConstraints{}); err != nil || payload != nil {
		t.Fatalf("empty payload = %#v, err = %v", payload, err)
	}
	testseam.Swap(t, &runtimeCommandParameterSpecsForPayload, func(*cobra.Command, string, RuntimeSchemaConstraints) ([]ParameterSpec, error) {
		return []ParameterSpec{{Name: "bad", Example: json.RawMessage("{")}}, nil
	})
	if _, err := runtimeCommandParameters(cmd, "sample.run", RuntimeSchemaConstraints{}); err == nil || !strings.Contains(err.Error(), "serialize Schema parameter") {
		t.Fatalf("payload serialization error = %v", err)
	}

	setFlagAnnotation(flag, runtimeSchemaFlagRequiredAnnotation, "true")
	if required, present := runtimeFlagRequiredState(flag); !required || !present {
		t.Fatalf("required annotation = %v/%v", required, present)
	}

	// Binding snapshot still supplies reviewed property mappings without MCP pin.
	testseam.Swap(t, &schemaParameterBindingData, func() (schemaParameterBindingSnapshot, error) {
		return schemaParameterBindingSnapshot{
			Bindings: map[string]map[string]string{"sample.run": {"value": "clawType"}},
		}, nil
	})
	specs, err := runtimeCommandParameterSpecs(cmd, "sample.run", RuntimeSchemaConstraints{})
	if err != nil {
		t.Fatalf("parameter specs error = %v", err)
	}
	if len(specs) != 1 || specs[0].Property != "clawType" {
		t.Fatalf("parameter specs = %#v", specs)
	}
	if prov := specs[0].FieldProvenance["property"]; prov.Source == "" {
		t.Fatalf("property provenance missing: %#v", specs[0].FieldProvenance)
	}
}

func TestCrossPlatformCoverageRuntimeSchemaPureHelperEdges(t *testing.T) {
	if runtimeCommandFlag(nil, "x") != nil || runtimeCommandFlag(&cobra.Command{Use: "run"}, " ") != nil {
		t.Fatal("invalid command flag lookup should be nil")
	}
	visitRuntimeCommandFlags(nil, nil, nil)

	cmd := &cobra.Command{Use: "run", Annotations: map[string]string{
		runtimeSchemaRulesAnnotation: "{",
		runtimeSchemaArgsAnnotation:  "{",
	}}
	if got := runtimeCommandConstraints(cmd); !runtimeSchemaConstraintsEmpty(got) {
		t.Fatalf("invalid constraints = %#v", got)
	}
	if got := runtimeCommandPositionals(cmd); got != nil {
		t.Fatalf("invalid positionals = %#v", got)
	}
	cmd.Annotations[runtimeSchemaArgsAnnotation] = `[{"name":"second","index":2},{"name":"first","index":1}]`
	if got := runtimeCommandPositionals(cmd); len(got) != 2 || got[0].Name != "first" {
		t.Fatalf("sorted positionals = %#v", got)
	}
	groups := runtimeannotate.NormalizeConstraints(runtimeannotate.RuntimeSchemaConstraints{
		RequireOneOf: [][]string{{" "}, {" ", "one", "one"}, {"one"}, {"one"}},
	}).RequireOneOf
	if !reflect.DeepEqual(groups, [][]string{{"one"}}) {
		t.Fatalf("normalized groups = %#v", groups)
	}
	if isGenericPayloadFlag(nil) {
		t.Fatal("nil flag cannot be a generic payload")
	}
	flags := pflag.NewFlagSet("payload", pflag.ContinueOnError)
	flags.String("params", "", "Additional JSON object payload merged after --json")
	if !isGenericPayloadFlag(flags.Lookup("params")) {
		t.Fatal("params payload flag was not recognized")
	}

	invalidRequired := &pflag.Flag{Name: "x", Usage: "required", Annotations: map[string][]string{runtimeSchemaFlagRequiredAnnotation: {"bad"}}}
	if required, present := runtimeFlagRequiredState(invalidRequired); !required || !present {
		t.Fatalf("usage required fallback = %v/%v", required, present)
	}
	if usageImpliesRequired("") || usageImpliesRequired("required when enabled") {
		t.Fatal("empty or conditional usage should not be unconditionally required")
	}
	if required, present := runtimeFlagRequiredState(&pflag.Flag{Name: "optional", Usage: "optional"}); required || present {
		t.Fatalf("optional required state = %v/%v", required, present)
	}
	if lowerCamelFlagName("---") != "---" || lowerCamelFlagName("one") != "one" || lowerCamelFlagName("one-two") != "oneTwo" {
		t.Fatal("lower camel conversion failed")
	}
	if inferredRuntimeFlagFormat(nil) != "" {
		t.Fatal("nil flag format should be empty")
	}
}

func TestCrossPlatformCoverageSchemaCompactProjectionEdges(t *testing.T) {
	if stripSchemaPayloadCompact(nil) != nil {
		t.Fatal("nil compact payload should stay nil")
	}
	if got := stripSchemaParametersCompact("raw"); got != "raw" {
		t.Fatalf("raw parameters = %#v", got)
	}
	parameters := map[string]any{
		"raw":   "value",
		"typed": map[string]any{"type": "string", "property": "remote"},
	}
	got := stripSchemaParametersCompact(parameters).(map[string]any)
	if got["raw"] != "value" || got["typed"].(map[string]any)["property"] != nil {
		t.Fatalf("compact parameters = %#v", got)
	}
	value := stripSchemaValueCompact(map[string]any{"required": false, "property": "remote"}).(map[string]any)
	if _, exists := value["property"]; exists {
		t.Fatalf("compact parameter value = %#v", value)
	}
	// Non-parameter nested maps fall through to payload compacting.
	nested := stripSchemaValueCompact(map[string]any{"description": "keep", "provenance": "drop"}).(map[string]any)
	if nested["description"] != "keep" {
		t.Fatalf("nested non-param map = %#v", nested)
	}
	if _, exists := nested["provenance"]; exists {
		t.Fatalf("nested non-param provenance should drop: %#v", nested)
	}
	// Type-only maps still count as parameter objects.
	typedOnly := stripSchemaValueCompact(map[string]any{"type": "string", "property": "remote"}).(map[string]any)
	if _, exists := typedOnly["property"]; exists {
		t.Fatalf("type-only param value = %#v", typedOnly)
	}
	mapSlice := stripSchemaValueCompact([]map[string]any{{"description": "leaf", "provenance": "drop"}}).([]map[string]any)
	if len(mapSlice) != 1 || mapSlice[0]["description"] != "leaf" {
		t.Fatalf("value compact []map = %#v", mapSlice)
	}
	if _, exists := mapSlice[0]["provenance"]; exists {
		t.Fatalf("value compact []map provenance should drop: %#v", mapSlice)
	}
	anySlice := stripSchemaValueCompact([]any{map[string]any{"description": "leaf", "provenance": "drop"}, "raw"}).([]any)
	if len(anySlice) != 2 || anySlice[1] != "raw" {
		t.Fatalf("value compact []any = %#v", anySlice)
	}

	payload := map[string]any{
		"description": "calendar",
		"provenance":  map[string]any{"source": "drop"},
		"parameters":  parameters,
		"product":     map[string]any{"description": "calendar", "provenance": "drop"},
		"products": []map[string]any{
			{"description": "calendar", "provenance": "drop"},
		},
		"tools": []any{
			map[string]any{"description": "leaf", "provenance": "drop"},
			"skip-me",
		},
		"constraints": map[string]any{"require_one_of": []any{}},
	}
	stripped := stripSchemaPayloadCompact(payload)
	if stripped["description"] != "calendar" {
		t.Fatalf("compact description = %#v", stripped["description"])
	}
	if _, exists := stripped["provenance"]; exists {
		t.Fatalf("compact should drop provenance: %#v", stripped)
	}
	if product, ok := stripped["product"].(map[string]any); !ok || product["description"] != "calendar" {
		t.Fatalf("compact product = %#v", stripped["product"])
	}
	if _, exists := stripped["product"].(map[string]any)["provenance"]; exists {
		t.Fatalf("nested product provenance should drop: %#v", stripped["product"])
	}
	if products, ok := stripped["products"].([]map[string]any); !ok || len(products) != 1 || products[0]["description"] != "calendar" {
		t.Fatalf("compact products = %#v", stripped["products"])
	}
	if tools, ok := stripped["tools"].([]any); !ok || len(tools) != 2 {
		t.Fatalf("compact tools = %#v", stripped["tools"])
	}
	if tool, ok := stripped["tools"].([]any)[0].(map[string]any); !ok || tool["description"] != "leaf" {
		t.Fatalf("compact tools[0] = %#v", stripped["tools"].([]any)[0])
	}
	if stripped["tools"].([]any)[1] != "skip-me" {
		t.Fatalf("compact tools[1] = %#v", stripped["tools"].([]any)[1])
	}
	// Non-map product values are retained verbatim.
	if got := stripSchemaPayloadCompact(map[string]any{"product": "raw"}); got["product"] != "raw" {
		t.Fatalf("non-map product = %#v", got["product"])
	}
	if got := stripSchemaPayloadCollectionCompact("raw"); got != "raw" {
		t.Fatalf("non-collection compact = %#v", got)
	}
}
