// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
)

func TestCrossPlatformCoverageDigitalEmployeeConnectLifecycleSchema(t *testing.T) {
	root := NewRootCommand()
	wants := map[string]string{"dingtalk-tag.connect": "dingtalk-tag connect"}
	for _, action := range []string{"status", "list", "stop", "restart", "unbind"} {
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
		"dingtalk-tag.connect":        {"agent-uuid": "agentUuid", "device-id": "deviceId", "local-agent-name": "localAgentName", "extensions": "extensions", "client-id": "clientId"},
		"dingtalk-tag.connect_unbind": {"agent-uuid": "agentUuid", "runtime-binding-id": "runtimeBindingId"},
	}
	root := NewRootCommand()
	payload := schemaContractPayloadForBoundCanonicals(t, root, "dingtalk-tag.connect", "dingtalk-tag.connect_unbind")
	for action, params := range wants {
		tool := payload.Tools[action]
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

func TestCrossPlatformCoverageDeapAgentLeavesReachFinalSchema(t *testing.T) {
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
				"name": "name", "description": "description", "dept-id": "deptId",
				"avatar-url":         "avatarUrl",
				"prompt":             "prompt",
				"supervisor-user-id": "digitalTagEmployeeProfile.supervisorUserId",
				"type":               "digitalTagEmployeeProfile.type",
				"response-mode":      "digitalTagEmployeeProfile.responseMode",
			},
		},
		"dingtalk-tag.set_visibility": {
			"dingtalk-tag manage set-visibility", "set_visibility", "write", "high", "user_required",
			map[string]string{
				"agent-uuid": "agentUuid", "visibility": "visibility",
				"user-ids": "staffIds", "dept-ids": "deptIds",
			},
		},
		"dingtalk-tag.get_digital_employee_detail": {
			"dingtalk-tag manage detail", "get_digital_employee_detail", "read", "low", "not_required",
			map[string]string{"agent-uuid": "agentUuid", "snapshot": "snapshot"},
		},
		"dingtalk-tag.list_digital_employees": {
			"dingtalk-tag manage list", "list_digital_employees", "read", "low", "not_required",
			map[string]string{
				"keyword": "keyword", "type": "type",
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
				"avatar-url": "avatarUrl", "prompt": "prompt",
				"supervisor-user-id": "digitalTagEmployeeProfile.supervisorUserId",
				"type":               "digitalTagEmployeeProfile.type",
				"response-mode":      "digitalTagEmployeeProfile.responseMode",
			},
		},
		"dingtalk-tag.publish_digital_employee": {
			"dingtalk-tag manage publish", "publish_digital_employee", "write", "high", "user_required",
			map[string]string{"agent-uuid": "agentUuid"},
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
			if flagName == "prompt" && parameter["required"] == true {
				t.Errorf("%s prompt must remain optional", canonical)
			}
			if flagName == "type" {
				wantRequired := canonical == "dingtalk-tag.create_digital_employee"
				gotRequired, _ := parameter["required"].(bool)
				if gotRequired != wantRequired {
					t.Errorf("%s type required=%t, want %t", canonical, gotRequired, wantRequired)
				}
			}
			if flagName == "response-mode" {
				defaultMode := schemaContractString(parameter["default"])
				wantDefault := ""
				if canonical == "dingtalk-tag.create_digital_employee" {
					wantDefault = "mention_only"
				}
				if defaultMode != wantDefault {
					t.Errorf("%s response-mode default=%q, want %q", canonical, defaultMode, wantDefault)
				}
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

func TestCrossPlatformCoverageDeapAgentSkillMCPLeavesReachFinalSchema(t *testing.T) {
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
		"dingtalk-tag.update_skill": {
			"dingtalk-tag capability skill update", "", "available",
			map[string]string{"agent-uuid": "agentUuid", "skill-id": "skillId", "enabled": "enabled", "file": "fileUrl"},
		},
		"dingtalk-tag.delete_skill": {
			"dingtalk-tag capability skill delete", "delete_skill", "available",
			map[string]string{"agent-uuid": "agentUuid", "skill-id": "skillId"},
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
			"dingtalk-tag capability mcp create", "", "available",
			map[string]string{"agent-uuid": "agentUuid", "config-file": ""},
		},
		"dingtalk-tag.update_mcp": {
			"dingtalk-tag capability mcp update", "", "available",
			map[string]string{"agent-uuid": "agentUuid", "mcp-id": "mcpId", "enabled": "enabled", "config-file": "configFile"},
		},
		"dingtalk-tag.delete_mcp": {
			"dingtalk-tag capability mcp delete", "delete_mcp", "available",
			map[string]string{"agent-uuid": "agentUuid", "mcp-id": "mcpId"},
		},
		"dingtalk-tag.list_mcps": {
			"dingtalk-tag capability mcp list", "list_mcps", "available",
			map[string]string{"agent-uuid": "agentUuid", "keywords": "keywords", "page": "page", "page-size": "pageSize"},
		},
		"dingtalk-tag.get_mcp_detail": {
			"dingtalk-tag capability mcp query", "query_mcp", "available",
			map[string]string{"agent-uuid": "agentUuid", "mcp-id": "mcpId"},
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
		if schemaContractMap(tool["parameters"])["agent-uuid"]["required"] != true {
			t.Errorf("%s agent-uuid must be required in final Schema", canonical)
		}
	}
}

func TestCrossPlatformCoverageDeapAgentSkillUpdatePublishesExactlyOneConstraint(t *testing.T) {
	payload := schemaContractPayloadForBoundCanonicals(t, NewRootCommand(), "dingtalk-tag.update_skill")
	tool := payload.Tools["dingtalk-tag.update_skill"]
	assertSchemaContractConstraintGroup(t, tool, "require_one_of", []string{"enabled", "file"})
	assertSchemaContractConstraintGroup(t, tool, "mutually_exclusive", []string{"enabled", "file"})
}

func TestCrossPlatformCoverageDeapAgentMCPAutoMountHelpAndFinalSchema(t *testing.T) {
	root := NewRootCommand()
	cmd, args, err := root.Find([]string{"dingtalk-tag", "capability", "mcp", "create"})
	if err != nil || len(args) != 0 {
		t.Fatalf("create resolution: %v %v", args, err)
	}
	var help bytes.Buffer
	cmd.SetOut(&help)
	if err := cmd.Help(); err != nil {
		t.Fatalf("render create help: %v", err)
	}
	payload := schemaContractPayloadForBoundCanonicals(t, root, "dingtalk-tag.create_mcp")
	tool := payload.Tools["dingtalk-tag.create_mcp"]
	for surface, description := range map[string]string{
		"help": help.String(), "final schema": schemaContractString(tool["description"]),
	} {
		for _, want := range []string{"自动挂载", "回读确认", "保留已有选择", "不克隆", "不自动发布", "stage=query_created_mcp", "stage=mount_draft", "禁止重复 create", "串行", "仅升级 CLI"} {
			if !strings.Contains(description, want) {
				t.Errorf("%s missing %q", surface, want)
			}
		}
		if strings.Contains(description, "不自动挂载") {
			t.Errorf("%s retains stale no-mount guidance", surface)
		}
	}
	for field, want := range map[string]string{
		"effect": "write", "risk": "high", "confirmation": "user_required", "idempotency": "unknown",
	} {
		if got := schemaContractString(tool[field]); got != want {
			t.Errorf("%s = %q, want %q", field, got, want)
		}
	}
}

func TestCrossPlatformCoverageDingTalkTagConnectReachesFinalSchema(t *testing.T) {
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
	if len(parameters) != 18 {
		t.Fatalf("dingtalk-tag.connect parameter count = %d, want 18", len(parameters))
	}
	for name, property := range map[string]string{
		"agent-uuid": "agentUuid", "channel": "channel", "client-id": "clientId",
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
	if _, ok := parameters["profile-only"]; ok {
		t.Error("dingtalk-tag.connect still exposes profile-only")
	}
}

func TestCrossPlatformCoverageEmployeeRemovedBindingCommandsAbsentFromSchema(t *testing.T) {
	NewRootCommand()
	for _, action := range []string{"bind", "rebind"} {
		if _, ok := cli.ResolveMeta("dingtalk-tag connect " + action); ok {
			t.Errorf("已删除命令仍出现在 Schema: %s", action)
		}
	}
	for _, path := range []string{"dingtalk-tag connect", "dingtalk-tag connect unbind"} {
		if _, ok := cli.ResolveMeta(path); !ok {
			t.Errorf("保留命令未交付到 Schema: %s", path)
		}
	}
}
