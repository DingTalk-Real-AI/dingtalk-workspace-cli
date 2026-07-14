// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestBuildEffectiveCommandRegistryMergesReviewedManualCommands(t *testing.T) {
	root := commandRegistryTestRoot("item get", "item legacy", "helper add")
	annotateTestCompatibilityPair(exactSchemaCommand(root, "item get"), exactSchemaCommand(root, "item legacy"))
	reviewed := mustCommandRegistry(t, []CommandSpec{{
		CanonicalPath:  "item.get_item",
		PrimaryCLIPath: "item get",
		Aliases:        []string{"item legacy"},
	}})
	manual := ManualSchemaHintSnapshot{
		Version: manualSchemaHintVersion,
		Commands: []ManualSchemaCommandHint{
			{
				CLIPath:       "item get",
				CanonicalPath: "item.get_item",
				Reason:        "Confirms the reviewed primary identity",
				Reviewed:      true,
			},
			{
				CLIPath:       "item legacy",
				CanonicalPath: "item.get_item",
				Reason:        "Reviewed compatibility alias",
				Reviewed:      true,
			},
			{
				CLIPath:       "helper add",
				CanonicalPath: "helper.add_helper",
				Reason:        "Reviewed existing helper command",
				Reviewed:      true,
			},
		},
	}

	effective, err := buildEffectiveCommandRegistry(root, reviewed, manual)
	if err != nil {
		t.Fatalf("buildEffectiveCommandRegistry() error = %v", err)
	}
	if got := len(effective.Commands); got != 2 {
		t.Fatalf("command count = %d, want 2", got)
	}
	item := effective.ByCanonical["item.get_item"]
	if strings.Join(item.Aliases, ",") != "item legacy" {
		t.Fatalf("item aliases = %#v", item.Aliases)
	}
	helper := effective.ByCanonical["helper.add_helper"]
	if helper.Source != "reviewed_manual_hint" || helper.ReviewReason != "Reviewed existing helper command" || helper.Visibility != SchemaVisibilityPublic {
		t.Fatalf("manual helper source = %#v", helper)
	}
	if effective.SourceHash() == reviewed.SourceHash() {
		t.Fatal("manual-only command was omitted from the effective registry hash")
	}
	bound, err := BindEffectiveCommandRegistry(root, effective)
	if err != nil {
		t.Fatalf("BindEffectiveCommandRegistry(effective) error = %v", err)
	}
	if got := bound.ByCLIPath["helper add"].CanonicalPath; got != "helper.add_helper" {
		t.Fatalf("manual-only bound canonical = %q", got)
	}
	registry, err := AssembleSchemaRegistryFromBound(bound)
	if err != nil {
		t.Fatalf("AssembleSchemaRegistryFromBound(effective) error = %v", err)
	}
	index, err := registry.Index()
	if err != nil {
		t.Fatalf("manual-only registry index error = %v", err)
	}
	if _, ok := index.Resolve("helper.add_helper"); !ok {
		t.Fatal("manual-only command was dropped before final SchemaRegistry delivery")
	}
	snapshot, err := registry.ToSnapshotPayload()
	if err != nil {
		t.Fatalf("manual-only snapshot serialization error = %v", err)
	}
	if _, ok := snapshot.Tools["helper.add_helper"]; !ok {
		t.Fatal("manual-only command was dropped from the Catalog full-tool projection")
	}
}

func TestBuildEffectiveCommandRegistryRejectsManualIdentityConflict(t *testing.T) {
	root := commandRegistryTestRoot("item get")
	reviewed := mustCommandRegistry(t, []CommandSpec{{
		CanonicalPath:  "item.get_item",
		PrimaryCLIPath: "item get",
	}})
	manual := ManualSchemaHintSnapshot{
		Version: manualSchemaHintVersion,
		Commands: []ManualSchemaCommandHint{{
			CLIPath:       "item get",
			CanonicalPath: "item.delete_item",
			Reason:        "Conflict fixture",
			Reviewed:      true,
		}},
	}

	_, err := buildEffectiveCommandRegistry(root, reviewed, manual)
	if err == nil || !strings.Contains(err.Error(), "conflicts with command registry canonical path") {
		t.Fatalf("error = %v", err)
	}
}

func TestBuildEffectiveCommandRegistryRejectsPhantomManualCommand(t *testing.T) {
	root := commandRegistryTestRoot("item get")
	reviewed := mustCommandRegistry(t, nil)
	manual := ManualSchemaHintSnapshot{
		Version: manualSchemaHintVersion,
		Commands: []ManualSchemaCommandHint{{
			CLIPath:       "item missing",
			CanonicalPath: "item.missing_item",
			Reason:        "Phantom fixture",
			Reviewed:      true,
		}},
	}

	_, err := buildEffectiveCommandRegistry(root, reviewed, manual)
	if err == nil || !strings.Contains(err.Error(), "does not resolve to an existing Cobra command") {
		t.Fatalf("error = %v", err)
	}
}

