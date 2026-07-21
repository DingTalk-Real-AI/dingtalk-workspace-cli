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

import "testing"

// TestProjectChatMessageExpandsForwarded guards that a forwarded chat record
// ("聊天记录") exposes its nested messages under "forwarded" instead of
// collapsing to the lossy top-level "[卡片]" summary, recursing through nested
// forwards, and that the string-"null" sender is nulled out. The per-field
// behaviour (sender/text/encryption) is covered in the chatmsg package tests.
func TestProjectChatMessageExpandsForwarded(t *testing.T) {
	row := projectChatMessage(map[string]any{
		"sender":     "hugozhu",
		"content":    "hugozhu与opencode-agent的聊天记录\nopencode-agent:[卡片]",
		"createTime": "2026-07-20 21:41:21",
		"forwardMessages": []any{
			map[string]any{"sender": "null", "content": "读下冬翔发给我的最近两条消息", "createTime": "2026-07-20 09:30:33"},
			map[string]any{"sender": "冬翔", "content": "W29 工作总结", "createTime": "2026-07-19 23:35:40",
				// nested forward inside a forward — must expand recursively.
				"forwardMessages": []any{
					map[string]any{"sender": "念晨", "content": "收到", "createTime": "2026-07-19 23:36:00"},
				},
			},
		},
	})

	if row["sender"] != "hugozhu" {
		t.Fatalf("top sender = %v, want hugozhu", row["sender"])
	}
	forwarded, ok := row["forwarded"].([]map[string]any)
	if !ok || len(forwarded) != 2 {
		t.Fatalf("forwarded = %#v, want 2 entries", row["forwarded"])
	}
	if forwarded[0]["sender"] != nil {
		t.Errorf("forwarded[0].sender = %v, want nil (string \"null\")", forwarded[0]["sender"])
	}
	if forwarded[0]["text"] != "读下冬翔发给我的最近两条消息" {
		t.Errorf("forwarded[0].text = %v", forwarded[0]["text"])
	}
	nested, ok := forwarded[1]["forwarded"].([]map[string]any)
	if !ok || len(nested) != 1 || nested[0]["sender"] != "念晨" {
		t.Errorf("nested forwarded = %#v, want 1 entry from 念晨", forwarded[1]["forwarded"])
	}

	// A plain message must not grow a "forwarded" key.
	plain := projectChatMessage(map[string]any{"sender": "念晨", "content": "hi", "createTime": "t"})
	if _, has := plain["forwarded"]; has {
		t.Errorf("plain message unexpectedly has forwarded key: %#v", plain)
	}
}
