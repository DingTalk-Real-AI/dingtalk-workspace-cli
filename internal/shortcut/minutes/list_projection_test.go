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
// so the resolver must probe "itemList", and each projected row must carry the
// real taskUuid that record-control commands consume. minutesId is deliberately
// not treated as a taskUuid (see callListProject).
func TestCallListProjectItemListShape(t *testing.T) {
	const raw = `{"result":{"itemList":[
		{"taskUuid":"uuid-1","title":"weekly sync"},
		{"task_uuid":"uuid-2","title":"design review"}
	]}}`
	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	got := callListProject(data)
	if len(got) != 2 {
		t.Fatalf("lower/upper mismatch: itemList has 2 entries, projection returned %d", len(got))
	}
	if got[0]["taskUuid"] != "uuid-1" || got[1]["taskUuid"] != "uuid-2" {
		t.Fatalf("taskUuid not projected from taskUuid/task_uuid keys: %v", got)
	}
}
