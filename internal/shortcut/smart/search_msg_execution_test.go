// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package smart

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type searchMsgExecutionCaller struct {
	calls          []platformCoverageCall
	failSecondPage bool
	failEnrichment bool
	omitPagination bool
	omitMgetItem   bool
	failPreflight  bool
	searchResponse string
	wrongMgetScope bool
}

func (f *searchMsgExecutionCaller) CallTool(_ context.Context, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	f.calls = append(f.calls, platformCoverageCall{product: product, tool: tool, args: args})
	if product == "chat" && tool == "get_conversation_info" {
		if f.failPreflight {
			return nil, errors.New("conversation not found")
		}
		return searchMsgToolResult(`{"result":{"openConversationId":"` + args["openConversationId"].(string) + `"}}`), nil
	}
	if product != "im" {
		return nil, errors.New("unexpected product")
	}
	switch tool {
	case "search_messages":
		if f.searchResponse != "" {
			return searchMsgToolResult(f.searchResponse), nil
		}
		if f.omitPagination {
			return searchMsgToolResult(`{"result":{"messages":[{"openMessageId":"m1","openConversationId":"cid-1","content":"sparse-1"}]}}`), nil
		}
		if args["cursor"] == "c2" {
			if f.failSecondPage {
				return nil, errors.New("second page unavailable")
			}
			return searchMsgToolResult(`{"result":{"messages":[{"openMessageId":"m2","openConversationId":"cid-2","content":"sparse-2"}],"hasMore":false}}`), nil
		}
		return searchMsgToolResult(`{"result":{"messages":[{"openMessageId":"m1","openConversationId":"cid-1","content":"sparse-1"}],"hasMore":true,"nextCursor":"c2"}}`), nil
	case "list_messages_by_ids":
		if f.failEnrichment {
			return nil, errors.New("mget unavailable")
		}
		if f.wrongMgetScope {
			return searchMsgToolResult(`{"result":[{"openMessageId":"m1","openConversationId":"cid-other","content":"detail-1"}]}`), nil
		}
		if f.omitMgetItem {
			return searchMsgToolResult(`{"result":[{"openMessageId":"m1","content":"detail-1"}]}`), nil
		}
		return searchMsgToolResult(`{"result":[{"openMessageId":"m1","content":"detail-1"},{"openMessageId":"m2","content":"detail-2"}]}`), nil
	default:
		return nil, errors.New("unexpected tool")
	}
}

func (f *searchMsgExecutionCaller) CallReadTool(ctx context.Context, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	return f.CallTool(ctx, product, tool, args)
}

func (*searchMsgExecutionCaller) Format() string { return "json" }
func (*searchMsgExecutionCaller) DryRun() bool   { return false }
func (*searchMsgExecutionCaller) Fields() string { return "" }
func (*searchMsgExecutionCaller) JQ() string     { return "" }

func searchMsgToolResult(text string) *edition.ToolResult {
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: text}}}
}

