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

package transport

import (
	"encoding/json"
	"fmt"

	dwsevent "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event"
)

// FrameType discriminates the JSON frames flowing over the IPC channel.
// Wire values are stable strings — bumping any of these is a protocol
// breaking change requiring a coordinated bus + consume rollout.
type FrameType string

const (
	FrameTypeHello       FrameType = "hello"        // consume → bus
	FrameTypeHelloAck    FrameType = "hello_ack"    // bus → consume
	FrameTypeEvent       FrameType = "event"        // bus → consume
	FrameTypeHeartbeat   FrameType = "heartbeat"    // bidirectional
	FrameTypeSourceState FrameType = "source_state" // bus → consume
	FrameTypeBye         FrameType = "bye"          // bidirectional
	FrameTypeStatusReq   FrameType = "status_req"   // consume/ad-hoc → bus
	FrameTypeStatusResp  FrameType = "status_resp"  // bus → consume/ad-hoc
)

// Hello is the first frame a consumer sends after dialing the bus. The bus
// uses event_types + filter to do server-side pushdown (plan §1 decision:
// only events matching this filter are written to this consumer's sendCh).
type Hello struct {
	Type        FrameType `json:"type"`
	ConsumerPID int       `json:"consumer_pid"`
	EventTypes  []string  `json:"event_types,omitempty"`  // wildcard list ("im.*", "approval.*"); empty = catch-all
	Filter      string    `json:"filter,omitempty"`       // optional regex over event_type
	SubscribeID string    `json:"subscribe_id,omitempty"` // optional personal subscription isolation key; empty = no subscribe_id filter
	Compact     bool      `json:"compact,omitempty"`      // hint to status output; bus does not transform payloads
	// Role distinguishes a real consumer (registered for events) from an
	// ad-hoc tooling connection (status/list/stop). Ad-hoc connections do
	// NOT register with the Hub.
	Role HelloRole `json:"role,omitempty"`
}

// HelloRole tags the purpose of a Hello connection.
type HelloRole string

const (
	HelloRoleConsumer HelloRole = ""       // default
	HelloRoleStatus   HelloRole = "status" // event list / event status
	HelloRoleStop     HelloRole = "stop"   // event stop (graceful trigger)
)

// HelloAck is the bus's reply on accepted Hello. SourceState/StateSource
// mirror the source.Machine snapshot at connect time; CredentialsSource
// fields tell users which channel actually supplied the credentials in use
// (env / app_config / keychain / plain_config), see plan "凭证来源拆字段".
type HelloAck struct {
	Type               FrameType `json:"type"`
	BusPID             int       `json:"bus_pid"`
	SourceState        string    `json:"source_state"`                // mirrors source.State string value
	StateSource        string    `json:"state_source"`                // "hook" | "inferred"
	ClientIDSource     string    `json:"client_id_source"`            // auth.CredentialSource string
	ClientSecretSource string    `json:"client_secret_source"`        // auth.CredentialSource string
	IdleTimeoutSecs    int       `json:"idle_timeout_secs,omitempty"` // bus's IdleTimeout for diagnostics
}

// Event wraps one delivered RawEvent for the wire. We keep the payload as
// a string (not nested JSON) so the bus does not need to parse / re-encode.
type Event struct {
	Type              FrameType         `json:"type"`
	Seq               uint64            `json:"seq"` // per-consumer monotonic, restarts at 1 on reconnect
	EventID           string            `json:"event_id"`
	EventBornTime     int64             `json:"event_born_time"`
	EventCorpID       string            `json:"event_corp_id,omitempty"`
	EventType         string            `json:"event_type"`
	EventUnifiedAppID string            `json:"event_unified_app_id,omitempty"`
	EventScope        string            `json:"event_scope,omitempty"`
	SubscribeID       string            `json:"subscribe_id,omitempty"`
	SourceID          string            `json:"source_id,omitempty"`
	RuleType          string            `json:"rule_type,omitempty"`
	Data              string            `json:"data"`
	Headers           map[string]string `json:"headers,omitempty"`
	ReceivedAtUnixMS  int64             `json:"received_at_unix_ms"`
}

