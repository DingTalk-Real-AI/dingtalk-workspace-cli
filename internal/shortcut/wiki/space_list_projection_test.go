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

package wiki

import (
	"encoding/json"
	"testing"
)

// TestCrossPlatformCoverageWikiSpaceListShape guards against projection-data-loss:
// list_wikiSpaces / search_wikiSpaces nest the list under result.wikiSpaces;
// the resolver must probe "wikiSpaces" or +space-list / +space-search silently
// return empty despite the backend returning spaces.
func TestCrossPlatformCoverageWikiSpaceListShape(t *testing.T) {
	const raw = `{"result":{"hasMore":false,"wikiSpaces":[
		{"workspaceId":"w1","name":"R&D wiki"},
		{"workspaceId":"w2","name":"product wiki"}
	]},"success":true}`
	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	items, _, err := requireWikiCollection(data, "wiki/list_wikiSpaces", "wikiSpaces")
	if err != nil {
		t.Fatal(err)
	}
	spaces := projectWikiRows(items, map[string][]string{"workspaceId": {"workspaceId"}, "name": {"name"}})
	if len(spaces) != 2 {
		t.Fatalf("lower/upper mismatch: result.wikiSpaces has 2 entries, projection returned %d (%v)", len(spaces), spaces)
	}
}

// TestSpaceListProjectTopLevelWikiSpaces covers the already-unwrapped shape.
func TestCrossPlatformCoverageWikiSpaceListTopLevelShape(t *testing.T) {
	const raw = `{"wikiSpaces":[{"workspaceId":"w1","name":"R&D wiki"}]}`
	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	items, _, err := requireWikiCollection(data, "wiki/list_wikiSpaces", "wikiSpaces")
	if err != nil {
		t.Fatal(err)
	}
	spaces := projectWikiRows(items, map[string][]string{"workspaceId": {"workspaceId"}, "name": {"name"}})
	if len(spaces) != 1 {
		t.Fatalf("top-level wikiSpaces: want 1, got %d (%v)", len(spaces), spaces)
	}
}

func TestCrossPlatformCoverageWikiCollectionsRejectFalseEmptySuccess(t *testing.T) {
	for name, data := range map[string]map[string]any{
		"missing":   {"success": true, "hasMore": false},
		"malformed": {"success": true, "wikiSpaces": map[string]any{}},
		"bad item":  {"success": true, "wikiSpaces": []any{"not-an-object"}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := requireWikiCollection(data, "wiki/list_wikiSpaces", "wikiSpaces"); err == nil {
				t.Fatal("malformed response was accepted as an empty success")
			}
		})
	}
	items, _, err := requireWikiCollection(map[string]any{"success": true, "wikiSpaces": []any{}}, "wiki/list_wikiSpaces", "wikiSpaces")
	if err != nil || len(items) != 0 {
		t.Fatalf("explicit empty array must remain valid: items=%v err=%v", items, err)
	}
}
