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

package mail

import (
	"encoding/json"
	"testing"
)

// TestThreadListProjectConversationsShape guards the exact
// list_mailbox_threads response contract, including lastModifiedDateTime.
func TestThreadListProjectConversationsShape(t *testing.T) {
	const raw = `{"result":{"conversations":[{
		"id":"thread-1",
		"subject":"Projection contract",
		"lastModifiedDateTime":"2026-07-26T10:00:00Z",
		"isRead":false
	}]}}`
	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}

	got := threadListProject(data)
	if len(got) != 1 {
		t.Fatalf("lower/upper mismatch: result.conversations has 1 entry, projection returned %d (%v)", len(got), got)
	}
	want := map[string]any{
		"conversationId": "thread-1",
		"subject":        "Projection contract",
		"lastUpdated":    "2026-07-26T10:00:00Z",
		"isRead":         false,
	}
	for key, value := range want {
		if got[0][key] != value {
			t.Fatalf("%s mismatch: want %v, got %v (row=%v)", key, value, got[0][key], got[0])
		}
	}
}
