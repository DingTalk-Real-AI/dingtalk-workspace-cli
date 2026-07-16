// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0

package agentmetadata

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

const defaultInterfaceSummaryRunes = 120

// InterfaceMetadataAudit records how sanitized MCP descriptions contributed
// to Agent summaries. Interface metadata is fallback-only and never infers
// effect, risk, confirmation, or idempotency.
type InterfaceMetadataAudit struct {
	Source             string   `json:"source,omitempty"`
	Revision           string   `json:"revision,omitempty"`
	SourceHash         string   `json:"source_hash,omitempty"`
	SourceTools        int      `json:"source_tools"`
	SurfaceTools       int      `json:"surface_tools"`
	EligibleSummaries  int      `json:"eligible_summaries"`
	AppliedSummaries   int      `json:"applied_summaries"`
	PreservedSummaries int      `json:"preserved_summaries"`
	RejectedTools      []string `json:"rejected_tools,omitempty"`
	OutsideSurface     []string `json:"outside_surface,omitempty"`
}

type interfaceMetadataFile struct {
	Version        int                              `json:"version"`
	Source         string                           `json:"source"`
	SourceRevision string                           `json:"source_revision,omitempty"`
	SourceHash     string                           `json:"source_hash"`
	Tools          map[string]interfaceMetadataTool `json:"tools"`
}

type interfaceMetadataTool struct {
	Title        string        `json:"title,omitempty"`
	Description  string        `json:"description,omitempty"`
	InterfaceRef *InterfaceRef `json:"interface_ref,omitempty"`
}

func applyInterfaceMetadataFallback(out *File, files map[string]sourceFile, opts Options, stats *Stats, origins sourceTracker) error {
	if strings.TrimSpace(opts.InterfaceMetadataPath) == "" {
		return nil
	}
	display := displayPath(opts.Root, resolvePath(opts.Root, opts.InterfaceMetadataPath))
	file, ok := files[display]
	if !ok {
		return fmt.Errorf("interface metadata source %s was not loaded", display)
	}
	var source interfaceMetadataFile
	if err := json.Unmarshal(file.data, &source); err != nil {
		return fmt.Errorf("decode interface metadata %s: %w", display, err)
	}
	if source.Version != 1 {
		return fmt.Errorf("decode interface metadata %s: unsupported version %d", display, source.Version)
	}

	audit := &InterfaceMetadataAudit{
		Source:      strings.TrimSpace(source.Source),
		Revision:    strings.TrimSpace(source.SourceRevision),
		SourceHash:  strings.TrimSpace(source.SourceHash),
		SourceTools: len(source.Tools),
	}
	stats.InterfaceMetadata = audit
	maxRunes := opts.MaxInterfaceSummaryRunes
	if maxRunes <= 0 {
		maxRunes = defaultInterfaceSummaryRunes
	}
	paths := make([]string, 0, len(source.Tools))
	for path := range source.Tools {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		if len(opts.ToolPaths) > 0 {
			if _, exists := opts.ToolPaths[path]; !exists {
				audit.OutsideSurface = append(audit.OutsideSurface, path)
				continue
			}
		}
		audit.SurfaceTools++
		incoming := source.Tools[path]
		metadata := out.Tools[path]
		sourceRef := display + "#tools." + path
		if incoming.InterfaceRef != nil {
			candidate := ToolMetadata{
				InterfaceRef:        incoming.InterfaceRef,
				interfaceRefRank:    selectionRankMCPFallback,
				interfaceRefOrigin:  sourceRef + ".interface_ref",
				InterfaceMode:       "mcp",
				interfaceModeRank:   selectionRankMCPFallback,
				interfaceModeOrigin: sourceRef + ".interface_ref",
				Availability:        "available",
				availabilityRank:    selectionRankMCPFallback,
				availabilityOrigin:  sourceRef + ".interface_ref",
			}
			recordFieldCandidate(&candidate, "interface_ref", incoming.InterfaceRef.ProductID+"."+incoming.InterfaceRef.RPCName,
				selectionRankMCPFallback, candidate.interfaceRefOrigin, "")
			if err := mergeRankedInterfaceRef(&metadata, candidate, path); err != nil {
				return err
			}
			for _, field := range []struct {
				name   string
				target *string
				rank   *int
				origin *string
				value  string
			}{
				{name: "interface_mode", target: &metadata.InterfaceMode, rank: &metadata.interfaceModeRank, origin: &metadata.interfaceModeOrigin, value: candidate.InterfaceMode},
				{name: "availability", target: &metadata.Availability, rank: &metadata.availabilityRank, origin: &metadata.availabilityOrigin, value: candidate.Availability},
			} {
				recordFieldCandidate(&metadata, field.name, field.value, selectionRankMCPFallback, candidate.interfaceRefOrigin, "")
				if err := mergeRankedString(field.target, field.rank, field.origin, field.value, selectionRankMCPFallback, candidate.interfaceRefOrigin, path, field.name); err != nil {
					return err
				}
			}
			metadata.SourceRefs = append(metadata.SourceRefs, sourceRef)
			out.Tools[path] = metadata
			origins.add(path, display, 0)
		}
		summary := summarizeInterfaceDescription(incoming.Description, maxRunes)
		if summary == "" {
			audit.RejectedTools = append(audit.RejectedTools, path)
			continue
		}
		audit.EligibleSummaries++
		if hasSurfaceAgentSummary(*out, opts.ToolPaths, path) {
			audit.PreservedSummaries++
			continue
		}

		metadata = out.Tools[path]
		metadata.AgentSummary = summary
		metadata.agentSummaryPresent = true
		metadata.AgentSummarySource = interfaceSummarySource(source)
		metadata.agentSummaryRank = selectionRankMCPFallback
		metadata.agentSummaryOrigin = display + "#tools." + path
		recordFieldCandidate(&metadata, "agent_summary", summary, selectionRankMCPFallback, metadata.agentSummaryOrigin, "")
		if metadata.Reviewed == nil {
			reviewed := false
			metadata.Reviewed = &reviewed
			metadata.reviewedRank = selectionRankMCPFallback
			metadata.reviewedOrigin = sourceRef
			recordTypedFieldCandidateValue(&metadata, "reviewed", false, true, metadata.reviewedRank, metadata.reviewedOrigin, "")
		}
		metadata.SourceRefs = append(metadata.SourceRefs, sourceRef)
		out.Tools[path] = metadata
		origins.add(path, display, 0)
		audit.AppliedSummaries++
	}
	sort.Strings(audit.RejectedTools)
	sort.Strings(audit.OutsideSurface)
	return nil
}

