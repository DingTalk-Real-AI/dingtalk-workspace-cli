package app

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/clisignal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
)

type processInterruption = clisignal.Interruption
type processSignalState = clisignal.State

var rootEscalateSignal = func(sig os.Signal) {
	signal.Reset(sig)
	redeliverProcessSignal(sig)
}
var (
	rootFindProcess = os.FindProcess
	rootExitProcess = os.Exit
)

func redeliverProcessSignal(sig os.Signal) {
	clisignal.Redeliver(sig, rootFindProcess, rootExitProcess)
}
func interruptionExitCode(sig os.Signal) int { return clisignal.ExitCode(sig) }
func installProcessSignalContext(parent context.Context, store *output.ResultStore) (context.Context, *processSignalState, func()) {
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	return manageProcessSignals(parent, store, signals, func() { signal.Stop(signals) }, rootEscalateSignal)
}
func manageProcessSignals(parent context.Context, store *output.ResultStore, signals <-chan os.Signal, stopNotify func(), escalate func(os.Signal)) (context.Context, *processSignalState, func()) {
	return clisignal.Manage(parent, processResultCompleted(store), signals, stopNotify, escalate)
}

func processResultCompleted(store *output.ResultStore) func() bool {
	return func() bool { _, _, completed, _ := output.StoredEmissionState(store); return completed }
}
