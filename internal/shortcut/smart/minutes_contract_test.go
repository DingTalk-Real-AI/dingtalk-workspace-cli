// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package smart

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/minutesdata"
	"github.com/spf13/cobra"
)

type transcriptFailWriter struct{}

func (transcriptFailWriter) Write([]byte) (int, error) {
	return 0, errors.New("fixture output failure")
}

func TestCrossPlatformCoverageMinutesResultContracts(t *testing.T) {
	if Transcript.OutputRollout != output.RolloutUnifiedActive {
		t.Fatalf("transcript rollout=%q, want unified_active", Transcript.OutputRollout)
	}
	transcriptResult, err := contract.NormalizeResultSpec(Transcript.Contract.Result, "minutes.shortcut_transcript")
	if err != nil {
		t.Fatalf("normalize transcript result: %v", err)
	}
	if transcriptResult == nil {
		t.Fatal("transcript result contract is missing")
	}
	if strings.Contains(string(transcriptResult.DataSchema), `"nextToken"`) {
		t.Fatal("transcript Result data_schema leaked pagination nextToken")
	}
	pagination, err := contract.NormalizePaginationSpec(Transcript.Contract.Pagination, "minutes.shortcut_transcript")
	if err != nil {
		t.Fatalf("normalize transcript pagination: %v", err)
	}
	if pagination == nil || pagination.CursorParameter != "cursor" {
		t.Fatalf("transcript pagination = %#v", pagination)
	}
	if MinutesDetail.Contract.Result == nil {
		t.Fatal("detail result contract is missing")
	}
	if _, err := contract.NormalizeResultSpec(MinutesDetail.Contract.Result, "minutes.shortcut_detail"); err != nil {
		t.Fatalf("normalize detail result: %v", err)
	}
}

func TestCrossPlatformCoverageMinutesTranscriptResultDefensiveBranches(t *testing.T) {
	legacy := &cobra.Command{Use: "legacy"}
	var legacyOut bytes.Buffer
	legacy.SetOut(&legacyOut)
	legacyRT := shortcut.RuntimeContextForTest(legacy, Transcript)
	readErr := errors.New("fixture read failure")
	partial := minutesdata.TranscriptResult{Pages: 1, NextToken: "n2"}
	if err := outputMinutesTranscriptResult(legacyRT, map[string]any{"taskUuid": "u1"}, partial, readErr); err == nil {
		t.Fatal("legacy partial transcript accepted")
	}
	if err := outputMinutesTranscriptResult(legacyRT, map[string]any{"taskUuid": "u1"}, minutesdata.TranscriptResult{Complete: true}, nil); err != nil {
		t.Fatalf("legacy complete transcript: %v", err)
	}

	failing := &cobra.Command{Use: "legacy-failing"}
	failing.SetOut(transcriptFailWriter{})
	if err := outputMinutesTranscriptResult(shortcut.RuntimeContextForTest(failing, Transcript), map[string]any{"taskUuid": "u1"}, minutesdata.TranscriptResult{}, nil); err == nil {
		t.Fatal("legacy output failure accepted")
	}

	newRuntime := func() *shortcut.RuntimeContext {
		cmd := &cobra.Command{Use: "unified"}
		ctx, _ := output.WithResultStore(context.Background())
		cmd.SetContext(ctx)
		output.SetCommandRollout(cmd, output.RolloutUnifiedActive)
		return shortcut.RuntimeContextForTest(cmd, Transcript)
	}
	invalid := minutesdata.TranscriptResult{Complete: true, NextToken: "unexpected"}
	if err := outputMinutesTranscriptResult(newRuntime(), map[string]any{"taskUuid": "u1"}, invalid, readErr); err != nil {
		t.Fatalf("invalid partial transcript should store failure without pagination: %v", err)
	}
	if err := outputMinutesTranscriptResult(newRuntime(), map[string]any{"taskUuid": "u1"}, invalid, nil); err == nil {
		t.Fatal("inconsistent unified pagination accepted")
	}
}
