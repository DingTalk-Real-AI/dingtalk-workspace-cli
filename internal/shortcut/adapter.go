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

package shortcut

import (
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
)

// adapter.go is the compatibility boundary between Shortcut declarations and
// the unified command runtime. Shortcut keeps its RuntimeContext and Execute
// hooks because they own MCP orchestration, while command owns command/flag
// construction, declarative validation, Schema annotations and confirmation.
//
// Shortcut.Risk remains the legacy runtime confirmation source when Safety is
// empty. Explicit Safety overrides Risk expansion; Contract is pass-through into
// ContractFinal when authored.
func FromShortcut(s Shortcut) corecmd.Spec {
	safety := s.Safety
	if !safetySpecDeclared(safety) {
		safety = shortcutSafetySpec(s.risk())
	}
	declaredContract := s.Contract
	if !declaredContract.Empty() && len(s.Aliases) > 0 && len(declaredContract.Identity.Aliases) == 0 {
		declaredContract.Identity.Aliases = make([]string, 0, len(s.Aliases))
		for _, alias := range s.Aliases {
			declaredContract.Identity.Aliases = append(
				declaredContract.Identity.Aliases,
				s.Service+" "+strings.TrimSpace(alias),
			)
		}
	}
	return corecmd.Spec{
		Use:     s.Command,
		Short:   s.Description,
		Example: shortcutExamples(s.Tips),
		Hidden:  s.Hidden,
		// Only the prose part: corecmd.New appends its own 参数约束
		// section, so the adapter must not pre-render it.
		Long:        shortcutIntentProse(s),
		Flags:       fromShortcutFlags(s.Flags),
		Constraints: fromShortcutConstraints(s.Constraints),
		Safety:      safety,
		Contract:    declaredContract,
		// Preserve the shipped Shortcut Catalog provenance: Cobra remains the
		// source for type/default/usage, while command adds Required/Enum/rules.
		ParameterProjection: corecmd.ProjectCobraParameters,
		Validate:            fromShortcutValidate(s),
		PostMount:           fromShortcutPostMount(s),
		// Multi-step body: command stays backend-agnostic, so the shortcut's own
		// RuntimeContext — which owns CallMCPData/CallMCPWriteData/Output — is
		// built here from the Ctx's command.
		Orchestrate: func(c *corecmd.Ctx) error {
			if s.Execute == nil {
				return apperrors.NewInternal(fmt.Sprintf(
					"shortcut %s %s 未实现 Execute", s.Service, s.Command))
			}
			return s.Execute(&RuntimeContext{cmd: c.Command(), shortcut: s})
		},
	}
}

func fromShortcutPostMount(s Shortcut) func(*cobra.Command) {
	hasVisibleFlagAliases := false
	for _, flag := range s.Flags {
		if flag.AliasesVisible && len(flag.Aliases) > 0 {
			hasVisibleFlagAliases = true
			break
		}
	}
	if len(s.Aliases) == 0 && strings.TrimSpace(s.SinglePositionalAliasFor) == "" && !hasVisibleFlagAliases {
		return nil
	}
	return func(cmd *cobra.Command) {
		cmd.Aliases = append([]string(nil), s.Aliases...)
		for _, flag := range s.Flags {
			if !flag.AliasesVisible {
				continue
			}
			for _, alias := range flag.Aliases {
				if mounted := cmd.Flags().Lookup(alias); mounted != nil {
					mounted.Hidden = false
				}
			}
		}
		name := strings.TrimSpace(s.SinglePositionalAliasFor)
		if name == "" {
			return
		}
		cmd.Args = func(cmd *cobra.Command, args []string) error {
			if err := cobra.MaximumNArgs(1)(cmd, args); err != nil {
				return err
			}
			if len(args) == 0 {
				return nil
			}
			flag := cmd.Flags().Lookup(name)
			if flag == nil {
				return apperrors.NewInternal(fmt.Sprintf(
					"shortcut %s %s positional alias flag --%s is not registered",
					s.Service, s.Command, name))
			}
			if flag.Changed {
				return apperrors.NewValidation(fmt.Sprintf("位置参数与 --%s 不能同时提供", name))
			}
			if err := cmd.Flags().Set(name, args[0]); err != nil {
				return apperrors.NewValidation(fmt.Sprintf("位置参数无法写入 --%s: %v", name, err))
			}
			return nil
		}
	}
}

func safetySpecDeclared(safety contract.SafetySpec) bool {
	return strings.TrimSpace(safety.Effect) != "" ||
		strings.TrimSpace(safety.Risk) != "" ||
		strings.TrimSpace(safety.Confirmation) != "" ||
		strings.TrimSpace(safety.Idempotency) != ""
}

