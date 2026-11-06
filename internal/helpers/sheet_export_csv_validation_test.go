// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import "testing"

func TestCrossPlatformCoverageSheetExportCsvValidationClassification(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		message string
	}{
		{"missing_node", nil, "--node is required"},
		{"invalid_render_option", []string{"node", "NODE", "value-render-option", "pretty"}, "--value-render-option 必须为"},
	}
	for _, dry := range []bool{false, true} {
		for _, tc := range cases {
			name := tc.name
			if dry {
				name += "_dry_run"
			}
			t.Run(name, func(t *testing.T) {
				caller := &scriptedToolCaller{dry: dry}
				err := executeSheetExportCsv(t, caller, tc.args...)
				requireDevValidationError(t, err, tc.message)
				if caller.calls != 0 {
					t.Fatalf("validation dispatched %d requests, want 0", caller.calls)
				}
			})
		}
	}
}
