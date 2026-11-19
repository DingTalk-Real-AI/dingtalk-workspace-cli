// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package a2ui

import (
	"encoding/json"
	"os"
	"testing"
)

func TestCrossPlatformCoverageSkillCompatibilitySnapshots(t *testing.T) {
	p := testProtocol(t)
	raw, err := os.ReadFile("testdata/skill-compat.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name     string
		Messages json.RawMessage
		Mode     string
		Valid    bool
		Status   string
		Checked  int
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			report, err := p.Lint(tc.Messages, false, true)
			if err != nil || !report.Valid {
				t.Fatalf("schema: %+v %v", report, err)
			}
			result, err := p.Preflight(report.A2UIMessages, tc.Mode)
			if err != nil || result["valid"] != tc.Valid {
				t.Fatalf("preflight: %+v %v", result, err)
			}
			chart := result["chartValidation"].(map[string]any)
			if chart["status"] != tc.Status || chart["checked"] != tc.Checked {
				t.Fatal(chart)
			}
		})
	}
}
