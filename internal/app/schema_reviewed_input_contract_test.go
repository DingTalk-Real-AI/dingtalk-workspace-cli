// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package app

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestSchemaReviewedInputMetadataUsesExecutableShapes(t *testing.T) {
	canonicals := []string{
		"aitable.field_update",
		"attendance.adjustment_search",
		"attendance.approve_list",
		"attendance.group_search",
		"attendance.overtime_search",
		"attendance.selfsetting_get",
		"attendance.vacation_update_type",
		"chat.list_owned_or_admin_groups",
		"sheet.batch_update",
		"sheet.chart_create",
		"sheet.chart_update",
		"sheet.create_pivot_table",
		"sheet.update_pivot_table",
		"sheet.range_batch_clear",
		"todo.add_todo_reminder",
		"todo.get_user_todos_in_current_org",
	}
	payload := schemaContractPayloadForBoundCanonicals(t, NewRootCommand(), canonicals...)

	optional := false
	parameterCases := []struct {
		canonical    string
		flag         string
		format       string
		enum         []string
		noEnum       bool
		example      string
		required     *bool
		requiredWhen string
		jsonKind     byte
	}{
		{canonical: "attendance.approve_list", flag: "start", format: "date"},
		{canonical: "attendance.approve_list", flag: "end", format: "date"},
		{canonical: "attendance.approve_list", flag: "types", noEnum: true, example: "overtime,leave"},
		{canonical: "attendance.adjustment_search", flag: "page", required: &optional},
		{canonical: "attendance.adjustment_search", flag: "limit", required: &optional},
		{canonical: "attendance.group_search", flag: "page", required: &optional},
		{canonical: "attendance.group_search", flag: "limit", required: &optional},
		{canonical: "attendance.overtime_search", flag: "page", required: &optional},
		{canonical: "attendance.overtime_search", flag: "limit", required: &optional},
		{canonical: "attendance.selfsetting_get", flag: "setting-scene", enum: []string{
			"checkRemind", "fastCheck", "checkResultNotify", "lackRemind",
			"personalAttendStatNotify", "bossAttendStatNotify",
		}},
		{canonical: "chat.list_owned_or_admin_groups", flag: "role", enum: []string{"OWNER", "ADMIN"}, required: &optional},
		{canonical: "chat.list_owned_or_admin_groups", flag: "limit", required: &optional},
		{canonical: "sheet.batch_update", flag: "operations", format: "json", jsonKind: '['},
		{canonical: "sheet.chart_create", flag: "properties", format: "json", jsonKind: '{'},
		{canonical: "sheet.chart_update", flag: "properties", format: "json", jsonKind: '{'},
		{canonical: "sheet.create_pivot_table", flag: "properties", format: "json", jsonKind: '{'},
		{canonical: "sheet.update_pivot_table", flag: "properties", format: "json", jsonKind: '{'},
		{canonical: "sheet.range_batch_clear", flag: "ranges", format: "json", jsonKind: '['},
		{canonical: "todo.add_todo_reminder", flag: "base-time", enum: []string{"dueTime", "customTime"}},
		{canonical: "todo.add_todo_reminder", flag: "due-date-offset", example: "-30", requiredWhen: "base-time is dueTime"},
		{canonical: "todo.add_todo_reminder", flag: "reminder-time-stamp", example: "2026-03-10T18:00:00+08:00", requiredWhen: "base-time is customTime"},
		{canonical: "todo.get_user_todos_in_current_org", flag: "role-types", noEnum: true, example: "creator,executor"},
	}
	for _, test := range parameterCases {
		t.Run(test.canonical+"/"+test.flag, func(t *testing.T) {
			tool := payload.Tools[test.canonical]
			parameter := schemaContractMap(tool["parameters"])[test.flag]
			if test.format != "" && parameter["format"] != test.format {
				t.Fatalf("format = %#v, want %q", parameter["format"], test.format)
			}
			if test.enum != nil {
				if got := schemaContractStringSlice(parameter["enum"]); !reflect.DeepEqual(got, test.enum) {
					t.Fatalf("enum = %#v, want %#v", got, test.enum)
				}
			}
			if test.noEnum {
				if got := schemaContractStringSlice(parameter["enum"]); len(got) != 0 {
					t.Fatalf("enum = %#v, want no scalar enum for a CSV parameter", got)
				}
			}
			if test.example != "" && parameter["example"] != test.example {
				t.Fatalf("example = %#v, want %q", parameter["example"], test.example)
			}
			if test.required != nil && parameter["required"] != *test.required {
				t.Fatalf("required = %#v, want %v", parameter["required"], *test.required)
			}
			if test.requiredWhen != "" && parameter["required_when"] != test.requiredWhen {
				t.Fatalf("required_when = %#v, want %q", parameter["required_when"], test.requiredWhen)
			}
			if test.jsonKind != 0 {
				example, ok := parameter["example"].(string)
				if !ok || example == "" {
					t.Fatalf("example = %#v, want non-empty JSON", parameter["example"])
				}
				var decoded any
				if err := json.Unmarshal([]byte(example), &decoded); err != nil {
					t.Fatalf("example is invalid JSON: %v", err)
				}
				if example[0] != test.jsonKind {
					t.Fatalf("example = %s, want JSON kind %q", example, test.jsonKind)
				}
				switch value := decoded.(type) {
				case []any:
					if len(value) == 0 {
						t.Fatal("example must not be an empty array")
					}
				case map[string]any:
					if len(value) == 0 {
						t.Fatal("example must not be an empty object")
					}
				}
			}
		})
	}

	wantUpdateFields := []string{"name", "unit", "paid", "per-hours", "when-can-leave", "visibility-rules"}
	assertSchemaContractConstraintGroup(t, payload.Tools["attendance.vacation_update_type"], "require_one_of", wantUpdateFields)
	assertSchemaContractConstraintGroup(t, payload.Tools["aitable.field_update"], "require_one_of", []string{"name", "config", "ai-config"})
}
