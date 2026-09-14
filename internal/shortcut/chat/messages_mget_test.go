// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type mgetTestCaller struct {
	larkAlignmentCaller
	call func(string, map[string]any) (map[string]any, error)
}

func (f *mgetTestCaller) CallTool(_ context.Context, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	f.calls = append(f.calls, larkAlignmentCall{product: product, tool: tool, args: args})
	data, err := f.call(product+"/"+tool, args)
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(data)
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: string(b)}}}, err
}
func (f *mgetTestCaller) CallReadTool(ctx context.Context, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	return f.CallTool(ctx, product, tool, args)
}

func invalidMgetIDError() error {
	return apperrors.NewAPI("invalid openMsgId: fixture", apperrors.WithReason("invalid_request"), apperrors.WithServerDiag(apperrors.ServerDiagnostics{ServerErrorCode: "PARAM_ERROR"}))
}

func runMgetTest(t *testing.T, f *mgetTestCaller, args ...string) (map[string]any, error) {
	t.Helper()
	helpers.InitDeps(f)
	root := newPlatformCoverageRoot()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs(append([]string{"chat", "+messages-mget"}, args...))
	err := root.Execute()
	out := map[string]any{}
	if buf.Len() > 0 {
		if e := json.Unmarshal(buf.Bytes(), &out); e != nil {
			t.Fatal(e)
		}
	}
	return out, err
}

func TestCrossPlatformCoverageMgetIsolatesBadIDsAndKeepsRequestOrder(t *testing.T) {
	f := &mgetTestCaller{call: func(tool string, args map[string]any) (map[string]any, error) {
		if tool != "im/list_messages_by_ids" {
			t.Fatalf("unexpected %s", tool)
		}
		ids := args["openMsgIds"].([]string)
		for _, id := range ids {
			if strings.HasPrefix(id, "bad") {
				return nil, invalidMgetIDError()
			}
		}
		items := []map[string]any{{"openMessageId": "foreign", "content": "must not escape"}}
		for i := len(ids) - 1; i >= 0; i-- {
			if ids[i] != "missing" {
				items = append(items, map[string]any{"openMessageId": ids[i]}, map[string]any{"openMessageId": ids[i]})
			}
		}
		return map[string]any{"result": items}, nil
	}}
	out, err := runMgetTest(t, f, "--message-id", "ok2,bad1,ok1,missing,bad2,ok2", "--no-reactions", "--no-threads")
	if err != nil {
		t.Fatal(err)
	}
	if out["foundCount"] != float64(2) || out["notFoundCount"] != float64(3) || out["complete"] != false || out["messagesComplete"] != false {
		t.Fatalf("counts: %#v", out)
	}
	items := out["messages"].([]any)
	if len(items) != 2 || items[0].(map[string]any)["messageId"] != "ok2" || items[1].(map[string]any)["messageId"] != "ok1" {
		t.Fatalf("items: %#v", items)
	}
	failures := out["failures"].([]any)
	for i, want := range []string{"invalid_message_id", "not_returned", "invalid_message_id"} {
		if failures[i].(map[string]any)["reason"] != want {
			t.Fatalf("failures: %#v", failures)
		}
	}
	if len(f.calls) > 9 {
		t.Fatalf("unbounded %d", len(f.calls))
	}
}

func TestCrossPlatformCoverageMgetSplitBoundAndNoSystemicFanout(t *testing.T) {
	t.Run("all invalid bounded", func(t *testing.T) {
		f := &mgetTestCaller{call: func(string, map[string]any) (map[string]any, error) { return nil, invalidMgetIDError() }}
		ids := []string{}
		for i := 0; i < 50; i++ {
			ids = append(ids, fmt.Sprintf("bad%d", i))
		}
		out, err := runMgetTest(t, f, "--msg-ids", strings.Join(ids, ","), "--no-reactions", "--no-threads")
		var typed *apperrors.Error
		if !errors.As(err, &typed) || typed.Reason != "message_batch_failed" || len(f.calls) != 99 || typed.Details["foundCount"] != 0 || len(typed.Details["failures"].([]map[string]any)) != 50 {
			t.Fatalf("all-invalid: %#v %v calls=%d", out, err, len(f.calls))
		}
	})
	for _, cause := range []error{apperrors.NewAuth("expired"), apperrors.NewAPI("limited", apperrors.WithReason("rate_limited")), apperrors.NewAPI("other required argument", apperrors.WithReason("invalid_request"), apperrors.WithServerDiag(apperrors.ServerDiagnostics{ServerErrorCode: "PARAM_ERROR"})), fmt.Errorf("invalid openMsgId: untyped")} {
		t.Run(cause.Error(), func(t *testing.T) {
			f := &mgetTestCaller{call: func(string, map[string]any) (map[string]any, error) { return nil, cause }}
			_, err := runMgetTest(t, f, "--msg-ids", "ok,bad", "--no-reactions", "--no-threads")
			if err == nil || len(f.calls) != 1 {
				t.Fatalf("systemic error fanout: %v %d", err, len(f.calls))
			}
		})
	}
	t.Run("systemic after success retains data", func(t *testing.T) {
		f := &mgetTestCaller{call: func(_ string, args map[string]any) (map[string]any, error) {
			ids := args["openMsgIds"].([]string)
			if len(ids) == 3 {
				return nil, invalidMgetIDError()
			}
			if ids[0] == "ok" {
				return map[string]any{"result": []map[string]any{{"openMessageId": "ok"}}}, nil
			}
			return nil, apperrors.NewAuth("expired")
		}}
		out, err := runMgetTest(t, f, "--msg-ids", "ok,bad,unknown")
		if err != nil || len(f.calls) != 3 || out["foundCount"] != float64(1) {
			t.Fatalf("lost partial: %#v %v", out, err)
		}
		for _, failure := range out["failures"].([]any) {
			if failure.(map[string]any)["reason"] != "query_aborted" {
				t.Fatal(failure)
			}
		}
	})
}

