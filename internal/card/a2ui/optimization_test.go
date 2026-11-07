// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package a2ui

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCrossPlatformCoverageAicardPreflightBoundaries(t *testing.T) {
	p := testProtocol(t)
	create := `{"version":"v1.0","createSurface":{"surfaceId":"s","catalogId":"https://dingtalk.com/card/a2ui/catalogs/public/catalog.json"}}`
	data := `{"version":"v1.0","updateDataModel":{"surfaceId":"s","path":"/","value":{}}}`
	content := `{"version":"v1.0","updateComponents":{"surfaceId":"s","components":[{"id":"root","component":"Text","text":"测试"}]}}`
	for _, msgs := range [][]string{{create, data, content}, {create, content, data}, {create, data, content, data}} {
		if e := p.CheckNewCard(msgs); e != nil {
			t.Fatal(e)
		}
	}
	if p.CheckNewCard([]string{create, content}) == nil {
		t.Fatal("preflight must reject missing data initialization")
	}
	if p.CheckNewCard([]string{create, data, content, `{"version":"v1.0","updateDataModel":{"surfaceId":"s","path":"/"}}`}) == nil {
		t.Fatal("deleting the root must not count as initialization")
	}
	bad := `{"version":"v1.0","updateComponents":{"surfaceId":"s","components":[{"id":"root","component":"Image","url":"data:image/jpeg;base64,tool error!"}]}}`
	r, e := p.Preflight([]string{bad}, "resources")
	if e != nil || r["valid"] != false {
		t.Fatal(r, e)
	}
	r, e = p.Preflight([]string{content}, "resources")
	if e != nil || r["valid"] != true {
		t.Fatal(r, e)
	}
}

func TestCrossPlatformCoverageAicardInlineDataResourcePolicy(t *testing.T) {
	p := testProtocol(t)
	for _, field := range []string{"url", "darkUrl", "imageUrl", "posterUrl", "coverUrl", "images"} {
		t.Run(field, func(t *testing.T) {
			var value any = "data:image/png;base64,YQ=="
			if field == "images" {
				value = []any{value}
			}
			component := map[string]any{"id": "root", "component": "Image", field: value}
			raw, err := json.Marshal(map[string]any{"version": "v1.0", "updateComponents": map[string]any{"surfaceId": "s", "components": []any{component}}})
			if err != nil {
				t.Fatal(err)
			}
			create := `{"version":"v1.0","createSurface":{"surfaceId":"s","catalogId":"https://dingtalk.com/card/a2ui/catalogs/public/catalog.json","dataModel":{}}}`
			for _, mode := range []string{"resources", "new-card"} {
				messages := []string{create, string(raw)}
				result, err := p.Preflight(messages, mode)
				if err != nil || result["valid"] != false || result["renderingVerified"] != false {
					t.Fatal(result, err)
				}
				diagnostics := result["diagnostics"].([]Diagnostic)
				if len(diagnostics) != 1 || diagnostics[0].Code != "resource.inline_data_unsupported" || diagnostics[0].Severity != "error" {
					t.Fatal(result)
				}
				if err := p.CheckNewCard(messages); err == nil {
					t.Fatal("inline resource data must reject a new card")
				}
			}
		})
	}
}

func TestCrossPlatformCoverageAicardSharedResourcePolicyCasesPreserveSchemaValidity(t *testing.T) {
	p := testProtocol(t)
	raw, err := os.ReadFile("testdata/resource-policy.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name                 string         `json:"name"`
		Component            map[string]any `json:"component"`
		Valid                bool           `json:"valid"`
		PointerSuffix        string         `json:"pointerSuffix"`
		WarningPointerSuffix string         `json:"warningPointerSuffix"`
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	create := `{"version":"v1.0","createSurface":{"surfaceId":"s","catalogId":"https://dingtalk.com/card/a2ui/catalogs/public/catalog.json","dataModel":{}}}`
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			component, _ := json.Marshal(tc.Component)
			lint, err := p.Lint(component, true, false)
			if err != nil || !lint.Valid {
				t.Fatal("resource policy must not change Schema validity:", lint, err)
			}
			update, _ := json.Marshal(map[string]any{"version": "v1.0", "updateComponents": map[string]any{"surfaceId": "s", "components": []any{tc.Component}}})
			for _, mode := range []string{"resources", "new-card"} {
				result, err := p.Preflight([]string{create, string(update)}, mode)
				if err != nil || result["valid"] != tc.Valid {
					t.Fatal(mode, result, err)
				}
				diagnostics := result["diagnostics"].([]Diagnostic)
				if tc.WarningPointerSuffix != "" {
					if len(diagnostics) != 1 || diagnostics[0].Code != "resource.markdown_data_unverified" || diagnostics[0].Severity != "warning" || !strings.HasSuffix(diagnostics[0].Pointer, tc.WarningPointerSuffix) {
						t.Fatal(diagnostics)
					}
				} else if tc.Valid {
					if len(diagnostics) != 0 {
						t.Fatal(diagnostics)
					}
				} else if len(diagnostics) != 1 || diagnostics[0].Code != "resource.inline_data_unsupported" || diagnostics[0].Severity != "error" || !strings.HasSuffix(diagnostics[0].Pointer, tc.PointerSuffix) {
					t.Fatal(diagnostics)
				}
			}
		})
	}
}