func TestBuildEffectiveCommandRegistryRejectsManualAliasCreation(t *testing.T) {
	root := commandRegistryTestRoot("item get", "item legacy")
	reviewed := mustCommandRegistry(t, []CommandSpec{{
		CanonicalPath:  "item.get_item",
		PrimaryCLIPath: "item get",
	}})
	manual := ManualSchemaHintSnapshot{
		Version: manualSchemaHintVersion,
		Commands: []ManualSchemaCommandHint{{
			CLIPath:       "item legacy",
			CanonicalPath: "item.get_item",
			Reason:        "Alias must be reviewed in the registry",
			Reviewed:      true,
		}},
	}

	_, err := buildEffectiveCommandRegistry(root, reviewed, manual)
	if err == nil || !strings.Contains(err.Error(), "cannot create an alias") {
		t.Fatalf("error = %v", err)
	}
}

func TestBindEffectiveCommandRegistryResolvesCompatibilityAliasLeaf(t *testing.T) {
	root := commandRegistryTestRoot("item get", "item legacy", "compat hidden")
	primary := exactSchemaCommand(root, "item get")
	alias := exactSchemaCommand(root, "item legacy")
	alias.Hidden = true
	annotateTestCompatibilityPair(primary, alias)
	hidden := exactSchemaCommand(root, "compat hidden")
	hidden.Hidden = true
	AttachRuntimeSchema(primary, "item", "get_item", "test")
	AttachRuntimeSchema(alias, "item", "get_item", "test")
	effective := mustEffectiveCommandRegistry(t, []CommandSpec{
		{
			CanonicalPath:  "item.get_item",
			PrimaryCLIPath: "item get",
			Aliases:        []string{"item legacy"},
		},
		{
			CanonicalPath:  "compat.hidden_helper",
			PrimaryCLIPath: "compat hidden",
		},
	})

	bound, err := BindEffectiveCommandRegistry(root, effective)
	if err != nil {
		t.Fatalf("BindEffectiveCommandRegistry() error = %v", err)
	}
	item := bound.ByCanonical["item.get_item"]
	if item.PrimaryCommand != primary || len(item.AliasCommands) != 1 || item.AliasCommands[0].Command != alias {
		t.Fatalf("item binding = %#v", item)
	}
	if item.AliasCommands[0].Path != "item legacy" || item.AliasCommands[0].Kind != AliasKindCompatibilityLeaf {
		t.Fatalf("compatibility alias binding = %#v", item.AliasCommands[0])
	}
	if bound.ByCanonical["compat.hidden_helper"].PrimaryCommand != hidden {
		t.Fatal("explicit reviewed hidden runnable leaf was not bound")
	}
}

func TestBindEffectiveCommandRegistryAcceptsEquivalentCompatibilityLeafContract(t *testing.T) {
	root := commandRegistryTestRoot("item get", "item legacy")
	primary := exactSchemaCommand(root, "item get")
	alias := exactSchemaCommand(root, "item legacy")
	alias.Hidden = true
	annotateTestCompatibilityPair(primary, alias)
	primary.Use = "get <item-id>"
	alias.Use = "legacy <item-id>"
	argsValidator := cobra.ExactArgs(1)
	primary.Args = argsValidator
	alias.Args = argsValidator
	primary.Flags().String("query", "all", "query expression (必填)")
	alias.Flags().String("query", "all", "query expression (必填)")
	if err := primary.MarkFlagRequired("query"); err != nil {
		t.Fatalf("mark primary required: %v", err)
	}
	if err := alias.MarkFlagRequired("query"); err != nil {
		t.Fatalf("mark alias required: %v", err)
	}
	AnnotateRuntimeFlagRequiredWhen(primary, "query", "item-id is present")
	AnnotateRuntimeFlagRequiredWhen(alias, "query", "item-id is present")
	positionals := []RuntimeSchemaPositional{{Name: "item-id", Type: "string", Required: true, Index: 0}}
	AnnotateRuntimePositionals(primary, positionals...)
	AnnotateRuntimePositionals(alias, positionals...)
	constraints := RuntimeSchemaConstraints{RequireTogether: [][]string{{"item-id", "query"}}}
	AnnotateRuntimeConstraints(primary, constraints)
	AnnotateRuntimeConstraints(alias, constraints)

	effective := mustEffectiveCommandRegistry(t, []CommandSpec{{
		CanonicalPath:  "item.get_item",
		PrimaryCLIPath: "item get",
		Aliases:        []string{"item legacy"},
	}})
	bound, err := BindEffectiveCommandRegistry(root, effective)
	if err != nil {
		t.Fatalf("BindEffectiveCommandRegistry() error = %v", err)
	}
	got := bound.ByCanonical["item.get_item"].AliasCommands[0]
	if got.Kind != AliasKindCompatibilityLeaf || got.Command != alias {
		t.Fatalf("compatibility alias = %#v", got)
	}
}

