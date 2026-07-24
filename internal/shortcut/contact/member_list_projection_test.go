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

package contact

import (
	"encoding/json"
	"testing"
)

// TestMemberListProjectDeptUserListShape guards against projection-data-loss:
// get_dept_members_by_deptId returns members under deptUserList, each wrapped in
// userInfo. The resolver must probe deptUserList AND unwrap userInfo or
// +list-dept-members silently returns empty despite the backend returning members.
func TestMemberListProjectDeptUserListShape(t *testing.T) {
	const raw = `{"deptUserList":[
		{"userInfo":{"userId":"u1","name":"Alice"}},
		{"userInfo":{"userId":"u2","name":"Bob"}}
	]}`
	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	got := memberListProject(data)
	if len(got) != 2 {
		t.Fatalf("lower/upper mismatch: deptUserList has 2 members, projection returned %d (%v)", len(got), got)
	}
	if got[0]["userId"] == nil || got[0]["name"] == nil {
		t.Fatalf("userInfo fields not unwrapped: %v", got[0])
	}
}

// TestMemberListProjectLabelUserListShape guards the role-member path:
// get_label_members_by_labelId nests members under labelUserList, each wrapped
// in userInfo. The resolver must probe labelUserList AND unwrap userInfo or
// +list-role-members silently returns empty despite the role having members.
func TestMemberListProjectLabelUserListShape(t *testing.T) {
	const raw = `{"labelUserList":[
		{"userInfo":{"userId":"u1","name":"Carol"}},
		{"userInfo":{"userId":"u2","name":"Dave"}}
	]}`
	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	got := memberListProject(data)
	if len(got) != 2 {
		t.Fatalf("lower/upper mismatch: labelUserList has 2 members, projection returned %d (%v)", len(got), got)
	}
	if got[0]["userId"] == nil || got[0]["name"] == nil {
		t.Fatalf("userInfo not unwrapped: %v", got[0])
	}
}

// TestMemberListProjectFlatRoleShape ensures the shared projection still handles
// a flat member shape unchanged.
func TestMemberListProjectFlatRoleShape(t *testing.T) {
	const raw = `{"result":[{"userId":"u9","name":"Alice"}]}`
	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	if got := memberListProject(data); len(got) != 1 {
		t.Fatalf("flat role-member shape: want 1, got %d (%v)", len(got), got)
	}
}
