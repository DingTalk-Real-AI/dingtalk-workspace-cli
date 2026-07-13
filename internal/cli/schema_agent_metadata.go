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

package cli

import (
	"embed"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
)

//go:generate go run ../generator/cmd_schema_agent_metadata -root ../.. -registry internal/cli/schema_command_registry.json -output-dir schema_agent_metadata -audit-output schema_agent_metadata_audit.json
// Rebuild all dependencies so the Catalog compiler cannot reuse the cli
// package cached by the preceding metadata generator with the old embedded
// JSON files.
//go:generate go run -a ../generator/cmd_schema_catalog -root ../.. -output schema_catalog.json

//go:embed schema_agent_metadata/*.json
var embeddedAgentMetadataFS embed.FS

const embeddedAgentMetadataSource = "embedded-skill-metadata"

type embeddedAgentMetadata struct {
	Version     int                             `json:"version"`
	SourceHash  string                          `json:"source_hash"`
	SurfaceHash string                          `json:"surface_hash,omitempty"`
	Coverage    embeddedAgentMetadataCoverage   `json:"coverage"`
	Products    map[string]agentProductMetadata `json:"products"`
	Domains     []string                        `json:"domains"`
	Tools       map[string]agentToolMetadata    `json:"tools"`
}

type embeddedAgentMetadataCoverage struct {
	SurfaceProducts        int `json:"surface_products,omitempty"`
	ProductsWithMetadata   int `json:"products_with_metadata"`
	SurfaceTools           int `json:"surface_tools,omitempty"`
	ToolsWithMetadata      int `json:"tools_with_metadata"`
	ToolsWithSummary       int `json:"tools_with_agent_summary,omitempty"`
	ToolsWithUseWhen       int `json:"tools_with_use_when,omitempty"`
	ToolsWithAvoidWhen     int `json:"tools_with_avoid_when,omitempty"`
	ToolsWithExamples      int `json:"tools_with_examples,omitempty"`
	ToolsWithInterfaceMode int `json:"tools_with_interface_mode,omitempty"`
	UnmatchedSkillTools    int `json:"unmatched_skill_tools,omitempty"`
	UnreviewedSkillTools   int `json:"unreviewed_skill_tools,omitempty"`
}

type agentProductMetadata struct {
	AgentSummary       string                     `json:"agent_summary,omitempty"`
	AgentSummarySource string                     `json:"agent_summary_source,omitempty"`
	UseWhen            []string                   `json:"use_when,omitempty"`
	AvoidWhen          []string                   `json:"avoid_when,omitempty"`
	SourceRefs         []string                   `json:"source_refs,omitempty"`
	FieldProvenance    map[string]FieldProvenance `json:"field_provenance,omitempty"`
}

type agentToolMetadata struct {
	AgentSummary       string                     `json:"agent_summary,omitempty"`
	AgentSummarySource string                     `json:"agent_summary_source,omitempty"`
	UseWhen            []string                   `json:"use_when,omitempty"`
	AvoidWhen          []string                   `json:"avoid_when,omitempty"`
	Prerequisites      []string                   `json:"prerequisites,omitempty"`
	Tips               []string                   `json:"tips,omitempty"`
	Effect             string                     `json:"effect,omitempty"`
	EffectSource       string                     `json:"effect_source,omitempty"`
	Risk               string                     `json:"risk,omitempty"`
	Confirmation       string                     `json:"confirmation,omitempty"`
	Idempotency        string                     `json:"idempotency,omitempty"`
	WorkflowRefs       []string                   `json:"workflow_refs,omitempty"`
	Examples           []string                   `json:"examples,omitempty"`
	Reviewed           *bool                      `json:"reviewed,omitempty"`
	SourceRefs         []string                   `json:"source_refs,omitempty"`
	InterfaceRef       *embeddedMCPInterfaceRef   `json:"interface_ref,omitempty"`
	InterfaceMode      string                     `json:"interface_mode,omitempty"`
	Availability       string                     `json:"availability,omitempty"`
	InterfaceReason    string                     `json:"interface_reason,omitempty"`
	FieldProvenance    map[string]FieldProvenance `json:"field_provenance,omitempty"`
}

