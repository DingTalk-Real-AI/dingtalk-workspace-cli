// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	"github.com/spf13/cobra"
)

func requirePanicCoverage(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	fn()
}

func TestCrossPlatformCoverageRuntimeCompatibilityAnnotationEdges(t *testing.T) {
	valid := RuntimeCompatibilityEquivalence{ID: "pair", Reason: "reviewed", Reviewed: true}
	requirePanicCoverage(t, func() { AnnotateRuntimeCompatibilityEquivalence(nil, &cobra.Command{}, valid) })
	requirePanicCoverage(t, func() {
		AnnotateRuntimeCompatibilityEquivalence(&cobra.Command{}, &cobra.Command{}, RuntimeCompatibilityEquivalence{})
	})
	primary := &cobra.Command{Use: "primary", Annotations: map[string]string{runtimeCompatibilityEquivalenceAnnotation: `{"id":"other","reason":"other","reviewed":true}`}}
	requirePanicCoverage(t, func() { AnnotateRuntimeCompatibilityEquivalence(primary, &cobra.Command{Use: "alias"}, valid) })

	cases := []struct {
		raw     string
		present bool
		wantErr bool
	}{
		{raw: "", present: true, wantErr: true},
		{raw: "{", present: true, wantErr: true},
		{raw: `{"id":"x","reason":"r","reviewed":true} {}`, present: true, wantErr: true},
		{raw: `{"id":" x","reason":"r","reviewed":true}`, present: true, wantErr: true},
		{raw: `{"id":"x","reason":"r","reviewed":true}`, present: true},
	}
	for _, tc := range cases {
		command := &cobra.Command{Annotations: map[string]string{runtimeCompatibilityEquivalenceAnnotation: tc.raw}}
		_, present, err := runtimeCompatibilityEquivalence(command)
		if present != tc.present || (err != nil) != tc.wantErr {
			t.Errorf("runtimeCompatibilityEquivalence(%q) = present:%v err:%v", tc.raw, present, err)
		}
	}
	if _, present, err := runtimeCompatibilityEquivalence(nil); present || err != nil {
		t.Fatalf("nil equivalence = %v %v", present, err)
	}
	if _, present, err := runtimeCompatibilityEquivalence(&cobra.Command{Annotations: map[string]string{}}); present || err != nil {
		t.Fatalf("missing equivalence = %v %v", present, err)
	}
}

func TestCrossPlatformCoverageBindEffectiveCommandRegistryWrapperEdges(t *testing.T) {
	oldValidate := bindValidateParameterBindings
	t.Cleanup(func() { bindValidateParameterBindings = oldValidate })
	if _, err := BindEffectiveCommandRegistry(nil, EffectiveCommandRegistry{}); err == nil || !strings.Contains(err.Error(), "root is nil") {
		t.Fatalf("nil root error = %v", err)
	}
	bindValidateParameterBindings = func() error { return errors.New("bindings failed") }
	if _, err := BindEffectiveCommandRegistry(&cobra.Command{Use: "dws"}, EffectiveCommandRegistry{}); err == nil || !strings.Contains(err.Error(), "bindings failed") {
		t.Fatalf("bindings error = %v", err)
	}
	bindValidateParameterBindings = oldValidate
	if _, err := BindEffectiveCommandRegistry(&cobra.Command{Use: "dws"}, EffectiveCommandRegistry{Commands: []CommandSpec{{CanonicalPath: "bad"}}}); err == nil || !strings.Contains(err.Error(), "bind effective") {
		t.Fatalf("invalid effective error = %v", err)
	}
}

