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

package chatmsg

import (
	"strings"
	"testing"
)

func TestCrossPlatformCoverageSender(t *testing.T) {
	// The display name lives under the bare "sender" key.
	if got := Sender(map[string]any{"sender": "念晨", "senderOpenDingTalkId": "D1"}); got != "念晨" {
		t.Fatalf("sender = %v, want 念晨", got)
	}
	// Falls back to the open id when no display name is present.
	if got := Sender(map[string]any{"senderOpenDingTalkId": "DXYZ"}); got != "DXYZ" {
		t.Fatalf("sender fallback = %v, want DXYZ", got)
	}
	// forwardMessages entries carry the literal string "null" — treat as absent.
	if got := Sender(map[string]any{"sender": "null"}); got != nil {
		t.Fatalf("sender \"null\" = %v, want nil", got)
	}
	if got := Sender(map[string]any{"sender": "null", "senderName": "念晨"}); got != "念晨" {
		t.Fatalf("sender \"null\" fallthrough = %v, want 念晨", got)
	}
	// A nested {name:…} sender object yields its display name, not the raw map.
	if got := Sender(map[string]any{"sender": map[string]any{"name": "Alice"}}); got != "Alice" {
		t.Fatalf("nested sender = %v, want Alice", got)
	}
	// A nested sender object with no usable name must not block the fallback.
	if got := Sender(map[string]any{"sender": map[string]any{"foo": "bar"}, "senderName": "Bob"}); got != "Bob" {
		t.Fatalf("nested-no-name fallthrough = %v, want Bob", got)
	}
	// A scalar numeric id is returned as-is.
	if got := Sender(map[string]any{"senderId": float64(42)}); got != float64(42) {
		t.Fatalf("numeric sender id = %v", got)
	}
}

func TestCrossPlatformCoverageProjectMessageV1PublishesSharedIdentityAndContext(t *testing.T) {
	row := ProjectMessageV1(map[string]any{
		"openMessageId":      "msg-1",
		"openConversationId": "cid-1",
		"openConvThreadId":   "thread-1",
		"sender": map[string]any{
			"name":           "张三",
			"openDingTalkId": "D1",
			"senderType":     "user",
		},
		"msgType":    "text",
		"content":    "你好",
		"createTime": "2026-08-03 10:00:00",
	}, true)
	for key, want := range map[string]any{
		"messageId":      "msg-1",
		"conversationId": "cid-1",
		"threadId":       "thread-1",
		"sender":         "张三",
		"senderId":       "D1",
		"senderType":     "user",
		"messageType":    "text",
		"text":           "你好",
	} {
		if row[key] != want {
			t.Errorf("%s = %#v, want %#v; row=%#v", key, row[key], want, row)
		}
	}
}

