// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import "testing"

func TestSchemaParameterBindingsMatchEmbeddedCatalog(t *testing.T) {
	loaded := embeddedSchemaCatalog()
	snapshot := runtimeSchemaParameterBindingData()
	bindingsSnapshot := snapshot.Bindings
	count := 0
	for canonical, bindings := range bindingsSnapshot {
		detail, ok := loaded.Snapshot.Tools[canonical]
		if !ok {
			t.Errorf("binding references unknown canonical path %q", canonical)
			continue
		}
		parameters, _ := detail["parameters"].(map[string]any)
		for flagName, propertyName := range bindings {
			count++
			parameter, _ := parameters[flagName].(map[string]any)
			if parameter == nil {
				t.Errorf("binding %s --%s references an unknown flag", canonical, flagName)
				continue
			}
			if got := schemaString(parameter["property"]); got != propertyName {
				t.Errorf("binding %s --%s property = %q, want %q", canonical, flagName, got, propertyName)
			}
		}
	}
	if count == 0 {
		t.Fatal("active parameter binding count is zero")
	}
	if snapshot.HistoricalBindingCount != 311 || len(snapshot.Migrations) != 5 || len(snapshot.Excluded) != 3 || len(snapshot.Added) != 25 {
		t.Fatalf("binding seed audit = historical:%d migrations:%d excluded:%d added:%d",
			snapshot.HistoricalBindingCount, len(snapshot.Migrations), len(snapshot.Excluded), len(snapshot.Added))
	}
	if got := bindingsSnapshot["calendar.get_calendar"]["id"]; got != "calendarId" {
		t.Fatalf("calendar.get_calendar --id property = %q, want calendarId", got)
	}
	for _, canonical := range []string{"dev.search_open_platform_docs_rag", "devdoc.search_open_platform_docs_rag"} {
		if got := bindingsSnapshot[canonical]["query"]; got != "keyword" {
			t.Errorf("%s --query property = %q, want keyword", canonical, got)
		}
	}
	for canonical, flagName := range map[string]string{
		"calendar.add_calendar_participant":    "event",
		"calendar.get_calendar_participants":   "event",
		"calendar.remove_calendar_participant": "event",
		"contact.get_dept_info_by_dept_id":     "dept",
		"contact.get_dept_members_by_deptId":   "depts",
		"contact.get_sub_depts_by_dept_id":     "dept",
		"oa.list_pending_approvals":            "limit",
		"oa.list_user_visible_process":         "limit",
	} {
		if bindingsSnapshot[canonical][flagName] == "" {
			t.Errorf("migrated binding %s --%s is missing", canonical, flagName)
		}
	}
}
