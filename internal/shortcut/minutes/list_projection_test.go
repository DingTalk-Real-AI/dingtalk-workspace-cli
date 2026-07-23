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

package minutes

import (
	"encoding/json"
	"testing"
)

// TestCallListProjectItemListShape guards against projection-data-loss end to
// end: list_by_keyword_and_time_range nests the minutes under result.itemList,
// so the resolver must probe "itemList"; and each projected row must carry a
// usable id. The real backend item uses taskUuid, but the API also emits
// minutesId — both must map to the projected taskUuid or callers get rows with
// a title but no id to act on.
func TestCallListProjectItemListShape(t *testing.T) {
	const raw = `{"result":{"itemList":[
		{"taskUuid":"7632756964333736","title":"周会纪要"},
		{"minutesId":"m2","title":"评审纪要"}
	]}}`
	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	got := callListProject(data)
	if len(got) != 2 {
		t.Fatalf("上下层数据不一致: 底层 result.itemList 有 2 条，投影返回 %d 条", len(got))
	}
	if got[0]["taskUuid"] != "7632756964333736" {
		t.Fatalf("taskUuid-shaped item lost its id: %v", got[0])
	}
	if got[1]["taskUuid"] != "m2" {
		t.Fatalf("minutesId-shaped item did not map to taskUuid: %v", got[1])
	}
}
