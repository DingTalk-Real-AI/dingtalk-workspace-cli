package helpers

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/open-dingtalk/dingtalk-stream-sdk-go/card"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
)

type approvalSequenceRunner struct {
	mu     sync.Mutex
	errors []error
	index  int
}

func (r *approvalSequenceRunner) Run(_ context.Context, inv executor.Invocation) (executor.Result, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var err error
	if r.index < len(r.errors) {
		err = r.errors[r.index]
	}
	r.index++
	inv.Implemented = true
	return executor.Result{Invocation: inv}, err
}

type approvalErrorNotifier struct{ err error }

func (n approvalErrorNotifier) sendOTOText(context.Context, []string, string) error { return n.err }

type approvalUnimplementedRunner struct{}

func (approvalUnimplementedRunner) Run(_ context.Context, inv executor.Invocation) (executor.Result, error) {
	return executor.Result{Invocation: inv}, nil
}

func TestCrossPlatformCoverageSheetAuditSinkRemainingEdges(t *testing.T) {
	originalSleep := helperSleep
	helperSleep = func(time.Duration) {}
	defer func() { helperSleep = originalSleep }()

	(&sheetAuditSink{}).record(context.Background(), nil)
	req := &ApprovalRequest{
		ID: "id", Summary: "summary", Requester: "requester", State: approvalExecuted,
		AutoApproved: true, CreatedAt: time.Now(), DecidedAt: time.Now(), DecidedBy: "owner",
	}
	(&sheetAuditSink{runner: &approvalSequenceRunner{}}).record(context.Background(), req)
	(&sheetAuditSink{runner: &approvalSequenceRunner{errors: []error{errors.New("THREADPOOL_BUSY"), nil}}}).record(context.Background(), req)
	(&sheetAuditSink{runner: &approvalSequenceRunner{errors: []error{errors.New("permanent")}}}).record(context.Background(), req)
}

func TestCrossPlatformCoverageApprovalDecisionAndReplyRemainingEdges(t *testing.T) {
	o := &approvalOrchestrator{ownerUserID: "owner", confirmPolicy: "remember", remembered: map[string]bool{"todo.create": false}}
	if got := o.gateDecision("requester", "todo.create"); got != "reject" {
		t.Fatalf("decision=%q", got)
	}
	if got, handled := (&approvalOrchestrator{}).handleReply(context.Background(), "r", "c", "plain", nil); handled || got != "plain" {
		t.Fatalf("disabled reply=(%q,%v)", got, handled)
	}

	gate := newApprovalGate("")
	o = newTextApprovalOrchestrator(gate, &fakeRunner{}, "owner", &fakeNotifier{})
	if got, handled := o.handleReply(context.Background(), "r", "c", "plain", nil); handled || got != "plain" {
		t.Fatalf("plain action=(%q,%v)", got, handled)
	}
	if got, handled := o.handleReply(context.Background(), "r", "c", `before [[ACTION:todo.create]] after`, nil); handled || strings.Contains(got, "[[ACTION") {
		t.Fatalf("unknown action=(%q,%v)", got, handled)
	}
	o.confirmPolicy = "remember"
	o.remembered = map[string]bool{"todo.create": false}
	if got, handled := o.handleReply(context.Background(), "r", "c", `[[ACTION:todo.create title="x"]]`, nil); handled || !strings.Contains(got, "此前已拒绝") {
		t.Fatalf("remember reject=(%q,%v)", got, handled)
	}

	missing := &ApprovalRequest{ID: "missing", Summary: "x", Action: plannedAction{Product: "todo", Tool: "tool"}}
	o.autoApproveAndExecute(context.Background(), missing, "c", nil)
	o.notifier = nil
	o.notifyOwnerDeferred(context.Background(), missing, errors.New("ignored"))

	o.notifier = approvalErrorNotifier{err: errors.New("notify")}
	if got, handled := o.handleReplyText(context.Background(), &ApprovalRequest{ID: "id", Summary: "x"}); !handled || got != "" {
		t.Fatalf("text notify error=(%q,%v)", got, handled)
	}
	if o.handleOwnerDecision(context.Background(), "owner", "同意") {
		t.Fatal("decision without pending was consumed")
	}
}

func TestCrossPlatformCoverageApprovalConcurrentDecisionSeamEdge(t *testing.T) {
	original := approvalOwnerDecide
	defer func() { approvalOwnerDecide = original }()
	gate := newApprovalGate("")
	notifier := &fakeNotifier{}
	o := newTextApprovalOrchestrator(gate, &fakeRunner{}, "owner", notifier)
	req := gate.Submit(ApprovalRequest{Requester: "r", Verb: "todo.create", Summary: "x"})
	approvalOwnerDecide = func(*approvalGate, string, bool, string) (*ApprovalRequest, bool) { return req, false }
	if !o.handleOwnerDecision(context.Background(), "owner", "同意") {
		t.Fatal("already decided keyword was not consumed")
	}
}

