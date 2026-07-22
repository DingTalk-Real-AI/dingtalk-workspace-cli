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

func TestListMessageProjectOne(t *testing.T) {
	// full field mapping + forwarded expansion; an encrypted body is marked (no
	// cross-conversation recovery), not leaked as base64.
	row := listMessageProjectOne(map[string]any{
		"openMessageId":        "mid",
		"senderOpenDingTalkId": "DXYZ",
		"msgType":              "text",
		"createTime":           "2026-07-19 13:37:03",
		"content":              testCipher,
		"forwardMessages": []any{
			map[string]any{"openMessageId": "c1", "senderOpenDingTalkId": "DA", "content": "子消息", "createTime": "t"},
		},
	})

	if row["messageId"] != "mid" || row["senderId"] != "DXYZ" || row["msgType"] != "text" {
		t.Fatalf("field mapping = %#v", row)
	}
	if row["createTime"] != "2026-07-19 13:37:03" {
		t.Errorf("createTime = %v", row["createTime"])
	}
	if s, _ := row["text"].(string); !strings.Contains(s, "加密消息") || strings.Contains(s, "||2||1||") {
		t.Errorf("encrypted text = %v, want marker", row["text"])
	}
	fwd, ok := row["forwarded"].([]map[string]any)
	if !ok || len(fwd) != 1 || fwd[0]["messageId"] != "c1" || fwd[0]["text"] != "子消息" {
		t.Errorf("forwarded = %#v", row["forwarded"])
	}

	// a bare message with no recognizable fields → empty row (no keys)
	row = listMessageProjectOne(map[string]any{"unrelated": 1})
	if len(row) != 0 {
		t.Errorf("empty message row = %#v, want no keys", row)
	}
}
