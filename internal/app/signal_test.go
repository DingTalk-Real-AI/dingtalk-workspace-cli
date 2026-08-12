package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
)

func signalSelf(t *testing.T, sig syscall.Signal) {
	t.Helper()
	process, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("find current process: %v", err)
	}
	if err := process.Signal(sig); err != nil {
		t.Skipf("current platform does not support process signal delivery: %v", err)
	}
}

func TestFrameworkSignalRedeliveryFallbackAndInterruptionMethods(t *testing.T) {
	originalFind, originalExit := rootFindProcess, rootExitProcess
	t.Cleanup(func() { rootFindProcess, rootExitProcess = originalFind, originalExit })
	rootFindProcess = func(int) (*os.Process, error) { return nil, errors.New("find failed") }
	exitCode := 0
	rootExitProcess = func(code int) { exitCode = code }
	rootEscalateSignal(syscall.SIGTERM)
	if exitCode != 143 {
		t.Fatalf("escalation exit=%d", exitCode)
	}
	exitCode = 0
	redeliverProcessSignal(syscall.SIGTERM)
	if exitCode != 143 {
		t.Fatalf("fallback exit=%d", exitCode)
	}
	rootFindProcess = func(int) (*os.Process, error) { return os.FindProcess(99999999) }
	exitCode = 0
	redeliverProcessSignal(syscall.SIGINT)
	if exitCode != 130 {
		t.Fatalf("signal fallback exit=%d", exitCode)
	}
	interrupted := &processInterruption{signal: syscall.SIGINT}
	if !errors.Is(interrupted, context.Canceled) || interrupted.ExitCode() != 130 || interrupted.Subtype() != "cancelled_by_user" || !strings.Contains(interrupted.Error(), "interrupt") {
		t.Fatalf("interruption=%v", interrupted)
	}
	terminated := &processInterruption{signal: syscall.SIGTERM}
	if terminated.ExitCode() != 143 || terminated.Subtype() != "terminated" {
		t.Fatalf("termination=%v", terminated)
	}
	state := &processSignalState{}
	if !state.record(syscall.SIGINT, nil) || state.record(syscall.SIGTERM, nil) {
		t.Fatal("signal state did not reject a second interruption")
	}
}

func TestFrameworkManageProcessSignalsNilAndEscalation(t *testing.T) {
	signals := make(chan os.Signal, 3)
	stopped, escalated := false, make(chan os.Signal, 1)
	ctx, _, stop := manageProcessSignals(context.Background(), nil, signals, func() { stopped = true }, func(sig os.Signal) { escalated <- sig })
	signals <- nil
	signals <- syscall.SIGINT
	<-ctx.Done()
	signals <- syscall.SIGTERM
	if got := <-escalated; got != syscall.SIGTERM {
		t.Fatalf("escalated=%v", got)
	}
	stop()
	stop()
	if !stopped {
		t.Fatal("signal notification was not stopped")
	}
}

func installSignalExecuteSeams(t *testing.T, unified bool, stdout, stderr io.Writer) {
	t.Helper()
	testseam.Protect(t, &os.Args)
	os.Args = []string{"dws"}
	testseam.Swap(t, &rootNormalizeProcessProfileArgs, func() func() { return func() {} })
	testseam.Swap(t, &rootRunPreParse, func(*cobra.Command, *pipeline.Engine) error { return nil })
	testseam.Swap(t, &rootStopAllStdioClients, func() {})
	testseam.Swap(t, &rootNewRootCommandWithEngine, func(ctx context.Context, _ *pipeline.Engine) *cobra.Command {
		cmd := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
		if unified {
			output.SetCommandRollout(cmd, output.RolloutUnifiedActive)
		}
		cmd.SetContext(ctx)
		cmd.SetOut(stdout)
		cmd.SetErr(stderr)
		return cmd
	})
}

