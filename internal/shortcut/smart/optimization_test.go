package smart

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type optimizationSearchCaller struct {
	searchMsgExecutionCaller
	reactionCalls   int
	missingReaction bool
}

func (f *optimizationSearchCaller) CallTool(ctx context.Context, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	if tool == "list_message_emotion_replies" {
		f.reactionCalls++
		ids, _ := args["openMessageIds"].([]string)
		rows := []map[string]any{}
		if !f.missingReaction {
			for _, id := range ids {
				rows = append(rows, map[string]any{"openMessageId": id, "emotionReplyList": []any{}})
			}
		}
		data, _ := json.Marshal(map[string]any{"result": rows})
		return searchMsgToolResult(string(data)), nil
	}
	return f.searchMsgExecutionCaller.CallTool(ctx, product, tool, args)
}
func (f *optimizationSearchCaller) CallReadTool(ctx context.Context, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	return f.CallTool(ctx, product, tool, args)
}

func TestCrossPlatformCoverageOptimizationSearchDefaultReactionAndMissingRows(t *testing.T) {
	for _, missing := range []bool{false, true} {
		f := &optimizationSearchCaller{missingReaction: missing}
		helpers.InitDeps(f)
		root := newPlatformCoverageRoot()
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetArgs([]string{"chat", "+messages-search", "--query", "fixture"})
		if err := root.Execute(); (err != nil) != missing {
			t.Fatalf("missing=%v err=%v", missing, err)
		}
		var p map[string]any
		if e := json.Unmarshal(out.Bytes(), &p); e != nil {
			t.Fatal(e)
		}
		if f.reactionCalls != 1 {
			t.Fatalf("default did not enrich: %d", f.reactionCalls)
		}
		if missing && p["complete"] != false {
			t.Fatal("missing reaction was reported complete")
		}
	}
}

func TestCrossPlatformCoverageOptimizationSearchThreadContainerPreservesOwner(t *testing.T) {
	d := map[string]any{"conversationMessagesList": []any{map[string]any{"openConversationId": "thread-cid", "singleChat": false, "messages": []any{map[string]any{"openMessageId": "m", "openConversationId": "parent-cid"}}}}}
	rows, err := verifiedSearchItems(d)
	if err != nil || len(rows) != 1 {
		t.Fatalf("%v %#v", err, rows)
	}
	if rows[0]["openConversationId"] != "parent-cid" || rows[0]["searchContainerId"] != "thread-cid" {
		t.Fatal(rows)
	}
	for _, d := range []map[string]any{{"hasMore": false}, {"conversationMessagesList": []any{map[string]any{"messages": []any{1}}}}} {
		if _, err := verifiedSearchItems(d); err == nil {
			t.Fatalf("shape accepted %#v", d)
		}
	}
}

func TestCrossPlatformCoverageOptimizationSearchTimeAndAliasMapping(t *testing.T) {
	cases := [][]string{{"--query", "fixture", "--start", "2026-09-01T00:00:00+08:00"}, {"--end", "2026-09-08T00:00:00+08:00"}, {"--query", "fixture", "--all-time"}, {"--query", "fixture", "--chat-type", "p2p"}, {"--query", "fixture", "--at-chatter-ids", testCurrentDOpenID}}
	for _, args := range cases {
		f := &searchMsgExecutionCaller{searchResponse: `{"result":{"messages":[],"hasMore":false}}`}
		_, err := executeSearchMsgResult(f, args...)
		if err != nil {
			t.Fatal(args, err)
		}
		call := f.calls[0]
		if call.tool != "search_messages" {
			t.Fatal(call)
		}
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "--all-time") {
			if _, ok := call.args["startTime"]; ok {
				t.Fatal(call.args)
			}
		}
		if strings.Contains(joined, "--chat-type") {
			if call.args["searchConvType"] != "single_chat" {
				t.Fatal(call.args)
			}
		}
		if strings.Contains(joined, "--at-chatter-ids") {
			if len(call.args["atOpenDingTakIds"].([]string)) != 1 {
				t.Fatal(call.args)
			}
		}
	}
}

