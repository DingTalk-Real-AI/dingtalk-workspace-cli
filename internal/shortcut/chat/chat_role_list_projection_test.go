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

// TestChatRoleListProjectRolesShape guards against projection-data-loss:
// list_custom_group_roles nests the roles under result.roles; the shared
// resolver must probe "roles" or +chat-role-list silently returns empty despite
// the group having custom roles.
func TestChatRoleListProjectRolesShape(t *testing.T) {
	const raw = `{"result":{"roles":[
		{"openRoleId":"r1","name":"CTO"},
		{"openRoleId":"r2","name":"CEO"}
	]}}`
	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	got := chatRoleListProject(data)
	if len(got) != 2 {
		t.Fatalf("lower/upper mismatch: result.roles has 2 entries, projection returned %d (%v)", len(got), got)
	}
	if got[0]["openRoleId"] == nil || got[0]["name"] == nil {
		t.Fatalf("role fields missing: %v", got[0])
	}
}

// TestChatGroupResolveListStillHandlesGroups ensures adding "roles" did not
// disturb the group-listing shape the resolver is shared with.
func TestChatGroupResolveListStillHandlesGroups(t *testing.T) {
	const raw = `{"result":{"groups":[{"openConversationId":"c1","title":"project group"}]}}`
	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	if got := chatGroupResolveList(data); len(got) != 1 {
		t.Fatalf("group shape regressed: want 1, got %d", len(got))
	}
}
