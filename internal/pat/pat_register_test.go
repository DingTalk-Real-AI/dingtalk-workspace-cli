// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package pat

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageRegisterCommandsMergesExistingPATShortcutGroup(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	shortcutGroup := &cobra.Command{Use: "pat", Short: "pat shortcuts"}
	shortcutGroup.AddCommand(&cobra.Command{Use: "+fixture"})
	root.AddCommand(&cobra.Command{Use: "aaa-other"})
	root.AddCommand(shortcutGroup)

	RegisterCommands(root, &fakeToolCaller{})

	var patGroups []*cobra.Command
	for _, command := range root.Commands() {
		if command.Name() == "pat" {
			patGroups = append(patGroups, command)
		}
	}
	if len(patGroups) != 1 {
		t.Fatalf("top-level PAT groups = %d, want 1", len(patGroups))
	}
	for _, child := range []string{"+fixture", "chmod", "browser-policy"} {
		if found, _, err := patGroups[0].Find([]string{child}); err != nil || found == nil || found.Name() != child {
			t.Errorf("merged PAT child %q missing: found=%v err=%v", child, found, err)
		}
	}
	if patGroups[0].Short != "行为授权管理" {
		t.Fatalf("native PAT group metadata was not preserved: %q", patGroups[0].Short)
	}
}
