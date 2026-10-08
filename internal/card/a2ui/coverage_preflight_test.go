// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package a2ui

import (
	"encoding/json"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/skills"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestCrossPlatformCoverageAicardPreflightErrorAndResourceCases(t *testing.T) {
	p := testProtocol(t)
	if _, err := p.Preflight(nil, "other"); err == nil || !strings.Contains(err.Error(), "unknown preflight") {
		t.Fatalf("unknown mode: %v", err)
	}
	if _, err := p.Preflight([]string{"{"}, "resources"); err == nil {
		t.Fatal("invalid JSON must return an error")
	}
	if err := p.CheckNewCard([]string{"{"}); err == nil {
		t.Fatal("CheckNewCard must propagate JSON parsing errors")
	}
	base := `{"version":"v1.0","createSurface":{"surfaceId":"s","catalogId":"https://dingtalk.com/card/a2ui/catalogs/public/catalog.json"}}`
	for _, tc := range []struct {
		name     string
		messages []string
		code     string
	}{
		{"missing create", nil, "delivery.create_surface_missing"},
		{"relative data path", []string{base, `{"updateDataModel":{"surfaceId":"s","path":"data","value":1}}`}, "delivery.data_model_path"},
		{"non-root data path", []string{base, `{"updateDataModel":{"surfaceId":"s","path":"/ready","value":1}}`}, "delivery.components_missing"},
		{"delete operation", []string{base, `{"deleteSurface":{"surfaceId":"s"}}`}, "delivery.unsupported_create_operation"},
		{"duplicate surface", []string{base, base}, "delivery.surface_duplicated"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := p.Preflight(tc.messages, "new-card")
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, d := range r["diagnostics"].([]Diagnostic) {
				found = found || d.Code == tc.code
			}
			if !found || r["valid"] != false {
				t.Fatalf("missing %s: %+v", tc.code, r)
			}
		})
	}
	for _, tc := range []struct {
		name  string
		url   string
		code  string
		valid bool
	}{
		{"missing separator", "data:image/png;base64", "resource.invalid_data_uri", false},
		{"invalid escaping", "data:image/png;base64,%zz", "resource.invalid_data_uri", false},
		{"empty base64", "data:image/png;base64,", "resource.invalid_base64", false},
		{"valid image encoding warns", "data:image/png;base64,YQ==", "resource.base64_image_unverified", true},
		{"valid non-image base64", "data:text/plain;base64,YQ==", "", true},
		{"plain data", "data:text/plain,hello", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			message, _ := json.Marshal(map[string]any{"updateComponents": map[string]any{"components": []any{map[string]any{"url": tc.url, "nested": []any{map[string]any{"darkUrl": tc.url}}, "metadata": map[string]any{"imageUrl": "data:image/png;base64,bad!"}}}}})
			r, err := p.Preflight([]string{string(message)}, "resources")
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, d := range r["diagnostics"].([]Diagnostic) {
				found = found || d.Code == tc.code
			}
			for _, diagnostic := range r["diagnostics"].([]Diagnostic) {
				if diagnostic.Code == tc.code && tc.valid && diagnostic.Severity != "warning" {
					t.Fatal("compatible resources must only warn:", diagnostic)
				}
			}
			if r["valid"] != tc.valid || (tc.code != "" && !found) {
				t.Fatalf("want %q: %+v", tc.code, r)
			}
		})
	}
}

func TestCrossPlatformCoverageAicardLintFragmentAndEmission(t *testing.T) {
	p := testProtocol(t)
	for _, tc := range []struct {
		input    string
		fragment bool
		valid    bool
		pointer  string
	}{
		{`{"id":"root","component":"Text","text":"ok"}`, true, true, ""},
		{`[{"id":"root","component":"Text","text":"ok"}]`, true, true, ""},
		{`{"id":"root","component":"Text","text":"ok","bogus":1}`, true, false, "/bogus"},
		{`{"version":"v1.0","bogus":1}`, true, false, "/bogus"},
		{`[{"id":"root","component":"Text","text":"ok","bogus":1}]`, true, false, "/0/bogus"},
		{`[{"version":"v1.0","createSurface":{"surfaceId":"s"}},{"id":"root","component":"Text"}]`, true, false, "/1"},
	} {
		r, err := p.Lint([]byte(tc.input), tc.fragment, true)
		if err != nil || r.Valid != tc.valid {
			t.Fatalf("%s: %v %+v", tc.input, err, r.Diagnostics)
		}
		if tc.valid {
			if len(r.A2UIMessages) != 1 {
				t.Fatalf("expected one serialized message: %+v", r)
			}
		} else {
			found := false
			for _, d := range r.Diagnostics {
				found = found || d.Pointer == tc.pointer
			}
			if !found {
				t.Fatalf("%s: %+v, want pointer %q", tc.input, r.Diagnostics, tc.pointer)
			}
		}
	}
	if r, err := p.Lint([]byte(`[{"version":"v1.0","createSurface":{"surfaceId":"s"}}]`), false, true); err != nil || !r.Valid || len(r.A2UIMessages) != 1 {
		t.Fatalf("envelope emission: %v %+v", err, r)
	}
}

func TestCrossPlatformCoverageAicardBundledValidatesEachFilesystem(t *testing.T) {
	source, err := fs.Sub(skills.FS, "multi/dingtalk-aicard")
	if err != nil {
		t.Fatal(err)
	}
	p, err := Bundled(source)
	if err != nil || p == nil {
		t.Fatalf("bundled load: %v", err)
	}
	if _, err := Bundled(fstest.MapFS{}); err == nil {
		t.Fatal("a prior valid filesystem must not mask a missing package")
	}
}

func TestCrossPlatformCoverageAicardFragmentDiagnosticOutsideWrappedComponent(t *testing.T) {
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("urn:test:envelope", map[string]any{"type": "array"}); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile("urn:test:envelope")
	if err != nil {
		t.Fatal(err)
	}
	p := &Protocol{schema: schema}
	r, err := p.Lint([]byte(`{"id":"root","component":"Text","text":"ok"}`), true, false)
	if err != nil || r.Valid || len(r.Diagnostics) == 0 || r.Diagnostics[0].Pointer != "" {
		t.Fatalf("envelope diagnostic must point at the fragment root: %v %+v", err, r)
	}
}

func TestCrossPlatformCoverageAicardRepeatedReferenceUsesVisitedNode(t *testing.T) {
	p := testProtocol(t)
	s := referenceSnapshot{nodes: map[string]referenceNode{}, location: "/0/createSurface/surfaceId"}
	s.update(map[string]any{"components": []any{
		map[string]any{"id": "root", "component": "Column", "children": []any{"child", "child"}},
		map[string]any{"id": "child", "component": "Text", "text": "shared"},
	}}, "/0/createSurface")
	if ds := p.checkReferences(s, "s"); len(ds) != 0 {
		t.Fatal(ds)
	}
}
