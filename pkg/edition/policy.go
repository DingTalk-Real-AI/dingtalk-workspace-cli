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

package edition

// CommandRule is one compiled-in command restriction rule contributed through
// Hooks.Restrict. Rules are an Agent guardrail: they bound which commands the
// binary that carries them will execute. They do not protect against a user
// replacing the binary; credential custody and server-side permissions own
// that boundary.
//
// Paths are slash-separated command paths without the "dws" root, lower case,
// for example "calendar/event/list". Allow and Deny are doublestar globs over
// those paths ("calendar/**", "**/delete").
//
// Within one rule every dimension must hold (AND) and Deny wins over Allow.
// Across rules a command is allowed when at least one rule allows it (OR): a
// Deny is scoped to its own rule and cannot veto another rule's Allow. To
// forbid a command globally, make every rule deny it or leave it un-allowed.
type CommandRule struct {
	// Name identifies the rule in diagnostics and denial errors. Required and
	// unique within the rule set.
	Name string
	// Description is display-only.
	Description string
	// DenyMessage is optional guidance appended to the denial error when this
	// rule is the reason a command was refused, for example who to contact to
	// get access. It is one line of plain text, at most 500 characters. It is
	// added to the error hint and details; the stable message, reason code,
	// and exit code are unchanged.
	//
	// It is used when this is the only rule that denied the command. When
	// several rules each denied it, only the messages of rules whose Deny
	// matched the command are used.
	DenyMessage string

	// Allow restricts the rule to matching command paths. Empty means the rule
	// applies no path restriction.
	Allow []string
	// Deny rejects matching command paths for this rule only.
	Deny []string

	// MaxEffect is the highest allowed declared effect, inclusive:
	// read < write < destructive. Empty means no effect limit.
	MaxEffect string
	// MaxRisk is the highest allowed declared risk, inclusive:
	// low < medium < high. Empty means no risk limit.
	MaxRisk string

	// AllowUnannotated lets this rule pass commands whose effect or risk
	// declaration is missing. It never admits an unknown (misspelled) value,
	// and a missing value is never treated as the lowest level.
	AllowUnannotated bool

	// AllowChannels names channel entries (generic invocation, plugin
	// management, and self-update commands) this rule opens, as exact
	// command paths. Channel entries are never matched by Allow globs and are
	// not opened by AllowUnannotated.
	AllowChannels []string
}

// Effect and risk ceilings accepted by CommandRule.MaxEffect / MaxRisk.
const (
	EffectRead        = "read"
	EffectWrite       = "write"
	EffectDestructive = "destructive"

	RiskLow    = "low"
	RiskMedium = "medium"
	RiskHigh   = "high"
)
