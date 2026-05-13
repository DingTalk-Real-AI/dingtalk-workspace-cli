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

package handlers

import (
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
)

// TestFullPreParsePipeline exercises the complete PreParse handler
// chain: AliasHandler → StickyHandler → ParamNameHandler. It
// simulates a model-generated CLI invocation with multiple errors
// and verifies the pipeline corrects all of them in one pass.
func TestFullPreParsePipeline(t *testing.T) {
	engine := pipeline.NewEngine()
	engine.RegisterAll(
		AliasHandler{},
		StickyHandler{},
		ParamNameHandler{},
	)

	// Numeric / boolean flag typing matters for the sticky guard. The
	// helper below declares known flags as int so digit-led suffixes
	// pass the suffixLooksLikeValue check.
	intSpecs := func(names ...string) []pipeline.FlagInfo {
		out := make([]pipeline.FlagInfo, len(names))
		for i, n := range names {
			out[i] = pipeline.FlagInfo{Name: n, Type: "int"}
		}
		return out
	}

	tests := []struct {
		name        string
		args        []string
		flags       []pipeline.FlagInfo
		want        string
		corrections int
	}{
		{
			name:        "camelCase + sticky combined",
			args:        []string{"--userId", "123", "--pageSize50"},
			flags:       intSpecs("user-id", "page-size"),
			want:        "--user-id 123 --page-size 50",
			corrections: 2, // alias(userId) + sticky(pageSize50)
		},
		{
			name:        "camelCase + typo combined",
			args:        []string{"--userId", "123", "--limt", "10"},
			flags:       intSpecs("user-id", "limit"),
			want:        "--user-id 123 --limit 10",
			corrections: 2, // alias(userId) + fuzzy(limt)
		},
		{
			name:        "triple error: case + sticky + typo",
			args:        []string{"--UserName", "alice", "--limit100", "--offse", "0"},
			flags:       append(flagSpecs("user-name"), intSpecs("limit", "offset")...),
			want:        "--user-name alice --limit 100 --offset 0",
			corrections: 3,
		},
		{
			name:        "snake_case + sticky",
			args:        []string{"--user_id", "42", "--pageSize20"},
			flags:       intSpecs("user-id", "page-size"),
			want:        "--user-id 42 --page-size 20",
			corrections: 2,
		},
		{
			name:        "all correct — zero corrections",
			args:        []string{"--user-id", "123", "--limit", "10"},
			flags:       intSpecs("user-id", "limit"),
			want:        "--user-id 123 --limit 10",
			corrections: 0,
		},
		{
			name:        "UPPER case flags",
			args:        []string{"--USER-ID", "999"},
			flags:       intSpecs("user-id"),
			want:        "--user-id 999",
			corrections: 1,
		},
		{
			name:        "= syntax with camelCase",
			args:        []string{"--userId=123", "--pageSize=50"},
			flags:       intSpecs("user-id", "page-size"),
			want:        "--user-id=123 --page-size=50",
			corrections: 2,
		},
		{
			name:        "camelCase sticky split with normalisation",
			args:        []string{"--limitValue100"},
			flags:       intSpecs("limit-value"),
			want:        "--limit-value 100",
			corrections: 1, // sticky handles both kebab-normalisation and split
		},

		// Hardening: a mistyped flag whose name happens to start with
		// a real flag must NOT be split. The pipeline should leave the
		// token untouched so Cobra can raise "unknown flag".
		{
			name: "typo --starttime1 not split (date-time format)",
			args: []string{"--starttime1", "2026-02-07"},
			flags: []pipeline.FlagInfo{
				{Name: "start", Type: "string", Format: "date-time"},
				{Name: "end", Type: "string", Format: "date-time"},
			},
			want:        "--starttime1 2026-02-07",
			corrections: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &pipeline.Context{
				Args:      append([]string{}, tt.args...),
				FlagSpecs: tt.flags,
			}
			if err := engine.RunPhase(pipeline.PreParse, ctx); err != nil {
				t.Fatalf("RunPhase error: %v", err)
			}
			got := strings.Join(ctx.Args, " ")
			if got != tt.want {
				t.Errorf("Args = %q, want %q", got, tt.want)
			}
			if len(ctx.Corrections) != tt.corrections {
				t.Errorf("Corrections = %d, want %d", len(ctx.Corrections), tt.corrections)
				for _, c := range ctx.Corrections {
					t.Logf("  %s/%s: %q → %q", c.Handler, c.Kind, c.Original, c.Corrected)
				}
			}
		})
	}
}

