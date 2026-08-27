// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/agentproduct"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type chatMessageSearchCall struct {
	productID string
	toolName  string
	args      map[string]any
}

func chatCallsByTool(calls []chatMessageSearchCall, toolName string) []chatMessageSearchCall {
	filtered := make([]chatMessageSearchCall, 0, 1)
	for _, call := range calls {
		if call.toolName == toolName {
			filtered = append(filtered, call)
		}
	}
	return filtered
}

type chatMessageSearchCaller struct {
	calls           []chatMessageSearchCall
	searchResponse  string
	searchResponses []string
	searchCalls     int
	searchError     error
	failPreflight   bool
	preflightError  error
}

func (c *chatMessageSearchCaller) CallTool(_ context.Context, productID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, chatMessageSearchCall{productID: productID, toolName: toolName, args: args})
	text := `{}`
	if toolName == "get_conversation_info" {
		if c.preflightError != nil {
			return nil, c.preflightError
		}
		if c.failPreflight {
			return nil, errors.New("conversation not found")
		}
		text = `{"result":{"openConversationId":"` + args["openConversationId"].(string) + `","convThreadEnabled":false}}`
	}
	if toolName == "search_messages_by_keyword" || toolName == "search_messages" {
		if c.searchError != nil {
			c.searchCalls++
			return nil, c.searchError
		}
		text = `{"result":{"messages":[],"hasMore":false}}`
		if c.searchCalls < len(c.searchResponses) {
			text = c.searchResponses[c.searchCalls]
		} else if c.searchResponse != "" {
			text = c.searchResponse
		}
		c.searchCalls++
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: text}}}, nil
}

func (*chatMessageSearchCaller) Format() string { return "json" }
func (*chatMessageSearchCaller) DryRun() bool   { return false }
func (*chatMessageSearchCaller) Fields() string { return "" }
func (*chatMessageSearchCaller) JQ() string     { return "" }

func TestCrossPlatformCoverageChatMessageSearchUsesMCPContracts(t *testing.T) {
	previousDeps := deps
	previousArgs := os.Args
	t.Cleanup(func() {
		deps = previousDeps
		os.Args = previousArgs
	})

	start := "2026-07-09T00:00:00+08:00"
	end := "2026-07-11T00:00:00+08:00"
	startTime, err := time.Parse(time.RFC3339, start)
	if err != nil {
		t.Fatalf("parse start time: %v", err)
	}
	endTime, err := time.Parse(time.RFC3339, end)
	if err != nil {
		t.Fatalf("parse end time: %v", err)
	}

	tests := []struct {
		name        string
		args        []string
		productID   string
		toolName    string
		wantToolArg map[string]any
		preflight   []string
	}{
		{
			name:      "keyword search",
			args:      []string{"message", "search", "--query", "categoryName", "--group", "cid-1", "--start", start, "--end", end, "--limit", "100", "--cursor", "0"},
			productID: "chat",
			toolName:  "search_messages_by_keyword",
			wantToolArg: map[string]any{
				"keyword":   "categoryName",
				"startTime": startTime.UnixMilli(),
				"endTime":   endTime.UnixMilli(),
				"limit":     100,
				"cursor":    "0",
			},
			preflight: []string{"cid-1"},
		},
		{
			name:      "advanced search",
			args:      []string{"message", "search-advanced", "--query", "categoryName", "--conversation-ids", "cid-1,cid-2", "--message-type", "text", "--only-robot", "--conversation-type", "group", "--start", start, "--end", end, "--limit", "100", "--cursor", "0"},
			productID: "im",
			toolName:  "search_messages",
			wantToolArg: map[string]any{
				"keyword":           "categoryName",
				"messageType":       "text",
				"onlyRobotMessages": true,
				"searchConvType":    "group",
				"startTime":         startTime.UnixMilli(),
				"endTime":           endTime.UnixMilli(),
				"limit":             100,
				"cursor":            "0",
			},
			preflight: []string{"cid-1", "cid-2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caller := &chatMessageSearchCaller{}
			InitDeps(caller)
			deps.Out.w = io.Discard
			os.Args = []string{"dws", "chat"}

			cmd := newChatCommand()
			cmd.SilenceErrors = true
			cmd.SilenceUsage = true
			cmd.SetOut(io.Discard)
			cmd.SetArgs(tt.args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("chat search returned error: %v", err)
			}
			if len(caller.calls) != len(tt.preflight)+1 {
				t.Fatalf("tool calls = %#v", caller.calls)
			}
			for index, conversationID := range tt.preflight {
				call := caller.calls[index]
				if call.productID != "chat" || call.toolName != "get_conversation_info" || call.args["openConversationId"] != conversationID {
					t.Fatalf("preflight[%d] = %#v", index, call)
				}
			}
			call := caller.calls[len(caller.calls)-1]
			if call.productID != tt.productID || call.toolName != tt.toolName {
				t.Fatalf("tool call = %s/%s, want %s/%s", call.productID, call.toolName, tt.productID, tt.toolName)
			}
			if !reflect.DeepEqual(call.args, tt.wantToolArg) {
				t.Fatalf("tool args = %#v, want %#v", call.args, tt.wantToolArg)
			}
		})
	}
}

