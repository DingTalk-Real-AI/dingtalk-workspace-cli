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

package contract

import "testing"

func TestWaitSpecValidateAcceptsReviewedShapes(t *testing.T) {
	cases := []WaitSpec{
		{
			Mode:        WaitModePoll,
			PollCommand: "oa approval-instance get",
			StatusQuery: "result.status",
			Terminal: map[string]ResultOutcome{
				"COMPLETED": ResultOutcomeSuccess,
				"REJECTED":  ResultOutcomeFailure,
			},
			PendingValues:      []string{"NEW", "RUNNING"},
			DefaultTimeoutSecs: 600,
		},
	}
	for i, spec := range cases {
		if err := spec.Validate("sample.tool"); err != nil {
			t.Fatalf("case %d: unexpected error: %v", i, err)
		}
	}
}

func TestWaitSpecValidateRejectsMalformedShapes(t *testing.T) {
	terminal := map[string]ResultOutcome{"COMPLETED": ResultOutcomeSuccess}
	cases := map[string]WaitSpec{
		"no mode": {
			Terminal: terminal,
		},
		"unknown mode": {
			Mode:     "webhook",
			Terminal: terminal,
		},
		"poll without poll_command": {
			Mode:        WaitModePoll,
			StatusQuery: "status",
			Terminal:    terminal,
		},
		"poll without status_query": {
			Mode:        WaitModePoll,
			PollCommand: "oa approval-instance get",
			Terminal:    terminal,
		},
		"event without event_key": {
			Mode:          WaitModeEvent,
			MatchField:    "process_instance_id",
			ResourceQuery: "id",
			StatusQuery:   "result.status",
			Terminal:      terminal,
		},
		"event without match_field": {
			Mode:          WaitModeEvent,
			EventKey:      "bpms_instance_change",
			ResourceQuery: "id",
			StatusQuery:   "result.status",
			Terminal:      terminal,
		},
		"event without resource_query": {
			Mode:        WaitModeEvent,
			EventKey:    "bpms_instance_change",
			MatchField:  "process_instance_id",
			StatusQuery: "result.status",
			Terminal:    terminal,
		},
		"auto missing poll_command": {
			Mode:          WaitModeAuto,
			EventKey:      "export_finished",
			MatchField:    "job_id",
			ResourceQuery: "job_id",
			StatusQuery:   "status",
			Terminal:      terminal,
		},
		"no terminal states": {
			Mode:        WaitModePoll,
			PollCommand: "oa approval-instance get",
			StatusQuery: "status",
		},
		"blank terminal status": {
			Mode:        WaitModePoll,
			PollCommand: "oa approval-instance get",
			StatusQuery: "status",
			Terminal:    map[string]ResultOutcome{" ": ResultOutcomeSuccess},
		},
		"terminal outcome pending": {
			Mode:        WaitModePoll,
			PollCommand: "oa approval-instance get",
			StatusQuery: "status",
			Terminal:    map[string]ResultOutcome{"COMPLETED": ResultOutcomePending, "REJECTED": ResultOutcomeFailure},
		},
		"terminal outcome partial": {
			Mode:        WaitModePoll,
			PollCommand: "oa approval-instance get",
			StatusQuery: "status",
			Terminal:    map[string]ResultOutcome{"COMPLETED": ResultOutcomePartialFailure, "REJECTED": ResultOutcomeFailure},
		},
		"terminal outcome outside closed set": {
			Mode:        WaitModePoll,
			PollCommand: "oa approval-instance get",
			StatusQuery: "status",
			Terminal:    map[string]ResultOutcome{"COMPLETED": ResultOutcome("explosion")},
		},
		"only pending terminal outcome": {
			Mode:        WaitModePoll,
			PollCommand: "oa approval-instance get",
			StatusQuery: "status",
			Terminal:    map[string]ResultOutcome{"NEW": ResultOutcomePending},
		},
		"status both terminal and pending": {
			Mode:          WaitModePoll,
			PollCommand:   "oa approval-instance get",
			StatusQuery:   "status",
			Terminal:      terminal,
			PendingValues: []string{"COMPLETED"},
		},
		"blank pending value": {
			Mode:          WaitModePoll,
			PollCommand:   "oa approval-instance get",
			StatusQuery:   "status",
			Terminal:      terminal,
			PendingValues: []string{"  "},
		},
		"negative timeout default": {
			Mode:               WaitModePoll,
			PollCommand:        "oa approval-instance get",
			StatusQuery:        "status",
			Terminal:           terminal,
			DefaultTimeoutSecs: -1,
		},
	}
	for name, spec := range cases {
		if err := spec.Validate("sample.tool"); err == nil {
			t.Fatalf("%s: expected error, got nil", name)
		}
	}
}

