// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package app

import (
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func TestCrossPlatformCoverageChatAtomicOwnersResolveToPublicShortcuts(t *testing.T) {
	root := NewRootCommand()
	owners := shortcut.PreferredShortcutOwnersSnapshot()
	if len(owners) < 80 {
		t.Fatalf("reviewed Chat atomic owners = %d, want at least 80", len(owners))
	}
	for atomicPath, ownerPath := range owners {
		atomic, remaining, err := root.Find(strings.Fields(atomicPath))
		if err != nil || atomic == nil || len(remaining) != 0 || !atomic.Runnable() {
			t.Errorf("atomic path %q is not runnable: command=%v remaining=%v err=%v", atomicPath, atomic, remaining, err)
			continue
		}
		if got := atomic.Annotations[preferredShortcutCLIPathAnnotation]; got != ownerPath {
			t.Errorf("atomic path %q preferred Shortcut = %q, want %q", atomicPath, got, ownerPath)
		}
		owner, remaining, err := root.Find(strings.Fields(ownerPath))
		if err != nil || owner == nil || len(remaining) != 0 || owner.Hidden || !owner.Runnable() {
			t.Errorf("preferred Shortcut %q is not public runnable: command=%v remaining=%v err=%v", ownerPath, owner, remaining, err)
		}
	}
}
