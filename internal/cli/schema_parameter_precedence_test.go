// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package cli

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestRuntimeSchemaScalarResolverUsesSourceRankNotInputOrder(t *testing.T) {
	tests := []struct {
		name string
		high runtimeSchemaFieldCandidate
		low  runtimeSchemaFieldCandidate
		want any
	}{
		{
			name: "higher source can lower non-cobra boolean",
			high: runtimeSchemaCandidate(false, true, "tool_schema_hint"),
			low:  runtimeSchemaCandidate(true, true, "mcp_metadata"),
			want: false,
		},
		{
			name: "higher source can raise boolean",
			high: runtimeSchemaCandidate(true, true, "native_annotation"),
			low:  runtimeSchemaCandidate(false, true, "tool_schema_hint"),
			want: true,
		},
		{
			name: "typed string beats hint string",
			high: runtimeSchemaCandidate("native", true, "native_annotation"),
			low:  runtimeSchemaCandidate("hint", true, "tool_schema_hint"),
			want: "native",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, candidates := range [][]runtimeSchemaFieldCandidate{{test.low, test.high}, {test.high, test.low}} {
				winner, err := resolveRuntimeSchemaCandidate("field", candidates...)
				if err != nil {
					t.Fatalf("resolveRuntimeSchemaCandidate() error = %v", err)
				}
				if !reflect.DeepEqual(winner.Value, test.want) {
					t.Fatalf("winner = %#v, want %#v", winner.Value, test.want)
				}
				if winner.Source != test.high.Source {
					t.Fatalf("winner source = %q, want %q", winner.Source, test.high.Source)
				}
			}
		})
	}
}

func TestResolveRequiredProjectionCannotLowerCobraHardRequired(t *testing.T) {
	winner, err := resolveRequiredProjection(true,
		runtimeSchemaManualCandidate(false, true, "reviewed lowering"),
		runtimeSchemaCandidate(true, true, "cobra_hard_required"),
	)
	if err != nil {
		t.Fatalf("resolveRequiredProjection() error = %v", err)
	}
	if winner.Value != true {
		t.Fatalf("winner value = %#v, want true", winner.Value)
	}
	if winner.Source != "cobra_hard_required" {
		t.Fatalf("winner source = %q, want cobra_hard_required", winner.Source)
	}
	if winner.Resolution != "cobra_hard_required_floor" {
		t.Fatalf("winner resolution = %q, want cobra_hard_required_floor", winner.Resolution)
	}
	foundManual := false
	for _, candidate := range winner.Compared {
		if candidate.Source == "reviewed_manual_hint" && candidate.Value == false {
			foundManual = true
		}
	}
	if !foundManual {
		t.Fatalf("compared candidates omitted suppressed manual false: %#v", winner.Compared)
	}
}

func TestResolveRequiredProjectionAllowsRaiseWithoutHardRequired(t *testing.T) {
	winner, err := resolveRequiredProjection(false,
		runtimeSchemaManualCandidate(true, true, "reviewed raise"),
		runtimeSchemaCandidate(false, true, "default"),
	)
	if err != nil {
		t.Fatalf("resolveRequiredProjection() error = %v", err)
	}
	if winner.Value != true || winner.Source != "reviewed_manual_hint" {
		t.Fatalf("winner = %#v source=%q", winner.Value, winner.Source)
	}
}

func TestRuntimeSchemaParameterSourcePrecedenceMatrix(t *testing.T) {
	tests := []struct {
		source     string
		rank       int
		precedence string
	}{
		{"reviewed_manual_hint", runtimeSchemaRankReviewedManual, runtimeSchemaPrecedenceReviewedManual},
		{"versioned_parameter_binding", runtimeSchemaRankVersionedBinding, runtimeSchemaPrecedenceVersionedBinding},
		{"require_one_of_constraint", runtimeSchemaRankConstraint, runtimeSchemaPrecedenceConstraint},
		{"typed_parameter_metadata", runtimeSchemaRankTypedMetadata, runtimeSchemaPrecedenceTypedMetadata},
		{"native_annotation", runtimeSchemaRankNativeAnnotation, runtimeSchemaPrecedenceNativeAnnotation},
		{"cobra_hard_required", runtimeSchemaRankCobraContract, runtimeSchemaPrecedenceCobra},
		{"tool_schema_hint", runtimeSchemaRankToolHint, runtimeSchemaPrecedenceToolHint},
		{"cobra_help", runtimeSchemaRankCobraHelp, runtimeSchemaPrecedenceCobraHelp},
		{"mcp_metadata", runtimeSchemaRankMCP, runtimeSchemaPrecedenceMCP},
		{"flag_name_inference", runtimeSchemaRankInference, runtimeSchemaPrecedenceInference},
		{"default", runtimeSchemaRankDefault, runtimeSchemaPrecedenceDefault},
	}
	for index, test := range tests {
		rank, precedence := runtimeSchemaSourcePriority(test.source)
		if rank != test.rank || precedence != test.precedence {
			t.Fatalf("source %q = rank %d/%q, want %d/%q", test.source, rank, precedence, test.rank, test.precedence)
		}
		if index > 0 && tests[index-1].rank <= test.rank {
			t.Fatalf("precedence matrix is not strictly descending at %q > %q", tests[index-1].source, test.source)
		}
	}
}

