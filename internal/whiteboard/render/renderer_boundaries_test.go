// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package render

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/whiteboard/opennodes"
)

func TestCrossPlatformCoverageSVGIdentityAndResourceBounds(t *testing.T) {
	if _, err := SVG(nil); err == nil {
		t.Fatal("nil source accepted")
	}
	if _, err := SVG(&opennodes.Source{Nodes: []map[string]any{{"id": "t", "type": "text", "text": strings.Repeat("x", maximumSVGBytes)}}}); err == nil {
		t.Fatal("oversized SVG accepted")
	}
	result, err := SVG(&opennodes.Source{Nodes: []map[string]any{
		{"type": ""}, {"id": "same", "type": "shape"}, {"id": "same", "type": "shape"},
		{"id": "hidden", "type": "shape", "hidden": true},
		{"id": "missing-parent", "type": "text", "parentId": "missing"},
		{"id": "invalid-parent", "type": "text", "parentId": "same"},
	}})
	if err != nil || result.RenderedCount != 5 || result.PlaceholderCount != 2 {
		t.Fatalf("identity result=%#v err=%v", result, err)
	}
	for _, code := range []string{"missing_node_id", "duplicate_node_id", "missing_parent", "invalid_parent_type"} {
		if !strings.Contains(strings.Join(SortedWarningCodes(result.Warnings), ","), code) {
			t.Errorf("missing warning %s", code)
		}
	}
	r := &renderer{items: []*item{{worldX: math.Inf(1), worldY: math.Inf(1), width: 1, height: 1}}}
	if got := r.sceneBounds(); got.Width != 640 || got.Height != 360 {
		t.Fatalf("nonfinite bounds=%#v", got)
	}
}

func TestCrossPlatformCoverageSVGSupportedAndFallbackShapes(t *testing.T) {
	for _, tc := range []struct {
		kind, geometry, catalog, fragment string
		placeholder                       bool
	}{
		{"sticky", "", "", "#fef3c7", false}, {"shape", "dml:diamond", "", "<polygon", false},
		{"shape", "unsupported", "", "不支持 geometry", true},
		{"icon", "", "task/task-done", "<circle", false}, {"icon", "", "task/task", "rx=\"4\"", false},
		{"icon", "", "unknown", "不支持 icon", true},
	} {
		t.Run(tc.kind+tc.geometry+tc.catalog, func(t *testing.T) {
			result, err := SVG(&opennodes.Source{Nodes: []map[string]any{{"id": "n", "type": tc.kind, "geometry": tc.geometry, "catalogId": tc.catalog}}})
			if err != nil || !strings.Contains(string(result.SVG), tc.fragment) || (result.PlaceholderCount > 0) != tc.placeholder {
				t.Fatalf("result=%#v err=%v", result, err)
			}
		})
	}
	for _, path := range []map[string]any{{"data": "M0 0 L10 10"}, {"data": "M0 0 L10 10", "intrinsicWidth": 10, "intrinsicHeight": 10}} {
		result, err := SVG(&opennodes.Source{Nodes: []map[string]any{{"id": "p", "type": "path", "path": path}}})
		if err != nil {
			t.Fatal(err)
		}
		if path["intrinsicWidth"] != nil && !strings.Contains(string(result.SVG), "scale(16 8)") {
			t.Fatal("path scaling missing")
		}
		if path["intrinsicWidth"] == nil && result.PlaceholderCount != 1 {
			t.Fatal("invalid intrinsic size not replaced")
		}
	}
	result, err := SVG(&opennodes.Source{Nodes: []map[string]any{{"id": "text", "type": "shape", "angle": 45, "text": map[string]any{"text": "one\ntwo"}, "style": map[string]any{"fontSize": 20}}}})
	if err != nil || result.ApproximateCount != 1 || !strings.Contains(string(result.SVG), "rotate(45") || !strings.Contains(string(result.SVG), `dy="25"`) {
		t.Fatalf("styled text=%#v err=%v", result, err)
	}
}

