package clitrack

import (
	"strconv"
	"time"

	"gitlab.alibaba-inc.com/aes/aem-go-sdk/aem"
)

// Execution is a DWS extension for reporting a command that ran in another
// process. ErrorSummary must already be sanitized by the command owner.
type Execution struct {
	Command      string
	ExitCode     int
	Duration     time.Duration
	CompletedAt  time.Time
	ErrorSummary string
}

// ReportExecution queues a completed execution without running business code,
// printing an error, changing stdout, or exiting the process. Standard reserved
// field protection still applies; c5 comes from the execution's error summary.
func (t *Tracker) ReportExecution(execution Execution) error {
	if t.inner == nil {
		return nil
	}
	fields := t.buildFields(execution.ExitCode, execution.Duration, execution.ErrorSummary, "", t.takeAttribution())
	if execution.Command != "" {
		fields["c1"] = execution.Command
	}
	if !execution.CompletedAt.IsZero() {
		fields["ts"] = strconv.FormatInt(execution.CompletedAt.UnixMilli(), 10)
	}
	return t.inner.Track(aem.Event{Type: "event", Fields: fields})
}

// Close drains queued events. This blocking API is a DWS extension for the
// detached sender only; a command process must never call it on its exit path.
func (t *Tracker) Close() error {
	if t.inner == nil {
		return nil
	}
	return t.inner.Close()
}
