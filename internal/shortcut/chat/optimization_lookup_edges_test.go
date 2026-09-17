package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"testing"
)

func TestCrossPlatformCoverageOptimizationExactCategoryAndReadStatus(t *testing.T) {
	for _, tc := range []struct {
		name      string
		flags     map[string]string
		responses map[string]string
		fail      string
		wantErr   bool
	}{
		{"category-known-absent", map[string]string{"category-id": "1", "feed-id": "missing", "no-detail": "true"}, map[string]string{"im/list_conversations_by_category": `{"result":{"conversations":[],"hasMore":false}}`, "im/get_conv_categories_info": `{"result":{"categories":[{"categoryId":1}]}}`}, "", false},
		{"category-unknown", map[string]string{"category-id": "1", "feed-id": "missing", "no-detail": "true"}, map[string]string{"im/list_conversations_by_category": `{"result":{"conversations":[],"hasMore":false}}`, "im/get_conv_categories_info": `{"result":{}}`}, "", true},
		{"category-reverse-positive", map[string]string{"category-id": "1", "feed-id": "cid", "no-detail": "true"}, map[string]string{"im/list_conversations_by_category": `{"result":{"conversations":[]}}`, "im/list_conv_categories_by_conv": `{"result":{"categories":[{"categoryId":1}]}}`}, "", false},
		{"category-reverse-error", map[string]string{"category-id": "1", "feed-id": "cid", "no-detail": "true"}, map[string]string{"im/list_conversations_by_category": `{"result":{"conversations":[]}}`}, "im/list_conv_categories_by_conv", true},
		{"category-no-id", map[string]string{"category-id": "1", "feed-id": "cid"}, map[string]string{"im/list_conversations_by_category": `{"result":{"conversations":[{}]}}`}, "", true},
		{"category-shape", map[string]string{"category-id": "1", "feed-id": "cid"}, map[string]string{"im/list_conversations_by_category": `{"result":{}}`}, "", true},
		{"category-invalid-pagination", map[string]string{"category-id": "1", "feed-id": "cid"}, map[string]string{"im/list_conversations_by_category": `{"result":{"conversations":[{"openConversationId":"cid"}],"hasMore":"false"}}`}, "", true},
		{"category-read-error", map[string]string{"category-id": "1", "feed-id": "cid"}, nil, "im/list_conversations_by_category", true},
		{"category-detail-error", map[string]string{"category-id": "1", "feed-id": "cid"}, map[string]string{"im/list_conversations_by_category": `{"result":{"conversations":[{"openConversationId":"cid"}]}}`}, "chat/get_conversation_info", true},
		{"category-empty-request", map[string]string{"category-id": "1"}, nil, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &larkAlignmentCaller{responses: tc.responses, failProductTool: tc.fail}
			rt := optimizationRuntime(t, f, FeedGroupQueryItem, tc.flags)
			err := executeExactFeedGroupQuery(rt)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v", err)
			}
			for _, c := range f.calls {
				if c.tool == "get_conv_categories_info" {
					v, ok := c.args["categoryIds"].([]int64)
					if !ok || len(v) != 1 || v[0] != 1 {
						t.Fatal("missing category identity", c.args)
					}
				}
			}
		})
	}
	source := `{"result":{"messages":[{"openMessageId":"m","openConversationId":"cid"}]}}`
	for _, tc := range []struct {
		name, users, response, fail string
		dry, hasUsers, err          bool
	}{
		{"user-id", "user-1,user-1", `{"result":{"memberReadStatusList":[{"openDingTalkId":"D-resolved"}]}}`, "", false, true, false},
		{"openid", optimizationOpenID, `{"result":{"memberReadStatusList":[{"openDingTalkId":"` + optimizationOpenID + `"}]}}`, "", false, true, false},
		{"foreign", optimizationOpenID, `{"result":{"memberReadStatusList":[{"openDingTalkId":"other"}]}}`, "", false, true, true},
		{"empty", "", "", "", false, true, true},
		{"resolve-error", "user-1", "", "contact/search_contact_by_key_word", false, true, true},
		{"status-error", "", "", "im/query_msg_read_status", false, false, true},
		{"dry", "user-1", "", "", true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &larkAlignmentCaller{dryRun: tc.dry, failProductTool: tc.fail, responses: map[string]string{"im/list_messages_by_ids": source, "im/query_msg_read_status": tc.response}}
			flags := map[string]string{"message-id": "m"}
			if tc.hasUsers {
				flags["users"] = tc.users
			}
			rt := optimizationRuntime(t, f, MessagesReadStatus, flags)
			err := executeOptimizedReadStatus(rt)
			if (err != nil) != tc.err {
				t.Fatalf("err=%v calls=%#v", err, f.calls)
			}
		})
	}
	f := &larkAlignmentCaller{responses: map[string]string{"im/list_messages_by_ids": source}}
	rt := optimizationRuntime(t, f, MessagesReadStatus, map[string]string{"message-id": "m"})
	if _, err := ReadExactMessage(rt, "cid", "m"); err != nil {
		t.Fatal(err)
	}
	rt.Command().SetOut(chatOutputErrorWriter{err: errors.New("fixture")})
	if OutputCheckedRead(rt, map[string]any{"complete": true}) == nil {
		t.Fatal("output error swallowed")
	}
	if _, err := StrictChatCollection(map[string]any{"items": []map[string]any{{"id": "a"}}}, "items"); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageOptimizationFavoriteEnrichmentFaults(t *testing.T) {
	for _, tc := range []struct {
		name     string
		rows     []map[string]any
		response string
		fail     string
		wantErr  bool
	}{
		{"missing-identity", []map[string]any{{}}, `{"result":{"messages":[]}}`, "", true},
		{"lookup-fail", []map[string]any{{"openMessageId": "m"}}, "", "im/list_messages_by_ids", true},
		{"lookup-shape", []map[string]any{{"openMessageId": "m"}}, `{"result":{}}`, "", true},
		{"wrong-conversation", []map[string]any{{"openMessageId": "m", "openConversationId": "cid"}}, `{"result":{"messages":[{"openMessageId":"m","openConversationId":"other"}]}}`, "", true},
		{"duplicate", []map[string]any{{"openMessageId": "m"}}, `{"result":{"messages":[{"openMessageId":"m"},{"openMessageId":"m"}]}}`, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &larkAlignmentCaller{responses: map[string]string{"im/list_messages_by_ids": tc.response}, failProductTool: tc.fail}
			rt := optimizationRuntime(t, f, FlagList, map[string]string{"no-reactions": "true"})
			fs := enrichFavoriteItems(rt, tc.rows)
			if (len(fs) > 0) != tc.wantErr {
				t.Fatal(fs)
			}
		})
	}
	// Two mget batches must share the same Thread budget.
	rows, raw := []map[string]any{}, []map[string]any{}
	for i := 0; i < 51; i++ {
		id := fmt.Sprintf("m%d", i)
		rows = append(rows, map[string]any{"openMessageId": id})
		raw = append(raw, map[string]any{"openMessageId": id, "openConversationId": "cid", "openConvThreadId": fmt.Sprintf("t%d", i)})
	}
	data, _ := json.Marshal(map[string]any{"result": map[string]any{"messages": raw}})
	f := &larkAlignmentCaller{responses: map[string]string{"im/list_messages_by_ids": string(data), "chat/list_topic_replies": `{"result":{"messages":[],"hasMore":false}}`}}
	rt := optimizationRuntime(t, f, FlagList, map[string]string{"with-threads": "true", "no-reactions": "true"})
	if fs := enrichFavoriteItems(rt, rows); len(fs) != 0 {
		t.Fatal(fs)
	}
	if f.callCounts["im/list_messages_by_ids"] != 2 || f.callCounts["chat/list_topic_replies"] != 50 || rows[50]["enrichmentComplete"] != false {
		t.Fatal("per-query budget or batch identity failed", f.callCounts)
	}
}
func TestCrossPlatformCoverageOptimizationDownloadIdentityRejections(t *testing.T) {
	data := `{"result":{"messages":[{"openMessageId":"m","openConversationId":"cid","resources":[{"resourceId":"r","resourceIdType":"fileId","resourceType":"file"}]}]}}`
	for _, kind := range []string{"image", "mediaId", "file"} {
		f := &larkAlignmentCaller{responses: map[string]string{"im/list_messages_by_ids": data}}
		rt := optimizationRuntime(t, f, MessagesResourceDownload, map[string]string{"resource-id": "r", "message-id": "m", "type": kind})
		_, _, _, err := resolveDownloadIdentity(rt)
		if (kind == "file") != (err == nil) {
			t.Fatalf("%s %v", kind, err)
		}
	}
}

