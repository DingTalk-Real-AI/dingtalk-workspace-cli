// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package chat

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func TestCrossPlatformCoverageActiveConversationsTotalTimeoutValidation(t *testing.T) {
	for _, timeout := range []string{"0", "-1", "3601"} {
		caller := &larkAlignmentCaller{}
		_, err := executeActiveConversationsForTest(t, caller, map[string]string{"total-timeout": timeout})
		if err == nil || apperrors.ExitCode(err) != apperrors.ExitCodeValidation || !strings.Contains(err.Error(), "--total-timeout") || len(caller.calls) != 0 {
			t.Fatalf("timeout=%s: error=%v calls=%v", timeout, err, caller.calls)
		}
	}
}

func TestCrossPlatformCoverageActiveConversationsTotalTimeoutPreservesBatchWithoutFiles(t *testing.T) {
	t.Chdir(t.TempDir())
	caller := &larkAlignmentCaller{responses: map[string]string{activeConversationsOperation: `{"result":{"conversationMessagesList":[{"openConversationId":"group-1","singleChat":false,"messages":[{"createTime":"2026-09-07T10:00:00+08:00"}]}],"hasMore":true,"nextCursor":"next"}}`}}
	started := time.Now()
	result, err := executeActiveConversationsForTest(t, caller, map[string]string{
		"start": "2026-09-07", "end": "2026-09-08", "page-delay": "60000", "total-timeout": "1",
	})
	if err != nil || time.Since(started) > 5*time.Second || len(caller.calls) != 1 || result["outcome"] != "partial_failure" {
		t.Fatalf("timeout did not preserve partial output: error=%v calls=%v result=%#v", err, caller.calls, result)
	}
	batch := result["data"].(map[string]any)["succeeded"].([]any)[0].(map[string]any)
	if batch["complete"] != false || batch["count"] != float64(1) || batch["pagesFetched"] != float64(1) || result["meta"].(map[string]any)["pagination"].(map[string]any)["next_token"] != "next" {
		t.Fatalf("lost progress or retry cursor: %#v", result)
	}
	for _, field := range []string{"checkpointFile", "totalPagesFetched"} {
		if _, present := batch[field]; present {
			t.Fatalf("removed persistence field returned: %s", field)
		}
	}
	entries, err := os.ReadDir(".")
	if err != nil || len(entries) != 0 {
		t.Fatalf("query unexpectedly wrote progress files: %v, %v", entries, err)
	}
}

func TestCrossPlatformCoverageActiveConversationsRejectsLateResponse(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	caller := &activeConversationsObservedCaller{
		larkAlignmentCaller: &larkAlignmentCaller{responses: map[string]string{activeConversationsOperation: `{"result":{"conversationMessagesList":[],"hasMore":false}}`}},
		afterRead:           cancel,
	}
	if _, err := executeActiveConversationsContextForTest(t, ctx, caller, map[string]string{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("late response bypassed cancellation: %v", err)
	}
	cmd := corecmd.New(shortcut.FromShortcut(ActiveConversations))
	if err := cmd.Flags().Set("start", "invalid"); err != nil {
		t.Fatal(err)
	}
	// Standalone Execute must still validate its range with an absent context.
	if err := executeActiveConversations(shortcut.RuntimeContextForTest(cmd, ActiveConversations)); err == nil {
		t.Fatal("standalone Execute accepted an invalid query")
	}
}
