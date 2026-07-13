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
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestRawEvent_DedupKey_UsesEventID(t *testing.T) {
	e := &RawEvent{EventID: "ev_abc"}
	if got := e.DedupKey(); got != "ev_abc" {
		t.Fatalf("DedupKey = %q, want %q", got, "ev_abc")
	}
}

func TestRawEvent_DedupKey_FallbackHashWhenIDEmpty(t *testing.T) {
	e := &RawEvent{
		EventType:     "im.message.receive_v1",
		EventBornTime: 1700000000000,
		Data:          `{"chat":"x"}`,
	}
	got := e.DedupKey()
	if got == "" {
		t.Fatal("fallback DedupKey should not be empty")
	}
	if !strings.HasPrefix(got, "im.message.receive_v1:1700000000000:") {
		t.Fatalf("fallback DedupKey = %q, want prefix type:born_time:", got)
	}
	// Same content → same key
	if got2 := e.DedupKey(); got2 != got {
		t.Fatalf("DedupKey not deterministic: %q vs %q", got, got2)
	}
	// Different Data → different key
	e2 := *e
	e2.Data = `{"chat":"y"}`
	if got3 := e2.DedupKey(); got3 == got {
		t.Fatalf("different Data should yield different key, got same: %q", got)
	}
}

func TestRawEvent_DedupKey_NilSafe(t *testing.T) {
	var e *RawEvent
	if e.DedupKey() != "" {
		t.Fatal("nil RawEvent DedupKey should be empty")
	}
}

func TestClientIDHash_StableAndPathSafe(t *testing.T) {
	pathSafe := regexp.MustCompile(`^[0-9a-f]{16}$`)
	cases := []string{
		"ding_abc123",
		"ding_with_special!@#",
		"../../escape",
		"with/slash",
		"with whitespace",
		"unicode-工号-123",
	}
	for _, in := range cases {
		got := ClientIDHash(in)
		if !pathSafe.MatchString(got) {
			t.Fatalf("ClientIDHash(%q) = %q, not path-safe hex8(16 chars)", in, got)
		}
		// Stable
		if got2 := ClientIDHash(in); got2 != got {
			t.Fatalf("ClientIDHash not deterministic for %q: %q vs %q", in, got, got2)
		}
		// Path traversal proof: the hash, used as a path segment, must not
		// contain any "..", "/", or "\".
		if strings.Contains(got, "..") || strings.ContainsAny(got, `/\`) {
			t.Fatalf("ClientIDHash(%q) = %q contains path-unsafe chars", in, got)
		}
		// And actually joining doesn't escape parent.
		joined := filepath.Join("/tmp/events", got)
		if !strings.HasPrefix(joined, "/tmp/events/") {
			t.Fatalf("path escape: %q", joined)
		}
	}
}

func TestClientIDHash_DifferentInputsDifferentHashes(t *testing.T) {
	a := ClientIDHash("ding_a")
	b := ClientIDHash("ding_b")
	if a == b {
		t.Fatal("different ClientIDs must produce different hashes")
	}
}

func TestClientIDHash_EmptyReturnsEmpty(t *testing.T) {
	if got := ClientIDHash(""); got != "" {
		t.Fatalf("ClientIDHash(\"\") = %q, want empty", got)
	}
}

func TestRedactSecret(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"abc", "***"},
		{"abcdef", "***"},
		{"abcdefg", "abc***efg"},
		{"this-is-a-very-long-secret-string", "thi***ing"},
	}
	for _, tc := range cases {
		if got := RedactSecret(tc.in); got != tc.want {
			t.Errorf("RedactSecret(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
