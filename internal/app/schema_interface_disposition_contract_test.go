// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package app

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"
)

func TestReviewedRoutedInterfacesReachFinalSchema(t *testing.T) {
	type interfaceCase struct {
		canonical    string
		mode         string
		reason       string
		sourceSuffix string
	}
	tests := []interfaceCase{
		{
			canonical:    "attendance.get_attendance_summary",
			mode:         "composite",
			reason:       "Reviewed unpinned remote adapter: the CLI calls attendance-wukong/get_user_attendance_summary, which is absent from the pinned MCP metadata snapshot; the incompatible attendance/get_attendance_summary contract must not be advertised.",
			sourceSuffix: "internal/cli/schema_hints/metadata/attendance.json",
		},
		{
			canonical:    "drive.list_files",
			mode:         "composite",
			reason:       "The CLI command routes by --workspace between drive/list_files and doc/list_nodes, so the reviewed executable wrapper has no single direct MCP interface.",
			sourceSuffix: "internal/cli/schema_hints/metadata/drive.json",
		},
		{
			canonical:    "chat.search_groups",
			mode:         "composite",
			reason:       "Reviewed unpinned remote adapter: the CLI calls im/search_groups with a flat payload, while the pinned snapshot only contains the incompatible chat/search_groups_by_keyword contract.",
			sourceSuffix: "internal/cli/schema_hints/metadata/chat.json",
		},
		{
			canonical:    "sheet.range_batch_set_style",
			mode:         "composite",
			reason:       "The CLI reads a local batch file and performs multiple sheet/update_range calls with local continue-on-error control; the workflow has no single direct MCP interface.",
			sourceSuffix: "internal/cli/schema_hints/metadata/sheet.json",
		},
		{
			canonical:    "sheet.range_read",
			mode:         "composite",
			reason:       "Reviewed unpinned remote adapter: the CLI calls sheet/get_cell_infos, which is absent from the pinned MCP metadata snapshot; the incompatible sheet/get_range contract must not be advertised.",
			sourceSuffix: "internal/cli/schema_hints/metadata/sheet.json",
		},
		{
			canonical:    "wiki.list_wikiSpaces",
			mode:         "composite",
			reason:       "The CLI command routes by --type between wiki/list_wikiSpaces and drive/list_spaces, so the reviewed executable wrapper has no single direct MCP interface.",
			sourceSuffix: "internal/cli/schema_hints/metadata/wiki.json",
		},
		{
			canonical:    "event.consume",
			mode:         "composite",
			reason:       "Reviewed composite workflow: the command creates or reuses a remote personal-event subscription and coordinates the local event bus and Stream consumer; no single pinned RPC represents the workflow.",
			sourceSuffix: "internal/cli/schema_hints/metadata/event.json",
		},
		{
			canonical:    "event.status",
			mode:         "composite",
			reason:       "Reviewed composite workflow: the command reads the remote personal-event subscription control plane and combines it with local bus and consumer state; no single pinned RPC represents the result.",
			sourceSuffix: "internal/cli/schema_hints/metadata/event.json",
		},
		{
			canonical:    "event.stop",
			mode:         "composite",
			reason:       "Reviewed composite workflow: the command deletes remote personal-event subscriptions, interrupts local consumers, updates local state, and may stop the local bus; no single pinned RPC represents the workflow.",
			sourceSuffix: "internal/cli/schema_hints/metadata/event.json",
		},
	}
	for _, canonical := range []string{
		"aitable.view_update_aggregate",
		"aitable.view_update_card",
		"aitable.view_update_field_widths",
		"aitable.view_update_timebar",
	} {
		tests = append(tests, interfaceCase{
			canonical:    canonical,
			mode:         "composite",
			reason:       "The CLI performs an aitable/get_views preflight, locally transforms the requested configuration, and then calls aitable/update_view; the two-call workflow has no single direct MCP interface.",
			sourceSuffix: "internal/cli/schema_hints/metadata/aitable.json",
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
				if got := schemaContractString(entry["precedence"]); got != "reviewed_explicit" {
					t.Errorf("%s provenance precedence = %q, want reviewed_explicit", field, got)
				}
				if got := schemaContractString(entry["source"]); !strings.HasSuffix(got, test.sourceSuffix) {
					t.Errorf("%s provenance source = %q, want suffix %q", field, got, test.sourceSuffix)
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
			if got := schemaContractString(entry["precedence"]); got != "reviewed_explicit" {
				t.Errorf("%s %s precedence = %q, want reviewed_explicit", canonical, field, got)
			}
			if got := schemaContractString(entry["source"]); !strings.Contains(got, "internal/cli/schema_hints/metadata/") {
				t.Errorf("%s %s source = %q, want reviewed interface disposition source", canonical, field, got)
			}
		}
	}
}

func TestReviewedInterfaceDispositionSourceOwnsRuntimeSurface(t *testing.T) {
	type hintFile struct {
		Source map[string]any            `json:"source"`
		Tools  map[string]map[string]any `json:"tools"`
	}
	load := func(path string) hintFile {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var value hintFile
		if err := json.Unmarshal(data, &value); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		return value
	}
	runtimeSurface := load("../cli/schema_hints/runtime-surface-completeness.json")
	legacyDispositionKeys := load("../cli/schema_hints/zz-interface-disposition-review.json").Tools
	dispositions := hintFile{Source: map[string]any{"reviewed": true}, Tools: map[string]map[string]any{}}
	for _, product := range []string{
		"attendance", "aitable", "chat", "drive", "event", "sheet", "wiki", "doc", "mail", "todo", "calendar", "conference", "contact", "dev", "devdoc", "ding", "live", "minutes", "oa", "pat", "report", "aisearch",
	} {
		path := "../cli/schema_hints/metadata/" + product + ".json"
		if _, err := os.Stat(path); err != nil {
			continue
		}
		file := load(path)
		for canonical, hint := range file.Tools {
			if _, ok := legacyDispositionKeys[canonical]; !ok {
				continue
			}
			trimmed := map[string]any{}
			for _, field := range []string{"interface_mode", "availability", "interface_ref", "interface_reason"} {
				if value, exists := hint[field]; exists {
					trimmed[field] = value
				}
			}
			dispositions.Tools[canonical] = trimmed
		}
	}
	if dispositions.Source["reviewed"] != true {
		t.Fatalf("interface disposition source reviewed = %#v, want true", dispositions.Source["reviewed"])
	}
	for canonical, hint := range runtimeSurface.Tools {
		if hint["reviewed"] != false {
			t.Errorf("%s runtime surface reviewed = %#v, want false", canonical, hint["reviewed"])
		}
		for _, field := range []string{"interface_mode", "availability", "interface_ref", "interface_reason"} {
			if _, exists := hint[field]; exists {
				t.Errorf("%s runtime surface still owns %s", canonical, field)
			}
		}
		if _, exists := dispositions.Tools[canonical]; !exists {
			t.Errorf("%s runtime surface has no reviewed interface disposition", canonical)
		}
	}

	allowedFields := map[string]bool{
		"interface_mode":   true,
		"availability":     true,
		"interface_ref":    true,
		"interface_reason": true,
	}
	canonicals := make([]string, 0, len(dispositions.Tools))
	for canonical, hint := range dispositions.Tools {
		canonicals = append(canonicals, canonical)
		for field := range hint {
			if !allowedFields[field] {
				t.Errorf("%s interface-only source contains non-interface field %s", canonical, field)
			}
		}
		mode := schemaContractString(hint["interface_mode"])
		if mode == "local" {
			t.Errorf("%s remote interface review is incorrectly classified local", canonical)
		}
		if schemaContractString(hint["availability"]) != "available" {
			t.Errorf("%s reviewed disposition is not available", canonical)
		}
		switch mode {
		case "mcp":
			ref := schemaInterfaceObject(hint["interface_ref"])
			if schemaContractString(ref["product_id"]) == "" || schemaContractString(ref["rpc_name"]) == "" {
				t.Errorf("%s reviewed mcp disposition has no complete interface_ref", canonical)
			}
		case "composite":
			if hint["interface_ref"] != nil {
				t.Errorf("%s reviewed composite disposition advertises interface_ref %#v", canonical, hint["interface_ref"])
			}
			if schemaContractString(hint["interface_reason"]) == "" {
				t.Errorf("%s reviewed composite disposition has no reason", canonical)
			}
		default:
			t.Errorf("%s reviewed disposition mode = %q", canonical, mode)
		}
	}
	sort.Strings(canonicals)
	payload := schemaContractPayloadForBoundCanonicals(t, NewRootCommand(), canonicals...)
	for _, canonical := range canonicals {
		want := dispositions.Tools[canonical]
		tool := payload.Tools[canonical]
		if got := schemaContractString(tool["interface_mode"]); got != schemaContractString(want["interface_mode"]) {
			t.Errorf("%s final interface_mode = %q, want %q", canonical, got, want["interface_mode"])
		}
		if got := schemaContractString(tool["availability"]); got != schemaContractString(want["availability"]) {
			t.Errorf("%s final availability = %q, want %q", canonical, got, want["availability"])
		}
		if schemaContractString(want["interface_mode"]) == "composite" {
			if tool["interface_ref"] != nil {
				t.Errorf("%s final composite interface_ref = %#v, want nil", canonical, tool["interface_ref"])
			}
			if got := schemaContractString(tool["interface_reason"]); got != schemaContractString(want["interface_reason"]) {
				t.Errorf("%s final interface_reason = %q, want %q", canonical, got, want["interface_reason"])
			}
		} else {
			gotRef := schemaInterfaceObject(tool["interface_ref"])
			wantRef := schemaInterfaceObject(want["interface_ref"])
			for _, field := range []string{"product_id", "rpc_name"} {
				if got := schemaContractString(gotRef[field]); got != schemaContractString(wantRef[field]) {
					t.Errorf("%s final interface_ref.%s = %q, want %q", canonical, field, got, wantRef[field])
				}
			}
		}
		provenance := schemaContractMap(tool["field_provenance"])
		fields := []string{"interface_mode", "availability", "interface_ref"}
		if schemaContractString(want["interface_mode"]) == "composite" {
			fields = append(fields, "interface_reason")
		}
		for _, field := range fields {
			entry := provenance[field]
			if got := schemaContractString(entry["precedence"]); got != "reviewed_explicit" {
				t.Errorf("%s final %s precedence = %q, want reviewed_explicit", canonical, field, got)
			}
			if got := schemaContractString(entry["source"]); !strings.Contains(got, "internal/cli/schema_hints/metadata/") {
				t.Errorf("%s final %s source = %q, want reviewed disposition source", canonical, field, got)
			}
		}
	}
}

func schemaInterfaceObject(value any) map[string]any {
	object, _ := value.(map[string]any)
	return object
}
