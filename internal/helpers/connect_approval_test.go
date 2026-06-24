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
	"testing"
	"time"
)

// TestApprovalGate_SubmitPending verifies Submit lands a request in Pending with
// a generated id and timestamp.
func TestApprovalGate_SubmitPending(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	g := newApprovalGate("client-A")
	req := g.Submit(ApprovalRequest{Requester: "u1", ConvID: "cid", Summary: "do thing"})
	if req.ID == "" {
		t.Fatal("Submit should assign an ID")
	}
	if req.State != approvalPending {
		t.Fatalf("state = %s, want pending", req.State)
	}
	if req.CreatedAt.IsZero() {
		t.Fatal("CreatedAt should be set")
	}
	if got := g.Get(req.ID); got == nil || got.State != approvalPending {
		t.Fatalf("Get returned %+v, want pending", got)
	}
}

// TestApprovalGate_DecideApproveReject covers both decision branches and the
// idempotency of a second decision.
func TestApprovalGate_DecideApproveReject(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	g := newApprovalGate("client-B")

	app := g.Submit(ApprovalRequest{Summary: "approve me"})
	decided, deciding := g.Decide(app.ID, true, "owner")
	if !deciding || decided.State != approvalApproved || decided.DecidedBy != "owner" {
		t.Fatalf("approve decision = %+v deciding=%v", decided, deciding)
	}
	// Second decision is a no-op.
	if _, again := g.Decide(app.ID, false, "owner"); again {
		t.Fatal("second Decide must not be the deciding call")
	}
	if g.Get(app.ID).State != approvalApproved {
		t.Fatal("state must stay approved after a redundant reject")
	}

	rej := g.Submit(ApprovalRequest{Summary: "reject me"})
	d2, _ := g.Decide(rej.ID, false, "owner")
	if d2.State != approvalRejected {
		t.Fatalf("reject decision = %s, want rejected", d2.State)
	}

	// Unknown id.
	if _, ok := g.Decide("nope", true, "owner"); ok {
		t.Fatal("deciding unknown id should report false")
	}
}

// TestApprovalGate_AwaitWakesOnDecide verifies Await blocks until a decision and
// returns the decided snapshot.
func TestApprovalGate_AwaitWakesOnDecide(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	g := newApprovalGate("client-C")
	app := g.Submit(ApprovalRequest{Summary: "wait for me"})

	go func() {
		time.Sleep(20 * time.Millisecond)
		g.Decide(app.ID, true, "owner")
	}()

	decided, got := g.Await(context.Background(), app.ID, time.Second)
	if !got || decided == nil || !decided.approved() {
		t.Fatalf("Await = %+v got=%v, want approved", decided, got)
	}
}

// TestApprovalGate_AwaitTimeout verifies a timeout leaves the request Pending and
// reports no decision.
func TestApprovalGate_AwaitTimeout(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	g := newApprovalGate("client-D")
	app := g.Submit(ApprovalRequest{Summary: "nobody decides"})
	decided, got := g.Await(context.Background(), app.ID, 10*time.Millisecond)
	if got {
		t.Fatal("Await should report no decision on timeout")
	}
	if decided.State != approvalPending {
		t.Fatalf("state = %s, want pending after timeout", decided.State)
	}
}

// TestApprovalGate_PersistAndRecover verifies a Pending request written by one
// gate is recovered by a fresh gate over the same config dir (restart survival).
func TestApprovalGate_PersistAndRecover(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", dir)

	g1 := newApprovalGate("client-E")
	app := g1.Submit(ApprovalRequest{Requester: "u9", Summary: "survive restart",
		Action: plannedAction{Product: "todo", Tool: "create_personal_todo"}})
	g1.setOutTrackID(app.ID, "card-123")

	// Fresh gate, same dir → must recover the request and its outTrackId.
	g2 := newApprovalGate("client-E")
	got := g2.Get(app.ID)
	if got == nil {
		t.Fatal("request not recovered from disk")
	}
	if got.State != approvalPending || got.Summary != "survive restart" {
		t.Fatalf("recovered = %+v", got)
	}
	if got.OutTrackID != "card-123" {
		t.Fatalf("recovered outTrackId = %q, want card-123", got.OutTrackID)
	}
	if byTrack := g2.findByOutTrackID("card-123"); byTrack == nil || byTrack.ID != app.ID {
		t.Fatal("findByOutTrackID failed after recovery")
	}
}

// TestApprovalGate_NoPersistWhenEmptyClient verifies an empty clientId keeps the
// gate in memory only (no disk writes, mirroring the session store).
func TestApprovalGate_NoPersistWhenEmptyClient(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	g := newApprovalGate("")
	if g.approvalDir() != "" {
		t.Fatal("empty clientId must disable persistence")
	}
	app := g.Submit(ApprovalRequest{Summary: "memory only"})
	if g.Get(app.ID) == nil {
		t.Fatal("in-memory request should still be retrievable")
	}
}

// TestParseActionMarker covers the simplified execution-class detection: a
// marker is extracted with its args and stripped from the human-facing reply,
// while plain Q&A is left untouched.
func TestParseActionMarker(t *testing.T) {
	reply := `好的，我来帮你建。[[ACTION:todo.create title="交方案" due="2026-06-14T18:00:00+08:00"]]`
	act, cleaned, found := parseActionMarker(reply)
	if !found {
		t.Fatal("marker should be detected")
	}
	if act.Verb != "todo.create" {
		t.Fatalf("verb = %s", act.Verb)
	}
	if act.Args["title"] != "交方案" || act.Args["due"] != "2026-06-14T18:00:00+08:00" {
		t.Fatalf("args = %+v", act.Args)
	}
	if cleaned != "好的，我来帮你建。" {
		t.Fatalf("cleaned = %q", cleaned)
	}

	if _, _, f := parseActionMarker("这是一个普通问题的回答，没有动作。"); f {
		t.Fatal("plain reply must not match")
	}
}

// TestToPlannedAction verifies the marker → command mapping, including the
// reject of a malformed (titleless) action.
func TestToPlannedAction(t *testing.T) {
	pa, summary, ok := toPlannedAction(detectedAction{
		Verb: "todo.create",
		Args: map[string]string{"title": "交方案", "due": "2026-06-14T18:00:00+08:00"},
	}, "owner-1")
	if !ok {
		t.Fatal("valid todo.create should map")
	}
	if pa.Product != "todo" || pa.Tool != "create_personal_todo" {
		t.Fatalf("planned = %+v", pa)
	}
	vo, _ := pa.Params["PersonalTodoCreateVO"].(map[string]any)
	if vo == nil || vo["subject"] != "交方案" {
		t.Fatalf("vo = %+v", vo)
	}
	if _, hasDue := vo["dueTime"]; !hasDue {
		t.Fatal("due should be parsed into dueTime")
	}
	execs, _ := vo["executorIds"].([]string)
	if len(execs) != 1 || execs[0] != "owner-1" {
		t.Fatalf("executors = %+v, want [owner-1]", execs)
	}
	if summary == "" {
		t.Fatal("summary should be non-empty")
	}

	if _, _, ok := toPlannedAction(detectedAction{Verb: "todo.create", Args: map[string]string{}}, "o"); ok {
		t.Fatal("titleless action must be rejected")
	}
	if _, _, ok := toPlannedAction(detectedAction{Verb: "unknown.verb"}, "o"); ok {
		t.Fatal("unknown verb must be rejected")
	}
}
