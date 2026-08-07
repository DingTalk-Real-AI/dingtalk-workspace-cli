// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

const (
	publicShortcutCount = 357
	// schemaPublishedShortcutCount counts every delivered *.shortcut_* tool,
	// including hidden leaves such as minutes.shortcut_minutes_search.
	schemaPublishedShortcutCount = 358
	// publiclyDeliveredShortcutCount is the public-catalog subset of that surface.
	publiclyDeliveredShortcutCount = 357
)

func TestDeliverySchemaCoversOrExactlyExcludesEveryPublicShortcutContract(t *testing.T) {
	tools := deliverySchemaAllToolsForHelpFlagTest(t, NewRootCommand())
	public := make([]shortcut.Shortcut, 0, publicShortcutCount)
	for _, candidate := range shortcut.All() {
		if candidate.UserDefined || !shortcut.InPublicCatalog(candidate.Service, candidate.Command) {
			continue
		}
		public = append(public, candidate)
	}
	if got := len(public); got != publicShortcutCount {
		t.Fatalf("public built-in shortcuts = %d, want %d", got, publicShortcutCount)
	}

	deliveredShortcuts := 0
	for canonical := range tools {
		if strings.Contains(canonical, ".shortcut_") {
			deliveredShortcuts++
		}
	}
	if deliveredShortcuts != schemaPublishedShortcutCount {
		t.Fatalf("delivery schema --all shortcut tools = %d, want %d", deliveredShortcuts, schemaPublishedShortcutCount)
	}

	exclusions, err := cli.ReviewedRuntimeSchemaExclusions()
	if err != nil {
		t.Fatal(err)
	}
	excludedPaths := make(map[string]bool, len(exclusions))
	for _, exclusion := range exclusions {
		if !exclusion.Reviewed || strings.TrimSpace(exclusion.Reason) == "" {
			t.Fatalf("unreviewed public command exclusion: %#v", exclusion)
		}
		excludedPaths[exclusion.CLIPath] = true
	}

	excludedShortcuts := 0
	for _, declared := range public {
		declared := declared
		t.Run(declared.Service+"/"+strings.TrimPrefix(declared.Command, "+"), func(t *testing.T) {
			canonical := shortcutSchemaCanonical(declared)
			tool := tools[canonical]
			if tool == nil {
				cliPath := declared.Service + " " + declared.Command
				if !excludedPaths[cliPath] {
					t.Fatalf("delivery schema --all is missing %s (%s) without an exact reviewed exclusion", canonical, cliPath)
				}
				excludedShortcuts++
				return
			}
			assertDeliveryShortcutIdentityAndSelection(t, tool, declared, canonical)
			assertDeliveryShortcutSafetyAndInterface(t, tool, declared, canonical)
			assertDeliveryShortcutParameters(t, tool, declared, canonical)
			assertDeliveryShortcutConstraints(t, tool, declared, canonical)
		})
	}
	if got, want := excludedShortcuts, publicShortcutCount-publiclyDeliveredShortcutCount; got != want {
		t.Fatalf("exactly excluded public shortcuts = %d, want %d", got, want)
	}
}

