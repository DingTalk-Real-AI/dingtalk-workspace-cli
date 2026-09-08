// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package chat

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func TestCrossPlatformCoverageActiveConversationsRejectsFractionalBoundariesBeforeRead(t *testing.T) {
	for _, tc := range []struct{ name, start, end, want string }{
		{"fractional start", "2026-09-07T10:00:00.900+08:00", "2026-09-07T12:00:00+08:00", "--start 仅支持整秒"},
		{"fractional end", "2026-09-07T10:00:00+08:00", "2026-09-07T12:00:00.900+08:00", "--end 仅支持整秒"},
		{"same second", "2026-09-07T10:00:00.100+08:00", "2026-09-07T10:00:00.900+08:00", "--start 仅支持整秒"},
		{"local fractional start", "2026-09-07 10:00:00.900", "2026-09-07 12:00:00", "--start 仅支持整秒"},
		{"comma fractional end", "2026-09-07 10:00:00", "2026-09-07 12:00:00,900", "--end 仅支持整秒"},
		{"below nanosecond start", "2026-09-07T10:00:00.0000000001+08:00", "2026-09-07T12:00:00+08:00", "--start 仅支持整秒"},
		{"below nanosecond end", "2026-09-07T10:00:00+08:00", "2026-09-07T12:00:00.0000000001+08:00", "--end 仅支持整秒"},
		{"invalid end", "2026-09-07", "not-a-time", "--end 必须是 RFC3339"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &larkAlignmentCaller{}
			_, err := executeActiveConversationsForTest(t, caller, map[string]string{"start": tc.start, "end": tc.end})
			if err == nil || apperrors.ExitCode(err) != apperrors.ExitCodeValidation || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want validation containing %q", err, tc.want)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("invalid boundary reached MCP: %#v", caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageActiveConversationsDefaultEndUsesEffectiveWholeSeconds(t *testing.T) {
	now := time.Date(2026, 9, 7, 12, 0, 0, 900_000_000, activeConversationsLocation)
	query, err := resolveActiveConversationTimeRange("2026-09-07 11:59:59", "", now)
	if err != nil || !query.end.Equal(now.Truncate(time.Second)) || query.end.Nanosecond() != 0 {
		t.Fatalf("default end = %#v, err = %v", query, err)
	}
	for _, start := range []string{"2026-09-07 12:00:00", "2026-09-07 12:00:01"} {
		if _, err := resolveActiveConversationTimeRange(start, "", now); err == nil || !strings.Contains(err.Error(), "晚于") {
			t.Fatalf("effective empty/reversed range accepted for %q: %v", start, err)
		}
	}
	for _, end := range []string{"2026-09-07T12:00:00.000+08:00", "2026-09-07T04:00:00.0000000000Z", "2026-09-07 12:00:00,000"} {
		query, err := resolveActiveConversationTimeRange("2026-09-07T11:59:59.000+08:00", end, now)
		if err != nil || query.start.Nanosecond() != 0 || !query.end.Equal(now.Truncate(time.Second)) {
			t.Fatalf("integral boundary %q = %#v, err=%v", end, query, err)
		}
	}
}

func TestCrossPlatformCoverageActiveConversationsBoundaryRoundTripPreservesMessagePrecision(t *testing.T) {
	caller := &larkAlignmentCaller{responses: map[string]string{
		activeConversationsOperation: `{"result":{"conversationMessagesList":[{"openConversationId":"cid-fractional-message","singleChat":true,"messages":[{"createTime":"2026-09-07T10:00:00.123+08:00"}]}],"hasMore":false}}`,
	}}
	envelope, err := executeActiveConversationsForTest(t, caller, map[string]string{
		"start": "2026-09-07T02:00:00.000Z", "end": "2026-09-07T04:00:00.000Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	data := envelope["data"].(map[string]any)
	if caller.calls[0].args["startTime"] != "2026-09-07 10:00:00" || caller.calls[0].args["endTime"] != "2026-09-07 12:00:00" ||
		data["start"] != "2026-09-07T10:00:00+08:00" || data["end"] != "2026-09-07T12:00:00+08:00" {
		t.Fatalf("effective range mismatch: calls=%#v data=%#v", caller.calls, data)
	}
	row := data["conversations"].([]any)[0].(map[string]any)
	if row["latestMessageTime"] != "2026-09-07T10:00:00.123+08:00" {
		t.Fatalf("message precision lost: %#v", row)
	}
}

func TestCrossPlatformCoverageActiveConversationsKeepsValidatedDefaultEnd(t *testing.T) {
	caller := &larkAlignmentCaller{responses: map[string]string{
		activeConversationsOperation: `{"result":{"conversationMessagesList":[],"hasMore":false}}`,
	}}
	helpers.InitDepsForTest(t, caller)
	cmd := corecmd.New(shortcut.FromShortcut(ActiveConversations))
	ctx, _ := output.WithResultStore(context.Background())
	cmd.SetContext(ctx)
	if err := cmd.Flags().Set("start", time.Now().Add(-time.Hour).Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	rt := shortcut.RuntimeContextForTest(cmd, ActiveConversations)
	if err := ActiveConversations.Validate(rt); err != nil {
		t.Fatal(err)
	}
	validated, err := activeConversationExecutionRange(rt)
	if err != nil || validated.end.Nanosecond() != 0 {
		t.Fatalf("validated range=%#v err=%v", validated, err)
	}
	// Cross the next second to catch an Execute hook recomputing time.Now().
	time.Sleep(time.Until(validated.end.Add(time.Second)))
	if err := ActiveConversations.Execute(rt); err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 || caller.calls[0].args["endTime"] != validated.end.In(activeConversationsLocation).Format("2006-01-02 15:04:05") {
		t.Fatalf("Execute changed the validated end: %#v", caller.calls)
	}
	// A later validation on a reused command replaces, rather than reuses, it.
	newEnd := validated.end.Add(time.Hour)
	if err := cmd.Flags().Set("end", newEnd.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if err := ActiveConversations.Validate(rt); err != nil {
		t.Fatal(err)
	}
	updated, err := activeConversationExecutionRange(rt)
	if err != nil || !updated.end.Equal(newEnd) {
		t.Fatalf("stale range=%#v err=%v", updated, err)
	}
}

func TestCrossPlatformCoverageActiveConversationsStandaloneExecuteValidatesTime(t *testing.T) {
	cmd := corecmd.New(shortcut.FromShortcut(ActiveConversations))
	rt := shortcut.RuntimeContextForTest(cmd, ActiveConversations)
	// A command without a prepared context still fails closed on invalid time.
	if _, err := activeConversationExecutionRange(rt); err == nil {
		t.Fatal("blank start accepted")
	}
	if err := cmd.Flags().Set("start", "2026-09-07"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("end", "2026-09-08"); err != nil {
		t.Fatal(err)
	}
	if err := ActiveConversations.Validate(rt); err != nil || cmd.Context() == nil {
		t.Fatalf("validation did not initialize a command context: %v", err)
	}
	cmd.SetContext(context.Background())
	if err := cmd.Flags().Set("start", "2026-09-07T10:00:00.100+08:00"); err != nil {
		t.Fatal(err)
	}
	if err := ActiveConversations.Execute(rt); err == nil || !strings.Contains(err.Error(), "整秒") {
		t.Fatalf("standalone Execute accepted a fractional boundary: %v", err)
	}
}