// Heartbeat is bidirectional and stateless. It exists only to give both
// endpoints a chance to notice a dead peer via Read failure.
type Heartbeat struct {
	Type FrameType `json:"type"`
}

// SourceState is pushed bus → consume whenever the connection state machine
// transitions to / from connected. Consumers may render it; v1 they just
// forward to stderr when not --quiet.
type SourceState struct {
	Type        FrameType `json:"type"`
	State       string    `json:"state"`        // source.State string
	StateSource string    `json:"state_source"` // hook | inferred
	Attempt     int       `json:"attempt,omitempty"`
}

// Bye is sent by either side at graceful shutdown. Reason is free-form
// for logs; structured shutdown causes are encoded in the value:
//
//	"client_done"   — consume reached --max-events/--duration or SIGINT
//	"shutdown"      — bus SIGTERM/SIGINT
//	"idle_timeout"  — bus IdleTimeout fired with no consumers
//	"stop_request"  — bus received explicit Stop RPC
type Bye struct {
	Type   FrameType `json:"type"`
	Reason string    `json:"reason"`
}

// StatusReq is an empty JSON frame ad-hoc tooling sends after Hello to
// request a full StatusResp. Bus replies with one StatusResp then closes
// the connection.
type StatusReq struct {
	Type FrameType `json:"type"`
}

// StatusResp is the bus's snapshot view for `dws event status` rendering.
// Counts are accumulated since bus start; per-consumer entries are sorted
// by PID for deterministic output.
type StatusResp struct {
	Type                 FrameType           `json:"type"`
	Bus                  StatusBus           `json:"bus"`
	SourceState          StatusSource        `json:"source_state"`
	Consumers            []StatusConsumer    `json:"consumers"`
	PerEventTypeCounters map[string]Counters `json:"per_event_type"`
}

// StatusBus is the bus daemon's identity / lifecycle view.
type StatusBus struct {
	PID            int                 `json:"pid"`
	UptimeSecs     int64               `json:"uptime_secs"`
	IdleTimeoutSec int                 `json:"idle_timeout_secs"`
	ClientID       string              `json:"client_id"`
	Edition        string              `json:"edition"`
	SourceKind     dwsevent.SourceKind `json:"source_kind,omitempty"`
	IdentityHash   string              `json:"identity_hash,omitempty"`
	SourceID       string              `json:"source_id,omitempty"`
}

// StatusSource is the source.Machine snapshot at status RPC time.
type StatusSource struct {
	State           string `json:"state"`
	Source          string `json:"source"`
	LastEventAtMS   int64  `json:"last_event_at_ms,omitempty"`
	LastReconnectMS int64  `json:"last_reconnect_at_ms,omitempty"`
	ReconnectCount  int    `json:"reconnect_count"`
}

// StatusConsumer is one consumer's per-IPC-connection view.
type StatusConsumer struct {
	PID            int      `json:"pid"`
	EventTypes     []string `json:"event_types,omitempty"`
	Filter         string   `json:"filter,omitempty"`
	SubscribeID    string   `json:"subscribe_id,omitempty"`
	SubscribedAtMS int64    `json:"subscribed_at_ms"`
	Received       uint64   `json:"received"`
	Dropped        uint64   `json:"dropped"`
}

// Counters is the shared "received / dropped" pair used by per-consumer
// and per-event-type rollups.
type Counters struct {
	Received uint64 `json:"received"`
	Dropped  uint64 `json:"dropped"`
}

// typeOnly is the minimal envelope used by DecodeFrame to peek at the
// "type" field before deciding which concrete struct to unmarshal into.
type typeOnly struct {
	Type FrameType `json:"type"`
}

// PeekType returns the FrameType of a raw frame without fully decoding it.
// Returns an error when the JSON is malformed or the type field is missing.
func PeekType(raw []byte) (FrameType, error) {
	var t typeOnly
	if err := json.Unmarshal(raw, &t); err != nil {
		return "", fmt.Errorf("transport: peek frame type: %w", err)
	}
	if t.Type == "" {
		return "", fmt.Errorf("transport: frame missing 'type' field")
	}
	return t.Type, nil
}