func TestRuntimeSchemaScalarResolverFailsEqualRankConflict(t *testing.T) {
	winner, err := resolveRuntimeSchemaCandidate("required",
		runtimeSchemaCandidate(false, true, "typed_parameter_metadata"),
		runtimeSchemaCandidate(false, true, "typed_parameter_metadata"),
	)
	if err != nil {
		t.Fatalf("equal values should coalesce: %v", err)
	}
	provenance := runtimeSchemaFieldProvenance(winner)
	selected := 0
	for _, candidate := range provenance.Candidates {
		if candidate.Selected != nil && *candidate.Selected {
			selected++
		}
	}
	if selected != 1 || len(provenance.Candidates) != 2 {
		t.Fatalf("coalesced provenance = %#v", provenance)
	}

	_, err = resolveRuntimeSchemaCandidate("required",
		runtimeSchemaCandidate(false, true, "typed_parameter_metadata"),
		runtimeSchemaCandidate(true, true, "typed_parameter_metadata"),
	)
	if err == nil || !strings.Contains(err.Error(), "conflicting equal-precedence") {
		t.Fatalf("equal-rank conflict error = %v", err)
	}
}

func TestRuntimeSchemaScalarResolverRejectsLowerRankConflictBehindWinner(t *testing.T) {
	high := runtimeSchemaManualCandidate(false, true, "reviewed winner")
	lowerA := runtimeSchemaStringCandidateAtRank(true, "lower-a", runtimeSchemaRankInference, runtimeSchemaPrecedenceInference)
	lowerB := runtimeSchemaStringCandidateAtRank(false, "lower-b", runtimeSchemaRankInference, runtimeSchemaPrecedenceInference)
	permutations := [][]runtimeSchemaFieldCandidate{
		{high, lowerA, lowerB},
		{high, lowerB, lowerA},
		{lowerA, high, lowerB},
		{lowerA, lowerB, high},
		{lowerB, high, lowerA},
		{lowerB, lowerA, high},
	}

	var stableError string
	for index, candidates := range permutations {
		_, err := resolveRuntimeSchemaCandidate("required", candidates...)
		if err == nil || !strings.Contains(err.Error(), "conflicting equal-precedence") ||
			!strings.Contains(err.Error(), "lower-a") || !strings.Contains(err.Error(), "lower-b") {
			t.Fatalf("permutation %d error = %v, want lower-rank conflict", index, err)
		}
		if stableError == "" {
			stableError = err.Error()
		} else if err.Error() != stableError {
			t.Fatalf("permutation %d error = %q, want stable %q", index, err, stableError)
		}
	}
}

