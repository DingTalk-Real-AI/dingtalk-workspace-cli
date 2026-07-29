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

// Package cmdcore is the shared, dispatch-agnostic base for building leaf
// commands. It concentrates flag registration, the alias/env/default effective
// value fallback chain, required validation, cross-flag constraint declaration
// checks + runtime enforcement, Risk-driven write confirmation, toolArgs
// assembly, and Agent Runtime Schema projection.
//
// It is deliberately dispatch-agnostic: it never calls an MCP tool. The
// LeafSpec framework (internal/helpers) and, later, the Shortcut framework wrap
// these primitives and supply their own dispatch (single-step MCP / multi-step
// orchestration / escape hatch). Extracting the primitives here lets both
// frameworks share one flag + constraint + risk + schema base, differing only
// in how they dispatch — the first step toward a single typed command registry.
//
// Behavioral contract: this package is a pure extraction of the logic that
// previously lived in internal/helpers/leaf.go. Flag registration, value
// fallback, required/constraint semantics, risk wording, and schema projection
// must remain byte-for-byte equivalent; check-generated-drift (catalog
// unchanged) and the leaf unit tests are the proof.
package cmdcore

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
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

// FlagSpec declares how a flag is registered and bound into MCP toolArgs. Its
// fields intentionally mirror the former helpers.LeafFlag one-for-one so that
// helpers can alias to it without touching any call site.
type FlagSpec struct {
	Name    string   // flag name (kebab-case)
	Usage   string   // registration usage text
	Kind    FlagKind // value type, defaults to KindString
	Default string   // registration default (only KindString uses it at registration; as a fallback-chain tail it applies to all kinds without shadowing aliases/env)

	// Required, when true, validates a non-empty effective value in RunE. Plain
	// Required flags aggregate into a cmdutil.ValidateRequiredFlags-compatible
	// error; when EnvVar is configured the env var is a fallback and, still
	// empty, RequiredHint (or a default hint) is reported.
	Required     bool
	RequiredHint string
	// MarkRequired, when true, calls cobra MarkFlagRequired (the hard floor for
	// the catalog required projection); cobra errors before RunE.
	MarkRequired bool

	Aliases []string // hidden aliases, registered with the main flag's Kind; used in order when the main flag is not explicitly provided
	EnvVar  string   // environment variable consulted when the effective value is empty (an integer flag's env value must be parseable)
	// ArgDefault covers the case where the registration default is empty but
	// toolArgs still needs a fallback; used as the arg value when the effective
	// value is empty.
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
}

// Risk declares a leaf command's side-effect level, driving pre-dispatch write
// confirmation. Values match the shortcut framework's Risk verbatim.
type Risk string

const (
	// RiskRead is a read-only operation; never prompts. Empty value == RiskRead.
	RiskRead Risk = "read"
	// RiskWrite mutates state; prompts unless --yes.
	RiskWrite Risk = "write"
	// RiskHighWrite is a destructive/irreversible operation; prompts unless --yes.
	RiskHighWrite Risk = "high-risk-write"
)

// Effective returns the effective risk, defaulting an empty value to read-only.
func (r Risk) Effective() Risk {
	if r == "" {
		return RiskRead
	}
	return r
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

// RegisterFlags registers every flag (plus hidden aliases and MarkFlagRequired)
// declared by the spec set onto cmd.
func RegisterFlags(cmd *cobra.Command, flags []FlagSpec) {
	for _, flag := range flags {
		RegisterFlag(cmd, flag.Kind, flag.Name, flag.Default, flag.Usage)
		// Aliases are registered with the main flag's Kind, otherwise an integer
		// alias's value would never be readable (silently dropped).
		for _, alias := range flag.Aliases {
			RegisterFlag(cmd, flag.Kind, alias, "", flag.Usage+" (alias)")
			_ = cmd.Flags().MarkHidden(alias)
		}
		if flag.MarkRequired {
			_ = cmd.MarkFlagRequired(flag.Name)
		}
	}
}

// RegisterFlag registers one flag by Kind; the registration default is only used
// by KindString (other kinds treat Default purely as a fallback-chain tail,
// matching the existing KindInt behavior).
func RegisterFlag(cmd *cobra.Command, kind FlagKind, name, def, usage string) {
	switch kind {
	case KindInt:
		cmd.Flags().Int(name, 0, usage)
	case KindBool:
		cmd.Flags().Bool(name, false, usage)
	case KindStringSlice:
		cmd.Flags().StringSlice(name, nil, usage)
	default:
		cmd.Flags().String(name, def, usage)
	}
}

// ValidateRequired reproduces the handwritten required semantics: plain Required
// flags report a unified "missing required flag(s)" error; Required flags with
// EnvVar/RequiredHint report their hint separately. The plain group is checked
// before the env group to preserve the handwritten order. Both groups use the
// declared "main flag → alias → env" fallback: a compatible alias counts as
// provided.
func ValidateRequired(cmd *cobra.Command, flags []FlagSpec) error {
	var plain []string
	for _, flag := range flags {
		if flag.Required && flag.EnvVar == "" && flag.RequiredHint == "" && !hasEffectiveValue(cmd, flag) {
			plain = append(plain, flag.Name)
		}
	}
	if err := cmdutil.MissingRequiredFlagsError(cmd, plain...); err != nil {
		return err
	}
	for _, flag := range flags {
		if !flag.Required || (flag.EnvVar == "" && flag.RequiredHint == "") {
			continue
		}
		if !hasEffectiveValue(cmd, flag) {
			hint := flag.RequiredHint
			if hint == "" {
				hint = fmt.Sprintf("flag --%s is required", flag.Name)
			}
			return fmt.Errorf("%s", hint)
		}
	}
	return nil
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
		return cmd.Flags().Changed(flag.Name)
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

// BuildArgs assembles toolArgs from the flag set by binding relationship.
func BuildArgs(cmd *cobra.Command, flags []FlagSpec) (map[string]any, error) {
	toolArgs := map[string]any{}
	for _, flag := range flags {
		bind := flag.Bind
		if bind == "" {
			bind = flag.Name
		}
		if flag.Kind == KindInt {
			v, err := integerValue(cmd, flag)
			if err != nil {
				return nil, err
			}
			// Keep "non-zero only" (putInt semantics).
			if v != 0 {
				toolArgs[bind] = int(v)
			}
			continue
		}
		if flag.Kind == KindBool {
			// Enter on Changed only: explicit false is still sent (matching the
			// handwritten "transmit on Changed" semantics).
			if cmd.Flags().Changed(flag.Name) {
				v, _ := cmd.Flags().GetBool(flag.Name)
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
			if value == nil {
				continue
			}
			toolArgs[bind] = value
			continue
		}
		toolArgs[bind] = effective
	}
	return toolArgs, nil
}

// effectiveValue reads the value by "explicit main flag → alias → env →
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
		default:
			panic(fmt.Sprintf("leaf %q: unknown constraint kind %q", use, constraint.Kind))
		}
		if len(constraint.Flags) < 2 {
			panic(fmt.Sprintf("leaf %q: constraint %s needs at least two flags", use, constraint.Kind))
		}
		for _, name := range constraint.Flags {
			if !declared[name] {
				panic(fmt.Sprintf("leaf %q: constraint %s references undeclared flag %q", use, constraint.Kind, name))
			}
		}
	}
}

