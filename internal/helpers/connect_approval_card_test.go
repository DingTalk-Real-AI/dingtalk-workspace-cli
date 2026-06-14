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

package helpers

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/open-dingtalk/dingtalk-stream-sdk-go/card"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
)

// ---- fakes (no network, no real runner) ----

type fakeCardSender struct {
	mu        sync.Mutex
	sent      []*ApprovalRequest
	updates   []string
	failSend  bool
	lastTrack string
}

func (f *fakeCardSender) SendApprovalCard(_ context.Context, _ string, req *ApprovalRequest) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failSend {
		return "", context.DeadlineExceeded
	}
	f.sent = append(f.sent, req)
	f.lastTrack = "track-" + req.ID
	return f.lastTrack, nil
}

func (f *fakeCardSender) UpdateApprovalCard(_ context.Context, _, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updates = append(f.updates, text)
	return nil
}

// fakeRunner records the invocations it is asked to run and reports them as
// implemented (so the orchestrator treats them as executed).
type fakeRunner struct {
	mu   sync.Mutex
	runs []executor.Invocation
}

func (r *fakeRunner) Run(_ context.Context, inv executor.Invocation) (executor.Result, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runs = append(r.runs, inv)
	inv.Implemented = true
	return executor.Result{Invocation: inv}, nil
}

func (r *fakeRunner) count() int { r.mu.Lock(); defer r.mu.Unlock(); return len(r.runs) }

// cardClickReq builds a card callback as DingTalk delivers it: the tapped
// button's action id plus its private params (approval id + decision).
func cardClickReq(outTrackID, approvalID, decision, actionID, userID string) *card.CardRequest {
	return &card.CardRequest{
		OutTrackId: outTrackID,
		UserId:     userID,
		CardActionData: card.PrivateCardActionData{
			CardPrivateData: card.CardPrivateData{
				ActionIdList: []string{actionID},
				Params: map[string]any{
					approvalParamID:       approvalID,
					approvalParamDecision: decision,
				},
			},
		},
	}
}

// TestDecodeCardAction verifies the action parser pulls the approval id and the
// approve/reject decision from a button tap, via params and via the action id
// fallback.
func TestDecodeCardAction(t *testing.T) {
	approve := cardClickReq("t1", "appr-1", approvalDecisionApprove, approvalActionApprove, "owner")
	id, ok2, ok := decodeCardAction(approve)
	if !ok || !ok2 || id != "appr-1" {
		t.Fatalf("approve decode: id=%q approve=%v ok=%v", id, ok2, ok)
	}

	reject := cardClickReq("t2", "appr-2", approvalDecisionReject, approvalActionReject, "owner")
	id, ok2, ok = decodeCardAction(reject)
	if !ok || ok2 || id != "appr-2" {
		t.Fatalf("reject decode: id=%q approve=%v ok=%v", id, ok2, ok)
	}

	// Decision param missing → fall back to the tapped action id.
	fallback := &card.CardRequest{
		OutTrackId: "t3",
		CardActionData: card.PrivateCardActionData{
			CardPrivateData: card.CardPrivateData{
				ActionIdList: []string{approvalActionApprove},
				Params:       map[string]any{approvalParamID: "appr-3"},
			},
		},
	}
	id, ok2, ok = decodeCardAction(fallback)
	if !ok || !ok2 || id != "appr-3" {
		t.Fatalf("fallback decode: id=%q approve=%v ok=%v", id, ok2, ok)
	}

	// A non-approval card → not ok.
	if _, _, ok := decodeCardAction(&card.CardRequest{OutTrackId: "x"}); ok {
		t.Fatal("non-approval card should not decode as a decision")
	}
}

// TestHandleCardCallback_DrivesDecide verifies a button callback flips the gate
// state and that an approval id missing from params is recovered by outTrackId.
func TestHandleCardCallback_DrivesDecide(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	gate := newApprovalGate("c1")
	o := newApprovalOrchestrator(gate, &fakeCardSender{}, &fakeRunner{}, "owner")

	app := gate.Submit(ApprovalRequest{Summary: "x"})
	gate.setOutTrackID(app.ID, "track-x")

	// Callback carries no approval id in params → associate by outTrackId.
	req := &card.CardRequest{
		OutTrackId: "track-x",
		UserId:     "owner",
		CardActionData: card.PrivateCardActionData{
			CardPrivateData: card.CardPrivateData{ActionIdList: []string{approvalActionApprove}},
		},
	}
	if _, err := o.handleCardCallback(context.Background(), req); err != nil {
		t.Fatalf("callback err: %v", err)
	}
	if got := gate.Get(app.ID); !got.approved() {
		t.Fatalf("state = %s, want approved after approve tap", got.State)
	}
}

