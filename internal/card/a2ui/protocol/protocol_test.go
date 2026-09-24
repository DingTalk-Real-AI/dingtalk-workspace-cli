// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package protocol

import (
	"strings"
	"testing"
)

func TestLoadAndValidateCreate(t *testing.T) {
	registry, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	result := registry.Validate([]map[string]any{{
		"version": "v1.0",
		"createSurface": map[string]any{
			"surfaceId": "card",
			"catalogId": registry.Catalog().CatalogID,
			"components": []any{
				map[string]any{"id": "root", "component": "Card", "child": "body"},
				map[string]any{"id": "body", "component": "Text", "text": "Hello"},
			},
		},
	}}, "create")
	if !result.Valid {
		t.Fatalf("expected valid create: %+v", result.Diagnostics)
	}
	if result.Components != 2 || result.SurfaceID != "card" {
		t.Fatalf("unexpected summary: %+v", result)
	}
}

func TestValidateGraphFollowsCardHeaderTrailingReference(t *testing.T) {
	registry, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	result := registry.Validate([]map[string]any{{
		"version": "v1.0",
		"createSurface": map[string]any{
			"surfaceId": "card",
			"catalogId": registry.Catalog().CatalogID,
			"components": []any{
				map[string]any{"id": "root", "component": "Card", "child": "header"},
				map[string]any{"id": "header", "component": "CardHeader", "title": "日程提醒", "trailing": "status"},
				map[string]any{"id": "status", "component": "Tag", "text": "需处理"},
			},
		},
	}}, "create")
	if !result.Valid || hasDiagnostic(result.Diagnostics, "A2UI_COMPONENT_UNREACHABLE") {
		t.Fatalf("expected trailing component to be reachable: %+v", result.Diagnostics)
	}

	missing := registry.Validate([]map[string]any{{
		"version": "v1.0",
		"createSurface": map[string]any{
			"surfaceId": "card",
			"catalogId": registry.Catalog().CatalogID,
			"components": []any{
				map[string]any{"id": "root", "component": "Card", "child": "header"},
				map[string]any{"id": "header", "component": "CardHeader", "title": "日程提醒", "trailing": "missing"},
			},
		},
	}}, "create")
	if missing.Valid || !hasDiagnostic(missing.Diagnostics, "A2UI_COMPONENT_REF_MISSING") {
		t.Fatalf("expected missing trailing reference failure: %+v", missing.Diagnostics)
	}
}

func TestValidateRejectsRadioButtonAndMissingCatalog(t *testing.T) {
	registry, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	result := registry.Validate([]map[string]any{{
		"version": "v1.0",
		"createSurface": map[string]any{
			"surfaceId":  "card",
			"components": []any{map[string]any{"id": "choice", "component": "RadioButton"}},
		},
	}}, "create")
	if result.Valid {
		t.Fatal("expected validation failure")
	}
	codes := map[string]bool{}
	for _, diagnostic := range result.Diagnostics {
		codes[diagnostic.Code] = true
	}
	if !codes["A2UI_CATALOG_REQUIRED"] || !codes["A2UI_COMPONENT_UNKNOWN"] {
		t.Fatalf("unexpected diagnostics: %+v", result.Diagnostics)
	}
}

func TestParseMessagesSupportsObjectArrayWireAndJSONL(t *testing.T) {
	inputs := []string{
		`[{"version":"v1.0","deleteSurface":{"surfaceId":"s"}}]`,
		`["{\"version\":\"v1.0\",\"deleteSurface\":{\"surfaceId\":\"s\"}}"]`,
		"{\"version\":\"v1.0\",\"deleteSurface\":{\"surfaceId\":\"s\"}}\n",
	}
	for _, input := range inputs {
		messages, err := ParseMessages(strings.NewReader(input))
		if err != nil || len(messages) != 1 {
			t.Fatalf("ParseMessages(%q) = %v, %v", input, messages, err)
		}
	}
}

func TestValidateUpdateAndAppendContracts(t *testing.T) {
	registry, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	valid := registry.Validate([]map[string]any{
		{"version": "v1.0", "updateDataModel": map[string]any{"surfaceId": "s", "path": "/stream/body", "value": "checkpoint"}},
		{"version": "v0.9.1", "appendDataModel": map[string]any{"surfaceId": "s", "path": "/stream/body", "value": " delta"}},
	}, "update")
	if !valid.Valid {
		t.Fatalf("expected valid update: %+v", valid.Diagnostics)
	}
	invalid := registry.Validate([]map[string]any{{
		"version":         "v1.0",
		"appendDataModel": map[string]any{"surfaceId": "s", "path": "/card/flowStatus", "value": "x"},
	}}, "update")
	if invalid.Valid {
		t.Fatal("expected invalid append")
	}
}

