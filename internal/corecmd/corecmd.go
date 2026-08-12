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

// Package corecmd is the shared, dispatch-agnostic base for building leaf
// commands. It concentrates flag registration, the alias/env/default effective
// value fallback chain, required validation, cross-flag constraint declaration
// checks + runtime enforcement, SafetySpec-driven confirmation, toolArgs
// assembly, and Agent Runtime Schema projection.
//
// Declaration vs execution (framework rule):
//
//   - Declare = Spec data fields (Flags, Constraints, Safety,
//     ConstParams, Use/Short/Long/Example). New registers, validates,
//     confirms, and embeds those facts into dws.schema.*.
//   - Execute = Validate / Invoke / Orchestrate / RunE / PostMount. Hooks
//     consume assembled args; they must not invent the CLI surface.
//   - Annotate = explicit cobra annotations when a fact is not (yet) a Contract
//     field (e.g. a cross-flag constraint or parameter metadata fact).
//     Inference-only Schema/help is forbidden.
//   - Selection / product routing prose is declared on ContractDecl /
//     ProductDecl (delivered as contract_final). schema_hints/ is retired.
//     Identity is collected from ContractFinal.Identity on the live leaves.
//     Interface / dry-run reviewed sources must not create CLI flags.
//
// Full ToolSpec field authority: RFC §5.0.4 / homology §1.4.
//
// It is deliberately dispatch-agnostic: it never calls an MCP tool. LeafSpec
// (internal/helpers) and Shortcut (internal/shortcut) wrap these primitives and
// supply dispatch. Invoke / Orchestrate / Ctx are the #830 transitional
// dispatch API still used in production; the RFC target is mcpbind + Handler
// (do not treat removal of Invoke/Orchestrate as already landed).
//
// Behavioral contract: flag registration, value fallback, required/constraint
// semantics, confirmation behavior, and schema projection stay shared across
// Leaf and Shortcut. Evidence is split: check-generated-drift proves build-time
// projection; runtime pipeline order is covered by this package's tests plus
// leaf/risk/constraint unit tests.
package corecmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/runtimeannotate"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/cmdutil"
)

// FlagKind is the value type of a flag.
type FlagKind int

const (
	// KindString is a string flag (default).
	KindString FlagKind = iota
	// KindInt is an integer flag (registered as cobra Int); it enters toolArgs
	// only when the value is non-zero, matching the handwritten "putInt only
	// when non-zero" semantics (e.g. devapp app-group-id).
	KindInt
	// KindBool is a boolean flag (registered as cobra Bool); it enters toolArgs
	// only when the user explicitly provided it (Changed), matching the
	// handwritten "transmit on Changed, explicit false is still sent" semantics.
	// Booleans do not participate in the alias/env fallback chain.
	KindBool
	// KindStringSlice is a string-list flag (registered as cobra StringSlice);
	// it enters toolArgs only when a non-empty element exists, elements are
	// always TrimSpace'd and empties dropped.
	KindStringSlice
)

// FlagValidationMode selects runtime validation semantics for a flag.
//
// The zero value keeps LeafSpec's fallback-aware effective-value semantics.
// ValidationShortcut preserves Shortcut's declaration-order checks: Required
// means the user must explicitly provide the flag token (even with a default),
// followed immediately by that flag's Enum validation.
type FlagValidationMode string

// ValidationShortcut is the declaration-order explicit-token mode described on
// FlagValidationMode.
const ValidationShortcut FlagValidationMode = "shortcut"

// FlagSpec declares how a flag is registered and bound into MCP toolArgs. Its
// fields intentionally mirror the former helpers.LeafFlag one-for-one so that
// helpers can alias to it without touching any call site.
type FlagSpec struct {
	Name      string   // flag name (kebab-case)
	Shorthand string   // optional one-character Cobra shorthand
	Usage     string   // registration usage text
	Kind      FlagKind // value type, defaults to KindString
	Default   string   // registration default for every Kind; also the fallback-chain tail when aliases/env are empty
	Hidden    bool     // hide the real flag from help/Schema while keeping it invocable

	// Required, when true, validates a non-empty effective value in RunE. Plain
	// Required flags aggregate into a cmdutil.ValidateRequiredFlags-compatible
	// error; when EnvVar is configured the env var is a fallback and, still
	// empty, RequiredHint (or a default hint) is reported.
	Required       bool
	ValidationMode FlagValidationMode
	RequiredError  string // exact missing-token error for ValidationShortcut
	RequiredHint   string
	// MarkRequired, when true, calls cobra MarkFlagRequired (the hard floor for
	// the catalog required projection); cobra errors before RunE. It cannot be
	// combined with Aliases (RegisterFlags panics on that declaration).
	MarkRequired bool

	Aliases []string // hidden aliases, registered with the main flag's Kind; used in order when the main flag is not explicitly provided
	EnvVar  string   // environment variable consulted when the effective value is empty (an integer flag's env value must be parseable)
	// ArgDefault covers the case where the registration default is empty but
	// toolArgs still needs a fallback. For KindString it is used when the
	// effective value is empty. For KindInt it is also the floor: when the
	// resolved integer is < 1, ArgDefault is emitted instead (cursor page-size
	// semantics).
	ArgDefault string
	// Bind is the toolArgs key; empty uses Name.
	Bind string
	// Transform converts a string effective value into the arg value; nil sends
	// it as-is. Returning (nil, nil) skips the key (for "nullable numeric: skip
	// on empty or parse failure" semantics).
	Transform func(raw string) (any, error)
	// OmitEmpty, when true, drops an empty effective value from toolArgs (KindInt
	// is always "non-zero only" and ignores this field).
	OmitEmpty bool
	// Trim, when true, TrimSpace's the effective value (main flag/alias/env
	// alike) and makes a whitespace-only value count as empty in required checks.
	Trim bool

	// Schema parameter final facts (embedded to dws.schema.*; assembly pass-through).
	Enum              []string // accepted values
	Format            string   // machine-readable format (e.g. uri)
	Example           string   // representative CLI value
	RequiredWhen      string   // conditional required expression (descriptive)
	SchemaDescription string   // Schema description; empty uses Usage
}

// ConstraintKind is the type of a cross-flag relationship constraint. Values
// match the shortcut framework's ConstraintKind verbatim.
type ConstraintKind string

const (
	// AtLeastOne requires at least one of Flags to be provided.
	AtLeastOne ConstraintKind = "at_least_one"
	// ExactlyOne requires exactly one of Flags to be provided.
	ExactlyOne ConstraintKind = "exactly_one"
	// MutuallyExclusive allows at most one of Flags.
	MutuallyExclusive ConstraintKind = "mutually_exclusive"
	// Custom documents validation implemented by Spec.Validate. command
	// validates the declaration and renders its help, but does not infer the
	// command-specific runtime rule.
	Custom ConstraintKind = "custom"
)

// Constraint declares a relationship over a group of flags. It is enforced
// after required validation and before the framework's Validate hook;
// "provided" reuses the effective-value fallback chain (explicit main flag →
// alias → env), so passing a compatible alias counts as provided — a capability
// the shortcut framework's bare Changed check lacks. The constraint is also
// projected into the Agent Runtime Schema (mutually_exclusive / require_one_of)
// and rendered into the --help "参数约束" section.
type Constraint struct {
	Kind  ConstraintKind
	Flags []string
	// Description, when non-empty, replaces the constraint's default help text.
	Description string
}

// ParameterProjectionMode selects how declared flags are embedded into Runtime
// Schema annotations. The zero value makes the declaration the final parameter
// authority (the LeafSpec/command default).
type ParameterProjectionMode string

