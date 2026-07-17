// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/spf13/cobra"
)

const commandRegistrySchemaRef = "./schema_command_registry.schema.json"

// schema_command_registry.json is the reviewed, typed command registry and the
// sole source of stable command identity and navigation. Catalog and generated
// metadata are downstream views and must never be read back here.

//go:embed schema_command_registry.json
var embeddedSchemaCommandRegistryJSON []byte

//go:embed schema_command_registry.schema.json
var embeddedSchemaCommandRegistrySchemaJSON []byte

var (
	commandRegistryProductIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)
	commandRegistryCanonicalPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*\.[A-Za-z0-9][A-Za-z0-9_.-]*$`)
	commandRegistryCLIPathToken     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]*$`)
)

type schemaCommandRegistrySnapshot struct {
	Schema   string                         `json:"$schema,omitempty"`
	Version  int                            `json:"version"`
	Products []schemaCommandRegistryProduct `json:"products"`
}

type schemaCommandRegistryProduct struct {
	ID    string                      `json:"id"`
	Tools []schemaCommandRegistryTool `json:"tools"`
}

type schemaCommandRegistryTool struct {
	CanonicalPath   string            `json:"canonical_path"`
	SourceProductID *string           `json:"source_product_id,omitempty"`
	CLIPath         string            `json:"cli_path"`
	Aliases         []string          `json:"aliases,omitempty"`
	Visibility      *SchemaVisibility `json:"visibility,omitempty"`
}

// CommandSpec is one reviewed command identity. Identity and navigation are
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

// CommandRegistry is the decoded reviewed identity registry.
type CommandRegistry struct {
	Commands    []CommandSpec
	ByCLIPath   map[string]CommandSpec
	ByCanonical map[string]CommandSpec
}

// EffectiveCommandRegistry is the reviewed registry after exact reviewed
// manual command additions have been merged. It remains independent of Cobra;
// binding is a separate fail-closed step.
type EffectiveCommandRegistry struct {
	Commands    []CommandSpec
	ByCLIPath   map[string]CommandSpec
	ByCanonical map[string]CommandSpec
}

var (
	embeddedSchemaCommandRegistryOnce sync.Once
	embeddedSchemaCommandRegistryData CommandRegistry
	embeddedSchemaCommandRegistryErr  error
	loadReviewedCommandRegistry       = loadEmbeddedCommandRegistry
	validateReviewedParameterBindings = ValidateEmbeddedSchemaParameterBindings
	loadReviewedManualSchemaHints     = embeddedManualSchemaHints
)

func loadEmbeddedCommandRegistry() (CommandRegistry, error) {
	embeddedSchemaCommandRegistryOnce.Do(func() {
		embeddedSchemaCommandRegistryData, embeddedSchemaCommandRegistryErr = decodeCommandRegistry(embeddedSchemaCommandRegistryJSON)
	})
	return cloneCommandRegistry(embeddedSchemaCommandRegistryData), embeddedSchemaCommandRegistryErr
}

// ValidateCommandRegistrySource validates a compatibility --surface input and
// requires it to be semantically identical to the embedded reviewed registry.
// Generators may retain the flag for migration, but cannot use it to introduce
// a second identity source.
func ValidateCommandRegistrySource(data []byte) (CommandRegistry, error) {
	candidate, err := decodeCommandRegistry(data)
	if err != nil {
		return CommandRegistry{}, err
	}
	embedded, err := loadReviewedCommandRegistry()
	if err != nil {
		return CommandRegistry{}, err
	}
	if !equalCommandRegistries(candidate, embedded) {
		return CommandRegistry{}, fmt.Errorf("command registry source disagrees with the embedded reviewed registry")
	}
	return candidate, nil
}

// EmbeddedCommandRegistrySourceHash returns the stable semantic hash used by
// all generated downstream views.
func EmbeddedCommandRegistrySourceHash() (string, error) {
	registry, err := loadReviewedCommandRegistry()
	if err != nil {
		return "", err
	}
	return registry.SourceHash(), nil
}

// SourceHash hashes only stable identity, navigation, and reviewed exposure.
// Formatting, product order, provenance labels, and omitted default
// source_product_id/visibility values do not affect it.
func (registry CommandRegistry) SourceHash() string {
	return hashCommandSpecs(registry.Commands)
}

// SourceHash includes reviewed manual additions because those entries are part
// of the effective identity registry delivered to downstream consumers.
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

