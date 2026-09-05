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

package output

import (
	"strings"
	"testing"
)

func pendingAcceptedResult() CommandResult {
	return Pending(map[string]any{"id": "job-1"}, &OperationInfo{
		ID:          "job-1",
		State:       "NEW",
		NextCommand: "dws wait-sample --id job-1",
	})
}

func TestWithOutcomeClosesSuccessPreservingDataAndMeta(t *testing.T) {
	result := WithOutcome(pendingAcceptedResult(), OutcomeSuccess)
	if result.Outcome() != OutcomeSuccess || result.ExitCode() != 0 {
		t.Fatalf("outcome=%s exit=%d", result.Outcome(), result.ExitCode())
	}
	env := result.envelope()
	if env.Meta == nil || env.Meta.Operation == nil {
		t.Fatal("operation info lost on success close")
	}
	if err := ValidateResult(result); err != nil {
		t.Fatal(err)
	}
}

func TestWithOutcomeFailureDropsDataAndCarriesError(t *testing.T) {
	result := WithOutcome(pendingAcceptedResult(), OutcomeFailure, WithErrorInfo(&ErrorInfo{
		Type:    "wait",
		Subtype: "terminal_failure",
		Message: "等待到达失败终态：REJECTED",
	}))
	if result.Outcome() != OutcomeFailure || result.ExitCode() != 8 {
		t.Fatalf("outcome=%s exit=%d, want failure/8", result.Outcome(), result.ExitCode())
	}
	env := result.envelope()
	if env.Data != nil {
		t.Fatal("failure close must drop data (I3)")
	}
	if env.Error == nil || env.Error.Type != "wait" {
		t.Fatal("failure close must carry error info (I3)")
	}
	if err := ValidateResult(result); err != nil {
		t.Fatal(err)
	}
}

func TestWithOperationTimedOutMarksStateAndKeepsResumeFacts(t *testing.T) {
	result := WithOutcome(pendingAcceptedResult(), OutcomePending, WithOperationTimedOut("RUNNING"))
	env := result.envelope()
	op := env.Meta.Operation
	if op.State != "RUNNING" || !op.TimedOut || op.ID != "job-1" || op.NextCommand != "dws wait-sample --id job-1" {
		t.Fatalf("operation=%+v", op)
	}
	if err := ValidateResult(result); err != nil {
		t.Fatal(err)
	}
}

func TestWithOperationTimedOutEmptyStatePreservesExisting(t *testing.T) {
	result := WithOutcome(pendingAcceptedResult(), OutcomePending, WithOperationTimedOut(""))
	op := result.envelope().Meta.Operation
	if op.State != "NEW" || !op.TimedOut {
		t.Fatalf("operation=%+v, want original state kept and timed_out set", op)
	}
	if err := ValidateResult(result); err != nil {
		t.Fatal(err)
	}
}

func TestWithOperationTimedOutWithoutOperationInfoLeavesEnvelopeUntouched(t *testing.T) {
	// A result without operation info keeps nil — ValidateResult must then
	// reject the pending envelope instead of the option synthesizing fake
	// resume facts.
	result := WithOutcome(Success(map[string]any{"ok": true}), OutcomePending, WithOperationTimedOut("RUNNING"))
	env := result.envelope()
	if env.Meta != nil && env.Meta.Operation != nil {
		t.Fatalf("operation=%+v, want untouched", env.Meta.Operation)
	}
	err := ValidateResult(result)
	if err == nil || !strings.Contains(err.Error(), "meta.operation") {
		t.Fatalf("err=%v, want pending-requires-operation rejection", err)
	}
}

func TestDataAccessorReturnsDeepCopy(t *testing.T) {
	result := pendingAcceptedResult()
	data, ok := result.Data().(map[string]any)
	if !ok {
		t.Fatalf("data=%#v", result.Data())
	}
	data["id"] = "mutated"
	again := result.Data().(map[string]any)
	if again["id"] != "job-1" {
		t.Fatalf("Data() aliased internal state: %#v", again)
	}
}

func TestWithOperationTerminalStateSyncsStateAndClearsTimedOut(t *testing.T) {
	for _, tc := range []struct {
		name    string
		outcome Outcome
	}{
		{"success", OutcomeSuccess},
		{"failure", OutcomeFailure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := []ResultOption{WithOperationTerminalState("COMPLETED")}
			if tc.outcome == OutcomeFailure {
				opts = append(opts, WithErrorInfo(&ErrorInfo{
					Type: "wait", Subtype: "terminal_failure", Message: "等待到达失败终态：COMPLETED",
				}))
			}
			result := WithOutcome(pendingAcceptedResult(), tc.outcome, opts...)
			op := result.envelope().Meta.Operation
			if op.State != "COMPLETED" || op.TimedOut {
				t.Fatalf("operation=%+v, want terminal state synced and timed_out cleared", op)
			}
			if op.ID != "job-1" || op.NextCommand != "dws wait-sample --id job-1" {
				t.Fatalf("operation=%+v, want resume identity facts preserved", op)
			}
			if err := ValidateResult(result); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestWithOperationTerminalStateBlankKeepsObservedState(t *testing.T) {
	// A terminal observation that carries no status must keep the last known
	// state rather than wipe it: operation.state may never be emptied by a
	// close transition.
	result := WithOutcome(pendingAcceptedResult(), OutcomeSuccess, WithOperationTerminalState("  "))
	op := result.envelope().Meta.Operation
	if op.State != "NEW" || op.TimedOut {
		t.Fatalf("operation=%+v, want original state kept and timed_out cleared", op)
	}
	if err := ValidateResult(result); err != nil {
		t.Fatal(err)
	}
}

func TestWithOperationTerminalStateWithoutOperationInfoLeavesEnvelopeUntouched(t *testing.T) {
	result := WithOutcome(Success(map[string]any{"ok": true}), OutcomeSuccess, WithOperationTerminalState("COMPLETED"))
	env := result.envelope()
	if env.Meta != nil && env.Meta.Operation != nil {
		t.Fatalf("operation=%+v, want untouched", env.Meta.Operation)
	}
}
