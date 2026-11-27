// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package chartgen

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"
)

func chartRaw(t *testing.T, value any) []byte {
	t.Helper()
	data, err := jsonMarshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestCrossPlatformCoverageStrictDecoderBoundaries(t *testing.T) {
	type child struct {
		Value int `json:"value"`
	}
	type input struct {
		Name     string
		Children []child `json:"children"`
	}
	for _, raw := range []string{`[]`, `{"Name":42}`, `{"children":[{"unknown":1}]}`} {
		var value input
		if err := decodeStrict([]byte(raw), &value); err == nil {
			t.Fatalf("accepted malformed typed input %s", raw)
		}
	}
	var value input
	if err := decodeStrict([]byte(`{"Name":"ok","children":[{"value":2}]}`), &value); err != nil || value.Name != "ok" || value.Children[0].Value != 2 {
		t.Fatalf("valid untagged field or nested array: %+v %v", value, err)
	}
	p := chartPreset("mixed", "category", "bar", "line")
	_, _, err := buildCategory(p, "Test", chartRaw(t, categoryData{Kind: "category", Categories: []string{"A"}, Series: []numericSeries{{Name: "one", Values: []float64{1}}}}))
	if err == nil || !strings.Contains(err.Error(), "expected 2 aligned series") {
		t.Fatalf("mixed-series alignment: %v", err)
	}
	single := chartPreset("single", "category", "bar")
	option, _, err := buildCategory(single, "Test", chartRaw(t, categoryData{Kind: "category", Categories: []string{"A"}, Series: []numericSeries{
		{Name: "one", Values: []float64{1}}, {Name: "two", Values: []float64{2}},
	}}))
	if err != nil {
		t.Fatalf("homogeneous series expansion: %v", err)
	}
	series := option["series"].([]map[string]any)
	if len(series) != 2 || series[1]["name"] != "two" || len(single.SeriesTypes) != 1 || len(single.SeriesStyles) != 1 {
		t.Fatalf("expansion lost business data or mutated preset: %+v %+v", option, single)
	}
}

func jsonMarshal(value any) ([]byte, error) {
	return json.Marshal(value)
}

func chartPreset(id, family string, types ...string) Preset {
	styles := make([]map[string]any, len(types))
	for i := range styles {
		styles[i] = map[string]any{}
	}
	return Preset{ID: id, Alias: strings.ToLower(id), Family: family, SeriesTypes: types, SeriesStyles: styles, CategoryAxis: "x"}
}

func chartExpectError(t *testing.T, want string, fn func() error) {
	t.Helper()
	err := fn()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want substring %q", err, want)
	}
}

func TestCrossPlatformCoverageChartBuildPrimitiveAndInjectedBoundaries(t *testing.T) {
	for _, value := range []string{"", " padded", strings.Repeat("x", 4)} {
		if err := validateLabel(value, "field", 3); err == nil {
			t.Fatalf("accepted label %q", value)
		}
	}
	if err := validateLabel("a\nb", "field", 10); err == nil {
		t.Fatal("accepted control character")
	}
	if err := validateLabel("ok", "field", 10); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"", strings.Repeat("a", 81), "bad/value"} {
		if err := validateSurfaceID(value); err == nil {
			t.Fatalf("accepted surface ID %q", value)
		}
	}
	if err := validateSurfaceID("Valid_1-id"); err != nil {
		t.Fatal(err)
	}
	if got := escapeMarkdown("a*b[c]!"); got != `a\*b\[c\]\!` {
		t.Fatalf("escaped markdown = %q", got)
	}
	if _, err := wireContent([]map[string]any{{"bad": func() {}}}); err == nil {
		t.Fatal("wireContent accepted an unsupported value")
	}

	originalRead := chartRandomRead
	originalWire := chartWireEncode
	t.Cleanup(func() { chartRandomRead, chartWireEncode = originalRead, originalWire })
	chartRandomRead = func([]byte) (int, error) { return 0, errors.New("random failed") }
	valid := chartRaw(t, map[string]any{"specVersion": "1.0", "presetId": "G26", "title": "Score", "data": map[string]any{"kind": "measure", "name": "Score", "value": 5, "min": 0, "max": 10}})
	chartExpectError(t, "random failed", func() error { _, err := Build(valid, ""); return err })
	chartRandomRead = originalRead
	for _, failAt := range []int{1, 2} {
		calls := 0
		chartWireEncode = func(messages []map[string]any) (string, error) {
			calls++
			if calls == failAt {
				return "", fmt.Errorf("wire %d failed", failAt)
			}
			return wireContent(messages)
		}
		chartExpectError(t, fmt.Sprintf("wire %d failed", failAt), func() error { _, err := Build(valid, "surface"); return err })
	}
	chartWireEncode = originalWire

	for _, tc := range []struct {
		name          string
		raw           []byte
		surface, want string
	}{
		{"empty", nil, "s", "1–262144"},
		{"large", make([]byte, 256*1024+1), "s", "1–262144"},
		{"invalid", []byte(`{`), "s", "invalid ChartSpec"},
		{"version", []byte(`{"specVersion":"2","presetId":"G26","title":"x","data":{}}`), "s", "unsupported"},
		{"title", []byte(`{"specVersion":"1.0","presetId":"G26","title":" x","data":{}}`), "s", "title"},
		{"preset", []byte(`{"specVersion":"1.0","presetId":"bad","title":"x","data":{}}`), "s", "unknown"},
		{"data", []byte(`{"specVersion":"1.0","presetId":"G26","title":"x"}`), "s", "data is required"},
		{"family", []byte(`{"specVersion":"1.0","presetId":"G26","title":"x","data":{"kind":"bad"}}`), "s", "G26"},
		{"surface", valid, "bad/value", "surface ID"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chartExpectError(t, tc.want, func() error { _, err := Build(tc.raw, tc.surface); return err })
		})
	}
	if artifact, err := Build(valid, ""); err != nil || !strings.HasPrefix(artifact.SurfaceID, "chart-") {
		t.Fatalf("generated surface: %#v %v", artifact, err)
	}
}

