// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package a2ui

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
)

func chartTestOption(kind string, points []any) map[string]any {
	option := map[string]any{"series": []any{map[string]any{"type": kind, "data": points}}}
	switch kind {
	case "bar", "line":
		option["xAxis"] = map[string]any{"type": "category", "data": []any{"A"}}
		option["yAxis"] = map[string]any{"type": "value"}
	case "scatter", "effectScatter":
		option["xAxis"] = map[string]any{"type": "value"}
		option["yAxis"] = map[string]any{"type": "value"}
	case "heatmap":
		option["xAxis"] = map[string]any{"type": "category", "data": []any{"A"}}
		option["yAxis"] = map[string]any{"type": "category", "data": []any{"B"}}
	case "candlestick":
		option["xAxis"] = map[string]any{"type": "category", "data": []any{"A"}}
		option["yAxis"] = map[string]any{"type": "value"}
	case "radar":
		option["radar"] = map[string]any{"indicator": []any{
			map[string]any{"name": "A", "max": 10.0},
			map[string]any{"name": "B", "max": 10.0},
			map[string]any{"name": "C", "max": 10.0},
		}}
	case "gauge":
		option["series"] = []any{map[string]any{"type": "gauge", "min": 0.0, "max": 10.0, "data": points}}
	case "sankey":
		option["series"] = []any{map[string]any{"type": "sankey", "data": points, "links": []any{map[string]any{"source": "A", "target": "B", "value": 1.0}}}}
	}
	return option
}

func chartTestCode(t *testing.T, option map[string]any, want string) {
	t.Helper()
	if code, _ := chartOptionError(option); code != want {
		t.Fatalf("chartOptionError() code = %q, want %q; option=%#v", code, want, option)
	}
}

func chartTestStaticComponents(component map[string]any) []any {
	return []any{
		map[string]any{"id": "root", "component": "Column", "children": []any{component["id"]}},
		component,
	}
}
func TestCrossPlatformCoverageLocalChartSafetyBoundaries(t *testing.T) {
	cases := []struct {
		name   string
		option map[string]any
		code   string
	}{
		{"axis-id", map[string]any{"xAxis": map[string]any{"gridId": "other"}}, "chart.local_coordinates"},
		{"axis-index-type", map[string]any{"xAxis": map[string]any{"gridIndex": true}}, "chart.local_coordinates"},
		{"axis-index-nonzero", map[string]any{"xAxis": map[string]any{"gridIndex": 1.0}}, "chart.local_coordinates"},
		{"grid-type", map[string]any{"grid": []any{}}, "chart.local_coordinates"},
		{"split-range", map[string]any{"xAxis": map[string]any{"splitNumber": 101.0}}, "chart.local_coordinates"},
		{"split-fraction", map[string]any{"xAxis": map[string]any{"splitNumber": 1.5}}, "chart.local_coordinates"},
		{"series-encode", map[string]any{"series": []any{map[string]any{"encode": map[string]any{}}}}, "chart.local_coordinates"},
		{"series-reference", map[string]any{"series": []any{map[string]any{"xAxisIndex": 2.0}}}, "chart.local_coordinates"},
		{"coordinate-mismatch", map[string]any{"series": []any{map[string]any{"type": "line", "coordinateSystem": "polar"}}}, "chart.local_coordinates"},
		{"radar-coordinate", map[string]any{"series": []any{map[string]any{"type": "radar", "coordinateSystem": "radar"}}}, ""},
		{"numeric-style", map[string]any{"width": json.Number("16384")}, ""},
		{"float-style", map[string]any{"width": 16384.0}, ""},
		{"float-style-overflow", map[string]any{"width": -16385.0}, "chart.local_values"},
		{"reserved-field", map[string]any{"nested": map[string]any{"_chartInternal": true}}, "chart.local_values"},
		{"numeric-style-overflow", map[string]any{"width": json.Number("16385")}, "chart.local_values"},
		{"numeric-style-invalid", map[string]any{"width": json.Number("invalid")}, "chart.local_values"},
		{"legend-type", map[string]any{"legend": true}, "chart.local_options"},
		{"show-type", map[string]any{"tooltip": map[string]any{"show": "yes"}}, "chart.local_options"},
		{"show-valid", map[string]any{"tooltip": map[string]any{"show": true}}, ""},
		{"selection-boolean", map[string]any{"legend": map[string]any{"selectedMode": false}}, ""},
		{"selection-single", map[string]any{"legend": map[string]any{"selectedMode": "single"}}, ""},
		{"selection-multiple", map[string]any{"legend": map[string]any{"selectedMode": "multiple"}}, ""},
		{"selection-invalid", map[string]any{"legend": map[string]any{"selectedMode": "unknown"}}, "chart.local_options"},
		{"selected-valid", map[string]any{"legend": map[string]any{"selected": map[string]any{"A": true, "B": false}}}, ""},
		{"selected-invalid", map[string]any{"legend": map[string]any{"selected": map[string]any{"A": "yes"}}}, "chart.local_options"},
		{"selected-type", map[string]any{"legend": map[string]any{"selected": false}}, "chart.local_options"},
		{"option-unknown", map[string]any{"tooltip": map[string]any{"unknown": true}}, "chart.local_options"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if code, _ := chartLocalError(tc.option); code != tc.code {
				t.Fatalf("code=%q want=%q", code, tc.code)
			}
		})
	}
	var deep any = "label"
	for i := 0; i < 34; i++ {
		deep = []any{deep}
	}
	if code, _ := chartLocalError(map[string]any{"nested": deep}); code != "chart.local_values" {
		t.Fatalf("depth limit: %q", code)
	}
	for _, kind := range []string{"pie", "line"} {
		series := []any{}
		for i := 0; i < 257; i++ {
			row := map[string]any{"type": kind, "name": fmt.Sprintf("Series %d", i)}
			if kind == "pie" {
				row["data"] = []any{map[string]any{"name": fmt.Sprintf("Item %d", i)}, map[string]any{"name": ""}}
			}
			series = append(series, row)
		}
		if code, _ := chartLocalError(map[string]any{"legend": map[string]any{"show": true}, "series": series}); code != "chart.local_options" {
			t.Fatalf("%s legend limit: %q", kind, code)
		}
	}
}

