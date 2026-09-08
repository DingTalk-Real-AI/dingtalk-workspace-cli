// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func executeActiveConversationsForTest(t *testing.T, caller edition.ToolCaller, values map[string]string) (map[string]any, error) {
	t.Helper()
	return executeActiveConversationsContextForTest(t, context.Background(), caller, values)
}

func executeActiveConversationsContextForTest(t *testing.T, parent context.Context, caller edition.ToolCaller, values map[string]string) (map[string]any, error) {
	t.Helper()
	helpers.InitDepsForTest(t, caller)
	cmd := corecmd.New(shortcut.FromShortcut(ActiveConversations))
	if err := cmd.Flags().Set("page-delay", "0"); err != nil {
		t.Fatal(err)
	}
	for name, value := range values {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("set --%s: %v", name, err)
		}
	}
	ctx, _ := output.WithResultStore(parent)
	cmd.SetContext(ctx)
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	rt := shortcut.RuntimeContextForTest(cmd, ActiveConversations)
	if err := ActiveConversations.Validate(rt); err != nil {
		return nil, err
	}
	if err := ActiveConversations.Execute(rt); err != nil {
		return nil, err
	}
	code, emitted, err := output.EmitStoredResult(cmd)
	if err != nil || !emitted {
		t.Fatalf("emit result: code=%d emitted=%t err=%v", code, emitted, err)
	}
	var envelope map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout.String())
	}
	wantCode := 0
	if envelope["outcome"] == "partial_failure" {
		wantCode = apperrors.ExitCodePartial
		if envelope["ok"] != false || envelope["error"] != nil {
			t.Fatalf("partial envelope = %#v", envelope)
		}
	}
	if code != wantCode || stderr.Len() != 0 {
		t.Fatalf("code=%d want=%d stderr=%s", code, wantCode, stderr.String())
	}
	return envelope, nil
}

