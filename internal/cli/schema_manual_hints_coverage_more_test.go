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
)

func TestCrossPlatformCoverageManualSchemaHintDecodeAndValidationEdges(t *testing.T) {
	valid := ManualSchemaHintSnapshot{
		Schema:     manualSchemaHintSchemaRef,
		Version:    manualSchemaHintVersion,
		AgentHints: manualAgentHintSetFixture(),
	}
	encoded, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot, err := DecodeManualSchemaHintSource(encoded); err != nil || snapshot.Version != manualSchemaHintVersion {
		t.Fatalf("DecodeManualSchemaHintSource() = %#v, %v", snapshot, err)
	}

	decodeCases := []struct {
		name string
		data []byte
		want string
	}{
		{name: "malformed", data: []byte("{"), want: "decode manual Schema hints"},
		{name: "second malformed value", data: append(append([]byte{}, encoded...), []byte(" {")...), want: "decode manual Schema hints"},
		{name: "multiple values", data: append(append([]byte{}, encoded...), []byte(" {}")...), want: "multiple JSON values"},
		{name: "version", data: []byte(`{"$schema":"./schema_manual_hints.schema.json","version":2}`), want: "unsupported"},
	}
	for _, tc := range decodeCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeManualSchemaHintSource(tc.data)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}

	validationCases := []struct {
		name   string
		mutate func(*ManualAgentHintSet)
		want   string
	}{
		{name: "blank revision key", mutate: func(h *ManualAgentHintSet) {
			h.Revisions[" "] = h.Revisions["ai-agent-contract-v1"]
			delete(h.Revisions, "ai-agent-contract-v1")
		}, want: "invalid revision key"},
		{name: "revision provenance", mutate: func(h *ManualAgentHintSet) {
			h.Revisions["ai-agent-contract-v1"] = ManualAgentHintRevision{}
		}, want: "requires generated_by and reason"},
		{name: "ai provenance", mutate: func(h *ManualAgentHintSet) {
			h.Revisions["ai-agent-contract-v1"] = ManualAgentHintRevision{GeneratedBy: "AI", Reason: "r"}
		}, want: "requires model and prompt_version"},
		{name: "product key", mutate: func(h *ManualAgentHintSet) {
			h.Products["bad.product"] = h.Products["sample"]
			delete(h.Products, "sample")
		}, want: "invalid product key"},
		{name: "product fields", mutate: func(h *ManualAgentHintSet) {
			item := h.Products["sample"]
			item.UseWhen = nil
			h.Products["sample"] = item
		}, want: "requires non-empty use_when"},
		{name: "trimmed tool key", mutate: func(h *ManualAgentHintSet) {
			h.Tools[" sample.search_items"] = h.Tools["sample.search_items"]
			delete(h.Tools, "sample.search_items")
		}, want: "invalid canonical tool key"},
		{name: "invalid tool key", mutate: func(h *ManualAgentHintSet) {
			h.Tools["sample"] = h.Tools["sample.search_items"]
			delete(h.Tools, "sample.search_items")
		}, want: "invalid canonical tool key"},
		{name: "tool fields", mutate: func(h *ManualAgentHintSet) {
			item := h.Tools["sample.search_items"]
			item.AgentSummary = ""
			h.Tools["sample.search_items"] = item
		}, want: "requires agent_summary and reason"},
		{name: "too many examples", mutate: func(h *ManualAgentHintSet) {
			item := h.Tools["sample.search_items"]
			item.Examples = []string{"dws a", "dws b", "dws c"}
			h.Tools["sample.search_items"] = item
		}, want: "maximum is 2"},
		{name: "invalid example syntax", mutate: func(h *ManualAgentHintSet) {
			item := h.Tools["sample.search_items"]
			item.Examples = []string{"dws sample | bad"}
			h.Tools["sample.search_items"] = item
		}, want: "invalid argv syntax"},
		{name: "example prefix", mutate: func(h *ManualAgentHintSet) {
			item := h.Tools["sample.search_items"]
			item.Examples = []string{"other command"}
			h.Tools["sample.search_items"] = item
		}, want: "must start with dws"},
		{name: "help example", mutate: func(h *ManualAgentHintSet) {
			item := h.Tools["sample.search_items"]
			item.Examples = []string{"dws sample --help=true"}
			h.Tools["sample.search_items"] = item
		}, want: "must demonstrate execution"},
		{name: "empty example", mutate: func(h *ManualAgentHintSet) {
			item := h.Tools["sample.search_items"]
			item.Examples = []string{" "}
			h.Tools["sample.search_items"] = item
		}, want: "empty examples entry"},
		{name: "invalid disposition", mutate: func(h *ManualAgentHintSet) {
			item := h.Tools["sample.search_items"]
			item.ExampleDispositions = []ManualAgentExampleDisposition{{}}
			h.Tools["sample.search_items"] = item
		}, want: "requires index"},
		{name: "unexpected product", mutate: func(h *ManualAgentHintSet) {}, want: "unexpected=[sample]"},
	}
	for _, tc := range validationCases {
		t.Run(tc.name, func(t *testing.T) {
			hints := manualAgentHintSetFixture()
			tc.mutate(&hints)
			expectedProducts := map[string]bool{"sample": true}
			if tc.name == "unexpected product" {
				expectedProducts = map[string]bool{}
			}
			err := ValidateManualAgentHintSet(hints, expectedProducts, map[string]bool{"sample.search_items": true})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}

	if err := validateNonEmptyManualAgentStrings("scope", "field", []string{"x", " x "}); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate error = %v", err)
	}
	if err := validateNonEmptyManualAgentStrings("scope", "field", []string{" "}); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("empty error = %v", err)
	}
	if !validManualAgentExampleReasonCode(ManualAgentExampleReasonLocalState) || validManualAgentExampleReasonCode("other") {
		t.Fatal("reason code validation disagrees with closed taxonomy")
	}
}

