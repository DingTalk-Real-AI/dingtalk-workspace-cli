// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package a2ui

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageChartNestParity(t *testing.T) {
	raw, err := os.ReadFile("testdata/chart-nest-parity.json")
	if err != nil {
		t.Fatal(err)
	}
	var suite struct {
		Cases []struct {
			ID          string         `json:"id"`
			Option      map[string]any `json:"option"`
			NestValid   bool           `json:"nestValid"`
			Status      string         `json:"status"`
			RepeatTitle int            `json:"repeatTitle"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &suite); err != nil {
		t.Fatal(err)
	}
	p := testProtocol(t)
	results := map[string]bool{}
	for _, tc := range suite.Cases {
		t.Run(tc.ID, func(t *testing.T) {
			if tc.RepeatTitle > 0 {
				title := object(tc.Option["title"])
				title["text"] = strings.Repeat(str(title["text"]), tc.RepeatTitle)
			}
			for _, bound := range []bool{false, true} {
				var data any = tc.Option
				if bound {
					data = map[string]any{"path": "/chart"}
				}
				doc, err := json.Marshal(map[string]any{"version": "v1.0", "createSurface": map[string]any{
					"surfaceId": "parity", "catalogId": "https://dingtalk.com/card/a2ui/catalogs/public/catalog.json",
					"dataModel":  map[string]any{"chart": tc.Option},
					"components": []any{map[string]any{"id": "root", "component": "Chart", "data": data}},
				}})
				if err != nil {
					t.Fatal(err)
				}
				report, err := p.Lint(append(append([]byte{'['}, doc...), ']'), false, true)
				if err != nil || !report.Valid {
					t.Fatalf("schema: %v %#v", err, report)
				}
				pf, err := p.Preflight(report.A2UIMessages, "new-card")
				if err != nil || pf["valid"] != tc.NestValid {
					t.Fatalf("bound=%v preflight=%#v err=%v", bound, pf, err)
				}
				chart := pf["chartValidation"].(map[string]any)
				if chart["status"] != tc.Status {
					t.Fatalf("bound=%v chart=%#v", bound, chart)
				}
				if tc.Status == "unverified" {
					if chart["checked"] != 0 || chart["unverified"].(int) < 1 {
						t.Fatal(chart)
					}
					pointer := "/0/createSurface/components/0/data"
					if bound {
						pointer += "/path"
					}
					diagnostics := pf["diagnostics"].([]Diagnostic)
					if len(diagnostics) != 1 || diagnostics[0].Code != "chart.series_type_unverified" ||
						diagnostics[0].Severity != "warning" || diagnostics[0].Pointer != pointer {
						t.Fatalf("bound=%v diagnostics=%#v", bound, diagnostics)
					}
				}
			}
			results[tc.ID] = tc.NestValid
		})
	}
	if path := os.Getenv("AICARD_CHART_PARITY_OUTPUT"); path != "" {
		encoded, err := json.Marshal(results)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, encoded, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
