// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
)

func TestMinutesFixedScopeLeavesBindToDistinctRegistryTools(t *testing.T) {
	root := NewRootCommand()
	effective, err := cli.BuildEffectiveCommandRegistry(root)
	if err != nil {
		t.Fatalf("build EffectiveCommandRegistry: %v", err)
	}
	bound, err := cli.BindEffectiveCommandRegistry(root, effective)
	if err != nil {
		t.Fatalf("bind EffectiveCommandRegistry: %v", err)
	}

	want := map[string]string{
		"minutes list all":    "minutes.list_accessible_minutes",
		"minutes list mine":   "minutes.list_by_keyword_and_time_range",
		"minutes list shared": "minutes.list_shared_minutes",
	}
	commands := map[any]bool{}
	canonicals := map[string]bool{}
	for path, canonical := range want {
		command, ok := bound.ByCLIPath[path]
		if !ok {
			t.Errorf("bound registry path %q is missing", path)
			continue
		}
		if command.CanonicalPath != canonical {
			t.Errorf("bound registry path %q canonical = %q, want %q", path, command.CanonicalPath, canonical)
		}
		if command.PrimaryCLIPath != path || len(command.Aliases) != 0 || len(command.AliasCommands) != 0 {
			t.Errorf("bound registry path %q retained alias navigation: primary=%q aliases=%v bound_aliases=%v", path, command.PrimaryCLIPath, command.Aliases, command.AliasCommands)
		}
		commands[command.PrimaryCommand] = true
		canonicals[command.CanonicalPath] = true
	}
	if len(commands) != len(want) || len(canonicals) != len(want) {
		t.Fatalf("minutes fixed-scope leaves collapsed: command pointers=%d canonicals=%d, want %d each", len(commands), len(canonicals), len(want))
	}
}
