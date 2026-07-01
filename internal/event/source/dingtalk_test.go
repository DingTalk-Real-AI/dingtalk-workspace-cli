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
	"testing"
	"time"

	dwsevent "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/event"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/payload"
)

func TestNew_RequiresClientID(t *testing.T) {
	if _, err := New(Config{ClientSecret: "secret"}); err == nil {
		t.Fatal("expected error when ClientID empty")
	}
}

func TestNew_RequiresClientSecret(t *testing.T) {
	if _, err := New(Config{ClientID: "id"}); err == nil {
		t.Fatal("expected error when ClientSecret empty")
	}
}

func TestNew_DefaultsNow(t *testing.T) {
	s, err := New(Config{ClientID: "id", ClientSecret: "secret"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if s.cfg.Now == nil {
		t.Fatal("Now should default to time.Now")
	}
}

func TestNew_AcceptsPortalTicketNormalWithoutClientSecret(t *testing.T) {
	if _, err := New(Config{
		ClientID: "id",
		PortalTicket: &PortalTicketConfig{
			TicketURL:   "https://example.com/stream/connections/ticket",
			AccessToken: "token",
			SourceID:    "pre_open_source",
			Mode:        "normal",
		},
	}); err != nil {
		t.Fatalf("New: %v", err)
	}
}

func TestNew_RejectsPortalTicketCustomWithoutSecret(t *testing.T) {
	if _, err := New(Config{
		ClientID: "id",
		PortalTicket: &PortalTicketConfig{
			TicketURL:   "https://example.com/stream/connections/ticket",
			AccessToken: "token",
			SourceID:    "pre_open_source",
			Mode:        "custom",
			ClientID:    "custom_client",
		},
	}); err == nil {
		t.Fatal("expected error when custom portal ticket secret is empty")
	}
}

func TestRequestPortalTicketCustomBody(t *testing.T) {
	var got struct {
		SourceID     string `json:"sourceId"`
		ChannelType  string `json:"channelType"`
		Mode         string `json:"mode"`
		ClientID     string `json:"clientId"`
		ClientSecret string `json:"clientSecret"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token := r.Header.Get("x-user-access-token"); token != "token-123" {
			t.Fatalf("x-user-access-token = %q", token)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"result": map[string]string{
				"endpoint": "wss://pre-wss-open-connection.dingtalk.com/connect",
				"ticket":   "ticket-123",
			},
		})
	}))
	defer srv.Close()

	ticket, err := requestPortalTicket(context.Background(), &PortalTicketConfig{
		TicketURL:    srv.URL,
		AccessToken:  "token-123",
		SourceID:     "pre_open_source",
		Mode:         "custom",
		ClientID:     "custom_client",
		ClientSecret: "custom_secret",
	})
	if err != nil {
		t.Fatalf("requestPortalTicket: %v", err)
	}
	if ticket.Endpoint == "" || ticket.Ticket == "" {
		t.Fatalf("ticket = %#v", ticket)
	}
	if got.SourceID != "pre_open_source" || got.ChannelType != "pre_open_source" {
		t.Fatalf("source fields = %#v", got)
	}
	if got.Mode != "custom" || got.ClientID != "custom_client" || got.ClientSecret != "custom_secret" {
		t.Fatalf("custom fields = %#v", got)
	}
}

func TestState_InitiallyDisconnected(t *testing.T) {
	s, _ := New(Config{ClientID: "id", ClientSecret: "secret"})
	if got := s.State().State; got != StateDisconnected {
		t.Fatalf("initial state = %s, want %s", got, StateDisconnected)
	}
}

func TestMakeHandler_TranslatesDataFrameToRawEvent(t *testing.T) {
	fixed := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	s, err := New(Config{
		ClientID:     "id",
		ClientSecret: "secret",
		Now:          func() time.Time { return fixed },
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	var got *dwsevent.RawEvent
	handler := s.makeHandler(func(e *dwsevent.RawEvent) { got = e })

	df := &payload.DataFrame{
		SpecVersion: "1.0",
		Type:        "EVENT",
		Time:        1700000000,
		Headers: payload.DataFrameHeader{
			"eventId":           "ev_abc",
			"eventBornTime":     "1700000000123",
			"eventCorpId":       "corp_x",
			"eventType":         "im.message.receive_v1",
			"eventUnifiedAppId": "app_y",
			"extra":             "passthrough",
		},
		Data: `{"chat":"hello"}`,
	}

	resp, err := handler(context.Background(), df)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if resp == nil {
		t.Fatal("handler returned nil resp")
	}

	if got == nil {
		t.Fatal("emit was not called")
	}
	if got.EventID != "ev_abc" {
		t.Errorf("EventID = %q, want %q", got.EventID, "ev_abc")
	}
	if got.EventType != "im.message.receive_v1" {
		t.Errorf("EventType = %q", got.EventType)
	}
	if got.EventCorpID != "corp_x" {
		t.Errorf("EventCorpID = %q", got.EventCorpID)
	}
	if got.EventUnifiedAppID != "app_y" {
		t.Errorf("EventUnifiedAppID = %q", got.EventUnifiedAppID)
	}
	if got.EventBornTime != 1700000000123 {
		t.Errorf("EventBornTime = %d", got.EventBornTime)
	}
	if got.Data != `{"chat":"hello"}` {
		t.Errorf("Data = %q", got.Data)
	}
	if !got.ReceivedAt.Equal(fixed) {
		t.Errorf("ReceivedAt = %v, want %v (injected Now)", got.ReceivedAt, fixed)
	}
	if got.Headers["extra"] != "passthrough" {
		t.Error("passthrough header lost")
	}

	// Also verify the connection state machine ticked: handler should have
	// called OnEvent, but since we never called OnConnected the visible
	// state remains disconnected. We only verify the lastEventAt timestamp.
	snap := s.State()
	if snap.LastEventAt.IsZero() {
		t.Error("OnEvent should have updated lastEventAt")
	}
}

func TestStart_RejectsNilEmit(t *testing.T) {
	s, _ := New(Config{ClientID: "id", ClientSecret: "secret"})
	err := s.Start(context.Background(), nil)
	if err == nil || err.Error() == "" {
		t.Fatalf("Start with nil emit should error, got %v", err)
	}
}

func TestMakeHandler_NilHeaders(t *testing.T) {
	s, _ := New(Config{ClientID: "id", ClientSecret: "secret"})
	handler := s.makeHandler(func(*dwsevent.RawEvent) {})

	df := &payload.DataFrame{
		// no Headers at all
		Data: "{}",
	}
	resp, err := handler(context.Background(), df)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if resp == nil {
		t.Fatal("nil resp")
	}
}

// Compile-time guard: the EventProcessResult we return must be the SDK's
// EventProcessResultSuccess type. If the SDK ever changes the shape, this
// will fail to compile.
var _ = event.NewEventProcessResultSuccess
