package app

import (
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageRootTimeoutRaisesFilePutBudget(t *testing.T) {
	t.Cleanup(func() { helpers.SetFilePutClientTimeout(helpers.DefaultFilePutTimeout) })

	probe := func() *cobra.Command {
		return &cobra.Command{Use: "put-timeout-probe", RunE: func(*cobra.Command, []string) error { return nil }}
	}
	raised := newRootCommandWithAssembly(nil, nil, func(root *cobra.Command) { root.AddCommand(probe()) })
	raised.SetArgs([]string{"--timeout", "3600", "put-timeout-probe"})
	if _, err := corecmd.ExecuteCForTest(raised); err != nil {
		t.Fatalf("ExecuteC: %v", err)
	}
	if got := helpers.FilePutClientTimeout(); got != time.Hour {
		t.Fatalf("file PUT budget after --timeout 3600 = %v, want 1h", got)
	}

	floored := newRootCommandWithAssembly(nil, nil, func(root *cobra.Command) { root.AddCommand(probe()) })
	floored.SetArgs([]string{"put-timeout-probe"})
	if _, err := corecmd.ExecuteCForTest(floored); err != nil {
		t.Fatalf("ExecuteC default: %v", err)
	}
	if got := helpers.FilePutClientTimeout(); got != helpers.DefaultFilePutTimeout {
		t.Fatalf("file PUT budget after default --timeout = %v, want %v", got, helpers.DefaultFilePutTimeout)
	}

	// Extreme --timeout values clamp at 24h instead of wrapping int64
	// nanoseconds during conversion.
	capped := newRootCommandWithAssembly(nil, nil, func(root *cobra.Command) { root.AddCommand(probe()) })
	capped.SetArgs([]string{"--timeout", "999999999999", "put-timeout-probe"})
	if _, err := corecmd.ExecuteCForTest(capped); err != nil {
		t.Fatalf("ExecuteC extreme: %v", err)
	}
	if got := helpers.FilePutClientTimeout(); got != 24*time.Hour {
		t.Fatalf("file PUT budget after extreme --timeout = %v, want 24h clamp", got)
	}
}