func TestExecuteSignalEmitsOneTypedUnifiedFailure(t *testing.T) {
	for _, tc := range []struct {
		name    string
		signal  syscall.Signal
		code    int
		subtype string
	}{
		{name: "SIGINT", signal: syscall.SIGINT, code: 130, subtype: "cancelled_by_user"},
		{name: "SIGTERM", signal: syscall.SIGTERM, code: 143, subtype: "terminated"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			installSignalExecuteSeams(t, true, &stdout, &stderr)
			testseam.Swap(t, &rootExecuteCommand, func(cmd *cobra.Command) (*cobra.Command, error) {
				signalSelf(t, tc.signal)
				<-cmd.Context().Done()
				return cmd, cmd.Context().Err()
			})

			if code := Execute(); code != tc.code {
				t.Fatalf("Execute code=%d, want %d", code, tc.code)
			}
			var env output.Envelope
			if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
				t.Fatalf("decode envelope: %v; output=%q", err, stdout.String())
			}
			if env.Error == nil || env.Error.Type != "internal" || env.Error.Subtype != tc.subtype || env.Error.ExitCode != tc.code {
				t.Fatalf("error=%+v, want internal/%s exit %d", env.Error, tc.subtype, tc.code)
			}
			if bytes.Count(stdout.Bytes(), []byte(`"outcome": "failure"`)) != 1 {
				t.Fatalf("stdout must contain one failure envelope: %s", stdout.String())
			}
		})
	}
}

func TestExecuteSignalLegacyExitCodes(t *testing.T) {
	for _, tc := range []struct {
		signal syscall.Signal
		code   int
	}{{syscall.SIGINT, 130}, {syscall.SIGTERM, 143}} {
		t.Run(tc.signal.String(), func(t *testing.T) {
			installSignalExecuteSeams(t, false, io.Discard, io.Discard)
			testseam.Swap(t, &rootExecuteCommand, func(cmd *cobra.Command) (*cobra.Command, error) {
				signalSelf(t, tc.signal)
				<-cmd.Context().Done()
				return cmd, cmd.Context().Err()
			})
			if code := Execute(); code != tc.code {
				t.Fatalf("Execute code=%d, want %d", code, tc.code)
			}
		})
	}
}

func TestExecuteDeadlineIsNotSignalCancellation(t *testing.T) {
	var stdout bytes.Buffer
	installSignalExecuteSeams(t, true, &stdout, io.Discard)
	testseam.Swap(t, &rootExecuteCommand, func(cmd *cobra.Command) (*cobra.Command, error) {
		return cmd, context.DeadlineExceeded
	})
	if code := Execute(); code != 5 {
		t.Fatalf("Execute code=%d, want internal deadline code 5", code)
	}
	var env output.Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Error == nil || env.Error.Subtype != "deadline_exceeded" || env.Error.ExitCode == 130 || env.Error.ExitCode == 143 {
		t.Fatalf("deadline error=%+v", env.Error)
	}
}

func TestSignalAfterFailedEmissionAttemptPreservesPublicationExitCode(t *testing.T) {
	var stdout bytes.Buffer
	installSignalExecuteSeams(t, true, &stdout, io.Discard)
	testseam.Swap(t, &rootExecuteCommand, func(cmd *cobra.Command) (*cobra.Command, error) {
		cmd.SetOut(failingWriter{})
		if err := output.StoreResult(cmd.Context(), output.Success(map[string]any{"ok": true})); err != nil {
			t.Fatal(err)
		}
		_, _, _ = output.EmitStoredResult(cmd)
		signalSelf(t, syscall.SIGINT)
		<-cmd.Context().Done()
		return cmd, cmd.Context().Err()
	})
	if code := Execute(); code != 5 {
		t.Fatalf("Execute code=%d, want publication failure code 5", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("second envelope emitted: %q", stdout.String())
	}
}

func TestSignalBeforeEmissionAttemptPreservesPublishedOutcome(t *testing.T) {
	var stdout bytes.Buffer
	installSignalExecuteSeams(t, true, &stdout, io.Discard)
	testseam.Swap(t, &rootExecuteCommand, func(cmd *cobra.Command) (*cobra.Command, error) {
		// Record cancellation before publication begins, then simulate a command
		// hook that has already committed its result and completes publication.
		// The wire result must remain authoritative over the earlier signal.
		signalSelf(t, syscall.SIGINT)
		<-cmd.Context().Done()
		if err := output.StoreResult(cmd.Context(), output.Success(map[string]any{"ok": true})); err != nil {
			t.Fatal(err)
		}
		if _, _, err := output.EmitStoredResult(cmd); err != nil {
			t.Fatal(err)
		}
		return cmd, cmd.Context().Err()
	})
	if code := Execute(); code != 0 {
		t.Fatalf("Execute code=%d, want published success code 0", code)
	}
	var env output.Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope: %v; output=%q", err, stdout.String())
	}
	if !env.OK || env.Outcome != output.OutcomeSuccess {
		t.Fatalf("published envelope=%+v, want successful outcome", env)
	}
	if got := bytes.Count(stdout.Bytes(), []byte(`"outcome": "success"`)); got != 1 {
		t.Fatalf("stdout contains %d success envelopes, want one: %s", got, stdout.String())
	}
}

