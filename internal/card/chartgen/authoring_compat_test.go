// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package chartgen

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func authoringInput(t *testing.T, preset, title string, data map[string]any) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"specVersion": "1.0", "presetId": preset, "title": title, "data": data})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestCrossPlatformCoverageAicardLabelsMatchTransportUnicodeBoundary(t *testing.T) {
	// Intentional Unicode inputs cover transport-incompatible labels, including
	// ZWJ emoji sequences; ordinary emoji and Chinese labels remain unchanged.
	for _, r := range []rune{0x200b, 0x200c, 0x200d, 0xfeff, 0x202a, 0x202e, 0x2028, 0x2029, 0x2066, 0x2069} {
		for _, field := range []string{"title", "category", "series", "unit"} {
			t.Run(fmt.Sprintf("%s/U+%04X", field, r), func(t *testing.T) {
				title, category, name, unit := "Sales", "East", "Revenue", "USD"
				value := "before" + string(r) + "after"
				switch field {
				case "title":
					title = value
				case "category":
					category = value
				case "series":
					name = value
				case "unit":
					unit = value
				}
				data := map[string]any{"kind": "category", "categories": []string{category}, "series": []map[string]any{{"name": name, "unit": unit, "values": []int{150}}}}
				artifact, err := Build(authoringInput(t, "G07", title, data), "unicode-boundary")
				if err == nil || !strings.Contains(err.Error(), "Unicode") || len(artifact.CreateMessages) != 0 {
					t.Fatalf("incompatible label produced a deliverable: err=%v artifact=%+v", err, artifact)
				}
			})
		}
	}
	for _, title := range []string{"Revenue", "区域销售额", "Revenue 📊", "Revenue ☀️"} {
		data := map[string]any{"kind": "category", "categories": []string{"华东"}, "series": []map[string]any{{"name": "销售额", "unit": "万元", "values": []int{150}}}}
		artifact, err := Build(authoringInput(t, "G07", title, data), "unicode-business-data")
		if err != nil {
			t.Fatal(title, err)
		}
		option := artifact.CreateMessages[0]["createSurface"].(map[string]any)["dataModel"].(map[string]any)["chart"].(map[string]any)
		if option["title"].(map[string]any)["text"] != title || !strings.Contains(artifact.Summary, "华东 150万元") {
			t.Fatalf("valid business text was changed: %#v %q", option, artifact.Summary)
		}
	}
}

func TestCrossPlatformCoverageAicardExpandedSeriesShowLegendWithoutChangingFacts(t *testing.T) {
	data := map[string]any{"kind": "category", "categories": []string{"East", "West"}, "series": []map[string]any{
		{"name": "Current", "values": []int{150, 120}}, {"name": "Previous", "values": []int{130, 110}}, {"name": "Target", "values": []int{180, 160}},
	}}
	artifact, err := Build(authoringInput(t, "G07", "Sales", data), "legend-facts")
	if err != nil {
		t.Fatal(err)
	}
	option := artifact.CreateMessages[0]["createSurface"].(map[string]any)["dataModel"].(map[string]any)["chart"].(map[string]any)
	if _, ok := option["legend"]; !ok {
		t.Fatal("expanded single-series preset lacks a legend")
	}
	series := option["series"].([]map[string]any)
	for i, expected := range data["series"].([]map[string]any) {
		values := expected["values"].([]int)
		if series[i]["name"] != expected["name"] || !reflect.DeepEqual(series[i]["data"], []float64{float64(values[0]), float64(values[1])}) {
			t.Fatalf("series facts changed: %#v", series[i])
		}
	}
	preset, _ := Lookup("G07")
	if preset.Legend || len(preset.SeriesTypes) != 1 {
		t.Fatalf("build mutated the shared preset: %#v", preset)
	}
}

func TestCrossPlatformCoverageAicardCategoryKindDiagnostic(t *testing.T) {
	data := map[string]any{"kind": "points", "categories": []string{"East"}, "series": []map[string]any{{"name": "Sales", "values": []int{150}}}}
	_, err := Build(authoringInput(t, "G07", "Sales", data), "kind-diagnostic")
	if err == nil || !strings.Contains(err.Error(), `got "points"`) || strings.Contains(err.Error(), "<nil>") {
		t.Fatalf("wrong kind lacks an actionable diagnostic: %v", err)
	}
}
