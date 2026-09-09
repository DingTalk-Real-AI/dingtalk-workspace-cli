package smart

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"testing"
	"time"
)

func TestCrossPlatformCoverageOptimizationStrictSearchEvidence(t *testing.T) {
	for _, data := range []map[string]any{{"conversationMessagesList": 1}, {"conversationMessagesList": []any{map[string]any{"messages": []any{map[string]any{}}}}}, {"messages": []any{map[string]any{}}}} {
		if _, err := verifiedSearchItems(data); err == nil {
			t.Fatal("shape/identity not checked")
		}
	}
	rows, err := verifiedSearchItems(map[string]any{"conversationMessagesList": []any{map[string]any{"title": "fixture", "openConversationId": "cid", "messages": []any{map[string]any{"openMessageId": "m"}}}}})
	if err != nil || rows[0]["conversationName"] != "fixture" || rows[0]["openConversationId"] != "cid" {
		t.Fatal(rows, err)
	}
	root := newPlatformCoverageRoot()
	cmd, _, _ := root.Find([]string{"chat", "+search-msg"})
	rt := shortcut.RuntimeContextForTest(cmd, SearchMsg)
	file := map[string]any{"resources": []any{map[string]any{"resourceId": "f", "resourceIdType": "fileId", "resourceType": "file"}}}
	result, err := verifySearchResultFilters(rt, map[string]any{"messageType": "file"}, []map[string]any{file})
	if err != nil || len(result) != 1 {
		t.Fatal(result, err)
	}
	if _, err := verifySearchResultFilters(rt, map[string]any{"searchConvType": "group_chat"}, []map[string]any{{}}); err == nil {
		t.Fatal("unknown group type accepted")
	}
}
func TestCrossPlatformCoverageOptimizationGroupCreatedMillis(t *testing.T) {
	f := &smartCoverageCaller{responses: map[string][]string{"chat/get_conversation_info": {`{"result":{"openConversationId":"cid","createAt":1780000000123}}`}}}
	helpers.InitDeps(f)
	root := newPlatformCoverageRoot()
	cmd, _, _ := root.Find([]string{"chat", "+chat-messages"})
	cmd.SetContext(context.Background())
	got, err := verifiedGroupCreationTime(shortcut.RuntimeContextForTest(cmd, ChatMessages), "cid")
	if err != nil || got.UnixMilli() != 1780000000123 {
		t.Fatal(got, err)
	}
}
func TestCrossPlatformCoverageOptimizationHistoryCursorErrors(t *testing.T) {
	root := newPlatformCoverageRoot()
	cmd, _, _ := root.Find([]string{"chat", "+chat-messages"})
	rt := shortcut.RuntimeContextForTest(cmd, ChatMessages)
	end := time.UnixMilli(1780000000123)
	r := chatMessagesRequest{tool: "tool", params: map[string]any{"openconversation_id": "cid"}, direction: "older", timeRange: chatMessageTimeRange{end: &end}}
	attachHistoryCursor(rt, r, nil)
	attachHistoryCursor(rt, r, map[string]any{"nextPage": map[string]any{}})
	for _, mode := range []string{"json", "time", "end"} {
		c := historyBinding(rt, r)
		c.Time = end.Format(time.RFC3339Nano)
		if mode == "time" {
			c.Time = "bad"
		}
		if mode == "end" {
			later := end.Add(time.Hour)
			c.End = &later
			_ = cmd.Flags().Set("end", end.Format(time.RFC3339Nano))
		}
		data, _ := json.Marshal(c)
		if mode == "json" {
			data = []byte("[]")
		}
		_ = cmd.Flags().Set("page-token", "dws-chat-v1."+base64.RawURLEncoding.EncodeToString(data))
		if restoreHistoryCursor(rt, &r) == nil {
			t.Fatal("invalid token accepted", mode)
		}
	}
}