func TestBindEffectiveCommandRegistryRejectsDifferentCompatibilityHandlersWithoutTypedReview(t *testing.T) {
	root := commandRegistryTestRoot("item get", "item legacy")
	primary := exactSchemaCommand(root, "item get")
	alias := exactSchemaCommand(root, "item legacy")
	alias.Hidden = true
	primary.Run = nil
	alias.Run = nil
	primary.RunE = compatibilityPrimaryRunE
	alias.RunE = compatibilityAliasRunE

	effective := mustEffectiveCommandRegistry(t, []CommandSpec{{
		CanonicalPath:  "item.get_item",
		PrimaryCLIPath: "item get",
		Aliases:        []string{"item legacy"},
	}})
	_, err := BindEffectiveCommandRegistry(root, effective)
	if err == nil || !strings.Contains(err.Error(), "execution handler implementation differs for RunE") {
		t.Fatalf("handler mismatch error = %v", err)
	}
	if !strings.Contains(err.Error(), "distinct canonical tools") {
		t.Fatalf("handler mismatch error is not actionable: %v", err)
	}
}

func TestBindEffectiveCommandRegistryRejectsIndependentLeafWithoutTypedReviewEvenWithSameHandler(t *testing.T) {
	root := commandRegistryTestRoot("item get", "item legacy")
	exactSchemaCommand(root, "item get")
	alias := exactSchemaCommand(root, "item legacy")
	alias.Hidden = true

	effective := mustEffectiveCommandRegistry(t, []CommandSpec{{
		CanonicalPath:  "item.get_item",
		PrimaryCLIPath: "item get",
		Aliases:        []string{"item legacy"},
	}})
	_, err := BindEffectiveCommandRegistry(root, effective)
	if err == nil || !strings.Contains(err.Error(), "independent compatibility leaves require the same reviewed typed compatibility equivalence") {
		t.Fatalf("missing compatibility review error = %v", err)
	}
}

func TestBindEffectiveCommandRegistryAcceptsDifferentHandlersWithMatchingTypedReview(t *testing.T) {
	root := commandRegistryTestRoot("item get", "item legacy")
	primary := exactSchemaCommand(root, "item get")
	alias := exactSchemaCommand(root, "item legacy")
	alias.Hidden = true
	primary.Run = nil
	alias.Run = nil
	primary.RunE = compatibilityPrimaryRunE
	alias.RunE = compatibilityAliasRunE
	AnnotateRuntimeCompatibilityEquivalence(primary, alias, RuntimeCompatibilityEquivalence{
		ID:       "item-get-legacy-v1",
		Reason:   "The compatibility wrapper adds presentation-only behavior before invoking the exact primary operation.",
		Reviewed: true,
	})

	effective := mustEffectiveCommandRegistry(t, []CommandSpec{{
		CanonicalPath:  "item.get_item",
		PrimaryCLIPath: "item get",
		Aliases:        []string{"item legacy"},
	}})
	if _, err := BindEffectiveCommandRegistry(root, effective); err != nil {
		t.Fatalf("reviewed handler equivalence was rejected: %v", err)
	}
}

func TestAnnotateRuntimeCompatibilityEquivalenceRejectsConflictingExistingReview(t *testing.T) {
	root := commandRegistryTestRoot("item get", "item legacy", "item older")
	primary := exactSchemaCommand(root, "item get")
	legacy := exactSchemaCommand(root, "item legacy")
	older := exactSchemaCommand(root, "item older")
	AnnotateRuntimeCompatibilityEquivalence(primary, legacy, RuntimeCompatibilityEquivalence{
		ID: "item-get-legacy-v1", Reason: "Reviewed first compatibility contract.", Reviewed: true,
	})

	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("conflicting compatibility review silently overwrote the existing marker")
		}
	}()
	AnnotateRuntimeCompatibilityEquivalence(primary, older, RuntimeCompatibilityEquivalence{
		ID: "item-get-older-v1", Reason: "Conflicting compatibility contract.", Reviewed: true,
	})
}

func compatibilityPrimaryRunE(*cobra.Command, []string) error { return nil }

func compatibilityAliasRunE(*cobra.Command, []string) error { return nil }

func TestBindEffectiveCommandRegistryIgnoresLazilyMaterializedHelpFlag(t *testing.T) {
	root := commandRegistryTestRoot("item get", "item legacy")
	primary := exactSchemaCommand(root, "item get")
	alias := exactSchemaCommand(root, "item legacy")
	alias.Hidden = true
	annotateTestCompatibilityPair(primary, alias)
	// Cobra does this only for the command selected by Execute. Binding after
	// parsing must remain independent of whether the primary or compatibility
	// path was selected first.
	primary.InitDefaultHelpFlag()

	effective := mustEffectiveCommandRegistry(t, []CommandSpec{{
		CanonicalPath:  "item.get_item",
		PrimaryCLIPath: "item get",
		Aliases:        []string{"item legacy"},
	}})
	if _, err := BindEffectiveCommandRegistry(root, effective); err != nil {
		t.Fatalf("lazy Cobra --help flag changed compatibility contract: %v", err)
	}
}

