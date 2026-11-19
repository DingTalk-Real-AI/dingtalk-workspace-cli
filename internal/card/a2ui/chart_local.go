// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package a2ui

import (
	"encoding/json"
	"math"
	"regexp"
	"strings"
)

// Mirrors the local renderer safety checks in Nest a2ui_chart_contract.h.
// Presentation properties are not inferred from the preset sample registry.
var chartColorPattern = regexp.MustCompile(`^(#[0-9a-fA-F]{3}|#[0-9a-fA-F]{4}|#[0-9a-fA-F]{6}|#[0-9a-fA-F]{8}|[a-zA-Z]{1,32}|(?i:rgb|rgba|hsl|hsla)\([0-9.,% +\-]*\))$`)

func chartLocalError(option map[string]any) (string, string) {
	for _, key := range []string{"config", "dataset", "encode"} {
		if _, exists := option[key]; exists {
			return "chart.local_coordinates", "Unsupported or mixed chart data protocol"
		}
	}
	badReference := func(item map[string]any) bool {
		for _, name := range []string{"xAxis", "yAxis", "radar", "grid", "dataset", "polar", "geo", "calendar", "parallel"} {
			if _, exists := item[name+"Id"]; exists {
				return true
			}
			if value, exists := item[name+"Index"]; exists {
				n, ok := chartValue(value)
				if !ok || n != 0 {
					return true
				}
			}
		}
		return false
	}
	if grid, exists := option["grid"]; exists && grid != nil {
		if _, ok := grid.(map[string]any); !ok {
			return "chart.local_coordinates", "Grid must be an object"
		}
	}
	for _, name := range []string{"xAxis", "yAxis", "radar"} {
		axis := object(option[name])
		if badReference(axis) {
			return "chart.local_coordinates", "Unsupported coordinate reference"
		}
		if value, exists := axis["splitNumber"]; exists {
			n, ok := chartValue(value)
			if !ok || n < 1 || n > 100 || math.Floor(n) != n {
				return "chart.local_coordinates", "Invalid axis splitNumber"
			}
		}
	}
	for _, raw := range array(option["series"]) {
		row := object(raw)
		if _, exists := row["encode"]; exists {
			return "chart.local_coordinates", "Unsupported series encode"
		}
		if badReference(row) {
			return "chart.local_coordinates", "Unsupported coordinate reference"
		}
		kind := str(row["type"])
		coordinate := "none"
		if kind == "bar" || kind == "line" || kind == "scatter" || kind == "effectScatter" || kind == "heatmap" || kind == "candlestick" {
			coordinate = "cartesian2d"
		} else if kind == "radar" {
			coordinate = "radar"
		}
		if value, exists := row["coordinateSystem"]; exists && chartKnownType(kind) && str(value) != coordinate {
			return "chart.local_coordinates", "Unsupported coordinate system"
		}
	}
	nodes := 0
	var visit func(any, string, bool, int) bool
	visit = func(value any, kind string, labels bool, depth int) bool {
		nodes++
		if depth > 32 || nodes > 50000 {
			return false
		}
		switch v := value.(type) {
		case string:
			if kind == "color" {
				return len(v) <= 64 && chartColorPattern.MatchString(v)
			}
			return kind == "text" || !strings.ContainsAny(v, "\"'<>")
		case map[string]any:
			for key, child := range v {
				if strings.HasPrefix(key, "_chart") {
					return false
				}
				if strings.Contains("|fontSize|lineHeight|width|height|borderWidth|borderRadius|shadowBlur|shadowOffsetX|shadowOffsetY|symbolSize|", "|"+key+"|") {
					switch n := child.(type) {
					case float64:
						if math.Abs(n) > 16384 {
							return false
						}
					case json.Number:
						f, err := n.Float64()
						if err != nil || math.Abs(f) > 16384 {
							return false
						}
					}
				}
				childKind := "token"
				if key == "color" || len(key) > 5 && strings.HasSuffix(key, "Color") {
					childKind = "color"
				} else if strings.Contains("|name|text|subtext|formatter|source|target|", "|"+key+"|") || labels && key == "data" {
					childKind = "text"
				}
				container := strings.Contains("|xAxis|yAxis|radiusAxis|angleAxis|singleAxis|legend|", "|"+key+"|")
				if !visit(child, childKind, container, depth+1) {
					return false
				}
			}
		case []any:
			for _, child := range v {
				if !visit(child, kind, labels, depth+1) {
					return false
				}
			}
		}
		return true
	}
	if !visit(option, "token", false, 0) {
		return "chart.local_values", "Chart exceeds local structure/style limits or contains unsafe colors, reserved fields, or markup in tokens"
	}
	for _, name := range []string{"legend", "tooltip"} {
		value, exists := option[name]
		if !exists {
			continue
		}
		fields, ok := value.(map[string]any)
		if !ok {
			return "chart.local_options", "Legend and tooltip must be objects"
		}
		for key, value := range fields {
			if key == "show" {
				if _, ok := value.(bool); ok {
					continue
				}
			}
			if name == "legend" && key == "selectedMode" {
				if _, ok := value.(bool); ok {
					continue
				}
				if str(value) == "single" || str(value) == "multiple" {
					continue
				}
			}
			if name == "legend" && key == "selected" {
				if selected, ok := value.(map[string]any); ok {
					valid := true
					for _, item := range selected {
						if _, ok := item.(bool); !ok {
							valid = false
						}
					}
					if valid {
						continue
					}
				}
			}
			return "chart.local_options", "Unsupported local legend or tooltip option"
		}
	}
	if legend, exists := option["legend"].(map[string]any); exists && legend["show"] != false {
		names := map[string]bool{}
		for _, raw := range array(option["series"]) {
			row := object(raw)
			kind := str(row["type"])
			if kind == "pie" || kind == "funnel" || kind == "radar" {
				for _, item := range array(row["data"]) {
					if name := str(object(item)["name"]); name != "" {
						names[name] = true
					}
				}
			} else if name := str(row["name"]); name != "" {
				names[name] = true
			}
		}
		if len(names) > 256 {
			return "chart.local_options", "Local chart legend limit exceeded"
		}
	}
	return "", ""
}
