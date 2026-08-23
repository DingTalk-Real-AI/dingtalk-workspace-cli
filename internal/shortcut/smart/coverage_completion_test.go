// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package smart

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/targetresolver"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type smartCoverageCaller struct {
	responses map[string][]string
	failAt    map[string]int
	counts    map[string]int
	arguments map[string][]map[string]any
}

func (c *smartCoverageCaller) CallTool(
	_ context.Context,
	product, tool string,
	args map[string]any,
) (*edition.ToolResult, error) {
	if c.counts == nil {
		c.counts = map[string]int{}
	}
	if c.arguments == nil {
		c.arguments = map[string][]map[string]any{}
	}
	key := product + "/" + tool
	c.counts[key]++
	c.arguments[key] = append(c.arguments[key], args)
	if c.failAt[key] == -1 || c.failAt[key] == c.counts[key] {
		return nil, errors.New("fixture failure")
	}
	responses := c.responses[key]
	text := `{"result":[]}`
	if len(responses) > 0 {
		index := c.counts[key] - 1
		if index >= len(responses) {
			index = len(responses) - 1
		}
		text = responses[index]
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: text}}}, nil
}

func (c *smartCoverageCaller) CallReadTool(ctx context.Context, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	return c.CallTool(ctx, product, tool, args)
}

func (*smartCoverageCaller) Format() string { return "json" }
func (*smartCoverageCaller) DryRun() bool   { return false }
func (*smartCoverageCaller) Fields() string { return "" }
func (*smartCoverageCaller) JQ() string     { return "" }

func TestCrossPlatformCoverageRichMessageProjections(t *testing.T) {
	message := map[string]any{
		"openMessageId":      "msg",
		"openConversationId": "cid",
		"threadId":           "thread",
		"msgType":            "text",
		"createTime":         "1",
		"updateTime":         "2",
		"content":            `{"mediaId":"@image"}`,
		"emotionReplyList":   []any{map[string]any{"emojiName": "赞", "replyCount": 1}},
		"quotedMessage": map[string]any{
			"openMessageId": "quoted",
			"content":       `{"mediaId":"@quoted-image"}`,
		},
	}
	for name, row := range map[string]map[string]any{
		"at-me":  atMeProject(message),
		"chat":   projectChatMessage(message),
		"search": searchMsgProject(message),
	} {
		for _, key := range []string{"threadId", "messageType", "updateTime", "quotedMessage", "resourceRefs"} {
			if _, ok := row[key]; !ok {
				t.Errorf("%s projection missing %s: %#v", name, key, row)
			}
		}
		resources := row["resourceRefs"].([]map[string]any)
		if len(resources) != 2 {
			t.Fatalf("%s projected resources = %#v", name, resources)
		}
		quotedArgs := resources[1]["download"].(map[string]any)["arguments"].(map[string]any)
		if resources[1]["resourceId"] != "@quoted-image" ||
			quotedArgs["message-id"] != "quoted" ||
			quotedArgs["open-conversation-id"] != "cid" {
			t.Fatalf("%s quoted resource context = %#v", name, resources[1])
		}
	}
}

