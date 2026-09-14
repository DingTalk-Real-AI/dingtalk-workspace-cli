package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

func TestFrameworkErrorProjectionPreservesRecoveryMetadata(t *testing.T) {
	next := time.Date(2026, 8, 10, 1, 2, 3, 0, time.FixedZone("test", 8*60*60))
	retry := int64(4)
	started := true
	leaf := &helpers.CLIError{Code: "UPSTREAM_CODE", Suggestion: "retry with id", Operation: "create"}
	call := &transport.CallError{Stage: transport.CallStage("decode"), HTTPStatus: 503, RPCCode: 91, TraceID: "call-trace", Cause: leaf}
	typed := &apperrors.Error{
		Category: apperrors.CategoryAPI, Message: "failed", Reason: "upstream_failed", Hint: "use status",
		Actions: []string{"dws status"}, Retryable: true, RetryableSet: true, RetryAfterSeconds: &retry,
		RPCCode: 92, RPCData: json.RawMessage(`{"task":"x"}`), Operation: "publish", ServerKey: "server",
		Origin: "gateway", FailureStage: "response", ExecutionStarted: &started, NextRetryAt: &next,
		AvailableFlags: []string{"--id"}, Snapshot: "/tmp/snapshot", Details: map[string]any{"id": "x"},
		ServerDiag: apperrors.ServerDiagnostics{TraceID: "typed-trace", ServerErrorCode: "SERVER_CODE", TechnicalDetail: "detail", FriendlyHint: "friendly", ActionURL: "https://example.test"},
		Cause:      call,
	}
	info := errorInfoFromExecutionError(typed)
	if info.Type != "api" || info.Subtype != "upstream_failed" || info.HTTPStatus != 503 || info.RPCCode != 92 || info.RequestID != "call-trace" || info.TraceID != "typed-trace" {
		t.Fatalf("projection=%+v", info)
	}
	if info.UpstreamCode != "SERVER_CODE" || info.Operation != "publish" || info.NextRetryAt == "" || info.Cause == "" || info.RPCData == nil || info.ExecutionStarted == nil || !*info.ExecutionStarted {
		t.Fatalf("recovery metadata=%+v", info)
	}

	innerOperation := &helpers.CLIError{Operation: "create"}
	outerWithoutOperation := &apperrors.Error{
		Category: apperrors.CategoryAPI,
		Message:  "failed",
		Cause:    innerOperation,
	}
	preserved := errorInfoFromExecutionError(outerWithoutOperation)
	if preserved.Operation != "create" {
		t.Fatalf("operation=%q, want inner operation preserved", preserved.Operation)
	}

	requestCall := &transport.CallError{Stage: transport.CallStage("request"), HTTPStatus: 429, RequestID: "request-id"}
	requestInfo := errorInfoFromExecutionError(requestCall)
	if requestInfo.RequestID != "request-id" || requestInfo.HTTPStatus != 429 {
		t.Fatalf("request projection=%+v", requestInfo)
	}
	partial := errorInfoFromExecutionError(&apperrors.Error{Category: apperrors.CategoryPartial, Message: "partial"})
	if partial.Type != "internal" {
		t.Fatalf("partial error type=%s", partial.Type)
	}
	for code, want := range map[int]string{1: "api", 2: "auth", 3: "validation", 4: "permission", 6: "discovery", 99: "internal"} {
		if got := errorTypeForExitCode(code); got != want {
			t.Fatalf("errorTypeForExitCode(%d)=%q", code, got)
		}
	}
}

