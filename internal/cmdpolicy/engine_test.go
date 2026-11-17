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
	"errors"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func declared(effect, risk string) safetyFacts { return safetyFacts{effect: effect, risk: risk} }

func TestCrossPlatformCoverageEvaluateTruthTable(t *testing.T) {
	readOnly := edition.CommandRule{Name: "readonly", Allow: []string{"calendar/**"}, MaxEffect: "read"}
	tests := []struct {
		name    string
		state   State
		rules   []edition.CommandRule
		subject subject
		allowed bool
		reason  string
		rule    string
	}{
		{
			name:    "allow glob and ceiling pass",
			rules:   []edition.CommandRule{readOnly},
			subject: subject{primary: "calendar/event/list", safety: declared("read", "low")},
			allowed: true, rule: "readonly",
		},
		{
			name:    "effect above ceiling",
			rules:   []edition.CommandRule{readOnly},
			subject: subject{primary: "calendar/event/delete", safety: declared("destructive", "high")},
			reason:  ReasonRiskTooHigh, rule: "readonly",
		},
		{
			name:    "risk above ceiling",
			rules:   []edition.CommandRule{{Name: "r", MaxRisk: "medium"}},
			subject: subject{primary: "a/b", safety: declared("read", "high")},
			reason:  ReasonRiskTooHigh, rule: "r",
		},
		{
			name:    "path outside allow",
			rules:   []edition.CommandRule{readOnly},
			subject: subject{primary: "contact/user/get", safety: declared("read", "low")},
			reason:  ReasonNotAllowed, rule: "readonly",
		},
		{
			name:    "empty allow applies no path restriction",
			rules:   []edition.CommandRule{{Name: "any-read", MaxEffect: "read"}},
			subject: subject{primary: "contact/user/get", safety: declared("read", "low")},
			allowed: true, rule: "any-read",
		},
		{
			name:    "deny wins over allow inside one rule",
			rules:   []edition.CommandRule{{Name: "a", Allow: []string{"calendar/**"}, Deny: []string{"calendar/event/delete"}}},
			subject: subject{primary: "calendar/event/delete", safety: declared("destructive", "high")},
			reason:  ReasonDenylisted, rule: "a",
		},
		{
			name: "deny is local: another rule may allow",
			rules: []edition.CommandRule{
				{Name: "a", Allow: []string{"calendar/**"}, Deny: []string{"calendar/event/delete"}},
				{Name: "b", Allow: []string{"calendar/**"}},
			},
			subject: subject{primary: "calendar/event/delete", safety: declared("destructive", "high")},
			allowed: true, rule: "b",
		},
		{
			name: "several rules all deny",
			rules: []edition.CommandRule{
				{Name: "a", Allow: []string{"calendar/**"}},
				{Name: "b", Allow: []string{"contact/**"}},
			},
			subject: subject{primary: "mail/send", safety: declared("write", "medium")},
			reason:  ReasonNoMatchingRule,
		},
		{
			name:    "deny matches an alias entry of the identity",
			rules:   []edition.CommandRule{{Name: "a", Allow: []string{"report/**"}, Deny: []string{"report/create"}, AllowUnannotated: true}},
			subject: subject{primary: "report/entry/submit", entries: []string{"report/create", "report/entry/submit"}, safety: declared("write", "medium")},
			reason:  ReasonDenylisted, rule: "a",
		},
		{
			name:    "allow matches the primary path only",
			rules:   []edition.CommandRule{{Name: "a", Allow: []string{"report/create"}}},
			subject: subject{primary: "report/entry/submit", entries: []string{"report/create", "report/entry/submit"}, safety: declared("write", "medium")},
			reason:  ReasonNotAllowed, rule: "a",
		},
		{
			name:    "unannotated denied by default",
			rules:   []edition.CommandRule{{Name: "a", Allow: []string{"**"}}},
			subject: subject{primary: "doctor"},
			reason:  ReasonNotAnnotated, rule: "a",
		},
		{
			name:    "missing risk alone is unannotated",
			rules:   []edition.CommandRule{{Name: "a"}},
			subject: subject{primary: "x", safety: declared("read", "")},
			reason:  ReasonNotAnnotated, rule: "a",
		},
		{
			name:    "allow unannotated admits a missing declaration and skips the ceiling for it",
			rules:   []edition.CommandRule{{Name: "a", Allow: []string{"auth/status"}, MaxEffect: "read", AllowUnannotated: true}},
			subject: subject{primary: "auth/status"},
			allowed: true, rule: "a",
		},
		{
			name:    "allow unannotated still applies the ceiling to the declared dimension",
			rules:   []edition.CommandRule{{Name: "a", MaxEffect: "read", AllowUnannotated: true}},
			subject: subject{primary: "x", safety: declared("write", "")},
			reason:  ReasonRiskTooHigh, rule: "a",
		},
		{
			name:    "invalid safety is never admitted",
			rules:   []edition.CommandRule{{Name: "a", AllowUnannotated: true}},
			subject: subject{primary: "x", safety: safetyFacts{invalid: "declared risk \"hgih\" is not one of low, medium, high"}},
			reason:  ReasonRiskInvalid,
		},
		{
			name:    "active policy without rules denies",
			subject: subject{primary: "calendar/event/list", safety: declared("read", "low")},
			reason:  ReasonNoMatchingRule,
		},
		{
			name:    "discovery entry needs no rule",
			subject: subject{primary: "schema"},
			allowed: true,
		},
		{
			name:    "channel entry is not opened by a glob, a ceiling, or allow unannotated",
			rules:   []edition.CommandRule{{Name: "a", Allow: []string{"**"}, MaxRisk: "high", AllowUnannotated: true}},
			subject: subject{primary: "mcp/published/invoke", safety: declared("write", "high")},
			reason:  ReasonChannelNotAllowed,
		},
		{
			name:    "unannotated channel entry is not opened by allow unannotated",
			rules:   []edition.CommandRule{{Name: "a", AllowUnannotated: true}},
			subject: subject{primary: "api"},
			reason:  ReasonChannelNotAllowed,
		},
		{
			name:    "named channel entry passing every other condition is opened",
			rules:   []edition.CommandRule{{Name: "a", AllowChannels: []string{"api"}, AllowUnannotated: true}},
			subject: subject{primary: "api"},
			allowed: true, rule: "a",
		},
		{
			// Naming is an extra AND condition, never a substitute: the rule
			// is scoped to calendar, so it cannot open api.
			name:    "named channel entry must still match allow",
			rules:   []edition.CommandRule{{Name: "a", Allow: []string{"calendar/**"}, MaxEffect: "read", AllowChannels: []string{"api"}, AllowUnannotated: true}},
			subject: subject{primary: "api"},
			reason:  ReasonNotAllowed, rule: "a",
		},
		{
			name:    "named unannotated channel entry still needs allow unannotated",
			rules:   []edition.CommandRule{{Name: "a", MaxEffect: "read", AllowChannels: []string{"api"}}},
			subject: subject{primary: "api"},
			reason:  ReasonNotAnnotated, rule: "a",
		},
		{
			name:    "named channel entry inside allow is opened",
			rules:   []edition.CommandRule{{Name: "a", Allow: []string{"mcp/**"}, AllowChannels: []string{"mcp/published/invoke"}}},
			subject: subject{primary: "mcp/published/invoke", safety: declared("write", "high")},
			allowed: true, rule: "a",
		},
		{
			name:    "deny matches a native alias spelling of the identity",
			rules:   []edition.CommandRule{{Name: "a", Allow: []string{"report/**"}, Deny: []string{"log/**"}}},
			subject: subject{primary: "report/entry/submit", entries: []string{"log/entry/submit", "report/entry/submit"}, safety: declared("write", "medium")},
			reason:  ReasonDenylisted, rule: "a",
		},
		{
			name:    "named channel entry still honours the ceiling",
			rules:   []edition.CommandRule{{Name: "a", MaxRisk: "medium", AllowChannels: []string{"mcp/published/invoke"}}},
			subject: subject{primary: "mcp/published/invoke", safety: declared("write", "high")},
			reason:  ReasonRiskTooHigh, rule: "a",
		},
		{
			name:    "named channel entry still honours deny",
			rules:   []edition.CommandRule{{Name: "a", Deny: []string{"api"}, AllowChannels: []string{"api"}}},
			subject: subject{primary: "api"},
			reason:  ReasonDenylisted, rule: "a",
		},
		{
			name:    "invalid policy denies business commands",
			state:   StateInvalid,
			subject: subject{primary: "calendar/event/list", safety: declared("read", "low")},
			reason:  ReasonPolicyInvalid,
		},
		{
			name:    "invalid policy denies schema",
			state:   StateInvalid,
			subject: subject{primary: "schema"},
			reason:  ReasonPolicyInvalid,
		},
		{
			name:    "invalid policy keeps the diagnostic command",
			state:   StateInvalid,
			subject: subject{primary: PolicyShowPath},
			allowed: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := tt.state
			if state == "" {
				state = StateActive
			}
			got := evaluate(state, tt.rules, tt.subject).decision
			if got.Allowed != tt.allowed {
				t.Fatalf("Allowed = %v, want %v (%+v)", got.Allowed, tt.allowed, got)
			}
			if got.ReasonCode != tt.reason {
				t.Fatalf("ReasonCode = %q, want %q (%s)", got.ReasonCode, tt.reason, got.Reason)
			}
			if got.RuleName != tt.rule {
				t.Fatalf("RuleName = %q, want %q", got.RuleName, tt.rule)
			}
		})
	}
}

