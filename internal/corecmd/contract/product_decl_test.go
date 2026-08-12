// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package contract

import "testing"

func TestCrossPlatformCoverageProductDeclRegistryRoundTrip(t *testing.T) {
	t.Cleanup(func() { ClearProductDeclForTest("sample") })
	ClearProductDeclForTest("sample")

	if HasProductDecl("sample") {
		t.Fatal("HasProductDecl before register must be false")
	}
	RegisterProductDecl(ProductDecl{})
	if HasProductDecl("") {
		t.Fatal("empty ID must not register")
	}

	RegisterProductDecl(ProductDecl{
		ID: " sample ",
		Selection: ProductSelectionDecl{
			AgentSummary: "Manage samples",
			UseWhen:      []string{"target is a sample"},
			AvoidWhen:    []string{"target is another product"},
		},
	})
	if !HasProductDecl("sample") {
		t.Fatal("HasProductDecl after register must be true")
	}
	got, ok := LookupProductDecl("sample")
	if !ok || got.ID != "sample" || got.Selection.AgentSummary != "Manage samples" {
		t.Fatalf("LookupProductDecl = %#v, ok=%v", got, ok)
	}
	ids := RegisteredProductDeclIDs()
	found := false
	for _, id := range ids {
		if id == "sample" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("RegisteredProductDeclIDs missing sample: %#v", ids)
	}

	selection, provenance := ProductSelectionFromDecl(got)
	if selection.AgentSummary != "Manage samples" || selection.AgentSummarySource != ProductDeclSourceRef {
		t.Fatalf("ProductSelectionFromDecl selection = %#v", selection)
	}
	for _, field := range []string{"agent_summary", "use_when", "avoid_when"} {
		prov, ok := provenance[field]
		if !ok || prov.Precedence != "contract_final" || prov.Source != ProductDeclProvenanceSource {
			t.Fatalf("field %s provenance = %#v", field, prov)
		}
	}

	ClearProductDeclForTest("sample")
	if HasProductDecl("sample") {
		t.Fatal("ClearProductDeclForTest must remove registration")
	}
}

func TestCrossPlatformCoverageProductDeclLookupRejectsWrongType(t *testing.T) {
	t.Cleanup(func() { productDecls.Delete("broken-type") })
	productDecls.Store("broken-type", "not-a-product-decl")
	if _, ok := LookupProductDecl("broken-type"); ok {
		t.Fatal("LookupProductDecl must reject non-ProductDecl values")
	}
}

func TestCrossPlatformCoverageProductDeclRegisterPanicsOnIncompleteSelection(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for incomplete ProductDecl")
		}
	}()
	RegisterProductDecl(ProductDecl{ID: "broken"})
}