func TestCrossPlatformCoverageRadarAndLegacySafety(t *testing.T) {
	groups := make([]any, 668)
	for i := range groups {
		groups[i] = map[string]any{"value": []any{1.0, 2.0, 3.0}}
	}
	chartTestCode(t, chartTestOption("radar", groups), "chart.series_data_invalid")
	option := chartTestOption("radar", []any{map[string]any{"value": []any{1.0, 2.0, 3.0}}})
	object(array(object(option["radar"])["indicator"])[0])["name"] = 1.0
	chartTestCode(t, option, "chart.value_invalid")
	p := testProtocol(t)
	p.chartLegacySchema = nil
	docs := []map[string]any{{"createSurface": map[string]any{"surfaceId": "legacy", "components": chartTestStaticComponents(map[string]any{"id": "chart", "component": "Chart", "data": map[string]any{"type": "bar", "data": []any{1.0}}})}}}
	diagnostics, status := p.checkChartBindings(docs, true)
	if len(diagnostics) != 1 || diagnostics[0].Code != "chart.legacy_structure_unavailable" || status["valid"] != false {
		t.Fatalf("legacy schema gap: %+v %+v", diagnostics, status)
	}
}

func TestCrossPlatformCoverageCandlestickValueAxis(t *testing.T) {
	for _, tc := range []struct {
		name  string
		axis  any
		valid bool
	}{
		{"missing", nil, false},
		{"string", "value", false},
		{"array", []any{map[string]any{"type": "value"}}, false},
		{"category", map[string]any{"type": "category", "data": []any{"A"}}, false},
		{"null_type", map[string]any{"type": nil}, false},
		{"value", map[string]any{"type": "value"}, true},
		{"renderer_default", map[string]any{}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			option := chartTestOption("candlestick", []any{[]any{2.0, 3.0, 1.0, 4.0}})
			delete(option, "yAxis")
			if tc.axis != nil {
				option["yAxis"] = tc.axis
			}
			want := "chart.axis_missing"
			if tc.valid {
				want = ""
			}
			chartTestCode(t, option, want)
		})
	}
}

func TestCrossPlatformCoverageChartSemanticPrimitiveBoundaries(t *testing.T) {
	if got, ok := chartTokens("/"); !ok || len(got) != 0 {
		t.Fatalf("root tokens = %#v, %v", got, ok)
	}
	if got, ok := chartTokens("/a~1b/~0c"); !ok || len(got) != 2 || got[0] != "a/b" || got[1] != "~c" {
		t.Fatalf("escaped tokens = %#v, %v", got, ok)
	}
	for _, path := range []string{"bad", "/bad~", "/bad~2"} {
		if _, ok := chartTokens(path); ok {
			t.Fatalf("accepted invalid token path %q", path)
		}
	}
	model := map[string]any{"object": map[string]any{"value": 3.0}, "array": []any{"zero"}, "scalar": true}
	if got, ok := chartGet(model, "/object/value"); !ok || got != 3.0 {
		t.Fatalf("object get = %#v, %v", got, ok)
	}
	if got, ok := chartGet(model, "/array/0"); !ok || got != "zero" {
		t.Fatalf("array get = %#v, %v", got, ok)
	}
	for _, path := range []string{"bad", "/missing", "/array/x", "/array/-1", "/array/1", "/scalar/x"} {
		if _, ok := chartGet(model, path); ok {
			t.Fatalf("chartGet accepted %q", path)
		}
	}
	if got, ok := chartSet(model, "bad", 1); ok || got["object"] == nil {
		t.Fatal("invalid set changed model")
	}
	if got, ok := chartSet(nil, "/a/b", 2); !ok || got["a"].(map[string]any)["b"] != 2 {
		t.Fatalf("nested set = %#v", got)
	}
	model, _ = chartSet(model, "/object/value", nil)
	if _, ok := model["object"].(map[string]any)["value"]; ok {
		t.Fatal("delete did not remove value")
	}
	root := map[string]any{"replacement": true}
	if got, ok := chartSet(model, "/", root); !ok || got["replacement"] != true {
		t.Fatalf("root set = %#v", got)
	}
	if got, ok := chartSet(model, "/", "not an object"); ok || got == nil {
		t.Fatalf("non-object root set = %#v, %v", got, ok)
	}
	arrayModel := map[string]any{"chart": map[string]any{"series": []any{map[string]any{"data": []any{1.0}}}}}
	arrayModel, ok := chartSet(arrayModel, "/chart/series/0/data/0", 2.0)
	if !ok || arrayModel["chart"].(map[string]any)["series"].([]any)[0].(map[string]any)["data"].([]any)[0] != 2.0 {
		t.Fatalf("array set = %#v, %v", arrayModel, ok)
	}
	if got, ok := chartSetValue(model, nil, root); !ok || got.(map[string]any)["replacement"] != true {
		t.Fatalf("direct root set = %#v, %v", got, ok)
	}
	arrayModel, ok = chartSet(arrayModel, "/chart/series/0/data/0", nil)
	if !ok || len(arrayModel["chart"].(map[string]any)["series"].([]any)[0].(map[string]any)["data"].([]any)) != 0 {
		t.Fatalf("array delete = %#v, %v", arrayModel, ok)
	}
	if _, ok := chartSet(arrayModel, "/chart/series/9/data/0", 3.0); ok {
		t.Fatal("out-of-range array update was accepted")
	}
	if _, ok := chartSetValue([]any{true}, []string{"0", "child"}, 1.0); ok {
		t.Fatal("nested scalar array update was accepted")
	}
	if _, ok := chartSetValue(true, []string{"child"}, 1.0); ok {
		t.Fatal("scalar update was accepted")
	}

	for _, value := range []any{json.Number("1.5"), 2.0} {
		if _, ok := chartValue(value); !ok {
			t.Fatalf("rejected number %#v", value)
		}
	}
	for _, value := range []any{"1", math.NaN(), math.Inf(1), 9007199254740992.0, json.Number("bad")} {
		if _, ok := chartValue(value); ok {
			t.Fatalf("accepted invalid number %#v", value)
		}
	}
	if chartNumbers(nil) || chartNumbers(make([]any, 2001)) || chartNumbers([]any{1.0, "bad"}) || !chartNumbers([]any{1.0, json.Number("2")}) {
		t.Fatal("chartNumbers boundary mismatch")
	}
	option := map[string]any{"xAxis": map[string]any{"type": "category", "data": []any{"A"}}, "yAxis": map[string]any{"type": "value"}}
	if len(chartAxis(option, "xAxis", "category")) != 1 || chartAxis(option, "yAxis", "category") != nil || chartAxis(option, "yAxis", "value") == nil {
		t.Fatal("chartAxis boundary mismatch")
	}
}

