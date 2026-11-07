// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import "testing"

// The leave/supply suite commands validate required flags inside RunE (they
// are not Cobra MarkFlagRequired leaves), so their runtime-required semantics
// must still reach the final delivered Schema through contract.ParamDecl
// Required declarations. Otherwise the Agent Schema publishes executable-time
// required parameters as optional and generated calls cannot run.
func TestCrossPlatformCoverageAttendanceApproveSuitePublishesRequiredParams(t *testing.T) {
	tests := []struct {
		canonical string
		cliPath   string
		required  []string
		optional  []string
	}{
		{
			canonical: "attendance.leave_duration",
			cliPath:   "attendance approve leave-duration",
			required:  []string{"leave-code", "start", "end"},
			optional:  []string{"user"},
		},
		{
			canonical: "attendance.leave_check",
			cliPath:   "attendance approve leave-check",
			required:  []string{"leave-code", "process-code", "start", "end", "duration-day", "duration-hour"},
			optional:  []string{"user", "proc-inst-id"},
		},
		{
			canonical: "attendance.supply_plans",
			cliPath:   "attendance approve supply-plans",
			required:  []string{"time"},
			optional:  []string{"user"},
		},
		{
			canonical: "attendance.supply_check",
			cliPath:   "attendance approve supply-check",
			required:  []string{"timestamp"},
			optional:  []string{"user"},
		},
	}

	snapshot := fullSchemaSnapshotForTest(t)
	for _, test := range tests {
		t.Run(test.canonical, func(t *testing.T) {
			tool := snapshot.Tools[test.canonical]
			if tool == nil {
				t.Fatalf("%s is missing from final Schema", test.canonical)
			}
			if got := schemaContractString(tool["primary_cli_path"]); got != test.cliPath {
				t.Fatalf("primary_cli_path = %q, want %q", got, test.cliPath)
			}
			parameters := schemaContractMap(tool["parameters"])
			for _, flag := range test.required {
				if parameters[flag] == nil {
					t.Fatalf("%s --%s is missing from final Schema", test.cliPath, flag)
				}
				if required, _ := parameters[flag]["required"].(bool); !required {
					t.Fatalf("%s --%s required = %#v, want true (runtime-required param must publish as required)", test.cliPath, flag, parameters[flag]["required"])
				}
			}
			for _, flag := range test.optional {
				if parameters[flag] == nil {
					t.Fatalf("%s --%s is missing from final Schema", test.cliPath, flag)
				}
				if required, _ := parameters[flag]["required"].(bool); required {
					t.Fatalf("%s --%s required = %#v, want false (optional param must not publish as required)", test.cliPath, flag, parameters[flag]["required"])
				}
			}
		})
	}
}
