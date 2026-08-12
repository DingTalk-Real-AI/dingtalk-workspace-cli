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

// TestBotFindProjectBotsShape guards the exact search_bots response contract:
// entries are nested under result.bots and expose botOpenDingTalkId. Losing
// either key makes +bot-find silently report no usable bots.
func TestBotFindProjectBotsShape(t *testing.T) {
	const raw = `{"result":{"bots":[
		{"botOpenDingTalkId":"bot-open-id-1","name":"Bot One"},
		{"botOpenDingTalkId":"bot-open-id-2","name":"Bot Two"}
	]}}`
	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}

	got := botFindProject(data)
	if len(got) != 2 {
		t.Fatalf("lower/upper mismatch: result.bots has 2 entries, projection returned %d (%v)", len(got), got)
	}
	if got[0]["openDingTalkId"] != "bot-open-id-1" || got[0]["name"] != "Bot One" {
		t.Fatalf("bot fields missing: %v", got[0])
	}
}

// TestBotSearchProjectRobotsShape ensures the shared resolver still accepts the
// search_my_robots response after adding the search_bots container.
func TestBotSearchProjectRobotsShape(t *testing.T) {
	const raw = `{"result":{"robots":[{"robotCode":"robot-1","robotName":"Robot One"}]}}`
	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}

	got := botSearchProject(data)
	if len(got) != 1 || got[0]["robotCode"] != "robot-1" || got[0]["robotName"] != "Robot One" {
		t.Fatalf("robots shape regressed: %v", got)
	}
}
