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

package pipeline

import (
	"errors"
	"strings"
	"testing"
)

// --- test helpers ---

type stubHandler struct {
	name    string
	phase   Phase
	fn      func(*Context) error
	called  bool
	callSeq *[]string
}

func (h *stubHandler) Name() string { return h.name }
func (h *stubHandler) Phase() Phase { return h.phase }
func (h *stubHandler) Handle(ctx *Context) error {
	h.called = true
	if h.callSeq != nil {
		*h.callSeq = append(*h.callSeq, h.name)
	}
	if h.fn != nil {
		return h.fn(ctx)
	}
	return nil
}

func newStub(name string, phase Phase, fn func(*Context) error) *stubHandler {
	return &stubHandler{name: name, phase: phase, fn: fn}
}

func newStubWithSeq(name string, phase Phase, seq *[]string) *stubHandler {
	return &stubHandler{name: name, phase: phase, callSeq: seq}
}

// --- tests ---

func TestNewEngine(t *testing.T) {
	e := NewEngine()
	if e == nil {
		t.Fatal("NewEngine returned nil")
	}
	if got := e.HandlerCount(); got != 0 {
		t.Errorf("HandlerCount = %d, want 0", got)
	}
}

func TestRegisterAndHandlerCount(t *testing.T) {
	e := NewEngine()
	e.Register(newStub("a", PreParse, nil))
	e.Register(newStub("b", PreParse, nil))
	e.Register(newStub("c", PostParse, nil))

	if got := e.HandlerCount(); got != 3 {
		t.Errorf("HandlerCount = %d, want 3", got)
	}
	if got := len(e.Handlers(PreParse)); got != 2 {
		t.Errorf("PreParse handlers = %d, want 2", got)
	}
	if got := len(e.Handlers(PostParse)); got != 1 {
		t.Errorf("PostParse handlers = %d, want 1", got)
	}
	if got := len(e.Handlers(PreRequest)); got != 0 {
		t.Errorf("PreRequest handlers = %d, want 0", got)
	}
}

func TestRegisterAll(t *testing.T) {
	e := NewEngine()
	e.RegisterAll(
		newStub("a", PreParse, nil),
		newStub("b", PostParse, nil),
		newStub("c", PreRequest, nil),
	)
	if got := e.HandlerCount(); got != 3 {
		t.Errorf("HandlerCount = %d, want 3", got)
	}
}

func TestHasHandlers(t *testing.T) {
	e := NewEngine()
	if e.HasHandlers(PreParse) {
		t.Error("HasHandlers(PreParse) = true on empty engine")
	}
	e.Register(newStub("a", PreParse, nil))
	if !e.HasHandlers(PreParse) {
		t.Error("HasHandlers(PreParse) = false after registration")
	}
	if e.HasHandlers(PostResponse) {
		t.Error("HasHandlers(PostResponse) = true, want false")
	}
}

func TestRunPhaseChainOrder(t *testing.T) {
	var seq []string
	e := NewEngine()
	e.Register(newStubWithSeq("first", PreParse, &seq))
	e.Register(newStubWithSeq("second", PreParse, &seq))
	e.Register(newStubWithSeq("third", PreParse, &seq))

	ctx := &Context{Args: []string{"test"}}
	if err := e.RunPhase(PreParse, ctx); err != nil {
		t.Fatalf("RunPhase returned error: %v", err)
	}
	want := "first,second,third"
	got := strings.Join(seq, ",")
	if got != want {
		t.Errorf("execution order = %q, want %q", got, want)
	}
}

func TestRunPhaseContextMutationPropagates(t *testing.T) {
	e := NewEngine()
	e.Register(newStub("append-foo", PreParse, func(ctx *Context) error {
		ctx.Args = append(ctx.Args, "--foo")
		return nil
	}))
	e.Register(newStub("append-bar", PreParse, func(ctx *Context) error {
		ctx.Args = append(ctx.Args, "--bar")
		return nil
	}))

	ctx := &Context{Args: []string{"cmd"}}
	if err := e.RunPhase(PreParse, ctx); err != nil {
		t.Fatalf("RunPhase error: %v", err)
	}
	want := "cmd,--foo,--bar"
	got := strings.Join(ctx.Args, ",")
	if got != want {
		t.Errorf("Args = %q, want %q", got, want)
	}
}

