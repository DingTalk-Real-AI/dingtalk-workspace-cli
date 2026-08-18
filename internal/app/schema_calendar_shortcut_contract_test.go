// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package app

import "testing"

func TestCrossPlatformCoverageCalendarAgendaFinalSchemaPreservesCompositeProperties(t *testing.T) {
	snapshot := fullSchemaSnapshotForTest(t)
	tool := snapshot.Tools["calendar.shortcut_agenda"]
	if tool == nil {
		t.Fatal("calendar.shortcut_agenda is missing from final Schema")
	}
	parameters := schemaContractMap(tool["parameters"])
	for flag, want := range map[string]string{
		"start": "start",
		"end":   "end",
	} {
		parameter := parameters[flag]
		if parameter == nil {
			t.Fatalf("calendar.shortcut_agenda --%s is missing from final Schema", flag)
		}
		if got := schemaContractString(parameter["property"]); got != want {
			t.Errorf("calendar.shortcut_agenda --%s property=%q, want %q", flag, got, want)
		}
	}
	if got := schemaContractString(tool["interface_mode"]); got != "composite" {
		t.Fatalf("calendar.shortcut_agenda interface_mode=%q, want composite", got)
	}
	result := schemaContractMap(tool["result"])
	dataSchema := schemaContractMap(result["data_schema"])
	properties := schemaContractMap(dataSchema["properties"])
	for _, field := range []string{"hasMore", "nextCursor"} {
		if _, exists := properties[field]; exists {
			t.Fatalf("calendar.shortcut_agenda Result data_schema leaked pagination field %q", field)
		}
	}
	if properties["complete"] == nil {
		t.Fatal("calendar.shortcut_agenda Result data_schema is missing complete")
	}
	pagination, ok := tool["pagination"].(map[string]any)
	if !ok {
		t.Fatalf("calendar.shortcut_agenda pagination=%T, want object", tool["pagination"])
	}
	if got := schemaContractString(pagination["meta_path"]); got != "meta.pagination" {
		t.Fatalf("calendar.shortcut_agenda pagination meta_path=%q, want meta.pagination", got)
	}
}