func TestCrossPlatformCoverageChartGraphBoundaries(t *testing.T) {
	nodes := map[string]bool{"A": true, "B": true}
	cases := []struct {
		name  string
		nodes map[string]bool
		links []any
		code  string
	}{
		{"unknown", nodes, []any{map[string]any{"source": "A", "target": "C", "value": 1.0}}, "chart.endpoint_invalid"},
		{"self", nodes, []any{map[string]any{"source": "A", "target": "A", "value": 1.0}}, "chart.endpoint_invalid"},
		{"bad value", nodes, []any{map[string]any{"source": "A", "target": "B", "value": 0.0}}, "chart.endpoint_invalid"},
		{"range", map[string]bool{"A": true, "B": true, "C": true}, []any{map[string]any{"source": "A", "target": "B", "value": 1.0}, map[string]any{"source": "B", "target": "C", "value": 1e-13}}, "chart.range_invalid"},
		{"isolated", map[string]bool{"A": true, "B": true, "C": true}, []any{map[string]any{"source": "A", "target": "B", "value": 1.0}}, "chart.endpoint_invalid"},
		{"cycle", nodes, []any{map[string]any{"source": "A", "target": "B", "value": 1.0}, map[string]any{"source": "B", "target": "A", "value": 1.0}}, "chart.endpoint_invalid"},
		{"valid", nodes, []any{map[string]any{"source": "A", "target": "B", "value": 1.0}}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if code, _ := chartGraphError(tc.nodes, tc.links); code != tc.code {
				t.Fatalf("code = %q, want %q", code, tc.code)
			}
		})
	}
}

