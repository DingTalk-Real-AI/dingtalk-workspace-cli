// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package agentmetadata

import (
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
)

func TestApplyContractFinalProductDeclarations(t *testing.T) {
	t.Cleanup(func() { contract.ClearProductDeclForTest("sample") })
	contract.RegisterProductDecl(contract.ProductDecl{
		ID: "sample",
		Selection: contract.ProductSelectionDecl{
			AgentSummary: "Manage samples",
			UseWhen:      []string{"target is a sample"},
			AvoidWhen:    []string{"target is another product"},
		},
	})

	file := &File{Products: map[string]ProductMetadata{}}
	if err := applyContractFinalProductDeclarations(file, Options{
		ProductIDs: map[string]bool{"sample": true},
	}); err != nil {
		t.Fatalf("applyContractFinalProductDeclarations: %v", err)
	}
	metadata, ok := file.Products["sample"]
	if !ok {
		t.Fatal("missing product after ProductDecl apply")
	}
	if metadata.AgentSummary != "Manage samples" || metadata.agentSummaryRank != selectionRankContractFinal {
		t.Fatalf("agent_summary = %#v rank=%d", metadata.AgentSummary, metadata.agentSummaryRank)
	}
	if metadata.useWhenRank != selectionRankContractFinal || metadata.avoidWhenRank != selectionRankContractFinal {
		t.Fatalf("list ranks use=%d avoid=%d", metadata.useWhenRank, metadata.avoidWhenRank)
	}
	if metadata.AgentSummarySource != contract.ProductDeclSourceRef {
		t.Fatalf("AgentSummarySource = %q", metadata.AgentSummarySource)
	}
	normalizeFile(file, 0)
	metadata = file.Products["sample"]
	for _, field := range []string{"agent_summary", "use_when", "avoid_when"} {
		prov, ok := metadata.FieldProvenance[field]
		if !ok || prov.Precedence != selectionPrecedenceContractFinal || prov.Source != productDeclOrigin {
			t.Fatalf("%s provenance = %#v", field, prov)
		}
	}

	// Surface filter: Decl outside ProductIDs must not materialize.
	file = &File{Products: map[string]ProductMetadata{}}
	if err := applyContractFinalProductDeclarations(file, Options{
		ProductIDs: map[string]bool{"other": true},
	}); err != nil {
		t.Fatalf("filtered apply: %v", err)
	}
	if _, ok := file.Products["sample"]; ok {
		t.Fatal("ProductDecl outside surface must not be applied")
	}

	if err := applyContractFinalProductDeclarations(nil, Options{}); err != nil {
		t.Fatalf("nil file: %v", err)
	}
}

func TestApplyContractFinalDeclarationsIncludesProducts(t *testing.T) {
	t.Cleanup(func() { contract.ClearProductDeclForTest("wired") })
	contract.RegisterProductDecl(contract.ProductDecl{
		ID: "wired",
		Selection: contract.ProductSelectionDecl{
			AgentSummary: "Wired product",
			UseWhen:      []string{"use wired"},
			AvoidWhen:    []string{"avoid wired"},
		},
	})
	file := &File{Products: map[string]ProductMetadata{}, Tools: map[string]ToolMetadata{}}
	if err := applyContractFinalDeclarations(file, Options{ProductIDs: map[string]bool{"wired": true}}); err != nil {
		t.Fatalf("applyContractFinalDeclarations: %v", err)
	}
	if summary := file.Products["wired"].AgentSummary; summary != "Wired product" {
		t.Fatalf("wired summary = %q", summary)
	}
	if err := applyContractFinalDeclarations(nil, Options{}); err != nil {
		t.Fatalf("nil file: %v", err)
	}
}
