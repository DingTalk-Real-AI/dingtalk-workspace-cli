// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package helpers

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
)

func executeChatListAllConversations(
	t *testing.T,
	caller *pagedCommandCaller,
	args ...string,
) (map[string]any, error) {
	t.Helper()
	previousDeps := deps
	t.Cleanup(func() { deps = previousDeps })

	InitDeps(caller)
	var stdout, stderr bytes.Buffer
	deps.Out.w = &stdout
	deps.Out.errW = &stderr
	cmd := newChatCommand()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err := corecmd.ExecuteForTest(cmd)
	if stdout.Len() == 0 {
		return nil, err
	}
	var payload map[string]any
	if decodeErr := json.Unmarshal(stdout.Bytes(), &payload); decodeErr != nil {
		t.Fatalf("decode stdout %q: %v", stdout.String(), decodeErr)
	}
	return payload, err
}

func testChatListAllConversationsPageAllAggregatesSupportedResponses(t *testing.T) {
	t.Helper()
	tests := []struct {
		name      string
		responses []scriptedToolStep
	}{
		{
			name: "map aliases",
			responses: []scriptedToolStep{
				{text: `{"result":{"conversationList":[{"openConversationId":"cid-1","title":"first"}],"hasMore":true,"nextCursor":2}}`},
				{text: `{"result":{"conversations":[{"openConversationId":"cid-1","title":"duplicate"},{"openConversationId":"cid-2"}],"hasMore":false}}`},
			},
		},
		{
			name: "gateway tuples",
			responses: []scriptedToolStep{
				{text: `{"result":[[{"openConversationId":"cid-1","title":"first"}],2,true]}`},
				{text: `{"result":[[{"openConversationId":"cid-2"}],0,false]}`},
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			caller := &pagedCommandCaller{steps: testCase.responses}
			payload, err := executeChatListAllConversations(
				t,
				caller,
				"list-all-conversations", "--page-all", "--page-limit", "2", "--page-delay", "0",
			)
			if err != nil {
				t.Fatalf("list-all-conversations: %v", err)
			}
			if len(caller.calls) != 2 {
				t.Fatalf("calls = %#v, want two pages", caller.calls)
			}
			if caller.calls[0].server != "im" || caller.calls[0].tool != "list_all_conversations" {
				t.Fatalf("first call = %#v", caller.calls[0])
			}
			if got := caller.calls[0].args; !reflect.DeepEqual(got, map[string]any{"limit": 100}) {
				t.Fatalf("first args = %#v", got)
			}
			if got := caller.calls[1].args["cursor"]; got != int64(2) {
				t.Fatalf("second cursor = %#v, want int64(2)", got)
			}

			result, ok := payload["result"].(map[string]any)
			if !ok {
				t.Fatalf("result = %#v", payload["result"])
			}
			items, ok := result["conversationList"].([]any)
			if !ok || len(items) != 2 {
				t.Fatalf("conversationList = %#v", result["conversationList"])
			}
			first := items[0].(map[string]any)
			second := items[1].(map[string]any)
			if first["openConversationId"] != "cid-1" || first["title"] != "first" || second["openConversationId"] != "cid-2" {
				t.Fatalf("conversationList = %#v", items)
			}
			paging, ok := payload["paging"].(map[string]any)
			if !ok || paging["pages"] != float64(2) || paging["total"] != float64(2) || paging["hasMore"] != false {
				t.Fatalf("paging = %#v", payload["paging"])
			}
		})
	}
}

func TestChatListAllConversationsPageAllAggregatesSupportedResponses(t *testing.T) {
	testChatListAllConversationsPageAllAggregatesSupportedResponses(t)
}

