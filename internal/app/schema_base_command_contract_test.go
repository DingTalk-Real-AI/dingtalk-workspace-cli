// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package app

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/spf13/cobra"
)

// TestFinalSchemaToolsHaveExecutableBaseCommands is the final Schema-to-Cobra
// delivery gate. It starts from the reviewed CommandRegistry and live Cobra
// tree, then verifies the complete final Schema projection against the bound
// commands. The Catalog is observed only as a delivery output; it is never
// used to discover or synthesize a command identity.
func TestFinalSchemaToolsHaveExecutableBaseCommands(t *testing.T) {
	root := NewRootCommand()
	snapshot := fullSchemaSnapshotForTest(t)
	effective, err := cli.BuildEffectiveCommandRegistry(root)
	if err != nil {
		t.Fatalf("build EffectiveCommandRegistry: %v", err)
	}
	bound, err := cli.BindEffectiveCommandRegistry(root, effective)
	if err != nil {
		t.Fatalf("bind EffectiveCommandRegistry to live Cobra tree: %v", err)
	}

	publicCanonicals := make([]string, 0, len(bound.Commands))
	for _, command := range bound.Commands {
		if command.Visibility == cli.SchemaVisibilityPublic {
			publicCanonicals = append(publicCanonicals, command.CanonicalPath)
		}
	}
	sort.Strings(publicCanonicals)
	finalCanonicals := make([]string, 0, len(snapshot.Tools))
	for canonical := range snapshot.Tools {
		finalCanonicals = append(finalCanonicals, canonical)
	}
	sort.Strings(finalCanonicals)
	if diff := schemaBaseCommandSetDiff(publicCanonicals, finalCanonicals); diff != "" {
		t.Fatalf("final Schema tool set differs from public BoundCommandRegistry: %s", diff)
	}

	for _, canonical := range finalCanonicals {
		canonical := canonical
		t.Run(canonical, func(t *testing.T) {
			tool := snapshot.Tools[canonical]
			command, ok := bound.ByCanonical[canonical]
			if !ok {
				t.Fatalf("final Schema tool has no BoundCommand")
			}
			if command.Visibility != cli.SchemaVisibilityPublic {
				t.Fatalf("final Schema tool binds non-public command visibility %q", command.Visibility)
			}
			if got := schemaBaseCommandString(tool["canonical_path"]); got != canonical {
				t.Fatalf("final canonical_path = %q, want %q", got, canonical)
			}
			if got := schemaBaseCommandString(tool["primary_cli_path"]); got != command.PrimaryCLIPath {
				t.Fatalf("final primary_cli_path = %q, want bound primary %q", got, command.PrimaryCLIPath)
			}
			if got := schemaBaseCommandString(tool["cli_path"]); got != command.PrimaryCLIPath {
				t.Fatalf("final canonical view cli_path = %q, want bound primary %q", got, command.PrimaryCLIPath)
			}

			primaryMatch, err := resolveSchemaBaseCommandPath(root, command.PrimaryCLIPath)
			if err != nil {
				t.Fatalf("resolve primary path exactly: %v", err)
			}
			if primaryMatch.command == nil {
				t.Fatalf("bound primary path %q does not exist in live Cobra tree", command.PrimaryCLIPath)
			}
			if primaryMatch.usedAlias {
				t.Fatalf("bound primary path %q resolves through Cobra Aliases", command.PrimaryCLIPath)
			}
			if primaryMatch.command != command.PrimaryCommand {
				t.Fatalf("bound primary pointer differs from exact live Cobra path %q", command.PrimaryCLIPath)
			}
			assertRunnableSchemaBaseCommand(t, command.PrimaryCommand, command.PrimaryCLIPath)

			// Cobra dispatches a parsed --help flag to Help without invoking the
			// command's Run/RunE or any business interface. Calling Help directly
			// therefore exercises the same renderer without network side effects.
			var help bytes.Buffer
			command.PrimaryCommand.SetOut(&help)
			command.PrimaryCommand.SetErr(&help)
			if err := command.PrimaryCommand.Help(); err != nil {
				t.Fatalf("render %q --help: %v", command.PrimaryCLIPath, err)
			}
			if strings.TrimSpace(help.String()) == "" {
				t.Fatalf("%q --help rendered an empty document", command.PrimaryCLIPath)
			}

			wantAliases := append([]string(nil), command.Aliases...)
			gotAliases := schemaBaseCommandStringSlice(tool["aliases"])
			sort.Strings(wantAliases)
			sort.Strings(gotAliases)
			if diff := schemaBaseCommandSetDiff(wantAliases, gotAliases); diff != "" {
				t.Fatalf("final aliases differ from BoundCommand: %s", diff)
			}

			boundAliases := make(map[string]cli.BoundAlias, len(command.AliasCommands))
			for _, alias := range command.AliasCommands {
				if _, duplicate := boundAliases[alias.Path]; duplicate {
					t.Fatalf("BoundCommand has duplicate alias %q", alias.Path)
				}
				boundAliases[alias.Path] = alias
			}
			for _, aliasPath := range wantAliases {
				alias, ok := boundAliases[aliasPath]
				if !ok {
					t.Fatalf("registry alias %q has no BoundAlias", aliasPath)
				}
				aliasMatch, err := resolveSchemaBaseCommandPath(root, aliasPath)
				if err != nil {
					t.Fatalf("resolve alias %q exactly: %v", aliasPath, err)
				}
				if aliasMatch.command == nil {
					t.Fatalf("bound alias %q does not exist in live Cobra tree", aliasPath)
				}
				if aliasMatch.command != alias.Command {
					t.Fatalf("BoundAlias pointer differs from exact live Cobra path %q", aliasPath)
				}
				assertRunnableSchemaBaseCommand(t, alias.Command, aliasPath)
				switch alias.Kind {
				case cli.AliasKindCobraAlias:
					if !aliasMatch.usedAlias || alias.Command != command.PrimaryCommand {
						t.Fatalf("Cobra alias %q must resolve through Aliases to the primary command pointer", aliasPath)
					}
				case cli.AliasKindCompatibilityLeaf:
					if aliasMatch.usedAlias || alias.Command == command.PrimaryCommand {
						t.Fatalf("compatibility alias %q must be a separate exact-name Cobra leaf", aliasPath)
					}
				default:
					t.Fatalf("alias %q has unknown binding kind %q", aliasPath, alias.Kind)
				}
				if indexed, ok := bound.ByCLIPath[aliasPath]; !ok || indexed.CanonicalPath != canonical {
					t.Fatalf("BoundCommandRegistry path index %q does not resolve to %q", aliasPath, canonical)
				}
			}
			if len(boundAliases) != len(wantAliases) {
				t.Fatalf("BoundCommand exposes %d alias bindings for %d reviewed aliases", len(boundAliases), len(wantAliases))
			}
		})
	}

	t.Logf("validated %d final Schema tools and their executable base commands", len(finalCanonicals))
}

