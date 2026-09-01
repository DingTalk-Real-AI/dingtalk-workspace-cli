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

package chat

import (
	"strings"
	"testing"
)

const testCipher = "SwzNkAraDE6lUHUNlVT3mjFdbxL6dWvmt77XtjACdpJx9VFibzTbW9KtDbkzGOYP||2||1||1"

func TestCrossPlatformCoverageListMessageProjectOne(t *testing.T) {
	// full field mapping + forwarded expansion; an encrypted body is marked (no
	// cross-conversation recovery), not leaked as base64.
	row := listMessageProjectOne(map[string]any{
		"openMessageId":        "mid",
		"senderOpenDingTalkId": "DXYZ",
		"msgType":              "text",
		"messageAiSendFlag":    "DWS",
		"createTime":           "2026-07-19 13:37:03",
		"updateTime":           "2026-07-19 14:00:00",
		"content":              testCipher,
		"emotionReplyList": []any{
			map[string]any{"emoji": "赞", "replyUsers": []any{"D1", "D2"}},
		},
		"forwardMessages": []any{
			map[string]any{"openMessageId": "c1", "senderOpenDingTalkId": "DA", "content": "子消息", "createTime": "t", "messageAiSendFlag": "DWS"},
		},
	})

	if row["messageId"] != "mid" || row["senderId"] != "DXYZ" || row["msgType"] != "text" {
		t.Fatalf("field mapping = %#v", row)
	}
	if row["messageAiSendFlag"] != "DWS" {
		t.Fatalf("AI send flag = %#v", row)
	}
	if row["createTime"] != "2026-07-19 13:37:03" {
		t.Errorf("createTime = %v", row["createTime"])
	}
	if row["updateTime"] != "2026-07-19 14:00:00" {
		t.Errorf("updateTime = %v", row["updateTime"])
	}
	if reactions, ok := row["reactions"].(map[string]any); !ok || len(reactions) == 0 {
		t.Errorf("reactions = %#v", row["reactions"])
	}
	if s, _ := row["text"].(string); !strings.Contains(s, "加密消息") || strings.Contains(s, "||2||1||") {
		t.Errorf("encrypted text = %v, want marker", row["text"])
	}
	fwd, ok := row["forwarded"].([]map[string]any)
	if !ok || len(fwd) != 1 || fwd[0]["messageId"] != "c1" || fwd[0]["text"] != "子消息" || fwd[0]["messageAiSendFlag"] != "DWS" {
		t.Errorf("forwarded = %#v", row["forwarded"])
	}

	// a bare message with no recognizable fields → empty row (no keys)
	row = listMessageProjectOne(map[string]any{"unrelated": 1})
	if len(row) != 0 {
		t.Errorf("empty message row = %#v, want no keys", row)
	}

	row = listMessageProjectOneWithReactions(map[string]any{
		"openMessageId": "mid",
		"emotionReplyList": []any{
			map[string]any{"emoji": "赞", "replyUsers": []any{"D1"}},
		},
	}, false)
	if _, ok := row["reactions"]; ok {
		t.Errorf("no-reactions projection leaked reactions: %#v", row)
	}
}

func TestCrossPlatformCoverageAttachMessageResourceDownloadsPreservesMessagesAndMarksIncomplete(t *testing.T) {
	payload := map[string]any{
		"messages":    []map[string]any{{"messageId": "msg-1"}},
		"complete":    true,
		"failedCount": 0,
		"failures":    []map[string]any{},
	}
	ledger := map[string]any{
		"failedCount": 1,
		"failures": []map[string]any{{
			"messageId": "msg-1",
			"error":     "download failed",
		}},
	}
	AttachMessageResourceDownloads(payload, ledger)
	if payload["complete"] != false || payload["failedCount"] != 1 {
		t.Fatalf("task completeness = %#v", payload)
	}
	messages, _ := payload["messages"].([]map[string]any)
	if len(messages) != 1 || messages[0]["messageId"] != "msg-1" {
		t.Fatalf("messages were dropped: %#v", messages)
	}
	failures, _ := payload["failures"].([]map[string]any)
	if len(failures) != 1 || failures[0]["stage"] != "resource-download" {
		t.Fatalf("resource failures = %#v", failures)
	}
}

func TestCrossPlatformCoverageListPinProjectPreservesThreadIdentity(t *testing.T) {
	got := listPinProject(map[string]any{
		"result": map[string]any{
			"messages": []any{
				map[string]any{
					"openMessageId":      "msg-1",
					"openConversationId": "cid-1",
					"openConvThreadId":   "thread-1",
					"messageAiSendFlag":  "DWS",
				},
			},
		},
	})
	if len(got) != 1 {
		t.Fatalf("pins = %#v", got)
	}
	if got[0]["messageId"] != "msg-1" || got[0]["threadId"] != "thread-1" || got[0]["messageAiSendFlag"] != "DWS" {
		t.Fatalf("pin identity = %#v", got[0])
	}
}