func TestCrossPlatformCoverageOptimizationGroupProjectionFailures(t *testing.T) {
	for _, tc := range []struct{ name, profile, fail string }{{"missing-profile", `{"result":{}}`, ""}, {"resolve-failed", `{"result":{"userId":"u"}}`, "contact/search_contact_by_key_word"}, {"manager-absent", `{"result":{"userId":"u"}}`, ""}} {
		f := &larkAlignmentCaller{failProductTool: tc.fail, responses: map[string]string{"contact/get_current_user_profile": tc.profile, "im/list_group_member_by_ids": `{"result":{"members":[]}}`}}
		rt := optimizationRuntime(t, f, ChatSearch, map[string]string{"is-manager": "true"})
		rows, err := filterVerifiedGroups(rt, []map[string]any{{"openConversationId": "cid"}})
		if tc.name == "manager-absent" {
			if err != nil || len(rows) != 0 {
				t.Fatal(rows, err)
			}
		} else if err == nil {
			t.Fatal("unverified manager accepted")
		}
	}
	f := &larkAlignmentCaller{}
	rt := optimizationRuntime(t, f, ChatSearch, map[string]string{"member-ids": optimizationOpenID})
	if _, err := filterVerifiedGroups(rt, make([]map[string]any, 101)); err == nil || len(f.calls) != 0 {
		t.Fatal("unbounded member lookups")
	}
	for _, kind := range []string{"unknown", "active_time"} {
		if sortVerifiedGroups(nil, kind, false) == nil {
			t.Fatal("unknown sorting contract")
		}
	}
	for _, value := range []any{"2026-09-08 10:00:00", "2026-09-08T10:00:00+08:00", "bad"} {
		rows := []map[string]any{{"openConversationId": "b", "createAt": value}, {"openConversationId": "a", "createAt": value}}
		err := sortVerifiedGroups(rows, "create_time", false)
		if value == "bad" {
			if err == nil {
				t.Fatal("bad sort field accepted")
			}
		} else if err != nil || rows[0]["openConversationId"] != "a" {
			t.Fatal(rows, err)
		}
	}
	if _, err := editMessageContent(optimizationRuntime(t, f, newMessagesEdit(), map[string]string{"content": `{"text":"proof","title":"title"}`})); err != nil {
		t.Fatal(err)
	}
	rt = optimizationRuntime(t, f, ConversationListTop, nil)
	if attachConversationDetails(rt, map[string]any{}, []map[string]any{{}}) == nil {
		t.Fatal("detail lookup without identity")
	}
	f.failProductTool = "im/list_messages_by_ids"
	rt = optimizationRuntime(t, f, MessagesResourceDownload, map[string]string{"message-id": "m", "resource-id": "r"})
	if _, _, _, err := resolveDownloadIdentity(rt); err == nil {
		t.Fatal("lookup failure accepted")
	}
	f = &larkAlignmentCaller{responses: map[string]string{"im/list_messages_by_ids": `{"result":{"messages":[{"openMessageId":"m","openConversationId":"cid","resources":[{"resourceId":"r","resourceIdType":"fileId","resourceType":"file"},{"resourceId":"r","resourceIdType":"mediaId","resourceType":"image"}]}]}}`}}
	rt = optimizationRuntime(t, f, MessagesResourceDownload, map[string]string{"message-id": "m", "resource-id": "r"})
	if _, _, _, err := resolveDownloadIdentity(rt); err == nil {
		t.Fatal("ambiguous resource identity accepted")
	}
}

func TestCrossPlatformCoverageOptimizationUnknownResourceTypeStopsDownload(t *testing.T) {
	testseam.Swap(t, &messageResourceReferences, func(map[string]any) []map[string]any {
		return []map[string]any{{"resourceId": "r", "type": "future-type"}}
	})
	f := &larkAlignmentCaller{responses: map[string]string{"im/list_messages_by_ids": `{"result":{"messages":[{"openMessageId":"m","openConversationId":"cid"}]}}`}}
	rt := optimizationRuntime(t, f, MessagesResourceDownload, map[string]string{"message-id": "m", "resource-id": "r"})
	if _, _, _, err := resolveDownloadIdentity(rt); err == nil || len(f.calls) != 1 {
		t.Fatal("unknown ID type was downloaded")
	}
}
