package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
)

var rootEscalateSignal = func(sig os.Signal) {
	signal.Reset(sig)
	if syscallSig, ok := sig.(syscall.Signal); ok {
		_ = syscall.Kill(os.Getpid(), syscallSig)
	}
}

type processInterruption struct {
	signal os.Signal
}

func (e *processInterruption) Error() string {
	return fmt.Sprintf("process interrupted by %s", e.signal)
}

func (e *processInterruption) Unwrap() error { return context.Canceled }

func (e *processInterruption) ExitCode() int {
	if e.signal == syscall.SIGTERM {
		return 143
	}
	return 130
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