func executeNativeScopedSearch(t *testing.T, caller *chatMessageSearchCaller, args ...string) (map[string]any, error) {
	t.Helper()
	previousDeps := deps
	t.Cleanup(func() { deps = previousDeps })
	InitDeps(caller)
	deps.Out.w = io.Discard
	cmd := newChatCommand()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	var output strings.Builder
	cmd.SetOut(&output)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(output.String()), &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func TestNativeScopedSearchFiltersGlobalResultsForBothEntries(t *testing.T) {
	start := "2026-07-09T00:00:00+08:00"
	end := "2026-07-11T00:00:00+08:00"
	for _, tt := range []struct {
		name       string
		args       []string
		tool       string
		scopeParam string
	}{
		{
			name:       "keyword search",
			args:       []string{"message", "search", "--query", "周报", "--group", "cid-target", "--start", start, "--end", end},
			tool:       "search_messages_by_keyword",
			scopeParam: "openConversationId",
		},
		{
			name:       "advanced search",
			args:       []string{"message", "search-advanced", "--query", "周报", "--conversation-ids", "cid-target"},
			tool:       "search_messages",
			scopeParam: "openConversationIds",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			caller := &chatMessageSearchCaller{searchResponse: `{
				"result": {
					"conversationMessagesList": [
						{"openConversationId":"cid-target","title":"目标群","messages":[{"openMessageId":"m-target","content":"目标"}]},
						{"openConversationId":"cid-other","title":"其他群","messages":[{"openMessageId":"m-other","content":"越界"}]}
					],
					"hasMore": false
				}
			}`}
			payload, err := executeNativeScopedSearch(t, caller, tt.args...)
			if err != nil {
				t.Fatal(err)
			}
			result, _ := payload["result"].(map[string]any)
			groups, _ := result["conversationMessagesList"].([]any)
			if len(groups) != 1 {
				t.Fatalf("result = %#v", result)
			}
			group, _ := groups[0].(map[string]any)
			if group["openConversationId"] != "cid-target" {
				t.Fatalf("group = %#v", group)
			}
			scope, _ := payload["scope"].(map[string]any)
			if scope["targetsValidated"] != true || scope["resultsWithinScope"] != true || scope["filterMode"] != "client" {
				t.Fatalf("scope = %#v", scope)
			}
			searchCall := caller.calls[len(caller.calls)-1]
			if searchCall.toolName != tt.tool {
				t.Fatalf("search call = %#v", searchCall)
			}
			if _, exists := searchCall.args[tt.scopeParam]; exists {
				t.Fatalf("global fallback unexpectedly forwarded %s: %#v", tt.scopeParam, searchCall.args)
			}
		})
	}
}

func TestNativeScopedSearchInvalidCIDStopsBeforeSearch(t *testing.T) {
	caller := &chatMessageSearchCaller{failPreflight: true}
	_, err := executeNativeScopedSearch(t, caller,
		"message", "search-advanced", "--query", "周报", "--conversation-ids", "cid-invalid")
	if err == nil {
		t.Fatal("invalid CID unexpectedly succeeded")
	}
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Reason != "search_conversation_scope_invalid" {
		t.Fatalf("error = %#v", err)
	}
	if len(caller.calls) != 1 || caller.calls[0].toolName != "get_conversation_info" {
		t.Fatalf("calls = %#v", caller.calls)
	}
}

func TestNativeScopedSearchPreservesPreflightAuthError(t *testing.T) {
	want := &CLIError{Code: CodeAuthNotConfigured, Message: "当前未登录"}
	caller := &chatMessageSearchCaller{preflightError: want}
	_, err := executeNativeScopedSearch(t, caller,
		"message", "search-advanced", "--query", "周报", "--conversation-ids", "cid-target")
	if err == nil {
		t.Fatal("auth failure unexpectedly succeeded")
	}
	var cliErr *CLIError
	if !errors.As(err, &cliErr) || cliErr.Code != CodeAuthNotConfigured {
		t.Fatalf("error = %#v", err)
	}
}

func TestCrossPlatformCoverageNativeScopedSearchPreservesAmbiguousMCPToolErrors(t *testing.T) {
	for _, test := range []struct {
		name string
		want *CLIError
	}{
		{
			name: "rate limited",
			want: &CLIError{
				Code:    CodeMCPToolError,
				Message: `{"success":false,"errorCode":"invalidRequest.rateLimited","errorMsg":"slow down"}`,
			},
		},
		{
			name: "permission denied",
			want: &CLIError{
				Code:    CodeMCPToolError,
				Message: `{"success":false,"errorCode":"forbidden.noPermission","errorMsg":"permission denied"}`,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			caller := &chatMessageSearchCaller{preflightError: test.want}
			_, err := executeNativeScopedSearch(t, caller,
				"message", "search-advanced", "--query", "周报", "--conversation-ids", "cid-target")
			if err != test.want {
				t.Fatalf("error = %#v, want original %#v", err, test.want)
			}
			if len(caller.calls) != 1 || caller.calls[0].toolName != "get_conversation_info" {
				t.Fatalf("calls = %#v", caller.calls)
			}
		})
	}
}

