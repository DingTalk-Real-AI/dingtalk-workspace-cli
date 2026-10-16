// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package render

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageOpenNodesSVGRotatedBounds(t *testing.T) {
	for _, tc := range []struct {
		name, node string
		want       Bounds
		rotated    bool
	}{
		{"rectangle", `"type":"shape","geometry":"dml:rect","angle":90`, Bounds{-24, -154, 348, 348}, true},
		{"diagonal", `"type":"shape","geometry":"dml:diamond","angle":45`, Bounds{-24, -124.20815280171308, 348, 288.41630560342617}, true},
		{"negative-angle", `"type":"sticky","angle":-90`, Bounds{-24, -154, 348, 348}, true},
		{"text", `"type":"text","text":"hello","angle":90`, Bounds{-24, -154, 348, 348}, true},
		{"frame", `"type":"frame","angle":90`, Bounds{-24, -154, 348, 348}, true},
		{"group", `"type":"group","angle":90`, Bounds{-24, -154, 348, 348}, true},
		{"placeholder", `"type":"image","angle":90`, Bounds{-24, -154, 348, 348}, true},
		{"invalid-path", `"type":"path","angle":90`, Bounds{-24, -154, 348, 348}, true},
		{"task", `"type":"icon","catalogId":"task/task","angle":90`, Bounds{-24, -154, 348, 348}, true},
		{"unrotated-path", `"type":"path","angle":90,"path":{"data":"M 0 0 L 300 40","intrinsicWidth":300,"intrinsicHeight":40}`, Bounds{-24, -24, 348, 88}, false},
		{"unrotated-done-icon", `"type":"icon","catalogId":"task/task-done","angle":90`, Bounds{-24, -24, 348, 88}, false},
		{"connector-endpoints", `"type":"connector","angle":90,"start":{"type":"point","point":{"x":0,"y":0}},"end":{"type":"point","point":{"x":100,"y":0}}`, Bounds{-24, -24, 148, 48}, false},
		{"zero", `"type":"shape","angle":0`, Bounds{-24, -24, 348, 88}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := parseSource(t, fmt.Sprintf(`{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"card","x":0,"y":0,"width":300,"height":40,%s}]}`, tc.node))
			result, err := SVG(source)
			if err != nil {
				t.Fatal(err)
			}
			got := result.Bounds
			if math.Abs(got.X-tc.want.X) > 1e-9 || math.Abs(got.Y-tc.want.Y) > 1e-9 || math.Abs(got.Width-tc.want.Width) > 1e-9 || math.Abs(got.Height-tc.want.Height) > 1e-9 {
				t.Fatalf("bounds=%+v want=%+v", got, tc.want)
			}
			if strings.Contains(string(result.SVG), `transform="rotate(`) != tc.rotated {
				t.Fatalf("unexpected rotation in %s", result.SVG)
			}
		})
	}
}

func TestCrossPlatformCoverageOpenNodesSVGRotationUsesWorldPosition(t *testing.T) {
	source := parseSource(t, `{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"parent","type":"frame","x":100,"y":200,"width":300,"height":40,"hidden":true},{"id":"child","type":"shape","parentId":"parent","x":0,"y":0,"width":300,"height":40,"angle":90}]}`)
	result, err := SVG(source)
	if err != nil {
		t.Fatal(err)
	}
	if result.Bounds != (Bounds{76, 46, 348, 348}) {
		t.Fatalf("world bounds=%+v", result.Bounds)
	}
	if !strings.Contains(string(result.SVG), `rotate(90 250 220)`) {
		t.Fatalf("world rotation missing: %s", result.SVG)
	}
}
