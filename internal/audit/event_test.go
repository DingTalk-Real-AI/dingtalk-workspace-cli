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

package audit

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func sampleEvent() *Event {
	ts := time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC)
	e := New(ts, "trace-abc")
	e.Actor = Actor{UserID: "staff-001", Name: "Zhang San"}
	e.Org = Org{CorpID: "corp-001", Name: "Example Corp"}
	e.Device = Device{DeviceID: "dev-9", SerialNo: "C02SN12345", OS: "darwin"}
	e.Intent.NLInput = "export last week's minutes to the desktop"
	e.Module = "minutes"
	e.Command = "minutes"
	e.Subcommand = "export"
	e.SubcommandDesc = "export meeting minutes"
	e.Target = Target{Type: "minutes", ID: "m-77", Name: "Q2 Strategy Review", Summary: "revenue and headcount", Sensitivity: SensitivityConfidential}
	e.Flow = Flow{Direction: DirectionLocalExport, LocalPath: "/Users/x/Desktop/q2.md", API: "minutes.export"}
	e.Outcome = "ok"
	e.ExitCode = 0
	return e
}

// Intent provenance must never claim the CLI observed the NL — it can't.
func TestNL_IntentProvenanceIsAgent(t *testing.T) {
	e := New(time.Unix(0, 0).UTC(), "t")
	if e.Intent.Provenance != ProvenanceAgent {
		t.Fatalf("NL intent must be agent-provenanced, got %q", e.Intent.Provenance)
	}
}

// RedactNone is verbatim; the local-file tier keeps everything.
func TestRedactNone_Verbatim(t *testing.T) {
	e := sampleEvent()
	got := e.Redact(RedactNone, "salt")
	if got.Intent.NLInput != e.Intent.NLInput || got.Device.SerialNo != e.Device.SerialNo {
		t.Fatal("RedactNone must not alter content")
	}
}

// RedactHashed must strip raw content/PII but keep correlatable hashes.
func TestRedactHashed_StripsRawPII(t *testing.T) {
	e := sampleEvent()
	got := e.Redact(RedactHashed, "salt")
	if strings.Contains(got.Intent.NLInput, "minutes") {
		t.Errorf("hashed NL still contains raw text: %q", got.Intent.NLInput)
	}
	if got.Device.SerialNo == "C02SN12345" {
		t.Error("serial must be hashed, not raw")
	}
	if got.Target.Summary != "" {
		t.Error("object summary must be dropped at hashed tier")
	}
	if !strings.HasPrefix(got.Intent.NLInput, "h:") {
		t.Errorf("expected hash marker, got %q", got.Intent.NLInput)
	}
	// Receiver must be untouched (no mutation of the local full record).
	if e.Intent.NLInput == got.Intent.NLInput {
		t.Error("Redact mutated the original event")
	}
}

// RedactMinimal is the ops-monitoring tier: dimensions only, zero content.
func TestRedactMinimal_DimensionsOnly(t *testing.T) {
	e := sampleEvent()
	got := e.Redact(RedactMinimal, "salt")
	if got.Intent.NLInput != "" || got.Target.ID != "" || got.Device.SerialNo != "" || got.Actor.UserID != "" {
		t.Error("minimal tier leaked identifying/content fields")
	}
	if got.Command != "minutes" || got.Outcome != "ok" || got.Flow.Direction != DirectionLocalExport {
		t.Error("minimal tier must keep monitoring dimensions")
	}
}

// FileSink writes one JSONL line per event, round-trippable.
func TestFileSink_JSONL(t *testing.T) {
	var buf bytes.Buffer
	s := NewFileSink(&buf)
	if err := s.Emit(sampleEvent()); err != nil {
		t.Fatal(err)
	}
	if err := s.Emit(sampleEvent()); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 JSONL lines, got %d", len(lines))
	}
	var back Event
	if err := json.Unmarshal([]byte(lines[0]), &back); err != nil {
		t.Fatalf("line not valid JSON: %v", err)
	}
	if back.TraceID != "trace-abc" || back.SchemaVersion != SchemaVersion {
		t.Errorf("round-trip lost fields: %+v", back)
	}
}

// RedactingSink must hand the wrapped sink the redacted copy, not the raw one.
func TestRedactingSink_AppliesLevel(t *testing.T) {
	var buf bytes.Buffer
	s := &RedactingSink{Inner: NewFileSink(&buf), Level: RedactMinimal, Salt: "s"}
	if err := s.Emit(sampleEvent()); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "C02SN12345") || strings.Contains(buf.String(), "headcount") {
		t.Error("forwarder shipped raw PII/content despite RedactMinimal")
	}
}