func TestBindEffectiveCommandRegistryCompatibilityGateIsOrderIndependentOfCanonicalMetadataMaterialization(t *testing.T) {
	const canonical = "compat.get_item"
	previousMetadata, hadMetadata := runtimeSchemaParameterMetadataByCanonical[canonical]
	previousConstraints, hadConstraints := runtimeSchemaConstraintsByCanonical[canonical]
	t.Cleanup(func() {
		if hadMetadata {
			runtimeSchemaParameterMetadataByCanonical[canonical] = previousMetadata
		} else {
			delete(runtimeSchemaParameterMetadataByCanonical, canonical)
		}
		if hadConstraints {
			runtimeSchemaConstraintsByCanonical[canonical] = previousConstraints
		} else {
			delete(runtimeSchemaConstraintsByCanonical, canonical)
		}
	})
	runtimeSchemaParameterMetadataByCanonical[canonical] = RuntimeSchemaParameterMetadata{
		RequiredWhen: map[string]string{"query": "item-id is present"},
		Formats:      map[string]string{"query": "typed-format"},
		Examples:     map[string]string{"query": "open"},
		Enums:        map[string][]string{"query": {"open", "closed"}},
	}
	runtimeSchemaConstraintsByCanonical[canonical] = RuntimeSchemaConstraints{
		RequireTogether: [][]string{{"item-id", "query"}},
	}

	root := commandRegistryTestRoot("item get", "item legacy")
	primary := exactSchemaCommand(root, "item get")
	alias := exactSchemaCommand(root, "item legacy")
	annotateTestCompatibilityPair(primary, alias)
	primary.Flags().String("query", "", "query")
	alias.Flags().String("query", "", "query")
	AnnotateRuntimeFlagFormat(primary, "query", "native-format")
	AnnotateRuntimeFlagFormat(alias, "query", "native-format")
	AnnotateRuntimeFlagExample(primary, "query", "native-example")
	AnnotateRuntimeFlagExample(alias, "query", "native-example")
	AnnotateRuntimeFlagEnum(primary, "query", "native-a", "native-b")
	AnnotateRuntimeFlagEnum(alias, "query", "native-a", "native-b")
	// Simulate a previous Schema collection pass, which materializes canonical
	// metadata only on the primary leaf. Binding must model the same canonical
	// facts on both paths and remain deterministic on the next pass.
	applyRuntimeSchemaParameterMetadata(primary, canonical)
	AnnotateRuntimeConstraints(primary, runtimeSchemaConstraintsByCanonical[canonical])
	primaryFlag := primary.Flags().Lookup("query")
	if got := firstFlagAnnotation(primaryFlag, "x-cli-format"); got != "native-format" {
		t.Fatalf("typed metadata overwrote native format annotation: %q", got)
	}
	if got := firstFlagAnnotation(primaryFlag, runtimeSchemaFlagMetadataFormatAnnotation); got != "typed-format" {
		t.Fatalf("typed format annotation = %q", got)
	}
	if got := firstFlagAnnotation(primaryFlag, runtimeSchemaFlagExampleAnnotation); got != "native-example" {
		t.Fatalf("typed metadata overwrote native example annotation: %q", got)
	}
	if got := firstFlagAnnotation(primaryFlag, runtimeSchemaFlagMetadataExampleAnnotation); got != "open" {
		t.Fatalf("typed example annotation = %q", got)
	}
	if got := runtimeFlagEnum(primaryFlag); !reflect.DeepEqual(got, []string{"native-a", "native-b"}) {
		t.Fatalf("typed metadata overwrote native enum annotation: %#v", got)
	}
	if got := runtimeFlagEnumAnnotation(primaryFlag, runtimeSchemaFlagMetadataEnumAnnotation); !reflect.DeepEqual(got, []string{"open", "closed"}) {
		t.Fatalf("typed enum annotation = %#v", got)
	}
	aliasAnnotations := effectiveCompatibilityFlagAnnotations(alias.Flags().Lookup("query"), nil, runtimeSchemaParameterMetadataByCanonical[canonical])
	if got := firstCompatibilityAnnotation(aliasAnnotations, "x-cli-format"); got != "native-format" {
		t.Fatalf("compatibility overlay dropped native format annotation: %q", got)
	}
	if got := firstCompatibilityAnnotation(aliasAnnotations, runtimeSchemaFlagMetadataFormatAnnotation); got != "typed-format" {
		t.Fatalf("compatibility overlay dropped typed format annotation: %q", got)
	}
	if got := aliasAnnotations["x-cli-enum"]; !reflect.DeepEqual(got, []string{"native-a", "native-b"}) {
		t.Fatalf("compatibility overlay dropped native enum annotation: %#v", got)
	}
	if got := aliasAnnotations[runtimeSchemaFlagMetadataEnumAnnotation]; !reflect.DeepEqual(got, []string{"open", "closed"}) {
		t.Fatalf("compatibility overlay dropped typed enum annotation: %#v", got)
	}

	effective := mustEffectiveCommandRegistry(t, []CommandSpec{{
		CanonicalPath:  canonical,
		PrimaryCLIPath: "item get",
		Aliases:        []string{"item legacy"},
	}})
	if _, err := BindEffectiveCommandRegistry(root, effective); err != nil {
		t.Fatalf("BindEffectiveCommandRegistry() after canonical metadata materialization error = %v", err)
	}
}

