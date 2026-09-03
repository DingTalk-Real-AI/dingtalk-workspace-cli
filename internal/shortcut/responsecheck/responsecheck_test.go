// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package responsecheck

import "testing"

func TestCrossPlatformCoverageRequireObjectCollection(t *testing.T) {
	tests := []struct {
		name    string
		data    map[string]any
		wantLen int
		wantErr bool
	}{
		{name: "non-empty", data: map[string]any{"success": true, "result": []any{map[string]any{"id": "one"}}}, wantLen: 1},
		{name: "content envelope", data: map[string]any{"content": map[string]any{"success": true, "result": []any{map[string]any{"id": "one"}}}}, wantLen: 1},
		{name: "explicit empty", data: map[string]any{"success": true, "result": []any{}}, wantLen: 0},
		{name: "missing reviewed path", data: map[string]any{"success": true}, wantErr: true},
		{name: "missing collection", data: map[string]any{"success": true, "result": map[string]any{}}, wantErr: true},
		{name: "wrong collection type", data: map[string]any{"success": true, "result": "bad"}, wantErr: true},
		{name: "malformed item", data: map[string]any{"success": true, "result": []any{"bad"}}, wantErr: true},
		{name: "empty item", data: map[string]any{"success": true, "result": []any{map[string]any{}}}, wantErr: true},
		{name: "missing success", data: map[string]any{"result": []any{}}, wantErr: true},
		{name: "remote failure", data: map[string]any{"success": false, "result": []any{}}, wantErr: true},
		{name: "empty response", data: map[string]any{}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := RequireObjectCollection(tc.data, "test/read", "result")
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tc.wantErr)
			}
			if err == nil && len(got) != tc.wantLen {
				t.Fatalf("len = %d, want %d", len(got), tc.wantLen)
			}
		})
	}
}

func TestCrossPlatformCoverageRequireObjectCollectionNested(t *testing.T) {
	data := map[string]any{
		"success": true,
		"result": map[string]any{
			"items": []any{map[string]any{"id": "one"}},
		},
	}
	items, err := RequireObjectCollection(data, "test/nested", "result.items")
	if err != nil {
		t.Fatalf("RequireObjectCollection: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %#v", items)
	}
}

func TestCrossPlatformCoverageRequireResultRejectsNull(t *testing.T) {
	if _, err := RequireResult(map[string]any{"success": true, "result": nil}, "test/null"); err == nil {
		t.Fatal("expected null result to fail closed")
	}
	if _, err := RequireObjectResult(map[string]any{"success": true, "result": []any{}}, "test/object"); err == nil {
		t.Fatal("expected array result to fail object validation")
	}
}

func TestCrossPlatformCoverageRequireSingleObjectResult(t *testing.T) {
	for _, data := range []map[string]any{
		{"success": true, "result": map[string]any{"id": "one"}},
		{"success": true, "result": []any{map[string]any{"id": "one"}}},
	} {
		object, err := RequireSingleObjectResult(data, "test/detail")
		if err != nil || object["id"] != "one" {
			t.Fatalf("valid detail shape rejected: object=%v err=%v", object, err)
		}
	}
	for _, data := range []map[string]any{
		{},
		{"success": true, "result": map[string]any{}},
		{"success": true, "result": []any{}},
		{"success": true, "result": []any{map[string]any{"id": "one"}, map[string]any{"id": "two"}}},
		{"success": true, "result": []any{"bad"}},
		{"success": true, "result": "bad"},
	} {
		if object, err := RequireSingleObjectResult(data, "test/detail"); err == nil {
			t.Fatalf("ambiguous/malformed detail returned success: %v", object)
		}
	}
}

func TestCrossPlatformCoverageResponseEnvelopeFailures(t *testing.T) {
	for _, tc := range []struct {
		name string
		data map[string]any
	}{
		{name: "missing success", data: map[string]any{"result": map[string]any{}}},
		{name: "malformed success", data: map[string]any{"success": "true"}},
		{name: "failure with explicit message", data: map[string]any{"success": false, "errorMsg": " reviewed failure "}},
		{name: "failure without message", data: map[string]any{"success": false, "error": 42}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := RequireSuccess(tc.data, "test/envelope"); err == nil {
				t.Fatal("expected fail-closed envelope validation")
			}
		})
	}

	if _, err := RequireResult(map[string]any{}, "test/result"); err == nil {
		t.Fatal("expected upstream envelope failure")
	}
	if _, err := RequireResult(map[string]any{"success": true}, "test/result"); err == nil {
		t.Fatal("expected missing result failure")
	}
	if object, err := RequireObjectResult(map[string]any{"success": true, "result": map[string]any{"id": "one"}}, "test/object"); err != nil || object["id"] != "one" {
		t.Fatalf("valid object result rejected: object=%v err=%v", object, err)
	}
	if _, err := RequireObjectResult(map[string]any{}, "test/object"); err == nil {
		t.Fatal("expected upstream object envelope failure")
	}
}

func TestCrossPlatformCoverageLookupObject(t *testing.T) {
	data := map[string]any{
		"result": map[string]any{
			"detail": map[string]any{"id": "one"},
			"scalar": "bad",
		},
	}
	object, err := LookupObject(data, "test/lookup", "result.detail")
	if err != nil || object["id"] != "one" {
		t.Fatalf("valid nested object rejected: object=%v err=%v", object, err)
	}
	if _, err := LookupObject(data, "test/lookup", "result.missing"); err == nil {
		t.Fatal("expected missing nested object failure")
	}
	if _, err := LookupObject(data, "test/lookup", "result.scalar"); err == nil {
		t.Fatal("expected malformed nested object failure")
	}
	if _, present := lookup(map[string]any{"result": "scalar"}, "result.detail"); present {
		t.Fatal("scalar parent must not satisfy a nested lookup")
	}
}

func TestCrossPlatformCoverageCollectionPathFallbackAndHelpers(t *testing.T) {
	items, err := RequireObjectCollection(
		map[string]any{"success": true, "result": map[string]any{"items": []any{map[string]any{"id": "one"}}}},
		"test/fallback",
		"result.records",
		"result.items",
	)
	if err != nil || len(items) != 1 {
		t.Fatalf("reviewed path fallback failed: items=%v err=%v", items, err)
	}
	if _, err := RequireObjectCollection(map[string]any{}, "test/fallback", "result"); err == nil {
		t.Fatal("expected collection envelope failure")
	}
	if got := firstString(map[string]any{"first": 7, "second": "   ", "third": " value "}, "first", "second", "third"); got != "value" {
		t.Fatalf("firstString = %q, want value", got)
	}
	if got := firstString(map[string]any{"only": "   "}, "only"); got != "" {
		t.Fatalf("firstString whitespace = %q, want empty", got)
	}
}