func TestCrossPlatformCoverageCleanText(t *testing.T) {
	// Out-of-office auto-reply: readable body lives in items[].data.text; the
	// decorative preview/config JSON lines and "empty" placeholder are dropped.
	autoReply := "* 仅你和对方可见\n" +
		`[{"text":{"minSupportVersion":"1.1","translateMap":{},"version":"1.2","items":[{"fallbackKey":"","data":{"text":"你好，我在出差中，消息回复可能不及时。"},"style":{"size":15,"bold":0},"type":"text"}]},"type":"markdown"}]` + "\n" +
		`{"previewUrl":"dingtalk://x","title":{"text":"自动回复","type":"text"}}` + "\n" +
		"empty\n" +
		`{"autoLayout":false,"enableForward":false}`
	if got, want := CleanText(autoReply), "* 仅你和对方可见\n你好，我在出差中，消息回复可能不及时。"; got != want {
		t.Fatalf("auto-reply cleaned = %q, want %q", got, want)
	}

	// P1 regression: ordinary text whose middle line is a JSON fragment (no
	// rich-content block anywhere) must be returned VERBATIM, not rewritten.
	mixed := "payload:\n{\"approved\":false}\nplease check"
	if got := CleanText(mixed); got != mixed {
		t.Fatalf("mixed text was rewritten: got %q, want %q", got, mixed)
	}

	// An ordinary JSON line must also survive when a different line contains a
	// recognised rich-content block. Card mode is not permission to discard
	// unrelated user-authored JSON.
	richAndPlain := `[{"items":[{"data":{"text":"卡片正文"}}]}]` + "\n" +
		`{"approved":false}`
	if got, want := CleanText(richAndPlain), "卡片正文\n{\"approved\":false}"; got != want {
		t.Fatalf("mixed rich/plain JSON was rewritten: got %q, want %q", got, want)
	}

	// Malformed items (non-map item, item whose "data" isn't a map) are skipped;
	// only the well-formed item's text is extracted.
	blob := `[{"items":["notmap",{"data":"notmap"},{"data":{"text":"有效正文"}}]}]`
	if got := CleanText(blob); got != "有效正文" {
		t.Fatalf("CleanText rich edge = %q, want 有效正文", got)
	}

	tests := map[string]string{
		"上周五 7.1 KW": "上周五 7.1 KW",
		"上周客户统计的[图片消息](mediaId=@lQ)":                              "上周客户统计的[图片消息](mediaId=@lQ)",
		"[文件] 简历.pdf fileId: qnY 注意：如需下载使用dws drive download命令下载": "[文件] 简历.pdf fileId: qnY 注意：如需下载使用dws drive download命令下载",
		"[讨论] 排期\n明天开会":                                           "[讨论] 排期\n明天开会",
		// a lone JSON object that isn't a rich-content block is left untouched
		`{"autoLayout":false,"enableForward":false}`: `{"autoLayout":false,"enableForward":false}`,
	}
	for in, want := range tests {
		if got := CleanText(in); got != want {
			t.Errorf("CleanText(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCrossPlatformCoverageIsEncryptedAndMarker(t *testing.T) {
	cipher := "SwzNkAraDE6lUHUNlVT3mjFdbxL6dWvmt77XtjACdpJx9VFibzTbW9KtDbkzGOYP\n" +
		"7oDptklFO+YzDltH+myErV6rkc8URHYykpeSDsMP6kznFa9E320NsIntfY771dx+\n" +
		"||2||1||196"
	if !IsEncrypted(cipher) {
		t.Fatalf("ciphertext not detected: %q", cipher)
	}
	if got := CleanText(cipher); !strings.Contains(got, "加密消息") || strings.Contains(got, "||2||1||") {
		t.Fatalf("encrypted cleaned = %q, want marker not ciphertext", got)
	}
	for _, s := range []string{
		"上周五 7.1 KW",
		"价格 100||2||1||3",
		"[图片消息](mediaId=@lQLPJwDw3VmNDcfMos0DhLB3OHPQeTBlzgov2Oi1ly4A)",
		"大哥，我看了一下我觉得有几个点可以关注一下",
		strings.Repeat("好", 20) + "||2||1||1", // long CJK body + trailer, not base64
	} {
		if IsEncrypted(s) {
			t.Errorf("false positive: %q flagged as encrypted", s)
		}
	}
}

func TestCrossPlatformCoverageText(t *testing.T) {
	if got := Text(map[string]any{"content": "你好"}); got != "你好" {
		t.Errorf("Text string = %v", got)
	}
	if got := Text(map[string]any{"content": map[string]any{"text": "嵌套"}}); got != "嵌套" {
		t.Errorf("Text nested = %v", got)
	}
	if got := Text(map[string]any{"plainText": "纯文本"}); got != "纯文本" {
		t.Errorf("Text plainText = %v", got)
	}
	if got := Text(map[string]any{"foo": 1}); got != nil {
		t.Errorf("Text none = %v, want nil", got)
	}
}

func TestCrossPlatformCoverageCreateTime(t *testing.T) {
	if got := CreateTime(map[string]any{"sendTime": "2026-07-19 13:37:03"}); got != "2026-07-19 13:37:03" {
		t.Errorf("CreateTime = %v", got)
	}
	if got := CreateTime(map[string]any{}); got != nil {
		t.Errorf("CreateTime empty = %v, want nil", got)
	}
}

func TestCrossPlatformCoverageStableMessageIdentity(t *testing.T) {
	message := map[string]any{
		"openMessageId":      "msg-1",
		"openConversationId": "cid-1",
		"openConvThreadId":   "thread-1",
		"msgType":            "text",
	}
	if got := MessageID(message); got != "msg-1" {
		t.Errorf("MessageID = %v", got)
	}
	if got := ConversationID(message); got != "cid-1" {
		t.Errorf("ConversationID = %v", got)
	}
	if got := ThreadID(message); got != "thread-1" {
		t.Errorf("ThreadID = %v", got)
	}
	if got := MessageType(message); got != "text" {
		t.Errorf("MessageType = %v", got)
	}
}

func TestCrossPlatformCoverageMessageLedgerNilAndCursorOnlyBoundaries(t *testing.T) {
	contract := CurrentMessageResultContract()
	if contract.Version != MessageListContractVersion || len(contract.MessageFields) == 0 || len(contract.EnvelopeFields) == 0 {
		t.Fatalf("message result contract = %#v", contract)
	}
	contract.MessageFields[0] = "mutated"
	contract.EnvelopeFields[0] = "mutated"
	second := CurrentMessageResultContract()
	if second.MessageFields[0] == "mutated" || second.EnvelopeFields[0] == "mutated" {
		t.Fatal("message result contract leaked mutable storage")
	}
	payload := NewMessageListPayload(nil)
	if payload["count"] != 0 || payload["messages"] == nil {
		t.Fatalf("nil message ledger = %#v", payload)
	}
	if StableMessageID(map[string]any{}) != "" {
		t.Fatal("missing message identity was fabricated")
	}
	ApplyMessagePagination(payload, map[string]any{"result": map[string]any{"nextCursor": "next"}}, nil, "older")
	if payload["paginationKnown"] != false || payload["failedCount"] != 1 {
		t.Fatalf("cursor-only pagination = %#v", payload)
	}
}

func TestCrossPlatformCoverageQuotedMessageIsBoundedAndSemantic(t *testing.T) {
	got := QuotedMessage(map[string]any{
		"quotedMessage": map[string]any{
			"openMessageId":      "quoted-1",
			"openConversationId": "cid-1",
			"openConvThreadId":   "thread-1",
			"sender":             "Alice",
			"content":            "原消息",
			"createTime":         "2026-07-28 10:00:00",
			"quotedMessage":      map[string]any{"openMessageId": "nested-must-not-expand"},
		},
	})
	if got["messageId"] != "quoted-1" || got["sender"] != "Alice" || got["text"] != "原消息" {
		t.Fatalf("quoted message = %#v", got)
	}
	if got["threadId"] != "thread-1" {
		t.Fatalf("quoted thread identity = %#v", got)
	}
	if _, recursive := got["quotedMessage"]; recursive {
		t.Fatalf("quoted message expanded recursively: %#v", got)
	}
}

func TestCrossPlatformCoverageResourcesRespectNestedMessageOwnership(t *testing.T) {
	message := map[string]any{
		"openMessageId":      "parent",
		"openConversationId": "cid-parent",
		"content":            map[string]any{"mediaId": "media-parent"},
		"quotedMessage": map[string]any{
			"openMessageId": "quoted",
			"content":       map[string]any{"mediaId": "media-quoted"},
		},
		"forwardMessages": []any{
			map[string]any{
				"openMessageId":      "forwarded",
				"openConversationId": "cid-forwarded",
				"content":            map[string]any{"mediaId": "media-forwarded"},
			},
		},
	}
	owned := Resources(message)
	if len(owned) != 1 || owned[0]["resourceId"] != "media-parent" {
		t.Fatalf("parent resources crossed message boundary: %#v", owned)
	}

	deep := ResourcesDeep(message)
	if len(deep) != 3 {
		t.Fatalf("deep resources = %#v", deep)
	}
	argumentsByResource := map[string]map[string]any{}
	for _, resource := range deep {
		download := resource["download"].(map[string]any)
		argumentsByResource[resource["resourceId"].(string)] = download["arguments"].(map[string]any)
	}
	for resourceID, want := range map[string][2]string{
		"media-parent":    {"parent", "cid-parent"},
		"media-quoted":    {"quoted", "cid-parent"},
		"media-forwarded": {"forwarded", "cid-forwarded"},
	} {
		arguments := argumentsByResource[resourceID]
		if arguments["message-id"] != want[0] ||
			arguments["open-conversation-id"] != want[1] {
			t.Errorf("%s arguments = %#v, want message=%q conversation=%q",
				resourceID, arguments, want[0], want[1])
		}
	}
}

func TestCrossPlatformCoverageResourceBoundaryHelpers(t *testing.T) {
	encoded := map[string]any{
		"openMessageId":      "parent",
		"openConversationId": "cid",
		"content":            `{"quotedMessage":{"openMessageId":"encoded-child","mediaId":"nested"}}`,
	}
	if got := Resources(encoded); got != nil {
		t.Fatalf("encoded nested resource bound to parent: %#v", got)
	}
	if got := ResourcesDeep(encoded); len(got) != 1 {
		t.Fatalf("encoded nested resources = %#v", got)
	} else {
		arguments := got[0]["download"].(map[string]any)["arguments"].(map[string]any)
		if arguments["message-id"] != "encoded-child" ||
			arguments["open-conversation-id"] != "cid" {
			t.Fatalf("encoded child arguments = %#v", arguments)
		}
	}
	if got := resourcesDeep(map[string]any{"mediaId": "x"}, "", maxResourceMessageDepth+1); got != nil {
		t.Fatalf("over-depth resources = %#v", got)
	}
	if got := resourcesDeep(
		map[string]any{"mediaId": "x"},
		"",
		maxResourceMessageDepth,
	); len(got) != 1 {
		t.Fatalf("max-depth owned resources = %#v", got)
	}
	if got := nestedMessageMaps([]map[string]any{{"id": "a"}}); len(got) != 1 {
		t.Fatalf("map slice = %#v", got)
	}
	if got := nestedMessageMaps(`[{"id":"a"}]`); len(got) != 1 {
		t.Fatalf("encoded message list = %#v", got)
	}
	if got := nestedMessageMaps([]any{"bad", map[string]any{"id": "a"}}); len(got) != 1 {
		t.Fatalf("mixed message list = %#v", got)
	}
	if got := nestedMessageMaps("{"); got != nil {
		t.Fatalf("invalid encoded messages = %#v", got)
	}
	if got := nestedMessageChildren([]map[string]any{
		{"content": "plain"},
		{"content": `{"forwardedMessages":[{"openMessageId":"m"}]}`},
	}); len(got) != 1 {
		t.Fatalf("nested message children = %#v", got)
	}
	if got := nestedMessageChildren([]any{
		"plain",
		map[string]any{"quoted": map[string]any{"openMessageId": "m"}},
	}); len(got) != 1 {
		t.Fatalf("mixed nested message children = %#v", got)
	}
	if got := nestedMessageChildren("{"); len(got) != 0 {
		t.Fatalf("invalid nested children = %#v", got)
	}
	if isNestedMessageBoundaryKey("content") {
		t.Fatal("ordinary content treated as a message boundary")
	}
}

func TestCrossPlatformCoverageUpdateTimeOmitsUneditedEcho(t *testing.T) {
	if got := UpdateTime(map[string]any{
		"createTime": "2026-07-19 13:37:03",
		"updateTime": "2026-07-19 13:37:03",
	}); got != nil {
		t.Errorf("UpdateTime echoed create time = %v, want nil", got)
	}
	if got := UpdateTime(map[string]any{
		"createTime": "2026-07-19 13:37:03",
		"updateTime": "2026-07-19 14:00:00",
	}); got != "2026-07-19 14:00:00" {
		t.Errorf("UpdateTime edited = %v", got)
	}
}

func TestCrossPlatformCoverageReactionsNormalizesEmotionReplyList(t *testing.T) {
	got := Reactions(map[string]any{
		"emotionReplyList": []any{
			map[string]any{
				"emoji":      "赞",
				"replyUsers": []any{"user-a", "user-b"},
			},
			map[string]any{
				"emotionName": "收到",
				"replyCount":  float64(3),
			},
		},
	})
	counts, ok := got["counts"].([]map[string]any)
	if !ok || len(counts) != 2 {
		t.Fatalf("reaction counts = %#v", got["counts"])
	}
	if counts[0]["emoji"] != "赞" || counts[0]["count"] != 2 {
		t.Errorf("first reaction count = %#v", counts[0])
	}
	if counts[1]["emoji"] != "收到" || counts[1]["count"] != float64(3) {
		t.Errorf("second reaction count = %#v", counts[1])
	}
	details, ok := got["details"].([]map[string]any)
	if !ok || len(details) != 2 {
		t.Fatalf("reaction details = %#v", got["details"])
	}
	users, ok := details[0]["replyUsers"].([]any)
	if !ok || len(users) != 2 {
		t.Errorf("reaction users = %#v", details[0]["replyUsers"])
	}
	if got := Reactions(map[string]any{}); got != nil {
		t.Errorf("empty reactions = %#v, want nil", got)
	}
}

func TestCrossPlatformCoverageApplyPaginationReadsNestedEnvelope(t *testing.T) {
	payload := map[string]any{"count": 98}
	ApplyPagination(payload, map[string]any{
		"result": map[string]any{
			"hasMore":    true,
			"nextCursor": "cursor-redacted-in-audits",
		},
	})
	if payload["hasMore"] != true || payload["complete"] != false {
		t.Errorf("pagination completeness = %#v", payload)
	}
	if payload["nextCursor"] != "cursor-redacted-in-audits" {
		t.Errorf("nextCursor = %#v", payload["nextCursor"])
	}

	payload = map[string]any{}
	ApplyPagination(payload, map[string]any{
		"data": map[string]any{"has_more": false},
	})
	if payload["hasMore"] != false || payload["complete"] != true {
		t.Errorf("completed pagination = %#v", payload)
	}
}

func TestCrossPlatformCoverageApplyMessagePaginationUsesExecutableTimeBoundary(t *testing.T) {
	payload := map[string]any{}
	ApplyMessagePagination(payload, map[string]any{
		"result": map[string]any{
			"hasMore":    true,
			"nextCursor": "not-a-message-list-cli-flag",
		},
	}, []map[string]any{
		{"createTime": "2026-07-28 10:00:00"},
		{"createTime": "2026-07-28 09:00:00"},
	}, "older")
	if _, leaked := payload["nextCursor"]; leaked {
		t.Fatalf("message pagination exposed unusable cursor: %#v", payload)
	}
	next, ok := payload["nextPage"].(map[string]any)
	if !ok || next["time"] != "2026-07-28 09:00:00" || next["direction"] != "older" {
		t.Fatalf("message nextPage = %#v", payload["nextPage"])
	}
}

func TestCrossPlatformCoverageApplyMessagePaginationFailsClosedWhenCompletenessIsUnknown(t *testing.T) {
	payload := map[string]any{}
	ApplyMessagePagination(payload, map[string]any{"result": map[string]any{"messages": []any{}}}, nil, "older")
	if payload["contractVersion"] != MessageListContractVersion ||
		payload["complete"] != false || payload["paginationKnown"] != false ||
		payload["failedCount"] != 1 {
		t.Fatalf("unknown pagination contract = %#v", payload)
	}
	failures, _ := payload["failures"].([]map[string]any)
	if len(failures) != 1 || failures[0]["stage"] != "pagination" {
		t.Fatalf("unknown pagination failures = %#v", failures)
	}
}

func TestCrossPlatformCoverageResourcesBuildsActionableDownloadReferences(t *testing.T) {
	resources := Resources(map[string]any{
		"openMessageId":      "msg-1",
		"openConversationId": "cid-1",
		"content":            `图片 [图片消息](mediaId=@image-a)`,
		"attachments": []any{
			map[string]any{"mediaId": "@image-b"},
			map[string]any{"content": `{"mediaId":"@image-a"}`},
		},
	})
	if len(resources) != 2 {
		t.Fatalf("resources = %#v", resources)
	}
	first := resources[0]
	if first["resourceId"] != "@image-a" || first["type"] != "mediaId" {
		t.Fatalf("first resource = %#v", first)
	}
	download, _ := first["download"].(map[string]any)
	arguments, _ := download["arguments"].(map[string]any)
	if download["ready"] != true ||
		arguments["message-id"] != "msg-1" ||
		arguments["open-conversation-id"] != "cid-1" ||
		arguments["resource-id"] != "@image-a" {
		t.Fatalf("download = %#v", download)
	}
}

func TestCrossPlatformCoverageResourcesKeepsNestedResourceNamesWithTheirOwner(t *testing.T) {
	message := map[string]any{
		"openMessageId":      "parent-message",
		"openConversationId": "cid-1",
		"content":            `{"fileId":"parent-file","fileName":"parent.pdf"}`,
		"quotedMessage": map[string]any{
			"openMessageId": "quoted-message",
			"content":       `{"fileId":"quoted-file","file_name":"quoted.pdf"}`,
		},
	}
	resources := ResourcesDeep(message)
	if len(resources) != 2 {
		t.Fatalf("resources = %#v", resources)
	}
	if resources[0]["resourceId"] != "parent-file" || resources[0]["name"] != "parent.pdf" {
		t.Fatalf("parent resource = %#v", resources[0])
	}
	if resources[1]["resourceId"] != "quoted-file" || resources[1]["name"] != "quoted.pdf" {
		t.Fatalf("quoted resource = %#v", resources[1])
	}
}

func TestCrossPlatformCoverageResourcesReportsMissingDownloadContext(t *testing.T) {
	resources := Resources(map[string]any{"content": `{"mediaId":"@image-a"}`})
	if len(resources) != 1 {
		t.Fatalf("resources = %#v", resources)
	}
	download, _ := resources[0]["download"].(map[string]any)
	if download["ready"] != false {
		t.Fatalf("download = %#v", download)
	}
	missing, _ := download["missing"].([]string)
	if len(missing) != 2 || missing[0] != "message-id" || missing[1] != "open-conversation-id" {
		t.Fatalf("missing = %#v", missing)
	}
}

func TestCrossPlatformCoverageResourcesTextMediaIDRequiresWordBoundary(t *testing.T) {
	resources := Resources(map[string]any{
		"openMessageId":      "msg-1",
		"openConversationId": "cid-1",
		"content":            `notmediaId=@false mediaId=@real`,
	})
	if len(resources) != 1 || resources[0]["resourceId"] != "@real" {
		t.Fatalf("resources = %#v, want only the bounded mediaId", resources)
	}
}

func TestCrossPlatformCoverageForwarded(t *testing.T) {
	var project func(m map[string]any) map[string]any
	project = func(m map[string]any) map[string]any {
		row := map[string]any{"text": Text(m)}
		if fwd := Forwarded(m, project); len(fwd) > 0 { // recurse
			row["forwarded"] = fwd
		}
		return row
	}
	if got := Forwarded(map[string]any{"content": "x"}, project); got != nil {
		t.Errorf("Forwarded none = %v", got)
	}
	fwd := Forwarded(map[string]any{
		"forwardMessages": []any{
			map[string]any{"content": "a"},
			"not-a-map",
			map[string]any{"content": "b", "forwardMessages": []any{
				map[string]any{"content": "nested"},
			}},
		},
	}, project)
	if len(fwd) != 2 || fwd[0]["text"] != "a" || fwd[1]["text"] != "b" {
		t.Fatalf("Forwarded = %#v", fwd)
	}
	nested, ok := fwd[1]["forwarded"].([]map[string]any)
	if !ok || len(nested) != 1 || nested[0]["text"] != "nested" {
		t.Errorf("nested forwarded = %#v", fwd[1]["forwarded"])
	}
}