func TestAicardMarkdownReviewDoesNotBlockAdversarialContent(t *testing.T) {
	// Exercise the full preflight on large syntax-like text without parsing it.
	p := testProtocol(t)
	shapes := []struct{ name, content string }{
		{"image openers", strings.Repeat("![", 30000)},
		{"unclosed tags", strings.Repeat("<img a", 10000)},
		{"reference openers", strings.Repeat("![a][", 12000)},
		{"backtick runs", strings.Repeat("` ``", 15000)},
		{"reference definitions", strings.Repeat("[a]: ", 12000)},
		{"shared bracket before a long destination", strings.Repeat("![", 15000) + "](" + strings.Repeat("a", 30000)},
		{"shared bracket with an inline reference", "[x]: data:a\n\n" + strings.Repeat("![", 30000) + "]"},
		{"tags sharing one close", strings.Repeat("<img ", 12000) + ">"},
		{"attributes sharing one close", strings.Repeat("<img src=a ", 6000) + ">"},
		{"overlapping destinations", strings.Repeat("![a](x", 10000)},
		{"overlapping entity destinations", strings.Repeat("![a](&", 10000)},
		{"backticks between blank lines", strings.Repeat("`\n\n", 20000)},
		{"nested labels with an unused reference", strings.Repeat("![", 30000) + strings.Repeat("]", 30000) + "\n\n[x]: data:image/png;base64,AA=="},
	}
	for _, shape := range shapes {
		started := time.Now()
		message, err := json.Marshal(map[string]any{"version": "v1.0", "updateComponents": map[string]any{"surfaceId": "s", "components": []any{map[string]any{"id": "root", "component": "Markdown", "content": shape.content}}}})
		if err != nil {
			t.Fatal(err)
		}
		result, err := p.Preflight([]string{string(message)}, "resources")
		if err != nil || result["valid"] != true {
			t.Fatalf("%s: %v %+v", shape.name, err, result)
		}
		if elapsed := time.Since(started); elapsed > time.Second {
			t.Fatalf("%s: scan took %v", shape.name, elapsed)
		}
	}
}

func TestCrossPlatformCoverageAicardBundleRoundtrip(t *testing.T) {
	p := testProtocol(t)
	names := []string{"Text", "Row", "Column", "ButtonGroup", "Tabs", "CheckBox", "promptText"}
	ordinary, err := p.ExplainMany(names, false)
	if err != nil {
		t.Fatal(err)
	}
	packed, err := p.ExplainMany(names, true)
	if err != nil {
		t.Fatal(err)
	}
	defs := packed["definitions"].(map[string]any)
	var unpack func(any) any
	unpack = func(v any) any {
		switch n := v.(type) {
		case map[string]any:
			if ref, ok := n["$contractRef"]; ok && len(n) == 1 {
				return unpack(defs[ref.(string)])
			}
			out := map[string]any{}
			for k, c := range n {
				out[k] = unpack(c)
			}
			return out
		case []any:
			out := []any{}
			for _, c := range n {
				out = append(out, unpack(c))
			}
			return out
		default:
			return v
		}
	}
	if !reflect.DeepEqual(ordinary["contracts"], unpack(packed["contracts"])) {
		t.Fatal("query deduplication lost structure")
	}
	a, _ := json.Marshal(ordinary)
	b, _ := json.Marshal(packed)
	if len(b) >= len(a) {
		t.Fatal("batch query did not reduce duplication")
	}
}
