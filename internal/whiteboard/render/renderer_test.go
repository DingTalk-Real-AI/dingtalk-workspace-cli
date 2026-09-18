// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package render

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/whiteboard/opennodes"
)

func parseSource(t *testing.T, raw string) *opennodes.Source {
	t.Helper()
	source, err := opennodes.Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func TestCrossPlatformCoverageOpenNodesSVGGoldenAndDeterminism(t *testing.T) {
	source := parseSource(t, `{
		"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[
			{"id":"frame","type":"frame","x":60,"y":100,"width":400,"height":220,"title":{"text":{"blocks":[{"type":"paragraph","runs":[{"text":"阶段"}]}]}}},
			{"id":"child","type":"shape","parentId":"frame","x":40,"y":50,"width":120,"height":60,"geometry":"dml:roundRect","style":{"fill":{"type":"solid","color":"#DBEAFE"}},"text":{"blocks":[{"type":"paragraph","runs":[{"text":"开始"}]}]}},
			{"id":"outside","type":"shape","x":520,"y":160,"width":120,"height":60,"geometry":"dml:rect"},
			{"id":"line","type":"connector","start":{"type":"node","nodeRef":{"scope":"request","id":"child"},"anchor":{"mode":"fixed","side":"right"}},"end":{"type":"node","nodeRef":{"scope":"request","id":"outside"},"anchor":{"mode":"fixed","side":"left"},"marker":{"catalogId":"arrow.filled"}},"routing":"straight"},
			{"id":"photo","type":"image","x":520,"y":260,"width":100,"height":80,"url":"https://secret.example/image.png"}
		]}`)
	first, err := SVG(source)
	if err != nil {
		t.Fatal(err)
	}
	second, err := SVG(source)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.SVG, second.SVG) {
		t.Fatal("same OpenNodes source produced different SVG bytes")
	}
	if first.Bounds != (Bounds{X: 36, Y: 76, Width: 628, Height: 288}) {
		t.Fatalf("bounds=%#v", first.Bounds)
	}
	if first.Fidelity != "placeholder" || first.NodeCount != 5 || first.RenderedCount != 5 || first.ExactCount != 2 || first.ApproximateCount != 2 || first.PlaceholderCount != 1 {
		t.Fatalf("result=%#v", first)
	}
	if strings.Contains(string(first.SVG), "secret.example") {
		t.Fatal("renderer leaked an external image URL")
	}
	sum := sha256.Sum256(first.SVG)
	gotGolden := hex.EncodeToString(sum[:])
	const wantGolden = "cf795dd2403ac4f66f050ad940e163bc68b74b3ca02f2880100f068a527d751a"
	if gotGolden != wantGolden {
		t.Fatalf("SVG golden digest=%s", gotGolden)
	}
}

func TestCrossPlatformCoverageOpenNodesSVGEscapesInjectionAndUsesPlaceholders(t *testing.T) {
	source := parseSource(t, `{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[
		{"id":"text","type":"text","x":0,"y":0,"width":200,"height":60,"text":"</text><script>alert(1)</script>&"},
		{"id":"path","type":"path","x":0,"y":100,"width":100,"height":40,"path":{"data":"M0 0\" onload=\"alert(1)","intrinsicWidth":100,"intrinsicHeight":40}},
		{"id":"future","type":"future-widget","x":140,"y":100,"width":100,"height":40}
	]}`)
	result, err := SVG(source)
	if err != nil {
		t.Fatal(err)
	}
	svg := string(result.SVG)
	if strings.Contains(svg, "<script>") || strings.Contains(svg, "onload=") {
		t.Fatalf("unsafe SVG content: %s", svg)
	}
	if !strings.Contains(svg, "&lt;/text&gt;&lt;script&gt;") {
		t.Fatalf("text was not XML escaped: %s", svg)
	}
	decoder := xml.NewDecoder(strings.NewReader(svg))
	for {
		if _, err := decoder.Token(); err != nil {
			if err.Error() == "EOF" {
				break
			}
			t.Fatalf("SVG is not well-formed XML: %v", err)
		}
	}
	if result.PlaceholderCount != 2 || result.Fidelity != "placeholder" {
		t.Fatalf("result=%#v", result)
	}
	codes := SortedWarningCodes(result.Warnings)
	if strings.Join(codes, ",") != "font_metrics_approximate,placeholder_rendered" {
		t.Fatalf("warning codes=%v", codes)
	}
}

func TestCrossPlatformCoverageOpenNodesSVGParentCycleAndLimits(t *testing.T) {
	cycle := parseSource(t, `{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[
		{"id":"a","type":"group","parentId":"b","x":1,"y":2,"width":20,"height":20},
		{"id":"b","type":"group","parentId":"a","x":3,"y":4,"width":20,"height":20}
	]}`)
	result, err := SVG(cycle)
	if err != nil {
		t.Fatal(err)
	}
	if !containsWarning(result.Warnings, "parent_cycle") {
		t.Fatalf("warnings=%#v", result.Warnings)
	}

	total := maximumNodeCount + 1
	nodes := make([]map[string]any, total)
	for index := range nodes {
		nodes[index] = map[string]any{"id": "n", "type": "shape"}
	}
	if _, err := SVG(&opennodes.Source{Nodes: nodes}); err == nil {
		t.Fatal("node limit was not enforced")
	}
}

func containsWarning(warnings []Warning, code string) bool {
	for _, warning := range warnings {
		if warning.Code == code {
			return true
		}
	}
	return false
}
