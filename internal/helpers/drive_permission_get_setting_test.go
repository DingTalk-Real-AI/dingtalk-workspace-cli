// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package helpers

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
)

// ── drive permission get-setting：跨产品路由与 --node 别名归一化 ──

func TestCrossPlatformCoverageDrivePermissionGetSettingRoutesToDrive(t *testing.T) {
	caller := &guardedMutationCaller{}
	err := executeGuardedMutationCommand(t, caller, newDriveCommand,
		"permission", "get-setting", "--node", "node-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("calls = %#v, want exactly one", caller.calls)
	}
	call := caller.calls[0]
	if call.productID != "drive" || call.toolName != "get_permission_setting" {
		t.Fatalf("call = %#v", call)
	}
	if len(call.args) != 1 || call.args["nodeId"] != "node-1" {
		t.Fatalf("args = %#v, want only nodeId=node-1", call.args)
	}
}

func TestCrossPlatformCoverageDrivePermissionGetSettingHiddenAliases(t *testing.T) {
	for _, alias := range []string{"url", "id", "node-id", "doc-id", "file-id"} {
		caller := &guardedMutationCaller{}
		err := executeGuardedMutationCommand(t, caller, newDriveCommand,
			"permission", "get-setting", "--"+alias, "node-alias")
		if err != nil {
			t.Fatalf("alias --%s: %v", alias, err)
		}
		if len(caller.calls) != 1 {
			t.Fatalf("alias --%s calls = %#v, want exactly one", alias, caller.calls)
		}
		call := caller.calls[0]
		if call.productID != "drive" || call.toolName != "get_permission_setting" {
			t.Fatalf("alias --%s call = %#v", alias, call)
		}
		if call.args["nodeId"] != "node-alias" {
			t.Fatalf("alias --%s args = %#v, want nodeId=node-alias", alias, call.args)
		}
	}
}

