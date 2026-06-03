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

package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// --- SelectFields tests ---

func TestSelectFieldsTopLevel(t *testing.T) {
	payload := map[string]any{
		"id":     "123",
		"name":   "Alice",
		"email":  "alice@example.com",
		"status": "active",
	}

	tests := []struct {
		name   string
		fields []string
		want   map[string]any
	}{
		{
			name:   "select single field",
			fields: []string{"name"},
			want:   map[string]any{"name": "Alice"},
		},
		{
			name:   "select multiple fields",
			fields: []string{"id", "name"},
			want:   map[string]any{"id": "123", "name": "Alice"},
		},
		{
			name:   "case insensitive",
			fields: []string{"ID", "Name"},
			want:   map[string]any{"id": "123", "name": "Alice"},
		},
		{
			name:   "non-existent field ignored",
			fields: []string{"name", "nonexistent"},
			want:   map[string]any{"name": "Alice"},
		},
		{
			name:   "empty fields returns all",
			fields: []string{},
			want:   payload,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SelectFields(payload, tt.fields)
			gotMap, ok := got.(map[string]any)
			if !ok {
				t.Fatalf("expected map, got %T", got)
			}
			if len(gotMap) != len(tt.want) {
				t.Errorf("got %d fields, want %d", len(gotMap), len(tt.want))
			}
			for key, wantVal := range tt.want {
				if gotVal, exists := gotMap[key]; !exists || gotVal != wantVal {
					t.Errorf("field %q = %v, want %v", key, gotVal, wantVal)
				}
			}
		})
	}
}

func TestSelectFieldsArray(t *testing.T) {
	payload := []any{
		map[string]any{"id": "1", "name": "Alice", "age": 30},
		map[string]any{"id": "2", "name": "Bob", "age": 25},
	}

	got := SelectFields(payload, []string{"id", "name"})
	arr, ok := got.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", got)
	}
	if len(arr) != 2 {
		t.Fatalf("got %d items, want 2", len(arr))
	}

	for i, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("item %d: expected map, got %T", i, item)
		}
		if len(m) != 2 {
			t.Errorf("item %d: got %d fields, want 2", i, len(m))
		}
		if _, hasAge := m["age"]; hasAge {
			t.Errorf("item %d: should not have 'age' field", i)
		}
	}
}

func TestSelectFieldsNestedDataList(t *testing.T) {
	// Simulates: dws chat search --query "CLI" --fields title,memberCount
	payload := map[string]any{
		"result": map[string]any{
			"total":   float64(3),
			"hasMore": false,
			"value": []any{
				map[string]any{"title": "CLI开源", "memberCount": float64(7), "extension": nil},
				map[string]any{"title": "CLI X", "memberCount": float64(10), "extension": nil},
				map[string]any{"title": "CLI Review", "memberCount": float64(3), "extension": nil},
			},
		},
		"success": true,
	}

	got := SelectFields(payload, []string{"title", "memberCount"})
	gotMap, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", got)
	}

	// Top-level structure preserved
	if gotMap["success"] != true {
		t.Error("top-level 'success' should be preserved")
	}

	result, ok := gotMap["result"].(map[string]any)
	if !ok {
		t.Fatalf("result should be map, got %T", gotMap["result"])
	}

	value, ok := result["value"].([]any)
	if !ok {
		t.Fatalf("result.value should be array, got %T", result["value"])
	}

	if len(value) != 3 {
		t.Fatalf("expected 3 items, got %d", len(value))
	}

	first, ok := value[0].(map[string]any)
	if !ok {
		t.Fatalf("first item should be map, got %T", value[0])
	}
	if first["title"] != "CLI开源" {
		t.Errorf("title = %v, want CLI开源", first["title"])
	}
	if first["memberCount"] != float64(7) {
		t.Errorf("memberCount = %v, want 7", first["memberCount"])
	}
	if _, hasExt := first["extension"]; hasExt {
		t.Error("extension should be filtered out")
	}
}

func TestSelectFieldsTopLevelList(t *testing.T) {
	// Top-level items array
	payload := map[string]any{
		"items": []any{
			map[string]any{"id": "1", "name": "A", "extra": "x"},
			map[string]any{"id": "2", "name": "B", "extra": "y"},
		},
		"total": float64(2),
	}

	got := SelectFields(payload, []string{"id", "name"})
	gotMap := got.(map[string]any)
	items := gotMap["items"].([]any)

	for i, item := range items {
		m := item.(map[string]any)
		if _, has := m["extra"]; has {
			t.Errorf("item %d: should not have 'extra'", i)
		}
		if m["id"] == nil || m["name"] == nil {
			t.Errorf("item %d: missing id or name", i)
		}
	}
	// total preserved
	if gotMap["total"] != float64(2) {
		t.Error("total should be preserved")
	}
}

func TestSelectFieldsPrimitive(t *testing.T) {
	got := SelectFields("hello", []string{"name"})
	if got != "hello" {
		t.Errorf("primitive should pass through, got %v", got)
	}
}

// --- ApplyJQ tests ---

