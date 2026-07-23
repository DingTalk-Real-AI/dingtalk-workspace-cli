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

package calendar

import (
	"encoding/json"
	"testing"
)

// TestRoomGroupsProjectGroupListShape guards against projection-data-loss:
// list_meeting_room_groups nests the groups under result.groupList; the resolver
// must probe "groupList" or +room-groups silently returns empty despite the
// backend returning meeting-room groups.
func TestRoomGroupsProjectGroupListShape(t *testing.T) {
	const raw = `{"result":{"groupList":[
		{"groupId":"g1","groupName":"北楼会议室"},
		{"groupId":"g2","groupName":"南楼会议室"}
	]}}`
	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	if got := roomGroupsProject(data); len(got) != 2 {
		t.Fatalf("上下层数据不一致: 底层 result.groupList 有 2 条，投影返回 %d 条", len(got))
	}
}
