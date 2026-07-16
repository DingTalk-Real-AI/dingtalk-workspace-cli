// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
)

func TestRuntimeSchemaCompletenessCoversPublicCommandTree(t *testing.T) {
	exclusions, err := cli.EmbeddedRuntimeSchemaExclusions()
	if err != nil {
		t.Fatal(err)
	}
	root := NewRootCommand()
	if err := cli.ValidateEmbeddedRuntimeSchemaCompleteness(root); err != nil {
		t.Fatal(err)
	}
	report := cli.RuntimeSchemaCompleteness(root, exclusions)
	if len(report.Missing) > 0 || len(report.InvalidExclusions) > 0 || len(report.StaleExclusions) > 0 {
		t.Fatalf("runtime schema completeness: missing=%v invalid=%v stale=%v", report.Missing, report.InvalidExclusions, report.StaleExclusions)
	}
	if !containsSchemaPath(report.Covered, "chat category create-smart") {
		t.Fatal("chat category create-smart is not covered by runtime Schema")
	}
	if !containsSchemaPath(report.Excluded, "agoal strategy list") {
		t.Fatal("agoal strategy list is not recorded as a reviewed exclusion")
	}
}

func containsSchemaPath(paths []string, want string) bool {
	for _, path := range paths {
		if path == want {
			return true
		}
	}
	return false
}
