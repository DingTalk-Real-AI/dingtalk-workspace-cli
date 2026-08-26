// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package devapp

import (
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func TestDevAppShortcutsRollOutPerTerminalCommand(t *testing.T) {
	active := map[string]bool{
		"+list": true, "+get": true, "+create": true, "+update": true,
		"+delete": true, "+enable": true, "+disable": true, "+webapp-get": true,
		"+webapp-config":   true,
		"+permission-list": true, "+version-list": true,
		"+robot-get": true, "+version-get": true,
		"+version-check-approval": true, "+version-status": true,
		"+credentials-get": true, "+member-list": true,
		"+member-add": true, "+member-remove": true,
		"+robot-config": true, "+robot-enable": true, "+robot-disable": true,
		"+event-list": true, "+event-subscribe": true, "+version-create": true,
	}
	seen := map[string]bool{}
	for _, item := range shortcut.All() {
		if item.Service != productDevApp {
			continue
		}
		seen[item.Command] = true
		want := output.RolloutDualValidate
		if active[item.Command] {
			want = output.RolloutUnifiedActive
		}
		if item.OutputRollout != want {
			t.Errorf("%s rollout=%s, want %s", item.Command, item.OutputRollout, want)
		}
		if want == output.RolloutUnifiedActive && item.Contract.Empty() {
			t.Errorf("%s is unified_active without a reviewed Contract", item.Command)
		}
		if want == output.RolloutUnifiedActive && item.Contract.Result == nil {
			t.Errorf("%s is unified_active without a reviewed result contract", item.Command)
		}
	}
	for name := range active {
		if !seen[name] {
			t.Errorf("active pilot shortcut %s is not registered", name)
		}
	}
	for _, paginated := range []string{"+list", "+permission-list", "+version-list"} {
		if !seen[paginated] {
			t.Errorf("paginated shortcut %s is not registered", paginated)
		}
		for _, item := range shortcut.All() {
			if item.Service == productDevApp && item.Command == paginated && item.Contract.Pagination == nil {
				t.Errorf("paginated shortcut %s has no standalone pagination contract", paginated)
			}
		}
	}
}
