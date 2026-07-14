// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
)

type runtimeSchemaMetadataSources struct {
	Agent embeddedAgentMetadata
	MCP   embeddedMCPMetadata
}

// ResolvedSchemaBuild is the single source-to-delivery hand-off used by the
// Catalog generator. Effective, Bound, and Registry are three views of one
// resolution pass: reviewed identity, executable Cobra binding, and the final
// typed Agent contract. Downstream gates and serializers must consume this
// value instead of rebuilding any view from the command tree.
//
// The command root is intentionally private. It lets delivery completeness
// inspect the same executable tree without allowing callers to construct a
// seemingly resolved build by assembling the exported fields themselves.
type ResolvedSchemaBuild struct {
	effective EffectiveCommandRegistry
	bound     BoundCommandRegistry
	registry  SchemaRegistry
	root      *cobra.Command
}

// RegistryHash returns the semantic identity/navigation hash attached to this
// resolved build. It is an envelope value, not a second registry input.
func (resolved ResolvedSchemaBuild) RegistryHash() string {
	return resolved.effective.SourceHash()
}

// CommandCount reports the reviewed effective command count for generator
// diagnostics without exposing a mutable registry view.
func (resolved ResolvedSchemaBuild) CommandCount() int {
	return len(resolved.effective.Commands)
}

func embeddedRuntimeSchemaMetadataSources() runtimeSchemaMetadataSources {
	return runtimeSchemaMetadataSources{
		Agent: runtimeAgentMetadata(),
		MCP:   runtimeMCPMetadata(),
	}
}

// ResolveSchemaBuild is the only assembly path from executable Cobra commands
// and reviewed metadata into the typed Agent contract. It applies reviewed
// manual annotations once, resolves identity once, binds Cobra once, and
// assembles one SchemaRegistry. Catalog gates and serialization consume the
// returned value directly; they never re-read annotations or merge sources.
func ResolveSchemaBuild(root *cobra.Command) (ResolvedSchemaBuild, error) {
	if root == nil {
		return ResolvedSchemaBuild{}, fmt.Errorf("resolve Schema build: root is nil")
	}
	if _, err := ApplyEmbeddedManualSchemaHints(root); err != nil {
		return ResolvedSchemaBuild{}, fmt.Errorf("apply reviewed manual Schema hints: %w", err)
	}
	effective, err := BuildEffectiveCommandRegistry(root)
	if err != nil {
		return ResolvedSchemaBuild{}, fmt.Errorf("build effective Schema command registry: %w", err)
	}
	bound, err := BindEffectiveCommandRegistry(root, effective)
	if err != nil {
		return ResolvedSchemaBuild{}, fmt.Errorf("bind effective Schema command registry: %w", err)
	}
	registry, err := AssembleSchemaRegistryFromBound(bound)
	if err != nil {
		return ResolvedSchemaBuild{}, err
	}
	return ResolvedSchemaBuild{
		effective: effective,
		bound:     bound,
		registry:  registry,
		root:      root,
	}, nil
}

// AssembleSchemaRegistry is retained for non-Catalog callers that only need
// the typed registry. Catalog production must use ResolveSchemaBuild so the
// bound/effective views remain attached to the exact same resolution pass.
func AssembleSchemaRegistry(root *cobra.Command) (SchemaRegistry, error) {
	resolved, err := ResolveSchemaBuild(root)
	if err != nil {
		return SchemaRegistry{}, err
	}
	if err := ValidateSchemaParameterBindingDelivery(resolved.bound, resolved.registry); err != nil {
		return SchemaRegistry{}, fmt.Errorf("validate final Schema parameter binding delivery: %w", err)
	}
	return resolved.registry, nil
}

// AssembleSchemaRegistryFromBound resolves non-identity sources into the
// single typed ToolSpec model. Command discovery is intentionally impossible
// below this boundary: callers must first provide a fail-closed bound registry.
func AssembleSchemaRegistryFromBound(bound BoundCommandRegistry) (SchemaRegistry, error) {
	if err := ValidateEmbeddedSchemaParameterBindings(); err != nil {
		return SchemaRegistry{}, fmt.Errorf("validate reviewed Schema parameter bindings: %w", err)
	}
	return assembleSchemaRegistryFromBound(bound, embeddedRuntimeSchemaMetadataSources())
}