func TestCrossPlatformCoverageEvaluateReportsShadowedDeny(t *testing.T) {
	rules := []edition.CommandRule{
		{Name: "a", Allow: []string{"calendar/**"}, Deny: []string{"calendar/event/delete"}},
		{Name: "b", Allow: []string{"calendar/**"}},
	}
	result := evaluate(StateActive, rules, subject{primary: "calendar/event/delete", safety: declared("destructive", "high")})
	if !result.decision.Allowed || len(result.deniedBy) != 1 || result.deniedBy[0] != "a" {
		t.Fatalf("evaluation = %+v, want allowed with deniedBy [a]", result)
	}
}

func TestCrossPlatformCoverageEvaluateDenyMessage(t *testing.T) {
	agenda := subject{primary: "calendar/+agenda", safety: declared("read", "low")}
	denyAgenda := edition.CommandRule{Name: "deny-agenda", Deny: []string{"calendar/+agenda"}, DenyMessage: "  请联系 IT 开通日程查询  "}
	authOnly := edition.CommandRule{Name: "auth-basic", Allow: []string{"auth/login"}, DenyMessage: "auth 规则的指引"}
	denyAgain := edition.CommandRule{Name: "deny-again", Deny: []string{"calendar/**"}, DenyMessage: "请联系 IT 开通日程查询"}
	denyOther := edition.CommandRule{Name: "deny-other", Deny: []string{"calendar/+agenda"}, DenyMessage: "工单入口：it.example.com"}

	tests := []struct {
		name  string
		rules []edition.CommandRule
		s     subject
		want  string
	}{
		{"single rule deny, trimmed", []edition.CommandRule{denyAgenda}, agenda, "请联系 IT 开通日程查询"},
		{"single rule allow miss", []edition.CommandRule{authOnly}, agenda, "auth 规则的指引"},
		{"several rules: only the Deny match speaks", []edition.CommandRule{authOnly, denyAgenda}, agenda, "请联系 IT 开通日程查询"},
		{"several rules: none Deny-matched", []edition.CommandRule{authOnly, {Name: "other", Allow: []string{"im/**"}, DenyMessage: "x"}}, agenda, ""},
		{"several Deny matches are de-duplicated", []edition.CommandRule{denyAgenda, denyAgain}, agenda, "请联系 IT 开通日程查询"},
		{"several Deny matches keep rule order", []edition.CommandRule{denyAgenda, denyOther}, agenda, "请联系 IT 开通日程查询 工单入口：it.example.com"},
		{"allowed command carries no message", []edition.CommandRule{denyAgenda}, subject{primary: "calendar/event/list", safety: declared("read", "low")}, ""},
		{"unnamed channel carries no message", []edition.CommandRule{denyAgenda}, subject{primary: "api"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := evaluate(StateActive, tt.rules, tt.s).decision
			if got.DenyMessage != tt.want {
				t.Fatalf("DenyMessage = %q, want %q (decision %+v)", got.DenyMessage, tt.want, got)
			}
		})
	}
}