func TestBindEffectiveCommandRegistryTreatsManualParameterHintsAsCanonicalProjection(t *testing.T) {
	root := commandRegistryTestRoot("item get", "item legacy")
	primary := exactSchemaCommand(root, "item get")
	alias := exactSchemaCommand(root, "item legacy")
	alias.Hidden = true
	annotateTestCompatibilityPair(primary, alias)
	primary.Flags().Bool("include-archived", false, "include archived items")
	alias.Flags().Bool("include-archived", false, "include archived items")

	required := false
	if err := annotateManualSchemaParameter(
		primary,
		"include-archived",
		ManualSchemaParameterHint{Required: &required},
		"Reviewed canonical projection may lower the Agent-facing required value.",
	); err != nil {
		t.Fatalf("annotateManualSchemaParameter() error = %v", err)
	}

	effective := mustEffectiveCommandRegistry(t, []CommandSpec{{
		CanonicalPath:  "item.get_item",
		PrimaryCLIPath: "item get",
		Aliases:        []string{"item legacy"},
	}})
	if _, err := BindEffectiveCommandRegistry(root, effective); err != nil {
		t.Fatalf("manual canonical projection must not create compatibility-leaf executable drift: %v", err)
	}
	if _, _, ok, err := runtimeManualSchemaParameter(alias, "include-archived"); err != nil || ok {
		t.Fatalf("manual projection was copied to compatibility leaf: present=%t err=%v", ok, err)
	}
}

func TestBindEffectiveCommandRegistryRejectsCompatibilityLeafSemanticDrift(t *testing.T) {
	tests := []struct {
		name   string
		want   string
		mutate func(primary, alias *cobra.Command)
	}{
		{
			name: "flag surface",
			want: "flag --query is missing",
			mutate: func(primary, _ *cobra.Command) {
				primary.Flags().String("query", "", "query")
			},
		},
		{
			name: "flag type",
			want: "flag --query type differs",
			mutate: func(primary, alias *cobra.Command) {
				primary.Flags().String("query", "", "query")
				alias.Flags().Int("query", 0, "query")
			},
		},
		{
			name: "flag default",
			want: "flag --limit default differs",
			mutate: func(primary, alias *cobra.Command) {
				primary.Flags().Int("limit", 20, "limit")
				alias.Flags().Int("limit", 50, "limit")
			},
		},
		{
			name: "flag hidden",
			want: "flag --query hidden state differs",
			mutate: func(primary, alias *cobra.Command) {
				primary.Flags().String("query", "", "query")
				alias.Flags().String("query", "", "query")
				_ = alias.Flags().MarkHidden("query")
			},
		},
		{
			name: "flag required",
			want: "required/required_when facts differ",
			mutate: func(primary, alias *cobra.Command) {
				primary.Flags().String("query", "", "query")
				alias.Flags().String("query", "", "query")
				_ = primary.MarkFlagRequired("query")
			},
		},
		{
			name: "flag required when",
			want: "required/required_when facts differ",
			mutate: func(primary, alias *cobra.Command) {
				primary.Flags().String("query", "", "query")
				alias.Flags().String("query", "", "query")
				AnnotateRuntimeFlagRequiredWhen(primary, "query", "mode is search")
				AnnotateRuntimeFlagRequiredWhen(alias, "query", "mode is list")
			},
		},
		{
			name: "persistent behavior",
			want: "local/persistent/inherited behavior differs",
			mutate: func(primary, alias *cobra.Command) {
				primary.PersistentFlags().String("scope", "", "scope")
				alias.Flags().String("scope", "", "scope")
			},
		},
		{
			name: "Args",
			want: "Args validator implementation differs",
			mutate: func(primary, alias *cobra.Command) {
				primary.Args = cobra.NoArgs
				alias.Args = cobra.MaximumNArgs(1)
			},
		},
		{
			name: "positionals",
			want: "runtime positionals differ",
			mutate: func(primary, alias *cobra.Command) {
				AnnotateRuntimePositionals(primary, RuntimeSchemaPositional{Name: "item-id", Required: true, Index: 0})
				AnnotateRuntimePositionals(alias, RuntimeSchemaPositional{Name: "item-id", Required: false, Index: 0})
			},
		},
		{
			name: "constraints",
			want: "runtime constraints differ",
			mutate: func(primary, alias *cobra.Command) {
				AnnotateRuntimeConstraints(primary, RuntimeSchemaConstraints{RequireOneOf: [][]string{{"query", "item-id"}}})
				AnnotateRuntimeConstraints(alias, RuntimeSchemaConstraints{RequireTogether: [][]string{{"query", "item-id"}}})
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := commandRegistryTestRoot("item get", "item legacy")
			primary := exactSchemaCommand(root, "item get")
			alias := exactSchemaCommand(root, "item legacy")
			alias.Hidden = true
			test.mutate(primary, alias)
			effective := mustEffectiveCommandRegistry(t, []CommandSpec{{
				CanonicalPath:  "item.get_item",
				PrimaryCLIPath: "item get",
				Aliases:        []string{"item legacy"},
			}})

			_, err := BindEffectiveCommandRegistry(root, effective)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
			if !strings.Contains(err.Error(), "item.get_item") || !strings.Contains(err.Error(), `compatibility leaf alias "item legacy"`) {
				t.Fatalf("error is not actionable: %v", err)
			}
		})
	}
}

