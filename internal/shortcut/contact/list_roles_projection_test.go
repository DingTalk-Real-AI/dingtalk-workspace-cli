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

// TestListRolesProjectGroupedShape guards against the projection-data-loss class:
// get_org_labels returns result[] as label GROUPS with the real roles nested one
// level under each group's labels[]. The projection MUST descend into those
// nested labels[] rather than treating group wrappers as roles — otherwise it
// silently returns empty (exit 0, no error envelope) while the backend has data.
func TestListRolesProjectGroupedShape(t *testing.T) {
	// Faithful get_org_labels shape (as returned by the backend / leaf command).
	const raw = `{"result":[
		{"groupName":"默认","labels":[
			{"labelId":101,"name":"角色A"},
			{"labelId":102,"name":"角色B"},
			{"labelId":103,"name":"角色C"}]},
		{"groupName":"职务","labels":[
			{"labelId":104,"name":"角色D"},
			{"labelId":105,"name":"角色E"}]}
	]}`
	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}

	roles := listRolesProject(data)
	if len(roles) != 5 {
		t.Fatalf("上下层数据不一致: 底层有 5 个角色，投影返回 %d 个 (roles=%v)", len(roles), roles)
	}
	for _, r := range roles {
		if r["labelId"] == nil || r["labelName"] == nil {
			t.Fatalf("projected role missing labelId/labelName: %v", r)
		}
	}
}

// TestListRolesProjectNestedWrapperAndNonMap covers the nested map-wrapper branch
// ({result:{labels:[...]}}) and a non-map array element, which must be passed
// through rather than dropped.
func TestListRolesProjectNestedWrapperAndNonMap(t *testing.T) {
	const raw = `{"result":{"labels":[
		{"groupName":"默认","labels":[{"labelId":1,"name":"角色A"}]},
		"junk"
	]}}`
	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	if roles := listRolesProject(data); len(roles) != 1 {
		t.Fatalf("nested wrapper + non-map: want 1 role, got %d (%v)", len(roles), roles)
	}
}

// TestListRolesProjectFlatShape ensures the flatten step is a no-op when the
// response is already a flat label list (response-shape drift tolerance).
func TestListRolesProjectFlatShape(t *testing.T) {
	const raw = `{"result":[
		{"labelId":1,"name":"销售"},
		{"labelId":2,"name":"市场"}
	]}`
	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	if roles := listRolesProject(data); len(roles) != 2 {
		t.Fatalf("flat shape: want 2 roles, got %d (%v)", len(roles), roles)
	}
}