func TestNormalizeWaitSpecTrimsStatusValuesIntoWireForm(t *testing.T) {
	in := &WaitSpec{
		Mode:               "  poll  ",
		PollCommand:        " oa approval-instance get ",
		StatusQuery:        " result.status ",
		Terminal:           map[string]ResultOutcome{" COMPLETED ": ResultOutcomeSuccess, "REJECTED": ResultOutcomeFailure},
		PendingValues:      []string{" NEW ", "RUNNING"},
		DefaultTimeoutSecs: 60,
	}
	out, err := NormalizeWaitSpec(in, "sample.tool")
	if err != nil {
		t.Fatalf("NormalizeWaitSpec() error = %v", err)
	}
	if out.Mode != WaitModePoll || out.PollCommand != "oa approval-instance get" || out.StatusQuery != "result.status" {
		t.Fatalf("normalized scalars: %#v", out)
	}
	if len(out.Terminal) != 2 {
		t.Fatalf("terminal=%#v, want two trimmed keys", out.Terminal)
	}
	if got := out.Terminal["COMPLETED"]; got != ResultOutcomeSuccess {
		t.Fatalf("terminal[COMPLETED]=%q, want success (key must be trimmed)", got)
	}
	if _, padded := out.Terminal[" COMPLETED "]; padded {
		t.Fatal("padded terminal key survived normalization")
	}
	for i, want := range []string{"NEW", "RUNNING"} {
		if out.PendingValues[i] != want {
			t.Fatalf("pending[%d]=%q, want %q", i, out.PendingValues[i], want)
		}
	}
	// The input declaration must stay untouched (defensive copy).
	if _, padded := in.Terminal[" COMPLETED "]; !padded {
		t.Fatal("NormalizeWaitSpec mutated its input terminal map")
	}
	if in.PendingValues[0] != " NEW " {
		t.Fatal("NormalizeWaitSpec mutated its input pending values")
	}
}

func TestNormalizeWaitSpecRejectsDuplicatesAndConflictsAfterTrim(t *testing.T) {
	terminal := map[string]ResultOutcome{"COMPLETED": ResultOutcomeSuccess}
	cases := map[string]*WaitSpec{
		"terminal keys collapsing after trim": {
			Mode:        WaitModePoll,
			PollCommand: "oa approval-instance get",
			StatusQuery: "status",
			Terminal:    map[string]ResultOutcome{"COMPLETED": ResultOutcomeSuccess, " COMPLETED ": ResultOutcomeFailure},
		},
		"pending values collapsing after trim": {
			Mode:          WaitModePoll,
			PollCommand:   "oa approval-instance get",
			StatusQuery:   "status",
			Terminal:      terminal,
			PendingValues: []string{"NEW", " NEW "},
		},
		"terminal/pending conflict hidden by padding": {
			Mode:          WaitModePoll,
			PollCommand:   "oa approval-instance get",
			StatusQuery:   "status",
			Terminal:      terminal,
			PendingValues: []string{" COMPLETED "},
		},
	}
	for name, spec := range cases {
		if _, err := NormalizeWaitSpec(spec, "sample.tool"); err == nil {
			t.Fatalf("%s: expected error, got nil", name)
		}
	}
}

func TestNormalizeWaitSpecNilReturnsNil(t *testing.T) {
	out, err := NormalizeWaitSpec(nil, "sample.tool")
	if err != nil || out != nil {
		t.Fatalf("NormalizeWaitSpec(nil) = %#v, %v", out, err)
	}
}
