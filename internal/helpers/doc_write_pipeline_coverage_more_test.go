package helpers

import (
	"bytes"
	"context"
	"encoding/json"
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

	chunked := strings.Repeat("x", DefaultMarkdownChunkRunes+100)
	installScriptedCaller(t, &scriptedToolCaller{steps: []scriptedToolStep{{text: `{}`}, {text: `{}`}}})
	if err := docWritePipeline(docWriteCoverageCommand(), "update_document", map[string]any{"nodeId": "node", "markdown": chunked}, chunked, "update"); err != nil {
		t.Fatalf("long content chunking: %v", err)
	}
}

func TestCrossPlatformCoverageDocWriteRejectsIndexWhenChunking(t *testing.T) {
	oldArgs := os.Args
	os.Args = []string{"dws", "doc"}
	t.Cleanup(func() { os.Args = oldArgs })

	// --index cannot be propagated across chunks: each chunk creates an unknown
	// number of blocks, so the insertion point for chunk 2 is unknowable. Before
	// this guard, chunkedWrite rebuilt the tool args with only nodeId/markdown/mode
	// and the flag was silently dropped.
	long := strings.Repeat("x", DefaultMarkdownChunkRunes+10)
	caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{}`}, {text: `{}`}}}
	installScriptedCaller(t, caller)
	err := docWritePipeline(docWriteCoverageCommand(), "update_document",
		map[string]any{"nodeId": "node", "mode": "append", "index": 3}, long, "update")
	var typed *apperrors.Error
	if err == nil || !errors.As(err, &typed) || typed.Reason != "doc_write_index_with_chunking" {
		t.Fatalf("expected a fail-closed validation error, got %#v", err)
	}
	if caller.calls != 0 {
		t.Fatalf("must reject before writing anything, calls=%d", caller.calls)
	}

	// The same args below the limit stay allowed, so the guard is scoped to
	// chunking rather than banning --index outright.
	installScriptedCaller(t, &scriptedToolCaller{steps: []scriptedToolStep{{text: `{}`}}})
	if err := docWritePipeline(docWriteCoverageCommand(), "update_document",
		map[string]any{"nodeId": "node", "mode": "append", "index": 3}, "short", "update"); err != nil {
		t.Fatalf("short content with --index must still write: %v", err)
	}
}

func TestCrossPlatformCoverageDocWriteSurfacesDegradations(t *testing.T) {
	oldArgs := os.Args
	os.Args = []string{"dws", "doc"}
	t.Cleanup(func() { os.Args = oldArgs })
	// An oversized table must be split with its header repeated, and the caller
	// must be told so — silently turning one table into three would otherwise
	// look like a clean write.
	rows := strings.Repeat("| 张三 | 技术部 | 10086 |\n", 4000)
	content := "| 姓名 | 部门 | 工号 |\n|---|---|---|\n" + rows
	caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{}`}, {text: `{}`}, {text: `{}`}, {text: `{}`}}}
	installScriptedCaller(t, caller)
	// installScriptedCaller initializes deps and discards output; capture stdout
	// afterwards so the result JSON can be inspected.
	var stdout bytes.Buffer
	deps.Out = NewFormatterWithWriters(&stdout, &bytes.Buffer{})
	if err := docWritePipeline(docWriteCoverageCommand(), "update_document",
		map[string]any{"nodeId": "node", "mode": "overwrite"}, content, "update"); err != nil {
		t.Fatalf("chunked table write: %v", err)
	}

	// The result JSON follows the [INFO]/[WARN] progress lines on the same stream.
	out := stdout.String()
	if !strings.Contains(out, "[WARN] 内容过长已分片") {
		t.Errorf("no warning line for the table split: %q", out)
	}
	var result DocWriteResult
	if err := json.Unmarshal([]byte(out[strings.Index(out, "{"):]), &result); err != nil {
		t.Fatalf("decode result: %v (stdout=%q)", err, out)
	}
	if result.ChunksWritten < 2 {
		t.Fatalf("expected a chunked write, got %#v", result)
	}
	if len(result.Degradations) == 0 {
		t.Fatalf("table split was not reported: %#v", result)
	}
	for _, d := range result.Degradations {
		if d.Kind != "table_split" || !strings.HasPrefix(d.InjectedPrefix, "| 姓名 ") {
			t.Errorf("degradation = %#v", d)
		}
	}
}

func TestCrossPlatformCoverageDocWritePreviewTruncatesByRune(t *testing.T) {
	// Slicing by byte would split a multi-byte character and put invalid UTF-8
	// into the progress line.
	for _, tc := range []struct {
		in   string
		n    int
		want string
	}{
		{"abc", 5, "abc"},
		{"abcdef", 3, "abc"},
		{strings.Repeat("界", 5), 2, "界界"},
	} {
		if got := previewRunes(tc.in, tc.n); got != tc.want {
			t.Errorf("previewRunes(%q,%d) = %q, want %q", tc.in, tc.n, got, tc.want)
		}
	}
}

func TestCrossPlatformCoverageChunkedWriteAdaptiveRetryRemainingCoverage(t *testing.T) {
	oldArgs := os.Args
	os.Args = []string{"dws", "doc"}
	t.Cleanup(func() { os.Args = oldArgs })
	markdown := strings.Repeat("x", 24000)
	caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{}`}, {err: errors.New("HSFTimeoutException")}, {text: `{}`}}}
	installScriptedCaller(t, caller)
	outcome, err := chunkedWrite(context.Background(), "update_document", map[string]any{"nodeId": "node"}, markdown, "update", 10000)
	if err == nil || outcome.written != 1 || caller.calls != 2 || !strings.Contains(err.Error(), "提交状态未知") {
		t.Fatalf("timeout must stop without replay: written=%d calls=%d err=%v", outcome.written, caller.calls, err)
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
			outcome, err := chunkedWrite(context.Background(), tc.tool, tc.args, markdown, tc.name, 10000)
			var typed *apperrors.Error
			if err == nil || !errors.As(err, &typed) {
				t.Fatalf("error = %#v", err)
			}
			if outcome.nodeID != tc.wantNode || outcome.written != 0 || caller.calls != 1 {
				t.Fatalf("node=%q written=%d calls=%d", outcome.nodeID, outcome.written, caller.calls)
			}
			if typed.Reason != "doc_write_commit_unknown" || typed.FailureStage != "chunk_1" || typed.ExecutionStarted == nil || !*typed.ExecutionStarted || !typed.RetryableSet || typed.Retryable {
				t.Fatalf("unknown commit metadata = %#v", typed)
			}
		})
	}
}
