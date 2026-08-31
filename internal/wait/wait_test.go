// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package wait

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
)

func loopSpec() LoopSpec {
	return LoopSpec{
		StatusQuery: "result.status",
		Terminal: map[string]contract.ResultOutcome{
			"COMPLETED": contract.ResultOutcomeSuccess,
			"REJECTED":  contract.ResultOutcomeFailure,
		},
		Pending:  []string{"NEW", "RUNNING"},
		Interval: time.Millisecond,
	}
}

func TestExtractStatusResolvesDottedPaths(t *testing.T) {
	doc := PollDoc{
		"result": map[string]any{
			"instance": map[string]any{"status": "RUNNING"},
			"count":    float64(3),
		},
	}
	if status, ok := ExtractStatus(doc, "result.instance.status"); !ok || status != "RUNNING" {
		t.Fatalf("status=%q ok=%v", status, ok)
	}
	if status, ok := ExtractStatus(doc, "result.count"); !ok || status != "3" {
		t.Fatalf("numeric status=%q ok=%v", status, ok)
	}
	if _, ok := ExtractStatus(doc, "result.missing"); ok {
		t.Fatal("missing path resolved")
	}
	if _, ok := ExtractStatus(doc, "result.instance.status.deep"); ok {
		t.Fatal("descending into a scalar resolved")
	}
	if _, ok := ExtractStatus(doc, ""); ok {
		t.Fatal("empty query resolved")
	}
}