func TestCrossPlatformCoverageMgetEnrichesReactionsAndBoundedThreads(t *testing.T) {
	f := &mgetTestCaller{call: func(tool string, args map[string]any) (map[string]any, error) {
		switch tool {
		case "im/list_messages_by_ids":
			return map[string]any{"result": []map[string]any{{"openMessageId": "root", "openConversationId": "cid", "openConvThreadId": "thread"}, {"openMessageId": "same-thread", "openConversationId": "cid", "openConvThreadId": "thread"}}}, nil
		case "chat/list_topic_replies":
			if !reflect.DeepEqual(args, map[string]any{"openconversationId": "cid", "topicId": "thread", "pageSize": 10, "forward": false}) {
				t.Fatal(args)
			}
			items := []map[string]any{}
			for i := 0; i < 12; i++ {
				items = append(items, map[string]any{"openMessageId": fmt.Sprintf("reply%d", i), "content": "reply"})
			}
			return map[string]any{"result": map[string]any{"messages": items, "hasMore": false}}, nil
		case "im/list_message_emotion_replies":
			ids := args["openMessageIds"].([]string)
			if len(ids) > 20 {
				t.Fatal("reaction budget")
			}
			items := []map[string]any{}
			for _, id := range ids {
				items = append(items, map[string]any{"openMessageId": id, "emotionReplyList": []map[string]any{{"emoji": "smile", "replyUsers": []string{"u1"}}}})
			}
			return map[string]any{"result": map[string]any{"messages": items}}, nil
		default:
			t.Fatalf("unexpected %s", tool)
			return nil, nil
		}
	}}
	out, err := runMgetTest(t, f, "--message-ids", "root,same-thread")
	if err != nil {
		t.Fatal(err)
	}
	ledger := out["enrichment"].(map[string]any)
	if out["messagesComplete"] != true || out["complete"] != false || ledger["threadRequests"] != float64(1) || ledger["threadReplyCount"] != float64(10) {
		t.Fatal(out)
	}
	for _, value := range out["messages"].([]any) {
		item := value.(map[string]any)
		if item["reactions"] == nil {
			t.Fatal(item)
		}
		view := item["thread"].(map[string]any)
		if view["rawReplies"] != nil || view["complete"] != false {
			t.Fatal(view)
		}
		for _, reply := range view["replies"].([]any) {
			if reply.(map[string]any)["reactions"] == nil || reply.(map[string]any)["conversationId"] != "cid" {
				t.Fatal(reply)
			}
		}
	}
}

func TestCrossPlatformCoverageMgetEnrichmentFailureKeepsMessages(t *testing.T) {
	for _, mode := range []string{"error", "unknown", "missing", "malformed", "empty", "no-threads"} {
		t.Run(mode, func(t *testing.T) {
			f := &mgetTestCaller{call: func(tool string, args map[string]any) (map[string]any, error) {
				if tool == "im/list_messages_by_ids" {
					return map[string]any{"result": []map[string]any{{"openMessageId": "root", "openConversationId": "cid", "openConvThreadId": "thread"}}}, nil
				}
				if tool == "chat/list_topic_replies" {
					if mode == "no-threads" {
						t.Fatal("must skip threads")
					}
					return map[string]any{"result": map[string]any{"messages": []any{}, "hasMore": false}}, nil
				}
				switch mode {
				case "error":
					return nil, fmt.Errorf("offline")
				case "unknown":
					return map[string]any{"success": true}, nil
				case "malformed":
					return map[string]any{"result": map[string]any{"messages": []map[string]any{{"openMessageId": "root", "emotionReplyList": []any{"invalid"}}}}}, nil
				case "missing":
					return map[string]any{"result": map[string]any{"messages": []any{}}}, nil
				default:
					return map[string]any{"result": map[string]any{"messages": []map[string]any{{"openMessageId": "root", "emotionReplyList": []any{}}}}}, nil
				}
			}}
			args := []string{"--msg-ids", "root"}
			if mode == "no-threads" {
				args = append(args, "--no-threads")
			}
			out, err := runMgetTest(t, f, args...)
			if err != nil || out["foundCount"] != float64(1) || out["messagesComplete"] != true {
				t.Fatalf("%#v %v", out, err)
			}
			wantComplete := mode == "empty" || mode == "no-threads"
			if out["complete"] != wantComplete {
				t.Fatalf("complete: %#v", out)
			}
		})
	}
}
