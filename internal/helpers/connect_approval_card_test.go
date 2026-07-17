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
	"fmt"
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
	fail bool // when true, Run returns an error (simulates identity not ready)
}

func (r *fakeRunner) Run(_ context.Context, inv executor.Invocation) (executor.Result, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runs = append(r.runs, inv)
	if r.fail {
		return executor.Result{}, context.DeadlineExceeded
	}
	inv.Implemented = true
	return executor.Result{Invocation: inv}, nil
}

func (r *fakeRunner) setFail(v bool) { r.mu.Lock(); defer r.mu.Unlock(); r.fail = v }

func (r *fakeRunner) count() int { r.mu.Lock(); defer r.mu.Unlock(); return len(r.runs) }

// fakeNotifier records the proactive 1:1 messages text-mode approval sends to
// the owner and the requester.
type fakeNotifier struct {
	mu   sync.Mutex
	sent []sentMsg
}

type sentMsg struct {
	to   []string
	text string
}

func (n *fakeNotifier) sendOTOText(_ context.Context, userIDs []string, text string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.sent = append(n.sent, sentMsg{to: append([]string(nil), userIDs...), text: text})
	return nil
}

// toUser reports whether any recorded message was sent to userID.
func (n *fakeNotifier) toUser(userID string) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	for _, m := range n.sent {
		for _, u := range m.to {
			if u == userID {
				return true
			}
		}
	}
	return false
}

func (n *fakeNotifier) count() int { n.mu.Lock(); defer n.mu.Unlock(); return len(n.sent) }

func TestDingTalkApprovalCardSenderCoverage(t *testing.T) {
	if newDingtalkApprovalCardSender("client", "secret", " ") != nil {
		t.Fatal("empty approval template created a sender")
	}
	rec, srv := newCardAPIServer(t)
	withCardAPIBase(t, srv.URL)
	sender, ok := newDingtalkApprovalCardSender("client", "secret", "template").(*dingtalkApprovalCardSender)
	if !ok {
		t.Fatal("approval sender type mismatch")
	}
	req := &ApprovalRequest{ID: "approval", Summary: "Create todo", Requester: "requester"}
	track, err := sender.SendApprovalCard(context.Background(), "owner", req)
	if err != nil || track != "dws_approval_approval" {
		t.Fatalf("SendApprovalCard() = %q, %v", track, err)
	}
	if err := sender.UpdateApprovalCard(context.Background(), track, "done"); err != nil {
		t.Fatal(err)
	}
	rec.fail["PUT /v1.0/card/instances"] = 500
	if err := sender.UpdateApprovalCard(context.Background(), track, "failed"); err == nil {
		t.Fatal("approval update HTTP failure was ignored")
	}
	rec.fail = map[string]int{"POST /v1.0/card/instances": 500}
	if _, err := sender.SendApprovalCard(context.Background(), "owner", req); err == nil {
		t.Fatal("approval create HTTP failure was ignored")
	}
}

func TestApprovalAutoRetryLifecycleCoverage(t *testing.T) {
	var nilOrchestrator *approvalOrchestrator
	nilOrchestrator.startAutoRetry(context.Background(), time.Millisecond)
	(&approvalOrchestrator{}).startAutoRetry(context.Background(), time.Millisecond)
	notifier := &fakeNotifier{}
	orchestrator := &approvalOrchestrator{notifier: notifier, gate: newApprovalGate("")}
	orchestrator.startAutoRetry(context.Background(), 0)
	ctx, cancel := context.WithCancel(context.Background())
	orchestrator.startAutoRetry(ctx, time.Millisecond)
	time.Sleep(3 * time.Millisecond)
	cancel()
	time.Sleep(time.Millisecond)
}

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

