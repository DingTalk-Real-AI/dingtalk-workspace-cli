package helpers

import (
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/spf13/cobra"
)

type coverageNameMismatchHandler struct{}

func (coverageNameMismatchHandler) Name() string { return "got" }

func (coverageNameMismatchHandler) Command(executor.Runner) *cobra.Command {
	return &cobra.Command{Use: "got"}
}

func TestCrossPlatformCoverageBuildCommandsNameMismatchPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected factory name mismatch panic")
		}
	}()
	buildCommands([]registeredFactory{{
		name:    "want",
		factory: func() Handler { return coverageNameMismatchHandler{} },
	}}, nil)
}

func TestCrossPlatformCoverageLoadContractRuntimeRejectsWrongType(t *testing.T) {
	cmd := &cobra.Command{Use: "wrong-type"}
	contractRuntimeByCmd.Store(cmd, "not-a-weak-pointer")
	if _, ok := loadContractRuntime(cmd); ok {
		t.Fatal("wrong stored type accepted")
	}
}