type embeddedAgentMetadataDomain struct {
	ProductID string                       `json:"product_id"`
	Tools     map[string]agentToolMetadata `json:"tools"`
}

var runtimeEmbeddedAgentMetadataLazy struct {
	once     sync.Once
	metadata embeddedAgentMetadata
}

var runtimeEmbeddedAgentMetadataLazyLoadCount atomic.Uint64

// runtimeAgentMetadata parses the generated Agent metadata on first Schema
// assembly only. Keeping the sync.Once at the access boundary ensures package
// initialization and ordinary CLI commands never deserialize the embedded
// fragments.
func runtimeAgentMetadata() embeddedAgentMetadata {
	runtimeEmbeddedAgentMetadataLazy.once.Do(func() {
		runtimeEmbeddedAgentMetadataLazyLoadCount.Add(1)
		runtimeEmbeddedAgentMetadataLazy.metadata = loadEmbeddedAgentMetadata()
	})
	return runtimeEmbeddedAgentMetadataLazy.metadata
}

func loadEmbeddedAgentMetadata() embeddedAgentMetadata {
	var metadata embeddedAgentMetadata
	index, err := embeddedAgentMetadataFS.ReadFile("schema_agent_metadata/index.json")
	if err != nil || json.Unmarshal(index, &metadata) != nil {
		return emptyEmbeddedAgentMetadata()
	}
	metadata.Tools = map[string]agentToolMetadata{}
	for _, domain := range metadata.Domains {
		domain = strings.TrimSpace(domain)
		if domain == "" || strings.Contains(domain, "/") || strings.Contains(domain, "\\") {
			return emptyEmbeddedAgentMetadata()
		}
		data, err := embeddedAgentMetadataFS.ReadFile("schema_agent_metadata/" + domain + ".json")
		if err != nil {
			return emptyEmbeddedAgentMetadata()
		}
		var fragment embeddedAgentMetadataDomain
		if err := json.Unmarshal(data, &fragment); err != nil || strings.TrimSpace(fragment.ProductID) != domain {
			return emptyEmbeddedAgentMetadata()
		}
		for path, tool := range fragment.Tools {
			metadata.Tools[path] = tool
		}
	}
	if metadata.Products == nil {
		metadata.Products = map[string]agentProductMetadata{}
	}
	return metadata
}

func emptyEmbeddedAgentMetadata() embeddedAgentMetadata {
	return embeddedAgentMetadata{
		Products: map[string]agentProductMetadata{},
		Tools:    map[string]agentToolMetadata{},
	}
}

// agentToolContractForPathsFromMetadata is the sole typed adapter from generated Agent
// metadata to runtime contract assembly. Path resolution happens once; all
// consumers receive the same resolved safety, interface, selection and
// provenance values without performing downstream map merges.
func agentToolContractForPathsFromMetadata(source embeddedAgentMetadata, paths ...string) (SafetySpec, InterfaceSpec, SelectionSpec, map[string]FieldProvenance, bool) {
	metadata, ok := lookupAgentToolMetadataFrom(source, paths...)
	if !ok {
		return SafetySpec{}, InterfaceSpec{}, SelectionSpec{}, nil, false
	}
	safety := SafetySpec{
		Effect:       strings.TrimSpace(metadata.Effect),
		EffectSource: strings.TrimSpace(metadata.EffectSource),
		Risk:         strings.TrimSpace(metadata.Risk),
		Confirmation: strings.TrimSpace(metadata.Confirmation),
		Idempotency:  strings.TrimSpace(metadata.Idempotency),
	}
	interfaceSpec := InterfaceSpec{
		Mode:         strings.TrimSpace(metadata.InterfaceMode),
		Availability: strings.TrimSpace(metadata.Availability),
		Reason:       strings.TrimSpace(metadata.InterfaceReason),
	}
	if metadata.InterfaceRef != nil {
		interfaceSpec.Ref = &InterfaceRefSpec{
			ProductID: strings.TrimSpace(metadata.InterfaceRef.ProductID),
			RPCName:   strings.TrimSpace(metadata.InterfaceRef.RPCName),
		}
	}
	selection := agentToolSelection(metadata)
	provenance := resolvedAgentToolProvenance(metadata.FieldProvenance, interfaceSpec, selection)
	return safety, interfaceSpec, selection, provenance, true
}

