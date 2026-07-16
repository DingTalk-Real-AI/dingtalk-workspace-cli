// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

type schemaParameterMappingFlagAudit struct {
	canonical      string
	flagName       string
	mode           string
	availability   string
	property       string
	metadataKey    string
	metadataParams map[string]embeddedMCPParamMeta
	parameter      map[string]any
}

func TestEmbeddedCatalogMCPParameterMappingsAreComplete(t *testing.T) {
	loaded := embeddedSchemaCatalog()
	if !embeddedSchemaCatalogAvailable() {
		t.Fatalf("embedded schema Catalog is unavailable: %v", embeddedSchemaCatalogError())
	}
	bindings, err := runtimeSchemaParameterBindingData()
	if err != nil {
		t.Fatalf("runtimeSchemaParameterBindingData() error = %v", err)
	}
	problems := auditSchemaParameterMappings(
		loaded.Snapshot.Tools,
		runtimeMCPMetadata(),
		bindings,
	)
	if len(problems) > 0 {
		t.Fatalf("final Catalog parameter mapping audit found %d problem(s):\n%s", len(problems), limitedSchemaMappingProblems(problems, 80))
	}
}

func TestEmbeddedCatalogDoesNotProjectHardRequiredFlagsAsOptional(t *testing.T) {
	loaded := embeddedSchemaCatalog()
	if !embeddedSchemaCatalogAvailable() {
		t.Fatalf("embedded schema Catalog is unavailable: %v", embeddedSchemaCatalogError())
	}
	var problems []string
	for canonical, tool := range loaded.Snapshot.Tools {
		parameters, _ := tool["parameters"].(map[string]any)
		for name, raw := range parameters {
			parameter, _ := raw.(map[string]any)
			if parameter["cli_required"] == true && parameter["required"] == false {
				problems = append(problems, canonical+" --"+name)
			}
		}
	}
	sort.Strings(problems)
	if len(problems) > 0 {
		t.Fatalf("Catalog projects Cobra hard-required flags as optional required=false: %v", problems)
	}
}

func TestEmbeddedCatalogLocalInterfacesAreExactAndReviewed(t *testing.T) {
	wantReasons := map[string]string{
		"audit.export":       "命令读取并导出本地审计日志文件，不绑定 pinned MCP RPC",
		"audit.tail":         "命令读取本地审计日志尾部，不绑定 pinned MCP RPC",
		"audit.verify":       "命令校验本地审计日志哈希链，不绑定 pinned MCP RPC",
		"dev.connect_status": "命令仅操作本地进程或策略文件，不调用 MCP 接口",
		"dev.connect_stop":   "命令仅操作本地进程或策略文件，不调用 MCP 接口",
		"event.list":         "命令读取 CLI 内置的个人事件目录，不绑定 pinned MCP RPC",
		"event.schema":       "命令读取 CLI 内置的个人事件 payload 定义，不绑定 pinned MCP RPC",
		"pat.browser_policy": "命令仅操作本地进程或策略文件，不调用 MCP 接口",
	}
	loaded := embeddedSchemaCatalog()
	if !embeddedSchemaCatalogAvailable() {
		t.Fatalf("embedded schema Catalog is unavailable: %v", embeddedSchemaCatalogError())
	}
	gotLocal := make(map[string]bool)
	for canonical, tool := range loaded.Snapshot.Tools {
		if schemaString(tool["interface_mode"]) != InterfaceModeLocal {
			continue
		}
		gotLocal[canonical] = true
		wantReason, reviewed := wantReasons[canonical]
		if !reviewed {
			t.Errorf("%s is classified local without a reviewed pure-local implementation", canonical)
			continue
		}
		if schemaString(tool["availability"]) != InterfaceAvailable {
			t.Errorf("%s local availability = %q, want available", canonical, schemaString(tool["availability"]))
		}
		if tool["interface_ref"] != nil {
			t.Errorf("%s local interface_ref = %#v, want nil", canonical, tool["interface_ref"])
		}
		if got := schemaString(tool["interface_reason"]); got != wantReason {
			t.Errorf("%s local reason = %q, want %q", canonical, got, wantReason)
		}
		provenance := schemaMap(tool["field_provenance"])
		for _, field := range []string{"interface_mode", "availability", "interface_ref", "interface_reason"} {
			entry := provenance[field]
			if schemaString(entry["precedence"]) != "reviewed_explicit" {
				t.Errorf("%s local %s precedence = %q, want reviewed_explicit", canonical, field, schemaString(entry["precedence"]))
			}
			if source := schemaString(entry["source"]); !strings.Contains(source, "internal/cli/schema_hints/metadata/") {
				t.Errorf("%s local %s source = %q, want metadata/", canonical, field, source)
			}
		}
	}
	for canonical := range wantReasons {
		if !gotLocal[canonical] {
			t.Errorf("reviewed pure-local command %s is no longer delivered as local", canonical)
		}
	}
}