func TestSignalAfterCompletedPrimaryPreservesEstablishedOutcome(t *testing.T) {
	var stdout bytes.Buffer
	installSignalExecuteSeams(t, true, &stdout, io.Discard)
	testseam.Swap(t, &rootExecuteCommand, func(cmd *cobra.Command) (*cobra.Command, error) {
		if err := output.StoreResult(cmd.Context(), output.Success(map[string]any{"ok": true})); err != nil {
			t.Fatal(err)
		}
		if _, _, err := output.EmitStoredResult(cmd); err != nil {
			t.Fatal(err)
		}
		signalSelf(t, syscall.SIGINT)
		<-cmd.Context().Done()
		return cmd, cmd.Context().Err()
	})
	if code := Execute(); code != 0 {
		t.Fatalf("Execute code=%d, want established success code 0", code)
	}
	if got := bytes.Count(stdout.Bytes(), []byte(`"outcome": "success"`)); got != 1 {
		t.Fatalf("stdout contains %d success envelopes, want one: %s", got, stdout.String())
	}
}

func TestExecuteSignalSubprocessExitStatus(t *testing.T) {
	if os.Getenv("DWS_SIGNAL_HELPER") == "1" {
		installSignalExecuteSeams(t, true, os.Stdout, os.Stderr)
		testseam.Swap(t, &rootExecuteCommand, func(cmd *cobra.Command) (*cobra.Command, error) {
			_, _ = fmt.Fprintln(os.Stderr, "READY")
			<-cmd.Context().Done()
			return cmd, cmd.Context().Err()
		})
		os.Exit(Execute())
	}

	for _, tc := range []struct {
		name    string
		signal  syscall.Signal
		code    int
		subtype string
	}{
		{name: "SIGINT", signal: syscall.SIGINT, code: 130, subtype: "cancelled_by_user"},
		{name: "SIGTERM", signal: syscall.SIGTERM, code: 143, subtype: "terminated"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestExecuteSignalSubprocessExitStatus$")
			cmd.Env = append(os.Environ(), "DWS_SIGNAL_HELPER=1")
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			stderr, err := cmd.StderrPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			if scanner := bufio.NewScanner(stderr); !scanner.Scan() || scanner.Text() != "READY" {
				t.Fatalf("helper readiness failed: %q, err=%v", scanner.Text(), scanner.Err())
			}
			if err := cmd.Process.Signal(tc.signal); err != nil {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
				t.Skipf("current platform does not support subprocess signal delivery: %v", err)
			}
			payload, readErr := io.ReadAll(stdout)
			if readErr != nil {
				t.Fatal(readErr)
			}
			waitErr := cmd.Wait()
			var exitErr *exec.ExitError
			if !errors.As(waitErr, &exitErr) || exitErr.ExitCode() != tc.code {
				t.Fatalf("wait error=%v, want exit %d", waitErr, tc.code)
			}
			var env output.Envelope
			if err := json.Unmarshal(payload, &env); err != nil {
				t.Fatalf("decode helper output: %v; output=%q", err, payload)
			}
			if env.Error == nil || env.Error.Subtype != tc.subtype || env.Error.ExitCode != tc.code {
				t.Fatalf("helper error=%+v", env.Error)
			}
		})
	}
}

func TestSecondSignalUsesEscalationSeam(t *testing.T) {
	signals := make(chan os.Signal, 2)
	escalated := make(chan os.Signal, 1)
	ctx, _, stop := manageProcessSignals(context.Background(), nil, signals, func() {}, func(sig os.Signal) {
		escalated <- sig
	})
	signals <- syscall.SIGINT
	<-ctx.Done()
	if !errors.Is(context.Cause(ctx), context.Canceled) {
		t.Fatalf("cause=%v, want cancellation", context.Cause(ctx))
	}
	signals <- syscall.SIGTERM
	if got := <-escalated; got != syscall.SIGTERM {
		t.Fatalf("escalated %v, want SIGTERM", got)
	}
	stop()
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }
