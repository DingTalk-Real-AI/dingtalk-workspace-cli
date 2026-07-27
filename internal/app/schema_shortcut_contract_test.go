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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

const publicShortcutSchemaCount = 210

func TestEmbeddedSchemaPublishesEveryPublicShortcutContract(t *testing.T) {
	tools := embeddedSchemaAllToolsForHelpFlagTest(t, NewRootCommand())
	public := make([]shortcut.Shortcut, 0, publicShortcutSchemaCount)
	for _, candidate := range shortcut.All() {
		if candidate.UserDefined || !shortcut.InPublicCatalog(candidate.Service, candidate.Command) {
			continue
		}
		public = append(public, candidate)
	}
	if got := len(public); got != publicShortcutSchemaCount {
		t.Fatalf("public built-in shortcuts = %d, want %d", got, publicShortcutSchemaCount)
	}

	deliveredShortcuts := 0
	for canonical := range tools {
		if strings.Contains(canonical, ".shortcut_") {
			deliveredShortcuts++
		}
	}
	if deliveredShortcuts != publicShortcutSchemaCount {
		t.Fatalf("embedded schema --all shortcut tools = %d, want %d", deliveredShortcuts, publicShortcutSchemaCount)
	}

	for _, declared := range public {
		declared := declared
		t.Run(declared.Service+"/"+strings.TrimPrefix(declared.Command, "+"), func(t *testing.T) {
			canonical := shortcutSchemaCanonical(declared)
			tool := tools[canonical]
			if tool == nil {
				t.Fatalf("embedded schema --all is missing %s (%s %s)", canonical, declared.Service, declared.Command)
			}
			assertEmbeddedShortcutIdentityAndSelection(t, tool, declared, canonical)
			assertEmbeddedShortcutSafetyAndInterface(t, tool, declared, canonical)
			assertEmbeddedShortcutParameters(t, tool, declared, canonical)
			assertEmbeddedShortcutConstraints(t, tool, declared, canonical)
		})
	}
}

func TestEmbeddedShortcutProgressiveQueriesReturnCompleteContracts(t *testing.T) {
	leaf := executeShortcutSchemaQuery(t, "--cli-path", "chat +messages-read-status")
	if got, want := schemaContractString(leaf["canonical_path"]), "chat.shortcut_messages_read_status"; got != want {
		t.Fatalf("shortcut leaf canonical_path = %q, want %q", got, want)
	}
	if got, want := schemaContractString(leaf["confirmation"]), "not_required"; got != want {
		t.Fatalf("shortcut leaf confirmation = %q, want %q", got, want)
	}
	conversationID := schemaContractMap(leaf["parameters"])["conversation-id"]
	if required, _ := conversationID["required"].(bool); !required {
		t.Fatal("public --conversation-id must become required after hidden compatibility aliases are removed from Schema")
	}
	if got := leaf["constraints"]; got != nil {
		t.Fatalf("shortcut leaf constraints = %#v, want omitted after hidden compatibility aliases collapse", got)
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
	if got, want := int(product["count"].(float64)), 120; got != want {
		t.Fatalf("schema chat count = %d, want %d", got, want)
	}
	summaries := schemaContractObjectSlice(productPayload["tools"])
	shortcutCount := 0
	for _, summary := range summaries {
		if strings.HasPrefix(schemaContractString(summary["canonical_path"]), "chat.shortcut_") {
			shortcutCount++
		}
	}
	if shortcutCount != 42 {
		t.Fatalf("schema chat shortcut summaries = %d, want 42", shortcutCount)
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

func assertEmbeddedShortcutIdentityAndSelection(
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

func assertEmbeddedShortcutSafetyAndInterface(
	t testing.TB,
	tool map[string]any,
	declared shortcut.Shortcut,
	canonical string,
) {
	t.Helper()
	risk := declared.Risk
	if risk == "" {
		risk = shortcut.RiskRead
	}
	wantEffect, wantRisk, wantConfirmation, wantIdempotency := "read", "low", "not_required", "idempotent"
	switch risk {
	case shortcut.RiskWrite:
		wantEffect, wantRisk, wantConfirmation, wantIdempotency = "write", "medium", "user_required", "unknown"
	case shortcut.RiskHighWrite:
		wantEffect, wantRisk, wantConfirmation, wantIdempotency = "destructive", "high", "user_required", "unknown"
	}
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

func assertEmbeddedShortcutParameters(
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
		if len(visible) == 1 && visible[0] == flagName {
			return true
		}
	}
	return false
}

func assertEmbeddedShortcutConstraints(
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
		flags := make([]string, 0, len(constraint.Flags))
		for _, flagName := range constraint.Flags {
			if public[flagName] {
				flags = append(flags, flagName)
			}
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
				for _, requiredText := range []string{"原文不能为空", "不能重复"} {
					if !strings.Contains(description, requiredText) {
						t.Errorf("%s --%s description does not publish custom constraint %q: %q", canonical, flagName, requiredText, description)
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

func mustShortcutJSON(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%#v", value)
	}
	return string(encoded)
}