func TestValidateRejectsMissingInitialBinding(t *testing.T) {
	registry, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	result := registry.Validate([]map[string]any{
		{"version": "v1.0", "createSurface": map[string]any{"surfaceId": "s", "catalogId": registry.Catalog().CatalogID}},
		{"version": "v1.0", "updateDataModel": map[string]any{"surfaceId": "s", "path": "/", "value": map[string]any{}}},
		{"version": "v1.0", "updateComponents": map[string]any{"surfaceId": "s", "components": []any{map[string]any{"id": "root", "component": "Text", "text": map[string]any{"path": "/missing"}}}}},
	}, "create")
	if result.Valid {
		t.Fatal("expected missing binding failure")
	}
	found := false
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code == "A2UI_BINDING_PATH_MISSING" {
			found = true
		}
	}
	if !found {
		t.Fatalf("diagnostics=%+v", result.Diagnostics)
	}
}

func TestValidateRejectsMalformedUpdateComponents(t *testing.T) {
	registry, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		body map[string]any
		code string
	}{
		{name: "missing", body: map[string]any{"surfaceId": "s"}, code: "A2UI_COMPONENTS_REQUIRED"},
		{name: "null", body: map[string]any{"surfaceId": "s", "components": nil}, code: "A2UI_COMPONENTS_TYPE"},
		{name: "object", body: map[string]any{"surfaceId": "s", "components": map[string]any{}}, code: "A2UI_COMPONENTS_TYPE"},
		{name: "empty", body: map[string]any{"surfaceId": "s", "components": []any{}}, code: "A2UI_COMPONENTS_EMPTY"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := registry.Validate([]map[string]any{{"version": "v1.0", "updateComponents": tc.body}}, "update")
			if result.Valid || !hasDiagnostic(result.Diagnostics, tc.code) {
				t.Fatalf("result=%+v", result)
			}
		})
	}
}

func TestDingTalkExtensionSemanticValidation(t *testing.T) {
	for _, valid := range []string{"/ui/action", "/ui/action/result_1", "/ui/a-b/c_d/segment9"} {
		if !validHostResultPath(valid) {
			t.Fatalf("valid path rejected: %s", valid)
		}
	}
	for _, invalid := range []string{"/ui/123", "/ui/a//b", "/ui/a~1b", "/other/action", "/ui/动作"} {
		if validHostResultPath(invalid) {
			t.Fatalf("invalid path accepted: %s", invalid)
		}
	}
	component := map[string]any{"metadata": map[string]any{"extensions": map[string]any{
		"dt_actionBindings":   map[string]any{},
		"dt_actionBindingsV1": map[string]any{"action": map[string]any{"resultPath": "/ui/123"}},
	}}}
	diagnostics := validateDingTalkExtensions(0, "/updateComponents/components/0", component)
	if !hasDiagnostic(diagnostics, "A2UI_ACTION_BINDINGS_VERSION") || !hasDiagnostic(diagnostics, "A2UI_HOST_RESULT_PATH") {
		t.Fatalf("diagnostics=%+v", diagnostics)
	}
}

func TestValidateDeliveryResourcesRejectsPreviewOnlyImageURLs(t *testing.T) {
	message := func(resource string) []map[string]any {
		return []map[string]any{{"version": "v1.0", "updateComponents": map[string]any{
			"surfaceId":  "s",
			"components": []any{map[string]any{"id": "hero", "component": "Image", "url": resource}},
		}}}
	}
	if err := ValidateDeliveryResources(message("https://img.alicdn.com/cover.png")); err != nil {
		t.Fatalf("https image rejected: %v", err)
	}
	for _, resource := range []string{"data:image/svg+xml;base64,PHN2Zy8+", "file:///tmp/cover.png", "http://example.com/cover.png", "/relative.png"} {
		if err := ValidateDeliveryResources(message(resource)); err == nil {
			t.Fatalf("unsafe image URL accepted: %s", resource)
		}
	}
}

func hasDiagnostic(diagnostics []Diagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}
