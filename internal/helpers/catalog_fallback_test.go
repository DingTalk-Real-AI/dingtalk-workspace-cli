// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0

package helpers

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/spf13/cobra"
)

func TestCatalogFallbackDryRunUsesFrozenInterfaceRef(t *testing.T) {
	root := &cobra.Command{Use: "dws", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	root.AddCommand(NewCatalogFallbackCommands(executor.EchoRunner{})...)

	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{
		"calendar", "book", "get",
		"--id", "catalog-fallback-test",
		"--dry-run", "--format", "json",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute fallback command: %v", err)
	}

	var invocation executor.Invocation
	if err := json.Unmarshal(stdout.Bytes(), &invocation); err != nil {
		t.Fatalf("decode dry-run payload: %v\n%s", err, stdout.String())
	}
	if invocation.Kind != "compat_invocation" || invocation.CanonicalProduct != "calendar" {
		t.Fatalf("unexpected invocation identity: %#v", invocation)
	}
	if invocation.Tool != "get_calendar" || invocation.CanonicalPath != "calendar.get_calendar" {
		t.Fatalf("unexpected interface ref: %#v", invocation)
	}
	if invocation.LegacyPath != "calendar book get" {
		t.Fatalf("legacy path = %q", invocation.LegacyPath)
	}
	if got := invocation.Params["calendarId"]; got != "catalog-fallback-test" {
		t.Fatalf("calendarId = %#v", got)
	}
}

func TestCatalogFallbackConstraintAcceptsVariadicPositional(t *testing.T) {
	cmd := &cobra.Command{Use: "chmod"}
	cmd.Flags().String("product", "", "")
	definition := cli.CatalogCommandDefinition{
		Positionals: []cli.RuntimeSchemaPositional{{
			Name: "scope", Type: "array", Variadic: true, Index: 0,
		}},
		Constraints: cli.RuntimeSchemaConstraints{
			RequireOneOf: [][]string{{"scope", "product"}},
		},
	}
	args := []string{"aitable.record:read", "chat.message:list"}
	if err := validateCatalogFallbackConstraints(cmd, args, definition); err != nil {
		t.Fatalf("validate mixed positional/flag constraint: %v", err)
	}
	params, err := collectCatalogFallbackParams(cmd, args, definition)
	if err != nil {
		t.Fatalf("collect positional params: %v", err)
	}
	scopes, ok := params["scope"].([]string)
	if !ok || len(scopes) != 2 {
		t.Fatalf("scope = %#v, want two variadic values", params["scope"])
	}
	if err := validateCatalogFallbackConstraints(cmd, nil, definition); err == nil {
		t.Fatal("missing positional and flag unexpectedly satisfied require_one_of")
	}
}
