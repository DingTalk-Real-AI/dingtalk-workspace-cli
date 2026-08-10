package ding

import (
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func TestCrossPlatformCoverageDingShortcutsUseUnifiedDingIDFlag(t *testing.T) {
	for _, spec := range []struct {
		name    string
		command string
		flags   []shortcut.Flag
	}{
		{name: "receiver status", command: ReceiverStatus.Command, flags: ReceiverStatus.Flags},
		{name: "recall personal", command: RecallPersonal.Command, flags: RecallPersonal.Flags},
	} {
		if len(spec.flags) != 1 || spec.flags[0].Name != "ding-id" {
			t.Fatalf("%s %s flags = %v, want [ding-id]", spec.name, spec.command, spec.flags)
		}
		flag := spec.flags[0]
		if len(flag.Aliases) != 1 || flag.Aliases[0] != "id" {
			t.Fatalf("%s aliases = %v, want [id]", spec.command, flag.Aliases)
		}
	}
}