func TestCrossPlatformCoverageCommandRegistryAliasBindingRemainingEdges(t *testing.T) {
	spec := CommandSpec{CanonicalPath: "sample.run", PrimaryCLIPath: "sample run"}
	root := commandRegistryTestRoot("sample run", "sample group")
	primary := exactSchemaCommand(root, "sample run")
	group := exactSchemaCommand(root, "sample group")
	group.Run = nil
	group.RunE = nil
	if _, err := bindCommandRegistryAlias(root, spec, primary, "sample missing"); err == nil || !strings.Contains(err.Error(), "stale alias") {
		t.Fatalf("stale alias error = %v", err)
	}
	if _, err := bindCommandRegistryAlias(root, spec, primary, "sample group"); err == nil || !strings.Contains(err.Error(), "not a runnable") {
		t.Fatalf("non-runnable alias error = %v", err)
	}
	if _, err := bindCommandRegistryAlias(root, spec, primary, "sample run"); err == nil || !strings.Contains(err.Error(), "duplicates primary") {
		t.Fatalf("duplicate primary alias error = %v", err)
	}

	conflictRoot := commandRegistryTestRoot("sample run", "sample alias")
	conflictPrimary := exactSchemaCommand(conflictRoot, "sample run")
	conflictAlias := exactSchemaCommand(conflictRoot, "sample alias")
	AttachRuntimeSchema(conflictAlias, "other", "run", "test")
	if _, err := bindCommandRegistryAlias(conflictRoot, spec, conflictPrimary, "sample alias"); err == nil || !strings.Contains(err.Error(), "native annotation") {
		t.Fatalf("annotation alias error = %v", err)
	}
}

func TestCrossPlatformCoverageCompatibilityLeafContractDependencyAndMetadataEdges(t *testing.T) {
	oldContracts := loadCompatibilityFlagContracts
	oldBindings := compatibilityParameterBindings
	t.Cleanup(func() {
		loadCompatibilityFlagContracts = oldContracts
		compatibilityParameterBindings = oldBindings
	})
	primary := &cobra.Command{Use: "primary", Run: func(*cobra.Command, []string) {}}
	alias := &cobra.Command{Use: "alias", Run: primary.Run}
	spec := CommandSpec{CanonicalPath: "sample.run", PrimaryCLIPath: "sample run"}
	loadCompatibilityFlagContracts = func(command *cobra.Command, _ string) (map[string]compatibilityFlagContract, error) {
		if command == alias {
			return nil, errors.New("alias flags failed")
		}
		return map[string]compatibilityFlagContract{}, nil
	}
	if _, err := compatibilityFlagContractProblems(spec.CanonicalPath, primary, alias); err == nil || !strings.Contains(err.Error(), "alias flags failed") {
		t.Fatalf("alias flag loader error = %v", err)
	}
	loadCompatibilityFlagContracts = func(*cobra.Command, string) (map[string]compatibilityFlagContract, error) {
		return nil, errors.New("flags failed")
	}
	if err := validateCompatibilityLeafContract(spec, primary, alias, "sample alias"); err == nil || !strings.Contains(err.Error(), "validate reviewed") {
		t.Fatalf("compatibility flag gate error = %v", err)
	}
	loadCompatibilityFlagContracts = oldContracts

	primary.Annotations = map[string]string{runtimeSchemaRulesAnnotation: "{"}
	if err := validateCompatibilityLeafContract(spec, primary, alias, "sample alias"); err == nil || !strings.Contains(err.Error(), "primary runtime constraints are invalid") {
		t.Fatalf("primary constraints error = %v", err)
	}
	primary.Annotations = nil
	alias.Annotations = map[string]string{runtimeSchemaRulesAnnotation: "{"}
	if err := validateCompatibilityLeafContract(spec, primary, alias, "sample alias"); err == nil || !strings.Contains(err.Error(), "compatibility runtime constraints are invalid") {
		t.Fatalf("alias constraints error = %v", err)
	}
	alias.Annotations = nil
	primary.Annotations = map[string]string{runtimeSchemaArgsAnnotation: "{"}
	if err := validateCompatibilityLeafContract(spec, primary, alias, "sample alias"); err == nil || !strings.Contains(err.Error(), "primary runtime positionals are invalid") {
		t.Fatalf("primary positionals error = %v", err)
	}
	primary.Annotations = nil
	alias.Annotations = map[string]string{runtimeSchemaArgsAnnotation: "{"}
	if err := validateCompatibilityLeafContract(spec, primary, alias, "sample alias"); err == nil || !strings.Contains(err.Error(), "compatibility runtime positionals are invalid") {
		t.Fatalf("alias positionals error = %v", err)
	}
	alias.Annotations = nil

	compatibilityParameterBindings = func() (schemaParameterBindingSnapshot, error) {
		return schemaParameterBindingSnapshot{}, errors.New("binding data failed")
	}
	if _, err := effectiveCompatibilityFlagContracts(primary, spec.CanonicalPath); err == nil || !strings.Contains(err.Error(), "binding data failed") {
		t.Fatalf("effective flag binding error = %v", err)
	}
	compatibilityParameterBindings = oldBindings
	if contracts, err := effectiveCompatibilityFlagContracts(nil, spec.CanonicalPath); err != nil || len(contracts) != 0 {
		t.Fatalf("nil flag contracts = %#v, %v", contracts, err)
	}

	command := &cobra.Command{Use: "command"}
	command.Flags().String("id", "", "id")
	flag := command.Flags().Lookup("id")
	flag.Annotations = map[string][]string{
		"keep": {"value"},
	}
	metadata := RuntimeSchemaParameterMetadata{
		Required:     []string{" id "},
		RequiredWhen: map[string]string{"id": "when"},
		Formats:      map[string]string{"id": "uuid"},
		Examples:     map[string]string{"id": "123"},
		Enums:        map[string][]string{"id": {"a", "b"}},
	}
	annotations := effectiveCompatibilityFlagAnnotations(flag, map[string]string{"id": " itemId "}, metadata)
	if annotations["keep"][0] != "value" || annotations[runtimeSchemaFlagBindingPropertyAnnotation][0] != "itemId" || len(annotations[runtimeSchemaFlagMetadataEnumAnnotation]) != 2 {
		t.Fatalf("effective annotations = %#v", annotations)
	}
	flag.Annotations = nil
	annotations = effectiveCompatibilityFlagAnnotations(flag, nil, RuntimeSchemaParameterMetadata{Enums: map[string][]string{"id": {"a"}}})
	if len(annotations[runtimeSchemaFlagMetadataEnumAnnotation]) != 1 {
		t.Fatalf("enum-only annotations = %#v", annotations)
	}
	if got := firstCompatibilityAnnotation(map[string][]string{"k": {" ", " v "}}, "k"); got != "v" {
		t.Fatalf("first annotation = %q", got)
	}
}

