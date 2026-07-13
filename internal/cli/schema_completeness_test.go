// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"reflect"
	"testing"

	"github.com/spf13/cobra"
)

func TestRuntimeSchemaCompletenessRequiresCoverageOrReviewedExclusion(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	covered := &cobra.Command{Use: "covered", Run: func(*cobra.Command, []string) {}}
	missing := &cobra.Command{Use: "missing", Run: func(*cobra.Command, []string) {}}
	excluded := &cobra.Command{Use: "excluded", Run: func(*cobra.Command, []string) {}}
	hidden := &cobra.Command{Use: "hidden", Hidden: true, Run: func(*cobra.Command, []string) {}}
	AttachRuntimeSchema(covered, "sample", "covered", "test")
	root.AddCommand(covered, missing, excluded, hidden)

	report := RuntimeSchemaCompleteness(root, []RuntimeSchemaExclusion{{
		CLIPath: "excluded", Reason: "reviewed test exclusion", Reviewed: true,
	}})
	if !reflect.DeepEqual(report.Covered, []string{"covered"}) {
		t.Fatalf("covered = %v", report.Covered)
	}
	if !reflect.DeepEqual(report.Excluded, []string{"excluded"}) {
		t.Fatalf("excluded = %v", report.Excluded)
	}
	if !reflect.DeepEqual(report.Missing, []string{"missing"}) {
		t.Fatalf("missing = %v", report.Missing)
	}
	if len(report.InvalidExclusions) != 0 || len(report.StaleExclusions) != 0 {
		t.Fatalf("invalid=%v stale=%v", report.InvalidExclusions, report.StaleExclusions)
	}
}

func TestRuntimeSchemaCompletenessRejectsInvalidAndStaleExclusions(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	current := &cobra.Command{Use: "current", Run: func(*cobra.Command, []string) {}}
	covered := &cobra.Command{Use: "covered", Run: func(*cobra.Command, []string) {}}
	AttachRuntimeSchema(covered, "sample", "covered", "test")
	root.AddCommand(current, covered)
	report := RuntimeSchemaCompleteness(root, []RuntimeSchemaExclusion{
		{CLIPath: "current", Reason: "", Reviewed: true},
		{CLIPath: "stale", Reason: "reviewed but obsolete", Reviewed: true},
		{CLIPath: "covered", Reason: "no longer needed", Reviewed: true},
	})
	if !reflect.DeepEqual(report.InvalidExclusions, []string{"current"}) {
		t.Fatalf("invalid = %v", report.InvalidExclusions)
	}
	if !reflect.DeepEqual(report.StaleExclusions, []string{"covered", "stale"}) {
		t.Fatalf("stale = %v", report.StaleExclusions)
	}
	if !reflect.DeepEqual(report.Missing, []string{"current"}) {
		t.Fatalf("missing = %v", report.Missing)
	}
}

func TestSchemaCatalogDeliveryCompletenessRejectsAnnotatedLeafOutsideSurface(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	product := &cobra.Command{Use: "sample"}
	covered := &cobra.Command{Use: "covered", Run: func(*cobra.Command, []string) {}}
	omitted := &cobra.Command{Use: "omitted", Run: func(*cobra.Command, []string) {}}
	AttachRuntimeSchema(covered, "sample", "covered", "test")
	AttachRuntimeSchema(omitted, "sample", "omitted", "test")
	product.AddCommand(covered, omitted)
	root.AddCommand(product)

	annotationReport := RuntimeSchemaCompleteness(root, nil)
	if !reflect.DeepEqual(annotationReport.Covered, []string{"sample covered", "sample omitted"}) || len(annotationReport.Missing) != 0 {
		t.Fatalf("annotation report = %#v", annotationReport)
	}

	snapshot, err := BuildSchemaCatalogSnapshot(root, SchemaCatalogBuildOptions{
		AllowedCanonicalPaths: map[string]bool{"sample.covered": true},
	})
	if err != nil {
		t.Fatalf("BuildSchemaCatalogSnapshot: %v", err)
	}
	report := SchemaCatalogDeliveryCompleteness(root, snapshot, nil)
	if !reflect.DeepEqual(report.Covered, []string{"sample covered"}) {
		t.Fatalf("covered = %v", report.Covered)
	}
	if !reflect.DeepEqual(report.Missing, []string{"sample omitted"}) {
		t.Fatalf("missing = %v", report.Missing)
	}
}

func TestSchemaCatalogDeliveryCompletenessAcceptsExactReviewedExclusion(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	product := &cobra.Command{Use: "sample"}
	covered := &cobra.Command{Use: "covered", Run: func(*cobra.Command, []string) {}}
	excluded := &cobra.Command{Use: "excluded", Run: func(*cobra.Command, []string) {}}
	product.AddCommand(covered, excluded)
	root.AddCommand(product)

	snapshot := SchemaCatalogSnapshot{Tools: map[string]map[string]any{
		"sample.covered": {"primary_cli_path": "sample covered"},
	}}
	report := SchemaCatalogDeliveryCompleteness(root, snapshot, []RuntimeSchemaExclusion{{
		CLIPath: "sample excluded", Reason: "reviewed test exclusion", Reviewed: true,
	}})
	if !reflect.DeepEqual(report.Covered, []string{"sample covered"}) ||
		!reflect.DeepEqual(report.Excluded, []string{"sample excluded"}) ||
		len(report.Missing) != 0 || len(report.StaleExclusions) != 0 {
		t.Fatalf("delivery report = %#v", report)
	}
}

func TestSchemaCatalogDeliveryCompletenessAcceptsDeliveredAlias(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	product := &cobra.Command{Use: "sample"}
	legacy := &cobra.Command{Use: "legacy", Run: func(*cobra.Command, []string) {}}
	product.AddCommand(legacy)
	root.AddCommand(product)

	snapshot := SchemaCatalogSnapshot{Tools: map[string]map[string]any{
		"sample.current": {
			"primary_cli_path": "sample current",
			"aliases":          []string{"sample legacy"},
		},
	}}
	report := SchemaCatalogDeliveryCompleteness(root, snapshot, nil)
	if !reflect.DeepEqual(report.Covered, []string{"sample legacy"}) || len(report.Missing) != 0 {
		t.Fatalf("delivery report = %#v", report)
	}
}
