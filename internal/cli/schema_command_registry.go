// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// Command identity is collected from the live Cobra command tree: every
// runnable leaf carrying ContractFinal.Identity contributes one CommandSpec
// (CollectIdentitySpecs in schema_identity_collect.go). The reviewed
// schema_command_registry/ was retired together with that switchover; the
// collector is the single source of stable command identity and navigation.
// Catalog and generated metadata are downstream views and must never be read
// back here. Peer reviewed inputs (param_concepts, exclusions, mapping
// ledger) stay separate — see AGENTS.md "Reviewed inputs". The bindings
// audit JSON and the MCP metadata/service-review pins are retired and must
// not reappear.

var (
	commandRegistryProductIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)
	commandRegistryCanonicalPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*\.[A-Za-z0-9][A-Za-z0-9_.-]*$`)
	commandRegistryCLIPathToken     = regexp.MustCompile(`^(?:[A-Za-z0-9][A-Za-z0-9._:-]*|\+[A-Za-z0-9][A-Za-z0-9._:-]*)$`)
)

// CommandSourceContractIdentity is the delivery source label for command
// identity collected from ContractFinal.Identity declarations. The collector
// (CollectIdentitySpecs) is the single identity source since the reviewed
// schema_command_registry retirement.
const CommandSourceContractIdentity = "contract_identity"

// CommandSpec is one command identity. Identity and navigation are
// deliberately kept together so no downstream renderer can independently
// invent a canonical name, primary path, or alias.
type CommandSpec struct {
	CanonicalPath   string
	SourceProductID string
	PrimaryCLIPath  string
	Aliases         []string
	Visibility      SchemaVisibility
	Source          string
	ReviewReason    string
}

// CommandRegistry is an indexed command identity set.
type CommandRegistry struct {
	Commands    []CommandSpec
	ByCLIPath   map[string]CommandSpec
	ByCanonical map[string]CommandSpec
}

// EffectiveCommandRegistry is the collected command identity registry after
// indexing. It remains independent of Cobra; binding is a separate
// fail-closed step.
type EffectiveCommandRegistry struct {
	Commands    []CommandSpec
	ByCLIPath   map[string]CommandSpec
	ByCanonical map[string]CommandSpec
}

// SourceHash hashes only stable identity, navigation, and reviewed exposure.
// Formatting, product order, provenance labels, and omitted default
// source_product_id/visibility values do not affect it.
func (registry CommandRegistry) SourceHash() string {
	return hashCommandSpecs(registry.Commands)
}

// SourceHash covers every effective command identity delivered to downstream
// consumers.
func (registry EffectiveCommandRegistry) SourceHash() string {
	return hashCommandSpecs(registry.Commands)
}

func hashCommandSpecs(commands []CommandSpec) string {
	rows := make([]string, 0, len(commands))
	for _, spec := range commands {
		productID, _, _ := splitManualSchemaCanonicalPath(spec.CanonicalPath)
		sourceProductID := strings.TrimSpace(spec.SourceProductID)
		if sourceProductID == productID {
			sourceProductID = ""
		}
		aliases := normalizeCommandAliases(spec.Aliases, normalizeSchemaCLIPath(spec.PrimaryCLIPath))
		visibility := spec.Visibility
		if visibility == "" {
			visibility = SchemaVisibilityPublic
		}
		rows = append(rows, productID+"\x00"+strings.TrimSpace(spec.CanonicalPath)+"\x00"+sourceProductID+"\x00"+normalizeSchemaCLIPath(spec.PrimaryCLIPath)+"\x00"+strings.Join(aliases, "\x00")+"\x00"+string(visibility))
	}
	sort.Strings(rows)
	sum := sha256.Sum256([]byte(strings.Join(rows, "\n")))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func newEffectiveCommandRegistry(commands []CommandSpec) (EffectiveCommandRegistry, error) {
	normalized, byPath, byCanonical, err := indexCommandSpecs(commands)
	if err != nil {
		return EffectiveCommandRegistry{}, err
	}
	return EffectiveCommandRegistry{Commands: normalized, ByCLIPath: byPath, ByCanonical: byCanonical}, nil
}

func indexCommandSpecs(commands []CommandSpec) ([]CommandSpec, map[string]CommandSpec, map[string]CommandSpec, error) {
	normalized := make([]CommandSpec, 0, len(commands))
	byPath := make(map[string]CommandSpec, len(commands))
	byCanonical := make(map[string]CommandSpec, len(commands))
	for _, raw := range commands {
		spec := cloneCommandSpec(raw)
		spec.CanonicalPath = strings.TrimSpace(spec.CanonicalPath)
		productID, _, ok := splitManualSchemaCanonicalPath(spec.CanonicalPath)
		if !ok || !commandRegistryCanonicalPattern.MatchString(spec.CanonicalPath) {
			return nil, nil, nil, fmt.Errorf("command identity has invalid canonical path %q", raw.CanonicalPath)
		}
		spec.SourceProductID = strings.TrimSpace(spec.SourceProductID)
		if spec.SourceProductID == "" {
			spec.SourceProductID = productID
		}
		if !validCommandRegistryProductID(spec.SourceProductID) {
			return nil, nil, nil, fmt.Errorf("command identity %s has invalid source_product_id %q", spec.CanonicalPath, raw.SourceProductID)
		}
		spec.PrimaryCLIPath = normalizeSchemaCLIPath(spec.PrimaryCLIPath)
		if !validCommandRegistryCLIPath(spec.PrimaryCLIPath) {
			return nil, nil, nil, fmt.Errorf("command identity %s has invalid primary cli path %q", spec.CanonicalPath, raw.PrimaryCLIPath)
		}
		aliases := make([]string, 0, len(spec.Aliases))
		seenAliases := make(map[string]bool, len(spec.Aliases))
		for _, rawAlias := range spec.Aliases {
			alias := normalizeSchemaCLIPath(rawAlias)
			if !validCommandRegistryCLIPath(alias) {
				return nil, nil, nil, fmt.Errorf("command identity %s has invalid alias path %q", spec.CanonicalPath, alias)
			}
			if alias == spec.PrimaryCLIPath {
				return nil, nil, nil, fmt.Errorf("command identity %s alias %q duplicates its primary path", spec.CanonicalPath, alias)
			}
			if seenAliases[alias] {
				return nil, nil, nil, fmt.Errorf("command identity %s has duplicate alias path %q", spec.CanonicalPath, alias)
			}
			seenAliases[alias] = true
			aliases = append(aliases, alias)
		}
		spec.Aliases = sortedUniqueStrings(aliases)
		if spec.Visibility == "" {
			spec.Visibility = SchemaVisibilityPublic
		}
		switch spec.Visibility {
		case SchemaVisibilityPublic, SchemaVisibilityCompat, SchemaVisibilityInternal:
		default:
			return nil, nil, nil, fmt.Errorf("command identity %s has invalid visibility %q", spec.CanonicalPath, spec.Visibility)
		}
		spec.Source = strings.TrimSpace(spec.Source)
		if spec.Source == "" {
			spec.Source = CommandSourceContractIdentity
		}
		spec.ReviewReason = strings.TrimSpace(spec.ReviewReason)
		if previous, exists := byCanonical[spec.CanonicalPath]; exists {
			return nil, nil, nil, fmt.Errorf("duplicate command identity canonical path %s (primary paths %q and %q)", spec.CanonicalPath, previous.PrimaryCLIPath, spec.PrimaryCLIPath)
		}
		byCanonical[spec.CanonicalPath] = spec
		for _, path := range append([]string{spec.PrimaryCLIPath}, spec.Aliases...) {
			if previous, exists := byPath[path]; exists {
				return nil, nil, nil, fmt.Errorf("command identity path %q belongs to both %s and %s", path, previous.CanonicalPath, spec.CanonicalPath)
			}
			byPath[path] = spec
		}
		normalized = append(normalized, spec)
	}
	for path, owner := range byPath {
		if canonicalOwner, exists := byCanonical[path]; exists {
			return nil, nil, nil, fmt.Errorf(
				"command identity CLI path %q for %s conflicts with canonical identity %s",
				path,
				owner.CanonicalPath,
				canonicalOwner.CanonicalPath,
			)
		}
	}
	sort.Slice(normalized, func(i, j int) bool { return normalized[i].CanonicalPath < normalized[j].CanonicalPath })
	return normalized, byPath, byCanonical, nil
}

func validCommandRegistryProductID(value string) bool {
	return commandRegistryProductIDPattern.MatchString(strings.TrimSpace(value))
}

// validCommandRegistryCLIPath is intentionally stricter than
// normalizeSchemaCLIPath: collected identity must already be canonical and
// may not rely on normalization to hide a leading dws, repeated whitespace,
// flags, or wildcard syntax.
func validCommandRegistryCLIPath(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || strings.HasPrefix(value, "dws ") || strings.ContainsAny(value, "*?[]") {
		return false
	}
	parts := strings.Split(value, " ")
	for _, part := range parts {
		if !commandRegistryCLIPathToken.MatchString(part) {
			return false
		}
	}
	return true
}

func normalizeCommandAliases(aliases []string, primary string) []string {
	normalized := make([]string, 0, len(aliases))
	for _, alias := range aliases {
		alias = normalizeSchemaCLIPath(alias)
		if alias != "" && alias != primary {
			normalized = append(normalized, alias)
		}
	}
	return sortedUniqueStrings(normalized)
}

// BuildEffectiveCommandRegistry assembles the effective command identity
// registry by collecting ContractFinal.Identity from the live Cobra leaves
// under root. The collector is the single identity source; there is no
// separate reviewed identity file to merge or overlay.
//
// Parameter mapping ledger (schema_parameter_mapping_ledger.go —
// mapping_exclusions / removals; active bindings retired to ParamDecl.Property)
// is validated at BindEffectiveCommandRegistry and catalog assembly, not here.
// Identity registry construction must not hard-depend on active binding rows.
func BuildEffectiveCommandRegistry(root *cobra.Command) (EffectiveCommandRegistry, error) {
	if root == nil {
		return EffectiveCommandRegistry{}, fmt.Errorf("build effective Schema command registry: root is nil")
	}
	collected, _, err := CollectIdentitySpecs(root)
	if err != nil {
		return EffectiveCommandRegistry{}, err
	}
	return newEffectiveCommandRegistry(collected)
}

func cloneCommandSpec(spec CommandSpec) CommandSpec {
	spec.Aliases = append([]string(nil), spec.Aliases...)
	return spec
}
