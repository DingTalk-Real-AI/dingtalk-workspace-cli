// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package schemaruntime

import (
	"slices"
	"sort"
	"strings"
)

// equalCommandMeta preserves exact DTO semantics, including nil versus empty
// slices, without reflect's per-entry map-value copies and visited map.
func equalCommandMeta(a, b CommandMeta) bool {
	return a.Identity.CLIPath == b.Identity.CLIPath &&
		a.Identity.Canonical == b.Identity.Canonical &&
		a.Identity.ProductID == b.Identity.ProductID &&
		a.Identity.Title == b.Identity.Title &&
		equalMetaStrings(a.Identity.Aliases, b.Identity.Aliases) &&
		a.Safety == b.Safety &&
		a.Selection.AgentSummary == b.Selection.AgentSummary &&
		equalMetaStrings(a.Selection.UseWhen, b.Selection.UseWhen) &&
		equalMetaStrings(a.Selection.AvoidWhen, b.Selection.AvoidWhen) &&
		equalMetaStrings(a.Selection.Prerequisites, b.Selection.Prerequisites) &&
		equalMetaStrings(a.Selection.Tips, b.Selection.Tips) &&
		equalMetaStrings(a.Selection.Examples, b.Selection.Examples)
}

func equalMetaStrings(a, b []string) bool {
	return (a == nil) == (b == nil) && slices.Equal(a, b)
}

func equalCommandMetaLookups(a, b map[string]CommandMeta) bool {
	if (a == nil) != (b == nil) || len(a) != len(b) {
		return false
	}
	for path, left := range a {
		right, ok := b[path]
		if !ok || !equalCommandMeta(left, right) {
			return false
		}
	}
	return true
}

// CommandMeta is the immutable runtime metadata projection for one command.
type CommandMeta struct {
	Identity  CommandIdentity
	Safety    CommandSafety
	Selection CommandSelection
}

// CommandIdentity is the stable identity of a command.
type CommandIdentity struct {
	CLIPath   string
	Canonical string
	Aliases   []string
	ProductID string
	Title     string
}

// CommandSafety is the runtime safety view projected from ToolSpec.
type CommandSafety struct {
	Effect       string
	Risk         string
	Confirmation string
	Idempotency  string
}

// ShouldRender reports whether any reviewed safety metadata is available.
func (s CommandSafety) ShouldRender() bool {
	return strings.TrimSpace(s.Effect) != "" ||
		strings.TrimSpace(s.Risk) != "" ||
		strings.TrimSpace(s.Confirmation) != "" ||
		strings.TrimSpace(s.Idempotency) != ""
}

// CommandSelection is the agent-facing selection metadata.
type CommandSelection struct {
	AgentSummary  string
	UseWhen       []string
	AvoidWhen     []string
	Prerequisites []string
	Tips          []string
	Examples      []string
}

// CommandMetaFromTool projects one typed leaf without reading wire maps.
func CommandMetaFromTool(tool ToolSpec) CommandMeta {
	return CommandMeta{
		Identity: CommandIdentity{
			CLIPath:   strings.TrimSpace(tool.Identity.CLIPath),
			Canonical: tool.Identity.CanonicalPath,
			Aliases:   append([]string(nil), tool.Identity.Aliases...),
			ProductID: tool.Identity.ProductID,
			Title:     tool.Title,
		},
		Safety: CommandSafety{
			Effect:       tool.Safety.Effect,
			Risk:         tool.Safety.Risk,
			Confirmation: tool.Safety.Confirmation,
			Idempotency:  tool.Safety.Idempotency,
		},
		Selection: CommandSelection{
			AgentSummary:  tool.Selection.AgentSummary,
			UseWhen:       append([]string(nil), tool.Selection.UseWhen...),
			AvoidWhen:     append([]string(nil), tool.Selection.AvoidWhen...),
			Prerequisites: append([]string(nil), tool.Selection.Prerequisites...),
			Tips:          append([]string(nil), tool.Selection.Tips...),
			Examples:      append([]string(nil), tool.Selection.Examples...),
		},
	}
}

// BuildCommandMetaLookup projects primary and compatibility alias paths.
func BuildCommandMetaLookup(registry SchemaRegistry) map[string]CommandMeta {
	lookup := make(map[string]CommandMeta)
	metas := make([]CommandMeta, 0, 64)
	for _, product := range registry.Products {
		for _, tool := range product.Tools {
			meta := CommandMetaFromTool(tool)
			if meta.Identity.CLIPath == "" {
				continue
			}
			lookup[meta.Identity.CLIPath] = meta
			metas = append(metas, meta)
		}
	}
	return RegisterCommandMetaAliases(lookup, metas)
}

// RegisterCommandMetaAliases keeps primary paths authoritative and resolves
// alias collisions by lexicographically smallest primary path.
func RegisterCommandMetaAliases(lookup map[string]CommandMeta, metas []CommandMeta) map[string]CommandMeta {
	sort.Slice(metas, func(i, j int) bool {
		return metas[i].Identity.CLIPath < metas[j].Identity.CLIPath
	})
	for _, meta := range metas {
		for _, alias := range meta.Identity.Aliases {
			alias = strings.TrimSpace(alias)
			if alias == "" || alias == meta.Identity.CLIPath {
				continue
			}
			if _, exists := lookup[alias]; !exists {
				lookup[alias] = meta
			}
		}
	}
	return lookup
}

// validMetaAliasExpansion checks the same primary-wins / lexical-alias-owner
// rule as RegisterCommandMetaAliases without copying/sorting large metadata.
func validMetaAliasExpansion(lookup map[string]CommandMeta) bool {
	if lookup == nil {
		return false
	}
	owners := make(map[string]string)
	for path, meta := range lookup {
		primary, found := lookup[meta.Identity.CLIPath]
		if !found || !equalCommandMeta(primary, meta) {
			return false
		}
		if path != meta.Identity.CLIPath {
			continue
		}
		for _, raw := range meta.Identity.Aliases {
			alias := strings.TrimSpace(raw)
			if alias == "" || alias == path {
				continue
			}
			if entry, found := lookup[alias]; found && entry.Identity.CLIPath == alias {
				continue
			}
			if owner, found := owners[alias]; !found || path < owner {
				owners[alias] = path
			}
		}
	}
	for alias, owner := range owners {
		entry, found := lookup[alias]
		if !found || entry.Identity.CLIPath != owner {
			return false
		}
	}
	for path, meta := range lookup {
		if path != meta.Identity.CLIPath && owners[path] != meta.Identity.CLIPath {
			return false
		}
	}
	return true
}

func commandMetaSubsetEqual(whole, subset map[string]CommandMeta) bool {
	for path, expected := range subset {
		actual, found := whole[path]
		if !found || !equalCommandMeta(actual, expected) {
			return false
		}
	}
	return true
}

func locatorSubsetEqual(whole, subset map[string]string) bool {
	for path, expected := range subset {
		actual, found := whole[path]
		if !found || actual != expected {
			return false
		}
	}
	return true
}
