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

package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestManualSchemaHintsIncludeExistingLeafAndOverrideParameters(t *testing.T) {
	root, leaf := manualSchemaHintTestTree()
	description := "Reviewed query text"
	property := "queryText"
	interfaceType := "object"
	required := true
	requiredWhen := "mode is advanced"
	snapshot := ManualSchemaHintSnapshot{
		Schema:  manualSchemaHintSchemaRef,
		Version: manualSchemaHintVersion,
		Commands: []ManualSchemaCommandHint{{
			CLIPath:       "sample item search",
			CanonicalPath: "sample.search_items",
			Reason:        "Reviewed public helper",
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
	}

	report, err := applyManualSchemaHints(root, snapshot)
	if err != nil {
		t.Fatalf("applyManualSchemaHints() error = %v", err)
	}
	if len(report.Commands) != 1 || report.Commands[0] != "sample item search" {
		t.Fatalf("report.Commands = %#v", report.Commands)
	}
	productID, toolName, reason, ok := runtimeManualSchemaIdentity(leaf)
	if !ok || productID != "sample" || toolName != "search_items" || reason != "Reviewed public helper" {
		t.Fatalf("Schema identity = %s.%s (%s)", productID, toolName, reason)
	}

	payload, err := runtimeSchemaPayloadForTest(root, []string{"sample.search_items"})
	if err != nil {
		t.Fatalf("runtimeSchemaPayloadForTest() error = %v", err)
	}
	parameters := schemaMap(payload["parameters"])
	query := parameters["query"]
	if query["description"] != description || query["property"] != property || query["interface_type"] != interfaceType || query["required"] != true || query["required_when"] != requiredWhen {
		t.Fatalf("query parameter = %#v", query)
	}
	if leaf.Flags().Lookup("query").Usage != "Original query text" {
		t.Fatalf("human Schema hint changed Cobra help: %q", leaf.Flags().Lookup("query").Usage)
	}
	if len(leaf.Flags().Lookup("query").Annotations[cobra.BashCompOneRequiredFlag]) != 0 {
		t.Fatal("manual Schema hint changed Cobra execution validation")
	}
}

func TestManualSchemaParameterHintsDoNotLeakAcrossSharedFlags(t *testing.T) {
	root, query := manualSchemaHintTestTree()
	get := &cobra.Command{Use: "get", RunE: func(*cobra.Command, []string) error { return nil }}
	get.Flags().AddFlag(query.Flags().Lookup("query"))
	query.Parent().AddCommand(get)

	required := false
	_, err := applyManualSchemaHints(root, ManualSchemaHintSnapshot{
		Schema:  manualSchemaHintSchemaRef,
		Version: manualSchemaHintVersion,
		Commands: []ManualSchemaCommandHint{{
			CLIPath:       "sample item search",
			CanonicalPath: "sample.search_items",
			Reason:        "Only the broad search command allows an omitted query.",
			Reviewed:      true,
			Parameters: map[string]ManualSchemaParameterHint{
				"query": {Required: &required},
			},
		}},
	})
	if err != nil {
		t.Fatalf("applyManualSchemaHints() error = %v", err)
	}

	canonical := "sample.get_item"
	previous, existed := runtimeSchemaParameterMetadataByCanonical[canonical]
	runtimeSchemaParameterMetadataByCanonical[canonical] = RuntimeSchemaParameterMetadata{Required: []string{"query"}}
	t.Cleanup(func() {
		if existed {
			runtimeSchemaParameterMetadataByCanonical[canonical] = previous
		} else {
			delete(runtimeSchemaParameterMetadataByCanonical, canonical)
		}
	})

	queryParameters, err := runtimeCommandParameters(query, "sample.search_items", nil, nil, RuntimeSchemaConstraints{})
	if err != nil {
		t.Fatalf("query parameters: %v", err)
	}
	if got := schemaParameterEntry(t, queryParameters, "query")["required"]; got != false {
		t.Fatalf("query required = %#v, want false", got)
	}

	getParameters, err := runtimeCommandParameters(get, canonical, nil, nil, RuntimeSchemaConstraints{})
	if err != nil {
		t.Fatalf("get parameters: %v", err)
	}
	getQuery := schemaParameterEntry(t, getParameters, "query")
	if getQuery["required"] != true {
		t.Fatalf("get required = %#v, want true", getQuery["required"])
	}
	if source := schemaParameterFieldSource(t, getQuery, "required"); source != "typed_parameter_metadata" {
		t.Fatalf("get required source = %q, want typed_parameter_metadata", source)
	}
	if _, _, present, err := runtimeManualSchemaParameter(get, "query"); err != nil || present {
		t.Fatalf("manual parameter leaked to sibling command: present=%t err=%v", present, err)
	}
}

func TestManualSchemaHintsRejectInvalidOrStaleInputs(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*ManualSchemaCommandHint)
		wantErr string
	}{
		{name: "missing command", mutate: func(h *ManualSchemaCommandHint) { h.CLIPath = "sample item missing" }, wantErr: "does not resolve"},
		{name: "wildcard command", mutate: func(h *ManualSchemaCommandHint) { h.CLIPath = "sample item *" }, wantErr: "invalid exact cli_path"},
		{name: "not reviewed", mutate: func(h *ManualSchemaCommandHint) { h.Reviewed = false }, wantErr: "not reviewed"},
		{name: "missing reason", mutate: func(h *ManualSchemaCommandHint) { h.Reason = "" }, wantErr: "has no reason"},
		{name: "bad canonical", mutate: func(h *ManualSchemaCommandHint) { h.CanonicalPath = "sample" }, wantErr: "invalid canonical_path"},
		{name: "missing flag", mutate: func(h *ManualSchemaCommandHint) {
			h.Parameters = map[string]ManualSchemaParameterHint{"missing": {Required: boolPointer(true)}}
		}, wantErr: "missing flag --missing"},
		{name: "invalid interface type", mutate: func(h *ManualSchemaCommandHint) {
			h.Parameters = map[string]ManualSchemaParameterHint{"query": {InterfaceType: stringPointer("made-up")}}
		}, wantErr: "unsupported interface_type"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root, _ := manualSchemaHintTestTree()
			hint := ManualSchemaCommandHint{
				CLIPath:       "sample item search",
				CanonicalPath: "sample.search_items",
				Reason:        "Reviewed public helper",
				Reviewed:      true,
			}
			test.mutate(&hint)
			_, err := applyManualSchemaHints(root, ManualSchemaHintSnapshot{
				Schema:   manualSchemaHintSchemaRef,
				Version:  manualSchemaHintVersion,
				Commands: []ManualSchemaCommandHint{hint},
			})
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func TestManualSchemaHintsRejectCanonicalConflict(t *testing.T) {
	root, leaf := manualSchemaHintTestTree()
	AttachRuntimeSchema(leaf, "sample", "existing", "test")
	_, err := applyManualSchemaHints(root, ManualSchemaHintSnapshot{
		Schema:  manualSchemaHintSchemaRef,
		Version: manualSchemaHintVersion,
		Commands: []ManualSchemaCommandHint{{
			CLIPath:       "sample item search",
			CanonicalPath: "sample.replacement",
			Reason:        "Reviewed public helper",
			Reviewed:      true,
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "conflicts with existing canonical path") {
		t.Fatalf("error = %v", err)
	}
}

func TestManualSchemaHintsRejectAmbiguousCobraAlias(t *testing.T) {
	root, search := manualSchemaHintTestTree()
	search.Aliases = []string{"find"}
	list := &cobra.Command{Use: "list", Aliases: []string{"find"}, RunE: func(*cobra.Command, []string) error { return nil }}
	search.Parent().AddCommand(list)

	_, err := applyManualSchemaHints(root, ManualSchemaHintSnapshot{
		Schema:  manualSchemaHintSchemaRef,
		Version: manualSchemaHintVersion,
		Commands: []ManualSchemaCommandHint{{
			CLIPath:       "sample item find",
			CanonicalPath: "sample.search_items",
			Reason:        "Ambiguous alias fixture",
			Reviewed:      true,
		}},
	})
	if err == nil || !strings.Contains(err.Error(), `cobra alias segment "find" is ambiguous`) {
		t.Fatalf("error = %v", err)
	}
}

func TestManualSchemaHintsResolveReviewedCobraAliasExactly(t *testing.T) {
	root := commandRegistryTestRoot("aitable record query")
	exactSchemaCommand(root, "aitable record").Aliases = []string{"records"}
	leaf := exactSchemaCommand(root, "aitable record query")

	report, err := applyManualSchemaHints(root, ManualSchemaHintSnapshot{
		Schema:  manualSchemaHintSchemaRef,
		Version: manualSchemaHintVersion,
		Commands: []ManualSchemaCommandHint{{
			CLIPath:       "aitable records query",
			CanonicalPath: "aitable.query_records",
			Reason:        "Reviewed ancestor Cobra alias",
			Reviewed:      true,
		}},
	})
	if err != nil {
		t.Fatalf("applyManualSchemaHints() error = %v", err)
	}
	if got := strings.Join(report.Commands, ","); got != "aitable records query" {
		t.Fatalf("report commands = %q", got)
	}
	productID, toolName, _, ok := runtimeManualSchemaIdentity(leaf)
	if !ok || productID+"."+toolName != "aitable.query_records" {
		t.Fatalf("manual identity = %s.%s, ok=%v", productID, toolName, ok)
	}
}

func TestDecodeManualSchemaHintsRejectsUnknownFields(t *testing.T) {
	_, err := decodeManualSchemaHints([]byte(`{"$schema":"./schema_manual_hints.schema.json","version":1,"commands":[],"allow_virtual_commands":true}`))
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("error = %v", err)
	}
}

func TestDecodeManualSchemaHintsRequiresDiscoverableSchema(t *testing.T) {
	_, err := decodeManualSchemaHints([]byte(`{"version":1,"commands":[]}`))
	if err == nil || !strings.Contains(err.Error(), "must declare $schema") {
		t.Fatalf("error = %v", err)
	}
}

func TestDecodeManualSchemaHintsRequiresAgentHints(t *testing.T) {
	_, err := decodeManualSchemaHints([]byte(`{"$schema":"./schema_manual_hints.schema.json","version":1,"commands":[]}`))
	if err == nil || !strings.Contains(err.Error(), "agent_hints.revisions must not be empty") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateManualAgentHintSetRequiresReviewedExactCoverage(t *testing.T) {
	hints := manualAgentHintSetFixture()
	if err := ValidateManualAgentHintSet(hints, map[string]bool{"sample": true}, map[string]bool{"sample.search_items": true}); err != nil {
		t.Fatalf("ValidateManualAgentHintSet() error = %v", err)
	}

	tests := []struct {
		name    string
		mutate  func(*ManualAgentHintSet)
		wantErr string
	}{
		{
			name: "unknown revision",
			mutate: func(hints *ManualAgentHintSet) {
				tool := hints.Tools["sample.search_items"]
				tool.Revision = "missing"
				hints.Tools["sample.search_items"] = tool
			},
			wantErr: "unknown revision",
		},
		{
			name: "unreviewed",
			mutate: func(hints *ManualAgentHintSet) {
				tool := hints.Tools["sample.search_items"]
				tool.Reviewed = false
				hints.Tools["sample.search_items"] = tool
			},
			wantErr: "must be reviewed",
		},
		{
			name: "missing examples",
			mutate: func(hints *ManualAgentHintSet) {
				tool := hints.Tools["sample.search_items"]
				tool.Examples = nil
				hints.Tools["sample.search_items"] = tool
			},
			wantErr: "requires non-empty examples",
		},
		{
			name: "confirmation bypass",
			mutate: func(hints *ManualAgentHintSet) {
				tool := hints.Tools["sample.search_items"]
				tool.Examples = []string{"dws sample item search --query x --yes"}
				hints.Tools["sample.search_items"] = tool
			},
			wantErr: "must not bypass confirmation",
		},
		{
			name: "missing Registry tool",
			mutate: func(hints *ManualAgentHintSet) {
				delete(hints.Tools, "sample.search_items")
			},
			wantErr: "missing=[sample.search_items]",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := manualAgentHintSetFixture()
			test.mutate(&candidate)
			err := ValidateManualAgentHintSet(candidate, map[string]bool{"sample": true}, map[string]bool{"sample.search_items": true})
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func TestDecodeManualSchemaHintsRejectsUnknownManualAgentField(t *testing.T) {
	_, err := decodeManualSchemaHints([]byte(`{
  "$schema":"./schema_manual_hints.schema.json",
  "version":1,
  "commands":[],
  "agent_hints":{
    "revisions":{"v1":{"generated_by":"ai","model":"gpt-5","prompt_version":"v1","reason":"fixture"}},
    "products":{},
    "tools":{},
    "allow_generated_overwrite":true
  }
}`))
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateManualAgentHintExamplesUsesBoundCobraContract(t *testing.T) {
	root, leaf := manualSchemaHintTestTree()
	root.PersistentFlags().String("format", "json", "output format")
	alias := &cobra.Command{Use: "find", RunE: func(*cobra.Command, []string) error { return nil }}
	alias.Flags().StringP("legacy-query", "l", "", "compatibility query")
	leaf.Parent().AddCommand(alias)
	boundSpec := BoundCommandSpec{
		CommandSpec: CommandSpec{
			CanonicalPath:  "sample.search_items",
			PrimaryCLIPath: "sample item search",
			Aliases:        []string{"sample item find"},
		},
		PrimaryCommand: leaf,
		AliasCommands: []BoundAlias{{
			Path:    "sample item find",
			Command: alias,
			Kind:    AliasKindCompatibilityLeaf,
		}},
	}
	bound := BoundCommandRegistry{
		Commands:    []BoundCommandSpec{boundSpec},
		ByCanonical: map[string]BoundCommandSpec{"sample.search_items": boundSpec},
		ByCLIPath: map[string]BoundCommandSpec{
			"sample item search": boundSpec,
			"sample item find":   boundSpec,
		},
	}
	hints := manualAgentHintSetFixture()
	tool := hints.Tools["sample.search_items"]
	tool.Examples = []string{"dws sample item find -l <item-id> --format json"}
	hints.Tools["sample.search_items"] = tool
	if err := ValidateManualAgentHintExamples(bound, hints); err != nil {
		t.Fatalf("ValidateManualAgentHintExamples() error = %v", err)
	}

	tool.Examples = []string{"dws sample item search -q 'value with spaces' --format=json"}
	hints.Tools["sample.search_items"] = tool
	if err := ValidateManualAgentHintExamples(bound, hints); err != nil {
		t.Fatalf("primary shorthand example error = %v", err)
	}

	tool.Examples = []string{"dws sample item search -l value"}
	hints.Tools["sample.search_items"] = tool
	if err := ValidateManualAgentHintExamples(bound, hints); err == nil || !strings.Contains(err.Error(), "unknown shorthand flag -l") {
		t.Fatalf("alias-only shorthand on primary error = %v", err)
	}

	tool.Examples = []string{"dws sample item search --invented value"}
	hints.Tools["sample.search_items"] = tool
	if err := ValidateManualAgentHintExamples(bound, hints); err == nil || !strings.Contains(err.Error(), "unknown flag --invented") {
		t.Fatalf("unknown flag error = %v", err)
	}

	tool.Examples = []string{"dws sample item list --query value"}
	hints.Tools["sample.search_items"] = tool
	if err := ValidateManualAgentHintExamples(bound, hints); err == nil || !strings.Contains(err.Error(), "does not use its reviewed") {
		t.Fatalf("wrong path error = %v", err)
	}
}

func TestValidateManualAgentHintExamplesRejectsUnsafeOrNonExecutingArgv(t *testing.T) {
	_, leaf := manualSchemaHintTestTree()
	boundSpec := BoundCommandSpec{
		CommandSpec: CommandSpec{
			CanonicalPath:  "sample.search_items",
			PrimaryCLIPath: "sample item search",
		},
		PrimaryCommand: leaf,
	}
	bound := BoundCommandRegistry{
		Commands:    []BoundCommandSpec{boundSpec},
		ByCanonical: map[string]BoundCommandSpec{"sample.search_items": boundSpec},
		ByCLIPath:   map[string]BoundCommandSpec{"sample item search": boundSpec},
	}
	tests := []struct {
		name    string
		example string
		wantErr string
	}{
		{name: "quoted operators are data", example: `dws sample item search --query "A && B > C"`},
		{name: "quoted multiline value is one argument", example: "dws sample item search --query '{\n\"key\": \"value\"\n}'"},
		{name: "placeholder is data", example: `dws sample item search --query <item-id>,<other_id>`},
		{name: "attached shorthand value", example: `dws sample item search -qvalue`},
		{name: "chaining", example: `dws sample item search --query x && dws sample item search --query y`, wantErr: "shell operator"},
		{name: "unquoted newline", example: "dws sample item search --query x\ndws sample item search --query y", wantErr: "unquoted newline shell operator"},
		{name: "semicolon", example: `dws sample item search --query x; dws sample item search --query y`, wantErr: "shell operator"},
		{name: "pipe", example: `dws sample item search --query x | tee out`, wantErr: "shell operator"},
		{name: "redirect", example: `dws sample item search --query x > out`, wantErr: "shell operator"},
		{name: "input redirect", example: `dws sample item search < input`, wantErr: "redirection operator"},
		{name: "command substitution", example: `dws sample item search --query $(whoami)`, wantErr: "shell expansion"},
		{name: "backticks", example: "dws sample item search --query `whoami`", wantErr: "shell expansion"},
		{name: "argument terminator", example: `dws sample item search -- --query x`, wantErr: "argument terminator"},
		{name: "help long", example: `dws sample item search --help`, wantErr: "not only --help"},
		{name: "help shorthand", example: `dws sample item search -h`, wantErr: "not only -h"},
		{name: "unknown shorthand", example: `dws sample item search -x value`, wantErr: "unknown shorthand flag -x"},
		{name: "unterminated quote", example: `dws sample item search --query "value`, wantErr: "unterminated quoted value"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			hints := manualAgentHintSetFixture()
			tool := hints.Tools["sample.search_items"]
			tool.Examples = []string{test.example}
			hints.Tools["sample.search_items"] = tool
			err := ValidateManualAgentHintExamples(bound, hints)
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateManualAgentHintExamples() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func TestBuildManualAgentExampleExecutionPlanUsesExplicitReviewedDispositions(t *testing.T) {
	_, leaf := manualSchemaHintTestTree()
	boundSpec := BoundCommandSpec{
		CommandSpec: CommandSpec{
			CanonicalPath:  "sample.search_items",
			PrimaryCLIPath: "sample item search",
		},
		PrimaryCommand: leaf,
	}
	bound := BoundCommandRegistry{
		Commands:    []BoundCommandSpec{boundSpec},
		ByCanonical: map[string]BoundCommandSpec{"sample.search_items": boundSpec},
		ByCLIPath:   map[string]BoundCommandSpec{"sample item search": boundSpec},
	}
	hints := manualAgentHintSetFixture()
	tool := hints.Tools["sample.search_items"]
	tool.Examples = []string{
		"dws sample item search --query first",
		"dws sample item search --query second",
	}
	hints.Tools["sample.search_items"] = tool

	registry := manualAgentExampleSchemaRegistry("sample.search_items", SafetySpec{})
	plan, err := BuildManualAgentExampleExecutionPlan(bound, registry, hints)
	if err != nil {
		t.Fatalf("BuildManualAgentExampleExecutionPlan() error = %v", err)
	}
	if plan.Total != 2 || plan.Contract != 2 || plan.DryRun != 0 || plan.ContractOnly != 0 {
		t.Fatalf("default plan counts = total:%d contract:%d dry_run:%d contract_only:%d", plan.Total, plan.Contract, plan.DryRun, plan.ContractOnly)
	}
	for _, execution := range plan.Examples {
		if execution.Mode != ManualAgentExampleModeContract || execution.DryRun != nil {
			t.Fatalf("example without an explicit capability = %#v, want contract-only validation", execution)
		}
	}

	registry.Products[0].Tools[0].DryRun = &DryRunSpec{PreviewKind: DryRunPreviewPlan}
	plan, err = BuildManualAgentExampleExecutionPlan(bound, registry, hints)
	if err != nil {
		t.Fatalf("BuildManualAgentExampleExecutionPlan() with explicit dry_run error = %v", err)
	}
	if plan.Total != 2 || plan.Contract != 0 || plan.DryRun != 2 || plan.ContractOnly != 0 {
		t.Fatalf("explicit capability plan = %#v", plan)
	}
	for _, execution := range plan.Examples {
		if execution.Mode != ManualAgentExampleModeDryRun || execution.DryRun == nil {
			t.Fatalf("explicit capability execution = %#v, want dry_run", execution)
		}
	}

	index := 1
	tool.ExampleDispositions = []ManualAgentExampleDisposition{{
		Index:      &index,
		Mode:       ManualAgentExampleModeContractOnly,
		ReasonCode: ManualAgentExampleReasonStatefulPreflight,
		Reason:     "The command checks an authenticated tenant before its dry-run branch.",
		Reviewed:   true,
	}}
	hints.Tools["sample.search_items"] = tool
	plan, err = BuildManualAgentExampleExecutionPlan(bound, registry, hints)
	if err != nil {
		t.Fatalf("BuildManualAgentExampleExecutionPlan() with disposition error = %v", err)
	}
	if plan.Total != 2 || plan.DryRun != 1 || plan.ContractOnly != 1 || plan.ReviewedContractOnly != 1 || plan.ContractOnlyByReason[ManualAgentExampleReasonStatefulPreflight] != 1 {
		t.Fatalf("reviewed plan = %#v", plan)
	}
	if plan.Examples[0].Mode != ManualAgentExampleModeDryRun || plan.Examples[1].Mode != ManualAgentExampleModeContractOnly {
		t.Fatalf("resolved example modes = %#v", plan.Examples)
	}

	tool.ExampleDispositions = nil
	hints.Tools["sample.search_items"] = tool
	registry = manualAgentExampleSchemaRegistry("sample.search_items", SafetySpec{Risk: "high", Confirmation: "user_required"})
	registry.Products[0].Tools[0].DryRun = &DryRunSpec{PreviewKind: DryRunPreviewPlan}
	plan, err = BuildManualAgentExampleExecutionPlan(bound, registry, hints)
	if err != nil {
		t.Fatalf("BuildManualAgentExampleExecutionPlan() with typed safety error = %v", err)
	}
	if plan.DryRun != 2 || plan.ContractOnly != 0 {
		t.Fatalf("typed safety plan = %#v", plan)
	}
	for _, execution := range plan.Examples {
		if execution.Mode != ManualAgentExampleModeDryRun || execution.Source != ManualAgentExampleDispositionDefault {
			t.Fatalf("typed safety execution = %#v, want default dry_run", execution)
		}
	}
	tool.ExampleDispositions = []ManualAgentExampleDisposition{{
		Index:      &index,
		Mode:       ManualAgentExampleModeContractOnly,
		ReasonCode: ManualAgentExampleReasonStatefulPreflight,
		Reason:     "The command performs an authenticated stateful lookup before its dry-run branch.",
		Reviewed:   true,
	}}
	hints.Tools["sample.search_items"] = tool
	plan, err = BuildManualAgentExampleExecutionPlan(bound, registry, hints)
	if err != nil {
		t.Fatalf("high-risk plan with reviewed stateful preflight disposition error = %v", err)
	}
	if plan.DryRun != 1 || plan.ContractOnly != 1 || plan.ReviewedContractOnly != 1 {
		t.Fatalf("high-risk reviewed disposition plan = %#v", plan)
	}
}

func TestBuildManualAgentExampleExecutionPlanRejectsInvalidDisposition(t *testing.T) {
	_, leaf := manualSchemaHintTestTree()
	boundSpec := BoundCommandSpec{
		CommandSpec:    CommandSpec{CanonicalPath: "sample.search_items", PrimaryCLIPath: "sample item search"},
		PrimaryCommand: leaf,
	}
	bound := BoundCommandRegistry{
		Commands:    []BoundCommandSpec{boundSpec},
		ByCanonical: map[string]BoundCommandSpec{"sample.search_items": boundSpec},
		ByCLIPath:   map[string]BoundCommandSpec{"sample item search": boundSpec},
	}
	zero, one := 0, 1
	tests := []struct {
		name         string
		dispositions []ManualAgentExampleDisposition
		wantErr      string
	}{
		{name: "missing index", dispositions: []ManualAgentExampleDisposition{{Mode: ManualAgentExampleModeContractOnly, ReasonCode: ManualAgentExampleReasonLocalState, Reason: "Requires local state", Reviewed: true}}, wantErr: "requires index"},
		{name: "out of range", dispositions: []ManualAgentExampleDisposition{{Index: &one, Mode: ManualAgentExampleModeContractOnly, ReasonCode: ManualAgentExampleReasonLocalState, Reason: "Requires local state", Reviewed: true}}, wantErr: "out of range"},
		{name: "duplicate", dispositions: []ManualAgentExampleDisposition{{Index: &zero, Mode: ManualAgentExampleModeContractOnly, ReasonCode: ManualAgentExampleReasonLocalState, Reason: "Requires local state", Reviewed: true}, {Index: &zero, Mode: ManualAgentExampleModeContractOnly, ReasonCode: ManualAgentExampleReasonLocalState, Reason: "Requires local state", Reviewed: true}}, wantErr: "duplicate"},
		{name: "not reviewed", dispositions: []ManualAgentExampleDisposition{{Index: &zero, Mode: ManualAgentExampleModeContractOnly, ReasonCode: ManualAgentExampleReasonLocalState, Reason: "Requires local state"}}, wantErr: "must be reviewed"},
		{name: "stores default", dispositions: []ManualAgentExampleDisposition{{Index: &zero, Mode: ManualAgentExampleModeDryRun, ReasonCode: ManualAgentExampleReasonLocalState, Reason: "Requires local state", Reviewed: true}}, wantErr: "invalid mode"},
		{name: "unknown mode", dispositions: []ManualAgentExampleDisposition{{Index: &zero, Mode: "skip", ReasonCode: ManualAgentExampleReasonLocalState, Reason: "Requires local state", Reviewed: true}}, wantErr: "invalid mode"},
		{name: "unknown reason code", dispositions: []ManualAgentExampleDisposition{{Index: &zero, Mode: ManualAgentExampleModeContractOnly, ReasonCode: "safe", Reason: "Skip a safe example", Reviewed: true}}, wantErr: "invalid reason_code"},
		{name: "empty reason", dispositions: []ManualAgentExampleDisposition{{Index: &zero, Mode: ManualAgentExampleModeContractOnly, ReasonCode: ManualAgentExampleReasonLocalState, Reviewed: true}}, wantErr: "non-empty reason"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			hints := manualAgentHintSetFixture()
			tool := hints.Tools["sample.search_items"]
			tool.ExampleDispositions = test.dispositions
			hints.Tools["sample.search_items"] = tool
			err := ValidateManualAgentHintExamples(bound, hints)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func TestManualAgentExampleContractRequiresCobraFlagsAndPositionalsEvenWhenContractOnly(t *testing.T) {
	_, leaf := manualSchemaHintTestTree()
	if err := leaf.MarkFlagRequired("query"); err != nil {
		t.Fatalf("MarkFlagRequired(query): %v", err)
	}
	leaf.Args = cobra.ExactArgs(1)
	boundSpec := BoundCommandSpec{
		CommandSpec:    CommandSpec{CanonicalPath: "sample.search_items", PrimaryCLIPath: "sample item search"},
		PrimaryCommand: leaf,
	}
	bound := BoundCommandRegistry{
		Commands:    []BoundCommandSpec{boundSpec},
		ByCanonical: map[string]BoundCommandSpec{"sample.search_items": boundSpec},
		ByCLIPath:   map[string]BoundCommandSpec{"sample item search": boundSpec},
	}
	index := 0
	hints := manualAgentHintSetFixture()
	tool := hints.Tools["sample.search_items"]
	tool.ExampleDispositions = []ManualAgentExampleDisposition{{
		Index:      &index,
		Mode:       ManualAgentExampleModeContractOnly,
		ReasonCode: ManualAgentExampleReasonStatefulPreflight,
		Reason:     "Reviewed interactive precondition",
		Reviewed:   true,
	}}

	tool.Examples = []string{"dws sample item search item-id"}
	hints.Tools["sample.search_items"] = tool
	if err := ValidateManualAgentHintExamples(bound, hints); err == nil || !strings.Contains(err.Error(), "missing required flag") {
		t.Fatalf("missing required flag error = %v", err)
	}

	tool.Examples = []string{"dws sample item search --query value"}
	hints.Tools["sample.search_items"] = tool
	if err := ValidateManualAgentHintExamples(bound, hints); err == nil || !strings.Contains(err.Error(), "invalid positional arguments") {
		t.Fatalf("missing positional error = %v", err)
	}

	tool.Examples = []string{"dws sample item search --query value item-id"}
	hints.Tools["sample.search_items"] = tool
	if err := ValidateManualAgentHintExamples(bound, hints); err != nil {
		t.Fatalf("valid contract-only example error = %v", err)
	}
}

func TestManualAgentExampleContractEnforcesTypedFlagGroupsBeforeDisposition(t *testing.T) {
	_, leaf := manualSchemaHintTestTree()
	leaf.Flags().String("robot-client-id", "", "robot client ID")
	leaf.Flags().String("unified-app-id", "", "unified app ID")
	leaf.Flags().String("start", "", "start value")
	leaf.Flags().String("end", "", "end value")
	leaf.Flags().String("brief", "", "brief output")
	leaf.Flags().String("verbose-output", "", "verbose output")
	AnnotateRuntimeConstraints(leaf, RuntimeSchemaConstraints{
		RequireOneOf:      [][]string{{"robot-client-id", "unified-app-id"}},
		RequireTogether:   [][]string{{"start", "end"}},
		MutuallyExclusive: [][]string{{"brief", "verbose-output"}},
	})
	boundSpec := BoundCommandSpec{
		CommandSpec:    CommandSpec{CanonicalPath: "sample.search_items", PrimaryCLIPath: "sample item search"},
		PrimaryCommand: leaf,
	}
	bound := BoundCommandRegistry{
		Commands:    []BoundCommandSpec{boundSpec},
		ByCanonical: map[string]BoundCommandSpec{"sample.search_items": boundSpec},
		ByCLIPath:   map[string]BoundCommandSpec{"sample item search": boundSpec},
	}
	index := 0
	hints := manualAgentHintSetFixture()
	tool := hints.Tools["sample.search_items"]
	tool.ExampleDispositions = []ManualAgentExampleDisposition{{
		Index:      &index,
		Mode:       ManualAgentExampleModeContractOnly,
		ReasonCode: ManualAgentExampleReasonStatefulPreflight,
		Reason:     "A reviewed local connection is required after argument validation.",
		Reviewed:   true,
	}}

	tests := []struct {
		name    string
		example string
		wantErr string
	}{
		{name: "require one of", example: "dws sample item search --query value", wantErr: "missing require_one_of"},
		{name: "require together", example: "dws sample item search --query value --robot-client-id robot --start now", wantErr: "incomplete require_together"},
		{name: "mutually exclusive", example: "dws sample item search --query value --robot-client-id robot --brief yes --verbose-output yes", wantErr: "mutually_exclusive"},
		{name: "valid", example: "dws sample item search --query value --unified-app-id app --start now --end later --brief yes"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tool.Examples = []string{test.example}
			hints.Tools["sample.search_items"] = tool
			err := ValidateManualAgentHintExamples(bound, hints)
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateManualAgentHintExamples() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func TestManualAgentExampleExecutionPlanEnforcesFinalTypedConstraints(t *testing.T) {
	_, leaf := manualSchemaHintTestTree()
	leaf.Flags().String("robot-client-id", "", "robot client ID")
	leaf.Flags().String("unified-app-id", "", "unified app ID")
	boundSpec := BoundCommandSpec{
		CommandSpec:    CommandSpec{CanonicalPath: "sample.search_items", PrimaryCLIPath: "sample item search"},
		PrimaryCommand: leaf,
	}
	bound := BoundCommandRegistry{
		Commands:    []BoundCommandSpec{boundSpec},
		ByCanonical: map[string]BoundCommandSpec{"sample.search_items": boundSpec},
		ByCLIPath:   map[string]BoundCommandSpec{"sample item search": boundSpec},
	}
	hints := manualAgentHintSetFixture()
	registry := manualAgentExampleSchemaRegistry("sample.search_items", SafetySpec{})
	registry.Products[0].Tools[0].Constraints = RuntimeSchemaConstraints{
		RequireOneOf: [][]string{{"robot-client-id", "unified-app-id"}},
	}

	if _, err := BuildManualAgentExampleExecutionPlan(bound, registry, hints); err == nil || !strings.Contains(err.Error(), "missing require_one_of") {
		t.Fatalf("final typed constraint error = %v", err)
	}
	tool := hints.Tools["sample.search_items"]
	tool.Examples = []string{"dws sample item search --query value --unified-app-id app"}
	hints.Tools["sample.search_items"] = tool
	if _, err := BuildManualAgentExampleExecutionPlan(bound, registry, hints); err != nil {
		t.Fatalf("valid final typed constraint error = %v", err)
	}
}

func TestManualAgentExampleExecutionPlanRejectsManualConfirmationClassification(t *testing.T) {
	_, leaf := manualSchemaHintTestTree()
	boundSpec := BoundCommandSpec{
		CommandSpec:    CommandSpec{CanonicalPath: "sample.search_items", PrimaryCLIPath: "sample item search"},
		PrimaryCommand: leaf,
	}
	bound := BoundCommandRegistry{
		Commands:    []BoundCommandSpec{boundSpec},
		ByCanonical: map[string]BoundCommandSpec{"sample.search_items": boundSpec},
		ByCLIPath:   map[string]BoundCommandSpec{"sample item search": boundSpec},
	}
	index := 0
	hints := manualAgentHintSetFixture()
	tool := hints.Tools["sample.search_items"]
	tool.ExampleDispositions = []ManualAgentExampleDisposition{{
		Index:      &index,
		Mode:       ManualAgentExampleModeContractOnly,
		ReasonCode: "confirmation_required",
		Reason:     "Do not author a typed safety fact manually.",
		Reviewed:   true,
	}}
	hints.Tools["sample.search_items"] = tool
	if err := ValidateManualAgentHintExamples(bound, hints); err == nil || !strings.Contains(err.Error(), "invalid reason_code") {
		t.Fatalf("manual confirmation disposition error = %v", err)
	}
}

func manualAgentExampleSchemaRegistry(canonical string, safety SafetySpec) SchemaRegistry {
	return SchemaRegistry{Products: []ProductSpec{{
		ID: "sample",
		Tools: []ToolSpec{{
			Identity: ToolIdentitySpec{CanonicalPath: canonical},
			Safety:   safety,
		}},
	}}}
}

func manualAgentHintSetFixture() ManualAgentHintSet {
	const revision = "ai-agent-contract-v1"
	return ManualAgentHintSet{
		Revisions: map[string]ManualAgentHintRevision{
			revision: {
				GeneratedBy:   "ai",
				Model:         "gpt-5",
				PromptVersion: "v1",
				Reason:        "Generate the reviewed fixture",
			},
		},
		Products: map[string]ManualAgentProductHint{
			"sample": {
				AgentSummary: "Manage sample items",
				UseWhen:      []string{"A sample item must be managed"},
				AvoidWhen:    []string{"The target is not a sample item"},
				Reviewed:     true,
				Revision:     revision,
				Reason:       "Reviewed product routing",
				Evidence:     []string{"sample product reference"},
			},
		},
		Tools: map[string]ManualAgentToolHint{
			"sample.search_items": {
				AgentSummary: "Search sample items",
				UseWhen:      []string{"Existing sample items must be found"},
				AvoidWhen:    []string{"A sample item must be created"},
				Examples:     []string{"dws sample item search --query value"},
				Reviewed:     true,
				Revision:     revision,
				Reason:       "Reviewed tool selection",
				Evidence:     []string{"dws sample item search --help"},
			},
		},
	}
}

func manualSchemaHintTestTree() (*cobra.Command, *cobra.Command) {
	root := &cobra.Command{Use: "dws"}
	product := &cobra.Command{Use: "sample"}
	group := &cobra.Command{Use: "item"}
	leaf := &cobra.Command{Use: "search", RunE: func(*cobra.Command, []string) error { return nil }}
	leaf.Flags().StringP("query", "q", "", "Original query text")
	group.AddCommand(leaf)
	product.AddCommand(group)
	root.AddCommand(product)
	return root, leaf
}

func boolPointer(value bool) *bool {
	return &value
}

func stringPointer(value string) *string {
	return &value
}
