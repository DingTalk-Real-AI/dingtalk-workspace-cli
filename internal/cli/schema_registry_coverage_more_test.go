// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageCommandRegistryLoaderErrorWrappers(t *testing.T) {
	previousRegistry := loadReviewedCommandRegistry
	previousBindings := validateReviewedParameterBindings
	previousManual := loadReviewedManualSchemaHints
	t.Cleanup(func() {
		loadReviewedCommandRegistry = previousRegistry
		validateReviewedParameterBindings = previousBindings
		loadReviewedManualSchemaHints = previousManual
	})

	loadReviewedCommandRegistry = func() (CommandRegistry, error) { return CommandRegistry{}, errors.New("registry failed") }
	if _, err := ValidateCommandRegistrySource(embeddedSchemaCommandRegistryJSON); err == nil || !strings.Contains(err.Error(), "registry failed") {
		t.Fatalf("ValidateCommandRegistrySource() error = %v", err)
	}
	if _, err := EmbeddedCommandRegistrySourceHash(); err == nil || !strings.Contains(err.Error(), "registry failed") {
		t.Fatalf("EmbeddedCommandRegistrySourceHash() error = %v", err)
	}
	if _, err := BuildEffectiveCommandRegistry(&cobra.Command{Use: "dws"}); err == nil || !strings.Contains(err.Error(), "registry failed") {
		t.Fatalf("BuildEffectiveCommandRegistry() registry error = %v", err)
	}

	loadReviewedCommandRegistry = previousRegistry
	validateReviewedParameterBindings = func() error { return errors.New("bindings failed") }
	if _, err := BuildEffectiveCommandRegistry(&cobra.Command{Use: "dws"}); err == nil || !strings.Contains(err.Error(), "bindings failed") {
		t.Fatalf("BuildEffectiveCommandRegistry() bindings error = %v", err)
	}

	validateReviewedParameterBindings = previousBindings
	loadReviewedManualSchemaHints = func() (ManualSchemaHintSnapshot, error) {
		return ManualSchemaHintSnapshot{}, errors.New("manual failed")
	}
	if _, err := BuildEffectiveCommandRegistry(&cobra.Command{Use: "dws"}); err == nil || !strings.Contains(err.Error(), "manual failed") {
		t.Fatalf("BuildEffectiveCommandRegistry() manual error = %v", err)
	}
}

func TestCrossPlatformCoverageCommandRegistryDecodeAndIndexRemainingEdges(t *testing.T) {
	base := func(products string) string {
		return `{"$schema":"./schema_command_registry.schema.json","version":1,"products":` + products + `}`
	}
	validTool := `{"canonical_path":"sample.get","cli_path":"sample get"}`
	for _, data := range []string{
		base(`[{"id":"sample","tools":[`+validTool+`]}]`) + ` {}`,
		`{"$schema":"./schema_command_registry.schema.json","version":2,"products":[]}`,
		`{"$schema":"wrong","version":1,"products":[]}`,
		base(`[]`),
		base(`[{"id":" sample","tools":[` + validTool + `]}]`),
		base(`[{"id":"sample","tools":[` + validTool + `]},{"id":"sample","tools":[` + validTool + `]}]`),
		base(`[{"id":"sample","tools":[]}]`),
		base(`[{"id":"sample","tools":[{"canonical_path":"other.get","cli_path":"sample get"}]}]`),
		base(`[{"id":"sample","tools":[{"canonical_path":"sample.get","source_product_id":" bad","cli_path":"sample get"}]}]`),
		base(`[{"id":"sample","tools":[{"canonical_path":"sample.get","source_product_id":"","cli_path":"sample get"}]}]`),
		base(`[{"id":"sample","tools":[{"canonical_path":"sample.get","cli_path":"bad *"}]}]`),
		base(`[{"id":"sample","tools":[{"canonical_path":"sample.get","cli_path":"sample get","aliases":["bad *"]}]}]`),
		base(`[{"id":"sample","tools":[{"canonical_path":"sample.get","cli_path":"sample get","aliases":["sample get"]}]}]`),
		base(`[{"id":"sample","tools":[{"canonical_path":"sample.get","cli_path":"sample get","aliases":["sample read","sample read"]}]}]`),
		base(`[{"id":"sample","tools":[{"canonical_path":"sample.get","cli_path":"sample get","visibility":"other"}]}]`),
	} {
		_, err := decodeCommandRegistry([]byte(data))
		if err == nil {
			t.Fatalf("invalid registry succeeded: %s", data)
		}
	}

	commands := []CommandSpec{{CanonicalPath: "sample.get", PrimaryCLIPath: "sample get"}}
	for _, candidate := range []CommandSpec{
		{CanonicalPath: "invalid", PrimaryCLIPath: "sample get"},
		{CanonicalPath: "sample.get", SourceProductID: "bad!", PrimaryCLIPath: "sample get"},
		{CanonicalPath: "sample.get", PrimaryCLIPath: "bad *"},
		{CanonicalPath: "sample.get", PrimaryCLIPath: "sample get", Aliases: []string{"bad *"}},
		{CanonicalPath: "sample.get", PrimaryCLIPath: "sample get", Visibility: "other"},
	} {
		if _, _, _, err := indexCommandSpecs([]CommandSpec{candidate}); err == nil {
			t.Fatalf("invalid indexed command succeeded: %#v", candidate)
		}
	}
	if _, _, _, err := indexCommandSpecs(append(commands, commands[0])); err == nil {
		t.Fatal("duplicate canonical command succeeded")
	}
	if _, _, _, err := indexCommandSpecs([]CommandSpec{
		commands[0], {CanonicalPath: "sample.read", PrimaryCLIPath: "sample read", Aliases: []string{"sample get"}},
	}); err == nil {
		t.Fatal("duplicate CLI path succeeded")
	}
	if _, _, _, err := indexCommandSpecs([]CommandSpec{{CanonicalPath: "sample.get", PrimaryCLIPath: "sample.get"}}); err == nil {
		t.Fatal("CLI path conflicting with canonical identity succeeded")
	}

	left, err := newCommandRegistry(commands)
	if err != nil {
		t.Fatal(err)
	}
	right := cloneCommandRegistry(left)
	right.ByCanonical["sample.get"] = CommandSpec{CanonicalPath: "sample.get", PrimaryCLIPath: "different"}
	if equalCommandRegistries(left, right) {
		t.Fatal("different registry details compared equal")
	}
	_ = hashCommandSpecs([]CommandSpec{{CanonicalPath: "sample.get", PrimaryCLIPath: "sample get"}})
}

