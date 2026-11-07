// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package a2ui

import (
	"encoding/json"
	"io/fs"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/skills"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func mustExplain(t *testing.T, p *Protocol, name string) map[string]any {
	t.Helper()
	result, err := p.Explain(name)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func testProtocol(t *testing.T) *Protocol {
	t.Helper()
	source, err := fs.Sub(skills.FS, "multi/dingtalk-aicard")
	if err != nil {
		t.Fatal(err)
	}
	p, err := Load(source)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCrossPlatformCoverageAicardAllExplainExamples(t *testing.T) {
	p := testProtocol(t)
	store, err := BundledExplain()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range store.Names() {
		expected, err := store.Lookup(name)
		if err != nil {
			t.Fatal(err)
		}
		result := mustExplain(t, p, name)
		if result["name"] != name || result["kind"] != expected["kind"] {
			t.Fatal(name, result)
		}
		if result["kind"] != "component" {
			continue
		}
		b, _ := json.Marshal(result["example"])
		r, err := p.Lint(b, true, false)
		if err != nil || !r.Valid {
			t.Fatalf("%s: %v %+v", name, err, r.Diagnostics)
		}
	}
	unknown := mustExplain(t, p, "Tabss")
	if unknown["kind"] != "unknown" || len(unknown["suggestions"].([]string)) == 0 {
		t.Fatal(unknown)
	}
}

func TestCrossPlatformCoverageAicardNestedFunctionsAndStructuralDiagnostics(t *testing.T) {
	p := testProtocol(t)
	var visible any = map[string]any{"path": "/ready"}
	for i := 0; i < 10; i++ {
		visible = map[string]any{"call": "not", "args": map[string]any{"value": visible}}
	}
	component := map[string]any{"id": "root", "component": "Text", "text": "t", "visible": visible}
	b, _ := json.Marshal(component)
	start := time.Now()
	r, err := p.Lint(b, true, false)
	if err != nil || !r.Valid {
		t.Fatal(err, r)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("nested validation took %v", elapsed)
	}
	component["bogus"] = true
	b, _ = json.Marshal(component)
	r, err = p.Lint(b, true, false)
	if err != nil || r.Valid {
		t.Fatal(err, r)
	}
	if len(r.Diagnostics) != 1 || r.Diagnostics[0].Code != "schema.unknown_property" || r.Diagnostics[0].Pointer != "/bogus" {
		t.Fatal(r)
	}
}

func TestCrossPlatformCoverageAicardUnicodeExtensionNames(t *testing.T) {
	p := testProtocol(t)
	for _, tc := range []struct {
		name  string
		valid bool
	}{{"中文", true}, {"éclair", true}, {"Δelta", true}, {"_ok", true}, {"a\n", true}, {"a\u200d", true}, {"\u088f", true}, {"a\u1acf", true}, {"\u0378", false}, {"123bad", false}, {"😀", false}, {"a-b", false}, {"", false}} {
		message := []any{map[string]any{"version": "v1.0", "createSurface": map[string]any{"surfaceId": "s", "metadata": map[string]any{"extensions": map[string]any{tc.name: map[string]any{}}}}}}
		b, _ := json.Marshal(message)
		r, err := p.Lint(b, false, false)
		if err != nil || r.Valid != tc.valid {
			t.Fatalf("%q expected %v: %v %+v", tc.name, tc.valid, err, r)
		}
	}
}

func TestCrossPlatformCoverageAicardButtonGroupWritebackOwner(t *testing.T) {
	p := testProtocol(t)
	writeback := object(mustExplain(t, p, "ButtonGroup")["hostWriteback"])
	if writeback["path"] != "buttons[].metadata.extensions.dt_actionBindingsV1.action.resultPath" {
		t.Fatal(writeback)
	}
	slots := array(writeback["slots"])
	if len(slots) != 1 || slots[0] != "action" {
		t.Fatal(writeback)
	}
}

func TestCrossPlatformCoverageAicardSyntaxAndShape(t *testing.T) {
	p := testProtocol(t)
	for _, s := range []string{"[]", "{}", "null", "[NaN]", "[] []", "[1]"} {
		r, err := p.Lint([]byte(s), false, false)
		if err != nil || r.Valid {
			t.Fatalf("%s %v %+v", s, err, r)
		}
	}
	// Strong runtime constraints are deliberately outside lint.
	for _, s := range []string{
		`{"id":"root","component":"Column","children":["missing"]}`,
		`{"id":"root","component":"Text","text":{"path":"/missing"}}`,
		`{"id":"root","component":"Column","children":[]}`,
	} {
		r, err := p.Lint([]byte(s), true, false)
		if err != nil || !r.Valid {
			t.Fatal(s, err, r)
		}
	}
}

func TestCrossPlatformCoverageAicardUTF8BOMInput(t *testing.T) {
	p := testProtocol(t)
	payload := []byte(`{"id":"root","component":"Text","text":"ok"}`)
	withBOM := append([]byte{0xef, 0xbb, 0xbf}, payload...)
	report, err := p.Lint(withBOM, true, false)
	if err != nil || !report.Valid {
		t.Fatalf("leading UTF-8 BOM should be accepted: %v %+v", err, report.Diagnostics)
	}
	if _, err := decode(append([]byte{' '}, withBOM...)); err == nil {
		t.Fatal("a BOM after whitespace must not be ignored")
	}
}

func TestCrossPlatformCoverageAicardNewCardBoundary(t *testing.T) {
	p := testProtocol(t)
	create := `{"version":"v1.0","createSurface":{"surfaceId":"s","catalogId":"https://dingtalk.com/card/a2ui/catalogs/public/catalog.json"}}`
	update := `{"version":"v1.0","updateComponents":{"surfaceId":"s","components":[{"id":"root","component":"Text","text":"正文"}]}}`
	for _, messages := range [][]string{{update}, {create, create}, {update, create}, {`{"version":"v1.0","createSurface":{"surfaceId":"s","catalogId":"bogus"}}`}} {
		if p.CheckNewCard(messages) == nil {
			t.Fatal(messages)
		}
	}
	if err := p.CheckNewCard([]string{create, `{"version":"v1.0","updateDataModel":{"surfaceId":"s","path":"/","value":{}}}`, update}); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageAicardConstantGuardPreservesCombinationSemantics(t *testing.T) {
	schema := `{"$schema":"https://json-schema.org/draft/2020-12/schema","allOf":[{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]},{"oneOf":[{"properties":{"call":{"const":"a"},"args":{"type":"object","properties":{"value":{"type":"boolean"}},"required":["value"],"unevaluatedProperties":false}},"required":["call","args"]},{"properties":{"call":{"const":"b"},"args":{"type":"number"}},"required":["args"]}]}],"unevaluatedProperties":false}`
	original, _ := decode([]byte(schema))
	copy, _ := decode([]byte(schema))
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("urn:test:original", original); err != nil {
		t.Fatal(err)
	}
	if err := compiler.AddResource("urn:test:guarded", guardTags(copy)); err != nil {
		t.Fatal(err)
	}
	a, err := compiler.Compile("urn:test:original")
	if err != nil {
		t.Fatal(err)
	}
	b, err := compiler.Compile("urn:test:guarded")
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{
		`{"id":"r","call":"a","args":{"value":true}}`,
		`{"id":"r","call":"b","args":2}`,
		`{"id":"r","args":2}`,
		`{"id":"r","call":"unknown","args":2}`,
		`{"id":"r","call":"a","args":{"value":true,"extra":1}}`,
		`{"id":"r","call":"a","args":{"value":true},"extra":1}`,
		`{"call":"a","args":{}}`, `null`, `[]`, `{}`,
	} {
		v, _ := decode([]byte(input))
		if (a.Validate(v) == nil) != (b.Validate(v) == nil) {
			t.Fatalf("validation criteria changed: %s", input)
		}
	}
}