func assembleSchemaRegistryFromBound(bound BoundCommandRegistry, metadata runtimeSchemaMetadataSources) (SchemaRegistry, error) {
	entries, err := collectRuntimeSchemaEntriesFromBound(bound)
	if err != nil {
		return SchemaRegistry{}, err
	}
	byProduct := make(map[string]*ProductSpec)
	for _, entry := range entries {
		tool, err := runtimeToolSpecFromMetadata(entry, metadata)
		if err != nil {
			return SchemaRegistry{}, err
		}
		product := byProduct[entry.ProductID]
		if product == nil {
			selection, provenance, _ := agentProductContractForIDsFromMetadata(metadata.Agent, entry.ProductID, entry.SourceProductID)
			product = &ProductSpec{
				ID:              entry.ProductID,
				Name:            entry.ProductName,
				Description:     entry.ProductName,
				Runtime:         true,
				Selection:       selection,
				FieldProvenance: provenance,
			}
			byProduct[entry.ProductID] = product
		}
		product.Tools = append(product.Tools, tool)
	}

	productIDs := make([]string, 0, len(byProduct))
	for productID := range byProduct {
		productIDs = append(productIDs, productID)
	}
	sort.Strings(productIDs)
	products := make([]ProductSpec, 0, len(productIDs))
	for _, productID := range productIDs {
		products = append(products, *byProduct[productID])
	}
	registry, err := SchemaRegistryFromRuntime("runtime-command", products)
	if err != nil {
		return SchemaRegistry{}, fmt.Errorf("build typed Schema registry: %w", err)
	}
	registry.InterfaceMetadata, err = marshalSchemaRaw(interfaceMetadataSummaryFrom(metadata.MCP))
	if err != nil {
		return SchemaRegistry{}, fmt.Errorf("encode interface metadata summary: %w", err)
	}
	registry.AgentMetadata, err = marshalSchemaRaw(agentMetadataSummaryFrom(metadata.Agent))
	if err != nil {
		return SchemaRegistry{}, fmt.Errorf("encode Agent metadata summary: %w", err)
	}
	return registry, nil
}

func runtimeToolSpecFromMetadata(entry runtimeSchemaEntry, metadata runtimeSchemaMetadataSources) (ToolSpec, error) {
	canonicalPath := entry.ProductID + "." + entry.ToolName
	dryRun, err := reviewedDryRunCapability(canonicalPath)
	if err != nil {
		return ToolSpec{}, fmt.Errorf("resolve reviewed dry-run capability for %s: %w", canonicalPath, err)
	}
	hint := runtimeSchemaHintForEntry(entry)
	embeddedMeta, hasEmbeddedMeta := embeddedMCPMetadataForEntryFrom(entry, metadata.Agent, metadata.MCP)
	title, description, metadataSource, textProvenance, err := runtimeToolTextMetadataFromMetadata(entry, metadata)
	if err != nil {
		return ToolSpec{}, fmt.Errorf("resolve Schema text metadata for %s: %w", canonicalPath, err)
	}
	constraints := runtimeCommandConstraints(entry.Command)
	parameters, err := runtimeCommandParameterSpecs(entry.Command, canonicalPath, hint.Parameters, embeddedMeta.Parameters, constraints)
	if err != nil {
		return ToolSpec{}, fmt.Errorf("resolve Schema parameters for %s: %w", canonicalPath, err)
	}

	paths := []string{entry.PrimaryCLIPath, entry.CLIPath, canonicalPath}
	paths = append(paths, entry.Aliases...)
	safety, interfaceSpec, selection, provenance, _ := agentToolContractForPathsFromMetadata(metadata.Agent, paths...)
	if metadataSource == "" && hasEmbeddedMeta {
		metadataSource = "embedded-mcp-metadata"
		textProvenance["metadata_source"] = runtimeSchemaFieldProvenance(
			runtimeSchemaStringCandidate(metadataSource, "metadata_source_resolution"),
		)
	}
	if provenance == nil {
		provenance = map[string]FieldProvenance{}
	}
	for field, fieldProvenance := range textProvenance {
		provenance[field] = fieldProvenance
	}
	provenance["canonical_path"] = entry.IdentityField
	if dryRun != nil {
		provenance["dry_run"] = resolvedFieldProvenance(
			*dryRun,
			"reviewed_dry_run_registry",
			"internal/cli/schema_dry_run_capabilities.go",
			"reviewed_explicit",
			"exact_canonical_lookup",
			"reviewed positive dry-run capability",
		)
	}

	sourceProductID := strings.TrimSpace(entry.SourceProductID)
	if sourceProductID == entry.ProductID {
		sourceProductID = ""
	}
	return ToolSpecFromRuntime(RuntimeToolSpecInput{
		Identity: ToolIdentitySpec{
			ProductID:       entry.ProductID,
			SourceProductID: sourceProductID,
			Name:            entry.ToolName,
			CLIName:         entry.CLIName,
			CanonicalPath:   canonicalPath,
			Path:            canonicalPath,
			CLIPath:         entry.CLIPath,
			PrimaryCLIPath:  entry.PrimaryCLIPath,
			Group:           entry.Group,
			Aliases:         append([]string(nil), entry.Aliases...),
			IsAlias:         false,
			Source:          entry.Source,
		},
		Display:         entry.ProductName,
		Title:           title,
		Description:     description,
		MetadataSource:  metadataSource,
		Parameters:      parameters,
		Constraints:     constraints,
		Positionals:     runtimeCommandPositionals(entry.Command),
		DryRun:          dryRun,
		Safety:          safety,
		Interface:       interfaceSpec,
		Selection:       selection,
		FieldProvenance: provenance,
	})
}