// constraintProvided decides whether a flag is "provided" for constraint
// purposes: an explicit main flag, explicit alias, or env var counts; the
// registration default/ArgDefault does not — otherwise a defaulted flag would
// always satisfy at_least_one and always trip mutually_exclusive. KindBool only
// counts Changed (booleans have no alias/env fallback semantics).
func constraintProvided(cmd *cobra.Command, flag FlagSpec) bool {
	switch flag.Kind {
	case KindBool:
		return cmd.Flags().Changed(flag.Name)
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
		}
	}
	return nil
}

// ConfirmRisk reproduces the shortcut framework's confirmRisk write-confirmation
// semantics: read-only, --dry-run, or --yes pass through; write/high-risk-write
// prompt in interactive mode and return false when the user declines. The prompt
// wording matches the shortcut runner verbatim (command path substitutes the
// shortcut's Service+Command), keeping atomic-command and smart-shortcut
// confirmation identical.
func ConfirmRisk(cmd *cobra.Command, risk Risk) bool {
	if risk.Effective() == RiskRead || BoolFlag(cmd, "dry-run") || BoolFlag(cmd, "yes") {
		return true
	}
	output := cmd.ErrOrStderr()
	fmt.Fprintf(output, "即将执行 %s（%s），确认继续？(yes/no): ", cmd.CommandPath(), risk.Effective())
	reader := bufio.NewReader(cmd.InOrStdin())
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "yes" || answer == "y"
}

// boolFlag robustly reads a bool flag that may live on the command, its
// inherited flags, or the root's persistent flags (e.g. root-injected global
// --yes / --dry-run). Returns the first flagset that resolves the name.
func BoolFlag(cmd *cobra.Command, name string) bool {
	if cmd == nil {
		return false
	}
	getters := []func(string) (bool, error){
		cmd.Flags().GetBool,
		cmd.InheritedFlags().GetBool,
	}
	if root := cmd.Root(); root != nil {
		getters = append(getters, root.PersistentFlags().GetBool)
	}
	for _, get := range getters {
		if v, err := get(name); err == nil {
			return v
		}
	}
	return false
}

// AnnotateConstraints projects the relationship constraints into the Agent
// Runtime Schema: exactly_one decomposes into require_one_of + mutually_exclusive
// (matching the handwritten commands' use of AnnotateRuntimeConstraints).
func AnnotateConstraints(cmd *cobra.Command, constraints []Constraint) {
	var projected cli.RuntimeSchemaConstraints
	for _, constraint := range constraints {
		flags := append([]string(nil), constraint.Flags...)
		switch constraint.Kind {
		case AtLeastOne:
			projected.RequireOneOf = append(projected.RequireOneOf, flags)
		case ExactlyOne:
			projected.RequireOneOf = append(projected.RequireOneOf, flags)
			projected.MutuallyExclusive = append(projected.MutuallyExclusive, flags)
		case MutuallyExclusive:
			projected.MutuallyExclusive = append(projected.MutuallyExclusive, flags)
		}
	}
	cli.AnnotateRuntimeConstraints(cmd, projected)
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
