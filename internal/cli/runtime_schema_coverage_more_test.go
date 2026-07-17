// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestCrossPlatformCoverageRuntimeSchemaLoaderAndAnnotationEdges(t *testing.T) {
	originalJSON := embeddedMCPMetadataJSON
	t.Cleanup(func() { embeddedMCPMetadataJSON = originalJSON })
	embeddedMCPMetadataJSON = []byte("{")
	if got := loadEmbeddedMCPMetadata(); got.Tools == nil || len(got.Tools) != 0 {
		t.Fatalf("invalid embedded metadata = %#v", got)
	}
	embeddedMCPMetadataJSON = []byte(`{"version":1}`)
	if got := loadEmbeddedMCPMetadata(); got.Tools == nil {
		t.Fatal("nil tools map was not normalized")
	}

	cmd := &cobra.Command{Use: "run", Short: "short", Long: "long"}
	AnnotateRuntimeToolMetadata(cmd, " title ", " description ", " source ")
	if cmd.Annotations[runtimeSchemaTitleAnnotation] != "title" {
		t.Fatalf("annotations = %#v", cmd.Annotations)
	}
	AnnotateRuntimePositionals(cmd, RuntimeSchemaPositional{Name: " ", Index: -1})
	if _, ok := cmd.Annotations[runtimeSchemaArgsAnnotation]; ok {
		t.Fatal("invalid positional should not be annotated")
	}
	if runtimeCommandTitle(nil) != "" || runtimeCommandDescription(nil) != "" {
		t.Fatal("nil command text should be empty")
	}
	cmd.Annotations[runtimeSchemaTitleAnnotation] = " annotated title "
	cmd.Annotations[runtimeSchemaDescAnnotation] = " annotated description "
	if runtimeCommandTitle(cmd) != "annotated title" || runtimeCommandDescription(cmd) != "annotated description" {
		t.Fatalf("annotated text = %q / %q", runtimeCommandTitle(cmd), runtimeCommandDescription(cmd))
	}
}

func TestCrossPlatformCoverageCollectRuntimeSchemaEntriesErrorsAndOrdering(t *testing.T) {
	originalValidate := validateReviewedParameterBindings
	originalRegistry := loadReviewedCommandRegistry
	originalManual := loadReviewedManualSchemaHints
	t.Cleanup(func() {
		validateReviewedParameterBindings = originalValidate
		loadReviewedCommandRegistry = originalRegistry
		loadReviewedManualSchemaHints = originalManual
	})

	validateReviewedParameterBindings = func() error { return errors.New("bindings failed") }
	if _, err := collectRuntimeSchemaEntries(&cobra.Command{Use: "dws"}); err == nil || !strings.Contains(err.Error(), "bindings failed") {
		t.Fatalf("validation error = %v", err)
	}

	validateReviewedParameterBindings = func() error { return nil }
	loadReviewedCommandRegistry = func() (CommandRegistry, error) {
		return CommandRegistry{Commands: []CommandSpec{{
			CanonicalPath: "sample.run", PrimaryCLIPath: "sample run",
			Visibility: SchemaVisibilityPublic, Source: "reviewed_registry", ReviewReason: "test binding failure",
		}}}, nil
	}
	loadReviewedManualSchemaHints = func() (ManualSchemaHintSnapshot, error) {
		return ManualSchemaHintSnapshot{Schema: manualSchemaHintSchemaRef, Version: manualSchemaHintVersion}, nil
	}
	if _, err := collectRuntimeSchemaEntries(&cobra.Command{Use: "dws"}); err == nil {
		t.Fatal("missing Cobra path should fail binding")
	}

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
	originalHints := defaultSchemaHintRegistry
	t.Cleanup(func() { defaultSchemaHintRegistry = originalHints })
	defaultSchemaHintRegistry = newSchemaHintRegistry()
	defaultSchemaHintRegistry.RegisterProduct("source", map[string]ToolSchemaHint{
		"run": {Title: "source title"},
	})
	hint := runtimeSchemaHintForEntry(runtimeSchemaEntry{ProductID: "target", SourceProductID: "source", ToolName: "run"})
	if hint.Title != "source title" {
		t.Fatalf("source product hint = %#v", hint)
	}
	if _, ok := embeddedMCPMetadataForEntryFrom(runtimeSchemaEntry{}, embeddedAgentMetadata{}, embeddedMCPMetadata{Tools: map[string]embeddedMCPToolMetadata{}}); ok {
		t.Fatal("empty lookup unexpectedly matched")
	}

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

	originalResolver := resolveRuntimeSchemaField
	t.Cleanup(func() { resolveRuntimeSchemaField = originalResolver })
	for _, field := range []string{"title", "description"} {
		resolveRuntimeSchemaField = func(got string, candidates ...runtimeSchemaFieldCandidate) (runtimeSchemaFieldCandidate, error) {
			if got == field {
				return runtimeSchemaFieldCandidate{}, errors.New("forced " + field)
			}
			return resolveRuntimeSchemaCandidate(got, candidates...)
		}
		_, _, _, _, err := runtimeToolTextMetadataFromMetadata(runtimeSchemaEntry{Title: "title", Description: "description", MetadataSource: "native"}, runtimeSchemaMetadataSources{})
		if err == nil || !strings.Contains(err.Error(), field) {
			t.Fatalf("%s error = %v", field, err)
		}
	}
	resolveRuntimeSchemaField = func(field string, candidates ...runtimeSchemaFieldCandidate) (runtimeSchemaFieldCandidate, error) {
		return runtimeSchemaStringCandidate("selected", "mcp_metadata"), nil
	}
	_, _, source, _, err := runtimeToolTextMetadataFromMetadata(runtimeSchemaEntry{}, runtimeSchemaMetadataSources{})
	if err != nil || source != "embedded-mcp-metadata" {
		t.Fatalf("MCP metadata source = %q, err = %v", source, err)
	}
}

