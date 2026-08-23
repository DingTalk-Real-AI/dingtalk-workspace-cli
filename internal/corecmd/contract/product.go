// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package contract

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// ProductDecl provenance labels. Assembly and Agent-metadata generation stamp
// ProductSpec.Selection winners with these sources (symmetric to leaf
// corecmd.contract / corecmd.ContractDecl).
const (
	ProductDeclProvenanceSource = "cli.product_decl"
	ProductDeclSourceRef        = "cli.ProductDecl"
)

// ProductSelectionDecl is the product-level Agent routing prose declared in
// code. Fields mirror the leaf SelectionSpec routing triple: agent summary,
// use-when, and avoid-when.
type ProductSelectionDecl struct {
	AgentSummary string
	UseWhen      []string
	AvoidWhen    []string
}

// ProductDecl is the product-level Schema routing declaration. Assembly writes
// ProductSpec.Selection with provenance contract_final.
type ProductDecl struct {
	ID        string
	Selection ProductSelectionDecl
}

var productDecls sync.Map // productID → ProductDecl

// RegisterProductDecl stores a product-level routing declaration.
// Light runtime write: one map store; no JSON bridge. A non-empty ID with
// incomplete selection panics: declared products are the final source and
// have no selection/ fallback for missing prose.
func RegisterProductDecl(decl ProductDecl) {
	productID := strings.TrimSpace(decl.ID)
	if productID == "" {
		return
	}
	missing := make([]string, 0, 3)
	if strings.TrimSpace(decl.Selection.AgentSummary) == "" {
		missing = append(missing, "Selection.AgentSummary")
	}
	if len(decl.Selection.UseWhen) == 0 {
		missing = append(missing, "Selection.UseWhen")
	}
	if len(decl.Selection.AvoidWhen) == 0 {
		missing = append(missing, "Selection.AvoidWhen")
	}
	if len(missing) > 0 {
		panic(fmt.Sprintf(
			"product %q ProductDecl is missing %s: a declared product is the final routing source and must carry full selection prose",
			productID, strings.Join(missing, ", ")))
	}
	decl.ID = productID
	productDecls.Store(productID, decl)
}

// LookupProductDecl returns the registered product declaration, if any.
func LookupProductDecl(productID string) (ProductDecl, bool) {
	productID = strings.TrimSpace(productID)
	if productID == "" {
		return ProductDecl{}, false
	}
	raw, ok := productDecls.Load(productID)
	if !ok {
		return ProductDecl{}, false
	}
	decl, ok := raw.(ProductDecl)
	if !ok {
		return ProductDecl{}, false
	}
	return decl, true
}

// HasProductDecl reports whether product-level routing is declared in code.
func HasProductDecl(productID string) bool {
	_, ok := LookupProductDecl(productID)
	return ok
}

// RegisteredProductDeclIDs returns sorted product IDs with an in-code Decl.
func RegisteredProductDeclIDs() []string {
	ids := make([]string, 0)
	productDecls.Range(func(key, _ any) bool {
		if id, ok := key.(string); ok {
			if id = strings.TrimSpace(id); id != "" {
				ids = append(ids, id)
			}
		}
		return true
	})
	sort.Strings(ids)
	return ids
}

// ProductSelectionFromDecl projects a ProductDecl into SelectionSpec plus
// contract_final FieldProvenance for ProductSpec assembly.
func ProductSelectionFromDecl(decl ProductDecl) (SelectionSpec, map[string]FieldProvenance) {
	selection := SelectionSpec{
		AgentSummary:       strings.TrimSpace(decl.Selection.AgentSummary),
		AgentSummarySource: ProductDeclSourceRef,
		UseWhen:            append([]string(nil), decl.Selection.UseWhen...),
		AvoidWhen:          append([]string(nil), decl.Selection.AvoidWhen...),
		SourceRefs:         []string{ProductDeclSourceRef},
		MetadataSource:     ProductDeclProvenanceSource,
	}.Normalized()
	provenance := map[string]FieldProvenance{
		"agent_summary": ResolvedFieldProvenance(
			selection.AgentSummary,
			ProductDeclProvenanceSource,
			ProductDeclSourceRef,
			"contract_final",
			"contract_pass_through",
			"ProductDecl final Schema pass-through",
		),
		"use_when": ResolvedFieldProvenance(
			selection.UseWhen,
			ProductDeclProvenanceSource,
			ProductDeclSourceRef,
			"contract_final",
			"contract_pass_through",
			"ProductDecl final Schema pass-through",
		),
		"avoid_when": ResolvedFieldProvenance(
			selection.AvoidWhen,
			ProductDeclProvenanceSource,
			ProductDeclSourceRef,
			"contract_final",
			"contract_pass_through",
			"ProductDecl final Schema pass-through",
		),
	}
	return selection, provenance
}
