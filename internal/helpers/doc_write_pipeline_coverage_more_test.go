package helpers

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
)

func docWriteCoverageCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "write"}
	cmd.Flags().String("content", "", "")
	cmd.Flags().String("content-file", "", "")
	cmd.Flags().String("content-path", "", "")
	cmd.Flags().String("markdown", "", "")
	return cmd
}

func TestCrossPlatformCoverageDocWritePipelineStrategyRemainingCoverage(t *testing.T) {
	oldArgs := os.Args
	os.Args = []string{"dws", "doc"}
	t.Cleanup(func() { os.Args = oldArgs })
	contentStdinCmd := docWriteCoverageCommand()
	_ = contentStdinCmd.Flags().Set("content", "-")
	if got := detectContentSource(contentStdinCmd); got != sourceStdin {
		t.Fatalf("content stdin source = %v", got)
	}
	stdinCmd := docWriteCoverageCommand()
	_ = stdinCmd.Flags().Set("markdown", "-")
	if got := detectContentSource(stdinCmd); got != sourceStdin {
		t.Fatalf("markdown stdin source = %v", got)
	}

	longLiteral := strings.Repeat("x", longContentWarningThreshold+1)
	installScriptedCaller(t, &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"nodeId":"node"}`}}})
	if err := docWritePipeline(docWriteCoverageCommand(), "create_document", map[string]any{"markdown": longLiteral}, longLiteral, "create"); err != nil {
		t.Fatalf("long literal single write: %v", err)
	}

	fallback := strings.Repeat("x", 5100)
	timeoutCaller := &scriptedToolCaller{steps: []scriptedToolStep{{err: errors.New("timeout")}, {text: `{}`}}}
	installScriptedCaller(t, timeoutCaller)
	err := docWritePipeline(docWriteCoverageCommand(), "update_document", map[string]any{"nodeId": "node", "mode": "overwrite", "markdown": fallback}, fallback, "update")
	if err == nil || !strings.Contains(err.Error(), "提交状态未知") || timeoutCaller.calls != 1 {
		t.Fatalf("single timeout must stop without replay: err=%v calls=%d", err, timeoutCaller.calls)
	}

	chunked := strings.Repeat("x", initialChunkSize+100)
	installScriptedCaller(t, &scriptedToolCaller{steps: []scriptedToolStep{{text: `{}`}, {text: `{}`}}})
	if err := docWritePipeline(docWriteCoverageCommand(), "update_document", map[string]any{"nodeId": "node", "markdown": chunked}, chunked, "update"); err != nil {
		t.Fatalf("long content chunking: %v", err)
	}
}

func TestCrossPlatformCoverageChunkedWriteAdaptiveRetryRemainingCoverage(t *testing.T) {
	oldArgs := os.Args
	os.Args = []string{"dws", "doc"}
	t.Cleanup(func() { os.Args = oldArgs })
	markdown := strings.Repeat("x", 24000)
	caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{}`}, {err: errors.New("HSFTimeoutException")}, {text: `{}`}}}
	installScriptedCaller(t, caller)
	_, written, _, err := chunkedWrite(context.Background(), "update_document", map[string]any{"nodeId": "node"}, markdown, "update", 10000)
	if err == nil || written != 1 || caller.calls != 2 || !strings.Contains(err.Error(), "提交状态未知") {
		t.Fatalf("timeout must stop without replay: written=%d calls=%d err=%v", written, caller.calls, err)
	}
}

func TestCrossPlatformCoverageDocWriteFirstChunkTimeoutIsUnknown(t *testing.T) {
	oldArgs := os.Args
	os.Args = []string{"dws", "doc"}
	t.Cleanup(func() { os.Args = oldArgs })
	markdown := strings.Repeat("x", 24000)
	tests := []struct {
		name     string
		tool     string
		args     map[string]any
		wantNode string
	}{
		{name: "create", tool: "create_document", args: map[string]any{"name": "doc"}},
		{name: "update", tool: "update_document", args: map[string]any{"nodeId": "node", "mode": "overwrite"}, wantNode: "node"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			caller := &scriptedToolCaller{steps: []scriptedToolStep{{err: errors.New("HSFTimeoutException")}, {text: `{}`}}}
			installScriptedCaller(t, caller)
			nodeID, written, _, err := chunkedWrite(context.Background(), tc.tool, tc.args, markdown, tc.name, 10000)
			var typed *apperrors.Error
			if err == nil || !errors.As(err, &typed) {
				t.Fatalf("error = %#v", err)
			}
			if nodeID != tc.wantNode || written != 0 || caller.calls != 1 {
				t.Fatalf("node=%q written=%d calls=%d", nodeID, written, caller.calls)
			}
			if typed.Reason != "doc_write_commit_unknown" || typed.FailureStage != "chunk_1" || typed.ExecutionStarted == nil || !*typed.ExecutionStarted || !typed.RetryableSet || typed.Retryable {
				t.Fatalf("unknown commit metadata = %#v", typed)
			}
		})
	}
}
