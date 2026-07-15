// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// RuntimeSchemaExclusion records a reviewed reason why a public executable
// command is intentionally not advertised as an Agent tool.
type RuntimeSchemaExclusion struct {
	CLIPath  string `json:"cli_path"`
	Reason   string `json:"reason"`
	Reviewed bool   `json:"reviewed"`
}

type runtimeSchemaExclusionSnapshot struct {
	Version int                           `json:"version"`
	Groups  []runtimeSchemaExclusionGroup `json:"groups"`
}

type runtimeSchemaExclusionGroup struct {
	ID       string   `json:"id"`
	Reason   string   `json:"reason"`
	Reviewed bool     `json:"reviewed"`
	Commands []string `json:"commands"`
}

var (
	completenessLoadExclusions = EmbeddedRuntimeSchemaExclusions
	completenessApplyManual    = ApplyEmbeddedManualSchemaHints
	completenessBuildEffective = BuildEffectiveCommandRegistry
	completenessBindEffective  = BindEffectiveCommandRegistry
	completenessRuntimeReport  = runtimeSchemaCompletenessFromBound
	completenessDeliveryReport = schemaCatalogDeliveryCompletenessAgainstLoadedAndBound
	completenessCollectEntries = collectRuntimeSchemaEntries
)

//go:embed schema_command_exclusions.json
var embeddedRuntimeSchemaExclusionsJSON []byte

// RuntimeSchemaCompletenessReport compares the public executable Cobra leaves
// with a reviewed Schema command set, such as runtime annotations or the final
// generated Catalog.
type RuntimeSchemaCompletenessReport struct {
	Covered           []string
	Excluded          []string
	Missing           []string
	InvalidExclusions []string
	StaleExclusions   []string
	DeliveryErrors    []string
}

// EmbeddedRuntimeSchemaExclusions returns the exact, reviewed list of public
// CLI leaves intentionally kept outside the stable Agent command contract.
func EmbeddedRuntimeSchemaExclusions() ([]RuntimeSchemaExclusion, error) {
	var snapshot runtimeSchemaExclusionSnapshot
	if err := json.Unmarshal(embeddedRuntimeSchemaExclusionsJSON, &snapshot); err != nil {
		return nil, fmt.Errorf("decode runtime schema exclusions: %w", err)
	}
	if snapshot.Version != 1 {
		return nil, fmt.Errorf("unsupported runtime schema exclusion version %d", snapshot.Version)
	}
	var exclusions []RuntimeSchemaExclusion
	seen := map[string]bool{}
	for _, group := range snapshot.Groups {
		if strings.TrimSpace(group.ID) == "" || strings.TrimSpace(group.Reason) == "" || !group.Reviewed {
			return nil, fmt.Errorf("runtime schema exclusion group %q is not reviewed or has no reason", group.ID)
		}
		for _, rawPath := range group.Commands {
			path := normalizeSchemaCLIPath(rawPath)
			if path == "" {
				return nil, fmt.Errorf("runtime schema exclusion group %q contains an empty command", group.ID)
			}
			if seen[path] {
				return nil, fmt.Errorf("duplicate runtime schema exclusion %q", path)
			}
			seen[path] = true
			exclusions = append(exclusions, RuntimeSchemaExclusion{CLIPath: path, Reason: group.Reason, Reviewed: true})
		}
	}
	return exclusions, nil
}

// ValidateEmbeddedRuntimeSchemaCompleteness enforces the reviewed reverse
// command-tree contract used by generation and CI.
func ValidateEmbeddedRuntimeSchemaCompleteness(root *cobra.Command) error {
	if _, err := completenessApplyManual(root); err != nil {
		return err
	}
	effective, err := completenessBuildEffective(root)
	if err != nil {
		return err
	}
	bound, err := completenessBindEffective(root, effective)
	if err != nil {
		return err
	}
	return validateResolvedRuntimeSchemaCompleteness(root, bound)
}

// validateResolvedRuntimeSchemaCompleteness checks the executable tree against
// the exact BoundCommandRegistry produced by the active resolution pass. It
// must not discover or bind commands again: Catalog generation uses this gate
// to ensure every pre-delivery check observes the same identity candidate.
func validateResolvedRuntimeSchemaCompleteness(root *cobra.Command, bound BoundCommandRegistry) error {
	exclusions, err := completenessLoadExclusions()
	if err != nil {
		return err
	}
	report := completenessRuntimeReport(root, exclusions, bound)
	if len(report.DeliveryErrors) > 0 {
		return fmt.Errorf("invalid effective Schema command registry: %s", strings.Join(report.DeliveryErrors, "; "))
	}
	if len(report.Missing) > 0 {
		return fmt.Errorf("public Cobra leaves missing from Schema or reviewed exclusions: %s", strings.Join(report.Missing, ", "))
	}
	if len(report.InvalidExclusions) > 0 {
		return fmt.Errorf("invalid runtime schema exclusions: %s", strings.Join(report.InvalidExclusions, ", "))
	}
	if len(report.StaleExclusions) > 0 {
		return fmt.Errorf("stale runtime schema exclusions: %s", strings.Join(report.StaleExclusions, ", "))
	}
	return nil
}