func TestCrossPlatformCoverageDenialErrorCarriesDenyMessage(t *testing.T) {
	base := Decision{ReasonCode: ReasonDenylisted, Reason: "r", RuleName: "deny-agenda"}
	var plain *apperrors.Error
	if !errors.As(DenialError("calendar/+agenda", base, StateActive), &plain) {
		t.Fatal("DenialError is not a classified error")
	}
	if _, ok := plain.Details["deny_message"]; ok {
		t.Fatalf("details = %+v, want no deny_message without a configured message", plain.Details)
	}

	base.DenyMessage = "请联系 IT 开通日程查询"
	var custom *apperrors.Error
	if !errors.As(DenialError("calendar/+agenda", base, StateActive), &custom) {
		t.Fatal("DenialError is not a classified error")
	}
	if custom.Details["deny_message"] != base.DenyMessage {
		t.Fatalf("details = %+v, want deny_message", custom.Details)
	}
	if custom.Hint != plain.Hint+". "+base.DenyMessage {
		t.Fatalf("hint = %q, want the standard hint followed by the deny message", custom.Hint)
	}
	if custom.Message != plain.Message || custom.Reason != plain.Reason {
		t.Fatalf("message/reason changed: %q/%q vs %q/%q", custom.Message, custom.Reason, plain.Message, plain.Reason)
	}
}

