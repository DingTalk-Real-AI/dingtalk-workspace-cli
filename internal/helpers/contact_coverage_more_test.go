package helpers

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageContactRemainingCompatibilityBranches(t *testing.T) {
	cmd := &cobra.Command{Use: "flags"}
	cmd.Flags().String("id", "", "")
	if got := contactFirstSetFlagName(cmd, "id"); got != "id" {
		t.Fatalf("default flag name=%q", got)
	}
	if got := contactFirstSetFlagName(cmd); got != "" {
		t.Fatalf("empty flag name=%q", got)
	}

	for _, args := range [][]string{
		{"user", "get", "--ids", "me"},
		{"get", "--ids", "self"},
	} {
		installScriptedCaller(t, &scriptedToolCaller{dry: true})
		if err := executeFilterCoverage(t, newContactCommand(), args...); err == nil || !strings.Contains(err.Error(), "真实的 userId") {
			t.Fatalf("args=%v placeholder err=%v", args, err)
		}
	}

	installScriptedCaller(t, &scriptedToolCaller{dry: true})
	if err := executeFilterCoverage(t, newContactCommand(), "get", "--dept", "1"); err != nil {
		t.Fatalf("compat dept get: %v", err)
	}

	root := newContactCommand()
	hint := findCoverageSubcommand(t, root, "department")
	if err := hint.RunE(hint, []string{"--help"}); err != nil {
		t.Fatalf("hint help: %v", err)
	}
	if err := hint.RunE(hint, []string{"unexpected"}); err == nil || !strings.Contains(err.Error(), "use: dws contact dept") {
		t.Fatalf("hint guidance err=%v", err)
	}

	if err := executeFilterCoverage(t, newContactCommand(), "user", "get", "--unknown"); err == nil || !strings.Contains(err.Error(), "See '") {
		t.Fatalf("flag help hint err=%v", err)
	}
}