func TestApplyJQIdentity(t *testing.T) {
	payload := map[string]any{"name": "Alice", "age": float64(30)}
	var buf bytes.Buffer
	if err := ApplyJQ(&buf, payload, "."); err != nil {
		t.Fatalf("ApplyJQ error: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if got["name"] != "Alice" {
		t.Errorf("name = %v, want Alice", got["name"])
	}
}

func TestApplyJQFieldAccess(t *testing.T) {
	payload := map[string]any{"name": "Alice", "age": float64(30)}
	var buf bytes.Buffer
	if err := ApplyJQ(&buf, payload, ".name"); err != nil {
		t.Fatalf("ApplyJQ error: %v", err)
	}
	got := strings.TrimSpace(buf.String())
	if got != `"Alice"` {
		t.Errorf("got %q, want %q", got, `"Alice"`)
	}
}

func TestApplyJQArrayIteration(t *testing.T) {
	payload := map[string]any{
		"items": []any{
			map[string]any{"name": "Alice"},
			map[string]any{"name": "Bob"},
		},
	}
	var buf bytes.Buffer
	if err := ApplyJQ(&buf, payload, ".items[].name"); err != nil {
		t.Fatalf("ApplyJQ error: %v", err)
	}
	got := strings.TrimSpace(buf.String())
	if !strings.Contains(got, `"Alice"`) || !strings.Contains(got, `"Bob"`) {
		t.Errorf("got %q, want Alice and Bob", got)
	}
}

func TestApplyJQSelect(t *testing.T) {
	payload := map[string]any{
		"items": []any{
			map[string]any{"name": "Alice", "active": true},
			map[string]any{"name": "Bob", "active": false},
		},
	}
	var buf bytes.Buffer
	if err := ApplyJQ(&buf, payload, `.items[] | select(.active) | .name`); err != nil {
		t.Fatalf("ApplyJQ error: %v", err)
	}
	got := strings.TrimSpace(buf.String())
	if got != `"Alice"` {
		t.Errorf("got %q, want %q", got, `"Alice"`)
	}
}

func TestApplyJQInvalidExpression(t *testing.T) {
	var buf bytes.Buffer
	err := ApplyJQ(&buf, map[string]any{}, ".[invalid")
	if err == nil {
		t.Fatal("expected error for invalid expression")
	}
	if !strings.Contains(err.Error(), "--jq") {
		t.Errorf("error should mention --jq, got %q", err.Error())
	}
}

func TestApplyJQLength(t *testing.T) {
	payload := []any{1, 2, 3, 4, 5}
	var buf bytes.Buffer
	if err := ApplyJQ(&buf, payload, "length"); err != nil {
		t.Fatalf("ApplyJQ error: %v", err)
	}
	got := strings.TrimSpace(buf.String())
	if got != "5" {
		t.Errorf("got %q, want %q", got, "5")
	}
}

func TestApplyJQObjectConstruction(t *testing.T) {
	payload := map[string]any{
		"first_name": "Alice",
		"last_name":  "Smith",
		"age":        float64(30),
	}
	var buf bytes.Buffer
	if err := ApplyJQ(&buf, payload, `{name: (.first_name + " " + .last_name), age}`); err != nil {
		t.Fatalf("ApplyJQ error: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if got["name"] != "Alice Smith" {
		t.Errorf("name = %v, want 'Alice Smith'", got["name"])
	}
}

func TestApplyJQEmptyResult(t *testing.T) {
	payload := map[string]any{"name": "Alice"}
	var buf bytes.Buffer
	if err := ApplyJQ(&buf, payload, ".nonexistent"); err != nil {
		t.Fatalf("ApplyJQ error: %v", err)
	}
	got := strings.TrimSpace(buf.String())
	if got != "null" {
		t.Errorf("got %q, want %q", got, "null")
	}
}

// --- WriteFiltered integration tests ---

func TestWriteFilteredWithFields(t *testing.T) {
	// Simulates a typical API response with nested data list
	payload := map[string]any{
		"result": map[string]any{
			"value": []any{
				map[string]any{"id": "1", "name": "test", "secret": "xxx"},
				map[string]any{"id": "2", "name": "test2", "secret": "yyy"},
			},
		},
		"success": true,
	}
	var buf bytes.Buffer
	if err := WriteFiltered(&buf, FormatJSON, payload, "id,name", ""); err != nil {
		t.Fatalf("WriteFiltered error: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	result := got["result"].(map[string]any)
	items := result["value"].([]any)
	first := items[0].(map[string]any)
	if _, hasSecret := first["secret"]; hasSecret {
		t.Error("should not have 'secret' field")
	}
	if first["id"] != "1" || first["name"] != "test" {
		t.Errorf("got %v", first)
	}
}

func TestWriteFilteredWithJQ(t *testing.T) {
	payload := map[string]any{"items": []any{"a", "b", "c"}}
	var buf bytes.Buffer
	if err := WriteFiltered(&buf, FormatJSON, payload, "", ".items | length"); err != nil {
		t.Fatalf("WriteFiltered error: %v", err)
	}
	got := strings.TrimSpace(buf.String())
	if got != "3" {
		t.Errorf("got %q, want %q", got, "3")
	}
}

func TestWriteFilteredJQTakesPrecedence(t *testing.T) {
	payload := map[string]any{"id": "1", "name": "test"}
	var buf bytes.Buffer
	// Both --fields and --jq provided: jq wins.
	if err := WriteFiltered(&buf, FormatJSON, payload, "id", ".name"); err != nil {
		t.Fatalf("WriteFiltered error: %v", err)
	}
	got := strings.TrimSpace(buf.String())
	if got != `"test"` {
		t.Errorf("got %q, want %q", got, `"test"`)
	}
}

func TestWriteFilteredNoFilters(t *testing.T) {
	payload := map[string]any{"id": "1"}
	var buf bytes.Buffer
	if err := WriteFiltered(&buf, FormatJSON, payload, "", ""); err != nil {
		t.Fatalf("WriteFiltered error: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if got["id"] != "1" {
		t.Errorf("id = %v, want 1", got["id"])
	}
}
