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
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
)

func TestParseRoute_Valid(t *testing.T) {
	r, err := ParseRoute(`^im\.message=dir:./im/`)
	if err != nil {
		t.Fatalf("ParseRoute: %v", err)
	}
	if !r.Pattern.MatchString("im.message.receive_v1") {
		t.Error("regex did not match expected event type")
	}
	if r.Dir != "./im/" {
		t.Errorf("Dir = %q", r.Dir)
	}
	if r.Raw != `^im\.message=dir:./im/` {
		t.Errorf("Raw = %q", r.Raw)
	}
}

func TestParseRoute_Invalid(t *testing.T) {
	cases := []string{
		"",                   // empty
		"no-separator",       // no =dir:
		"=dir:./x/",          // empty regex
		"^im=dir:",           // empty path
		"(unclosed=dir:./x/", // invalid regex
		"=dir:",              // both empty
	}
	for _, in := range cases {
		if _, err := ParseRoute(in); err == nil {
			t.Errorf("ParseRoute(%q) should error", in)
		}
	}
}

func TestParseRoutes_StopsOnFirstError(t *testing.T) {
	good := `^im=dir:./im/`
	bad := `(unclosed=dir:./x/`
	out, err := ParseRoutes([]string{good, bad, good})
	if err == nil {
		t.Fatal("expected parse error")
	}
	if len(out) != 1 {
		t.Errorf("partial parse should have 1 entry, got %d", len(out))
	}
}

func TestRouter_FirstMatchWins(t *testing.T) {
	routes, err := ParseRoutes([]string{
		`^im\.message=dir:./im/`,
		`^im\.=dir:./other-im/`,
	})
	if err != nil {
		t.Fatal(err)
	}
	r := NewRouter(routes)

	// First rule should win for im.message.* events.
	got := r.Match(transport.Event{EventType: "im.message.receive_v1"})
	if got != "./im/" {
		t.Errorf("Match im.message = %q, want ./im/", got)
	}
	// Second rule covers im.chat.*
	got = r.Match(transport.Event{EventType: "im.chat.member.bot.added_v1"})
	if got != "./other-im/" {
		t.Errorf("Match im.chat = %q, want ./other-im/", got)
	}
}

func TestRouter_NoMatchReturnsEmpty(t *testing.T) {
	routes, _ := ParseRoutes([]string{`^im\.=dir:./im/`})
	r := NewRouter(routes)
	if got := r.Match(transport.Event{EventType: "approval.task"}); got != "" {
		t.Fatalf("no-match should return empty, got %q", got)
	}
}

func TestRouter_NilSafe(t *testing.T) {
	var r *Router
	if got := r.Match(transport.Event{EventType: "x"}); got != "" {
		t.Fatalf("nil Router.Match should return empty, got %q", got)
	}
	if rules := r.Rules(); rules != nil {
		t.Fatalf("nil Router.Rules should return nil, got %v", rules)
	}
}