// TestFullPostParsePipeline exercises the PostParse handler chain
// with the ParamValueHandler normalising multiple value types in a
// single invocation.
func TestFullPostParsePipeline(t *testing.T) {
	engine := pipeline.NewEngine()
	engine.RegisterAll(ParamValueHandler{})

	ctx := &pipeline.Context{
		Command: "aitable.list_records",
		Params: map[string]any{
			"verbose":    "yes",
			"limit":      "1,000",
			"status":     "ACTIVE",
			"created_at": "2024/03/29",
			"name":       "test",
		},
		Schema: map[string]any{
			"properties": map[string]any{
				"verbose":    map[string]any{"type": "boolean"},
				"limit":      map[string]any{"type": "integer"},
				"status":     map[string]any{"type": "string", "enum": []any{"active", "inactive"}},
				"created_at": map[string]any{"type": "string", "format": "date"},
				"name":       map[string]any{"type": "string"},
			},
		},
	}

	if err := engine.RunPhase(pipeline.PostParse, ctx); err != nil {
		t.Fatalf("RunPhase error: %v", err)
	}

	if got := ctx.Params["verbose"]; got != true {
		t.Errorf("verbose = %v, want true", got)
	}
	if got := ctx.Params["limit"]; got != int64(1000) {
		t.Errorf("limit = %v, want 1000", got)
	}
	if got := ctx.Params["status"]; got != "active" {
		t.Errorf("status = %q, want %q", got, "active")
	}
	if got := ctx.Params["created_at"]; got != "2024-03-29" {
		t.Errorf("created_at = %q, want %q", got, "2024-03-29")
	}
	// "name" is a plain string — should be untouched.
	if got := ctx.Params["name"]; got != "test" {
		t.Errorf("name = %q, want %q", got, "test")
	}
	if len(ctx.Corrections) != 4 {
		t.Errorf("Corrections = %d, want 4", len(ctx.Corrections))
	}
}

// TestFullPipelineEndToEnd exercises all phases in sequence:
// PreParse → PostParse, simulating a model invocation that has
// errors at both the argv level and the value level.
func TestFullPipelineEndToEnd(t *testing.T) {
	engine := pipeline.NewEngine()
	engine.RegisterAll(
		AliasHandler{},
		StickyHandler{},
		ParamNameHandler{},
		ParamValueHandler{},
	)

	// Phase 1: PreParse — fix raw argv.
	ctx := &pipeline.Context{
		Args: []string{
			"--userId", "u001",
			"--pageSize50",
			"--verbosetrue",
		},
		FlagSpecs: []pipeline.FlagInfo{
			{Name: "user-id", Type: "string"},
			{Name: "page-size", Type: "int"},
			{Name: "verbose", Type: "bool"},
		},
	}

	if err := engine.RunPhase(pipeline.PreParse, ctx); err != nil {
		t.Fatalf("PreParse error: %v", err)
	}

	want := "--user-id u001 --page-size 50 --verbose true"
	got := strings.Join(ctx.Args, " ")
	if got != want {
		t.Errorf("after PreParse: Args = %q, want %q", got, want)
	}

	preParseCorrections := len(ctx.Corrections)
	if preParseCorrections != 3 {
		t.Errorf("PreParse corrections = %d, want 3", preParseCorrections)
	}

	// Phase 2: PostParse — simulate Cobra having parsed the corrected
	// args into structured params, then normalise values.
	ctx.Command = "chat.send_message"
	ctx.Params = map[string]any{
		"user_id":   "u001",
		"page_size": "50",
		"verbose":   "true",
	}
	ctx.Schema = map[string]any{
		"properties": map[string]any{
			"user_id":   map[string]any{"type": "string"},
			"page_size": map[string]any{"type": "integer"},
			"verbose":   map[string]any{"type": "boolean"},
		},
	}

	if err := engine.RunPhase(pipeline.PostParse, ctx); err != nil {
		t.Fatalf("PostParse error: %v", err)
	}

	if got := ctx.Params["verbose"]; got != true {
		t.Errorf("verbose = %v (%T), want true (bool)", got, got)
	}
	// page_size "50" has no grouping separators — unchanged.
	if got := ctx.Params["page_size"]; got != "50" {
		t.Errorf("page_size = %v, want %q", got, "50")
	}

	totalCorrections := len(ctx.Corrections)
	postParseCorrections := totalCorrections - preParseCorrections
	if postParseCorrections != 1 {
		t.Errorf("PostParse corrections = %d, want 1", postParseCorrections)
	}
}