func TestCrossPlatformCoverageValidateRules(t *testing.T) {
	valid := []edition.CommandRule{
		{Name: "a", Allow: []string{"calendar/**", "im/+messages-send"}, Deny: []string{"**/delete"}, MaxEffect: "write", MaxRisk: "high", AllowChannels: []string{"api"}},
		{Name: "b"},
	}
	if err := ValidateRules(valid); err != nil {
		t.Fatalf("ValidateRules(valid) = %v", err)
	}
	if err := ValidateRules(nil); err != nil {
		t.Fatalf("ValidateRules(nil) = %v", err)
	}
	invalid := map[string][]edition.CommandRule{
		"no name":                 {{}},
		"declared more than once": {{Name: "a"}, {Name: "a"}},
		"max effect":              {{Name: "a", MaxEffect: "high-risk-write"}},
		"max risk":                {{Name: "a", MaxRisk: "read"}},
		"not a valid glob":        {{Name: "a", Allow: []string{"calendar/[a"}}},
		"empty segment":           {{Name: "a", Deny: []string{"calendar//list"}}},
		"has an empty segment":    {{Name: "a", Allow: []string{"/calendar"}}},
		"lower case":              {{Name: "a", Allow: []string{"Calendar/**"}}},
		"\"dws\" root":            {{Name: "a", Allow: []string{"dws/calendar/**"}}},
		"whitespace":              {{Name: "a", Allow: []string{"calendar event"}}},
		"path is empty":           {{Name: "a", Allow: []string{""}}},
		"exact command path":      {{Name: "a", AllowChannels: []string{"plugin/**"}}},
		"reviewed channel entry":  {{Name: "a", AllowChannels: []string{"calendar/event/list"}}},
		"control character":       {{Name: "a", DenyMessage: "line one\nline two"}},
		"longer than 500":         {{Name: "a", DenyMessage: strings.Repeat("字", 501)}},
	}
	if err := ValidateRules([]edition.CommandRule{{Name: "a", DenyMessage: strings.Repeat("字", 500)}}); err != nil {
		t.Fatalf("ValidateRules(500-character deny message) = %v", err)
	}
	for want, rules := range invalid {
		err := ValidateRules(rules)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("ValidateRules(%+v) = %v, want error containing %q", rules, err, want)
		}
	}
}

