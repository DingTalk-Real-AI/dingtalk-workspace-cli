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

// Package cmdpolicy implements the compiled-in command restriction policy
// contributed through edition.Hooks.Restrict.
//
// The policy is resolved once per executable command tree into an immutable
// Snapshot owned by that tree's root. Execution, diagnostics, and Schema
// delivery all read that one snapshot; there is no process-global policy.
//
// This package must not import internal/cli: Schema delivery consumes the
// snapshot, so the dependency runs the other way.
package cmdpolicy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// State is the lifecycle state of the command restriction policy.
type State string

const (
	// StateDisabled: no Restrict hook. The CLI behaves exactly as without
	// this package.
	StateDisabled State = "disabled"
	// StateActive: a valid rule set is enforced. An empty rule set is valid
	// and denies every business command.
	StateActive State = "active"
	// StateInvalid: the hook panicked or returned an invalid rule set. Only
	// the invalid-state diagnostic commands remain.
	StateInvalid State = "invalid"
)

// Reason codes are a closed, stable set surfaced in denial errors.
const (
	ReasonChannelNotAllowed = "channel_not_allowed"
	ReasonDenylisted        = "command_denylisted"
	ReasonNotAllowed        = "domain_not_allowed"
	ReasonNotAnnotated      = "risk_not_annotated"
	ReasonRiskInvalid       = "risk_invalid"
	ReasonRiskTooHigh       = "risk_too_high"
	ReasonNoMatchingRule    = "no_matching_rule"
	ReasonIdentityConflict  = "identity_conflict"
	ReasonPolicyInvalid     = "policy_invalid"
	// ReasonAllChildrenDenied is reported for a group (or a Schema product or
	// group) in which no command is permitted.
	ReasonAllChildrenDenied = "all_children_denied"
)

// Decision is the verdict for one command identity.
type Decision struct {
	Allowed    bool
	ReasonCode string
	Reason     string
	// RuleName is the rule that allowed the command, or the single rule that
	// denied it. It is empty when several rules each denied it.
	RuleName string
	// DenyMessage is the distributor-authored guidance attributed to a denial;
	// see edition.CommandRule.DenyMessage for which rules contribute.
	DenyMessage string
}

// DeniedPath is one denied executable entry, for diagnostics.
type DeniedPath struct {
	Path       string
	ReasonCode string
	Reason     string
	RuleName   string
}

// ChannelStatus reports how one reviewed channel entry resolved.
type ChannelStatus struct {
	Path     string
	Allowed  bool
	RuleName string
}

// Hint kinds.
const (
	HintDenyOverridden = "deny_overridden"
	HintRuleAllowsAll  = "rule_allows_all"
)

// Hint is a non-blocking diagnostic about how the rule set behaves.
type Hint struct {
	Kind      string
	Path      string
	Rule      string
	OtherRule string
	Message   string
}

// Snapshot is the immutable resolved policy of one command tree. A nil
// *Snapshot is valid and means the policy is disabled.
type Snapshot struct {
	state         State
	source        string
	rules         []edition.CommandRule
	invalidReason string

	// decisions holds the verdict of every evaluated executable entry, keyed
	// by policy path. Paths that are not executable entries are absent.
	decisions map[string]Decision
	// identities maps every known identifier of a command (entry paths and
	// the canonical dot path) to the policy path holding its decision.
	identities map[string]string
	channels   []ChannelStatus
	hints      []Hint
}

// Resolve calls the Restrict hook exactly once and validates its rules. It
// never returns nil for a non-nil hook: a panic or an invalid rule set yields
// an Invalid snapshot so the failure cannot degrade to "disabled".
func Resolve(hooks *edition.Hooks) *Snapshot {
	if hooks == nil || hooks.Restrict == nil {
		return nil
	}
	snapshot := &Snapshot{state: StateActive, source: hooks.Name}
	rules, err := callRestrict(hooks.Restrict)
	if err == nil {
		err = ValidateRules(rules)
	}
	if err != nil {
		snapshot.state = StateInvalid
		snapshot.invalidReason = err.Error()
		return snapshot
	}
	snapshot.rules = cloneRules(rules)
	return snapshot
}

func callRestrict(restrict func() []edition.CommandRule) (rules []edition.CommandRule, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			rules, err = nil, fmt.Errorf("Restrict hook panicked: %v", recovered)
		}
	}()
	return restrict(), nil
}

// ValidateRules checks a rule set against the rule contract. It is shared by
// runtime resolution and the edition contract tests.
func ValidateRules(rules []edition.CommandRule) error {
	names := make(map[string]bool, len(rules))
	for index, rule := range rules {
		if rule.Name == "" {
			return fmt.Errorf("rule #%d has no name", index)
		}
		if names[rule.Name] {
			return fmt.Errorf("rule name %q is declared more than once", rule.Name)
		}
		names[rule.Name] = true
		if err := validateDenyMessage(rule.DenyMessage); err != nil {
			return fmt.Errorf("rule %q deny message: %w", rule.Name, err)
		}
		if rule.MaxEffect != "" {
			if _, ok := effectRank[rule.MaxEffect]; !ok {
				return fmt.Errorf("rule %q: max effect %q is not one of read, write, destructive", rule.Name, rule.MaxEffect)
			}
		}
		if rule.MaxRisk != "" {
			if _, ok := riskRank[rule.MaxRisk]; !ok {
				return fmt.Errorf("rule %q: max risk %q is not one of low, medium, high", rule.Name, rule.MaxRisk)
			}
		}
		for _, glob := range rule.Allow {
			if err := validateRulePath(glob, true); err != nil {
				return fmt.Errorf("rule %q allow: %w", rule.Name, err)
			}
		}
		for _, glob := range rule.Deny {
			if err := validateRulePath(glob, true); err != nil {
				return fmt.Errorf("rule %q deny: %w", rule.Name, err)
			}
		}
		for _, channel := range rule.AllowChannels {
			if err := validateRulePath(channel, false); err != nil {
				return fmt.Errorf("rule %q allow channels: %w", rule.Name, err)
			}
			if !channelSet[channel] {
				return fmt.Errorf("rule %q allow channels: %q is not a reviewed channel entry", rule.Name, channel)
			}
		}
	}
	return nil
}