func marshalSchemaRaw(value any) (json.RawMessage, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(data), nil
}

func runtimeSchemaPayloadFromRegistry(registry SchemaRegistry, args []string) (map[string]any, error) {
	index, err := registry.Index()
	if err != nil {
		return nil, err
	}
	registry = index.Registry()
	if len(args) == 0 {
		snapshot, err := registry.ToSnapshotPayload()
		if err != nil {
			return nil, err
		}
		return snapshot.Catalog, nil
	}

	raw := strings.TrimSpace(args[0])
	if tool, ok := index.Resolve(raw); ok {
		tool = schemaToolForResolvedPath(tool, raw)
		return tool.ToPayload()
	}
	tokens := splitSchemaPathTokens(raw)
	if len(tokens) == 1 {
		if product, ok := index.Product(tokens[0]); ok {
			payload, err := product.ToSummaryPayload()
			if err != nil {
				return nil, err
			}
			return map[string]any{
				"kind":    "schema",
				"level":   "product",
				"count":   len(product.Tools),
				"product": payload,
				"source":  registry.Source,
			}, nil
		}
	}
	if len(tokens) > 1 {
		path := strings.Join(tokens, " ")
		if product, ok := index.Product(tokens[0]); ok {
			tools := make([]map[string]any, 0)
			for _, tool := range product.Tools {
				if !schemaToolUnderGroup(tool, path) {
					continue
				}
				summary, summaryErr := tool.ToSummaryPayload()
				if summaryErr != nil {
					return nil, summaryErr
				}
				tools = append(tools, summary)
			}
			if len(tools) > 0 {
				return map[string]any{
					"kind":   "schema",
					"level":  "group",
					"path":   path,
					"count":  len(tools),
					"tools":  tools,
					"source": registry.Source,
				}, nil
			}
		}
	}
	return nil, apperrors.NewValidation("unknown runtime schema path " + strconvQuote(raw))
}

func runtimeSchemaAllPayloadFromRegistry(registry SchemaRegistry) (map[string]any, error) {
	return registry.ToPayload()
}

func schemaToolForResolvedPath(tool ToolSpec, raw string) ToolSpec {
	normalized := normalizeSchemaCLIPath(raw)
	if normalized == "" || normalized == tool.Identity.CLIPath || normalized == tool.Identity.PrimaryCLIPath {
		return tool
	}
	for _, alias := range tool.Identity.Aliases {
		if normalizeSchemaCLIPath(alias) == normalized {
			tool.Identity.CLIPath = normalizeSchemaCLIPath(alias)
			tool.Identity.IsAlias = true
			return tool
		}
	}
	return tool
}