const (
	// ProjectCobraParameters preserves Cobra usage/type/default provenance and
	// annotates only facts Cobra cannot express: Required and Enum. Shortcut
	// uses this mode to converge its runtime without rewriting Catalog facts.
	ProjectCobraParameters ParameterProjectionMode = "cobra"
)

// Spec is the single typed definition of a leaf command, shared by the
// LeafSpec and (via FromShortcut) Shortcut frameworks.
//
// Declaration surface is the final Schema data source for managed leaves:
//
//	Flags (+ parameter Schema fields), Constraints, Safety, ConstParams,
//	Use/Short/Long/Example, Contract (ToolSpec groups)
//
// Schema assembly pass-throughs embedded dws.schema.* — no reviewed/hints
// parallel authority for declared fields. Safety uses contract.SafetySpec
// directly: confirmation drives the runtime gate, while effect/risk/idempotency
// are published unchanged. No safety field is inferred from another.
//
// Execution surface (hooks — not declaration):
//
//   - RunE — full escape hatch: the framework only registers flags/constraints/
//     help and hands control over.
//   - Invoke — #830 transitional single-step dispatch: runs after required/
//     constraint/Validate checks, args assembly and the Safety confirmation
//     gate, receiving the assembled toolArgs. Target: mcpbind Bind.
//   - Orchestrate — #830 transitional multi-step dispatch: same checks and
//     confirmation, receives only the Ctx. Target: Handler / orchestration.
//   - Validate / PostMount — orchestration only; must not register business flags
//     or assemble business params that belong in Flags/ConstParams.
//
// Exactly one of RunE / Invoke / Orchestrate must be set; New validates this at
// construction time. corecmd stays dispatch-agnostic and never calls a backend:
// the adapters (FromLeafSpec / FromShortcut) supply the body.
type Spec struct {
	Use           string
	Short         string
	Long          string
	Example       string
	Hidden        bool
	OutputRollout output.RolloutState

	Flags       []FlagSpec
	Constraints []Constraint
	// ParameterProjection controls whether parameter facts are final
	// declaration annotations or Cobra-backed compatibility facts.
	ParameterProjection ParameterProjectionMode
	// Safety is the command's single safety source. The same contract.SafetySpec
	// is used for runtime confirmation and the published Schema. A completely
	// empty value keeps the historical read-only default; a non-empty value
	// must declare effect/risk/confirmation/idempotency together.
	Safety contract.SafetySpec
	// ConfirmFirst runs the Safety confirmation before required/constraint/
	// Validate checks instead of after them. Use it where the legacy semantics
	// were guard-first (a write without --yes fails fast with
	// confirmation_required regardless of parameter completeness). The default
	// preserves the shortcut order (checks first, confirmation just before the
	// backend call).
	ConfirmFirst bool
	// ConstParams are fixed toolArgs merged after flag assembly (e.g. precheckOnly).
	// They are payload declaration, not user flags, and never satisfy Required.
	ConstParams map[string]any
	// Contract is the authoring-time leaf contract declaration (selection /
	// interface / parameters / dry-run / identity). When non-empty, embed
	// converts it once to ContractFinal for Catalog pass-through.
	Contract ContractDecl

	// Validate is the cross-flag validation hook, run after required/constraint
	// checks and before args assembly; nil skips it. Not a declaration surface.
	Validate func(cmd *cobra.Command, args []string) error
	// PostMount adjusts the built command after flag registration and before
	// RunE is set (Args/DisableAutoGenTag/annotate/…); always runs. Business
	// flags belong in Flags, not here.
	PostMount func(cmd *cobra.Command)
	// RunE fully replaces the generated body (escape hatch).
	RunE func(cmd *cobra.Command, args []string) error
	// Invoke executes a single-step command with the assembled toolArgs.
	Invoke func(c *Ctx, toolArgs map[string]any) error
	// ResultInvoke executes once and returns an immutable framework 2.0 result.
	ResultInvoke func(c *Ctx, toolArgs map[string]any) (output.CommandResult, error)
	// Orchestrate executes a multi-step command; it assembles whatever payloads
	// it needs from the Ctx.
	Orchestrate func(c *Ctx) error
}

// Ctx is the framework-neutral execution context handed to Invoke/Orchestrate.
// It deliberately knows nothing about MCP or any other backend: it exposes the
// command, its positional args, and typed flag access that reuses the declared
// alias → env → default fallback chain, so a consumer reading a flag through Ctx
// gets exactly the value the required/constraint checks saw.
type Ctx struct {
	cmd   *cobra.Command
	args  []string
	flags map[string]FlagSpec
}

// newCtx builds the execution context for one command invocation.
func newCtx(cmd *cobra.Command, args []string, flags []FlagSpec) *Ctx {
	byName := make(map[string]FlagSpec, len(flags))
	for _, flag := range flags {
		byName[flag.Name] = flag
	}
	return &Ctx{cmd: cmd, args: args, flags: byName}
}

// Command returns the running cobra command.
func (c *Ctx) Command() *cobra.Command { return c.cmd }

// Args returns the positional arguments.
func (c *Ctx) Args() []string { return c.args }

// Str returns a flag's effective string value (explicit → alias → env →
// default). An undeclared name yields "".
func (c *Ctx) Str(name string) string {
	flag, ok := c.flags[name]
	if !ok {
		return ""
	}
	return EffectiveValue(c.cmd, flag)
}

// Int returns a flag's effective integer value; an unparseable or undeclared
// value yields 0 (BuildArgs reports the precise parse error for Invoke specs).
func (c *Ctx) Int(name string) int {
	flag, ok := c.flags[name]
	if !ok {
		return 0
	}
	v, err := integerValue(c.cmd, flag)
	if err != nil {
		return 0
	}
	return int(v)
}

// Bool returns a boolean flag's value.
func (c *Ctx) Bool(name string) bool {
	v, _ := c.cmd.Flags().GetBool(name)
	return v
}

// StrSlice returns a list flag's effective elements (trimmed, empties dropped).
func (c *Ctx) StrSlice(name string) []string {
	flag, ok := c.flags[name]
	if !ok {
		return nil
	}
	return sliceValue(c.cmd, flag)
}

// Changed reports whether the user explicitly passed the flag.
func (c *Ctx) Changed(name string) bool { return c.cmd.Flags().Changed(name) }

// DryRun reports the effective global --dry-run.
func (c *Ctx) DryRun() bool { return BoolFlag(c.cmd, "dry-run") }

// Yes reports the effective global --yes.
func (c *Ctx) Yes() bool { return BoolFlag(c.cmd, "yes") }

