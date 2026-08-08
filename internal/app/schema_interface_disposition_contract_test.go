// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package app

import (
	"sort"
	"testing"
)

func TestReviewedRoutedInterfacesReachFinalSchema(t *testing.T) {
	type interfaceCase struct {
		canonical string
		mode      string
		reason    string
	}
	tests := []interfaceCase{
		{
			canonical: "attendance.get_attendance_summary",
			mode:      "composite",
			reason:    "Reviewed unpinned remote adapter: the CLI calls attendance-wukong/get_user_attendance_summary, which is absent from the pinned MCP metadata snapshot; the incompatible attendance/get_attendance_summary contract must not be advertised.",
		},
		{
			canonical: "drive.list_files",
			mode:      "composite",
			reason:    "The CLI command routes by --workspace between drive/list_files and doc/list_nodes, so the reviewed executable wrapper has no single direct MCP interface.",
		},
		{
			canonical: "chat.search_groups",
			mode:      "composite",
			reason:    "Reviewed unpinned remote adapter: the CLI calls im/search_groups with a flat payload, while the pinned snapshot only contains the incompatible chat/search_groups_by_keyword contract.",
		},
		{
			canonical: "sheet.range_batch_set_style",
			mode:      "composite",
			reason:    "The CLI assembles style cell matrices locally from --ranges or a local batch file and submits them as one sheet/batch_update operations array; no single direct MCP interface represents the wrapper input shape.",
		},
		{
			canonical: "sheet.range_read",
			mode:      "composite",
			reason:    "Reviewed unpinned remote adapter: the CLI calls sheet/get_cell_infos, which is absent from the pinned MCP metadata snapshot; the incompatible sheet/get_range contract must not be advertised.",
		},
		{
			canonical: "wiki.list_wikiSpaces",
			mode:      "composite",
			reason:    "The CLI command routes by --type between wiki/list_wikiSpaces and drive/list_spaces, so the reviewed executable wrapper has no single direct MCP interface.",
		},
		{
			canonical: "event.consume",
			mode:      "composite",
			reason:    "Reviewed composite workflow: the command creates or reuses a remote personal-event subscription and coordinates the local event bus and Stream consumer; no single pinned RPC represents the workflow.",
		},
		{
			canonical: "event.status",
			mode:      "composite",
			reason:    "Reviewed composite workflow: the command reads the remote personal-event subscription control plane and combines it with local bus and consumer state; no single pinned RPC represents the result.",
		},
		{
			canonical: "event.stop",
			mode:      "composite",
			reason:    "Reviewed composite workflow: the command deletes remote personal-event subscriptions, interrupts local consumers, updates local state, and may stop the local bus; no single pinned RPC represents the workflow.",
		},
	}
	for _, canonical := range []string{
		"aitable.view_update_aggregate",
		"aitable.view_update_card",
		"aitable.view_update_field_widths",
		"aitable.view_update_timebar",
	} {
		tests = append(tests, interfaceCase{
			canonical: canonical,
			mode:      "composite",
			reason:    "The CLI performs an aitable/get_views preflight, locally transforms the requested configuration, and then calls aitable/update_view; the two-call workflow has no single direct MCP interface.",
		})
	}

	canonicals := make([]string, 0, len(tests))
	for _, test := range tests {
		canonicals = append(canonicals, test.canonical)
	}
	payload := schemaContractPayloadForBoundCanonicals(t, NewRootCommand(), canonicals...)

	for _, test := range tests {
		test := test
		t.Run(test.canonical, func(t *testing.T) {
			tool := payload.Tools[test.canonical]
			if got := schemaContractString(tool["interface_mode"]); got != test.mode {
				t.Errorf("interface_mode = %q, want %q", got, test.mode)
			}
			if got := schemaContractString(tool["availability"]); got != "available" {
				t.Errorf("availability = %q, want available", got)
			}
			if tool["interface_ref"] != nil {
				t.Errorf("interface_ref = %#v, want nil for %s wrapper", tool["interface_ref"], test.mode)
			}
			if got := schemaContractString(tool["interface_reason"]); got != test.reason {
				t.Errorf("interface_reason = %q, want %q", got, test.reason)
			}

			provenance := schemaContractMap(tool["field_provenance"])
			for _, field := range []string{"interface_mode", "availability", "interface_ref", "interface_reason"} {
				entry := provenance[field]
				if entry == nil {
					t.Errorf("missing %s provenance", field)
					continue
				}
				if got := schemaContractString(entry["precedence"]); got != "contract_final" {
					t.Errorf("%s provenance precedence = %q, want contract_final", field, got)
				}
				if got := schemaContractString(entry["source"]); got != "corecmd.contract" {
					t.Errorf("%s provenance source = %q, want corecmd.contract", field, got)
				}
			}
			if got := provenance["interface_ref"]["value"]; got != nil {
				t.Errorf("interface_ref provenance value = %#v, want explicit null", got)
			}
		})
	}
}

