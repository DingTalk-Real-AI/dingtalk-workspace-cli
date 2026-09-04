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

const (
	validationArgsAdapterAnnotation = "dws.runtime.validation_args_adapter"
	validationFlagAdapterAnnotation = "dws.runtime.validation_flag_adapter"
	validationConstraintAnnotation  = "dws.runtime.validation_constraint_adapter"
)

// InstallValidationAdapters installs typed-error boundaries on the final Cobra
// tree. It must run after built-in, edition, and plugin commands are mounted so
// every positional validator, flag parser, and Cobra flag constraint shares the
// same validation contract.
func InstallValidationAdapters(root *cobra.Command) {
	if root == nil {
		return
	}
	var visit func(*cobra.Command)
	visit = func(cmd *cobra.Command) {
		installCommandValidationAdapters(cmd, true)
		for _, child := range cmd.Commands() {
			visit(child)
		}
	}
	visit(root)
}

// installLocalValidationAdapters covers constraints owned by a corecmd command
// even when it is executed standalone in an embedding process. Flag parsing is
// deliberately installed only on the final tree so a command mounted later can
// still inherit the root's richer flag error handler.
func installLocalValidationAdapters(cmd *cobra.Command) {
	installCommandValidationAdapters(cmd, false)
}

func installCommandValidationAdapters(cmd *cobra.Command, includeFlagParser bool) {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	if positional := cmd.Args; positional != nil && cmd.Annotations[validationArgsAdapterAnnotation] != "true" {
		cmd.Args = func(current *cobra.Command, args []string) error {
			return apperrors.NormalizeValidation(
				positional(current, args),
				apperrors.WithReason("invalid_positionals"),
			)
		}
		cmd.Annotations[validationArgsAdapterAnnotation] = "true"
	}

	if includeFlagParser && cmd.Annotations[validationFlagAdapterAnnotation] != "true" {
		flagError := cmd.FlagErrorFunc()
		cmd.SetFlagErrorFunc(func(current *cobra.Command, err error) error {
			return apperrors.NormalizeValidation(
				flagError(current, err),
				apperrors.WithReason("invalid_flag"),
			)
		})
		cmd.Annotations[validationFlagAdapterAnnotation] = "true"
	}

	if cmd.Annotations[validationConstraintAnnotation] == "true" {
		return
	}

	preRunE := cmd.PreRunE
	preRun := cmd.PreRun
	cmd.PreRun = nil
	cmd.PreRunE = func(current *cobra.Command, args []string) error {
		if preRunE != nil {
			if err := preRunE(current, args); err != nil {
				// PreRunE is a general lifecycle hook; only explicit validation
				// boundaries may change an error's category and exit code.
				return err
			}
		} else if preRun != nil {
			preRun(current, args)
		}
		if err := current.ValidateRequiredFlags(); err != nil {
			return normalizeRequiredFlagError(current, err)
		}
		return apperrors.NormalizeValidation(
			current.ValidateFlagGroups(),
			apperrors.WithReason("invalid_flag_group"),
		)
	}
	cmd.Annotations[validationConstraintAnnotation] = "true"
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