// TestHandleReply_RejectDoesNotExecute is the reject-path guarantee: a rejected
// approval must NOT run the planned command.
func TestHandleReply_RejectDoesNotExecute(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	gate := newApprovalGate("c2")
	sender := &fakeCardSender{}
	runner := &fakeRunner{}
	o := newApprovalOrchestrator(gate, sender, runner, "owner")
	o.awaitWindow = time.Second

	var replies []string
	var rmu sync.Mutex
	reply := func(_ context.Context, _, text string) error {
		rmu.Lock()
		replies = append(replies, text)
		rmu.Unlock()
		return nil
	}

	// The owner rejects shortly after the card is sent.
	go func() {
		deadline := time.Now().Add(time.Second)
		for time.Now().Before(deadline) {
			sender.mu.Lock()
			n := len(sender.sent)
			var id string
			if n > 0 {
				id = sender.sent[0].ID
			}
			sender.mu.Unlock()
			if id != "" {
				gate.Decide(id, false, "owner")
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()

	agentReply := `好的。[[ACTION:todo.create title="交方案"]]`
	out, handled := o.handleReply(context.Background(), "requester", "conv-1", agentReply, reply)
	if !handled {
		t.Fatal("an action reply must be handled by the gate")
	}
	if out != "" {
		t.Fatalf("handled reply should return empty connector reply, got %q", out)
	}
	if runner.count() != 0 {
		t.Fatalf("reject path must NOT execute, runner ran %d times", runner.count())
	}
	if gate.Get(gate.list()[0].ID).State != approvalRejected {
		t.Fatal("request should be rejected")
	}
	rmu.Lock()
	joined := strings.Join(replies, " | ")
	rmu.Unlock()
	if !strings.Contains(joined, "没批准") {
		t.Fatalf("reject reply not posted: %q", joined)
	}
}

// TestClassifyPlannedAction verifies the gate's read/write bridge: it classifies
// on the human command path first and falls back to product + RPC tool name.
func TestClassifyPlannedAction(t *testing.T) {
	cases := []struct {
		name string
		pa   plannedAction
		want CmdClass
	}{
		{
			name: "write via legacy path",
			pa:   plannedAction{Product: "todo", Tool: "create_personal_todo", LegacyPath: "todo task create"},
			want: CmdClassWrite,
		},
		{
			name: "read via legacy path",
			pa:   plannedAction{Product: "todo", Tool: "list_personal_todo", LegacyPath: "todo task list"},
			want: CmdClassRead,
		},
		{
			name: "empty legacy path falls back to product+tool",
			pa:   plannedAction{Product: "todo", Tool: "create_personal_todo"},
			want: CmdClassWrite,
		},
		{
			name: "unrecognised path falls back to tool verb",
			pa:   plannedAction{Product: "todo", Tool: "delete_personal_todo", LegacyPath: "todo xyz123"},
			want: CmdClassWrite,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyPlannedAction(tc.pa); got != tc.want {
				t.Fatalf("classifyPlannedAction(%+v) = %s, want %s", tc.pa, got, tc.want)
			}
		})
	}
}

// TestHandleReply_ReadClassBypassesGate proves the "写类才拦" wiring: when the
// planned command classifies as read-only, the gate does not engage — no card is
// sent, nothing runs, and the cleaned reply goes out normally (handled=false).
// We force the todo.create path to read-class via an override so the test drives
// the real handleReply → classifyPlannedAction path end to end.
func TestHandleReply_ReadClassBypassesGate(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	SetCmdClassOverride("todo task create", CmdClassRead)
	defer SetCmdClassOverride("todo task create", CmdClassUnknown)

	gate := newApprovalGate("c3")
	sender := &fakeCardSender{}
	runner := &fakeRunner{}
	o := newApprovalOrchestrator(gate, sender, runner, "owner")

	reply := func(_ context.Context, _, _ string) error { return nil }
	agentReply := `好的，已为你查到。[[ACTION:todo.create title="交方案"]]`
	out, handled := o.handleReply(context.Background(), "requester", "conv-1", agentReply, reply)

	if handled {
		t.Fatal("a read-class action must NOT be handled by the gate")
	}
	if !strings.Contains(out, "已为你查到") || strings.Contains(out, "[[ACTION") {
		t.Fatalf("read-class reply should be the cleaned text without the marker, got %q", out)
	}
	sender.mu.Lock()
	nSent := len(sender.sent)
	sender.mu.Unlock()
	if nSent != 0 {
		t.Fatalf("read-class action must not send a confirmation card, sent %d", nSent)
	}
	if runner.count() != 0 {
		t.Fatalf("read-class action must not execute, runner ran %d times", runner.count())
	}
	if len(gate.list()) != 0 {
		t.Fatalf("read-class action must not create an approval request, got %d", len(gate.list()))
	}
}

// TestParseDecisionWord checks whole-message decision parsing: a bare keyword
// decides, anything else (including a keyword embedded in a sentence) does not.
func TestParseDecisionWord(t *testing.T) {
	cases := []struct {
		in          string
		wantApprove bool
		wantOk      bool
	}{
		{"同意", true, true},
		{"  同意  ", true, true},
		{"通过", true, true},
		{"Yes", true, true},
		{"OK", true, true},
		{"拒绝", false, true},
		{"no", false, true},
		{"不同意", false, true},
		{"同意他的看法", false, false}, // embedded, not a bare decision
		{"帮我创建个待办", false, false},
		{"", false, false},
	}
	for _, tc := range cases {
		gotApprove, gotOk := parseDecisionWord(tc.in)
		if gotOk != tc.wantOk || (tc.wantOk && gotApprove != tc.wantApprove) {
			t.Fatalf("parseDecisionWord(%q) = (%v,%v), want (%v,%v)", tc.in, gotApprove, gotOk, tc.wantApprove, tc.wantOk)
		}
	}
}

// TestPendingForConv finds the pending request for a conversation and stops
// returning it once decided.
func TestPendingForConv(t *testing.T) {
	gate := newApprovalGate("") // in-memory
	a := gate.Submit(ApprovalRequest{ConvID: "c1", Summary: "a"})
	gate.Submit(ApprovalRequest{ConvID: "c2", Summary: "b"})

	if got := gate.pendingForConv("c1"); got == nil || got.ID != a.ID {
		t.Fatalf("pendingForConv(c1) = %v, want request %s", got, a.ID)
	}
	if got := gate.pendingForConv("none"); got != nil {
		t.Fatalf("pendingForConv(none) = %v, want nil", got)
	}
	gate.Decide(a.ID, true, "owner")
	if got := gate.pendingForConv("c1"); got != nil {
		t.Fatalf("decided request must not be pending, got %v", got)
	}
}

// TestHandleReply_TextMode_OwnerApproves is the text-approval happy path: an
// action prompts a confirmation (no execution yet); a non-owner "同意" is
// ignored; the owner's "同意" decides and executes.
func TestHandleReply_TextMode_OwnerApproves(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	gate := newApprovalGate("ct1")
	runner := &fakeRunner{}
	o := newTextApprovalOrchestrator(gate, runner, "owner")

	var replies []string
	reply := func(_ context.Context, _, text string) error { replies = append(replies, text); return nil }

	out, handled := o.handleReply(context.Background(), "requester", "conv-1", `好的。[[ACTION:todo.create title="交方案"]]`, reply)
	if !handled || out != "" {
		t.Fatalf("text-mode action: handled=%v out=%q, want handled with empty out", handled, out)
	}
	if runner.count() != 0 {
		t.Fatalf("must not execute before the owner decides, ran %d", runner.count())
	}
	if len(gate.list()) != 1 {
		t.Fatalf("expected one pending request, got %d", len(gate.list()))
	}
	if !strings.Contains(strings.Join(replies, "|"), "需要主人确认") {
		t.Fatalf("confirmation prompt not posted: %v", replies)
	}

	// A non-owner saying 同意 must not decide or execute.
	if o.handleOwnerDecision(context.Background(), "intruder", "conv-1", "同意", reply) {
		t.Fatal("non-owner must not be able to approve")
	}
	if runner.count() != 0 {
		t.Fatalf("non-owner approval must not execute, ran %d", runner.count())
	}

	// The owner saying 同意 decides and executes.
	if !o.handleOwnerDecision(context.Background(), "owner", "conv-1", "同意", reply) {
		t.Fatal("owner approval must be consumed")
	}
	if runner.count() != 1 {
		t.Fatalf("owner approval must execute exactly once, ran %d", runner.count())
	}
	if gate.list()[0].State != approvalExecuted {
		t.Fatalf("request state = %s, want executed", gate.list()[0].State)
	}
}

// TestHandleReply_TextMode_OwnerRejects: the owner's "拒绝" declines without
// executing.
func TestHandleReply_TextMode_OwnerRejects(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	gate := newApprovalGate("ct2")
	runner := &fakeRunner{}
	o := newTextApprovalOrchestrator(gate, runner, "owner")
	reply := func(_ context.Context, _, _ string) error { return nil }

	o.handleReply(context.Background(), "requester", "conv-1", `[[ACTION:todo.create title="x"]]`, reply)
	if !o.handleOwnerDecision(context.Background(), "owner", "conv-1", "拒绝", reply) {
		t.Fatal("owner rejection must be consumed")
	}
	if runner.count() != 0 {
		t.Fatalf("rejection must NOT execute, ran %d", runner.count())
	}
	if gate.list()[0].State != approvalRejected {
		t.Fatalf("state = %s, want rejected", gate.list()[0].State)
	}
}

// TestHandleOwnerDecision_NotADecision: an ordinary owner message (not a bare
// keyword) is not consumed, so it still reaches the agent.
func TestHandleOwnerDecision_NotADecision(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	gate := newApprovalGate("ct3")
	o := newTextApprovalOrchestrator(gate, &fakeRunner{}, "owner")
	reply := func(_ context.Context, _, _ string) error { return nil }
	gate.Submit(ApprovalRequest{ConvID: "conv-1", Summary: "x", State: approvalPending})

	if o.handleOwnerDecision(context.Background(), "owner", "conv-1", "顺便帮我查下天气", reply) {
		t.Fatal("a non-decision owner message must pass through to the agent")
	}
}

// list is a tiny test helper to enumerate the gate's requests.
func (g *approvalGate) list() []*ApprovalRequest {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]*ApprovalRequest, 0, len(g.reqs))
	for _, r := range g.reqs {
		cp := *r
		out = append(out, &cp)
	}
	return out
}
