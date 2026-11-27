// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package chartgen

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

func buildNamedValues(p Preset, title string, raw json.RawMessage) (map[string]any, string, error) {
	var data namedData
	if err := decodeStrict(raw, &data); err != nil || data.Kind != "named_values" {
		return nil, "", fmt.Errorf("expected named_values data: %v", err)
	}
	if err := validateNamedValueNumbers(raw); err != nil {
		return nil, "", err
	}
	limit := 24
	if p.SeriesTypes[0] == "funnel" {
		limit = 12
	}
	if len(data.Items) < 2 || len(data.Items) > limit {
		return nil, "", fmt.Errorf("named_values requires 2–%d items for %s", limit, p.SeriesTypes[0])
	}
	if err := optionalLabel(data.Unit, "unit"); err != nil {
		return nil, "", err
	}
	seen := map[string]bool{}
	items := make([]map[string]any, 0, len(data.Items))
	entries := []string{}
	positive := false
	last := math.Inf(1)
	for i, item := range data.Items {
		if err := validateLabel(item.Name, "item name", 80); err != nil {
			return nil, "", err
		}
		if seen[item.Name] || !finite(item.Value) || item.Value < 0 {
			return nil, "", fmt.Errorf("duplicate name or invalid value for %q", item.Name)
		}
		if p.SeriesTypes[0] == "funnel" && item.Value > last {
			return nil, "", fmt.Errorf("funnel stages must be in descending order")
		}
		last = item.Value
		seen[item.Name] = true
		positive = positive || item.Value > 0
		items = append(items, map[string]any{"name": item.Name, "value": item.Value})
		if i < 8 {
			entries = append(entries, fmt.Sprintf("%s %s%s", item.Name, displayNumber(item.Value), data.Unit))
		}
	}
	if !positive {
		return nil, "", fmt.Errorf("at least one item must be positive")
	}
	if len(items) > 8 {
		entries = append(entries, fmt.Sprintf("… (+%d)", len(items)-8))
	}
	option := baseOption(p, title)
	option["series"] = []map[string]any{styledSeries(p, 0, "", items)}
	return option, summary(title, entries), nil
}

func buildPoints(p Preset, title string, raw json.RawMessage) (map[string]any, string, error) {
	var data pointsData
	if err := decodeStrict(raw, &data); err != nil || data.Kind != "points" {
		return nil, "", fmt.Errorf("expected points data: %v", err)
	}
	if err := validatePointNumbers(raw); err != nil {
		return nil, "", err
	}
	if len(data.Groups) != len(p.SeriesTypes) {
		return nil, "", fmt.Errorf("expected %d point groups", len(p.SeriesTypes))
	}
	for _, field := range []struct{ value, name string }{{data.XLabel, "xLabel"}, {data.YLabel, "yLabel"}} {
		if err := validateLabel(field.value, field.name, 80); err != nil {
			return nil, "", err
		}
	}
	option := baseOption(p, title)
	option["xAxis"] = map[string]any{"type": "value", "name": data.XLabel}
	option["yAxis"] = map[string]any{"type": "value", "name": data.YLabel}
	series := make([]map[string]any, 0, len(data.Groups))
	entries := []string{}
	total := 0
	groupNames := map[string]bool{}
	for i, group := range data.Groups {
		if err := validateLabel(group.Name, "group name", 80); err != nil {
			return nil, "", err
		}
		if groupNames[group.Name] {
			return nil, "", fmt.Errorf("duplicate point group %q", group.Name)
		}
		groupNames[group.Name] = true
		if len(group.Points) == 0 {
			return nil, "", fmt.Errorf("group %q has no points", group.Name)
		}
		total += len(group.Points)
		if total > 2000 {
			return nil, "", fmt.Errorf("points chart exceeds 2000 points")
		}
		points := make([][2]float64, len(group.Points))
		for j, point := range group.Points {
			points[j] = point
		}
		series = append(series, styledSeries(p, i, group.Name, points))
		if i < 3 {
			entries = append(entries, fmt.Sprintf("%s (%d): (%s, %s), …", group.Name, len(points), displayNumber(points[0][0]), displayNumber(points[0][1])))
		}
	}
	option["series"] = series
	return option, summary(title, entries), nil
}

