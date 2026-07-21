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

package app

import (
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
)

// TestCalendarEventListParamConceptPilotBehaviorUnchanged is the P2 pilot
// behaviour-preservation gate. The hand-written hidden spelling variants on
// `dws calendar event list` (start-time / min-time / end-time / max-results /
// next-cursor / calendar / ...) were removed from the Cobra command and now
// live only in the reviewed concept dictionary. This test drives the REAL
// PreParse pipeline (newPipelineEngine → AliasHandler + SemanticAliasHandler
// wired to cli.LookupParamAlias) exactly the way root.go's RunPreParse does,
// then proves the corrected argv still parses onto the canonical flag with the
// original value intact — i.e. every previously-accepted spelling keeps
// working end to end even though its dedicated hidden flag is gone.
func TestCalendarEventListParamConceptPilotBehaviorUnchanged(t *testing.T) {
	engine := newPipelineEngine()

	cases := []struct {
		emitted   string // spelling the model/user typed (dedicated flag removed)
		value     string
		canonical string // real flag the pipeline must reduce it to
		isInt     bool
	}{
		// time_start concept members + scoped aliases → --start
		{"start-time", "2026-03-10T14:00:00+08:00", "start", false},
		{"startTime", "2026-03-10T14:00:00+08:00", "start", false},
		{"start_time", "2026-03-10T14:00:00+08:00", "start", false},
		{"start-date", "2026-03-10T14:00:00+08:00", "start", false},
		{"min-time", "2026-03-10T14:00:00+08:00", "start", false},
		{"time-min", "2026-03-10T14:00:00+08:00", "start", false},
		{"from", "2026-03-10T14:00:00+08:00", "start", false},
		{"since", "2026-03-10T14:00:00+08:00", "start", false},
		{"date", "2026-03-10T14:00:00+08:00", "start", false},
		// end family (scoped aliases; no global time_end concept) → --end
		{"end-time", "2026-03-10T18:00:00+08:00", "end", false},
		{"endTime", "2026-03-10T18:00:00+08:00", "end", false},
		{"end-date", "2026-03-10T18:00:00+08:00", "end", false},
		{"max-time", "2026-03-10T18:00:00+08:00", "end", false},
		{"time-max", "2026-03-10T18:00:00+08:00", "end", false},
		// pagination_size concept members → --limit (int)
		{"max-results", "50", "limit", true},
		{"maxResults", "50", "limit", true},
		{"page-size", "50", "limit", true},
		{"size", "50", "limit", true},
		// page_cursor concept members → --cursor
		{"next-cursor", "TOKEN123", "cursor", false},
		{"nextCursor", "TOKEN123", "cursor", false},
		{"page-token", "TOKEN123", "cursor", false},
		{"next-token", "TOKEN123", "cursor", false},
		// morphology / scoped alias → --calendar-id
		{"calendar", "primary", "calendar-id", false},
		{"calendarId", "primary", "calendar-id", false},
	}

	for _, tc := range cases {
		t.Run(tc.emitted, func(t *testing.T) {
			// Fresh command tree per case: ParseFlags mutates flag state.
			root := NewRootCommand()
			target := mustFindCommand(t, root, "calendar", "event", "list")

			ctx := &pipeline.Context{
				Args:      []string{"calendar", "event", "list", "--" + tc.emitted, tc.value},
				Command:   target.CommandPath(),
				FlagSpecs: pipeline.FlagInfoFromCommand(target),
			}
			if err := engine.RunPhase(pipeline.PreParse, ctx); err != nil {
				t.Fatalf("PreParse error = %v", err)
			}

			joined := strings.Join(ctx.Args, " ")
			if !strings.Contains(joined, "--"+tc.canonical+" "+tc.value) {
				t.Fatalf("--%s not reduced to --%s: args = %v", tc.emitted, tc.canonical, ctx.Args)
			}
			// Token-level check so a canonical that is a prefix of the emitted
			// spelling (e.g. --calendar → --calendar-id) is not a false match.
			if tc.emitted != tc.canonical {
				for _, a := range ctx.Args {
					if !strings.HasPrefix(a, "--") {
						continue
					}
					bare := strings.SplitN(strings.TrimPrefix(a, "--"), "=", 2)[0]
					if bare == tc.emitted {
						t.Fatalf("emitted spelling --%s survived after reduction: args = %v", tc.emitted, ctx.Args)
					}
				}
			}

			// Behaviour unchanged: Cobra parses the corrected argv onto the
			// canonical flag, and the original value lands unmodified.
			flagArgs := ctx.Args[3:]
			if err := target.ParseFlags(flagArgs); err != nil {
				t.Fatalf("Cobra ParseFlags(%v) error = %v", flagArgs, err)
			}
			if tc.isInt {
				got, err := target.Flags().GetInt(tc.canonical)
				if err != nil || got != 50 {
					t.Fatalf("flag --%s = %d (err %v), want 50", tc.canonical, got, err)
				}
			} else {
				got, err := target.Flags().GetString(tc.canonical)
				if err != nil || got != tc.value {
					t.Fatalf("flag --%s = %q (err %v), want %q", tc.canonical, got, err, tc.value)
				}
			}
		})
	}
}

// TestCalendarEventListKeepsCountExclusion pins the reviewed decision that
// pagination_size deliberately excludes --count (count != limit). The kept
// hidden --count flag must be left untouched by the pipeline: it is a real
// flag, not a concept member, so it must not be rewritten to --limit.
func TestCalendarEventListKeepsCountExclusion(t *testing.T) {
	engine := newPipelineEngine()
	root := NewRootCommand()
	target := mustFindCommand(t, root, "calendar", "event", "list")

	ctx := &pipeline.Context{
		Args:      []string{"calendar", "event", "list", "--count", "5"},
		Command:   target.CommandPath(),
		FlagSpecs: pipeline.FlagInfoFromCommand(target),
	}
	if err := engine.RunPhase(pipeline.PreParse, ctx); err != nil {
		t.Fatalf("PreParse error = %v", err)
	}
	if joined := strings.Join(ctx.Args, " "); !strings.Contains(joined, "--count 5") {
		t.Fatalf("--count must not be rewritten: args = %v", ctx.Args)
	}
	if len(ctx.Corrections) != 0 {
		t.Fatalf("--count triggered corrections %#v, want none", ctx.Corrections)
	}
	if err := target.ParseFlags(ctx.Args[3:]); err != nil {
		t.Fatalf("Cobra ParseFlags error = %v", err)
	}
	if got, _ := target.Flags().GetInt("count"); got != 5 {
		t.Fatalf("flag --count = %d, want 5", got)
	}
}