// resolvedAgentToolProvenance is the typed source adapter for generated Agent
// metadata. The generator stores concrete interface refs in its compact
// "product.rpc" identity form and stores reviewed no-ref dispositions as JSON
// null. The final typed contract stores an object or JSON null, so project the
// concrete compact identity here. Missing provenance is never synthesized:
// constructors and snapshot loaders remain validate-only and fail closed.
func resolvedAgentToolProvenance(source map[string]FieldProvenance, interfaceSpec InterfaceSpec, selection SelectionSpec) map[string]FieldProvenance {
	out := cloneFieldProvenance(source)
	if provenance, ok := out["interface_ref"]; ok {
		out["interface_ref"] = projectAgentInterfaceRefProvenance(provenance, interfaceSpec.Ref)
	}
	return out
}

func projectAgentInterfaceRefProvenance(provenance FieldProvenance, ref *InterfaceRefSpec) FieldProvenance {
	finalValue, _ := json.Marshal(ref)
	legacy := "<none>"
	if ref != nil {
		legacy = strings.TrimSpace(ref.ProductID) + "." + strings.TrimSpace(ref.RPCName)
	}
	legacyValue, _ := json.Marshal(legacy)
	project := func(value json.RawMessage) json.RawMessage {
		if string(value) == string(legacyValue) || string(value) == string(finalValue) {
			return append(json.RawMessage(nil), finalValue...)
		}
		// Keep a disagreeing source value untouched. ToolSpec validation will
		// then fail instead of this adapter laundering a resolver conflict.
		return value
	}
	provenance.Value = project(provenance.Value)
	for index := range provenance.Candidates {
		candidate := &provenance.Candidates[index]
		if candidate.Selected != nil && *candidate.Selected {
			candidate.Value = project(candidate.Value)
		}
	}
	return provenance
}

func resolvedFieldProvenance(value any, source, sourceRef, precedence, resolution, reviewReason string) FieldProvenance {
	raw, err := json.Marshal(value)
	if err != nil {
		raw = json.RawMessage("null")
	}
	selected := true
	return FieldProvenance{
		Value:        append(json.RawMessage(nil), raw...),
		Source:       strings.TrimSpace(source),
		SourceRef:    strings.TrimSpace(sourceRef),
		Precedence:   strings.TrimSpace(precedence),
		Resolution:   strings.TrimSpace(resolution),
		ReviewReason: strings.TrimSpace(reviewReason),
		Candidates: []FieldCandidateProvenance{{
			Value:        append(json.RawMessage(nil), raw...),
			Source:       strings.TrimSpace(source),
			SourceRef:    strings.TrimSpace(sourceRef),
			Precedence:   strings.TrimSpace(precedence),
			ReviewReason: strings.TrimSpace(reviewReason),
			Selected:     &selected,
		}},
	}
}

// agentProductSelectionForIDsFromMetadata exposes generated product routing
// through the same typed SelectionSpec used by ToolSpec.
func agentProductSelectionForIDsFromMetadata(source embeddedAgentMetadata, ids ...string) (SelectionSpec, bool) {
	selection, _, ok := agentProductContractForIDsFromMetadata(source, ids...)
	return selection, ok
}

func agentProductContractForIDsFromMetadata(source embeddedAgentMetadata, ids ...string) (SelectionSpec, map[string]FieldProvenance, bool) {
	for _, id := range ids {
		metadata, ok := source.Products[strings.TrimSpace(id)]
		if !ok {
			continue
		}
		selection := SelectionSpec{
			AgentSummary:       strings.TrimSpace(metadata.AgentSummary),
			AgentSummarySource: strings.TrimSpace(metadata.AgentSummarySource),
			UseWhen:            cloneOptionalStrings(metadata.UseWhen),
			AvoidWhen:          cloneOptionalStrings(metadata.AvoidWhen),
			SourceRefs:         cloneOptionalStrings(metadata.SourceRefs),
			MetadataSource:     embeddedAgentMetadataSource,
		}.normalized()
		return selection, cloneFieldProvenance(metadata.FieldProvenance), true
	}
	return SelectionSpec{}, nil, false
}

