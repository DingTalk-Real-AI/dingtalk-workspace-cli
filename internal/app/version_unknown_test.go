// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	stderrors "errors"
	"fmt"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageVersionUnknownKeepsParserEvidence(t *testing.T) {
	cmd := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true, Run: func(*cobra.Command, []string) { t.Fatal("unknown flag executed") }}
	var original error
	cmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		original = err
		return flagErrorWithSuggestions(cmd, err)
	})
	cmd.SetArgs([]string{"--json"})
	err := corecmd.ExecuteForTest(cmd)
	var structured *apperrors.Error
	if !isUnknownInvocationError(err) || !stderrors.As(err, &structured) || structured.Cause != original || !stderrors.Is(err, original) {
		t.Fatalf("parser evidence lost: %v", err)
	}
	if apperrors.ExitCode(err) != 3 || structured.Hint == "" || structured.Reason != "unknown_flag" {
		t.Fatalf("parser presentation changed: %#v", structured)
	}
	resolution := cmdutil.NewCommandResolution(cmd, "missing", cmdutil.ResolutionUnknownSubcommand, []string{"existing"}, "").Err()
	if !isUnknownInvocationError(fmt.Errorf("preparse: %w", resolution)) {
		t.Fatal("preparse resolution marker was lost")
	}
	for _, err := range []error{nil, stderrors.New("unknown command missing"), apperrors.NewValidation("unknown flag: --json", apperrors.WithReason("unknown_flag"))} {
		if isUnknownInvocationError(err) {
			t.Fatalf("inferred parser origin from business error: %v", err)
		}
	}
}
