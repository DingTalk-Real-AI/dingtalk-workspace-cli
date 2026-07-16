// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package agentmetadata

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

const HintFileVersion = 1
const HintIndexFormat = "dws-agent-hint-index"

// HintFile is a versioned source contract. Imported files provide a reviewed
// baseline; explicit files can override scalar fields while Skills continue to
// supply routing and workflow context.
type HintFile struct {
	Version         int                        `json:"version"`
	Source          HintSource                 `json:"source"`
	Coverage        HintCoverage               `json:"coverage,omitempty"`
	Products        map[string]HintProduct     `json:"products,omitempty"`
	Tools           map[string]HintTool        `json:"tools,omitempty"`
	ReferenceReview map[string]ReferenceReview `json:"reference_review,omitempty"`
}

// HintIndex is the human-authored entrypoint. It maps product IDs to metadata
// (safety/interface/parameters/runtime_gate) and selection (Agent routing prose)
// HintFile paths. Index-level runtime_gates were removed; gates live on each
// metadata tool as runtime_gate.
type HintIndex struct {
	Version         int                        `json:"version"`
	Format          string                     `json:"format"`
	Source          HintSource                 `json:"source"`
	Coverage        HintCoverage               `json:"coverage,omitempty"`
	Metadata        map[string]string          `json:"metadata"`
	Selection       map[string]string          `json:"selection"`
	ReferenceReview map[string]ReferenceReview `json:"reference_review,omitempty"`
}

type HintSource struct {
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Reviewed   bool   `json:"reviewed,omitempty"`
	Repository string `json:"repository,omitempty"`
	Revision   string `json:"revision,omitempty"`
	Channel    string `json:"channel,omitempty"`
	SourceHash string `json:"source_hash,omitempty"`
}

type HintCoverage struct {
	SourceProducts  int `json:"source_products,omitempty"`
	MatchedProducts int `json:"matched_products,omitempty"`
	SourceTools     int `json:"source_tools,omitempty"`
	EligibleTools   int `json:"eligible_tools,omitempty"`
	MatchedTools    int `json:"matched_tools,omitempty"`
	UnmatchedTools  int `json:"unmatched_tools,omitempty"`
}

type HintProduct struct {
	AgentSummary        string   `json:"agent_summary,omitempty"`
	UseWhen             []string `json:"use_when,omitempty"`
	AvoidWhen           []string `json:"avoid_when,omitempty"`
	SourceRefs          []string `json:"source_refs,omitempty"`
	agentSummaryPresent bool
}

type HintTool struct {
	AgentSummary           string                     `json:"agent_summary,omitempty"`
	UseWhen                []string                   `json:"use_when,omitempty"`
	AvoidWhen              []string                   `json:"avoid_when,omitempty"`
	Prerequisites          []string                   `json:"prerequisites,omitempty"`
	Tips                   []string                   `json:"tips,omitempty"`
	Effect                 string                     `json:"effect,omitempty"`
	Risk                   string                     `json:"risk,omitempty"`
	Confirmation           string                     `json:"confirmation,omitempty"`
	Idempotency            string                     `json:"idempotency,omitempty"`
	RuntimeGate            string                     `json:"runtime_gate,omitempty"`
	CLIPath                string                     `json:"cli_path,omitempty"`
	Parameters             map[string]json.RawMessage `json:"parameters,omitempty"`
	WorkflowRefs           []string                   `json:"workflow_refs,omitempty"`
	Examples               []string                   `json:"examples,omitempty"`
	Reviewed               *bool                      `json:"reviewed,omitempty"`
	ReviewReason           string                     `json:"review_reason,omitempty"`
	SourceRefs             []string                   `json:"source_refs,omitempty"`
	InterfaceRef           *InterfaceRef              `json:"interface_ref,omitempty"`
	InterfaceMode          string                     `json:"interface_mode,omitempty"`
	Availability           string                     `json:"availability,omitempty"`
	InterfaceReason        string                     `json:"interface_reason,omitempty"`
	agentSummaryPresent    bool
	effectPresent          bool
	riskPresent            bool
	confirmationPresent    bool
	idempotencyPresent     bool
	interfaceRefPresent    bool
	interfaceModePresent   bool
	availabilityPresent    bool
	interfaceReasonPresent bool
}

func (hint *HintProduct) UnmarshalJSON(data []byte) error {
	type wire HintProduct
	var decoded wire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(data, &fields)
	*hint = HintProduct(decoded)
	_, hint.agentSummaryPresent = fields["agent_summary"]
	return nil
}

