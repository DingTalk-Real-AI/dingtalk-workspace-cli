// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitableprotocol

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestCrossPlatformCoverageTemplateErrorsCannotProducePartialConfiguration(t *testing.T) {
	if _, err := ProjectChartTemplates(`{`, []string{"LINE"}); err == nil {
		t.Fatal("malformed template accepted")
	}
	if _, err := ParseJSONC("{\"items\":[1 \t, ]}"); err != nil {
		t.Fatal(err)
	}
	resources := map[string]map[string]any{"table": {"id": "", "scalar": 1}}
	for _, value := range []any{
		map[string]any{"$ref": "table.scalar.id"}, map[string]any{"$ref": "table.id"},
		map[string]any{"nested": map[string]any{"$ref": "missing.id"}},
		[]any{map[string]any{"$ref": "missing.id"}},
	} {
		if _, err := BindReferences(value, resources); err == nil {
			t.Fatalf("invalid reference accepted: %#v", value)
		}
	}
	for _, tc := range []struct{ template, parameters map[string]any }{
		{},
		{template: map[string]any{"chartType": "LINE", "unsupported": make(chan int)}},
		{template: map[string]any{"chartType": "LINE", "value": json.Number("1e1000")}},
		{template: map[string]any{}},
		{template: map[string]any{"chartType": "LINE"}, parameters: map[string]any{"chartType": "PIE"}},
		{template: map[string]any{"chartType": "LINE"}, parameters: map[string]any{"schemaVersion": 1}},
	} {
		if _, err := ExpandChartTemplate(tc.template, tc.parameters); err == nil {
			t.Fatalf("invalid expansion accepted: %#v", tc)
		}
	}
	template := map[string]any{"chartType": "LINE", "style": map[string]any{"font": 1, "color": "red"}, "items": []any{1}}
	parameters := map[string]any{"style": map[string]any{"font": 2}, "items": []any{2, 3}, "sheet": "T"}
	got, err := ExpandChartTemplate(template, parameters)
	if err != nil || got["style"].(map[string]any)["color"] != "red" || got["style"].(map[string]any)["font"] != 2 || !reflect.DeepEqual(got["items"], []any{2, 3}) || template["style"].(map[string]any)["font"] != 1 {
		t.Fatalf("merge or original changed: got=%#v original=%#v error=%v", got, template, err)
	}
}

func TestCrossPlatformCoverageRegionCoverageRejectsInvalidCardsAndRealGaps(t *testing.T) {
	card := func(id string, x, y, w, h int) map[string]any {
		return map[string]any{"i": id, "x": x, "y": y, "w": w, "h": h}
	}
	expected := []map[string]any{card("bad", 0, 0, 2, 2)}
	if issues, err := CompareRootLayout([]map[string]any{card("bad", 0, 0, 0, 2)}, expected, 12); err != nil || len(issues) == 0 {
		t.Fatalf("invalid design missed: %v %v", issues, err)
	}
	cards := []map[string]any{card("invalid", 0, 0, 0, 4), card("left", 0, 0, 1, 4), card("late", 1, 4, 1, 2), card("gap", 1, 1, 1, 1)}
	results, err := CheckLayoutRegions(cards, []LayoutRegion{{ID: "region", X: 0, Y: 0, W: 2, H: 2}}, 12)
	if err != nil || len(results) != 1 || results[0].Covered || !reflect.DeepEqual(results[0].UncoveredColumns, []int{1}) {
		t.Fatalf("false coverage for actual gap: %#v %v", results, err)
	}
}

func TestCrossPlatformCoverageStructuredTemplateProjection(t *testing.T) {
	if _, err := ProjectStructuredChartTemplates(make(chan int), []string{"LINE"}); err == nil {
		t.Fatal("unserializable template value accepted")
	}
	templates, err := ProjectStructuredChartTemplates(map[string]any{"LINE": map[string]any{"chartType": "LINE"}}, []string{"LINE"})
	if err != nil || len(templates) != 1 {
		t.Fatalf("%#v %v", templates, err)
	}
}