// validateResolvedSchemaCatalogDeliveryCompleteness compares the serialized
// delivery with the exact bound registry that produced its SchemaRegistry.
// This closes the final path where completeness used to rebuild identity from
// Cobra after the source registry had already passed generation gates.
func validateResolvedSchemaCatalogDeliveryCompleteness(root *cobra.Command, bound BoundCommandRegistry, snapshot SchemaCatalogSnapshot) error {
	exclusions, err := completenessLoadExclusions()
	if err != nil {
		return err
	}
	return validateSchemaCatalogDeliveryCompletenessFromBound(root, bound, snapshot, exclusions)
}

func validateSchemaCatalogDeliveryCompletenessFromBound(root *cobra.Command, bound BoundCommandRegistry, snapshot SchemaCatalogSnapshot, exclusions []RuntimeSchemaExclusion) error {
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("encode final Schema Catalog for delivery validation: %w", err)
	}
	loaded, err := decodeSchemaCatalogSnapshot(encoded)
	if err != nil {
		return fmt.Errorf("load final Schema Catalog through production loader: %w", err)
	}
	report := completenessDeliveryReport(root, loaded, exclusions, bound)
	if len(report.DeliveryErrors) > 0 {
		return fmt.Errorf("invalid final Schema Catalog delivery: %s", strings.Join(report.DeliveryErrors, "; "))
	}
	if len(report.Missing) > 0 {
		return fmt.Errorf("public Cobra leaves missing from final Schema Catalog or reviewed exclusions: %s", strings.Join(report.Missing, ", "))
	}
	if len(report.InvalidExclusions) > 0 {
		return fmt.Errorf("invalid runtime schema exclusions: %s", strings.Join(report.InvalidExclusions, ", "))
	}
	if len(report.StaleExclusions) > 0 {
		return fmt.Errorf("stale runtime schema exclusions: %s", strings.Join(report.StaleExclusions, ", "))
	}
	return nil
}

// RuntimeSchemaCompleteness scans the real command tree in the reverse
// direction: every public executable leaf must either belong to the effective
// reviewed CommandRegistry or have a reviewed exclusion with a non-empty
// reason.
func RuntimeSchemaCompleteness(root *cobra.Command, exclusions []RuntimeSchemaExclusion) RuntimeSchemaCompletenessReport {
	entries, err := completenessCollectEntries(root)
	if err != nil {
		return RuntimeSchemaCompletenessReport{DeliveryErrors: []string{err.Error()}}
	}
	coveredPaths := map[string]bool{}
	for _, entry := range entries {
		addSchemaCoveredPath(coveredPaths, entry.CLIPath)
		addSchemaCoveredPath(coveredPaths, entry.PrimaryCLIPath)
		for _, alias := range entry.Aliases {
			addSchemaCoveredPath(coveredPaths, alias)
		}
	}
	return runtimeSchemaCompletenessAgainstPaths(root, exclusions, coveredPaths)
}

func runtimeSchemaCompletenessFromBound(root *cobra.Command, exclusions []RuntimeSchemaExclusion, bound BoundCommandRegistry) RuntimeSchemaCompletenessReport {
	coveredPaths := map[string]bool{}
	for _, command := range bound.Commands {
		if command.Visibility != SchemaVisibilityPublic {
			continue
		}
		addSchemaCoveredPath(coveredPaths, command.PrimaryCLIPath)
		for _, alias := range command.Aliases {
			addSchemaCoveredPath(coveredPaths, alias)
		}
	}
	return runtimeSchemaCompletenessAgainstPaths(root, exclusions, coveredPaths)
}

func schemaCatalogDeliveryCompletenessAgainstLoadedAndBound(root *cobra.Command, loaded loadedSchemaCatalog, exclusions []RuntimeSchemaExclusion, bound BoundCommandRegistry) RuntimeSchemaCompletenessReport {
	expectedByPath, mappingErrors := runtimeSchemaIdentityByBound(bound)
	return schemaCatalogDeliveryCompletenessAgainstLoadedAndIdentity(root, loaded, exclusions, expectedByPath, mappingErrors)
}