func (hint *HintTool) UnmarshalJSON(data []byte) error {
	type wire HintTool
	var decoded wire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(data, &fields)
	*hint = HintTool(decoded)
	_, hint.agentSummaryPresent = fields["agent_summary"]
	_, hint.effectPresent = fields["effect"]
	_, hint.riskPresent = fields["risk"]
	_, hint.confirmationPresent = fields["confirmation"]
	_, hint.idempotencyPresent = fields["idempotency"]
	_, hint.interfaceRefPresent = fields["interface_ref"]
	_, hint.interfaceModePresent = fields["interface_mode"]
	_, hint.availabilityPresent = fields["availability"]
	_, hint.interfaceReasonPresent = fields["interface_reason"]
	return nil
}

type hintFileRole string

const (
	hintRoleImported  hintFileRole = "imported"
	hintRoleMetadata  hintFileRole = "metadata"
	hintRoleSelection hintFileRole = "selection"
	hintRoleIndex     hintFileRole = "index"
	hintRoleLegacy    hintFileRole = "legacy"
)

type parsedHintSource struct {
	file sourceFile
	hint HintFile
	role hintFileRole
}

func parseHintSources(out *File, files []sourceFile, opts Options, stats *Stats, origins sourceTracker) (usedSelection bool, err error) {
	if strings.TrimSpace(opts.HintsDir) == "" {
		return false, nil
	}
	hintsRoot := filepath.Clean(resolvePath(opts.Root, opts.HintsDir))
	hintsPrefix := hintsRoot + string(filepath.Separator)
	byPath := map[string]sourceFile{}
	for _, file := range files {
		cleaned := filepath.Clean(file.path)
		if cleaned == hintsRoot || strings.HasPrefix(cleaned, hintsPrefix) {
			byPath[cleaned] = file
		}
	}

	indexPath := filepath.Join(hintsRoot, "index.json")
	type selectedHint struct {
		file sourceFile
		role hintFileRole
	}
	selected := []selectedHint{}
	if indexFile, ok := byPath[indexPath]; ok {
		var index HintIndex
		if err := json.Unmarshal(indexFile.data, &index); err != nil {
			return false, fmt.Errorf("decode Agent hint index %s: %w", indexFile.display, err)
		}
		if index.Version != HintFileVersion {
			return false, fmt.Errorf("decode Agent hint index %s: unsupported version %d", indexFile.display, index.Version)
		}
		if strings.TrimSpace(index.Format) != HintIndexFormat {
			return false, fmt.Errorf("decode Agent hint index %s: unsupported format %q", indexFile.display, index.Format)
		}
		kind := strings.ToLower(strings.TrimSpace(index.Source.Kind))
		if kind == "" {
			kind = "explicit"
		}
		if kind != "explicit" {
			return false, fmt.Errorf("decode Agent hint index %s: unsupported source kind %q", indexFile.display, index.Source.Kind)
		}
		appendMapped := func(label string, mapping map[string]string, role hintFileRole) error {
			productIDs := make([]string, 0, len(mapping))
			for productID := range mapping {
				productIDs = append(productIDs, productID)
			}
			sort.Strings(productIDs)
			for _, productID := range productIDs {
				rel := strings.TrimSpace(mapping[productID])
				if rel == "" {
					return fmt.Errorf("decode Agent hint index %s: %s %s missing path", indexFile.display, label, productID)
				}
				productPath := filepath.Clean(filepath.Join(hintsRoot, filepath.FromSlash(rel)))
				if productPath != hintsRoot && !strings.HasPrefix(productPath, hintsPrefix) {
					return fmt.Errorf("decode Agent hint index %s: %s %s path escapes hints root", indexFile.display, label, productID)
				}
				file, ok := byPath[productPath]
				if !ok {
					return fmt.Errorf("decode Agent hint index %s: %s %s file missing: %s", indexFile.display, label, productID, rel)
				}
				selected = append(selected, selectedHint{file: file, role: role})
			}
			return nil
		}
		if len(index.Metadata) == 0 {
			return false, fmt.Errorf("decode Agent hint index %s: metadata map is required", indexFile.display)
		}
		if len(index.Selection) == 0 {
			return false, fmt.Errorf("decode Agent hint index %s: selection map is required", indexFile.display)
		}
		if err := appendMapped("metadata", index.Metadata, hintRoleMetadata); err != nil {
			return false, err
		}
		if err := appendMapped("selection", index.Selection, hintRoleSelection); err != nil {
			return false, err
		}
		usedSelection = true
		importedRoot := filepath.Join(hintsRoot, "imported")
		importedPrefix := importedRoot + string(filepath.Separator)
		imported := []sourceFile{}
		for path, file := range byPath {
			if path == importedRoot || strings.HasPrefix(path, importedPrefix) {
				imported = append(imported, file)
			}
		}
		sort.Slice(imported, func(i, j int) bool { return imported[i].display < imported[j].display })
		prefixed := make([]selectedHint, 0, len(imported)+len(selected))
		for _, file := range imported {
			prefixed = append(prefixed, selectedHint{file: file, role: hintRoleImported})
		}
		selected = append(prefixed, selected...)
		if len(index.ReferenceReview) > 0 || index.Coverage != (HintCoverage{}) {
			index.Source.Kind = kind
			payload, _ := json.Marshal(HintFile{
				Version:         HintFileVersion,
				Source:          index.Source,
				Coverage:        index.Coverage,
				ReferenceReview: index.ReferenceReview,
			})
			selected = append(selected, selectedHint{
				file: sourceFile{
					path:    indexPath,
					display: indexFile.display,
					data:    payload,
				},
				role: hintRoleIndex,
			})
		}
	} else {
		for _, file := range files {
			cleaned := filepath.Clean(file.path)
			if cleaned == hintsRoot || strings.HasPrefix(cleaned, hintsPrefix) {
				selected = append(selected, selectedHint{file: file, role: hintRoleLegacy})
			}
		}
	}

	parsed := []parsedHintSource{}
	for _, item := range selected {
		file := item.file
		var hint HintFile
		if err := json.Unmarshal(file.data, &hint); err != nil {
			return false, fmt.Errorf("decode Agent hint %s: %w", file.display, err)
		}
		if hint.Version != HintFileVersion {
			return false, fmt.Errorf("decode Agent hint %s: unsupported version %d", file.display, hint.Version)
		}
		if err := validateHintFileRole(file.display, hint, item.role); err != nil {
			return false, err
		}
		kind := strings.ToLower(strings.TrimSpace(hint.Source.Kind))
		if kind == "" {
			kind = "explicit"
		}
		if kind != "explicit" && kind != "imported" {
			return false, fmt.Errorf("decode Agent hint %s: unsupported source kind %q", file.display, hint.Source.Kind)
		}
		for path, tool := range hint.Tools {
			tool.InterfaceMode = strings.TrimSpace(tool.InterfaceMode)
			tool.Availability = strings.TrimSpace(tool.Availability)
			tool.InterfaceReason = strings.TrimSpace(tool.InterfaceReason)
			if tool.InterfaceMode == "unavailable" {
				return false, fmt.Errorf("decode Agent hint %s: tool %s uses legacy interface_mode=unavailable; migrate to interface_mode=mcp, local, or composite with availability=unavailable", file.display, path)
			}
			if tool.InterfaceMode != "" && tool.InterfaceMode != "mcp" && tool.InterfaceMode != "composite" && tool.InterfaceMode != "local" {
				return false, fmt.Errorf("decode Agent hint %s: tool %s has unsupported interface_mode %q", file.display, path, tool.InterfaceMode)
			}
			if tool.Availability != "" && tool.Availability != "available" && tool.Availability != "unavailable" {
				return false, fmt.Errorf("decode Agent hint %s: tool %s has unsupported availability %q", file.display, path, tool.Availability)
			}
			if tool.InterfaceRef == nil {
				hint.Tools[path] = tool
				continue
			}
			tool.InterfaceRef.ProductID = strings.TrimSpace(tool.InterfaceRef.ProductID)
			tool.InterfaceRef.RPCName = strings.TrimSpace(tool.InterfaceRef.RPCName)
			if tool.InterfaceRef.ProductID == "" || tool.InterfaceRef.RPCName == "" {
				return false, fmt.Errorf("decode Agent hint %s: tool %s has incomplete interface_ref", file.display, path)
			}
			hint.Tools[path] = tool
		}
		for path, review := range hint.ReferenceReview {
			status := strings.TrimSpace(review.Status)
			target := normalizeCommandPath(review.Target)
			reason := strings.TrimSpace(review.Reason)
			switch status {
			case "alias":
				if target == "" {
					return false, fmt.Errorf("decode Agent hint %s: reference %s alias is missing target", file.display, path)
				}
			case "group", "stale", "out_of_surface":
				if target != "" {
					return false, fmt.Errorf("decode Agent hint %s: reference %s status %s cannot have target", file.display, path, status)
				}
			default:
				return false, fmt.Errorf("decode Agent hint %s: reference %s has unsupported status %q", file.display, path, status)
			}
			if reason == "" {
				return false, fmt.Errorf("decode Agent hint %s: reference %s is missing reason", file.display, path)
			}
			review.Status, review.Target, review.Reason = status, target, reason
			hint.ReferenceReview[path] = review
		}
		hint.Source.Kind = kind
		parsed = append(parsed, parsedHintSource{file: file, hint: hint, role: item.role})
	}
	sort.Slice(parsed, func(i, j int) bool {
		left, right := hintKindPriority(parsed[i].hint.Source.Kind), hintKindPriority(parsed[j].hint.Source.Kind)
		if left != right {
			return left < right
		}
		return parsed[i].file.display < parsed[j].file.display
	})
	for _, source := range parsed {
		if err := applyHintSource(out, source, stats, origins); err != nil {
			return false, err
		}
	}
	return usedSelection, nil
}

