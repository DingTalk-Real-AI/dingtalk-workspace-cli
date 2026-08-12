// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageIdentityCollectEdges(t *testing.T) {
	if _, err := BuildEffectiveFromSpecs(nil); err != nil {
		t.Fatalf("BuildEffectiveFromSpecs(nil) error = %v", err)
	}
	if got := primaryCLIPathFromIdentity(contract.ToolIdentitySpec{CLIPath: "doc create"}); got != "doc create" {
		t.Fatalf("primaryCLIPathFromIdentity fill = %q", got)
	}
	if got := primaryCLIPathFromIdentity(contract.ToolIdentitySpec{
		PrimaryCLIPath: "doc make",
		CLIPath:        "doc create",
	}); got != "doc make" {
		t.Fatalf("primaryCLIPathFromIdentity primary wins = %q", got)
	}

	walkIdentityLeaves(nil, func(*cobra.Command) { t.Fatal("nil root must not walk") })
	helpRoot := &cobra.Command{Use: "root"}
	helpRoot.AddCommand(&cobra.Command{Use: "help", Run: func(*cobra.Command, []string) {}})
	walked := 0
	walkIdentityLeaves(helpRoot, func(*cobra.Command) { walked++ })
	if walked != 0 {
		t.Fatalf("help child must be skipped, walked=%d", walked)
	}

	if _, _, err := CollectIdentitySpecs(nil); err == nil || !strings.Contains(err.Error(), "root is nil") {
		t.Fatalf("CollectIdentitySpecs(nil) error = %v", err)
	}

	if err := validateCollectedUniqueness([]CommandSpec{
		{CanonicalPath: "a.run", PrimaryCLIPath: "a run"},
		{CanonicalPath: "a.run", PrimaryCLIPath: "a other"},
	}); err == nil || !strings.Contains(err.Error(), "duplicate canonical") {
		t.Fatalf("duplicate canonical error = %v", err)
	}
	if err := validateCollectedUniqueness([]CommandSpec{
		{CanonicalPath: "a.run", PrimaryCLIPath: "shared"},
		{CanonicalPath: "b.run", PrimaryCLIPath: "shared"},
	}); err == nil || !strings.Contains(err.Error(), "duplicate primary") {
		t.Fatalf("duplicate primary error = %v", err)
	}
	if err := validateCollectedUniqueness([]CommandSpec{
		{CanonicalPath: "b.run", PrimaryCLIPath: "shared"},
		{CanonicalPath: "a.run", PrimaryCLIPath: "a run", Aliases: []string{"shared"}},
	}); err == nil || !strings.Contains(err.Error(), "collides with a primary") {
		t.Fatalf("alias/primary collision during scan error = %v", err)
	}
	if err := validateCollectedUniqueness([]CommandSpec{
		{CanonicalPath: "a.run", PrimaryCLIPath: "a run", Aliases: []string{"shared"}},
		{CanonicalPath: "b.run", PrimaryCLIPath: "shared"},
	}); err == nil || !strings.Contains(err.Error(), "collides with a primary") {
		t.Fatalf("alias/primary collision during final check error = %v", err)
	}
	if err := validateCollectedUniqueness([]CommandSpec{
		{CanonicalPath: "a.run", PrimaryCLIPath: "a run", Aliases: []string{"twin"}},
		{CanonicalPath: "b.run", PrimaryCLIPath: "b run", Aliases: []string{"twin"}},
	}); err == nil || !strings.Contains(err.Error(), "declared by both") {
		t.Fatalf("alias/alias collision error = %v", err)
	}
	if err := validateCollectedUniqueness([]CommandSpec{
		{CanonicalPath: "a.run", PrimaryCLIPath: "a run", Aliases: []string{"", "a alias"}},
	}); err != nil {
		t.Fatalf("empty alias skip error = %v", err)
	}

	root := &cobra.Command{Use: "dws"}
	leaf := &cobra.Command{Use: "run", Run: func(*cobra.Command, []string) {}}
	dup := &cobra.Command{Use: "alias-run", Run: func(*cobra.Command, []string) {}}
	root.AddCommand(&cobra.Command{Use: "sample", Run: nil})
	sample := root.Commands()[0]
	sample.AddCommand(leaf)
	sample.AddCommand(dup)
	identity := contract.ContractFinalPayload{
		Identity: &contract.ToolIdentitySpec{
			ProductID: "sample", Name: "run", CanonicalPath: "sample.run",
			CLIPath: "sample run", PrimaryCLIPath: "sample run",
		},
	}
	contractfinal.RegisterRuntimeContractFinal(leaf, identity)
	contractfinal.RegisterRuntimeContractFinal(dup, identity)
	t.Cleanup(func() {
		ClearRuntimeContractFinalForTest(leaf)
		ClearRuntimeContractFinalForTest(dup)
	})
	if _, _, err := CollectIdentitySpecs(root); err == nil || !strings.Contains(err.Error(), "duplicate canonical") {
		t.Fatalf("CollectIdentitySpecs uniqueness error = %v", err)
	}
	ClearRuntimeContractFinalForTest(dup)

	diagnostics := DiagnoseMissingPrimaries(root, []CommandSpec{
		{CanonicalPath: "missing.run", PrimaryCLIPath: "missing path"},
		{CanonicalPath: "sample.other", PrimaryCLIPath: "sample run"},
	})
	if len(diagnostics) < 2 {
		t.Fatalf("DiagnoseMissingPrimaries = %#v", diagnostics)
	}

	bare := &cobra.Command{Use: "bare", Run: func(*cobra.Command, []string) {}}
	root.AddCommand(bare)
	diagnostics = DiagnoseMissingPrimaries(root, []CommandSpec{
		{CanonicalPath: "bare.run", PrimaryCLIPath: "bare"},
	})
	if len(diagnostics) != 1 || !strings.Contains(diagnostics[0], "no ContractFinal.Identity") {
		t.Fatalf("DiagnoseMissingPrimaries bare = %#v", diagnostics)
	}

	problems := CompareCommandSpecEquivalence(
		[]CommandSpec{{
			CanonicalPath: "a.run", PrimaryCLIPath: "a run", SourceProductID: "a",
			Source: "s1", Visibility: SchemaVisibilityPublic, Aliases: []string{"a old"},
		}},
		[]CommandSpec{
			{
				CanonicalPath: "a.run", PrimaryCLIPath: "a primary", SourceProductID: "b",
				Source: "s2", Visibility: SchemaVisibilityInternal, Aliases: []string{"a alias"},
			},
			{CanonicalPath: "b.run", PrimaryCLIPath: "b run"},
		},
	)
	joined := strings.Join(problems, "\n")
	for _, needle := range []string{
		"missing from collected: b.run",
		"primary_cli_path",
		"source_product_id",
		"source:",
		"visibility",
		"aliases:",
	} {
		if !strings.Contains(joined, needle) {
			t.Fatalf("CompareCommandSpecEquivalence missing %q in %#v", needle, problems)
		}
	}
	if problems := CompareCommandSpecEquivalence(
		[]CommandSpec{{CanonicalPath: "extra.run", PrimaryCLIPath: "extra"}},
		nil,
	); len(problems) != 1 || !strings.Contains(problems[0], "extra in collected") {
		t.Fatalf("extra collected problems = %#v", problems)
	}
}
