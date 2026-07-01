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

package source

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	dwsevent "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event"
	"github.com/gorilla/websocket"
	streamevent "github.com/open-dingtalk/dingtalk-stream-sdk-go/event"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/payload"
)

func TestPersonalSourceFetchTicketConnectsAndACKs(t *testing.T) {
	var wsEndpoint string
	ackCh := make(chan payload.DataFrameResponse, 1)
	upgrader := websocket.Upgrader{}
	mux := http.NewServeMux()
	mux.HandleFunc("/ticket", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-user-access-token"); got != "token-1" {
			t.Fatalf("x-user-access-token = %q", got)
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode ticket request: %v", err)
		}
		if req["sourceId"] != "open" || req["mode"] != "normal" {
			t.Fatalf("ticket request = %#v", req)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"result": map[string]any{
				"endpoint": wsEndpoint,
				"ticket":   "ticket-1",
			},
		})
	})
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("ticket"); got != "ticket-1" {
			t.Fatalf("ticket query = %q", got)
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade: %v", err)
		}
		defer conn.Close()
		df := payload.DataFrame{
			Type: "event",
			Headers: payload.DataFrameHeader{
				payload.DataFrameHeaderKMessageId:         "msg-1",
				streamevent.DataFrameHeaderKEventId:       "evt-1",
				streamevent.DataFrameHeaderKEventBornTime: "1234",
				streamevent.DataFrameHeaderKEventCorpId:   "corp-1",
				streamevent.DataFrameHeaderKEventType:     "im.message.mention_v1",
				"subscribeId":                             "sub-1",
				"ruleType":                                "at",
				"sourceId":                                "open",
			},
			Data: `{"message":{"text":"hi"}}`,
		}
		if err := conn.WriteJSON(df); err != nil {
			t.Fatalf("write dataframe: %v", err)
		}
		var ack payload.DataFrameResponse
		if err := conn.ReadJSON(&ack); err != nil {
			t.Fatalf("read ack: %v", err)
		}
		ackCh <- ack
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	wsEndpoint = "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"

	src, err := NewPersonal(PersonalConfig{
		AccessToken: "token-1",
		ClientID:    "client-1",
		SourceID:    "open",
		TicketURL:   srv.URL + "/ticket",
		TicketMode:  "normal",
		HTTPClient:  srv.Client(),
		Now:         func() time.Time { return time.Unix(10, 0) },
	})
	if err != nil {
		t.Fatalf("NewPersonal() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	evCh := make(chan *dwsevent.RawEvent, 1)
	done := make(chan error, 1)
	go func() {
		done <- src.Start(ctx, func(ev *dwsevent.RawEvent) { evCh <- ev })
	}()

	select {
	case ev := <-evCh:
		if ev.SubscribeID != "sub-1" || ev.RuleType != "at" || ev.SourceID != "open" {
			t.Fatalf("event personal fields = %#v", ev)
		}
		if ev.EventType != "im.message.mention_v1" || ev.EventID != "evt-1" {
			t.Fatalf("event = %#v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for event")
	}
	select {
	case ack := <-ackCh:
		if ack.Code != payload.DataFrameResponseStatusCodeKOK {
			t.Fatalf("ack code = %d", ack.Code)
		}
		if ack.GetHeader(payload.DataFrameHeaderKMessageId) != "msg-1" {
			t.Fatalf("ack message id = %q", ack.GetHeader(payload.DataFrameHeaderKMessageId))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for ack")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("source did not stop after cancel")
	}
}