func TestCrossPlatformCoverageChartFamilySharedBoundaries(t *testing.T) {
	var decoded map[string]any
	if err := decodeStrict([]byte(`{} {}`), &decoded); err == nil {
		t.Fatal("decodeStrict accepted two JSON values")
	}
	for _, value := range []float64{math.NaN(), math.Inf(1), 9007199254740992} {
		if finite(value) {
			t.Fatalf("finite(%v) = true", value)
		}
	}
	if !finite(1) {
		t.Fatal("finite(1) = false")
	}
	if err := labels(nil, "labels", 1, 2); err == nil {
		t.Fatal("accepted empty labels")
	}
	if err := labels([]string{" bad"}, "labels", 1, 2); err == nil {
		t.Fatal("accepted invalid label")
	}
	if err := labels([]string{"A", "A"}, "labels", 1, 2); err == nil {
		t.Fatal("accepted duplicate label")
	}
	if err := labels([]string{"A"}, "labels", 1, 2); err != nil {
		t.Fatal(err)
	}
	if err := optionalLabel(" bad", "optional"); err == nil {
		t.Fatal("accepted invalid optional label")
	}
	if err := optionalLabel("", "optional"); err != nil {
		t.Fatal(err)
	}
	p := chartPreset("G", "category", "bar")
	p.Legend = true
	p.SeriesStyles[0]["stack"] = "total"
	if baseOption(p, "Title")["legend"] == nil || styledSeries(p, 0, "Series", []float64{1})["stack"] != "total" {
		t.Fatal("style helpers lost metadata")
	}
	if got := summary("Title", []string{strings.Repeat("x", 1100)}); len([]rune(got)) != 1000 || !strings.HasSuffix(got, "…") {
		t.Fatalf("summary was not truncated: %d", len([]rune(got)))
	}
	chartExpectError(t, "unknown chart family", func() error {
		_, _, err := buildOption(Preset{ID: "X", Family: "bad"}, "Title", []byte(`{}`))
		return err
	})
	chartExpectError(t, "X:", func() error {
		_, _, err := buildOption(Preset{ID: "X", Family: "category"}, "Title", []byte(`{}`))
		return err
	})
	p = chartPreset("X", "category", "bar")
	p.SeriesStyles[0]["bad"] = func() {}
	data := categoryData{Kind: "category", Categories: []string{"A"}, Series: []numericSeries{{Name: "S", Values: []float64{1}}}}
	chartExpectError(t, "cannot be encoded", func() error { _, _, err := buildOption(p, "Title", chartRaw(t, data)); return err })
	p = chartPreset("X", "category", "bar")
	chartExpectError(t, "256 KiB", func() error {
		_, _, err := buildOption(p, strings.Repeat("x", 257*1024), chartRaw(t, data))
		return err
	})
}

