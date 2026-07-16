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
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type chatMessageSearchCall struct {
	productID string
	toolName  string
	args      map[string]any
}

type chatMessageSearchCaller struct {
	calls []chatMessageSearchCall
}

func (c *chatMessageSearchCaller) CallTool(_ context.Context, productID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, chatMessageSearchCall{productID: productID, toolName: toolName, args: args})
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: `{}`}}}, nil
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
	}{
		{
			name:      "keyword search",
			args:      []string{"message", "search", "--query", "categoryName", "--group", "cid-1", "--start", start, "--end", end, "--limit", "100", "--cursor", "0"},
			productID: "chat",
			toolName:  "search_messages_by_keyword",
			wantToolArg: map[string]any{
				"keyword":            "categoryName",
				"openConversationId": "cid-1",
				"startTime":          startTime.UnixMilli(),
				"endTime":            endTime.UnixMilli(),
				"limit":              100,
				"cursor":             "0",
			},
		},
		{
			name:      "advanced search",
			args:      []string{"message", "search-advanced", "--query", "categoryName", "--conversation-ids", "cid-1,cid-2", "--start", start, "--end", end, "--limit", "100", "--cursor", "0"},
			productID: "im",
			toolName:  "search_messages",
			wantToolArg: map[string]any{
				"keyword":             "categoryName",
				"openConversationIds": []string{"cid-1", "cid-2"},
				"startTime":           startTime.UnixMilli(),
				"endTime":             endTime.UnixMilli(),
				"limit":               100,
				"cursor":              "0",
			},
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
			cmd.SetArgs(tt.args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("chat search returned error: %v", err)
			}
			if len(caller.calls) != 1 {
				t.Fatalf("tool call count = %d, want 1", len(caller.calls))
			}
			call := caller.calls[0]
			if call.productID != tt.productID || call.toolName != tt.toolName {
				t.Fatalf("tool call = %s/%s, want %s/%s", call.productID, call.toolName, tt.productID, tt.toolName)
			}
			if !reflect.DeepEqual(call.args, tt.wantToolArg) {
				t.Fatalf("tool args = %#v, want %#v", call.args, tt.wantToolArg)
			}
		})
	}
}

type chatChangedContractCaller struct {
	calls        []chatMessageSearchCall
	resolveUsers bool
}

func (c *chatChangedContractCaller) CallTool(_ context.Context, productID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, chatMessageSearchCall{productID: productID, toolName: toolName, args: args})
	text := `{}`
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
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs(args)
	return cmd.Execute()
}

func TestCrossPlatformCoverageChatMessageListUsesObservedGroupKeys(t *testing.T) {
	caller := &chatChangedContractCaller{}
	err := executeChatChangedContract(t, caller,
		"message", "list", "--group", "cid-1", "--time", "2026-07-15 09:00:00", "--limit", "50")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 || caller.calls[0].toolName != "list_conversation_message_v2" {
		t.Fatalf("calls = %#v", caller.calls)
	}
	want := map[string]any{"openCid": "cid-1", "cid": "cid-1", "time": "2026-07-15 09:00:00", "forward": true, "limit": 50}
	if !reflect.DeepEqual(caller.calls[0].args, want) {
		t.Fatalf("tool args = %#v, want %#v", caller.calls[0].args, want)
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
