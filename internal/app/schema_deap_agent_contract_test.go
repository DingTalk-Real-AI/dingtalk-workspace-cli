// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"reflect"
	"testing"
)

func TestDeapAgentLeavesReachFinalSchema(t *testing.T) {
	wants := map[string]struct {
		cliPath      string
		tool         string
		effect       string
		risk         string
		confirmation string
		parameters   map[string]string
	}{
		"dingtalk-tag.create_digital_employee": {
			"dingtalk-tag manage create", "create_digital_employee", "write", "medium", "not_required",
			map[string]string{
				"name": "name", "description": "description", "dept-id": "deptId", "dept-name": "deptName",
				"icon": "icon", "profile-json": "digitalTagEmployeeProfile",
				"employee-no":       "digitalTagEmployeeProfile.employeeNo",
				"position-name":     "digitalTagEmployeeProfile.positionName",
				"supervisor-uid":    "digitalTagEmployeeProfile.directSupervisorUid",
				"main-program-type": "digitalTagEmployeeProfile.mainProgramType",
				"response-mode":     "digitalTagEmployeeProfile.responseMode",
			},
		},
		"dingtalk-tag.get_digital_employee_detail": {
			"dingtalk-tag manage detail", "get_digital_employee_detail", "read", "low", "not_required",
			map[string]string{"agent-uuid": "agentUuid", "type": "type"},
		},
		"dingtalk-tag.list_digital_employees": {
			"dingtalk-tag manage list", "list_digital_employees", "read", "low", "not_required",
			map[string]string{
				"keyword": "keyword", "main-program-type": "mainProgramType",
				"page": "page", "page-size": "pageSize",
			},
		},
		"dingtalk-tag.login": {
			"dingtalk-tag manage login", "", "write", "high", "not_required",
			map[string]string{"agent-uuid": "agentUuid", "client-id": "clientId"},
		},
		"dingtalk-tag.update_digital_employee_draft": {
			"dingtalk-tag manage save-draft", "update_digital_employee_draft", "write", "high", "user_required",
			map[string]string{
				"agent-uuid": "agentUuid", "name": "name", "description": "description", "dept-id": "deptId",
				"dept-name": "deptName", "icon": "icon", "prompt": "prompt", "profile-json": "digitalTagEmployeeProfile",
				"employee-no":       "digitalTagEmployeeProfile.employeeNo",
				"position-name":     "digitalTagEmployeeProfile.positionName",
				"supervisor-uid":    "digitalTagEmployeeProfile.directSupervisorUid",
				"main-program-type": "digitalTagEmployeeProfile.mainProgramType",
				"response-mode":     "digitalTagEmployeeProfile.responseMode",
				"skills-file":       "skills", "mcps-file": "mcps",
			},
		},
		"dingtalk-tag.publish_digital_employee": {
			"dingtalk-tag manage publish", "publish_digital_employee", "write", "high", "user_required",
			map[string]string{"agent-uuid": "agentUuid", "allow-join-group": "allowJoinGroup"},
		},
		"dingtalk-tag.delete_digital_employee": {
			"dingtalk-tag manage delete", "delete_digital_employee", "destructive", "high", "user_required",
			map[string]string{"agent-uuid": "agentUuid"},
		},
		"dingtalk-tag.query_de_run_status": {
			"dingtalk-tag run run-status", "query_de_run_status", "read", "low", "not_required",
			map[string]string{"source-id": "sourceId", "source-type": "sourceType", "agent-uuid": "agentUuid"},
		},
		"dingtalk-tag.query_de_trace": {
			"dingtalk-tag run trace", "query_de_trace", "read", "high", "not_required",
			map[string]string{"source-id": "sourceId", "source-type": "sourceType", "agent-uuid": "agentUuid"},
		},
	}
	canonicals := make([]string, 0, len(wants))
	for canonical := range wants {
		canonicals = append(canonicals, canonical)
	}
	payload := schemaContractPayloadForBoundCanonicals(t, NewRootCommand(), canonicals...)
	for canonical, want := range wants {
		tool := payload.Tools[canonical]
		if got := schemaContractString(tool["primary_cli_path"]); got != want.cliPath {
			t.Errorf("%s primary_cli_path = %q, want %q", canonical, got, want.cliPath)
		}
		for field, expected := range map[string]string{
			"effect": want.effect, "risk": want.risk,
			"confirmation": want.confirmation, "availability": "available",
		} {
			if got := schemaContractString(tool[field]); got != expected {
				t.Errorf("%s %s = %q, want %q", canonical, field, got, expected)
			}
		}
		if want.tool == "" {
			if got := schemaContractString(tool["interface_mode"]); got != "composite" {
				t.Errorf("%s interface_mode = %q, want composite", canonical, got)
			}
			if ref := schemaInterfaceObject(tool["interface_ref"]); len(ref) != 0 {
				t.Errorf("%s composite unexpectedly exposes interface_ref %#v", canonical, ref)
			}
		} else {
			ref := schemaInterfaceObject(tool["interface_ref"])
			if got := schemaContractString(ref["product_id"]); got != "deap-dev" {
				t.Errorf("%s interface product = %q, want deap-dev", canonical, got)
			}
			if got := schemaContractString(ref["rpc_name"]); got != want.tool {
				t.Errorf("%s interface rpc = %q, want %q", canonical, got, want.tool)
			}
		}
		parameters := schemaContractMap(tool["parameters"])
		if len(parameters) != len(want.parameters) {
			t.Errorf("%s parameter count = %d, want %d: %#v", canonical, len(parameters), len(want.parameters), parameters)
		}
		for flagName, property := range want.parameters {
			parameter := parameters[flagName]
			if parameter == nil {
				t.Errorf("%s missing parameter %s", canonical, flagName)
				continue
			}
			if got := schemaContractString(parameter["property"]); got != property {
				t.Errorf("%s parameter %s property = %q, want %q", canonical, flagName, got, property)
			}
			if flagName == "response-mode" {
				got := schemaContractStringSlice(parameter["enum"])
				wantModes := []string{"mention_only", "targeted_proactive", "mention_only,targeted_proactive"}
				if !reflect.DeepEqual(got, wantModes) {
					t.Errorf("%s response-mode enum = %#v, want %#v", canonical, got, wantModes)
				}
			}
		}
		for _, forbidden := range []string{"org-id", "user-id", "agent-type"} {
			if _, ok := parameters[forbidden]; ok {
				t.Errorf("%s exposes forbidden parameter %s", canonical, forbidden)
			}
		}
	}
}