func executeSearchMsg(t *testing.T, caller *searchMsgExecutionCaller, args ...string) map[string]any {
	t.Helper()
	payload, err := executeSearchMsgResult(caller, args...)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func executeSearchMsgResult(caller *searchMsgExecutionCaller, args ...string) (map[string]any, error) {
	helpers.InitDeps(caller)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs(append([]string{"chat", "+search-msg", "--yes"}, args...))
	if err := root.Execute(); err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func TestSearchMsgPagesAndEnrichesWithAdvancedFilters(t *testing.T) {
	caller := &searchMsgExecutionCaller{}
	payload := executeSearchMsg(t, caller,
		"--query", "周报",
		"--chat-id", "cid-1,cid-2",
		"--sender", "42,Dsender",
		"--at-ids", "43,Dat",
		"--is-at-me",
		"--message-type", "text",
		"--only-robot",
		"--chat-type", "group",
		"--start", "2026-07-01T00:00:00+08:00",
		"--end", "2026-07-02T00:00:00+08:00",
		"--page-size", "50",
		"--page-token", "p0",
		"--page-all",
		"--page-limit", "3",
	)

	if len(caller.calls) != 5 {
		t.Fatalf("calls = %#v", caller.calls)
	}
	if caller.calls[0].tool != "get_conversation_info" || caller.calls[0].args["openConversationId"] != "cid-1" ||
		caller.calls[1].tool != "get_conversation_info" || caller.calls[1].args["openConversationId"] != "cid-2" {
		t.Fatalf("scope preflight calls = %#v", caller.calls[:2])
	}
	first := caller.calls[2]
	if first.product != "im" || first.tool != "search_messages" {
		t.Fatalf("first call = %#v", first)
	}
	for key, want := range map[string]any{
		"senderUserIds":        []string{"42"},
		"senderOpenDingTakIds": []string{"Dsender"},
		"atUserIds":            []string{"43"},
		"atOpenDingTakIds":     []string{"Dat"},
		"messageType":          "text",
		"onlyRobotMessages":    true,
		"searchConvType":       "group",
	} {
		if !reflect.DeepEqual(first.args[key], want) {
			t.Errorf("%s = %#v, want %#v", key, first.args[key], want)
		}
	}
	if _, exists := first.args["openConversationIds"]; exists {
		t.Fatalf("global fallback unexpectedly forwarded openConversationIds: %#v", first.args)
	}
	if caller.calls[3].args["cursor"] != "c2" {
		t.Fatalf("second cursor = %#v", caller.calls[3].args["cursor"])
	}
	if ids := caller.calls[4].args["openMsgIds"]; !reflect.DeepEqual(ids, []string{"m1", "m2"}) {
		t.Fatalf("mget ids = %#v", ids)
	}
	if payload["complete"] != true || payload["count"] != float64(2) ||
		payload["pagesFetched"] != float64(2) || payload["enrichedCount"] != float64(2) ||
		payload["failedCount"] != float64(0) {
		t.Fatalf("payload = %#v", payload)
	}
	messages, _ := payload["messages"].([]any)
	firstMessage, _ := messages[0].(map[string]any)
	if firstMessage["text"] != "detail-1" {
		t.Fatalf("enriched message = %#v", firstMessage)
	}
	scope, _ := payload["scope"].(map[string]any)
	if scope["targetsValidated"] != true || scope["filterMode"] != "client" || scope["resultsWithinScope"] != true {
		t.Fatalf("scope = %#v", scope)
	}
}

func TestSearchMsgLaterPageFailurePublishesPartialLedger(t *testing.T) {
	caller := &searchMsgExecutionCaller{failSecondPage: true}
	payload := executeSearchMsg(t, caller,
		"--query", "周报",
		"--page-all",
		"--no-enrich",
	)
	if payload["complete"] != false || payload["count"] != float64(1) ||
		payload["failedCount"] != float64(1) {
		t.Fatalf("payload = %#v", payload)
	}
	failures, _ := payload["failures"].([]any)
	failure, _ := failures[0].(map[string]any)
	if failure["stage"] != "search-page" {
		t.Fatalf("failure = %#v", failure)
	}
}

func TestSearchMsgEnrichmentFailureKeepsSearchHits(t *testing.T) {
	caller := &searchMsgExecutionCaller{failEnrichment: true}
	payload := executeSearchMsg(t, caller, "--query", "周报")
	if payload["complete"] != false || payload["count"] != float64(1) ||
		payload["enrichedCount"] != float64(0) || payload["failedCount"] != float64(1) {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestSearchMsgMissingPaginationCannotClaimComplete(t *testing.T) {
	caller := &searchMsgExecutionCaller{omitPagination: true}
	payload := executeSearchMsg(t, caller, "--query", "周报", "--no-enrich")
	if payload["complete"] != false || payload["count"] != float64(1) ||
		payload["failedCount"] != float64(1) {
		t.Fatalf("payload = %#v", payload)
	}
	failures, _ := payload["failures"].([]any)
	failure, _ := failures[0].(map[string]any)
	if failure["stage"] != "search-pagination" {
		t.Fatalf("failure = %#v", failure)
	}
}

func TestSearchMsgMissingMgetItemPublishesFailureLedger(t *testing.T) {
	caller := &searchMsgExecutionCaller{omitMgetItem: true}
	payload := executeSearchMsg(t, caller, "--query", "周报", "--page-all")
	if payload["complete"] != false || payload["count"] != float64(2) ||
		payload["enrichedCount"] != float64(1) || payload["failedCount"] != float64(1) {
		t.Fatalf("payload = %#v", payload)
	}
	failures, _ := payload["failures"].([]any)
	failure, _ := failures[0].(map[string]any)
	if failure["stage"] != "message-enrichment" {
		t.Fatalf("failure = %#v", failure)
	}
	if missing, _ := failure["missingMessageIds"].([]any); len(missing) != 1 || missing[0] != "m2" {
		t.Fatalf("failure = %#v", failure)
	}
}

func TestSearchMsgScopedFallbackFiltersGlobalResults(t *testing.T) {
	caller := &searchMsgExecutionCaller{searchResponse: `{
		"result": {
			"conversationMessagesList": [
				{"openConversationId":"cid-target","title":"目标群","messages":[{"openMessageId":"m-target","content":"目标"}]},
				{"openConversationId":"cid-other","title":"其他群","messages":[{"openMessageId":"m-other","content":"越界"}]}
			],
			"hasMore": false
		}
	}`}
	payload := executeSearchMsg(t, caller, "--group", "cid-target", "--query", "目标", "--no-enrich")
	if payload["count"] != float64(1) || payload["complete"] != true {
		t.Fatalf("payload = %#v", payload)
	}
	messages, _ := payload["messages"].([]any)
	message, _ := messages[0].(map[string]any)
	if message["conversationId"] != "cid-target" || message["messageId"] != "m-target" {
		t.Fatalf("messages = %#v", messages)
	}
	if len(caller.calls) != 2 || caller.calls[0].tool != "get_conversation_info" || caller.calls[1].tool != "search_messages" {
		t.Fatalf("calls = %#v", caller.calls)
	}
	if _, exists := caller.calls[1].args["openConversationIds"]; exists {
		t.Fatalf("global fallback unexpectedly forwarded scope: %#v", caller.calls[1].args)
	}
}

func TestSearchMsgScopedValidEmptyResultIsComplete(t *testing.T) {
	caller := &searchMsgExecutionCaller{searchResponse: `{"result":{"messages":[],"hasMore":false}}`}
	payload := executeSearchMsg(t, caller, "--group", "cid-empty", "--query", "不存在", "--no-enrich")
	if payload["count"] != float64(0) || payload["complete"] != true {
		t.Fatalf("payload = %#v", payload)
	}
	scope, _ := payload["scope"].(map[string]any)
	if scope["targetsValidated"] != true || scope["sourceComplete"] != true {
		t.Fatalf("scope = %#v", scope)
	}
}

func TestSearchMsgScopedEmptyPartialScanCannotClaimComplete(t *testing.T) {
	caller := &searchMsgExecutionCaller{searchResponse: `{
		"result": {
			"conversationMessagesList": [
				{"openConversationId":"cid-other","messages":[{"openMessageId":"m-other"}]}
			],
			"hasMore": true,
			"nextCursor": "c2"
		}
	}`}
	payload := executeSearchMsg(t, caller,
		"--group", "cid-target", "--query", "周报", "--no-enrich", "--page-limit", "1")
	if payload["count"] != float64(0) || payload["complete"] != false || payload["failedCount"] != float64(1) {
		t.Fatalf("payload = %#v", payload)
	}
	scope, _ := payload["scope"].(map[string]any)
	if scope["sourceComplete"] != false {
		t.Fatalf("scope = %#v", scope)
	}
	failures, _ := payload["failures"].([]any)
	failure, _ := failures[0].(map[string]any)
	if failure["stage"] != "search-page-limit" {
		t.Fatalf("failure = %#v", failure)
	}
}

func TestSearchMsgInvalidCIDStopsBeforeGlobalSearch(t *testing.T) {
	caller := &searchMsgExecutionCaller{failPreflight: true}
	_, err := executeSearchMsgResult(caller, "--group", "cid-invalid", "--query", "周报", "--no-enrich")
	if err == nil {
		t.Fatal("invalid CID unexpectedly succeeded")
	}
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Reason != "search_conversation_scope_invalid" {
		t.Fatalf("error = %#v", err)
	}
	if len(caller.calls) != 1 || caller.calls[0].tool != "get_conversation_info" {
		t.Fatalf("calls = %#v", caller.calls)
	}
}

func TestSearchMsgMissingConversationIdentityFailsClosed(t *testing.T) {
	caller := &searchMsgExecutionCaller{searchResponse: `{"result":{"messages":[{"openMessageId":"m1","content":"unknown"}],"hasMore":false}}`}
	_, err := executeSearchMsgResult(caller, "--group", "cid-target", "--query", "周报", "--no-enrich")
	if err == nil {
		t.Fatal("unverifiable scoped result unexpectedly succeeded")
	}
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Reason != "search_conversation_scope_unverified" {
		t.Fatalf("error = %#v", err)
	}
}

func TestSearchMsgEnrichmentCannotMoveMessageOutsideScope(t *testing.T) {
	caller := &searchMsgExecutionCaller{
		searchResponse: `{"result":{"messages":[{"openMessageId":"m1","openConversationId":"cid-target","content":"sparse"}],"hasMore":false}}`,
		wrongMgetScope: true,
	}
	_, err := executeSearchMsgResult(caller, "--group", "cid-target", "--query", "周报")
	if err == nil {
		t.Fatal("out-of-scope enrichment unexpectedly succeeded")
	}
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Reason != "search_conversation_scope_violation" {
		t.Fatalf("error = %#v", err)
	}
}