func TestBindEffectiveCommandRegistryResolvesAncestorCobraAlias(t *testing.T) {
	root := commandRegistryTestRoot("item action get")
	group := exactSchemaCommand(root, "item action")
	group.Aliases = []string{"ops"}
	primary := exactSchemaCommand(root, "item action get")
	effective := mustEffectiveCommandRegistry(t, []CommandSpec{{
		CanonicalPath:  "item.get_item",
		PrimaryCLIPath: "item action get",
		Aliases:        []string{"item ops get"},
	}})

	bound, err := BindEffectiveCommandRegistry(root, effective)
	if err != nil {
		t.Fatalf("BindEffectiveCommandRegistry() error = %v", err)
	}
	alias := bound.ByCanonical["item.get_item"].AliasCommands[0]
	if alias.Command != primary || alias.Kind != AliasKindCobraAlias {
		t.Fatalf("ancestor Cobra alias binding = %#v", alias)
	}
}

func TestBindEffectiveCommandRegistryRejectsAmbiguousCobraAlias(t *testing.T) {
	root := commandRegistryTestRoot("item get", "item list")
	exactSchemaCommand(root, "item get").Aliases = []string{"fetch"}
	exactSchemaCommand(root, "item list").Aliases = []string{"fetch"}
	effective := mustEffectiveCommandRegistry(t, []CommandSpec{{
		CanonicalPath:  "item.get_item",
		PrimaryCLIPath: "item get",
		Aliases:        []string{"item fetch"},
	}})

	_, err := BindEffectiveCommandRegistry(root, effective)
	if err == nil || !strings.Contains(err.Error(), `cobra alias segment "fetch" is ambiguous`) {
		t.Fatalf("error = %v", err)
	}
}

func TestBindEffectiveCommandRegistryPrefersExactNameOverCobraAlias(t *testing.T) {
	root := commandRegistryTestRoot("item get", "item fetch")
	primary := exactSchemaCommand(root, "item get")
	compatibility := exactSchemaCommand(root, "item fetch")
	primary.Aliases = []string{"fetch"}
	compatibility.Hidden = true
	annotateTestCompatibilityPair(primary, compatibility)
	effective := mustEffectiveCommandRegistry(t, []CommandSpec{{
		CanonicalPath:  "item.get_item",
		PrimaryCLIPath: "item get",
		Aliases:        []string{"item fetch"},
	}})

	bound, err := BindEffectiveCommandRegistry(root, effective)
	if err != nil {
		t.Fatalf("BindEffectiveCommandRegistry() error = %v", err)
	}
	alias := bound.ByCanonical["item.get_item"].AliasCommands[0]
	if alias.Command != compatibility || alias.Kind != AliasKindCompatibilityLeaf {
		t.Fatalf("name-priority alias binding = %#v", alias)
	}
}

func TestBindEffectiveCommandRegistryResolvesCobraAliasToPrimaryCommand(t *testing.T) {
	root := commandRegistryTestRoot("item get")
	primary := exactSchemaCommand(root, "item get")
	primary.Aliases = []string{"fetch"}
	AttachRuntimeSchema(primary, "item", "get_item", "test")
	effective := mustEffectiveCommandRegistry(t, []CommandSpec{{
		CanonicalPath:  "item.get_item",
		PrimaryCLIPath: "item get",
		Aliases:        []string{"item fetch"},
	}})

	bound, err := BindEffectiveCommandRegistry(root, effective)
	if err != nil {
		t.Fatalf("BindEffectiveCommandRegistry() error = %v", err)
	}
	alias := bound.ByCanonical["item.get_item"].AliasCommands[0]
	if alias.Path != "item fetch" || alias.Command != primary || alias.Kind != AliasKindCobraAlias {
		t.Fatalf("Cobra alias binding = %#v", alias)
	}
	if got := bound.ByCLIPath["item fetch"].CanonicalPath; got != "item.get_item" {
		t.Fatalf("alias canonical = %q", got)
	}
}

func TestBindEffectiveCommandRegistryRejectsCobraAliasToDifferentPrimary(t *testing.T) {
	root := commandRegistryTestRoot("item get", "item list")
	list := exactSchemaCommand(root, "item list")
	list.Aliases = []string{"fetch"}
	effective := mustEffectiveCommandRegistry(t, []CommandSpec{{
		CanonicalPath:  "item.get_item",
		PrimaryCLIPath: "item get",
		Aliases:        []string{"item fetch"},
	}})

	_, err := BindEffectiveCommandRegistry(root, effective)
	if err == nil || !strings.Contains(err.Error(), "resolves to a different command") {
		t.Fatalf("error = %v", err)
	}
}

