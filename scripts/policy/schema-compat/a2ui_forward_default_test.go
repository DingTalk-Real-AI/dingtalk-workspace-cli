// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package main

import (
	"encoding/json"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
)

func TestCrossPlatformCoverageA2UIForwardDefaultPolicy(t *testing.T) {
	const tool = "chat/chat.create_and_send_a2ui_card"
	const parameter = "support-forward"
	before := parameterSchema{Type: `"boolean"`, Property: "supportForward", Default: "false"}
	after := before
	after.Default = "true"
	if failures := checkParameterCompatibility(tool, parameter, before, after); len(failures) != 0 {
		t.Fatalf("exact reviewed transition failed: %v", failures)
	}
	for _, unchanged := range []parameterSchema{before, after} {
		if failures := checkParameterCompatibility(tool, parameter, unchanged, unchanged); len(failures) != 0 {
			t.Fatalf("unchanged default failed: %v", failures)
		}
	}

	tests := []struct {
		name   string
		mutate func(*string, *string, *parameterSchema, *parameterSchema)
	}{
		{"other tool", func(tool, _ *string, _, _ *parameterSchema) { *tool = "chat/chat.messages_send" }},
		{"other product", func(tool, _ *string, _, _ *parameterSchema) { *tool = "other/chat.create_and_send_a2ui_card" }},
		{"other parameter", func(_, name *string, _, _ *parameterSchema) { *name = "enable-forward" }},
		{"reverse", func(_, _ *string, a, b *parameterSchema) { a.Default, b.Default = "true", "false" }},
		{"missing before", func(_, _ *string, a, _ *parameterSchema) { a.Default = "" }},
		{"missing after", func(_, _ *string, _, b *parameterSchema) { b.Default = "" }},
		{"string before", func(_, _ *string, a, _ *parameterSchema) { a.Default = `"false"` }},
		{"string after", func(_, _ *string, _, b *parameterSchema) { b.Default = `"true"` }},
		{"wrong type", func(_, _ *string, a, b *parameterSchema) { a.Type, b.Type = `"string"`, `"string"` }},
		{"wrong property", func(_, _ *string, a, b *parameterSchema) { a.Property, b.Property = "other", "other" }},
		{"type drift", func(_, _ *string, _, b *parameterSchema) { b.Type = `"string"` }},
		{"property drift", func(_, _ *string, _, b *parameterSchema) { b.Property = "other" }},
		{"property provenance drift", func(_, _ *string, _, b *parameterSchema) { b.PropertySource = "changed" }},
		{"interface type drift", func(_, _ *string, _, b *parameterSchema) { b.InterfaceType = "boolean" }},
		{"interface default drift", func(_, _ *string, _, b *parameterSchema) { b.InterfaceDefault = "true" }},
		{"required drift", func(_, _ *string, _, b *parameterSchema) { b.Required = true }},
		{"CLI required drift", func(_, _ *string, _, b *parameterSchema) { b.CLIRequired = true }},
		{"required when drift", func(_, _ *string, _, b *parameterSchema) { b.RequiredWhen = "always" }},
		{"anyOf drift", func(_, _ *string, _, b *parameterSchema) { b.AnyOf = `[{"type":"boolean"}]` }},
		{"format drift", func(_, _ *string, _, b *parameterSchema) { b.Format = "other" }},
		{"enum drift", func(_, _ *string, _, b *parameterSchema) { b.Enum = []string{"true"} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			toolPath, name, oldParameter, newParameter := tool, parameter, before, after
			test.mutate(&toolPath, &name, &oldParameter, &newParameter)
			assertSchemaFailureContains(t, checkParameterCompatibility(toolPath, name, oldParameter, newParameter), "changed default")
		})
	}
}