// TestFullFivePhasePipeline exercises all five phases in order:
// Register → PreParse → PostParse → PreRequest → PostResponse,
// simulating a complete command lifecycle from registration through
// response output.
func TestFullFivePhasePipeline(t *testing.T) {
	engine := pipeline.NewEngine()
	engine.RegisterAll(
		RegisterHandler{},
		AliasHandler{},
		StickyHandler{},
		ParamNameHandler{},
		ParamValueHandler{},
		PreRequestHandler{},
		PostResponseHandler{},
	)

	// Verify all five phases have handlers.
	for _, phase := range []pipeline.Phase{
		pipeline.Register,
		pipeline.PreParse,
		pipeline.PostParse,
		pipeline.PreRequest,
		pipeline.PostResponse,
	} {
		if !engine.HasHandlers(phase) {
			t.Fatalf("engine missing handlers for phase %v", phase)
		}
	}

	// Phase 1: Register — command tree being built.
	ctx := &pipeline.Context{
		Command: "aitable",
	}
	if err := engine.RunPhase(pipeline.Register, ctx); err != nil {
		t.Fatalf("Register error: %v", err)
	}

	// Phase 2: PreParse — fix raw argv.
	ctx.Args = []string{
		"--userId", "u001",
		"--pageSize50",
		"--verbosetrue",
	}
	ctx.FlagSpecs = []pipeline.FlagInfo{
		{Name: "user-id", Type: "string"},
		{Name: "page-size", Type: "int"},
		{Name: "verbose", Type: "bool"},
	}

	if err := engine.RunPhase(pipeline.PreParse, ctx); err != nil {
		t.Fatalf("PreParse error: %v", err)
	}

	want := "--user-id u001 --page-size 50 --verbose true"
	got := strings.Join(ctx.Args, " ")
	if got != want {
		t.Errorf("after PreParse: Args = %q, want %q", got, want)
	}
	preParseCorrections := len(ctx.Corrections)

	// Phase 3: PostParse — simulate Cobra having parsed the corrected
	// args into structured params, then normalise values.
	ctx.Command = "aitable.query_records"
	ctx.Params = map[string]any{
		"user_id":   "u001",
		"page_size": "1,000",
		"verbose":   "yes",
	}
	ctx.Schema = map[string]any{
		"properties": map[string]any{
			"user_id":   map[string]any{"type": "string"},
			"page_size": map[string]any{"type": "integer"},
			"verbose":   map[string]any{"type": "boolean"},
		},
	}

	if err := engine.RunPhase(pipeline.PostParse, ctx); err != nil {
		t.Fatalf("PostParse error: %v", err)
	}

	if got := ctx.Params["verbose"]; got != true {
		t.Errorf("verbose = %v (%T), want true (bool)", got, got)
	}
	if got := ctx.Params["page_size"]; got != int64(1000) {
		t.Errorf("page_size = %v, want 1000", got)
	}
	postParseCorrections := len(ctx.Corrections) - preParseCorrections
	if postParseCorrections != 2 {
		t.Errorf("PostParse corrections = %d, want 2", postParseCorrections)
	}

	// Phase 4: PreRequest — inspect final payload before dispatch.
	ctx.Payload = ctx.Params
	if err := engine.RunPhase(pipeline.PreRequest, ctx); err != nil {
		t.Fatalf("PreRequest error: %v", err)
	}
	// Verify payload was not corrupted.
	if ctx.Payload["user_id"] != "u001" {
		t.Error("PreRequest corrupted Payload")
	}

	// Phase 5: PostResponse — process response before output.
	ctx.Response = map[string]any{
		"records": []any{
			map[string]any{"id": "rec001", "fields": map[string]any{"name": "test"}},
		},
		"total": 1,
	}
	if err := engine.RunPhase(pipeline.PostResponse, ctx); err != nil {
		t.Fatalf("PostResponse error: %v", err)
	}
	// Verify response was not corrupted.
	if ctx.Response["total"] != 1 {
		t.Error("PostResponse corrupted Response")
	}
}