func TestBindEffectiveCommandRegistryRejectsAliasAsPrimaryPath(t *testing.T) {
	root := commandRegistryTestRoot("item get")
	exactSchemaCommand(root, "item get").Aliases = []string{"fetch"}
	effective := mustEffectiveCommandRegistry(t, []CommandSpec{{
		CanonicalPath:  "item.get_item",
		PrimaryCLIPath: "item fetch",
	}})

	_, err := BindEffectiveCommandRegistry(root, effective)
	if err == nil || !strings.Contains(err.Error(), "primary path") || !strings.Contains(err.Error(), "real Cobra command names") {
		t.Fatalf("error = %v", err)
	}
}

func TestBuildEffectiveCommandRegistryResolvesManualAliasBeforeRegistryPolicy(t *testing.T) {
	root := commandRegistryTestRoot("item action get")
	exactSchemaCommand(root, "item action").Aliases = []string{"ops"}
	reviewed := mustCommandRegistry(t, []CommandSpec{{
		CanonicalPath:  "item.get_item",
		PrimaryCLIPath: "item action get",
	}})
	manual := ManualSchemaHintSnapshot{
		Version: manualSchemaHintVersion,
		Commands: []ManualSchemaCommandHint{{
			CLIPath:       "item ops get",
			CanonicalPath: "item.get_item",
			Reason:        "Reviewed ancestor Cobra alias",
			Reviewed:      true,
		}},
	}

	_, err := buildEffectiveCommandRegistry(root, reviewed, manual)
	if err == nil || !strings.Contains(err.Error(), "cannot create an alias") {
		t.Fatalf("error = %v", err)
	}
}

func TestBuildEffectiveCommandRegistryRejectsAmbiguousManualAlias(t *testing.T) {
	root := commandRegistryTestRoot("item get", "item list")
	exactSchemaCommand(root, "item get").Aliases = []string{"fetch"}
	exactSchemaCommand(root, "item list").Aliases = []string{"fetch"}
	reviewed := mustCommandRegistry(t, []CommandSpec{{
		CanonicalPath:  "item.get_item",
		PrimaryCLIPath: "item get",
	}})
	manual := ManualSchemaHintSnapshot{
		Version: manualSchemaHintVersion,
		Commands: []ManualSchemaCommandHint{{
			CLIPath:       "item fetch",
			CanonicalPath: "item.get_item",
			Reason:        "Ambiguous alias fixture",
			Reviewed:      true,
		}},
	}

	_, err := buildEffectiveCommandRegistry(root, reviewed, manual)
	if err == nil || !strings.Contains(err.Error(), `cobra alias segment "fetch" is ambiguous`) {
		t.Fatalf("error = %v", err)
	}
}

func TestBindEffectiveCommandRegistryRejectsStaleRegistryPath(t *testing.T) {
	root := commandRegistryTestRoot("item get")
	effective := mustEffectiveCommandRegistry(t, []CommandSpec{{
		CanonicalPath:  "item.delete_item",
		PrimaryCLIPath: "item delete",
	}})

	_, err := BindEffectiveCommandRegistry(root, effective)
	if err == nil || !strings.Contains(err.Error(), "stale cli path") {
		t.Fatalf("error = %v", err)
	}
}

func TestBindEffectiveCommandRegistryDoesNotPrefixMatch(t *testing.T) {
	root := commandRegistryTestRoot("item get")
	effective := mustEffectiveCommandRegistry(t, []CommandSpec{{
		CanonicalPath:  "item.get_item",
		PrimaryCLIPath: "item ge",
	}})

	_, err := BindEffectiveCommandRegistry(root, effective)
	if err == nil || !strings.Contains(err.Error(), `stale cli path "item ge"`) {
		t.Fatalf("error = %v", err)
	}
}

func TestBindEffectiveCommandRegistryRejectsNativeAnnotationConflict(t *testing.T) {
	root := commandRegistryTestRoot("item get")
	leaf := exactSchemaCommand(root, "item get")
	AttachRuntimeSchema(leaf, "item", "delete_item", "test")
	effective := mustEffectiveCommandRegistry(t, []CommandSpec{{
		CanonicalPath:  "item.get_item",
		PrimaryCLIPath: "item get",
	}})

	_, err := BindEffectiveCommandRegistry(root, effective)
	if err == nil || !strings.Contains(err.Error(), "conflicts with native annotation item.delete_item") {
		t.Fatalf("error = %v", err)
	}
}

