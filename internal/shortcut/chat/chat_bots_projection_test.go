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
	"encoding/json"
	"testing"
)

// TestChatBotsProjectBotsShape guards against projection-data-loss:
// list_group_bots nests its entries under result.bots; the shared resolver
// must probe "bots" or +chat-bots silently reports zero robots.
func TestChatBotsProjectBotsShape(t *testing.T) {
	const raw = `{"result":{"bots":[
		{"openBotId":"bot-1","name":"AI小钉"},
		{"openBotId":"bot-2","name":"本地opencode"},
		{"openBotId":"bot-3","name":"hermes助手"},
		{"openBotId":"bot-4","name":"claudecode 助手"},
		{"openBotId":"bot-5","name":"ClawdBot"}
	]}}`
	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}

	got := chatBotsProject(data)
	if len(got) != 5 {
		t.Fatalf("lower/upper mismatch: result.bots has 5 entries, projection returned %d (%v)", len(got), got)
	}
	if got[0]["openBotId"] != "bot-1" || got[0]["name"] != "AI小钉" {
		t.Fatalf("bot fields missing: %v", got[0])
	}
}
