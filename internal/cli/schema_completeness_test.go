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