func validateHintFileRole(display string, hint HintFile, role hintFileRole) error {
	switch role {
	case hintRoleMetadata:
		for path, tool := range hint.Tools {
			if hasSelectionOnlyFields(tool) {
				return fmt.Errorf("decode Agent hint %s: metadata tool %s must not carry selection fields", display, path)
			}
			gate := strings.TrimSpace(tool.RuntimeGate)
			if gate != "" && gate != "none" && gate != "confirm_delete" && gate != "typed_yes" && gate != "confirm_dangerous" {
				return fmt.Errorf("decode Agent hint %s: tool %s has unsupported runtime_gate %q", display, path, gate)
			}
			conf := strings.TrimSpace(tool.Confirmation)
			if gate != "" && gate != "none" && conf != "" && conf != "user_required" {
				return fmt.Errorf("decode Agent hint %s: tool %s runtime_gate=%s requires confirmation=user_required", display, path, gate)
			}
			if (gate == "" || gate == "none") && conf == "user_required" {
				return fmt.Errorf("decode Agent hint %s: tool %s confirmation=user_required requires runtime_gate != none", display, path)
			}
		}
		for path := range hint.Products {
			return fmt.Errorf("decode Agent hint %s: metadata file must not carry products routing block (%s)", display, path)
		}
	case hintRoleSelection:
		for path, tool := range hint.Tools {
			if hasMetadataOnlyFields(tool) {
				return fmt.Errorf("decode Agent hint %s: selection tool %s must not carry metadata fields", display, path)
			}
		}
	}
	return nil
}