func TestNativeScopedSearchScansUntilTargetConversationAppears(t *testing.T) {
	caller := &chatMessageSearchCaller{searchResponses: []string{
		`{"result":{"conversationMessagesList":[{"openConversationId":"cid-other","messages":[{"openMessageId":"m-other"}]}],"hasMore":true,"nextCursor":"c2"}}`,
		`{"result":{"conversationMessagesList":[{"openConversationId":"cid-target","messages":[{"openMessageId":"m-target"}]}],"hasMore":false}}`,
	}}
	payload, err := executeNativeScopedSearch(t, caller,
		"message", "search-advanced", "--query", "周报", "--conversation-ids", "cid-target")
	if err != nil {
		t.Fatal(err)
	}
	result, _ := payload["result"].(map[string]any)
	if result["pagesFetched"] != float64(2) || result["complete"] != true {
		t.Fatalf("result = %#v", result)
	}
	groups, _ := result["conversationMessagesList"].([]any)
	if len(groups) != 1 {
		t.Fatalf("groups = %#v", groups)
	}
	searchCalls := make([]chatMessageSearchCall, 0, 2)
	for _, call := range caller.calls {
		if call.toolName == "search_messages" {
			searchCalls = append(searchCalls, call)
		}
	}
	if len(searchCalls) != 2 || searchCalls[1].args["cursor"] != "c2" {
		t.Fatalf("search calls = %#v", searchCalls)
	}
}

func TestCrossPlatformCoverageNativeScopedSearchPageAllOptions(t *testing.T) {
	t.Run("page limit preserves continuation", func(t *testing.T) {
		caller := &chatMessageSearchCaller{searchResponse: `{
			"result": {
				"conversationMessagesList": [
					{"openConversationId":"cid-target","messages":[{"openMessageId":"m1"}]}
				],
				"hasMore": true,
				"nextCursor": "c2"
			}
		}`}
		payload, err := executeNativeScopedSearch(t, caller,
			"message", "search-advanced", "--query", "周报", "--conversation-ids", "cid-target",
			"--page-all", "--page-limit", "1", "--page-delay", "0")
		if err != nil {
			t.Fatal(err)
		}
		paging, _ := payload["paging"].(map[string]any)
		if paging["pages"] != float64(1) || paging["total"] != float64(1) || paging["truncated"] != true {
			t.Fatalf("paging = %#v", paging)
		}
		if caller.searchCalls != 1 {
			t.Fatalf("search calls = %d, want 1", caller.searchCalls)
		}
	})

	t.Run("max items truncates within filtered page", func(t *testing.T) {
		caller := &chatMessageSearchCaller{searchResponse: `{
			"result": {
				"conversationMessagesList": [
					{"openConversationId":"cid-target","messages":[
						{"openMessageId":"m1"},
						{"openMessageId":"m2"}
					]}
				],
				"hasMore": false
			}
		}`}
		payload, err := executeNativeScopedSearch(t, caller,
			"message", "search-advanced", "--query", "周报", "--conversation-ids", "cid-target",
			"--page-all", "--max-items", "1", "--page-delay", "0")
		if err != nil {
			t.Fatal(err)
		}
		result, _ := payload["result"].(map[string]any)
		groups, _ := result["conversationMessagesList"].([]any)
		group, _ := groups[0].(map[string]any)
		messages, _ := group["messages"].([]any)
		if len(messages) != 1 {
			t.Fatalf("messages = %#v", messages)
		}
		paging, _ := payload["paging"].(map[string]any)
		if paging["total"] != float64(1) || paging["truncatedWithinPage"] != true || paging["resumeCursorReliable"] != false {
			t.Fatalf("paging = %#v", paging)
		}
	})
}

func TestNativeScopedSearchMissingConversationIdentityFailsClosed(t *testing.T) {
	caller := &chatMessageSearchCaller{searchResponse: `{"result":{"messages":[{"openMessageId":"m1"}],"hasMore":false}}`}
	_, err := executeNativeScopedSearch(t, caller,
		"message", "search-advanced", "--query", "周报", "--conversation-ids", "cid-target")
	if err == nil {
		t.Fatal("unverifiable scoped result unexpectedly succeeded")
	}
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Reason != "search_conversation_scope_unverified" {
		t.Fatalf("error = %#v", err)
	}
}

func TestNativeScopedSearchValidEmptyResultIsComplete(t *testing.T) {
	caller := &chatMessageSearchCaller{searchResponse: `{"result":{"messages":[],"hasMore":false}}`}
	payload, err := executeNativeScopedSearch(t, caller,
		"message", "search-advanced", "--query", "周报", "--conversation-ids", "cid-empty")
	if err != nil {
		t.Fatal(err)
	}
	result, _ := payload["result"].(map[string]any)
	if result["complete"] != true || result["hasMore"] != false {
		t.Fatalf("result = %#v", result)
	}
	scope, _ := payload["scope"].(map[string]any)
	if scope["targetsValidated"] != true || scope["sourceComplete"] != true {
		t.Fatalf("scope = %#v", scope)
	}
}