func TestDeliveryShortcutProgressiveQueriesReturnCompleteContracts(t *testing.T) {
	leaf := executeShortcutSchemaQuery(t, "--cli-path", "chat +messages-read-status")
	if got, want := schemaContractString(leaf["canonical_path"]), "chat.shortcut_messages_read_status"; got != want {
		t.Fatalf("shortcut leaf canonical_path = %q, want %q", got, want)
	}
	if got, want := schemaContractString(leaf["confirmation"]), "not_required"; got != want {
		t.Fatalf("shortcut leaf confirmation = %q, want %q", got, want)
	}
	conversationID := schemaContractMap(leaf["parameters"])["conversation-id"]
	if required, _ := conversationID["required"].(bool); required {
		t.Fatal("public --conversation-id must stay optional when hidden siblings still satisfy the declared exactly_one group")
	}
	wantMessagesConstraints := map[string]any{
		"require_one_of":     [][]string{{"conversation-id", "group", "id"}},
		"mutually_exclusive": [][]string{{"conversation-id", "group", "id"}},
	}
	if got := leaf["constraints"]; !schemaContractJSONEqual(got, wantMessagesConstraints) {
		t.Fatalf("shortcut leaf constraints = %#v, want %#v", got, wantMessagesConstraints)
	}

	constrainedLeaf := executeShortcutSchemaQuery(t, "--cli-path", "calendar +freebusy")
	wantConstraints := map[string]any{
		"require_one_of": [][]string{{"users", "rooms"}},
	}
	if got := constrainedLeaf["constraints"]; !schemaContractJSONEqual(got, wantConstraints) {
		t.Fatalf("shortcut leaf constraints = %#v, want %#v", got, wantConstraints)
	}

	product := executeShortcutSchemaQuery(t, "chat")
	productPayload, _ := product["product"].(map[string]any)
	if got, want := int(product["count"].(float64)), 187; got != want {
		t.Fatalf("schema chat count = %d, want %d", got, want)
	}
	summaries := schemaContractObjectSlice(productPayload["tools"])
	shortcutCount := 0
	for _, summary := range summaries {
		if strings.HasPrefix(schemaContractString(summary["canonical_path"]), "chat.shortcut_") {
			shortcutCount++
		}
	}
	if shortcutCount != 98 {
		t.Fatalf("schema chat shortcut summaries = %d, want 98", shortcutCount)
	}
}

