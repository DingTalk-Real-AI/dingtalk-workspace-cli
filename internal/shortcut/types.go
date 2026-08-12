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

// Package shortcut provides a declarative framework for high-fidelity CLI
// commands (shortcuts) on top of the DingTalk MCP runtime.
//
// A Shortcut is a curated, thin wrapper over a raw MCP tool call: it declares
// its flags, validation and execution logic once, and the framework compiles it
// into a cobra command wired to the shared executor.Runner, output formatting,
// dry-run and identity handling. This provides a `+command`
// shortcut layer, but executes through DWS's MCP dispatch instead of a native SDK.
//
// Shortcuts are surfaced as `dws <service> +<command>` (e.g. `dws contact
// +search-user`). The `+` prefix keeps them visually distinct from the
// dynamically-discovered MCP leaf commands and from hand-written helper commands.
package shortcut

import (
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
)

// Risk classifies the side effect of running a shortcut. It drives whether a
// confirmation prompt is required before execution (see internal/safety).
type Risk string

const (
	// RiskRead is a read-only operation; never prompts.
	RiskRead Risk = "read"
	// RiskWrite mutates state; may prompt unless --yes is set.
	RiskWrite Risk = "write"
	// RiskHighWrite is a destructive/irreversible operation; requires explicit
	// confirmation (or --yes) before execution.
	RiskHighWrite Risk = "high-risk-write"
)

// SemanticDisposition records the reviewed relationship between a Shortcut and
// the underlying Runtime Schema surface.
type SemanticDisposition string

const (
	DispositionPrimarySmart    SemanticDisposition = "primary_smart"
	DispositionSemanticAdapter SemanticDisposition = "semantic_adapter"
	DispositionSchemaLeaf      SemanticDisposition = "schema_leaf"
	DispositionAliasInternal   SemanticDisposition = "alias_internal"
)

// Availability is independent from live-account evidence. A missing fixture or
// permission does not make an implemented command unavailable.
type Availability string

const (
	AvailabilityAvailable   Availability = "available"
	AvailabilityUnavailable Availability = "unavailable"
	AvailabilityDeprecated  Availability = "deprecated"
)

// FlagType is the pflag value type a Flag registers as.
type FlagType string

const (
	FlagString      FlagType = "string"
	FlagBool        FlagType = "bool"
	FlagInt         FlagType = "int"
	FlagStringSlice FlagType = "string_slice"
)

// Flag is a declarative CLI flag definition. The runner registers each Flag onto
// the generated cobra command, applying defaults, and validates Enum/Required
// before the Execute hook runs.
type Flag struct {
	// Name is the long flag name (kebab-case), e.g. "user-ids".
	Name string `json:"name"`
	// Shorthand is the optional one-character CLI spelling, e.g. "o" for --output.
	Shorthand string `json:"shorthand,omitempty"`
	// Type is the value type; defaults to FlagString when empty.
	Type FlagType `json:"type"`
	// Default is the default value rendered as a string.
	Default string `json:"default"`
	// Desc is the help text shown in --help.
	Desc string `json:"description"`
	// Required, when true, makes the framework error if the flag is not set.
	Required bool `json:"required"`
	// RequiredWhen publishes a conditional-required rule that Required cannot
	// express, e.g. "identity=bot" for a credential only mandatory under one
	// identity. It is published Schema metadata so an Agent can predict the
	// failure; enforcement stays in Validate, which remains authoritative.
	RequiredWhen string `json:"required_when,omitempty"`
	// Enum, when non-empty, restricts the accepted values (string flags only).
	Enum []string `json:"enum"`
	// Hidden hides the flag from --help while keeping it usable.
	Hidden bool `json:"-"`
	// Aliases are hidden executable flag spellings for compatibility. They do
	// not create additional Schema parameters; validation and value fallback
	// remain attached to the canonical Name. AliasesVisible is a narrow
	// compatibility escape hatch for aliases that were historically public.
	Aliases        []string `json:"-"`
	AliasesVisible bool     `json:"-"`
}

// ConstraintKind is a machine-readable cross-parameter or custom validation
// rule published by `dws shortcut list` and rendered in leaf help.
type ConstraintKind string

const (
	// ConstraintAtLeastOne requires one or more of Flags.
	ConstraintAtLeastOne ConstraintKind = "at_least_one"
	// ConstraintExactlyOne requires exactly one of Flags.
	ConstraintExactlyOne ConstraintKind = "exactly_one"
	// ConstraintMutuallyExclusive permits zero or one of Flags.
	ConstraintMutuallyExclusive ConstraintKind = "mutually_exclusive"
	// ConstraintCustom documents validation enforced by Shortcut.Validate.
	ConstraintCustom ConstraintKind = "custom"
)