func TestCrossPlatformCoverageNativeScopedSearchFailureAndPaginationBranches(t *testing.T) {
	t.Run("empty scope uses the native search call", func(t *testing.T) {
		caller := &chatMessageSearchCaller{}
		previousDeps := deps
		t.Cleanup(func() { deps = previousDeps })
		InitDeps(caller)
		deps.Out.w = io.Discard

		cmd := &cobra.Command{Use: "search"}
		err := runConversationScopedMessageSearch(
			cmd,
			"im",
			"search_messages",
			"openConversationIds",
			map[string]any{"keyword": "周报"},
			[]string{"", " "},
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(caller.calls) != 1 || caller.calls[0].toolName != "search_messages" {
			t.Fatalf("calls = %#v", caller.calls)
		}
	})

	t.Run("invalid page options fail before preflight", func(t *testing.T) {
		caller := &chatMessageSearchCaller{}
		previousDeps := deps
		t.Cleanup(func() { deps = previousDeps })
		InitDeps(caller)

		cmd := &cobra.Command{Use: "search"}
		AddPagedMCPFlags(cmd)
		if err := cmd.Flags().Set("page-all", "true"); err != nil {
			t.Fatal(err)
		}
		if err := cmd.Flags().Set("page-limit", "0"); err != nil {
			t.Fatal(err)
		}
		err := runConversationScopedMessageSearch(
			cmd,
			"im",
			"search_messages",
			"openConversationIds",
			map[string]any{"keyword": "周报"},
			[]string{"cid-target"},
		)
		if err == nil || !strings.Contains(err.Error(), "--page-limit must be between 1 and 500") {
			t.Fatalf("error = %v", err)
		}
		if len(caller.calls) != 0 {
			t.Fatalf("invalid paging made calls: %#v", caller.calls)
		}
	})

	t.Run("cancelled context interrupts page delay", func(t *testing.T) {
		caller := &chatMessageSearchCaller{searchResponse: `{
			"result": {
				"messages": [{"openMessageId":"m1","openConversationId":"cid-target"}],
				"hasMore": true,
				"nextCursor": "c2"
			}
		}`}
		previousDeps := deps
		t.Cleanup(func() { deps = previousDeps })
		InitDeps(caller)
		deps.Out.w = io.Discard

		cmd := &cobra.Command{Use: "search"}
		AddPagedMCPFlags(cmd)
		if err := cmd.Flags().Set("page-all", "true"); err != nil {
			t.Fatal(err)
		}
		if err := cmd.Flags().Set("page-delay", "60000"); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		cmd.SetContext(ctx)
		err := runConversationScopedMessageSearch(
			cmd,
			"im",
			"search_messages",
			"openConversationIds",
			map[string]any{"keyword": "周报", "limit": 100, "cursor": "0"},
			[]string{"cid-target"},
		)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context canceled", err)
		}
		if caller.searchCalls != 1 {
			t.Fatalf("search calls = %d, want 1", caller.searchCalls)
		}
	})

	t.Run("lower search error", func(t *testing.T) {
		caller := &chatMessageSearchCaller{searchError: errors.New("search unavailable")}
		_, err := executeNativeScopedSearch(t, caller,
			"message", "search-advanced", "--query", "周报", "--conversation-ids", "cid-target")
		if err == nil {
			t.Fatal("lower search error was ignored")
		}
	})

	t.Run("stalled cursor fails closed", func(t *testing.T) {
		caller := &chatMessageSearchCaller{searchResponse: `{
			"result": {
				"messages": [{"openMessageId":"m1","openConversationId":"cid-target"}],
				"hasMore": true
			}
		}`}
		_, err := executeNativeScopedSearch(t, caller,
			"message", "search-advanced", "--query", "周报", "--conversation-ids", "cid-target")
		var typed *apperrors.Error
		if !errors.As(err, &typed) || typed.Reason != "search_conversation_scope_cursor_stalled" {
			t.Fatalf("error = %#v", err)
		}
	})

	t.Run("result limit preserves continuation", func(t *testing.T) {
		caller := &chatMessageSearchCaller{searchResponse: `{
			"result": {
				"messages": [{"openMessageId":"m1","openConversationId":"cid-target"}],
				"hasMore": true,
				"nextCursor": "c2"
			}
		}`}
		payload, err := executeNativeScopedSearch(t, caller,
			"message", "search-advanced", "--query", "周报", "--conversation-ids", "cid-target", "--limit", "1")
		if err != nil {
			t.Fatal(err)
		}
		result, _ := payload["result"].(map[string]any)
		if result["complete"] != false || result["hasMore"] != true || result["nextCursor"] != "c2" {
			t.Fatalf("result = %#v", result)
		}
	})

	t.Run("duplicate message ids are removed across pages", func(t *testing.T) {
		caller := &chatMessageSearchCaller{searchResponses: []string{
			`{"result":{"messages":[{"openMessageId":"m1","openConversationId":"cid-target"}],"hasMore":true,"nextCursor":"c2"}}`,
			`{"result":{"messages":[{"openMessageId":"m1","openConversationId":"cid-target"},{"openMessageId":"m2","openConversationId":"cid-target"}],"hasMore":false}}`,
		}}
		payload, err := executeNativeScopedSearch(t, caller,
			"message", "search-advanced", "--query", "周报", "--conversation-ids", "cid-target")
		if err != nil {
			t.Fatal(err)
		}
		result, _ := payload["result"].(map[string]any)
		groups, _ := result["conversationMessagesList"].([]any)
		group, _ := groups[0].(map[string]any)
		messages, _ := group["messages"].([]any)
		if len(messages) != 2 {
			t.Fatalf("deduplicated messages = %#v", messages)
		}
	})

	if got := uniqueNonEmptyStrings([]string{" cid ", "", "cid"}); !reflect.DeepEqual(got, []string{"cid"}) {
		t.Fatalf("uniqueNonEmptyStrings = %#v", got)
	}
	for _, test := range []struct {
		value any
		want  int
	}{
		{value: int64(7), want: 7},
		{value: json.Number("8"), want: 8},
		{value: float64(9), want: 9},
		{value: int64(0), want: 11},
	} {
		if got := positiveSearchLimit(test.value, 11); got != test.want {
			t.Errorf("positiveSearchLimit(%#v) = %d, want %d", test.value, got, test.want)
		}
	}
	if cleanSearchCursor(nil) != "" || cleanSearchCursor(" null ") != "" || cleanSearchCursor(" c2 ") != "c2" {
		t.Fatal("cleanSearchCursor did not normalize sentinel values")
	}
}

