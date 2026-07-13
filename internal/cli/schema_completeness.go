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

//go:embed schema_command_exclusions.json
var embeddedRuntimeSchemaExclusionsJSON []byte

// RuntimeSchemaCompletenessReport compares the public executable Cobra leaves
// with the commands represented by runtime Schema annotations.
type RuntimeSchemaCompletenessReport struct {
	Covered           []string
	Excluded          []string
	Missing           []string
	InvalidExclusions []string
	StaleExclusions   []string
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
	if _, err := ApplyEmbeddedManualSchemaHints(root); err != nil {
		return err
	}
	exclusions, err := EmbeddedRuntimeSchemaExclusions()
	if err != nil {
		return err
	}
	report := RuntimeSchemaCompleteness(root, exclusions)
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

// RuntimeSchemaCompleteness scans the real command tree in the reverse
// direction: every public executable leaf must either carry a Schema identity
// or have a reviewed exclusion with a non-empty reason.
func RuntimeSchemaCompleteness(root *cobra.Command, exclusions []RuntimeSchemaExclusion) RuntimeSchemaCompletenessReport {
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
	coveredPaths := map[string]bool{}
	for _, entry := range collectRuntimeSchemaEntries(root) {
		coveredPaths[normalizeSchemaCLIPath(entry.CLIPath)] = true
		coveredPaths[normalizeSchemaCLIPath(entry.PrimaryCLIPath)] = true
		for _, alias := range entry.Aliases {
			coveredPaths[normalizeSchemaCLIPath(alias)] = true
		}
	}
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
	var walk func(*cobra.Command, bool)
	walk = func(command *cobra.Command, hiddenAncestor bool) {
		hidden := hiddenAncestor || command.Hidden
		if command != root && hidden {
			return
		}
		if command.Runnable() && !command.HasSubCommands() {
			fn(command)
			return
		}
		for _, child := range command.Commands() {
			if child.Name() == "help" || !child.IsAvailableCommand() {
				continue
			}
			walk(child, hidden)
		}
	}
	walk(root, false)
}

func normalizeSchemaCLIPath(path string) string {
	parts := strings.Fields(strings.TrimSpace(path))
	if len(parts) > 0 && parts[0] == "dws" {
		parts = parts[1:]
	}
	return strings.Join(parts, " ")
}
