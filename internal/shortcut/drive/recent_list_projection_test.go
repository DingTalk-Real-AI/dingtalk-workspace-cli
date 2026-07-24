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

package drive

import (
	"encoding/json"
	"testing"
)

// TestRecentListProjectResultWrapper guards against projection-data-loss:
// get_recent_list nests its payload under result.recentItems; the projection
// must descend into result or +recent silently returns empty despite backend
// records.
func TestRecentListProjectResultWrapper(t *testing.T) {
	const raw = `{"result":{"hasMore":true,"nextCursor":"c2","recentItems":[
		{"name":"weekly report","nodeType":"doc","nodeId":"n1","docUrl":"https://x/1"},
		{"name":"budget sheet","nodeType":"sheet","nodeId":"n2","docUrl":"https://x/2"}
	]}}`
	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	out := recentListProject(data)
	if got, _ := out["count"].(int); got != 2 {
		t.Fatalf("lower/upper mismatch: result.recentItems has 2 entries, projection count=%v (%v)", out["count"], out)
	}
	if out["hasMore"] != true || out["nextCursor"] != "c2" {
		t.Fatalf("pagination fields lost from result wrapper: %v", out)
	}
}

// TestRecentListProjectNoItems covers the no-recentItems branch (payload not
// found under any wrapper), which must yield an empty list, not a panic.
func TestRecentListProjectNoItems(t *testing.T) {
	const raw = `{"result":{"totalCount":0},"success":true}`
	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	if out := recentListProject(data); out["count"].(int) != 0 {
		t.Fatalf("no recentItems: want count 0, got %v", out["count"])
	}
}

// TestRecentListProjectTopLevel covers the already-unwrapped shape.
func TestRecentListProjectTopLevel(t *testing.T) {
	const raw = `{"recentItems":[{"name":"weekly report","nodeId":"n1"}],"hasMore":false}`
	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	if out := recentListProject(data); out["count"].(int) != 1 {
		t.Fatalf("top-level recentItems: want count 1, got %v", out["count"])
	}
}
