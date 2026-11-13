// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitableprotocol

import "fmt"

type LayoutIssue struct {
	ChartID string `json:"chartId"`
	OtherID string `json:"otherId,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// DiagnoseRootLayout checks geometry and declared minW/minH in the current
// snapshot. It never claims component readability or guesses child grid widths.
func DiagnoseRootLayout(items []map[string]any, columns int) []LayoutIssue {
	issues := []LayoutIssue{}
	type rect struct {
		id         string
		x, y, w, h int
	}
	valid := []rect{}
	seen := map[string]bool{}
	for _, item := range items {
		id, _ := item["i"].(string)
		if id == "" || seen[id] {
			issues = append(issues, LayoutIssue{ChartID: id, Code: "invalid_identity", Message: "布局 ID 缺失或重复"})
			continue
		}
		seen[id] = true
		if err := ValidateRootChartLayout(item, columns); err != nil {
			issues = append(issues, LayoutIssue{ChartID: id, Code: "invalid_geometry", Message: err.Error()})
			continue
		}
		x, _ := requiredLayoutInteger(item, "x")
		y, _ := requiredLayoutInteger(item, "y")
		w, _ := requiredLayoutInteger(item, "w")
		h, _ := requiredLayoutInteger(item, "h")
		for _, dimension := range []string{"minW", "minH"} {
			size := w
			if dimension == "minH" {
				size = h
			}
			if _, exists := item[dimension]; !exists {
				continue
			}
			minimum, err := requiredLayoutInteger(item, dimension)
			if err != nil || minimum <= 0 {
				issues = append(issues, LayoutIssue{ChartID: id, Code: "invalid_constraint", Message: dimension + " 必须是正整数"})
			} else if size < minimum {
				issues = append(issues, LayoutIssue{ChartID: id, Code: "below_minimum", Message: fmt.Sprintf("%s=%d，实际尺寸 %d", dimension, minimum, size)})
			}
		}
		for _, prev := range valid {
			// Subtraction keeps extreme y/h values from overflowing.
			vertical := (y >= prev.y && y-prev.y < prev.h) || (prev.y >= y && prev.y-y < h)
			if x < prev.x+prev.w && prev.x < x+w && vertical {
				issues = append(issues, LayoutIssue{ChartID: id, OtherID: prev.id, Code: "overlap", Message: "根布局卡片重叠"})
			}
		}
		valid = append(valid, rect{id, x, y, w, h})
	}
	return issues
}

// DiagnoseChartMinimumSizes reports only confirmed type/version rules; unknown types and child layouts remain unchecked.
func DiagnoseChartMinimumSizes(items, charts []map[string]any, columns int) ([]LayoutIssue, []string) {
	issues, checked := []LayoutIssue{}, []string{}
	if columns != DashboardGridColumnsV1 && columns != DashboardGridColumnsV2 {
		return issues, checked
	}
	types, seen := map[string]string{}, map[string]bool{}
	for _, chart := range charts {
		id, _ := chart["chartId"].(string)
		kind, _ := chart["chartType"].(string)
		if id == "" {
			continue
		}
		if seen[id] {
			delete(types, id)
			continue
		}
		seen[id], types[id] = true, kind
	}
	layoutIDs := map[string]int{}
	for _, item := range items {
		id, _ := item["i"].(string)
		layoutIDs[id]++
	}
	for _, item := range items {
		id, _ := item["i"].(string)
		if id == "" || layoutIDs[id] != 1 || ValidateRootChartLayout(item, columns) != nil {
			continue
		}
		minW, minH := 0, 0
		switch types[id] {
		case "BAR", "LINE", "HISTOGRAM", "PIE":
			minW, minH = 3, 4
		case "SUMMARY":
			minW, minH = 2, 4
		default:
			continue
		}
		if columns == DashboardGridColumnsV2 {
			minW *= 4
			if types[id] == "SUMMARY" {
				minH = 8
			} else {
				minH *= 3
			}
		}
		checked = append(checked, id)
		w, _ := requiredLayoutInteger(item, "w")
		h, _ := requiredLayoutInteger(item, "h")
		if w < minW || h < minH {
			issues = append(issues, LayoutIssue{ChartID: id, Code: "below_type_minimum", Message: fmt.Sprintf("%s 最小尺寸 %dx%d，实际 %dx%d；仅提示，不自动调整布局", types[id], minW, minH, w, h)})
		}
	}
	return issues, checked
}