func TestSchemaParameterPropertyRoot(t *testing.T) {
	tests := []struct {
		property string
		want     string
		ok       bool
	}{
		{property: "request", want: "request", ok: true},
		{property: "request.value", want: "request", ok: true},
		{property: "request[].value", want: "request", ok: true},
		{property: " request[0].value ", want: "request", ok: true},
		{property: "", ok: false},
		{property: ".value", ok: false},
		{property: "[].value", ok: false},
	}
	for _, test := range tests {
		t.Run(test.property, func(t *testing.T) {
			got, ok := schemaParameterPropertyRoot(test.property)
			if got != test.want || ok != test.ok {
				t.Fatalf("schemaParameterPropertyRoot(%q) = %q/%t, want %q/%t", test.property, got, ok, test.want, test.ok)
			}
		})
	}
}

func TestSchemaParameterMappingAuditPreservesFalseAndNestedValues(t *testing.T) {
	ref := map[string]any{"product_id": "sample", "rpc_name": "read"}
	tools := map[string]map[string]any{
		"sample.read": {
			"canonical_path": "sample.read",
			"interface_mode": "mcp",
			"availability":   "available",
			"interface_ref":  ref,
			"parameters": map[string]any{
				"enabled": map[string]any{
					"property": "request[].enabled",
					"required": false,
					"default":  false,
				},
			},
		},
	}
	metadata := embeddedMCPMetadata{Tools: map[string]embeddedMCPToolMetadata{
		"sample.read": {
			InterfaceRef: &embeddedMCPInterfaceRef{ProductID: "sample", RPCName: "read"},
			Parameters: map[string]embeddedMCPParamMeta{
				"request": {Type: "object"},
			},
		},
	}}
	if problems := auditSchemaParameterMappings(tools, metadata, schemaParameterBindingSnapshot{}); len(problems) != 0 {
		t.Fatalf("false/zero-valued parameter was rejected: %v", problems)
	}
}