func hasSelectionOnlyFields(tool HintTool) bool {
	return scalarIsPresent(tool.AgentSummary, tool.agentSummaryPresent) ||
		tool.UseWhen != nil ||
		tool.AvoidWhen != nil ||
		tool.Examples != nil ||
		tool.Prerequisites != nil ||
		tool.Tips != nil ||
		tool.WorkflowRefs != nil
}

func hasMetadataOnlyFields(tool HintTool) bool {
	return scalarIsPresent(tool.Effect, tool.effectPresent) ||
		scalarIsPresent(tool.Risk, tool.riskPresent) ||
		scalarIsPresent(tool.Confirmation, tool.confirmationPresent) ||
		scalarIsPresent(tool.Idempotency, tool.idempotencyPresent) ||
		strings.TrimSpace(tool.RuntimeGate) != "" ||
		len(tool.Parameters) > 0 ||
		strings.TrimSpace(tool.CLIPath) != "" ||
		tool.InterfaceRef != nil || tool.interfaceRefPresent ||
		scalarIsPresent(tool.InterfaceMode, tool.interfaceModePresent) ||
		scalarIsPresent(tool.Availability, tool.availabilityPresent) ||
		scalarIsPresent(tool.InterfaceReason, tool.interfaceReasonPresent)
}

func hintKindPriority(kind string) int {
	if kind == "imported" {
		return 0
	}
	return 1
}

