// Package clisignal shares process interruption semantics between CLI entrypoints.
package clisignal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

var (
	escalateFindProcess = os.FindProcess
	escalateExit        = os.Exit
)

func Escalate(sig os.Signal) {
	signal.Reset(sig)
	Redeliver(sig, escalateFindProcess, escalateExit)
}

// Redeliver asks the current process to handle the second signal
// with the platform's default semantics. Platforms that cannot deliver the
// requested signal through os.Process.Signal fall back to the conventional
// CLI exit status instead of leaving the process running after escalation.
func Redeliver(sig os.Signal, findProcess func(int) (*os.Process, error), exitProcess func(int)) {
	process, err := findProcess(os.Getpid())
	if err == nil {
		err = process.Signal(sig)
	}
	if err != nil {
		exitProcess(ExitCode(sig))
	}
}

func ExitCode(sig os.Signal) int {
	if sig == syscall.SIGTERM {
		return 143
	}
	return 130
}

// NewInterruption retains the first signal as the primary cancellation cause.
func NewInterruption(sig os.Signal) *Interruption { return &Interruption{signal: sig} }

type Interruption struct {
	signal os.Signal
	detail error
}

func (e *Interruption) Error() string {
	message := fmt.Sprintf("process interrupted by %s", e.signal)
	if e.detail != nil {
		return fmt.Sprintf("%s: %v", message, e.detail)
	}
	return message
}

func (e *Interruption) Unwrap() error { return context.Canceled }

// WithCancellationDetail keeps the signal as the primary process error while
// retaining actionable context from a command that stopped because of that
// signal. Plain context cancellation and nested signal errors add no useful
// detail, and unrelated command failures must not be relabelled as part of the
// interruption. The detail is deliberately not exposed through Unwrap so an
// inner structured error cannot override the signal's exit code or subtype.
func (e *Interruption) WithCancellationDetail(err error) *Interruption {
	if e == nil || err == nil || err == context.Canceled || !errors.Is(err, context.Canceled) {
		return e
	}
	var interrupted *Interruption
	if errors.As(err, &interrupted) {
		return e
	}
	return &Interruption{signal: e.signal, detail: err}
}

func (e *Interruption) ExitCode() int {
	return ExitCode(e.signal)
}

func (e *Interruption) Subtype() string {
	if e.signal == syscall.SIGTERM {
		return "terminated"
	}
	return "cancelled_by_user"
}

// State records the first signal and the publication state observed with it.
// The zero value is ready for use.
type State struct {
	mu                       sync.Mutex
	interruption             *Interruption
	primaryCompletedAtSignal bool
}

func (s *State) Record(sig os.Signal, primaryCompleted func() bool) (first bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.interruption != nil {
		return false
	}
	if primaryCompleted != nil {
		s.primaryCompletedAtSignal = primaryCompleted()
	}
	s.interruption = NewInterruption(sig)
	return true
}

func (s *State) Outcome() (*Interruption, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.interruption, s.primaryCompletedAtSignal
}

func Install(parent context.Context, primaryCompleted func() bool) (context.Context, *State, func()) {
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	return Manage(parent, primaryCompleted, signals, func() { signal.Stop(signals) }, Escalate)
}

func Manage(
	parent context.Context,
	primaryCompleted func() bool,
	signals <-chan os.Signal,
	stopNotify func(),
	escalate func(os.Signal),
) (context.Context, *State, func()) {
	ctx, cancel := context.WithCancelCause(parent)
	state := &State{}
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
				if state.Record(sig, primaryCompleted) {
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
