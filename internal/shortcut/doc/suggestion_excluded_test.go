package doc

import (
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func TestCrossPlatformCoverageDocSuggestionExcludedShortcuts(t *testing.T) {
	for _, declaration := range []shortcut.Shortcut{Create, CheckpointUpdate, Import} {
		for _, flag := range []string{"--suggestion", "--sg"} {
			t.Run(declaration.Command+flag, func(t *testing.T) {
				caller := &docCoverageCaller{}
				err := runDocCoverage(t, declaration, caller, flag)
				if err == nil || !strings.Contains(err.Error(), "unknown flag") {
					t.Fatalf("error = %v, want unknown flag", err)
				}
				if caller.calls != 0 {
					t.Fatalf("excluded shortcut made %d RPC calls", caller.calls)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageDocSuggestionWholeUpdateRejected(t *testing.T) {
	for _, action := range []string{"append", "overwrite", "block_delete"} {
		caller := &docCoverageCaller{}
		if err := runDocCoverage(t, Update, caller, "--node", "n", "--command", action, "--content", "x", "--suggestion", "--yes"); err == nil || caller.calls != 0 {
			t.Fatalf("whole/body-only suggestion escaped: %v %#v", err, caller.history)
		}
	}
}
