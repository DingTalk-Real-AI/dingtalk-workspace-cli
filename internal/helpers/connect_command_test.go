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

import "testing"

func TestParseConnectControlCommand(t *testing.T) {
	cases := []struct {
		in       string
		wantName string // "" means no match
	}{
		// Recognised commands and their aliases.
		{"/new", "new"},
		{"/start", "new"},
		{"/reset", "new"},
		{"/clear", "clear"},
		// Case-insensitive and trimmed.
		{"/NEW", "new"},
		{"  /Clear  ", "clear"},
		{"/New\n", "new"},
		// A normal question that merely starts with a slash is NOT a command —
		// it must be forwarded to the agent, not swallowed.
		{"/new 这个功能怎么实现?", ""},
		{"/clear the cache please", ""},
		{"/new x", ""},
		// Non-commands.
		{"", ""},
		{"   ", ""},
		{"你好", ""},
		{"new", ""},      // missing slash
		{"/unknown", ""}, // unknown token
		{"/", ""},
	}
	for _, c := range cases {
		got, ok := parseConnectControlCommand(c.in)
		if c.wantName == "" {
			if ok {
				t.Errorf("parseConnectControlCommand(%q) = (%+v, true), want no match", c.in, got)
			}
			continue
		}
		if !ok {
			t.Errorf("parseConnectControlCommand(%q) = no match, want name %q", c.in, c.wantName)
			continue
		}
		if got.name != c.wantName {
			t.Errorf("parseConnectControlCommand(%q).name = %q, want %q", c.in, got.name, c.wantName)
		}
		if got.ack == "" {
			t.Errorf("parseConnectControlCommand(%q).ack is empty, want a user-facing confirmation", c.in)
		}
		if !got.resetsSession() {
			t.Errorf("parseConnectControlCommand(%q).resetsSession() = false, want true", c.in)
		}
	}
}