func TestFrameworkExecutePreparseUnifiedErrorAndEmissionFallback(t *testing.T) {
	for _, failWriter := range []bool{false, true} {
		t.Run(map[bool]string{false: "unified", true: "fallback"}[failWriter], func(t *testing.T) {
			testseam.Protect(t, &os.Args)
			os.Args = []string{"dws", "leaf"}
			testseam.Swap(t, &rootNormalizeProcessProfileArgs, func() func() { return func() {} })
			testseam.Swap(t, &rootStopAllStdioClients, func() {})
			testseam.Swap(t, &rootRunPreParse, func(*cobra.Command, *pipeline.Engine) error { return errors.New("bad preparse") })
			var stdout bytes.Buffer
			testseam.Swap(t, &rootNewRootCommandWithEngine, func(ctx context.Context, _ *pipeline.Engine) *cobra.Command {
				root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
				root.SetContext(ctx)
				leaf := &cobra.Command{Use: "leaf"}
				output.SetCommandRollout(leaf, output.RolloutUnifiedActive)
				if failWriter {
					leaf.SetOut(frameworkFailWriter{})
				} else {
					leaf.SetOut(&stdout)
				}
				leaf.SetErr(&bytes.Buffer{})
				root.AddCommand(leaf)
				return root
			})
			if code := Execute(); code != 3 {
				t.Fatalf("Execute code=%d", code)
			}
			if !failWriter && !strings.Contains(stdout.String(), `"outcome": "failure"`) {
				t.Fatalf("stdout=%q", stdout.String())
			}
		})
	}
}

func TestFrameworkPublicRootRequiresResultFromActiveCommand(t *testing.T) {
	root := NewRootCommand(context.Background())
	leaf := &cobra.Command{Use: "active-no-result", RunE: func(*cobra.Command, []string) error { return nil }}
	output.SetCommandRollout(leaf, output.RolloutUnifiedActive)
	root.AddCommand(leaf)
	root.SetArgs([]string{"active-no-result"})
	if _, err := root.ExecuteC(); err == nil || !strings.Contains(err.Error(), "without a CommandResult") {
		t.Fatalf("ExecuteC error=%v", err)
	}
}

func TestFrameworkAbortOutputSinkRemoveFailure(t *testing.T) {
	originalRemove := rootRemoveFile
	t.Cleanup(func() { rootRemoveFile = originalRemove })
	file, err := os.CreateTemp(t.TempDir(), "abort-*")
	if err != nil {
		t.Fatal(err)
	}
	rootRemoveFile = func(string) error { return errors.New("remove failed") }
	cmd := &cobra.Command{Use: "abort"}
	cmd.SetContext(context.WithValue(context.Background(), outputFileContextKey{}, &outputSinkState{file: file, tempPath: file.Name()}))
	if err := abortOutputSink(cmd); err == nil || !strings.Contains(err.Error(), "remove temporary") {
		t.Fatalf("abort error=%v", err)
	}
}

