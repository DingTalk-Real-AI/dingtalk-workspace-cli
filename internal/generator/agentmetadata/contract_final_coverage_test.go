// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package agentmetadata

import (
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageContractFinalNilAndEmptyGuards(t *testing.T) {
	if err := applyContractFinalDeclarations(nil, Options{}); err != nil {
		t.Fatalf("nil file: %v", err)
	}
	if err := applyContractFinalProductDeclarations(nil, Options{}); err != nil {
		t.Fatalf("nil product file: %v", err)
	}
	file := &File{Tools: map[string]ToolMetadata{}}
	if err := applyContractFinalDeclarations(file, Options{}); err != nil {
		t.Fatalf("empty ByCanonical: %v", err)
	}
}

func TestCrossPlatformCoverageContractFinalSkipsWithoutOverlay(t *testing.T) {
	plain := &cobra.Command{Use: "plain"}
	file := &File{Tools: map[string]ToolMetadata{"sample run": {AgentSummary: "existing"}}}
	if err := applyContractFinalDeclarations(file, Options{
		BoundCommands: cli.BoundCommandRegistry{ByCanonical: map[string]cli.BoundCommandSpec{
			"sample.run": {PrimaryCommand: plain},
		}},
		CanonicalToolPaths: map[string]string{"sample.run": "sample run"},
	}); err != nil {
		t.Fatalf("no overlay: %v", err)
	}
	if file.Tools["sample run"].AgentSummary != "existing" {
		t.Fatalf("tool = %#v", file.Tools["sample run"])
	}
}

func TestCrossPlatformCoverageContractFinalMissingCanonicalProjection(t *testing.T) {
	declared := &cobra.Command{Use: "run"}
	contractfinal.RegisterRuntimeContractFinal(declared, contract.ContractFinalPayload{
		Selection: &contract.SelectionSpec{
			AgentSummary: "Declared",
			UseWhen:      []string{"use"},
			AvoidWhen:    []string{"avoid"},
			Examples:     []string{"dws sample run"},
		},
	})
	t.Cleanup(func() { contractfinal.ClearRuntimeContractFinalForTest(declared) })

	err := applyContractFinalDeclarations(&File{Tools: map[string]ToolMetadata{}}, Options{
		BoundCommands: cli.BoundCommandRegistry{ByCanonical: map[string]cli.BoundCommandSpec{
			"sample.run": {PrimaryCommand: declared},
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "no canonical CLI projection") {
		t.Fatalf("missing projection error = %v", err)
	}
}

func TestCrossPlatformCoverageContractFinalProductDeclFilterAndLookupSkip(t *testing.T) {
	t.Cleanup(func() {
		contract.ClearProductDeclForTest("filtered")
		contract.ClearProductDeclForTest("broken-type")
	})
	contract.RegisterProductDecl(contract.ProductDecl{
		ID: "filtered",
		Selection: contract.ProductSelectionDecl{
			AgentSummary: "Filtered product",
			UseWhen:      []string{"use filtered"},
			AvoidWhen:    []string{"avoid filtered"},
		},
	})
	contract.StoreProductDeclRawForTest("broken-type", "not-a-product-decl")

	file := &File{}
	if err := applyContractFinalProductDeclarations(file, Options{
		ProductIDs: map[string]bool{"other": true},
	}); err != nil {
		t.Fatalf("filtered products: %v", err)
	}
	if _, ok := file.Products["filtered"]; ok {
		t.Fatal("filtered product must be skipped when not in ProductIDs")
	}

	if err := applyContractFinalProductDeclarations(&File{}, Options{}); err != nil {
		t.Fatalf("broken lookup skip: %v", err)
	}
}

func TestCrossPlatformCoverageContractFinalProductDeclMergeConflicts(t *testing.T) {
	t.Cleanup(func() { contract.ClearProductDeclForTest("conflict") })
	contract.RegisterProductDecl(contract.ProductDecl{
		ID: "conflict",
		Selection: contract.ProductSelectionDecl{
			AgentSummary: "Declared summary",
			UseWhen:      []string{"use declared"},
			AvoidWhen:    []string{"avoid declared"},
		},
	})
	file := &File{Products: map[string]ProductMetadata{
		"conflict": {
			AgentSummary: "Existing summary", agentSummaryPresent: true,
			agentSummaryRank: selectionRankContractFinal, agentSummaryOrigin: productDeclOrigin,
		},
	}}
	// Hit applyContractFinalDeclarations's product-decl error return (not only
	// the helper) so platform -run coverage counts the wrapper statement.
	if err := applyContractFinalDeclarations(file, Options{}); err == nil ||
		!strings.Contains(err.Error(), "agent_summary") {
		t.Fatalf("summary conflict error = %v", err)
	}

	file = &File{Products: map[string]ProductMetadata{
		"conflict": {
			UseWhen: []string{"existing"}, useWhenPresent: true,
			useWhenRank: selectionRankContractFinal, useWhenOrigin: productDeclOrigin,
		},
	}}
	if err := applyContractFinalProductDeclarations(file, Options{}); err == nil ||
		!strings.Contains(err.Error(), "use_when") {
		t.Fatalf("use_when conflict error = %v", err)
	}

	file = &File{Products: map[string]ProductMetadata{
		"conflict": {
			AvoidWhen: []string{"existing"}, avoidWhenPresent: true,
			avoidWhenRank: selectionRankContractFinal, avoidWhenOrigin: productDeclOrigin,
		},
	}}
	if err := applyContractFinalProductDeclarations(file, Options{}); err == nil ||
		!strings.Contains(err.Error(), "avoid_when") {
		t.Fatalf("avoid_when conflict error = %v", err)
	}
}

func TestCrossPlatformCoverageContractFinalToolMergeConflict(t *testing.T) {
	declared := &cobra.Command{Use: "run"}
	contractfinal.RegisterRuntimeContractFinal(declared, contract.ContractFinalPayload{
		Selection: &contract.SelectionSpec{
			AgentSummary: "Declared summary",
			UseWhen:      []string{"use declared"},
			AvoidWhen:    []string{"avoid declared"},
			Examples:     []string{"dws sample run"},
		},
	})
	t.Cleanup(func() { contractfinal.ClearRuntimeContractFinalForTest(declared) })

	file := &File{Tools: map[string]ToolMetadata{
		"sample run": {
			AgentSummary: "Existing", agentSummaryPresent: true,
			agentSummaryRank: selectionRankContractFinal, agentSummaryOrigin: contractFinalOrigin,
		},
	}}
	if err := applyContractFinalDeclarations(file, Options{
		BoundCommands: cli.BoundCommandRegistry{ByCanonical: map[string]cli.BoundCommandSpec{
			"sample.run": {PrimaryCommand: declared},
		}},
		CanonicalToolPaths: map[string]string{"sample.run": "sample run"},
	}); err == nil || !strings.Contains(err.Error(), "agent_summary") {
		t.Fatalf("tool merge conflict error = %v", err)
	}
}
