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
		args      cobra.PositionalArgs
		flagError func(*cobra.Command, error) error
		preRunE   func(*cobra.Command, []string) error
		preRun    func(*cobra.Command, []string)
	}
	var hooks []commandHooks
	var collect func(*cobra.Command) error
	collect = func(cmd *cobra.Command) error {
		if cmd.Annotations[preparedCommandAnnotation] != "" {
			return fmt.Errorf("corecmd.PrepareCommandTree: %q is already prepared", cmd.CommandPath())
		}
		hooks = append(hooks, commandHooks{cmd, cmd.Args, cmd.FlagErrorFunc(), cmd.PreRunE, cmd.PreRun})
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
		if hook.args != nil {
			cmd.Args = func(current *cobra.Command, args []string) error {
				return apperrors.NormalizeValidation(hook.args(current, args), apperrors.WithReason("invalid_positionals"))
			}
		}
		cmd.SetFlagErrorFunc(func(current *cobra.Command, parserErr error) error {
			if apperrors.PreserveClassification(parserErr) {
				return parserErr
			}
			err := hook.flagError(current, parserErr)
			if err == nil {
				err = parserErr
			}
			return apperrors.NormalizeValidation(err, apperrors.WithReason("invalid_flag"))
		})
		cmd.PreRun = nil
		if hook.preRunE == nil && hook.preRun == nil {
			cmd.PreRunE = validateCobraFlagConstraints
		} else {
			cmd.PreRunE = func(current *cobra.Command, args []string) error {
				if hook.preRunE != nil {
					if err := hook.preRunE(current, args); err != nil {
						return err
					}
				} else {
					hook.preRun(current, args)
				}
				return validateCobraFlagConstraints(current, args)
			}
		}
		if cmd.Annotations == nil {
			cmd.Annotations = make(map[string]string)
		}
		cmd.Annotations[preparedCommandAnnotation] = "true"
	}
	return nil
}

// Cobra runs its own required/group checks again after PreRun. Keep their
// annotations intact for help, completion and Schema. This early check exists
// only to type errors at their source, after business PreRun alias resolution.
func validateCobraFlagConstraints(current *cobra.Command, _ []string) error {
	if err := current.ValidateRequiredFlags(); err != nil {
		return normalizeRequiredFlagError(current, err)
	}
	return apperrors.NormalizeValidation(current.ValidateFlagGroups(), apperrors.WithReason("invalid_flag_group"))
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