func equalCommandRegistries(left, right CommandRegistry) bool {
	if len(left.Commands) != len(right.Commands) || left.SourceHash() != right.SourceHash() {
		return false
	}
	for canonical, leftSpec := range left.ByCanonical {
		rightSpec, ok := right.ByCanonical[canonical]
		if !ok || leftSpec.SourceProductID != rightSpec.SourceProductID || leftSpec.PrimaryCLIPath != rightSpec.PrimaryCLIPath || strings.Join(leftSpec.Aliases, "\x00") != strings.Join(rightSpec.Aliases, "\x00") || leftSpec.Visibility != rightSpec.Visibility {
			return false
		}
	}
	return true
}

func decodeCommandRegistry(data []byte) (CommandRegistry, error) {
	var snapshot schemaCommandRegistrySnapshot
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&snapshot); err != nil {
		return CommandRegistry{}, fmt.Errorf("decode reviewed Schema command registry: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("multiple JSON values")
		}
		return CommandRegistry{}, fmt.Errorf("decode reviewed Schema command registry: %w", err)
	}
	if snapshot.Version != 1 {
		return CommandRegistry{}, fmt.Errorf("unsupported Schema command registry version %d", snapshot.Version)
	}
	if strings.TrimSpace(snapshot.Schema) != commandRegistrySchemaRef {
		return CommandRegistry{}, fmt.Errorf("schema command registry must declare $schema=%q", commandRegistrySchemaRef)
	}
	if len(snapshot.Products) == 0 {
		return CommandRegistry{}, fmt.Errorf("schema command registry contains no products")
	}

	products := append([]schemaCommandRegistryProduct(nil), snapshot.Products...)
	sort.Slice(products, func(i, j int) bool { return products[i].ID < products[j].ID })
	commands := make([]CommandSpec, 0)
	seenProducts := make(map[string]bool, len(products))
	for _, product := range products {
		productID := strings.TrimSpace(product.ID)
		if productID != product.ID || !validCommandRegistryProductID(productID) {
			return CommandRegistry{}, fmt.Errorf("schema command registry contains invalid product id %q", product.ID)
		}
		if seenProducts[productID] {
			return CommandRegistry{}, fmt.Errorf("schema command registry contains duplicate product id %q", productID)
		}
		seenProducts[productID] = true
		if len(product.Tools) == 0 {
			return CommandRegistry{}, fmt.Errorf("schema command registry product %s contains no commands", productID)
		}
		for _, tool := range product.Tools {
			canonical := strings.TrimSpace(tool.CanonicalPath)
			canonicalProduct, _, ok := splitManualSchemaCanonicalPath(canonical)
			if canonical != tool.CanonicalPath || !ok || !commandRegistryCanonicalPattern.MatchString(canonical) || canonicalProduct != productID {
				return CommandRegistry{}, fmt.Errorf("schema command registry product %s contains invalid canonical path %q", productID, canonical)
			}
			sourceProductID := productID
			if tool.SourceProductID != nil {
				rawSourceProductID := *tool.SourceProductID
				sourceProductID = strings.TrimSpace(rawSourceProductID)
				if sourceProductID != rawSourceProductID || !validCommandRegistryProductID(sourceProductID) {
					return CommandRegistry{}, fmt.Errorf("schema command registry tool %s has invalid source_product_id %q", canonical, rawSourceProductID)
				}
			}
			if !validReviewedCommandRegistryCLIPath(tool.CLIPath) {
				return CommandRegistry{}, fmt.Errorf("schema command registry tool %s has invalid primary cli path %q", canonical, tool.CLIPath)
			}
			seenAliases := make(map[string]bool, len(tool.Aliases))
			for _, alias := range tool.Aliases {
				if !validReviewedCommandRegistryCLIPath(alias) {
					return CommandRegistry{}, fmt.Errorf("schema command registry tool %s has invalid alias path %q", canonical, alias)
				}
				if alias == tool.CLIPath {
					return CommandRegistry{}, fmt.Errorf("schema command registry tool %s alias %q duplicates its primary cli path", canonical, alias)
				}
				if seenAliases[alias] {
					return CommandRegistry{}, fmt.Errorf("schema command registry tool %s contains duplicate alias %q", canonical, alias)
				}
				seenAliases[alias] = true
			}
			visibility := SchemaVisibilityPublic
			if tool.Visibility != nil {
				visibility = *tool.Visibility
				switch visibility {
				case SchemaVisibilityPublic, SchemaVisibilityCompat, SchemaVisibilityInternal:
				default:
					return CommandRegistry{}, fmt.Errorf("schema command registry tool %s has invalid visibility %q", canonical, visibility)
				}
			}
			commands = append(commands, CommandSpec{
				CanonicalPath:   canonical,
				SourceProductID: sourceProductID,
				PrimaryCLIPath:  tool.CLIPath,
				Aliases:         tool.Aliases,
				Visibility:      visibility,
				Source:          "reviewed_command_registry",
			})
		}
	}
	return newCommandRegistry(commands)
}