// TestFullFivePhasePipelineWithEngineRun exercises all five phases
// using Engine.Run (single shot) to verify the ordering is correct
// end-to-end.
func TestFullFivePhasePipelineWithEngineRun(t *testing.T) {
	var seq []string

	engine := pipeline.NewEngine()
	engine.RegisterAll(
		&phaseTracker{name: "reg", phase: pipeline.Register, seq: &seq},
		&phaseTracker{name: "pre-parse", phase: pipeline.PreParse, seq: &seq},
		&phaseTracker{name: "post-parse", phase: pipeline.PostParse, seq: &seq},
		&phaseTracker{name: "pre-req", phase: pipeline.PreRequest, seq: &seq},
		&phaseTracker{name: "post-resp", phase: pipeline.PostResponse, seq: &seq},
	)

	ctx := &pipeline.Context{Command: "test.tool"}
	if err := engine.Run(ctx); err != nil {
		t.Fatalf("Engine.Run error: %v", err)
	}

	want := "reg,pre-parse,post-parse,pre-req,post-resp"
	got := strings.Join(seq, ",")
	if got != want {
		t.Errorf("phase execution order = %q, want %q", got, want)
	}
}

// TestFivePhasePipelineCorrectHandlerCounts verifies that the
// production-equivalent engine has the expected handler distribution.
func TestFivePhasePipelineCorrectHandlerCounts(t *testing.T) {
	engine := pipeline.NewEngine()
	engine.RegisterAll(
		RegisterHandler{},
		AliasHandler{},
		StickyHandler{},
		ParamNameHandler{},
		ParamValueHandler{},
		PreRequestHandler{},
		PostResponseHandler{},
	)

	tests := []struct {
		phase pipeline.Phase
		want  int
	}{
		{pipeline.Register, 1},
		{pipeline.PreParse, 3},
		{pipeline.PostParse, 1},
		{pipeline.PreRequest, 1},
		{pipeline.PostResponse, 1},
	}
	for _, tt := range tests {
		if got := len(engine.Handlers(tt.phase)); got != tt.want {
			t.Errorf("Handlers(%v) = %d, want %d", tt.phase, got, tt.want)
		}
	}
	if got := engine.HandlerCount(); got != 7 {
		t.Errorf("HandlerCount = %d, want 7", got)
	}
}

// phaseTracker is a test helper that records its name when Handle is called.
type phaseTracker struct {
	name  string
	phase pipeline.Phase
	seq   *[]string
}

func (h *phaseTracker) Name() string          { return h.name }
func (h *phaseTracker) Phase() pipeline.Phase { return h.phase }
func (h *phaseTracker) Handle(_ *pipeline.Context) error {
	*h.seq = append(*h.seq, h.name)
	return nil
}

// TestPreParseDoesNotBreakValidArgs verifies that valid, correctly
// formatted args pass through the pipeline without modification.
func TestPreParseDoesNotBreakValidArgs(t *testing.T) {
	engine := pipeline.NewEngine()
	engine.RegisterAll(
		AliasHandler{},
		StickyHandler{},
		ParamNameHandler{},
	)

	original := []string{
		"--user-id", "123",
		"--limit", "10",
		"--offset", "0",
		"--format", "json",
		"--verbose",
	}
	ctx := &pipeline.Context{
		Args:      append([]string{}, original...),
		FlagSpecs: flagSpecs("user-id", "limit", "offset", "format", "verbose"),
	}

	if err := engine.RunPhase(pipeline.PreParse, ctx); err != nil {
		t.Fatalf("RunPhase error: %v", err)
	}

	got := strings.Join(ctx.Args, " ")
	want := strings.Join(original, " ")
	if got != want {
		t.Errorf("Args changed!\n  got:  %q\n  want: %q", got, want)
	}
	if len(ctx.Corrections) != 0 {
		t.Errorf("unexpected corrections: %d", len(ctx.Corrections))
		for _, c := range ctx.Corrections {
			t.Logf("  %s: %q → %q", c.Handler, c.Original, c.Corrected)
		}
	}
}