func TestFrameworkOutputSinkHookWrappingAndCleanupEdges(t *testing.T) {
	installOutputSinkRunBoundary(nil)
	plain := &cobra.Command{Use: "plain"}
	plain.SetContext(context.Background())
	installOutputSinkRunBoundary(plain)

	// newBoundaryChild builds a leaf whose --output lives on the root's
	// persistent flag set, matching production wiring (a local --output flag
	// belongs to the leaf's own business contract and skips the sink).
	newBoundaryChild := func(outputPath string) *cobra.Command {
		root := &cobra.Command{Use: "root"}
		root.PersistentFlags().String("output", outputPath, "")
		cmd := &cobra.Command{Use: "leaf"}
		root.AddCommand(cmd)
		cmd.SetContext(context.Background())
		return cmd
	}

	var calls int
	cmd := newBoundaryChild("")
	cmd.RunE = func(*cobra.Command, []string) error { calls++; return nil }
	cmd.PostRunE = func(*cobra.Command, []string) error { calls++; return nil }
	installOutputSinkRunBoundary(cmd)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if err := cmd.PostRunE(cmd, nil); err != nil {
		t.Fatal(err)
	}

	runOnly := newBoundaryChild("")
	runOnly.Run = func(*cobra.Command, []string) { calls++ }
	runOnly.PostRun = func(*cobra.Command, []string) { calls++ }
	installOutputSinkRunBoundary(runOnly)
	if runOnly.Run != nil || runOnly.RunE == nil {
		t.Fatal("Run-only leaf must be converted to RunE so sink setup errors surface")
	}
	if err := runOnly.RunE(runOnly, nil); err != nil {
		t.Fatal(err)
	}
	runOnly.PostRun(runOnly, nil)
	if calls != 4 {
		t.Fatalf("hook calls=%d", calls)
	}

	// A sink setup failure at Run entry returns before the business hook runs.
	testseam.Swap(t, &rootCreateTemp, func(string, string) (*os.File, error) { return nil, errors.New("create failed") })
	failCmd := newBoundaryChild(filepath.Join(t.TempDir(), "out.txt"))
	businessRan := false
	failCmd.RunE = func(*cobra.Command, []string) error { businessRan = true; return nil }
	installOutputSinkRunBoundary(failCmd)
	if err := failCmd.RunE(failCmd, nil); err == nil || !strings.Contains(err.Error(), "create failed") {
		t.Fatalf("Run entry sink setup error=%v", err)
	}
	if businessRan {
		t.Fatal("business hook ran after sink setup failure")
	}
	testseam.Swap(t, &rootCreateTemp, os.CreateTemp)

	// A Run entry business error aborts the open sink: the temporary file is
	// removed and the final target is never created.
	abortTarget := filepath.Join(t.TempDir(), "result.txt")
	abortCmd := newBoundaryChild(abortTarget)
	abortCmd.RunE = func(*cobra.Command, []string) error { return errors.New("boom") }
	installOutputSinkRunBoundary(abortCmd)
	if err := abortCmd.RunE(abortCmd, nil); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("Run entry business error=%v", err)
	}
	if _, err := os.Stat(abortTarget); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("target exists after aborted run: %v", err)
	}
	assertNoOutputTemps(t, abortTarget)

	// A second configureOutputSink call on an already-open sink (a reused
	// command tree stacks one Run wrapper per ExecuteC) must not replace the
	// live sink with a second temporary file.
	repeatTarget := filepath.Join(t.TempDir(), "result.txt")
	repeatCmd := newBoundaryChild(repeatTarget)
	if err := configureOutputSink(repeatCmd); err != nil {
		t.Fatal(err)
	}
	first := outputSinkForCommand(repeatCmd)
	if first == nil {
		t.Fatal("first configureOutputSink did not open a sink")
	}
	if err := configureOutputSink(repeatCmd); err != nil {
		t.Fatal(err)
	}
	if second := outputSinkForCommand(repeatCmd); second != first {
		t.Fatal("configureOutputSink replaced an open sink")
	}
	if err := abortOutputSink(repeatCmd); err != nil {
		t.Fatal(err)
	}
	assertNoOutputTemps(t, repeatTarget)

	file2, err := os.CreateTemp(t.TempDir(), "sink-error-*")
	if err != nil {
		t.Fatal(err)
	}
	errorCmd := &cobra.Command{Use: "error"}
	errorCmd.SetContext(context.WithValue(context.Background(), outputFileContextKey{}, &outputSinkState{file: file2, tempPath: file2.Name(), target: "unused"}))
	if err := runWithOutputSinkErrorCleanup(errorCmd, func() error { return errors.New("boom") }); err == nil {
		t.Fatal("run error swallowed")
	}

	file3, err := os.CreateTemp(t.TempDir(), "sink-panic-*")
	if err != nil {
		t.Fatal(err)
	}
	panicCmd := &cobra.Command{Use: "panic"}
	panicCmd.SetContext(context.WithValue(context.Background(), outputFileContextKey{}, &outputSinkState{file: file3, tempPath: file3.Name(), target: "unused"}))
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("panic swallowed")
			}
		}()
		_ = runWithOutputSinkErrorCleanup(panicCmd, func() error { panic("boom") })
	}()

	if closeOutputSink(nil) != nil || abortOutputSink(nil) != nil || outputSinkForCommand(nil) != nil {
		t.Fatal("nil sink guards failed")
	}
	finished := &outputSinkState{finished: true, file: file3}
	finishedCmd := &cobra.Command{Use: "finished"}
	finishedCmd.SetContext(context.WithValue(context.Background(), outputFileContextKey{}, finished))
	if closeOutputSink(finishedCmd) != nil || abortOutputSink(finishedCmd) != nil {
		t.Fatal("finished sink was processed twice")
	}
}

type frameworkFailWriter struct{}

