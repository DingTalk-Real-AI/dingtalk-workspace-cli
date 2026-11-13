// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitableprotocol

import (
	"math"
	"testing"
)

func designCard(id string, x, y, w, h int) map[string]any {
	return map[string]any{"i": id, "x": x, "y": y, "w": w, "h": h}
}

func TestCrossPlatformCoverageLayoutDesignAndRegionUnion(t *testing.T) {
	a := designCard("A", 0, 0, 6, 2)
	b := designCard("B", 0, 2, 6, 2)
	if issues, err := CompareRootLayout([]map[string]any{b, a}, []map[string]any{a, b}, 12); err != nil || len(issues) != 0 {
		t.Fatalf("ID comparison depends on array order: %v %v", issues, err)
	}
	if issues, err := CompareRootLayout([]map[string]any{a, a, designCard("C", 8, 0, 2, 2)}, []map[string]any{a, b}, 12); err != nil || len(issues) != 3 {
		t.Fatalf("missing/extra/duplicate: %v %v", issues, err)
	}
	if issues, _ := CompareRootLayout([]map[string]any{designCard("A", 1, 0, 5, 2)}, []map[string]any{a}, 12); len(issues) != 2 {
		t.Fatalf("position/size drift lost: %v", issues)
	}
	if _, err := CompareRootLayout(nil, []map[string]any{a, a}, 12); err == nil {
		t.Fatal("ambiguous design accepted")
	}
	if _, err := CompareRootLayout(nil, []map[string]any{designCard("A", 12, 0, 1, 1)}, 12); err == nil {
		t.Fatal("invalid design accepted")
	}
	region := LayoutRegion{ID: "left", X: 0, Y: 0, W: 6, H: 4}
	covered, err := CheckLayoutRegions([]map[string]any{a, b}, []LayoutRegion{region}, 12)
	if err != nil || !covered[0].Covered {
		t.Fatalf("adjacent union: %v %v", covered, err)
	}
	covered, err = CheckLayoutRegions([]map[string]any{a, a}, []LayoutRegion{region}, 12)
	if err != nil || covered[0].Covered || len(covered[0].UncoveredColumns) != 6 {
		t.Fatalf("overlap counted twice: %v %v", covered, err)
	}
	region.ChartIDs = []string{"A"}
	covered, err = CheckLayoutRegions([]map[string]any{a, b}, []LayoutRegion{region}, 12)
	if err != nil || covered[0].Covered {
		t.Fatalf("selector ignored: %v %v", covered, err)
	}
	region.ChartIDs = []string{"A", "A"}
	if _, err := CheckLayoutRegions(nil, []LayoutRegion{region}, 12); err == nil {
		t.Fatal("duplicate selector accepted")
	}
	if _, err := CheckLayoutRegions(nil, []LayoutRegion{{ID: "outside", X: 12, W: 1, H: 1}}, 12); err == nil {
		t.Fatal("outside region accepted")
	}
	covered, err = CheckLayoutRegions([]map[string]any{designCard("huge", 0, 1, 12, math.MaxInt)}, []LayoutRegion{{ID: "extreme", X: 0, Y: math.MaxInt - 3, W: 12, H: 3}}, 12)
	if err != nil || !covered[0].Covered {
		t.Fatalf("height overflow changed coverage: %v %v", covered, err)
	}
}