func applyHintSource(out *File, parsed parsedHintSource, stats *Stats, origins sourceTracker) error {
	hint := parsed.hint
	sourceLabel := hintSourceLabel(hint.Source, parsed.file.display)
	sourceRef := parsed.file.display
	stats.HintFiles++
	for rawPath, review := range hint.ReferenceReview {
		path := normalizeCommandPath(rawPath)
		if path == "" {
			continue
		}
		review.Status = strings.TrimSpace(review.Status)
		review.Target = normalizeCommandPath(review.Target)
		review.Reason = strings.TrimSpace(review.Reason)
		stats.referenceReviews[path] = review
	}

	productIDs := make([]string, 0, len(hint.Products))
	for productID := range hint.Products {
		productIDs = append(productIDs, productID)
	}
	sort.Strings(productIDs)
	for _, rawProductID := range productIDs {
		productID := strings.TrimSpace(rawProductID)
		if productID == "" {
			continue
		}
		incoming := hint.Products[rawProductID]
		metadata := out.Products[productID]
		rank := hintSelectionRank(hint.Source, nil)
		incomingSummaryPresent := scalarIsPresent(incoming.AgentSummary, incoming.agentSummaryPresent)
		previousRank := metadata.agentSummaryRank
		if err := mergeRankedStringValue(
			&metadata.AgentSummary, &metadata.agentSummaryPresent, &metadata.agentSummaryRank, &metadata.agentSummaryOrigin,
			incoming.AgentSummary, incomingSummaryPresent, rank, sourceRef, productID, "agent_summary",
		); err != nil {
			return err
		}
		if incomingSummaryPresent && metadata.agentSummaryOrigin == sourceRef && rank >= previousRank {
			metadata.AgentSummarySource = sourceLabel
		}
		useWhenPresent := incoming.UseWhen != nil
		recordProductListCandidate(&metadata, "use_when", incoming.UseWhen, useWhenPresent, rank, sourceRef)
		if err := mergeRankedStringList(&metadata.UseWhen, &metadata.useWhenPresent, &metadata.useWhenRank, &metadata.useWhenOrigin, incoming.UseWhen, useWhenPresent, rank, sourceRef, productID, "use_when"); err != nil {
			return err
		}
		avoidWhenPresent := incoming.AvoidWhen != nil
		recordProductListCandidate(&metadata, "avoid_when", incoming.AvoidWhen, avoidWhenPresent, rank, sourceRef)
		if err := mergeRankedStringList(&metadata.AvoidWhen, &metadata.avoidWhenPresent, &metadata.avoidWhenRank, &metadata.avoidWhenOrigin, incoming.AvoidWhen, avoidWhenPresent, rank, sourceRef, productID, "avoid_when"); err != nil {
			return err
		}
		metadata.SourceRefs = append(metadata.SourceRefs, parsed.file.display)
		metadata.SourceRefs = append(metadata.SourceRefs, incoming.SourceRefs...)
		out.Products[productID] = metadata
		stats.HintProducts++
	}

	toolPaths := make([]string, 0, len(hint.Tools))
	for path := range hint.Tools {
		toolPaths = append(toolPaths, path)
	}
	sort.Strings(toolPaths)
	for _, rawPath := range toolPaths {
		path := normalizeHintToolPath(rawPath)
		if path == "" {
			continue
		}
		incoming := hint.Tools[rawPath]
		if !hasAgentHintFields(incoming) {
			continue
		}
		metadata := out.Tools[path]
		rank := hintSelectionRank(hint.Source, incoming.Reviewed)
		reviewReason := hintCandidateReviewReason(hint.Source, incoming)
		previousSummaryRank := metadata.agentSummaryRank
		incomingSummaryPresent := scalarIsPresent(incoming.AgentSummary, incoming.agentSummaryPresent)
		recordStringFieldCandidate(&metadata, "agent_summary", incoming.AgentSummary, incomingSummaryPresent, rank, sourceRef, reviewReason)
		if err := mergeRankedStringValue(
			&metadata.AgentSummary, &metadata.agentSummaryPresent, &metadata.agentSummaryRank, &metadata.agentSummaryOrigin,
			incoming.AgentSummary, incomingSummaryPresent, rank, sourceRef, path, "agent_summary",
		); err != nil {
			return err
		}
		if incomingSummaryPresent && metadata.agentSummaryOrigin == sourceRef && rank >= previousSummaryRank {
			metadata.AgentSummarySource = sourceLabel
		}
		for _, list := range []struct {
			name          string
			target        *[]string
			targetPresent *bool
			targetRank    *int
			origin        *string
			incoming      []string
		}{
			{"use_when", &metadata.UseWhen, &metadata.useWhenPresent, &metadata.useWhenRank, &metadata.useWhenOrigin, incoming.UseWhen},
			{"avoid_when", &metadata.AvoidWhen, &metadata.avoidWhenPresent, &metadata.avoidWhenRank, &metadata.avoidWhenOrigin, incoming.AvoidWhen},
			{"prerequisites", &metadata.Prerequisites, &metadata.prerequisitesPresent, &metadata.prerequisitesRank, &metadata.prerequisitesOrigin, incoming.Prerequisites},
			{"tips", &metadata.Tips, &metadata.tipsPresent, &metadata.tipsRank, &metadata.tipsOrigin, incoming.Tips},
			{"workflow_refs", &metadata.WorkflowRefs, &metadata.workflowRefsPresent, &metadata.workflowRefsRank, &metadata.workflowRefsOrigin, incoming.WorkflowRefs},
			{"examples", &metadata.Examples, &metadata.examplesPresent, &metadata.examplesRank, &metadata.examplesOrigin, incoming.Examples},
		} {
			incomingPresent := list.incoming != nil
			recordListFieldCandidate(&metadata, list.name, list.incoming, incomingPresent, rank, sourceRef, reviewReason)
			if err := mergeRankedStringList(list.target, list.targetPresent, list.targetRank, list.origin, list.incoming, incomingPresent, rank, sourceRef, path, list.name); err != nil {
				return err
			}
		}
		if err := mergeEffectValue(&metadata, incoming.Effect, scalarIsPresent(incoming.Effect, incoming.effectPresent), "agent-hint", rank, sourceRef, reviewReason, path); err != nil {
			return err
		}
		if err := mergeRiskValue(&metadata, incoming.Risk, scalarIsPresent(incoming.Risk, incoming.riskPresent), rank, sourceRef, reviewReason, path); err != nil {
			return err
		}
		if err := mergeConfirmationValue(&metadata, incoming.Confirmation, scalarIsPresent(incoming.Confirmation, incoming.confirmationPresent), rank, sourceRef, reviewReason, path); err != nil {
			return err
		}
		idempotencyPresent := scalarIsPresent(incoming.Idempotency, incoming.idempotencyPresent)
		recordStringFieldCandidate(&metadata, "idempotency", incoming.Idempotency, idempotencyPresent, rank, sourceRef, reviewReason)
		if err := mergeRankedStringValue(
			&metadata.Idempotency, &metadata.idempotencyPresent, &metadata.idempotencyRank, &metadata.idempotencyOrigin,
			incoming.Idempotency, idempotencyPresent, rank, sourceRef, path, "idempotency",
		); err != nil {
			return err
		}
		if incoming.Reviewed != nil {
			recordTypedFieldCandidateValue(&metadata, "reviewed", *incoming.Reviewed, true, rank, sourceRef, reviewReason)
			if metadata.Reviewed == nil || rank > metadata.reviewedRank {
				value := *incoming.Reviewed
				metadata.Reviewed = &value
				metadata.reviewedRank = rank
				metadata.reviewedOrigin = sourceRef
			} else if rank == metadata.reviewedRank && *metadata.Reviewed == *incoming.Reviewed {
				metadata.reviewedOrigin = stableSource(metadata.reviewedOrigin, sourceRef)
			}
		}
		if incoming.InterfaceRef != nil || incoming.interfaceRefPresent {
			candidate := ToolMetadata{
				InterfaceRef:        incoming.InterfaceRef,
				interfaceRefPresent: true,
				interfaceRefRank:    rank,
				interfaceRefOrigin:  sourceRef,
			}
			var value any
			if incoming.InterfaceRef != nil {
				value = incoming.InterfaceRef.ProductID + "." + incoming.InterfaceRef.RPCName
			}
			recordTypedFieldCandidateValue(&candidate, "interface_ref", value, true, rank, sourceRef, reviewReason)
			if err := mergeRankedInterfaceRef(&metadata, candidate, path); err != nil {
				return err
			}
		}
		for _, field := range []struct {
			name            string
			target          *string
			targetPresent   *bool
			targetRank      *int
			origin          *string
			incoming        string
			incomingPresent bool
		}{
			{"interface_mode", &metadata.InterfaceMode, &metadata.interfaceModePresent, &metadata.interfaceModeRank, &metadata.interfaceModeOrigin, incoming.InterfaceMode, scalarIsPresent(incoming.InterfaceMode, incoming.interfaceModePresent)},
			{"availability", &metadata.Availability, &metadata.availabilityPresent, &metadata.availabilityRank, &metadata.availabilityOrigin, incoming.Availability, scalarIsPresent(incoming.Availability, incoming.availabilityPresent)},
			{"interface_reason", &metadata.InterfaceReason, &metadata.interfaceReasonPresent, &metadata.interfaceReasonRank, &metadata.interfaceReasonOrigin, incoming.InterfaceReason, scalarIsPresent(incoming.InterfaceReason, incoming.interfaceReasonPresent)},
		} {
			recordStringFieldCandidate(&metadata, field.name, field.incoming, field.incomingPresent, rank, sourceRef, reviewReason)
			if err := mergeRankedStringValue(field.target, field.targetPresent, field.targetRank, field.origin, field.incoming, field.incomingPresent, rank, sourceRef, path, field.name); err != nil {
				return err
			}
		}
		metadata.SourceRefs = append(metadata.SourceRefs, parsed.file.display)
		metadata.SourceRefs = append(metadata.SourceRefs, incoming.SourceRefs...)
		out.Tools[path] = metadata
		origins.add(path, parsed.file.display, 0)
		stats.HintTools++
		if strings.EqualFold(strings.TrimSpace(incoming.Risk), "high") {
			stats.RiskRules++
		}
	}
	return nil
}