func schemaCatalogDeliveryCompletenessAgainstLoadedAndIdentity(
	root *cobra.Command,
	loaded loadedSchemaCatalog,
	exclusions []RuntimeSchemaExclusion,
	expectedByPath map[string]runtimeSchemaResolvedIdentity,
	mappingErrors []string,
) RuntimeSchemaCompletenessReport {
	coveredPaths := map[string]bool{}
	deliveryErrors := append([]string(nil), mappingErrors...)
	deliveryErrors = append(deliveryErrors, schemaDeliveryInvariantErrors(loaded)...)

	// Public completeness is intentionally limited to visible runnable leaves.
	// A hidden compatibility leaf may still be a valid primary path or alias and
	// is checked separately against all resolved runtime Schema entries below.
	walkPublicRunnableLeaves(root, func(leaf *cobra.Command) {
		path := normalizeSchemaCLIPath(strings.Join(commandPathParts(leaf), " "))
		expected := expectedByPath[path]
		if expected.CanonicalPath == "" {
			return
		}
		payload, err := deliverySchemaPayload(loaded, []string{path})
		if err != nil {
			return
		}
		if got := strings.TrimSpace(schemaString(payload["canonical_path"])); got == expected.CanonicalPath {
			coveredPaths[path] = true
		} else {
			deliveryErrors = append(deliveryErrors, fmt.Sprintf("public CLI path %q resolves to %q, want %q", path, got, expected.CanonicalPath))
		}
	})

	canonicals := loaded.Index.CanonicalPaths()
	for _, canonical := range canonicals {
		tool, ok := deliveryIndexResolve(loaded.Index, canonical)
		if !ok {
			deliveryErrors = append(deliveryErrors, fmt.Sprintf("typed Schema index lost canonical %q", canonical))
			continue
		}
		primaryPath := normalizeSchemaCLIPath(tool.Identity.PrimaryCLIPath)
		if expected := expectedByPath[primaryPath]; expected.CanonicalPath == canonical && expected.Source != "" {
			if gotSource := strings.TrimSpace(tool.Identity.Source); gotSource != expected.Source {
				deliveryErrors = append(deliveryErrors, fmt.Sprintf("tool %s identity source is %q, want resolved source %q", canonical, gotSource, expected.Source))
			}
		}
		if payload, err := deliverySchemaPayload(loaded, []string{canonical}); err != nil {
			deliveryErrors = append(deliveryErrors, fmt.Sprintf("canonical %q is not queryable: %v", canonical, err))
		} else if got := strings.TrimSpace(schemaString(payload["canonical_path"])); got != canonical {
			deliveryErrors = append(deliveryErrors, fmt.Sprintf("canonical %q resolves to %q", canonical, got))
		} else if expectedPayload, renderErr := deliveryToolPayload(tool); renderErr != nil {
			deliveryErrors = append(deliveryErrors, fmt.Sprintf("canonical %q cannot render typed payload: %v", canonical, renderErr))
		} else if !schemaJSONEqual(payload, expectedPayload) {
			deliveryErrors = append(deliveryErrors, fmt.Sprintf("canonical %q query differs from final ToolSpec", canonical))
		}

		paths := []string{tool.Identity.CLIPath, tool.Identity.PrimaryCLIPath}
		paths = append(paths, tool.Identity.Aliases...)
		seenPaths := map[string]bool{}
		for _, rawPath := range paths {
			path := normalizeSchemaCLIPath(rawPath)
			if path == "" || seenPaths[path] {
				continue
			}
			seenPaths[path] = true
			expected := expectedByPath[path]
			if expected.CanonicalPath == "" {
				deliveryErrors = append(deliveryErrors, fmt.Sprintf("tool %s advertises non-executable Schema path %q", canonical, path))
				continue
			}
			if expected.CanonicalPath != canonical {
				deliveryErrors = append(deliveryErrors, fmt.Sprintf("tool %s advertises path %q owned by %s", canonical, path, expected.CanonicalPath))
				continue
			}
			payload, err := deliverySchemaPayload(loaded, []string{path})
			if err != nil {
				deliveryErrors = append(deliveryErrors, fmt.Sprintf("tool %s path %q is not queryable: %v", canonical, path, err))
				continue
			}
			if got := strings.TrimSpace(schemaString(payload["canonical_path"])); got != canonical {
				deliveryErrors = append(deliveryErrors, fmt.Sprintf("tool %s path %q resolves to %q", canonical, path, got))
				continue
			}
			expectedTool := schemaToolForResolvedPath(tool, path)
			expectedPayload, renderErr := deliveryToolPayload(expectedTool)
			if renderErr != nil {
				deliveryErrors = append(deliveryErrors, fmt.Sprintf("tool %s path %q cannot render typed payload: %v", canonical, path, renderErr))
			} else if !schemaJSONEqual(payload, expectedPayload) {
				deliveryErrors = append(deliveryErrors, fmt.Sprintf("tool %s path %q query differs from final ToolSpec", canonical, path))
			}
		}
	}

	report := runtimeSchemaCompletenessAgainstPaths(root, exclusions, coveredPaths)
	report.DeliveryErrors = sortedUniqueSchemaStrings(deliveryErrors)
	return report
}

