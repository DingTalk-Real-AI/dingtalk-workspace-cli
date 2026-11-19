// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package chartgen

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

type numericSeries struct {
	Name   string    `json:"name"`
	Unit   string    `json:"unit,omitempty"`
	Values []float64 `json:"values"`
}

type categoryData struct {
	Kind           string          `json:"kind"`
	Categories     []string        `json:"categories"`
	CategoryLabel  string          `json:"categoryLabel,omitempty"`
	ValueLabel     string          `json:"valueLabel,omitempty"`
	Series         []numericSeries `json:"series"`
	StackValues    string          `json:"stackValues,omitempty"`
	CumulativeMode string          `json:"cumulativeMode,omitempty"`
}

type namedItem struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
}

type namedData struct {
	Kind  string      `json:"kind"`
	Unit  string      `json:"unit,omitempty"`
	Items []namedItem `json:"items"`
}

type pointGroup struct {
	Name   string       `json:"name"`
	Points [][2]float64 `json:"points"`
}

type pointsData struct {
	Kind   string       `json:"kind"`
	XLabel string       `json:"xLabel"`
	YLabel string       `json:"yLabel"`
	Groups []pointGroup `json:"groups"`
}

type matrixCell struct {
	X     int     `json:"x"`
	Y     int     `json:"y"`
	Value float64 `json:"value"`
}

type matrixData struct {
	Kind    string       `json:"kind"`
	XLabels []string     `json:"xLabels"`
	YLabels []string     `json:"yLabels"`
	Cells   []matrixCell `json:"cells"`
	Sparse  bool         `json:"sparse,omitempty"`
}

type candleData struct {
	Kind       string       `json:"kind"`
	Categories []string     `json:"categories"`
	Unit       string       `json:"unit,omitempty"`
	Name       string       `json:"name"`
	Candles    [][4]float64 `json:"candles"`
}

type indicator struct {
	Name string  `json:"name"`
	Max  float64 `json:"max"`
}

type radarGroup struct {
	Name   string    `json:"name"`
	Values []float64 `json:"values"`
}

type radarData struct {
	Kind       string       `json:"kind"`
	Indicators []indicator  `json:"indicators"`
	Groups     []radarGroup `json:"groups"`
}

type measureData struct {
	Kind  string  `json:"kind"`
	Name  string  `json:"name"`
	Unit  string  `json:"unit,omitempty"`
	Value float64 `json:"value"`
	Min   float64 `json:"min"`
	Max   float64 `json:"max"`
}

type flowLink struct {
	Source string  `json:"source"`
	Target string  `json:"target"`
	Value  float64 `json:"value"`
}

type flowData struct {
	Kind  string     `json:"kind"`
	Nodes []string   `json:"nodes"`
	Links []flowLink `json:"links"`
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && math.Abs(value) <= 9007199254740991
}

func labels(values []string, field string, min, max int) error {
	if len(values) < min || len(values) > max {
		return fmt.Errorf("%s must contain %d–%d labels", field, min, max)
	}
	seen := map[string]bool{}
	for _, value := range values {
		if err := validateLabel(value, field, 80); err != nil {
			return err
		}
		if seen[value] {
			return fmt.Errorf("%s contains a duplicate label %q", field, value)
		}
		seen[value] = true
	}
	return nil
}

func optionalLabel(value, field string) error {
	if value != "" {
		return validateLabel(value, field, 80)
	}
	return nil
}

func baseOption(p Preset, title string) map[string]any {
	option := map[string]any{"title": map[string]any{"text": title}, "animation": false}
	if p.Legend {
		option["legend"] = map[string]any{}
	}
	return option
}

func styledSeries(p Preset, index int, name string, data any) map[string]any {
	series := map[string]any{"type": p.SeriesTypes[index], "data": data}
	if name != "" {
		series["name"] = name
	}
	for key, value := range p.SeriesStyles[index] {
		series[key] = value
	}
	return series
}

func displayNumber(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func summary(title string, entries []string) string {
	result := title + "：" + strings.Join(entries, "；") + "。"
	runes := []rune(result)
	if len(runes) > 1000 {
		return string(runes[:999]) + "…"
	}
	return result
}

func buildOption(p Preset, title string, raw json.RawMessage) (map[string]any, string, error) {
	var option map[string]any
	var result string
	var err error
	switch p.Family {
	case "category":
		option, result, err = buildCategory(p, title, raw)
	case "named_values":
		option, result, err = buildNamedValues(p, title, raw)
	case "points":
		option, result, err = buildPoints(p, title, raw)
	case "matrix":
		option, result, err = buildMatrix(p, title, raw)
	case "ohlc":
		option, result, err = buildOHLC(p, title, raw)
	case "radar":
		option, result, err = buildRadar(p, title, raw)
	case "measure":
		option, result, err = buildMeasure(p, title, raw)
	case "flow":
		option, result, err = buildFlow(p, title, raw)
	default:
		return nil, "", fmt.Errorf("unknown chart family %q", p.Family)
	}
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", p.ID, err)
	}
	encoded, err := json.Marshal(option)
	if err != nil || len(encoded) > 256*1024 {
		return nil, "", fmt.Errorf("chart option exceeds 256 KiB or cannot be encoded")
	}
	return option, result, nil
}