func TestCrossPlatformCoverageChartOptionErrorBoundaries(t *testing.T) {
	valid := []map[string]any{
		chartTestOption("bar", []any{1.0}),
		chartTestOption("line", []any{1.0}),
		chartTestOption("scatter", []any{[]any{1.0, 2.0}}),
		chartTestOption("effectScatter", []any{[]any{1.0, 2.0}}),
		chartTestOption("pie", []any{map[string]any{"name": "A", "value": 1.0}}),
		chartTestOption("funnel", []any{map[string]any{"name": "A", "value": 1.0}}),
		chartTestOption("heatmap", []any{[]any{0.0, 0.0, 1.0}}),
		chartTestOption("candlestick", []any{[]any{2.0, 3.0, 1.0, 4.0}}),
		chartTestOption("radar", []any{map[string]any{"value": []any{1.0, 2.0, 3.0}}}),
		chartTestOption("gauge", []any{map[string]any{"value": 5.0}}),
		chartTestOption("sankey", []any{map[string]any{"name": "A"}, map[string]any{"name": "B"}}),
	}
	for i, option := range valid {
		if code, message := chartOptionError(option); code != "" {
			t.Fatalf("valid option %d: %s %s", i, code, message)
		}
	}
	mixed := chartTestOption("bar", []any{1.0})
	mixed["series"] = append(mixed["series"].([]any), map[string]any{"type": "line", "data": []any{2.0}})
	chartTestCode(t, mixed, "")

	chartTestCode(t, map[string]any{"oversize": strings.Repeat("x", 257*1024), "series": []any{map[string]any{"type": "bar", "data": []any{1.0}}}}, "chart.option_size")
	chartTestCode(t, map[string]any{"series": []any{}}, "chart.series_count")
	manySeries := make([]any, 33)
	for i := range manySeries {
		manySeries[i] = map[string]any{"type": "bar", "data": []any{1.0}}
	}
	chartTestCode(t, map[string]any{"series": manySeries}, "chart.series_count")
	// A registered client template may own an unknown type; malformed names remain invalid.
	chartTestCode(t, map[string]any{"series": []any{map[string]any{"type": "unknown", "data": []any{1.0}}}}, "")
	chartTestCode(t, map[string]any{"series": []any{map[string]any{"type": "invalid-type", "data": []any{1.0}}}}, "chart.series_type_unknown")
	chartTestCode(t, map[string]any{"series": []any{map[string]any{"type": "bar", "data": []any{}}}}, "chart.series_data_invalid")
	tooMany := make([]any, 2001)
	chartTestCode(t, map[string]any{"series": []any{map[string]any{"type": "bar", "data": tooMany}}}, "chart.series_data_invalid")
	twoThousand := make([]any, 1001)
	for i := range twoThousand {
		twoThousand[i] = []any{1.0, 2.0}
	}
	chartTestCode(t, map[string]any{
		"xAxis": map[string]any{"type": "value"},
		"yAxis": map[string]any{"type": "value"},
		"series": []any{
			map[string]any{"type": "scatter", "data": twoThousand},
			map[string]any{"type": "effectScatter", "data": twoThousand},
		},
	}, "chart.series_data_invalid")

	bar := chartTestOption("bar", []any{1.0})
	delete(bar, "xAxis")
	chartTestCode(t, bar, "chart.axis_missing")
	for _, labels := range [][]any{{}, make([]any, 81)} {
		bar = chartTestOption("bar", []any{1.0})
		bar["xAxis"] = map[string]any{"type": "category", "data": labels}
		chartTestCode(t, bar, "chart.axis_missing")
	}
	bar = chartTestOption("bar", []any{1.0, 2.0})
	chartTestCode(t, bar, "chart.series_length")
	bar = chartTestOption("bar", []any{"bad"})
	chartTestCode(t, bar, "chart.value_invalid")

	scatter := chartTestOption("scatter", []any{[]any{1.0, 2.0}})
	delete(scatter, "yAxis")
	chartTestCode(t, scatter, "chart.axis_missing")
	chartTestCode(t, chartTestOption("scatter", []any{[]any{1.0}}), "chart.value_invalid")
	chartTestCode(t, chartTestOption("scatter", []any{[]any{1.0, "bad"}}), "chart.value_invalid")

	chartTestCode(t, chartTestOption("pie", make([]any, 25)), "chart.series_data_invalid")
	chartTestCode(t, chartTestOption("funnel", make([]any, 13)), "chart.series_data_invalid")
	chartTestCode(t, chartTestOption("pie", []any{map[string]any{"name": "", "value": 1.0}}), "chart.value_invalid")
	chartTestCode(t, chartTestOption("pie", []any{map[string]any{"name": "A", "value": -1.0}}), "chart.value_invalid")
	chartTestCode(t, chartTestOption("pie", []any{map[string]any{"name": "A", "value": 0.0}}), "chart.value_invalid")
	chartTestCode(t, chartTestOption("pie", []any{map[string]any{"name": "A", "value": 2.0}, map[string]any{"name": "A", "value": 1.0}}), "chart.value_invalid")
	chartTestCode(t, chartTestOption("funnel", []any{map[string]any{"name": "A", "value": 1.0}, map[string]any{"name": "B", "value": 2.0}}), "chart.series_order")

	heat := chartTestOption("heatmap", []any{[]any{0.0, 0.0, 1.0}})
	delete(heat, "xAxis")
	chartTestCode(t, heat, "chart.axis_missing")
	heat = chartTestOption("heatmap", []any{[]any{0.0, 0.0, 1.0}})
	heat["xAxis"] = map[string]any{"type": "category", "data": []any{}}
	chartTestCode(t, heat, "chart.axis_missing")
	chartTestCode(t, chartTestOption("heatmap", []any{[]any{0.0, 0.0}}), "chart.value_invalid")
	for _, point := range []any{[]any{-1.0, 0.0, 1.0}, []any{0.5, 0.0, 1.0}, []any{1.0, 0.0, 1.0}, []any{0.0, 0.0, "bad"}} {
		chartTestCode(t, chartTestOption("heatmap", []any{point}), "chart.range_invalid")
	}

	candle := chartTestOption("candlestick", []any{[]any{2.0, 3.0, 1.0, 4.0}})
	delete(candle, "xAxis")
	chartTestCode(t, candle, "chart.axis_missing")
	candle = chartTestOption("candlestick", []any{[]any{2.0, 3.0, 1.0, 4.0}})
	candle["xAxis"] = map[string]any{"type": "category", "data": []any{}}
	chartTestCode(t, candle, "chart.axis_missing")
	candle = chartTestOption("candlestick", []any{[]any{2.0, 3.0, 1.0, 4.0}, []any{2.0, 3.0, 1.0, 4.0}})
	chartTestCode(t, candle, "chart.series_length")
	chartTestCode(t, chartTestOption("candlestick", []any{[]any{1.0, 2.0, 0.0}}), "chart.value_invalid")
	chartTestCode(t, chartTestOption("candlestick", []any{[]any{2.0, 3.0, 2.5, 4.0}}), "chart.range_invalid")

	radar := chartTestOption("radar", []any{map[string]any{"value": []any{1.0, 2.0, 3.0}}})
	radar["radar"] = map[string]any{"indicator": []any{map[string]any{"max": 10.0}}}
	chartTestCode(t, radar, "chart.axis_missing")
	chartTestCode(t, chartTestOption("radar", []any{map[string]any{"value": []any{1.0}}}), "chart.series_length")
	radar = chartTestOption("radar", []any{map[string]any{"value": []any{1.0, 2.0, 3.0}}})
	radar["radar"].(map[string]any)["indicator"].([]any)[0] = map[string]any{"name": "A", "max": 0.0}
	chartTestCode(t, radar, "chart.range_invalid")
	radar = chartTestOption("radar", []any{map[string]any{"value": []any{11.0, 2.0, 3.0}}})
	chartTestCode(t, radar, "chart.range_invalid")

	gauge := chartTestOption("gauge", []any{map[string]any{"value": 5.0}})
	gauge["series"].([]any)[0].(map[string]any)["min"] = 1.0
	chartTestCode(t, gauge, "chart.range_invalid")
	chartTestCode(t, chartTestOption("gauge", []any{map[string]any{"value": 1.0}, map[string]any{"value": 2.0}}), "")
	chartTestCode(t, chartTestOption("gauge", []any{
		map[string]any{"value": 1.0}, map[string]any{"value": 2.0}, map[string]any{"value": 3.0},
		map[string]any{"value": 4.0}, map[string]any{"value": 5.0}, map[string]any{"value": 6.0},
	}), "chart.series_count")
	chartTestCode(t, chartTestOption("gauge", []any{map[string]any{"value": 11.0}}), "chart.range_invalid")

	nodes65 := make([]any, 65)
	for i := range nodes65 {
		nodes65[i] = map[string]any{"name": string(rune('A' + i))}
	}
	chartTestCode(t, chartTestOption("sankey", nodes65), "chart.series_data_invalid")
	chartTestCode(t, chartTestOption("sankey", []any{map[string]any{"name": "A"}, map[string]any{"name": "A"}}), "chart.value_invalid")
	sankey := chartTestOption("sankey", []any{map[string]any{"name": "A"}, map[string]any{"name": "B"}})
	sankey["series"].([]any)[0].(map[string]any)["links"] = []any{}
	chartTestCode(t, sankey, "chart.series_data_invalid")
	sankey = chartTestOption("sankey", []any{map[string]any{"name": "A"}, map[string]any{"name": "B"}, map[string]any{"name": "C"}})
	sankey["series"].([]any)[0].(map[string]any)["links"] = []any{map[string]any{"source": "A", "target": "B", "value": 1.0}}
	chartTestCode(t, sankey, "chart.endpoint_invalid")

	unsupportedMixed := chartTestOption("bar", []any{1.0})
	unsupportedMixed["series"] = append(unsupportedMixed["series"].([]any), map[string]any{"type": "pie", "data": []any{map[string]any{"name": "A", "value": 1.0}}})
	chartTestCode(t, unsupportedMixed, "chart.series_type_unknown")

	doublePie := chartTestOption("pie", []any{map[string]any{"name": "A", "value": 1.0}})
	doublePie["series"] = append(doublePie["series"].([]any), map[string]any{"type": "pie", "data": []any{map[string]any{"name": "B", "value": 2.0}}})
	chartTestCode(t, doublePie, "chart.series_count")

	for name, axes := range map[string]map[string]any{
		"missing value axis": {"xAxis": map[string]any{"type": "category", "data": []any{"A"}}},
		"two category axes": {
			"xAxis": map[string]any{"type": "category", "data": []any{"A"}},
			"yAxis": map[string]any{"type": "category", "data": []any{"A"}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			option := map[string]any{"series": []any{map[string]any{"type": "bar", "data": []any{1.0}}}}
			for key, value := range axes {
				option[key] = value
			}
			chartTestCode(t, option, "chart.axis_missing")
		})
	}
	horizontal := map[string]any{
		"xAxis":  map[string]any{"type": "value"},
		"yAxis":  map[string]any{"type": "category", "data": []any{"A"}},
		"series": []any{map[string]any{"type": "bar", "data": []any{1.0}}},
	}
	chartTestCode(t, horizontal, "")
}