func TestCrossPlatformCoverageChatMessagesOpenIDRoute(t *testing.T) {
	caller := &smartCoverageCaller{responses: map[string][]string{
		"chat/list_individual_chat_message": {`{"result":{"messages":[]}}`},
	}}
	helpers.InitDeps(caller)
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{"chat", "+chat-messages", "--open-dingtalk-id", testCurrentDOpenID, "--limit", "1", "--yes"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if caller.counts["chat/list_individual_chat_message"] != 1 {
		t.Fatalf("calls = %#v", caller.counts)
	}
}

func TestCrossPlatformCoverageChatMembersListOutcomes(t *testing.T) {
	successResponses := map[string][]string{
		"chat/get_group_members": {`{"result":{"hasMore":false,"list":[{"memberEmpName":"成员","openDingtalkId":"D-user"}]}}`},
		"bot/list_group_bots":    {`{"result":{"bots":[{"robotName":"机器人","robotCode":"robot"}]}}`},
	}
	cases := []struct {
		name      string
		memberArg string
		failAt    map[string]int
		wantError bool
	}{
		{name: "both success"},
		{name: "user only failure", memberArg: "user", failAt: map[string]int{"chat/get_group_members": 1}, wantError: true},
		{name: "bot only failure", memberArg: "bot", failAt: map[string]int{"bot/list_group_bots": 1}, wantError: true},
		{name: "both failure", failAt: map[string]int{"chat/get_group_members": 1, "bot/list_group_bots": 1}, wantError: true},
		{name: "partial user failure", failAt: map[string]int{"chat/get_group_members": 1}},
		{name: "partial bot failure", failAt: map[string]int{"bot/list_group_bots": 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caller := &smartCoverageCaller{responses: successResponses, failAt: tc.failAt}
			helpers.InitDeps(caller)
			root := newPlatformCoverageRoot()
			args := []string{"chat", "+chat-members-list", "--conversation-id", "cid"}
			if tc.memberArg != "" {
				args = append(args, "--member-types", tc.memberArg)
			}
			root.SetArgs(args)
			err := root.Execute()
			if (err != nil) != tc.wantError {
				t.Fatalf("error = %v, wantError=%v", err, tc.wantError)
			}
		})
	}

	for _, raw := range [][]string{{""}, {"unknown"}} {
		if _, _, err := resolveMemberTypes(raw); err == nil {
			t.Fatalf("resolveMemberTypes(%q) succeeded", raw)
		}
	}
	if users, bots, err := resolveMemberTypes(nil); err != nil || !users || !bots {
		t.Fatalf("default member types = %v %v %v", users, bots, err)
	}
}

func TestCrossPlatformCoverageChatMembersListPaginatesUserBucketAndDeduplicates(t *testing.T) {
	caller := &smartCoverageCaller{responses: map[string][]string{
		"chat/get_group_members": {
			`{"result":{"hasMore":true,"nextCursor":"2","list":[{"memberEmpName":"A","openDingtalkId":"D1"}]}}`,
			`{"result":{"hasMore":false,"list":[{"memberEmpName":"A","openDingtalkId":"D1"},{"memberEmpName":"B","openDingtalkId":"D2"}]}}`,
		},
		"bot/list_group_bots": {`{"result":{"bots":[{"robotName":"R","robotCode":"robot"}]}}`},
	}}
	helpers.InitDeps(caller)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{"chat", "+chat-members-list", "--conversation-id", "cid"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if caller.counts["chat/get_group_members"] != 2 {
		t.Fatalf("member page calls = %#v", caller.counts)
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	counts, _ := payload["counts"].(map[string]any)
	buckets, _ := payload["buckets"].(map[string]any)
	users, _ := buckets["users"].(map[string]any)
	if payload["contractVersion"] != groupMembersContractVersion || payload["complete"] != true ||
		counts["users"] != float64(2) || counts["bots"] != float64(1) ||
		users["pagesFetched"] != float64(2) || users["complete"] != true {
		t.Fatalf("member buckets = %#v", payload)
	}
}

func TestCrossPlatformCoverageChatMembersListPageLimitPublishesContinuation(t *testing.T) {
	caller := &smartCoverageCaller{responses: map[string][]string{
		"chat/get_group_members": {
			`{"result":{"hasMore":true,"nextCursor":"2","list":[{"memberEmpName":"A","openDingtalkId":"D1"}]}}`,
		},
	}}
	helpers.InitDeps(caller)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{
		"chat", "+chat-members-list", "--conversation-id", "cid",
		"--member-types", "user", "--page-limit", "1",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["complete"] != false || payload["hasMore"] != true || payload["nextCursor"] != "2" {
		t.Fatalf("bounded member payload = %#v", payload)
	}
	buckets := payload["buckets"].(map[string]any)
	users := buckets["users"].(map[string]any)
	if users["stopReason"] != "page_limit" || users["failedCount"] != float64(0) {
		t.Fatalf("bounded user bucket = %#v", users)
	}
}

func TestCrossPlatformCoverageChatMembersListKeepsEarlierPagesWhenLaterReadFails(t *testing.T) {
	caller := &smartCoverageCaller{
		responses: map[string][]string{
			"chat/get_group_members": {
				`{"result":{"hasMore":true,"nextCursor":"2","list":[{"memberEmpName":"A","openDingtalkId":"D1"}]}}`,
			},
		},
		failAt: map[string]int{"chat/get_group_members": 2},
	}
	helpers.InitDeps(caller)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{
		"chat", "+chat-members-list", "--conversation-id", "cid", "--member-types", "user",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["complete"] != false || payload["partial"] != true ||
		payload["hasMore"] != true || payload["nextCursor"] != "2" ||
		payload["failedCount"] != float64(1) {
		t.Fatalf("partial member payload = %#v", payload)
	}
	buckets := payload["buckets"].(map[string]any)
	users := buckets["users"].(map[string]any)
	if users["count"] != float64(1) || users["pagesFetched"] != float64(1) ||
		users["stopReason"] != "read_failure" || users["partial"] != true {
		t.Fatalf("partial user bucket = %#v", users)
	}
}

func TestCrossPlatformCoverageChatMembersGroupResolutionAndProjection(t *testing.T) {
	cases := []struct {
		name      string
		search    string
		fail      bool
		wantError bool
	}{
		{name: "search error", fail: true, wantError: true},
		{name: "none", search: `{"result":[]}`, wantError: true},
		{name: "ambiguous", search: `{"result":[{"openConversationId":"c1","title":"群"},{"openConversationId":"c2","title":"群"}]}`, wantError: true},
		{name: "one", search: `{"result":[{"openConversationId":"c1","title":"群"}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			failAt := map[string]int{}
			if tc.fail {
				failAt["im/search_groups"] = 1
			}
			caller := &smartCoverageCaller{
				responses: map[string][]string{
					"im/search_groups":       {tc.search},
					"chat/get_group_members": {`{"result":{"list":[]}}`},
					"bot/list_group_bots":    {`{"result":{"bots":[]}}`},
				},
				failAt: failAt,
			}
			helpers.InitDeps(caller)
			root := newPlatformCoverageRoot()
			root.SetArgs([]string{"chat", "+chat-members-list", "--group", "群"})
			err := root.Execute()
			if (err != nil) != tc.wantError {
				t.Fatalf("error = %v, wantError=%v", err, tc.wantError)
			}
		})
	}

	bots := groupBotProject(map[string]any{"bots": []any{"invalid", map[string]any{"name": "bot"}}})
	if len(bots) != 1 || bots[0]["name"] != "bot" {
		t.Fatalf("bots = %#v", bots)
	}
	if groupBotProject(map[string]any{"result": "invalid"}) == nil {
		t.Fatal("empty bot projection should be a non-nil empty slice")
	}
}

func TestCrossPlatformCoverageSearchValidationAndTimeErrors(t *testing.T) {
	cases := [][]string{
		{},
		{"--query", "x", "--start", "2026-07-01T00:00:00Z"},
		{"--query", "x", "--days", "0"},
		{"--query", "x", "--limit", "101"},
		{"--query", "x", "--page-limit", "0"},
		{"--query", "x", "--start", "bad", "--end", "2026-07-02T00:00:00Z"},
		{"--query", "x", "--start", "2026-07-01T00:00:00Z", "--end", "bad"},
		{"--query", "x", "--start", "2026-07-02T00:00:00Z", "--end", "2026-07-01T00:00:00Z"},
	}
	for _, tail := range cases {
		helpers.InitDeps(&smartCoverageCaller{})
		root := newPlatformCoverageRoot()
		root.SetArgs(append([]string{"chat", "+search-msg", "--yes"}, tail...))
		if err := root.Execute(); err == nil {
			t.Errorf("invalid search args succeeded: %v", tail)
		}
	}
}

func TestCrossPlatformCoverageSearchPaginationFailureModes(t *testing.T) {
	cases := []struct {
		name      string
		responses []string
		failAt    int
		args      []string
		wantError bool
	}{
		{name: "initial error", failAt: 1, args: []string{"--query", "x"}, wantError: true},
		{
			name: "duplicate and inferred cursor",
			responses: []string{
				`{"result":{"messages":[{"openMessageId":"m1"},{"openMessageId":"m1"}],"nextCursor":"c2"}}`,
				`{"result":{"messages":[],"hasMore":false}}`,
			},
			args: []string{"--query", "x", "--page-all", "--no-enrich"},
		},
		{
			name:      "stalled cursor",
			responses: []string{`{"result":{"messages":[],"hasMore":true,"nextCursor":"same"}}`},
			args:      []string{"--query", "x", "--cursor", "same", "--page-all", "--no-enrich"},
		},
		{
			name:      "page limit",
			responses: []string{`{"result":{"messages":[],"hasMore":true,"nextCursor":"next"}}`},
			args:      []string{"--query", "x", "--page-all", "--page-limit", "1", "--no-enrich"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caller := &smartCoverageCaller{
				responses: map[string][]string{"im/search_messages": tc.responses},
				failAt:    map[string]int{"im/search_messages": tc.failAt},
			}
			helpers.InitDeps(caller)
			root := newPlatformCoverageRoot()
			var output bytes.Buffer
			root.SetOut(&output)
			root.SetArgs(append([]string{"chat", "+search-msg", "--yes"}, tc.args...))
			err := root.Execute()
			if (err != nil) != tc.wantError {
				t.Fatalf("error = %v, wantError=%v", err, tc.wantError)
			}
			if err == nil {
				var payload map[string]any
				if decodeErr := json.Unmarshal(output.Bytes(), &payload); decodeErr != nil {
					t.Fatalf("decode %q: %v", strings.TrimSpace(output.String()), decodeErr)
				}
				if payload["complete"] != false && tc.name != "duplicate and inferred cursor" {
					t.Fatalf("payload = %#v", payload)
				}
			}
		})
	}
}

func TestCrossPlatformCoverageSmartResolutionAdapters(t *testing.T) {
	if defaultChatPageLimit(0, 50) != 50 || defaultChatPageLimit(7, 50) != 7 {
		t.Fatal("default page limit normalization failed")
	}
	if err := localChatOptionError("fixture", "fixture"); err == nil {
		t.Fatal("local validation error unexpectedly nil")
	}
	users := []contactUser{
		{userID: "u1", openDingTalkID: "d1", name: "甲"},
		{openDingTalkID: "d2", name: "乙"},
	}
	if got := usersWithUserID(users); len(got) != 1 || got[0].userID != "u1" {
		t.Fatalf("usersWithUserID = %#v", got)
	}
	extracted := extractUsers(map[string]any{"result": []any{
		map[string]any{"userId": "u1", "openDingTalkId": "d1", "name": "甲"},
	}})
	if len(extracted) != 1 || extracted[0].openDingTalkID != "d1" {
		t.Fatalf("extractUsers = %#v", extracted)
	}
	if extractUsers(map[string]any{}) != nil {
		t.Fatal("empty extraction should return nil")
	}
	labels := userLabels(users)
	if len(labels) != 2 || !strings.Contains(labels[0], "甲") {
		t.Fatalf("user labels = %#v", labels)
	}
	resolved := targetresolver.User{UserID: "u", OpenDingTalkID: "d", Name: "姓名"}
	if got := toResolvedUser(fromResolvedUser(resolved)); got != resolved {
		t.Fatalf("user adapter round trip = %#v", got)
	}

	groups := extractGroupsForSend(map[string]any{"result": []any{
		map[string]any{"openConversationId": "cid-1", "title": "项目群"},
		map[string]any{"openConversationId": "cid-2", "title": "项目群备份"},
	}})
	if len(groups) != 2 || groups[0].id != "cid-1" {
		t.Fatalf("extractGroupsForSend = %#v", groups)
	}
	preferred := preferExactGroupMatches(groups, "项目群")
	if len(preferred) != 1 || preferred[0].id != "cid-1" {
		t.Fatalf("preferExactGroupMatches = %#v", preferred)
	}
	if labels := sendGroupLabels(groups); len(labels) != 2 || !strings.Contains(labels[0], "项目群") {
		t.Fatalf("group labels = %#v", labels)
	}
}

func TestCrossPlatformCoverageChatMessagesValidationAndFailureBoundaries(t *testing.T) {
	invalid := [][]string{
		{"--group", "cid", "--page-limit", "2"},
		{"--group", "cid", "--page-all", "--page-limit", "0"},
		{"--group", "cid", "--page-all", "--page-limit", "501"},
		{"--group", "cid", "--page-all", "--max-results", "-1"},
		{"--group", "cid", "--overwrite"},
		{"--group", "cid", "--output", "/absolute.json"},
		{"--group", "missing"},
		{"--user-query", "missing"},
	}
	for _, tail := range invalid {
		helpers.InitDeps(&smartCoverageCaller{})
		root := newPlatformCoverageRoot()
		root.SetArgs(append([]string{"chat", "+chat-messages"}, tail...))
		if err := root.Execute(); err == nil {
			t.Errorf("invalid chat messages args succeeded: %v", tail)
		}
	}

	for _, tc := range []struct {
		name      string
		responses []string
		failAt    int
		args      []string
		wantError bool
	}{
		{name: "single read failure", failAt: 1, args: []string{"--group", "cid123456789"}, wantError: true},
		{name: "all first read failure", failAt: 1, args: []string{"--group", "cid123456789", "--page-all"}, wantError: true},
		{name: "all later read failure", responses: []string{`{"result":{"messages":[{"openMessageId":"m1","createTime":"2"}],"hasMore":true,"nextCursor":1767225600123}}`}, failAt: 2, args: []string{"--group", "cid123456789", "--page-all"}, wantError: true},
		{name: "missing pagination", responses: []string{`{"result":{"messages":[]}}`}, args: []string{"--group", "cid123456789", "--page-all"}, wantError: true},
		{name: "empty page with continuation", responses: []string{`{"result":{"messages":[],"hasMore":true}}`}, args: []string{"--group", "cid123456789", "--page-all"}, wantError: true},
		{name: "result limit without boundary", responses: []string{`{"result":{"messages":[{"openMessageId":"m1"},{"openMessageId":"m2"}],"hasMore":true}}`}, args: []string{"--group", "cid123456789", "--page-all", "--max-results", "1"}, wantError: true},
		{name: "result limit with boundary", responses: []string{`{"result":{"messages":[{"openMessageId":"m1","createTime":"2"}],"hasMore":true,"nextCursor":1767225600123}}`}, args: []string{"--group", "cid123456789", "--page-all", "--max-results", "1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &smartCoverageCaller{
				responses: map[string][]string{"chat/list_conversation_message_v2": tc.responses},
				failAt:    map[string]int{"chat/list_conversation_message_v2": tc.failAt},
			}
			helpers.InitDeps(caller)
			root := newPlatformCoverageRoot()
			root.SetArgs(append([]string{"chat", "+chat-messages"}, tc.args...))
			err := root.Execute()
			if (err != nil) != tc.wantError {
				t.Fatalf("error = %v, wantError=%v", err, tc.wantError)
			}
		})
	}

	caller := &smartCoverageCaller{responses: map[string][]string{
		"chat/list_conversation_message_v2": {`{"result":{"messages":[],"hasMore":false}}`},
	}}
	helpers.InitDeps(caller)
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{"chat", "+chat-messages", "--group", "cid123456789", "--output", "messages.json", "--dry-run"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageGroupMemberAndAtMeFailureBoundaries(t *testing.T) {
	for _, args := range [][]string{
		{"chat", "+group-members", "--group", "missing"},
		{"chat", "+group-members", "--group", "cid", "--page-limit", "0"},
		{"chat", "+chat-members-list", "--conversation-id", "cid", "--page-limit", "501"},
		{"chat", "+at-me", "--group", "missing"},
	} {
		helpers.InitDeps(&smartCoverageCaller{})
		root := newPlatformCoverageRoot()
		root.SetArgs(args)
		if err := root.Execute(); err == nil {
			t.Errorf("invalid args succeeded: %v", args)
		}
	}

	caller := &smartCoverageCaller{
		responses: map[string][]string{
			"im/search_groups":       {`{"result":[{"openConversationId":"cid","title":"群"}]}`},
			"chat/get_group_members": {`{"result":{"hasMore":true,"list":[]}}`},
		},
	}
	helpers.InitDeps(caller)
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{"chat", "+group-members", "--group", "群"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{
		{"chat", "+chat-messages", "--user", "u1"},
		{"chat", "+unread-chats", "--count", "1"},
	} {
		helpers.InitDeps(&smartCoverageCaller{responses: map[string][]string{
			"chat/list_individual_chat_message":      {`{"result":{"messages":[],"hasMore":false}}`},
			"chat/list_unread_conversation_messages": {`{"result":[]}`},
		}})
		root := newPlatformCoverageRoot()
		root.SetArgs(args)
		if err := root.Execute(); err != nil {
			t.Fatalf("args %v: %v", args, err)
		}
	}

	helpers.InitDeps(&smartCoverageCaller{})
	root = newPlatformCoverageRoot()
	root.SetArgs([]string{"chat", "+search-msg", "--query", "x", "--chat-query", "missing", "--yes"})
	if err := root.Execute(); err == nil {
		t.Fatal("missing search chat query unexpectedly resolved")
	}

	helpers.InitDeps(&smartCoverageCaller{responses: map[string][]string{
		"contact/search_contact_by_key_word": {`{"result":[{"userId":"u1","name":"甲"}]}`},
		"im/search_messages":                 {`{"result":{"messages":[],"hasMore":false}}`},
	}})
	root = newPlatformCoverageRoot()
	root.SetArgs([]string{"chat", "+search-msg", "--query", "x", "--sender-query", "甲", "--yes"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
}