// NewCommand builds a cobra command from a Spec. It is the single
// orchestration path: dispatch declaration check → flag registration →
// constraint declaration checks → Runtime Schema projection → constraint help →
// PostMount → generated RunE{ [ConfirmFirst: ConfirmSafety →]
// required → constraints → Validate → BuildArgs → ConfirmSafety →
// Invoke/Orchestrate }.
//
// Behavior matches the former helpers.NewLeafCommand, which always dispatched
// (Call → Server → callMCPTool) and therefore could not express a dispatcher-less
// spec. Here that is a programming error caught at construction time, so a
// malformed spec can never run the pipeline — write-confirmation prompt
// included — and then silently exit 0 having done nothing.
func New(spec Spec) *cobra.Command {
	validateDispatchDecl(spec)
	validateSafetySpec(spec)
	validateContractDecl(spec)
	// Help prose inherits the declaration when not authored separately:
	// Selection.Examples (already contract-validated against the real flags)
	// double as the --help Example block, keeping one authored source.
	example := spec.Example
	if strings.TrimSpace(example) == "" && len(spec.Contract.Selection.Examples) > 0 {
		example = "  " + strings.Join(spec.Contract.Selection.Examples, "\n  ")
	}
	cmd := &cobra.Command{
		Use:     spec.Use,
		Short:   spec.Short,
		Long:    spec.Long,
		Example: example,
		Hidden:  spec.Hidden,
	}
	RegisterFlags(cmd, spec.Flags)
	ValidateConstraintDecls(spec.Use, spec.Flags, spec.Constraints)
	embedContractIntoSchema(cmd, spec)
	AnnotateConstraints(cmd, spec.Constraints)
	if help := ConstraintHelp(spec.Constraints); help != "" {
		cmd.Long = strings.TrimRight(cmd.Long, "\n") + help
	}
	if spec.PostMount != nil {
		spec.PostMount(cmd)
	}
	if spec.OutputRollout != "" {
		output.SetCommandRollout(cmd, spec.OutputRollout)
	}
	if spec.ConfirmFirst {
		if cmd.Annotations == nil {
			cmd.Annotations = map[string]string{}
		}
		cmd.Annotations[ConfirmFirstAnnotation] = "true"
	}
	if spec.RunE != nil {
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			if err := runDeclaredPreflight(cmd, args, spec); err != nil {
				return err
			}
			if !spec.ConfirmFirst {
				if err := ConfirmSafety(cmd, spec.Safety); err != nil {
					return err
				}
			}
			return spec.RunE(cmd, args)
		}
		return cmd
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if err := runDeclaredPreflight(cmd, args, spec); err != nil {
			return err
		}
		ctx := newCtx(cmd, args, spec.Flags)
		if spec.Orchestrate != nil {
			if !spec.ConfirmFirst {
				if err := ConfirmSafety(cmd, spec.Safety); err != nil {
					return err
				}
			}
			return spec.Orchestrate(ctx)
		}
		toolArgs, err := BuildArgs(cmd, spec.Flags)
		if err != nil {
			return err
		}
		for key, value := range spec.ConstParams {
			toolArgs[key] = value
		}
		if !spec.ConfirmFirst {
			if err := ConfirmSafety(cmd, spec.Safety); err != nil {
				return err
			}
		}
		if spec.ResultInvoke != nil {
			if !output.UsesUnifiedResult(cmd) {
				return fmt.Errorf("command %q uses ResultInvoke without an active unified-result rollout", cmd.CommandPath())
			}
			result, err := spec.ResultInvoke(ctx, toolArgs)
			if err != nil {
				return err
			}
			return output.StoreResult(cmd.Context(), result)
		}
		return spec.Invoke(ctx, toolArgs)
	}
	return cmd
}

// ConfirmFirstAnnotation marks commands whose Spec declared ConfirmFirst. The
// delivery gate reads it to tell a declared guard-first command apart from an
// accidental confirm-before-validate inversion: guard-first is only legitimate
// when the declaration says so.
const ConfirmFirstAnnotation = "dws.contract.confirm_first"

// HasDeclaredConfirmFirst reports whether cmd was built from a Spec that
// declared ConfirmFirst.
func HasDeclaredConfirmFirst(cmd *cobra.Command) bool {
	return cmd != nil && cmd.Annotations != nil && cmd.Annotations[ConfirmFirstAnnotation] == "true"
}

// runDeclaredPreflight runs the checks the Spec itself declares, in the one
// order both dispatch paths share. ConfirmFirst is the declared opt-out for
// legacy guard-first commands: those confirm before parameter completeness is
// known. Keeping this in one function is deliberate — when the RunE escape
// hatch carried its own copy it silently dropped every declared check, so a
// spec could publish Required flags that nothing enforced.
func runDeclaredPreflight(cmd *cobra.Command, args []string, spec Spec) error {
	if spec.ConfirmFirst {
		if err := ConfirmSafety(cmd, spec.Safety); err != nil {
			return err
		}
	}
	if err := ValidateRequired(cmd, spec.Flags); err != nil {
		return err
	}
	if err := ValidateEnums(cmd, spec.Flags); err != nil {
		return err
	}
	if err := ValidateConstraints(cmd, spec.Flags, spec.Constraints); err != nil {
		return err
	}
	if spec.Validate != nil {
		return spec.Validate(cmd, args)
	}
	return nil
}

// validateDispatchDecl enforces "exactly one dispatcher" at build time. Like
// ValidateConstraintDecls this panics: a spec with no runnable body (or with two
// competing ones) is a programming error that every test and startup path should
// trip immediately, not a condition to surface at user run time.
func validateDispatchDecl(spec Spec) {
	declared := 0
	if spec.RunE != nil {
		declared++
	}
	if spec.Invoke != nil {
		declared++
	}
	if spec.ResultInvoke != nil {
		declared++
	}
	if spec.Orchestrate != nil {
		declared++
	}
	if declared != 1 {
		panic(fmt.Sprintf(
			"command %q must declare exactly one of RunE/Invoke/Orchestrate, got %d (ResultInvoke is also a dispatcher)",
			spec.Use, declared))
	}
	// ConfirmFirst only changes the ordering of a declared confirmation gate.
	if spec.ConfirmFirst && strings.TrimSpace(spec.Safety.Confirmation) != "user_required" {
		panic(fmt.Sprintf(
			"command %q sets ConfirmFirst but Safety.Confirmation is not user_required",
			spec.Use))
	}
}

// validateSafetySpec rejects partial safety declarations. A zero value remains
// the historical read-only default, but once any field is authored all four
// independent Schema dimensions must be explicit.
func validateSafetySpec(spec Spec) {
	safety := spec.Safety
	fields := []struct {
		name  string
		value string
	}{
		{"Safety.Effect", safety.Effect},
		{"Safety.Risk", safety.Risk},
		{"Safety.Confirmation", safety.Confirmation},
		{"Safety.Idempotency", safety.Idempotency},
	}
	declared := false
	for _, field := range fields {
		if strings.TrimSpace(field.value) != "" {
			declared = true
			break
		}
	}
	if !declared {
		return
	}
	missing := make([]string, 0, len(fields))
	for _, field := range fields {
		if strings.TrimSpace(field.value) == "" {
			missing = append(missing, field.name)
		}
	}
	if len(missing) > 0 {
		panic(fmt.Sprintf(
			"command %q declares partial SafetySpec; missing %s: safety fields are independent and are never inferred from one another",
			spec.Use, strings.Join(missing, ", ")))
	}
}

// RegisterFlags registers every flag (plus hidden aliases and MarkFlagRequired)
// declared by the spec set onto cmd.
func RegisterFlags(cmd *cobra.Command, flags []FlagSpec) {
	for _, flag := range flags {
		// MarkFlagRequired only knows the main name: a user passing a declared
		// alias would get Cobra's "required flag(s) not set" even though the
		// framework validation considers the alias provided. The combination is
		// unsupported and rejected at build time.
		if flag.MarkRequired && len(flag.Aliases) > 0 {
			panic(fmt.Sprintf(
				"flag %q: MarkRequired cannot be combined with Aliases: cobra MarkFlagRequired only recognizes the main name, so a value passed via an alias would be rejected",
				flag.Name))
		}
		registerFlagP(cmd, flag.Kind, flag.Name, flag.Shorthand, flag.Default, flag.Usage)
		// Aliases are registered with the main flag's Kind, otherwise an integer
		// alias's value would never be readable (silently dropped).
		for _, alias := range flag.Aliases {
			RegisterFlag(cmd, flag.Kind, alias, "", flag.Usage+" (alias)")
			_ = cmd.Flags().MarkHidden(alias)
			if registered := cmd.Flags().Lookup(alias); registered != nil {
				runtimeannotate.SetFlagAnnotation(
					registered,
					runtimeannotate.AnnotationFlagAliasOf,
					flag.Name,
				)
				runtimeannotate.SetFlagAnnotation(
					registered,
					runtimeannotate.AnnotationFlagAliasOrigin,
					runtimeannotate.FlagAliasOriginCorecmdV1,
				)
			}
		}
		if flag.MarkRequired {
			_ = cmd.MarkFlagRequired(flag.Name)
		}
		if flag.Hidden {
			_ = cmd.Flags().MarkHidden(flag.Name)
		}
	}
}

