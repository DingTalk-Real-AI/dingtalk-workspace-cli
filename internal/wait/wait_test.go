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
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
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
	testseam.Swap(t, &sleep, func(time.Duration) {})
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

func TestRunTimesOutAsPending(t *testing.T) {
	testseam.Swap(t, &sleep, func(time.Duration) {})
	spec := loopSpec()
	spec.Timeout = time.Millisecond
	outcome, err := Run(context.Background(), spec, func(context.Context) (PollDoc, error) {
		return PollDoc{"result": map[string]any{"status": "RUNNING"}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.TimedOut || outcome.Outcome != contract.ResultOutcomePending {
		t.Fatalf("timedOut=%v outcome=%s", outcome.TimedOut, outcome.Outcome)
	}
}

func TestRunFailsClosedOnUnknownStatus(t *testing.T) {
	testseam.Swap(t, &sleep, func(time.Duration) {})
	_, err := Run(context.Background(), loopSpec(), func(context.Context) (PollDoc, error) {
		return PollDoc{"result": map[string]any{"status": "Mystery"}}, nil
	})
	if !IsUnknownStatus(err) {
		t.Fatalf("err=%v want unknown-status", err)
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