func TestRuntimeCommandParameterSpecsBuildTypedContractDirectly(t *testing.T) {
	root, leaf := manualSchemaHintTestTree()
	leaf.Flags().Int("limit", 20, "Optional page size")
	if err := leaf.MarkFlagRequired("query"); err != nil {
		t.Fatalf("MarkFlagRequired() error = %v", err)
	}
	setFlagAnnotation(leaf.Flags().Lookup("query"), runtimeSchemaFlagExampleAnnotation, "hello")
	required := false
	_, err := applyManualSchemaHints(root, ManualSchemaHintSnapshot{
		Schema:  manualSchemaHintSchemaRef,
		Version: manualSchemaHintVersion,
		Commands: []ManualSchemaCommandHint{{
			CLIPath:       "sample item search",
			CanonicalPath: "sample.search_items",
			Reason:        "Review optional Agent projection",
			Reviewed:      true,
			Parameters: map[string]ManualSchemaParameterHint{
				"query": {Required: &required},
			},
		}},
	})
	if err != nil {
		t.Fatalf("applyManualSchemaHints() error = %v", err)
	}

	specs, err := runtimeCommandParameterSpecs(leaf, "sample.search_items", nil, map[string]embeddedMCPParamMeta{
		"limit": {Default: "50"},
	}, RuntimeSchemaConstraints{})
	if err != nil {
		t.Fatalf("runtimeCommandParameterSpecs() error = %v", err)
	}
	if got := []string{specs[0].Name, specs[1].Name}; !reflect.DeepEqual(got, []string{"limit", "query"}) {
		t.Fatalf("typed parameter order = %#v", got)
	}
	limit, query := specs[0], specs[1]
	if got := rawSchemaString(t, limit.Default); got != "20" {
		t.Fatalf("typed CLI default = %q", got)
	}
	if got := rawSchemaString(t, limit.InterfaceDefault); got != "50" {
		t.Fatalf("typed interface default = %q", got)
	}
	if !query.Required || !query.CLIRequired {
		t.Fatalf("typed required projection = %v, CLIRequired = %v", query.Required, query.CLIRequired)
	}
	if got := rawSchemaString(t, query.Example); got != "hello" {
		t.Fatalf("typed example = %q", got)
	}
	provenance := query.FieldProvenance["required"]
	if provenance.Source != "cobra_hard_required" || provenance.Resolution != "cobra_hard_required_floor" {
		t.Fatalf("typed required provenance = %#v", provenance)
	}
	if !typedProvenanceHasCandidate(t, provenance, "reviewed_manual_hint", false) {
		t.Fatalf("typed provenance omitted suppressed manual false: %#v", provenance)
	}
	if !typedProvenanceHasCandidate(t, provenance, "cobra_hard_required", true) {
		t.Fatalf("typed provenance omitted Cobra observation: %#v", provenance)
	}
	for _, parameter := range []ParameterSpec{limit, query} {
		emptyRequiredWhen, ok := parameter.FieldProvenance["required_when"]
		if !ok || string(emptyRequiredWhen.Value) != `""` || emptyRequiredWhen.Source != "default" {
			t.Fatalf("%s empty required_when provenance = %#v", parameter.Name, emptyRequiredWhen)
		}
	}

	// The compatibility wrapper serializes the typed values and preserves the
	// historical JSON string representation of defaults/examples.
	payload, err := runtimeCommandParameters(leaf, "sample.search_items", nil, map[string]embeddedMCPParamMeta{
		"limit": {Default: "50"},
	}, RuntimeSchemaConstraints{})
	if err != nil {
		t.Fatalf("runtimeCommandParameters() error = %v", err)
	}
	limitPayload := schemaParameterEntry(t, payload, "limit")
	queryPayload := schemaParameterEntry(t, payload, "query")
	if limitPayload["default"] != "20" || limitPayload["interface_default"] != "50" {
		t.Fatalf("wire defaults = %#v", limitPayload)
	}
	if queryPayload["example"] != "hello" || queryPayload["required"] != true || queryPayload["cli_required"] != true {
		t.Fatalf("wire query = %#v", queryPayload)
	}
}

func rawSchemaString(t *testing.T, value json.RawMessage) string {
	t.Helper()
	var decoded string
	if err := json.Unmarshal(value, &decoded); err != nil {
		t.Fatalf("decode raw Schema string %q: %v", value, err)
	}
	return decoded
}

func typedProvenanceHasCandidate(t *testing.T, provenance FieldProvenance, source string, value bool) bool {
	t.Helper()
	for _, candidate := range provenance.Candidates {
		if candidate.Source != source {
			continue
		}
		var decoded bool
		if err := json.Unmarshal(candidate.Value, &decoded); err != nil {
			t.Fatalf("decode provenance candidate %q: %v", candidate.Value, err)
		}
		if decoded == value {
			return true
		}
	}
	return false
}

func TestRuntimeSchemaParameterPrecedenceManualWinsAllProjectedSources(t *testing.T) {
	root, leaf := manualSchemaHintTestTree()
	flag := leaf.Flags().Lookup("query")
	setFlagAnnotation(flag, runtimeSchemaFlagBindingPropertyAnnotation, "boundQuery")
	setFlagAnnotation(flag, runtimeSchemaFlagTypeAnnotation, "boolean")
	setFlagAnnotation(flag, runtimeSchemaFlagDescriptionAnnotation, "Native description")
	setFlagAnnotation(flag, runtimeSchemaFlagRequiredAnnotation, "false")
	setFlagAnnotation(flag, runtimeSchemaFlagRequiredWhenAnnotation, "native condition")
	setFlagAnnotation(flag, runtimeSchemaFlagMetadataRequiredAnnotation, "false")
	setFlagAnnotation(flag, runtimeSchemaFlagMetadataRequiredWhenAnnotation, "typed condition")

	description := "Reviewed description"
	property := "manualQuery"
	interfaceType := "object"
	required := true
	requiredWhen := "manual condition"
	_, err := applyManualSchemaHints(root, ManualSchemaHintSnapshot{
		Schema:  manualSchemaHintSchemaRef,
		Version: manualSchemaHintVersion,
		Commands: []ManualSchemaCommandHint{{
			CLIPath:       "sample item search",
			CanonicalPath: "sample.search_items",
			Reason:        "Reviewed conflict resolution",
			Reviewed:      true,
			Parameters: map[string]ManualSchemaParameterHint{
				"query": {
					Description:   &description,
					Property:      &property,
					InterfaceType: &interfaceType,
					Required:      &required,
					RequiredWhen:  &requiredWhen,
				},
			},
		}},
	})
	if err != nil {
		t.Fatalf("applyManualSchemaHints() error = %v", err)
	}

	parameters, err := runtimeCommandParameters(leaf, "sample.search_items", map[string]ParameterSchemaHint{
		"manualQuery": {
			Type:         "string",
			Description:  "Tool hint description",
			Required:     boolPointer(false),
			RequiredWhen: "tool hint condition",
		},
	}, map[string]embeddedMCPParamMeta{
		"manualQuery": {
			Type:         "array",
			Description:  "MCP description",
			Required:     boolPointer(false),
			RequiredWhen: "MCP condition",
		},
	}, RuntimeSchemaConstraints{})
	if err != nil {
		t.Fatalf("runtimeCommandParameters() error = %v", err)
	}
	query := schemaParameterEntry(t, parameters, "query")
	if query["description"] != description || query["property"] != property || query["interface_type"] != interfaceType || query["required"] != true || query["required_when"] != requiredWhen {
		t.Fatalf("resolved query = %#v", query)
	}
	for _, field := range []string{"description", "property", "interface_type", "required", "required_when"} {
		if source := schemaParameterFieldSource(t, query, field); source != "reviewed_manual_hint" {
			t.Fatalf("%s source = %q, want reviewed_manual_hint", field, source)
		}
	}
	if reason := schemaParameterFieldReviewReason(t, query, "property"); reason != "Reviewed conflict resolution" {
		t.Fatalf("property review_reason = %q", reason)
	}

	// Manual review is a separate resolver input; it must not erase the
	// independently owned annotations that it outranks.
	if got := firstFlagAnnotation(flag, runtimeSchemaFlagTypeAnnotation); got != "boolean" {
		t.Fatalf("runtime type annotation = %q", got)
	}
	if got := firstFlagAnnotation(flag, runtimeSchemaFlagRequiredWhenAnnotation); got != "native condition" {
		t.Fatalf("runtime required_when annotation = %q", got)
	}
}

