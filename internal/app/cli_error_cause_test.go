// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
)

func TestCrossPlatformCoverageCLIErrorCauseProjection(t *testing.T) {
	err := &helpers.CLIError{Code: "MCP_TOOL_ERROR", Message: "invalid result", Cause: errors.New("dry-run response executed must be present and false"), Details: map[string]any{"commitState": "committed", "verified": false}}
	info := errorInfoFromExecutionError(err)
	raw, marshalErr := json.Marshal(info)
	if marshalErr != nil || strings.Contains(string(raw), "cause") {
		t.Fatalf("internal cause exposed: %s, %v", raw, marshalErr)
	}
	if !reflect.DeepEqual(info.Details, err.Details) {
		t.Fatalf("details lost alongside cause: %#v", info.Details)
	}
	err.Cause = nil
	if errorInfoFromExecutionError(err).Cause != "" {
		t.Fatal("invented cause")
	}
}

func TestCrossPlatformCoverageInternalCausesExcludedFromWire(t *testing.T) {
	for _, secret := range []string{"/private/customer/secret.svg", "Bearer private-cause-canary", "https://internal.example/?authSyncToken=private-canary"} {
		t.Run(secret, func(t *testing.T) {
			cause := errors.New(secret)
			cliErr := &helpers.CLIError{Code: helpers.CodeInvalidParam, Message: "Invalid source", Suggestion: "Check source structure", Cause: cause}
			for _, err := range []error{
				cliErr,
				fmt.Errorf("operation failed: %w", cliErr),
				apperrors.NewValidation("Invalid source", apperrors.WithCause(cause)),
				apperrors.NewValidation("Invalid source", apperrors.WithCause(cliErr)),
				&helpers.CLIError{Code: helpers.CodeInvalidParam, Message: "Invalid source", Cause: apperrors.NewValidation("Invalid source", apperrors.WithCause(cause))},
			} {
				if !errors.Is(err, cause) {
					t.Fatal("internal error chain lost")
				}
				info := errorInfoFromExecutionError(err)
				_, wire := emitChatResultWireForTest(t, output.FailureWithExitCode(info, apperrors.ExitCode(err)))
				raw, marshalErr := json.Marshal(wire)
				if marshalErr != nil || strings.Contains(string(raw), secret) || strings.Contains(string(raw), `"cause"`) {
					t.Fatalf("internal cause leaked in wire: %s, %v", raw, marshalErr)
				}
				if !strings.Contains(string(raw), "Invalid source") {
					t.Fatalf("public diagnostic lost: %s", raw)
				}
			}
		})
	}
}
