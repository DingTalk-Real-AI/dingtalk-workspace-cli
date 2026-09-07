// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package hrmregister

import "testing"

func TestHrmregisterToolSpecsCoverLifecycleSurface(t *testing.T) {
	if len(hrmregisterToolSpecs) != 33 {
		t.Fatalf("tool specs = %d, want 33", len(hrmregisterToolSpecs))
	}
	commands := map[string]bool{}
	tools := map[string]bool{}
	for _, spec := range hrmregisterToolSpecs {
		if spec.command == "" || spec.tool == "" || spec.description == "" || spec.intent == "" {
			t.Fatalf("incomplete tool spec: %#v", spec)
		}
		if commands[spec.command] {
			t.Fatalf("duplicate command %s", spec.command)
		}
		if tools[spec.tool] {
			t.Fatalf("duplicate tool %s", spec.tool)
		}
		commands[spec.command] = true
		tools[spec.tool] = true
	}
}

func TestHrmregisterReadableTimeConversion(t *testing.T) {
	value, err := parseMillis("2026-09-15")
	if err != nil || value <= 0 {
		t.Fatalf("parseMillis = %d, %v", value, err)
	}
	if _, err := parseMillis("not-a-date"); err == nil {
		t.Fatal("invalid date was accepted")
	}
}

func TestHrmregisterJSONValidation(t *testing.T) {
	if _, err := parseJSONArray(`[{"fieldCode":"name"}]`, "--groups-json"); err != nil {
		t.Fatal(err)
	}
	if _, err := parseJSONArray(`[]`, "--groups-json"); err == nil {
		t.Fatal("empty JSON array was accepted")
	}
	if err := validateJSONObjectString(`{"staffId":"staff-1"}`, "--input-json"); err != nil {
		t.Fatal(err)
	}
}