func TestSchemaParameterMappingAuditExclusionRules(t *testing.T) {
	directMetadata := embeddedMCPMetadata{Tools: map[string]embeddedMCPToolMetadata{
		"sample.read": {
			InterfaceRef: &embeddedMCPInterfaceRef{ProductID: "sample", RPCName: "read"},
			Parameters:   map[string]embeddedMCPParamMeta{"request": {Type: "object"}},
		},
	}}
	excludedParameter := func(candidate string) map[string]any {
		return map[string]any{
			"required": false,
			"field_provenance": map[string]any{
				"property": map[string]any{
					"value":  "",
					"source": "reviewed_mapping_exclusion",
					"candidates": []any{
						map[string]any{"value": "", "source": "reviewed_mapping_exclusion", "selected": true},
						map[string]any{"value": candidate, "source": "flag_name_inference", "selected": false},
					},
				},
			},
		}
	}
	tool := func(mode string, parameter map[string]any) map[string]map[string]any {
		tools := map[string]map[string]any{
			"sample.read": {
				"canonical_path": "sample.read",
				"interface_mode": mode,
				"availability":   "available",
				"interface_ref":  map[string]any{"product_id": "sample", "rpc_name": "read"},
				"parameters":     map[string]any{"selector": parameter},
			},
		}
		if mode == InterfaceModeComposite {
			tools["sample.read"]["interface_ref"] = nil
			tools["sample.read"]["interface_reason"] = "Reviewed composite fixture"
			tools["sample.read"]["field_provenance"] = map[string]any{
				"interface_mode":   map[string]any{"precedence": "reviewed_explicit"},
				"availability":     map[string]any{"precedence": "reviewed_explicit"},
				"interface_ref":    map[string]any{"precedence": "reviewed_explicit"},
				"interface_reason": map[string]any{"precedence": "reviewed_explicit"},
			}
		}
		return tools
	}
	key := "sample.read --selector"
	tests := []struct {
		name        string
		tools       map[string]map[string]any
		metadata    embeddedMCPMetadata
		snapshot    schemaParameterBindingSnapshot
		wantProblem string
	}{
		{
			name:     "reviewed MCP mismatch",
			tools:    tool(InterfaceModeMCP, excludedParameter("cliOnly")),
			metadata: directMetadata,
			snapshot: schemaParameterBindingSnapshot{MappingExclusions: map[string]string{key: "CLI selector is resolved before the RPC"}},
		},
		{
			name:        "redundant MCP exclusion",
			tools:       tool(InterfaceModeMCP, excludedParameter("request.child")),
			metadata:    directMetadata,
			snapshot:    schemaParameterBindingSnapshot{MappingExclusions: map[string]string{key: "stale"}},
			wantProblem: "already resolves",
		},
		{
			name:     "local selector",
			tools:    tool(InterfaceModeLocal, excludedParameter("selector")),
			metadata: embeddedMCPMetadata{Tools: map[string]embeddedMCPToolMetadata{}},
			snapshot: schemaParameterBindingSnapshot{MappingExclusions: map[string]string{key: "local planner input"}},
		},
		{
			name:     "composite selector",
			tools:    tool(InterfaceModeComposite, excludedParameter("selector")),
			metadata: embeddedMCPMetadata{Tools: map[string]embeddedMCPToolMetadata{}},
			snapshot: schemaParameterBindingSnapshot{MappingExclusions: map[string]string{key: "fans out to multiple calls"}},
		},
		{
			name:     "binding conflict",
			tools:    tool(InterfaceModeLocal, excludedParameter("selector")),
			metadata: embeddedMCPMetadata{Tools: map[string]embeddedMCPToolMetadata{}},
			snapshot: schemaParameterBindingSnapshot{
				MappingExclusions: map[string]string{key: "local planner input"},
				Bindings:          map[string]map[string]string{"sample.read": {"selector": "request"}},
			},
			wantProblem: "conflicts with versioned binding",
		},
		{
			name:        "unknown exact key",
			tools:       tool(InterfaceModeLocal, excludedParameter("selector")),
			metadata:    embeddedMCPMetadata{Tools: map[string]embeddedMCPToolMetadata{}},
			snapshot:    schemaParameterBindingSnapshot{MappingExclusions: map[string]string{"sample.read --missing": "not exact"}},
			wantProblem: "does not reference an exact final Catalog parameter",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			problems := auditSchemaParameterMappings(test.tools, test.metadata, test.snapshot)
			if test.wantProblem == "" {
				if len(problems) != 0 {
					t.Fatalf("audit problems = %v", problems)
				}
				return
			}
			if !schemaMappingProblemsContain(problems, test.wantProblem) {
				t.Fatalf("audit problems = %v, want substring %q", problems, test.wantProblem)
			}
		})
	}
}

