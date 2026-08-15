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
		{
			Mode:       WaitModeEvent,
			EventKey:   "bpms_instance_change",
			MatchField: "process_instance_id",
			Terminal: map[string]ResultOutcome{
				"COMPLETED": ResultOutcomeSuccess,
			},
		},
		{
			Mode:        WaitModeAuto,
			PollCommand: "doc export get",
			StatusQuery: "status",
			EventKey:    "export_finished",
			MatchField:  "job_id",
			Terminal: map[string]ResultOutcome{
				"SUCCESS": ResultOutcomeSuccess,
				"FAILED":  ResultOutcomeFailure,
			},
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
			Mode:       WaitModeEvent,
			MatchField: "id",
			Terminal:   terminal,
		},
		"event without match_field": {
			Mode:     WaitModeEvent,
			EventKey: "bpms_instance_change",
			Terminal: terminal,
		},
		"no terminal states": {
			Mode:        WaitModePoll,
			PollCommand: "oa approval-instance get",
			StatusQuery: "status",
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
