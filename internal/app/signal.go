package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
)

var rootEscalateSignal = func(sig os.Signal) {
	signal.Reset(sig)
	redeliverProcessSignal(sig)
}

var (
	rootFindProcess = os.FindProcess
	rootExitProcess = os.Exit
)

// redeliverProcessSignal asks the current process to handle the second signal
// with the platform's default semantics. Platforms that cannot deliver the
// requested signal through os.Process.Signal fall back to the conventional
// CLI exit status instead of leaving the process running after escalation.
func redeliverProcessSignal(sig os.Signal) {
	process, err := rootFindProcess(os.Getpid())
	if err == nil {
		err = process.Signal(sig)
	}
	if err != nil {
		rootExitProcess(interruptionExitCode(sig))
	}
}

func interruptionExitCode(sig os.Signal) int {
	if sig == syscall.SIGTERM {
		return 143
	}
	return 130
}

type processInterruption struct {
	signal os.Signal
	detail error
}

func (e *processInterruption) Error() string {
	message := fmt.Sprintf("process interrupted by %s", e.signal)
	if e.detail != nil {
		return fmt.Sprintf("%s: %v", message, e.detail)
	}
	return message
}

func (e *processInterruption) Unwrap() error { return context.Canceled }

// withCancellationDetail keeps the signal as the primary process error while
// retaining actionable context from a command that stopped because of that
// signal. Plain context cancellation and nested signal errors add no useful
// detail, and unrelated command failures must not be relabelled as part of the
// interruption. The detail is deliberately not exposed through Unwrap so an
// inner structured error cannot override the signal's exit code or subtype.
func (e *processInterruption) withCancellationDetail(err error) *processInterruption {
	if e == nil || err == nil || err == context.Canceled || !errors.Is(err, context.Canceled) {
		return e
	}
	var interrupted *processInterruption
	if errors.As(err, &interrupted) {
		return e
	}
	return &processInterruption{signal: e.signal, detail: err}
}

func (e *processInterruption) ExitCode() int {
	return interruptionExitCode(e.signal)
}

func (e *processInterruption) Subtype() string {
	if e.signal == syscall.SIGTERM {
		return "terminated"
	}
	return "cancelled_by_user"
}

type processSignalState struct {
	mu                       sync.Mutex
	interruption             *processInterruption
	primaryCompletedAtSignal bool
}

func (s *processSignalState) record(sig os.Signal, store *output.ResultStore) (first bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.interruption != nil {
		return false
	}
	_, _, s.primaryCompletedAtSignal, _ = output.StoredEmissionState(store)
	s.interruption = &processInterruption{signal: sig}
	return true
}

func (s *processSignalState) outcome() (*processInterruption, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.interruption, s.primaryCompletedAtSignal
}

func installProcessSignalContext(parent context.Context, store *output.ResultStore) (context.Context, *processSignalState, func()) {
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	return manageProcessSignals(parent, store, signals, func() { signal.Stop(signals) }, rootEscalateSignal)
}

func manageProcessSignals(
	parent context.Context,
	store *output.ResultStore,
	signals <-chan os.Signal,
	stopNotify func(),
	escalate func(os.Signal),
) (context.Context, *processSignalState, func()) {
	ctx, cancel := context.WithCancelCause(parent)
	state := &processSignalState{}
	done := make(chan struct{})
	stopped := make(chan struct{})
	var stopOnce sync.Once

	go func() {
		defer close(stopped)
		for {
			select {
			case sig := <-signals:
				if sig == nil {
					continue
				}
				if state.record(sig, store) {
					cancel(state.interruption)
					continue
				}
				escalate(sig)
				return
			case <-done:
				return
			}
		}
	}()

	stop := func() {
		stopOnce.Do(func() {
			stopNotify()
			close(done)
			<-stopped
			cancel(context.Canceled)
		})
	}
	return ctx, state, stop
}