func TestDeapAgentSkillMCPLeavesReachFinalSchema(t *testing.T) {
	wants := map[string]struct {
		cliPath      string
		tool         string
		availability string
		parameters   map[string]string
	}{
		"dingtalk-tag.create_skill_from_file": {
			"dingtalk-tag capability skill create", "", "available",
			map[string]string{"agent-uuid": "agentUuid", "file": "file"},
		},
		"dingtalk-tag.list_skills": {
			"dingtalk-tag capability skill list", "list_skills", "available",
			map[string]string{"agent-uuid": "agentUuid", "snapshot": "snapshot"},
		},
		"dingtalk-tag.get_skill_detail": {
			"dingtalk-tag capability skill query", "query_skill", "available",
			map[string]string{"agent-uuid": "agentUuid", "skill-id": "skillId", "snapshot": "snapshot"},
		},
		"dingtalk-tag.create_mcp": {
			"dingtalk-tag capability mcp create", "create_mcp", "available",
			map[string]string{"config-file": "config"},
		},
		"dingtalk-tag.list_mcps": {
			"dingtalk-tag capability mcp list", "list_mcps", "available",
			map[string]string{"keywords": "keywords", "page": "page", "page-size": "pageSize"},
		},
		"dingtalk-tag.get_mcp_detail": {
			"dingtalk-tag capability mcp query", "query_mcp", "available",
			map[string]string{"mcp-id": "mcpId"},
		},
	}
	canonicals := make([]string, 0, len(wants))
	for canonical := range wants {
		canonicals = append(canonicals, canonical)
	}
	payload := schemaContractPayloadForBoundCanonicals(t, NewRootCommand(), canonicals...)
	for canonical, want := range wants {
		tool := payload.Tools[canonical]
		if got := schemaContractString(tool["primary_cli_path"]); got != want.cliPath {
			t.Errorf("%s primary_cli_path = %q, want %q", canonical, got, want.cliPath)
		}
		if got := schemaContractString(tool["availability"]); got != want.availability {
			t.Errorf("%s availability = %q, want %q", canonical, got, want.availability)
		}
		if want.availability == "unavailable" || want.tool == "" {
			if got := schemaContractString(tool["interface_mode"]); got != "composite" {
				t.Errorf("%s interface_mode = %q, want composite", canonical, got)
			}
			if ref := schemaInterfaceObject(tool["interface_ref"]); len(ref) != 0 {
				t.Errorf("%s composite unexpectedly exposes interface_ref %#v", canonical, ref)
			}
		} else {
			ref := schemaInterfaceObject(tool["interface_ref"])
			if got := schemaContractString(ref["product_id"]); got != "deap-dev" {
				t.Errorf("%s interface product = %q, want deap-dev", canonical, got)
			}
			if got := schemaContractString(ref["rpc_name"]); got != want.tool {
				t.Errorf("%s interface rpc = %q, want %q", canonical, got, want.tool)
			}
		}
		parameters := schemaContractMap(tool["parameters"])
		if len(parameters) != len(want.parameters) {
			t.Errorf("%s parameter count = %d, want %d: %#v", canonical, len(parameters), len(want.parameters), parameters)
		}
		for flagName, property := range want.parameters {
			parameter := parameters[flagName]
			if parameter == nil {
				t.Errorf("%s missing parameter %s", canonical, flagName)
				continue
			}
			if got := schemaContractString(parameter["property"]); got != property {
				t.Errorf("%s parameter %s property = %q, want %q", canonical, flagName, got, property)
			}
		}
	}
}
