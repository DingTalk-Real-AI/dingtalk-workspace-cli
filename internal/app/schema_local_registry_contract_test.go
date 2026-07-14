// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"reflect"
	"sort"
	"testing"
)

func TestEventRegistryDeliversOneTypedSchemaPath(t *testing.T) {
	paths := map[string]string{
		"event.consume": "event consume",
		"event.list":    "event list",
		"event.schema":  "event schema",
		"event.status":  "event status",
		"event.stop":    "event stop",
	}
	wantModes := map[string]string{
		"event.consume": "composite",
		"event.list":    "local",
		"event.schema":  "local",
		"event.status":  "composite",
		"event.stop":    "composite",
	}
	canonicals := make([]string, 0, len(paths))
	for canonical := range paths {
		canonicals = append(canonicals, canonical)
	}
	sort.Strings(canonicals)
	payload := schemaContractPayloadForBoundCanonicals(t, NewRootCommand(), canonicals...)
	if len(payload.Tools) != len(paths) {
		t.Fatalf("event fixture tools = %d, want %d", len(payload.Tools), len(paths))
	}
	if _, exists := payload.Tools["event._bus"]; exists {
		t.Fatal("hidden event _bus leaked into final Schema")
	}
	for canonical, primary := range paths {
		tool := payload.Tools[canonical]
		if tool == nil {
			t.Errorf("missing event tool %s", canonical)
			continue
		}
		if got := schemaContractString(tool["primary_cli_path"]); got != primary {
			t.Errorf("%s primary path = %q, want %q", canonical, got, primary)
		}
		if tool["interface_mode"] != wantModes[canonical] || tool["availability"] != "available" {
			t.Errorf("%s interface disposition = %v/%v, want %s/available", canonical, tool["interface_mode"], tool["availability"], wantModes[canonical])
		}
		if schemaContractString(tool["interface_reason"]) == "" {
			t.Errorf("%s has no reviewed interface reason", canonical)
		}
	}

	consume := payload.Tools["event.consume"]
	consumeParams := schemaContractMap(consume["parameters"])
	for _, hiddenOrGlobal := range []string{"as", "debug", "help", "profile", "timeout", "yes"} {
		if _, exists := consumeParams[hiddenOrGlobal]; exists {
			t.Errorf("event.consume exposes hidden/global flag --%s as a tool parameter", hiddenOrGlobal)
		}
	}
	for flag, wantType := range map[string]string{
		"dry-run":     "boolean",
		"duration":    "string",
		"event-types": "array",
		"max-events":  "integer",
	} {
		if got := schemaContractString(consumeParams[flag]["type"]); got != wantType {
			t.Errorf("event.consume --%s type = %q, want %q", flag, got, wantType)
		}
	}
	if _, exists := consumeParams["duration"]["default"]; exists {
		t.Error("event.consume --duration leaked zero default 0s")
	}
	if consume["dry_run"] != nil {
		t.Errorf("event.consume declares an audited dry_run capability without a deterministic clean-environment preview: %#v", consume["dry_run"])
	}
	assertSchemaContractPositional(t, consume, "event_key", false)
	assertSchemaContractConstraintGroup(t, consume, "require_one_of", []string{"event_key", "subscribe-id"})

	eventSchema := payload.Tools["event.schema"]
	assertSchemaContractPositional(t, eventSchema, "event_key", true)

	stop := payload.Tools["event.stop"]
	if stop["effect"] != "destructive" || stop["risk"] != "high" || stop["confirmation"] != "user_required" {
		t.Errorf("event.stop safety = effect:%v risk:%v confirmation:%v", stop["effect"], stop["risk"], stop["confirmation"])
	}
	dryRun, _ := stop["dry_run"].(map[string]any)
	if dryRun["preview_kind"] != "request" {
		t.Errorf("event.stop dry_run = %#v, want request preview", stop["dry_run"])
	}
	assertSchemaContractPositional(t, stop, "subscribe_id", false)
	assertSchemaContractConstraintGroup(t, stop, "require_one_of", []string{"all", "subscribe_id"})
	assertSchemaContractConstraintGroup(t, stop, "mutually_exclusive", []string{"all", "subscribe_id"})
}

func TestDevDocSearchBindingAndRequiredAlternativesReachFinalSchema(t *testing.T) {
	canonicals := []string{"dev.search_open_platform_docs_rag", "devdoc.search_open_platform_docs_rag"}
	payload := schemaContractPayloadForBoundCanonicals(t, NewRootCommand(), canonicals...)
	for _, canonical := range canonicals {
		tool := payload.Tools[canonical]
		query := schemaContractMap(tool["parameters"])["query"]
		if got := schemaContractString(query["property"]); got != "keyword" {
			t.Errorf("%s --query property = %q, want keyword", canonical, got)
		}
		if query["required"] != false {
			t.Errorf("%s --query required = %#v, want false because positional keyword is an alternative", canonical, query["required"])
		}
		assertSchemaContractConstraintGroup(t, tool, "require_one_of", []string{"query", "keyword"})
	}
}

func assertSchemaContractPositional(t *testing.T, tool map[string]any, name string, required bool) {
	t.Helper()
	positionals, _ := tool["positionals"].([]any)
	for _, raw := range positionals {
		positional, _ := raw.(map[string]any)
		if positional["name"] == name {
			if positional["required"] != required {
				t.Errorf("positional %s required = %#v, want %v", name, positional["required"], required)
			}
			return
		}
	}
	t.Errorf("missing positional %s in %#v", name, tool["positionals"])
}

func assertSchemaContractConstraintGroup(t *testing.T, tool map[string]any, kind string, want []string) {
	t.Helper()
	constraints, _ := tool["constraints"].(map[string]any)
	groups, _ := constraints[kind].([]any)
	for _, rawGroup := range groups {
		rawValues, _ := rawGroup.([]any)
		values := make([]string, 0, len(rawValues))
		for _, value := range rawValues {
			if text, ok := value.(string); ok {
				values = append(values, text)
			}
		}
		if reflect.DeepEqual(values, want) {
			return
		}
	}
	t.Errorf("constraint %s lacks group %v: %#v", kind, want, constraints[kind])
}
