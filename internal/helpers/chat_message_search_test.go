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

func TestChatMessageSearchUsesMCPContracts(t *testing.T) {
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