func TestCrossPlatformCoverageNativeScopedSearchDryRunShowsCompositePlanWithoutCallingTools(t *testing.T) {
	caller := &chatMessageSearchCaller{}
	previousDeps := deps
	t.Cleanup(func() { deps = previousDeps })
	InitDeps(caller)
	cmd := &cobra.Command{Use: "search"}
	cmd.Flags().Bool("dry-run", true, "")
	AddPagedMCPFlags(cmd)
	for name, value := range map[string]string{
		"page-all":   "true",
		"page-limit": "7",
		"max-items":  "9",
		"page-delay": "11",
	} {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("set --%s: %v", name, err)
		}
	}
	var output strings.Builder
	cmd.SetOut(&output)
	err := runConversationScopedMessageSearch(
		cmd,
		"im",
		"search_messages",
		"openConversationIds",
		map[string]any{
			"keyword":             "周报",
			"openConversationIds": []string{"cid-target"},
			"limit":               100,
			"cursor":              "0",
		},
		[]string{"cid-target"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("dry-run made calls: %#v", caller.calls)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(output.String()), &payload); err != nil {
		t.Fatal(err)
	}
	plan, _ := payload["plan"].([]any)
	if payload["dry_run"] != true || payload["executed"] != false || len(plan) != 3 {
		t.Fatalf("payload = %#v", payload)
	}
	searchStage, _ := plan[1].(map[string]any)
	arguments, _ := searchStage["arguments"].(map[string]any)
	if _, exists := arguments["openConversationIds"]; exists {
		t.Fatalf("dry-run global search still carries scope: %#v", searchStage)
	}
	if searchStage["pageAll"] != true ||
		searchStage["pageLimit"] != float64(7) ||
		searchStage["maxItems"] != float64(9) ||
		searchStage["pageDelay"] != float64(11) {
		t.Fatalf("dry-run paging = %#v", searchStage)
	}
}

type chatChangedContractCaller struct {
	calls        []chatMessageSearchCall
	resolveUsers bool
}

func (c *chatChangedContractCaller) CallTool(_ context.Context, productID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, chatMessageSearchCall{productID: productID, toolName: toolName, args: args})
	text := `{}`
	if toolName == "list_messages_by_ids" {
		messageID := args["openMsgIds"].([]string)[0]
		text = `{"result":[{"openMessageId":"` + messageID + `","openConversationId":"cid"}]}`
	}
	if toolName == "get_conversation_info" {
		text = `{"result":{"openConversationId":"` + args["openConversationId"].(string) + `","convThreadEnabled":false}}`
	}
	if c.resolveUsers && toolName == "get_user_info_by_user_ids" {
		text = `{"result":[{"userId":"123","openDingTalkId":"open-123"}]}`
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: text}}}, nil
}

func (*chatChangedContractCaller) Format() string { return "json" }
func (*chatChangedContractCaller) DryRun() bool   { return false }
func (*chatChangedContractCaller) Fields() string { return "" }
func (*chatChangedContractCaller) JQ() string     { return "" }

func executeChatChangedContract(t *testing.T, caller *chatChangedContractCaller, args ...string) error {
	t.Helper()
	previousDeps := deps
	t.Cleanup(func() { deps = previousDeps })
	InitDeps(caller)
	deps.Out.w = io.Discard
	cmd := newChatCommand()
	installExampleGlobalFlags(cmd)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs(append(append([]string(nil), args...), "--yes"))
	return cmd.Execute()
}