func TestRuntimeSchemaParameterPrecedenceManualCannotLowerCobraHardRequired(t *testing.T) {
	root, leaf := manualSchemaHintTestTree()
	if err := leaf.MarkFlagRequired("query"); err != nil {
		t.Fatalf("MarkFlagRequired() error = %v", err)
	}
	required := false
	_, err := applyManualSchemaHints(root, ManualSchemaHintSnapshot{
		Schema:  manualSchemaHintSchemaRef,
		Version: manualSchemaHintVersion,
		Commands: []ManualSchemaCommandHint{{
			CLIPath:       "sample item search",
			CanonicalPath: "sample.search_items",
			Reason:        "Review optional projection",
			Reviewed:      true,
			Parameters: map[string]ManualSchemaParameterHint{
				"query": {Required: &required},
			},
		}},
	})
	if err != nil {
		t.Fatalf("applyManualSchemaHints() error = %v", err)
	}
	parameters, err := runtimeCommandParameters(leaf, "sample.search_items", nil, nil, RuntimeSchemaConstraints{})
	if err != nil {
		t.Fatalf("runtimeCommandParameters() error = %v", err)
	}
	query := schemaParameterEntry(t, parameters, "query")
	if query["required"] != true {
		t.Fatalf("required = %#v, want true", query["required"])
	}
	if source := schemaParameterFieldSource(t, query, "required"); source != "cobra_hard_required" {
		t.Fatalf("required source = %q", source)
	}
	if resolution := schemaParameterFieldResolution(t, query, "required"); resolution != "cobra_hard_required_floor" {
		t.Fatalf("required resolution = %q", resolution)
	}
	if query["cli_required"] != true {
		t.Fatalf("cli_required = %#v, want true", query["cli_required"])
	}
	if !schemaParameterFieldHasCandidate(t, query, "required", "reviewed_manual_hint", false) {
		t.Fatalf("required provenance does not retain suppressed manual false: %#v", schemaParameterFieldProvenance(t, query, "required"))
	}
	if !schemaParameterFieldHasCandidate(t, query, "required", "cobra_hard_required", true) {
		t.Fatalf("required provenance does not retain Cobra observation: %#v", schemaParameterFieldProvenance(t, query, "required"))
	}
	if required, annotated := runtimeFlagRequiredState(leaf.Flags().Lookup("query")); !required || !annotated {
		t.Fatalf("runtimeFlagRequiredState() = %v, %v", required, annotated)
	}
}