func TestCrossPlatformCoverageChartAxisAndSeriesValidation(t *testing.T) {
	t.Run("horizontal-category-axis", func(t *testing.T) {
		option := chartTestOption("effectScatter", []any{2.0})
		option["yAxis"] = map[string]any{"type": "category", "data": []any{"A"}}
		chartTestCode(t, option, "")
		option["yAxis"] = map[string]any{"type": "category", "data": []any{1.0}}
		chartTestCode(t, option, "chart.axis_missing")
	})
	t.Run("category-series-requires-scalar-values", func(t *testing.T) {
		option := chartTestOption("scatter", []any{[]any{1.0, 2.0}})
		option["xAxis"] = map[string]any{"type": "category", "data": []any{"A"}}
		chartTestCode(t, option, "chart.value_invalid")
	})
	t.Run("effect-scatter-missing-coordinate", func(t *testing.T) {
		chartTestCode(t, chartTestOption("effectScatter", []any{[]any{1.0}}), "chart.value_invalid")
	})
	t.Run("invalid-funnel-range", func(t *testing.T) {
		for _, bounds := range []map[string]any{
			{"min": "invalid"}, {"max": "invalid"}, {"min": -1.0}, {"min": 2.0, "max": 2.0},
		} {
			option := chartTestOption("funnel", []any{map[string]any{"name": "A", "value": 2.0}})
			series := object(array(option["series"])[0])
			for key, value := range bounds {
				series[key] = value
			}
			chartTestCode(t, option, "chart.range_invalid")
		}
	})
	t.Run("gauge-name-requires-string", func(t *testing.T) {
		chartTestCode(t, chartTestOption("gauge", []any{map[string]any{"name": 1.0, "value": 5.0}}), "chart.value_invalid")
	})
}

