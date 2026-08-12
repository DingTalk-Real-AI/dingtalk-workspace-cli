// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"fmt"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageCompletenessAndDeliveryErrorBranches(t *testing.T) {
	prevGroups := reviewedRuntimeSchemaExclusionGroups
	t.Cleanup(func() { reviewedRuntimeSchemaExclusionGroups = prevGroups })

	reviewedRuntimeSchemaExclusionGroups = []runtimeSchemaExclusionGroup{{
		ID: "", Reason: "x", Reviewed: true, Commands: []string{"x"},
	}}
	if _, err := ReviewedRuntimeSchemaExclusions(); err == nil || !strings.Contains(err.Error(), "not reviewed") {
		t.Fatalf("blank group id error = %v", err)
	}
	reviewedRuntimeSchemaExclusionGroups = []runtimeSchemaExclusionGroup{{
		ID: "g", Reason: "reason", Reviewed: true, Commands: []string{" ", "ok"},
	}}
	if _, err := ReviewedRuntimeSchemaExclusions(); err == nil || !strings.Contains(err.Error(), "empty command") {
		t.Fatalf("empty command error = %v", err)
	}
	reviewedRuntimeSchemaExclusionGroups = []runtimeSchemaExclusionGroup{{
		ID: "g", Reason: "reason", Reviewed: true, Commands: []string{"dup", "dup"},
	}}
	if _, err := ReviewedRuntimeSchemaExclusions(); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate exclusion error = %v", err)
	}
	reviewedRuntimeSchemaExclusionGroups = prevGroups

	prevBuild := completenessBuildEffective
	prevBind := completenessBindEffective
	prevLoad := completenessLoadExclusions
	prevRuntime := completenessRuntimeReport
	prevDelivery := completenessDeliveryReport
	t.Cleanup(func() {
		completenessBuildEffective = prevBuild
		completenessBindEffective = prevBind
		completenessLoadExclusions = prevLoad
		completenessRuntimeReport = prevRuntime
		completenessDeliveryReport = prevDelivery
	})

	root := &cobra.Command{Use: "dws"}
	completenessBuildEffective = func(*cobra.Command) (EffectiveCommandRegistry, error) {
		return EffectiveCommandRegistry{}, fmt.Errorf("build boom")
	}
	if err := ValidateRuntimeSchemaCompleteness(root); err == nil || !strings.Contains(err.Error(), "build boom") {
		t.Fatalf("build error = %v", err)
	}
	completenessBuildEffective = func(*cobra.Command) (EffectiveCommandRegistry, error) {
		return EffectiveCommandRegistry{}, nil
	}
	completenessBindEffective = func(*cobra.Command, EffectiveCommandRegistry) (BoundCommandRegistry, error) {
		return BoundCommandRegistry{}, fmt.Errorf("bind boom")
	}
	if err := ValidateRuntimeSchemaCompleteness(root); err == nil || !strings.Contains(err.Error(), "bind boom") {
		t.Fatalf("bind error = %v", err)
	}
	completenessBindEffective = func(*cobra.Command, EffectiveCommandRegistry) (BoundCommandRegistry, error) {
		return BoundCommandRegistry{}, nil
	}
	completenessLoadExclusions = func() ([]RuntimeSchemaExclusion, error) {
		return nil, fmt.Errorf("excl boom")
	}
	if err := validateResolvedRuntimeSchemaCompleteness(root, BoundCommandRegistry{}); err == nil || !strings.Contains(err.Error(), "excl boom") {
		t.Fatalf("excl error = %v", err)
	}
	completenessLoadExclusions = func() ([]RuntimeSchemaExclusion, error) { return nil, nil }
	completenessRuntimeReport = func(*cobra.Command, []RuntimeSchemaExclusion, BoundCommandRegistry) RuntimeSchemaCompletenessReport {
		return RuntimeSchemaCompletenessReport{DeliveryErrors: []string{"delivery"}}
	}
	if err := validateResolvedRuntimeSchemaCompleteness(root, BoundCommandRegistry{}); err == nil || !strings.Contains(err.Error(), "delivery") {
		t.Fatalf("delivery report error = %v", err)
	}
	completenessRuntimeReport = func(*cobra.Command, []RuntimeSchemaExclusion, BoundCommandRegistry) RuntimeSchemaCompletenessReport {
		return RuntimeSchemaCompletenessReport{Missing: []string{"m"}}
	}
	if err := validateResolvedRuntimeSchemaCompleteness(root, BoundCommandRegistry{}); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("missing report error = %v", err)
	}
	completenessRuntimeReport = func(*cobra.Command, []RuntimeSchemaExclusion, BoundCommandRegistry) RuntimeSchemaCompletenessReport {
		return RuntimeSchemaCompletenessReport{InvalidExclusions: []string{"i"}}
	}
	if err := validateResolvedRuntimeSchemaCompleteness(root, BoundCommandRegistry{}); err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("invalid report error = %v", err)
	}
	completenessRuntimeReport = func(*cobra.Command, []RuntimeSchemaExclusion, BoundCommandRegistry) RuntimeSchemaCompletenessReport {
		return RuntimeSchemaCompletenessReport{StaleExclusions: []string{"s"}}
	}
	if err := validateResolvedRuntimeSchemaCompleteness(root, BoundCommandRegistry{}); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("stale report error = %v", err)
	}

	completenessLoadExclusions = func() ([]RuntimeSchemaExclusion, error) {
		return nil, fmt.Errorf("delivery excl boom")
	}
	if err := validateResolvedSchemaCatalogDeliveryCompleteness(root, BoundCommandRegistry{}, SchemaCatalogSnapshot{}); err == nil || !strings.Contains(err.Error(), "delivery excl boom") {
		t.Fatalf("delivery excl error = %v", err)
	}
	completenessLoadExclusions = func() ([]RuntimeSchemaExclusion, error) { return nil, nil }
	prevIface := loadCatalogValidateInterfaces
	prevProv := loadCatalogValidateProvenance
	t.Cleanup(func() {
		loadCatalogValidateInterfaces = prevIface
		loadCatalogValidateProvenance = prevProv
	})
	loadCatalogValidateInterfaces = func(SchemaRegistry) error { return nil }
	loadCatalogValidateProvenance = func(SchemaRegistry) error { return nil }
	registry := SchemaRegistry{
		Source: SchemaSourceRuntimeAssembled,
		Products: []ProductSpec{{
			ID: "sample",
			Tools: []ToolSpec{{
				Identity: contract.ToolIdentitySpec{
					CLIPath: "sample run", CanonicalPath: "sample.run", ProductID: "sample",
					Name: "run", Path: "sample.run", PrimaryCLIPath: "sample run",
				},
				Safety:    contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "yes"},
				Selection: contract.SelectionSpec{AgentSummary: "s"},
			}},
		}},
	}
	payload, err := registry.ToSnapshotPayload()
	if err != nil {
		t.Fatal(err)
	}
	snapshot := SchemaCatalogSnapshot{
		Version: SchemaCatalogSnapshotVersion,
		Catalog: payload.Catalog,
		Tools:   payload.Tools,
	}
	snapshot.SourceHash = schemaCatalogSnapshotHash(snapshot)
	for _, tc := range []struct {
		name   string
		report RuntimeSchemaCompletenessReport
		want   string
	}{
		{"snap", RuntimeSchemaCompletenessReport{DeliveryErrors: []string{"snap"}}, "snap"},
		{"missing", RuntimeSchemaCompletenessReport{Missing: []string{"m"}}, "missing"},
		{"invalid", RuntimeSchemaCompletenessReport{InvalidExclusions: []string{"i"}}, "invalid"},
		{"stale", RuntimeSchemaCompletenessReport{StaleExclusions: []string{"s"}}, "stale"},
	} {
		completenessDeliveryReport = func(*cobra.Command, loadedSchemaCatalog, []RuntimeSchemaExclusion, BoundCommandRegistry) RuntimeSchemaCompletenessReport {
			return tc.report
		}
		if err := validateSchemaCatalogDeliveryCompletenessFromBound(root, BoundCommandRegistry{}, snapshot, nil); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s error = %v", tc.name, err)
		}
	}
}
