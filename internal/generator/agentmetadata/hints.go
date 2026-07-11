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

// HintFile is a versioned source contract. Imported files provide a reviewed
// baseline; explicit files can override scalar fields while Skills continue to
// supply routing and workflow context.
type HintFile struct {
	Version  int                    `json:"version"`
	Source   HintSource             `json:"source"`
	Coverage HintCoverage           `json:"coverage,omitempty"`
	Products map[string]HintProduct `json:"products,omitempty"`
	Tools    map[string]HintTool    `json:"tools,omitempty"`
}

type HintSource struct {
	Kind       string `json:"kind"`
	Name       string `json:"name"`
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
	AgentSummary string   `json:"agent_summary,omitempty"`
	UseWhen      []string `json:"use_when,omitempty"`
	AvoidWhen    []string `json:"avoid_when,omitempty"`
	SourceRefs   []string `json:"source_refs,omitempty"`
}

type HintTool struct {
	AgentSummary  string        `json:"agent_summary,omitempty"`
	UseWhen       []string      `json:"use_when,omitempty"`
	AvoidWhen     []string      `json:"avoid_when,omitempty"`
	Prerequisites []string      `json:"prerequisites,omitempty"`
	Tips          []string      `json:"tips,omitempty"`
	Effect        string        `json:"effect,omitempty"`
	Risk          string        `json:"risk,omitempty"`
	Confirmation  string        `json:"confirmation,omitempty"`
	Idempotency   string        `json:"idempotency,omitempty"`
	WorkflowRefs  []string      `json:"workflow_refs,omitempty"`
	Examples      []string      `json:"examples,omitempty"`
	Reviewed      *bool         `json:"reviewed,omitempty"`
	SourceRefs    []string      `json:"source_refs,omitempty"`
	InterfaceRef  *InterfaceRef `json:"interface_ref,omitempty"`
}

type parsedHintSource struct {
	file sourceFile
	hint HintFile
}

func parseHintSources(out *File, files []sourceFile, opts Options, stats *Stats, origins sourceTracker) error {
	if strings.TrimSpace(opts.HintsDir) == "" {
		return nil
	}
	hintsRoot := filepath.Clean(resolvePath(opts.Root, opts.HintsDir))
	hintsPrefix := hintsRoot + string(filepath.Separator)
	parsed := []parsedHintSource{}
	for _, file := range files {
		cleaned := filepath.Clean(file.path)
		if cleaned != hintsRoot && !strings.HasPrefix(cleaned, hintsPrefix) {
			continue
		}
		var hint HintFile
		if err := json.Unmarshal(file.data, &hint); err != nil {
			return fmt.Errorf("decode Agent hint %s: %w", file.display, err)
		}
		if hint.Version != HintFileVersion {
			return fmt.Errorf("decode Agent hint %s: unsupported version %d", file.display, hint.Version)
		}
		kind := strings.ToLower(strings.TrimSpace(hint.Source.Kind))
		if kind == "" {
			kind = "explicit"
		}
		if kind != "explicit" && kind != "imported" {
			return fmt.Errorf("decode Agent hint %s: unsupported source kind %q", file.display, hint.Source.Kind)
		}
		for path, tool := range hint.Tools {
			if tool.InterfaceRef == nil {
				continue
			}
			tool.InterfaceRef.ProductID = strings.TrimSpace(tool.InterfaceRef.ProductID)
			tool.InterfaceRef.RPCName = strings.TrimSpace(tool.InterfaceRef.RPCName)
			if tool.InterfaceRef.ProductID == "" || tool.InterfaceRef.RPCName == "" {
				return fmt.Errorf("decode Agent hint %s: tool %s has incomplete interface_ref", file.display, path)
			}
			hint.Tools[path] = tool
		}
		hint.Source.Kind = kind
		parsed = append(parsed, parsedHintSource{file: file, hint: hint})
	}
	sort.Slice(parsed, func(i, j int) bool {
		left, right := hintKindPriority(parsed[i].hint.Source.Kind), hintKindPriority(parsed[j].hint.Source.Kind)
		if left != right {
			return left < right
		}
		return parsed[i].file.display < parsed[j].file.display
	})
	for _, source := range parsed {
		applyHintSource(out, source, stats, origins)
	}
	return nil
}

func hintKindPriority(kind string) int {
	if kind == "imported" {
		return 0
	}
	return 1
}

func applyHintSource(out *File, parsed parsedHintSource, stats *Stats, origins sourceTracker) {
	hint := parsed.hint
	explicit := hint.Source.Kind == "explicit"
	sourceLabel := hintSourceLabel(hint.Source, parsed.file.display)
	stats.HintFiles++

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
		if value := strings.TrimSpace(incoming.AgentSummary); value != "" && (explicit || metadata.AgentSummary == "") {
			metadata.AgentSummary = value
			metadata.AgentSummarySource = sourceLabel
		}
		metadata.UseWhen = append(metadata.UseWhen, incoming.UseWhen...)
		metadata.AvoidWhen = append(metadata.AvoidWhen, incoming.AvoidWhen...)
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
		if value := strings.TrimSpace(incoming.AgentSummary); value != "" && (explicit || metadata.AgentSummary == "") {
			metadata.AgentSummary = value
			metadata.AgentSummarySource = sourceLabel
		}
		metadata.UseWhen = append(metadata.UseWhen, incoming.UseWhen...)
		metadata.AvoidWhen = append(metadata.AvoidWhen, incoming.AvoidWhen...)
		metadata.Prerequisites = append(metadata.Prerequisites, incoming.Prerequisites...)
		metadata.Tips = append(metadata.Tips, incoming.Tips...)
		metadata.WorkflowRefs = append(metadata.WorkflowRefs, incoming.WorkflowRefs...)
		metadata.Examples = append(metadata.Examples, incoming.Examples...)
		applyHintScalar(&metadata.Effect, incoming.Effect, explicit)
		if metadata.Effect != "" && metadata.EffectSource == "" {
			metadata.EffectSource = "agent-hint"
		}
		applyHintScalar(&metadata.Risk, incoming.Risk, explicit)
		applyHintScalar(&metadata.Confirmation, incoming.Confirmation, explicit)
		applyHintScalar(&metadata.Idempotency, incoming.Idempotency, explicit)
		if incoming.Reviewed != nil && (explicit || metadata.Reviewed == nil) {
			value := *incoming.Reviewed
			metadata.Reviewed = &value
		}
		if incoming.InterfaceRef != nil && (explicit || metadata.InterfaceRef == nil) {
			value := *incoming.InterfaceRef
			metadata.InterfaceRef = &value
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
}

func hasAgentHintFields(hint HintTool) bool {
	return strings.TrimSpace(hint.AgentSummary) != "" ||
		len(hint.UseWhen) > 0 ||
		len(hint.AvoidWhen) > 0 ||
		len(hint.Prerequisites) > 0 ||
		len(hint.Tips) > 0 ||
		strings.TrimSpace(hint.Effect) != "" ||
		strings.TrimSpace(hint.Risk) != "" ||
		strings.TrimSpace(hint.Confirmation) != "" ||
		strings.TrimSpace(hint.Idempotency) != "" ||
		len(hint.WorkflowRefs) > 0 ||
		len(hint.Examples) > 0 ||
		hint.Reviewed != nil ||
		len(hint.SourceRefs) > 0 ||
		hint.InterfaceRef != nil
}

func applyHintScalar(target *string, incoming string, override bool) {
	incoming = strings.TrimSpace(incoming)
	if incoming != "" && (override || strings.TrimSpace(*target) == "") {
		*target = incoming
	}
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