func TestCrossPlatformCoverageCompatibilityFlagContractDifferenceEdges(t *testing.T) {
	oldContracts := loadCompatibilityFlagContracts
	t.Cleanup(func() { loadCompatibilityFlagContracts = oldContracts })
	primaryContracts := map[string]compatibilityFlagContract{
		"primary-only": {},
		"shared":       {Type: "string", Default: "a", NoOptDefault: "a", Shorthand: "s", Deprecated: "old", ShorthandDeprecated: "short", Origin: "local", Annotations: map[string][]string{"a": {"b"}}},
	}
	aliasContracts := map[string]compatibilityFlagContract{
		"alias-only": {},
		"shared":     {Type: "string", Default: "a", NoOptDefault: "b", Shorthand: "x", Hidden: true, Deprecated: "new", ShorthandDeprecated: "new-short", Origin: "inherited", Required: compatibilityFlagRequiredContract{CobraRequired: true}, Annotations: map[string][]string{"c": {"d"}}},
	}
	primary := &cobra.Command{Use: "primary"}
	alias := &cobra.Command{Use: "alias"}
	loadCompatibilityFlagContracts = func(command *cobra.Command, _ string) (map[string]compatibilityFlagContract, error) {
		if command == primary {
			return primaryContracts, nil
		}
		return aliasContracts, nil
	}
	problems, err := compatibilityFlagContractProblems("sample.run", primary, alias)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(problems, ";")
	for _, want := range []string{"exists only", "missing from", "no-option", "shorthand differs", "hidden state", "deprecation behavior", "shorthand deprecation", "local/persistent", "required/required_when", "annotations differ"} {
		if !strings.Contains(joined, want) {
			t.Errorf("problems %q missing %q", joined, want)
		}
	}
}

