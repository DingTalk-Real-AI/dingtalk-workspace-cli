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

// resetApplyFlags returns applyCmd flags to documented defaults between
// subtests. See resetChmodFlags for rationale.
func resetApplyFlags(t *testing.T) {
	t.Helper()
	defaults := map[string]string{
		"agentCode":  "",
		"grant-type": "session",
		"session-id": "",
	}
	for name, def := range defaults {
		if f := applyCmd.Flags().Lookup(name); f != nil {
			_ = f.Value.Set(def)
			f.Changed = false
		}
	}
	t.Cleanup(func() {
		for name, def := range defaults {
			if f := applyCmd.Flags().Lookup(name); f != nil {
				_ = f.Value.Set(def)
				f.Changed = false
			}
		}
	})
}

// ---------------------------------------------------------------------------
// T1 · Agent-code env fallback tests for `dws pat apply`
// ---------------------------------------------------------------------------

// TestApply_agentCode_env_fallback mirrors TestChmod_agentCode_env_fallback
// for the orchestrator entry. Host-integration FAQ in docs/pat/host-integration.md
// documents this behaviour explicitly.
func TestApply_agentCode_env_fallback(t *testing.T) {
	resetApplyFlags(t)
	t.Setenv(agentCodeEnv, "qoderwork")

	fake := &fakeToolCaller{resultOK: true}
	installFakeCaller(t, fake)

	_ = applyCmd.Flags().Set("grant-type", "once")
	if err := runApply(applyCmd, []string{"aitable.record:read"}); err != nil {
		t.Fatalf("apply runApply error = %v", err)
	}
	if got := fake.gotArgs["agentCode"]; got != "qoderwork" {
		t.Fatalf("agentCode in argv = %v, want %q (env fallback)", got, "qoderwork")
	}
}

// TestApply_agentCode_env_invalid ensures malformed
// DINGTALK_DWS_AGENTCODE values are rejected before any MCP call.
// Contract: docs/pat/contract.md §9.
func TestApply_agentCode_env_invalid(t *testing.T) {
	resetApplyFlags(t)
	t.Setenv(agentCodeEnv, "bad value with space!")

	fake := &fakeToolCaller{resultOK: true}
	installFakeCaller(t, fake)
	_ = applyCmd.Flags().Set("grant-type", "once")

	err := runApply(applyCmd, []string{"aitable.record:read"})
	if err == nil {
		t.Fatalf("expected validateAgentCode error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid agentCode") {
		t.Fatalf("error = %q, want to mention 'invalid agentCode'", err.Error())
	}
	if !strings.Contains(err.Error(), agentCodeEnv) {
		t.Fatalf("error = %q, want to attribute to %s env", err.Error(), agentCodeEnv)
	}
	if fake.callN != 0 {
		t.Fatalf("CallTool was invoked %d times; validator must short-circuit before MCP", fake.callN)
	}
}

// TestApply_agentCode_flag_wins_over_env locks in the Priority-1 rule:
// when both --agentCode and DINGTALK_DWS_AGENTCODE are set the flag wins
// silently.
func TestApply_agentCode_flag_wins_over_env(t *testing.T) {
	resetApplyFlags(t)
	t.Setenv(agentCodeEnv, "envval")

	fake := &fakeToolCaller{resultOK: true}
	installFakeCaller(t, fake)

	_ = applyCmd.Flags().Set("grant-type", "once")
	_ = applyCmd.Flags().Set("agentCode", "flagval")

	if err := runApply(applyCmd, []string{"aitable.record:read"}); err != nil {
		t.Fatalf("runApply error = %v", err)
	}
	if got := fake.gotArgs["agentCode"]; got != "flagval" {
		t.Fatalf("agentCode in argv = %v, want %q (flag must win over env)", got, "flagval")
	}
}

// TestApply_agentCode_legacy_env_not_recognized is the reverse-guard
// mirror of TestChmod_agentCode_legacy_env_not_recognized. After SSOT
// hard-removal, exporting only the legacy DWS_AGENTCODE must cause
// `dws pat apply` to fail with a canonical-env hint and perform no MCP
// call.
func TestApply_agentCode_legacy_env_not_recognized(t *testing.T) {
	resetApplyFlags(t)
	t.Setenv(agentCodeEnv, "")
	t.Setenv("DWS_AGENTCODE", "legacyval")

	fake := &fakeToolCaller{resultOK: true}
	installFakeCaller(t, fake)
	_ = applyCmd.Flags().Set("grant-type", "once")

	err := runApply(applyCmd, []string{"aitable.record:read"})
	if err == nil {
		t.Fatalf("expected hard error when only legacy DWS_AGENTCODE is set, got nil")
	}
	if !strings.Contains(err.Error(), "DINGTALK_DWS_AGENTCODE") {
		t.Fatalf("error = %q, want to name canonical DINGTALK_DWS_AGENTCODE env", err.Error())
	}
	if fake.callN != 0 {
		t.Fatalf("CallTool was invoked %d times; legacy env must not satisfy --agentCode", fake.callN)
	}
}