func TestCrossPlatformCoverageRawNumericErrorBoundaries(t *testing.T) {
	malformed := json.RawMessage(`{`)
	for name, validate := range map[string]func(json.RawMessage) error{
		"category":     validateCategoryNumbers,
		"named values": validateNamedValueNumbers,
		"points":       validatePointNumbers,
		"matrix":       validateMatrixNumbers,
		"ohlc":         validateOHLCNumbers,
		"radar":        validateRadarNumbers,
		"measure":      validateMeasureNumbers,
		"flow":         validateFlowNumbers,
	} {
		t.Run(name+" malformed", func(t *testing.T) {
			if err := validate(malformed); err == nil {
				t.Fatal("accepted malformed JSON")
			}
		})
	}
	invalidNumbers := []struct {
		name     string
		raw      json.RawMessage
		validate func(json.RawMessage) error
	}{
		{"named value", json.RawMessage(`{"items":[{"value":null}]}`), validateNamedValueNumbers},
		{"matrix coordinate", json.RawMessage(`{"cells":[{"x":null,"y":0,"value":1}]}`), validateMatrixNumbers},
		{"ohlc coordinate", json.RawMessage(`{"candles":[[1,2,null,3]]}`), validateOHLCNumbers},
		{"radar indicator", json.RawMessage(`{"indicators":[{"max":null}]}`), validateRadarNumbers},
		{"radar value", json.RawMessage(`{"indicators":[{"max":1}],"groups":[{"values":[null]}]}`), validateRadarNumbers},
		{"flow value", json.RawMessage(`{"links":[{"value":null}]}`), validateFlowNumbers},
	}
	for _, tc := range invalidNumbers {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.validate(tc.raw); err == nil {
				t.Fatal("accepted missing numeric value")
			}
		})
	}
}

func TestCrossPlatformCoverageFamilyRawNumericGuards(t *testing.T) {
	cases := []struct {
		name string
		run  func() error
	}{
		{"named values", func() error {
			_, _, err := buildNamedValues(chartPreset("G13", "named_values", "pie"), "T", []byte(`{"kind":"named_values","items":[{"name":"A","value":null},{"name":"B","value":1}]}`))
			return err
		}},
		{"matrix", func() error {
			_, _, err := buildMatrix(chartPreset("G22", "matrix", "heatmap"), "T", []byte(`{"kind":"matrix","xLabels":["A"],"yLabels":["B"],"cells":[{"x":null,"y":0,"value":1}]}`))
			return err
		}},
		{"radar", func() error {
			p := chartPreset("G29", "radar", "radar")
			p.RadarGroupCount = 1
			_, _, err := buildRadar(p, "T", []byte(`{"kind":"radar","indicators":[{"name":"A","max":null},{"name":"B","max":1},{"name":"C","max":1}],"groups":[{"name":"G","values":[0,0,0]}]}`))
			return err
		}},
		{"flow", func() error {
			_, _, err := buildFlow(chartPreset("G44", "flow", "sankey"), "T", []byte(`{"kind":"flow","nodes":["A","B"],"links":[{"source":"A","target":"B","value":null}]}`))
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); err == nil {
				t.Fatal("accepted invalid raw numeric input")
			}
		})
	}
}

