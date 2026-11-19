package a2ui

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageChartArrayAppendReplay(t *testing.T) {
	for _, path := range []string{"/rows/-", "/rows/2"} {
		model, valid := chartSet(map[string]any{"rows": []any{1, 2}}, path, 3)
		if !valid || !reflect.DeepEqual(model["rows"], []any{1, 2, 3}) {
			t.Fatalf("append %s: %#v %v", path, model, valid)
		}
	}
	for _, path := range []string{"/rows/+2", "/rows/02", "/rows/4", "/rows/٢"} {
		model, valid := chartSet(map[string]any{"rows": []any{1, 2}}, path, 3)
		if valid || !reflect.DeepEqual(model["rows"], []any{1, 2}) {
			t.Fatalf("index %s: %#v %v", path, model, valid)
		}
	}
	model, valid := chartSet(map[string]any{"rows": []any{1, 2}}, "/rows/9", nil)
	if !valid || !reflect.DeepEqual(model["rows"], []any{1, 2}) {
		t.Fatalf("deletion: %#v %v", model, valid)
	}
}

func TestCrossPlatformCoverageChartUpdatePathDepth(t *testing.T) {
	path := strings.Repeat("/x", 2048)
	model := map[string]any{}
	updated, valid := chartSet(model, path+"/x", 1)
	if valid || len(updated) != 0 {
		t.Fatal("an oversized path must fail without creating containers")
	}
	updated, valid = chartSet(model, path, 1)
	value, found := chartGet(updated, path)
	if !valid || !found || value != 1 {
		t.Fatal("an update at the depth limit must be applied")
	}
}

func TestCrossPlatformCoverageChartCompatibilityRepair(t *testing.T) {
	p := testProtocol(t)
	raw, err := os.ReadFile("testdata/chart-compat-repair.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name                       string
		Messages                   json.RawMessage
		StrictValid, ExistingValid bool
		Status                     string
		Checked                    int
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			r, err := p.Lint(tc.Messages, false, true)
			if err != nil || !r.Valid {
				t.Fatalf("schema: %+v %v", r, err)
			}
			strict, err := p.Preflight(r.A2UIMessages, "new-card")
			if err != nil || strict["valid"] != tc.StrictValid {
				t.Fatalf("strict: %#v %v", strict, err)
			}
			chart := strict["chartValidation"].(map[string]any)
			if chart["status"] != tc.Status || chart["checked"] != tc.Checked {
				t.Fatalf("chart: %#v", chart)
			}
			existing, err := p.PreflightForExistingContent(r.A2UIMessages, "new-card")
			if err != nil || existing["valid"] != tc.ExistingValid {
				t.Fatalf("existing: %#v %v", existing, err)
			}
		})
	}
}