func TestRuntimeSchemaReviewedMappingExclusionSelectsEmptyProperty(t *testing.T) {
	key := runtimeSchemaParameterMappingKey("sample.read", "local-only")
	snapshot := schemaParameterBindingSnapshot{
		MappingExclusions: map[string]string{key: "CLI-only selector is resolved before the RPC call"},
	}
	binding, exclusion, err := runtimeSchemaParameterMappingCandidates(snapshot, "sample.read", "local-only")
	if err != nil {
		t.Fatal(err)
	}
	winner, err := resolveRuntimeSchemaCandidate("property",
		binding,
		exclusion,
		runtimeSchemaStringCandidate("localOnly", "flag_name_inference"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !winner.Present || winner.Value != "" || winner.Source != "reviewed_mapping_exclusion" || winner.ReviewReason == "" {
		t.Fatalf("mapping exclusion winner = %#v", winner)
	}
	provenance := runtimeSchemaFieldProvenance(winner)
	if provenance.Source != "reviewed_mapping_exclusion" || provenance.ReviewReason == "" {
		t.Fatalf("mapping exclusion provenance = %#v", provenance)
	}
	var value string
	if err := json.Unmarshal(provenance.Value, &value); err != nil || value != "" {
		t.Fatalf("mapping exclusion provenance value = %q/%v", provenance.Value, err)
	}
	selected := 0
	for _, candidate := range provenance.Candidates {
		if candidate.Selected != nil && *candidate.Selected {
			selected++
			if candidate.Source != "reviewed_mapping_exclusion" {
				t.Fatalf("selected candidate = %#v", candidate)
			}
		}
	}
	if selected != 1 {
		t.Fatalf("mapping exclusion selected candidate count = %d", selected)
	}
	payload, err := (ParameterSpec{
		Name: "local-only", Type: "boolean", Description: "test", Property: "",
		FieldProvenance: map[string]FieldProvenance{"property": provenance},
	}).ToPayload()
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := payload["property"]; exists {
		t.Fatalf("excluded property leaked into payload: %#v", payload)
	}
	if schemaMap(payload["field_provenance"])["property"]["source"] != "reviewed_mapping_exclusion" {
		t.Fatalf("excluded property provenance missing from payload: %#v", payload)
	}

	conflict := snapshot
	conflict.Bindings = map[string]map[string]string{"sample.read": {"local-only": "localOnly"}}
	if _, _, err := runtimeSchemaParameterMappingCandidates(conflict, "sample.read", "local-only"); err == nil || !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("binding + exclusion conflict error = %v", err)
	}
	missingReason := schemaParameterBindingSnapshot{MappingExclusions: map[string]string{key: " "}}
	if _, _, err := runtimeSchemaParameterMappingCandidates(missingReason, "sample.read", "local-only"); err == nil || !strings.Contains(err.Error(), "no reason") {
		t.Fatalf("empty exclusion reason error = %v", err)
	}
	rank, precedence := runtimeSchemaSourcePriority("reviewed_mapping_exclusion")
	if rank != runtimeSchemaRankVersionedBinding || precedence != runtimeSchemaPrecedenceMappingExclusion {
		t.Fatalf("mapping exclusion precedence = %d/%q", rank, precedence)
	}
	manualWinner, err := resolveRuntimeSchemaCandidate("property", exclusion, runtimeSchemaManualCandidate("reviewedProperty", true, "reviewed override"))
	if err != nil || manualWinner.Source != "reviewed_manual_hint" || manualWinner.Value != "reviewedProperty" {
		t.Fatalf("reviewed manual must remain above mapping exclusion: winner=%#v err=%v", manualWinner, err)
	}
}

func TestReviewedCompositeParameterMappingRejectsInference(t *testing.T) {
	parameter := func(property, source, reviewReason string) map[string]any {
		return map[string]any{
			"property": property,
			"field_provenance": map[string]any{
				"property": map[string]any{
					"source":        source,
					"review_reason": reviewReason,
				},
			},
		}
	}
	tests := []struct {
		name      string
		parameter map[string]any
		want      bool
	}{
		{name: "versioned binding", parameter: parameter("request.id", "versioned_parameter_binding", ""), want: true},
		{name: "reviewed manual", parameter: parameter("request.id", "reviewed_manual_hint", "reviewed mapping"), want: true},
		{name: "reviewed tool hint", parameter: parameter("request.id", "tool_schema_hint", "reviewed mapping"), want: true},
		{name: "mapping exclusion", parameter: parameter("", "reviewed_mapping_exclusion", "reviewed CLI-only input"), want: true},
		{name: "flag inference", parameter: parameter("requestId", "flag_name_inference", ""), want: false},
		{name: "unreviewed tool hint", parameter: parameter("request.id", "tool_schema_hint", ""), want: false},
		{name: "nonempty exclusion", parameter: parameter("request.id", "reviewed_mapping_exclusion", "reviewed"), want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, _ := reviewedCompositeParameterMapping(test.parameter)
			if got != test.want {
				t.Fatalf("reviewedCompositeParameterMapping() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestRuntimeCommandParameterSpecsPreserveReviewedEmptyPropertyProvenance(t *testing.T) {
	cmd := &cobra.Command{Use: "query"}
	cmd.Flags().Bool("all", false, "fetch every page")

	parameters, err := runtimeCommandParameterSpecs(cmd, "aitable.query_records", nil, nil, RuntimeSchemaConstraints{})
	if err != nil {
		t.Fatal(err)
	}
	if len(parameters) != 1 {
		t.Fatalf("parameter count = %d, want 1", len(parameters))
	}
	parameter := parameters[0]
	if parameter.Property != "" {
		t.Fatalf("excluded property = %q, want empty", parameter.Property)
	}
	provenance, exists := parameter.FieldProvenance["property"]
	if !exists || provenance.Source != "reviewed_mapping_exclusion" || provenance.ReviewReason == "" {
		t.Fatalf("excluded property provenance = %#v", provenance)
	}
	payload, err := parameter.ToPayload()
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := payload["property"]; exists {
		t.Fatalf("excluded property leaked into payload: %#v", payload)
	}
	if got := schemaString(schemaMap(payload["field_provenance"])["property"]["source"]); got != "reviewed_mapping_exclusion" {
		t.Fatalf("delivered property provenance source = %q", got)
	}
}

func TestSchemaParameterBindingCorrectionsAreReviewed(t *testing.T) {
	snapshot, err := runtimeSchemaParameterBindingData()
	if err != nil {
		t.Fatalf("runtimeSchemaParameterBindingData() error = %v", err)
	}
	required := map[string]schemaParameterBindingCorrection{
		"calendar.search_calendar --query": {
			OldProperty: "calendarName",
			NewProperty: "query",
		},
		"minutes.query_minutes_by_tag_id --limit": {
			OldProperty: "size",
			NewProperty: "maxResults",
		},
	}
	flags := finalSchemaCatalogFlagIndex(embeddedSchemaCatalog().Snapshot.Tools)
	for key, correction := range snapshot.Corrections {
		flag, exists := flags[key]
		if !exists {
			t.Errorf("correction %q does not reference an exact final Catalog parameter", key)
			continue
		}
		oldProperty := strings.TrimSpace(correction.OldProperty)
		newProperty := strings.TrimSpace(correction.NewProperty)
		if oldProperty == "" || newProperty == "" || oldProperty == newProperty {
			t.Errorf("correction %q has invalid old/new properties: %#v", key, correction)
		}
		if strings.TrimSpace(correction.Reason) == "" || !correction.Reviewed {
			t.Errorf("correction %q is not reviewed with a non-empty reason: %#v", key, correction)
		}
		if got := snapshot.Bindings[flag.canonical][flag.flagName]; got != newProperty {
			t.Errorf("correction %q active binding = %q, want new_property %q", key, got, newProperty)
		}
	}
	for key, want := range required {
		got, exists := snapshot.Corrections[key]
		if !exists {
			t.Errorf("required reviewed correction %q is missing", key)
			continue
		}
		if got.OldProperty != want.OldProperty || got.NewProperty != want.NewProperty {
			t.Errorf("correction %q = %q -> %q, want %q -> %q", key, got.OldProperty, got.NewProperty, want.OldProperty, want.NewProperty)
		}
	}
}

func auditSchemaParameterMappings(tools map[string]map[string]any, metadata embeddedMCPMetadata, snapshot schemaParameterBindingSnapshot) []string {
	flags := finalSchemaCatalogFlagIndex(tools)
	problems := make([]string, 0)

	canonicals := make([]string, 0, len(tools))
	for canonical := range tools {
		canonicals = append(canonicals, canonical)
	}
	sort.Strings(canonicals)
	for _, canonical := range canonicals {
		tool := tools[canonical]
		mode := strings.TrimSpace(schemaString(tool["interface_mode"]))
		availability := strings.TrimSpace(schemaString(tool["availability"]))
		if mode == InterfaceModeComposite && availability == InterfaceAvailable {
			problems = append(problems, auditCompositeSchemaDisposition(canonical, tool)...)
			if strings.HasPrefix(strings.TrimSpace(schemaString(tool["interface_reason"])), "Reviewed unpinned remote adapter:") {
				parameters := schemaMap(tool["parameters"])
				for flagName, parameter := range parameters {
					if ok, detail := reviewedCompositeParameterMapping(parameter); !ok {
						problems = append(problems, fmt.Sprintf(
							"%s unpinned remote adapter has no reviewed explicit parameter mapping: %s",
							runtimeSchemaParameterMappingKey(canonical, flagName), detail,
						))
					}
				}
			}
			continue
		}
		if mode != InterfaceModeMCP || availability != InterfaceAvailable {
			continue
		}
		metadataKey, pinned, resolveProblems := pinnedMCPParameterMetadata(canonical, tool, metadata)
		problems = append(problems, resolveProblems...)
		if len(pinned.Parameters) == 0 {
			continue
		}
		parameters := schemaMap(tool["parameters"])
		flagNames := make([]string, 0, len(parameters))
		for flagName := range parameters {
			flagNames = append(flagNames, flagName)
		}
		sort.Strings(flagNames)
		for _, flagName := range flagNames {
			key := runtimeSchemaParameterMappingKey(canonical, flagName)
			flag := flags[key]
			flag.metadataKey = metadataKey
			flag.metadataParams = pinned.Parameters
			flags[key] = flag
			reason, excluded := snapshot.MappingExclusions[key]
			if excluded && strings.TrimSpace(reason) != "" {
				continue
			}
			root, ok := schemaParameterPropertyRoot(flag.property)
			if ok {
				_, ok = pinned.Parameters[root]
			}
			if !ok {
				problems = append(problems, fmt.Sprintf(
					"%s property %q does not resolve to pinned MCP metadata %s; add/fix a binding or an exact reviewed mapping_exclusions entry",
					key, flag.property, metadataKey,
				))
			}
		}
	}

	exclusionKeys := make([]string, 0, len(snapshot.MappingExclusions))
	for key := range snapshot.MappingExclusions {
		exclusionKeys = append(exclusionKeys, key)
	}
	sort.Strings(exclusionKeys)
	for _, key := range exclusionKeys {
		reason := strings.TrimSpace(snapshot.MappingExclusions[key])
		flag, exists := flags[key]
		if !exists {
			problems = append(problems, fmt.Sprintf("mapping_exclusions %q does not reference an exact final Catalog parameter", key))
			continue
		}
		if reason == "" {
			problems = append(problems, fmt.Sprintf("mapping_exclusions %q has an empty reviewed reason", key))
		}
		if binding := strings.TrimSpace(snapshot.Bindings[flag.canonical][flag.flagName]); binding != "" {
			problems = append(problems, fmt.Sprintf("mapping_exclusions %q conflicts with versioned binding %q", key, binding))
		}
		if strings.TrimSpace(flag.property) != "" {
			problems = append(problems, fmt.Sprintf("mapping_exclusions %q must deliver an omitted property, got %q", key, flag.property))
		}
		switch flag.mode {
		case InterfaceModeLocal, InterfaceModeComposite:
			// These modes do not claim one direct MCP parameter map. The exact,
			// reviewed reason and omitted final property are the whole contract.
			continue
		case InterfaceModeMCP:
			if flag.availability != InterfaceAvailable || len(flag.metadataParams) == 0 {
				problems = append(problems, fmt.Sprintf("mapping_exclusions %q is not attached to an available pinned MCP parameter map", key))
				continue
			}
			directProperty := schemaExcludedDirectPropertyCandidate(flag.parameter)
			root, ok := schemaParameterPropertyRoot(directProperty)
			if !ok {
				problems = append(problems, fmt.Sprintf("mapping_exclusions %q has no lower-priority direct property candidate to review", key))
				continue
			}
			if _, resolves := flag.metadataParams[root]; resolves {
				problems = append(problems, fmt.Sprintf("mapping_exclusions %q is stale: candidate property %q already resolves to %s", key, directProperty, flag.metadataKey))
			}
		default:
			problems = append(problems, fmt.Sprintf("mapping_exclusions %q is only valid for mcp, local, or composite tools", key))
		}
	}

	sort.Strings(problems)
	return problems
}

func reviewedCompositeParameterMapping(parameter map[string]any) (bool, string) {
	property := strings.TrimSpace(schemaString(parameter["property"]))
	provenance := schemaMap(parameter["field_provenance"])["property"]
	source := strings.TrimSpace(schemaString(provenance["source"]))
	reviewReason := strings.TrimSpace(schemaString(provenance["review_reason"]))
	switch source {
	case "versioned_parameter_binding", "typed_parameter_metadata", "native_annotation":
		if property == "" {
			return false, fmt.Sprintf("%s selected an empty property", source)
		}
		return true, ""
	case "reviewed_manual_hint", "tool_schema_hint":
		if reviewReason == "" {
			return false, fmt.Sprintf("%s has no review_reason", source)
		}
		if property == "" {
			return false, fmt.Sprintf("%s selected an empty property without a mapping exclusion", source)
		}
		return true, ""
	case "reviewed_mapping_exclusion":
		if property != "" {
			return false, fmt.Sprintf("reviewed_mapping_exclusion delivered property %q", property)
		}
		return true, ""
	default:
		return false, fmt.Sprintf("property provenance source is %q", source)
	}
}

func auditCompositeSchemaDisposition(canonical string, tool map[string]any) []string {
	problems := make([]string, 0)
	if tool["interface_ref"] != nil {
		problems = append(problems, fmt.Sprintf("%s composite interface must not advertise a direct interface_ref", canonical))
	}
	if strings.TrimSpace(schemaString(tool["interface_reason"])) == "" {
		problems = append(problems, fmt.Sprintf("%s composite interface has no reviewed disposition reason", canonical))
	}
	provenance := schemaMap(tool["field_provenance"])
	for _, field := range []string{"interface_mode", "availability", "interface_ref", "interface_reason"} {
		entry := provenance[field]
		if strings.TrimSpace(schemaString(entry["precedence"])) != "reviewed_explicit" {
			problems = append(problems, fmt.Sprintf("%s composite %s is not backed by reviewed_explicit provenance", canonical, field))
		}
	}
	return problems
}

func finalSchemaCatalogFlagIndex(tools map[string]map[string]any) map[string]schemaParameterMappingFlagAudit {
	flags := make(map[string]schemaParameterMappingFlagAudit)
	for canonical, tool := range tools {
		for flagName, parameter := range schemaMap(tool["parameters"]) {
			key := runtimeSchemaParameterMappingKey(canonical, flagName)
			flags[key] = schemaParameterMappingFlagAudit{
				canonical:    canonical,
				flagName:     flagName,
				mode:         strings.TrimSpace(schemaString(tool["interface_mode"])),
				availability: strings.TrimSpace(schemaString(tool["availability"])),
				property:     strings.TrimSpace(schemaString(parameter["property"])),
				parameter:    parameter,
			}
		}
	}
	return flags
}

func pinnedMCPParameterMetadata(canonical string, tool map[string]any, metadata embeddedMCPMetadata) (string, embeddedMCPToolMetadata, []string) {
	refMap, _ := tool["interface_ref"].(map[string]any)
	wantRef := embeddedMCPInterfaceRef{
		ProductID: strings.TrimSpace(schemaString(refMap["product_id"])),
		RPCName:   strings.TrimSpace(schemaString(refMap["rpc_name"])),
	}
	if wantRef.ProductID == "" || wantRef.RPCName == "" {
		return "", embeddedMCPToolMetadata{}, []string{fmt.Sprintf("%s has no complete pinned interface_ref", canonical)}
	}
	if exact, ok := metadata.Tools[canonical]; ok {
		problems := comparePinnedMCPRef(canonical, canonical, exact, wantRef)
		return canonical, exact, problems
	}

	keys := make([]string, 0)
	for key, candidate := range metadata.Tools {
		if candidate.InterfaceRef != nil &&
			strings.TrimSpace(candidate.InterfaceRef.ProductID) == wantRef.ProductID &&
			strings.TrimSpace(candidate.InterfaceRef.RPCName) == wantRef.RPCName {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return "", embeddedMCPToolMetadata{}, []string{fmt.Sprintf("%s pinned interface_ref %s.%s has no MCP metadata", canonical, wantRef.ProductID, wantRef.RPCName)}
	}
	first := metadata.Tools[keys[0]]
	wantParameters := sortedMCPParameterKeys(first.Parameters)
	for _, key := range keys[1:] {
		if got := strings.Join(sortedMCPParameterKeys(metadata.Tools[key].Parameters), "\x00"); got != strings.Join(wantParameters, "\x00") {
			return "", embeddedMCPToolMetadata{}, []string{fmt.Sprintf("%s pinned interface_ref %s.%s has ambiguous parameter maps across metadata keys %s", canonical, wantRef.ProductID, wantRef.RPCName, strings.Join(keys, ", "))}
		}
	}
	return strings.Join(keys, "|"), first, nil
}

func comparePinnedMCPRef(canonical, metadataKey string, metadata embeddedMCPToolMetadata, want embeddedMCPInterfaceRef) []string {
	if metadata.InterfaceRef == nil {
		return []string{fmt.Sprintf("MCP metadata %s selected by %s has no interface_ref", metadataKey, canonical)}
	}
	gotProduct := strings.TrimSpace(metadata.InterfaceRef.ProductID)
	gotRPC := strings.TrimSpace(metadata.InterfaceRef.RPCName)
	if gotProduct != want.ProductID || gotRPC != want.RPCName {
		return []string{fmt.Sprintf("%s pins %s.%s but canonical MCP metadata %s pins %s.%s", canonical, want.ProductID, want.RPCName, metadataKey, gotProduct, gotRPC)}
	}
	return nil
}

func sortedMCPParameterKeys(parameters map[string]embeddedMCPParamMeta) []string {
	keys := make([]string, 0, len(parameters))
	for key := range parameters {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func schemaParameterPropertyRoot(property string) (string, bool) {
	property = strings.TrimSpace(property)
	if property == "" {
		return "", false
	}
	end := len(property)
	if index := strings.IndexAny(property, ".["); index >= 0 {
		end = index
	}
	root := strings.TrimSpace(property[:end])
	return root, root != ""
}

func schemaExcludedDirectPropertyCandidate(parameter map[string]any) string {
	fieldProvenance := schemaMap(parameter["field_provenance"])["property"]
	for _, raw := range schemaMapSlice(fieldProvenance["candidates"]) {
		if schemaString(raw["source"]) == "reviewed_mapping_exclusion" {
			continue
		}
		if value := strings.TrimSpace(schemaString(raw["value"])); value != "" {
			return value
		}
	}
	return ""
}

func limitedSchemaMappingProblems(problems []string, limit int) string {
	if len(problems) <= limit {
		return strings.Join(problems, "\n")
	}
	return strings.Join(problems[:limit], "\n") + fmt.Sprintf("\n... %d additional problem(s) omitted", len(problems)-limit)
}

func schemaMappingProblemsContain(problems []string, substring string) bool {
	for _, problem := range problems {
		if strings.Contains(problem, substring) {
			return true
		}
	}
	return false
}