// Exercise the real Cobra -> published Schema -> normalization path. The
// published default is absent for false and a JSON string for true, unlike the
// hand-built boolean contracts above.
func TestCrossPlatformCoverageA2UIForwardDefaultPublishedSchema(t *testing.T) {
	const tool = "chat/chat.create_and_send_a2ui_card"
	const parameter = "support-forward"
	before := publishedA2UIForwardParameter(t, "false")
	after := publishedA2UIForwardParameter(t, "true")
	if before.Default != "" || after.Default != `"true"` {
		t.Fatalf("published defaults changed: before=%q after=%q", before.Default, after.Default)
	}
	if failures := checkParameterCompatibility(tool, parameter, before, after); len(failures) != 0 {
		t.Fatalf("published false -> true transition failed: %v", failures)
	}
	for _, unchanged := range []parameterSchema{before, after} {
		if failures := checkParameterCompatibility(tool, parameter, unchanged, unchanged); len(failures) != 0 {
			t.Fatalf("unchanged published default failed: %v", failures)
		}
	}
	tests := []struct {
		name   string
		mutate func(*string, *string, *parameterSchema, *parameterSchema)
	}{
		{"other tool", func(tool, _ *string, _, _ *parameterSchema) { *tool = "chat/chat.shortcut_messages_send" }},
		{"other parameter", func(_, name *string, _, _ *parameterSchema) { *name = "enable-forward" }},
		{"reverse", func(_, _ *string, a, b *parameterSchema) { *a, *b = *b, *a }},
		{"quoted false", func(_, _ *string, a, _ *parameterSchema) { a.Default = `"false"` }},
		{"boolean true", func(_, _ *string, _, b *parameterSchema) { b.Default = "true" }},
		{"non-boolean type", func(_, _ *string, a, b *parameterSchema) { a.Type, b.Type = `"string"`, `"string"` }},
		{"wrong property", func(_, _ *string, a, b *parameterSchema) { a.Property, b.Property = "other", "other" }},
		{"inferred property", func(_, _ *string, a, b *parameterSchema) {
			a.PropertySource, b.PropertySource = "flag_name_inference", "flag_name_inference"
		}},
		{"required drift", func(_, _ *string, _, b *parameterSchema) { b.Required = true }},
		{"interface default drift", func(_, _ *string, _, b *parameterSchema) { b.InterfaceDefault = "true" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			toolPath, name, oldParameter, newParameter := tool, parameter, before, after
			test.mutate(&toolPath, &name, &oldParameter, &newParameter)
			assertSchemaFailureContains(t, checkParameterCompatibility(toolPath, name, oldParameter, newParameter), "changed default")
		})
	}
}

func publishedA2UIForwardParameter(t *testing.T, defaultValue string) parameterSchema {
	t.Helper()
	root := app.NewRootCommand()
	command, _, err := root.Find([]string{"chat", "message", "send-a2ui-card"})
	if err != nil {
		t.Fatal(err)
	}
	flag := command.Flags().Lookup("support-forward")
	if flag == nil {
		t.Fatal("send-a2ui-card has no support-forward flag")
	}
	flag.DefValue = defaultValue
	if err := flag.Value.Set(defaultValue); err != nil {
		t.Fatal(err)
	}
	effective, err := cli.BuildEffectiveCommandRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	const canonical = "chat.create_and_send_a2ui_card"
	spec, ok := effective.ByCanonical[canonical]
	if !ok {
		t.Fatal("A2UI send command is absent from the effective registry")
	}
	bound, err := cli.BindEffectiveCommandRegistry(root, cli.EffectiveCommandRegistry{Commands: []cli.CommandSpec{spec}})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := cli.AssembleSchemaRegistryFromBound(bound)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := registry.ToSnapshotPayload()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(payload.Tools[canonical])
	if err != nil {
		t.Fatal(err)
	}
	var tool struct {
		Parameters map[string]json.RawMessage `json:"parameters"`
	}
	if err := json.Unmarshal(raw, &tool); err != nil {
		t.Fatal(err)
	}
	parameter, err := normalizeParameter(tool.Parameters["support-forward"])
	if err != nil {
		t.Fatal(err)
	}
	return parameter
}
