// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitableprotocol

import (
	"fmt"
	"sort"
)

// LayoutRegion requires the union of the selected cards to cover this rectangle.
// Empty ChartIDs selects all cards. Coordinates always use the root grid.
type LayoutRegion struct {
	ID       string   `json:"id"`
	X        int      `json:"x"`
	Y        int      `json:"y"`
	W        int      `json:"w"`
	H        int      `json:"h"`
	ChartIDs []string `json:"chartIds,omitempty"`
}

type LayoutRegionResult struct {
	ID               string `json:"id"`
	Covered          bool   `json:"covered"`
	UncoveredColumns []int  `json:"uncoveredColumns"`
}

// CompareRootLayout compares the complete declared design by ID and geometry.
// Intended overlaps may match a design even when the separate overlap check fails.
func CompareRootLayout(actual, expected []map[string]any, columns int) ([]LayoutIssue, error) {
	want := map[string]map[string]any{}
	for _, item := range expected {
		id, _ := item["i"].(string)
		if id == "" || want[id] != nil {
			return nil, fmt.Errorf("expected-layout 的 i 必须非空且唯一")
		}
		if err := ValidateRootChartLayout(item, columns); err != nil {
			return nil, err
		}
		want[id] = item
	}
	issues := []LayoutIssue{}
	seen := map[string]bool{}
	for _, item := range actual {
		id, _ := item["i"].(string)
		if seen[id] {
			issues = append(issues, LayoutIssue{ChartID: id, Code: "design_duplicate", Message: "实际布局 ID 重复"})
			continue
		}
		seen[id] = true
		design := want[id]
		if design == nil {
			issues = append(issues, LayoutIssue{ChartID: id, Code: "design_extra", Message: "实际布局含设计外卡片"})
			continue
		}
		for _, key := range []string{"x", "y", "w", "h"} {
			got, err := requiredLayoutInteger(item, key)
			expectedValue, _ := requiredLayoutInteger(design, key)
			if err != nil || got != expectedValue {
				issues = append(issues, LayoutIssue{ChartID: id, Code: "design_geometry", Message: key + " 与设计不符"})
			}
		}
		if err := ValidateRootChartLayout(item, columns); err != nil {
			issues = append(issues, LayoutIssue{ChartID: id, Code: "design_invalid", Message: err.Error()})
		}
	}
	// Keep missing-item diagnostics deterministic in the user's design order.
	for _, item := range expected {
		id := item["i"].(string)
		if !seen[id] {
			issues = append(issues, LayoutIssue{ChartID: id, Code: "design_missing", Message: "设计卡片未出现在实际布局"})
		}
	}
	return issues, nil
}

// CheckLayoutRegions verifies coverage by interval union in each grid column.
// It never sums overlapping areas, multiplies unbounded heights, or treats a
// region's bounding box as proof of coverage. Only valid root cards contribute.
func CheckLayoutRegions(items []map[string]any, regions []LayoutRegion, columns int) ([]LayoutRegionResult, error) {
	results := []LayoutRegionResult{}
	seen := map[string]bool{}
	identities := map[string]int{}
	for _, item := range items {
		id, _ := item["i"].(string)
		identities[id]++
	}
	for _, region := range regions {
		if region.ID == "" || seen[region.ID] || region.X < 0 || region.Y < 0 || region.W <= 0 || region.H <= 0 || region.W > columns || region.X > columns-region.W {
			return nil, fmt.Errorf("regions 的 id 必须唯一，坐标非负、宽高为正且位于根网格")
		}
		seen[region.ID] = true
		selected := map[string]bool{}
		for _, id := range region.ChartIDs {
			if id == "" || selected[id] {
				return nil, fmt.Errorf("regions.chartIds 必须为不同的非空 ID")
			}
			selected[id] = true
		}
		result := LayoutRegionResult{ID: region.ID, Covered: true, UncoveredColumns: []int{}}
		for column := region.X; column < region.X+region.W; column++ {
			intervals := [][2]int{}
			for _, item := range items {
				id, _ := item["i"].(string)
				if id == "" || identities[id] != 1 {
					continue
				}
				if len(selected) > 0 && !selected[id] {
					continue
				}
				if ValidateRootChartLayout(item, columns) != nil {
					continue
				}
				x, _ := requiredLayoutInteger(item, "x")
				w, _ := requiredLayoutInteger(item, "w")
				if column < x || column >= x+w {
					continue
				}
				y, _ := requiredLayoutInteger(item, "y")
				h, _ := requiredLayoutInteger(item, "h")
				start, end := 0, 0
				if y >= region.Y {
					start = y - region.Y
					if start >= region.H {
						continue
					}
					end = start + min(h, region.H-start)
				} else if offset := region.Y - y; offset < h {
					end = min(region.H, h-offset)
				}
				if end > start {
					intervals = append(intervals, [2]int{start, end})
				}
			}
			sort.Slice(intervals, func(i, j int) bool { return intervals[i][0] < intervals[j][0] })
			coveredUntil := 0
			for _, interval := range intervals {
				if interval[0] > coveredUntil {
					break
				}
				coveredUntil = max(coveredUntil, interval[1])
			}
			if coveredUntil < region.H {
				result.Covered = false
				result.UncoveredColumns = append(result.UncoveredColumns, column)
			}
		}
		results = append(results, result)
	}
	return results, nil
}