func TestCrossPlatformCoverageResolveStates(t *testing.T) {
	if got := Resolve(nil); got != nil {
		t.Fatalf("Resolve(nil) = %+v, want nil", got)
	}
	if got := Resolve(&edition.Hooks{Name: "open"}); got != nil || got.State() != StateDisabled || got.Enabled() {
		t.Fatalf("Resolve(no hook) = %+v, want disabled nil snapshot", got)
	}
	empty := Resolve(&edition.Hooks{Name: "acme", Restrict: func() []edition.CommandRule { return nil }})
	if empty.State() != StateActive || !empty.Enabled() || empty.Source() != "acme" {
		t.Fatalf("empty rule set = %+v, want active", empty)
	}
	invalid := Resolve(&edition.Hooks{Restrict: func() []edition.CommandRule { return []edition.CommandRule{{}} }})
	if invalid.State() != StateInvalid || invalid.InvalidReason() == "" {
		t.Fatalf("invalid rule set = %+v, want invalid with a reason", invalid)
	}
	panicked := Resolve(&edition.Hooks{Restrict: func() []edition.CommandRule { panic("boom") }})
	if panicked.State() != StateInvalid || !strings.Contains(panicked.InvalidReason(), "boom") {
		t.Fatalf("panicking hook = %+v, want invalid", panicked)
	}
	calls := 0
	Resolve(&edition.Hooks{Restrict: func() []edition.CommandRule { calls++; return nil }})
	if calls != 1 {
		t.Fatalf("Restrict called %d times, want exactly 1", calls)
	}
}

func TestCrossPlatformCoverageSnapshotRulesAreCopies(t *testing.T) {
	source := []edition.CommandRule{{Name: "a", Allow: []string{"calendar/**"}}}
	snapshot := Resolve(&edition.Hooks{Restrict: func() []edition.CommandRule { return source }})
	source[0].Allow[0] = "**"
	got := snapshot.Rules()
	got[0].Allow[0] = "mutated"
	if again := snapshot.Rules(); again[0].Allow[0] != "calendar/**" {
		t.Fatalf("snapshot rule mutated through a caller slice: %q", again[0].Allow[0])
	}
}

func TestCrossPlatformCoverageLegacyRiskAdapter(t *testing.T) {
	// Must stay identical to internal/cli applyContractRiskToSafety.
	tests := []struct{ legacy, effect, risk string }{
		{"read", "read", "low"},
		{"write", "write", "medium"},
		{"high-risk-write", "destructive", "high"},
		{" write ", "write", "medium"},
	}
	for _, tt := range tests {
		effect, risk, known := legacyRiskFacts(tt.legacy)
		if !known || effect != tt.effect || risk != tt.risk {
			t.Errorf("legacyRiskFacts(%q) = %q/%q/%v, want %q/%q", tt.legacy, effect, risk, known, tt.effect, tt.risk)
		}
	}
	for _, unknown := range []string{"", "low", "medium", "high", "destructive"} {
		if _, _, known := legacyRiskFacts(unknown); known {
			t.Errorf("legacyRiskFacts(%q) is known; legacy values must not be read as the new taxonomy", unknown)
		}
	}
	if got := validatedSafety(safetyFacts{effect: "read", risk: "green"}); got.invalid == "" {
		t.Fatal("an unknown risk value must be invalid, not missing")
	}
}

func TestCrossPlatformCoveragePaths(t *testing.T) {
	if got := DeclaredPath("dws report  entry submit"); got != "report/entry/submit" {
		t.Fatalf("DeclaredPath = %q", got)
	}
	if got := DeclaredPath("report create"); got != "report/create" {
		t.Fatalf("DeclaredPath = %q", got)
	}
	if got := DisplayPath("report/entry/submit"); got != "dws report entry submit" {
		t.Fatalf("DisplayPath = %q", got)
	}
	if _, ok := matchAny([]string{"calendar/*"}, "calendar/event/list"); ok {
		t.Fatal("a single star must not cross a path segment")
	}
	if glob, ok := matchAny([]string{"x", "**/delete"}, "calendar/event/delete"); !ok || glob != "**/delete" {
		t.Fatalf("matchAny = %q, %v", glob, ok)
	}
}