func TestRuntimeSchemaHardRequiredFloorReachesFinalParameterPayload(t *testing.T) {
	root, leaf := manualSchemaHintTestTree()
	if err := leaf.MarkFlagRequired("query"); err != nil {
		t.Fatalf("MarkFlagRequired() error = %v", err)
	}
	required := false
	_, err := applyManualSchemaHints(root, ManualSchemaHintSnapshot{
		Schema:  manualSchemaHintSchemaRef,
		Version: manualSchemaHintVersion,
		Commands: []ManualSchemaCommandHint{{
			CLIPath:       "sample item search",
			CanonicalPath: "sample.search_items",
			Reason:        "Attempted optional projection must not weaken Cobra hard-required",
			Reviewed:      true,
			Parameters: map[string]ManualSchemaParameterHint{
				"query": {Required: &required},
			},
		}},
	})
	if err != nil {
		t.Fatalf("applyManualSchemaHints() error = %v", err)
	}

	specs, err := runtimeCommandParameterSpecs(leaf, "sample.search_items", nil, nil, RuntimeSchemaConstraints{})
	if err != nil {
		t.Fatalf("runtimeCommandParameterSpecs() error = %v", err)
	}
	var query ParameterSpec
	for _, spec := range specs {
		if spec.Name == "query" {
			query = spec
			break
		}
	}
	if query.Name == "" {
		t.Fatal("missing query parameter spec")
	}
	payload, err := query.ToPayload()
	if err != nil {
		t.Fatalf("ToPayload() error = %v", err)
	}
	if payload["required"] != true {
		t.Fatalf("final payload required = %#v, want true", payload["required"])
	}
	if payload["cli_required"] != true {
		t.Fatalf("final payload cli_required = %#v, want true", payload["cli_required"])
	}
	provenance, _ := payload["field_provenance"].(map[string]any)
	requiredProvenance, _ := provenance["required"].(map[string]any)
	if schemaString(requiredProvenance["source"]) != "cobra_hard_required" {
		t.Fatalf("final required provenance source = %#v", requiredProvenance["source"])
	}
	if schemaString(requiredProvenance["resolution"]) != "cobra_hard_required_floor" {
		t.Fatalf("final required provenance resolution = %#v", requiredProvenance["resolution"])
	}
	if requiredProvenance["value"] != true {
		t.Fatalf("final required provenance value = %#v, want true", requiredProvenance["value"])
	}
}

func TestRuntimeSchemaParameterPrecedenceConstraintCannotLowerCobraHardRequired(t *testing.T) {
	_, leaf := manualSchemaHintTestTree()
	leaf.Flags().String("scope", "", "Optional scope")
	if err := leaf.MarkFlagRequired("query"); err != nil {
		t.Fatalf("MarkFlagRequired() error = %v", err)
	}
	parameters, err := runtimeCommandParameters(leaf, "sample.search_items", nil, nil, RuntimeSchemaConstraints{
		RequireOneOf: [][]string{{"query", "scope"}},
	})
	if err != nil {
		t.Fatalf("runtimeCommandParameters() error = %v", err)
	}
	query := schemaParameterEntry(t, parameters, "query")
	if query["required"] != true || query["cli_required"] != true {
		t.Fatalf("resolved query = %#v", query)
	}
	if source := schemaParameterFieldSource(t, query, "required"); source != "cobra_hard_required" {
		t.Fatalf("required source = %q", source)
	}
	if !schemaParameterFieldHasCandidate(t, query, "required", "require_one_of_constraint", false) {
		t.Fatalf("required provenance omitted constraint candidate: %#v", schemaParameterFieldProvenance(t, query, "required"))
	}
}

func TestRuntimeSchemaParameterPrecedenceBindingSelectsMCPMetadata(t *testing.T) {
	_, leaf := manualSchemaHintTestTree()
	canonical := "sample.search_items"
	applyRuntimeSchemaParameterBindingsFrom(leaf, canonical, map[string]map[string]string{
		canonical: {"query": "queryText"},
	})

	parameters, err := runtimeCommandParameters(leaf, canonical, nil, map[string]embeddedMCPParamMeta{
		"queryText": {
			Type:         "object",
			Description:  "MCP query object",
			Required:     boolPointer(true),
			RequiredWhen: "MCP condition",
		},
	}, RuntimeSchemaConstraints{})
	if err != nil {
		t.Fatalf("runtimeCommandParameters() error = %v", err)
	}
	query := schemaParameterEntry(t, parameters, "query")
	if query["property"] != "queryText" || query["interface_type"] != "object" || query["required"] != true || query["required_when"] != "MCP condition" {
		t.Fatalf("resolved query = %#v", query)
	}
	if source := schemaParameterFieldSource(t, query, "property"); source != "versioned_parameter_binding" {
		t.Fatalf("property source = %q", source)
	}
	if source := schemaParameterFieldSource(t, query, "interface_type"); source != "mcp_metadata" {
		t.Fatalf("interface_type source = %q", source)
	}
	if source := schemaParameterFieldSource(t, query, "required"); source != "mcp_metadata" {
		t.Fatalf("required source = %q", source)
	}
	if source := schemaParameterFieldSource(t, query, "required_when"); source != "mcp_metadata" {
		t.Fatalf("required_when source = %q", source)
	}
}