func buildCategory(p Preset, title string, raw json.RawMessage) (map[string]any, string, error) {
	var data categoryData
	if err := decodeStrict(raw, &data); err != nil {
		return nil, "", fmt.Errorf("expected category data: %w", err)
	}
	if data.Kind != "category" {
		return nil, "", fmt.Errorf("expected category data kind, got %q", data.Kind)
	}
	if err := validateCategoryNumbers(raw); err != nil {
		return nil, "", err
	}
	if err := labels(data.Categories, "categories", 1, 80); err != nil {
		return nil, "", err
	}
	homogeneous := len(p.SeriesTypes) > 0
	for _, kind := range p.SeriesTypes {
		homogeneous = homogeneous && kind == p.SeriesTypes[0]
	}
	if len(data.Series) < 1 || len(data.Series) > 32 || (!homogeneous && len(data.Series) != len(p.SeriesTypes)) {
		if homogeneous {
			return nil, "", fmt.Errorf("expected 1 to 32 aligned series")
		}
		return nil, "", fmt.Errorf("expected %d aligned series", len(p.SeriesTypes))
	}
	if homogeneous {
		p = clonePreset(p)
		p.Legend = p.Legend || len(data.Series) > 1
		for len(p.SeriesTypes) < len(data.Series) {
			p.SeriesTypes = append(p.SeriesTypes, p.SeriesTypes[0])
			p.SeriesStyles = append(p.SeriesStyles, p.SeriesStyles[0])
		}
	}
	if err := optionalLabel(data.CategoryLabel, "categoryLabel"); err != nil {
		return nil, "", err
	}
	if err := optionalLabel(data.ValueLabel, "valueLabel"); err != nil {
		return nil, "", err
	}
	if len(data.Series)*len(data.Categories) > 2000 {
		return nil, "", fmt.Errorf("category chart exceeds 2000 points")
	}
	if p.ID == "G31" {
		if data.StackValues != "raw" && data.StackValues != "already_percent" {
			return nil, "", fmt.Errorf("G31 requires stackValues raw or already_percent")
		}
	} else if data.StackValues != "" {
		return nil, "", fmt.Errorf("stackValues is only supported by G31")
	}
	if p.ID == "G34" {
		if data.CumulativeMode != "increments" && data.CumulativeMode != "already_cumulative" {
			return nil, "", fmt.Errorf("G34 requires cumulativeMode increments or already_cumulative")
		}
	} else if data.CumulativeMode != "" {
		return nil, "", fmt.Errorf("cumulativeMode is only supported by G34")
	}
	option := baseOption(p, title)
	categoryAxis := map[string]any{"type": "category", "data": data.Categories}
	valueAxis := map[string]any{"type": "value"}
	if data.CategoryLabel != "" {
		categoryAxis["name"] = data.CategoryLabel
	}
	if data.ValueLabel != "" {
		valueAxis["name"] = data.ValueLabel
	}
	if p.ID == "G31" {
		valueAxis["name"] = "%"
	}
	if p.CategoryAxis == "x" {
		option["xAxis"], option["yAxis"] = categoryAxis, valueAxis
	} else if p.CategoryAxis == "y" {
		option["xAxis"], option["yAxis"] = valueAxis, categoryAxis
	} else {
		return nil, "", fmt.Errorf("category preset has no category axis")
	}
	series := make([]map[string]any, 0, len(data.Series))
	entries := []string{}
	seriesNames := map[string]bool{}
	for i, item := range data.Series {
		if err := validateLabel(item.Name, "series name", 80); err != nil {
			return nil, "", err
		}
		if seriesNames[item.Name] {
			return nil, "", fmt.Errorf("duplicate series name %q", item.Name)
		}
		seriesNames[item.Name] = true
		if err := optionalLabel(item.Unit, "series unit"); err != nil {
			return nil, "", err
		}
		if len(item.Values) != len(data.Categories) {
			return nil, "", fmt.Errorf("series %q length does not match categories", item.Name)
		}
		values := append([]float64(nil), item.Values...)
		for _, value := range values {
			if p.ID == "G31" && value < 0 {
				return nil, "", fmt.Errorf("percentage stack cannot contain negative values")
			}
		}
		if p.ID == "G34" && data.CumulativeMode == "increments" {
			for j := 1; j < len(values); j++ {
				values[j] += values[j-1]
				if !finite(values[j]) {
					return nil, "", fmt.Errorf("cumulative value exceeds exact numeric range")
				}
			}
		}
		series = append(series, styledSeries(p, i, item.Name, values))
		parts := []string{}
		for j := 0; j < len(values) && j < 5; j++ {
			parts = append(parts, fmt.Sprintf("%s %s%s", data.Categories[j], displayNumber(values[j]), item.Unit))
		}
		if len(values) > 5 {
			parts = append(parts, fmt.Sprintf("… (+%d)", len(values)-5))
		}
		if i < 3 {
			entries = append(entries, item.Name+" "+strings.Join(parts, ", "))
		}
	}
	if len(data.Series) > 3 {
		entries = append(entries, fmt.Sprintf("… (+%d)", len(data.Series)-3))
	}
	if p.ID == "G31" {
		for j := range data.Categories {
			total := 0.0
			for _, item := range series {
				total += item["data"].([]float64)[j]
			}
			if data.StackValues == "raw" {
				if total <= 0 {
					return nil, "", fmt.Errorf("percentage stack has a zero or negative total")
				}
				for _, item := range series {
					item["data"].([]float64)[j] = item["data"].([]float64)[j] * 100 / total
				}
			} else if math.Abs(total-100) > 0.01 {
				return nil, "", fmt.Errorf("already_percent values must total 100 per category")
			}
		}
		if data.StackValues == "raw" {
			entries = append(entries, "Σ → 100%")
		}
	}
	option["series"] = series
	return option, summary(title, entries), nil
}
