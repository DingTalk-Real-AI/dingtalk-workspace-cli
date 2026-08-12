package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"reflect"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type chatMessagePaginationCaller struct {
	steps []scriptedToolStep
	calls []pagedCommandCall
}

func (c *chatMessagePaginationCaller) CallTool(_ context.Context, serverID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	copied := map[string]any{}
	for k, v := range args {
		copied[k] = v
	}
	c.calls = append(c.calls, pagedCommandCall{server: serverID, tool: toolName, args: copied})
	if len(c.steps) == 0 {
		return textToolResult(`{"result":{"messages":[],"items":[],"hasMore":false,"nextCursor":"0"}}`), nil
	}
	step := c.steps[len(c.calls)-1]
	if step.err != nil {
		return nil, step.err
	}
	return textToolResult(step.text), nil
}

func (*chatMessagePaginationCaller) Format() string { return "json" }
func (*chatMessagePaginationCaller) DryRun() bool   { return false }
func (*chatMessagePaginationCaller) Fields() string { return "" }
func (*chatMessagePaginationCaller) JQ() string     { return "" }

func executeChatMessagePaginationCommand(t *testing.T, caller *chatMessagePaginationCaller, args ...string) (map[string]any, error) {
	t.Helper()
	oldDeps := deps
	oldSleep := helperSleep
	t.Cleanup(func() {
		deps = oldDeps
		helperSleep = oldSleep
	})
	InitDeps(caller)
	out := &bytes.Buffer{}
	deps.Out.w = out
	deps.Out.errW = io.Discard
	helperSleep = func(d time.Duration) {}

	root := newChatCommand()
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	err := root.ExecuteContext(context.Background())
	if out.Len() == 0 {
		return nil, err
	}
	var parsed map[string]any
	if unmarshalErr := json.Unmarshal(out.Bytes(), &parsed); unmarshalErr != nil {
		t.Fatalf("stdout JSON = %q, err = %v", out.String(), unmarshalErr)
	}
	return parsed, err
}

func TestChatMessagePaginationDefaultSinglePageUnchanged(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		server string
		tool   string
		want   map[string]any
	}{
		{
			name:   "list-all",
			args:   []string{"message", "list-all", "--start", "2026-08-01 00:00:00", "--end", "2026-08-02 00:00:00"},
			server: "",
			tool:   "search_messages_by_time_range",
			want:   map[string]any{"startTime": "2026-08-01 00:00:00", "endTime": "2026-08-02 00:00:00", "limit": 50, "cursor": "0"},
		},
		{
			name:   "list-by-sender",
			args:   []string{"message", "list-by-sender", "--sender-user-id", "u1", "--start", "2026-08-01T00:00:00+08:00", "--end", "2026-08-02T00:00:00+08:00"},
			server: "",
			tool:   "search_messages_by_sender",
			want:   map[string]any{"senderUserId": "u1", "startTime": float64(1785513600000), "endTime": float64(1785600000000), "limit": 50, "cursor": "0"},
		},
		{
			name:   "list-mentions",
			args:   []string{"message", "list-mentions", "--group", "cid1", "--start", "2026-08-01T00:00:00+08:00", "--end", "2026-08-02T00:00:00+08:00"},
			server: "",
			tool:   "search_at_me_message",
			want:   map[string]any{"openConversationId": "cid1", "startTime": float64(1785513600000), "endTime": float64(1785600000000), "limit": 50, "cursor": "0"},
		},
		{
			name:   "list-focused",
			args:   []string{"message", "list-focused"},
			server: "",
			tool:   "list_special_focus_messages",
			want:   map[string]any{"limit": 50},
		},
		{
			name:   "search",
			args:   []string{"message", "search", "--query", "发布", "--start", "2026-08-01T00:00:00+08:00", "--end", "2026-08-02T00:00:00+08:00"},
			server: "chat",
			tool:   "search_messages_by_keyword",
			want:   map[string]any{"keyword": "发布", "startTime": float64(1785513600000), "endTime": float64(1785600000000), "limit": 100, "cursor": "0"},
		},
		{
			name:   "search-advanced",
			args:   []string{"message", "search-advanced", "--query", "周报", "--start", "2026-08-01T00:00:00+08:00", "--end", "2026-08-02T00:00:00+08:00"},
			server: "im",
			tool:   "search_messages",
			want:   map[string]any{"keyword": "周报", "startTime": float64(1785513600000), "endTime": float64(1785600000000), "limit": 100, "cursor": "0"},
		},
		{
			name:   "list-favorites",
			args:   []string{"message", "list-favorites"},
			server: "im",
			tool:   "list_message_favorites",
			want:   map[string]any{"cursor": int64(0), "size": "20"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caller := &chatMessagePaginationCaller{}
			args := append([]string{}, tt.args...)
			args = append(args, "--page-limit", "2", "--max-items", "1", "--page-delay", "0")
			_, err := executeChatMessagePaginationCommand(t, caller, args...)
			if err != nil {
				t.Fatal(err)
			}
			if len(caller.calls) != 1 {
				t.Fatalf("calls = %#v, want one fallback call", caller.calls)
			}
			got := caller.calls[0]
			if got.server != tt.server || got.tool != tt.tool || !argsEqual(got.args, tt.want) {
				t.Fatalf("call = %#v, want server=%s tool=%s args=%#v", got, tt.server, tt.tool, tt.want)
			}
		})
	}
}