func TestCrossPlatformCoverageEffectiveCommandRegistryRemainingErrors(t *testing.T) {
	if _, err := BuildEffectiveCommandRegistry(nil); err == nil {
		t.Fatal("nil effective root succeeded")
	}
	if _, err := buildEffectiveCommandRegistry(nil, CommandRegistry{}, ManualSchemaHintSnapshot{Version: manualSchemaHintVersion}); err == nil {
		t.Fatal("nil internal effective root succeeded")
	}
	root := &cobra.Command{Use: "dws"}
	sample := &cobra.Command{Use: "sample"}
	get := &cobra.Command{Use: "get", Aliases: []string{"read"}, Run: func(*cobra.Command, []string) {}}
	group := &cobra.Command{Use: "group"}
	sample.AddCommand(get, group)
	root.AddCommand(sample)
	if _, err := buildEffectiveCommandRegistry(root, CommandRegistry{}, ManualSchemaHintSnapshot{}); err == nil {
		t.Fatal("invalid manual hint version succeeded")
	}
	if _, err := buildEffectiveCommandRegistry(root, CommandRegistry{Commands: []CommandSpec{{CanonicalPath: "invalid", PrimaryCLIPath: "bad"}}}, ManualSchemaHintSnapshot{Version: manualSchemaHintVersion}); err == nil {
		t.Fatal("invalid reviewed registry succeeded")
	}

	entry := func(path, canonical string) ManualSchemaCommandHint {
		return ManualSchemaCommandHint{CLIPath: path, CanonicalPath: canonical, Reviewed: true, Reason: "fixture"}
	}
	for _, commands := range [][]ManualSchemaCommandHint{
		{entry("bad *", "sample.get")},
		{entry("sample get", "sample.get"), entry("sample get", "sample.get")},
		{{CLIPath: "sample get", CanonicalPath: "sample.get", Reviewed: false}},
		{entry("sample get", "invalid")},
		{entry("sample missing", "sample.missing")},
		{entry("sample group", "sample.group")},
	} {
		if _, err := buildEffectiveCommandRegistry(root, CommandRegistry{}, ManualSchemaHintSnapshot{Version: manualSchemaHintVersion, Commands: commands}); err == nil {
			t.Fatalf("invalid manual commands succeeded: %#v", commands)
		}
	}

	base, err := newCommandRegistry([]CommandSpec{{CanonicalPath: "sample.get", PrimaryCLIPath: "sample get"}})
	if err != nil {
		t.Fatal(err)
	}
	if effective, err := buildEffectiveCommandRegistry(root, base, ManualSchemaHintSnapshot{Version: manualSchemaHintVersion, Commands: []ManualSchemaCommandHint{entry("sample get", "sample.get")}}); err != nil || len(effective.Commands) != 1 {
		t.Fatalf("existing manual command = %#v, %v", effective, err)
	}
	if _, err := buildEffectiveCommandRegistry(root, base, ManualSchemaHintSnapshot{Version: manualSchemaHintVersion, Commands: []ManualSchemaCommandHint{entry("sample read", "sample.other")}}); err == nil {
		t.Fatal("Cobra alias canonical conflict succeeded")
	}
	if _, err := buildEffectiveCommandRegistry(root, CommandRegistry{}, ManualSchemaHintSnapshot{Version: manualSchemaHintVersion, Commands: []ManualSchemaCommandHint{entry("sample read", "sample.get")}}); err == nil || !strings.Contains(err.Error(), "real command path") {
		t.Fatalf("unregistered Cobra alias error = %v", err)
	}
	if _, err := buildEffectiveCommandRegistry(root, base, ManualSchemaHintSnapshot{Version: manualSchemaHintVersion, Commands: []ManualSchemaCommandHint{entry("sample read", "sample.get")}}); err == nil || !strings.Contains(err.Error(), "cannot create an alias") {
		t.Fatalf("matching Cobra alias error = %v", err)
	}
	if clone := cloneCommandRegistry(CommandRegistry{Commands: []CommandSpec{{CanonicalPath: "invalid"}}}); len(clone.Commands) != 0 {
		t.Fatalf("invalid cloned registry = %#v", clone)
	}
}