func TestCrossPlatformCoverageChatMessageListUsesMCPMetadataGroupKey(t *testing.T) {
	caller := &chatChangedContractCaller{}
	err := executeChatChangedContract(t, caller,
		"message", "list", "--group", "cid-1", "--time", "2026-07-15 09:00:00", "--limit", "50")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 || caller.calls[0].toolName != "list_conversation_message_v2" {
		t.Fatalf("calls = %#v", caller.calls)
	}
	want := map[string]any{"openconversation_id": "cid-1", "time": "2026-07-15 09:00:00", "forward": true, "limit": 50}
	if !reflect.DeepEqual(caller.calls[0].args, want) {
		t.Fatalf("tool args = %#v, want %#v", caller.calls[0].args, want)
	}
}

func TestCrossPlatformCoverageChatMessageListDefaultTimeUsesShanghaiLocation(t *testing.T) {
	previousLocal := time.Local
	t.Cleanup(func() { time.Local = previousLocal })

	tests := []struct {
		name       string
		args       []string
		wantTool   string
		wantTarget map[string]any
	}{
		{
			name:       "group",
			args:       []string{"message", "list", "--group", "cid-1", "--limit", "50"},
			wantTool:   "list_conversation_message_v2",
			wantTarget: map[string]any{"openconversation_id": "cid-1"},
		},
		{
			name:       "user",
			args:       []string{"message", "list", "--user", "user-1", "--limit", "50"},
			wantTool:   "list_individual_chat_message",
			wantTarget: map[string]any{"userId": "user-1"},
		},
		{
			name:       "user open DingTalk ID fallback",
			args:       []string{"message", "list", "--user", helperCurrentDOpenID, "--limit", "50"},
			wantTool:   "list_individual_chat_message",
			wantTarget: map[string]any{"openDingTalkId": helperCurrentDOpenID},
		},
		{
			name:       "open DingTalk ID",
			args:       []string{"message", "list", "--open-dingtalk-id", helperCurrentDOpenID, "--limit", "50"},
			wantTool:   "list_individual_chat_message",
			wantTarget: map[string]any{"openDingTalkId": helperCurrentDOpenID},
		},
		{
			name:       "direct user",
			args:       []string{"message", "list-direct", "--user", "user-1", "--limit", "50"},
			wantTool:   "list_individual_chat_message",
			wantTarget: map[string]any{"userId": "user-1"},
		},
		{
			name:       "direct open DingTalk ID",
			args:       []string{"message", "list-direct", "--open-dingtalk-id", helperCurrentDOpenID, "--limit", "50"},
			wantTool:   "list_individual_chat_message",
			wantTarget: map[string]any{"openDingTalkId": helperCurrentDOpenID},
		},
	}

	for _, loc := range []*time.Location{time.UTC, time.FixedZone("EST", -5*3600)} {
		t.Run(loc.String(), func(t *testing.T) {
			time.Local = loc
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					caller := &chatChangedContractCaller{}
					before := time.Now()
					err := executeChatChangedContract(t, caller, tt.args...)
					after := time.Now()
					if err != nil {
						t.Fatal(err)
					}
					if len(caller.calls) != 1 {
						t.Fatalf("calls = %#v, want one MCP call", caller.calls)
					}
					if caller.calls[0].toolName != tt.wantTool {
						t.Fatalf("tool = %q, want %q", caller.calls[0].toolName, tt.wantTool)
					}
					for key, want := range tt.wantTarget {
						if got := caller.calls[0].args[key]; got != want {
							t.Fatalf("arg %s = %#v, want %#v", key, got, want)
						}
					}
					raw, ok := caller.calls[0].args["time"].(string)
					if !ok || raw == "" {
						t.Fatalf("time arg = %#v, want non-empty string", caller.calls[0].args["time"])
					}
					gotMs, err := parseISOTimeToMillis("time", raw)
					if err != nil {
						t.Fatalf("time arg = %q, parse err = %v", raw, err)
					}
					wantMin := before.Add(-time.Second).UnixMilli()
					wantMax := after.Add(time.Second).UnixMilli()
					if gotMs < wantMin || gotMs > wantMax {
						t.Fatalf("time arg = %q (%d), want between %d and %d", raw, gotMs, wantMin, wantMax)
					}
					if caller.calls[0].args["forward"] != false {
						t.Fatalf("forward = %#v, want false when --time is omitted", caller.calls[0].args["forward"])
					}
				})
			}
		})
	}
}

func TestCrossPlatformCoverageChatAuditUsesUserIDs(t *testing.T) {
	caller := &chatChangedContractCaller{}
	err := executeChatChangedContract(t, caller,
		"group", "audit-join-validation",
		"--group", "cid-1", "--record-id", "123", "--applicant", "user-a", "--inviter", "user-b", "--status", "AuditApprove")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 || caller.calls[0].productID != "im" || caller.calls[0].toolName != "audit_join_group" {
		t.Fatalf("calls = %#v", caller.calls)
	}
	want := map[string]any{
		"openConversationId": "cid-1", "applyRecordId": int64(123),
		"applicantUid": "user-a", "inviterUid": "user-b", "status": "AuditApprove",
	}
	if !reflect.DeepEqual(caller.calls[0].args, want) {
		t.Fatalf("tool args = %#v, want %#v", caller.calls[0].args, want)
	}
}