// TestToPlannedAction_Actions covers the action verbs the twin can execute:
// each maps to the right product/tool, and missing required args degrade to
// "not an action" (ok=false) so a malformed marker never submits a no-op.
func TestToPlannedAction_Actions(t *testing.T) {
	cases := []struct {
		name        string
		act         detectedAction
		wantOK      bool
		wantProduct string
		wantTool    string
	}{
		{"todo", detectedAction{Verb: "todo.create", Args: map[string]string{"title": "交方案"}}, true, "todo", "create_personal_todo"},
		{"calendar", detectedAction{Verb: "calendar.create", Args: map[string]string{"title": "评审会", "start": "2026-06-20T10:00:00+08:00", "end": "2026-06-20T11:00:00+08:00"}}, true, "calendar", "create_calendar_event"},
		{"calendar missing time", detectedAction{Verb: "calendar.create", Args: map[string]string{"title": "评审会"}}, false, "", ""},
		{"doc", detectedAction{Verb: "doc.create", Args: map[string]string{"name": "周报"}}, true, "doc", "create_document"},
		{"doc missing name", detectedAction{Verb: "doc.create", Args: map[string]string{}}, false, "", ""},
		{"unknown verb", detectedAction{Verb: "drive.delete", Args: map[string]string{}}, false, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pa, summary, ok := toPlannedAction(tc.act, "owner")
			if ok != tc.wantOK {
				t.Fatalf("ok=%v want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if pa.Product != tc.wantProduct || pa.Tool != tc.wantTool {
				t.Fatalf("got %s/%s want %s/%s", pa.Product, pa.Tool, tc.wantProduct, tc.wantTool)
			}
			if summary == "" {
				t.Fatal("summary should not be empty for a valid action")
			}
			// Every twin action is write-class → must be gated.
			if classifyPlannedAction(pa) != CmdClassWrite {
				t.Fatalf("%s should classify as write (gated), got %s", tc.name, classifyPlannedAction(pa))
			}
		})
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

// TestHandleReply_TextMode_OwnerApproves is the private text-approval happy
// path: an action privately DMs the OWNER (the requester sees nothing, no
// execution yet); a non-owner "同意" is ignored; the owner's "同意" — sent from
// their own chat, NOT the request conversation — decides and executes.
func TestHandleReply_TextMode_OwnerApproves(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	gate := newApprovalGate("ct1")
	runner := &fakeRunner{}
	notifier := &fakeNotifier{}
	o := newTextApprovalOrchestrator(gate, runner, "owner", notifier)

	// A group requester (not the owner) asks for an action.
	out, handled := o.handleReply(context.Background(), "requester", "conv-group", `好的。[[ACTION:todo.create title="交方案"]]`, nil)
	if !handled || out != "" {
		t.Fatalf("text-mode action: handled=%v out=%q, want handled with empty out (requester sees nothing)", handled, out)
	}
	if runner.count() != 0 {
		t.Fatalf("must not execute before the owner decides, ran %d", runner.count())
	}
	if len(gate.list()) != 1 {
		t.Fatalf("expected one pending request, got %d", len(gate.list()))
	}
	// The approval prompt went privately to the OWNER, never to the requester.
	if !notifier.toUser("owner") {
		t.Fatalf("approval prompt was not DMed to the owner: %+v", notifier.sent)
	}
	if notifier.toUser("requester") {
		t.Fatal("the requester must NOT be notified of the approval")
	}

	// A non-owner saying 同意 must not decide or execute.
	if o.handleOwnerDecision(context.Background(), "intruder", "同意") {
		t.Fatal("non-owner must not be able to approve")
	}
	if runner.count() != 0 {
		t.Fatalf("non-owner approval must not execute, ran %d", runner.count())
	}

	// The owner saying 同意 decides and executes; the requester then gets the
	// outcome.
	if !o.handleOwnerDecision(context.Background(), "owner", "同意") {
		t.Fatal("owner approval must be consumed")
	}
	if runner.count() != 1 {
		t.Fatalf("owner approval must execute exactly once, ran %d", runner.count())
	}
	if gate.list()[0].State != approvalExecuted {
		t.Fatalf("request state = %s, want executed", gate.list()[0].State)
	}
	if !notifier.toUser("requester") {
		t.Fatal("the requester should get the final outcome after execution")
	}
}

// TestHandleReply_TextMode_OwnerSelfAutoApproves: when the requester IS the
// owner, the action runs immediately with NO confirmation round-trip, but is
// still recorded (auto_approved=true, decided-by owner, executed) for audit.
func TestHandleReply_TextMode_OwnerSelfAutoApproves(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	gate := newApprovalGate("ctself")
	runner := &fakeRunner{}
	notifier := &fakeNotifier{}
	o := newTextApprovalOrchestrator(gate, runner, "owner", notifier)

	var replies []string
	reply := func(_ context.Context, _, text string) error { replies = append(replies, text); return nil }

	// The OWNER asks for the action themselves.
	out, handled := o.handleReply(context.Background(), "owner", "conv-owner", `好。[[ACTION:todo.create title="自己的待办"]]`, reply)
	if !handled || out != "" {
		t.Fatalf("owner self-request: handled=%v out=%q", handled, out)
	}
	// Executed immediately — no second confirmation asked.
	if runner.count() != 1 {
		t.Fatalf("owner self-request must execute immediately, ran %d", runner.count())
	}
	if notifier.count() != 0 {
		t.Fatalf("owner self-request must NOT send an approval DM, sent %d", notifier.count())
	}
	// Still recorded for audit: auto_approved + executed + decided by owner.
	rec := gate.list()[0]
	if !rec.AutoApproved || rec.State != approvalExecuted || rec.DecidedBy != "owner" {
		t.Fatalf("audit record wrong: autoApproved=%v state=%s decidedBy=%s", rec.AutoApproved, rec.State, rec.DecidedBy)
	}
	if !strings.Contains(strings.Join(replies, "|"), "已为你完成") {
		t.Fatalf("owner should get the result, replies=%v", replies)
	}
}

// fakeAudit records the requests handed to the audit sink.
type fakeAudit struct {
	mu   sync.Mutex
	recs []*ApprovalRequest
}

func (a *fakeAudit) record(_ context.Context, req *ApprovalRequest) {
	a.mu.Lock()
	defer a.mu.Unlock()
	cp := *req
	a.recs = append(a.recs, &cp)
}

func (a *fakeAudit) count() int { a.mu.Lock(); defer a.mu.Unlock(); return len(a.recs) }

// TestAudit_RecordsTerminalOutcomes verifies the audit sink gets one terminal
// record per action: an auto-executed owner request, and an approved request.
func TestAudit_RecordsTerminalOutcomes(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	gate := newApprovalGate("cta")
	audit := &fakeAudit{}
	o := newTextApprovalOrchestrator(gate, &fakeRunner{}, "owner", &fakeNotifier{})
	o.audit = audit
	reply := func(_ context.Context, _, _ string) error { return nil }

	// Owner self-request → auto-executed → audited once, as executed.
	o.handleReply(context.Background(), "owner", "conv-o", `[[ACTION:todo.create title="自己"]]`, reply)
	if audit.count() != 1 {
		t.Fatalf("auto-exec must produce one audit record, got %d", audit.count())
	}
	if audit.recs[0].State != approvalExecuted || !audit.recs[0].AutoApproved {
		t.Fatalf("auto audit record: state=%s auto=%v", audit.recs[0].State, audit.recs[0].AutoApproved)
	}

	// Someone else's request, approved by the owner → audited as executed.
	o.handleReply(context.Background(), "requester", "conv-g", `[[ACTION:todo.create title="别人"]]`, reply)
	if audit.count() != 1 {
		t.Fatal("a pending (not yet decided) request must not be audited yet")
	}
	o.handleOwnerDecision(context.Background(), "owner", "同意")
	if audit.count() != 2 {
		t.Fatalf("owner approval must produce a second audit record, got %d", audit.count())
	}
	if audit.recs[1].State != approvalExecuted || audit.recs[1].DecidedBy != "owner" {
		t.Fatalf("approved audit record: state=%s decidedBy=%s", audit.recs[1].State, audit.recs[1].DecidedBy)
	}
}

// TestDeferredExecution_HoldsAndRetries is the graceful-degradation path: when
// execution fails (identity not ready), the request is DEFERRED (not lost, not
// failed), the owner is privately notified; after the owner recovers and replies
// "重试", the backlog is replayed, executes, and the requester gets the result.
func TestDeferredExecution_HoldsAndRetries(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	gate := newApprovalGate("ctdef")
	runner := &fakeRunner{fail: true} // identity not ready → execution fails
	notifier := &fakeNotifier{}
	o := newTextApprovalOrchestrator(gate, runner, "owner", notifier)

	// A non-owner request, approved by the owner, but execution fails → deferred.
	o.handleReply(context.Background(), "requester", "conv-g", `[[ACTION:todo.create title="待恢复"]]`, nil)
	if !o.handleOwnerDecision(context.Background(), "owner", "同意") {
		t.Fatal("owner approval must be consumed")
	}
	rec := gate.list()[0]
	if rec.State != approvalDeferred {
		t.Fatalf("failed execution must DEFER (not fail), got state=%s", rec.State)
	}
	// Owner was told to recover; requester was NOT told it failed (task not lost).
	if !notifier.toUser("owner") {
		t.Fatal("owner should be notified of the deferred action")
	}
	if notifier.toUser("requester") {
		t.Fatal("requester must NOT be told it failed — their task is held, not lost")
	}

	// A bare non-retry word from the owner should pass through (not consumed).
	if o.handleOwnerDecision(context.Background(), "owner", "在吗") {
		t.Fatal("ordinary owner message must not be consumed")
	}

	// Owner recovers (identity now works) and replies "重试" → backlog flushes.
	runner.setFail(false)
	if !o.handleOwnerDecision(context.Background(), "owner", "重试") {
		t.Fatal("retry word must be consumed")
	}
	if gate.list()[0].State != approvalExecuted {
		t.Fatalf("after retry the deferred request must execute, got %s", gate.list()[0].State)
	}
	if !notifier.toUser("requester") {
		t.Fatal("requester should get the outcome after the backlog is flushed")
	}
}

// TestConfirmPolicy_Auto: a non-owner request runs without asking under "auto".
func TestConfirmPolicy_Auto(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	gate := newApprovalGate("ctauto2")
	runner := &fakeRunner{}
	notifier := &fakeNotifier{}
	o := newTextApprovalOrchestrator(gate, runner, "owner", notifier)
	o.confirmPolicy = "auto"

	_, handled := o.handleReply(context.Background(), "someone", "c1", `[[ACTION:todo.create title="x"]]`, func(_ context.Context, _, _ string) error { return nil })
	if !handled {
		t.Fatal("auto policy should handle (run) the action")
	}
	if runner.count() != 1 {
		t.Fatalf("auto policy must execute without asking, ran %d", runner.count())
	}
	if gate.list()[0].State != approvalExecuted {
		t.Fatalf("state=%s want executed", gate.list()[0].State)
	}
}

// TestConfirmPolicy_Manual: a non-owner request asks the owner (default).
func TestConfirmPolicy_Manual(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	gate := newApprovalGate("ctman")
	runner := &fakeRunner{}
	notifier := &fakeNotifier{}
	o := newTextApprovalOrchestrator(gate, runner, "owner", notifier) // no policy = manual

	o.handleReply(context.Background(), "someone", "c1", `[[ACTION:todo.create title="x"]]`, nil)
	if runner.count() != 0 {
		t.Fatal("manual policy must NOT execute before the owner decides")
	}
	if gate.list()[0].State != approvalPending {
		t.Fatalf("state=%s want pending (awaiting owner)", gate.list()[0].State)
	}
	if !notifier.toUser("owner") {
		t.Fatal("manual policy should DM the owner for approval")
	}
}

// TestConfirmPolicy_Remember: ask once per verb, then reuse the decision.
func TestConfirmPolicy_Remember(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	gate := newApprovalGate("ctrem")
	runner := &fakeRunner{}
	notifier := &fakeNotifier{}
	o := newTextApprovalOrchestrator(gate, runner, "owner", notifier)
	o.confirmPolicy = "remember"

	// First todo.create from a non-owner → asks (pending).
	o.handleReply(context.Background(), "someone", "c1", `[[ACTION:todo.create title="第一次"]]`, nil)
	if gate.latestPending() == nil {
		t.Fatal("first request should ask the owner")
	}
	// Owner approves → remembered, executes.
	o.handleOwnerDecision(context.Background(), "owner", "同意")
	if runner.count() != 1 {
		t.Fatalf("approved request should execute, ran %d", runner.count())
	}

	// Second todo.create → reused decision, auto-runs (no new pending).
	_, handled := o.handleReply(context.Background(), "someone", "c2", `[[ACTION:todo.create title="第二次"]]`, func(_ context.Context, _, _ string) error { return nil })
	if !handled || runner.count() != 2 {
		t.Fatalf("remembered approve should auto-run the 2nd request, handled=%v ran=%d", handled, runner.count())
	}
}

// TestScopeEnforcement: an action outside the role's allowlist is refused up
// front (the requester is told, nothing reaches the gate); an in-scope action
// proceeds; an empty allowlist allows everything.
func TestScopeEnforcement(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	gate := newApprovalGate("ctscope")
	runner := &fakeRunner{}
	o := newTextApprovalOrchestrator(gate, runner, "owner", &fakeNotifier{})
	o.allowedScopes = []string{"todo"} // HR-style: only todo

	// doc.create is out of scope → refused, NOT gated, requester told.
	out, handled := o.handleReply(context.Background(), "requester", "c1", `[[ACTION:doc.create name="周报"]]`, nil)
	if handled {
		t.Fatal("out-of-scope action must not be handled by the gate")
	}
	if !strings.Contains(out, "没有") || !strings.Contains(out, "doc") {
		t.Fatalf("requester should be told it's out of scope, got %q", out)
	}
	if len(gate.list()) != 0 {
		t.Fatal("out-of-scope action must not create an approval request")
	}

	// todo.create is in scope → proceeds (owner self-request auto-executes).
	o.handleReply(context.Background(), "owner", "c2", `[[ACTION:todo.create title="排期"]]`, func(_ context.Context, _, _ string) error { return nil })
	if len(gate.list()) != 1 {
		t.Fatalf("in-scope action should be processed, got %d requests", len(gate.list()))
	}

	// scopeAllows: empty allowlist = allow all.
	o.allowedScopes = nil
	if !o.scopeAllows("anything") {
		t.Fatal("empty allowlist must allow all products")
	}
}

// TestAutoFlushDeferred verifies the background retry: silent while the identity
// is still wrong (no spam), and it drains + notifies once execution works.
func TestAutoFlushDeferred(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	gate := newApprovalGate("ctauto")
	runner := &fakeRunner{fail: true}
	notifier := &fakeNotifier{}
	o := newTextApprovalOrchestrator(gate, runner, "owner", notifier)

	// Defer one (identity not ready).
	o.handleReply(context.Background(), "owner", "conv-o", `[[ACTION:todo.create title="自动恢复"]]`, func(_ context.Context, _, _ string) error { return nil })
	deferNotices := notifier.count() // the deferred DM
	if gate.list()[0].State != approvalDeferred {
		t.Fatalf("want deferred, got %s", gate.list()[0].State)
	}

	// Auto-retry while still failing → stays deferred, NO new owner message.
	o.autoFlushDeferred(context.Background())
	if gate.list()[0].State != approvalDeferred {
		t.Fatal("should still be deferred while identity is wrong")
	}
	if notifier.count() != deferNotices {
		t.Fatalf("auto-retry must stay silent when nothing completes (sent %d, was %d)", notifier.count(), deferNotices)
	}

	// Identity recovers → auto-retry drains it and notifies once.
	runner.setFail(false)
	o.autoFlushDeferred(context.Background())
	if gate.list()[0].State != approvalExecuted {
		t.Fatalf("auto-retry should execute once identity is back, got %s", gate.list()[0].State)
	}
	notifier.mu.Lock()
	last := notifier.sent[len(notifier.sent)-1].text
	notifier.mu.Unlock()
	if !strings.Contains(last, "已自动恢复") || !strings.Contains(last, "自动恢复") {
		t.Fatalf("owner should get an auto-recovery notice, got %q", last)
	}
}

// TestIsTransientSheetErr distinguishes retryable throttles from permanent errors.
func TestIsTransientSheetErr(t *testing.T) {
	for _, e := range []string{"system error: THREADPOOL_BUSY", "server busy", "request timeout", "HTTP 429 too many requests"} {
		if !isTransientSheetErr(fmt.Errorf("%s", e)) {
			t.Fatalf("%q should be transient", e)
		}
	}
	for _, e := range []string{"invalid nodeId", "permission denied", "not found"} {
		if isTransientSheetErr(fmt.Errorf("%s", e)) {
			t.Fatalf("%q should NOT be transient", e)
		}
	}
	if isTransientSheetErr(nil) {
		t.Fatal("nil is not transient")
	}
}

// TestFlushDeferred_ReportsCompletedItems verifies the owner gets a per-item
// completion list (not just a count) when the backlog is flushed.
func TestFlushDeferred_ReportsCompletedItems(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	gate := newApprovalGate("ctflush")
	runner := &fakeRunner{fail: true}
	notifier := &fakeNotifier{}
	o := newTextApprovalOrchestrator(gate, runner, "owner", notifier)

	o.handleReply(context.Background(), "owner", "conv-o", `[[ACTION:todo.create title="我的待办"]]`, func(_ context.Context, _, _ string) error { return nil })
	if gate.list()[0].State != approvalDeferred {
		t.Fatalf("want deferred, got %s", gate.list()[0].State)
	}

	runner.setFail(false)
	o.handleOwnerDecision(context.Background(), "owner", "重试")

	notifier.mu.Lock()
	var last string
	if len(notifier.sent) > 0 {
		last = notifier.sent[len(notifier.sent)-1].text
	}
	notifier.mu.Unlock()
	if !strings.Contains(last, "已补做 1 个") || !strings.Contains(last, "创建待办：我的待办") {
		t.Fatalf("flush summary should list the completed item, got %q", last)
	}
}

// TestIsRetryWord checks whole-message retry-keyword matching.
func TestIsRetryWord(t *testing.T) {
	for _, w := range []string{"重试", " 恢复 ", "retry", "继续"} {
		if !isRetryWord(w) {
			t.Fatalf("%q should be a retry word", w)
		}
	}
	for _, w := range []string{"重试一下他的方案", "", "你好"} {
		if isRetryWord(w) {
			t.Fatalf("%q should NOT be a retry word", w)
		}
	}
}

// TestHandleReply_TextMode_OwnerRejects: the owner's "拒绝" declines without
// executing.
func TestHandleReply_TextMode_OwnerRejects(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	gate := newApprovalGate("ct2")
	runner := &fakeRunner{}
	notifier := &fakeNotifier{}
	o := newTextApprovalOrchestrator(gate, runner, "owner", notifier)

	o.handleReply(context.Background(), "requester", "conv-group", `[[ACTION:todo.create title="x"]]`, nil)
	if !o.handleOwnerDecision(context.Background(), "owner", "拒绝") {
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
	o := newTextApprovalOrchestrator(gate, &fakeRunner{}, "owner", &fakeNotifier{})
	gate.Submit(ApprovalRequest{ConvID: "conv-1", Summary: "x", State: approvalPending})

	if o.handleOwnerDecision(context.Background(), "owner", "顺便帮我查下天气") {
		t.Fatal("a non-decision owner message must pass through to the agent")
	}
}

// TestLatestPending returns the newest pending request across conversations.
func TestLatestPending(t *testing.T) {
	gate := newApprovalGate("")
	gate.Submit(ApprovalRequest{ConvID: "c1", Summary: "old", CreatedAt: time.Unix(100, 0)})
	newer := gate.Submit(ApprovalRequest{ConvID: "c2", Summary: "new", CreatedAt: time.Unix(200, 0)})
	if got := gate.latestPending(); got == nil || got.ID != newer.ID {
		t.Fatalf("latestPending = %v, want newest %s", got, newer.ID)
	}
	gate.Decide(newer.ID, true, "owner")
	if got := gate.latestPending(); got == nil || got.Summary != "old" {
		t.Fatalf("after deciding newest, latestPending should fall back to the older pending, got %v", got)
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
