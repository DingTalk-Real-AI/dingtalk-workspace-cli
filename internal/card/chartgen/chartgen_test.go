// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package chartgen

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/card/a2ui"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/skills"
)

func exampleData(p Preset) map[string]any {
	switch p.Family {
	case "category":
		series := []map[string]any{}
		for i := range p.SeriesTypes {
			series = append(series, map[string]any{"name": fmt.Sprintf("Current series %d", i+1), "unit": "items", "values": []float64{float64(i + 1), float64(i + 2)}})
		}
		data := map[string]any{"kind": "category", "categories": []string{"Current A", "Current B"}, "series": series}
		if p.ID == "G31" {
			data["stackValues"] = "raw"
		}
		if p.ID == "G34" {
			data["cumulativeMode"] = "increments"
		}
		return data
	case "named_values":
		return map[string]any{"kind": "named_values", "unit": "items", "items": []map[string]any{{"name": "Current A", "value": 8}, {"name": "Current B", "value": 5}}}
	case "points":
		groups := []map[string]any{}
		for i := range p.SeriesTypes {
			groups = append(groups, map[string]any{"name": fmt.Sprintf("Current group %d", i+1), "points": [][]float64{{1, 2}, {3, 4}}})
		}
		return map[string]any{"kind": "points", "xLabel": "Current X", "yLabel": "Current Y", "groups": groups}
	case "matrix":
		return map[string]any{"kind": "matrix", "xLabels": []string{"Current A", "Current B"}, "yLabels": []string{"Current C", "Current D"}, "cells": []map[string]any{{"x": 0, "y": 0, "value": 1}, {"x": 1, "y": 0, "value": 2}, {"x": 0, "y": 1, "value": 3}, {"x": 1, "y": 1, "value": 4}}}
	case "ohlc":
		return map[string]any{"kind": "ohlc", "categories": []string{"Current A", "Current B"}, "name": "Current price", "candles": [][]float64{{5, 6, 4, 7}, {6, 7, 5, 8}}}
	case "radar":
		groups := []map[string]any{}
		for i := 0; i < p.RadarGroupCount; i++ {
			groups = append(groups, map[string]any{"name": fmt.Sprintf("Current group %d", i+1), "values": []float64{20, 30, 40}})
		}
		return map[string]any{"kind": "radar", "indicators": []map[string]any{{"name": "Current A", "max": 100}, {"name": "Current B", "max": 100}, {"name": "Current C", "max": 100}}, "groups": groups}
	case "measure":
		return map[string]any{"kind": "measure", "name": "Current score", "unit": "points", "value": 75, "min": 0, "max": 100}
	case "flow":
		return map[string]any{"kind": "flow", "nodes": []string{"Current source", "Current target"}, "links": []map[string]any{{"source": "Current source", "target": "Current target", "value": 3}}}
	default:
		panic(p.Family)
	}
}

func changedData(p Preset) map[string]any {
	data := exampleData(p)
	switch p.Family {
	case "category":
		data["series"].([]map[string]any)[0]["values"].([]float64)[0]++
	case "named_values":
		data["items"].([]map[string]any)[0]["value"] = 9.0
	case "points":
		data["groups"].([]map[string]any)[0]["points"].([][]float64)[0][0]++
	case "matrix":
		data["cells"].([]map[string]any)[0]["value"] = 9.0
	case "ohlc":
		data["candles"].([][]float64)[0][0]++
	case "radar":
		data["groups"].([]map[string]any)[0]["values"].([]float64)[0]++
	case "measure":
		data["value"] = 76.0
	case "flow":
		data["links"].([]map[string]any)[0]["value"] = 4.0
	}
	return data
}