func TestCrossPlatformCoverageActiveConversationsAggregatesAllPages(t *testing.T) {
	caller := &larkAlignmentCaller{sequenceResponses: map[string][]string{
		"chat/search_messages_by_time_range": {
			`{"result":{"conversationMessagesList":[{"openConversationId":"cid-group","title":"项目群","singleChat":false,"messages":[{"openMessageId":"m1","createTime":1788746400000}]},{"openConversationId":"cid-direct","title":"张三","singleChat":true,"messages":[{"openMessageId":"m2","sendTime":"2026-09-07 11:00:00"}]}],"hasMore":true,"nextCursor":"cursor-2"}}`,
			`{"result":{"conversationMessagesList":[{"openConversationId":"cid-group","title":"项目群（新名称）","singleChat":false,"messages":[{"openMessageId":"m3","createTime":"2026-09-07T12:00:00+08:00"}]},{"openConversationId":"cid-unknown","messages":[{"openMessageId":"m4","timestamp":1788742800}]}],"hasMore":false}}`,
		},
	}}
	envelope, err := executeActiveConversationsForTest(t, caller, map[string]string{
		"start": "2026-09-07T00:00:00+08:00",
		"end":   "2026-09-08T00:00:00+08:00",
		"limit": "2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if envelope["ok"] != true || envelope["outcome"] != "success" {
		t.Fatalf("envelope = %#v", envelope)
	}
	data := envelope["data"].(map[string]any)
	if data["count"] != float64(3) || data["complete"] != true || data["pagesFetched"] != float64(2) || data["unknownTypeCount"] != float64(1) {
		t.Fatalf("data = %#v", data)
	}
	conversations := data["conversations"].([]any)
	group := conversations[0].(map[string]any)
	direct := conversations[1].(map[string]any)
	unknown := conversations[2].(map[string]any)
	if group["conversationId"] != "cid-group" || group["name"] != "项目群（新名称）" || group["type"] != "group" || group["latestMessageTime"] != "2026-09-07T12:00:00+08:00" {
		t.Fatalf("group = %#v", group)
	}
	if direct["conversationId"] != "cid-direct" || direct["type"] != "direct" || direct["latestMessageTime"] != "2026-09-07T11:00:00+08:00" {
		t.Fatalf("direct = %#v", direct)
	}
	if unknown["conversationId"] != "cid-unknown" || unknown["type"] != "unknown" {
		t.Fatalf("unknown = %#v", unknown)
	}
	if unknown["name"] != "" || unknown["nameKnown"] != false || group["nameKnown"] != true || data["pageSize"] != float64(2) {
		t.Fatalf("name/page-size projection = %#v", data)
	}
	if len(caller.calls) != 2 || caller.calls[0].product != "chat" || caller.calls[0].tool != "search_messages_by_time_range" || caller.calls[0].args["cursor"] != "0" || caller.calls[1].args["cursor"] != "cursor-2" {
		t.Fatalf("calls = %#v", caller.calls)
	}
	if caller.calls[0].args["startTime"] != "2026-09-07 00:00:00" || caller.calls[0].args["endTime"] != "2026-09-08 00:00:00" {
		t.Fatalf("range args = %#v", caller.calls[0].args)
	}
	meta := envelope["meta"].(map[string]any)
	pagination := meta["pagination"].(map[string]any)
	if pagination["endpoint_exhausted"] != true || pagination["pages"] != float64(2) || pagination["items"] != float64(3) {
		t.Fatalf("pagination = %#v", pagination)
	}
}

func TestCrossPlatformCoverageActiveConversationsPageLimitIsResumable(t *testing.T) {
	caller := &larkAlignmentCaller{responses: map[string]string{
		"chat/search_messages_by_time_range": `{"result":{"conversationMessagesList":[{"openConversationId":"cid-1","title":"群一","singleChat":false,"messages":[{"createTime":"2026-09-07 10:00:00"}]}],"hasMore":true,"nextCursor":"cursor-2"}}`,
	}}
	envelope, err := executeActiveConversationsForTest(t, caller, map[string]string{
		"start":      "2026-09-07 00:00:00",
		"end":        "2026-09-08 00:00:00",
		"page-limit": "1",
	})
	if err != nil {
		t.Fatal(err)
	}
	data := envelope["data"].(map[string]any)
	pagination := envelope["meta"].(map[string]any)["pagination"].(map[string]any)
	if data["complete"] != false || pagination["endpoint_exhausted"] != false || pagination["next_token"] != "cursor-2" || pagination["pages"] != float64(1) {
		t.Fatalf("envelope = %#v", envelope)
	}
}

func TestCrossPlatformCoverageActiveConversationsResumedBatchIsNotGloballyComplete(t *testing.T) {
	caller := &larkAlignmentCaller{responses: map[string]string{
		"chat/search_messages_by_time_range": `{"result":{"conversationMessagesList":[{"openConversationId":"cid-2","singleChat":true,"messages":[{"createTime":"2026-09-07 11:00:00"}]}],"hasMore":false}}`,
	}}
	envelope, err := executeActiveConversationsForTest(t, caller, map[string]string{
		"start":  "2026-09-07 00:00:00",
		"end":    "2026-09-08 00:00:00",
		"cursor": "cursor-2",
	})
	if err != nil {
		t.Fatal(err)
	}
	data := envelope["data"].(map[string]any)
	pagination := envelope["meta"].(map[string]any)["pagination"].(map[string]any)
	if data["complete"] != false || pagination["endpoint_exhausted"] != true {
		t.Fatalf("resumed envelope = %#v", envelope)
	}
}

func TestCrossPlatformCoverageActiveConversationsEmptyResult(t *testing.T) {
	for name, response := range map[string]string{
		"explicit empty":    `{"result":{"conversationMessagesList":[],"hasMore":false}}`,
		"terminal omission": `{"result":{"hasMore":false}}`,
		"terminal null":     `{"result":{"conversationMessagesList":null,"hasMore":false}}`,
	} {
		t.Run(name, func(t *testing.T) {
			envelope, err := executeActiveConversationsForTest(t, &larkAlignmentCaller{responses: map[string]string{
				"chat/search_messages_by_time_range": response,
			}}, map[string]string{
				"start": "2026-09-07T00:00:00+08:00",
				"end":   "2026-09-08T00:00:00+08:00",
			})
			if err != nil {
				t.Fatal(err)
			}
			data := envelope["data"].(map[string]any)
			if data["count"] != float64(0) || data["complete"] != true || len(data["conversations"].([]any)) != 0 {
				t.Fatalf("data = %#v", data)
			}
		})
	}
}

func TestCrossPlatformCoverageActiveConversationsFailsClosedOnInvalidResponses(t *testing.T) {
	tests := map[string][]string{
		"missing result":       {`{"success":true}`},
		"missing pagination":   {`{"result":{"conversationMessagesList":[]}}`},
		"missing cursor":       {`{"result":{"conversationMessagesList":[],"hasMore":true}}`},
		"missing conversation": {`{"result":{"conversationMessagesList":[{"messages":[{"createTime":1788746400000}]}],"hasMore":false}}`},
		"empty messages":       {`{"result":{"conversationMessagesList":[{"openConversationId":"cid-1","messages":[]}],"hasMore":false}}`},
		"invalid time":         {`{"result":{"conversationMessagesList":[{"openConversationId":"cid-1","messages":[{"createTime":"bad"}]}],"hasMore":false}}`},
		"malformed type":       {`{"result":{"conversationMessagesList":[{"openConversationId":"cid-1","singleChat":"false","messages":[{"createTime":1788746400000}]}],"hasMore":false}}`},
		"stalled cursor":       {`{"result":{"conversationMessagesList":[],"hasMore":true,"nextCursor":"0"}}`},
		"object cursor":        {`{"result":{"conversationMessagesList":[],"hasMore":true,"nextCursor":{"cursor":1}}}`},
		"boolean cursor":       {`{"result":{"conversationMessagesList":[],"hasMore":true,"nextCursor":true}}`},
		"fractional cursor":    {`{"result":{"conversationMessagesList":[],"hasMore":true,"nextCursor":1.5}}`},
		"inexact cursor":       {`{"result":{"conversationMessagesList":[],"hasMore":true,"nextCursor":9007199254740993}}`},
	}
	for name, responses := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := executeActiveConversationsForTest(t, &larkAlignmentCaller{sequenceResponses: map[string][]string{
				"chat/search_messages_by_time_range": responses,
			}}, map[string]string{
				"start": "2026-09-07T00:00:00+08:00",
				"end":   "2026-09-08T00:00:00+08:00",
			})
			if err == nil {
				t.Fatal("invalid response unexpectedly succeeded")
			}
		})
	}
}

