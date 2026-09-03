// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
)

func TestRuntimeSchemaCompletenessCoversPublicCommandTree(t *testing.T) {
	exclusions, err := cli.ReviewedRuntimeSchemaExclusions()
	if err != nil {
		t.Fatal(err)
	}
	root := NewSchemaSourceRootCommand()
	if err := cli.ValidateRuntimeSchemaCompleteness(root); err != nil {
		t.Fatal(err)
	}
	report := cli.RuntimeSchemaCompleteness(root, exclusions)
	if len(report.Missing) > 0 || len(report.InvalidExclusions) > 0 || len(report.StaleExclusions) > 0 {
		t.Fatalf("runtime schema completeness: missing=%v invalid=%v stale=%v", report.Missing, report.InvalidExclusions, report.StaleExclusions)
	}
	if !containsSchemaPath(report.Covered, "chat category create-smart") {
		t.Fatal("chat category create-smart is not covered by runtime Schema")
	}
	for _, path := range missingChatCatalogCoveragePaths() {
		if !containsSchemaPath(report.Covered, path) {
			t.Fatalf("%s is not covered by runtime Schema", path)
		}
	}
	if !containsSchemaPath(report.Excluded, "agoal strategy list") {
		t.Fatal("agoal strategy list is not recorded as a reviewed exclusion")
	}
}

func TestRuntimeSchemaCompletenessDoesNotExcludeMissingChatCatalogPaths(t *testing.T) {
	exclusions, err := cli.ReviewedRuntimeSchemaExclusions()
	if err != nil {
		t.Fatal(err)
	}
	excluded := map[string]bool{}
	for _, exclusion := range exclusions {
		excluded[exclusion.CLIPath] = true
	}
	for _, path := range missingChatCatalogCoveragePaths() {
		if excluded[path] {
			t.Fatalf("%s must not remain in runtime Schema exclusions", path)
		}
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

func missingChatCatalogCoveragePaths() []string {
	return []string{
		"chat category add-conv",
		"chat category create",
		"chat category delete",
		"chat category remove-conv",
		"chat category rename",
		"chat chmod",
		"chat clear-all-red-point",
		"chat clear-messages",
		"chat clear-red-point",
		"chat data-auth cross-org",
		"chat emotion favorite",
		"chat emotion list",
		"chat emotion send",
		"chat group audit-join-validation",
		"chat group list-all",
		"chat group list-join-validations",
		"chat group members list-by-ids",
		"chat group notice create",
		"chat group notice edit",
		"chat group notice get",
		"chat group notice list",
		"chat group share-invite",
		"chat group update-alias",
		"chat hide",
		"chat list-all-conversations",
		"chat mark-read",
		"chat mark-unread",
		"chat message list-emotion-replies",
		"chat message set-top-msg",
		"chat message unset-top-msg",
		"chat mute-at-all",
		"chat mute-red-envelope",
		"chat text translate",
	}
}