func TestCrossPlatformCoverageCompatibilityHandlerAndArgsRemainingEdges(t *testing.T) {
	if problems := compatibilityHandlerContractProblems(nil, &cobra.Command{}); len(problems) != 1 {
		t.Fatalf("nil handler problems = %v", problems)
	}
	if compatibilityHandlerPointer(1) != 0 {
		t.Fatal("non-function handler has pointer")
	}
	if compatibilityHandlerPointer(nil) != 0 {
		t.Fatal("nil handler has pointer")
	}
	primary := &cobra.Command{Use: "primary", Run: func(*cobra.Command, []string) {}}
	alias := &cobra.Command{Use: "alias", Run: func(*cobra.Command, []string) {}}
	primary.Annotations = map[string]string{runtimeCompatibilityEquivalenceAnnotation: "{"}
	alias.Annotations = map[string]string{runtimeCompatibilityEquivalenceAnnotation: "{"}
	joined := strings.Join(compatibilityHandlerContractProblems(primary, alias), ";")
	if !strings.Contains(joined, "primary compatibility equivalence is invalid") || !strings.Contains(joined, "compatibility equivalence is invalid") {
		t.Fatalf("invalid review problems = %q", joined)
	}
	primary.Annotations = nil
	alias.Annotations = nil
	if joined := strings.Join(compatibilityHandlerContractProblems(primary, alias), ";"); !strings.Contains(joined, "require the same reviewed") || !strings.Contains(joined, "implementation differs") {
		t.Fatalf("missing review problems = %q", joined)
	}
	AnnotateRuntimeCompatibilityEquivalence(primary, alias, RuntimeCompatibilityEquivalence{ID: "one-sided", Reason: "r", Reviewed: true})
	delete(alias.Annotations, runtimeCompatibilityEquivalenceAnnotation)
	if joined := strings.Join(compatibilityHandlerContractProblems(primary, alias), ";"); !strings.Contains(joined, "must be present on both") {
		t.Fatalf("one-sided review problems = %q", joined)
	}
	delete(primary.Annotations, runtimeCompatibilityEquivalenceAnnotation)
	AnnotateRuntimeCompatibilityEquivalence(primary, alias, RuntimeCompatibilityEquivalence{ID: "pair", Reason: "r", Reviewed: true})
	alias.Annotations[runtimeCompatibilityEquivalenceAnnotation] = `{"id":"other","reason":"r","reviewed":true}`
	if joined := strings.Join(compatibilityHandlerContractProblems(primary, alias), ";"); !strings.Contains(joined, "equivalence differs") {
		t.Fatalf("different review problems = %q", joined)
	}

	if problems := compatibilityArgsContractProblems(nil, alias); len(problems) != 1 {
		t.Fatalf("nil args problems = %v", problems)
	}
	p := &cobra.Command{Use: "p [id]", DisableFlagParsing: true, TraverseChildren: true, ValidArgs: []string{"a"}, ArgAliases: []string{"x"}}
	a := &cobra.Command{Use: "a", ValidArgs: []string{"b"}, ArgAliases: []string{"y"}}
	p.FParseErrWhitelist.UnknownFlags = true
	joined = strings.Join(compatibilityArgsContractProblems(p, a), ";")
	for _, want := range []string{"DisableFlagParsing", "TraverseChildren", "positional Use", "ValidArgs", "ArgAliases"} {
		if !strings.Contains(joined, want) {
			t.Errorf("args problems %q missing %q", joined, want)
		}
	}
	p.Args = cobra.NoArgs
	if joined := strings.Join(compatibilityArgsContractProblems(p, a), ";"); !strings.Contains(joined, "presence differs") {
		t.Fatalf("args presence problems = %q", joined)
	}
	a.Args = cobra.MaximumNArgs(1)
	if joined := strings.Join(compatibilityArgsContractProblems(p, a), ";"); !strings.Contains(joined, "implementation differs") {
		t.Fatalf("args implementation problems = %q", joined)
	}
	shared := func(command *cobra.Command, _ []string) error {
		if command.Annotations["reject"] == "true" {
			return errors.New("reject")
		}
		return nil
	}
	p.Args, a.Args = shared, shared
	p.Annotations = map[string]string{}
	a.Annotations = map[string]string{"reject": "true"}
	if joined := strings.Join(compatibilityArgsContractProblems(p, a), ";"); !strings.Contains(joined, "Args behavior differs") {
		t.Fatalf("args behavior problems = %q", joined)
	}
	panicCommand := &cobra.Command{Args: func(*cobra.Command, []string) error { panic("boom") }}
	if got := runCompatibilityArgsValidator(panicCommand, nil); !strings.Contains(got, "panic") {
		t.Fatalf("panic validator result = %q", got)
	}
	if syntax := compatibilityUseArgsSyntax(&cobra.Command{Use: "run"}); syntax != "" {
		t.Fatalf("empty args syntax = %q", syntax)
	}
}