func TestCrossPlatformCoverageActiveConversationsValidationAndContract(t *testing.T) {
	if ActiveConversations.OutputRollout != output.RolloutUnifiedActive || ActiveConversations.Contract.Result == nil || ActiveConversations.Contract.Pagination == nil || ActiveConversations.Safety.Effect != "read" {
		t.Fatalf("declaration is incomplete: %#v", ActiveConversations)
	}
	if !json.Valid(ActiveConversations.Contract.Result.DataSchema) || ActiveConversations.Contract.Identity.CLIPath != "chat +active-conversations" {
		t.Fatal("result schema or command identity is invalid")
	}
	tests := []struct {
		name   string
		values map[string]string
		want   string
	}{
		{name: "blank start", values: map[string]string{"start": "   "}, want: "不能为空白"},
		{name: "bad start", values: map[string]string{"start": "bad"}, want: "RFC3339"},
		{name: "reversed range", values: map[string]string{"start": "2026-09-08", "end": "2026-09-07"}, want: "晚于"},
		{name: "bad limit", values: map[string]string{"start": "2026-09-07", "limit": "0"}, want: "1-100"},
		{name: "bad page limit", values: map[string]string{"start": "2026-09-07", "page-limit": "501"}, want: "1-500"},
		{name: "negative delay", values: map[string]string{"start": "2026-09-07", "page-delay": "-1"}, want: "0-60000"},
		{name: "excessive delay", values: map[string]string{"start": "2026-09-07", "page-delay": "60001"}, want: "0-60000"},
		{name: "cursor without fixed end", values: map[string]string{"start": "2026-09-07", "cursor": "next"}, want: "同一 --end"},
		{name: "cursor with empty end", values: map[string]string{"start": "2026-09-07", "cursor": "next", "end": ""}, want: "同一 --end"},
		{name: "cursor with whitespace end", values: map[string]string{"start": "2026-09-07", "cursor": "next", "end": "   "}, want: "同一 --end"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd := corecmd.New(shortcut.FromShortcut(ActiveConversations))
			for name, value := range test.values {
				if err := cmd.Flags().Set(name, value); err != nil {
					t.Fatal(err)
				}
			}
			err := ActiveConversations.Validate(shortcut.RuntimeContextForTest(cmd, ActiveConversations))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("err=%v, want substring %q", err, test.want)
			}
		})
	}
}

