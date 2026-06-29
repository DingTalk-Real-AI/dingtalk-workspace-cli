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

package helpers

import (
	"testing"
	"time"
)

func TestConnectGateAllowlists(t *testing.T) {
	cases := []struct {
		name       string
		users      []string
		groups     []string
		staff      string
		convType   string
		convID     string
		wantOK     bool
		wantReason string
	}{
		{"no policy allows everyone", nil, nil, "u1", "1", "", true, ""},
		{"user in list", []string{"u1", "u2"}, nil, "u1", "1", "", true, ""},
		{"user not in list", []string{"u1"}, nil, "u9", "1", "", false, "user-not-allowed"},
		{"group in list", nil, []string{"cid-a"}, "u9", "2", "cid-a", true, ""},
		{"group not in list", nil, []string{"cid-a"}, "u9", "2", "cid-b", false, "group-not-allowed"},
		{"group list does not gate DMs", nil, []string{"cid-a"}, "u9", "1", "", true, ""},
		{"both lists: group ok user not", []string{"u1"}, []string{"cid-a"}, "u9", "2", "cid-a", false, "user-not-allowed"},
		{"both lists: both ok", []string{"u1"}, []string{"cid-a"}, "u1", "2", "cid-a", true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := newConnectGate(tc.users, tc.groups, 0)
			ok, reason := g.allow(tc.staff, tc.convType, tc.convID)
			if ok != tc.wantOK || reason != tc.wantReason {
				t.Fatalf("allow = (%v, %q), want (%v, %q)", ok, reason, tc.wantOK, tc.wantReason)
			}
		})
	}
}

func TestConnectGateRateLimit(t *testing.T) {
	g := newConnectGate(nil, nil, 2)
	now := time.Unix(1_700_000_000, 0)
	g.now = func() time.Time { return now }

	for i := 0; i < 2; i++ {
		if ok, _ := g.allow("u1", "1", ""); !ok {
			t.Fatalf("message %d should pass", i+1)
		}
	}
	if ok, reason := g.allow("u1", "1", ""); ok || reason != "rate-limited" {
		t.Fatalf("3rd message = (%v, %q), want rate-limited", ok, reason)
	}
	// Another sender is unaffected.
	if ok, _ := g.allow("u2", "1", ""); !ok {
		t.Fatal("other sender should pass")
	}
	// The window slides: a minute later the sender passes again.
	now = now.Add(61 * time.Second)
	if ok, _ := g.allow("u1", "1", ""); !ok {
		t.Fatal("after window slides the sender should pass")
	}
}

func TestConnectGateDisabled(t *testing.T) {
	var nilGate *connectGate
	if nilGate.enabled() {
		t.Fatal("nil gate must report disabled")
	}
	if ok, _ := nilGate.allow("u", "1", ""); !ok {
		t.Fatal("nil gate must allow")
	}
	if newConnectGate(nil, nil, 0).enabled() {
		t.Fatal("empty gate must report disabled")
	}
}

func TestSplitCommaList(t *testing.T) {
	got := splitCommaList(" a, b ,, c ")
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("splitCommaList = %v", got)
	}
	if splitCommaList("") != nil {
		t.Fatal("empty input should yield nil")
	}
}
