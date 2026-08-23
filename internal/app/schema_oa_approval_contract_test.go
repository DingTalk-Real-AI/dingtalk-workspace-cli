// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"strings"
	"testing"
)

func TestOAApprovalDualModeConstraintsReachEmbeddedSchema(t *testing.T) {
	tools := deliverySchemaAllToolsForHelpFlagTest(t, NewRootCommand())

	tests := []struct {
		canonical         string
		optional          []string
		requireTogether   []string
		mutuallyExclusive []string
	}{
		{
			canonical:         "oa.forecast_process",
			optional:          []string{"request", "process-code", "dept-id", "form-values"},
			requireTogether:   []string{"process-code", "dept-id", "form-values"},
			mutuallyExclusive: []string{"process-code", "dept-id", "form-values"},
		},
		{
			canonical:         "oa.start_process_instance",
			optional:          []string{"request", "process-code", "dept-id", "form-values", "originator-user-id", "approvers", "approvers-action-type", "cc-list", "cc-position"},
			requireTogether:   []string{"process-code", "form-values"},
			mutuallyExclusive: []string{"process-code", "dept-id", "form-values", "originator-user-id", "approvers", "approvers-action-type", "cc-list", "cc-position"},
		},
	}

	for _, test := range tests {
		t.Run(test.canonical, func(t *testing.T) {
			tool := tools[test.canonical]
			parameters := schemaContractMap(tool["parameters"])
			for _, name := range test.optional {
				if got := parameters[name]["required"]; got != false {
					t.Errorf("--%s required = %#v, want false for dual-mode command", name, got)
				}
			}
			assertSchemaContractConstraintGroup(t, tool, "require_one_of", []string{"request", "process-code"})
			assertSchemaContractConstraintGroup(t, tool, "require_together", test.requireTogether)
			for _, name := range test.mutuallyExclusive {
				assertSchemaContractConstraintGroup(t, tool, "mutually_exclusive", []string{"request", name})
			}
			constraints, _ := tool["constraints"].(map[string]any)
			groups, _ := constraints["mutually_exclusive"].([]any)
			if len(groups) != len(test.mutuallyExclusive) {
				t.Errorf("mutually_exclusive group count = %d, want %d: %#v", len(groups), len(test.mutuallyExclusive), groups)
			}
			for _, rawGroup := range groups {
				group, _ := rawGroup.([]any)
				if len(group) != 2 {
					t.Errorf("mutually_exclusive contains an over-broad group: %#v", group)
				}
			}

			hasRequestOnlyExample := false
			for _, example := range schemaContractStringSlice(tool["examples"]) {
				if strings.Contains(example, " --request ") && !strings.Contains(example, " --process-code ") && !strings.Contains(example, " --form-values ") {
					hasRequestOnlyExample = true
				}
			}
			if !hasRequestOnlyExample {
				t.Errorf("examples do not contain a request-only invocation: %#v", tool["examples"])
			}
		})
	}

	create := tools["oa.start_process_instance"]
	if got := schemaContractString(create["confirmation"]); got != "user_required" {
		t.Errorf("create-instance confirmation = %q, want user_required", got)
	}
}
