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
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	dwsevent "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/bus"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
	"github.com/gorilla/websocket"
	streamevent "github.com/open-dingtalk/dingtalk-stream-sdk-go/event"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/payload"
)

func TestPersonalSourceFetchTicketConnectsAndACKs(t *testing.T) {
	logs := capturePersonalSourceDebugLogs(t)
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
				streamevent.DataFrameHeaderKEventType:     "user_im_message_receive_at",
				"subscribeId":                             "sub-1",
				"ruleType":                                "at",
				"sourceId":                                "open",
				"accessToken":                             "header-secret-token",
			},
			Data: `{"message":{"text":"hi"},"access_token":"data-secret-token","client_secret":"data-secret","ticket":"data-ticket","Authorization":"Bearer data-auth"}`,
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
		if ev.EventType != "user_im_message_receive_at" || ev.EventID != "evt-1" {
			t.Fatalf("event = %#v", ev)
		}
		out := logs.String()
		for _, want := range []string{"personal source received dataframe", "user_im_message_receive_at", "evt-1", "sub-1", "sourceId", "message", "<redacted>"} {
			if !strings.Contains(out, want) {
				t.Fatalf("debug log missing %q: %s", want, out)
			}
		}
		for _, leaked := range []string{"header-secret-token", "data-secret-token", "data-secret", "data-ticket", "Bearer data-auth"} {
			if strings.Contains(out, leaked) {
				t.Fatalf("debug log leaked %q: %s", leaked, out)
			}
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

func TestPersonalSourceParsesUppercaseHeaders(t *testing.T) {
	src := personalSourceForRawEventTests()
	data := `{"payload":true}`
	raw := src.rawEventFromDataFrame(&payload.DataFrame{
		Time: 12345,
		Headers: payload.DataFrameHeader{
			"EVENT_TYPE": "user_im_message_receive_o2o",
			"SUB_ID":     "sub-1",
			"SOURCE_ID":  "pre_open_source",
			"MESSAGE_ID": "evt-1",
		},
		Data: data,
	})

	if raw.EventType != "user_im_message_receive_o2o" {
		t.Fatalf("EventType = %q", raw.EventType)
	}
	if raw.SubscribeID != "sub-1" {
		t.Fatalf("SubscribeID = %q", raw.SubscribeID)
	}
	if raw.SourceID != "pre_open_source" {
		t.Fatalf("SourceID = %q", raw.SourceID)
	}
	if raw.EventID != "evt-1" {
		t.Fatalf("EventID = %q", raw.EventID)
	}
	if raw.EventScope != "personal" {
		t.Fatalf("EventScope = %q", raw.EventScope)
	}
	if raw.Data != data {
		t.Fatalf("Data changed: %q", raw.Data)
	}
}

func TestPersonalSourceParsesTopicFallbackAndIgnoresWildcard(t *testing.T) {
	src := personalSourceForRawEventTests()
	raw := src.rawEventFromDataFrame(&payload.DataFrame{
		Headers: payload.DataFrameHeader{
			"TOPIC": "user_im_message_receive_o2o",
			"topic": "*",
		},
	})
	if raw.EventType != "user_im_message_receive_o2o" {
		t.Fatalf("EventType from TOPIC = %q", raw.EventType)
	}

	raw = src.rawEventFromDataFrame(&payload.DataFrame{
		Headers: payload.DataFrameHeader{"topic": "*"},
	})
	if raw.EventType != "" {
		t.Fatalf("EventType from wildcard topic = %q, want empty", raw.EventType)
	}
}

func TestPersonalSourceParsesDataFallbackWithoutChangingData(t *testing.T) {
	src := personalSourceForRawEventTests()
	payloadJSON := `{"eventKey":"user_im_message_receive_o2o","subId":"sub-data","eventId":"evt-data","ext":{"ruleType":"singleChat"}}`
	encodedPayload, err := json.Marshal(payloadJSON)
	if err != nil {
		t.Fatal(err)
	}
	data := string(encodedPayload)

	raw := src.rawEventFromDataFrame(&payload.DataFrame{Data: data})
	if raw.EventType != "user_im_message_receive_o2o" {
		t.Fatalf("EventType = %q", raw.EventType)
	}
	if raw.SubscribeID != "sub-data" {
		t.Fatalf("SubscribeID = %q", raw.SubscribeID)
	}
	if raw.EventID != "evt-data" {
		t.Fatalf("EventID = %q", raw.EventID)
	}
	if raw.RuleType != "singleChat" {
		t.Fatalf("RuleType = %q", raw.RuleType)
	}
	if raw.Data != data {
		t.Fatalf("Data changed: %q", raw.Data)
	}
}

func TestPersonalSourceParsesSourceTagDataFallback(t *testing.T) {
	src := personalSourceForRawEventTests()
	data := `{"source":{"tag":"user_im_message_receive_group"},"subId":"sub-group"}`

	raw := src.rawEventFromDataFrame(&payload.DataFrame{Data: data})
	if raw.EventType != "user_im_message_receive_group" {
		t.Fatalf("EventType = %q", raw.EventType)
	}
	if raw.SubscribeID != "sub-group" {
		t.Fatalf("SubscribeID = %q", raw.SubscribeID)
	}
}

func TestPersonalSourceParsedHeadersPassNormalBusFilter(t *testing.T) {
	src := personalSourceForRawEventTests()
	raw := src.rawEventFromDataFrame(&payload.DataFrame{
		Headers: payload.DataFrameHeader{
			"EVENT_TYPE": "user_im_message_receive_o2o",
			"SUB_ID":     "sub-1",
		},
	})
	h := bus.NewHub(10)
	c, err := h.Register(transport.Hello{
		EventTypes:  []string{"user_im_message_receive_o2o"},
		SubscribeID: "sub-1",
	})
	if err != nil {
		t.Fatal(err)
	}

	h.Deliver(raw)

	select {
	case frame := <-c.SendCh:
		ev, ok := frame.(transport.Event)
		if !ok {
			t.Fatalf("frame = %T, want transport.Event", frame)
		}
		if ev.EventType != "user_im_message_receive_o2o" || ev.SubscribeID != "sub-1" {
			t.Fatalf("event = %#v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for filtered event")
	}
}

func personalSourceForRawEventTests() *PersonalSource {
	return &PersonalSource{cfg: PersonalConfig{
		SourceID: "fallback_source",
		Now:      func() time.Time { return time.Unix(20, 0) },
	}}
}

func capturePersonalSourceDebugLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() {
		slog.SetDefault(previous)
	})
	return &buf
}