func TestViewGetWrappersUsePinnedGetViewsInterface(t *testing.T) {
	canonicals := []string{
		"aitable.view_get_aggregate",
		"aitable.view_get_card",
		"aitable.view_get_field_widths",
		"aitable.view_get_fill_color_rule",
		"aitable.view_get_filter",
		"aitable.view_get_group",
		"aitable.view_get_sort",
		"aitable.view_get_timebar",
		"aitable.view_get_visible_fields",
	}
	payload := schemaContractPayloadForBoundCanonicals(t, NewRootCommand(), canonicals...)
	for _, canonical := range canonicals {
		tool := payload.Tools[canonical]
		if got := schemaContractString(tool["interface_mode"]); got != "mcp" {
			t.Errorf("%s interface_mode = %q, want mcp", canonical, got)
		}
		if got := schemaContractString(tool["availability"]); got != "available" {
			t.Errorf("%s availability = %q, want available", canonical, got)
		}
		ref := schemaInterfaceObject(tool["interface_ref"])
		if product, rpc := schemaContractString(ref["product_id"]), schemaContractString(ref["rpc_name"]); product != "aitable" || rpc != "get_views" {
			t.Errorf("%s interface_ref = %q/%q, want aitable/get_views", canonical, product, rpc)
		}
		if got := schemaContractString(tool["interface_reason"]); got != "" {
			t.Errorf("%s interface_reason = %q, want empty for direct pinned interface", canonical, got)
		}
		parameters := schemaContractMap(tool["parameters"])
		for flag, property := range map[string]string{
			"base-id":  "baseId",
			"table-id": "tableId",
			"view-id":  "viewIds",
		} {
			if got := schemaContractString(parameters[flag]["property"]); got != property {
				t.Errorf("%s --%s property = %q, want %q", canonical, flag, got, property)
			}
		}
		provenance := schemaContractMap(tool["field_provenance"])
		for _, field := range []string{"interface_mode", "availability", "interface_ref"} {
			entry := provenance[field]
			if got := schemaContractString(entry["precedence"]); got != "contract_final" {
				t.Errorf("%s %s precedence = %q, want contract_final", canonical, field, got)
			}
			if got := schemaContractString(entry["source"]); got != "corecmd.contract" {
				t.Errorf("%s %s source = %q, want corecmd.contract", canonical, field, got)
			}
		}
	}
}

// TestReviewedInterfaceDispositionSourceOwnsRuntimeSurface asserts that every
// delivered Catalog tool owns interface disposition via ContractFinal
// (corecmd.contract). Former schema_hints audit JSON
// (runtime-surface-completeness / zz-interface-disposition-review) is retired.
func TestReviewedInterfaceDispositionSourceOwnsRuntimeSurface(t *testing.T) {
	tools := deliverySchemaAllToolsForHelpFlagTest(t, NewRootCommand())
	canonicals := make([]string, 0, len(tools))
	for canonical := range tools {
		canonicals = append(canonicals, canonical)
	}
	sort.Strings(canonicals)
	if len(canonicals) == 0 {
		t.Fatal("delivery schema --all contains no tools")
	}
	for _, canonical := range canonicals {
		tool := tools[canonical]
		mode := schemaContractString(tool["interface_mode"])
		availability := schemaContractString(tool["availability"])
		switch mode {
		case "mcp", "composite", "local":
		default:
			t.Errorf("%s interface_mode = %q, want mcp|composite|local", canonical, mode)
		}
		switch availability {
		case "available", "unavailable":
		default:
			t.Errorf("%s availability = %q, want available|unavailable", canonical, availability)
		}
		switch {
		case availability == "unavailable":
			if tool["interface_ref"] != nil {
				t.Errorf("%s unavailable interface_ref = %#v, want nil", canonical, tool["interface_ref"])
			}
			if schemaContractString(tool["interface_reason"]) == "" {
				t.Errorf("%s unavailable disposition missing interface_reason", canonical)
			}
		case mode == "mcp":
			ref := schemaInterfaceObject(tool["interface_ref"])
			if schemaContractString(ref["product_id"]) == "" || schemaContractString(ref["rpc_name"]) == "" {
				t.Errorf("%s mcp disposition has incomplete interface_ref", canonical)
			}
		case mode == "composite":
			if tool["interface_ref"] != nil {
				t.Errorf("%s composite interface_ref = %#v, want nil", canonical, tool["interface_ref"])
			}
			if schemaContractString(tool["interface_reason"]) == "" {
				t.Errorf("%s composite disposition missing interface_reason", canonical)
			}
		case mode == "local":
			if tool["interface_ref"] != nil {
				t.Errorf("%s local interface_ref = %#v, want nil", canonical, tool["interface_ref"])
			}
		}
		provenance := schemaContractMap(tool["field_provenance"])
		fields := []string{"interface_mode", "availability", "interface_ref"}
		if mode == "composite" || availability == "unavailable" {
			fields = append(fields, "interface_reason")
		}
		for _, field := range fields {
			entry := provenance[field]
			if got := schemaContractString(entry["precedence"]); got != "contract_final" {
				t.Errorf("%s %s precedence = %q, want contract_final", canonical, field, got)
			}
			if got := schemaContractString(entry["source"]); got != "corecmd.contract" {
				t.Errorf("%s %s source = %q, want corecmd.contract", canonical, field, got)
			}
		}
	}
}

func schemaInterfaceObject(value any) map[string]any {
	object, _ := value.(map[string]any)
	return object
}