func schemaToolUnderGroup(tool ToolSpec, group string) bool {
	prefix := normalizeSchemaCLIPath(group) + " "
	paths := append([]string{tool.Identity.CLIPath, tool.Identity.PrimaryCLIPath}, tool.Identity.Aliases...)
	for _, path := range paths {
		if strings.HasPrefix(normalizeSchemaCLIPath(path), prefix) {
			return true
		}
	}
	return false
}

// validateSchemaRegistryAgainstCommandRegistry compares identities as exact
// sets, not counts. A stale/missing primary path or alias must fail generation
// even when another executable path happens to preserve the canonical count.
func validateSchemaRegistryAgainstCommandRegistry(registry SchemaRegistry, commandRegistry EffectiveCommandRegistry) error {
	index, err := registry.Index()
	if err != nil {
		return err
	}
	publicCommands := make(map[string]CommandSpec)
	for canonical, command := range commandRegistry.ByCanonical {
		if command.Visibility == SchemaVisibilityPublic {
			publicCommands[canonical] = command
		}
	}
	if got, want := len(index.CanonicalPaths()), len(publicCommands); got != want {
		return fmt.Errorf("typed Schema registry contains %d canonical tools, reviewed CommandRegistry contains %d", got, want)
	}
	for canonical, expected := range publicCommands {
		tool, ok := index.Resolve(canonical)
		if !ok {
			return fmt.Errorf("reviewed CommandRegistry canonical %s is missing from typed Schema registry", canonical)
		}
		if err := validateCanonicalToolIdentity(tool); err != nil {
			return err
		}
		if actual := normalizeSchemaCLIPath(tool.Identity.PrimaryCLIPath); actual != expected.PrimaryCLIPath {
			return fmt.Errorf("schema tool %s primary path %q disagrees with reviewed CommandRegistry %q", canonical, actual, expected.PrimaryCLIPath)
		}
		if actual := normalizeSchemaCLIPath(tool.Identity.CLIPath); actual != expected.PrimaryCLIPath {
			return fmt.Errorf("schema tool %s canonical cli path %q disagrees with reviewed CommandRegistry primary path %q", canonical, actual, expected.PrimaryCLIPath)
		}
		if tool.Identity.IsAlias {
			return fmt.Errorf("schema tool %s canonical identity must have is_alias=false", canonical)
		}
		actualSourceProduct := strings.TrimSpace(tool.Identity.SourceProductID)
		if actualSourceProduct == "" {
			actualSourceProduct = tool.Identity.ProductID
		}
		expectedSourceProduct := strings.TrimSpace(expected.SourceProductID)
		if expectedSourceProduct == "" {
			expectedSourceProduct = tool.Identity.ProductID
		}
		if actualSourceProduct != expectedSourceProduct {
			return fmt.Errorf("schema tool %s source product %q disagrees with reviewed CommandRegistry %q", canonical, actualSourceProduct, expectedSourceProduct)
		}
		if actualSource := strings.TrimSpace(tool.Identity.Source); actualSource != strings.TrimSpace(expected.Source) {
			return fmt.Errorf("schema tool %s identity source %q disagrees with effective CommandRegistry %q", canonical, actualSource, expected.Source)
		}
		actualAliases := sortedUniqueStrings(tool.Identity.Aliases)
		expectedAliases := sortedUniqueStrings(expected.Aliases)
		if strings.Join(actualAliases, "\x00") != strings.Join(expectedAliases, "\x00") {
			return fmt.Errorf("schema tool %s aliases %q disagree with reviewed CommandRegistry %q", canonical, actualAliases, expectedAliases)
		}
	}
	return nil
}

