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

package smart

import (
	"strings"
	"testing"
)

const testCipher = "SwzNkAraDE6lUHUNlVT3mjFdbxL6dWvmt77XtjACdpJx9VFibzTbW9KtDbkzGOYP||2||1||1"

func TestAtMeProject(t *testing.T) {
	// nested sender object + plain text
	row := atMeProject(map[string]any{
		"sender":             map[string]any{"name": "念晨"},
		"createTime":         "2026-07-19 13:37:03",
		"content":            "普通消息",
		"conversationTitle":  "群A",
		"openConversationId": "cid1",
	})
	if row["sender"] != "念晨" || row["text"] != "普通消息" || row["conversation"] != "群A" {
		t.Fatalf("atMeProject nested = %#v", row)
	}

	// encrypted content → marked (never leaked); id-only sender fallback; a
	// forwarded sub-message whose sender is the literal "null" must be nulled.
	row = atMeProject(map[string]any{
		"senderId":      "DXYZ",
		"openMessageId": "m1",
		"content":       testCipher,
		"forwardMessages": []any{
			map[string]any{"sender": "null", "content": "子消息", "createTime": "t"},
		},
	})
	if row["sender"] != "DXYZ" {
		t.Errorf("atMeProject id-fallback sender = %v", row["sender"])
	}
	if s, _ := row["text"].(string); !strings.Contains(s, "加密消息") {
		t.Errorf("atMeProject encrypted text = %v, want marker", row["text"])
	}
	fwd, ok := row["forwarded"].([]map[string]any)
	if !ok || len(fwd) != 1 {
		t.Fatalf("atMeProject forwarded = %#v", row["forwarded"])
	}
	if fwd[0]["sender"] != nil {
		t.Errorf("forwarded sub sender = %v, want nil (literal \"null\")", fwd[0]["sender"])
	}

	// no sender / no text at all → nils, no forwarded key
	row = atMeProject(map[string]any{"createTime": "t"})
	if row["sender"] != nil || row["text"] != nil {
		t.Errorf("atMeProject empty = %#v", row)
	}
	if _, has := row["forwarded"]; has {
		t.Errorf("atMeProject plain unexpectedly has forwarded")
	}
}

func TestSearchMsgProject(t *testing.T) {
	// nested sender + plain text + messageId
	row := searchMsgProject(map[string]any{
		"sender":     map[string]any{"nick": "千启"},
		"createTime": "2026-07-19 13:37:03",
		"content":    "命中关键词的消息",
		"msgId":      "mid1",
	})
	if row["sender"] != "千启" || row["text"] != "命中关键词的消息" {
		t.Fatalf("searchMsgProject = %#v", row)
	}

	// encrypted → marker; id-only sender; forwarded "null" sender nulled.
	row = searchMsgProject(map[string]any{
		"senderId":      "DAAA",
		"openMessageId": "m2",
		"content":       testCipher,
		"forwardMessages": []any{
			map[string]any{"sender": "null", "content": "转发子消息", "createTime": "t"},
		},
	})
	if row["sender"] != "DAAA" {
		t.Errorf("searchMsgProject sender = %v", row["sender"])
	}
	if s, _ := row["text"].(string); !strings.Contains(s, "加密消息") {
		t.Errorf("searchMsgProject encrypted text = %v, want marker", row["text"])
	}
	fwd, ok := row["forwarded"].([]map[string]any)
	if !ok || len(fwd) != 1 || fwd[0]["sender"] != nil {
		t.Errorf("searchMsgProject forwarded = %#v", row["forwarded"])
	}

	// no sender / no text
	row = searchMsgProject(map[string]any{"createTime": "t"})
	if row["sender"] != nil || row["text"] != nil {
		t.Errorf("searchMsgProject empty = %#v", row)
	}
}

// TestSenderHelpers exercises the atMe/searchMsg sender key families directly:
// a senderName-family key (first probe loop), a flat string under "sender"
// (second loop), and the "null" sentinel normalisation.
func TestSenderHelpers(t *testing.T) {
	cases := []struct {
		fn   func(map[string]any) any
		name string
	}{
		{atMeSender, "atMeSender"},
		{searchMsgSender, "searchMsgSender"},
	}
	for _, c := range cases {
		if got := c.fn(map[string]any{"senderName": "张三"}); got != "张三" {
			t.Errorf("%s senderName = %v, want 张三", c.name, got)
		}
		if got := c.fn(map[string]any{"sender": "李四"}); got != "李四" {
			t.Errorf("%s flat sender = %v, want 李四", c.name, got)
		}
		if got := c.fn(map[string]any{"senderName": "null"}); got != nil {
			t.Errorf("%s \"null\" = %v, want nil", c.name, got)
		}
	}
}