func TestCrossPlatformCoverageActiveConversationsLaterPageFailurePreservesVerifiedBatch(t *testing.T) {
	first := `{"result":{"conversationMessagesList":[{"openConversationId":"cid-1","title":"原名称","singleChat":false,"messages":[{"createTime":"2026-09-07 10:00:00"}]}],"hasMore":true,"nextCursor":"cursor-2"}}`
	for name, second := range map[string]string{
		"read failure":          "",
		"invalid page":          `{"result":{"hasMore":"false"}}`,
		"malformed later group": `{"result":{"conversationMessagesList":[{"openConversationId":"cid-1","title":"不可提交的名称","singleChat":false,"messages":[{"createTime":"2026-09-07 12:00:00"}]},{"openConversationId":"cid-2","messages":[]}],"hasMore":false}}`,
		"conflicting type":      `{"result":{"conversationMessagesList":[{"openConversationId":"cid-1","singleChat":true,"messages":[{"createTime":"2026-09-07 12:00:00"}]}],"hasMore":false}}`,
		"rewound cursor":        `{"result":{"conversationMessagesList":[{"openConversationId":"cid-2","messages":[{"createTime":"2026-09-07 12:00:00"}]}],"hasMore":true,"nextCursor":"0"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			caller := &larkAlignmentCaller{sequenceResponses: map[string][]string{
				activeConversationsOperation: {first, second},
			}}
			if second == "" {
				caller.failProductToolAt = map[string]int{activeConversationsOperation: 2}
			}
			envelope, err := executeActiveConversationsForTest(t, caller, map[string]string{
				"start": "2026-09-07", "end": "2026-09-08", "limit": "10",
			})
			if err != nil {
				t.Fatal(err)
			}
			batch := assertActiveConversationsPartialForTest(t, envelope, 1, 1, "cursor-2")
			row := batch["conversations"].([]any)[0].(map[string]any)
			if row["conversationId"] != "cid-1" || row["name"] != "原名称" || row["type"] != "group" || row["latestMessageTime"] != "2026-09-07T10:00:00+08:00" {
				t.Fatalf("failed page polluted verified state: %#v", row)
			}
			if batch["pageSize"] != float64(10) || len(caller.calls) != 2 {
				t.Fatalf("batch=%#v calls=%#v", batch, caller.calls)
			}
		})
	}
}

func assertActiveConversationsPartialForTest(t *testing.T, envelope map[string]any, pages, count int, cursor string) map[string]any {
	t.Helper()
	if envelope["ok"] != false || envelope["outcome"] != "partial_failure" {
		t.Fatalf("expected partial failure: %#v", envelope)
	}
	data := envelope["data"].(map[string]any)
	succeeded := data["succeeded"].([]any)
	failed := data["failed"].([]any)
	if data["total"] != float64(2) || len(succeeded) != 1 || len(failed) != 1 || len(data["unknown"].([]any)) != 0 {
		t.Fatalf("partial ledger = %#v", data)
	}
	batch := succeeded[0].(map[string]any)
	if batch["id"] != "completed-pages" || batch["count"] != float64(count) || batch["pagesFetched"] != float64(pages) || batch["complete"] != false {
		t.Fatalf("verified aggregate = %#v", batch)
	}
	failure := failed[0].(map[string]any)["error"].(map[string]any)
	details := failure["details"].(map[string]any)
	if details["failedPage"] != float64(pages+1) || details["failedCursor"] != cursor || failure["message"] == "" || failure["type"] == "" {
		t.Fatalf("failed page = %#v", failure)
	}
	pagination := envelope["meta"].(map[string]any)["pagination"].(map[string]any)
	if pagination["endpoint_exhausted"] != false || pagination["next_token"] != cursor || pagination["pages"] != float64(pages) {
		t.Fatalf("partial continuation = %#v", pagination)
	}
	return batch
}

func TestCrossPlatformCoverageActiveConversationsEmptyPageThenFailure(t *testing.T) {
	caller := &larkAlignmentCaller{
		responses:         map[string]string{activeConversationsOperation: `{"result":{"conversationMessagesList":[],"hasMore":true,"nextCursor":"cursor-2"}}`},
		failProductToolAt: map[string]int{activeConversationsOperation: 2},
	}
	envelope, err := executeActiveConversationsForTest(t, caller, map[string]string{"start": "2026-09-07", "end": "2026-09-08"})
	if err != nil {
		t.Fatal(err)
	}
	assertActiveConversationsPartialForTest(t, envelope, 1, 0, "cursor-2")
}

func TestCrossPlatformCoverageActiveConversationsFirstPageFailureAndResumedZero(t *testing.T) {
	for _, numeric := range []bool{false, true} {
		t.Run(map[bool]string{false: "string", true: "number"}[numeric], func(t *testing.T) {
			next := `"0"`
			if numeric {
				next = "0"
			}
			caller := &larkAlignmentCaller{responses: map[string]string{
				activeConversationsOperation: `{"result":{"conversationMessagesList":[],"hasMore":true,"nextCursor":` + next + `}}`,
			}}
			envelope, err := executeActiveConversationsForTest(t, caller, map[string]string{"start": "2026-09-07", "end": "2026-09-08", "cursor": "cursor-2"})
			if err == nil || envelope != nil || len(caller.calls) != 1 {
				t.Fatalf("rewound continuation: envelope=%#v err=%v calls=%#v", envelope, err, caller.calls)
			}
		})
	}
	caller := &larkAlignmentCaller{failProductTool: activeConversationsOperation}
	if envelope, err := executeActiveConversationsForTest(t, caller, map[string]string{"start": "2026-09-07", "end": "2026-09-08"}); err == nil || envelope != nil {
		t.Fatalf("first page must fail without inventing success: %#v, %v", envelope, err)
	}
}

type activeConversationsObservedCaller struct {
	*larkAlignmentCaller
	completedAt []time.Time
	afterRead   func()
}

func (c *activeConversationsObservedCaller) CallTool(ctx context.Context, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	result, err := c.larkAlignmentCaller.CallTool(ctx, product, tool, args)
	c.completedAt = append(c.completedAt, time.Now())
	if c.afterRead != nil {
		c.afterRead()
	}
	return result, err
}

func (c *activeConversationsObservedCaller) CallReadTool(ctx context.Context, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	return c.CallTool(ctx, product, tool, args)
}

func TestCrossPlatformCoverageActiveConversationsDelayAndFixedDefaultEnd(t *testing.T) {
	caller := &activeConversationsObservedCaller{larkAlignmentCaller: &larkAlignmentCaller{sequenceResponses: map[string][]string{
		activeConversationsOperation: {
			`{"result":{"conversationMessagesList":[],"hasMore":true,"nextCursor":2}}`,
			`{"result":{"conversationMessagesList":[],"hasMore":false}}`,
		},
	}}}
	cmd := corecmd.New(shortcut.FromShortcut(ActiveConversations))
	defaultDelay, err := cmd.Flags().GetInt("page-delay")
	if err != nil || defaultDelay != 200 {
		t.Fatalf("default page-delay=%d err=%v", defaultDelay, err)
	}
	envelope, err := executeActiveConversationsForTest(t, caller, map[string]string{"start": "2026-09-01", "page-delay": "200"})
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 2 || len(caller.completedAt) != 2 || caller.completedAt[1].Sub(caller.completedAt[0]) < 200*time.Millisecond {
		t.Fatalf("missing inter-page delay: %#v", caller.completedAt)
	}
	for _, key := range []string{"startTime", "endTime", "limit"} {
		if !reflect.DeepEqual(caller.calls[0].args[key], caller.calls[1].args[key]) {
			t.Fatalf("%s changed across pages: %#v", key, caller.calls)
		}
	}
	data := envelope["data"].(map[string]any)
	end, err := time.Parse(time.RFC3339, data["end"].(string))
	if err != nil || end.In(activeConversationsLocation).Format("2006-01-02 15:04:05") != caller.calls[0].args["endTime"] || caller.calls[1].args["cursor"] != "2" {
		t.Fatalf("wire time/cursor differs from result: %#v, %#v, %v", data, caller.calls, err)
	}
}

func TestCrossPlatformCoverageActiveConversationsCancelPaginationWait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	caller := &activeConversationsObservedCaller{larkAlignmentCaller: &larkAlignmentCaller{responses: map[string]string{
		activeConversationsOperation: `{"result":{"conversationMessagesList":[],"hasMore":true,"nextCursor":"cursor-2"}}`,
	}}}
	caller.afterRead = func() { time.AfterFunc(20*time.Millisecond, cancel) }
	started := time.Now()
	envelope, err := executeActiveConversationsContextForTest(t, ctx, caller, map[string]string{"start": "2026-09-07", "end": "2026-09-08", "page-delay": "60000"})
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(started) > 2*time.Second || len(caller.calls) != 1 {
		t.Fatalf("cancellation failed to stop paging: %#v", caller.calls)
	}
	assertActiveConversationsPartialForTest(t, envelope, 1, 0, "cursor-2")
	if err := waitActiveConversationPage(ctx, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("zero-delay path ignored cancellation: %v", err)
	}
}