func hasSurfaceAgentSummary(file File, paths map[string]string, canonical string) bool {
	livePath := canonical
	if resolved := strings.TrimSpace(paths[canonical]); resolved != "" {
		livePath = resolved
	}
	for path, metadata := range file.Tools {
		if !scalarIsPresent(metadata.AgentSummary, metadata.agentSummaryPresent) {
			continue
		}
		candidate := path
		if resolved := strings.TrimSpace(paths[path]); resolved != "" {
			candidate = resolved
		}
		if candidate == livePath {
			return true
		}
	}
	return false
}

func interfaceSummarySource(source interfaceMetadataFile) string {
	name := "mcp-interface"
	if value := strings.TrimSpace(source.Source); value != "" {
		name = value
	}
	if revision := strings.TrimSpace(source.SourceRevision); revision != "" {
		if len(revision) > 12 {
			revision = revision[:12]
		}
		name += "@" + revision
	}
	return name
}

func summarizeInterfaceDescription(description string, maxRunes int) string {
	description = strings.ReplaceAll(description, "\\n", "\n")
	description = strings.ReplaceAll(description, "\r\n", "\n")
	paragraph := make([]string, 0, 4)
	for _, raw := range strings.Split(description, "\n") {
		line := cleanInterfaceSummaryLine(raw)
		if line == "" {
			if len(paragraph) > 0 {
				break
			}
			continue
		}
		paragraph = append(paragraph, line)
	}
	text := strings.Join(strings.Fields(strings.Join(paragraph, " ")), " ")
	if text == "" || interfaceIdentifierOnly(text) {
		return ""
	}
	if maxRunes <= 0 {
		maxRunes = defaultInterfaceSummaryRunes
	}
	if sentence := firstInterfaceSentence(text); sentence != "" && utf8.RuneCountInString(sentence) <= maxRunes {
		return sentence
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	limit := maxRunes - 3
	if limit < 1 {
		limit = 1
	}
	cut := limit
	for index := limit - 1; index >= limit/2; index-- {
		if strings.ContainsRune("，,；; ", runes[index]) {
			cut = index
			break
		}
	}
	return strings.TrimSpace(string(runes[:cut])) + "..."
}

func cleanInterfaceSummaryLine(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimLeft(value, "#*- ")
	value = strings.ReplaceAll(value, "**", "")
	value = strings.ReplaceAll(value, "__", "")
	value = strings.ReplaceAll(value, "`", "")
	return strings.TrimSpace(value)
}

func firstInterfaceSentence(value string) string {
	runes := []rune(value)
	for index, current := range runes {
		if index < 3 {
			continue
		}
		if strings.ContainsRune("。！？!?；;", current) {
			return strings.TrimSpace(string(runes[:index+1]))
		}
		if current == '.' && index+1 < len(runes) && runes[index+1] == ' ' {
			return strings.TrimSpace(string(runes[:index+1]))
		}
	}
	return ""
}

func interfaceIdentifierOnly(value string) bool {
	if value == "" {
		return false
	}
	for _, current := range value {
		if current > 127 {
			return false
		}
		if (current >= 'a' && current <= 'z') ||
			(current >= 'A' && current <= 'Z') ||
			(current >= '0' && current <= '9') ||
			strings.ContainsRune("_.-", current) {
			continue
		}
		return false
	}
	return true
}