func TestCrossPlatformCoverageCompatibilityPathAndHelperRemainingEdges(t *testing.T) {
	if got := compatibilityJSON(func() {}); !strings.Contains(got, "0x") {
		t.Fatalf("compatibilityJSON(func) = %q", got)
	}
	positionals := []contract.RuntimeSchemaPositional{{Index: 0, Name: "z"}, {Index: 0, Name: "a"}, {Index: 1, Name: "x"}}
	command := &cobra.Command{Annotations: map[string]string{runtimeSchemaArgsAnnotation: compatibilityJSON(positionals)}}
	got, err := strictCompatibilityPositionals(command)
	if err != nil || got[0].Name != "a" || got[2].Index != 1 {
		t.Fatalf("strict positionals = %#v, %v", got, err)
	}

	if match, err := resolveExactCobraPath(nil, "x"); err != nil || match.Command != nil {
		t.Fatalf("nil root match = %#v, %v", match, err)
	}
	root := &cobra.Command{Use: "dws"}
	if match, err := resolveExactCobraPath(root, "dws"); err != nil || match.Command != nil {
		t.Fatalf("empty root path match = %#v, %v", match, err)
	}
	one := &cobra.Command{Use: "one", Aliases: []string{"alias"}, Run: func(*cobra.Command, []string) {}}
	two := &cobra.Command{Use: "two", Aliases: []string{"alias"}, Run: func(*cobra.Command, []string) {}}
	root.AddCommand(one, two)
	if _, err := resolveExactCobraPath(root, "alias"); err == nil || !strings.Contains(err.Error(), "alias segment") {
		t.Fatalf("ambiguous alias error = %v", err)
	}
	if match, err := resolveExactCobraPath(root, "missing"); err != nil || match.Command != nil {
		t.Fatalf("missing match = %#v, %v", match, err)
	}
	ambiguousRoot := &cobra.Command{Use: "dws"}
	ambiguousRoot.AddCommand(
		&cobra.Command{Use: "same", Run: func(*cobra.Command, []string) {}},
		&cobra.Command{Use: "same", Run: func(*cobra.Command, []string) {}},
	)
	if _, err := resolveExactCobraPath(ambiguousRoot, "same"); err == nil || !strings.Contains(err.Error(), "command segment") {
		t.Fatalf("ambiguous exact command error = %v", err)
	}
	commands := appendDistinctCobraCommand(nil, one)
	commands = appendDistinctCobraCommand(commands, one)
	if len(commands) != 1 {
		t.Fatalf("distinct commands = %#v", commands)
	}

	spec := CommandSpec{CanonicalPath: "sample.run", PrimaryCLIPath: "sample run"}
	if _, err := bindCommandRegistryPath(root, spec, "missing"); err == nil || !strings.Contains(err.Error(), "stale cli path") {
		t.Fatalf("stale path error = %v", err)
	}
	if _, err := bindCommandRegistryPath(ambiguousRoot, spec, "same"); err == nil || !strings.Contains(err.Error(), "primary path") {
		t.Fatalf("ambiguous primary path error = %v", err)
	}
	nonRunnableRoot := &cobra.Command{Use: "dws"}
	nonRunnableRoot.AddCommand(&cobra.Command{Use: "group"})
	if _, err := bindCommandRegistryPath(nonRunnableRoot, spec, "group"); err == nil || !strings.Contains(err.Error(), "not a runnable") {
		t.Fatalf("non-runnable primary error = %v", err)
	}
	if runnableSchemaLeaf(nil) {
		t.Fatal("nil command runnable")
	}
	group := &cobra.Command{Use: "group", Run: func(*cobra.Command, []string) {}}
	group.AddCommand(&cobra.Command{Use: "child", Run: func(*cobra.Command, []string) {}})
	if runnableSchemaLeaf(group) {
		t.Fatal("command with child is leaf")
	}
	if origin := compatibilityFlagOrigin(&cobra.Command{Use: "empty"}, "missing"); origin != "effective_unknown" {
		t.Fatalf("unknown flag origin = %q", origin)
	}

	incomplete := &cobra.Command{Annotations: map[string]string{runtimeSchemaProductAnnotation: "sample"}}
	if err := validateCommandRegistryAnnotation(incomplete, "sample run", spec); err == nil || !strings.Contains(err.Error(), "incomplete native") {
		t.Fatalf("incomplete annotation error = %v", err)
	}

	// ContractFinal without Identity fails closed once a Contract is declared.
	noIdentity := &cobra.Command{Use: "run"}
	contractfinal.RegisterRuntimeContractFinal(noIdentity, contract.ContractFinalPayload{Description: "x"})
	t.Cleanup(func() { ClearRuntimeContractFinalForTest(noIdentity) })
	if err := validateCommandRegistryAnnotation(noIdentity, "sample run", spec); err == nil || !strings.Contains(err.Error(), "without Identity") {
		t.Fatalf("missing Identity error = %v", err)
	}
	mismatch := &cobra.Command{Use: "run"}
	contractfinal.RegisterRuntimeContractFinal(mismatch, contract.ContractFinalPayload{
		Identity: &contract.ToolIdentitySpec{
			ProductID: "other", Name: "run", CanonicalPath: "other.run",
			CLIPath: "sample run", PrimaryCLIPath: "sample run",
		},
	})
	t.Cleanup(func() { ClearRuntimeContractFinalForTest(mismatch) })
	if err := validateCommandRegistryAnnotation(mismatch, "sample run", spec); err == nil || !strings.Contains(err.Error(), "Contract.Identity mismatch") {
		t.Fatalf("Identity mismatch error = %v", err)
	}
	aligned := &cobra.Command{Use: "run"}
	contractfinal.RegisterRuntimeContractFinal(aligned, contract.ContractFinalPayload{
		Identity: &contract.ToolIdentitySpec{
			ProductID: "sample", Name: "run", CanonicalPath: "sample.run",
			CLIPath: "sample run", PrimaryCLIPath: "sample run",
		},
	})
	t.Cleanup(func() { ClearRuntimeContractFinalForTest(aligned) })
	if err := validateCommandRegistryAnnotation(aligned, "sample run", spec); err != nil {
		t.Fatalf("aligned Identity must pass: %v", err)
	}
}