func TestCrossPlatformCoverageBuildCategoryBoundaries(t *testing.T) {
	base := categoryData{Kind: "category", Categories: []string{"A", "B"}, Series: []numericSeries{{Name: "S", Unit: "u", Values: []float64{1, 2}}}}
	p := chartPreset("G01", "category", "bar")
	cases := []struct {
		name, want string
		preset     Preset
		data       any
	}{
		{"decode", "expected category", p, map[string]any{"kind": "category", "unknown": true}},
		{"kind", "expected category", p, categoryData{Kind: "bad"}},
		{"labels", "categories", p, categoryData{Kind: "category"}},
		{"series count", "expected 1", p, categoryData{Kind: "category", Categories: []string{"A"}}},
		{"category label", "categoryLabel", p, categoryData{Kind: "category", Categories: []string{"A"}, CategoryLabel: " bad", Series: []numericSeries{{Name: "S", Values: []float64{1}}}}},
		{"value label", "valueLabel", p, categoryData{Kind: "category", Categories: []string{"A"}, ValueLabel: " bad", Series: []numericSeries{{Name: "S", Values: []float64{1}}}}},
		{"stack unsupported", "only supported", p, categoryData{Kind: "category", Categories: []string{"A"}, StackValues: "raw", Series: []numericSeries{{Name: "S", Values: []float64{1}}}}},
		{"cumulative unsupported", "only supported", p, categoryData{Kind: "category", Categories: []string{"A"}, CumulativeMode: "increments", Series: []numericSeries{{Name: "S", Values: []float64{1}}}}},
		{"axis", "no category axis", func() Preset { q := p; q.CategoryAxis = ""; return q }(), base},
		{"series name", "series name", p, categoryData{Kind: "category", Categories: []string{"A"}, Series: []numericSeries{{Name: " bad", Values: []float64{1}}}}},
		{"unit", "series unit", p, categoryData{Kind: "category", Categories: []string{"A"}, Series: []numericSeries{{Name: "S", Unit: " bad", Values: []float64{1}}}}},
		{"length", "length does not match", p, categoryData{Kind: "category", Categories: []string{"A"}, Series: []numericSeries{{Name: "S"}}}},
		{"nonfinite", "nonfinite", p, categoryData{Kind: "category", Categories: []string{"A"}, Series: []numericSeries{{Name: "S", Values: []float64{9007199254740992}}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chartExpectError(t, tc.want, func() error { _, _, err := buildCategory(tc.preset, "Title", chartRaw(t, tc.data)); return err })
		})
	}

	dup := chartPreset("G01", "category", "bar", "bar")
	dupData := categoryData{Kind: "category", Categories: []string{"A"}, Series: []numericSeries{{Name: "S", Values: []float64{1}}, {Name: "S", Values: []float64{2}}}}
	chartExpectError(t, "duplicate series", func() error { _, _, err := buildCategory(dup, "Title", chartRaw(t, dupData)); return err })
	large := chartPreset("G01", "category", "bar")
	large.SeriesTypes = make([]string, 26)
	large.SeriesStyles = make([]map[string]any, 26)
	largeData := categoryData{Kind: "category", Categories: make([]string, 80), Series: make([]numericSeries, 26)}
	for i := range largeData.Categories {
		largeData.Categories[i] = fmt.Sprintf("C%d", i)
	}
	for i := range largeData.Series {
		largeData.Series[i] = numericSeries{Name: fmt.Sprintf("S%d", i), Values: make([]float64, 80)}
		large.SeriesTypes[i] = "bar"
		large.SeriesStyles[i] = map[string]any{}
	}
	chartExpectError(t, "2000 points", func() error { _, _, err := buildCategory(large, "Title", chartRaw(t, largeData)); return err })

	g31 := chartPreset("G31", "category", "bar")
	g31Data := categoryData{Kind: "category", Categories: []string{"A"}, Series: []numericSeries{{Name: "S", Values: []float64{1}}}}
	chartExpectError(t, "stackValues", func() error { _, _, err := buildCategory(g31, "Title", chartRaw(t, g31Data)); return err })
	g31Data.StackValues = "raw"
	g31Data.Series[0].Values[0] = -1
	chartExpectError(t, "negative", func() error { _, _, err := buildCategory(g31, "Title", chartRaw(t, g31Data)); return err })
	g31Data.Series[0].Values[0] = 0
	chartExpectError(t, "zero or negative", func() error { _, _, err := buildCategory(g31, "Title", chartRaw(t, g31Data)); return err })
	g31Data.StackValues = "already_percent"
	g31Data.Series[0].Values[0] = 50
	chartExpectError(t, "total 100", func() error { _, _, err := buildCategory(g31, "Title", chartRaw(t, g31Data)); return err })

	g34 := chartPreset("G34", "category", "line")
	g34Data := categoryData{Kind: "category", Categories: []string{"A", "B"}, Series: []numericSeries{{Name: "S", Values: []float64{1, 2}}}}
	chartExpectError(t, "cumulativeMode", func() error { _, _, err := buildCategory(g34, "Title", chartRaw(t, g34Data)); return err })
	g34Data.CumulativeMode = "increments"
	g34Data.Series[0].Values = []float64{9007199254740991, 1}
	chartExpectError(t, "exact numeric range", func() error { _, _, err := buildCategory(g34, "Title", chartRaw(t, g34Data)); return err })

	yAxis := base
	yPreset := p
	yPreset.CategoryAxis = "y"
	yAxis.CategoryLabel = "Category"
	yAxis.ValueLabel = "Value"
	if _, _, err := buildCategory(yPreset, "Title", chartRaw(t, yAxis)); err != nil {
		t.Fatal(err)
	}
	many := chartPreset("G01", "category", "bar", "bar", "bar", "bar")
	manyData := categoryData{Kind: "category", Categories: []string{"A", "B", "C", "D", "E", "F"}, Series: []numericSeries{}}
	for i := 0; i < 4; i++ {
		manyData.Series = append(manyData.Series, numericSeries{Name: fmt.Sprintf("S%d", i), Values: []float64{1, 2, 3, 4, 5, 6}})
	}
	if _, text, err := buildCategory(many, "Title", chartRaw(t, manyData)); err != nil || strings.Count(text, "… (+1)") != 4 {
		t.Fatalf("many category: %v %s", err, text)
	}
}

func TestCrossPlatformCoverageBuildSimpleFamilyErrorBoundaries(t *testing.T) {
	named := chartPreset("G13", "named_values", "pie")
	funnel := chartPreset("G45", "named_values", "funnel")
	points := chartPreset("G19", "points", "scatter")
	matrix := chartPreset("G20", "matrix", "heatmap")
	matrix.VisualMapStyle = map[string]any{"orient": "horizontal"}
	ohlc := chartPreset("G22", "ohlc", "candlestick")
	radar := chartPreset("G23", "radar", "radar")
	radar.RadarGroupCount = 1
	radar.RadarStyle = map[string]any{"shape": "circle"}
	measure := chartPreset("G26", "measure", "gauge")
	flow := chartPreset("G29", "flow", "sankey")

	type familyCase struct {
		name, want string
		run        func() error
	}
	cases := []familyCase{
		{"named decode", "expected named_values", func() error { _, _, err := buildNamedValues(named, "T", []byte(`{"kind":"bad"}`)); return err }},
		{"named count", "2–24", func() error {
			_, _, err := buildNamedValues(named, "T", chartRaw(t, namedData{Kind: "named_values", Items: []namedItem{{Name: "A", Value: 1}}}))
			return err
		}},
		{"named unit", "unit", func() error {
			_, _, err := buildNamedValues(named, "T", chartRaw(t, namedData{Kind: "named_values", Unit: " bad", Items: []namedItem{{Name: "A", Value: 1}, {Name: "B", Value: 2}}}))
			return err
		}},
		{"named label", "item name", func() error {
			_, _, err := buildNamedValues(named, "T", chartRaw(t, namedData{Kind: "named_values", Items: []namedItem{{Name: " bad", Value: 1}, {Name: "B", Value: 2}}}))
			return err
		}},
		{"named duplicate", "duplicate", func() error {
			_, _, err := buildNamedValues(named, "T", chartRaw(t, namedData{Kind: "named_values", Items: []namedItem{{Name: "A", Value: 1}, {Name: "A", Value: 2}}}))
			return err
		}},
		{"funnel order", "descending", func() error {
			_, _, err := buildNamedValues(funnel, "T", chartRaw(t, namedData{Kind: "named_values", Items: []namedItem{{Name: "A", Value: 1}, {Name: "B", Value: 2}}}))
			return err
		}},
		{"named positive", "positive", func() error {
			_, _, err := buildNamedValues(named, "T", chartRaw(t, namedData{Kind: "named_values", Items: []namedItem{{Name: "A"}, {Name: "B"}}}))
			return err
		}},
		{"points decode", "expected points", func() error { _, _, err := buildPoints(points, "T", []byte(`{"kind":"bad"}`)); return err }},
		{"points groups", "expected 1", func() error {
			_, _, err := buildPoints(points, "T", chartRaw(t, pointsData{Kind: "points"}))
			return err
		}},
		{"points axis", "xLabel", func() error {
			_, _, err := buildPoints(points, "T", chartRaw(t, pointsData{Kind: "points", Groups: []pointGroup{{Name: "G", Points: [][2]float64{{1, 2}}}}}))
			return err
		}},
		{"points name", "group name", func() error {
			_, _, err := buildPoints(points, "T", chartRaw(t, pointsData{Kind: "points", XLabel: "X", YLabel: "Y", Groups: []pointGroup{{Name: " bad", Points: [][2]float64{{1, 2}}}}}))
			return err
		}},
		{"points empty", "no points", func() error {
			_, _, err := buildPoints(points, "T", chartRaw(t, pointsData{Kind: "points", XLabel: "X", YLabel: "Y", Groups: []pointGroup{{Name: "G"}}}))
			return err
		}},
		{"points finite", "nonfinite", func() error {
			_, _, err := buildPoints(points, "T", chartRaw(t, pointsData{Kind: "points", XLabel: "X", YLabel: "Y", Groups: []pointGroup{{Name: "G", Points: [][2]float64{{9007199254740992, 2}}}}}))
			return err
		}},
		{"matrix decode", "expected matrix", func() error { _, _, err := buildMatrix(matrix, "T", []byte(`{"kind":"bad"}`)); return err }},
		{"matrix x", "xLabels", func() error {
			_, _, err := buildMatrix(matrix, "T", chartRaw(t, matrixData{Kind: "matrix"}))
			return err
		}},
		{"matrix y", "yLabels", func() error {
			_, _, err := buildMatrix(matrix, "T", chartRaw(t, matrixData{Kind: "matrix", XLabels: []string{"A"}}))
			return err
		}},
		{"matrix cells", "1–2000", func() error {
			_, _, err := buildMatrix(matrix, "T", chartRaw(t, matrixData{Kind: "matrix", XLabels: []string{"A"}, YLabels: []string{"B"}}))
			return err
		}},
		{"matrix coordinate", "invalid or duplicate", func() error {
			_, _, err := buildMatrix(matrix, "T", chartRaw(t, matrixData{Kind: "matrix", XLabels: []string{"A"}, YLabels: []string{"B"}, Cells: []matrixCell{{X: 1, Y: 0, Value: 1}}}))
			return err
		}},
		{"matrix incomplete", "incomplete", func() error {
			_, _, err := buildMatrix(matrix, "T", chartRaw(t, matrixData{Kind: "matrix", XLabels: []string{"A", "B"}, YLabels: []string{"C"}, Cells: []matrixCell{{X: 0, Y: 0, Value: 1}}}))
			return err
		}},
		{"ohlc decode", "expected ohlc", func() error { _, _, err := buildOHLC(ohlc, "T", []byte(`{"kind":"bad"}`)); return err }},
		{"ohlc labels", "categories", func() error { _, _, err := buildOHLC(ohlc, "T", chartRaw(t, candleData{Kind: "ohlc"})); return err }},
		{"ohlc name", "series name", func() error {
			_, _, err := buildOHLC(ohlc, "T", chartRaw(t, candleData{Kind: "ohlc", Categories: []string{"A"}}))
			return err
		}},
		{"ohlc unit", "unit", func() error {
			_, _, err := buildOHLC(ohlc, "T", chartRaw(t, candleData{Kind: "ohlc", Categories: []string{"A"}, Name: "S", Unit: " bad", Candles: [][4]float64{{1, 2, 0, 3}}}))
			return err
		}},
		{"ohlc length", "count", func() error {
			_, _, err := buildOHLC(ohlc, "T", chartRaw(t, candleData{Kind: "ohlc", Categories: []string{"A"}, Name: "S"}))
			return err
		}},
		{"ohlc tuple", "invalid OHLC", func() error {
			_, _, err := buildOHLC(ohlc, "T", chartRaw(t, candleData{Kind: "ohlc", Categories: []string{"A"}, Name: "S", Candles: [][4]float64{{1, 2, 2, 3}}}))
			return err
		}},
		{"radar decode", "expected radar", func() error { _, _, err := buildRadar(radar, "T", []byte(`{"kind":"bad"}`)); return err }},
		{"radar count", "3–12", func() error { _, _, err := buildRadar(radar, "T", chartRaw(t, radarData{Kind: "radar"})); return err }},
		{"radar max", "max", func() error {
			_, _, err := buildRadar(radar, "T", chartRaw(t, radarData{Kind: "radar", Indicators: []indicator{{"A", 0}, {"B", 1}, {"C", 1}}, Groups: []radarGroup{{Name: "G", Values: []float64{0, 0, 0}}}}))
			return err
		}},
		{"radar labels", "indicator names", func() error {
			_, _, err := buildRadar(radar, "T", chartRaw(t, radarData{Kind: "radar", Indicators: []indicator{{"A", 1}, {"A", 1}, {"C", 1}}, Groups: []radarGroup{{Name: "G", Values: []float64{0, 0, 0}}}}))
			return err
		}},
		{"radar name", "group name", func() error {
			_, _, err := buildRadar(radar, "T", chartRaw(t, radarData{Kind: "radar", Indicators: []indicator{{"A", 1}, {"B", 1}, {"C", 1}}, Groups: []radarGroup{{Name: " bad", Values: []float64{0, 0, 0}}}}))
			return err
		}},
		{"radar length", "length mismatch", func() error {
			_, _, err := buildRadar(radar, "T", chartRaw(t, radarData{Kind: "radar", Indicators: []indicator{{"A", 1}, {"B", 1}, {"C", 1}}, Groups: []radarGroup{{Name: "G", Values: []float64{0}}}}))
			return err
		}},
		{"radar range", "bounds", func() error {
			_, _, err := buildRadar(radar, "T", chartRaw(t, radarData{Kind: "radar", Indicators: []indicator{{"A", 1}, {"B", 1}, {"C", 1}}, Groups: []radarGroup{{Name: "G", Values: []float64{2, 0, 0}}}}))
			return err
		}},
		{"measure decode", "expected measure", func() error { _, _, err := buildMeasure(measure, "T", []byte(`{"kind":"bad"}`)); return err }},
		{"measure name", "measure name", func() error {
			_, _, err := buildMeasure(measure, "T", chartRaw(t, measureData{Kind: "measure", Max: 1}))
			return err
		}},
		{"measure unit", "unit", func() error {
			_, _, err := buildMeasure(measure, "T", chartRaw(t, measureData{Kind: "measure", Name: "M", Unit: " bad", Max: 1}))
			return err
		}},
		{"measure range", "gauge", func() error {
			_, _, err := buildMeasure(measure, "T", chartRaw(t, measureData{Kind: "measure", Name: "M", Min: 1, Max: 2, Value: 1}))
			return err
		}},
		{"flow decode", "expected flow", func() error { _, _, err := buildFlow(flow, "T", []byte(`{"kind":"bad"}`)); return err }},
		{"flow nodes", "nodes", func() error { _, _, err := buildFlow(flow, "T", chartRaw(t, flowData{Kind: "flow"})); return err }},
		{"flow links", "1–256", func() error {
			_, _, err := buildFlow(flow, "T", chartRaw(t, flowData{Kind: "flow", Nodes: []string{"A", "B"}}))
			return err
		}},
		{"flow endpoint", "unknown endpoint", func() error {
			_, _, err := buildFlow(flow, "T", chartRaw(t, flowData{Kind: "flow", Nodes: []string{"A", "B"}, Links: []flowLink{{"A", "C", 1}}}))
			return err
		}},
		{"flow range", "12 orders", func() error {
			_, _, err := buildFlow(flow, "T", chartRaw(t, flowData{Kind: "flow", Nodes: []string{"A", "B", "C"}, Links: []flowLink{{"A", "B", 1}, {"B", "C", 1e-13}}}))
			return err
		}},
		{"flow isolated", "isolated", func() error {
			_, _, err := buildFlow(flow, "T", chartRaw(t, flowData{Kind: "flow", Nodes: []string{"A", "B", "C"}, Links: []flowLink{{"A", "B", 1}}}))
			return err
		}},
		{"flow cycle", "cycle", func() error {
			_, _, err := buildFlow(flow, "T", chartRaw(t, flowData{Kind: "flow", Nodes: []string{"A", "B"}, Links: []flowLink{{"A", "B", 1}, {"B", "A", 1}}}))
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { chartExpectError(t, tc.want, tc.run) })
	}

	items := make([]namedItem, 9)
	for i := range items {
		items[i] = namedItem{Name: fmt.Sprintf("N%d", i), Value: float64(9 - i)}
	}
	if _, text, err := buildNamedValues(named, "T", chartRaw(t, namedData{Kind: "named_values", Items: items})); err != nil || !strings.Contains(text, "… (+1)") {
		t.Fatalf("named summary: %v %s", err, text)
	}
	twoGroups := chartPreset("G19", "points", "scatter", "scatter")
	dupGroups := pointsData{Kind: "points", XLabel: "X", YLabel: "Y", Groups: []pointGroup{{Name: "G", Points: [][2]float64{{1, 2}}}, {Name: "G", Points: [][2]float64{{2, 3}}}}}
	chartExpectError(t, "duplicate point", func() error { _, _, err := buildPoints(twoGroups, "T", chartRaw(t, dupGroups)); return err })
	tooManyPoints := make([][2]float64, 2001)
	chartExpectError(t, "2000 points", func() error {
		_, _, err := buildPoints(points, "T", chartRaw(t, pointsData{Kind: "points", XLabel: "X", YLabel: "Y", Groups: []pointGroup{{Name: "G", Points: tooManyPoints}}}))
		return err
	})
	square := matrix
	square.ID = "G43"
	chartExpectError(t, "square", func() error {
		_, _, err := buildMatrix(square, "T", chartRaw(t, matrixData{Kind: "matrix", XLabels: []string{"A"}, YLabels: []string{"B", "C"}, Cells: []matrixCell{{0, 0, 1}}, Sparse: true}))
		return err
	})
	if _, _, err := buildMatrix(matrix, "T", chartRaw(t, matrixData{Kind: "matrix", XLabels: []string{"A", "B"}, YLabels: []string{"C"}, Cells: []matrixCell{{0, 0, 1}}, Sparse: true})); err != nil {
		t.Fatal(err)
	}
	candles := make([][4]float64, 6)
	categories := make([]string, 6)
	for i := range candles {
		candles[i] = [4]float64{1, 2, 0, 3}
		categories[i] = fmt.Sprintf("C%d", i)
	}
	if _, text, err := buildOHLC(ohlc, "T", chartRaw(t, candleData{Kind: "ohlc", Categories: categories, Name: "S", Unit: "u", Candles: candles})); err != nil || !strings.Contains(text, "… (+1)") {
		t.Fatalf("ohlc summary: %v %s", err, text)
	}
	twoRadar := radar
	twoRadar.RadarGroupCount = 2
	dupRadar := radarData{Kind: "radar", Indicators: []indicator{{"A", 1}, {"B", 1}, {"C", 1}}, Groups: []radarGroup{{"G", []float64{0, 0, 0}}, {"G", []float64{0, 0, 0}}}}
	chartExpectError(t, "duplicate radar", func() error { _, _, err := buildRadar(twoRadar, "T", chartRaw(t, dupRadar)); return err })
	validRadar := radarData{Kind: "radar", Indicators: []indicator{{"A", 1}, {"B", 1}, {"C", 1}}, Groups: []radarGroup{{"G", []float64{0, 0, 0}}}}
	if option, _, err := buildRadar(radar, "T", chartRaw(t, validRadar)); err != nil || option["radar"].(map[string]any)["shape"] != "circle" {
		t.Fatalf("radar style: option=%#v err=%v", option, err)
	}
	links := []flowLink{{"A", "B", 1}, {"A", "B", 2}, {"A", "B", 3}, {"A", "B", 4}, {"A", "B", 5}, {"A", "B", 6}}
	if _, text, err := buildFlow(flow, "T", chartRaw(t, flowData{Kind: "flow", Nodes: []string{"A", "B"}, Links: links})); err != nil || !strings.Contains(text, "… (+1)") {
		t.Fatalf("flow summary: %v %s", err, text)
	}
}

func TestCrossPlatformCoverageRegistryValidationAndDetachedCopies(t *testing.T) {
	originalBytes := presetBytes
	t.Cleanup(func() { presetBytes, loadOnce, loaded, loadErr = originalBytes, sync.Once{}, registry{}, nil })
	reset := func(data []byte) { presetBytes, loadOnce, loaded, loadErr = data, sync.Once{}, registry{}, nil }
	validRegistry := func() registry {
		value := registry{Version: "1.0", Presets: make([]Preset, 43)}
		for i := range value.Presets {
			value.Presets[i] = chartPreset(fmt.Sprintf("X%02d", i), "category", "bar")
			value.Presets[i].Alias = fmt.Sprintf("alias-%02d", i)
		}
		return value
	}
	reset([]byte(`{`))
	if _, err := load(); err == nil {
		t.Fatal("accepted invalid registry JSON")
	}
	reset(chartRaw(t, registry{Version: "1.0"}))
	if _, err := List(); err == nil {
		t.Fatal("accepted incomplete registry")
	}
	bad := validRegistry()
	bad.Presets[0].ID = "G16"
	reset(chartRaw(t, bad))
	if _, err := Lookup("x"); err == nil {
		t.Fatal("accepted excluded preset")
	}
	dup := validRegistry()
	dup.Presets[1].Alias = dup.Presets[0].ID
	reset(chartRaw(t, dup))
	if _, err := load(); err == nil {
		t.Fatal("accepted duplicate preset")
	}
	good := validRegistry()
	good.Presets[0].RadarStyle = map[string]any{"shape": "circle"}
	good.Presets[0].VisualMapStyle = map[string]any{"orient": "horizontal"}
	reset(chartRaw(t, good))
	list, err := List()
	if err != nil || len(list) != 43 {
		t.Fatalf("list: %v %d", err, len(list))
	}
	list[0].SeriesTypes[0] = "mutated"
	list[0].SeriesStyles[0]["x"] = true
	list[0].RadarStyle["shape"] = "mutated"
	list[0].VisualMapStyle["orient"] = "mutated"
	copy, err := Lookup(strings.ToUpper(good.Presets[0].Alias))
	if err != nil || copy.SeriesTypes[0] != "bar" || copy.RadarStyle["shape"] != "circle" {
		t.Fatalf("detached lookup: %#v %v", copy, err)
	}
	if _, err := Lookup("missing"); err == nil {
		t.Fatal("accepted missing preset")
	}
}