// EffectiveSafety returns the exact safety declaration used by the runtime and
// ContractFinal. Management/listing projections must use this instead of
// re-inferring confirmation from the legacy Risk enum.
func EffectiveSafety(s Shortcut) contract.SafetySpec {
	if safetySpecDeclared(s.Safety) {
		return s.Safety
	}
	return shortcutSafetySpec(s.risk())
}

func shortcutExamples(tips []string) string {
	if len(tips) == 0 {
		return ""
	}
	return "  " + strings.Join(tips, "\n  ")
}

func fromShortcutValidate(s Shortcut) func(*cobra.Command, []string) error {
	if s.Validate == nil {
		return nil
	}
	return func(cmd *cobra.Command, _ []string) error {
		return s.Validate(&RuntimeContext{cmd: cmd, shortcut: s})
	}
}

// shortcutSafetySpec is the temporary compatibility boundary while the live
// Shortcut framework still owns its legacy Risk enum. command and Leaf do not
// retain that enum: the adapter expands it once into the existing Schema model.
func shortcutSafetySpec(risk Risk) contract.SafetySpec {
	switch risk {
	case RiskWrite:
		return contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "user_required", Idempotency: "unknown",
		}
	case RiskHighWrite:
		return contract.SafetySpec{
			Effect: "destructive", Risk: "high",
			Confirmation: "user_required", Idempotency: "unknown",
		}
	default:
		return contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		}
	}
}

// shortcutIntentProse returns just the intent/description prose so the
// constraint section is rendered exactly once by corecmd.New.
func shortcutIntentProse(s Shortcut) string {
	prose := strings.TrimSpace(s.Intent)
	if prose == "" {
		prose = strings.TrimSpace(s.Description)
	}
	return prose
}

// fromShortcutFlags maps every Shortcut flag fact into command.
// ValidationShortcut preserves declaration-order Required/Enum checks, the
// historical "the token itself must be present" contract and its exact
// missing-flag message even when a registration default exists.
func fromShortcutFlags(flags []Flag) []corecmd.FlagSpec {
	if len(flags) == 0 {
		return nil
	}
	out := make([]corecmd.FlagSpec, 0, len(flags))
	for _, f := range flags {
		out = append(out, corecmd.FlagSpec{
			Name:           f.Name,
			Shorthand:      f.Shorthand,
			Usage:          flagHelp(f),
			Kind:           fromShortcutFlagKind(f.Type),
			Default:        f.Default,
			Hidden:         f.Hidden,
			Required:       f.Required,
			RequiredWhen:   f.RequiredWhen,
			ValidationMode: corecmd.ValidationShortcut,
			RequiredError:  fmt.Sprintf("缺少必填参数 --%s：%s", f.Name, f.Desc),
			Enum:           append([]string(nil), f.Enum...),
			Aliases:        append([]string(nil), f.Aliases...),
		})
	}
	return out
}

// fromShortcutFlagKind maps the shortcut FlagType to the command FlagKind; an
// empty type defaults to string, matching the Shortcut framework.
func fromShortcutFlagKind(t FlagType) corecmd.FlagKind {
	switch t {
	case FlagBool:
		return corecmd.KindBool
	case FlagInt:
		return corecmd.KindInt
	case FlagStringSlice:
		return corecmd.KindStringSlice
	default:
		return corecmd.KindString
	}
}

// fromShortcutConstraints maps both generic relationships and custom
// declaration/help facts. Custom runtime checks remain in Shortcut.Validate.
func fromShortcutConstraints(constraints []Constraint) []corecmd.Constraint {
	if len(constraints) == 0 {
		return nil
	}
	out := make([]corecmd.Constraint, 0, len(constraints))
	for _, c := range constraints {
		kind, ok := fromShortcutConstraintKind(c.Kind)
		if !ok {
			panic(fmt.Sprintf("unknown shortcut constraint kind %q", c.Kind))
		}
		out = append(out, corecmd.Constraint{
			Kind:        kind,
			Flags:       append([]string(nil), c.Flags...),
			Description: c.Description,
		})
	}
	return out
}

// fromShortcutConstraintKind maps a shortcut ConstraintKind to command.
func fromShortcutConstraintKind(k ConstraintKind) (corecmd.ConstraintKind, bool) {
	switch k {
	case ConstraintAtLeastOne:
		return corecmd.AtLeastOne, true
	case ConstraintExactlyOne:
		return corecmd.ExactlyOne, true
	case ConstraintMutuallyExclusive:
		return corecmd.MutuallyExclusive, true
	case ConstraintCustom:
		return corecmd.Custom, true
	default:
		return "", false
	}
}
