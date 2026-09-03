// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"errors"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"testing"
)

func TestCrossPlatformCoverageAITableDateFilterInputErrorCategory(t *testing.T) {
	caller := &aitableTestCaller{responses: []string{`{"data":{"fields":[{"fieldId":"date","type":"date"}]}}`}}
	err := runAitableCoverageCommand(t, caller, "view", "update", "filter", "--base-id=b", "--table-id=t", "--view-id=view", `--json=[{"operator":"date_eq","operands":["date",{"type":"relative","period":"day","offset":"0"}]}]`)
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Category != apperrors.CategoryValidation || len(caller.calls) != 1 {
		t.Fatalf("error=%#v calls=%v", err, caller.calls)
	}
}
