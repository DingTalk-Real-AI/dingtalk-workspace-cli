// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package app

import (
	"bytes"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func TestUpgradeCommand_BlockedInEmbeddedMode(t *testing.T) {
	prev := edition.Get()
	edition.Override(&edition.Hooks{IsEmbedded: true, Name: "embedded"})
	t.Cleanup(func() { edition.Override(prev) })

	cases := []struct {
		name string
		args []string
	}{
		{"check", []string{"--check"}},
		{"list", []string{"--list"}},
		{"rollback", []string{"--rollback"}},
		{"plain", []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newUpgradeCommand()
			var out, errBuf bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&errBuf)
			cmd.SetArgs(tc.args)

			err := cmd.Execute()
			if err == nil {
				t.Fatalf("upgrade %v in embedded mode must return error, got nil", tc.args)
			}
			msg := err.Error()
			if !strings.Contains(msg, "嵌入模式") {
				t.Errorf("error message should mention 嵌入模式, got: %q", msg)
			}
			if !strings.Contains(msg, "embedded") {
				t.Errorf("error message should include edition name, got: %q", msg)
			}
			if !strings.Contains(msg, "dws upgrade") {
				t.Errorf("error message should reference dws upgrade for clarity, got: %q", msg)
			}
		})
	}
}

func TestUpgradeCommand_NotBlockedInOpenSourceMode(t *testing.T) {
	prev := edition.Get()
	edition.Override(&edition.Hooks{IsEmbedded: false, Name: "open"})
	t.Cleanup(func() { edition.Override(prev) })

	cmd := newUpgradeCommand()
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{"--check"})

	err := cmd.Execute()
	if err != nil && strings.Contains(err.Error(), "嵌入模式") {
		t.Errorf("open-source mode must not be blocked by embedded guard, got: %v", err)
	}
}
