// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestRunSchemaModes(t *testing.T) {
	directory := t.TempDir()
	raw := filepath.Join(directory, "raw.json")
	writeTestFile(t, raw, `{"kind":"schema","level":"catalog","products":[{"id":"doc","tools":[{"canonical_path":"doc.create","parameters":{"title":{"type":"string","required":true}}}]}]}`)

	var normalized, stderr bytes.Buffer
	if code := run([]string{"--normalize", raw}, &normalized, &stderr); code != 0 {
		t.Fatalf("normalize code=%d stderr=%s", code, stderr.String())
	}
	baseline := filepath.Join(directory, "baseline.json")
	writeTestFile(t, baseline, normalized.String())

	var stdout bytes.Buffer
	stderr.Reset()
	if code := run([]string{"--check", baseline, "--current", raw}, &stdout, &stderr); code != 0 {
		t.Fatalf("check code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "compatibility check: ok") {
		t.Fatalf("unexpected check output %q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"--merge", baseline, "--current", raw}, &stdout, &stderr); code != 0 {
		t.Fatalf("merge code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"doc.create"`) {
		t.Fatalf("unexpected merge output %q", stdout.String())
	}

	empty := filepath.Join(directory, "empty.json")
	writeTestFile(t, empty, `{"kind":"schema","products":[]}`)
	stderr.Reset()
	if code := run([]string{"--check", baseline, "--current", empty}, &stdout, &stderr); code != 1 {
		t.Fatalf("incompatible check code=%d, want 1", code)
	}

	for _, args := range [][]string{
		nil,
		{"--normalize", raw, "--check", baseline},
		{"--check", baseline},
		{"--normalize", filepath.Join(directory, "missing")},
		{"--unknown"},
	} {
		stderr.Reset()
		if code := run(args, &stdout, &stderr); code != 2 {
			t.Errorf("run(%v) code=%d, want 2", args, code)
		}
	}

	stderr.Reset()
	if code := run([]string{"--normalize", raw}, failingWriter{}, &stderr); code != 2 {
		t.Fatalf("write failure code=%d, want 2", code)
	}
}

func TestNormalizeRawFileValidation(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "invalid json", body: `{`, want: "unexpected end"},
		{name: "wrong kind", body: `{"kind":"other","products":[]}`, want: "unexpected kind"},
		{name: "missing products", body: `{"kind":"schema"}`, want: "products array is missing"},
		{name: "missing product id", body: `{"kind":"schema","products":[{"tools":[]}]}`, want: "product without id"},
		{name: "duplicate product", body: `{"kind":"schema","products":[{"id":"doc"},{"id":"doc"}]}`, want: "duplicate product"},
		{name: "missing tool id", body: `{"kind":"schema","products":[{"id":"doc","tools":[{}]}]}`, want: "tool without"},
		{name: "duplicate tool", body: `{"kind":"schema","products":[{"id":"doc","tools":[{"name":"read"},{"cli_name":"read"}]}]}`, want: "duplicate tool"},
		{name: "invalid parameter", body: `{"kind":"schema","products":[{"id":"doc","tools":[{"name":"read","parameters":{"id":1}}]}]}`, want: "parameter id"},
		{name: "invalid required", body: `{"kind":"schema","products":[{"id":"doc","tools":[{"name":"read","parameters":{"id":{"type":"string","required":"yes"}}}]}]}`, want: "required must be a boolean"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "raw.json")
			writeTestFile(t, path, test.body)
			_, err := normalizeRawFile(path)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("normalizeRawFile() error=%v, want %q", err, test.want)
			}
		})
	}
}

func TestNormalizeCurrentSchemaPayload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema.json")
	writeTestFile(t, path, `{
		"kind":"schema",
		"level":"catalog",
		"products":[{
			"id":"doc",
			"tools":[{
				"canonical_path":"doc.create",
				"cli_path":"doc create",
				"parameters":{
					"title":{"type":"string","required":true},
					"format":{"type":["string","null"],"required":false}
				}
			}]
		}]
	}`)

	contract, err := normalizeRawFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tool := contract.Products["doc"].Tools["doc.create"]
	if got := strings.Join(tool.Required, ","); got != "title" {
		t.Fatalf("required parameters = %q, want title", got)
	}
	if got := tool.Parameters["title"]; got != `"string"` {
		t.Fatalf("title type = %q", got)
	}
	if got := tool.Parameters["format"]; got != `["string","null"]` {
		t.Fatalf("format type = %q", got)
	}
}