func TestCrossPlatformCoverageChartBindingThroughCardChild(t *testing.T) {
	p := testProtocol(t)
	for _, tc := range []struct {
		path   string
		status string
	}{
		{"/chart", "valid"},
		{"chart", "unverified"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			docs := []map[string]any{{"createSurface": map[string]any{
				"surfaceId": "s",
				"dataModel": map[string]any{"chart": chartTestOption("bar", []any{1.0})},
				"components": []any{
					map[string]any{"id": "root", "component": "Card", "child": "chart"},
					map[string]any{"id": "chart", "component": "Chart", "data": map[string]any{"path": tc.path}},
				},
			}}}
			diagnostics, result := p.checkChartBindings(docs, true)
			if len(diagnostics) != 0 || result["status"] != tc.status || result["valid"] != true {
				t.Fatalf("binding %q: diagnostics=%#v result=%#v", tc.path, diagnostics, result)
			}
		})
	}
}

func TestCrossPlatformCoverageCheckChartBindingsAllStates(t *testing.T) {
	p := testProtocol(t)
	valid := chartTestOption("bar", []any{1.0})
	docs := []map[string]any{
		{"updateDataModel": map[string]any{"surfaceId": "missing", "path": "/ignored", "value": true}},
		{"updateComponents": map[string]any{"surfaceId": "s", "components": []any{
			map[string]any{"id": "early", "component": "Chart", "data": map[string]any{"path": "/chart"}},
		}}},
		{"createSurface": map[string]any{"surfaceId": "s", "dataModel": map[string]any{"chart": valid, "legacy": map[string]any{"mark": "line"}}, "components": []any{
			map[string]any{"id": "root", "component": "Column", "children": []any{"plain", "valid", "legacy", "missing", "wrong"}},
			map[string]any{"id": "plain", "component": "Text"},
			map[string]any{"id": "valid", "component": "Chart", "data": map[string]any{"path": "/chart"}},
			map[string]any{"id": "legacy", "component": "Chart", "data": map[string]any{"path": "/legacy"}},
		}}},
		{"updateDataModel": map[string]any{"surfaceId": "s", "path": "/extra/value", "value": 1.0}},
		{"updateDataModel": map[string]any{"surfaceId": "s", "path": "/extra/value", "value": nil}},
		{"updateComponents": map[string]any{"surfaceId": "s", "components": []any{
			map[string]any{"id": "missing", "component": "Chart", "data": map[string]any{"path": "/missing"}},
			map[string]any{"id": "wrong", "component": "Chart", "data": "not an object"},
		}}},
	}
	diagnostics, status := p.checkChartBindings(docs, true)
	if len(diagnostics) != 2 || status["checked"] != 1 || status["unverified"] != 1 || status["status"] != "invalid" || status["valid"] != false {
		t.Fatalf("mixed state diagnostics=%#v status=%#v", diagnostics, status)
	}
	emptyDocs := []map[string]any{
		{"createSurface": map[string]any{"surfaceId": "s", "dataModel": map[string]any{}, "components": []any{}}},
	}
	_, status = p.checkChartBindings(emptyDocs, true)
	if status["status"] != "not_applicable" || status["valid"] != true {
		t.Fatalf("not-applicable status=%#v", status)
	}
	legacyDocs := []map[string]any{
		{"createSurface": map[string]any{
			"surfaceId":  "s",
			"dataModel":  map[string]any{"legacy": map[string]any{"mark": "line"}},
			"components": chartTestStaticComponents(map[string]any{"id": "legacy", "component": "Chart", "data": map[string]any{"path": "/legacy"}}),
		}},
	}
	_, status = p.checkChartBindings(legacyDocs, true)
	if status["status"] != "unverified" || status["valid"] != true {
		t.Fatalf("unverified status=%#v", status)
	}
	validDocs := []map[string]any{
		{"createSurface": map[string]any{
			"surfaceId":  "s",
			"dataModel":  map[string]any{"chart": valid},
			"components": chartTestStaticComponents(map[string]any{"id": "valid", "component": "Chart", "data": map[string]any{"path": "/chart"}}),
		}},
	}
	_, status = p.checkChartBindings(validDocs, true)
	if status["status"] != "valid" || status["valid"] != true {
		t.Fatalf("valid status=%#v", status)
	}
	invalidOptionDocs := []map[string]any{
		{"createSurface": map[string]any{
			"surfaceId":  "s",
			"dataModel":  map[string]any{"chart": map[string]any{"series": []any{}}},
			"components": chartTestStaticComponents(map[string]any{"id": "invalid", "component": "Chart", "data": map[string]any{"path": "/chart"}}),
		}},
	}
	diagnostics, status = p.checkChartBindings(invalidOptionDocs, true)
	if len(diagnostics) != 1 || diagnostics[0].Code != "chart.series_count" || status["status"] != "invalid" {
		t.Fatalf("invalid option diagnostics=%#v status=%#v", diagnostics, status)
	}

	doublePie := chartTestOption("pie", []any{map[string]any{"name": "A", "value": 1.0}})
	doublePie["series"] = append(doublePie["series"].([]any), map[string]any{"type": "pie", "data": []any{map[string]any{"name": "B", "value": 2.0}}})
	diagnostics, status = p.checkChartBindings([]map[string]any{{"createSurface": map[string]any{
		"surfaceId":  "s",
		"dataModel":  map[string]any{"chart": doublePie},
		"components": chartTestStaticComponents(map[string]any{"id": "double", "component": "Chart", "data": map[string]any{"path": "/chart"}}),
	}}}, true)
	if len(diagnostics) != 1 || diagnostics[0].Code != "chart.series_count" || status["status"] != "invalid" {
		t.Fatalf("bound double-pie diagnostics=%#v status=%#v", diagnostics, status)
	}

	legacyWithSeries := map[string]any{
		"type":   "lineChart",
		"data":   []any{map[string]any{"x": "A", "y": 1.0}},
		"xAxis":  map[string]any{"type": "category", "data": []any{"A"}},
		"yAxis":  map[string]any{"type": "value"},
		"series": []any{map[string]any{"type": "bar", "data": []any{1.0}}},
	}
	diagnostics, status = p.checkChartBindings([]map[string]any{{"createSurface": map[string]any{
		"surfaceId":  "s",
		"dataModel":  map[string]any{"chart": legacyWithSeries},
		"components": chartTestStaticComponents(map[string]any{"id": "legacy", "component": "Chart", "data": map[string]any{"path": "/chart"}}),
	}}}, true)
	if len(diagnostics) != 0 || status["status"] != "unverified" || status["unverified"] != 1 || status["checked"] != 0 {
		t.Fatalf("legacy-with-series diagnostics=%#v status=%#v", diagnostics, status)
	}
	legacyWithoutSeries := map[string]any{
		"type": "lineChart",
		"data": []any{map[string]any{"x": "A", "y": 1.0}},
	}
	diagnostics, status = p.checkChartBindings([]map[string]any{{"createSurface": map[string]any{
		"surfaceId":  "s",
		"dataModel":  map[string]any{"chart": legacyWithoutSeries},
		"components": chartTestStaticComponents(map[string]any{"id": "legacy", "component": "Chart", "data": map[string]any{"path": "/chart"}}),
	}}}, true)
	if len(diagnostics) != 0 || status["status"] != "unverified" || status["unverified"] != 1 || status["checked"] != 0 {
		t.Fatalf("legacy-without-series diagnostics=%#v status=%#v", diagnostics, status)
	}

	for _, unsafe := range []bool{false, true} {
		partialLegacy := chartTestOption("bar", []any{1.0})
		partialLegacy["type"] = "barChart"
		if unsafe {
			partialLegacy["series"].([]any)[0].(map[string]any)["itemStyle"] = map[string]any{"color": "url(evil)"}
		}
		for _, bound := range []bool{false, true} {
			data := any(partialLegacy)
			model := map[string]any{"chart": partialLegacy}
			if bound {
				data = map[string]any{"path": "/chart"}
			}
			diagnostics, status = p.checkChartBindings([]map[string]any{{"createSurface": map[string]any{
				"surfaceId": "s", "dataModel": model,
				"components": chartTestStaticComponents(map[string]any{"id": "partial", "component": "Chart", "data": data}),
			}}}, true)
			if len(diagnostics) == 0 || diagnostics[0].Code != "schema.missing_required" || status["status"] != "invalid" {
				t.Fatalf("partial legacy must fail strict validation: unsafe=%v bound=%v diagnostics=%#v status=%#v", unsafe, bound, diagnostics, status)
			}
		}
	}

	partialLegacyWithoutSeries := []struct {
		name string
		data map[string]any
	}{
		{"type-only", map[string]any{"type": "barChart"}},
		{"data-only", map[string]any{"data": []any{}}},
		{"invalid-type", map[string]any{"type": nil, "data": []any{}}},
	}
	for _, test := range partialLegacyWithoutSeries {
		for _, bound := range []bool{false, true} {
			data := any(test.data)
			model := map[string]any{"chart": test.data}
			if bound {
				data = map[string]any{"path": "/chart"}
			}
			diagnostics, status = p.checkChartBindings([]map[string]any{{"createSurface": map[string]any{
				"surfaceId": "s", "dataModel": model,
				"components": chartTestStaticComponents(map[string]any{"id": "partial", "component": "Chart", "data": data}),
			}}}, true)
			hasStructuralError := false
			for _, diagnostic := range diagnostics {
				if diagnostic.Code == "schema.missing_required" || diagnostic.Code == "schema.type_mismatch" {
					hasStructuralError = true
				}
			}
			if !hasStructuralError || status["status"] != "invalid" {
				t.Fatalf("partial legacy without series must fail strict validation: name=%s bound=%v diagnostics=%#v status=%#v", test.name, bound, diagnostics, status)
			}
		}
	}

	updated := chartTestOption("bar", []any{1.0})
	diagnostics, status = p.checkChartBindings([]map[string]any{
		{"createSurface": map[string]any{
			"surfaceId":  "s",
			"dataModel":  map[string]any{"chart": updated},
			"components": chartTestStaticComponents(map[string]any{"id": "updated", "component": "Chart", "data": map[string]any{"path": "/chart"}}),
		}},
		{"updateDataModel": map[string]any{"surfaceId": "s", "path": "/chart/series/0/data/0", "value": 2.0}},
	}, true)
	if len(diagnostics) != 0 || status["status"] != "valid" {
		t.Fatalf("array update diagnostics=%#v status=%#v", diagnostics, status)
	}
	diagnostics, status = p.checkChartBindings([]map[string]any{
		{"createSurface": map[string]any{
			"surfaceId":  "s",
			"dataModel":  map[string]any{"chart": chartTestOption("bar", []any{1.0})},
			"components": chartTestStaticComponents(map[string]any{"id": "updated", "component": "Chart", "data": map[string]any{"path": "/chart"}}),
		}},
		{"updateDataModel": map[string]any{"surfaceId": "s", "path": "/chart/series/9/data/0", "value": 2.0}},
	}, true)
	if len(diagnostics) != 1 || diagnostics[0].Code != "chart.data_model_update_unsupported" || status["status"] != "invalid" {
		t.Fatalf("unsupported array update diagnostics=%#v status=%#v", diagnostics, status)
	}

	p.hasPureChartVariant = false
	diagnostics, status = p.checkChartBindings(validDocs, true)
	if len(diagnostics) != 1 || diagnostics[0].Code != "chart.option_structure_unavailable" || diagnostics[0].Severity != "warning" || status["status"] != "unverified" || status["checked"] != 1 || status["unverified"] != 1 || status["valid"] != true {
		t.Fatalf("missing pure-series schema diagnostics=%#v status=%#v", diagnostics, status)
	}
}