func buildMatrix(p Preset, title string, raw json.RawMessage) (map[string]any, string, error) {
	var data matrixData
	if err := decodeStrict(raw, &data); err != nil || data.Kind != "matrix" {
		return nil, "", fmt.Errorf("expected matrix data: %v", err)
	}
	if err := validateMatrixNumbers(raw); err != nil {
		return nil, "", err
	}
	if err := labels(data.XLabels, "xLabels", 1, 64); err != nil {
		return nil, "", err
	}
	if err := labels(data.YLabels, "yLabels", 1, 64); err != nil {
		return nil, "", err
	}
	if p.ID == "G43" && len(data.XLabels) != len(data.YLabels) {
		return nil, "", fmt.Errorf("G43 requires a square matrix")
	}
	if len(data.Cells) == 0 || len(data.Cells) > 2000 {
		return nil, "", fmt.Errorf("matrix requires 1–2000 cells")
	}
	seen := map[[2]int]bool{}
	points := make([][3]any, 0, len(data.Cells))
	min, max := math.Inf(1), math.Inf(-1)
	for _, cell := range data.Cells {
		key := [2]int{cell.X, cell.Y}
		if cell.X < 0 || cell.X >= len(data.XLabels) || cell.Y < 0 || cell.Y >= len(data.YLabels) || seen[key] || !finite(cell.Value) {
			return nil, "", fmt.Errorf("matrix has an invalid or duplicate coordinate (%d,%d)", cell.X, cell.Y)
		}
		seen[key] = true
		points = append(points, [3]any{cell.X, cell.Y, cell.Value})
		min = math.Min(min, cell.Value)
		max = math.Max(max, cell.Value)
	}
	if !data.Sparse && len(seen) != len(data.XLabels)*len(data.YLabels) {
		return nil, "", fmt.Errorf("matrix is incomplete; set sparse only when missing cells are intentional")
	}
	option := baseOption(p, title)
	option["xAxis"] = map[string]any{"type": "category", "data": data.XLabels}
	option["yAxis"] = map[string]any{"type": "category", "data": data.YLabels}
	visual := map[string]any{"min": min, "max": max}
	for key, value := range p.VisualMapStyle {
		visual[key] = value
	}
	option["visualMap"] = visual
	option["series"] = []map[string]any{styledSeries(p, 0, "", points)}
	return option, summary(title, []string{fmt.Sprintf("%d×%d", len(data.XLabels), len(data.YLabels)), fmt.Sprintf("(%d)", len(points)), fmt.Sprintf("[%s, %s]", displayNumber(min), displayNumber(max))}), nil
}

func buildOHLC(p Preset, title string, raw json.RawMessage) (map[string]any, string, error) {
	var data candleData
	if err := decodeStrict(raw, &data); err != nil || data.Kind != "ohlc" {
		return nil, "", fmt.Errorf("expected ohlc data: %v", err)
	}
	if err := validateOHLCNumbers(raw); err != nil {
		return nil, "", err
	}
	if err := labels(data.Categories, "categories", 1, 80); err != nil {
		return nil, "", err
	}
	if err := validateLabel(data.Name, "series name", 80); err != nil {
		return nil, "", err
	}
	if err := optionalLabel(data.Unit, "unit"); err != nil {
		return nil, "", err
	}
	if len(data.Candles) != len(data.Categories) {
		return nil, "", fmt.Errorf("OHLC count must match categories")
	}
	entries := []string{}
	for i, candle := range data.Candles {
		open, close, low, high := candle[0], candle[1], candle[2], candle[3]
		if !finite(open) || !finite(close) || !finite(low) || !finite(high) || low > math.Min(open, close) || high < math.Max(open, close) {
			return nil, "", fmt.Errorf("invalid OHLC tuple at %d", i)
		}
		if i < 5 {
			entries = append(entries, fmt.Sprintf("%s OHLC (%s, %s, %s, %s)%s", data.Categories[i], displayNumber(open), displayNumber(high), displayNumber(low), displayNumber(close), data.Unit))
		}
	}
	option := baseOption(p, title)
	option["xAxis"] = map[string]any{"type": "category", "data": data.Categories}
	yAxis := map[string]any{"type": "value"}
	if data.Unit != "" {
		yAxis["name"] = data.Unit
	}
	option["yAxis"] = yAxis
	option["series"] = []map[string]any{styledSeries(p, 0, data.Name, data.Candles)}
	if len(data.Candles) > 5 {
		entries = append(entries, fmt.Sprintf("… (+%d)", len(data.Candles)-5))
	}
	return option, summary(title, entries), nil
}