func agentToolSelection(metadata agentToolMetadata) SelectionSpec {
	var reviewed *bool
	if metadata.Reviewed != nil {
		value := *metadata.Reviewed
		reviewed = &value
	}
	return SelectionSpec{
		AgentSummary:       strings.TrimSpace(metadata.AgentSummary),
		AgentSummarySource: strings.TrimSpace(metadata.AgentSummarySource),
		UseWhen:            cloneOptionalStrings(metadata.UseWhen),
		AvoidWhen:          cloneOptionalStrings(metadata.AvoidWhen),
		Prerequisites:      cloneOptionalStrings(metadata.Prerequisites),
		Tips:               cloneOptionalStrings(metadata.Tips),
		WorkflowRefs:       cloneOptionalStrings(metadata.WorkflowRefs),
		Examples:           cloneOptionalStrings(metadata.Examples),
		Reviewed:           reviewed,
		SourceRefs:         cloneOptionalStrings(metadata.SourceRefs),
		MetadataSource:     embeddedAgentMetadataSource,
	}.normalized()
}

func cloneFieldProvenance(source map[string]FieldProvenance) map[string]FieldProvenance {
	if len(source) == 0 {
		return nil
	}
	out := make(map[string]FieldProvenance, len(source))
	for field, provenance := range source {
		provenance.Value = append(json.RawMessage(nil), provenance.Value...)
		provenance.Candidates = cloneFieldCandidates(provenance.Candidates)
		provenance.OverriddenCandidates = cloneFieldCandidates(provenance.OverriddenCandidates)
		out[field] = provenance
	}
	return out
}

func cloneFieldCandidates(source []FieldCandidateProvenance) []FieldCandidateProvenance {
	if len(source) == 0 {
		return nil
	}
	out := make([]FieldCandidateProvenance, len(source))
	for index, candidate := range source {
		candidate.Value = append(json.RawMessage(nil), candidate.Value...)
		if candidate.Selected != nil {
			value := *candidate.Selected
			candidate.Selected = &value
		}
		out[index] = candidate
	}
	return out
}

func lookupAgentToolMetadataFrom(source embeddedAgentMetadata, paths ...string) (agentToolMetadata, bool) {
	seen := map[string]bool{}
	for _, path := range paths {
		for _, candidate := range []string{
			strings.TrimSpace(path),
			strings.Join(splitSchemaPathTokens(path), " "),
		} {
			if candidate == "" || seen[candidate] {
				continue
			}
			seen[candidate] = true
			if metadata, ok := source.Tools[candidate]; ok {
				return metadata, true
			}
		}
	}
	return agentToolMetadata{}, false
}

func agentMetadataSummaryFrom(metadata embeddedAgentMetadata) map[string]any {
	summary := map[string]any{
		"source":                 embeddedAgentMetadataSource,
		"version":                metadata.Version,
		"source_hash":            strings.TrimSpace(metadata.SourceHash),
		"products_with_metadata": len(metadata.Products),
		"tools_with_metadata":    len(metadata.Tools),
	}
	if metadata.SurfaceHash != "" {
		summary["surface_hash"] = metadata.SurfaceHash
	}
	coverage := metadata.Coverage
	if coverage.SurfaceProducts > 0 {
		summary["surface_products"] = coverage.SurfaceProducts
	}
	if coverage.SurfaceTools > 0 {
		summary["surface_tools"] = coverage.SurfaceTools
	}
	if coverage.ToolsWithSummary > 0 {
		summary["tools_with_agent_summary"] = coverage.ToolsWithSummary
	}
	if coverage.UnmatchedSkillTools > 0 {
		summary["unmatched_skill_tools"] = coverage.UnmatchedSkillTools
	}
	return summary
}
