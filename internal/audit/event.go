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

// Package audit defines the structured audit event emitted once per dws
// command invocation, plus pluggable sinks that decide WHERE it goes.
//
// Design principles (open-source norms):
//   - One versioned schema, two emission channels: a local file the operator
//     owns (transparent, always inspectable) and an OPTIONAL forwarder to a
//     sink the *deploying organization* configures — never hardcoded to the
//     vendor.
//   - Field minimization by tier: Redact() produces the remote-safe view so a
//     forwarder can ship hashed/minimal data even when the local file keeps the
//     full record.
//   - Honest provenance: fields the CLI binary cannot observe (e.g. the user's
//     natural-language intent, which only the orchestrating agent sees) are
//     marked Provenance != ProvenanceCLI so consumers know they were injected
//     from an outer layer rather than measured.
package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// SchemaVersion is bumped on any breaking change to Event's JSON shape.
// v2 adds the Client block (agent_id / host_agent / channel / cli_version).
const SchemaVersion = "2"

// Direction enumerates where data flowed as a result of the command.
type Direction string

const (
	// DirectionLocalExport: data left the tenant onto the local disk.
	DirectionLocalExport Direction = "local-export"
	// DirectionExternalAPI: data crossed to an endpoint outside the tenant.
	DirectionExternalAPI Direction = "external-api"
	// DirectionIntraTenant: data moved between objects inside DingTalk
	// (person id / group id / doc id), no egress.
	DirectionIntraTenant Direction = "intra-tenant"
	// DirectionRead: read-only, no data movement.
	DirectionRead Direction = "read"
)

// Provenance records which layer produced a field — the audit's honesty knob.
type Provenance string

const (
	// ProvenanceCLI: observed by the dws binary itself (trustworthy).
	ProvenanceCLI Provenance = "cli"
	// ProvenanceAgent: injected by the orchestrating agent/skill layer via
	// env (e.g. DWS_AUDIT_NL_INTENT). The binary cannot verify it.
	ProvenanceAgent Provenance = "agent"
)

// Sensitivity is a coarse classification of the operated object, used to
// decide whether the record itself needs stricter handling downstream.
type Sensitivity string

const (
	SensitivityUnknown      Sensitivity = "unknown"
	SensitivityPublic       Sensitivity = "public"
	SensitivityInternal     Sensitivity = "internal"
	SensitivityConfidential Sensitivity = "confidential"
)

// Actor identifies the human behind the invocation.
type Actor struct {
	UserID string `json:"user_id,omitempty"` // DingTalk open_id / staff id
	Name   string `json:"name,omitempty"`
}

// Org identifies the tenant.
type Org struct {
	CorpID string `json:"corp_id,omitempty"`
	Name   string `json:"name,omitempty"`
}

// Client identifies the dws install, version, and the integration channel.
//
// Trust tiers:
//   - AgentID/Source/CLIVersion: dws-managed install state / compiled-in —
//     not caller-asserted-per-call.
//   - Channel (DWS_CHANNEL): SEMI-trusted. The gateway validates channel
//     membership against allowedChannels (an unregistered channel is rejected,
//     see auth.classifyDenialReason), so it can't be an arbitrary value — but
//     it is NOT yet cryptographically bound, so one registered channel could
//     still impersonate another. Recorded for "which agent/channel called",
//     flagged as semi-trusted until the gateway signs it (see audit TODO).
//
// Deliberately ABSENT (fully forgeable, plain env labels — see audit TODO):
// host_agent (DINGTALK_AGENT), agent_code (DINGTALK_DWS_AGENTCODE). Added only
// once the gateway hands back a SIGNED agent identity.
type Client struct {
	AgentID    string `json:"agent_id,omitempty"`    // install identity: install-time UUID (x-dws-agent-id)
	Channel    string `json:"channel,omitempty"`     // channel / which agent: DWS_CHANNEL (gateway validates membership, semi-trusted)
	Source     string `json:"source,omitempty"`      // identity source, defaults to "dws"
	CLIVersion string `json:"cli_version,omitempty"` // dws version
}

// Device identifies the machine. DeviceID/SerialNo are NEW collection and
// count as personal information under PIPL — both are empty unless the operator
// explicitly opts in (see collector).
type Device struct {
	DeviceID string `json:"device_id,omitempty"`
	SerialNo string `json:"sn_no,omitempty"` // hardware serial; sensitive PII
	OS       string `json:"os,omitempty"`
	Hostname string `json:"hostname,omitempty"`
}

// Intent is the user's natural-language request. Provenance is ALWAYS
// ProvenanceAgent because the dws binary never sees NL — only structured argv.
type Intent struct {
	NLInput    string     `json:"nl_input,omitempty"`
	Provenance Provenance `json:"provenance"`
}