func hasAgentHintFields(hint HintTool) bool {
	return scalarIsPresent(hint.AgentSummary, hint.agentSummaryPresent) ||
		hint.UseWhen != nil ||
		hint.AvoidWhen != nil ||
		hint.Prerequisites != nil ||
		hint.Tips != nil ||
		scalarIsPresent(hint.Effect, hint.effectPresent) ||
		scalarIsPresent(hint.Risk, hint.riskPresent) ||
		scalarIsPresent(hint.Confirmation, hint.confirmationPresent) ||
		scalarIsPresent(hint.Idempotency, hint.idempotencyPresent) ||
		strings.TrimSpace(hint.RuntimeGate) != "" ||
		len(hint.Parameters) > 0 ||
		hint.WorkflowRefs != nil ||
		hint.Examples != nil ||
		hint.Reviewed != nil ||
		len(hint.SourceRefs) > 0 ||
		hint.InterfaceRef != nil || hint.interfaceRefPresent ||
		scalarIsPresent(hint.InterfaceMode, hint.interfaceModePresent) ||
		scalarIsPresent(hint.Availability, hint.availabilityPresent) ||
		scalarIsPresent(hint.InterfaceReason, hint.interfaceReasonPresent)
}

func hintSelectionRank(source HintSource, reviewed *bool) int {
	if reviewed != nil {
		if *reviewed {
			return selectionRankReviewedExplicit
		}
		return selectionRankUnreviewedExplicit
	}
	if source.Reviewed {
		return selectionRankReviewedExplicit
	}
	if strings.EqualFold(strings.TrimSpace(source.Kind), "imported") {
		return selectionRankImported
	}
	return selectionRankExplicit
}

func hintCandidateReviewReason(source HintSource, tool HintTool) string {
	if reason := strings.TrimSpace(tool.ReviewReason); reason != "" {
		return reason
	}
	label := hintSourceLabel(source, "Agent hint")
	if tool.Reviewed != nil && *tool.Reviewed {
		return label + " marks this tool as reviewed"
	}
	if source.Reviewed {
		return label + " is a reviewed Agent hint source"
	}
	if strings.EqualFold(strings.TrimSpace(source.Kind), "explicit") && tool.Reviewed == nil {
		return label + " is an explicit Agent hint source"
	}
	return ""
}

func normalizeHintToolPath(raw string) string {
	raw = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "dws "))
	if raw == "" {
		return ""
	}
	if !strings.ContainsAny(raw, " \t") && strings.Contains(raw, ".") {
		return raw
	}
	return normalizeCommandPath(raw)
}

func hintSourceLabel(source HintSource, fallback string) string {
	name := strings.TrimSpace(source.Name)
	if name == "" {
		name = fallback
	}
	if revision := strings.TrimSpace(source.Revision); revision != "" {
		if len(revision) > 12 {
			revision = revision[:12]
		}
		name += "@" + revision
	}
	return name
}