func TestCrossPlatformCoverageManualAgentExamplePlanAndParserEdges(t *testing.T) {
	_, leaf := manualSchemaHintTestTree()
	spec := BoundCommandSpec{
		CommandSpec:    CommandSpec{CanonicalPath: "sample.search_items", PrimaryCLIPath: "sample item search"},
		PrimaryCommand: leaf,
	}
	bound := BoundCommandRegistry{Commands: []BoundCommandSpec{spec}, ByCanonical: map[string]BoundCommandSpec{"sample.search_items": spec}}
	hints := manualAgentHintSetFixture()

	registries := []struct {
		name     string
		registry SchemaRegistry
		want     string
	}{
		{name: "empty canonical", registry: manualAgentExampleSchemaRegistry("", SafetySpec{}), want: "empty canonical path"},
		{name: "duplicate canonical", registry: SchemaRegistry{Products: []ProductSpec{{Tools: []ToolSpec{{Identity: ToolIdentitySpec{CanonicalPath: "sample.search_items"}}, {Identity: ToolIdentitySpec{CanonicalPath: "sample.search_items"}}}}}}, want: "duplicate tool"},
		{name: "missing typed tool", registry: SchemaRegistry{}, want: "missing from final typed"},
	}
	for _, tc := range registries {
		t.Run(tc.name, func(t *testing.T) {
			_, err := BuildManualAgentExampleExecutionPlan(bound, tc.registry, hints)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}

	unknownHints := manualAgentHintSetFixture()
	unknownHints.Tools["other.tool"] = unknownHints.Tools["sample.search_items"]
	delete(unknownHints.Tools, "sample.search_items")
	if _, err := buildManualAgentExampleExecutionPlan(bound, nil, unknownHints); err == nil || !strings.Contains(err.Error(), "unknown canonical") {
		t.Fatalf("unknown canonical error = %v", err)
	}

	nilSpec := spec
	nilSpec.PrimaryCommand = nil
	nilBound := BoundCommandRegistry{ByCanonical: map[string]BoundCommandSpec{"sample.search_items": nilSpec}}
	if _, err := buildManualAgentExampleExecutionPlan(nilBound, nil, hints); err == nil || !strings.Contains(err.Error(), "no bound Cobra command") {
		t.Fatalf("nil command error = %v", err)
	}

	leaf.Annotations = map[string]string{runtimeSchemaRulesAnnotation: "{"}
	if _, err := buildManualAgentExampleExecutionPlan(bound, nil, hints); err == nil || !strings.Contains(err.Error(), "invalid executable constraints") {
		t.Fatalf("constraints error = %v", err)
	}
	leaf.Annotations = map[string]string{runtimeSchemaArgsAnnotation: "{"}
	if _, err := buildManualAgentExampleExecutionPlan(bound, nil, hints); err == nil || !strings.Contains(err.Error(), "invalid executable positionals") {
		t.Fatalf("positionals error = %v", err)
	}
	leaf.Annotations = nil

	index := 0
	tool := hints.Tools["sample.search_items"]
	tool.ExampleDispositions = []ManualAgentExampleDisposition{{Index: &index, Mode: ManualAgentExampleModeContractOnly, ReasonCode: ManualAgentExampleReasonLocalState, Reason: "local", Reviewed: true}}
	hints.Tools["sample.search_items"] = tool
	if _, err := BuildManualAgentExampleExecutionPlan(bound, manualAgentExampleSchemaRegistry("sample.search_items", SafetySpec{}), hints); err == nil || !strings.Contains(err.Error(), "narrows no explicit") {
		t.Fatalf("disposition error = %v", err)
	}

	hints = manualAgentHintSetFixture()
	extraRegistry := manualAgentExampleSchemaRegistry("sample.search_items", SafetySpec{})
	extraRegistry.Products[0].Tools = append(extraRegistry.Products[0].Tools, ToolSpec{Identity: ToolIdentitySpec{CanonicalPath: "sample.extra"}})
	if _, err := BuildManualAgentExampleExecutionPlan(bound, extraRegistry, hints); err == nil || !strings.Contains(err.Error(), "has no Manual Agent hint examples") {
		t.Fatalf("extra typed tool error = %v", err)
	}

	parserCases := []struct {
		input string
		want  string
	}{
		{input: `dws x "value\`, want: "trailing escape in double-quoted"},
		{input: `dws x "${value}"`, want: "shell expansion"},
		{input: `dws x value\`, want: "trailing escape"},
		{input: `dws x <bad value>`, want: "redirection"},
		{input: `dws x # comment`, want: "comments"},
	}
	for _, tc := range parserCases {
		if _, err := ParseManualAgentExampleArgv(tc.input); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("ParseManualAgentExampleArgv(%q) error = %v, want %q", tc.input, err, tc.want)
		}
	}
	argv, err := ParseManualAgentExampleArgv(`dws x "a b" 'c d' escaped\ value <id>`)
	if err != nil || !reflect.DeepEqual(argv, []string{"dws", "x", "a b", "c d", "escaped value", "<id>"}) {
		t.Fatalf("parsed argv = %#v, %v", argv, err)
	}

	paths := []manualAgentExamplePath{{Path: "sample item search"}, {Path: "sample item deeply search"}}
	if remainder, matched, ok := matchManualAgentExamplePath([]string{"dws", "sample", "item", "deeply", "search", "x"}, paths); !ok || matched.Path != "sample item deeply search" || !reflect.DeepEqual(remainder, []string{"x"}) {
		t.Fatalf("match = %#v %#v %v", remainder, matched, ok)
	}
	if _, _, ok := matchManualAgentExamplePath(nil, paths); ok {
		t.Fatal("empty argv matched")
	}
	if _, _, ok := matchManualAgentExamplePath([]string{"other", "x"}, paths); ok {
		t.Fatal("non-dws argv matched")
	}
	if _, _, ok := matchManualAgentExamplePath([]string{"dws", "x"}, []manualAgentExamplePath{{Path: " "}, {Path: "x y"}}); ok {
		t.Fatal("empty or overlong path matched")
	}
}

func TestCrossPlatformCoverageManualAgentExampleCobraContractEdges(t *testing.T) {
	root, leaf := manualSchemaHintTestTree()
	root.PersistentFlags().StringP("format", "f", "", "format")
	leaf.Flags().BoolP("verbose", "v", false, "verbose")

	cases := []struct {
		name string
		cmd  *cobra.Command
		args []string
		pos  []RuntimeSchemaPositional
		want string
	}{
		{name: "nil command", want: "command is nil"},
		{name: "terminator", cmd: leaf, args: []string{"--"}, want: "terminator"},
		{name: "empty long", cmd: leaf, args: []string{"--=x"}, want: "empty long"},
		{name: "long help", cmd: leaf, args: []string{"--help=true"}, want: "not only --help"},
		{name: "unknown long", cmd: leaf, args: []string{"--missing=x"}, want: "unknown flag"},
		{name: "missing long value", cmd: leaf, args: []string{"--query"}, want: "requires a value"},
		{name: "non ascii shorthand", cmd: leaf, args: []string{"-é"}, want: "non-ASCII"},
		{name: "short help", cmd: leaf, args: []string{"-h"}, want: "not only -h"},
		{name: "unknown shorthand", cmd: leaf, args: []string{"-x"}, want: "unknown shorthand"},
		{name: "missing shorthand value", cmd: leaf, args: []string{"-q"}, want: "requires a value"},
		{name: "required positional", cmd: leaf, pos: []RuntimeSchemaPositional{{Index: 0, Name: "id", Required: true}}, want: "missing required positional"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateManualAgentExampleCobraContract(tc.cmd, tc.args, RuntimeSchemaConstraints{}, tc.pos)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}

	query := leaf.Flags().Lookup("query")
	query.Annotations = map[string][]string{cobra.BashCompOneRequiredFlag: {"true"}}
	if err := validateManualAgentExampleCobraContract(leaf, nil, RuntimeSchemaConstraints{}, nil); err == nil || !strings.Contains(err.Error(), "missing required flag") {
		t.Fatalf("required flag error = %v", err)
	}
	query.Annotations = nil

	constraints := RuntimeSchemaConstraints{RequireOneOf: [][]string{{"id"}}}
	positionals := []RuntimeSchemaPositional{{Index: 0, Name: "id", Required: true, Variadic: true}}
	if err := validateManualAgentExampleCobraContract(leaf, []string{"value"}, constraints, positionals); err != nil {
		t.Fatalf("variadic positional contract = %v", err)
	}
	leaf.Args = cobra.NoArgs
	if err := validateManualAgentExampleCobraContract(leaf, []string{"value"}, RuntimeSchemaConstraints{}, nil); err == nil || !strings.Contains(err.Error(), "invalid positional") {
		t.Fatalf("Args validator error = %v", err)
	}
	leaf.Args = nil
	if err := validateManualAgentExampleCobraContract(leaf, []string{"-vf=json", "-qvalue"}, RuntimeSchemaConstraints{}, nil); err != nil {
		t.Fatalf("combined and persistent shorthand contract = %v", err)
	}

	merged := mergeManualAgentExamplePositionals(
		[]RuntimeSchemaPositional{{Index: 1, Name: "b"}, {Index: 0, Name: "z"}},
		[]RuntimeSchemaPositional{{Index: 0, Name: "a"}, {Index: 1, Name: "b"}},
	)
	if got := []string{merged[0].Name, merged[1].Name, merged[2].Name}; !reflect.DeepEqual(got, []string{"a", "z", "b"}) {
		t.Fatalf("merged positionals = %#v", merged)
	}
	if err := validateManualAgentExampleConstraints(map[string]bool{}, RuntimeSchemaConstraints{RequireOneOf: [][]string{{"a", ""}}}); err == nil {
		t.Fatal("missing require_one_of accepted")
	}
	if err := validateManualAgentExampleConstraints(map[string]bool{"a": true}, RuntimeSchemaConstraints{RequireTogether: [][]string{{"a", "b"}}}); err == nil {
		t.Fatal("incomplete require_together accepted")
	}
	if err := validateManualAgentExampleConstraints(map[string]bool{"a": true, "b": true}, RuntimeSchemaConstraints{MutuallyExclusive: [][]string{{"a", "b"}}}); err == nil {
		t.Fatal("mutually exclusive flags accepted")
	}
	if got := manualAgentExampleProvidedFlagCount(map[string]bool{"a": true}, []string{"--a", "a", " "}); got != 1 {
		t.Fatalf("provided count = %d", got)
	}
	if got := manualAgentExampleFlagGroup([]string{" --b ", "", "a"}); got != "--a, --b" {
		t.Fatalf("flag group = %q", got)
	}
	if runtimeCommandFlagByShorthand(nil, "q") != nil || runtimeCommandFlagByShorthand(leaf, "qq") != nil {
		t.Fatal("invalid shorthand lookup succeeded")
	}
	if runtimeCommandFlagByShorthand(leaf, "q") != query || runtimeCommandFlagByShorthand(leaf, "f") == nil {
		t.Fatal("local or persistent shorthand lookup failed")
	}
}

func TestCrossPlatformCoverageApplyManualSchemaHintsAndAnnotationEdges(t *testing.T) {
	if _, err := applyManualSchemaHints(nil, ManualSchemaHintSnapshot{Version: manualSchemaHintVersion}); err == nil || !strings.Contains(err.Error(), "root is nil") {
		t.Fatalf("nil root error = %v", err)
	}
	root, leaf := manualSchemaHintTestTree()
	if _, err := applyManualSchemaHints(root, ManualSchemaHintSnapshot{Version: 99}); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("version error = %v", err)
	}

	base := ManualSchemaCommandHint{CLIPath: "sample item search", CanonicalPath: "sample.search_items", Reason: "reviewed", Reviewed: true}
	applyCases := []struct {
		name   string
		setup  func(*cobra.Command, *ManualSchemaCommandHint)
		params map[string]ManualSchemaParameterHint
		want   string
	}{
		{name: "duplicate", setup: func(*cobra.Command, *ManualSchemaCommandHint) {}, want: "duplicate"},
		{name: "non runnable", setup: func(cmd *cobra.Command, _ *ManualSchemaCommandHint) { cmd.RunE = nil }, want: "public runnable"},
		{name: "hidden", setup: func(cmd *cobra.Command, _ *ManualSchemaCommandHint) { cmd.Parent().Hidden = true }, want: "public runnable"},
		{name: "empty flag", params: map[string]ManualSchemaParameterHint{" ": {Required: boolPointer(true)}}, want: "empty flag name"},
		{name: "no overrides", params: map[string]ManualSchemaParameterHint{"query": {}}, want: "no Schema overrides"},
		{name: "empty description", params: map[string]ManualSchemaParameterHint{"query": {Description: stringPointer(" ")}}, want: "empty description"},
		{name: "empty property", params: map[string]ManualSchemaParameterHint{"query": {Property: stringPointer(" ")}}, want: "empty property"},
		{name: "empty required when", params: map[string]ManualSchemaParameterHint{"query": {RequiredWhen: stringPointer(" ")}}, want: "empty required_when"},
	}
	for _, tc := range applyCases {
		t.Run(tc.name, func(t *testing.T) {
			candidateRoot, candidateLeaf := manualSchemaHintTestTree()
			hint := base
			hint.Parameters = tc.params
			if tc.setup != nil {
				tc.setup(candidateLeaf, &hint)
			}
			commands := []ManualSchemaCommandHint{hint}
			if tc.name == "duplicate" {
				commands = append(commands, hint)
			}
			_, err := applyManualSchemaHints(candidateRoot, ManualSchemaHintSnapshot{Version: manualSchemaHintVersion, Commands: commands})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}

	previousRegistryLoader := loadManualCommandRegistry
	loadManualCommandRegistry = func() (CommandRegistry, error) { return CommandRegistry{}, errors.New("registry failed") }
	if _, err := applyManualSchemaHints(root, ManualSchemaHintSnapshot{Version: manualSchemaHintVersion}); err == nil || !strings.Contains(err.Error(), "registry failed") {
		t.Fatalf("registry loader error = %v", err)
	}
	loadManualCommandRegistry = previousRegistryLoader

	loadManualCommandRegistry = func() (CommandRegistry, error) {
		return CommandRegistry{ByCLIPath: map[string]CommandSpec{"sample item search": {CanonicalPath: "sample.other"}}}, nil
	}
	if _, err := applyManualSchemaHints(root, ManualSchemaHintSnapshot{Version: manualSchemaHintVersion, Commands: []ManualSchemaCommandHint{base}}); err == nil || !strings.Contains(err.Error(), "reviewed CommandRegistry canonical path") {
		t.Fatalf("reviewed registry conflict = %v", err)
	}
	loadManualCommandRegistry = previousRegistryLoader

	aliasRoot, aliasLeaf := manualSchemaHintTestTree()
	aliasLeaf.Aliases = []string{"find"}
	aliasHint := base
	aliasHint.CLIPath = "sample item find"
	if _, err := applyManualSchemaHints(aliasRoot, ManualSchemaHintSnapshot{Version: manualSchemaHintVersion, Commands: []ManualSchemaCommandHint{aliasHint}}); err == nil || !strings.Contains(err.Error(), "not present in reviewed CommandRegistry") {
		t.Fatalf("missing alias registry path = %v", err)
	}
	loadManualCommandRegistry = func() (CommandRegistry, error) {
		return CommandRegistry{ByCLIPath: map[string]CommandSpec{"sample item search": {CanonicalPath: "sample.other"}}}, nil
	}
	if _, err := applyManualSchemaHints(aliasRoot, ManualSchemaHintSnapshot{Version: manualSchemaHintVersion, Commands: []ManualSchemaCommandHint{aliasHint}}); err == nil || !strings.Contains(err.Error(), "conflicts with real command path") {
		t.Fatalf("alias registry conflict = %v", err)
	}
	loadManualCommandRegistry = previousRegistryLoader

	previousApply := applyManualParameter
	applyManualParameter = func(*cobra.Command, string, ManualSchemaParameterHint, string) error {
		return errors.New("annotation failed")
	}
	hint := base
	hint.Parameters = map[string]ManualSchemaParameterHint{"query": {Required: boolPointer(true)}}
	if _, err := applyManualSchemaHints(root, ManualSchemaHintSnapshot{Version: manualSchemaHintVersion, Commands: []ManualSchemaCommandHint{hint}}); err == nil || !strings.Contains(err.Error(), "annotation failed") {
		t.Fatalf("annotation error = %v", err)
	}
	applyManualParameter = previousApply

	annotateManualSchemaIdentity(nil, "x.y", "reason")
	if _, _, _, ok := runtimeManualSchemaIdentity(nil); ok {
		t.Fatal("nil command has identity")
	}
	if _, _, _, ok := runtimeManualSchemaIdentity(&cobra.Command{Annotations: map[string]string{runtimeSchemaManualIdentityAnnotation: "invalid"}}); ok {
		t.Fatal("invalid identity accepted")
	}
	if err := annotateManualSchemaParameter(nil, "x", ManualSchemaParameterHint{}, ""); err == nil {
		t.Fatal("nil annotation command accepted")
	}
	if err := annotateManualSchemaParameter(leaf, "missing", ManualSchemaParameterHint{}, ""); err == nil {
		t.Fatal("missing annotation flag accepted")
	}
	previousMarshal := marshalManualParameter
	marshalManualParameter = func(any) ([]byte, error) { return nil, errors.New("marshal failed") }
	if err := annotateManualSchemaParameter(leaf, "query", ManualSchemaParameterHint{}, ""); err == nil || !strings.Contains(err.Error(), "marshal failed") {
		t.Fatalf("marshal error = %v", err)
	}
	marshalManualParameter = previousMarshal

	bad := &cobra.Command{Annotations: map[string]string{runtimeManualSchemaParameterKey(runtimeSchemaManualParameterAnnotation, "query"): "{"}}
	if _, _, _, err := runtimeManualSchemaParameter(bad, "query"); err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("decode parameter error = %v", err)
	}
	if hint, reason, ok, err := runtimeManualSchemaParameter(nil, "query"); err != nil || ok || reason != "" || !reflect.DeepEqual(hint, ManualSchemaParameterHint{}) {
		t.Fatalf("nil parameter = %#v %q %v %v", hint, reason, ok, err)
	}
	if hint, _, ok, err := runtimeManualSchemaParameter(&cobra.Command{Annotations: map[string]string{}}, "query"); err != nil || ok || !reflect.DeepEqual(hint, ManualSchemaParameterHint{}) {
		t.Fatalf("missing parameter = %#v %v %v", hint, ok, err)
	}

	for _, value := range []string{"string", "array", "object", "integer", "number", "boolean"} {
		if !supportedManualSchemaInterfaceType(" " + value + " ") {
			t.Fatalf("interface type %q rejected", value)
		}
	}
	if supportedManualSchemaInterfaceType("unknown") {
		t.Fatal("unknown interface type accepted")
	}
	if value := trimmedManualSchemaString(nil); value != nil {
		t.Fatalf("trimmed nil = %#v", value)
	}
	if product, tool, ok := splitManualSchemaCanonicalPath(" product.tool "); !ok || product != "product" || tool != "tool" {
		t.Fatalf("canonical split = %q %q %v", product, tool, ok)
	}
	for _, path := range []string{"", "product", ".tool", "product.", "bad product.tool"} {
		if _, _, ok := splitManualSchemaCanonicalPath(path); ok {
			t.Fatalf("invalid canonical %q accepted", path)
		}
	}
	if publicRunnableSchemaLeaf(nil) || publicRunnableSchemaLeaf(&cobra.Command{Use: "x"}) {
		t.Fatal("non-runnable command accepted")
	}
	parent := &cobra.Command{Use: "parent"}
	parent.AddCommand(&cobra.Command{Use: "child", Run: func(*cobra.Command, []string) {}})
	if publicRunnableSchemaLeaf(parent) {
		t.Fatal("command with children accepted as leaf")
	}
}

func TestCrossPlatformCoverageEmbeddedManualSchemaHintWrapperErrors(t *testing.T) {
	previous := loadManualSchemaHints
	loadManualSchemaHints = func() (ManualSchemaHintSnapshot, error) {
		return ManualSchemaHintSnapshot{}, errors.New("hints failed")
	}
	t.Cleanup(func() { loadManualSchemaHints = previous })
	if _, err := ApplyEmbeddedManualSchemaHints(&cobra.Command{}); err == nil || !strings.Contains(err.Error(), "hints failed") {
		t.Fatalf("ApplyEmbeddedManualSchemaHints() error = %v", err)
	}
	if _, err := ValidateEmbeddedManualAgentExampleDelivery(BoundCommandRegistry{}, SchemaRegistry{}); err == nil || !strings.Contains(err.Error(), "hints failed") {
		t.Fatalf("ValidateEmbeddedManualAgentExampleDelivery() error = %v", err)
	}
}
