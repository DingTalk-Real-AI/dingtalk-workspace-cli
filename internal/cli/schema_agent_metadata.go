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
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
)

// schema_agent_metadata/*.json is retired as a shipped artifact. Production
// Agent selection / safety / interface authority is leaf ContractFinal and
// ProductDecl via RegisterSchemaSourceRoot → ResolveSchemaBuild.
// InstallBuildTimeAgentMetadataJSON is a CI/local dump helper for
// cmd_schema_catalog only; it is not a production source.

// Wire provenance / metadata_source labels (#602 Catalog contract).
// Const identifiers may be renamed; the string values must not change.
const (
	ProvenanceEmbeddedSkillMetadata = "embedded-skill-metadata"
	ProvenanceReviewedManual        = "reviewed_manual"
)

type agentMetadata struct {
	Version     int                             `json:"version"`
	SourceHash  string                          `json:"source_hash"`
	SurfaceHash string                          `json:"surface_hash,omitempty"`
	Coverage    agentMetadataCoverage           `json:"coverage"`
	Products    map[string]agentProductMetadata `json:"products"`
	Domains     []string                        `json:"domains"`
	Tools       map[string]agentToolMetadata    `json:"tools"`
}

type agentMetadataCoverage struct {
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
	AgentSummary       string                              `json:"agent_summary,omitempty"`
	AgentSummarySource string                              `json:"agent_summary_source,omitempty"`
	UseWhen            []string                            `json:"use_when,omitempty"`
	AvoidWhen          []string                            `json:"avoid_when,omitempty"`
	SourceRefs         []string                            `json:"source_refs,omitempty"`
	FieldProvenance    map[string]contract.FieldProvenance `json:"field_provenance,omitempty"`
}

type agentToolMetadata struct {
	AgentSummary       string                              `json:"agent_summary,omitempty"`
	AgentSummarySource string                              `json:"agent_summary_source,omitempty"`
	UseWhen            []string                            `json:"use_when,omitempty"`
	AvoidWhen          []string                            `json:"avoid_when,omitempty"`
	Prerequisites      []string                            `json:"prerequisites,omitempty"`
	Tips               []string                            `json:"tips,omitempty"`
	Effect             string                              `json:"effect,omitempty"`
	EffectSource       string                              `json:"effect_source,omitempty"`
	Risk               string                              `json:"risk,omitempty"`
	Confirmation       string                              `json:"confirmation,omitempty"`
	Idempotency        string                              `json:"idempotency,omitempty"`
	WorkflowRefs       []string                            `json:"workflow_refs,omitempty"`
	Examples           []string                            `json:"examples,omitempty"`
	Reviewed           *bool                               `json:"reviewed,omitempty"`
	SourceRefs         []string                            `json:"source_refs,omitempty"`
	InterfaceRef       *embeddedMCPInterfaceRef            `json:"interface_ref,omitempty"`
	InterfaceMode      string                              `json:"interface_mode,omitempty"`
	Availability       string                              `json:"availability,omitempty"`
	InterfaceReason    string                              `json:"interface_reason,omitempty"`
	FieldProvenance    map[string]contract.FieldProvenance `json:"field_provenance,omitempty"`
}

var runtimeAgentMetadataLazy struct {
	once     sync.Once
	metadata agentMetadata
}

var runtimeAgentMetadataLazyLoadCount atomic.Uint64

var (
	buildTimeAgentMetadataMu       sync.Mutex
	buildTimeAgentMetadataOverride *agentMetadata
)

// InstallBuildTimeAgentMetadataJSON installs generator-produced Agent metadata
// for cmd_schema_catalog CI/local dump assembly only. Production binaries never
// call this; production authority remains leaf ContractFinal / ProductDecl.
// The dump helper injects an in-memory snapshot so schema_agent_metadata/ is
// neither committed nor embedded.
func InstallBuildTimeAgentMetadataJSON(data []byte) error {
	var metadata agentMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return fmt.Errorf("decode build-time Agent metadata: %w", err)
	}
	if metadata.Products == nil {
		metadata.Products = map[string]agentProductMetadata{}
	}
	if metadata.Tools == nil {
		metadata.Tools = map[string]agentToolMetadata{}
	}
	buildTimeAgentMetadataMu.Lock()
	defer buildTimeAgentMetadataMu.Unlock()
	copied := metadata
	buildTimeAgentMetadataOverride = &copied
	return nil
}

