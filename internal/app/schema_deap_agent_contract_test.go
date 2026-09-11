// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"reflect"
	"testing"
)

func TestDigitalEmployeeConnectLifecycleSchema(t *testing.T) {
	root := NewRootCommand()
	wants := map[string]string{"dingtalk-tag.connect": "dingtalk-tag connect"}
	for _, action := range []string{"status", "list", "stop", "restart", "bind", "unbind", "rebind"} {
		wants["dingtalk-tag.connect_"+action] = "dingtalk-tag connect " + action
	}
	var names []string
	for name := range wants {
		names = append(names, name)
	}
	payload := schemaContractPayloadForBoundCanonicals(t, root, names...)
	for name, path := range wants {
		if got := schemaContractString(payload.Tools[name]["primary_cli_path"]); got != path {
			t.Errorf("%s path = %q, want %q", name, got, path)
		}
	}
	cmd, args, err := root.Find([]string{"dingtalk-tag", "connect", "list"})
	if err != nil || len(args) != 0 || cmd.Name() != "list" {
		t.Fatalf("list resolution: %v %v", args, err)
	}
	if cmd.Flags().Lookup("agent-uuid") != nil {
		t.Fatal("list must not inherit connect's required employee flag")
	}
}

func TestCrossPlatformCoverageEmployeeServerBindingFinalSchema(t *testing.T) {
	wants := map[string]map[string]string{
		"bind":   {"agent-uuid": "agentUuid", "device-id": "deviceId", "local-agent-name": "localAgentName", "extensions": "extensions"},
		"unbind": {"agent-uuid": "agentUuid", "runtime-binding-id": "runtimeBindingId"},
		"rebind": {"agent-uuid": "agentUuid", "runtime-binding-id": "runtimeBindingId", "device-id": "deviceId", "local-agent-name": "localAgentName", "extensions": "extensions", "client-id": "clientId"},
	}
	root := NewRootCommand()
	payload := schemaContractPayloadForBoundCanonicals(t, root, "dingtalk-tag.connect_bind", "dingtalk-tag.connect_unbind", "dingtalk-tag.connect_rebind")
	for action, params := range wants {
		tool := payload.Tools["dingtalk-tag.connect_"+action]
		if schemaContractString(tool["interface_mode"]) != "composite" {
			t.Errorf("%s must declare server-side effects", action)
		}
		if schemaContractString(tool["confirmation"]) != "user_required" || schemaContractString(tool["effect"]) != "write" || tool["result"] == nil {
			t.Fatalf("incomplete %s contract", action)
		}
		got := schemaContractMap(tool["parameters"])
		for name, property := range params {
			if schemaContractString(got[name]["property"]) != property {
				t.Errorf("%s missing mapping %s", action, name)
			}
		}
		for _, name := range []string{"identity", "user-id", "org-id", "profile-only"} {
			if _, ok := got[name]; ok {
				t.Errorf("%s exposes %s", action, name)
			}
		}
	}
}

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

func TestDingTalkTagConnectProfileOnlyReachesFinalSchema(t *testing.T) {
	payload := schemaContractPayloadForBoundCanonicals(t, NewRootCommand(), "dingtalk-tag.connect")
	tool := payload.Tools["dingtalk-tag.connect"]
	for field, want := range map[string]string{
		"primary_cli_path": "dingtalk-tag connect",
		"effect":           "write",
		"risk":             "high",
		"confirmation":     "user_required",
		"interface_mode":   "composite",
	} {
		if got := schemaContractString(tool[field]); got != want {
			t.Errorf("dingtalk-tag.connect %s = %q, want %q", field, got, want)
		}
	}
	parameters := schemaContractMap(tool["parameters"])
	if len(parameters) != 19 {
		t.Fatalf("dingtalk-tag.connect parameter count = %d, want 19", len(parameters))
	}
	for name, property := range map[string]string{
		"agent-uuid": "agentUuid", "channel": "channel", "profile-only": "profileOnly", "client-id": "clientId",
		"device-id": "deviceId", "local-agent-name": "localAgentName", "extensions": "extensions",
	} {
		parameter := parameters[name]
		if parameter == nil {
			t.Errorf("dingtalk-tag.connect missing parameter %s", name)
			continue
		}
		if got := schemaContractString(parameter["property"]); got != property {
			t.Errorf("dingtalk-tag.connect parameter %s property = %q, want %q", name, got, property)
		}
	}
	channel := parameters["channel"]
	if got := schemaContractString(channel["required_when"]); got != "" {
		t.Errorf("dingtalk-tag.connect channel required_when = %q", got)
	}
	if required, _ := channel["required"].(bool); required {
		t.Error("dingtalk-tag.connect channel must not be unconditionally required")
	}
	profileOnly := parameters["profile-only"]
	if got := schemaContractString(profileOnly["type"]); got != "boolean" {
		t.Errorf("dingtalk-tag.connect profile-only type = %q, want boolean", got)
	}
}