func TestCrossPlatformCoverageDrivePermissionGetSettingRequiresNode(t *testing.T) {
	caller := &guardedMutationCaller{}
	err := executeGuardedMutationCommand(t, caller, newDriveCommand,
		"permission", "get-setting")
	if err == nil || !strings.Contains(err.Error(), "flag --node is required") {
		t.Fatalf("err = %v, want flag --node is required", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("calls = %#v, want none before required-flag validation", caller.calls)
	}
}

// ── drive permission get-setting：ResultSpec 返回值契约 ──

func TestCrossPlatformCoverageDrivePermissionGetSettingResultContract(t *testing.T) {
	drive := newDriveCommand()
	leaf, _, err := drive.Find([]string{"permission", "get-setting"})
	if err != nil || leaf == nil {
		t.Fatalf("find drive permission get-setting: command=%v err=%v", leaf, err)
	}
	final, ok := contractfinal.RuntimeContractFinal(leaf)
	if !ok || final.Identity == nil || final.Identity.CanonicalPath != "drive.get_permission_setting" {
		t.Fatalf("get-setting ContractFinal identity = %#v, found = %v", final.Identity, ok)
	}
	if final.Result == nil {
		t.Fatal("get-setting final Result is nil")
	}
	wantOutcomes := []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure}
	if !reflect.DeepEqual(final.Result.Outcomes, wantOutcomes) {
		t.Fatalf("outcomes = %#v, want %#v", final.Result.Outcomes, wantOutcomes)
	}

	var root map[string]any
	if err := json.Unmarshal(final.Result.DataSchema, &root); err != nil {
		t.Fatalf("result data_schema is not JSON: %v\n%s", err, final.Result.DataSchema)
	}
	assertSchemaRequired(t, root, "docUrl", "nodeId", "shareScope", "policies")
	properties := resultSchemaProperties(t, final.Result.DataSchema)
	if got := sortedContractSchemaKeys(properties); !reflect.DeepEqual(got, []string{"docUrl", "nodeId", "permissionMode", "policies", "shareScope"}) {
		t.Fatalf("result properties = %#v", got)
	}

	permissionMode, ok := properties["permissionMode"].(map[string]any)
	if !ok {
		t.Fatalf("permissionMode = %#v, want schema object", properties["permissionMode"])
	}
	if got := schemaEnumValues(t, permissionMode); !reflect.DeepEqual(got, []string{"INHERITED", "INDEPENDENT", "<null>"}) {
		t.Fatalf("permissionMode enum = %#v", got)
	}
	if types, ok := permissionMode["type"].([]any); !ok || len(types) != 2 || types[0] != "string" || types[1] != "null" {
		t.Fatalf("permissionMode type = %#v, want [string null]", permissionMode["type"])
	}

	shareScope := schemaObjectProperty(t, properties, "shareScope")
	shareScopeProperties := schemaProperties(t, shareScope)
	if got := sortedContractSchemaKeys(shareScopeProperties); !reflect.DeepEqual(got, []string{"canRecommend", "canSearch", "defaultRole", "linkShare", "partnerIncluded", "visibility"}) {
		t.Fatalf("shareScope properties = %#v", got)
	}
	assertSchemaRequired(t, shareScope, "linkShare")
	if got := schemaEnumValues(t, shareScopeProperties["visibility"].(map[string]any)); !reflect.DeepEqual(got, []string{"PRIVATE", "ORGANIZATION", "PUBLIC", "<null>"}) {
		t.Fatalf("visibility enum = %#v", got)
	}
	if got := schemaEnumValues(t, shareScopeProperties["defaultRole"].(map[string]any)); !reflect.DeepEqual(got, []string{"READER", "DOWNLOADER", "EDITOR", "MANAGER", "<null>"}) {
		t.Fatalf("defaultRole enum = %#v", got)
	}
	linkShareProperties := schemaProperties(t, schemaObjectProperty(t, shareScopeProperties, "linkShare"))
	if got := sortedContractSchemaKeys(linkShareProperties); !reflect.DeepEqual(got, []string{"expireAt", "expireDays", "forCurrentNode", "requirePassword"}) {
		t.Fatalf("linkShare properties = %#v", got)
	}

	policies, ok := properties["policies"].(map[string]any)
	if !ok || policies["type"] != "array" {
		t.Fatalf("policies = %#v, want array schema", properties["policies"])
	}
	policiesDescription, _ := policies["description"].(string)
	if !strings.Contains(policiesDescription, "未下发或不受支持的策略不会返回") || !strings.Contains(policiesDescription, "node_spread_scope 仅文件夹类节点返回") {
		t.Fatalf("policies description = %q", policiesDescription)
	}
	items, ok := policies["items"].(map[string]any)
	if !ok {
		t.Fatalf("policies items = %#v", policies["items"])
	}
	assertSchemaRequired(t, items, "code", "name", "description")
	itemProperties := schemaProperties(t, items)
	if got := sortedContractSchemaKeys(itemProperties); !reflect.DeepEqual(got, []string{"allowedValues", "code", "description", "inherited", "name", "value"}) {
		t.Fatalf("policy item properties = %#v", got)
	}
	code, ok := itemProperties["code"].(map[string]any)
	if !ok {
		t.Fatalf("code = %#v", itemProperties["code"])
	}
	if got := schemaEnumValues(t, code); !reflect.DeepEqual(got, []string{
		"external_share", "external_share_manager_only", "member_invite", "member_invite_org_only",
		"comment", "permission_apply", "external_permission_apply", "watermark", "node_spread",
		"online_content_copy", "node_move_forbidden", "node_spread_scope",
	}) {
		t.Fatalf("code enum = %#v", got)
	}
	value, ok := itemProperties["value"].(map[string]any)
	if !ok {
		t.Fatalf("value = %#v", itemProperties["value"])
	}
	valueDescription, _ := value["description"].(string)
	for _, fragment := range []string{"ENABLED/DISABLED", "READER_AND_ABOVE", "NOBODY", "ALL_NODES", "PREVIEWABLE_ONLY", "不低于该角色才允许", "所有人禁止", "限制对所有文档生效", "仅对可预览的文档"} {
		if !strings.Contains(valueDescription, fragment) {
			t.Fatalf("value description missing %q: %s", fragment, valueDescription)
		}
	}
	for _, field := range []string{"name", "description"} {
		entry, ok := itemProperties[field].(map[string]any)
		if !ok || entry["type"] != "string" {
			t.Fatalf("policy item %s = %#v, want string schema", field, itemProperties[field])
		}
		entryDescription, _ := entry["description"].(string)
		if !strings.Contains(entryDescription, "确定性字段") || !strings.Contains(entryDescription, "只要该策略返回就必带") {
			t.Fatalf("policy item %s description = %q", field, entryDescription)
		}
	}
	inherited, ok := itemProperties["inherited"].(map[string]any)
	if !ok {
		t.Fatalf("inherited = %#v", itemProperties["inherited"])
	}
	if inheritedDescription, _ := inherited["description"].(string); !strings.Contains(inheritedDescription, "继承自上级") {
		t.Fatalf("inherited description = %q", inherited["description"])
	}
	allowedValues, ok := itemProperties["allowedValues"].(map[string]any)
	if !ok {
		t.Fatalf("allowedValues = %#v", itemProperties["allowedValues"])
	}
	if allowedValuesDescription, _ := allowedValues["description"].(string); !strings.Contains(allowedValuesDescription, "与 value 同一值域") {
		t.Fatalf("allowedValues description = %q", allowedValues["description"])
	}
}

func schemaEnumValues(t *testing.T, property map[string]any) []string {
	t.Helper()
	raw, ok := property["enum"].([]any)
	if !ok {
		t.Fatalf("schema enum = %#v, want array", property["enum"])
	}
	values := make([]string, 0, len(raw))
	for _, value := range raw {
		switch typed := value.(type) {
		case string:
			values = append(values, typed)
		case nil:
			values = append(values, "<null>")
		default:
			t.Fatalf("schema enum value = %#v, want string or null", value)
		}
	}
	return values
}
