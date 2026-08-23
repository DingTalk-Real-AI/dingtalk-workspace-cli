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

package agentmetadata

import (
	"fmt"
	"sort"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	"github.com/spf13/cobra"
)

// applyContractFinalDeclarations merges each registered Contract final
// overlay (corecmd.ContractDecl pass-through) into the matching tool record as
// the top-precedence candidate. Declared tools stop depending on selection
// hint files: the in-code declaration is the single final source for every
// field it carries. Non-declared tools are untouched.
//
// Runs after hint reconciliation so rank ordering decides the winner; the
// contract rank outranks every file/manual source. Product-level ProductDecl
// routing is applied in the same pass.
func applyContractFinalDeclarations(file *File, opts Options) error {
	if file == nil {
		return nil
	}
	if err := applyContractFinalProductDeclarations(file, opts); err != nil {
		return err
	}
	if len(opts.BoundCommands.ByCanonical) == 0 {
		return nil
	}
	canonicals := make([]string, 0, len(opts.BoundCommands.ByCanonical))
	for canonical := range opts.BoundCommands.ByCanonical {
		canonicals = append(canonicals, canonical)
	}
	sort.Strings(canonicals)
	for _, canonical := range canonicals {
		bound := opts.BoundCommands.ByCanonical[canonical]
		metadata, ok := contractFinalToolMetadata(bound.PrimaryCommand)
		if !ok {
			continue
		}
		primary := normalizeCommandPath(opts.CanonicalToolPaths[canonical])
		if primary == "" {
			// A declared command without a registry projection means the
			// declaration would silently vanish from the artifact — surface
			// the misconfiguration instead of skipping it.
			return fmt.Errorf("declared tool %s has a Contract final overlay but no canonical CLI projection", canonical)
		}
		merged, err := mergeToolMetadata(file.Tools[primary], metadata, primary)
		if err != nil {
			return err
		}
		file.Tools[primary] = merged
	}
	return nil
}

// applyContractFinalProductDeclarations merges each RegisterProductDecl into
// the matching product record as the top-precedence (contract_final)
// candidate. Declared products no longer require a selection/ products{} row.
func applyContractFinalProductDeclarations(file *File, opts Options) error {
	if file == nil {
		return nil
	}
	if file.Products == nil {
		file.Products = map[string]ProductMetadata{}
	}
	for _, productID := range contract.RegisteredProductDeclIDs() {
		if len(opts.ProductIDs) > 0 && !opts.ProductIDs[productID] {
			continue
		}
		decl, ok := contract.LookupProductDecl(productID)
		if !ok {
			continue
		}
		incoming := contractFinalProductMetadata(decl)
		existing := file.Products[productID]
		if err := mergeRankedStringValue(
			&existing.AgentSummary, &existing.agentSummaryPresent, &existing.agentSummaryRank, &existing.agentSummaryOrigin,
			incoming.AgentSummary, incoming.agentSummaryPresent, selectionRankContractFinal, productDeclOrigin, productID, "agent_summary",
		); err != nil {
			return err
		}
		if incoming.agentSummaryPresent && existing.agentSummaryOrigin == productDeclOrigin {
			existing.AgentSummarySource = incoming.AgentSummarySource
		}
		recordProductListCandidate(&existing, "use_when", incoming.UseWhen, incoming.useWhenPresent, selectionRankContractFinal, productDeclOrigin)
		if err := mergeRankedStringList(
			&existing.UseWhen, &existing.useWhenPresent, &existing.useWhenRank, &existing.useWhenOrigin,
			incoming.UseWhen, incoming.useWhenPresent, selectionRankContractFinal, productDeclOrigin, productID, "use_when",
		); err != nil {
			return err
		}
		recordProductListCandidate(&existing, "avoid_when", incoming.AvoidWhen, incoming.avoidWhenPresent, selectionRankContractFinal, productDeclOrigin)
		if err := mergeRankedStringList(
			&existing.AvoidWhen, &existing.avoidWhenPresent, &existing.avoidWhenRank, &existing.avoidWhenOrigin,
			incoming.AvoidWhen, incoming.avoidWhenPresent, selectionRankContractFinal, productDeclOrigin, productID, "avoid_when",
		); err != nil {
			return err
		}
		existing.SourceRefs = append(existing.SourceRefs, incoming.SourceRefs...)
		file.Products[productID] = existing
	}
	return nil
}

// productDeclOrigin labels ProductDecl candidates in Agent-metadata provenance.
const productDeclOrigin = contract.ProductDeclProvenanceSource

func contractFinalProductMetadata(decl contract.ProductDecl) ProductMetadata {
	metadata := ProductMetadata{
		AgentSummarySource: contract.ProductDeclSourceRef,
		SourceRefs:         []string{contract.ProductDeclSourceRef},
	}
	if summary := strings.TrimSpace(decl.Selection.AgentSummary); summary != "" {
		metadata.AgentSummary = summary
		metadata.agentSummaryPresent = true
		metadata.agentSummaryRank = selectionRankContractFinal
		metadata.agentSummaryOrigin = productDeclOrigin
	}
	if decl.Selection.UseWhen != nil {
		metadata.UseWhen = append([]string(nil), decl.Selection.UseWhen...)
		metadata.useWhenPresent = true
		metadata.useWhenRank = selectionRankContractFinal
		metadata.useWhenOrigin = productDeclOrigin
	}
	if decl.Selection.AvoidWhen != nil {
		metadata.AvoidWhen = append([]string(nil), decl.Selection.AvoidWhen...)
		metadata.avoidWhenPresent = true
		metadata.avoidWhenRank = selectionRankContractFinal
		metadata.avoidWhenOrigin = productDeclOrigin
	}
	return metadata
}