func TestNormalizeLegacyToolRequiredList(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema.json")
	writeTestFile(t, path, `{"kind":"schema","products":[{"id":"doc","tools":[{"canonical_path":"doc.create","parameters":{"title":{"type":"string"}},"required":["title"]}]}]}`)

	contract, err := normalizeRawFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(contract.Products["doc"].Tools["doc.create"].Required, ","); got != "title" {
		t.Fatalf("legacy required parameters = %q, want title", got)
	}
}

func TestSchemaTypeAndHelpers(t *testing.T) {
	if got := schemaType(map[string]any{"type": []any{"string", "null"}}); got != `["string","null"]` {
		t.Fatalf("schemaType(type)=%q", got)
	}
	if got := schemaType(map[string]any{"oneOf": []any{"a"}}); got != `oneOf:["a"]` {
		t.Fatalf("schemaType(oneOf)=%q", got)
	}
	if got := schemaType(map[string]any{}); got != "unspecified" {
		t.Fatalf("schemaType(empty)=%q", got)
	}
	if got := firstNonEmpty(" ", " value "); got != "value" {
		t.Fatalf("firstNonEmpty()=%q", got)
	}
	if got := uniqueSorted([]string{"b", "a", "b"}); strings.Join(got, ",") != "a,b" {
		t.Fatalf("uniqueSorted()=%v", got)
	}
}

func writeTestFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestSchemaCompatibilityAllowsAdditions(t *testing.T) {
	baseline := schemaContract{Version: 1, Products: map[string]productSchema{
		"doc": {Tools: map[string]toolSchema{
			"doc.create": {Parameters: map[string]string{"title": `"string"`}},
		}},
	}}
	current := cloneContract(baseline)
	current.Products["doc"].Tools["doc.read"] = toolSchema{}
	current.Products["sheet"] = productSchema{Tools: map[string]toolSchema{}}
	if failures := checkCompatibility(baseline, current); len(failures) != 0 {
		t.Fatalf("additions should be compatible: %v", failures)
	}
}

func TestSchemaCompatibilityRejectsRemovedAndNewRequiredInputs(t *testing.T) {
	baseline := schemaContract{Version: 1, Products: map[string]productSchema{
		"doc": {Tools: map[string]toolSchema{
			"doc.create": {Parameters: map[string]string{"title": `"string"`}},
			"doc.read":   {},
		}},
	}}
	current := schemaContract{Version: 1, Products: map[string]productSchema{
		"doc": {Tools: map[string]toolSchema{
			"doc.create": {
				Parameters: map[string]string{"title": `"string"`, "folder": `"string"`},
				Required:   []string{"folder"},
			},
		}},
	}}
	if failures := checkCompatibility(baseline, current); len(failures) != 2 {
		t.Fatalf("got %d failures, want 2: %v", len(failures), failures)
	}
}

func TestMergeContracts(t *testing.T) {
	historical := schemaContract{Version: 1, Products: map[string]productSchema{
		"doc": {Tools: map[string]toolSchema{
			"read": {Parameters: map[string]string{"id": `"string"`}},
		}},
	}}
	current := cloneContract(historical)
	tool := current.Products["doc"].Tools["read"]
	tool.Parameters["format"] = `"string"`
	current.Products["doc"].Tools["read"] = tool
	current.Products["sheet"] = productSchema{Tools: map[string]toolSchema{}}
	merged, failures := mergeContracts(historical, current)
	if len(failures) != 0 || merged.Products["doc"].Tools["read"].Parameters["format"] == "" {
		t.Fatalf("merge=%v failures=%v", merged, failures)
	}

	tool = current.Products["doc"].Tools["read"]
	tool.Parameters["id"] = `"number"`
	tool.Required = []string{"format"}
	current.Products["doc"].Tools["read"] = tool
	if _, failures := mergeContracts(historical, current); len(failures) != 2 {
		t.Fatalf("incompatible merge failures=%v", failures)
	}
}