func TestCrossPlatformCoverageValidateContractIdentityAgainstCommandSpecEdges(t *testing.T) {
	path := "sample run"
	baseSpec := CommandSpec{
		CanonicalPath:  "sample.run",
		PrimaryCLIPath: "sample run",
		Aliases:        []string{"sample alias"},
	}

	if err := validateContractIdentityAgainstCommandSpec(contract.ToolIdentitySpec{}, CommandSpec{
		CanonicalPath:  "not-a-canonical",
		PrimaryCLIPath: path,
	}, path); err == nil || !strings.Contains(err.Error(), "invalid canonical path") {
		t.Fatalf("invalid canonical error = %v", err)
	}

	// CLIPath empty fills from PrimaryCLIPath (and the reverse) before compare.
	if err := validateContractIdentityAgainstCommandSpec(contract.ToolIdentitySpec{
		ProductID: "sample", Name: "run", CanonicalPath: "sample.run",
		PrimaryCLIPath: "sample run", Aliases: []string{"sample alias"},
	}, baseSpec, path); err != nil {
		t.Fatalf("PrimaryCLIPath-only identity must pass after mutual fill: %v", err)
	}
	if err := validateContractIdentityAgainstCommandSpec(contract.ToolIdentitySpec{
		ProductID: "sample", Name: "run", CanonicalPath: "sample.run",
		CLIPath: "sample run", Aliases: []string{"sample alias"},
	}, baseSpec, path); err != nil {
		t.Fatalf("CLIPath-only identity must pass after mutual fill: %v", err)
	}

	// Length mismatch and same-length element mismatch both reject via set compare.
	if err := validateContractIdentityAgainstCommandSpec(contract.ToolIdentitySpec{
		ProductID: "sample", Name: "run", CanonicalPath: "sample.run",
		CLIPath: "sample run", PrimaryCLIPath: "sample run",
		Aliases: []string{"sample alias", "sample extra"},
	}, baseSpec, path); err == nil || !strings.Contains(err.Error(), "aliases") {
		t.Fatalf("alias length mismatch error = %v", err)
	}
	if err := validateContractIdentityAgainstCommandSpec(contract.ToolIdentitySpec{
		ProductID: "sample", Name: "run", CanonicalPath: "sample.run",
		CLIPath: "sample run", PrimaryCLIPath: "sample run",
		Aliases: []string{"sample other"},
	}, baseSpec, path); err == nil || !strings.Contains(err.Error(), "aliases") {
		t.Fatalf("same-length alias member mismatch error = %v", err)
	}
	if stringSlicesEqualAsSet([]string{"a"}, []string{"b"}) {
		t.Fatal("stringSlicesEqualAsSet must reject same-length unequal members")
	}
	if stringSlicesEqualAsSet([]string{"a"}, []string{"a", "b"}) {
		t.Fatal("stringSlicesEqualAsSet must reject length mismatch")
	}
}
