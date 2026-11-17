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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// subject is one command identity as the engine sees it: a primary path, every
// executable entry path that reaches the same operation, and trusted Safety.
type subject struct {
	primary string
	entries []string
	safety  safetyFacts
}

func (s subject) inSet(set map[string]bool) bool {
	if set[s.primary] {
		return true
	}
	for _, entry := range s.entries {
		if set[entry] {
			return true
		}
	}
	return false
}

// evaluation is a verdict plus the facts hints are derived from.
type evaluation struct {
	decision Decision
	// deniedBy lists rules whose Deny matched the subject, whatever the final
	// verdict. A non-empty list on an allowed subject is a shadowed Deny.
	deniedBy []string
}

// evaluate applies the rule contract to one subject. The order is fixed:
// invalid policy, discovery entries, channel entries, invalid Safety, rules.
func evaluate(state State, rules []edition.CommandRule, s subject) evaluation {
	if state == StateInvalid {
		if s.inSet(invalidStateSet) {
			return evaluation{decision: Decision{Allowed: true}}
		}
		return evaluation{decision: Decision{
			ReasonCode: ReasonPolicyInvalid,
			Reason:     "the command restriction policy is invalid; every business command is denied",
		}}
	}
	if s.inSet(discoverySet) {
		return evaluation{decision: Decision{Allowed: true}}
	}
	channel := s.inSet(channelSet)
	if s.safety.invalid != "" {
		return evaluation{decision: Decision{ReasonCode: ReasonRiskInvalid, Reason: s.safety.invalid}}
	}

	var result evaluation
	denials := make([]Decision, 0, len(rules))
	for _, rule := range rules {
		verdict, denyMatched := evaluateRule(rule, s, channel)
		if denyMatched {
			result.deniedBy = append(result.deniedBy, rule.Name)
		}
		if verdict.Allowed {
			if !result.decision.Allowed {
				result.decision = verdict
			}
			continue
		}
		denials = append(denials, verdict)
	}
	if result.decision.Allowed {
		return result
	}
	result.decision = mergeDenials(denials, channel)
	return result
}

// evaluateRule checks every dimension of one rule (AND). denyMatched reports a
// Deny hit independently of the verdict so shadowed Denies can be diagnosed.
func evaluateRule(rule edition.CommandRule, s subject, channel bool) (verdict Decision, denyMatched bool) {
	deny := func(code, format string, args ...any) Decision {
		return Decision{
			ReasonCode: code, Reason: fmt.Sprintf(format, args...),
			RuleName: rule.Name, DenyMessage: strings.TrimSpace(rule.DenyMessage),
		}
	}
	var denyGlob, denyPath string
	for _, path := range s.paths() {
		if glob, ok := matchAny(rule.Deny, path); ok {
			denyGlob, denyPath, denyMatched = glob, path, true
			break
		}
	}

	// Naming a channel entry is an additional condition of the rule, not a
	// substitute for the others: a named channel still has to pass this
	// rule's Deny, Allow, declaration, and ceiling checks below. Without the
	// name no glob, ceiling, or AllowUnannotated can open it.
	if channel && !containsExact(rule.AllowChannels, s.paths()) {
		return deny(ReasonChannelNotAllowed, "%q is a channel entry and rule %q does not name it in AllowChannels", s.primary, rule.Name), denyMatched
	}

	if denyMatched {
		return deny(ReasonDenylisted, "command path %q matched deny pattern %q", denyPath, denyGlob), true
	}
	if len(rule.Allow) > 0 {
		if _, ok := matchAny(rule.Allow, s.primary); !ok {
			return deny(ReasonNotAllowed, "command path %q is not in the allow list of rule %q", s.primary, rule.Name), false
		}
	}
	if (s.safety.effect == "" || s.safety.risk == "") && !rule.AllowUnannotated {
		return deny(ReasonNotAnnotated, "command %q has no complete effect/risk declaration and rule %q does not allow unannotated commands", s.primary, rule.Name), false
	}
	if verdict, ok := exceedsCeiling(rule, s, deny); ok {
		return verdict, false
	}
	return Decision{Allowed: true, RuleName: rule.Name}, false
}

// exceedsCeiling compares declared effect/risk against the rule ceilings. A
// missing dimension is skipped here, never treated as the lowest level; the
// caller has already decided whether a missing declaration is admissible.
func exceedsCeiling(rule edition.CommandRule, s subject, deny func(string, string, ...any) Decision) (Decision, bool) {
	if rule.MaxEffect != "" && s.safety.effect != "" && effectRank[s.safety.effect] > effectRank[rule.MaxEffect] {
		return deny(ReasonRiskTooHigh, "command effect %q exceeds max effect %q of rule %q", s.safety.effect, rule.MaxEffect, rule.Name), true
	}
	if rule.MaxRisk != "" && s.safety.risk != "" && riskRank[s.safety.risk] > riskRank[rule.MaxRisk] {
		return deny(ReasonRiskTooHigh, "command risk %q exceeds max risk %q of rule %q", s.safety.risk, rule.MaxRisk, rule.Name), true
	}
	return Decision{}, false
}

// mergeDenials keeps the specific reason when exactly one rule judged the
// subject, and reports no_matching_rule otherwise. A channel entry nobody
// named reports channel_not_allowed whatever the number of rules.
func mergeDenials(denials []Decision, channel bool) Decision {
	if channel {
		for _, denial := range denials {
			if denial.ReasonCode != ReasonChannelNotAllowed {
				channel = false
				break
			}
		}
		if channel {
			return Decision{ReasonCode: ReasonChannelNotAllowed, Reason: "this command is a channel entry and no rule names it in AllowChannels"}
		}
	}
	switch len(denials) {
	case 0:
		return Decision{ReasonCode: ReasonNoMatchingRule, Reason: "the policy has no rules; every business command is denied"}
	case 1:
		return denials[0]
	}
	parts := make([]string, len(denials))
	for i, denial := range denials {
		parts[i] = denial.RuleName + ": " + denial.ReasonCode
	}
	return Decision{
		ReasonCode:  ReasonNoMatchingRule,
		Reason:      "no rule allows this command (" + strings.Join(parts, "; ") + ")",
		DenyMessage: denylistedMessages(denials),
	}
}

// denylistedMessages picks the guidance for a command several rules denied.
// Only a rule whose Deny matched expressed an intent about this command; a
// rule that merely does not cover it (an unrelated Allow, a ceiling) has
// nothing to say about it, so its message is left out. Messages keep rule
// order and are de-duplicated.
func denylistedMessages(denials []Decision) string {
	var messages []string
	seen := make(map[string]bool, len(denials))
	for _, denial := range denials {
		if denial.ReasonCode != ReasonDenylisted || denial.DenyMessage == "" || seen[denial.DenyMessage] {
			continue
		}
		seen[denial.DenyMessage] = true
		messages = append(messages, denial.DenyMessage)
	}
	return strings.Join(messages, " ")
}

func (s subject) paths() []string {
	paths := make([]string, 0, len(s.entries)+1)
	paths = append(paths, s.primary)
	for _, entry := range s.entries {
		if entry != s.primary {
			paths = append(paths, entry)
		}
	}
	return paths
}

func containsExact(values, candidates []string) bool {
	for _, value := range values {
		for _, candidate := range candidates {
			if value == candidate {
				return true
			}
		}
	}
	return false
}
