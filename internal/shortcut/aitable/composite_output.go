// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"fmt"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

const aitableCompositeContractVersion = "aitable.composite.v1"

type compositeResult struct {
	ContractVersion string           `json:"contractVersion"`
	Operation       string           `json:"operation"`
	Status          string           `json:"status"`
	Executed        bool             `json:"executed"`
	Retryable       bool             `json:"retryable"`
	RequestedCount  int              `json:"requestedCount,omitempty"`
	CompletedCount  int              `json:"completedCount,omitempty"`
	FailedCount     int              `json:"failedCount,omitempty"`
	Resolved        map[string]any   `json:"resolved,omitempty"`
	Plan            []compositeStep  `json:"plan,omitempty"`
	CompletedSteps  []compositeStep  `json:"completedSteps,omitempty"`
	Verification    map[string]any   `json:"verification,omitempty"`
	Checkpoint      map[string]any   `json:"checkpoint,omitempty"`
	NextCommand     string           `json:"nextCommand,omitempty"`
	KnownEffects    []map[string]any `json:"knownSideEffects,omitempty"`
	Warnings        []string         `json:"warnings,omitempty"`
	Result          map[string]any   `json:"result,omitempty"`
}

type compositeStep struct {
	Index     int            `json:"index"`
	Name      string         `json:"name"`
	Tool      string         `json:"tool,omitempty"`
	Status    string         `json:"status"`
	Offset    int            `json:"offset,omitempty"`
	Count     int            `json:"count,omitempty"`
	Arguments map[string]any `json:"arguments,omitempty"`
	Result    map[string]any `json:"result,omitempty"`
	Error     string         `json:"error,omitempty"`
}

func newCompositeResult(operation string) compositeResult {
	return compositeResult{
		ContractVersion: aitableCompositeContractVersion,
		Operation:       operation,
		Status:          "success",
		Executed:        true,
	}
}

func compositeError(result compositeResult, cause error, retryable bool) error {
	result.Retryable = retryable
	if result.Status == "success" {
		result.Status = "unknown"
	}
	return apperrors.NewAPI(fmt.Sprintf("AI Table composite %s ended with status %s", result.Operation, result.Status),
		apperrors.WithOperation("aitable."+result.Operation),
		apperrors.WithOrigin("mcp"),
		apperrors.WithFailureStage("composite_execution"),
		apperrors.WithExecutionStarted(result.Executed),
		apperrors.WithRetryable(retryable),
		apperrors.WithReason("aitable_composite_"+result.Status),
		apperrors.WithHint("inspect error.details.result before retrying; unknown means the remote effect could not be proven"),
		apperrors.WithDetails(map[string]any{"result": result}),
		apperrors.WithCause(cause),
	)
}
