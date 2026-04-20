package pat

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestPatHelpMentionsCLAWTypeAndLegacyChannelCompatibility(t *testing.T) {
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
		"host-control",
		"rewind-desktop",
		"dws-wukong",
		"wukong",
		"DWS_CHANNEL='...;host-control'",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("pat help missing %q\n%s", want, help)
		}
	}
}

func TestPatCallbackHelpMentionsStableCommandsAndSelector(t *testing.T) {
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
		"list-super-admins",
		"send-apply",
		"poll-flow",
		"DWS_CHANNEL='...;host-control'",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("callback help missing %q\n%s", want, help)
		}
	}
}
