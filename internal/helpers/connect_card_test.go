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
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/open-dingtalk/dingtalk-stream-sdk-go/chatbot"
)

// cardAPIRecorder fakes the DingTalk card endpoints and records every call.
type cardAPIRecorder struct {
	mu     sync.Mutex
	calls  []string         // "METHOD path"
	bodies []map[string]any // parsed request bodies, same order
	tokens int              // accessToken request count
	fail   map[string]int   // "METHOD path" -> HTTP status to return
}

func newCardAPIServer(t *testing.T) (*cardAPIRecorder, *httptest.Server) {
	t.Helper()
	rec := &cardAPIRecorder{fail: map[string]int{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		key := r.Method + " " + r.URL.Path

		rec.mu.Lock()
		if r.URL.Path == "/v1.0/oauth2/accessToken" {
			rec.tokens++
		} else {
			rec.calls = append(rec.calls, key)
			rec.bodies = append(rec.bodies, body)
		}
		status := rec.fail[key]
		rec.mu.Unlock()

		if status != 0 {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"code":"boom"}`))
			return
		}
		if r.URL.Path == "/v1.0/oauth2/accessToken" {
			_, _ = w.Write([]byte(`{"accessToken":"tok-1","expireIn":7200}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	t.Cleanup(srv.Close)
	return rec, srv
}

func withCardAPIBase(t *testing.T, base string) {
	t.Helper()
	old := dingtalkCardAPIBase
	dingtalkCardAPIBase = base
	oldGap, oldRepair := aiCardFrameGap, aiCardRepairDelay
	aiCardFrameGap, aiCardRepairDelay = 0, 0
	t.Cleanup(func() {
		dingtalkCardAPIBase = old
		aiCardFrameGap, aiCardRepairDelay = oldGap, oldRepair
	})
}

func groupCallback() *chatbot.BotCallbackDataModel {
	return &chatbot.BotCallbackDataModel{
		ConversationId:   "cid-group-1",
		ConversationType: "2",
		SenderStaffId:    "staff-1",
	}
}

// TestAICardCreateFinalizeSequence verifies the hermes-contract happy path:
// create (empty content = Thinking) → deliver → ONE finalized streaming frame.
func TestAICardCreateFinalizeSequence(t *testing.T) {
	rec, srv := newCardAPIServer(t)
	withCardAPIBase(t, srv.URL)

	c := newAICardClient("ding-client", "ding-secret", defaultAICardTemplateID)
	card, err := c.createAndDeliver(context.Background(), groupCallback())
	if err != nil {
		t.Fatalf("createAndDeliver: %v", err)
	}
	if !strings.HasPrefix(card.outTrackID, "dws_") {
		t.Fatalf("outTrackID = %q, want dws_ prefix", card.outTrackID)
	}
	if err := c.finalize(context.Background(), card, "你好，**答案是 42**"); err != nil {
		t.Fatalf("finalize: %v", err)
	}

	wantSeq := []string{
		"POST /v1.0/card/instances",
		"POST /v1.0/card/instances/deliver",
		"PUT /v1.0/card/instances", // INPUTING + content
		"PUT /v1.0/card/streaming", // finalize frame
		"PUT /v1.0/card/instances", // FINISHED
	}
	if strings.Join(rec.calls, ",") != strings.Join(wantSeq, ",") {
		t.Fatalf("call sequence = %v, want %v", rec.calls, wantSeq)
	}
	if rec.tokens != 1 {
		t.Fatalf("token requests = %d, want 1 (cached)", rec.tokens)
	}

	// create: hermes template + empty "content" param (renders Thinking)
	create := rec.bodies[0]
	if create["cardTemplateId"] != defaultAICardTemplateID || create["callbackType"] != "STREAM" {
		t.Fatalf("create payload wrong: %v", create)
	}
	pm := create["cardData"].(map[string]any)["cardParamMap"].(map[string]any)
	if _, ok := pm["config"]; !ok {
		t.Fatalf("create cardParamMap = %v, want config key (openclaw contract)", pm)
	}

	// deliver: group space + robotCode
	deliver := rec.bodies[1]
	if deliver["openSpaceId"] != "dtv1.card//IM_GROUP.cid-group-1" {
		t.Fatalf("deliver openSpaceId = %v", deliver["openSpaceId"])
	}
	if deliver["imGroupOpenDeliverModel"].(map[string]any)["robotCode"] != "ding-client" {
		t.Fatalf("deliver robotCode missing: %v", deliver)
	}

	// INPUTING then FINISHED flowStatus around a finalized streaming frame
	statusOf := func(b map[string]any) string {
		pm := b["cardData"].(map[string]any)["cardParamMap"].(map[string]any)
		v, _ := pm["flowStatus"].(string)
		return v
	}
	if statusOf(rec.bodies[2]) != aiCardFlowInputing {
		t.Fatalf("flowStatus[2] = %v, want INPUTING", statusOf(rec.bodies[2]))
	}
	stream := rec.bodies[3]
	if stream["key"] != "msgContent" || stream["isFinalize"] != true || stream["isError"] != false {
		t.Fatalf("finalize frame wrong: %v", stream)
	}
	if statusOf(rec.bodies[4]) != aiCardFlowFinished {
		t.Fatalf("flowStatus[4] = %v, want FINISHED", statusOf(rec.bodies[4]))
	}
}

// TestAICardDeliverOneToOne checks 1:1 messages deliver into the IM_ROBOT
// space with the hermes shape (spaceType only, no robotCode).
func TestAICardDeliverOneToOne(t *testing.T) {
	rec, srv := newCardAPIServer(t)
	withCardAPIBase(t, srv.URL)

	c := newAICardClient("ding-client", "ding-secret", defaultAICardTemplateID)
	data := &chatbot.BotCallbackDataModel{ConversationType: "1", SenderStaffId: "staff-9"}
	if _, err := c.createAndDeliver(context.Background(), data); err != nil {
		t.Fatalf("createAndDeliver: %v", err)
	}
	deliver := rec.bodies[1]
	if deliver["openSpaceId"] != "dtv1.card//IM_ROBOT.staff-9" {
		t.Fatalf("deliver openSpaceId = %v, want IM_ROBOT.staff-9", deliver["openSpaceId"])
	}
	model := deliver["imRobotOpenDeliverModel"].(map[string]any)
	if model["spaceType"] != "IM_ROBOT" || model["robotCode"] != "ding-client" {
		t.Fatalf("imRobotOpenDeliverModel = %v", model)
	}
}

// TestAICardCreateFailure checks errors surface so callers fall back to plain
// replies (never silently lost), and that missing staffId in 1:1 is rejected.
func TestAICardCreateFailure(t *testing.T) {
	rec, srv := newCardAPIServer(t)
	withCardAPIBase(t, srv.URL)
	rec.fail["POST /v1.0/card/instances"] = 500

	c := newAICardClient("ding-client", "ding-secret", defaultAICardTemplateID)
	if _, err := c.createAndDeliver(context.Background(), groupCallback()); err == nil {
		t.Fatal("want error when card create fails")
	}

	rec.fail = map[string]int{}
	if _, err := c.createAndDeliver(context.Background(),
		&chatbot.BotCallbackDataModel{ConversationType: "1"}); err == nil {
		t.Fatal("want error for 1:1 without senderStaffId")
	}
}

// TestAICardMarkFailed checks the stuck-Thinking remedy: a finalized error
// frame, errors swallowed.
func TestAICardMarkFailed(t *testing.T) {
	rec, srv := newCardAPIServer(t)
	withCardAPIBase(t, srv.URL)

	c := newAICardClient("ding-client", "ding-secret", defaultAICardTemplateID)
	c.markFailed(context.Background(), &aiCardInstance{outTrackID: "dws_x"})
	last := rec.bodies[len(rec.bodies)-1]
	pm := last["cardData"].(map[string]any)["cardParamMap"].(map[string]any)
	if pm["flowStatus"] != aiCardFlowFailed {
		t.Fatalf("markFailed frame wrong: %v", last)
	}
}

// TestRobotConnectReplyCardFlag checks the flag default and dry-run surface.
func TestRobotConnectReplyCardFlag(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"default: no template -> text + emotions", nil, "thinking/done表态"},
		{"template configured -> ai-card", []string{"--card-template", "tpl-1.schema"}, `"replyStyle": "ai-card"`},
		{"public alias -> ai-card", []string{"--card-template", "public"}, `"replyStyle": "ai-card"`},
		{"explicit off", []string{"--reply-card=false"}, `"replyStyle": "text/markdown"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := newDevAppTestRoot(&captureRunner{})
			var out strings.Builder
			root.SetOut(&out)
			root.SetErr(&out)
			args := append([]string{"dev", "connect",
				"--channel", "claudecode",
				"--robot-client-id", "a", "--robot-client-secret", "b", "--dry-run"}, tc.args...)
			root.SetArgs(args)
			if err := root.Execute(); err != nil {
				t.Fatalf("Execute: %v\n%s", err, out.String())
			}
			if !strings.Contains(out.String(), tc.want) {
				t.Fatalf("missing %q in:\n%s", tc.want, out.String())
			}
		})
	}
}

// TestAICardEmotions covers the Thinking chip contract: reply on receive,
// recall+Done on completion (POST /v1.0/robot/emotion/*).
func TestAICardEmotions(t *testing.T) {
	rec, srv := newCardAPIServer(t)
	withCardAPIBase(t, srv.URL)

	c := newAICardClient("ding-client", "ding-secret", defaultAICardTemplateID)
	if err := c.markThinking(context.Background(), "cid-1", "msg-1"); err != nil {
		t.Fatalf("markThinking: %v", err)
	}
	c.swapThinkingToDone(context.Background(), "cid-1", "msg-1")

	wantSeq := []string{
		"POST /v1.0/robot/emotion/reply",  // Thinking
		"POST /v1.0/robot/emotion/recall", // Thinking recalled
		"POST /v1.0/robot/emotion/reply",  // Done
	}
	if strings.Join(rec.calls, ",") != strings.Join(wantSeq, ",") {
		t.Fatalf("call sequence = %v, want %v", rec.calls, wantSeq)
	}
	first := rec.bodies[0]
	if first["emotionName"] != "🤔Thinking" || first["openMsgId"] != "msg-1" ||
		first["robotCode"] != "ding-client" {
		t.Fatalf("thinking payload wrong: %v", first)
	}
	te := first["textEmotion"].(map[string]any)
	if te["emotionId"] != "2659900" || te["backgroundId"] != "im_bg_1" || te["text"] != "🤔Thinking" {
		t.Fatalf("textEmotion wrong: %v", te)
	}
	if rec.bodies[2]["emotionName"] != "🥳Done" {
		t.Fatalf("done payload wrong: %v", rec.bodies[2])
	}

	// Missing ids must error (e.g. payloads without MsgId).
	if err := c.markThinking(context.Background(), "", "msg-1"); err == nil {
		t.Fatal("want error for missing conversation id")
	}
}

// TestAICardCustomTemplate checks --card-template plumbs through to create.
func TestAICardCustomTemplate(t *testing.T) {
	rec, srv := newCardAPIServer(t)
	withCardAPIBase(t, srv.URL)

	c := newAICardClient("ding-client", "ding-secret", "my-own-template.schema")
	if _, err := c.createAndDeliver(context.Background(), groupCallback()); err != nil {
		t.Fatalf("createAndDeliver: %v", err)
	}
	if rec.bodies[0]["cardTemplateId"] != "my-own-template.schema" {
		t.Fatalf("cardTemplateId = %v, want custom template", rec.bodies[0]["cardTemplateId"])
	}
}

func TestAICardOTOAndRepairCoverage(t *testing.T) {
	rec, srv := newCardAPIServer(t)
	withCardAPIBase(t, srv.URL)
	c := newAICardClient("ding-client", "ding-secret", defaultAICardTemplateID)
	if err := c.sendOTOText(context.Background(), nil, "ignored"); err != nil {
		t.Fatal(err)
	}
	if err := c.sendOTOText(context.Background(), []string{"u1", "u2"}, "hello"); err != nil {
		t.Fatal(err)
	}
	c.repair(context.Background(), &aiCardInstance{outTrackID: "track"}, "fixed")
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	oldDelay := aiCardRepairDelay
	aiCardRepairDelay = time.Hour
	c.repair(cancelled, &aiCardInstance{outTrackID: "track"}, "ignored")
	aiCardRepairDelay = oldDelay
	if len(rec.calls) < 2 || rec.calls[0] != "POST /v1.0/robot/oToMessages/batchSend" {
		t.Fatalf("OTO/repair calls = %#v", rec.calls)
	}
}

func TestAtMentionSendGroupReplyCoverage(t *testing.T) {
	_, srv := newCardAPIServer(t)
	withCardAPIBase(t, srv.URL)
	poller := &atMentionPoller{
		clientID:  "ding-client",
		channel:   "test",
		botClient: newAICardClient("ding-client", "ding-secret", ""),
	}
	if err := poller.sendGroupReply(context.Background(), "conversation", "reply"); err != nil {
		t.Fatal(err)
	}

	oldBase := dingtalkCardAPIBase
	dingtalkCardAPIBase = "%"
	if err := poller.sendGroupReply(context.Background(), "conversation", "reply"); err == nil {
		t.Fatal("invalid group reply URL succeeded")
	}
	dingtalkCardAPIBase = oldBase
}
