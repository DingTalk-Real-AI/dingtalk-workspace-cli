package helpers

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageAISearchRemainingFallbackBranches(t *testing.T) {
	if got := flagValue(&cobra.Command{Use: "empty"}, "missing"); got != "" {
		t.Fatalf("missing flag=%q", got)
	}
	if got := parseDimensions(""); len(got) != 1 || got[0] != "all" {
		t.Fatalf("empty dimensions=%v", got)
	}
	if got := parseDimensions(" , "); len(got) != 1 || got[0] != "all" {
		t.Fatalf("blank dimensions=%v", got)
	}

	installScriptedCaller(t, &scriptedToolCaller{dry: true})
	for _, args := range [][]string{
		{"--query", "alice"},
		{"enterprise", "--types", ","},
		{"behavior", "--types", ","},
	} {
		if err := executeFilterCoverage(t, newAisearchCommand(), args...); err != nil {
			t.Fatalf("args=%v err=%v", args, err)
		}
	}
}

func TestCrossPlatformCoverageAisearchPersonAcceptsRedundantTypeSelector(t *testing.T) {
	installScriptedCaller(t, &scriptedToolCaller{dry: true})
	if err := executeFilterCoverage(t, newAisearchCommand(), "search", "--query", "张三", "--type", "person"); err != nil {
		t.Fatal(err)
	}
	if err := executeFilterCoverage(t, newAisearchCommand(), "search", "--query", "张三", "--type", "document"); err == nil {
		t.Fatal("invalid person type selector unexpectedly succeeded")
	}
}