func TestCrossPlatformCoverageRuntimeSchemaCandidateAndProvenanceEdges(t *testing.T) {
	left := runtimeSchemaStringCandidateAtPriority("same", true, "same", 1, "z")
	right := runtimeSchemaStringCandidateAtPriority("same", true, "same", 1, "a")
	winner, err := resolveRuntimeSchemaCandidate("ordering", left, right)
	if err != nil || winner.Precedence != "a" {
		t.Fatalf("precedence tie ordering winner = %#v, err = %v", winner, err)
	}
	if got := runtimeSchemaFieldProvenance(runtimeSchemaFieldCandidate{}); !reflect.DeepEqual(got, FieldProvenance{}) {
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

	originalBindingData := schemaParameterBindingData
	originalResolver := resolveRuntimeSchemaField
	originalPayloadSpecs := runtimeCommandParameterSpecsForPayload
	t.Cleanup(func() {
		schemaParameterBindingData = originalBindingData
		resolveRuntimeSchemaField = originalResolver
		runtimeCommandParameterSpecsForPayload = originalPayloadSpecs
	})

	if specs, err := runtimeCommandParameterSpecs(nil, "sample.run", nil, nil, RuntimeSchemaConstraints{}); err != nil || specs != nil {
		t.Fatalf("nil command specs = %#v, err = %v", specs, err)
	}
	schemaParameterBindingData = func() (schemaParameterBindingSnapshot, error) {
		return schemaParameterBindingSnapshot{}, errors.New("load failed")
	}
	if _, err := runtimeCommandParameterSpecs(cmd, "sample.run", nil, nil, RuntimeSchemaConstraints{}); err == nil || !strings.Contains(err.Error(), "load failed") {
		t.Fatalf("binding load error = %v", err)
	}
	schemaParameterBindingData = func() (schemaParameterBindingSnapshot, error) {
		return schemaParameterBindingSnapshot{Bindings: map[string]map[string]string{}, MappingExclusions: map[string]string{}}, nil
	}
	if _, _, err := runtimeSchemaParameterMappingCandidates(schemaParameterBindingSnapshot{
		Bindings:          map[string]map[string]string{},
		MappingExclusions: map[string]string{"sample.run --value": " "},
	}, "sample.run", "value"); err == nil {
		t.Fatal("empty mapping exclusion reason should fail")
	}
	schemaParameterBindingData = func() (schemaParameterBindingSnapshot, error) {
		return schemaParameterBindingSnapshot{
			Bindings:          map[string]map[string]string{},
			MappingExclusions: map[string]string{"sample.run --value": " "},
		}, nil
	}
	if _, err := runtimeCommandParameterSpecs(cmd, "sample.run", nil, nil, RuntimeSchemaConstraints{}); err == nil || !strings.Contains(err.Error(), "mapping exclusion") {
		t.Fatalf("mapping exclusion error = %v", err)
	}
	schemaParameterBindingData = func() (schemaParameterBindingSnapshot, error) {
		return schemaParameterBindingSnapshot{Bindings: map[string]map[string]string{}, MappingExclusions: map[string]string{}}, nil
	}

	cmd.Annotations = map[string]string{
		runtimeManualSchemaParameterKey(runtimeSchemaManualParameterAnnotation, "value"): "{",
	}
	if _, err := runtimeCommandParameterSpecs(cmd, "sample.run", nil, nil, RuntimeSchemaConstraints{}); err == nil || !strings.Contains(err.Error(), "reviewed manual") {
		t.Fatalf("manual hint error = %v", err)
	}
	cmd.Annotations = nil

	for _, target := range []string{"property", "interface_type", "description", "required", "required_when", "format", "enum", "example"} {
		resolveRuntimeSchemaField = func(field string, candidates ...runtimeSchemaFieldCandidate) (runtimeSchemaFieldCandidate, error) {
			if field == target {
				return runtimeSchemaFieldCandidate{}, errors.New("forced " + target)
			}
			return resolveRuntimeSchemaCandidate(field, candidates...)
		}
		if _, err := runtimeCommandParameterSpecs(cmd, "sample.run", nil, nil, RuntimeSchemaConstraints{}); err == nil || !strings.Contains(err.Error(), target) {
			t.Fatalf("%s resolution error = %v", target, err)
		}
	}
	resolveRuntimeSchemaField = originalResolver

	badHint := map[string]ParameterSchemaHint{"value": {FlagName: "other"}}
	if _, err := runtimeCommandParameterSpecs(cmd, "sample.run", badHint, nil, RuntimeSchemaConstraints{}); err == nil || !strings.Contains(err.Error(), "does not identify") {
		t.Fatalf("hint flag mismatch error = %v", err)
	}
	if specs, err := runtimeCommandParameterSpecs(&cobra.Command{Use: "empty"}, "sample.empty", nil, nil, RuntimeSchemaConstraints{}); err != nil || specs != nil {
		t.Fatalf("empty specs = %#v, err = %v", specs, err)
	}
	if payload, err := runtimeCommandParameters(nil, "", nil, nil, RuntimeSchemaConstraints{}); err != nil || payload != nil {
		t.Fatalf("empty payload = %#v, err = %v", payload, err)
	}
	runtimeCommandParameterSpecsForPayload = func(*cobra.Command, string, map[string]ParameterSchemaHint, map[string]embeddedMCPParamMeta, RuntimeSchemaConstraints) ([]ParameterSpec, error) {
		return []ParameterSpec{{Name: "bad", Example: json.RawMessage("{")}}, nil
	}
	if _, err := runtimeCommandParameters(cmd, "sample.run", nil, nil, RuntimeSchemaConstraints{}); err == nil || !strings.Contains(err.Error(), "serialize Schema parameter") {
		t.Fatalf("payload serialization error = %v", err)
	}

	setFlagAnnotation(flag, runtimeSchemaFlagRequiredAnnotation, "true")
	if required, present := runtimeFlagRequiredState(flag); !required || !present {
		t.Fatalf("required annotation = %v/%v", required, present)
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
	groups := normalizeRuntimeSchemaGroups([][]string{{" "}, {" ", "one", "one"}, {"one"}, {"one"}}, 1)
	if !reflect.DeepEqual(groups, [][]string{{"one"}}) {
		t.Fatalf("normalized groups = %#v", groups)
	}
	if meta, ok := lookupEmbeddedMCPParam(map[string]embeddedMCPParamMeta{"flag": {Type: "string"}}, "property", "flag"); !ok || meta.Type != "string" {
		t.Fatalf("flag fallback metadata = %#v/%v", meta, ok)
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
}
