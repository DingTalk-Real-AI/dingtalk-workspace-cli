// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"fmt"
	"runtime"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
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

func aitableRecoveryCommand(argv ...string) string {
	return aitableRecoveryCommandForPlatform(runtime.GOOS, argv...)
}

func aitableRecoveryCommandForPlatform(goos string, argv ...string) string {
	quoted := make([]string, len(argv))
	for index, arg := range argv {
		// POSIX 单引号不能保护 cmd.exe；Windows 下只允许跨 shell 均可裸放的
		// 保守白名单值进入可复制命令，其余值改成不含 shell 元字符的占位符。
		if goos == "windows" && helpers.ShellQuoteArg(arg) != arg {
			quoted[index] = aitableRecoveryPlaceholder(argv, index)
			continue
		}
		quoted[index] = helpers.ShellQuoteArg(arg)
	}
	return strings.Join(quoted, " ")
}

func aitableRecoveryPlaceholder(argv []string, index int) string {
	if index > 0 {
		switch argv[index-1] {
		case "--query":
			return "REPLACE_QUERY"
		case "--base-id":
			return "REPLACE_BASE_ID"
		case "--table-id":
			return "REPLACE_TABLE_ID"
		}
	}
	return "REPLACE_VALUE"
}

func compositeError(result compositeResult, cause error, retryable bool) error {
	result.Retryable = retryable
	if result.Status == "success" {
		result.Status = "unknown"
	}
	options := []apperrors.Option{
		apperrors.WithOperation("aitable." + result.Operation),
		apperrors.WithOrigin("mcp"),
		apperrors.WithFailureStage("composite_execution"),
		apperrors.WithExecutionStarted(result.Executed),
		apperrors.WithRetryable(retryable),
		apperrors.WithReason("aitable_composite_" + result.Status),
		apperrors.WithHint("inspect error.details.result before retrying; unknown means the remote effect could not be proven"),
		apperrors.WithDetails(map[string]any{"result": result}),
		apperrors.WithCause(cause),
	}
	if result.NextCommand != "" {
		options = append(options, apperrors.WithActions(result.NextCommand))
	}
	if flags := compositeRecoveryFlags(result.Operation); len(flags) > 0 {
		options = append(options, apperrors.WithAvailableFlags(flags...))
	}
	return apperrors.NewAPI(fmt.Sprintf("AI Table composite %s ended with status %s", result.Operation, result.Status), options...)
}

func compositeRecoveryFlags(operation string) []string {
	switch operation {
	case "base_bootstrap":
		return []string{"name", "folder-id", "template-id", "tables"}
	case "table_bootstrap":
		return []string{"base-id", "name", "fields"}
	default:
		return nil
	}
}