func TestCommandRegistryRejectsAliasCollision(t *testing.T) {
	_, err := newEffectiveCommandRegistry([]CommandSpec{
		{
			CanonicalPath:  "item.get_item",
			PrimaryCLIPath: "item get",
			Aliases:        []string{"item shared"},
		},
		{
			CanonicalPath:  "item.delete_item",
			PrimaryCLIPath: "item delete",
			Aliases:        []string{"item shared"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "belongs to both") {
		t.Fatalf("error = %v", err)
	}
}

func TestCommandRegistryDefaultsAndValidatesReviewedVisibility(t *testing.T) {
	registry := mustEffectiveCommandRegistry(t, []CommandSpec{{
		CanonicalPath:  "item.get_item",
		PrimaryCLIPath: "item get",
	}})
	if got := registry.ByCanonical["item.get_item"].Visibility; got != SchemaVisibilityPublic {
		t.Fatalf("default visibility = %q, want public", got)
	}

	_, err := newEffectiveCommandRegistry([]CommandSpec{{
		CanonicalPath:  "item.delete_item",
		PrimaryCLIPath: "item delete",
		Visibility:     SchemaVisibility("unreviewed"),
	}})
	if err == nil || !strings.Contains(err.Error(), "invalid visibility") {
		t.Fatalf("invalid visibility error = %v", err)
	}
}

func TestAssemblerUsesReviewedCommandVisibility(t *testing.T) {
	root := commandRegistryTestRoot("doc-comment run", "item internal")
	effective := mustEffectiveCommandRegistry(t, []CommandSpec{
		{
			CanonicalPath:  "doc-comment.run",
			PrimaryCLIPath: "doc-comment run",
			Visibility:     SchemaVisibilityPublic,
		},
		{
			CanonicalPath:  "item.internal",
			PrimaryCLIPath: "item internal",
			Visibility:     SchemaVisibilityInternal,
		},
	})
	bound, err := BindEffectiveCommandRegistry(root, effective)
	if err != nil {
		t.Fatalf("BindEffectiveCommandRegistry() error = %v", err)
	}
	registry, err := AssembleSchemaRegistryFromBound(bound)
	if err != nil {
		t.Fatalf("AssembleSchemaRegistryFromBound() error = %v", err)
	}
	index, err := registry.Index()
	if err != nil {
		t.Fatalf("SchemaRegistry.Index() error = %v", err)
	}
	if _, ok := index.Resolve("doc-comment.run"); !ok {
		t.Fatal("reviewed public command was filtered by legacy product visibility")
	}
	if _, ok := index.Resolve("item.internal"); ok {
		t.Fatal("reviewed internal command was emitted by the assembler")
	}
}

func TestCommandRegistrySourceValidationIsFailClosed(t *testing.T) {
	registry, err := ValidateCommandRegistrySource(embeddedSchemaCommandRegistryJSON)
	if err != nil {
		t.Fatalf("ValidateCommandRegistrySource(embedded) error = %v", err)
	}
	hash, err := EmbeddedCommandRegistrySourceHash()
	if err != nil {
		t.Fatalf("EmbeddedCommandRegistrySourceHash() error = %v", err)
	}
	if hash == "" || hash != registry.SourceHash() {
		t.Fatalf("embedded hash = %q, decoded hash = %q", hash, registry.SourceHash())
	}

	drifted := strings.Replace(string(embeddedSchemaCommandRegistryJSON), `"cli_path": "aisearch person"`, `"cli_path": "aisearch people"`, 1)
	if drifted == string(embeddedSchemaCommandRegistryJSON) {
		t.Fatal("test fixture did not mutate embedded registry")
	}
	_, err = ValidateCommandRegistrySource([]byte(drifted))
	if err == nil || !strings.Contains(err.Error(), "disagrees with the embedded reviewed registry") {
		t.Fatalf("drift error = %v", err)
	}

	unknownField := strings.Replace(string(embeddedSchemaCommandRegistryJSON), `"version": 1`, `"version": 1, "unreviewed": true`, 1)
	_, err = ValidateCommandRegistrySource([]byte(unknownField))
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown-field error = %v", err)
	}
}

func commandRegistryTestRoot(paths ...string) *cobra.Command {
	root := &cobra.Command{Use: "dws"}
	groups := map[string]*cobra.Command{}
	for _, path := range paths {
		parts := strings.Fields(path)
		current := root
		prefix := ""
		for idx, part := range parts {
			if prefix == "" {
				prefix = part
			} else {
				prefix += " " + part
			}
			if idx == len(parts)-1 {
				current.AddCommand(&cobra.Command{Use: part, Run: func(*cobra.Command, []string) {}})
				continue
			}
			next := groups[prefix]
			if next == nil {
				next = &cobra.Command{Use: part}
				groups[prefix] = next
				current.AddCommand(next)
			}
			current = next
		}
	}
	return root
}

func mustCommandRegistry(t *testing.T, commands []CommandSpec) CommandRegistry {
	t.Helper()
	registry, err := newCommandRegistry(commands)
	if err != nil {
		t.Fatalf("newCommandRegistry() error = %v", err)
	}
	return registry
}

func mustEffectiveCommandRegistry(t *testing.T, commands []CommandSpec) EffectiveCommandRegistry {
	t.Helper()
	registry, err := newEffectiveCommandRegistry(commands)
	if err != nil {
		t.Fatalf("newEffectiveCommandRegistry() error = %v", err)
	}
	return registry
}

func annotateTestCompatibilityPair(primary, alias *cobra.Command) {
	AnnotateRuntimeCompatibilityEquivalence(primary, alias, RuntimeCompatibilityEquivalence{
		ID:       "test-compatibility-pair-v1",
		Reason:   "The focused test explicitly reviews these independently registered leaves as semantically equivalent.",
		Reviewed: true,
	})
}