// schemaRegistryProjectionErrors proves every public view is a pure
// projection of the final typed registry: product/group summaries and --all
// must never re-resolve annotations or metadata independently from leaf
// lookup.
func schemaRegistryProjectionErrors(loaded loadedSchemaCatalog) []string {
	var problems []string
	all, err := deliveryRegistryPayload(loaded.Registry)
	if err != nil {
		return []string{fmt.Sprintf("render final Schema --all projection: %v", err)}
	}
	allByCanonical := map[string]map[string]any{}
	for _, product := range schemaMapSlice(all["products"]) {
		for _, tool := range schemaMapSlice(product["tools"]) {
			canonical := strings.TrimSpace(schemaString(tool["canonical_path"]))
			if canonical == "" {
				problems = append(problems, "Schema --all contains a tool without canonical_path")
				continue
			}
			if _, exists := allByCanonical[canonical]; exists {
				problems = append(problems, fmt.Sprintf("Schema --all contains duplicate tool %s", canonical))
			}
			allByCanonical[canonical] = tool
		}
	}

	groupPaths := map[string]bool{}
	for _, product := range loaded.Registry.Products {
		expectedProduct, renderErr := renderRegistryProductSummary(product)
		if renderErr != nil {
			problems = append(problems, fmt.Sprintf("render product %s summary: %v", product.ID, renderErr))
		} else if actual, queryErr := deliverySchemaPayload(loaded, []string{product.ID}); queryErr != nil {
			problems = append(problems, fmt.Sprintf("product %s is not queryable: %v", product.ID, queryErr))
		} else if !schemaJSONEqual(actual["product"], expectedProduct) {
			problems = append(problems, fmt.Sprintf("product %s query differs from final ProductSpec", product.ID))
		}
		for _, tool := range product.Tools {
			canonical := tool.Identity.CanonicalPath
			expectedTool, renderErr := deliveryToolPayload(tool)
			if renderErr != nil {
				problems = append(problems, fmt.Sprintf("render final ToolSpec %s: %v", canonical, renderErr))
			} else if actual, exists := allByCanonical[canonical]; !exists {
				problems = append(problems, fmt.Sprintf("Schema --all is missing final ToolSpec %s", canonical))
			} else if !schemaJSONEqual(actual, expectedTool) {
				problems = append(problems, fmt.Sprintf("Schema --all tool %s differs from final ToolSpec", canonical))
			}
			paths := append([]string{tool.Identity.CLIPath, tool.Identity.PrimaryCLIPath}, tool.Identity.Aliases...)
			for _, rawPath := range paths {
				tokens := splitSchemaPathTokens(rawPath)
				for length := 2; length < len(tokens); length++ {
					groupPaths[strings.Join(tokens[:length], " ")] = true
				}
			}
		}
	}
	if len(allByCanonical) != len(loaded.Index.CanonicalPaths()) {
		problems = append(problems, fmt.Sprintf("Schema --all contains %d tools, typed index contains %d", len(allByCanonical), len(loaded.Index.CanonicalPaths())))
	}

	groups := make([]string, 0, len(groupPaths))
	for path := range groupPaths {
		groups = append(groups, path)
	}
	sort.Strings(groups)
	for _, path := range groups {
		tokens := splitSchemaPathTokens(path)
		product, ok := loaded.Index.Product(tokens[0])
		if !ok {
			problems = append(problems, fmt.Sprintf("group %s has no typed product", path))
			continue
		}
		expected := make([]map[string]any, 0)
		for _, tool := range product.Tools {
			if !schemaToolUnderGroup(tool, path) {
				continue
			}
			summary, renderErr := deliveryToolSummary(tool)
			if renderErr != nil {
				problems = append(problems, fmt.Sprintf("render group %s tool %s: %v", path, tool.Identity.CanonicalPath, renderErr))
				continue
			}
			expected = append(expected, summary)
		}
		actual, queryErr := deliverySchemaPayload(loaded, []string{path})
		if queryErr != nil {
			problems = append(problems, fmt.Sprintf("group %s is not queryable: %v", path, queryErr))
		} else if !schemaJSONEqual(actual["tools"], expected) {
			problems = append(problems, fmt.Sprintf("group %s query differs from final ToolSpec summaries", path))
		}
	}
	return sortedUniqueSchemaStrings(problems)
}

