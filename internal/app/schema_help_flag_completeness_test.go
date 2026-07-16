// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// TestFinalSchemaParametersMatchExecutableHelpFlags is the fast, in-process
// Help <-> Schema parameter completeness gate. It deliberately starts from the
// reviewed registry and its exact Cobra bindings, then compares every public
// primary leaf with the final delivered ToolSpec projection. The binder has
// already proved that reviewed compatibility leaves have the same executable
// contract as their primary, so aliases do not create a second parameter
// source here.
func TestFinalSchemaParametersMatchExecutableHelpFlags(t *testing.T) {
	root := NewRootCommand()
	bound := boundSchemaCommandsForHelpFlagTest(t, root)
	snapshot := fullSchemaSnapshotForTest(t)
	assertSchemaParametersMatchExecutableHelpFlags(t, bound, snapshot.Tools, "source-built final Schema")
}

// TestEmbeddedSchemaParametersMatchExecutableHelpFlags runs the same exact-set
// gate against the artifact that ships in the binary. Going through the real
// schema --all command is intentional: a stale generated Catalog must fail
// even when a fresh source-built snapshot would agree with Cobra Help.
func TestEmbeddedSchemaParametersMatchExecutableHelpFlags(t *testing.T) {
	root := NewRootCommand()
	bound := boundSchemaCommandsForHelpFlagTest(t, root)
	tools := embeddedSchemaAllToolsForHelpFlagTest(t, root)
	assertSchemaParametersMatchExecutableHelpFlags(t, bound, tools, "embedded schema --all")
}

func boundSchemaCommandsForHelpFlagTest(t testing.TB, root *cobra.Command) cli.BoundCommandRegistry {
	t.Helper()
	effective, err := cli.BuildEffectiveCommandRegistry(root)
	if err != nil {
		t.Fatalf("build EffectiveCommandRegistry: %v", err)
	}
	bound, err := cli.BindEffectiveCommandRegistry(root, effective)
	if err != nil {
		t.Fatalf("bind EffectiveCommandRegistry to live Cobra tree: %v", err)
	}
	return bound
}