func buildRadar(p Preset, title string, raw json.RawMessage) (map[string]any, string, error) {
	var data radarData
	if err := decodeStrict(raw, &data); err != nil || data.Kind != "radar" {
		return nil, "", fmt.Errorf("expected radar data: %v", err)
	}
	if err := validateRadarNumbers(raw); err != nil {
		return nil, "", err
	}
	if len(data.Indicators) < 3 || len(data.Indicators) > 12 || len(data.Groups) != p.RadarGroupCount {
		return nil, "", fmt.Errorf("radar requires 3–12 indicators and %d groups", p.RadarGroupCount)
	}
	indicatorNames := make([]string, 0, len(data.Indicators))
	indicators := make([]map[string]any, 0, len(data.Indicators))
	for _, item := range data.Indicators {
		if !finite(item.Max) || item.Max <= 0 {
			return nil, "", fmt.Errorf("radar indicator max must be positive")
		}
		indicatorNames = append(indicatorNames, item.Name)
		indicators = append(indicators, map[string]any{"name": item.Name, "max": item.Max})
	}
	if err := labels(indicatorNames, "indicator names", 3, 12); err != nil {
		return nil, "", err
	}
	groups := make([]map[string]any, 0, len(data.Groups))
	entries := []string{}
	groupNames := map[string]bool{}
	for _, group := range data.Groups {
		if err := validateLabel(group.Name, "radar group name", 80); err != nil {
			return nil, "", err
		}
		if groupNames[group.Name] {
			return nil, "", fmt.Errorf("duplicate radar group %q", group.Name)
		}
		groupNames[group.Name] = true
		if len(group.Values) != len(data.Indicators) {
			return nil, "", fmt.Errorf("radar group %q length mismatch", group.Name)
		}
		for i, value := range group.Values {
			if !finite(value) || value < 0 || value > data.Indicators[i].Max {
				return nil, "", fmt.Errorf("radar group %q exceeds indicator bounds", group.Name)
			}
		}
		groups = append(groups, map[string]any{"name": group.Name, "value": group.Values})
		parts := []string{}
		for i := 0; i < len(group.Values) && i < 5; i++ {
			parts = append(parts, fmt.Sprintf("%s %s", indicatorNames[i], displayNumber(group.Values[i])))
		}
		entries = append(entries, group.Name+" "+strings.Join(parts, ", "))
	}
	option := baseOption(p, title)
	radar := map[string]any{"indicator": indicators}
	for key, value := range p.RadarStyle {
		radar[key] = value
	}
	option["radar"] = radar
	option["series"] = []map[string]any{styledSeries(p, 0, "", groups)}
	return option, summary(title, entries), nil
}