// Target is the object the command acted on.
type Target struct {
	Type        string      `json:"type,omitempty"` // group / doc / minutes / table ...
	ID          string      `json:"id,omitempty"`
	Name        string      `json:"name,omitempty"`
	Summary     string      `json:"summary,omitempty"` // short digest for sensitivity review
	Sensitivity Sensitivity `json:"sensitivity,omitempty"`
}

// Flow records the data-movement footprint of the command.
type Flow struct {
	Direction Direction `json:"direction"`
	LocalPath string    `json:"local_path,omitempty"` // for local-export
	Endpoint  string    `json:"endpoint,omitempty"`   // for external-api
	API       string    `json:"api,omitempty"`        // MCP tool / REST path invoked
	PeerIDs   []string  `json:"peer_ids,omitempty"`   // for intra-tenant (person/group/doc ids)
}

// Event is the full audit record for one dws command. The local file keeps it
// verbatim; Redact() derives the remote-safe view for forwarders.
type Event struct {
	SchemaVersion string    `json:"schema_version"`
	Timestamp     time.Time `json:"ts"`
	TraceID       string    `json:"trace_id"` // == transport execution_id / x-dingtalk-trace-id

	Actor  Actor  `json:"actor"`
	Org    Org    `json:"org"`
	Client Client `json:"client"`
	Device Device `json:"device"`
	Intent Intent `json:"intent"`

	Module         string `json:"module"`          // operated module: doc / group / minutes / table
	Command        string `json:"command"`         // skill command, e.g. "doc"
	Subcommand     string `json:"subcommand"`      // skill subcommand, e.g. "create"
	SubcommandDesc string `json:"subcommand_desc"` // static, from command catalog

	Target Target `json:"target"`
	Flow   Flow   `json:"flow"`

	Outcome  string `json:"outcome"` // ok / error
	ErrClass string `json:"err_class,omitempty"`
	ExitCode int    `json:"exit_code"`
}

// New stamps schema version and intent provenance. The caller supplies the
// timestamp and trace id (the package never reads the wall clock itself, to
// stay deterministic and testable).
func New(ts time.Time, traceID string) *Event {
	return &Event{
		SchemaVersion: SchemaVersion,
		Timestamp:     ts,
		TraceID:       traceID,
		Intent:        Intent{Provenance: ProvenanceAgent},
	}
}

// RedactLevel controls how much a forwarder ships.
type RedactLevel int

const (
	// RedactNone: ship verbatim. Only legitimate when the sink is inside the
	// enterprise's own trust boundary (e.g. its internal audit store).
	RedactNone RedactLevel = iota
	// RedactHashed: replace free-text/PII with stable salted hashes so the
	// sink can still correlate without holding raw content.
	RedactHashed
	// RedactMinimal: drop all content; keep only counters/dimensions needed
	// for ops monitoring ("did this release break a command at scale").
	RedactMinimal
)

// Redact returns a copy adjusted for the given level. The receiver is unchanged
// so the local full record is never mutated.
func (e *Event) Redact(level RedactLevel, salt string) *Event {
	cp := *e
	switch level {
	case RedactNone:
		return &cp
	case RedactHashed:
		cp.Intent.NLInput = hashed(cp.Intent.NLInput, salt)
		cp.Actor.Name = ""
		cp.Target.Name = hashed(cp.Target.Name, salt)
		cp.Target.Summary = ""
		cp.Device.SerialNo = hashed(cp.Device.SerialNo, salt)
		cp.Client.AgentID = hashed(cp.Client.AgentID, salt)
		cp.Flow.PeerIDs = hashEach(cp.Flow.PeerIDs, salt)
		return &cp
	case RedactMinimal:
		// Keep only the dimensions an ops dashboard needs; drop every
		// content-bearing or identifying field.
		return &Event{
			SchemaVersion: cp.SchemaVersion,
			Timestamp:     cp.Timestamp,
			TraceID:       cp.TraceID,
			Org:           Org{CorpID: cp.Org.CorpID},
			Client:        Client{CLIVersion: cp.Client.CLIVersion, Channel: cp.Client.Channel}, // version + channel are ops dimensions; drop the install id
			Module:        cp.Module,
			Command:       cp.Command,
			Subcommand:    cp.Subcommand,
			Flow:          Flow{Direction: cp.Flow.Direction, API: cp.Flow.API},
			Outcome:       cp.Outcome,
			ErrClass:      cp.ErrClass,
			ExitCode:      cp.ExitCode,
			Intent:        Intent{Provenance: cp.Intent.Provenance},
		}
	default:
		return &cp
	}
}

func hashed(s, salt string) string {
	if s == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(salt + ":" + s))
	return "h:" + hex.EncodeToString(sum[:8])
}

func hashEach(in []string, salt string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = hashed(s, salt)
	}
	return out
}