func (frameworkFailWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestCrossPlatformCoverageFrameworkExecutePanicBeforeEmissionUsesUnifiedFailure(t *testing.T) {
	for _, failWriter := range []bool{false, true} {
		t.Run(map[bool]string{false: "emits", true: "fallback"}[failWriter], func(t *testing.T) {
			testseam.Protect(t, &os.Args)
			os.Args = []string{"dws"}
			testseam.Swap(t, &rootNormalizeProcessProfileArgs, func() func() { return func() {} })
			testseam.Swap(t, &rootRunPreParse, func(*cobra.Command, *pipeline.Engine) error { return nil })
			testseam.Swap(t, &rootStopAllStdioClients, func() {})
			var stdout bytes.Buffer
			testseam.Swap(t, &rootNewRootCommandWithEngine, func(ctx context.Context, _ *pipeline.Engine) *cobra.Command {
				cmd := &cobra.Command{Use: "dws"}
				cmd.SetContext(ctx)
				output.SetCommandRollout(cmd, output.RolloutUnifiedActive)
				if failWriter {
					cmd.SetOut(frameworkFailWriter{})
				} else {
					cmd.SetOut(&stdout)
				}
				cmd.SetErr(&bytes.Buffer{})
				return cmd
			})
			testseam.Swap(t, &rootExecuteCommand, func(*cobra.Command) (*cobra.Command, error) { panic("before emission") })
			if code := Execute(); code != 5 {
				t.Fatalf("Execute code=%d", code)
			}
			if !failWriter && !strings.Contains(stdout.String(), `"outcome": "failure"`) {
				t.Fatalf("stdout=%q", stdout.String())
			}
		})
	}
}

func TestCrossPlatformCoverageFrameworkExecuteRareOutcomeBranches(t *testing.T) {
	t.Run("preparse interrupted", func(t *testing.T) {
		var stdout bytes.Buffer
		installSignalExecuteSeams(t, true, &stdout, io.Discard)
		testseam.Swap(t, &rootRunPreParse, func(cmd *cobra.Command, _ *pipeline.Engine) error {
			signalSelf(t, syscall.SIGINT)
			<-cmd.Context().Done()
			return errors.New("preparse failed")
		})
		if code := Execute(); code != 130 {
			t.Fatalf("Execute code=%d", code)
		}
	})

	t.Run("nil executed after emission attempt", func(t *testing.T) {
		installSignalExecuteSeams(t, true, io.Discard, io.Discard)
		testseam.Swap(t, &rootExecuteCommand, func(cmd *cobra.Command) (*cobra.Command, error) {
			cmd.SetOut(frameworkFailWriter{})
			if err := output.StoreResult(cmd.Context(), output.Success(map[string]any{"ok": true})); err != nil {
				t.Fatal(err)
			}
			_, _, _ = output.EmitStoredResult(cmd)
			signalSelf(t, syscall.SIGINT)
			<-cmd.Context().Done()
			return nil, cmd.Context().Err()
		})
		if code := Execute(); code != 5 {
			t.Fatalf("Execute code=%d", code)
		}
	})

	t.Run("publication failure after emission", func(t *testing.T) {
		var stdout bytes.Buffer
		installSignalExecuteSeams(t, true, &stdout, io.Discard)
		testseam.Swap(t, &rootExecuteCommand, func(cmd *cobra.Command) (*cobra.Command, error) {
			if err := output.StoreResult(cmd.Context(), output.Success(map[string]any{"ok": true})); err != nil {
				t.Fatal(err)
			}
			if _, _, err := output.EmitStoredResult(cmd); err != nil {
				t.Fatal(err)
			}
			return cmd, newOutputPublicationError("publish", errors.New("rename failed"))
		})
		if code := Execute(); code != 5 {
			t.Fatalf("Execute code=%d", code)
		}
	})

	t.Run("publication failure envelope writer also fails", func(t *testing.T) {
		installSignalExecuteSeams(t, true, io.Discard, io.Discard)
		testseam.Swap(t, &rootExecuteCommand, func(cmd *cobra.Command) (*cobra.Command, error) {
			if err := output.StoreResult(cmd.Context(), output.Success(map[string]any{"ok": true})); err != nil {
				t.Fatal(err)
			}
			if _, _, err := output.EmitStoredResult(cmd); err != nil {
				t.Fatal(err)
			}
			file, err := os.CreateTemp(t.TempDir(), "finished-output-*")
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			state := &outputSinkState{file: file, original: frameworkFailWriter{}, finished: true}
			cmd.SetContext(context.WithValue(cmd.Context(), outputFileContextKey{}, state))
			return cmd, newOutputPublicationError("publish", errors.New("rename failed"))
		})
		if code := Execute(); code != 5 {
			t.Fatalf("Execute code=%d", code)
		}
	})

	t.Run("failure envelope cannot be written", func(t *testing.T) {
		installSignalExecuteSeams(t, true, io.Discard, io.Discard)
		testseam.Swap(t, &rootExecuteCommand, func(cmd *cobra.Command) (*cobra.Command, error) {
			cmd.SetOut(frameworkFailWriter{})
			return cmd, errors.New("business failed")
		})
		if code := Execute(); code != 5 {
			t.Fatalf("Execute code=%d", code)
		}
	})

	t.Run("late output publication warning", func(t *testing.T) {
		installSignalExecuteSeams(t, false, io.Discard, io.Discard)
		testseam.Swap(t, &rootRenameFile, func(string, string) error { return errors.New("rename failed") })
		testseam.Swap(t, &rootExecuteCommand, func(cmd *cobra.Command) (*cobra.Command, error) {
			file, err := os.CreateTemp(t.TempDir(), "late-output-*")
			if err != nil {
				t.Fatal(err)
			}
			state := &outputSinkState{file: file, tempPath: file.Name(), target: filepath.Join(t.TempDir(), "result.json")}
			cmd.SetContext(context.WithValue(cmd.Context(), outputFileContextKey{}, state))
			return cmd, nil
		})
		if code := Execute(); code != 5 {
			t.Fatalf("Execute code=%d", code)
		}
	})

	for _, tc := range []struct {
		name       string
		unified    bool
		original   io.Writer
		wantOutput bool
	}{
		{name: "unified late publication failure", unified: true, original: &bytes.Buffer{}, wantOutput: true},
		{name: "legacy late publication failure", original: io.Discard},
		{name: "late publication failure writer fails", unified: true, original: frameworkFailWriter{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			installSignalExecuteSeams(t, tc.unified, io.Discard, io.Discard)
			testseam.Swap(t, &rootRenameFile, func(string, string) error { return errors.New("rename failed") })
			var original io.Writer = tc.original
			testseam.Swap(t, &rootExecuteCommand, func(cmd *cobra.Command) (*cobra.Command, error) {
				file, err := os.CreateTemp(t.TempDir(), "panic-output-*")
				if err != nil {
					t.Fatal(err)
				}
				cmd.SetOut(file)
				cmd.SetContext(context.WithValue(cmd.Context(), outputFileContextKey{}, &outputSinkState{
					file: file, original: original, tempPath: file.Name(), target: filepath.Join(t.TempDir(), "result.json"),
				}))
				panic("after sink open")
			})
			if code := Execute(); code != 5 {
				t.Fatalf("Execute code=%d", code)
			}
			if tc.wantOutput && !strings.Contains(tc.original.(*bytes.Buffer).String(), `"outcome": "failure"`) {
				t.Fatalf("stdout=%q", tc.original.(*bytes.Buffer).String())
			}
		})
	}

	t.Run("abort failure is diagnostic", func(t *testing.T) {
		installSignalExecuteSeams(t, false, io.Discard, io.Discard)
		testseam.Swap(t, &rootExecuteCommand, func(cmd *cobra.Command) (*cobra.Command, error) {
			file, err := os.CreateTemp(t.TempDir(), "abort-output-*")
			if err != nil {
				t.Fatal(err)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			cmd.SetContext(context.WithValue(cmd.Context(), outputFileContextKey{}, &outputSinkState{
				file: file, original: io.Discard, tempPath: file.Name(), target: filepath.Join(t.TempDir(), "result.json"),
			}))
			return cmd, errors.New("business failed")
		})
		if code := Execute(); code != 5 {
			t.Fatalf("Execute code=%d", code)
		}
	})

	t.Run("publication helper requires observable finished transaction", func(t *testing.T) {
		cmd := &cobra.Command{Use: "unified"}
		output.SetCommandRollout(cmd, output.RolloutUnifiedActive)
		file, err := os.CreateTemp(t.TempDir(), "unfinished-output-*")
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		cmd.SetContext(context.WithValue(context.Background(), outputFileContextKey{}, &outputSinkState{
			file: file, finished: true,
		}))
		if _, handled, emitErr := emitOutputPublicationFailure(cmd, newOutputPublicationError("publish", errors.New("rename failed"))); handled || emitErr != nil {
			t.Fatalf("handled=%v err=%v", handled, emitErr)
		}
	})
}

func TestCrossPlatformCoverageExecuteDeterministicInterruptionBranches(t *testing.T) {
	install := func(t *testing.T, state *processSignalState, stdout, stderr io.Writer) {
		t.Helper()
		testseam.Protect(t, &os.Args)
		os.Args = []string{"dws"}
		testseam.Swap(t, &rootNormalizeProcessProfileArgs, func() func() { return func() {} })
		testseam.Swap(t, &rootStopAllStdioClients, func() {})
		testseam.Swap(t, &rootInstallProcessSignalContext, func(ctx context.Context, _ *output.ResultStore) (context.Context, *processSignalState, func()) {
			return ctx, state, func() {}
		})
		testseam.Swap(t, &rootNewRootCommandWithEngine, func(ctx context.Context, _ *pipeline.Engine) *cobra.Command {
			cmd := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
			output.SetCommandRollout(cmd, output.RolloutUnifiedActive)
			cmd.SetContext(ctx)
			cmd.SetOut(stdout)
			cmd.SetErr(stderr)
			return cmd
		})
	}
	interrupted := func(primaryCompleted bool) *processSignalState {
		return &processSignalState{
			interruption:             &processInterruption{signal: os.Interrupt},
			primaryCompletedAtSignal: primaryCompleted,
		}
	}

	t.Run("preparse interruption emits unified failure", func(t *testing.T) {
		var stdout bytes.Buffer
		install(t, interrupted(false), &stdout, io.Discard)
		testseam.Swap(t, &rootRunPreParse, func(*cobra.Command, *pipeline.Engine) error { return errors.New("preparse failed") })
		testseam.Swap(t, &rootExecuteCommand, func(*cobra.Command) (*cobra.Command, error) {
			t.Fatal("preparse failure reached command execution")
			return nil, nil
		})
		if code, _, summary := ExecuteWithTelemetry(); code != 130 || summary == "" || !strings.Contains(stdout.String(), `"outcome": "failure"`) {
			t.Fatalf("preparse interruption = code %d summary %q stdout %q", code, summary, stdout.String())
		}
	})

	t.Run("interruption before emission becomes primary error", func(t *testing.T) {
		var stdout bytes.Buffer
		install(t, interrupted(false), &stdout, io.Discard)
		testseam.Swap(t, &rootRunPreParse, func(*cobra.Command, *pipeline.Engine) error { return nil })
		testseam.Swap(t, &rootExecuteCommand, func(cmd *cobra.Command) (*cobra.Command, error) { return cmd, nil })
		if code, _, summary := ExecuteWithTelemetry(); code != 130 || summary == "" || !strings.Contains(stdout.String(), `"outcome": "failure"`) {
			t.Fatalf("pre-emission interruption = code %d summary %q stdout %q", code, summary, stdout.String())
		}
	})

	t.Run("late hook error preserves emitted result", func(t *testing.T) {
		install(t, interrupted(true), io.Discard, io.Discard)
		testseam.Swap(t, &rootRunPreParse, func(*cobra.Command, *pipeline.Engine) error { return nil })
		testseam.Swap(t, &rootExecuteCommand, func(cmd *cobra.Command) (*cobra.Command, error) {
			if err := output.StoreResult(cmd.Context(), output.Success(map[string]any{"ok": true})); err != nil {
				t.Fatal(err)
			}
			if _, _, err := output.EmitStoredResult(cmd); err != nil {
				t.Fatal(err)
			}
			return cmd, errors.New("late hook failed")
		})
		if code, _, summary := ExecuteWithTelemetry(); code != 0 || summary != "late hook failed" {
			t.Fatalf("late hook result = code %d summary %q", code, summary)
		}
	})

	t.Run("interruption after emission preserves emitted result", func(t *testing.T) {
		install(t, interrupted(false), io.Discard, io.Discard)
		testseam.Swap(t, &rootRunPreParse, func(*cobra.Command, *pipeline.Engine) error { return nil })
		testseam.Swap(t, &rootExecuteCommand, func(cmd *cobra.Command) (*cobra.Command, error) {
			if err := output.StoreResult(cmd.Context(), output.Success(map[string]any{"ok": true})); err != nil {
				t.Fatal(err)
			}
			if _, _, err := output.EmitStoredResult(cmd); err != nil {
				t.Fatal(err)
			}
			return cmd, nil
		})
		if code, _, summary := ExecuteWithTelemetry(); code != 0 || summary == "" {
			t.Fatalf("post-emission interruption = code %d summary %q", code, summary)
		}
	})

	t.Run("publication failure replaces unobservable result", func(t *testing.T) {
		var original bytes.Buffer
		install(t, interrupted(false), io.Discard, io.Discard)
		testseam.Swap(t, &rootRunPreParse, func(*cobra.Command, *pipeline.Engine) error { return nil })
		testseam.Swap(t, &rootExecuteCommand, func(cmd *cobra.Command) (*cobra.Command, error) {
			if err := output.StoreResult(cmd.Context(), output.Success(map[string]any{"ok": true})); err != nil {
				t.Fatal(err)
			}
			if _, _, err := output.EmitStoredResult(cmd); err != nil {
				t.Fatal(err)
			}
			file, err := os.CreateTemp(t.TempDir(), "finished-output-*")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = file.Close() })
			cmd.SetContext(context.WithValue(cmd.Context(), outputFileContextKey{}, &outputSinkState{file: file, original: &original, finished: true}))
			publicationErr := newOutputPublicationError("publish", errors.New("rename failed"))
			if _, handled, emitErr := emitOutputPublicationFailure(cmd, publicationErr); !handled || emitErr != nil {
				t.Fatalf("precondition publication failure = handled %v error %v unified %v state %v", handled, emitErr, output.UsesUnifiedResult(cmd), outputSinkForCommand(cmd) != nil)
			}
			return cmd, publicationErr
		})
		if code, _, summary := ExecuteWithTelemetry(); code != 5 || summary == "" {
			t.Fatalf("publication failure = code %d summary %q output %q", code, summary, original.String())
		}
	})
}

