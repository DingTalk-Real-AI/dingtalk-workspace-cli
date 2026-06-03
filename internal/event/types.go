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

package event

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// RawEvent is one event delivered by the Source layer to the Hub. It mirrors
// the DingTalk Stream SDK's EventHeader (5 fields) plus the raw JSON payload
// from DataFrame.Data, plus the local receive timestamp used for ordering and
// dedup fallback keys.
//
// Field naming follows Go conventions (camelCase capitalised); JSON tags
// follow the SDK's wire format so that NDJSON output is consistent with what
// DingTalk documents on its open platform.
type RawEvent struct {
	EventID           string            `json:"event_id"`             // DataFrameHeader "eventId"
	EventBornTime     int64             `json:"event_born_time"`      // milliseconds; verified P0
	EventCorpID       string            `json:"event_corp_id"`        // tenant corp id
	EventType         string            `json:"event_type"`           // catch-all/filter routing key
	EventUnifiedAppID string            `json:"event_unified_app_id"` // app id
	Data              string            `json:"data"`                 // raw JSON payload from SDK
	Headers           map[string]string `json:"headers,omitempty"`    // full DataFrame.Headers, passthrough
	ReceivedAt        time.Time         `json:"received_at"`          // bus receive time (UTC)
}

// EmitFn is the non-blocking handoff from Source to Hub. Implementations MUST
// return immediately (drop-oldest on the consumer's sendCh) so the SDK's
// callback can ACK without delay — see plan invariant #1.
type EmitFn func(*RawEvent)

// DedupKey returns the LRU dedup key for this event. Prefers EventID (the
// SDK-provided identifier); falls back to a content-derived hash when the
// SDK does not populate EventID (rare, but observed on legacy event types
// per the P0 escape-hatch verification).
func (e *RawEvent) DedupKey() string {
	if e == nil {
		return ""
	}
	if e.EventID != "" {
		return e.EventID
	}
	// Fallback: type + born_time + sha256(data)[:16]
	h := sha256.Sum256([]byte(e.Data))
	return e.EventType + ":" + itoa(e.EventBornTime) + ":" + hex.EncodeToString(h[:8])
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// ClientIDHash returns the path-safe identifier derived from the (possibly
// unsafe) ClientID string. We use sha256(clientID)[:16] hex-encoded; this is
// the value placed in filesystem paths and Named Pipe names. The original
// ClientID is stored in bus.meta and shown in status output. See plan
// "路径安全" decision in §1.
func ClientIDHash(clientID string) string {
	if clientID == "" {
		return ""
	}
	h := sha256.Sum256([]byte(clientID))
	return hex.EncodeToString(h[:16])
}

// RedactSecret returns a redacted form of a secret string for logging and
// error messages: first 3 + "***" + last 3 characters. Short secrets
// (<= 6 chars) are fully masked. Empty input yields empty output.
func RedactSecret(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 6 {
		return "***"
	}
	return s[:3] + "***" + s[len(s)-3:]
}