// Constraint declares a shortcut parameter relationship. Known kinds are
// enforced by the runner; custom constraints are enforced by Validate and must
// include a concrete Description.
type Constraint struct {
	Kind        ConstraintKind `json:"kind"`
	Flags       []string       `json:"flags"`
	Description string         `json:"description,omitempty"`
}

// Shortcut is the declarative definition of a single high-fidelity command.
//
// The zero-value is not usable; Service, Command and Execute are required.
// The framework injects the global --format/--dry-run/--jq/--yes flags from the
// root command, so shortcuts must not redeclare them.
type Shortcut struct {
	// OutputRollout selects the single active output contract for this exact
	// command in the current release. It is internal release metadata, never a
	// user/Agent flag.
	OutputRollout output.RolloutState
	// Service is the top-level command group, e.g. "contact". Multiple
	// shortcuts sharing a Service are mounted under the same parent command.
	Service string
	// Command is the leaf name including its "+" prefix, e.g. "+search-user".
	Command string
	// Aliases are hidden, compatibility-only Cobra spellings for the same
	// command identity. Agent-facing Skill and Schema examples must continue to
	// use Command; reviewed Schema aliases are declared separately in the
	// CommandRegistry.
	Aliases []string
	// SinglePositionalAliasFor optionally treats one positional argument as the
	// value of the named string flag. It is intended only for unambiguous search
	// compatibility such as `+chat-search "群名"`; it never changes the
	// canonical flag-based contract published to Agents.
	SinglePositionalAliasFor string
	// Product is the canonical MCP product id used to build the invocation.
	// Defaults to Service when empty.
	Product string
	// Description is the one-line help shown in --help.
	Description string
	// Intent is a fuller natural-language description of what the command does
	// and WHEN to reach for it. Unlike the terse Description, it is written for
	// human discovery and AI-agent intent matching (e.g. "当你只知道某人姓名、需要
	// 拿到其 userId 以便后续发消息或指派任务时使用"). Surfaced in `--help` (as the
	// long description) and in `dws shortcut list`.
	Intent string
	// Risk classifies the side effect; defaults to RiskRead when empty.
	// Kept as the runtime confirmation source when Safety is empty.
	Risk Risk
	// Safety is an optional explicit Schema/runtime safety declaration. When
	// non-empty it overrides Risk expansion in FromShortcut; otherwise Risk
	// still drives ConfirmSafety so existing Execute bodies stay unchanged.
	Safety contract.SafetySpec
	// Contract is the final Agent Contract overlay (selection/interface/dry-run).
	// Empty fails Catalog assembly; every Shortcut must declare Contract.
	Contract corecmd.ContractDecl
	// Flags are the command-specific flags. Global flags are injected separately.
	Flags []Flag
	// Constraints publish and enforce relationships that individual flags cannot
	// express, such as "exactly one of --group and --user". Custom constraints
	// describe checks implemented by Validate.
	Constraints []Constraint
	// Tips are optional usage examples appended to --help.
	Tips []string
	// Hidden hides the command from listings while keeping it invocable.
	Hidden bool
	// Disposition is the reviewed semantic relation to the Runtime Schema leaf
	// surface. It determines default Agent discovery independently from live
	// fixture evidence.
	Disposition SemanticDisposition
	// SemanticDelta explains the concrete value added beyond renaming a leaf.
	SemanticDelta string
	// Availability describes whether the implementation is shipped and
	// callable; it does not encode the current account's permissions.
	Availability Availability
	// PrimaryCommand links aliases/internal compatibility entries to their
	// preferred semantic entry.
	PrimaryCommand string
	// SemanticReviewed is true only when the disposition and visibility came
	// from the reviewed semantic catalog.
	SemanticReviewed bool
	// UserDefined identifies shortcuts loaded from the user's config
	// directory. Distribution-owned Schema and interface snapshots exclude
	// these runtime extensions even if another root loaded them earlier.
	UserDefined bool

	// Validate optionally checks resolved flag values before execution. Return a
	// non-nil error to abort with a validation message. Runs after built-in
	// Required/Enum checks.
	Validate func(rt *RuntimeContext) error
	// Execute performs the shortcut. It is required. Typically it builds a
	// params map from rt's flags, calls rt.CallMCP, and rt.Output's the result.
	Execute func(rt *RuntimeContext) error
}

// product returns the canonical MCP product id for building invocations.
func (s Shortcut) product() string {
	if s.Product != "" {
		return s.Product
	}
	return s.Service
}

// risk returns the effective risk, defaulting to read.
func (s Shortcut) risk() Risk {
	if s.Risk == "" {
		return RiskRead
	}
	return s.Risk
}