type frameworkPanicWriter struct{}

func (frameworkPanicWriter) Write([]byte) (int, error) { panic("writer panic") }

func TestCrossPlatformCoverageFrameworkRootHookErrors(t *testing.T) {
	t.Run("flag group validation", func(t *testing.T) {
		root := NewRootCommand(context.Background())
		leaf := &cobra.Command{Use: "exclusive", RunE: func(*cobra.Command, []string) error { return nil }}
		leaf.Flags().Bool("left", false, "")
		leaf.Flags().Bool("right", false, "")
		leaf.MarkFlagsMutuallyExclusive("left", "right")
		root.AddCommand(leaf)
		root.SetOut(io.Discard)
		root.SetErr(io.Discard)
		root.SetArgs([]string{"exclusive", "--left", "--right"})
		if err := root.Execute(); err == nil {
			t.Fatal("expected mutually-exclusive flag error")
		}
	})

	t.Run("edition pre-run error", func(t *testing.T) {
		old := edition.Get()
		t.Cleanup(func() { edition.Override(old) })
		edition.Override(&edition.Hooks{AfterPersistentPreRun: func(*cobra.Command, []string) error {
			return errors.New("edition hook failed")
		}})
		root := NewRootCommand(context.Background())
		root.SetOut(io.Discard)
		root.SetErr(io.Discard)
		root.SetArgs([]string{"version"})
		if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "edition hook failed") {
			t.Fatalf("Execute error=%v", err)
		}
	})

	t.Run("post-run emission panic", func(t *testing.T) {
		root := NewRootCommand(context.Background())
		cmd := &cobra.Command{Use: "panic-output"}
		output.SetCommandRollout(cmd, output.RolloutUnifiedActive)
		ctx, _ := output.WithResultStore(context.Background())
		cmd.SetContext(ctx)
		cmd.SetOut(frameworkPanicWriter{})
		if err := output.StoreResult(ctx, output.Success(map[string]any{"ok": true})); err != nil {
			t.Fatal(err)
		}
		defer func() {
			if recover() == nil {
				t.Fatal("expected post-run panic")
			}
		}()
		_ = root.PersistentPostRunE(cmd, nil)
	})
}
