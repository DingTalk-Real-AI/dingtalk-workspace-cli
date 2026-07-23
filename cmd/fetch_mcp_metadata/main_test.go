// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package main

import (
	"reflect"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

func TestLoadRegistryInterfaceRefsUsesSplitRegistry(t *testing.T) {
	refs := loadRegistryInterfaceRefs()
	if len(refs) == 0 {
		t.Fatal("loadRegistryInterfaceRefs() returned no reviewed commands")
	}

	got, ok := refs["calendar.list_calendars"]
	if !ok {
		t.Fatal("calendar.list_calendars missing from reassembled split registry")
	}
	if got["product_id"] != "calendar" || got["rpc_name"] != "list_calendars" {
		t.Fatalf("calendar.list_calendars ref = %#v", got)
	}
}

func TestMergeLiveMCPToolRefreshesExistingMetadata(t *testing.T) {
	const canonical = "calendar.list_calendars"
	reviewedRef := map[string]any{
		"product_id": "calendar-helper",
		"rpc_name":   "list_user_calendars",
	}
	allTools := map[string]map[string]any{
		canonical: {
			"title":         "old title",
			"description":   "old description",
			"interface_ref": reviewedRef,
			"parameters": map[string]any{
				"stale": map[string]any{"type": "string"},
			},
		},
	}
	live := transport.ToolDescriptor{
		Name:        "list_calendars",
		Title:       "new title",
		Description: "new description",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"cursor": map[string]any{
					"type":        "string",
					"description": "next page cursor",
				},
			},
			"required": []any{"cursor"},
		},
	}
	fallbackRef := map[string]string{
		"product_id": "calendar",
		"rpc_name":   "list_calendars",
	}

	mergeLiveMCPTool(allTools, canonical, live, fallbackRef)

	got := allTools[canonical]
	if got["title"] != "new title" || got["description"] != "new description" {
		t.Fatalf("live metadata was not refreshed: %#v", got)
	}
	if !reflect.DeepEqual(got["interface_ref"], reviewedRef) {
		t.Fatalf("interface_ref = %#v, want reviewed mapping %#v", got["interface_ref"], reviewedRef)
	}
	params, ok := got["parameters"].(map[string]map[string]any)
	if !ok {
		t.Fatalf("parameters type = %T, want refreshed parameter map", got["parameters"])
	}
	if _, stale := params["stale"]; stale {
		t.Fatalf("stale parameter survived refresh: %#v", params)
	}
	if cursor := params["cursor"]; cursor["type"] != "string" || cursor["description"] != "next page cursor" || cursor["required"] != true {
		t.Fatalf("cursor parameter = %#v", cursor)
	}
}
