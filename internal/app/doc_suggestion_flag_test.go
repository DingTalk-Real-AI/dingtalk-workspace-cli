package app

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageDocSuggestionRejectsRemovedAliasAfterPreParse(t *testing.T) {
	for _, path := range []string{"doc read", "doc block insert", "doc block update", "doc block delete"} {
		for _, flag := range []string{"--sg", "--sg=false"} {
			t.Run(path+flag, func(t *testing.T) {
				t.Setenv("DWS_CONFIG_DIR", t.TempDir())
				t.Setenv("DO_NOT_TRACK", "1")
				helpers.InitDepsForTest(t, helpers.GetCaller())
				runner := &countingErrorRunner{}
				testseam.Swap(t, &rootNewCommandRunnerWithFlags, func(*GlobalFlags) executor.Runner {
					return runner
				})
				root := NewRootCommand()
				var output bytes.Buffer
				root.SetOut(&output)
				root.SetErr(&output)
				args := append(strings.Fields(path), flag)
				root.SetArgs(args)
				_, err := pipeline.RunPreParseArgs(root, newPipelineEngine(), args)
				if err == nil {
					err = root.Execute()
				}
				var classified *apperrors.Error
				if !errors.As(err, &classified) || classified.Category != apperrors.CategoryValidation || classified.Reason != "unknown_flag" || apperrors.ExitCode(err) != 3 {
					t.Errorf("error = %#v, want unknown_flag validation with exit code 3", err)
				}
				if err == nil || !strings.Contains(err.Error(), "--sg") || runner.calls.Load() != 0 {
					t.Fatalf("error=%v runner calls=%d, want removed flag rejected before RPC", err, runner.calls.Load())
				}
			})
		}
	}
}

func TestCrossPlatformCoverageDocSuggestionHiddenInRootHelp(t *testing.T) {
	for _, path := range []string{"doc read", "doc block insert", "doc block update", "doc block delete", "doc +update"} {
		t.Run(path, func(t *testing.T) {
			t.Setenv("DWS_CONFIG_DIR", t.TempDir())
			t.Setenv("DO_NOT_TRACK", "1")
			helpers.InitDepsForTest(t, helpers.GetCaller())
			runner := &countingErrorRunner{}
			testseam.Swap(t, &rootNewCommandRunnerWithFlags, func(*GlobalFlags) executor.Runner { return runner })
			root := NewRootCommand()
			var output bytes.Buffer
			root.SetOut(&output)
			root.SetErr(&output)
			args := append(strings.Fields(path), "--help")
			root.SetArgs(args)
			if _, err := pipeline.RunPreParseArgs(root, newPipelineEngine(), args); err != nil {
				t.Fatal(err)
			}
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(output.String(), "--suggestion") || strings.Contains(output.String(), "--sg") || strings.Contains(output.String(), "建议模式") || runner.calls.Load() != 0 {
				t.Fatalf("help exposes rollout selector or calls RPC: calls=%d output=%s", runner.calls.Load(), output.String())
			}
		})
	}
}