// maxDenyMessageRunes bounds CommandRule.DenyMessage so a denial stays one
// readable line in a terminal and in an Agent's context.
const maxDenyMessageRunes = 500

// validateDenyMessage rejects text that could not be shown as one plain line:
// the message is printed to terminals, so control characters are not allowed.
func validateDenyMessage(message string) error {
	if utf8.RuneCountInString(message) > maxDenyMessageRunes {
		return fmt.Errorf("longer than %d characters", maxDenyMessageRunes)
	}
	for _, r := range message {
		if unicode.IsControl(r) {
			return fmt.Errorf("contains control character %U; use one line of plain text", r)
		}
	}
	return nil
}

// RuleAllowsAll reports whether a rule has neither a path restriction nor an
// effect/risk ceiling, so it admits every declared non-channel command.
func RuleAllowsAll(rule edition.CommandRule) bool {
	return len(rule.Allow) == 0 && rule.MaxEffect == "" && rule.MaxRisk == ""
}

func cloneRules(rules []edition.CommandRule) []edition.CommandRule {
	out := make([]edition.CommandRule, len(rules))
	for i, rule := range rules {
		rule.Allow = append([]string(nil), rule.Allow...)
		rule.Deny = append([]string(nil), rule.Deny...)
		rule.AllowChannels = append([]string(nil), rule.AllowChannels...)
		out[i] = rule
	}
	return out
}

// State reports the policy state. A nil snapshot is disabled.
func (s *Snapshot) State() State {
	if s == nil {
		return StateDisabled
	}
	return s.state
}

// Enabled reports whether the Restrict hook is present (active or invalid).
// Runtime plugins and user-defined shortcuts are not loaded while enabled.
func (s *Snapshot) Enabled() bool { return s.State() != StateDisabled }

// Source is the edition name that contributed the policy.
func (s *Snapshot) Source() string {
	if s == nil {
		return ""
	}
	return s.source
}

// InvalidReason explains an Invalid state.
func (s *Snapshot) InvalidReason() string {
	if s == nil {
		return ""
	}
	return s.invalidReason
}

// Rules returns a copy of the validated rules.
func (s *Snapshot) Rules() []edition.CommandRule {
	if s == nil {
		return nil
	}
	return cloneRules(s.rules)
}

// Decision returns the verdict for an executable entry path. ok is false when
// the path is not an evaluated entry (a navigation group, the root, or a
// command mounted after the policy was applied).
func (s *Snapshot) Decision(path string) (Decision, bool) {
	if s == nil {
		return Decision{}, false
	}
	decision, ok := s.decisions[path]
	return decision, ok
}

// Lookup resolves any identifier of a command (a policy path, an authored
// space-separated CLI path, an alias, or a canonical dot path) to its verdict.
func (s *Snapshot) Lookup(identifier string) (Decision, bool) {
	if s == nil {
		return Decision{}, false
	}
	for _, key := range []string{identifier, DeclaredPath(identifier)} {
		if path, ok := s.identities[key]; ok {
			return s.Decision(path)
		}
	}
	return Decision{}, false
}

// ResolveIdentifier is Lookup that also returns the policy path the identifier
// resolved to.
func (s *Snapshot) ResolveIdentifier(identifier string) (path string, decision Decision, ok bool) {
	if s == nil {
		return "", Decision{}, false
	}
	for _, key := range []string{identifier, DeclaredPath(identifier)} {
		if resolved, found := s.identities[key]; found {
			decision, ok = s.Decision(resolved)
			return resolved, decision, ok
		}
	}
	return "", Decision{}, false
}

// SubtreeDenied reports whether path names a branch with executable entries
// none of which is allowed: a denied command, or a group whose every command
// is denied. A path with no executable entry at or below it reports false.
func (s *Snapshot) SubtreeDenied(path string) bool {
	if s == nil || path == "" {
		return false
	}
	prefix := path + "/"
	found := false
	for entry, decision := range s.decisions {
		if entry != path && !strings.HasPrefix(entry, prefix) {
			continue
		}
		if decision.Allowed {
			return false
		}
		found = true
	}
	return found
}

// ViewHash identifies the permitted command surface of an active policy: the
// same rule set over the same base catalog yields the same visible commands.
// It is empty unless the policy is active.
func (s *Snapshot) ViewHash() string {
	if s.State() != StateActive {
		return ""
	}
	encoded, err := json.Marshal(s.rules)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// DeniedPaths lists every denied executable entry, sorted by path.
func (s *Snapshot) DeniedPaths() []DeniedPath {
	if s == nil {
		return nil
	}
	out := make([]DeniedPath, 0, len(s.decisions))
	for path, decision := range s.decisions {
		if !decision.Allowed {
			out = append(out, DeniedPath{Path: path, ReasonCode: decision.ReasonCode, Reason: decision.Reason, RuleName: decision.RuleName})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// Channels reports every reviewed channel entry present in the tree.
func (s *Snapshot) Channels() []ChannelStatus {
	if s == nil {
		return nil
	}
	return append([]ChannelStatus(nil), s.channels...)
}

// Hints returns the non-blocking rule-set diagnostics.
func (s *Snapshot) Hints() []Hint {
	if s == nil {
		return nil
	}
	return append([]Hint(nil), s.hints...)
}
