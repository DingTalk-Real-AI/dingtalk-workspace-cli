// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package render

import (
	"math"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/whiteboard/opennodes"
)

func TestCrossPlatformCoverageSVGBlockLineHeightAndBlankParagraphs(t *testing.T) {
	for _, carrier := range []string{"text", "shape", "stickyNote", "frame"} {
		t.Run(carrier, func(t *testing.T) {
			text := `{"blocks":[{"type":"paragraph","lineHeight":1,"runs":[{"text":" 检验 ","marks":{"fontSize":16}}]},{"type":"bulletList","lineHeight":2,"runs":[{"text":"触发","marks":{"fontSize":14}},{"text":"配置","marks":{"fontSize":24}}]},{"type":"orderedList","runs":[{"text":""}]}]}`
			field := `"text":` + text
			if carrier == "frame" {
				field = `"title":{"text":` + text + `}`
			}
			source := parseSource(t, `{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"n","type":"`+carrier+`","x":0,"y":0,"width":200,"height":200,`+field+`}]}`)
			result, err := SVG(source)
			if err != nil {
				t.Fatal(err)
			}
			svg := string(result.SVG)
			if result.PlaceholderCount != 0 || result.Fidelity != "approximate" {
				t.Fatalf("unsupported carrier: %#v", result)
			}
			if !strings.Contains(svg, `xml:space="preserve"`) || !strings.Contains(svg, "> 检验 </tspan>") || !strings.Contains(svg, `font-size="24"`) {
				t.Fatalf("lost rich text: %s", svg)
			}
			// One positioned tspan per paragraph, with the blank paragraph retaining its default 14*1.15 height.
			if strings.Count(svg, `data-line-height=`) != 3 || !strings.Contains(svg, `data-line-height="1.15"`) || !strings.Contains(svg, `data-line-height="2"`) {
				t.Fatalf("lost paragraph metrics: %s", svg)
			}
			if carrier == "text" && (!strings.Contains(svg, `y="67.95"`) || !strings.Contains(svg, `y="99.95"`) || !strings.Contains(svg, `y="132"`)) {
				t.Fatalf("unexpected line positions: %s", svg)
			}
			if !containsWarning(result.Warnings, "font_metrics_approximate") {
				t.Fatal("missing approximation warning")
			}
		})
	}
}

func TestCrossPlatformCoverageSVGLayoutRejectsMalformedDirectInputs(t *testing.T) {
	for _, tc := range []struct {
		value   any
		message string
	}{
		{map[string]any{"blocks": []any{nil}}, "invalid text block"},
		{map[string]any{"blocks": []any{map[string]any{"lineHeight": 0.5}}}, "invalid lineHeight"},
		{map[string]any{"blocks": []any{map[string]any{"runs": []any{nil}}}}, "invalid text run"},
		{map[string]any{"blocks": []any{map[string]any{"runs": []any{map[string]any{"text": "n", "marks": map[string]any{"fontSize": 0}}}}}}, "invalid fontSize"},
	} {
		// SVG is also a public Go entry point, whose callers can bypass Parse.
		source := &opennodes.Source{Nodes: []map[string]any{{"id": "n", "type": "text", "text": tc.value}}}
		if _, err := SVG(source); err == nil || !strings.Contains(err.Error(), tc.message) {
			t.Fatalf("expected %q: %v", tc.message, err)
		}
		renderer := &renderer{}
		if output := renderer.nodeTextAt(&item{node: map[string]any{}}, tc.value, 0, 0, "middle"); output != "" || renderer.layoutError == nil {
			t.Fatalf("malformed direct text emitted SVG: %q", output)
		}
	}
	blank := &opennodes.Source{Nodes: []map[string]any{{"id": "n", "type": "text", "text": map[string]any{"blocks": []any{map[string]any{"runs": []any{}}}}}}}
	result, err := SVG(blank)
	if err != nil || !strings.Contains(string(result.SVG), `data-line-height="1.15"`) {
		t.Fatalf("blank direct block lost height: %v", err)
	}
}

func TestCrossPlatformCoverageSVGRejectsNonFiniteDerivedPositions(t *testing.T) {
	source := &opennodes.Source{Nodes: []map[string]any{
		{"id": "a", "type": "shape", "x": -1e308}, {"id": "b", "type": "shape", "x": 1e308},
	}}
	if _, err := SVG(source); err == nil || !strings.Contains(err.Error(), "scene bounds") {
		t.Fatalf("overflowed viewport was accepted: %v", err)
	}
	text := map[string]any{"blocks": []any{
		map[string]any{"runs": []any{map[string]any{"text": "a", "marks": map[string]any{"fontSize": 5e307}}}},
		map[string]any{"runs": []any{map[string]any{"text": "b", "marks": map[string]any{"fontSize": 5e307}}}},
	}}
	source.Nodes = []map[string]any{{"id": "n", "type": "text", "x": 0, "y": math.MaxFloat64, "height": 10, "text": text}}
	if _, err := SVG(source); err == nil || !strings.Contains(err.Error(), "text position") {
		t.Fatalf("overflowed baseline was accepted: %v", err)
	}
}

func TestCrossPlatformCoverageSVGLineHeightExtendsViewportAndRejectsOverflow(t *testing.T) {
	source := parseSource(t, `{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"n","type":"text","x":0,"y":0,"width":200,"height":10,"text":{"blocks":[{"type":"paragraph","lineHeight":10,"runs":[{"text":"检验","marks":{"fontSize":20}}]},{"type":"paragraph","lineHeight":10,"runs":[{"text":"配置","marks":{"fontSize":20}}]}]}}]}`)
	result, err := SVG(source)
	if err != nil {
		t.Fatal(err)
	}
	if result.Bounds.Y > -195 || result.Bounds.Height < 400+2*DefaultPadding {
		t.Fatalf("text clipped by viewport: %#v", result.Bounds)
	}
	block := source.Nodes[0]["text"].(map[string]any)["blocks"].([]any)[0].(map[string]any)
	block["lineHeight"] = 1e308
	if _, err := SVG(source); err == nil {
		t.Fatal("direct SVG caller accepted overflowing text layout")
	}
}