// contractFinalToolMetadata maps a registered Contract final overlay to a
// ToolMetadata candidate whose fields carry the contract_final rank/origin.
// Absent payload sections stay absent (never authored-empty); declared
// sections map field-by-field, preserving explicit empty-list authorship.
func contractFinalToolMetadata(command *cobra.Command) (ToolMetadata, bool) {
	payload, ok := contractfinal.RuntimeContractFinal(command)
	if !ok {
		return ToolMetadata{}, false
	}
	metadata := ToolMetadata{
		AgentSummarySource: "corecmd.ContractDecl",
		SourceRefs:         []string{"corecmd.ContractDecl"},
		reviewedRank:       selectionRankContractFinal,
		reviewedOrigin:     contractFinalOrigin,
	}
	// Declarations live in reviewed code, so the Agent reviewed flag is
	// assembly-derived true (mirrors the catalog pass-through).
	reviewed := true
	metadata.Reviewed = &reviewed

	if selection := payload.Selection; selection != nil {
		if summary := strings.TrimSpace(selection.AgentSummary); summary != "" {
			metadata.AgentSummary = summary
			metadata.agentSummaryPresent = true
			metadata.agentSummaryRank = selectionRankContractFinal
			metadata.agentSummaryOrigin = contractFinalOrigin
		}
		for _, list := range []struct {
			value   []string
			out     *[]string
			present *bool
			rank    *int
			origin  *string
		}{
			{selection.UseWhen, &metadata.UseWhen, &metadata.useWhenPresent, &metadata.useWhenRank, &metadata.useWhenOrigin},
			{selection.AvoidWhen, &metadata.AvoidWhen, &metadata.avoidWhenPresent, &metadata.avoidWhenRank, &metadata.avoidWhenOrigin},
			{selection.Prerequisites, &metadata.Prerequisites, &metadata.prerequisitesPresent, &metadata.prerequisitesRank, &metadata.prerequisitesOrigin},
			{selection.Tips, &metadata.Tips, &metadata.tipsPresent, &metadata.tipsRank, &metadata.tipsOrigin},
			{selection.WorkflowRefs, &metadata.WorkflowRefs, &metadata.workflowRefsPresent, &metadata.workflowRefsRank, &metadata.workflowRefsOrigin},
			{selection.Examples, &metadata.Examples, &metadata.examplesPresent, &metadata.examplesRank, &metadata.examplesOrigin},
		} {
			if list.value == nil {
				continue
			}
			*list.out = list.value
			*list.present = true
			*list.rank = selectionRankContractFinal
			*list.origin = contractFinalOrigin
		}
	}

	if safety := payload.Safety; safety != nil {
		if effect := strings.TrimSpace(safety.Effect); effect != "" {
			metadata.Effect = effect
			metadata.EffectSource = contractFinalOrigin
			metadata.effectPresent = true
			metadata.effectRank = selectionRankContractFinal
			metadata.effectOrigin = contractFinalOrigin
		}
		if risk := strings.TrimSpace(safety.Risk); risk != "" {
			metadata.Risk = risk
			metadata.riskPresent = true
			metadata.riskRank = selectionRankContractFinal
			metadata.riskOrigin = contractFinalOrigin
		}
		if confirmation := strings.TrimSpace(safety.Confirmation); confirmation != "" {
			metadata.Confirmation = confirmation
			metadata.confirmationPresent = true
			metadata.confirmationRank = selectionRankContractFinal
			metadata.confirmationOrigin = contractFinalOrigin
		}
		if idempotency := strings.TrimSpace(safety.Idempotency); idempotency != "" {
			metadata.Idempotency = idempotency
			metadata.idempotencyPresent = true
			metadata.idempotencyRank = selectionRankContractFinal
			metadata.idempotencyOrigin = contractFinalOrigin
		}
	}

	if iface := payload.Interface; iface != nil {
		if mode := strings.TrimSpace(iface.Mode); mode != "" {
			metadata.InterfaceMode = mode
			metadata.interfaceModePresent = true
			metadata.interfaceModeRank = selectionRankContractFinal
			metadata.interfaceModeOrigin = contractFinalOrigin
		}
		if availability := strings.TrimSpace(iface.Availability); availability != "" {
			metadata.Availability = availability
			metadata.availabilityPresent = true
			metadata.availabilityRank = selectionRankContractFinal
			metadata.availabilityOrigin = contractFinalOrigin
		}
		if reason := strings.TrimSpace(iface.Reason); reason != "" {
			metadata.InterfaceReason = reason
			metadata.interfaceReasonPresent = true
			metadata.interfaceReasonRank = selectionRankContractFinal
			metadata.interfaceReasonOrigin = contractFinalOrigin
		}
		if iface.Ref != nil {
			metadata.InterfaceRef = &InterfaceRef{ProductID: iface.Ref.ProductID, RPCName: iface.Ref.RPCName}
			metadata.interfaceRefPresent = true
			metadata.interfaceRefRank = selectionRankContractFinal
			metadata.interfaceRefOrigin = contractFinalOrigin
		}
	}
	return metadata, true
}
