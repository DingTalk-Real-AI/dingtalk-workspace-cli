// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package app

import (
	"sort"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/spf13/cobra"
)

// TestSheetFinalSchemaConfirmationMatchesRuntimeGuards closes the delivery
// invariant from the final typed Schema to the executable Cobra leaf. The
// runtime marker can only be installed by the command-local wrapper that also
// executes the typed confirmation guard.
func TestSheetFinalSchemaConfirmationMatchesRuntimeGuards(t *testing.T) {
	snapshot := fullSchemaSnapshotForTest(t)
	root := NewRootCommand()

	schemaPaths := make(map[string]string)
	for canonical, tool := range snapshot.Tools {
		primaryPath := schemaContractString(tool["primary_cli_path"])
		if primaryPath == "" {
			primaryPath = schemaContractString(tool["cli_path"])
		}
		if !strings.HasPrefix(primaryPath, "sheet ") || schemaContractString(tool["confirmation"]) != "user_required" {
			continue
		}
		if previous := schemaPaths[primaryPath]; previous != "" {
			t.Fatalf("final Schema maps both %q and %q to Sheet path %q", previous, canonical, primaryPath)
		}
		schemaPaths[primaryPath] = canonical

		command := exactCommandForTest(root, primaryPath)
		if command == nil {
			t.Errorf("%s final Schema path %q has no executable Cobra leaf", canonical, primaryPath)
			continue
		}
		if !helpers.HasSheetMutationConfirmationGuard(command) {
			t.Errorf("%s (%s) declares confirmation=user_required but has no command-local runtime guard", canonical, primaryPath)
		}
	}
	if len(schemaPaths) == 0 {
		t.Fatal("final Schema contains no Sheet confirmation=user_required leaves")
	}

	guardedPaths := make(map[string]bool)
	rootPrefix := root.CommandPath() + " "
	var visit func(*cobra.Command)
	visit = func(command *cobra.Command) {
		if helpers.HasSheetMutationConfirmationGuard(command) {
			path := strings.TrimPrefix(command.CommandPath(), rootPrefix)
			guardedPaths[path] = true
		}
		for _, child := range command.Commands() {
			visit(child)
		}
	}
	visit(root)

	var missingGuards, undeclaredGuards []string
	for path, canonical := range schemaPaths {
		if !guardedPaths[path] {
			missingGuards = append(missingGuards, canonical+" ("+path+")")
		}
	}
	for path := range guardedPaths {
		if schemaPaths[path] == "" {
			undeclaredGuards = append(undeclaredGuards, path)
		}
	}
	if len(missingGuards) != 0 || len(undeclaredGuards) != 0 {
		sort.Strings(missingGuards)
		sort.Strings(undeclaredGuards)
		t.Fatalf("final Sheet Schema confirmation set differs from command-local runtime guards: missing=%v undeclared=%v", missingGuards, undeclaredGuards)
	}
}