func TestCrossPlatformCoverageApprovalFlushAndExecuteRemainingEdges(t *testing.T) {
	gate := newApprovalGate("")
	notifier := &fakeNotifier{}
	o := newTextApprovalOrchestrator(gate, &fakeRunner{}, "owner", notifier)
	if !o.flushDeferred(context.Background()) {
		t.Fatal("empty flush was not consumed")
	}

	stuckRunner := &fakeRunner{fail: true}
	o = newTextApprovalOrchestrator(gate, stuckRunner, "owner", notifier)
	req := gate.Submit(ApprovalRequest{Requester: "r", Summary: "stuck", Action: plannedAction{Product: "todo", Tool: "tool"}})
	gate.Decide(req.ID, true, "owner")
	gate.markDeferred(req.ID, "old")
	if !o.flushDeferred(context.Background()) {
		t.Fatal("stuck flush was not consumed")
	}

	if _, err := (&approvalOrchestrator{}).execute(context.Background(), req); err == nil {
		t.Fatal("nil runner returned nil")
	}
	o.runner = approvalUnimplementedRunner{}
	if got, err := o.execute(context.Background(), req); err != nil || !strings.Contains(got, "未实际执行") {
		t.Fatalf("unimplemented=%q err=%v", got, err)
	}
}

func TestCrossPlatformCoverageApprovalCardFlowRemainingEdges(t *testing.T) {
	reply := func(context.Context, string, string) error { return nil }
	req := ApprovalRequest{Requester: "r", Summary: "x", Action: plannedAction{Product: "todo", Tool: "tool"}}

	gate := newApprovalGate("")
	pending := gate.Submit(req)
	o := newApprovalOrchestrator(gate, &fakeCardSender{failSend: true}, &fakeRunner{}, "owner")
	if got, handled := o.handleReplyCard(context.Background(), pending, "c", reply); handled || !strings.Contains(got, "发送失败") {
		t.Fatalf("send fail=(%q,%v)", got, handled)
	}

	gate = newApprovalGate("")
	pending = gate.Submit(req)
	o = newApprovalOrchestrator(gate, &fakeCardSender{}, &fakeRunner{}, "owner")
	o.awaitWindow = time.Nanosecond
	if _, handled := o.handleReplyCard(context.Background(), pending, "c", reply); !handled {
		t.Fatal("timeout was not handled")
	}

	gate = newApprovalGate("")
	pending = gate.Submit(req)
	gate.Decide(pending.ID, true, "owner")
	o = newApprovalOrchestrator(gate, &fakeCardSender{}, &fakeRunner{fail: true}, "owner")
	if _, handled := o.handleReplyCard(context.Background(), pending, "c", reply); !handled {
		t.Fatal("execution failure was not handled")
	}
}

func TestCrossPlatformCoverageApprovalCardCallbackRemainingEdges(t *testing.T) {
	var nilOrchestrator *approvalOrchestrator
	if _, err := nilOrchestrator.handleCardCallback(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	gate := newApprovalGate("")
	o := newApprovalOrchestrator(gate, &fakeCardSender{}, &fakeRunner{}, "owner")
	if _, err := o.handleCardCallback(context.Background(), &card.CardRequest{OutTrackId: "malformed"}); err != nil {
		t.Fatal(err)
	}
	noAssociation := cardClickReq("missing", "", approvalDecisionApprove, approvalActionApprove, "owner")
	if _, err := o.handleCardCallback(context.Background(), noAssociation); err != nil {
		t.Fatal(err)
	}

	req := gate.Submit(ApprovalRequest{Summary: "x"})
	gate.Decide(req.ID, false, "owner")
	duplicate := cardClickReq("", req.ID, approvalDecisionReject, approvalActionReject, "owner")
	if response, err := o.handleCardCallback(context.Background(), duplicate); err != nil || response.UserPrivateData.CardParamMap["dwsDecision"] != "拒绝" {
		t.Fatalf("duplicate response=%+v err=%v", response, err)
	}

	fallbackReject := &card.CardRequest{CardActionData: card.PrivateCardActionData{CardPrivateData: card.CardPrivateData{ActionIdList: []string{approvalActionReject}, Params: map[string]any{approvalParamID: "id"}}}}
	if id, approve, ok := decodeCardAction(fallbackReject); !ok || approve || id != "id" {
		t.Fatalf("fallback reject=(%q,%v,%v)", id, approve, ok)
	}
}

func TestCrossPlatformCoverageDingTalkApprovalCardDeliveryFailureEdge(t *testing.T) {
	recorder, server := newCardAPIServer(t)
	withCardAPIBase(t, server.URL)
	recorder.fail["POST /v1.0/card/instances/deliver"] = 500
	sender := newDingtalkApprovalCardSender("client", "secret", "template")
	if _, err := sender.SendApprovalCard(context.Background(), "owner", &ApprovalRequest{ID: "id", Summary: "x"}); err == nil {
		t.Fatal("delivery failure returned nil")
	}
}