// validateSchemaRegistryAgentMetadata compares exact canonical sets after
// resolving generated metadata keys through the same SchemaIndex. Counts alone
// cannot detect one missing tool being masked by one duplicate alias.
func validateSchemaRegistryAgentMetadata(registry SchemaRegistry) error {
	index, err := registry.Index()
	if err != nil {
		return err
	}
	metadata := runtimeAgentMetadata()
	resolved := make(map[string]string, len(metadata.Tools))
	var problems []string
	keys := make([]string, 0, len(metadata.Tools))
	for key := range metadata.Tools {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		tool, ok := index.Resolve(key)
		if !ok {
			problems = append(problems, fmt.Sprintf("Agent metadata key %q does not resolve in final Schema registry", key))
			continue
		}
		canonical := tool.Identity.CanonicalPath
		if previous := resolved[canonical]; previous != "" && previous != key {
			problems = append(problems, fmt.Sprintf("Agent metadata keys %q and %q both resolve to %s", previous, key, canonical))
			continue
		}
		resolved[canonical] = key
	}
	for _, canonical := range index.CanonicalPaths() {
		if resolved[canonical] == "" {
			problems = append(problems, fmt.Sprintf("final Schema tool %s has no generated Agent metadata", canonical))
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return nil
}

// validateFinalSchemaProvenanceCoverage ensures every field resolved from
// competing contract sources carries a mechanically checkable winner. The
// per-field equality invariant itself is enforced by ToolSpec.Validate.
func validateFinalSchemaProvenanceCoverage(registry SchemaRegistry) error {
	var problems []string
	require := func(owner, field string, provenance map[string]FieldProvenance) {
		if _, ok := provenance[field]; !ok {
			problems = append(problems, fmt.Sprintf("Schema %s has no provenance for %s", owner, field))
		}
	}
	for _, product := range registry.Products {
		if strings.TrimSpace(product.Selection.AgentSummary) != "" {
			require("product "+product.ID, "agent_summary", product.FieldProvenance)
		}
		if product.Selection.UseWhen != nil {
			require("product "+product.ID, "use_when", product.FieldProvenance)
		}
		if product.Selection.AvoidWhen != nil {
			require("product "+product.ID, "avoid_when", product.FieldProvenance)
		}
		for _, tool := range product.Tools {
			canonical := tool.Identity.CanonicalPath
			if err := tool.Validate(); err != nil {
				problems = append(problems, err.Error())
				continue
			}
			for _, field := range requiredToolProvenanceFields {
				require("tool "+canonical, field, tool.FieldProvenance)
			}
			if tool.DryRun != nil {
				require("tool "+canonical, "dry_run", tool.FieldProvenance)
			}
			// interface_reason is part of the final interface contract only when
			// the disposition requires or actually delivers a reason. An MCP or
			// local available command with no reason has no resolver winner to
			// invent; unavailable/composite commands fail closed without one.
			if strings.TrimSpace(tool.Interface.Reason) != "" ||
				strings.TrimSpace(tool.Interface.Mode) == InterfaceModeComposite ||
				strings.TrimSpace(tool.Interface.Availability) == InterfaceUnavailable {
				require("tool "+canonical, "interface_reason", tool.FieldProvenance)
			}
			selectionValues := map[string][]string{
				"use_when":      tool.Selection.UseWhen,
				"avoid_when":    tool.Selection.AvoidWhen,
				"prerequisites": tool.Selection.Prerequisites,
				"tips":          tool.Selection.Tips,
				"workflow_refs": tool.Selection.WorkflowRefs,
				"examples":      tool.Selection.Examples,
			}
			for _, field := range conditionalSelectionProvenanceFields {
				// A non-nil empty slice is an authored [] winner, not absence.
				if selectionValues[field] != nil {
					require("tool "+canonical, field, tool.FieldProvenance)
				}
			}
			if tool.Selection.Reviewed != nil {
				require("tool "+canonical, "reviewed", tool.FieldProvenance)
			}
			for field, value := range map[string]string{
				"title":           tool.Title,
				"description":     tool.Description,
				"metadata_source": tool.MetadataSource,
			} {
				if strings.TrimSpace(value) != "" {
					require("tool "+canonical, field, tool.FieldProvenance)
				}
			}
			for _, parameter := range tool.Parameters {
				owner := "tool " + canonical + " parameter " + parameter.Name
				for _, field := range requiredParameterProvenanceFields {
					require(owner, field, parameter.FieldProvenance)
				}
				if parameter.CLIRequired {
					require(owner, "cli_required", parameter.FieldProvenance)
				}
				if parameter.InterfaceType != "" {
					require(owner, "interface_type", parameter.FieldProvenance)
				}
				if parameter.Format != "" {
					require(owner, "format", parameter.FieldProvenance)
				}
				if len(parameter.Example) > 0 {
					require(owner, "example", parameter.FieldProvenance)
				}
				if len(parameter.Enum) > 0 {
					require(owner, "enum", parameter.FieldProvenance)
				}
			}
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return nil
}