func newCommandRegistry(commands []CommandSpec) (CommandRegistry, error) {
	normalized, byPath, byCanonical, err := indexCommandSpecs(commands)
	if err != nil {
		return CommandRegistry{}, err
	}
	return CommandRegistry{Commands: normalized, ByCLIPath: byPath, ByCanonical: byCanonical}, nil
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
			return nil, nil, nil, fmt.Errorf("schema command registry contains invalid canonical path %q", raw.CanonicalPath)
		}
		spec.SourceProductID = strings.TrimSpace(spec.SourceProductID)
		if spec.SourceProductID == "" {
			spec.SourceProductID = productID
		}
		if !validCommandRegistryProductID(spec.SourceProductID) {
			return nil, nil, nil, fmt.Errorf("schema command registry tool %s has invalid source_product_id %q", spec.CanonicalPath, raw.SourceProductID)
		}
		spec.PrimaryCLIPath = normalizeSchemaCLIPath(spec.PrimaryCLIPath)
		if !validReviewedCommandRegistryCLIPath(spec.PrimaryCLIPath) {
			return nil, nil, nil, fmt.Errorf("schema command registry tool %s has invalid primary cli path %q", spec.CanonicalPath, raw.PrimaryCLIPath)
		}
		aliases := make([]string, 0, len(spec.Aliases))
		seenAliases := make(map[string]bool, len(spec.Aliases))
		for _, rawAlias := range spec.Aliases {
			alias := normalizeSchemaCLIPath(rawAlias)
			if !validReviewedCommandRegistryCLIPath(alias) {
				return nil, nil, nil, fmt.Errorf("schema command registry tool %s has invalid alias path %q", spec.CanonicalPath, alias)
			}
			if alias == spec.PrimaryCLIPath {
				return nil, nil, nil, fmt.Errorf("schema command registry tool %s alias %q duplicates its primary path", spec.CanonicalPath, alias)
			}
			if seenAliases[alias] {
				return nil, nil, nil, fmt.Errorf("schema command registry tool %s has duplicate alias path %q", spec.CanonicalPath, alias)
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
			return nil, nil, nil, fmt.Errorf("schema command registry tool %s has invalid visibility %q", spec.CanonicalPath, spec.Visibility)
		}
		spec.Source = strings.TrimSpace(spec.Source)
		if spec.Source == "" {
			spec.Source = "reviewed_command_registry"
		}
		spec.ReviewReason = strings.TrimSpace(spec.ReviewReason)
		if previous, exists := byCanonical[spec.CanonicalPath]; exists {
			return nil, nil, nil, fmt.Errorf("duplicate Schema command registry canonical path %s (primary paths %q and %q)", spec.CanonicalPath, previous.PrimaryCLIPath, spec.PrimaryCLIPath)
		}
		byCanonical[spec.CanonicalPath] = spec
		for _, path := range append([]string{spec.PrimaryCLIPath}, spec.Aliases...) {
			if previous, exists := byPath[path]; exists {
				return nil, nil, nil, fmt.Errorf("schema command registry path %q belongs to both %s and %s", path, previous.CanonicalPath, spec.CanonicalPath)
			}
			byPath[path] = spec
		}
		normalized = append(normalized, spec)
	}
	for path, owner := range byPath {
		if canonicalOwner, exists := byCanonical[path]; exists {
			return nil, nil, nil, fmt.Errorf(
				"schema command registry CLI path %q for %s conflicts with canonical identity %s",
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

// validReviewedCommandRegistryCLIPath is intentionally stricter than
// normalizeSchemaCLIPath: reviewed source must already be canonical and may
// not rely on normalization to hide a leading dws, repeated whitespace,
// flags, or wildcard syntax.
func validReviewedCommandRegistryCLIPath(value string) bool {
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

// BuildEffectiveCommandRegistry loads the reviewed registry and merges the
// embedded manual additions. Manual entries may only name an exact existing
// public runnable leaf. They may add a new command identity, but can never
// rewrite an existing registry identity or create an alias; aliases belong in
// the reviewed base CommandRegistry.
func BuildEffectiveCommandRegistry(root *cobra.Command) (EffectiveCommandRegistry, error) {
	if root == nil {
		return EffectiveCommandRegistry{}, fmt.Errorf("build effective Schema command registry: root is nil")
	}
	if err := validateReviewedParameterBindings(); err != nil {
		return EffectiveCommandRegistry{}, fmt.Errorf("validate reviewed Schema parameter bindings: %w", err)
	}
	reviewed, err := loadReviewedCommandRegistry()
	if err != nil {
		return EffectiveCommandRegistry{}, err
	}
	manual, err := loadReviewedManualSchemaHints()
	if err != nil {
		return EffectiveCommandRegistry{}, err
	}
	return buildEffectiveCommandRegistry(root, reviewed, manual)
}

func buildEffectiveCommandRegistry(root *cobra.Command, reviewed CommandRegistry, manual ManualSchemaHintSnapshot) (EffectiveCommandRegistry, error) {
	if root == nil {
		return EffectiveCommandRegistry{}, fmt.Errorf("build effective Schema command registry: root is nil")
	}
	if manual.Version != manualSchemaHintVersion {
		return EffectiveCommandRegistry{}, fmt.Errorf("unsupported manual Schema hint version %d", manual.Version)
	}
	base, err := newCommandRegistry(reviewed.Commands)
	if err != nil {
		return EffectiveCommandRegistry{}, err
	}
	commands := append([]CommandSpec(nil), base.Commands...)
	byCanonical := base.ByCanonical
	byPath := base.ByCLIPath
	seenManualPaths := map[string]bool{}

	for _, raw := range manual.Commands {
		path := normalizeSchemaCLIPath(raw.CLIPath)
		canonical := strings.TrimSpace(raw.CanonicalPath)
		reason := strings.TrimSpace(raw.Reason)
		if path == "" || strings.ContainsAny(path, "*?[]") {
			return EffectiveCommandRegistry{}, fmt.Errorf("manual Schema hint has invalid exact cli_path %q", raw.CLIPath)
		}
		if seenManualPaths[path] {
			return EffectiveCommandRegistry{}, fmt.Errorf("duplicate manual Schema hint for %q", path)
		}
		seenManualPaths[path] = true
		if !raw.Reviewed || reason == "" {
			return EffectiveCommandRegistry{}, fmt.Errorf("manual Schema hint %q is not reviewed or has no reason", path)
		}
		productID, _, ok := splitManualSchemaCanonicalPath(canonical)
		if !ok {
			return EffectiveCommandRegistry{}, fmt.Errorf("manual Schema hint %q has invalid canonical_path %q", path, canonical)
		}
		match, resolveErr := resolveExactCobraPath(root, path)
		if resolveErr != nil {
			return EffectiveCommandRegistry{}, fmt.Errorf("manual Schema hint %q cannot be resolved exactly: %w", path, resolveErr)
		}
		command := match.Command
		if command == nil {
			return EffectiveCommandRegistry{}, fmt.Errorf("manual Schema hint %q does not resolve to an existing Cobra command", path)
		}
		if !publicRunnableSchemaLeaf(command) {
			return EffectiveCommandRegistry{}, fmt.Errorf("manual Schema hint %q must target a public runnable Cobra leaf", path)
		}
		namePath := normalizeSchemaCLIPath(strings.Join(commandPathParts(command), " "))
		if match.UsedAlias {
			nameSpec, nameExists := byPath[namePath]
			if !nameExists {
				return EffectiveCommandRegistry{}, fmt.Errorf("manual Schema hint %q uses a Cobra alias, but real command path %q is not present in reviewed CommandRegistry", path, namePath)
			}
			if nameSpec.CanonicalPath != canonical {
				return EffectiveCommandRegistry{}, fmt.Errorf("manual Schema hint %q canonical path %q conflicts with real command path %q canonical path %q", path, canonical, namePath, nameSpec.CanonicalPath)
			}
		}

		pathSpec, pathExists := byPath[path]
		canonicalSpec, canonicalExists := byCanonical[canonical]
		if pathExists && pathSpec.CanonicalPath != canonical {
			return EffectiveCommandRegistry{}, fmt.Errorf("manual Schema hint %q canonical path %q conflicts with command registry canonical path %q", path, canonical, pathSpec.CanonicalPath)
		}
		if pathExists {
			continue
		}
		if canonicalExists {
			return EffectiveCommandRegistry{}, fmt.Errorf("manual Schema hint %q cannot create an alias for %s; add the alias to the reviewed CommandRegistry", path, canonicalSpec.CanonicalPath)
		}
		commands = append(commands, CommandSpec{
			CanonicalPath:   canonical,
			SourceProductID: productID,
			PrimaryCLIPath:  path,
			Visibility:      SchemaVisibilityPublic,
			Source:          "reviewed_manual_hint",
			ReviewReason:    reason,
		})
		// Update the working indexes so later manual entries cannot collide.
		added := commands[len(commands)-1]
		byCanonical[canonical] = added
		byPath[path] = added
	}
	return newEffectiveCommandRegistry(commands)
}

func cloneCommandRegistry(registry CommandRegistry) CommandRegistry {
	clone, err := newCommandRegistry(registry.Commands)
	if err != nil {
		return CommandRegistry{}
	}
	return clone
}

func cloneCommandSpec(spec CommandSpec) CommandSpec {
	spec.Aliases = append([]string(nil), spec.Aliases...)
	return spec
}
