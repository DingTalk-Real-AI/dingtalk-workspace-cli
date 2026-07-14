// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
)

// Keep the folder-list wrapper and the free-form KQL search command as two
// independent Agent contracts. The list command translates --folder-id into a
// query before calling the same RPC, so treating it as a search alias loses a
// real executable parameter surface.
func TestMailListAndSearchRemainDistinctRegistryCommands(t *testing.T) {
	root := NewRootCommand()
	effective, err := cli.BuildEffectiveCommandRegistry(root)
	if err != nil {
		t.Fatalf("BuildEffectiveCommandRegistry() error = %v", err)
	}

	list, listOK := effective.ByCanonical["mail.list_emails"]
	search, searchOK := effective.ByCanonical["mail.search_emails"]
	if !listOK || !searchOK {
		t.Fatalf("mail registry split missing: list=%t search=%t", listOK, searchOK)
	}
	if list.PrimaryCLIPath != "mail message list" || search.PrimaryCLIPath != "mail message search" {
		t.Fatalf("mail registry paths: list=%q search=%q", list.PrimaryCLIPath, search.PrimaryCLIPath)
	}
	if len(list.Aliases) != 0 || len(search.Aliases) != 0 {
		t.Fatalf("mail list/search must not alias each other: list=%v search=%v", list.Aliases, search.Aliases)
	}

	bound, err := cli.BindEffectiveCommandRegistry(root, effective)
	if err != nil {
		t.Fatalf("BindEffectiveCommandRegistry() error = %v", err)
	}
	listCommand := bound.ByCanonical[list.CanonicalPath].PrimaryCommand
	searchCommand := bound.ByCanonical[search.CanonicalPath].PrimaryCommand
	if listCommand == nil || searchCommand == nil || listCommand == searchCommand {
		t.Fatalf("mail list/search Cobra bindings are not distinct: list=%p search=%p", listCommand, searchCommand)
	}
	if listCommand.Flags().Lookup("folder-id") == nil || listCommand.Flags().Lookup("query") != nil {
		t.Fatal("mail list Cobra surface must expose --folder-id and not --query")
	}
	if searchCommand.Flags().Lookup("query") == nil || searchCommand.Flags().Lookup("folder-id") != nil {
		t.Fatal("mail search Cobra surface must expose --query and not --folder-id")
	}
}