func schemaParameterFieldHasCandidate(t *testing.T, parameter map[string]any, field, source string, value any) bool {
	t.Helper()
	items, ok := schemaParameterFieldProvenance(t, parameter, field)["candidates"].([]map[string]any)
	if !ok {
		if raw, rawOK := schemaParameterFieldProvenance(t, parameter, field)["candidates"].([]any); rawOK {
			for _, candidate := range raw {
				item, _ := candidate.(map[string]any)
				if schemaString(item["source"]) == source && schemaParameterJSONEqual(item["value"], value) {
					return true
				}
			}
		}
		return false
	}
	for _, item := range items {
		if schemaString(item["source"]) == source && schemaParameterJSONEqual(item["value"], value) {
			return true
		}
	}
	return false
}

func schemaParameterJSONEqual(left, right any) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && string(leftJSON) == string(rightJSON)
}

func TestRuntimeSchemaParameterPrecedenceTypedMetadataBeatsToolHint(t *testing.T) {
	_, leaf := manualSchemaHintTestTree()
	canonical := "sample.search_items"
	flag := leaf.Flags().Lookup("query")
	setFlagAnnotation(flag, "x-cli-format", "native-format")
	setFlagAnnotation(flag, runtimeSchemaFlagExampleAnnotation, "native-example")
	setFlagAnnotationValues(flag, "x-cli-enum", "native-a", "native-b")
	previous, existed := runtimeSchemaParameterMetadataByCanonical[canonical]
	runtimeSchemaParameterMetadataByCanonical[canonical] = RuntimeSchemaParameterMetadata{
		Required:     []string{"query"},
		RequiredWhen: map[string]string{"query": "typed condition"},
		Formats:      map[string]string{"query": "typed-format"},
		Examples:     map[string]string{"query": "typed-example"},
		Enums:        map[string][]string{"query": {"typed-a", "typed-b"}},
	}
	t.Cleanup(func() {
		if existed {
			runtimeSchemaParameterMetadataByCanonical[canonical] = previous
		} else {
			delete(runtimeSchemaParameterMetadataByCanonical, canonical)
		}
	})
	applyRuntimeSchemaParameterMetadata(leaf, canonical)

	parameters, err := runtimeCommandParameters(leaf, canonical, map[string]ParameterSchemaHint{
		"query": {Required: boolPointer(false), RequiredWhen: "tool hint condition"},
	}, map[string]embeddedMCPParamMeta{
		"query": {Format: "mcp-format", Enum: []string{"mcp-a", "mcp-b"}},
	}, RuntimeSchemaConstraints{})
	if err != nil {
		t.Fatalf("runtimeCommandParameters() error = %v", err)
	}
	query := schemaParameterEntry(t, parameters, "query")
	if query["required"] != true || query["required_when"] != "typed condition" ||
		query["format"] != "typed-format" || query["example"] != "typed-example" ||
		!schemaParameterJSONEqual(query["enum"], []string{"typed-a", "typed-b"}) {
		t.Fatalf("resolved query = %#v", query)
	}
	for _, test := range []struct {
		field string
		value any
	}{
		{field: "required", value: true},
		{field: "required_when", value: "typed condition"},
		{field: "format", value: "typed-format"},
		{field: "example", value: "typed-example"},
		{field: "enum", value: []string{"typed-a", "typed-b"}},
	} {
		t.Run(test.field, func(t *testing.T) {
			if source := schemaParameterFieldSource(t, query, test.field); source != "typed_parameter_metadata" {
				t.Fatalf("%s source = %q", test.field, source)
			}
			if !schemaParameterFieldHasCandidate(t, query, test.field, "typed_parameter_metadata", test.value) {
				t.Fatalf("%s provenance omitted typed candidate %#v: %#v", test.field, test.value, schemaParameterFieldProvenance(t, query, test.field))
			}
		})
	}
	for _, candidate := range []struct {
		field  string
		source string
		value  any
	}{
		{field: "format", source: "native_annotation", value: "native-format"},
		{field: "format", source: "mcp_metadata", value: "mcp-format"},
		{field: "example", source: "native_annotation", value: "native-example"},
		{field: "enum", source: "native_annotation", value: []string{"native-a", "native-b"}},
		{field: "enum", source: "mcp_metadata", value: []string{"mcp-a", "mcp-b"}},
	} {
		if !schemaParameterFieldHasCandidate(t, query, candidate.field, candidate.source, candidate.value) {
			t.Errorf("%s provenance omitted %s=%#v: %#v", candidate.field, candidate.source, candidate.value, schemaParameterFieldProvenance(t, query, candidate.field))
		}
	}
}