func assertRunnableSchemaBaseCommand(t *testing.T, command *cobra.Command, path string) {
	t.Helper()
	if command == nil || !command.Runnable() || command.HasSubCommands() {
		t.Fatalf("Schema path %q does not bind a runnable Cobra leaf", path)
	}
}

type schemaBaseCommandPathMatch struct {
	command   *cobra.Command
	usedAlias bool
}

// resolveSchemaBaseCommandPath independently resolves exact Cobra names and
// aliases for the delivery contract test. Like the production binder, it does
// not accept Cobra prefix matching or suggestions.
func resolveSchemaBaseCommandPath(root *cobra.Command, rawPath string) (schemaBaseCommandPathMatch, error) {
	parts := strings.Fields(strings.TrimSpace(rawPath))
	if len(parts) > 0 && root != nil && parts[0] == root.Name() {
		parts = parts[1:]
	}
	if root == nil || len(parts) == 0 {
		return schemaBaseCommandPathMatch{}, nil
	}

	current := root
	usedAlias := false
	for _, part := range parts {
		exact := schemaBaseCommandChildrenNamed(current, part, false)
		if len(exact) > 1 {
			return schemaBaseCommandPathMatch{}, fmt.Errorf("command segment %q is ambiguous", part)
		}
		if len(exact) == 1 {
			current = exact[0]
			continue
		}
		aliases := schemaBaseCommandChildrenNamed(current, part, true)
		if len(aliases) > 1 {
			return schemaBaseCommandPathMatch{}, fmt.Errorf("alias segment %q is ambiguous", part)
		}
		if len(aliases) == 0 {
			return schemaBaseCommandPathMatch{}, nil
		}
		current = aliases[0]
		usedAlias = true
	}
	return schemaBaseCommandPathMatch{command: current, usedAlias: usedAlias}, nil
}

func schemaBaseCommandChildrenNamed(parent *cobra.Command, name string, aliases bool) []*cobra.Command {
	var matches []*cobra.Command
	for _, child := range parent.Commands() {
		matched := child.Name() == name
		if aliases {
			matched = false
			for _, alias := range child.Aliases {
				if alias == name {
					matched = true
					break
				}
			}
		}
		if !matched {
			continue
		}
		seen := false
		for _, existing := range matches {
			if existing == child {
				seen = true
				break
			}
		}
		if !seen {
			matches = append(matches, child)
		}
	}
	return matches
}

func schemaBaseCommandString(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func schemaBaseCommandStringSlice(value any) []string {
	var values []string
	switch typed := value.(type) {
	case []string:
		values = append(values, typed...)
	case []any:
		for _, item := range typed {
			if text, ok := item.(string); ok {
				values = append(values, text)
			}
		}
	}
	for index := range values {
		values[index] = strings.TrimSpace(values[index])
	}
	return values
}

func schemaBaseCommandSetDiff(want, got []string) string {
	wantSet := make(map[string]bool, len(want))
	gotSet := make(map[string]bool, len(got))
	for _, value := range want {
		wantSet[value] = true
	}
	for _, value := range got {
		gotSet[value] = true
	}
	var missing, extra []string
	for value := range wantSet {
		if !gotSet[value] {
			missing = append(missing, value)
		}
	}
	for value := range gotSet {
		if !wantSet[value] {
			extra = append(extra, value)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing) == 0 && len(extra) == 0 && len(want) == len(got) {
		return ""
	}
	return fmt.Sprintf("missing=%v extra=%v want_count=%d got_count=%d", missing, extra, len(want), len(got))
}