func TestCrossPlatformCoverageAicardBoundChartOptionsReusePublicStructure(t *testing.T) {
	p := testProtocol(t)
	tests := map[string]map[string]any{
		"unknown top-level field": {
			"config": map[string]any{"unexpected": true},
			"xAxis":  map[string]any{"type": "category", "data": []any{"A"}},
			"yAxis":  map[string]any{"type": "value"},
			"series": []any{map[string]any{"type": "bar", "data": []any{1.0}}},
		},
		"numeric category label": {
			"xAxis":  map[string]any{"type": "category", "data": []any{42.0}},
			"yAxis":  map[string]any{"type": "value"},
			"series": []any{map[string]any{"type": "bar", "data": []any{1.0}}},
		},
	}
	for name, option := range tests {
		t.Run(name, func(t *testing.T) {
			docs := []map[string]any{{"createSurface": map[string]any{
				"surfaceId":  "s",
				"dataModel":  map[string]any{"chart": option},
				"components": chartTestStaticComponents(map[string]any{"id": "chart", "component": "Chart", "data": map[string]any{"path": "/chart"}}),
			}}}
			diagnostics, status := p.checkChartBindings(docs, true)
			want := "chart.axis_missing"
			if name == "unknown top-level field" {
				want = "chart.local_coordinates"
			}
			if len(diagnostics) != 1 || diagnostics[0].Code != want || status["status"] != "invalid" {
				t.Fatalf("bound option must fail the same structure: diagnostics=%#v status=%#v", diagnostics, status)
			}
			docs[0]["createSurface"].(map[string]any)["components"].([]any)[1].(map[string]any)["data"] = option
			diagnostics, status = p.checkChartBindings(docs, true)
			if len(diagnostics) != 1 || diagnostics[0].Code != want || status["status"] != "invalid" {
				t.Fatalf("inline option must fail semantic validation: diagnostics=%#v status=%#v", diagnostics, status)
			}
		})
	}
}

