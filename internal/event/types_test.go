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
	"math"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestItoaCoversSignedBoundaries(t *testing.T) {
	for value, want := range map[int64]string{
		0: "0", 1: "1", -1: "-1", math.MaxInt64: "9223372036854775807", math.MinInt64: "-9223372036854775808",
	} {
		if got := itoa(value); got != want {
			t.Errorf("itoa(%d) = %q, want %q", value, got, want)
		}
	}
}

func TestRawEvent_DedupKey_UsesEventID(t *testing.T) {
	e := &RawEvent{EventID: "ev_abc", EventType: "im.message", SubscribeID: "sub-1"}
	copy := *e
	copy.Data = `{"differentEnvelope":true}`
	copy.EventBornTime = 123
	copy.ReceivedAt = copy.ReceivedAt.Add(1)
	copy.Headers = map[string]string{"token": "must-not-affect-key"}
	if got := e.DedupKey(); got == "" || got != copy.DedupKey() {
		t.Fatalf("same delivery ID must dedup regardless of envelope: %q vs %q", got, copy.DedupKey())
	}
	copy.EventID = "ev_other"
	if e.DedupKey() == copy.DedupKey() {
		t.Fatal("different EventIDs must not dedup")
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
	e2 = *e
	e2.EventBornTime++
	if e2.DedupKey() == got {
		t.Fatal("fallback must distinguish born time")
	}
}

func TestCrossPlatformCoverageRawEventDedupRoutingIdentity(t *testing.T) {
	for _, eventID := range []string{"same-business-event", ""} {
		for name, mutate := range map[string]func(*RawEvent){
			"subscription": func(e *RawEvent) { e.SubscribeID = "another-sub" },
			"source":       func(e *RawEvent) { e.SourceID = "another-source" },
			"tenant":       func(e *RawEvent) { e.EventCorpID = "another-corp" },
			"app":          func(e *RawEvent) { e.EventUnifiedAppID = "another-app" },
			"scope":        func(e *RawEvent) { e.EventScope = "another-scope" },
			"event-type":   func(e *RawEvent) { e.EventType = "user_im_message_receive_user" },
			"missing-sub":  func(e *RawEvent) { e.SubscribeID = "" },
		} {
			t.Run(eventID+"/"+name, func(t *testing.T) {
				original := RawEvent{EventID: eventID, EventType: "user_im_message_receive_o2o",
					SubscribeID: "current-sub", SourceID: "source", EventCorpID: "corp",
					EventUnifiedAppID: "app", EventScope: "scope", Data: `{"msgId":"same"}`}
				other := original
				mutate(&other)
				if original.DedupKey() == other.DedupKey() {
					t.Fatal("distinct routing identities must survive dedup independently")
				}
			})
		}
	}
}

func TestCrossPlatformCoverageRawEventDedupFieldBoundariesAndIdentityKind(t *testing.T) {
	a := RawEvent{SourceID: "a:b", EventCorpID: "c", EventID: "same"}
	b := RawEvent{SourceID: "a", EventCorpID: "b:c", EventID: "same"}
	if a.DedupKey() == b.DedupKey() {
		t.Fatal("separator-containing fields must not collide")
	}
	fallback := RawEvent{Data: "same", EventBornTime: 123}
	identified := fallback
	identified.EventID = "123:0967115f2813a354" // Same literal as the fallback identity.
	if fallback.DedupKey() == identified.DedupKey() {
		t.Fatal("SDK ID and content fallback namespaces must differ")
	}
}

func BenchmarkRawEvent_DedupKey(b *testing.B) {
	for _, size := range []int{64, 64 * 1024} {
		for _, id := range []string{"event-id", ""} {
			b.Run(itoa(int64(size))+"/"+id, func(b *testing.B) {
				e := RawEvent{EventID: id, EventType: "user_im_message_receive_o2o",
					SubscribeID: "sub-current", SourceID: "source", Data: strings.Repeat("x", size)}
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					e.DedupKey()
				}
			})
		}
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
		parent := t.TempDir()
		joined := filepath.Join(parent, got)
		rel, err := filepath.Rel(parent, joined)
		if err != nil || rel != got {
			t.Fatalf("path escape: joined=%q rel=%q err=%v", joined, rel, err)
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
