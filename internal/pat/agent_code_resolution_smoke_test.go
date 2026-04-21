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

package pat

import (
	"strings"
	"testing"
)

// D5 Smoke B — agent code two-tier resolution contract.
//
// SSOT §2 / §3.2 and docs/pat/contract.md §9 freeze the `--agentCode`
// resolution chain at exactly two tiers plus a hard error:
//
//	--agentCode flag  >  DINGTALK_DWS_AGENTCODE env  >  error
//
// No legacy aliases (DWS_AGENTCODE / DINGTALK_AGENTCODE / REWIND_AGENTCODE)
// are consulted. This table-driven smoke test is the CI-level guard that
// a future refactor does not silently reintroduce a third tier or swap
// precedence. It exercises resolveAgentCode directly — the single
// function every `dws pat *` command delegates to — so all subcommands
// inherit the same defence.
//
// For the negative "DWS_AGENTCODE alone is invalid" assertion we rely
// on the dedicated TestChmod_agentCode_legacy_env_not_recognized /
// TestApply_agentCode_legacy_env_not_recognized tests (already in
// this package) to avoid duplication.
func TestResolveAgentCode_TwoTierChain_Smoke(t *testing.T) {
	// Not parallel — mutates process env via t.Setenv.

	cases := []struct {
		name       string
		flag       string
		primaryEnv string // value of DINGTALK_DWS_AGENTCODE; "" means unset
		legacyEnv  string // value of DWS_AGENTCODE; "" means unset — MUST NOT change outcome
		required   bool
		wantCode   string // expected resolved agent code; "" when error expected
		wantErr    bool
		wantErrSub string // required substring in the error message (when wantErr)
	}{
		{
			name:       "flag only — selects flag, env unset",
			flag:       "agt-flag",
			primaryEnv: "",
			legacyEnv:  "",
			required:   true,
			wantCode:   "agt-flag",
		},
		{
			name:       "env only — selects DINGTALK_DWS_AGENTCODE",
			flag:       "",
			primaryEnv: "agt-env",
			legacyEnv:  "",
			required:   true,
			wantCode:   "agt-env",
		},
		{
			name:       "neither set — hard error names canonical env",
			flag:       "",
			primaryEnv: "",
			legacyEnv:  "",
			required:   true,
			wantErr:    true,
			wantErrSub: "DINGTALK_DWS_AGENTCODE",
		},
		{
			name:       "flag and env both set, different values — flag wins",
			flag:       "agt-flag",
			primaryEnv: "agt-env",
			legacyEnv:  "",
			required:   true,
			wantCode:   "agt-flag",
		},
		{
			name:       "flag and env both set, same value — flag wins (idempotent)",
			flag:       "agt-same",
			primaryEnv: "agt-same",
			legacyEnv:  "",
			required:   true,
			wantCode:   "agt-same",
		},
		{
			name:       "required=false, neither set — empty, no error (scopes default agent)",
			flag:       "",
			primaryEnv: "",
			legacyEnv:  "",
			required:   false,
			wantCode:   "",
		},
		{
			name:       "legacy DWS_AGENTCODE set alone — ignored, errors out",
			flag:       "",
			primaryEnv: "",
			legacyEnv:  "agt-legacy",
			required:   true,
			wantErr:    true,
			wantErrSub: "DINGTALK_DWS_AGENTCODE",
		},
		{
			name:       "legacy DWS_AGENTCODE set with primary — primary wins, legacy ignored",
			flag:       "",
			primaryEnv: "agt-primary",
			legacyEnv:  "agt-legacy",
			required:   true,
			wantCode:   "agt-primary",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(agentCodeEnv, tc.primaryEnv)
			t.Setenv("DWS_AGENTCODE", tc.legacyEnv)

			got, err := resolveAgentCode(tc.flag, tc.required)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("resolveAgentCode(%q, required=%v) err = nil, want non-nil; flag=%q env=%q legacy=%q",
						tc.flag, tc.required, tc.flag, tc.primaryEnv, tc.legacyEnv)
				}
				if tc.wantErrSub != "" && !strings.Contains(err.Error(), tc.wantErrSub) {
					t.Fatalf("resolveAgentCode() err = %q, want substring %q", err.Error(), tc.wantErrSub)
				}
				// Defensive: hard-fail hint must NOT advertise DWS_AGENTCODE
				// as a usable fallback. "DINGTALK_DWS_AGENTCODE" legitimately
				// contains "DWS_AGENTCODE" as a sub-substring; splitting on
				// the canonical env first lets us check only the residue.
				residue := strings.ReplaceAll(err.Error(), "DINGTALK_DWS_AGENTCODE", "")
				if strings.Contains(residue, "DWS_AGENTCODE") {
					t.Fatalf("resolveAgentCode() err = %q MUST NOT advertise legacy DWS_AGENTCODE", err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveAgentCode(%q, required=%v) err = %v, want nil; flag=%q env=%q",
					tc.flag, tc.required, err, tc.flag, tc.primaryEnv)
			}
			if got != tc.wantCode {
				t.Fatalf("resolveAgentCode(%q, required=%v) = %q, want %q; flag=%q env=%q legacy=%q",
					tc.flag, tc.required, got, tc.wantCode, tc.flag, tc.primaryEnv, tc.legacyEnv)
			}
		})
	}
}