// RegisterFlag registers one flag by Kind. Default is applied at registration
// for every kind so --help DefValue matches the declared fallback.
// Malformed KindInt / KindBool Default values panic at registration (fail-closed)
// instead of silently degrading to 0 / false.
func RegisterFlag(cmd *cobra.Command, kind FlagKind, name, def, usage string) {
	registerFlagP(cmd, kind, name, "", def, usage)
}

func registerFlagP(cmd *cobra.Command, kind FlagKind, name, shorthand, def, usage string) {
	switch kind {
	case KindInt:
		defInt := 0
		if def != "" {
			v, err := strconv.Atoi(def)
			if err != nil {
				panic(fmt.Sprintf("flag %q: invalid KindInt Default %q", name, def))
			}
			defInt = v
		}
		cmd.Flags().IntP(name, shorthand, defInt, usage)
	case KindBool:
		defBool := false
		if def != "" {
			switch def {
			case "true":
				defBool = true
			case "false":
				defBool = false
			default:
				panic(fmt.Sprintf("flag %q: invalid KindBool Default %q (want \"true\" or \"false\")", name, def))
			}
		}
		cmd.Flags().BoolP(name, shorthand, defBool, usage)
	case KindStringSlice:
		var defaults []string
		if value := strings.TrimSpace(def); value != "" {
			defaults = strings.Split(value, ",")
		}
		cmd.Flags().StringSliceP(name, shorthand, defaults, usage)
	default:
		cmd.Flags().StringP(name, shorthand, def, usage)
	}
}

// ValidateRequired reproduces the handwritten required semantics: plain Required
// flags report a unified "missing required flag(s)" error; Required flags with
// EnvVar/RequiredHint report their hint separately. The plain group is checked
// before the env group to preserve the handwritten order. Both groups use the
// declared "main flag → alias → env" fallback: a compatible alias counts as
// provided. Shortcut mode keeps its stricter contract: it demands an explicit
// token, so the main flag or an alias must be Changed on the command line — a
// registration default does not satisfy it.
func ValidateRequired(cmd *cobra.Command, flags []FlagSpec) error {
	for _, flag := range flags {
		if flag.ValidationMode != ValidationShortcut {
			continue
		}
		if flag.Required {
			if !flagNameProvided(cmd, flag) {
				message := strings.TrimSpace(flag.RequiredError)
				if message == "" {
					message = fmt.Sprintf("缺少必填参数 --%s", flag.Name)
				}
				return apperrors.NewValidation(message)
			}
			switch flag.Kind {
			case KindStringSlice:
				if !sliceHasValue(sliceValue(cmd, flag)) {
					return apperrors.NewValidation(fmt.Sprintf("必填参数 --%s 不能为空", flag.Name))
				}
			case KindString:
				if strings.TrimSpace(EffectiveValue(cmd, flag)) == "" {
					return apperrors.NewValidation(fmt.Sprintf("必填参数 --%s 不能为空", flag.Name))
				}
			}
		}
		if err := validateEnum(cmd, flag); err != nil {
			return err
		}
	}

	var plain []string
	for _, flag := range flags {
		if flag.Required && flag.ValidationMode != ValidationShortcut &&
			flag.EnvVar == "" && flag.RequiredHint == "" && !hasEffectiveValue(cmd, flag) {
			plain = append(plain, flag.Name)
		}
	}
	if err := cmdutil.MissingRequiredFlagsError(cmd, plain...); err != nil {
		return err
	}
	for _, flag := range flags {
		if !flag.Required || flag.ValidationMode == ValidationShortcut ||
			(flag.EnvVar == "" && flag.RequiredHint == "") {
			continue
		}
		if !hasEffectiveValue(cmd, flag) {
			hint := flag.RequiredHint
			if hint == "" {
				hint = fmt.Sprintf("flag --%s is required", flag.Name)
			}
			return apperrors.NewValidation(hint)
		}
	}
	return nil
}

// ValidateEnums enforces the accepted values declared on changed flags. A
// registration default does not trigger validation, matching Shortcut's
// historical behavior.
func ValidateEnums(cmd *cobra.Command, flags []FlagSpec) error {
	for _, flag := range flags {
		if flag.ValidationMode == ValidationShortcut {
			continue
		}
		if err := validateEnum(cmd, flag); err != nil {
			return err
		}
	}
	return nil
}

// validateEnum enforces the declared accepted values. Explicit CLI tokens
// (main name or alias) are always validated. An env-sourced value is validated
// too when the flag was not explicitly provided but resolves a non-empty
// EnvVar value: the env path feeds Required checks and BuildArgs, so an
// out-of-enum env value must not ship to the backend (bool flags are skipped —
// booleans have no env fallback). Env still never counts as "provided" for
// explicit-token checks; this only closes the enum gap. Registration defaults
// remain unvalidated.
func validateEnum(cmd *cobra.Command, flag FlagSpec) error {
	if len(flag.Enum) == 0 {
		return nil
	}
	var values []string
	switch {
	case flagNameProvided(cmd, flag):
		if flag.Kind == KindStringSlice {
			values = sliceValue(cmd, flag)
		} else {
			values = []string{EffectiveValue(cmd, flag)}
		}
	case flag.Kind != KindBool && flag.Kind != KindStringSlice && flag.EnvVar != "":
		// Not explicitly provided: validate the env fallback when it resolves
		// a non-empty string-ish value. Slices never consume env (explicit
		// tokens only), and a value invalid for the flag kind — e.g. a
		// non-integer env value on KindInt — is left to the integer parse
		// path, which reports the precise error.
		if env := strings.TrimSpace(os.Getenv(flag.EnvVar)); env != "" {
			values = []string{env}
		}
	}
	for _, value := range values {
		value = strings.TrimSpace(value)
		valid := false
		for _, allowed := range flag.Enum {
			if value == allowed {
				valid = true
				break
			}
		}
		if !valid {
			return apperrors.NewValidation(fmt.Sprintf(
				"参数 --%s 取值 %q 不合法，允许值：%s",
				flag.Name, value, strings.Join(flag.Enum, ", ")))
		}
	}
	return nil
}

// flagNameProvided reports whether the flag was explicitly passed on the
// command line under its main name or any declared alias. Registration
// defaults and environment variables do not count as provided: those feed the
// effective-value chain, not the explicit-token checks used by Shortcut
// Required. validateEnum additionally validates a resolved EnvVar value, but
// env still never counts as provided.
func flagNameProvided(cmd *cobra.Command, flag FlagSpec) bool {
	if cmd.Flags().Changed(flag.Name) {
		return true
	}
	for _, alias := range flag.Aliases {
		if cmd.Flags().Changed(alias) {
			return true
		}
	}
	return false
}

