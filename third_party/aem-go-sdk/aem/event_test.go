package aem

import (
	"testing"
)

func TestEvent_ToMap_FiltersEmptyValues(t *testing.T) {
	e := Event{
		Type: "api",
		Fields: map[string]string{
			"url":     "/api/user",
			"status":  "200",
			"empty":   "",
			"another": "value",
		},
	}
	m := e.toMap()

	if m["type"] != "api" {
		t.Errorf("type = %q, want %q", m["type"], "api")
	}
	if m["url"] != "/api/user" {
		t.Errorf("url = %q, want %q", m["url"], "/api/user")
	}
	if _, ok := m["empty"]; ok {
		t.Error("empty value should be filtered out")
	}
}

func TestEvent_ToMap_OmitsTypeWhenEmpty(t *testing.T) {
	e := Event{Fields: map[string]string{"x": "1"}}
	m := e.toMap()

	if _, ok := m["type"]; ok {
		t.Error("type should be omitted when Event.Type is empty")
	}
}

func TestEvent_ToMap_DoesNotMutateOriginal(t *testing.T) {
	original := map[string]string{"a": "1", "b": ""}
	e := Event{Type: "test", Fields: original}
	_ = e.toMap()

	// 验证 toMap 没有修改原 map
	if _, ok := original["b"]; !ok {
		t.Error("toMap mutated original Fields map (b was deleted)")
	}
}