func TestCrossPlatformCoverageSVGConnectorRoutingAndAnchors(t *testing.T) {
	endpoint := func(x int) map[string]any {
		return map[string]any{"type": "point", "point": map[string]any{"x": x, "y": x}, "marker": map[string]any{"catalogId": "unsupported"}}
	}
	for _, routing := range []string{"curve", "orthogonal", "straight"} {
		result, err := SVG(&opennodes.Source{Nodes: []map[string]any{{"id": "c", "type": "connector", "start": endpoint(0), "end": endpoint(100), "routing": routing}}})
		if err != nil || result.ApproximateCount != 1 || result.Fidelity != "approximate" {
			t.Fatalf("%s: %#v %v", routing, result, err)
		}
	}
	r := &renderer{byID: map[string]*item{"n": {worldX: 10, worldY: 20, width: 100, height: 80}}}
	for side, want := range map[string]point{"top": {60, 20}, "bottom": {60, 100}, "left": {10, 60}, "right": {110, 60}, "center": {60, 60}} {
		got, ok := r.endpoint(map[string]any{"type": "node", "nodeRef": map[string]any{"id": "n"}, "anchor": map[string]any{"side": side}})
		if !ok || got != want {
			t.Fatalf("anchor %s=%v/%v", side, got, ok)
		}
	}
	for _, bad := range []any{nil, map[string]any{"type": "unknown"}, map[string]any{"type": "point", "point": nil}, map[string]any{"type": "node", "nodeRef": map[string]any{"id": "missing"}}} {
		if _, ok := r.endpoint(bad); ok {
			t.Fatalf("invalid endpoint accepted: %#v", bad)
		}
	}
	for _, waypoints := range [][]any{{map[string]any{"x": 5, "y": 8}}, {"invalid"}} {
		n := &item{nodeType: "connector", node: map[string]any{"start": endpoint(0), "end": endpoint(10), "waypoints": waypoints}}
		p, ok := r.connectorPoints(n)
		if _, valid := waypoints[0].(map[string]any); ok != valid || (ok && len(p) != 3) {
			t.Fatalf("waypoints=%v/%v", p, ok)
		}
	}
	if out := r.renderConnector(&item{nodeType: "connector", node: map[string]any{}}); !strings.Contains(out, "端点无法") {
		t.Fatal(out)
	}
}

func TestCrossPlatformCoverageSVGStyleSafety(t *testing.T) {
	for _, tc := range []struct {
		style        map[string]any
		fill, stroke string
		width        float64
		approx       bool
	}{
		{map[string]any{"fill": map[string]any{"type": "none"}}, "none", "#000", 1, false},
		{map[string]any{"fill": map[string]any{"color": "url(evil)"}}, "#fff", "#000", 1, true},
		{map[string]any{"fill": map[string]any{"type": "gradient"}}, "#fff", "#000", 1, true},
		{map[string]any{"stroke": map[string]any{"paint": map[string]any{"color": "#abc"}, "width": 3}}, "#fff", "#abc", 3, false},
		{map[string]any{"stroke": map[string]any{"paint": map[string]any{"color": "evil"}, "width": 0}}, "#fff", "#000", 1, true},
		{map[string]any{"stroke": map[string]any{"paint": map[string]any{"type": "gradient"}}}, "#fff", "#000", 1, true},
		{map[string]any{"effects": []any{}}, "#fff", "#000", 1, true},
	} {
		f, s, w, a := style(map[string]any{"style": tc.style}, "#fff", "#000", 1)
		if f != tc.fill || s != tc.stroke || w != tc.width || a != tc.approx {
			t.Fatalf("style=%v %v %v %v", f, s, w, a)
		}
		if tc.approx {
			res, err := SVG(&opennodes.Source{Nodes: []map[string]any{{"id": "n", "type": "shape", "style": tc.style}}})
			if err != nil || res.ApproximateCount != 1 {
				t.Fatalf("approximation=%#v %v", res, err)
			}
		}
	}
	for _, id := range []string{"arrow.open", "none", "unknown"} {
		attr, approx := markerAttribute(map[string]any{"marker": map[string]any{"catalogId": id}}, "marker-end")
		if (id == "arrow.open" && !strings.Contains(attr, "arrow-open")) || approx != (id == "unknown") {
			t.Fatalf("marker %s=%s %v", id, attr, approx)
		}
	}
	for _, v := range []any{float64(2), float32(2), int(2), int64(2), json.Number("2")} {
		if n, ok := finiteNumber(v); !ok || n != 2 {
			t.Fatalf("number %v", v)
		}
	}
	if !reflect.DeepEqual(textLines(map[string]any{"text": "a\r\nb"}), []string{"a", "b"}) {
		t.Fatal("nested text lines")
	}
	if truncate("你好世界", 3) != "你好…" || printableType("") != "unknown" {
		t.Fatal("placeholder labels")
	}
}