func TestCrossPlatformCoverageChatListAllConversationsPreservesInt64Cursor(t *testing.T) {
	for _, response := range []string{
		`{"result":{"conversationList":[],"hasMore":true,"nextCursor":9007199254740993}}`,
		`{"result":[[],9007199254740993,true]}`,
		`{"result":{"conversationList":[],"hasMore":true,"nextCursor":9223372036854775807}}`,
		`{"result":[[],9223372036854775807,true]}`,
	} {
		t.Run(response, func(t *testing.T) {
			caller := &pagedCommandCaller{steps: []scriptedToolStep{
				{text: response},
				{text: `{"result":{"conversationList":[],"hasMore":false}}`},
			}}
			_, err := executeChatListAllConversations(t, caller,
				"list-all-conversations", "--page-all", "--page-limit", "2", "--page-delay", "0")
			if err != nil {
				t.Fatalf("list-all-conversations: %v", err)
			}
			want := int64(9007199254740993)
			if strings.Contains(response, "9223372036854775807") {
				want = int64(9223372036854775807)
			}
			if len(caller.calls) != 2 || caller.calls[1].args["cursor"] != want {
				t.Fatalf("calls = %#v, want second-page cursor %d", caller.calls, want)
			}
		})
	}
}

func TestCrossPlatformCoverageChatListAllConversationsDefaultKeepsSinglePageResponse(t *testing.T) {
	caller := &pagedCommandCaller{steps: []scriptedToolStep{{
		text: `{"result":{"conversations":[{"openConversationId":"cid-1"}],"hasMore":true,"nextCursor":2}}`,
	}}}
	payload, err := executeChatListAllConversations(
		t,
		caller,
		"list-all-conversations", "--limit", "50", "--cursor", "7", "--exclude-muted",
	)
	if err != nil {
		t.Fatalf("list-all-conversations: %v", err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("calls = %#v, want one page", caller.calls)
	}
	if got := caller.calls[0].args; !reflect.DeepEqual(got, map[string]any{
		"limit":        50,
		"cursor":       int64(7),
		"excludeMuted": true,
	}) {
		t.Fatalf("args = %#v", got)
	}
	if _, exists := payload["paging"]; exists {
		t.Fatalf("single-page response unexpectedly contains paging: %#v", payload)
	}
	result, ok := payload["result"].(map[string]any)
	if !ok || result["conversations"] == nil || result["conversationList"] != nil {
		t.Fatalf("single-page response was normalized: %#v", payload)
	}
}

func TestCrossPlatformCoverageChatListAllConversationsRejectsUnsupportedPagedResponses(t *testing.T) {
	tests := []struct {
		name      string
		response  string
		wantError string
	}{
		{
			name:      "map items are not array",
			response:  `{"result":{"conversationList":{},"hasMore":false}}`,
			wantError: "result.conversationList must be array",
		},
		{
			name:      "map omits hasMore",
			response:  `{"result":{"conversationList":[]}}`,
			wantError: "missing result.hasMore",
		},
		{
			name:      "map hasMore is not boolean",
			response:  `{"result":{"conversationList":[],"hasMore":"yes"}}`,
			wantError: "result.hasMore must be boolean",
		},
		{
			name:      "tuple items are not array",
			response:  `{"result":[{},0,false]}`,
			wantError: "result[0] must be array",
		},
		{
			name:      "tuple hasMore is not boolean",
			response:  `{"result":[[],0,"yes"]}`,
			wantError: "result[2] must be boolean",
		},
		{
			name:      "result has unsupported shape",
			response:  `{"result":"invalid"}`,
			wantError: "result must be object or gateway tuple",
		},
		{
			name:      "map hasMore omits next cursor",
			response:  `{"result":{"conversationList":[],"hasMore":true}}`,
			wantError: "missing result.nextCursor",
		},
		{
			name:      "map cursor overflows int64",
			response:  `{"result":{"conversationList":[],"hasMore":true,"nextCursor":9223372036854775808}}`,
			wantError: "nextCursor must be int64",
		},
		{
			name:      "tuple cursor is fractional",
			response:  `{"result":[[],1.5,true]}`,
			wantError: "nextCursor must be int64",
		},
		{
			name:      "trailing JSON",
			response:  `{"result":[[],0,false]} {}`,
			wantError: "exactly one JSON object",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			_, _, _, _, err := parseChatListAllConversationsPage(testCase.response)
			if err == nil || !strings.Contains(err.Error(), testCase.wantError) {
				t.Fatalf("parse error = %v, want containing %q", err, testCase.wantError)
			}
		})
	}
}

func TestCrossPlatformCoverageChatListAllConversationsPageAllAggregatesSupportedResponses(t *testing.T) {
	testChatListAllConversationsPageAllAggregatesSupportedResponses(t)
}