func TestRuntimeSchemaFormatPrecedenceMatrix(t *testing.T) {
	tests := []struct {
		name        string
		native      string
		mcp         string
		usage       string
		want        string
		wantSource  string
		wantSources []string
	}{
		{
			name:        "native beats mcp and inference",
			native:      "native-format",
			mcp:         "mcp-format",
			usage:       "RFC3339 timestamp",
			want:        "native-format",
			wantSource:  "native_annotation",
			wantSources: []string{"native_annotation", "mcp_metadata", "usage_format_inference", "default"},
		},
		{
			name:        "mcp beats inference",
			mcp:         "mcp-format",
			usage:       "RFC3339 timestamp",
			want:        "mcp-format",
			wantSource:  "mcp_metadata",
			wantSources: []string{"mcp_metadata", "usage_format_inference", "default"},
		},
		{
			name:        "inference beats default",
			usage:       "RFC3339 timestamp",
			want:        "date-time",
			wantSource:  "usage_format_inference",
			wantSources: []string{"usage_format_inference", "default"},
		},
		{name: "empty default stays omitted"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, leaf := manualSchemaHintTestTree()
			flag := leaf.Flags().Lookup("query")
			if test.native != "" {
				setFlagAnnotation(flag, "x-cli-format", test.native)
			}
			if test.usage != "" {
				flag.Usage = test.usage
			}
			parameters, err := runtimeCommandParameters(leaf, "sample.search_items", nil, map[string]embeddedMCPParamMeta{
				"query": {Format: test.mcp},
			}, RuntimeSchemaConstraints{})
			if err != nil {
				t.Fatalf("runtimeCommandParameters() error = %v", err)
			}
			query := schemaParameterEntry(t, parameters, "query")
			if test.want == "" {
				if _, ok := query["format"]; ok {
					t.Fatalf("empty format must stay omitted: %#v", query)
				}
				provenance, _ := query["field_provenance"].(map[string]any)
				if _, ok := provenance["format"]; ok {
					t.Fatalf("default-only omitted format must not publish provenance: %#v", provenance["format"])
				}
				return
			}
			if query["format"] != test.want {
				t.Fatalf("format = %#v, want %q", query["format"], test.want)
			}
			if source := schemaParameterFieldSource(t, query, "format"); source != test.wantSource {
				t.Fatalf("format source = %q, want %q", source, test.wantSource)
			}
			for _, source := range test.wantSources {
				value := any("")
				switch source {
				case "native_annotation":
					value = test.native
				case "mcp_metadata":
					value = test.mcp
				case "usage_format_inference":
					value = "date-time"
				}
				if !schemaParameterFieldHasCandidate(t, query, "format", source, value) {
					t.Errorf("format provenance omitted %s=%#v: %#v", source, value, schemaParameterFieldProvenance(t, query, "format"))
				}
			}
		})
	}
}

func TestRuntimeSchemaEnumCandidatesUseUnifiedConflictGate(t *testing.T) {
	winner, err := resolveRuntimeSchemaCandidate("enum",
		runtimeSchemaEnumCandidate([]string{" a ", "a", "b", "b"}, "typed_parameter_metadata"),
	)
	if err != nil {
		t.Fatalf("duplicate enum normalization error = %v", err)
	}
	if !schemaParameterJSONEqual(winner.Value, []string{"a", "b"}) {
		t.Fatalf("normalized enum winner = %#v, want [a b]", winner.Value)
	}

	if _, err := resolveRuntimeSchemaCandidate("enum",
		runtimeSchemaEnumCandidate([]string{"a", "b"}, "typed_parameter_metadata"),
		runtimeSchemaEnumCandidate([]string{"a", "b"}, "typed_parameter_metadata"),
	); err != nil {
		t.Fatalf("equal enum candidates must coalesce: %v", err)
	}
	if _, err := resolveRuntimeSchemaCandidate("enum",
		runtimeSchemaEnumCandidate([]string{"a", "b"}, "typed_parameter_metadata"),
		runtimeSchemaEnumCandidate([]string{"a", "c"}, "typed_parameter_metadata"),
	); err == nil || !strings.Contains(err.Error(), "conflicting equal-precedence") {
		t.Fatalf("conflicting enum candidates error = %v", err)
	}
}

