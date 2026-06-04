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

// Package telemetry emits anonymous, dimensions-only operational metrics for
// each dws command invocation — error rate, latency, command distribution and
// version/platform health. It is the ops-monitoring counterpart to the audit
// package, but deliberately MUCH smaller:
//
//   - It carries ONLY coarse dimensions: never object names, free text, peer
//     ids, device fingerprints, or the user's natural-language intent. There is
//     nothing to redact because nothing sensitive is ever collected.
//   - It is independent of the audit package and its DWS_AUDIT_* switches, so
//     an operator can run ops telemetry without enabling compliance auditing.
//   - It is OFF by default. The CLI emits nothing unless the operator opts in
//     via DWS_TELEMETRY_ENABLED, and the destination is operator-configured.
//
// What it deliberately does NOT collect (so reviewers can verify the privacy
// boundary by inspecting this struct alone): actor identity (user id/name),
// target object names/ids, free-text intent, device id / serial number, and
// request/response bodies.
package telemetry

import "time"

// SchemaVersion is bumped on any breaking change to Event's JSON shape.
const SchemaVersion = "1"

// Event is the full operational record for one dws command. Every field is a
// low-cardinality (or trace) dimension safe to ship to a central ops sink.
type Event struct {
	SchemaVersion string    `json:"schema_version"`
	Timestamp     time.Time `json:"ts"`
	TraceID       string    `json:"trace_id"` // == transport execution_id, for joining with server-side logs

	CorpID     string `json:"corp_id,omitempty"`     // tenant dimension, for per-org health; best-effort
	CLIVersion string `json:"cli_version,omitempty"` // "did this release break a command at scale"
	Channel    string `json:"channel,omitempty"`     // which integration/agent drove dws (DWS_CHANNEL)
	OS         string `json:"os,omitempty"`          // runtime.GOOS — coarse platform, not PII

	Module     string `json:"module"`     // operated product, e.g. "doc"
	Command    string `json:"command"`    // skill command
	Subcommand string `json:"subcommand"` // skill subcommand, e.g. "create"

	Outcome    string `json:"outcome"`            // "ok" | "error"
	ErrClass   string `json:"err_class,omitempty"` // error category when outcome=error
	ExitCode   int    `json:"exit_code"`
	DurationMS int64  `json:"duration_ms"` // wall-clock latency of the invocation
}

// New stamps the schema version, timestamp and trace id. The caller supplies
// the wall clock so callers stay testable and deterministic.
func New(ts time.Time, traceID string) *Event {
	return &Event{
		SchemaVersion: SchemaVersion,
		Timestamp:     ts,
		TraceID:       traceID,
	}
}
