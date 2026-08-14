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
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/targetresolver"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type searchMsgExecutionCaller struct {
	calls              []platformCoverageCall
	failSecondPage     bool
	failEnrichment     bool
	omitPagination     bool
	omitMgetItem       bool
	failPreflight      bool
	preflightError     error
	searchResponse     string
	wrongMgetScope     bool
	missingMgetCID     bool
	firstResponse      string
	mgetResponse       string
	numericZeroEnd     bool
	contactResponse    string
	groupResponse      string
	failContactKeyword string
	failGroupKeyword   string
}

const (
	testCurrentDOpenID  = "DAAAAAAAAAAAiE"
	testCurrentDOpenID2 = "DAQEBAQEBAQEiE"
)

func (f *searchMsgExecutionCaller) CallTool(_ context.Context, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	f.calls = append(f.calls, platformCoverageCall{product: product, tool: tool, args: args})
	if product == "chat" && tool == "get_conversation_info" {
		if f.preflightError != nil {
			return nil, f.preflightError
		}
		if f.failPreflight {
			return nil, errors.New("conversation not found")
		}
		return searchMsgToolResult(`{"result":{"openConversationId":"` + args["openConversationId"].(string) + `"}}`), nil
	}
	if product == "contact" && tool == "search_contact_by_key_word" {
		if f.failContactKeyword != "" && args["keyword"] == f.failContactKeyword {
			return nil, errors.New("fixture contact failure")
		}
		if f.contactResponse != "" {
			return searchMsgToolResult(f.contactResponse), nil
		}
		keyword := args["keyword"].(string)
		return searchMsgToolResult(`{"result":[{"userId":"user-` + keyword + `","openDingTalkId":"` + testCurrentDOpenID + `","name":"` + keyword + `"}],"hasMore":false}`), nil
	}
	if product != "im" {
		return nil, errors.New("unexpected product")
	}
	switch tool {
	case "search_groups":
		if f.failGroupKeyword != "" && args["keyword"] == f.failGroupKeyword {
			return nil, errors.New("fixture group failure")
		}
		if f.groupResponse != "" {
			return searchMsgToolResult(f.groupResponse), nil
		}
		return searchMsgToolResult(`{"result":[{"openConversationId":"cid-resolved-group","title":"` + args["keyword"].(string) + `"}],"hasMore":false}`), nil
	case "search_messages":
		if f.searchResponse != "" {
			return searchMsgToolResult(f.searchResponse), nil
		}
		if f.omitPagination {
			return searchMsgToolResult(`{"result":{"messages":[{"openMessageId":"m1","openConversationId":"cid-1","content":"sparse-1"}]}}`), nil
		}
		if f.numericZeroEnd {
			return searchMsgToolResult(`{"result":{"messages":[{"openMessageId":"m1","content":"sparse-1"}],"hasMore":false,"nextCursor":0}}`), nil
		}
		if args["cursor"] == "c2" {
			if f.failSecondPage {
				return nil, errors.New("second page unavailable")
			}
			return searchMsgToolResult(`{"result":{"messages":[{"openMessageId":"m2","openConversationId":"cid-2","senderOpenDingTalkId":"` + testCurrentDOpenID + `","content":"sparse-2"}],"hasMore":false}}`), nil
		}
		if f.firstResponse != "" {
			return searchMsgToolResult(f.firstResponse), nil
		}
		return searchMsgToolResult(`{"result":{"messages":[{"openMessageId":"m1","openConversationId":"cid-1","senderOpenDingTalkId":"` + testCurrentDOpenID + `","content":"sparse-1"}],"hasMore":true,"nextCursor":"c2"}}`), nil
	case "list_messages_by_ids":
		if f.failEnrichment {
			return nil, errors.New("mget unavailable")
		}
		if f.wrongMgetScope {
			return searchMsgToolResult(`{"result":[{"openMessageId":"m1","openConversationId":"cid-other","content":"detail-1"}]}`), nil
		}
		if f.missingMgetCID {
			return searchMsgToolResult(`{"result":[{"openMessageId":"m1","openConversationId":null,"content":"detail-1"}]}`), nil
		}
		if f.omitMgetItem {
			return searchMsgToolResult(`{"result":[{"openMessageId":"m1","content":"detail-1"}]}`), nil
		}
		if f.mgetResponse != "" {
			return searchMsgToolResult(f.mgetResponse), nil
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

func TestSearchMsgMixedSenderClassifiesFormatWithoutIDPreflight(t *testing.T) {
	tests := []string{"D-prefix-fixture-user", "d-prefix-fixture-user", "测试用户甲", "fixture-user-id"}
	for _, target := range tests {
		t.Run(target, func(t *testing.T) {
			caller := &searchMsgExecutionCaller{searchResponse: `{"result":{"messages":[],"hasMore":false}}`}
			payload := executeSearchMsg(t, caller, "--sender", target, "--no-enrich")
			if payload["complete"] != true || len(caller.calls) != 2 {
				t.Fatalf("payload=%#v calls=%#v", payload, caller.calls)
			}
			if caller.calls[0].product != "contact" || caller.calls[0].tool != "search_contact_by_key_word" ||
				caller.calls[1].tool != "search_messages" {
				t.Fatalf("calls=%#v", caller.calls)
			}
			if got, want := caller.calls[1].args["senderOpenDingTakIds"], []string{testCurrentDOpenID}; !reflect.DeepEqual(got, want) {
				t.Fatalf("senderOpenDingTakIds=%#v want=%#v", got, want)
			}
		})
	}

	caller := &searchMsgExecutionCaller{searchResponse: `{"result":{"messages":[],"hasMore":false}}`}
	executeSearchMsg(t, caller, "--sender", testCurrentDOpenID, "--no-enrich")
	if len(caller.calls) != 1 || caller.calls[0].tool != "search_messages" {
		t.Fatalf("format-valid ID unexpectedly preflighted: %#v", caller.calls)
	}
}

func TestSearchMsgSenderScopeFiltersBackendOverReturn(t *testing.T) {
	caller := &searchMsgExecutionCaller{searchResponse: `{"result":{"messages":[
		{"openMessageId":"wanted","senderOpenDingTalkId":"DAAAAAAAAAAAiE","content":"keep"},
		{"openMessageId":"other","senderOpenDingTalkId":"DAQEBAQEBAQEiE","content":"drop"}
	],"hasMore":false}}`}
	payload := executeSearchMsg(t, caller, "--sender", testCurrentDOpenID, "--no-enrich")
	if payload["count"] != float64(1) {
		t.Fatalf("payload=%#v", payload)
	}
	messages := payload["messages"].([]any)
	if messages[0].(map[string]any)["messageId"] != "wanted" {
		t.Fatalf("messages=%#v", messages)
	}
}

func TestCrossPlatformCoverageSearchMsgSenderScopeIgnoresNonMatchingIdentityFamily(t *testing.T) {
	caller := &searchMsgExecutionCaller{searchResponse: `{"result":{"messages":[
		{"openMessageId":"wanted","senderOpenDingTalkId":"DAAAAAAAAAAAiE","content":"keep"},
		{"openMessageId":"other-family","senderUserId":"other-user","content":"drop"}
	],"hasMore":false}}`}
	payload := executeSearchMsg(t, caller, "--sender", testCurrentDOpenID, "--no-enrich")
	if payload["complete"] != true || payload["count"] != float64(1) || payload["failedCount"] != float64(0) {
		t.Fatalf("payload=%#v", payload)
	}

	filtered, unverifiable := filterSearchSenderScope(
		[]map[string]any{
			{"openMessageId": "wanted-user", "senderUserId": "wanted-user"},
			{"openMessageId": "other-open-family", "senderOpenDingTalkId": testCurrentDOpenID},
		},
		[]targetresolver.UserResolution{{Selected: targetresolver.User{UserID: "wanted-user"}}},
	)
	if len(filtered) != 1 || len(unverifiable) != 0 {
		t.Fatalf("reverse family filtered=%#v unverifiable=%#v", filtered, unverifiable)
	}
}

func TestCrossPlatformCoverageSearchMsgResolutionFailureEdges(t *testing.T) {
	for _, tc := range []struct {
		name   string
		caller *searchMsgExecutionCaller
		args   []string
	}{
		{
			name:   "group target",
			caller: &searchMsgExecutionCaller{failGroupKeyword: "missing-group"},
			args:   []string{"--group", "missing-group", "--no-enrich"},
		},
		{
			name:   "sender query",
			caller: &searchMsgExecutionCaller{failContactKeyword: "missing-query"},
			args:   []string{"--sender-query", "missing-query", "--no-enrich"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := executeSearchMsgResult(tc.caller, tc.args...); err == nil {
				t.Fatal("resolution failure unexpectedly succeeded")
			}
		})
	}
}

func TestCrossPlatformCoverageSearchMsgRejectsAmbiguousDirectSender(t *testing.T) {
	caller := &searchMsgExecutionCaller{contactResponse: `{"result":[
		{"userId":"fixture-user-1","name":"测试同名发送者"},
		{"userId":"fixture-user-2","name":"测试同名发送者"}
	],"hasMore":false}`}
	_, err := executeSearchMsgResult(caller, "--sender", "测试同名发送者", "--no-enrich")
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Reason != "resolution_ambiguous" {
		t.Fatalf("error=%#v, want resolution_ambiguous", err)
	}
}

func TestCrossPlatformCoverageSearchMsgStableUserIDContinuesWhenDirectoryIsUnavailable(t *testing.T) {
	t.Run("unrelated unique directory candidate never replaces the supplied user id", func(t *testing.T) {
		caller := &searchMsgExecutionCaller{
			contactResponse: `{"result":[{"userId":"other-user","name":"其他用户"}],"hasMore":false}`,
			searchResponse: `{"result":{"messages":[
				{"openMessageId":"wanted","senderUserId":"fixture-user-id","content":"keep"},
				{"openMessageId":"other","senderUserId":"other-user","content":"drop"}
			],"hasMore":false}}`,
		}
		payload := executeSearchMsg(t, caller, "--sender", "fixture-user-id", "--no-enrich")
		if len(caller.calls) != 2 || caller.calls[0].tool != "search_contact_by_key_word" ||
			caller.calls[1].tool != "search_messages" {
			t.Fatalf("calls=%#v", caller.calls)
		}
		if got := caller.calls[1].args["senderUserIds"]; !reflect.DeepEqual(got, []string{"fixture-user-id"}) {
			t.Fatalf("senderUserIds=%#v args=%#v", got, caller.calls[1].args)
		}
		if _, exists := caller.calls[1].args["senderOpenDingTakIds"]; exists {
			t.Fatalf("unrelated candidate leaked into request: %#v", caller.calls[1].args)
		}
		if payload["complete"] != true || payload["count"] != float64(1) {
			t.Fatalf("payload=%#v", payload)
		}
	})

	t.Run("mixed sender preserves positive matches and blocks complete negative conclusion", func(t *testing.T) {
		caller := &searchMsgExecutionCaller{
			failContactKeyword: "stable-user-id",
			searchResponse: `{"result":{"messages":[
				{"openMessageId":"wanted","senderUserId":"stable-user-id","content":"keep"},
				{"openMessageId":"other","senderUserId":"other-user","content":"drop"}
			],"hasMore":false}}`,
		}
		payload := executeSearchMsg(t, caller, "--sender", "stable-user-id", "--no-enrich")
		if len(caller.calls) != 2 || caller.calls[0].tool != "search_contact_by_key_word" ||
			caller.calls[1].tool != "search_messages" {
			t.Fatalf("calls=%#v", caller.calls)
		}
		if got := caller.calls[1].args["senderUserIds"]; !reflect.DeepEqual(got, []string{"stable-user-id"}) {
			t.Fatalf("senderUserIds=%#v", got)
		}
		if payload["count"] != float64(1) || payload["complete"] != true || payload["failedCount"] != float64(0) {
			t.Fatalf("payload=%#v", payload)
		}
		scope := payload["senderScope"].(map[string]any)
		if _, exists := scope["status"]; exists || scope["targetsResolved"] != true {
			t.Fatalf("senderScope=%#v", scope)
		}
	})

	t.Run("mixed sender without a stable id match cannot produce a complete negative conclusion", func(t *testing.T) {
		caller := &searchMsgExecutionCaller{
			failContactKeyword: "possibly-a-name",
			searchResponse:     `{"result":{"messages":[],"hasMore":false}}`,
		}
		payload := executeSearchMsg(t, caller, "--sender", "possibly-a-name", "--no-enrich")
		if payload["count"] != float64(0) || payload["complete"] != false || payload["failedCount"] != float64(1) {
			t.Fatalf("payload=%#v", payload)
		}
		scope := payload["senderScope"].(map[string]any)
		if scope["status"] != "identity_unverified" || scope["targetsResolved"] != false {
			t.Fatalf("senderScope=%#v", scope)
		}
	})

	t.Run("id-only at target bypasses directory", func(t *testing.T) {
		caller := &searchMsgExecutionCaller{
			failContactKeyword: "stable-at-user-id",
			searchResponse:     `{"result":{"messages":[],"hasMore":false}}`,
		}
		payload := executeSearchMsg(t, caller, "--at-ids", "stable-at-user-id", "--no-enrich")
		if len(caller.calls) != 1 || caller.calls[0].tool != "search_messages" {
			t.Fatalf("calls=%#v", caller.calls)
		}
		if got := caller.calls[0].args["atUserIds"]; !reflect.DeepEqual(got, []string{"stable-at-user-id"}) {
			t.Fatalf("atUserIds=%#v", got)
		}
		if payload["complete"] != true {
			t.Fatalf("payload=%#v", payload)
		}
	})
}

func TestCrossPlatformCoverageSearchMsgSenderScopeFailureEdges(t *testing.T) {
	t.Run("execute rejects senderless backend rows", func(t *testing.T) {
		caller := &searchMsgExecutionCaller{searchResponse: `{"result":{"messages":[{"content":"missing identity"}],"hasMore":false}}`}
		_, err := executeSearchMsgResult(caller, "--sender", testCurrentDOpenID, "--no-enrich")
		var typed *apperrors.Error
		if !errors.As(err, &typed) || typed.Reason != "search_sender_scope_unverified" {
			t.Fatalf("error=%#v", err)
		}
	})

	resolutions := []targetresolver.UserResolution{{
		Query:    "fixture-user",
		Selected: targetresolver.User{UserID: "wanted-user"},
	}}
	filtered, unverifiable := filterSearchSenderScope(
		[]map[string]any{
			{"content": "missing identity"},
			{"openMessageId": "wanted", "senderUserId": "wanted-user"},
			{"openMessageId": "open-family", "senderOpenDingTalkId": testCurrentDOpenID},
		},
		resolutions,
	)
	if len(filtered) != 1 || !reflect.DeepEqual(unverifiable, []string{"<unknown>"}) {
		t.Fatalf("filtered=%#v unverifiable=%#v", filtered, unverifiable)
	}
	err := searchSenderScopeUnverifiedError(
		append(resolutions, resolutions...),
		[]string{"message-1"},
	)
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Reason != "search_sender_scope_unverified" {
		t.Fatalf("error=%#v", err)
	}
}

func TestCrossPlatformCoverageSearchMsgExplicitDaysMetadata(t *testing.T) {
	caller := &searchMsgExecutionCaller{searchResponse: `{"result":{"messages":[],"hasMore":false}}`}
	payload := executeSearchMsg(t, caller, "--query", "fixture", "--days", "3", "--no-enrich")
	coverage := payload["timeCoverage"].(map[string]any)
	if coverage["source"] != "explicit_days" || coverage["days"] != float64(3) {
		t.Fatalf("coverage=%#v", coverage)
	}
}

func TestCrossPlatformCoverageSearchMsgRejectsNonConversationIDAlias(t *testing.T) {
	caller := &searchMsgExecutionCaller{}
	_, err := executeSearchMsgResult(caller, "--query", "fixture", "--chat-id", "group-name", "--no-enrich")
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Reason != "target_type_mismatch" {
		t.Fatalf("error=%#v", err)
	}
}

func TestCrossPlatformCoverageSearchMsgGroupNameAndDefaultWindowMetadata(t *testing.T) {
	caller := &searchMsgExecutionCaller{searchResponse: `{"result":{"messages":[],"hasMore":false}}`}
	payload := executeSearchMsg(t, caller, "--group", "项目讨论群", "--no-enrich")
	if len(caller.calls) != 3 || caller.calls[0].tool != "search_groups" ||
		caller.calls[1].tool != "get_conversation_info" || caller.calls[2].tool != "search_messages" {
		t.Fatalf("calls=%#v", caller.calls)
	}
	coverage := payload["timeCoverage"].(map[string]any)
	guard := payload["conclusionGuard"].(map[string]any)
	if coverage["source"] != "implicit_default" || coverage["days"] != float64(7) ||
		guard["absenceAcrossAllHistoryAllowed"] != false {
		t.Fatalf("coverage=%#v guard=%#v", coverage, guard)
	}
}

func TestCrossPlatformCoverageSearchMsgPagesAndEnrichesWithAdvancedFilters(t *testing.T) {
	caller := &searchMsgExecutionCaller{}
	payload := executeSearchMsg(t, caller,
		"--query", "周报",
		"--chat-id", "cid-1,cid-2",
		"--sender", testCurrentDOpenID,
		"--at-ids", testCurrentDOpenID2,
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
		"senderOpenDingTakIds": []string{testCurrentDOpenID},
		"atOpenDingTakIds":     []string{testCurrentDOpenID2},
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

func TestCrossPlatformCoverageSearchMsgLaterPageFailurePublishesPartialLedger(t *testing.T) {
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

func TestCrossPlatformCoverageSearchMsgEnrichmentFailureKeepsSearchHits(t *testing.T) {
	caller := &searchMsgExecutionCaller{failEnrichment: true}
	payload := executeSearchMsg(t, caller, "--query", "周报")
	if payload["complete"] != false || payload["count"] != float64(1) ||
		payload["enrichedCount"] != float64(0) || payload["failedCount"] != float64(1) {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestCrossPlatformCoverageSearchMsgMissingPaginationCannotClaimComplete(t *testing.T) {
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

func TestCrossPlatformCoverageSearchMsgNumericZeroCursorIsComplete(t *testing.T) {
	caller := &searchMsgExecutionCaller{numericZeroEnd: true}
	payload := executeSearchMsg(t, caller, "--query", "周报", "--no-enrich")
	if payload["complete"] != true || payload["hasMore"] != false || payload["paginationKnown"] != true {
		t.Fatalf("numeric zero cursor pagination = %#v", payload)
	}
	if payload["nextCursor"] != "" {
		t.Fatalf("numeric zero cursor exposed as next page: %#v", payload["nextCursor"])
	}
}

func TestCrossPlatformCoverageSearchMsgMissingMgetItemPublishesFailureLedger(t *testing.T) {
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

func TestCrossPlatformCoverageSearchMsgPreservesAmbiguousPreflightMCPToolErrors(t *testing.T) {
	for _, test := range []struct {
		name string
		want *helpers.CLIError
	}{
		{
			name: "rate limited",
			want: &helpers.CLIError{
				Code:    helpers.CodeMCPToolError,
				Message: `{"success":false,"errorCode":"invalidRequest.rateLimited","errorMsg":"slow down"}`,
			},
		},
		{
			name: "permission denied",
			want: &helpers.CLIError{
				Code:    helpers.CodeMCPToolError,
				Message: `{"success":false,"errorCode":"forbidden.noPermission","errorMsg":"permission denied"}`,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			caller := &searchMsgExecutionCaller{preflightError: test.want}
			_, err := executeSearchMsgResult(caller,
				"--group", "cid-target", "--query", "周报", "--no-enrich")
			if err != test.want {
				t.Fatalf("error = %#v, want original %#v", err, test.want)
			}
			if len(caller.calls) != 1 || caller.calls[0].tool != "get_conversation_info" {
				t.Fatalf("calls = %#v", caller.calls)
			}
		})
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

func TestCrossPlatformCoverageSearchMsgScopeFailureBranches(t *testing.T) {
	want := errors.New("preflight unavailable")
	caller := &searchMsgExecutionCaller{preflightError: want}
	_, err := executeSearchMsgResult(caller, "--group", "cid-target", "--query", "周报", "--no-enrich")
	if !errors.Is(err, want) {
		t.Fatalf("preflight error = %v, want %v", err, want)
	}

	caller = &searchMsgExecutionCaller{
		searchResponse: `{"result":{"messages":[{"openMessageId":"m1","openConversationId":"cid-target","content":"sparse"}],"hasMore":false}}`,
		missingMgetCID: true,
	}
	_, err = executeSearchMsgResult(caller, "--group", "cid-target", "--query", "周报")
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Reason != "search_conversation_scope_unverified" {
		t.Fatalf("enrichment scope error = %#v", err)
	}

	err = searchScopeViolationError(
		[]string{"cid-target"},
		[]map[string]any{{"openMessageId": "m-empty"}, {"openMessageId": "m-other", "openConversationId": "cid-other"}},
	)
	typed = nil
	if !errors.As(err, &typed) || typed.Reason != "search_conversation_scope_violation" {
		t.Fatalf("scope violation error = %#v", err)
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

func TestCrossPlatformCoverageSearchMsgLarkTimeAliasesAndAscendingOrder(t *testing.T) {
	caller := &searchMsgExecutionCaller{
		firstResponse: `{"result":{"messages":[{"openMessageId":"m2","createTime":1782892800000,"content":"later"},{"openMessageId":"m1","createTime":1782806400000,"content":"earlier"}],"hasMore":false}}`,
	}
	payload := executeSearchMsg(t, caller,
		"--query", "周报",
		"--start-time", "2026-07-01T00:00:00+08:00",
		"--end-time", "2026-07-03T00:00:00+08:00",
		"--sort", "asc",
		"--no-enrich",
	)
	if len(caller.calls) != 1 {
		t.Fatalf("calls = %#v", caller.calls)
	}
	wantStart, _ := time.Parse(time.RFC3339, "2026-07-01T00:00:00+08:00")
	wantEnd, _ := time.Parse(time.RFC3339, "2026-07-03T00:00:00+08:00")
	if caller.calls[0].args["startTime"] != wantStart.UnixMilli() ||
		caller.calls[0].args["endTime"] != wantEnd.UnixMilli() {
		t.Fatalf("time params = %#v", caller.calls[0].args)
	}
	messages := payload["messages"].([]any)
	if messages[0].(map[string]any)["messageId"] != "m1" ||
		messages[1].(map[string]any)["messageId"] != "m2" {
		t.Fatalf("ascending messages = %#v", messages)
	}
	rangeMeta := payload["queryRange"].(map[string]any)
	if rangeMeta["order"] != "asc" || rangeMeta["semantics"] != "[start,end)" {
		t.Fatalf("queryRange = %#v", rangeMeta)
	}
}