// hasEffectiveValue decides whether a Required flag is satisfied, matching the
// BuildArgs entry predicate (KindInt non-zero, string non-empty, KindBool
// explicitly provided, KindStringSlice has a non-empty element). Integer parse
// failure counts as provided so BuildArgs reports the precise invalid-integer
// error.
func hasEffectiveValue(cmd *cobra.Command, flag FlagSpec) bool {
	switch flag.Kind {
	case KindInt:
		v, err := integerValue(cmd, flag)
		if err != nil {
			return true
		}
		return v != 0
	case KindBool:
		// Bool presence is any declared name changed (booleans have no
		// env/default fallback semantics).
		return flagNameProvided(cmd, flag)
	case KindStringSlice:
		return sliceValue(cmd, flag) != nil
	}
	return EffectiveValue(cmd, flag) != ""
}

// sliceValue reads a list flag's effective value by "explicit main flag →
// explicit alias" order: elements are TrimSpace'd and empties dropped, and an
// all-empty result counts as not provided (returns nil). Lists do not
// participate in the env/Default fallback chain.
func sliceValue(cmd *cobra.Command, flag FlagSpec) []string {
	names := append([]string{flag.Name}, flag.Aliases...)
	for _, name := range names {
		if !cmd.Flags().Changed(name) {
			continue
		}
		raw, _ := cmd.Flags().GetStringSlice(name)
		var out []string
		for _, value := range raw {
			if value = strings.TrimSpace(value); value != "" {
				out = append(out, value)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
}

// bindKey is the toolArgs key for a flag: the declared Bind, or the flag name
// converted kebab-to-camel.
//
// The alternative default — sending the kebab name verbatim — is accepted by no
// backend, so a forgotten Bind used to produce a silently wrong payload key;
// deriving it makes that failure mode correct instead.
//
// It is deliberately *not* a licence to drop the explicit declarations: Bind is
// also the declared source of the Schema property mapping, and relying on this
// derivation downgrades that field's provenance to flag_name_inference, which
// the "declared or annotated, never inference-only" rule forbids (measured:
// dropping the 93 mechanical declarations produced 552 lines of catalog drift).
func bindKey(flag FlagSpec) string {
	if flag.Bind != "" {
		return flag.Bind
	}
	parts := strings.Split(flag.Name, "-")
	if len(parts) <= 1 {
		return flag.Name
	}
	for i := 1; i < len(parts); i++ {
		if parts[i] != "" {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
}

// BuildArgs assembles toolArgs from the flag set by binding relationship.
func BuildArgs(cmd *cobra.Command, flags []FlagSpec) (map[string]any, error) {
	toolArgs := map[string]any{}
	for _, flag := range flags {
		bind := bindKey(flag)
		if flag.Kind == KindInt {
			v, err := integerValue(cmd, flag)
			if err != nil {
				return nil, err
			}
			// ArgDefault floors values < 1 (cursor page-size: 0/-1 → default).
			if v < 1 && flag.ArgDefault != "" {
				parsed, err := strconv.ParseInt(flag.ArgDefault, 10, 64)
				if err != nil {
					return nil, fmt.Errorf("flag --%s: invalid ArgDefault %q", flag.Name, flag.ArgDefault)
				}
				v = parsed
			}
			// Keep "non-zero only" (putInt semantics).
			if v != 0 {
				toolArgs[bind] = int(v)
			}
			continue
		}
		if flag.Kind == KindBool {
			// Enter on Changed only (main OR alias): explicit false is still
			// sent (matching the handwritten "transmit on Changed" semantics).
			if flagNameProvided(cmd, flag) {
				v, _ := cmd.Flags().GetBool(flag.Name)
				if !cmd.Flags().Changed(flag.Name) {
					for _, alias := range flag.Aliases {
						if cmd.Flags().Changed(alias) {
							v, _ = cmd.Flags().GetBool(alias)
							break
						}
					}
				}
				toolArgs[bind] = v
			}
			continue
		}
		if flag.Kind == KindStringSlice {
			if v := sliceValue(cmd, flag); v != nil {
				toolArgs[bind] = v
			}
			continue
		}
		effective := EffectiveValue(cmd, flag)
		if effective == "" && flag.ArgDefault != "" {
			effective = flag.ArgDefault
		}
		if effective == "" && flag.OmitEmpty {
			continue
		}
		if flag.Transform != nil {
			value, err := flag.Transform(effective)
			if err != nil {
				return nil, err
			}
			// Required is checked on the pre-transform string. A transform that
			// collapses separator-only input ("," / ";") to nil/empty must still
			// fail required flags locally — otherwise BuildArgs would omit the
			// key and ConfirmFirst write paths could reach the backend.
			if value == nil || emptyTransformResult(value) {
				if flag.Required {
					message := strings.TrimSpace(flag.RequiredHint)
					if message == "" {
						message = strings.TrimSpace(flag.RequiredError)
					}
					if message == "" {
						message = fmt.Sprintf("必填参数 --%s 不能为空", flag.Name)
					}
					return nil, apperrors.NewValidation(message)
				}
				continue
			}
			toolArgs[bind] = value
			continue
		}
		toolArgs[bind] = effective
	}
	return toolArgs, nil
}

// emptyTransformResult reports whether a Transform produced an empty payload
// that must not satisfy a Required flag (nil is handled by the caller).
func emptyTransformResult(value any) bool {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v) == ""
	case []string:
		return len(v) == 0
	case []any:
		return len(v) == 0
	default:
		return false
	}
}

// EffectiveValue reads the value by "explicit main flag → alias → env →
// registration default" order (string form, integers uniformly formatted);
// Trim TrimSpace's the result.
func EffectiveValue(cmd *cobra.Command, flag FlagSpec) string {
	v := rawValue(cmd, flag)
	if flag.Trim {
		v = strings.TrimSpace(v)
	}
	return v
}

// rawValue reads the un-trimmed effective value. The main flag wins only when
// explicitly provided (Changed) and non-empty; the registration default is
// demoted to a chain tail and no longer shadows aliases/env. When Trim is set,
// candidates are judged empty after trimming, so whitespace-only and empty fall
// through to the next fallback level.
func rawValue(cmd *cobra.Command, flag FlagSpec) string {
	usable := func(v string) bool {
		if flag.Trim {
			v = strings.TrimSpace(v)
		}
		return v != ""
	}
	if cmd.Flags().Changed(flag.Name) {
		if v := flagString(cmd, flag.Kind, flag.Name); usable(v) {
			return v
		}
	}
	for _, alias := range flag.Aliases {
		if !cmd.Flags().Changed(alias) {
			continue
		}
		if v := flagString(cmd, flag.Kind, alias); usable(v) {
			return v
		}
	}
	if flag.EnvVar != "" {
		if v := os.Getenv(flag.EnvVar); usable(v) {
			return v
		}
	}
	return flag.Default
}

// flagString reads a flag by registered type and normalizes to string form so
// integer flags can reuse the same fallback chain (required checks, aliases, env).
func flagString(cmd *cobra.Command, kind FlagKind, name string) string {
	switch kind {
	case KindInt:
		v, _ := cmd.Flags().GetInt(name)
		return strconv.Itoa(v)
	default:
		return cmdutil.MustGetFlag(cmd, name)
	}
}

// integerValue reads an integer flag's effective value by the fallback chain; an
// env-provided string must be parseable, otherwise it errors rather than
// silently dropping.
func integerValue(cmd *cobra.Command, flag FlagSpec) (int64, error) {
	raw := EffectiveValue(cmd, flag)
	if raw == "" {
		return 0, nil
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("flag --%s: invalid integer value %q", flag.Name, raw)
	}
	return v, nil
}

// ValidateConstraintDecls validates constraint declarations at build time: an
// unknown kind, an under-sized flag group, or a reference to an undeclared flag
// is a programming error and panics so any test/startup path fails immediately
// rather than at user runtime. use is only used for the panic message.
func ValidateConstraintDecls(use string, flags []FlagSpec, constraints []Constraint) {
	declared := map[string]bool{}
	for _, flag := range flags {
		declared[flag.Name] = true
	}
	for _, constraint := range constraints {
		switch constraint.Kind {
		case AtLeastOne, ExactlyOne, MutuallyExclusive:
			if len(constraint.Flags) < 2 {
				panic(fmt.Sprintf("command %q: constraint %s needs at least two flags", use, constraint.Kind))
			}
		case Custom:
			if len(constraint.Flags) < 1 {
				panic(fmt.Sprintf("command %q: constraint %s needs at least one flag", use, constraint.Kind))
			}
			if strings.TrimSpace(constraint.Description) == "" {
				panic(fmt.Sprintf("command %q: custom constraint requires a description", use))
			}
		default:
			panic(fmt.Sprintf("command %q: unknown constraint kind %q", use, constraint.Kind))
		}
		for _, name := range constraint.Flags {
			if !declared[name] {
				panic(fmt.Sprintf("command %q: constraint %s references undeclared flag %q", use, constraint.Kind, name))
			}
		}
	}
}

// constraintProvided decides whether a flag is "provided" for constraint
// purposes: an explicit main flag, explicit alias, or env var counts; the
// registration default/ArgDefault does not — otherwise a defaulted flag would
// always satisfy at_least_one and always trip mutually_exclusive. KindBool
// counts Changed on any declared name (booleans have no env fallback
// semantics).
func constraintProvided(cmd *cobra.Command, flag FlagSpec) bool {
	switch flag.Kind {
	case KindBool:
		return flagNameProvided(cmd, flag)
	case KindStringSlice:
		if cmd.Flags().Changed(flag.Name) {
			if v, _ := cmd.Flags().GetStringSlice(flag.Name); sliceHasValue(v) {
				return true
			}
		}
		for _, alias := range flag.Aliases {
			if !cmd.Flags().Changed(alias) {
				continue
			}
			if v, _ := cmd.Flags().GetStringSlice(alias); sliceHasValue(v) {
				return true
			}
		}
		return false
	}
	usable := func(v string) bool { return strings.TrimSpace(v) != "" }
	if cmd.Flags().Changed(flag.Name) && usable(flagString(cmd, flag.Kind, flag.Name)) {
		return true
	}
	for _, alias := range flag.Aliases {
		if cmd.Flags().Changed(alias) && usable(flagString(cmd, flag.Kind, alias)) {
			return true
		}
	}
	return flag.EnvVar != "" && usable(os.Getenv(flag.EnvVar))
}

// ValidateConstraints enforces the relationship constraints. Error wording
// matches the shortcut framework's RuntimeContext.AtLeastOne/ExactlyOne/
// MutuallyExclusive verbatim, so atomic commands and smart shortcuts fail
// identically for users and agents.
func ValidateConstraints(cmd *cobra.Command, flags []FlagSpec, constraints []Constraint) error {
	flagsByName := map[string]FlagSpec{}
	for _, flag := range flags {
		flagsByName[flag.Name] = flag
	}
	for _, constraint := range constraints {
		var set []string
		for _, name := range constraint.Flags {
			if constraintProvided(cmd, flagsByName[name]) {
				set = append(set, name)
			}
		}
		switch constraint.Kind {
		case AtLeastOne:
			if len(set) == 0 {
				return apperrors.NewValidation(fmt.Sprintf(
					"请至少指定 %s 之一", dashed(constraint.Flags)))
			}
		case ExactlyOne:
			switch len(set) {
			case 1:
			case 0:
				return apperrors.NewValidation(fmt.Sprintf("请指定 %s 之一", dashed(constraint.Flags)))
			default:
				return apperrors.NewValidation(fmt.Sprintf(
					"参数 %s 只能指定其一（当前指定了 %s）", dashed(constraint.Flags), dashed(set)))
			}
		case MutuallyExclusive:
			if len(set) > 1 {
				return apperrors.NewValidation(fmt.Sprintf(
					"参数 %s 互斥，只能指定其一（当前指定了 %s）", dashed(constraint.Flags), dashed(set)))
			}
		case Custom:
			// The declaration is published and rendered in help. Its actual
			// command-specific rule remains owned by Spec.Validate.
		}
	}
	return nil
}

// ConfirmSafety enforces the command's declared confirmation requirement.
// Effect, risk and idempotency are metadata only and never imply confirmation.
// Semantics:
//
//   - read-only, --dry-run, --yes, or --user-say-yes → nil (proceed);
//   - interactive yes/y → nil;
//   - interactive decline → validation "用户取消了操作" (existing command path);
//   - no interactive answer (EOF / closed stdin) → confirmation_required.
//
// EOF must not be treated as decline: that silently drops writes in agent/CI.
// Prompt text is terminal-gated; a readable piped answer is still honored for
// non-Sheet leaves. Sheet destructive commands keep a separate --yes-only
// outer gate (helpers.protectSheetMutationCommand) so agents cannot authorize
// those via stdin alone.
func ConfirmSafety(cmd *cobra.Command, safety contract.SafetySpec) error {
	if strings.TrimSpace(safety.Confirmation) != "user_required" ||
		confirmationBypass(cmd) {
		return nil
	}
	// Only print the interactive prompt on a real terminal. In non-interactive
	// environments (agent/CI: pipe, closed stdin, /dev/null) the prompt is
	// noise on stderr ahead of the structured confirmation_required error —
	// callers there should pass --yes/--dry-run. A piped answer
	// (printf 'yes\n' | cmd) is still honored for general ConfirmSafety; Sheet
	// mutations additionally require --yes via protectSheetMutationCommand.
	if stdinIsTerminalFn(cmd.InOrStdin()) {
		fmt.Fprintf(
			cmd.ErrOrStderr(),
			"即将执行 %s（effect=%s, risk=%s），确认继续？(yes/no): ",
			cmd.CommandPath(), strings.TrimSpace(safety.Effect), strings.TrimSpace(safety.Risk),
		)
	}
	reader := bufio.NewReader(cmd.InOrStdin())
	answer, err := reader.ReadString('\n')
	if errors.Is(err, io.EOF) && strings.TrimSpace(answer) == "" {
		return confirmationRequiredError(cmd.CommandPath())
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return confirmationRequiredError(cmd.CommandPath())
	}
	answer = strings.TrimSpace(strings.ToLower(answer))
	if answer == "yes" || answer == "y" {
		return nil
	}
	return apperrors.NewValidation("用户取消了操作")
}

// stdinIsTerminalFn is the ConfirmSafety TTY probe. Tests may stub it; production
// keeps the real ioctl-backed check.
var stdinIsTerminalFn = stdinIsTerminal

// stdinIsTerminal reports whether the given input is a real terminal. Only
// *os.File inputs can be terminals; cobra SetIn buffers and other readers are
// treated as non-interactive. An ioctl-level TTY check is required — a plain
// character-device stat would misclassify redirects like `< /dev/null`.
func stdinIsTerminal(in io.Reader) bool {
	file, ok := in.(*os.File)
	if !ok {
		return false
	}
	fd := file.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

func confirmationRequiredError(operation string) error {
	return apperrors.NewValidation(
		fmt.Sprintf("%s 需要用户确认，当前环境无法交互；加 --dry-run 预览，或确认后加 --yes 执行", operation),
		apperrors.WithReason("confirmation_required"),
		apperrors.WithHint("非交互环境（agent/CI）必须显式传入 --yes，不能依赖 stdin 提示"),
		apperrors.WithActions("确认目标与变更影响", "以相同参数追加 --yes 执行"),
	)
}

// BoolFlag robustly reads a bool flag that may live on the command, its
// inherited flags, or the root's persistent flags (e.g. root-injected global
// --yes / --dry-run).
//
// It ORs across flagsets, matching confirmationBypass. Returning the first
// resolving flagset instead let a leaf-local --dry-run (default false) shadow a
// root persistent --dry-run=true, so confirmation could be bypassed as a dry run
// while Ctx.DryRun() reported false — a real write with no confirmation.
func BoolFlag(cmd *cobra.Command, name string) bool {
	if cmd == nil {
		return false
	}
	for _, get := range boolFlagGetters(cmd) {
		if v, err := get(name); err == nil && v {
			return true
		}
	}
	return false
}

// confirmationBypass reports whether any flagset on the command tree has
// --yes / --dry-run / --user-say-yes set true. BoolFlag already ORs across
// flagsets, so a leaf-local --dry-run defaulting to false cannot shadow a
// root persistent --dry-run=true (markdown overwrite / agent global dry-run).
func confirmationBypass(cmd *cobra.Command) bool {
	for _, name := range []string{"yes", "dry-run", "user-say-yes"} {
		if BoolFlag(cmd, name) {
			return true
		}
	}
	return false
}

func boolFlagGetters(cmd *cobra.Command) []func(string) (bool, error) {
	if cmd == nil {
		return nil
	}
	getters := []func(string) (bool, error){
		cmd.Flags().GetBool,
		cmd.InheritedFlags().GetBool,
	}
	if root := cmd.Root(); root != nil {
		getters = append(getters, root.PersistentFlags().GetBool)
	}
	return getters
}

// embedContractIntoSchema projects Spec declaration onto the live Cobra
// leaf as the final Schema payload (dws.schema.*). Assembly pass-throughs
// these annotations; declared fields are the final source with no parallel
// authority.
func embedContractIntoSchema(cmd *cobra.Command, spec Spec) {
	if spec.ParameterProjection == ProjectCobraParameters {
		for _, flag := range spec.Flags {
			name := strings.TrimSpace(flag.Name)
			if name == "" || flag.Hidden {
				continue
			}
			if flag.Required || flag.MarkRequired {
				runtimeannotate.AnnotateRuntimeRequiredFlags(cmd, name)
			}
			if len(flag.Enum) > 0 {
				runtimeannotate.AnnotateRuntimeFlagEnum(cmd, name, flag.Enum...)
			}
			// Same class as Enum: a declared rule the collected identity
			// cannot derive. Without it a credential that is mandatory under
			// one identity is published as plainly optional.
			if flag.RequiredWhen != "" {
				runtimeannotate.AnnotateRuntimeFlagRequiredWhen(cmd, name, flag.RequiredWhen)
			}
		}
		embedContractDecl(cmd, spec)
		return
	}

	runtimeannotate.AnnotateRuntimeContract(cmd)
	required := make([]string, 0, len(spec.Flags))
	for _, flag := range spec.Flags {
		name := strings.TrimSpace(flag.Name)
		if name == "" || flag.Hidden {
			continue
		}
		requiredFlag := flag.Required || flag.MarkRequired
		runtimeannotate.AnnotateRuntimeFlag(cmd, name, strings.TrimSpace(flag.Bind), flagKindSchemaType(flag.Kind), requiredFlag)
		desc := strings.TrimSpace(flag.SchemaDescription)
		if desc == "" {
			desc = strings.TrimSpace(flag.Usage)
		}
		if desc != "" {
			runtimeannotate.AnnotateRuntimeFlagDescription(cmd, name, desc)
		}
		if flag.RequiredWhen != "" {
			runtimeannotate.AnnotateRuntimeFlagRequiredWhen(cmd, name, flag.RequiredWhen)
		}
		if flag.Format != "" {
			runtimeannotate.AnnotateRuntimeFlagFormat(cmd, name, flag.Format)
		}
		if flag.Example != "" {
			runtimeannotate.AnnotateRuntimeFlagExample(cmd, name, flag.Example)
		}
		if len(flag.Enum) > 0 {
			runtimeannotate.AnnotateRuntimeFlagEnum(cmd, name, flag.Enum...)
		}
		if requiredFlag {
			required = append(required, name)
		}
	}
	runtimeannotate.AnnotateRuntimeRequiredFlags(cmd, required...)
	embedContractDecl(cmd, spec)
}

// embedContractDecl does a light runtime write: only when ContractDecl is authored,
// convert once through contractfinal.RegisterRuntimeContractFinal (annotate + store).
func embedContractDecl(cmd *cobra.Command, spec Spec) {
	if spec.Contract.empty() {
		return
	}
	AttachContract(cmd, spec.Safety, spec.Contract, spec.Short, spec.Long)
}

// AttachContract registers a ContractFinal overlay on an existing leaf without
// replacing its RunE/Execute body. Used to migrate reviewed facts onto helpers
// while keeping execution substance frozen. Overwrites any prior ContractFinal
// on cmd; does not alter an already-installed ConfirmSafety closure.
//
// Production registration always goes through contractfinal.RegisterRuntimeContractFinal
// (annotate + store). Do not call the store/Register APIs except via
// contractfinal — no cli-root wrapper exists.
//
// Title/Description stored on the payload are the declared Contract values
// only. Catalog assembly may prefer Cobra Long for delivered description
// (Short never enters description) and must stamp provenance to the real
// winner (cobra_help vs contract_final). Declared Title still wins over Short.
func AttachContract(cmd *cobra.Command, safety contract.SafetySpec, decl ContractDecl, short, long string) {
	if cmd == nil || decl.empty() {
		return
	}
	// short/long remain in the signature so call sites keep passing Cobra prose;
	// Catalog assembly (not this store) prefers Long for description and may
	// use Short only as a title fallback after declared Title.
	_, _ = short, long
	// Reuse NewCommand's completeness rules so bind-time attaches cannot ship
	// a partial declaration that would only fail in generated artifacts.
	validateContractDecl(Spec{Use: cmd.Name(), Safety: safety, Contract: decl})
	validateSafetySpec(Spec{Use: cmd.Name(), Safety: safety})

	payload := contract.ContractFinalPayload{
		Title:       strings.TrimSpace(decl.Title),
		Description: strings.TrimSpace(decl.Description),
		Safety:      schemaSafetyFromDecl(safety),
	}
	if n := len(decl.Positionals); n > 0 {
		payload.Positionals = append([]contract.RuntimeSchemaPositional(nil), decl.Positionals...)
	}
	if decl.DryRun != nil && strings.TrimSpace(decl.DryRun.PreviewKind) != "" {
		d := *decl.DryRun
		d.PreviewKind = strings.TrimSpace(d.PreviewKind)
		payload.DryRun = &d
	}
	if decl.Result != nil {
		result, err := contract.NormalizeResultSpec(decl.Result, decl.Identity.CanonicalPath)
		if err != nil {
			panic(fmt.Sprintf("command %q has invalid Contract.Result: %v", cmd.Name(), err))
		}
		payload.Result = result
	}
	if decl.Pagination != nil {
		pagination, err := contract.NormalizePaginationSpec(decl.Pagination, decl.Identity.CanonicalPath)
		if err != nil {
			panic(fmt.Sprintf("command %q has invalid Contract.Pagination: %v", cmd.Name(), err))
		}
		payload.Pagination = pagination
	}
	if decl.Interface != nil {
		iface := &contract.InterfaceSpec{
			Mode:         strings.TrimSpace(decl.Interface.Mode),
			Availability: strings.TrimSpace(decl.Interface.Availability),
			Reason:       strings.TrimSpace(decl.Interface.Reason),
		}
		if decl.Interface.Ref != nil {
			ref := *decl.Interface.Ref
			ref.ProductID = strings.TrimSpace(ref.ProductID)
			ref.RPCName = strings.TrimSpace(ref.RPCName)
			if ref.ProductID != "" || ref.RPCName != "" {
				iface.Ref = &ref
			}
		}
		if iface.Mode != "" || iface.Ref != nil || iface.Availability != "" || iface.Reason != "" {
			payload.Interface = iface
		}
	}
	if sel := decl.Selection; strings.TrimSpace(sel.AgentSummary) != "" || len(sel.UseWhen) > 0 ||
		len(sel.AvoidWhen) > 0 || len(sel.Examples) > 0 || len(sel.Prerequisites) > 0 ||
		len(sel.Tips) > 0 || len(sel.WorkflowRefs) > 0 {
		copied := sel
		copied.AgentSummary = strings.TrimSpace(sel.AgentSummary)
		copied.AgentSummarySource = "corecmd.ContractDecl"
		copied.SourceRefs = []string{"corecmd.ContractDecl"}
		copied.MetadataSource = "corecmd.contract"
		copied.Reviewed = nil
		// Match the product path: trim-free, deduped selection lists so leaf
		// and product pass-throughs never diverge.
		copied = copied.Normalized()
		payload.Selection = &copied
	}
	if id := decl.Identity; strings.TrimSpace(id.ProductID) != "" || strings.TrimSpace(id.Name) != "" ||
		strings.TrimSpace(id.CanonicalPath) != "" || strings.TrimSpace(id.CLIPath) != "" ||
		strings.TrimSpace(id.PrimaryCLIPath) != "" {
		copied := id
		copied.ProductID = strings.TrimSpace(id.ProductID)
		copied.SourceProductID = strings.TrimSpace(id.SourceProductID)
		copied.Name = strings.TrimSpace(id.Name)
		copied.Path = strings.TrimSpace(id.Path)
		copied.CLIName = strings.TrimSpace(id.CLIName)
		copied.CanonicalPath = strings.TrimSpace(id.CanonicalPath)
		copied.CLIPath = strings.TrimSpace(id.CLIPath)
		copied.PrimaryCLIPath = strings.TrimSpace(id.PrimaryCLIPath)
		if copied.CLIPath == "" {
			copied.CLIPath = copied.PrimaryCLIPath
		}
		if copied.PrimaryCLIPath == "" {
			copied.PrimaryCLIPath = copied.CLIPath
		}
		copied.Group = strings.TrimSpace(id.Group)
		copied.Source = strings.TrimSpace(id.Source)
		if len(id.Aliases) > 0 {
			copied.Aliases = make([]string, 0, len(id.Aliases))
			for _, alias := range id.Aliases {
				copied.Aliases = append(copied.Aliases, strings.TrimSpace(alias))
			}
		}
		payload.Identity = &copied
	}
	if len(decl.Parameters) > 0 {
		payload.Parameters = append([]contract.ParamDecl(nil), decl.Parameters...)
	}
	contractfinal.RegisterRuntimeContractFinal(cmd, payload)
}

// schemaSafetyFromDecl copies the single command SafetySpec into the final
// Schema payload. The zero value keeps the historical read-only default.
func schemaSafetyFromDecl(safety contract.SafetySpec) *contract.SafetySpec {
	out := effectiveSafetySpec(safety)
	out.EffectSource = "corecmd.contract"
	return &out
}

func effectiveSafetySpec(safety contract.SafetySpec) contract.SafetySpec {
	out := contract.SafetySpec{
		Effect:       strings.TrimSpace(safety.Effect),
		Risk:         strings.TrimSpace(safety.Risk),
		Confirmation: strings.TrimSpace(safety.Confirmation),
		Idempotency:  strings.TrimSpace(safety.Idempotency),
	}
	if out.Effect == "" && out.Risk == "" && out.Confirmation == "" && out.Idempotency == "" {
		out.Effect = "read"
		out.Risk = "low"
		out.Confirmation = "not_required"
		out.Idempotency = "idempotent"
	}
	return out
}

func flagKindSchemaType(kind FlagKind) string {
	switch kind {
	case KindInt:
		return "integer"
	case KindBool:
		return "boolean"
	case KindStringSlice:
		return "array"
	default:
		return "string"
	}
}

// AnnotateConstraints projects the relationship constraints into the Agent
// Runtime Schema: exactly_one decomposes into require_one_of + mutually_exclusive
// (matching the handwritten commands' use of AnnotateRuntimeConstraints).
//
// When a group still has hidden siblings, the full declared flag list is
// projected (not collapsed to a single visible "required"). ValidateConstraints
// accepts any member of the declared group — including hidden — so marking the
// sole visible flag required would falsely claim declare ≡ execute.
func AnnotateConstraints(cmd *cobra.Command, constraints []Constraint) {
	var projected runtimeannotate.RuntimeSchemaConstraints
	var required []string
	for _, constraint := range constraints {
		visible := make([]string, 0, len(constraint.Flags))
		for _, name := range constraint.Flags {
			flag := cmd.Flags().Lookup(name)
			if flag != nil && !flag.Hidden {
				visible = append(visible, name)
			}
		}
		flags := visible
		if len(visible) < len(constraint.Flags) {
			flags = append([]string(nil), constraint.Flags...)
		}
		switch constraint.Kind {
		case AtLeastOne:
			if len(flags) == 1 {
				required = append(required, flags[0])
			} else if len(flags) > 1 {
				projected.RequireOneOf = append(projected.RequireOneOf, flags)
			}
		case ExactlyOne:
			if len(flags) == 1 {
				required = append(required, flags[0])
			} else if len(flags) > 1 {
				projected.RequireOneOf = append(projected.RequireOneOf, flags)
				projected.MutuallyExclusive = append(projected.MutuallyExclusive, flags)
			}
		case MutuallyExclusive:
			if len(flags) > 1 {
				projected.MutuallyExclusive = append(projected.MutuallyExclusive, flags)
			}
		}
	}
	runtimeannotate.AnnotateRuntimeRequiredFlags(cmd, required...)
	runtimeannotate.AnnotateRuntimeConstraints(cmd, projected)
}

// ConstraintHelp renders the --help "参数约束" section, matching the shortcut
// leaf help shape; returns "" when there are no constraints.
func ConstraintHelp(constraints []Constraint) string {
	if len(constraints) == 0 {
		return ""
	}
	lines := make([]string, 0, len(constraints))
	for _, constraint := range constraints {
		text := strings.TrimSpace(constraint.Description)
		if text == "" {
			switch constraint.Kind {
			case AtLeastOne:
				text = fmt.Sprintf("%s 至少指定一个", dashed(constraint.Flags))
			case ExactlyOne:
				text = fmt.Sprintf("%s 必须且只能指定一个", dashed(constraint.Flags))
			case MutuallyExclusive:
				text = fmt.Sprintf("%s 互斥，最多指定一个", dashed(constraint.Flags))
			}
		}
		lines = append(lines, "  - "+text)
	}
	return "\n\n参数约束：\n" + strings.Join(lines, "\n")
}

func dashed(flags []string) string {
	out := make([]string, len(flags))
	for i, f := range flags {
		out[i] = "--" + f
	}
	return strings.Join(out, "、")
}

func sliceHasValue(values []string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}
