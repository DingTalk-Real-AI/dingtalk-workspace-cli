// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package builtin_test

import (
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/builtin"
)

func TestBaseCommandsExposeSortedDistributionShortcuts(t *testing.T) {
	commands := builtin.BaseCommands()
	if len(commands) == 0 {
		t.Fatal("BaseCommands returned no distribution shortcuts")
	}
	for index, command := range commands {
		if index > 0 && commands[index-1].Name() > command.Name() {
			t.Fatalf("BaseCommands are not sorted: %q before %q", commands[index-1].Name(), command.Name())
		}
		children := command.Commands()
		if len(children) == 0 {
			t.Fatalf("base shortcut service %q has no commands", command.Name())
		}
		for _, child := range children {
			if !strings.HasPrefix(child.Name(), "+") {
				t.Fatalf("base shortcut %q under %q is not mounted as a +command", child.Name(), command.Name())
			}
		}
	}
}
