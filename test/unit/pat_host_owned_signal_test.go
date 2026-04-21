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

package unit_test

import (
	"testing"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
)

// TestHostOwnsPATFlow_OnlySignal is the wire-level guard for
// docs/pat/contract.md §7 and SSOT §1 + §2 "custom authorization card"
// contract: the CLI switches to host-owned PAT mode iff the host injects
// DINGTALK_DWS_AGENTCODE. DINGTALK_AGENT / claw-type is purely a
// server-side routing tag and must NOT influence the decision, in either
// direction.
//
// Regression guard: several earlier drafts conflated the two signals,
// causing third-party Agent hosts that only set DINGTALK_DWS_AGENTCODE
// to still see a browser popup (CLI-owned fallback). This table locks
// in the decoupling.
func TestHostOwnsPATFlow_OnlySignal(t *testing.T) {
	cases := []struct {
		name      string
		agentCode string
		agentEnv  string
		want      bool
	}{
		{
			name:      "no signal → CLI-owned",
			agentCode: "",
			agentEnv:  "",
			want:      false,
		},
		{
			name:      "agent code only → host-owned",
			agentCode: "agt-cursor",
			agentEnv:  "",
			want:      true,
		},
		{
			name:      "agent code + DINGTALK_AGENT=default → host-owned",
			agentCode: "agt-cursor",
			agentEnv:  "default",
			want:      true,
		},
		{
			name:      "agent code + DINGTALK_AGENT=business → host-owned",
			agentCode: "agt-cursor",
			agentEnv:  "sales-copilot",
			want:      true,
		},
		{
			name:      "DINGTALK_AGENT=business, no agent code → CLI-owned",
			agentCode: "",
			agentEnv:  "sales-copilot",
			want:      false,
		},
		{
			name:      "DINGTALK_AGENT=default, no agent code → CLI-owned",
			agentCode: "",
			agentEnv:  "default",
			want:      false,
		},
		{
			name:      "whitespace-only agent code → CLI-owned",
			agentCode: "   ",
			agentEnv:  "sales-copilot",
			want:      false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(authpkg.AgentCodeEnv, tc.agentCode)
			// DINGTALK_AGENT is set purely to demonstrate that it does NOT
			// influence the host-owned decision. The literal env name is
			// used here because the auth package no longer exports a
			// DingTalkAgentEnv constant (it is not part of the PAT
			// decision surface).
			t.Setenv("DINGTALK_AGENT", tc.agentEnv)

			if got := authpkg.HostOwnsPATFlow(); got != tc.want {
				t.Fatalf(
					"HostOwnsPATFlow() = %v, want %v (agentCode=%q, DINGTALK_AGENT=%q)",
					got, tc.want, tc.agentCode, tc.agentEnv,
				)
			}
		})
	}
}
