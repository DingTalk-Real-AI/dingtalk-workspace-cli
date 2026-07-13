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
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestWriterReader_Roundtrip(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	if err := w.WriteJSON(Hello{Type: FrameTypeHello, ConsumerPID: 42, EventTypes: []string{"im.*"}}); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	if err := w.WriteJSON(Heartbeat{Type: FrameTypeHeartbeat}); err != nil {
		t.Fatalf("WriteJSON 2: %v", err)
	}

	r := NewReader(&buf)
	var h Hello
	if err := r.ReadJSON(&h); err != nil {
		t.Fatalf("ReadJSON 1: %v", err)
	}
	if h.ConsumerPID != 42 || h.Type != FrameTypeHello || len(h.EventTypes) != 1 || h.EventTypes[0] != "im.*" {
		t.Fatalf("decoded Hello = %+v", h)
	}
	var hb Heartbeat
	if err := r.ReadJSON(&hb); err != nil {
		t.Fatalf("ReadJSON 2: %v", err)
	}
	if hb.Type != FrameTypeHeartbeat {
		t.Fatalf("hb type = %s", hb.Type)
	}
}

func TestReader_CleanEOFBetweenFrames(t *testing.T) {
	var buf bytes.Buffer
	NewWriter(&buf).WriteJSON(Heartbeat{Type: FrameTypeHeartbeat})

	r := NewReader(&buf)
	if _, err := r.Read(); err != nil {
		t.Fatalf("first read: %v", err)
	}
	_, err := r.Read()
	if !errors.Is(err, io.EOF) {
		t.Fatalf("second read after clean close = %v, want EOF", err)
	}
}

func TestReader_FrameTooLarge(t *testing.T) {
	// Synthesise a > MaxFrameBytes line with no \n
	big := strings.Repeat("A", MaxFrameBytes+10)
	r := NewReader(strings.NewReader(big + "\n"))
	_, err := r.Read()
	if !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("Read oversized = %v, want ErrFrameTooLarge", err)
	}
}

func TestWriter_FrameTooLarge(t *testing.T) {
	// Build a Hello with a filter string that would marshal larger than the cap.
	huge := strings.Repeat("x", MaxFrameBytes)
	w := NewWriter(io.Discard)
	err := w.WriteJSON(Hello{Type: FrameTypeHello, Filter: huge})
	if !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("WriteJSON oversized = %v, want ErrFrameTooLarge", err)
	}
}

func TestReader_MalformedJSONErrors(t *testing.T) {
	r := NewReader(strings.NewReader("{not json}\n"))
	var x Hello
	if err := r.ReadJSON(&x); err == nil {
		t.Fatal("expected decode error for malformed JSON")
	}
}

func TestPeekType(t *testing.T) {
	cases := []struct {
		raw  string
		want FrameType
		err  bool
	}{
		{`{"type":"hello"}`, FrameTypeHello, false},
		{`{"type":"event","seq":1}`, FrameTypeEvent, false},
		{`{"foo":"bar"}`, "", true},
		{`not json`, "", true},
	}
	for _, tc := range cases {
		got, err := PeekType([]byte(tc.raw))
		if (err != nil) != tc.err {
			t.Errorf("PeekType(%q) err = %v, wantErr=%v", tc.raw, err, tc.err)
			continue
		}
		if got != tc.want {
			t.Errorf("PeekType(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}
