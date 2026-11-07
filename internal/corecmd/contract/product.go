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
	"path"
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

// HelpDocumentation is one stable, human-readable documentation link shown
// in service and leaf Help. It is deliberately not part of SelectionSpec or
// the public Schema wire: Help references guide further reading without
// changing the executable command contract.
type HelpDocumentation struct {
	Label string
	URL   string
}

// HelpReferences declares the embedded Skills and deeper documentation that
// are relevant to a product. Leaf Help inherits the declaration by ProductID.
type HelpReferences struct {
	RelatedSkills []string
	Documentation []HelpDocumentation
}

const embeddedSkillDocumentationBaseURL = "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/blob/main/skills/multi"

// SkillDocumentation builds a stable GitHub link to a file in an embedded
// multi-Skill. Product declarations use this helper so URLs cannot drift from
// the repository layout while retaining an explicit label and path.
func SkillDocumentation(label, skill, relativePath string) HelpDocumentation {
	return HelpDocumentation{
		Label: strings.TrimSpace(label),
		URL:   embeddedSkillDocumentationBaseURL + "/" + path.Join(strings.TrimSpace(skill), strings.TrimSpace(relativePath)),
	}
}

// ProductDecl is the product-level Schema routing declaration. Assembly writes
// ProductSpec.Selection with provenance contract_final. HelpReferences remains
// internal-only and is consumed exclusively by Help rendering.
type ProductDecl struct {
	ID             string
	Selection      ProductSelectionDecl
	HelpReferences HelpReferences
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
	decl.HelpReferences = normalizeHelpReferences(productID, decl.HelpReferences)
	productDecls.Store(productID, decl)
}

func normalizeHelpReferences(productID string, refs HelpReferences) HelpReferences {
	out := HelpReferences{}
	seenSkills := make(map[string]bool, len(refs.RelatedSkills))
	for _, skill := range refs.RelatedSkills {
		skill = strings.TrimSpace(skill)
		if skill == "" || seenSkills[skill] {
			continue
		}
		seenSkills[skill] = true
		out.RelatedSkills = append(out.RelatedSkills, skill)
	}
	seenDocs := make(map[string]bool, len(refs.Documentation))
	for _, document := range refs.Documentation {
		document.Label = strings.TrimSpace(document.Label)
		document.URL = strings.TrimSpace(document.URL)
		if document.Label == "" || document.URL == "" {
			panic(fmt.Sprintf("product %q HelpReferences documentation requires both Label and URL", productID))
		}
		if !strings.HasPrefix(document.URL, "https://") {
			panic(fmt.Sprintf("product %q HelpReferences documentation URL must use HTTPS: %q", productID, document.URL))
		}
		if seenDocs[document.URL] {
			continue
		}
		seenDocs[document.URL] = true
		out.Documentation = append(out.Documentation, document)
	}
	if len(out.RelatedSkills) == 0 {
		out.RelatedSkills = nil
	}
	if len(out.Documentation) == 0 {
		out.Documentation = nil
	}
	return out
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
	decl.Selection.UseWhen = append([]string(nil), decl.Selection.UseWhen...)
	decl.Selection.AvoidWhen = append([]string(nil), decl.Selection.AvoidWhen...)
	decl.HelpReferences.RelatedSkills = append([]string(nil), decl.HelpReferences.RelatedSkills...)
	decl.HelpReferences.Documentation = append([]HelpDocumentation(nil), decl.HelpReferences.Documentation...)
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