func TestRunPhaseErrorAbortsChain(t *testing.T) {
	boom := errors.New("boom")
	h1 := newStub("ok", PreParse, nil)
	h2 := newStub("fail", PreParse, func(*Context) error { return boom })
	h3 := newStub("never", PreParse, nil)

	e := NewEngine()
	e.RegisterAll(h1, h2, h3)

	ctx := &Context{}
	err := e.RunPhase(PreParse, ctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, boom) {
		t.Errorf("error = %v, want wrapped boom", err)
	}
	if !strings.Contains(err.Error(), "fail") {
		t.Errorf("error should contain handler name, got %q", err.Error())
	}
	if !h1.called {
		t.Error("h1 should have been called")
	}
	if !h2.called {
		t.Error("h2 should have been called")
	}
	if h3.called {
		t.Error("h3 should NOT have been called after error")
	}
}

func TestRunPhaseEmptyPhaseNoError(t *testing.T) {
	e := NewEngine()
	ctx := &Context{}
	if err := e.RunPhase(PreParse, ctx); err != nil {
		t.Errorf("RunPhase on empty phase returned error: %v", err)
	}
}

func TestRunAllPhasesOrder(t *testing.T) {
	var seq []string
	e := NewEngine()
	e.Register(newStubWithSeq("post-resp", PostResponse, &seq))
	e.Register(newStubWithSeq("pre-parse", PreParse, &seq))
	e.Register(newStubWithSeq("register", Register, &seq))
	e.Register(newStubWithSeq("post-parse", PostParse, &seq))
	e.Register(newStubWithSeq("pre-req", PreRequest, &seq))

	ctx := &Context{}
	if err := e.Run(ctx); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	want := "register,pre-parse,post-parse,pre-req,post-resp"
	got := strings.Join(seq, ",")
	if got != want {
		t.Errorf("phase execution order = %q, want %q", got, want)
	}
}

func TestRunAbortsOnPhaseError(t *testing.T) {
	var seq []string
	e := NewEngine()
	e.Register(newStubWithSeq("pre-parse", PreParse, &seq))
	e.Register(newStub("post-parse-fail", PostParse, func(*Context) error {
		return errors.New("fail")
	}))
	e.Register(newStubWithSeq("pre-req", PreRequest, &seq))

	ctx := &Context{}
	err := e.Run(ctx)
	if err == nil {
		t.Fatal("expected error")
	}
	if len(seq) != 1 || seq[0] != "pre-parse" {
		t.Errorf("seq = %v, want [pre-parse]", seq)
	}
}

func TestContextAddCorrection(t *testing.T) {
	ctx := &Context{}
	ctx.AddCorrection("alias", PreParse, "--userId", "--userId", "--user-id", "alias")
	ctx.AddCorrection("sticky", PreParse, "--limit", "--limit100", "--limit 100", "sticky")

	if got := len(ctx.Corrections); got != 2 {
		t.Fatalf("Corrections count = %d, want 2", got)
	}
	c := ctx.Corrections[0]
	if c.Handler != "alias" || c.Original != "--userId" || c.Corrected != "--user-id" {
		t.Errorf("first correction = %+v", c)
	}
}

func TestPhaseString(t *testing.T) {
	tests := []struct {
		phase Phase
		want  string
	}{
		{Register, "register"},
		{PreParse, "pre-parse"},
		{PostParse, "post-parse"},
		{PreRequest, "pre-request"},
		{PostResponse, "post-response"},
		{Phase(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.phase.String(); got != tt.want {
			t.Errorf("Phase(%d).String() = %q, want %q", tt.phase, got, tt.want)
		}
	}
}

func TestRunPhaseErrorContainsPhaseAndHandler(t *testing.T) {
	e := NewEngine()
	e.Register(newStub("my-handler", PostParse, func(*Context) error {
		return errors.New("bad value")
	}))

	err := e.RunPhase(PostParse, &Context{})
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "post-parse") {
		t.Errorf("error should contain phase name, got %q", msg)
	}
	if !strings.Contains(msg, "my-handler") {
		t.Errorf("error should contain handler name, got %q", msg)
	}
}

func TestMultipleHandlersSameAndDifferentPhases(t *testing.T) {
	var seq []string
	e := NewEngine()
	e.Register(newStubWithSeq("pp1", PreParse, &seq))
	e.Register(newStubWithSeq("pp2", PreParse, &seq))
	e.Register(newStubWithSeq("op1", PostParse, &seq))
	e.Register(newStubWithSeq("rq1", PreRequest, &seq))
	e.Register(newStubWithSeq("rq2", PreRequest, &seq))
	e.Register(newStubWithSeq("rs1", PostResponse, &seq))

	ctx := &Context{}
	if err := e.Run(ctx); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	want := "pp1,pp2,op1,rq1,rq2,rs1"
	got := strings.Join(seq, ",")
	if got != want {
		t.Errorf("order = %q, want %q", got, want)
	}
}
