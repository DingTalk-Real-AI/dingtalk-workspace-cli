// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import "testing"

func TestSchemaParameterBindingsMatchEmbeddedCatalog(t *testing.T) {
	count := 0
	for canonical, bindings := range runtimeSchemaParameterBindings {
		detail, ok := runtimeEmbeddedSchemaCatalog.Snapshot.Tools[canonical]
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
	if count != 303 {
		t.Fatalf("parameter binding count = %d, want 303", count)
	}
	if got := runtimeSchemaParameterBindings["calendar.get_calendar"]["id"]; got != "calendarId" {
		t.Fatalf("calendar.get_calendar --id property = %q, want calendarId", got)
	}
}