func executeShortcutSchemaQuery(t testing.TB, args ...string) map[string]any {
	t.Helper()
	root := NewRootCommand()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(append([]string{"schema"}, append(args, "--format", "json")...))
	if err := root.Execute(); err != nil {
		t.Fatalf("execute dws schema %q: %v; stderr=%s", strings.Join(args, " "), err, stderr.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode dws schema %q: %v", strings.Join(args, " "), err)
	}
	return payload
}

func shortcutSchemaCanonical(declared shortcut.Shortcut) string {
	name := strings.ReplaceAll(strings.TrimPrefix(declared.Command, "+"), "-", "_")
	return declared.Service + ".shortcut_" + name
}

func assertDeliveryShortcutIdentityAndSelection(
	t testing.TB,
	tool map[string]any,
	declared shortcut.Shortcut,
	canonical string,
) {
	t.Helper()
	if got, want := schemaContractString(tool["canonical_path"]), canonical; got != want {
		t.Errorf("canonical_path = %q, want %q", got, want)
	}
	if got, want := schemaContractString(tool["primary_cli_path"]), declared.Service+" "+declared.Command; got != want {
		t.Errorf("%s primary_cli_path = %q, want %q", canonical, got, want)
	}
	if got, want := schemaContractString(tool["agent_summary"]), declared.Description; got != want {
		t.Errorf("%s agent_summary = %q, want %q", canonical, got, want)
	}
	if got, want := schemaContractStringSlice(tool["use_when"]), []string{declared.Intent}; !schemaContractJSONEqual(got, want) {
		t.Errorf("%s use_when = %#v, want %#v", canonical, got, want)
	}
	if len(schemaContractStringSlice(tool["avoid_when"])) == 0 {
		t.Errorf("%s has no reviewed avoid_when", canonical)
	}
	examples := schemaContractStringSlice(tool["examples"])
	if len(examples) == 0 || len(examples) > 2 {
		t.Errorf("%s examples = %d, want 1..2", canonical, len(examples))
	}
	for _, example := range examples {
		if strings.Contains(example, "--yes") {
			t.Errorf("%s stores unsafe example %q", canonical, example)
		}
		if !strings.HasPrefix(example, "dws "+declared.Service+" "+declared.Command) {
			t.Errorf("%s example does not use its primary path: %q", canonical, example)
		}
	}
}

func assertDeliveryShortcutSafetyAndInterface(
	t testing.TB,
	tool map[string]any,
	declared shortcut.Shortcut,
	canonical string,
) {
	t.Helper()
	safety := shortcut.EffectiveSafety(declared)
	wantEffect, wantRisk := safety.Effect, safety.Risk
	wantConfirmation, wantIdempotency := safety.Confirmation, safety.Idempotency
	for field, want := range map[string]string{
		"effect":         wantEffect,
		"risk":           wantRisk,
		"confirmation":   wantConfirmation,
		"idempotency":    wantIdempotency,
		"interface_mode": "composite",
		"availability":   "available",
	} {
		if got := schemaContractString(tool[field]); got != want {
			t.Errorf("%s %s = %q, want %q", canonical, field, got, want)
		}
	}
	if strings.TrimSpace(schemaContractString(tool["interface_reason"])) == "" {
		t.Errorf("%s has no reviewed composite interface reason", canonical)
	}
}

func assertDeliveryShortcutParameters(
	t testing.TB,
	tool map[string]any,
	declared shortcut.Shortcut,
	canonical string,
) {
	t.Helper()
	parameters := schemaContractMap(tool["parameters"])
	publicFlags := make([]shortcut.Flag, 0, len(declared.Flags))
	for _, flag := range declared.Flags {
		if !flag.Hidden {
			publicFlags = append(publicFlags, flag)
			if flag.AliasesVisible {
				for _, alias := range flag.Aliases {
					aliasFlag := flag
					aliasFlag.Name = alias
					aliasFlag.Default = ""
					aliasFlag.Aliases = nil
					publicFlags = append(publicFlags, aliasFlag)
				}
			}
		}
	}
	if got, want := len(parameters), len(publicFlags); got != want {
		t.Errorf("%s parameters = %d, want %d", canonical, got, want)
	}
	for _, flag := range publicFlags {
		parameter := parameters[flag.Name]
		if parameter == nil {
			t.Errorf("%s is missing parameter --%s", canonical, flag.Name)
			continue
		}
		flagType := flag.Type
		if flagType == "" {
			flagType = shortcut.FlagString
		}
		wantType := map[shortcut.FlagType]string{
			shortcut.FlagString:      "string",
			shortcut.FlagBool:        "boolean",
			shortcut.FlagInt:         "integer",
			shortcut.FlagStringSlice: "array",
		}[flagType]
		if got := schemaContractString(parameter["type"]); got != wantType {
			t.Errorf("%s --%s type = %q, want %q", canonical, flag.Name, got, wantType)
		}
		if got, _ := parameter["required"].(bool); got != shortcutSchemaRequired(declared, flag.Name) {
			t.Errorf("%s --%s required = %t, want %t", canonical, flag.Name, got, shortcutSchemaRequired(declared, flag.Name))
		}
		if got, want := schemaContractString(parameter["default"]), shortcutSchemaDefault(flag); got != want {
			t.Errorf("%s --%s default = %q, want %q", canonical, flag.Name, got, want)
		}
		gotEnum := schemaContractStringSlice(parameter["enum"])
		if len(flag.Enum) == 0 {
			if len(gotEnum) != 0 {
				t.Errorf("%s --%s enum = %#v, want empty", canonical, flag.Name, gotEnum)
			}
		} else if !schemaContractJSONEqual(gotEnum, flag.Enum) {
			t.Errorf("%s --%s enum = %#v, want %#v", canonical, flag.Name, gotEnum, flag.Enum)
		}
	}
}

func shortcutSchemaDefault(flag shortcut.Flag) string {
	value := strings.TrimSpace(flag.Default)
	switch flag.Type {
	case shortcut.FlagBool:
		if value != "true" {
			return ""
		}
	case shortcut.FlagInt:
		if value == "0" {
			return ""
		}
	case shortcut.FlagStringSlice:
		if value != "" {
			return "[" + value + "]"
		}
	}
	return value
}

func shortcutSchemaRequired(declared shortcut.Shortcut, flagName string) bool {
	for _, flag := range declared.Flags {
		if flag.Name == flagName && flag.Required {
			return true
		}
		if flag.Required && flag.AliasesVisible {
			for _, alias := range flag.Aliases {
				if alias == flagName {
					return true
				}
			}
		}
	}
	public := make(map[string]bool, len(declared.Flags))
	for _, flag := range declared.Flags {
		if !flag.Hidden {
			public[flag.Name] = true
		}
	}
	for _, constraint := range declared.Constraints {
		if constraint.Kind != shortcut.ConstraintAtLeastOne && constraint.Kind != shortcut.ConstraintExactlyOne {
			continue
		}
		visible := make([]string, 0, len(constraint.Flags))
		for _, constrained := range constraint.Flags {
			if public[constrained] {
				visible = append(visible, constrained)
			}
		}
		// Match AnnotateConstraints: only collapse to required when the projected
		// group has a single member (no remaining hidden siblings).
		flags := visible
		if len(visible) < len(constraint.Flags) {
			flags = append([]string(nil), constraint.Flags...)
		}
		if len(flags) == 1 && flags[0] == flagName {
			return true
		}
	}
	return false
}

func assertDeliveryShortcutConstraints(
	t testing.TB,
	tool map[string]any,
	declared shortcut.Shortcut,
	canonical string,
) {
	t.Helper()
	public := make(map[string]bool, len(declared.Flags))
	for _, flag := range declared.Flags {
		if !flag.Hidden {
			public[flag.Name] = true
		}
	}
	want := map[string][][]string{}
	for _, constraint := range declared.Constraints {
		visible := make([]string, 0, len(constraint.Flags))
		for _, flagName := range constraint.Flags {
			if public[flagName] {
				visible = append(visible, flagName)
			}
		}
		// Match AnnotateConstraints declare≡execute projection: keep the full
		// declared group when any hidden sibling remains.
		flags := visible
		if len(visible) < len(constraint.Flags) {
			flags = append([]string(nil), constraint.Flags...)
		}
		switch constraint.Kind {
		case shortcut.ConstraintAtLeastOne:
			if len(flags) > 1 {
				want["require_one_of"] = append(want["require_one_of"], flags)
			}
		case shortcut.ConstraintExactlyOne:
			if len(flags) > 1 {
				want["require_one_of"] = append(want["require_one_of"], flags)
				want["mutually_exclusive"] = append(want["mutually_exclusive"], flags)
			}
		case shortcut.ConstraintMutuallyExclusive:
			if len(flags) > 1 {
				want["mutually_exclusive"] = append(want["mutually_exclusive"], flags)
			}
		case shortcut.ConstraintCustom:
			for _, flagName := range flags {
				description := schemaContractString(schemaContractMap(tool["parameters"])[flagName]["description"])
				for _, evidence := range shortcutCustomConstraintEvidence(constraint.Description) {
					if !strings.Contains(description, evidence) {
						t.Errorf("%s --%s description does not publish custom constraint evidence %q: %q", canonical, flagName, evidence, description)
					}
				}
			}
		default:
			t.Errorf("%s has unsupported declared shortcut constraint %q", canonical, constraint.Kind)
		}
	}
	if len(want) == 0 {
		if got := tool["constraints"]; got != nil {
			t.Errorf("%s constraints = %#v, want omitted", canonical, got)
		}
		return
	}
	if got := tool["constraints"]; !schemaContractJSONEqual(got, want) {
		t.Errorf("%s constraints = %s, want %s", canonical, mustShortcutJSON(got), mustShortcutJSON(want))
	}
}

func shortcutCustomConstraintEvidence(description string) []string {
	// Custom constraints are prose rather than a typed wire contract. Require
	// their decision-relevant facts to survive in the delivered parameter
	// description while allowing the renderer to reorder connective wording.
	probes := []string{
		"原文=>替换",
		"不能为空",
		"不能重复",
		"大于 0",
		"工作目录",
		"相对路径",
		"绝对路径",
		"..",
		"最多 15 个字符",
		"能力矩阵",
	}
	evidence := make([]string, 0, len(probes))
	for _, probe := range probes {
		if strings.Contains(description, probe) {
			evidence = append(evidence, probe)
		}
	}
	if len(evidence) > 0 {
		return evidence
	}
	return []string{strings.TrimSpace(description)}
}

func mustShortcutJSON(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%#v", value)
	}
	return string(encoded)
}
