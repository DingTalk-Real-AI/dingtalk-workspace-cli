// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package helpers

import (
	"errors"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
)

var validateRuntimeResult = output.ValidateResult

// ReturnIncompleteResult closes an incomplete-result invocation according to
// the command's output rollout without issuing another business request.
//
// Unified commands return the structured terminal error and publish no
// success-shaped payload. Dual-validation commands validate the once-built
// shadow result, preserve the established legacy bytes, and then return the
// structured terminal error. Legacy commands preserve both their output and
// their historical terminal error (which may be nil).
func ReturnIncompleteResult(
	cmd *cobra.Command,
	shadow output.CommandResult,
	terminalErr error,
	legacyErr error,
	writeLegacy func() error,
) error {
	if output.UsesUnifiedResult(cmd) {
		return terminalErr
	}
	if output.CommandRollout(cmd) == output.RolloutDualValidate {
		if validationErr := validateRuntimeResult(shadow); validationErr != nil {
			return errors.Join(terminalErr, validationErr)
		}
	}
	if outputErr := writeLegacy(); outputErr != nil {
		return outputErr
	}
	if output.CommandRollout(cmd) == output.RolloutDualValidate {
		return terminalErr
	}
	return legacyErr
}