func TestRuntimeSchemaParameterPrecedenceNativeAndCobraBeatHints(t *testing.T) {
	_, leaf := manualSchemaHintTestTree()
	flag := leaf.Flags().Lookup("query")
	setFlagAnnotation(flag, runtimeSchemaFlagTypeAnnotation, "boolean")
	setFlagAnnotation(flag, runtimeSchemaFlagDescriptionAnnotation, "Native description")
	setFlagAnnotation(flag, runtimeSchemaFlagRequiredAnnotation, "false")

	parameters, err := runtimeCommandParameters(leaf, "sample.search_items", map[string]ParameterSchemaHint{
		"query": {
			Type:        "object",
			Description: "Tool hint description",
			Required:    boolPointer(true),
		},
	}, map[string]embeddedMCPParamMeta{
		"query": {
			Type:        "array",
			Description: "MCP description",
			Required:    boolPointer(true),
		},
	}, RuntimeSchemaConstraints{})
	if err != nil {
		t.Fatalf("runtimeCommandParameters() error = %v", err)
	}
	query := schemaParameterEntry(t, parameters, "query")
	if query["interface_type"] != "boolean" || query["description"] != "Native description" || query["required"] != false {
		t.Fatalf("resolved query = %#v", query)
	}
	for _, field := range []string{"interface_type", "description", "required"} {
		if source := schemaParameterFieldSource(t, query, field); source != "native_annotation" {
			t.Fatalf("%s source = %q, want native_annotation", field, source)
		}
	}

	// Without a native required override, the executable Cobra contract wins
	// over descriptive Tool/MCP hints, while CLIRequired remains independently
	// observable in the final projection.
	flag.Annotations[runtimeSchemaFlagRequiredAnnotation] = nil
	if err := leaf.MarkFlagRequired("query"); err != nil {
		t.Fatalf("MarkFlagRequired() error = %v", err)
	}
	parameters, err = runtimeCommandParameters(leaf, "sample.search_items", map[string]ParameterSchemaHint{
		"query": {Required: boolPointer(false)},
	}, map[string]embeddedMCPParamMeta{
		"query": {Required: boolPointer(false)},
	}, RuntimeSchemaConstraints{})
	if err != nil {
		t.Fatalf("runtimeCommandParameters() error = %v", err)
	}
	query = schemaParameterEntry(t, parameters, "query")
	if query["required"] != true || query["cli_required"] != true {
		t.Fatalf("Cobra-required query = %#v", query)
	}
	if source := schemaParameterFieldSource(t, query, "required"); source != "cobra_hard_required" {
		t.Fatalf("required source = %q, want cobra_hard_required", source)
	}
}

func TestRuntimeSchemaParameterPrecedenceToolHintBeatsMCPWithoutChangingFlagIdentity(t *testing.T) {
	_, leaf := manualSchemaHintTestTree()
	parameters, err := runtimeCommandParameters(leaf, "sample.search_items", map[string]ParameterSchemaHint{
		"query": {
			FlagName:     "query",
			Type:         "object",
			Description:  "Tool hint description",
			Required:     boolPointer(true),
			RequiredWhen: "tool condition",
		},
	}, map[string]embeddedMCPParamMeta{
		"query": {
			Type:         "array",
			Description:  "MCP description",
			Required:     boolPointer(false),
			RequiredWhen: "MCP condition",
		},
	}, RuntimeSchemaConstraints{})
	if err != nil {
		t.Fatalf("runtimeCommandParameters() error = %v", err)
	}
	query := schemaParameterEntry(t, parameters, "query")
	if query["interface_type"] != "object" || query["required"] != true || query["required_when"] != "tool condition" {
		t.Fatalf("resolved query = %#v", query)
	}
	// Cobra Help owns the CLI description and outranks descriptive hints.
	if query["description"] != "Original query text" {
		t.Fatalf("description = %#v, want Cobra usage", query["description"])
	}
	for _, field := range []string{"interface_type", "required", "required_when"} {
		if source := schemaParameterFieldSource(t, query, field); source != "tool_schema_hint" {
			t.Fatalf("%s source = %q, want tool_schema_hint", field, source)
		}
	}

	_, err = runtimeCommandParameters(leaf, "sample.search_items", map[string]ParameterSchemaHint{
		"query": {FlagName: "renamed-query"},
	}, nil, RuntimeSchemaConstraints{})
	if err == nil || !strings.Contains(err.Error(), "does not identify the existing Cobra flag") {
		t.Fatalf("phantom flag rename error = %v", err)
	}
}

func schemaParameterEntry(t *testing.T, parameters map[string]any, name string) map[string]any {
	t.Helper()
	entry, ok := parameters[name].(map[string]any)
	if !ok {
		t.Fatalf("parameter %q = %#v", name, parameters[name])
	}
	return entry
}

func schemaParameterFieldSource(t *testing.T, parameter map[string]any, field string) string {
	t.Helper()
	return schemaString(schemaParameterFieldProvenance(t, parameter, field)["source"])
}

func schemaParameterFieldResolution(t *testing.T, parameter map[string]any, field string) string {
	t.Helper()
	return schemaString(schemaParameterFieldProvenance(t, parameter, field)["resolution"])
}

func schemaParameterFieldReviewReason(t *testing.T, parameter map[string]any, field string) string {
	t.Helper()
	return schemaString(schemaParameterFieldProvenance(t, parameter, field)["review_reason"])
}

func schemaParameterFieldProvenance(t *testing.T, parameter map[string]any, field string) map[string]any {
	t.Helper()
	provenance, ok := parameter["field_provenance"].(map[string]any)
	if !ok {
		t.Fatalf("field_provenance = %#v", parameter["field_provenance"])
	}
	fieldProvenance, ok := provenance[field].(map[string]any)
	if !ok {
		t.Fatalf("field provenance for %q = %#v", field, provenance[field])
	}
	return fieldProvenance
}
