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

package cmdpolicy

import (
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/runtimeannotate"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

// safetyFacts are the trusted effect/risk facts of one command. Only
// declarations compiled into the binary are read: ContractFinal Safety first,
// then the residual legacy runtime annotations. A user-defined shortcut's
// self-declared risk is never consulted.
//
// An empty field means "not declared". invalid is set when a declared value is
// not part of the closed taxonomy; that is a code defect, not a missing
// annotation, and no rule can admit it.
type safetyFacts struct {
	effect  string
	risk    string
	invalid string
}

var (
	effectRank = map[string]int{edition.EffectRead: 0, edition.EffectWrite: 1, edition.EffectDestructive: 2}
	riskRank   = map[string]int{edition.RiskLow: 0, edition.RiskMedium: 1, edition.RiskHigh: 2}
)

func commandSafety(cmd *cobra.Command) safetyFacts {
	var facts safetyFacts
	if final, ok := contractfinal.RuntimeContractFinal(cmd); ok && final.Safety != nil {
		facts.effect = strings.TrimSpace(final.Safety.Effect)
		facts.risk = strings.TrimSpace(final.Safety.Risk)
	}
	if facts.effect == "" && facts.risk == "" {
		if legacy, ok := runtimeannotate.RuntimeContractRisk(cmd); ok {
			effect, risk, known := legacyRiskFacts(legacy)
			if !known {
				facts.invalid = fmt.Sprintf("legacy risk annotation %q is not a known value", legacy)
				return facts
			}
			facts.effect, facts.risk = effect, risk
		}
	}
	if _, gated := runtimeannotate.RuntimeContractGate(cmd); gated {
		// A runtime confirmation gate proves the command is not a plain read.
		// Mirror the Schema overlay: raise, never lower.
		if facts.effect == "" || facts.effect == edition.EffectRead {
			facts.effect = edition.EffectWrite
		}
		if facts.risk == "" || facts.risk == edition.RiskLow {
			facts.risk = edition.RiskMedium
		}
	}
	return validatedSafety(facts)
}

func validatedSafety(facts safetyFacts) safetyFacts {
	if facts.invalid != "" {
		return facts
	}
	if facts.effect != "" {
		if _, ok := effectRank[facts.effect]; !ok {
			facts.invalid = fmt.Sprintf("declared effect %q is not one of read, write, destructive", facts.effect)
			return facts
		}
	}
	if facts.risk != "" {
		if _, ok := riskRank[facts.risk]; !ok {
			facts.invalid = fmt.Sprintf("declared risk %q is not one of low, medium, high", facts.risk)
		}
	}
	return facts
}

// legacyRiskFacts adapts the retired single-axis Contract Risk annotation to
// the effect/risk pair. The table must stay identical to the Schema overlay
// (internal/cli applyContractRiskToSafety). The legacy strings are a different
// vocabulary and must never be compared as risk values directly.
func legacyRiskFacts(legacy string) (effect, risk string, known bool) {
	switch strings.TrimSpace(legacy) {
	case "read":
		return edition.EffectRead, edition.RiskLow, true
	case "write":
		return edition.EffectWrite, edition.RiskMedium, true
	case "high-risk-write":
		return edition.EffectDestructive, edition.RiskHigh, true
	}
	return "", "", false
}
