package pat

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestPatHelpMentionsCLAWTypeAsOnlySelector(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	RegisterCommands(root, nil)

	cmd, _, err := root.Find([]string{"pat"})
	if err != nil {
		t.Fatalf("Find(pat) error = %v", err)
	}

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Help(); err != nil {
		t.Fatalf("Help() error = %v", err)
	}

	help := out.String()
	for _, want := range []string{
		"CLAW_TYPE",
		"只由 CLAW_TYPE 选择",
		"host-control",
		"rewind-desktop",
		"dws-wukong",
		"wukong",
		"DWS_CHANNEL",
		"不提供 PAT 宿主接管的任何回退路径",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("pat help missing %q\n%s", want, help)
		}
	}
	if strings.Contains(help, "DWS_CHANNEL='...;host-control'") {
		t.Fatalf("pat help should not mention legacy DWS_CHANNEL suffix\n%s", help)
	}
}

func TestPatCallbackHelpMentionsStableCommandsAndSelectorOnly(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	RegisterCommands(root, nil)

	cmd, _, err := root.Find([]string{"pat", "callback"})
	if err != nil {
		t.Fatalf("Find(pat callback) error = %v", err)
	}

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Help(); err != nil {
		t.Fatalf("Help() error = %v", err)
	}

	help := out.String()
	for _, want := range []string{
		"CLAW_TYPE",
		"只由 CLAW_TYPE 选择",
		"list-super-admins",
		"send-apply",
		"poll-flow",
		"DWS_CHANNEL",
		"不提供 PAT 宿主接管的任何回退路径",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("callback help missing %q\n%s", want, help)
		}
	}
	if strings.Contains(help, "DWS_CHANNEL='...;host-control'") {
		t.Fatalf("callback help should not mention legacy DWS_CHANNEL suffix\n%s", help)
	}
}
