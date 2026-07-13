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

package consume

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
)

func sampleEvent() transport.Event {
	return transport.Event{
		Type:              transport.FrameTypeEvent,
		Seq:               42,
		EventID:           "ev_abc",
		EventBornTime:     1700000000123,
		EventType:         "im.message.receive_v1",
		EventCorpID:       "corp_x",
		EventUnifiedAppID: "app_y",
		Data:              `{"message":{"message_id":"om_x","chat_id":"oc_y","content":"hi"},"sender":{"sender_id":{"open_id":"ou_z"}}}`,
		ReceivedAtUnixMS:  1700000000999,
	}
}

func TestNormalizeFormat(t *testing.T) {
	cases := []struct {
		in       string
		want     Format
		fellback bool
	}{
		{"", FormatNDJSON, false},
		{"ndjson", FormatNDJSON, false},
		{"json", FormatJSON, false},
		{"pretty", FormatPretty, false},
		{"raw", FormatRaw, false},
		{"compact", FormatCompact, false},
		{"table", FormatNDJSON, true},
		{"csv", FormatNDJSON, true},
		{"yaml", FormatNDJSON, true}, // typo / unsupported → fallback
	}
	for _, c := range cases {
		got, fb := NormalizeFormat(c.in)
		if got != c.want || fb != c.fellback {
			t.Errorf("NormalizeFormat(%q) = (%s, %v), want (%s, %v)", c.in, got, fb, c.want, c.fellback)
		}
	}
}

func TestNDJSONFormatter_OneLinePerEvent(t *testing.T) {
	f, _ := NewFormatter(FormatNDJSON)
	out, err := f.Render(sampleEvent())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(out), "\n") {
		t.Fatal("ndjson output must end with \\n")
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("ndjson must be exactly one line, got %d: %s", len(lines), out)
	}
	// Must be valid JSON
	var ev transport.Event
	if err := json.Unmarshal([]byte(lines[0]), &ev); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	if ev.EventID != "ev_abc" {
		t.Errorf("round-trip lost EventID: %q", ev.EventID)
	}
}

func TestPrettyFormatter_MultilineIndented(t *testing.T) {
	f, _ := NewFormatter(FormatPretty)
	out, err := f.Render(sampleEvent())
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "\n  ") {
		t.Fatal("pretty output should have 2-space indentation")
	}
	if !strings.HasSuffix(s, "\n") {
		t.Fatal("pretty output should end with \\n")
	}
	// Strip trailing newline and ensure round-trip works (still valid JSON).
	var ev transport.Event
	if err := json.Unmarshal([]byte(strings.TrimRight(s, "\n")), &ev); err != nil {
		t.Fatalf("pretty not valid JSON: %v", err)
	}
}

func TestRawFormatter_WritesDataVerbatim(t *testing.T) {
	f, _ := NewFormatter(FormatRaw)
	out, err := f.Render(transport.Event{Data: `{"foo":"bar"}`})
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "{\"foo\":\"bar\"}\n" {
		t.Errorf("raw output = %q", out)
	}
}

func TestRawFormatter_PreservesExistingTrailingNewline(t *testing.T) {
	f, _ := NewFormatter(FormatRaw)
	out, _ := f.Render(transport.Event{Data: "already-ends-newline\n"})
	if string(out) != "already-ends-newline\n" {
		t.Errorf("raw should not double the trailing \\n, got %q", out)
	}
}

func TestRawFormatter_EmptyDataYieldsBareNewline(t *testing.T) {
	f, _ := NewFormatter(FormatRaw)
	out, _ := f.Render(transport.Event{})
	if string(out) != "\n" {
		t.Errorf("empty Data → %q, want bare \\n", out)
	}
}

func TestCompactFormatter_DispatchesPerEventType(t *testing.T) {
	f, _ := NewFormatter(FormatCompact)
	out, err := f.Render(sampleEvent())
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(strings.TrimRight(string(out), "\n")), &got); err != nil {
		t.Fatalf("compact not valid JSON: %v", err)
	}
	// IM message processor should have lifted message_id/chat_id/etc.
	if got["message_id"] != "om_x" || got["chat_id"] != "oc_y" {
		t.Fatalf("compact output missing lifted fields: %+v", got)
	}
	// Header field `type` must equal event_type.
	if got["type"] != "im.message.receive_v1" {
		t.Errorf("type = %v", got["type"])
	}
}

func TestNewFormatter_RejectsUnknown(t *testing.T) {
	if _, err := NewFormatter(Format("nope")); err == nil {
		t.Fatal("expected error for unknown format")
	}
}