func TestCrossPlatformCoverageAllPresetsBuildFromCurrentFacts(t *testing.T) {
	source, err := fs.Sub(skills.FS, "multi/dingtalk-aicard")
	if err != nil {
		t.Fatal(err)
	}
	protocol, err := a2ui.Bundled(source)
	if err != nil {
		t.Fatal(err)
	}
	presets, err := List()
	if err != nil || len(presets) != 43 {
		t.Fatalf("presets: %d, %v", len(presets), err)
	}
	for _, preset := range presets {
		t.Run(preset.ID, func(t *testing.T) {
			input, err := json.Marshal(map[string]any{"specVersion": "1.0", "presetId": preset.ID, "title": "Current result", "data": exampleData(preset)})
			if err != nil {
				t.Fatal(err)
			}
			card, err := Build(input, "test-"+preset.ID)
			if err != nil {
				t.Fatal(err)
			}
			if card.PresetAlias != preset.Alias || card.SurfaceID != "test-"+preset.ID || len(card.FinishAnnotations) != 1 || card.FinishAnnotations[0].ComponentID != "summary" {
				t.Fatalf("incomplete artifact: %+v", card)
			}
			if !strings.Contains(card.Summary, "Current") || strings.Contains(card.CreateContent, "搜索") {
				t.Fatalf("summary or sample leakage: %s", card.Summary)
			}
			var createWire, finishWire []string
			if err := json.Unmarshal([]byte(card.CreateContent), &createWire); err != nil || len(createWire) != 1 {
				t.Fatalf("create wire: %v", err)
			}
			if err := json.Unmarshal([]byte(card.FinishContent), &finishWire); err != nil || len(finishWire) != 1 {
				t.Fatalf("finish wire: %v", err)
			}
			if !strings.Contains(finishWire[0], "test-"+preset.ID) {
				t.Fatalf("finish changed surface: %s", finishWire[0])
			}
			serialized, err := json.Marshal(card.CreateMessages)
			if err != nil {
				t.Fatal(err)
			}
			report, err := protocol.Lint(serialized, false, true)
			if err != nil || !report.Valid {
				t.Fatalf("schema: %+v %v", report.Diagnostics, err)
			}
			preflight, err := protocol.Preflight(report.A2UIMessages, "new-card")
			if err != nil || preflight["valid"] != true {
				t.Fatalf("preflight: %+v %v", preflight, err)
			}
			check := preflight["chartValidation"].(map[string]any)
			if check["checked"] != 1 || check["unverified"] != 0 {
				t.Fatalf("chart content unverified: %+v", check)
			}
			changedInput, _ := json.Marshal(map[string]any{"specVersion": "1.0", "presetId": preset.ID, "title": "Current result", "data": changedData(preset)})
			changed, err := Build(changedInput, "test-"+preset.ID)
			if err != nil || changed.CreateContent == card.CreateContent || changed.Summary == card.Summary {
				t.Fatalf("business fact change did not propagate through chart and summary: %v", err)
			}
		})
	}
}

func TestCrossPlatformCoverageRejectMissingFactsAndExcludedPresets(t *testing.T) {
	for _, preset := range []string{"G16", "G25", "G46"} {
		input, _ := json.Marshal(map[string]any{"specVersion": "1.0", "presetId": preset, "title": "Current result", "data": exampleData(Preset{Family: "measure"})})
		if _, err := Build(input, "test"); err == nil {
			t.Fatalf("accepted excluded preset %s", preset)
		}
	}
	bad := []byte(`{"specVersion":"1.0","presetId":"G01","title":"Current result","data":{"kind":"category","categories":["A"],"series":[]}}`)
	if _, err := Build(bad, "test"); err == nil {
		t.Fatal("accepted no series")
	}
	unknown := []byte(`{"specVersion":"1.0","presetId":"G01","title":"Current result","data":{"kind":"category","categories":["A"],"series":[],"rendererOption":{}}}`)
	if _, err := Build(unknown, "test"); err == nil {
		t.Fatal("accepted arbitrary renderer option")
	}
}

func TestCrossPlatformCoverageRejectNestRendererBoundaries(t *testing.T) {
	cases := []struct {
		id   string
		data map[string]any
		want string
	}{
		{"G01", func() map[string]any {
			d := exampleData(Preset{Family: "category", SeriesTypes: []string{"bar", "bar"}})
			labels := make([]string, 81)
			values := make([]float64, 81)
			for i := range labels {
				labels[i] = fmt.Sprintf("C%d", i)
				values[i] = 1
			}
			d["categories"] = labels
			for _, s := range d["series"].([]map[string]any) {
				s["values"] = values
			}
			return d
		}(), "1–80"},
		{"G13", func() map[string]any {
			d := exampleData(Preset{Family: "named_values"})
			items := make([]map[string]any, 25)
			for i := range items {
				items[i] = map[string]any{"name": fmt.Sprintf("P%d", i), "value": 1}
			}
			d["items"] = items
			return d
		}(), "2–24"},
		{"G26", func() map[string]any { d := exampleData(Preset{Family: "measure"}); d["min"] = 1; return d }(), "min=0"},
		{"G29", func() map[string]any {
			d := exampleData(Preset{Family: "flow"})
			d["links"] = []map[string]any{{"source": "Current source", "target": "Current target", "value": 1}, {"source": "Current target", "target": "Current source", "value": 1}}
			return d
		}(), "cycle"},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			raw, _ := json.Marshal(map[string]any{"specVersion": "1.0", "presetId": tc.id, "title": "Current result", "data": tc.data})
			_, err := Build(raw, "test")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %s, got %v", tc.want, err)
			}
		})
	}
}
