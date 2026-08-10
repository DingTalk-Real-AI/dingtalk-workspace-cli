package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
	"github.com/spf13/cobra"
)

func TestPublicRootDirectExecuteFailsWhenUnifiedSinkCannotPublish(t *testing.T) {
	oldClose := rootCloseFile
	t.Cleanup(func() { rootCloseFile = oldClose })
	closeCalls := 0
	rootCloseFile = func(file *os.File) error {
		closeCalls++
		if err := file.Close(); err != nil {
			return err
		}
		return errors.New("late close diagnostic")
	}

	root := NewRootCommand(context.Background())
	leaf := &cobra.Command{
		Use: "lifecycle-unified",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return output.StoreResult(cmd.Context(), output.Success(map[string]any{"id": "ok"}))
		},
	}
	output.SetCommandRollout(leaf, output.RolloutUnifiedActive)
	root.AddCommand(leaf)
	root.SetArgs([]string{"lifecycle-unified", "--output", filepath.Join(t.TempDir(), "result.json")})

	executed, err := root.ExecuteC()
	if err == nil || apperrors.ExitCode(err) != 5 {
		t.Fatalf("direct ExecuteC error=%v, want publication failure with exit 5", err)
	}
	if executed != leaf {
		t.Fatalf("executed=%v, want lifecycle leaf", executed)
	}
	if closeCalls != 1 {
		t.Fatalf("output sink close calls=%d, want 1", closeCalls)
	}
}

func TestPublicRootDirectExecutePreservesLegacyCloseError(t *testing.T) {
	oldClose := rootCloseFile
	t.Cleanup(func() { rootCloseFile = oldClose })
	rootCloseFile = func(file *os.File) error {
		_ = file.Close()
		return errors.New("legacy close failed")
	}

	root := NewRootCommandWithEngine(context.Background(), nil)
	root.AddCommand(&cobra.Command{Use: "lifecycle-legacy", RunE: func(*cobra.Command, []string) error { return nil }})
	root.SetArgs([]string{"lifecycle-legacy", "--output", filepath.Join(t.TempDir(), "result.txt")})
	if _, err := root.ExecuteC(); err == nil || !strings.Contains(err.Error(), "legacy close failed") {
		t.Fatalf("legacy direct ExecuteC error=%v, want close failure", err)
	}
}

func TestPublicRootDirectExecuteClosesSinkOnHandlerError(t *testing.T) {
	oldClose := rootCloseFile
	t.Cleanup(func() { rootCloseFile = oldClose })
	closeCalls := 0
	rootCloseFile = func(file *os.File) error {
		closeCalls++
		return file.Close()
	}

	root := NewRootCommand(context.Background())
	root.AddCommand(&cobra.Command{Use: "lifecycle-error", RunE: func(*cobra.Command, []string) error {
		return errors.New("handler failed")
	}})
	root.SetArgs([]string{"lifecycle-error", "--output", filepath.Join(t.TempDir(), "result.txt")})
	if _, err := root.ExecuteC(); err == nil || !strings.Contains(err.Error(), "handler failed") {
		t.Fatalf("direct ExecuteC error=%v, want handler failure", err)
	}
	if closeCalls != 1 {
		t.Fatalf("output sink close calls=%d, want 1", closeCalls)
	}
}

func TestExecutePanicAfterEmissionPreservesSingleResultAndExitCode(t *testing.T) {
	oldNormalize := rootNormalizeProcessProfileArgs
	oldExecute := rootExecuteCommand
	oldNewRoot := rootNewRootCommandWithEngine
	oldPreParse := rootRunPreParse
	oldStop := rootStopAllStdioClients
	oldArgs := os.Args
	t.Cleanup(func() {
		rootNormalizeProcessProfileArgs = oldNormalize
		rootExecuteCommand = oldExecute
		rootNewRootCommandWithEngine = oldNewRoot
		rootRunPreParse = oldPreParse
		rootStopAllStdioClients = oldStop
		os.Args = oldArgs
	})
	os.Args = []string{"dws"}
	rootNormalizeProcessProfileArgs = func() func() { return func() {} }
	rootRunPreParse = func(*cobra.Command, *pipeline.Engine) error { return nil }
	rootStopAllStdioClients = func() {}
	var stdout, stderr bytes.Buffer
	rootNewRootCommandWithEngine = func(ctx context.Context, _ *pipeline.Engine) *cobra.Command {
		cmd := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
		output.SetCommandRollout(cmd, output.RolloutUnifiedActive)
		cmd.SetOut(&stdout)
		cmd.SetErr(&stderr)
		cmd.SetContext(ctx)
		return cmd
	}
	rootExecuteCommand = func(cmd *cobra.Command) (*cobra.Command, error) {
		result := output.Failure(&output.ErrorInfo{Type: "validation", Message: "bad input"})
		if err := output.StoreResult(cmd.Context(), result); err != nil {
			t.Fatal(err)
		}
		if _, _, err := output.EmitStoredResult(cmd); err != nil {
			t.Fatal(err)
		}
		panic("after emission")
	}

	if code := Execute(); code != 3 {
		t.Fatalf("Execute code=%d, want emitted validation code 3", code)
	}
	if got := strings.Count(stdout.String(), `"outcome": "failure"`); got != 1 {
		t.Fatalf("stdout contains %d envelopes, want one: %s", got, stdout.String())
	}
	if !strings.Contains(stderr.String(), "panicked after result emission attempt") {
		t.Fatalf("panic diagnostic missing: %q", stderr.String())
	}
}

func TestErrorInfoProjectionKeepsTraceIDDistinctFromRequestID(t *testing.T) {
	err := apperrors.NewAPI("failed", apperrors.WithTraceID("trace-1"))
	info := errorInfoFromExecutionError(err)
	if info.TraceID != "trace-1" || info.RequestID != "" {
		t.Fatalf("projection trace_id=%q request_id=%q", info.TraceID, info.RequestID)
	}
}