func TestCrossPlatformCoverageExistingContentKeepsChartBindingsAdvisory(t *testing.T) {
	p := testProtocol(t)
	for _, model := range []string{
		`{}`,
		`{"chart":null}`,
		`{"chart":{"config":{"unexpected":true},"xAxis":{"type":"category","data":["A"]},"yAxis":{"type":"value"},"series":[{"type":"bar","data":[1]}]}}`,
	} {
		messages := []string{fmt.Sprintf(`{"version":"v1.0","createSurface":{"surfaceId":"s","catalogId":"https://dingtalk.com/card/a2ui/catalogs/public/catalog.json","dataModel":%s,"components":[{"id":"root","component":"Chart","data":{"path":"/chart"}}]}}`, model)}
		strict, err := p.Preflight(messages, "new-card")
		if err != nil || strict["valid"] == true {
			t.Fatalf("strict preflight must reject unresolved or invalid Chart data: report=%#v err=%v", strict, err)
		}
		compatible, err := p.PreflightForExistingContent(messages, "new-card")
		if err != nil || compatible["valid"] != true {
			t.Fatalf("existing-content preflight must preserve compatibility: report=%#v err=%v", compatible, err)
		}
		diagnostics := compatible["diagnostics"].([]Diagnostic)
		if len(diagnostics) == 0 || diagnostics[0].Severity != "warning" {
			t.Fatalf("compatibility result must retain an advisory diagnostic: %#v", diagnostics)
		}
		chart := compatible["chartValidation"].(map[string]any)
		if chart["status"] != "unverified" || chart["valid"] != true {
			t.Fatalf("compatibility Chart status=%#v", chart)
		}
	}
}
