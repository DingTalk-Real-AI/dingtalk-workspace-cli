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

package corecmd

import (
	"fmt"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const preparedCommandAnnotation = "dws.internal.corecmd.prepared"

// WithValidation compiles a validation boundary into an execution step. A
// validation failure stops the continuation; continuation errors pass through
// unchanged. Both managed commands and metadata-only migration wrappers use
// this function so callers do not reimplement failure/continuation ordering.
func WithValidation(validate, next func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	if next == nil {
		panic("corecmd.WithValidation: next is nil")
	}
	if validate == nil {
		return next
	}
	return func(cmd *cobra.Command, args []string) error {
		if err := apperrors.NormalizeValidation(validate(cmd, args), apperrors.WithReason("invalid_parameters")); err != nil {
			return err
		}
		return next(cmd, args)
	}
}

// PrepareCommandTree installs the framework's Cobra adapters once, after all
// built-in, edition and plugin commands and flag handlers have been mounted.
// It snapshots effective handlers before modifying any node, preserving Cobra's
// nearest-handler semantics without capturing already adapted ancestors.
//
// Commands must not be mounted or have their hooks replaced after preparation;
// transparent lifecycle decorators may chain the installed handlers. Repeated
// execution retains Cobra's flag values and Changed bits: construct a new tree
// for independent invocations. Repeated preparation is a construction error.
func PrepareCommandTree(root *cobra.Command) error {
	if root == nil {
		return fmt.Errorf("corecmd.PrepareCommandTree: root is nil")
	}
	if root.Parent() != nil {
		return fmt.Errorf("corecmd.PrepareCommandTree: %q is not a root", root.CommandPath())
	}
	type commandHooks struct {
		cmd       *cobra.Command
		flagError func(*cobra.Command, error) error
	}
	var hooks []commandHooks
	var collect func(*cobra.Command) error
	collect = func(cmd *cobra.Command) error {
		if cmd.Annotations[preparedCommandAnnotation] != "" {
			return fmt.Errorf("corecmd.PrepareCommandTree: %q is already prepared", cmd.CommandPath())
		}
		hooks = append(hooks, commandHooks{cmd, cmd.FlagErrorFunc()})
		for _, child := range cmd.Commands() {
			if err := collect(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := collect(root); err != nil {
		return err
	}
	for _, hook := range hooks {
		cmd := hook.cmd
		// Native Cobra validation failures use one shared adapter. Inheritance
		// also covers help/completion nodes created later by ExecuteC.
		cmd.SetValidationErrorFunc(normalizeCobraValidationError)
		flagError := hook.flagError
		cmd.SetFlagErrorFunc(func(current *cobra.Command, parserErr error) error {
			if apperrors.PreserveClassification(parserErr) {
				return parserErr
			}
			err := flagError(current, parserErr)
			if err == nil {
				err = parserErr
			}
			// NormalizeValidation also preserves classifications returned by the handler.
			return apperrors.NormalizeValidation(err, apperrors.WithReason("invalid_flag"))
		})
		if cmd.Annotations == nil {
			cmd.Annotations = make(map[string]string)
		}
		cmd.Annotations[preparedCommandAnnotation] = "true"
	}
	return nil
}

// Cobra invokes this only at native validation failures, retaining its original
// lifecycle order and checking required/groups once after business PreRun hooks.
// Args and business hooks themselves are left intact during preparation.
func normalizeCobraValidationError(cmd *cobra.Command, stage cobra.ValidationStage, err error) error {
	if err == nil {
		return nil
	}
	switch stage {
	case cobra.ValidationStageArgs:
		return apperrors.NormalizeValidation(err, apperrors.WithReason("invalid_positionals"))
	case cobra.ValidationStageRequiredFlags:
		if apperrors.PreserveClassification(err) {
			return err
		}
		return normalizeRequiredFlagError(cmd, err)
	case cobra.ValidationStageFlagGroups:
		return apperrors.NormalizeValidation(err, apperrors.WithReason("invalid_flag_group"))
	default:
		// A future dependency stage needs an explicit framework policy.
		return err
	}
}

func normalizeRequiredFlagError(cmd *cobra.Command, err error) error {
	if message := missingRequiredFlagMessage(cmd); message != "" {
		return apperrors.NewValidation(
			message,
			apperrors.WithReason("missing_required_flags"),
			apperrors.WithCause(err),
		)
	}
	return apperrors.NormalizeValidation(err, apperrors.WithReason("missing_required_flags"))
}

func missingRequiredFlagMessage(cmd *cobra.Command) string {
	missing := make([]string, 0)
	cmd.Flags().VisitAll(func(flag *pflag.Flag) {
		required := flag.Annotations[cobra.BashCompOneRequiredFlag]
		if len(required) > 0 && required[0] == "true" && !flag.Changed {
			missing = append(missing, "--"+flag.Name)
		}
	})
	if len(missing) == 0 {
		return ""
	}
	return fmt.Sprintf("missing required flag(s): %s", strings.Join(missing, ", "))
}
