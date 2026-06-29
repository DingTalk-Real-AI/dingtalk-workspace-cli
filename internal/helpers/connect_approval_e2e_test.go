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
)

// TestApprovalGate_EndToEndApprove exercises the whole vertical slice with no
// network: an agent reply carrying a todo.create marker → Submit → approval card
// → owner taps [Approve] via a card callback → the planned todo.create runs on
// the (fake) runner → the group is told it was executed.
//
// It mirrors exactly what runStreamConnector wires together, but with the card
// sender and runner faked, so the orchestration logic is verified end to end.
func TestApprovalGate_EndToEndApprove(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())

	gate := newApprovalGate("e2e-client")
	sender := &fakeCardSender{}
	runner := &fakeRunner{}
	o := newApprovalOrchestrator(gate, sender, runner, "owner-007")
	o.awaitWindow = 2 * time.Second

	var replies []string
	var rmu sync.Mutex
	groupReply := func(_ context.Context, _, text string) error {
		rmu.Lock()
		replies = append(replies, text)
		rmu.Unlock()
		return nil
	}

	// Simulate the owner tapping [Approve] once the card has been delivered, via
	// the exact card-callback path the Stream router would invoke.
	go func() {
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			sender.mu.Lock()
			track := sender.lastTrack
			sender.mu.Unlock()
			if track != "" {
				_, _ = o.handleCardCallback(context.Background(), &card.CardRequest{
					OutTrackId: track,
					UserId:     "owner-007",
					CardActionData: card.PrivateCardActionData{
						CardPrivateData: card.CardPrivateData{
							ActionIdList: []string{approvalActionApprove},
							Params: map[string]any{
								approvalParamDecision: approvalDecisionApprove,
							},
						},
					},
				})
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()

	// This is the agent reply for: 群里有人 @机器人 "帮我建个待办：明天下班前交方案".
	agentReply := `收到，我来帮你建这个待办。[[ACTION:todo.create title="明天下班前交方案" due="2026-06-14T18:00:00+08:00"]]`

	out, handled := o.handleReply(context.Background(), "requester-zhang", "group-conv-9", agentReply, groupReply)

	if !handled {
		t.Fatal("action reply must be handled by the gate")
	}
	if out != "" {
		t.Fatalf("handled reply returns empty connector reply, got %q", out)
	}

	// The planned command ran exactly once, and it was todo.create.
	if runner.count() != 1 {
		t.Fatalf("expected exactly 1 execution, got %d", runner.count())
	}
	runner.mu.Lock()
	inv := runner.runs[0]
	runner.mu.Unlock()
	if inv.CanonicalProduct != "todo" || inv.Tool != "create_personal_todo" {
		t.Fatalf("executed wrong command: %s.%s", inv.CanonicalProduct, inv.Tool)
	}
	vo, _ := inv.Params["PersonalTodoCreateVO"].(map[string]any)
	if vo == nil || vo["subject"] != "明天下班前交方案" {
		t.Fatalf("todo subject wrong: %+v", vo)
	}

	// State machine reached Executed.
	final := gate.list()[0]
	if final.State != approvalExecuted {
		t.Fatalf("final state = %s, want executed", final.State)
	}

	// The owner saw [Approve]/[Reject] (the card was delivered), and the group
	// got the executed-confirmation reply.
	if len(sender.sent) != 1 {
		t.Fatalf("expected 1 card delivered, got %d", len(sender.sent))
	}
	rmu.Lock()
	joined := strings.Join(replies, " | ")
	rmu.Unlock()
	if !strings.Contains(joined, "已同意") || !strings.Contains(joined, "已执行") {
		t.Fatalf("group not told of execution: %q", joined)
	}

	t.Logf("E2E approve chain OK:\n"+
		"  card delivered to owner: %v\n"+
		"  approval state: %s\n"+
		"  command executed: %s.%s (subject=%q)\n"+
		"  group replies: %s",
		sender.sent[0].Summary, final.State, inv.CanonicalProduct, inv.Tool, vo["subject"], joined)
}
