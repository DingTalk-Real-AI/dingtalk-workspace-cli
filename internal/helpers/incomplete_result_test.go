// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package helpers

import (
	"errors"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageReturnIncompleteResultHonorsEveryRollout(t *testing.T) {
	terminalErr := errors.New("structured incomplete result")
	legacyErr := errors.New("legacy page failure")
	validShadow := output.Success(map[string]any{"complete": false})

	tests := []struct {
		name       string
		rollout    output.RolloutState
		shadow     output.CommandResult
		legacyErr  error
		writeErr   error
		wantWrite  bool
		wantLegacy bool
		wantExtra  error
	}{
		{name: "unified omits legacy payload", rollout: output.RolloutUnifiedActive, shadow: validShadow},
		{name: "dual validates and writes", rollout: output.RolloutDualValidate, shadow: validShadow, wantWrite: true},
		{name: "dual rejects invalid shadow before writing", rollout: output.RolloutDualValidate, shadow: output.Failure(nil)},
		{name: "legacy preserves historical nil", rollout: output.RolloutLegacyOnly, shadow: validShadow, wantWrite: true, wantLegacy: true},
		{name: "legacy preserves historical error", rollout: output.RolloutLegacyOnly, shadow: validShadow, legacyErr: legacyErr, wantWrite: true, wantLegacy: true, wantExtra: legacyErr},
		{name: "writer error keeps legacy precedence", rollout: output.RolloutDualValidate, shadow: validShadow, writeErr: errors.New("write failed"), wantWrite: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "partial"}
			output.SetCommandRollout(cmd, tc.rollout)
			wrote := false
			got := ReturnIncompleteResult(cmd, tc.shadow, terminalErr, tc.legacyErr, func() error {
				wrote = true
				return tc.writeErr
			})
			if wrote != tc.wantWrite {
				t.Fatalf("write called = %t, want %t", wrote, tc.wantWrite)
			}
			if tc.writeErr != nil {
				if got != tc.writeErr {
					t.Fatalf("result = %v, want exact write error", got)
				}
				return
			}
			if tc.wantLegacy {
				if !errors.Is(got, tc.legacyErr) || (tc.legacyErr == nil && got != nil) {
					t.Fatalf("legacy result = %v, want %v", got, tc.legacyErr)
				}
				return
			}
			if !errors.Is(got, terminalErr) {
				t.Fatalf("result = %v, want terminal error", got)
			}
			if tc.wantExtra != nil && !errors.Is(got, tc.wantExtra) {
				t.Fatalf("result = %v, want legacy error", got)
			}
			if tc.name == "dual rejects invalid shadow before writing" && got == terminalErr {
				t.Fatal("invalid shadow was not joined to terminal error")
			}
		})
	}
}
