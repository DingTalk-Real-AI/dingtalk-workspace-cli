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

// TestCalendarEventListNativeFallbacksAndCentralAliasesCoexist locks the
// boundary between the command's original hidden compatibility flags and the
// new central semantic normalizer. Existing real flags stay untouched and are
// handled by calendar.go's flagOrFallback chain; only spellings that are not
// real flags (for example --date, --from, and --since) are rewritten centrally.
func TestCalendarEventListNativeFallbacksAndCentralAliasesCoexist(t *testing.T) {
	engine := newPipelineEngine()

	cases := []struct {
		emitted   string
		value     string
		canonical string
		isInt     bool
		native    bool
	}{
		// Existing Calendar compatibility flags remain native.
		{"start-time", "2026-03-10T14:00:00+08:00", "start", false, true},
		{"startTime", "2026-03-10T14:00:00+08:00", "start", false, true},
		{"start_time", "2026-03-10T14:00:00+08:00", "start", false, true},
		{"start-date", "2026-03-10T14:00:00+08:00", "start", false, true},
		{"min-time", "2026-03-10T14:00:00+08:00", "start", false, true},
		{"time-min", "2026-03-10T14:00:00+08:00", "start", false, true},
		{"end-time", "2026-03-10T18:00:00+08:00", "end", false, true},
		{"endTime", "2026-03-10T18:00:00+08:00", "end", false, true},
		{"end-date", "2026-03-10T18:00:00+08:00", "end", false, true},
		{"max-time", "2026-03-10T18:00:00+08:00", "end", false, true},
		{"time-max", "2026-03-10T18:00:00+08:00", "end", false, true},
		{"max-results", "50", "limit", true, true},
		{"maxResults", "50", "limit", true, true},
		{"page-size", "50", "limit", true, true},
		{"size", "50", "limit", true, true},
		{"next-cursor", "TOKEN123", "cursor", false, true},
		{"nextCursor", "TOKEN123", "cursor", false, true},
		{"page-token", "TOKEN123", "cursor", false, true},
		{"next-token", "TOKEN123", "cursor", false, true},
		{"calendar", "primary", "calendar-id", false, true},
		{"calendarId", "primary", "calendar-id", false, true},
		// These spellings have no native Calendar flag and remain central aliases.
		{"from", "2026-03-10T14:00:00+08:00", "start", false, false},
		{"since", "2026-03-10T14:00:00+08:00", "start", false, false},
		{"date", "2026-03-10T14:00:00+08:00", "start", false, false},
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

			parsedFlag := tc.canonical
			if tc.native {
				parsedFlag = tc.emitted
				if len(ctx.Corrections) != 0 {
					t.Fatalf("native --%s triggered central corrections: %#v", tc.emitted, ctx.Corrections)
				}
				if joined := strings.Join(ctx.Args, " "); !strings.Contains(joined, "--"+tc.emitted+" "+tc.value) {
					t.Fatalf("native --%s did not survive unchanged: args = %v", tc.emitted, ctx.Args)
				}
			} else {
				if joined := strings.Join(ctx.Args, " "); !strings.Contains(joined, "--"+tc.canonical+" "+tc.value) {
					t.Fatalf("--%s not reduced to --%s: args = %v", tc.emitted, tc.canonical, ctx.Args)
				}
				if len(ctx.Corrections) != 1 {
					t.Fatalf("central --%s corrections = %#v, want one", tc.emitted, ctx.Corrections)
				}
			}

			flagArgs := ctx.Args[3:]
			if err := target.ParseFlags(flagArgs); err != nil {
				t.Fatalf("Cobra ParseFlags(%v) error = %v", flagArgs, err)
			}
			if tc.isInt {
				got, err := target.Flags().GetInt(parsedFlag)
				if err != nil || got != 50 {
					t.Fatalf("flag --%s = %d (err %v), want 50", parsedFlag, got, err)
				}
			} else {
				got, err := target.Flags().GetString(parsedFlag)
				if err != nil || got != tc.value {
					t.Fatalf("flag --%s = %q (err %v), want %q", parsedFlag, got, err, tc.value)
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
