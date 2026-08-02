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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type searchMsgExecutionCaller struct {
	calls          []platformCoverageCall
	failSecondPage bool
	failEnrichment bool
	omitPagination bool
	omitMgetItem   bool
}

func (f *searchMsgExecutionCaller) CallTool(_ context.Context, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	f.calls = append(f.calls, platformCoverageCall{product: product, tool: tool, args: args})
	if product != "im" {
		return nil, errors.New("unexpected product")
	}
	switch tool {
	case "search_messages":
		if f.omitPagination {
			return searchMsgToolResult(`{"result":{"messages":[{"openMessageId":"m1","content":"sparse-1"}]}}`), nil
		}
		if args["cursor"] == "c2" {
			if f.failSecondPage {
				return nil, errors.New("second page unavailable")
			}
			return searchMsgToolResult(`{"result":{"messages":[{"openMessageId":"m2","content":"sparse-2"}],"hasMore":false}}`), nil
		}
		return searchMsgToolResult(`{"result":{"messages":[{"openMessageId":"m1","content":"sparse-1"}],"hasMore":true,"nextCursor":"c2"}}`), nil
	case "list_messages_by_ids":
		if f.failEnrichment {
			return nil, errors.New("mget unavailable")
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
	helpers.InitDeps(caller)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs(append([]string{"chat", "+search-msg", "--yes"}, args...))
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatalf("decode output: %v\n%s", err, output.String())
	}
	return payload
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

	if len(caller.calls) != 3 {
		t.Fatalf("calls = %#v", caller.calls)
	}
	first := caller.calls[0]
	if first.product != "im" || first.tool != "search_messages" {
		t.Fatalf("first call = %#v", first)
	}
	for key, want := range map[string]any{
		"openConversationIds":  []string{"cid-1", "cid-2"},
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
	if caller.calls[1].args["cursor"] != "c2" {
		t.Fatalf("second cursor = %#v", caller.calls[1].args["cursor"])
	}
	if ids := caller.calls[2].args["openMsgIds"]; !reflect.DeepEqual(ids, []string{"m1", "m2"}) {
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
