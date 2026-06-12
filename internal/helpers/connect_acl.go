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
	"strings"
	"sync"
	"time"
)

// connectGate enforces the connector's access policy: optional user/group
// allowlists plus a per-sender rate limit. A long-lived Q&A bot is otherwise
// drivable (and billable — every message is an LLM call) by anyone who can
// reach it in a group.
type connectGate struct {
	allowedUsers  map[string]struct{}
	allowedGroups map[string]struct{}
	perMinute     int

	mu   sync.Mutex
	hits map[string][]time.Time
	now  func() time.Time // test hook
}

// splitCommaList splits a comma-separated flag/env value, trimming blanks.
func splitCommaList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// newConnectGate builds a gate from staffId / openConversationId allowlists
// and a per-sender messages-per-minute cap (0 = unlimited).
func newConnectGate(users, groups []string, perMinute int) *connectGate {
	g := &connectGate{perMinute: perMinute, hits: map[string][]time.Time{}, now: time.Now}
	for _, u := range users {
		if u = strings.TrimSpace(u); u != "" {
			if g.allowedUsers == nil {
				g.allowedUsers = map[string]struct{}{}
			}
			g.allowedUsers[u] = struct{}{}
		}
	}
	for _, grp := range groups {
		if grp = strings.TrimSpace(grp); grp != "" {
			if g.allowedGroups == nil {
				g.allowedGroups = map[string]struct{}{}
			}
			g.allowedGroups[grp] = struct{}{}
		}
	}
	return g
}

// enabled reports whether any policy is configured at all.
func (g *connectGate) enabled() bool {
	return g != nil && (len(g.allowedUsers) > 0 || len(g.allowedGroups) > 0 || g.perMinute > 0)
}

// allow reports whether a message may proceed; on denial, reason names the
// rule that fired (for the connector log). Group messages must pass the group
// allowlist AND, when a user allowlist is set, the sender allowlist too.
func (g *connectGate) allow(staffID, convType, convID string) (bool, string) {
	if g == nil {
		return true, ""
	}
	if convType == "2" && len(g.allowedGroups) > 0 {
		if _, ok := g.allowedGroups[convID]; !ok {
			return false, "group-not-allowed"
		}
	}
	if len(g.allowedUsers) > 0 {
		if _, ok := g.allowedUsers[staffID]; !ok {
			return false, "user-not-allowed"
		}
	}
	if g.perMinute > 0 && staffID != "" {
		g.mu.Lock()
		defer g.mu.Unlock()
		now := g.now()
		cutoff := now.Add(-time.Minute)
		kept := g.hits[staffID][:0]
		for _, t := range g.hits[staffID] {
			if t.After(cutoff) {
				kept = append(kept, t)
			}
		}
		if len(kept) >= g.perMinute {
			g.hits[staffID] = kept
			return false, "rate-limited"
		}
		g.hits[staffID] = append(kept, now)
		// Bound memory against sender-id churn: keep only the current sender
		// when the map grows absurdly large.
		if len(g.hits) > 4096 {
			g.hits = map[string][]time.Time{staffID: g.hits[staffID]}
		}
	}
	return true, ""
}