func TestChatMessagePaginationPageAllAggregatesSevenCommands(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		server    string
		tool      string
		itemPath  string
		cursorOne any
		cursorTwo any
		pageOne   string
		pageTwo   string
	}{
		{
			name: "list-all", args: []string{"message", "list-all", "--start", "2026-08-01 00:00:00", "--end", "2026-08-02 00:00:00"},
			server: "chat", tool: "search_messages_by_time_range", itemPath: "conversationMessagesList", cursorOne: "0", cursorTwo: "c2",
			pageOne: `{"result":{"conversationMessagesList":[{"openConversationId":"cid1","title":"群1","messages":[{"id":"m1"}]}],"hasMore":true,"nextCursor":"c2"}}`,
			pageTwo: `{"result":{"conversationMessagesList":[{"openConversationId":"cid1","title":"群1","messages":[{"id":"m2"}]}],"hasMore":false,"nextCursor":""}}`,
		},
		{
			name: "list-by-sender", args: []string{"message", "list-by-sender", "--sender-user-id", "u1", "--start", "2026-08-01T00:00:00+08:00", "--end", "2026-08-02T00:00:00+08:00"},
			server: "chat", tool: "search_messages_by_sender", itemPath: "conversationMessagesList", cursorOne: "0", cursorTwo: "c2",
			pageOne: `{"result":{"conversationMessagesList":[{"openConversationId":"cid1","title":"洄川","messages":[{"id":"m1"}]}],"hasMore":true,"nextCursor":"c2"}}`,
			pageTwo: `{"result":{"conversationMessagesList":[{"openConversationId":"cid1","title":"洄川","messages":[{"id":"m2"}]}],"hasMore":false,"nextCursor":""}}`,
		},
		{
			name: "list-mentions", args: []string{"message", "list-mentions", "--start", "2026-08-01T00:00:00+08:00", "--end", "2026-08-02T00:00:00+08:00"},
			server: "chat", tool: "search_at_me_message", itemPath: "conversationMessagesList", cursorOne: "0", cursorTwo: "c2",
			pageOne: `{"result":{"conversationMessagesList":[{"openConversationId":"cid1","title":"群1","messages":[{"id":"m1"}]}],"hasMore":true,"nextCursor":"c2"}}`,
			pageTwo: `{"result":{"conversationMessagesList":[{"openConversationId":"cid1","title":"群1","messages":[{"id":"m2"}]}],"hasMore":false,"nextCursor":""}}`,
		},
		{
			name: "list-focused", args: []string{"message", "list-focused"},
			server: "chat", tool: "list_special_focus_messages", itemPath: "messages", cursorOne: nil, cursorTwo: int64(2),
			pageOne: `{"result":{"messages":[{"id":"m1"}],"hasMore":true,"nextCursor":2}}`,
			pageTwo: `{"result":{"messages":[{"id":"m2"}],"hasMore":false,"nextCursor":0}}`,
		},
		{
			name: "search", args: []string{"message", "search", "--query", "发布", "--start", "2026-08-01T00:00:00+08:00", "--end", "2026-08-02T00:00:00+08:00"},
			server: "chat", tool: "search_messages_by_keyword", itemPath: "conversationMessagesList", cursorOne: "0", cursorTwo: "c2",
			pageOne: `{"result":{"conversationMessagesList":[{"openConversationId":"cid1","title":"群1","messages":[{"id":"m1"}]}],"hasMore":true,"nextCursor":"c2"}}`,
			pageTwo: `{"result":{"conversationMessagesList":[{"openConversationId":"cid1","title":"群1","messages":[{"id":"m2"}]}],"hasMore":false,"nextCursor":""}}`,
		},
		{
			name: "search-advanced", args: []string{"message", "search-advanced", "--query", "周报"},
			server: "im", tool: "search_messages", itemPath: "conversationMessagesList", cursorOne: "0", cursorTwo: "c2",
			pageOne: `{"result":{"conversationMessagesList":[{"openConversationId":"cid1","title":"群1","messages":[{"id":"m1"}]}],"hasMore":true,"nextCursor":"c2"}}`,
			pageTwo: `{"result":{"conversationMessagesList":[{"openConversationId":"cid1","title":"群1","messages":[{"id":"m2"}]}],"hasMore":false,"nextCursor":""}}`,
		},
		{
			name: "list-favorites", args: []string{"message", "list-favorites"},
			server: "im", tool: "list_message_favorites", itemPath: "items", cursorOne: int64(0), cursorTwo: int64(20),
			pageOne: `{"result":{"items":[{"id":"f1"}],"hasMore":true,"nextCursor":20}}`,
			pageTwo: `{"result":{"items":[{"id":"f2"}],"hasMore":false,"nextCursor":0}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caller := &chatMessagePaginationCaller{steps: []scriptedToolStep{{text: tt.pageOne}, {text: tt.pageTwo}}}
			args := append([]string{}, tt.args...)
			args = append(args, "--page-all", "--page-delay", "0")
			got, err := executeChatMessagePaginationCommand(t, caller, args...)
			if err != nil {
				t.Fatal(err)
			}
			items := got["result"].(map[string]any)[tt.itemPath].([]any)
			if tt.itemPath == "conversationMessagesList" {
				messages := items[0].(map[string]any)["messages"].([]any)
				if len(items) != 1 || len(messages) != 2 {
					t.Fatalf("conversation items = %#v", items)
				}
				if tt.name == "search" {
					projected, ok := got["messages"].([]any)
					if !ok || len(projected) != 2 {
						t.Fatalf("projected messages = %#v", got["messages"])
					}
					for i, wantID := range []string{"m1", "m2"} {
						message, ok := projected[i].(map[string]any)
						if !ok || message["messageId"] != wantID || message["openMessageId"] != wantID {
							t.Fatalf("projected message %d = %#v", i, projected[i])
						}
						if _, exists := message["text"]; !exists {
							t.Fatalf("projected message %d missing text: %#v", i, message)
						}
					}
				}
			} else if len(items) != 2 {
				t.Fatalf("items = %#v", items)
			}
			if len(caller.calls) != 2 {
				t.Fatalf("calls = %#v, want two pages", caller.calls)
			}
			if caller.calls[0].server != tt.server || caller.calls[0].tool != tt.tool {
				t.Fatalf("first call = %#v", caller.calls[0])
			}
			if !reflect.DeepEqual(caller.calls[0].args["cursor"], tt.cursorOne) {
				t.Fatalf("first cursor = %#v, want %#v", caller.calls[0].args["cursor"], tt.cursorOne)
			}
			if !reflect.DeepEqual(caller.calls[1].args["cursor"], tt.cursorTwo) {
				t.Fatalf("second cursor = %#v, want %#v", caller.calls[1].args["cursor"], tt.cursorTwo)
			}
			paging := got["paging"].(map[string]any)
			if paging["pages"].(float64) != 2 || paging["total"].(float64) != 2 || paging["truncated"] != false {
				t.Fatalf("paging = %#v", paging)
			}
		})
	}
}

func argsEqual(got, want map[string]any) bool {
	if len(got) != len(want) {
		return false
	}
	for key, wantValue := range want {
		gotValue, ok := got[key]
		if !ok {
			return false
		}
		switch w := wantValue.(type) {
		case float64:
			g, ok := gotValue.(int64)
			if !ok || float64(g) != w {
				return false
			}
		default:
			if !reflect.DeepEqual(gotValue, wantValue) {
				return false
			}
		}
	}
	return true
}
