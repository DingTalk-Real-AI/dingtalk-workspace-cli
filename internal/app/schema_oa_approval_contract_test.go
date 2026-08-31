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

func TestOAApprovalListMCPContractsReachEmbeddedSchema(t *testing.T) {
	tools := deliverySchemaAllToolsForHelpFlagTest(t, NewRootCommand())
	common := map[string]string{
		"page":               "pageNumber",
		"limit":              "pageSize",
		"query":              "query",
		"process-code":       "processCode",
		"originator-user-id": "originatorUserId",
		"create-time-from":   "createTimeFrom",
		"create-time-to":     "createTimeTo",
		"finish-time-from":   "finishTimeFrom",
		"finish-time-to":     "finishTimeTo",
	}
	tests := []struct {
		canonical string
		rpc       string
		extra     map[string]string
		retired   []string
	}{
		{canonical: "oa.get_todo_tasks", rpc: "get_todo_tasks", extra: map[string]string{"create-before": "createBefore"}, retired: []string{"user-id", "process-instance-status", "process-instance-result"}},
		{canonical: "oa.get_done_tasks", rpc: "get_done_tasks", extra: map[string]string{"process-instance-status": "processInstanceStatus"}, retired: []string{"user-id", "process-instance-result"}},
		{canonical: "oa.get_submitted_instances", rpc: "get_submitted_instances", extra: map[string]string{"process-instance-status": "processInstanceStatus"}, retired: []string{"user-id", "process-instance-result"}},
		{canonical: "oa.get_noticed_instances", rpc: "get_noticed_instances", extra: map[string]string{"unread-only": "unreadOnly"}, retired: []string{"user-id", "process-instance-status", "process-instance-result"}},
		{canonical: "oa.shortcut_list_pending", rpc: "get_todo_tasks", extra: map[string]string{"create-before": "createBefore"}, retired: []string{"user-id", "process-instance-status", "process-instance-result"}},
		{canonical: "oa.shortcut_list_executed", rpc: "get_done_tasks", extra: map[string]string{"process-instance-status": "processInstanceStatus"}, retired: []string{"user-id", "process-instance-result"}},
		{canonical: "oa.shortcut_list_submitted", rpc: "get_submitted_instances", extra: map[string]string{"process-instance-status": "processInstanceStatus"}, retired: []string{"user-id", "process-instance-result"}},
		{canonical: "oa.shortcut_list_cc", rpc: "get_noticed_instances", extra: map[string]string{"unread-only": "unreadOnly"}, retired: []string{"user-id", "process-instance-status", "process-instance-result"}},
	}

	for _, test := range tests {
		t.Run(test.canonical, func(t *testing.T) {
			tool, ok := tools[test.canonical]
			if !ok {
				t.Fatalf("missing Schema tool %s", test.canonical)
			}
			if !strings.HasPrefix(test.canonical, "oa.shortcut_") {
				interfaceRef := schemaInterfaceObject(tool["interface_ref"])
				if got := schemaContractString(interfaceRef["product_id"]); got != "oa" {
					t.Errorf("interface product_id = %q, want oa", got)
				}
				if got := schemaContractString(interfaceRef["rpc_name"]); got != test.rpc {
					t.Errorf("interface rpc_name = %q, want %q", got, test.rpc)
				}
			}

			parameters := schemaContractMap(tool["parameters"])
			for name, property := range common {
				if got := schemaContractString(parameters[name]["property"]); got != property {
					t.Errorf("--%s property = %q, want %q", name, got, property)
				}
			}
			for name, property := range test.extra {
				if got := schemaContractString(parameters[name]["property"]); got != property {
					t.Errorf("--%s property = %q, want %q", name, got, property)
				}
			}
			for _, name := range test.retired {
				if _, exists := parameters[name]; exists {
					t.Errorf("retired --%s remains in Schema", name)
				}
			}
			for _, name := range []string{"page", "limit"} {
				if got := schemaContractString(parameters[name]["type"]); got != "integer" {
					t.Errorf("--%s type = %q, want integer", name, got)
				}
			}
			if test.rpc == "get_noticed_instances" {
				if got := schemaContractString(parameters["unread-only"]["type"]); got != "boolean" {
					t.Errorf("--unread-only type = %q, want boolean", got)
				}
			}
		})
	}

	if _, exists := tools["oa.list_pending_approvals"]; exists {
		t.Fatal("retired oa.list_pending_approvals remains in embedded Schema")
	}
	if NewRootCommand().PersistentFlags().Lookup("profile") == nil {
		t.Fatal("global --profile flag is no longer registered")
	}
}
