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
	"context"
	"log/slog"
	"sync/atomic"

	dwsevent "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event"
)

// emit is the actual SDK handoff. Dedup and fan-out are delivery-scoped;
// diagnostics use a bounded queue so disk/logging I/O cannot delay cloud ACKs.
func (d *daemon) emit(raw *dwsevent.RawEvent) {
	if raw == nil {
		return
	}
	if d.dedup.Seen(raw.DedupKey()) {
		d.audit.record(raw, "duplicate_redelivery", 0, 0)
		return
	}
	consumers, matched := d.hub.deliver(raw)
	outcome := "routed"
	if matched == 0 {
		outcome = "no_matching_consumer"
	}
	d.audit.record(raw, outcome, consumers, matched)
}

// Only routing metadata is retained: never Data, Headers, tokens or a RawEvent
// pointer. A full queue drops diagnostics (not business events) and reports it.
type deliveryRecord struct {
	eventID, eventType, subscribeID, sourceID, outcome string
	consumers, matched                                 int
}

type deliveryAudit struct {
	log     *slog.Logger
	records chan deliveryRecord
	dropped atomic.Uint64
}

func newDeliveryAudit(log *slog.Logger) *deliveryAudit {
	return &deliveryAudit{log: log, records: make(chan deliveryRecord, 256)}
}

func (a *deliveryAudit) record(raw *dwsevent.RawEvent, outcome string, consumers, matched int) {
	if a == nil || (outcome == "routed" && !a.log.Enabled(context.Background(), slog.LevelDebug)) {
		return
	}
	record := deliveryRecord{raw.EventID, raw.EventType, raw.SubscribeID, raw.SourceID, outcome, consumers, matched}
	select {
	case a.records <- record:
	default:
		a.dropped.Add(1)
	}
}

func (a *deliveryAudit) reportDropped() {
	if dropped := a.dropped.Swap(0); dropped > 0 {
		a.log.Warn("bus: delivery diagnostic queue full", "dropped_diagnostics", dropped)
	}
}

func (a *deliveryAudit) write(record deliveryRecord) {
	a.reportDropped()
	level := slog.LevelInfo
	if record.outcome == "routed" {
		level = slog.LevelDebug
	}
	a.log.Log(context.Background(), level, "bus: event delivery",
		"event_id", record.eventID, "event_type", record.eventType,
		"subscribe_id", record.subscribeID, "source_id", record.sourceID,
		"outcome", record.outcome, "consumers", record.consumers, "matched_consumers", record.matched)
}

func (a *deliveryAudit) run(ctx context.Context) {
	for {
		select {
		case record := <-a.records:
			a.write(record)
		case <-ctx.Done():
			// Drain at most one queue's worth on shutdown; never wait for a
			// source that continues calling emit after cancellation.
			for range cap(a.records) {
				select {
				case record := <-a.records:
					a.write(record)
				default:
					a.reportDropped()
					return
				}
			}
			a.reportDropped()
			return
		}
	}
}
