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
)

//go:generate go run ../generator/cmd_schema_agent_metadata -root ../.. -surface internal/cli/schema_command_surface.json -output-dir schema_agent_metadata -audit-output schema_agent_metadata_audit.json
//go:generate go run ../generator/cmd_schema_catalog -surface schema_command_surface.json -output schema_catalog.json

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
	SurfaceProducts      int `json:"surface_products,omitempty"`
	ProductsWithMetadata int `json:"products_with_metadata"`
	SurfaceTools         int `json:"surface_tools,omitempty"`
	ToolsWithMetadata    int `json:"tools_with_metadata"`
	ToolsWithSummary     int `json:"tools_with_agent_summary,omitempty"`
	UnmatchedSkillTools  int `json:"unmatched_skill_tools,omitempty"`
}

type agentProductMetadata struct {
	AgentSummary       string   `json:"agent_summary,omitempty"`
	AgentSummarySource string   `json:"agent_summary_source,omitempty"`
	UseWhen            []string `json:"use_when,omitempty"`
	AvoidWhen          []string `json:"avoid_when,omitempty"`
	SourceRefs         []string `json:"source_refs,omitempty"`
}

type agentToolMetadata struct {
	AgentSummary       string   `json:"agent_summary,omitempty"`
	AgentSummarySource string   `json:"agent_summary_source,omitempty"`
	UseWhen            []string `json:"use_when,omitempty"`
	AvoidWhen          []string `json:"avoid_when,omitempty"`
	Prerequisites      []string `json:"prerequisites,omitempty"`
	Tips               []string `json:"tips,omitempty"`
	Effect             string   `json:"effect,omitempty"`
	EffectSource       string   `json:"effect_source,omitempty"`
	Risk               string   `json:"risk,omitempty"`
	Confirmation       string   `json:"confirmation,omitempty"`
	Idempotency        string   `json:"idempotency,omitempty"`
	WorkflowRefs       []string `json:"workflow_refs,omitempty"`
	Examples           []string `json:"examples,omitempty"`
	Reviewed           *bool    `json:"reviewed,omitempty"`
	SourceRefs         []string `json:"source_refs,omitempty"`
}

type embeddedAgentMetadataDomain struct {
	ProductID string                       `json:"product_id"`
	Tools     map[string]agentToolMetadata `json:"tools"`
}

var runtimeEmbeddedAgentMetadata = loadEmbeddedAgentMetadata()

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

func applyAgentProductMetadata(target map[string]any, productIDs ...string) bool {
	if target == nil {
		return false
	}
	for _, productID := range productIDs {
		productID = strings.TrimSpace(productID)
		metadata, ok := runtimeEmbeddedAgentMetadata.Products[productID]
		if !ok {
			continue
		}
		setString(target, "agent_summary", metadata.AgentSummary)
		setString(target, "agent_summary_source", metadata.AgentSummarySource)
		setStringSlice(target, "use_when", metadata.UseWhen)
		setStringSlice(target, "avoid_when", metadata.AvoidWhen)
		setStringSlice(target, "agent_source_refs", metadata.SourceRefs)
		target["agent_metadata_source"] = embeddedAgentMetadataSource
		return true
	}
	return false
}

func applyCompactAgentProductMetadata(target map[string]any, productIDs ...string) bool {
	if target == nil {
		return false
	}
	for _, productID := range productIDs {
		metadata, ok := runtimeEmbeddedAgentMetadata.Products[strings.TrimSpace(productID)]
		if !ok {
			continue
		}
		if summary := strings.TrimSpace(metadata.AgentSummary); summary != "" {
			target["agent_summary"] = summary
		} else if len(metadata.UseWhen) > 0 {
			target["use_when"] = []string{metadata.UseWhen[0]}
		}
		return true
	}
	return false
}

func applyAgentToolMetadata(target map[string]any, includeExamples bool, paths ...string) bool {
	if target == nil {
		return false
	}
	metadata, ok := lookupAgentToolMetadata(paths...)
	if !ok {
		return false
	}
	setString(target, "agent_summary", metadata.AgentSummary)
	setString(target, "agent_summary_source", metadata.AgentSummarySource)
	setStringSlice(target, "use_when", metadata.UseWhen)
	setStringSlice(target, "avoid_when", metadata.AvoidWhen)
	setStringSlice(target, "prerequisites", metadata.Prerequisites)
	setStringSlice(target, "tips", metadata.Tips)
	setString(target, "effect", metadata.Effect)
	setString(target, "risk", metadata.Risk)
	setString(target, "confirmation", metadata.Confirmation)
	setString(target, "idempotency", metadata.Idempotency)
	setStringSlice(target, "workflow_refs", metadata.WorkflowRefs)
	if metadata.Reviewed != nil {
		target["reviewed"] = *metadata.Reviewed
	}
	if includeExamples {
		setStringSlice(target, "examples", metadata.Examples)
		setString(target, "effect_source", metadata.EffectSource)
		setStringSlice(target, "agent_source_refs", metadata.SourceRefs)
	}
	target["agent_metadata_source"] = embeddedAgentMetadataSource
	return true
}

func lookupAgentToolMetadata(paths ...string) (agentToolMetadata, bool) {
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
			if metadata, ok := runtimeEmbeddedAgentMetadata.Tools[candidate]; ok {
				return metadata, true
			}
		}
	}
	return agentToolMetadata{}, false
}

func agentMetadataSummary() map[string]any {
	metadata := runtimeEmbeddedAgentMetadata
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

func setString(target map[string]any, key, value string) {
	if value = strings.TrimSpace(value); value != "" {
		target[key] = value
	}
}

func setStringSlice(target map[string]any, key string, values []string) {
	if len(values) > 0 {
		target[key] = append([]string(nil), values...)
	}
}
