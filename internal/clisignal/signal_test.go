package clisignal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageSignalLifecycle(t *testing.T) {
	for _, sig := range []os.Signal{os.Interrupt, syscall.SIGTERM} {
		t.Run(sig.String(), func(t *testing.T) {
			signals, escalated := make(chan os.Signal, 2), make(chan os.Signal, 1)
			stopped := 0
			ctx, state, stop := Manage(context.Background(), nil, signals,
				func() { stopped++ }, func(value os.Signal) { escalated <- value })
			t.Cleanup(stop)
			signals <- sig
			select {
			case <-ctx.Done():
			case <-time.After(2 * time.Second):
				t.Fatal("first signal did not cancel execution")
			}
			interrupted, completed := state.Outcome()
			if interrupted == nil || interrupted.ExitCode() != ExitCode(sig) || completed || !errors.Is(context.Cause(ctx), context.Canceled) {
				t.Fatalf("first signal outcome: %v, completed %v, cause %v", interrupted, completed, context.Cause(ctx))
			}
			signals <- syscall.SIGTERM
			select {
			case got := <-escalated:
				if got != syscall.SIGTERM {
					t.Fatalf("second signal changed: %v", got)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("second signal did not escalate")
			}
			stop()
			stop()
			if stopped != 1 {
				t.Fatalf("notification stopped %d times", stopped)
			}
			if final, _ := state.Outcome(); final != interrupted {
				t.Fatal("second signal replaced the first cause")
			}
		})
	}
}

func TestCrossPlatformCoverageSignalRedeliverExitCodesAndDetail(t *testing.T) {
	if ExitCode(os.Interrupt) != 130 || ExitCode(syscall.SIGTERM) != 143 {
		t.Fatal("signal exit codes")
	}
	exited := 0
	Redeliver(os.Interrupt, func(int) (*os.Process, error) { return nil, errors.New("missing process") }, func(code int) { exited = code })
	if exited != 130 {
		t.Fatalf("interrupt fallback exit = %d", exited)
	}
	exited = 0
	Redeliver(syscall.SIGTERM, func(int) (*os.Process, error) { return nil, errors.New("missing process") }, func(code int) { exited = code })
	if exited != 143 {
		t.Fatalf("term fallback exit = %d", exited)
	}

	signal.Ignore(os.Interrupt)
	t.Cleanup(func() { signal.Reset(os.Interrupt) })
	exited = 0
	live, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	Redeliver(os.Interrupt, func(int) (*os.Process, error) { return live, nil }, func(code int) { exited = code })
	if exited != 0 && exited != 130 {
		t.Fatalf("live process Redeliver exit = %d", exited)
	}

	interrupt := NewInterruption(os.Interrupt)
	if interrupt.Subtype() != "cancelled_by_user" || interrupt.Unwrap() != context.Canceled {
		t.Fatalf("interrupt interruption: %#v", interrupt)
	}
	term := NewInterruption(syscall.SIGTERM)
	if term.Subtype() != "terminated" {
		t.Fatalf("term subtype = %q", term.Subtype())
	}
	if interrupt.WithCancellationDetail(nil) != interrupt || interrupt.WithCancellationDetail(context.Canceled) != interrupt {
		t.Fatal("plain cancellation must not add detail")
	}
	nested := NewInterruption(syscall.SIGTERM)
	if interrupt.WithCancellationDetail(nested) != interrupt {
		t.Fatal("nested interruption must not override the first signal")
	}
	detailed := interrupt.WithCancellationDetail(fmt.Errorf("command stopped: %w", context.Canceled))
	if detailed == interrupt || !strings.Contains(detailed.Error(), "command stopped") || detailed.Unwrap() != context.Canceled {
		t.Fatalf("detailed interruption = %v", detailed)
	}

	var state State
	if !state.Record(os.Interrupt, func() bool { return true }) {
		t.Fatal("first record must succeed")
	}
	if state.Record(syscall.SIGTERM, nil) {
		t.Fatal("second record must be ignored")
	}
	got, completed := state.Outcome()
	if got == nil || got.ExitCode() != 130 || !completed {
		t.Fatalf("state outcome = %#v completed=%v", got, completed)
	}
}

func TestCrossPlatformCoverageInstallStopAndEscalateIgnoredSignal(t *testing.T) {
	ctx, state, stop := Install(context.Background(), func() bool { return false })
	if ctx == nil || state == nil {
		t.Fatal("Install returned a nil context or state")
	}
	stop()
	stop()
	// Swap the live process/exit seams so Escalate → Redeliver is covered
	// in-process on every GOOS, including Windows where SIGWINCH is undefined
	// and os.Process.Signal is not a safe way to ignore a second signal.
	testseam.Swap(t, &escalateFindProcess, func(int) (*os.Process, error) {
		return nil, errors.New("skip live signal")
	})
	exited := 0
	testseam.Swap(t, &escalateExit, func(code int) { exited = code })
	Escalate(os.Interrupt)
	if exited != 130 {
		t.Fatalf("Escalate fallback exit = %d, want 130", exited)
	}
}
