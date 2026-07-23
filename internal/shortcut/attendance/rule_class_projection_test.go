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

package attendance

import (
	"encoding/json"
	"testing"
)

// TestSearchClassProjectShiftVOShape guards against projection-data-loss:
// get_class_list nests the list under result.items and wraps each shift's
// identity under shiftVO. The projection must unwrap shiftVO or +search-class
// silently returns empty despite the backend returning shifts.
func TestSearchClassProjectShiftVOShape(t *testing.T) {
	const raw = `{"result":{"items":[
		{"shiftVO":{"id":957395083,"name":"默认班次"}},
		{"shiftVO":{"id":957395084,"name":"早班"}}
	]}}`
	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	got := searchClassProject(data)
	if len(got) != 2 {
		t.Fatalf("上下层数据不一致: 底层 2 个班次，投影返回 %d 个 (%v)", len(got), got)
	}
	if got[0]["name"] == nil || got[0]["classId"] == nil {
		t.Fatalf("shiftVO fields not unwrapped: %v", got[0])
	}
}

// TestSearchRuleProjectEntityVOShape guards get_overtime_rule (result.atRuleList)
// and get_adjustment_rule (result.adjustmentList), both wrapping the rule under
// entityVO. The resolver must probe those keys and unwrap entityVO or
// +search-overtime-rule / +search-adjustment-rule silently return empty.
func TestSearchRuleProjectEntityVOShape(t *testing.T) {
	for _, tc := range []struct {
		name, key string
	}{
		{"overtime", "atRuleList"},
		{"adjustment", "adjustmentList"},
	} {
		raw := `{"result":{"` + tc.key + `":[
			{"entityVO":{"id":11,"name":"工作日加班"},"permissionVO":{}},
			{"entityVO":{"id":12,"name":"节假日加班"},"permissionVO":{}}
		]}}`
		var data map[string]any
		if err := json.Unmarshal([]byte(raw), &data); err != nil {
			t.Fatalf("[%s] unmarshal fixture: %v", tc.name, err)
		}
		got := searchRuleProject(data)
		if len(got) != 2 {
			t.Fatalf("[%s] 上下层数据不一致: 底层 result.%s 有 2 条，投影返回 %d 条", tc.name, tc.key, len(got))
		}
		if got[0]["ruleId"] == nil || got[0]["name"] == nil {
			t.Fatalf("[%s] entityVO fields not unwrapped: %v", tc.name, got[0])
		}
	}
}