func embeddedSchemaAllToolsForHelpFlagTest(t testing.TB, root *cobra.Command) map[string]map[string]any {
	t.Helper()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"schema", "--all", "--format", "json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute embedded schema --all: %v; stderr=%s", err, stderr.String())
	}
	var payload struct {
		Products []struct {
			Tools []map[string]any `json:"tools"`
		} `json:"products"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode embedded schema --all: %v", err)
	}
	tools := make(map[string]map[string]any)
	for _, product := range payload.Products {
		for _, tool := range product.Tools {
			canonical := strings.TrimSpace(schemaContractString(tool["canonical_path"]))
			if canonical == "" {
				t.Fatal("embedded schema --all contains an empty canonical path")
			}
			if _, exists := tools[canonical]; exists {
				t.Fatalf("embedded schema --all contains duplicate canonical %q", canonical)
			}
			tools[canonical] = tool
		}
	}
	if len(tools) == 0 {
		t.Fatal("embedded schema --all contains no tools")
	}
	return tools
}

func assertSchemaParametersMatchExecutableHelpFlags(
	t testing.TB,
	bound cli.BoundCommandRegistry,
	tools map[string]map[string]any,
	source string,
) {
	t.Helper()
	problems := schemaHelpFlagCompletenessProblems(bound, tools)
	checked := 0
	for _, command := range bound.Commands {
		if command.Visibility == cli.SchemaVisibilityPublic {
			checked++
		}
	}
	if len(problems) > 0 {
		t.Fatalf("%s parameter surface differs from executable Cobra Help:\n%s", source, strings.Join(problems, "\n"))
	}
	t.Logf("validated Help-visible flags against %s parameters for %d public tools", source, checked)
}

func TestSchemaHelpFlagCompletenessRejectsStaleCatalogFlags(t *testing.T) {
	leaf := &cobra.Command{Use: "run", Run: func(*cobra.Command, []string) {}}
	leaf.Flags().String("fresh", "", "new executable input")
	bound := cli.BoundCommandRegistry{Commands: []cli.BoundCommandSpec{{
		CommandSpec: cli.CommandSpec{
			CanonicalPath:  "sample.run",
			PrimaryCLIPath: "sample run",
			Visibility:     cli.SchemaVisibilityPublic,
		},
		PrimaryCommand: leaf,
	}}}
	tools := map[string]map[string]any{
		"sample.run": {
			"parameters": map[string]any{
				"stale": map[string]any{"type": "string"},
			},
		},
	}

	problems := schemaHelpFlagCompletenessProblems(bound, tools)
	joined := strings.Join(problems, "\n")
	if !strings.Contains(joined, `missing_in_schema=["fresh"]`) ||
		!strings.Contains(joined, `extra_in_schema=["stale"]`) {
		t.Fatalf("stale Catalog flag drift was not reported: %s", joined)
	}
}

func schemaHelpFlagCompletenessProblems(bound cli.BoundCommandRegistry, tools map[string]map[string]any) []string {
	var problems []string
	public := make(map[string]bool)
	checked := 0
	for _, command := range bound.Commands {
		if command.Visibility != cli.SchemaVisibilityPublic {
			continue
		}
		public[command.CanonicalPath] = true
		checked++
		tool, ok := tools[command.CanonicalPath]
		if !ok {
			problems = append(problems, fmt.Sprintf(
				"canonical=%q path=%q missing final Schema tool",
				command.CanonicalPath,
				command.PrimaryCLIPath,
			))
			continue
		}
		if problem := schemaHelpFlagCompletenessProblem(
			command.CanonicalPath,
			command.PrimaryCLIPath,
			command.PrimaryCommand,
			tool,
		); problem != "" {
			problems = append(problems, problem)
		}
	}
	for canonical := range tools {
		if !public[canonical] {
			problems = append(problems, fmt.Sprintf("canonical=%q is an unexpected final Schema tool", canonical))
		}
	}
	if checked != len(tools) {
		problems = append(problems, fmt.Sprintf(
			"public BoundCommand count=%d final Schema tool count=%d",
			checked,
			len(tools),
		))
	}
	sort.Strings(problems)
	return problems
}

func TestSchemaHelpFlagCompletenessRejectsAncestorPersistentLeak(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	root.PersistentFlags().String("format", "json", "output format")
	product := &cobra.Command{Use: "product"}
	product.PersistentFlags().String("leaked", "", "product-scoped option")
	leaf := &cobra.Command{Use: "run", Run: func(*cobra.Command, []string) {}}
	leaf.Flags().String("declared", "", "declared option")
	leaf.Flags().String("json", "", "Base JSON object payload for this tool invocation")
	leaf.Flags().String("hidden", "", "internal option")
	_ = leaf.Flags().MarkHidden("hidden")
	root.AddCommand(product)
	product.AddCommand(leaf)

	tool := map[string]any{
		"parameters": map[string]any{
			"declared": map[string]any{"type": "string"},
		},
	}
	problem := schemaHelpFlagCompletenessProblem("product.run", "product run", leaf, tool)
	if !strings.Contains(problem, `missing_in_schema=["leaked"]`) {
		t.Fatalf("ancestor persistent leak was not reported: %s", problem)
	}
	if strings.Contains(problem, "format") || strings.Contains(problem, "json") || strings.Contains(problem, "hidden") {
		t.Fatalf("root controls and reviewed non-Schema flags must be excluded: %s", problem)
	}
}

func schemaHelpFlagCompletenessProblem(canonical, path string, command *cobra.Command, tool map[string]any) string {
	helpFlags := schemaHelpVisibleFlagNames(command)
	schemaFlags := make(map[string]bool)
	for name := range schemaContractMap(tool["parameters"]) {
		schemaFlags[name] = true
	}

	missing := schemaFlagNameDifference(helpFlags, schemaFlags)
	extra := schemaFlagNameDifference(schemaFlags, helpFlags)
	if len(missing) == 0 && len(extra) == 0 {
		return ""
	}
	return fmt.Sprintf(
		"canonical=%q path=%q missing_in_schema=%s extra_in_schema=%s",
		canonical,
		path,
		schemaQuotedFlagNames(missing),
		schemaQuotedFlagNames(extra),
	)
}

// schemaHelpVisibleFlagNames models Cobra's leaf Help surface without rendering
// text. Local flags and ancestor persistent flags are executable tool inputs.
// Only the reviewed root execution controls are omitted; an unexpected new
// root persistent flag must fail this gate instead of being silently treated as
// process scaffolding.
func schemaHelpVisibleFlagNames(command *cobra.Command) map[string]bool {
	visible := make(map[string]bool)
	if command == nil {
		return visible
	}

	visit := func(flag *pflag.Flag, rootPersistent bool) {
		if flag == nil || flag.Hidden || flag.Name == "help" || schemaGenericPayloadEscapeHatch(flag) {
			return
		}
		if rootPersistent && schemaRootExecutionControl(flag.Name) {
			return
		}
		visible[flag.Name] = true
	}
	command.LocalNonPersistentFlags().VisitAll(func(flag *pflag.Flag) { visit(flag, false) })
	command.PersistentFlags().VisitAll(func(flag *pflag.Flag) { visit(flag, false) })
	root := command.Root()
	for parent := command.Parent(); parent != nil; parent = parent.Parent() {
		isRoot := parent == root
		parent.PersistentFlags().VisitAll(func(flag *pflag.Flag) { visit(flag, isRoot) })
	}
	return visible
}

func schemaRootExecutionControl(name string) bool {
	switch name {
	case "client-id", "client-secret", "debug", "dry-run", "fields", "format", "jq", "mock",
		"output", "profile", "timeout", "token", "verbose", "yes":
		return true
	default:
		return false
	}
}

func schemaGenericPayloadEscapeHatch(flag *pflag.Flag) bool {
	if flag == nil {
		return false
	}
	switch flag.Name {
	case "json":
		return strings.TrimSpace(flag.Usage) == "Base JSON object payload for this tool invocation"
	case "params":
		return strings.TrimSpace(flag.Usage) == "Additional JSON object payload merged after --json"
	default:
		return false
	}
}

func schemaFlagNameDifference(left, right map[string]bool) []string {
	result := make([]string, 0)
	for name := range left {
		if !right[name] {
			result = append(result, name)
		}
	}
	sort.Strings(result)
	return result
}

func schemaQuotedFlagNames(names []string) string {
	quoted := make([]string, 0, len(names))
	for _, name := range names {
		quoted = append(quoted, fmt.Sprintf("%q", name))
	}
	return "[" + strings.Join(quoted, ",") + "]"
}