func TestCrossPlatformCoverageChatAuditRejectsUnsupportedStatus(t *testing.T) {
	caller := &chatChangedContractCaller{}
	err := executeChatChangedContract(t, caller,
		"group", "audit-join-validation",
		"--group", "cid-1", "--record-id", "123", "--applicant", "user-a", "--inviter", "user-b", "--status", "AuditRefuse")
	if err == nil {
		t.Fatal("expected unsupported audit status error")
	}
	if !strings.Contains(err.Error(), `unsupported audit status "AuditRefuse"`) {
		t.Fatalf("error = %v, want unsupported status", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("unsupported status must not call MCP: %#v", caller.calls)
	}
}

func TestCrossPlatformCoverageChatSendResolvesUserBeforeDispatch(t *testing.T) {
	caller := &chatChangedContractCaller{resolveUsers: true}
	err := executeChatChangedContract(t, caller, "message", "send", "--user", "123", "--text", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 2 || caller.calls[1].toolName != "send_personal_message" {
		t.Fatalf("calls = %#v", caller.calls)
	}
	if got := caller.calls[1].args["receiverOpenDingTalkId"]; got != "open-123" {
		t.Fatalf("receiverOpenDingTalkId = %#v, args = %#v", got, caller.calls[1].args)
	}
	if _, leaked := caller.calls[1].args["receiverUid"]; leaked {
		t.Fatalf("resolved send must not include receiverUid: %#v", caller.calls[1].args)
	}
}

func TestChatSendAndReplyDefaultToAgentProductForIMClawType(t *testing.T) {
	t.Setenv(agentproduct.EnvName, "qwenwork")

	tests := []struct {
		name string
		args []string
	}{
		{
			name: "send",
			args: []string{"message", "send", "--open-dingtalk-id", helperCurrentDOpenID, "--text", "hello"},
		},
		{
			name: "reply",
			args: []string{"message", "reply", "--conversation-id", "cid", "--ref-msg-id", "mid", "--ref-sender", helperCurrentDOpenID, "--text", "hello"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			caller := &chatChangedContractCaller{}
			if err := executeChatChangedContract(t, caller, tc.args...); err != nil {
				t.Fatal(err)
			}
			sendCalls := chatCallsByTool(caller.calls, "send_personal_message")
			if len(sendCalls) != 1 {
				t.Fatalf("calls = %#v", caller.calls)
			}
			if got := sendCalls[0].args["clawType"]; got != "qwenwork" {
				t.Fatalf("clawType = %#v, want qwenwork", got)
			}
		})
	}
}

func TestChatSendAndReplyDisableAITagWithEmptyClawType(t *testing.T) {
	t.Setenv(agentproduct.EnvName, "qwenwork")

	tests := []struct {
		name string
		args []string
	}{
		{
			name: "send",
			args: []string{"message", "send", "--open-dingtalk-id", helperCurrentDOpenID, "--text", "hello", "--ai-tag=false"},
		},
		{
			name: "reply",
			args: []string{"message", "reply", "--conversation-id", "cid", "--ref-msg-id", "mid", "--ref-sender", helperCurrentDOpenID, "--text", "hello", "--ai-tag=false"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			caller := &chatChangedContractCaller{}
			if err := executeChatChangedContract(t, caller, tc.args...); err != nil {
				t.Fatal(err)
			}
			sendCalls := chatCallsByTool(caller.calls, "send_personal_message")
			if len(sendCalls) != 1 {
				t.Fatalf("calls = %#v", caller.calls)
			}
			got, present := sendCalls[0].args["clawType"]
			if !present || got != "" {
				t.Fatalf("clawType = %#v, present = %v; want present empty string", got, present)
			}
		})
	}
}

func TestCrossPlatformCoverageChatCurrentUserSendAndReplyMentions(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		contentField string
		wantContent  string
		wantAtAll    bool
		wantOpenIDs  []string
	}{
		{
			name: "send",
			args: []string{
				"message", "send", "--group", "cid",
				"--text", "收到 @" + helperCurrentDOpenID + " 和 <@" + helperCurrentDOpenID2 + ">",
				"--at-open-dingtalk-ids", helperCurrentDOpenID + "," + helperCurrentDOpenID2,
				"--at-all",
			},
			contentField: "text",
			wantContent:  "<@all> 收到 <@" + helperCurrentDOpenID + "> 和 <@" + helperCurrentDOpenID2 + ">",
			wantAtAll:    true,
			wantOpenIDs:  []string{helperCurrentDOpenID, helperCurrentDOpenID2},
		},
		{
			name: "send keeps missing member placeholders unchanged",
			args: []string{
				"message", "send", "--group", "cid",
				"--text", "DWS 发消息自测",
				"--at-open-dingtalk-ids", helperCurrentDOpenID,
			},
			contentField: "text",
			wantContent:  "DWS 发消息自测",
			wantOpenIDs:  []string{helperCurrentDOpenID},
		},
		{
			name: "reply",
			args: []string{
				"message", "reply",
				"--conversation-id", "cid",
				"--ref-msg-id", "mid",
				"--ref-sender", helperCurrentDOpenID,
				"--text", "收到 @" + helperCurrentDOpenID + " 和 <@" + helperCurrentDOpenID2 + ">",
				"--at-open-dingtalk-ids", helperCurrentDOpenID + "," + helperCurrentDOpenID2,
				"--at-all",
			},
			contentField: "content",
			wantContent:  "<@all> 收到 <@" + helperCurrentDOpenID + "> 和 <@" + helperCurrentDOpenID2 + ">",
			wantAtAll:    true,
			wantOpenIDs:  []string{helperCurrentDOpenID, helperCurrentDOpenID2},
		},
		{
			name: "reply adds missing member placeholders",
			args: []string{
				"message", "reply",
				"--conversation-id", "cid",
				"--ref-msg-id", "mid",
				"--ref-sender", helperCurrentDOpenID,
				"--text", "DWS synthetic reply mention test",
				"--at-open-dingtalk-ids", helperCurrentDOpenID + "," + helperCurrentDOpenID2 + "," + helperCurrentDOpenID,
			},
			contentField: "content",
			wantContent:  "<@" + helperCurrentDOpenID + "> <@" + helperCurrentDOpenID2 + "> DWS synthetic reply mention test",
			wantOpenIDs:  []string{helperCurrentDOpenID, helperCurrentDOpenID2, helperCurrentDOpenID},
		},
		{
			name: "reply adds missing member placeholders after at-all",
			args: []string{
				"message", "reply",
				"--conversation-id", "cid",
				"--ref-msg-id", "mid",
				"--ref-sender", helperCurrentDOpenID,
				"--text", "请大家确认",
				"--at-open-dingtalk-ids", helperCurrentDOpenID,
				"--at-all",
			},
			contentField: "content",
			wantContent:  "<@all> <@" + helperCurrentDOpenID + "> 请大家确认",
			wantAtAll:    true,
			wantOpenIDs:  []string{helperCurrentDOpenID},
		},
		{
			name: "reply at-all preserves alliance word",
			args: []string{
				"message", "reply",
				"--conversation-id", "cid",
				"--ref-msg-id", "mid",
				"--ref-sender", helperCurrentDOpenID,
				"--text", "联系 @alliance",
				"--at-all",
			},
			contentField: "content",
			wantContent:  "<@all> 联系 @alliance",
			wantAtAll:    true,
		},
		{
			name: "reply without at flags preserves alliance word",
			args: []string{
				"message", "reply",
				"--conversation-id", "cid",
				"--ref-msg-id", "mid",
				"--ref-sender", helperCurrentDOpenID,
				"--text", "联系 @alliance",
			},
			contentField: "content",
			wantContent:  "联系 @alliance",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			caller := &chatChangedContractCaller{}
			if err := executeChatChangedContract(t, caller, tc.args...); err != nil {
				t.Fatal(err)
			}
			sendCalls := chatCallsByTool(caller.calls, "send_personal_message")
			if len(sendCalls) != 1 {
				t.Fatalf("calls = %#v", caller.calls)
			}
			args := sendCalls[0].args
			gotAtAll, hasAtAll := args["atAll"]
			if tc.wantAtAll {
				if !hasAtAll || gotAtAll != true {
					t.Fatalf("atAll = %#v, present = %v; want true", gotAtAll, hasAtAll)
				}
			} else if hasAtAll {
				t.Fatalf("atAll = %#v; want absent", gotAtAll)
			}
			gotOpenIDs, hasOpenIDs := args["atOpenDingTalkIds"]
			if len(tc.wantOpenIDs) > 0 {
				if !hasOpenIDs || !reflect.DeepEqual(gotOpenIDs, tc.wantOpenIDs) {
					t.Fatalf("atOpenDingTalkIds = %#v, present = %v; want %#v", gotOpenIDs, hasOpenIDs, tc.wantOpenIDs)
				}
			} else if hasOpenIDs {
				t.Fatalf("atOpenDingTalkIds = %#v; want absent", gotOpenIDs)
			}
			var content map[string]string
			if err := json.Unmarshal([]byte(args["content"].(string)), &content); err != nil {
				t.Fatal(err)
			}
			if got := content[tc.contentField]; got != tc.wantContent {
				t.Fatalf("content[%q] = %q; want %q", tc.contentField, got, tc.wantContent)
			}
		})
	}
}

func TestCrossPlatformCoverageChatSendFailsClosedWhenUserCannotResolve(t *testing.T) {
	caller := &chatChangedContractCaller{}
	err := executeChatChangedContract(t, caller, "message", "send", "--user", "123", "--text", "hello")
	if err == nil || !strings.Contains(err.Error(), "pass --open-dingtalk-id instead") {
		t.Fatalf("error = %v, want explicit resolution failure", err)
	}
	for _, call := range caller.calls {
		if call.toolName == "send_personal_message" {
			t.Fatalf("unresolved user must not be dispatched: %#v", caller.calls)
		}
	}
}