func TestCrossPlatformCoverageOptimizationUnsupportedMessageTypeStopsBeforeRead(t *testing.T) {
	for _, kind := range []string{"text", "image", "unknown"} {
		f := &searchMsgExecutionCaller{}
		_, err := executeSearchMsgResult(f, "--message-type", kind)
		if err == nil || len(f.calls) != 0 {
			t.Fatalf("%s: %v %#v", kind, err, f.calls)
		}
	}
}

func TestCrossPlatformCoverageOptimizationHistoryCursorBinding(t *testing.T) {
	root := newPlatformCoverageRoot()
	cmd, _, err := root.Find([]string{"chat", "+chat-messages"})
	if err != nil {
		t.Fatal(err)
	}
	rt := shortcut.RuntimeContextForTest(cmd, ChatMessages)
	start := time.UnixMilli(1780000000123)
	end := start.Add(time.Hour)
	r := chatMessagesRequest{tool: "list_conversation_message_v2", params: map[string]any{"openconversation_id": "cid"}, direction: "newer", timeRange: chatMessageTimeRange{configured: true, start: &start, end: &end, order: "asc"}}
	p := map[string]any{"nextPage": map[string]any{"time": start.Add(time.Minute).UTC().Format(time.RFC3339Nano)}}
	attachHistoryCursor(rt, r, p)
	token, _ := p["nextPageToken"].(string)
	if token == "" {
		t.Fatal(p)
	}
	if err := cmd.Flags().Set("page-token", token); err != nil {
		t.Fatal(err)
	}
	if err := restoreHistoryCursor(rt, &r); err != nil {
		t.Fatal(err)
	}
	if r.params["time"] != start.Add(time.Minute).UTC().Format(time.RFC3339Nano) {
		t.Fatal(r.params)
	}
	r.params["openconversation_id"] = "other"
	if restoreHistoryCursor(rt, &r) == nil {
		t.Fatal("cross-conversation token accepted")
	}
	r.params["openconversation_id"] = "cid"
	r.direction = "older"
	if restoreHistoryCursor(rt, &r) == nil {
		t.Fatal("cross-direction token accepted")
	}
	for _, bad := range []string{"lark-token", "dws-chat-v1.!", "dws-chat-v1.e30"} {
		_ = cmd.Flags().Set("page-token", bad)
		if restoreHistoryCursor(rt, &r) == nil {
			t.Fatal("bad token accepted")
		}
	}
}

func TestCrossPlatformCoverageReactionStreamKeepsExtendedSearchPredicates(t *testing.T) {
	for _, tc := range []struct {
		name     string
		args     []string
		eligible bool
	}{
		{name: "default bounded", eligible: true},
		{name: "with threads", args: []string{"--with-threads"}},
		{name: "all time", args: []string{"--all-time"}},
		{name: "start only", args: []string{"--start", "2026-09-01T00:00:00Z"}},
		{name: "end only", args: []string{"--end", "2026-09-08T00:00:00Z"}},
		{name: "paired boundaries", args: []string{"--start", "2026-09-01T00:00:00Z", "--end", "2026-09-08T00:00:00Z"}, eligible: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := newPlatformCoverageRoot()
			cmd, _, err := root.Find([]string{"chat", "+search-msg"})
			if err != nil {
				t.Fatal(err)
			}
			if err := cmd.ParseFlags(append([]string{"--has-reactions", "--page-all", "--group", "cid"}, tc.args...)); err != nil {
				t.Fatal(err)
			}
			rt := shortcut.RuntimeContextForTest(cmd, SearchMsg)
			if got := scopedConversationReactionStreamEligible(rt); got != tc.eligible {
				t.Fatalf("eligible=%v want %v", got, tc.eligible)
			}
		})
	}
}