type runtimeSchemaResolvedIdentity struct {
	CanonicalPath string
	Source        string
}

func runtimeSchemaIdentityByBound(bound BoundCommandRegistry) (map[string]runtimeSchemaResolvedIdentity, []string) {
	identityByPath := map[string]runtimeSchemaResolvedIdentity{}
	var conflicts []string
	for _, command := range bound.Commands {
		if command.Visibility != SchemaVisibilityPublic {
			continue
		}
		canonical := strings.TrimSpace(command.CanonicalPath)
		paths := append([]string{command.PrimaryCLIPath}, command.Aliases...)
		for _, rawPath := range paths {
			path := normalizeSchemaCLIPath(rawPath)
			if path == "" || canonical == "" {
				continue
			}
			if existing := identityByPath[path]; existing.CanonicalPath != "" && existing.CanonicalPath != canonical {
				conflicts = append(conflicts, fmt.Sprintf("bound Schema path %q belongs to both %s and %s", path, existing.CanonicalPath, canonical))
				continue
			}
			identityByPath[path] = runtimeSchemaResolvedIdentity{
				CanonicalPath: canonical,
				Source:        strings.TrimSpace(command.Source),
			}
		}
	}
	return identityByPath, sortedUniqueSchemaStrings(conflicts)
}

func sortedUniqueSchemaStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func addSchemaCoveredPath(coveredPaths map[string]bool, rawPath string) {
	if path := normalizeSchemaCLIPath(rawPath); path != "" {
		coveredPaths[path] = true
	}
}

func runtimeSchemaCompletenessAgainstPaths(root *cobra.Command, exclusions []RuntimeSchemaExclusion, coveredPaths map[string]bool) RuntimeSchemaCompletenessReport {
	report := RuntimeSchemaCompletenessReport{}
	exclusionByPath := make(map[string]RuntimeSchemaExclusion, len(exclusions))
	for _, exclusion := range exclusions {
		path := normalizeSchemaCLIPath(exclusion.CLIPath)
		if path == "" || strings.TrimSpace(exclusion.Reason) == "" || !exclusion.Reviewed {
			report.InvalidExclusions = append(report.InvalidExclusions, firstNonEmptySchemaString(path, strings.TrimSpace(exclusion.CLIPath), "<empty>"))
			continue
		}
		exclusion.CLIPath = path
		exclusionByPath[path] = exclusion
	}

	seenPublic := map[string]bool{}
	usedExclusions := map[string]bool{}
	walkPublicRunnableLeaves(root, func(leaf *cobra.Command) {
		path := normalizeSchemaCLIPath(strings.Join(commandPathParts(leaf), " "))
		if path == "" {
			return
		}
		seenPublic[path] = true
		if coveredPaths[path] {
			report.Covered = append(report.Covered, path)
			return
		}
		if _, ok := exclusionByPath[path]; ok {
			report.Excluded = append(report.Excluded, path)
			usedExclusions[path] = true
			return
		}
		report.Missing = append(report.Missing, path)
	})

	for path := range exclusionByPath {
		if !seenPublic[path] || !usedExclusions[path] {
			report.StaleExclusions = append(report.StaleExclusions, path)
		}
	}
	sort.Strings(report.Covered)
	sort.Strings(report.Excluded)
	sort.Strings(report.Missing)
	sort.Strings(report.InvalidExclusions)
	sort.Strings(report.StaleExclusions)
	return report
}

func walkPublicRunnableLeaves(root *cobra.Command, fn func(*cobra.Command)) {
	if root == nil {
		return
	}
	var walk func(*cobra.Command)
	walk = func(command *cobra.Command) {
		if command.Runnable() && !command.HasSubCommands() {
			fn(command)
			return
		}
		for _, child := range command.Commands() {
			if child.Name() == "help" || !child.IsAvailableCommand() {
				continue
			}
			walk(child)
		}
	}
	walk(root)
}

func normalizeSchemaCLIPath(path string) string {
	parts := strings.Fields(strings.TrimSpace(path))
	if len(parts) > 0 && parts[0] == "dws" {
		parts = parts[1:]
	}
	return strings.Join(parts, " ")
}