// ClearBuildTimeAgentMetadata removes a cmd_schema_catalog dump-helper injection.
func ClearBuildTimeAgentMetadata() {
	buildTimeAgentMetadataMu.Lock()
	defer buildTimeAgentMetadataMu.Unlock()
	buildTimeAgentMetadataOverride = nil
}

// runtimeAgentMetadata returns build-time injected Agent metadata when the
// CI dump helper installed one; otherwise an empty snapshot. Shipped
// binaries no longer embed schema_agent_metadata/*.json; production Agent
// facts come from ContractFinal / ProductDecl.
func runtimeAgentMetadata() agentMetadata {
	buildTimeAgentMetadataMu.Lock()
	override := buildTimeAgentMetadataOverride
	buildTimeAgentMetadataMu.Unlock()
	if override != nil {
		return *override
	}
	runtimeAgentMetadataLazy.once.Do(func() {
		runtimeAgentMetadataLazyLoadCount.Add(1)
		runtimeAgentMetadataLazy.metadata = emptyAgentMetadata()
	})
	return runtimeAgentMetadataLazy.metadata
}

func emptyAgentMetadata() agentMetadata {
	return agentMetadata{
		Products: map[string]agentProductMetadata{},
		Tools:    map[string]agentToolMetadata{},
	}
}

func cloneFieldProvenance(source map[string]contract.FieldProvenance) map[string]contract.FieldProvenance {
	if len(source) == 0 {
		return nil
	}
	out := make(map[string]contract.FieldProvenance, len(source))
	for field, provenance := range source {
		provenance.Value = append(json.RawMessage(nil), provenance.Value...)
		provenance.Candidates = cloneFieldCandidates(provenance.Candidates)
		provenance.OverriddenCandidates = cloneFieldCandidates(provenance.OverriddenCandidates)
		out[field] = provenance
	}
	return out
}

func cloneFieldCandidates(source []contract.FieldCandidateProvenance) []contract.FieldCandidateProvenance {
	if len(source) == 0 {
		return nil
	}
	out := make([]contract.FieldCandidateProvenance, len(source))
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

// agentMetadataSummaryFromProducts publishes Catalog-level Agent coverage from
// the assembled Schema surface (ContractFinal / ProductDecl). This keeps
// runtime delivery and CI dumps hash-aligned without requiring build-time
// Agent metadata inject as a published summary source.
func agentMetadataSummaryFromProducts(products []ProductSpec) map[string]any {
	productsWith := 0
	toolsWith := 0
	toolsWithSummary := 0
	surfaceTools := 0
	for _, product := range products {
		if productHasPublishedAgentMetadata(product) {
			productsWith++
		}
		for _, tool := range product.Tools {
			surfaceTools++
			if toolHasPublishedAgentMetadata(tool) {
				toolsWith++
			}
			if strings.TrimSpace(tool.Selection.AgentSummary) != "" {
				toolsWithSummary++
			}
		}
	}
	summary := map[string]any{
		"source":                 ProvenanceEmbeddedSkillMetadata,
		"version":                1,
		"source_hash":            "",
		"products_with_metadata": productsWith,
		"tools_with_metadata":    toolsWith,
	}
	if len(products) > 0 {
		summary["surface_products"] = len(products)
	}
	if surfaceTools > 0 {
		summary["surface_tools"] = surfaceTools
	}
	if toolsWithSummary > 0 {
		summary["tools_with_agent_summary"] = toolsWithSummary
	}
	return summary
}

func productHasPublishedAgentMetadata(product ProductSpec) bool {
	return strings.TrimSpace(product.Selection.AgentSummary) != "" ||
		len(product.Selection.UseWhen) > 0 ||
		len(product.Selection.AvoidWhen) > 0
}

func toolHasPublishedAgentMetadata(tool ToolSpec) bool {
	return strings.TrimSpace(tool.Selection.AgentSummary) != "" ||
		len(tool.Selection.UseWhen) > 0 ||
		len(tool.Selection.AvoidWhen) > 0 ||
		len(tool.Selection.Examples) > 0
}
