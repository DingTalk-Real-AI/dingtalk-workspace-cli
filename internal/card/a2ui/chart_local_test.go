// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package a2ui

import (
	"encoding/json"
	"testing"
)

func TestReviewLocalPresentationAndSafety(t *testing.T) {
	p := testProtocol(t)
	for _, extra := range []map[string]any{
		{"tooltip": map[string]any{"show": true}}, {"grid": map[string]any{"left": 10.0}},
		{"color": []any{"#123456"}}, {"title": map[string]any{"subtext": "Detail"}},
		{"yAxis": map[string]any{"type": "value", "min": 0.0}},
		{"xAxis": map[string]any{"type": "category", "data": []any{"A"}, "axisLabel": map[string]any{"rotate": 30.0}}},
	} {
		option := chartTestOption("bar", []any{1500000.0})
		for key, value := range extra {
			option[key] = value
		}
		chartTestCode(t, option, "")
		for _, binding := range []bool{false, true} {
			var data any = option
			if binding {
				data = map[string]any{"path": "/chart"}
			}
			message, err := json.Marshal(map[string]any{"version": "v1.0", "createSurface": map[string]any{
				"surfaceId": "s", "catalogId": "https://dingtalk.com/card/a2ui/catalogs/public/catalog.json",
				"dataModel":  map[string]any{"chart": option},
				"components": []any{map[string]any{"id": "root", "component": "Chart", "data": data}},
			}})
			if err != nil {
				t.Fatal(err)
			}
			report, err := p.Lint(append(append([]byte{'['}, message...), ']'), false, false)
			if err != nil || !report.Valid {
				t.Fatalf("binding=%v err=%v report=%#v", binding, err, report)
			}
			preflight, err := p.Preflight([]string{string(message)}, "new-card")
			if err != nil || preflight["valid"] != true {
				t.Fatalf("binding=%v err=%v report=%#v", binding, err, preflight)
			}
		}
	}
	for _, extra := range []map[string]any{
		{"legend": map[string]any{"top": 10.0}}, {"legend": map[string]any{"data": []any{"A"}}},
		{"visualMap": map[string]any{"inRange": map[string]any{"color": []any{"url(evil)"}}}},
		{"_chartInternal": true}, {"grid": map[string]any{"width": 1e20}}, {"dataset": map[string]any{}},
	} {
		option := chartTestOption("bar", []any{1.0})
		for key, value := range extra {
			option[key] = value
		}
		if code, _ := chartOptionError(option); code == "" {
			t.Fatalf("accepted unsafe chart: %#v", extra)
		}
	}
}
