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

package bus

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"

	dwsevent "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/dedup"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
)

func TestCrossPlatformCoverageEventDeliveryOverlappingSubscriptions(t *testing.T) {
	for name, order := range map[string][]string{
		"old-target-first":       {"old", "current", "current"},
		"all-direct-first":       {"all-direct", "current", "current"},
		"current-first":          {"current", "old", "current"},
		"same-target-redelivery": {"current", "current"},
	} {
		t.Run(name, func(t *testing.T) {
			hub := NewHub(10)
			consumer, err := hub.Register(transport.Hello{EventTypes: []string{"user_im_message_receive_o2o"}, SubscribeID: "current"})
			if err != nil {
				t.Fatal(err)
			}
			d := daemon{hub: hub, dedup: dedup.New()}
			d.emit(nil)
			for _, sub := range order {
				d.emit(&dwsevent.RawEvent{EventID: "same-message", EventType: "user_im_message_receive_o2o", SubscribeID: sub})
			}
			if consumer.received.Load() != 1 || consumer.dropped.Load() != 0 || len(consumer.SendCh) != 1 {
				t.Fatalf("current delivery lost or duplicated: received=%d dropped=%d frames=%d",
					consumer.received.Load(), consumer.dropped.Load(), len(consumer.SendCh))
			}
		})
	}
}

func TestCrossPlatformCoverageEventDeliveryDifferentTypesAndFallback(t *testing.T) {
	for _, eventID := range []string{"same-event", ""} {
		hub := NewHub(10)
		consumer, err := hub.Register(transport.Hello{SubscribeID: "current"})
		if err != nil {
			t.Fatal(err)
		}
		d := daemon{hub: hub, dedup: dedup.New()}
		for _, sub := range []string{"old", "current"} {
			for _, typ := range []string{"user_im_message_receive_o2o", "user_im_message_receive_user"} {
				raw := &dwsevent.RawEvent{EventID: eventID, EventType: typ, SubscribeID: sub, Data: `{"msgId":"same"}`}
				d.emit(raw)
				d.emit(raw)
			}
		}
		if consumer.received.Load() != 2 || len(consumer.SendCh) != 2 {
			t.Fatalf("ID=%q: expected one copy of each event type; received=%d frames=%d", eventID, consumer.received.Load(), len(consumer.SendCh))
		}
	}
}

func TestCrossPlatformCoverageEventDeliveryAuditMetadataOnly(t *testing.T) {
	var logs bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	hub := NewHub(10)
	if _, err := hub.Register(transport.Hello{EventTypes: []string{"user_im_message_receive_o2o"}, SubscribeID: "current"}); err != nil {
		t.Fatal(err)
	}
	d := daemon{hub: hub, dedup: dedup.New(), audit: newDeliveryAudit(log)}
	raw := &dwsevent.RawEvent{EventID: "event-1", EventType: "user_im_message_receive_o2o",
		SubscribeID: "old", SourceID: "source-1", Data: `{"content":"SECRET-BODY"}`,
		Headers: map[string]string{"authorization": "SECRET-TOKEN"}}
	d.emit(raw) // Unmatched old subscription.
	current := *raw
	current.SubscribeID = "current"
	d.emit(&current)
	d.emit(&current) // Same subscription redelivery, not a different subscription.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	d.audit.run(ctx)
	if strings.Contains(logs.String(), "SECRET-") || strings.Contains(logs.String(), "authorization") {
		t.Fatalf("business data or credentials leaked: %s", logs.String())
	}
	dec := json.NewDecoder(&logs)
	for _, outcome := range []string{"no_matching_consumer", "routed", "duplicate_redelivery"} {
		var row map[string]any
		if err := dec.Decode(&row); err != nil {
			t.Fatal(err)
		}
		if row["outcome"] != outcome || row["event_id"] != "event-1" || row["source_id"] != "source-1" {
			t.Fatalf("wrong audit metadata: %#v", row)
		}
		if outcome == "routed" && (row["matched_consumers"] != float64(1) || row["consumers"] != float64(1)) {
			t.Fatalf("wrong routing counts: %#v", row)
		}
	}
}

func TestCrossPlatformCoverageEventDeliveryAuditNonBlockingQueue(t *testing.T) {
	var logs bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logs, nil))
	audit := newDeliveryAudit(log)
	raw := &dwsevent.RawEvent{EventID: "same", SubscribeID: "current", EventType: "im.message"}
	audit.record(raw, "routed", 1, 1)
	if len(audit.records) != 0 {
		t.Fatal("normal deliveries should not fill diagnostics at default log level")
	}
	for range cap(audit.records) + 3 {
		audit.record(raw, "duplicate_redelivery", 0, 0)
	}
	if audit.dropped.Load() != 3 {
		t.Fatalf("expected bounded non-blocking diagnostic drops, got %d", audit.dropped.Load())
	}
	hub := NewHub(10)
	consumer, err := hub.Register(transport.Hello{SubscribeID: "current"})
	if err != nil {
		t.Fatal(err)
	}
	d := daemon{hub: hub, dedup: dedup.New(), audit: audit}
	d.emit(raw)
	if consumer.received.Load() != 1 {
		t.Fatal("full diagnostic queue must not prevent business delivery")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	audit.run(ctx)
	if !strings.Contains(logs.String(), `"dropped_diagnostics":3`) || !strings.Contains(logs.String(), `"outcome":"duplicate_redelivery"`) {
		t.Fatalf("missing default-level anomaly/overflow diagnostics: %s", logs.String())
	}
}

func BenchmarkDaemonEmit_OverlappingSubscription(b *testing.B) {
	hub := NewHub(100)
	if _, err := hub.Register(transport.Hello{SubscribeID: "current"}); err != nil {
		b.Fatal(err)
	}
	d := daemon{hub: hub, dedup: dedup.New(), audit: newDeliveryAudit(slog.New(slog.NewTextHandler(io.Discard, nil)))}
	raw := &dwsevent.RawEvent{EventID: "same", SubscribeID: "old", EventType: "user_im_message_receive_o2o"}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		d.emit(raw)
	}
}