func buildMeasure(p Preset, title string, raw json.RawMessage) (map[string]any, string, error) {
	var data measureData
	if err := decodeStrict(raw, &data); err != nil || data.Kind != "measure" {
		return nil, "", fmt.Errorf("expected measure data: %v", err)
	}
	if err := validateMeasureNumbers(raw); err != nil {
		return nil, "", err
	}
	if err := validateLabel(data.Name, "measure name", 80); err != nil {
		return nil, "", err
	}
	if err := optionalLabel(data.Unit, "unit"); err != nil {
		return nil, "", err
	}
	if !finite(data.Min) || !finite(data.Max) || !finite(data.Value) || data.Min != 0 || data.Max <= 0 || data.Value < 0 || data.Value > data.Max {
		return nil, "", fmt.Errorf("gauge requires min=0, positive max, and a value inside that range")
	}
	option := baseOption(p, title)
	series := styledSeries(p, 0, "", []map[string]any{{"name": data.Name, "value": data.Value}})
	series["min"], series["max"] = data.Min, data.Max
	option["series"] = []map[string]any{series}
	return option, summary(title, []string{fmt.Sprintf("%s %s%s [%s, %s]", data.Name, displayNumber(data.Value), data.Unit, displayNumber(data.Min), displayNumber(data.Max))}), nil
}

func buildFlow(p Preset, title string, raw json.RawMessage) (map[string]any, string, error) {
	var data flowData
	if err := decodeStrict(raw, &data); err != nil || data.Kind != "flow" {
		return nil, "", fmt.Errorf("expected flow data: %v", err)
	}
	if err := validateFlowNumbers(raw); err != nil {
		return nil, "", err
	}
	if err := labels(data.Nodes, "nodes", 2, 64); err != nil {
		return nil, "", err
	}
	if len(data.Links) == 0 || len(data.Links) > 256 {
		return nil, "", fmt.Errorf("flow requires 1–256 links")
	}
	nodes := map[string]bool{}
	for _, node := range data.Nodes {
		nodes[node] = true
	}
	links := make([]map[string]any, 0, len(data.Links))
	entries := []string{}
	adjacency := map[string][]string{}
	indegree := map[string]int{}
	degree := map[string]int{}
	minWeight, maxWeight := math.Inf(1), 0.0
	for i, link := range data.Links {
		if !nodes[link.Source] || !nodes[link.Target] || link.Source == link.Target || !finite(link.Value) || link.Value <= 0 {
			return nil, "", fmt.Errorf("flow link %d has an unknown endpoint or invalid value", i)
		}
		links = append(links, map[string]any{"source": link.Source, "target": link.Target, "value": link.Value})
		adjacency[link.Source] = append(adjacency[link.Source], link.Target)
		indegree[link.Target]++
		degree[link.Source]++
		degree[link.Target]++
		minWeight = math.Min(minWeight, link.Value)
		maxWeight = math.Max(maxWeight, link.Value)
		if i < 5 {
			entries = append(entries, fmt.Sprintf("%s → %s %s", link.Source, link.Target, displayNumber(link.Value)))
		}
	}
	if minWeight/maxWeight < 1e-12 {
		return nil, "", fmt.Errorf("flow link weights span more than 12 orders of magnitude")
	}
	queue := []string{}
	for _, node := range data.Nodes {
		if degree[node] == 0 {
			return nil, "", fmt.Errorf("flow has an isolated node %q", node)
		}
		if indegree[node] == 0 {
			queue = append(queue, node)
		}
	}
	for i := 0; i < len(queue); i++ {
		for _, target := range adjacency[queue[i]] {
			indegree[target]--
			if indegree[target] == 0 {
				queue = append(queue, target)
			}
		}
	}
	if len(queue) != len(data.Nodes) {
		return nil, "", fmt.Errorf("flow contains a directed cycle")
	}
	option := baseOption(p, title)
	chartNodes := make([]map[string]any, 0, len(data.Nodes))
	for _, name := range data.Nodes {
		chartNodes = append(chartNodes, map[string]any{"name": name})
	}
	series := styledSeries(p, 0, "", chartNodes)
	series["links"] = links
	option["series"] = []map[string]any{series}
	if len(links) > 5 {
		entries = append(entries, fmt.Sprintf("… (+%d)", len(links)-5))
	}
	return option, summary(title, entries), nil
}
