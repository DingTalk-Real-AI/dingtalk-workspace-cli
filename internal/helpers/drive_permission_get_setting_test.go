// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package helpers

import (
	"strings"
	"testing"
)

// ── drive permission get-setting：跨产品路由与 --node 别名归一化 ──

func TestCrossPlatformCoverageDrivePermissionGetSettingRoutesToDoc(t *testing.T) {
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
	if call.productID != "doc" || call.toolName != "get_permission_setting" {
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
		if call.productID != "doc" || call.toolName != "get_permission_setting" {
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
