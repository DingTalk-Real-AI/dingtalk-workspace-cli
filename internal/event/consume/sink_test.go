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
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
)

func TestStdoutSink_WritesVerbatim(t *testing.T) {
	var buf bytes.Buffer
	s := NewStdoutSink(&buf)
	if err := s.Write(transport.Event{}, []byte("line one\n")); err != nil {
		t.Fatal(err)
	}
	if err := s.Write(transport.Event{}, []byte("line two\n")); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "line one\nline two\n" {
		t.Fatalf("got: %q", buf.String())
	}
}

func TestFileDirSink_WritesPerEvent(t *testing.T) {
	dir := t.TempDir()
	s := NewFileDirSink(dir)
	ev := transport.Event{
		EventType:        "im.message.receive_v1",
		EventID:          "ev_abc",
		ReceivedAtUnixMS: 1700000000123,
	}
	body := []byte(`{"hello":"world"}`)
	if err := s.Write(ev, body); err != nil {
		t.Fatalf("Write: %v", err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("expected 1 file, got %d", len(entries))
	}
	got, _ := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if !bytes.Equal(got, body) {
		t.Fatalf("body mismatch: %q", got)
	}
	if entries[0].Name() != "im.message.receive_v1_ev_abc_1700000000123.json" {
		t.Errorf("filename = %q", entries[0].Name())
	}
}

func TestFileDirSink_MkdirAutomatic(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "events")
	s := NewFileDirSink(dir)
	err := s.Write(transport.Event{EventType: "x", EventID: "1", ReceivedAtUnixMS: 1}, []byte("ok"))
	if err != nil {
		t.Fatalf("Write should auto-mkdir: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("dir not created: %v", err)
	}
}

func TestFileDirSink_AtomicWriteNoTmpLeft(t *testing.T) {
	dir := t.TempDir()
	s := NewFileDirSink(dir)
	_ = s.Write(transport.Event{EventType: "x", EventID: "1", ReceivedAtUnixMS: 1}, []byte("ok"))
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("tmp file leaked: %s", e.Name())
		}
	}
}

func TestSafePart_StripsPathSeparators(t *testing.T) {
	cases := []struct{ in, want string }{
		{"normal", "normal"},
		{"im.message.receive_v1", "im.message.receive_v1"}, // dots OK
		{"a/b", "a_b"},
		{"a\\b", "a_b"},
		{"a:b", "a_b"},
		{"../../etc/passwd", ".._.._etc_passwd"}, // slashes → _; dots preserved (safe — no separator anchor)
		{"  spaces  ", "spaces"},
		{"", ""},
	}
	for _, c := range cases {
		if got := safePart(c.in); got != c.want {
			t.Errorf("safePart(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestBuildFilename_DefaultsForMissingFields(t *testing.T) {
	got := buildFilename(transport.Event{})
	// Should contain unknown_no-id_<ts>.json
	if !strings.HasPrefix(got, "unknown_no-id_") || !strings.HasSuffix(got, ".json") {
		t.Fatalf("default filename shape unexpected: %q", got)
	}
}

func TestRoutedSink_MatchedGoesToDir(t *testing.T) {
	tmp := t.TempDir()
	imDir := filepath.Join(tmp, "im")
	var fallback bytes.Buffer
	routes, _ := ParseRoutes([]string{`^im\.=dir:` + imDir})
	rs := NewRoutedSink(NewRouter(routes), NewStdoutSink(&fallback))

	// IM event → file in imDir
	if err := rs.Write(transport.Event{EventType: "im.message.receive_v1", EventID: "x", ReceivedAtUnixMS: 1}, []byte("body")); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(imDir); len(entries) != 1 {
		t.Errorf("expected 1 file in %s, got %d", imDir, len(entries))
	}
	if fallback.Len() != 0 {
		t.Errorf("fallback should be empty for matched route")
	}

	// Non-IM event → fallback (stdout)
	_ = rs.Write(transport.Event{EventType: "approval.task", EventID: "y", ReceivedAtUnixMS: 2}, []byte("body2\n"))
	if fallback.String() != "body2\n" {
		t.Errorf("fallback got: %q", fallback.String())
	}
}
