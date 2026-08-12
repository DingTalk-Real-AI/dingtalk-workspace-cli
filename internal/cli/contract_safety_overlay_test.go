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

package cli

import (
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
)

func TestCrossPlatformCoverageApplyContractRiskToSafety(t *testing.T) {
	base := contract.SafetySpec{Idempotency: "unknown", Risk: "low", Confirmation: "not_required", Effect: "read"}
	got := applyContractRiskToSafety(base, "high-risk-write")
	if got.Effect != "destructive" || got.Risk != "high" || got.Confirmation != "user_required" {
		t.Fatalf("high-risk overlay = %#v", got)
	}
	if got.Idempotency != "unknown" {
		t.Fatalf("idempotency should be preserved, got %q", got.Idempotency)
	}
	got = applyContractRiskToSafety(base, "write")
	if got.Effect != "write" || got.Risk != "medium" || got.Confirmation != "user_required" {
		t.Fatalf("write overlay = %#v", got)
	}
	got = applyContractRiskToSafety(base, "read")
	if got.Effect != "read" || got.Risk != "low" || got.Confirmation != "not_required" {
		t.Fatalf("read overlay = %#v", got)
	}
}

func TestCrossPlatformCoverageApplyContractGateToSafety(t *testing.T) {
	base := contract.SafetySpec{Confirmation: "not_required", Effect: "read", Risk: "low"}
	got := applyContractGateToSafety(base, "devAppRequireWriteGuard")
	if got.Confirmation != "user_required" {
		t.Fatalf("gate must force user_required, got %#v", got)
	}
	if got.Effect != "write" || got.EffectSource != "corecmd.contract_gate" {
		t.Fatalf("gate effect overlay = %#v", got)
	}
	if got.Risk != "medium" {
		t.Fatalf("gate risk overlay = %#v", got)
	}
	reviewed := contract.SafetySpec{Confirmation: "not_required", Effect: "destructive", Risk: "high", EffectSource: "reviewed"}
	got = applyContractGateToSafety(reviewed, "devAppRequireWriteGuard")
	if got.Effect != "destructive" || got.Risk != "high" || got.Confirmation != "user_required" {
		t.Fatalf("gate must keep reviewed effect/risk: %#v", got)
	}
	if got = applyContractGateToSafety(base, "   "); got != base {
		t.Fatalf("blank gate must be a no-op, got %#v", got)
	}
}
