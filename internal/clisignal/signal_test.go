package clisignal

import (
	"context"
	"errors"
	"os"
	"syscall"
	"testing"
	"time"
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