func TestRunReturnsTerminalOnFirstPoll(t *testing.T) {
	polls := 0
	outcome, err := Run(context.Background(), loopSpec(), func(context.Context) (PollDoc, error) {
		polls++
		return PollDoc{"result": map[string]any{"status": "COMPLETED"}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if polls != 1 || outcome.Attempts != 1 {
		t.Fatalf("polls=%d attempts=%d", polls, outcome.Attempts)
	}
	if outcome.Outcome != contract.ResultOutcomeSuccess || outcome.Status != "COMPLETED" {
		t.Fatalf("outcome=%s status=%s", outcome.Outcome, outcome.Status)
	}
}

func TestRunPollsUntilTerminal(t *testing.T) {
	seen := []string{"NEW", "RUNNING", "RUNNING", "COMPLETED"}
	index := 0
	outcome, err := Run(context.Background(), loopSpec(), func(context.Context) (PollDoc, error) {
		status := seen[index]
		index++
		return PollDoc{"result": map[string]any{"status": status}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Attempts != len(seen) || outcome.Outcome != contract.ResultOutcomeSuccess {
		t.Fatalf("attempts=%d outcome=%s", outcome.Attempts, outcome.Outcome)
	}
}

func TestRunTimesOutAsPendingDuringWait(t *testing.T) {
	spec := loopSpec()
	spec.Timeout = 5 * time.Millisecond
	polls := 0
	outcome, err := Run(context.Background(), spec, func(context.Context) (PollDoc, error) {
		polls++
		return PollDoc{"result": map[string]any{"status": "RUNNING"}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.TimedOut || outcome.Outcome != contract.ResultOutcomePending {
		t.Fatalf("timedOut=%v outcome=%s", outcome.TimedOut, outcome.Outcome)
	}
	if outcome.Status != "RUNNING" {
		t.Fatalf("status=%q, want last observed", outcome.Status)
	}
	if polls == 0 {
		t.Fatal("timeout during wait must still have polled at least once")
	}
}

func TestRunTimesOutAsPendingWhenPollerRespectsDeadline(t *testing.T) {
	spec := loopSpec()
	spec.Timeout = 5 * time.Millisecond
	polls := 0
	// A context-aware poller: blocks until the deadline, then reports the
	// cancellation as an error — the loop must close it as timed-out pending,
	// never as a poll failure.
	outcome, err := Run(context.Background(), spec, func(ctx context.Context) (PollDoc, error) {
		polls++
		<-ctx.Done()
		return nil, ctx.Err()
	})
	if err != nil {
		t.Fatalf("deadline during poll closed as error: %v", err)
	}
	if !outcome.TimedOut || outcome.Outcome != contract.ResultOutcomePending {
		t.Fatalf("timedOut=%v outcome=%s", outcome.TimedOut, outcome.Outcome)
	}
}

func TestRunTimesOutBeforeFirstPoll(t *testing.T) {
	// An already-expired deadline is our own timeout: close as timed-out pending.
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	outcome, err := Run(ctx, loopSpec(), func(context.Context) (PollDoc, error) {
		t.Fatal("poller ran on an expired context")
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.TimedOut || outcome.Attempts != 0 || outcome.Outcome != contract.ResultOutcomePending {
		t.Fatalf("timedOut=%v attempts=%d outcome=%s", outcome.TimedOut, outcome.Attempts, outcome.Outcome)
	}
}

func TestRunPropagatesParentCancellation(t *testing.T) {
	// A cancelled parent context (e.g. Ctrl-C) must propagate the cancellation
	// error, not be swallowed as a timed-out pending success.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Run(ctx, loopSpec(), func(context.Context) (PollDoc, error) {
		t.Fatal("poller ran on a cancelled context")
		return nil, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v, want context.Canceled", err)
	}
}

func TestRunPropagatesParentCancellationDuringWait(t *testing.T) {
	// Cancellation arriving between polls must propagate, not time out.
	ctx, cancel := context.WithCancel(context.Background())
	polls := 0
	go func() {
		time.Sleep(3 * time.Millisecond)
		cancel()
	}()
	spec := loopSpec()
	spec.Interval = time.Hour // force the loop into the wait-between-polls select
	_, err := Run(ctx, spec, func(context.Context) (PollDoc, error) {
		polls++
		return PollDoc{"result": map[string]any{"status": "RUNNING"}}, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v, want context.Canceled", err)
	}
}

func TestRunEventPropagatesParentCancellation(t *testing.T) {
	// Event path: a cancelled parent context must propagate, not time out.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stream := &fakeEventStream{block: true}
	_, err := RunEvent(ctx, EventLoopSpec{
		StatusQuery: "status",
		MatchField:  "id",
		Terminal:    map[string]contract.ResultOutcome{"DONE": contract.ResultOutcomeSuccess},
		Pending:     []string{"RUNNING"},
	}, "res-1", stream)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v, want context.Canceled", err)
	}
}

func TestRunFailsClosedOnUnknownStatus(t *testing.T) {
	_, err := Run(context.Background(), loopSpec(), func(context.Context) (PollDoc, error) {
		return PollDoc{"result": map[string]any{"status": "Mystery"}}, nil
	})
	if !IsUnknownStatus(err) {
		t.Fatalf("err=%v want unknown-status", err)
	}
}

func TestRunFailsOnMissingStatusQuery(t *testing.T) {
	_, err := Run(context.Background(), loopSpec(), func(context.Context) (PollDoc, error) {
		return PollDoc{"unexpected": true}, nil
	})
	if err == nil || !errors.Is(err, err) {
		t.Fatalf("err=%v", err)
	}
}

func TestRunPropagatesPollerError(t *testing.T) {
	boom := errors.New("rpc down")
	_, err := Run(context.Background(), loopSpec(), func(context.Context) (PollDoc, error) {
		return nil, boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err=%v", err)
	}
}

func TestUnknownStatusErrorCarriesStatusAndQuery(t *testing.T) {
	err := &ErrUnknownStatus{Status: "Mystery", Query: "result.status"}
	message := err.Error()
	if !strings.Contains(message, "Mystery") || !strings.Contains(message, "result.status") {
		t.Fatalf("message=%q", message)
	}
}

func TestRunAppliesDefaultIntervalWhenUnset(t *testing.T) {
	spec := loopSpec()
	spec.Interval = 0
	polls := 0
	// Terminal on the second poll forces one interval wait; with Interval=0
	// the loop must still work using DefaultPollInterval (not spin/panic).
	_, err := Run(context.Background(), spec, func(context.Context) (PollDoc, error) {
		polls++
		if polls == 1 {
			return PollDoc{"result": map[string]any{"status": "NEW"}}, nil
		}
		return PollDoc{"result": map[string]any{"status": "COMPLETED"}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if polls != 2 {
		t.Fatalf("polls=%d", polls)
	}
}

func TestRunTimesOutDuringWaitBetweenPolls(t *testing.T) {
	spec := loopSpec()
	spec.Interval = time.Hour // the deadline wins long before the next poll
	spec.Timeout = 5 * time.Millisecond
	outcome, err := Run(context.Background(), spec, func(context.Context) (PollDoc, error) {
		return PollDoc{"result": map[string]any{"status": "RUNNING"}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.TimedOut || outcome.Outcome != contract.ResultOutcomePending || outcome.Status != "RUNNING" {
		t.Fatalf("outcome=%+v", outcome)
	}
}

type stringStatus string

func (s stringStatus) String() string { return string(s) }

func TestExtractStatusCoversScalarShapes(t *testing.T) {
	doc := PollDoc{
		"result": map[string]any{
			"flag":     true,
			"small":    7,
			"big":      int64(9007199254740993),
			"fraction": 1.5,
			"custom":   stringStatus("CUSTOM"),
			"nested":   map[string]any{"deep": "x"},
		},
	}
	cases := map[string]string{
		"result.flag":     "true",
		"result.small":    "7",
		"result.big":      "9007199254740993",
		"result.fraction": "1.5",
		"result.custom":   "CUSTOM",
	}
	for query, want := range cases {
		if got, ok := ExtractStatus(doc, query); !ok || got != want {
			t.Fatalf("query=%s got=%q ok=%v want=%q", query, got, ok, want)
		}
	}
	if _, ok := ExtractStatus(doc, "result.nested"); ok {
		t.Fatal("non-scalar nested map must not resolve")
	}
	if _, ok := ExtractStatus(doc, "result..flag"); ok {
		t.Fatal("empty segment must not resolve")
	}
}

func TestNextIntervalCapsAtMax(t *testing.T) {
	if got := nextInterval(MaxPollInterval); got != MaxPollInterval {
		t.Fatalf("nextInterval(max)=%s", got)
	}
	if got := nextInterval(10 * time.Millisecond); got != 15*time.Millisecond {
		t.Fatalf("nextInterval(10ms)=%s", got)
	}
}

type fakeEventStream struct {
	events []PollDoc
	err    error // returned after events are exhausted (nil = clean end)
	block  bool  // hold until the context deadline
}

func (f *fakeEventStream) Recv(ctx context.Context) (PollDoc, error) {
	if f.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if len(f.events) > 0 {
		doc := f.events[0]
		f.events = f.events[1:]
		return doc, nil
	}
	if f.err != nil {
		return nil, f.err
	}
	return nil, io.EOF
}

func eventLoopSpec() EventLoopSpec {
	return EventLoopSpec{
		StatusQuery: "result.status",
		MatchField:  "process_instance_id",
		Terminal: map[string]contract.ResultOutcome{
			"COMPLETED": contract.ResultOutcomeSuccess,
			"REJECTED":  contract.ResultOutcomeFailure,
		},
		Pending: []string{"RUNNING"},
	}
}

func approvalEvent(instance, status string) PollDoc {
	return PollDoc{"process_instance_id": instance, "result": map[string]any{"status": status}}
}

func TestRunEventReturnsCorrelatedTerminal(t *testing.T) {
	stream := &fakeEventStream{events: []PollDoc{
		approvalEvent("other-instance", "COMPLETED"), // other resource: ignored
		approvalEvent("job-1", "RUNNING"),            // correlated pending: kept waiting
		approvalEvent("job-1", "COMPLETED"),
	}}
	outcome, err := RunEvent(context.Background(), eventLoopSpec(), "job-1", stream)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Outcome != contract.ResultOutcomeSuccess || outcome.Status != "COMPLETED" {
		t.Fatalf("outcome=%+v", outcome)
	}
}

func TestRunEventFailsClosedOnUnknownCorrelatedStatus(t *testing.T) {
	stream := &fakeEventStream{events: []PollDoc{approvalEvent("job-1", "Mystery")}}
	_, err := RunEvent(context.Background(), eventLoopSpec(), "job-1", stream)
	if !IsUnknownStatus(err) {
		t.Fatalf("err=%v", err)
	}
}

func TestRunEventRejectsCorrelatedEventWithoutStatus(t *testing.T) {
	stream := &fakeEventStream{events: []PollDoc{
		{"process_instance_id": "job-1"}, // correlated but no status document
	}}
	_, err := RunEvent(context.Background(), eventLoopSpec(), "job-1", stream)
	if err == nil || !strings.Contains(err.Error(), "status query") {
		t.Fatalf("err=%v", err)
	}
}

func TestRunEventStreamEndSurfacesFallbackSentinel(t *testing.T) {
	stream := &fakeEventStream{events: []PollDoc{approvalEvent("job-1", "RUNNING")}}
	_, err := RunEvent(context.Background(), eventLoopSpec(), "job-1", stream)
	if !errors.Is(err, ErrEventStreamEnded) {
		t.Fatalf("err=%v, want ErrEventStreamEnded", err)
	}

	failing := &fakeEventStream{err: errors.New("transport reset")}
	_, err = RunEvent(context.Background(), eventLoopSpec(), "job-1", failing)
	if !errors.Is(err, ErrEventStreamEnded) {
		t.Fatalf("err=%v, want ErrEventStreamEnded wrapping the transport error", err)
	}
}

func TestRunEventTimesOutAsPendingWhileBlocked(t *testing.T) {
	spec := eventLoopSpec()
	spec.Timeout = 5 * time.Millisecond
	stream := &fakeEventStream{block: true}
	outcome, err := RunEvent(context.Background(), spec, "job-1", stream)
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.TimedOut || outcome.Outcome != contract.ResultOutcomePending {
		t.Fatalf("outcome=%+v", outcome)
	}
}
