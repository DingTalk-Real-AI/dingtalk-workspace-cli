// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const completeSchemaJSON = `{
  "kind":"schema",
  "level":"catalog",
  "products":[{
    "id":"doc",
    "tools":[{
      "canonical_path":"doc.create",
      "primary_cli_path":"doc create",
      "interface_mode":"local",
      "interface_ref":{"transport":"local"},
      "availability":"available",
      "parameters":{
        "title":{
          "type":"string",
          "property":"title",
          "required":true,
          "cli_required":true,
          "interface_type":"string",
          "default":null,
          "field_provenance":{}
        },
        "format":{
          "type":["string","null"],
          "property":"format",
          "required":false,
          "interface_type":"string",
          "default":"markdown",
          "enum":["markdown","text"],
          "field_provenance":{}
        }
      },
      "constraints":{"require_one_of":[["title","format"]]},
      "positionals":[{
        "name":"content",
        "index":0,
        "type":"string",
        "required":false,
        "description":"original prose"
      }],
      "effect":"write",
      "risk":"medium",
      "confirmation":"not_required",
      "idempotency":"unknown",
      "field_provenance":{}
    }]
  }]
}`

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestRunSchemaModes(t *testing.T) {
	directory := t.TempDir()
	raw := filepath.Join(directory, "raw.json")
	writeTestFile(t, raw, completeSchemaJSON)

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
	if code := run([]string{"--check", baseline, "--current", empty}, &stdout, &stderr); code != 2 {
		t.Fatalf("empty current contract code=%d, want 2", code)
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
		{name: "empty products", body: `{"kind":"schema","products":[]}`, want: "contains no products"},
		{name: "empty tools", body: `{"kind":"schema","products":[{"id":"doc","tools":[]}]}`, want: "contains no tools"},
		{name: "missing product id", body: `{"kind":"schema","products":[{"tools":[]}]}`, want: "product without id"},
		{name: "duplicate product", body: `{"kind":"schema","products":[{"id":"doc"},{"id":"doc"}]}`, want: "duplicate product"},
		{name: "compact tool rejected", body: `{"kind":"schema","products":[{"id":"doc","tools":[{"canonical_path":"doc.create","parameters":{},"effect":"write","risk":"medium","confirmation":"not_required","idempotency":"unknown","interface_mode":"local","availability":"available"}]}]}`, want: "not a complete schema --all leaf"},
		{name: "invalid required", body: strings.Replace(completeSchemaJSON, `"required":true`, `"required":"yes"`, 1), want: "cannot unmarshal string"},
		{name: "incomplete parameter", body: strings.Replace(completeSchemaJSON, `"field_provenance":{}`, `"incomplete":true`, 1), want: "not a complete schema --all parameter"},
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

func TestNormalizeCompleteSchemaPayload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema.json")
	writeTestFile(t, path, completeSchemaJSON)

	contract, err := normalizeRawFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tool := contract.Products["doc"].Tools["doc.create"]
	if tool.PrimaryCLIPath != "doc create" || tool.Constraints == "" || tool.Effect != "write" {
		t.Fatalf("normalized tool contract is incomplete: %#v", tool)
	}
	if len(tool.Positionals) != 1 || tool.Positionals[0].Name != "content" {
		t.Fatalf("normalized positionals = %#v", tool.Positionals)
	}
	if got := tool.Parameters["title"]; got.Type != `"string"` || !got.Required || got.Property != "title" || got.InterfaceType != "string" {
		t.Fatalf("title parameter = %#v", got)
	}
	if got := tool.Parameters["format"]; got.Type != `["string","null"]` || got.Default != `"markdown"` {
		t.Fatalf("format parameter = %#v", got)
	}
}

func TestSchemaCompatibilityIgnoresPositionalDescription(t *testing.T) {
	directory := t.TempDir()
	baselinePath := filepath.Join(directory, "baseline.json")
	currentPath := filepath.Join(directory, "current.json")
	writeTestFile(t, baselinePath, completeSchemaJSON)
	writeTestFile(t, currentPath, strings.Replace(completeSchemaJSON, "original prose", "edited prose only", 1))

	baseline, err := normalizeRawFile(baselinePath)
	if err != nil {
		t.Fatal(err)
	}
	current, err := normalizeRawFile(currentPath)
	if err != nil {
		t.Fatal(err)
	}
	if failures := checkCompatibility(baseline, current); len(failures) != 0 {
		t.Fatalf("positional description edit should be compatible: %v", failures)
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
	if !enumNarrowed([]string{"a", "b"}, []string{"a"}) || enumNarrowed([]string{"a"}, []string{"a", "b"}) {
		t.Fatal("enum narrowing classification is incorrect")
	}
}

func TestSchemaCompatibilityAllowsAdditionsAndLooserInputs(t *testing.T) {
	baseline := baselineContract()
	mutateTool(&baseline, func(tool *toolSchema) {
		tool.DryRun = ""
	})
	current := cloneContract(baseline)
	mutateParameter(&current, func(parameter *parameterSchema) {
		parameter.Required = false
		parameter.CLIRequired = false
		parameter.Enum = append(parameter.Enum, "html")
	})
	mutateTool(&current, func(tool *toolSchema) {
		tool.Parameters["folder"] = parameterSchema{Type: `"string"`}
		tool.DryRun = `{"mode":"native"}`
	})
	current.Products["doc"].Tools["doc.read"] = toolSchema{Parameters: map[string]parameterSchema{}}
	current.Products["sheet"] = productSchema{Tools: map[string]toolSchema{}}
	if failures := checkCompatibility(baseline, current); len(failures) != 0 {
		t.Fatalf("compatible additions should pass: %v", failures)
	}
}

func TestSchemaCompatibilityAllowsLooserAndAppendedOptionalPositionals(t *testing.T) {
	baseline := baselineContract()
	mutateTool(&baseline, func(tool *toolSchema) {
		tool.Positionals[0].Required = true
	})
	current := cloneContract(baseline)
	mutateTool(&current, func(tool *toolSchema) {
		tool.Positionals[0].Required = false
		tool.Positionals = append(tool.Positionals, positionalSchema{
			Name:  "template",
			Index: 1,
			Type:  "string",
		})
	})

	if failures := checkCompatibility(baseline, current); len(failures) != 0 {
		t.Fatalf("looser and appended optional positionals should pass: %v", failures)
	}
}

func TestCompatiblePositionals(t *testing.T) {
	baseline := []positionalSchema{
		{Name: "content", Index: 0, Type: "string", Required: true},
		{Name: "format", Index: 1, Type: "string"},
	}
	tests := []struct {
		name       string
		old        []positionalSchema
		current    []positionalSchema
		compatible bool
	}{
		{name: "unchanged", old: baseline, current: clonePositionals(baseline), compatible: true},
		{name: "required becomes optional", old: baseline, current: []positionalSchema{
			{Name: "content", Index: 0, Type: "string"},
			{Name: "format", Index: 1, Type: "string"},
		}, compatible: true},
		{name: "append optional", old: baseline, current: append(clonePositionals(baseline), positionalSchema{
			Name: "template", Index: 2, Type: "string",
		}), compatible: true},
		{name: "last positional becomes variadic", old: baseline, current: []positionalSchema{
			{Name: "content", Index: 0, Type: "string", Required: true},
			{Name: "format", Index: 1, Type: "string", Variadic: true},
		}, compatible: true},
		{name: "removed", old: baseline, current: clonePositionals(baseline[:1])},
		{name: "renamed", old: baseline, current: []positionalSchema{
			{Name: "body", Index: 0, Type: "string", Required: true},
			{Name: "format", Index: 1, Type: "string"},
		}},
		{name: "reindexed", old: baseline, current: []positionalSchema{
			{Name: "content", Index: 1, Type: "string", Required: true},
			{Name: "format", Index: 2, Type: "string"},
		}},
		{name: "retyped", old: baseline, current: []positionalSchema{
			{Name: "content", Index: 0, Type: "number", Required: true},
			{Name: "format", Index: 1, Type: "string"},
		}},
		{name: "optional becomes required", old: baseline, current: []positionalSchema{
			{Name: "content", Index: 0, Type: "string", Required: true},
			{Name: "format", Index: 1, Type: "string", Required: true},
		}},
		{name: "append required", old: baseline, current: append(clonePositionals(baseline), positionalSchema{
			Name: "template", Index: 2, Type: "string", Required: true,
		})},
		{name: "variadic becomes fixed", old: []positionalSchema{
			{Name: "content", Index: 0, Type: "string", Variadic: true},
		}, current: []positionalSchema{
			{Name: "content", Index: 0, Type: "string"},
		}},
		{name: "append after variadic", old: []positionalSchema{
			{Name: "content", Index: 0, Type: "string", Variadic: true},
		}, current: []positionalSchema{
			{Name: "content", Index: 0, Type: "string", Variadic: true},
			{Name: "format", Index: 1, Type: "string"},
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := compatiblePositionals(test.old, test.current); got != test.compatible {
				t.Fatalf("compatiblePositionals() = %t, want %t", got, test.compatible)
			}
		})
	}
}

func TestSchemaCompatibilityRejectsContractDrift(t *testing.T) {
	tests := []struct {
		name   string
		want   string
		mutate func(*schemaContract)
	}{
		{name: "removed product", want: "historical schema product", mutate: func(contract *schemaContract) { delete(contract.Products, "doc") }},
		{name: "removed tool", want: "historical schema tool", mutate: func(contract *schemaContract) { delete(contract.Products["doc"].Tools, "doc.create") }},
		{name: "removed parameter", want: "lost parameter", mutate: func(contract *schemaContract) {
			mutateTool(contract, func(tool *toolSchema) { delete(tool.Parameters, "title") })
		}},
		{name: "changed type", want: "changed type", mutate: func(contract *schemaContract) {
			mutateParameter(contract, func(parameter *parameterSchema) { parameter.Type = `"number"` })
		}},
		{name: "new required", want: "newly required", mutate: func(contract *schemaContract) {
			mutateParameter(contract, func(parameter *parameterSchema) { parameter.Required = true })
		}},
		{name: "new cli required", want: "newly cli_required", mutate: func(contract *schemaContract) {
			mutateParameter(contract, func(parameter *parameterSchema) { parameter.CLIRequired = true })
		}},
		{name: "changed required when", want: "changed required_when", mutate: func(contract *schemaContract) {
			mutateParameter(contract, func(parameter *parameterSchema) { parameter.RequiredWhen = "scope=team" })
		}},
		{name: "changed property", want: "changed property", mutate: func(contract *schemaContract) {
			mutateParameter(contract, func(parameter *parameterSchema) { parameter.Property = "subject" })
		}},
		{name: "cleared property without a reviewed exclusion", want: "changed property", mutate: func(contract *schemaContract) {
			mutateParameter(contract, func(parameter *parameterSchema) {
				parameter.Property = ""
				parameter.PropertySource = "flag_name_inference"
			})
		}},
		{name: "redirected property despite a reviewed exclusion", want: "changed property", mutate: func(contract *schemaContract) {
			mutateParameter(contract, func(parameter *parameterSchema) {
				parameter.Property = "subject"
				parameter.PropertySource = propertySourceReviewedMappingExclusion
			})
		}},
		{name: "changed interface type", want: "changed interface_type", mutate: func(contract *schemaContract) {
			mutateParameter(contract, func(parameter *parameterSchema) { parameter.InterfaceType = "integer" })
		}},
		{name: "changed default", want: "changed default", mutate: func(contract *schemaContract) {
			mutateParameter(contract, func(parameter *parameterSchema) { parameter.Default = `"html"` })
		}},
		{name: "changed interface default", want: "changed interface_default", mutate: func(contract *schemaContract) {
			mutateParameter(contract, func(parameter *parameterSchema) { parameter.InterfaceDefault = `"html"` })
		}},
		{name: "changed format", want: "changed format", mutate: func(contract *schemaContract) {
			mutateParameter(contract, func(parameter *parameterSchema) { parameter.Format = "uri" })
		}},
		{name: "narrowed enum", want: "narrowed enum", mutate: func(contract *schemaContract) {
			mutateParameter(contract, func(parameter *parameterSchema) { parameter.Enum = []string{"markdown"} })
		}},
		{name: "changed primary cli path", want: "changed primary_cli_path", mutate: func(contract *schemaContract) {
			mutateTool(contract, func(tool *toolSchema) { tool.PrimaryCLIPath = "doc make" })
		}},
		{name: "changed interface mode", want: "changed interface_mode", mutate: func(contract *schemaContract) {
			mutateTool(contract, func(tool *toolSchema) { tool.InterfaceMode = "mcp" })
		}},
		{name: "changed constraints", want: "changed constraints", mutate: func(contract *schemaContract) {
			mutateTool(contract, func(tool *toolSchema) { tool.Constraints = `{}` })
		}},
		{name: "changed positionals", want: "changed positionals", mutate: func(contract *schemaContract) {
			mutateTool(contract, func(tool *toolSchema) { tool.Positionals[0].Name = "id" })
		}},
		{name: "changed interface mapping", want: "changed interface_ref", mutate: func(contract *schemaContract) {
			mutateTool(contract, func(tool *toolSchema) { tool.InterfaceRef = `{"transport":"mcp"}` })
		}},
		{name: "changed availability", want: "changed availability", mutate: func(contract *schemaContract) {
			mutateTool(contract, func(tool *toolSchema) { tool.Availability = "unavailable" })
		}},
		{name: "changed confirmation", want: "changed confirmation", mutate: func(contract *schemaContract) {
			mutateTool(contract, func(tool *toolSchema) { tool.Confirmation = "user_required" })
		}},
		{name: "changed risk", want: "changed risk", mutate: func(contract *schemaContract) { mutateTool(contract, func(tool *toolSchema) { tool.Risk = "high" }) }},
		{name: "changed effect", want: "changed effect", mutate: func(contract *schemaContract) {
			mutateTool(contract, func(tool *toolSchema) { tool.Effect = "destructive" })
		}},
		{name: "changed idempotency", want: "changed idempotency", mutate: func(contract *schemaContract) {
			mutateTool(contract, func(tool *toolSchema) { tool.Idempotency = "idempotent" })
		}},
		{name: "removed dry run", want: "changed or removed dry_run", mutate: func(contract *schemaContract) { mutateTool(contract, func(tool *toolSchema) { tool.DryRun = "" }) }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			current := baselineContract()
			test.mutate(&current)
			failures := strings.Join(checkCompatibility(baselineContract(), current), "\n")
			if !strings.Contains(failures, test.want) {
				t.Fatalf("failures=%q, want %q", failures, test.want)
			}
		})
	}
}

func TestMergeContracts(t *testing.T) {
	historical := baselineContract()
	current := cloneContract(historical)
	mutateTool(&current, func(tool *toolSchema) {
		tool.Parameters["folder"] = parameterSchema{Type: `"string"`}
	})
	merged, failures := mergeContracts(historical, current)
	if len(failures) != 0 || merged.Products["doc"].Tools["doc.create"].Parameters["folder"].Type == "" {
		t.Fatalf("merge=%v failures=%v", merged, failures)
	}

	mutateParameter(&current, func(parameter *parameterSchema) {
		parameter.Type = `"number"`
	})
	if _, failures := mergeContracts(historical, current); len(failures) == 0 {
		t.Fatal("incompatible merge unexpectedly passed")
	}
}

func baselineContract() schemaContract {
	return schemaContract{Version: schemaContractVersion, Products: map[string]productSchema{
		"doc": {Tools: map[string]toolSchema{
			"doc.create": {
				PrimaryCLIPath: "doc create",
				InterfaceMode:  "local",
				InterfaceRef:   `{"transport":"local"}`,
				Availability:   "available",
				Parameters: map[string]parameterSchema{
					"title": {
						Type:          `"string"`,
						Property:      "title",
						InterfaceType: "string",
					},
					"format": {
						Type:          `"string"`,
						Property:      "format",
						InterfaceType: "string",
						Default:       `"markdown"`,
						Enum:          []string{"markdown", "text"},
					},
				},
				Constraints: `{"require_one_of":[["title","format"]]}`,
				Positionals: []positionalSchema{{
					Name:  "content",
					Index: 0,
					Type:  "string",
				}},
				DryRun:       `{"mode":"native"}`,
				Effect:       "write",
				Risk:         "medium",
				Confirmation: "not_required",
				Idempotency:  "unknown",
			},
		}},
	}}
}

func mutateTool(contract *schemaContract, mutate func(*toolSchema)) {
	product := contract.Products["doc"]
	tool := product.Tools["doc.create"]
	mutate(&tool)
	product.Tools["doc.create"] = tool
	contract.Products["doc"] = product
}

func mutateParameter(contract *schemaContract, mutate func(*parameterSchema)) {
	mutateTool(contract, func(tool *toolSchema) {
		parameter := tool.Parameters["format"]
		mutate(&parameter)
		tool.Parameters["format"] = parameter
	})
}

func clonePositionals(source []positionalSchema) []positionalSchema {
	return append([]positionalSchema(nil), source...)
}

func writeTestFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestCrossPlatformCoverageSchemaCompatMCPRetirementAndConstraintExpansion covers
// the MCP pin retirement allowance (clearing interface_type) and declare≡execute
// constraint member expansion used by the platform coverage gate.
func TestCrossPlatformCoverageSchemaCompatMCPRetirementAndConstraintExpansion(t *testing.T) {
	baseline := baselineContract()
	current := cloneContract(baseline)
	mutateParameter(&current, func(parameter *parameterSchema) {
		parameter.InterfaceType = ""
	})
	mutateTool(&current, func(tool *toolSchema) {
		tool.Constraints = `{"require_one_of":[["title","format","legacy-format"]],"mutually_exclusive":[["title","format","legacy-title"]]}`
	})
	// Baseline has require_one_of only; a new mutually-exclusive group that
	// restricts two historical public parameters must remain incompatible.
	if failures := checkCompatibility(baseline, current); len(failures) == 0 {
		t.Fatal("adding a new constraint group must fail compatibility")
	}

	// Match baseline shape: expand require_one_of members only, clear interface_type.
	current = cloneContract(baseline)
	mutateParameter(&current, func(parameter *parameterSchema) {
		parameter.InterfaceType = ""
	})
	mutateTool(&current, func(tool *toolSchema) {
		tool.Constraints = `{"require_one_of":[["title","format","legacy-format"]]}`
	})
	if failures := checkCompatibility(baseline, current); len(failures) != 0 {
		t.Fatalf("clearing interface_type and expanding constraint members should pass: %v", failures)
	}

	// Hidden-sibling expansion from empty historical constraints remains allowed.
	emptyConstraints := cloneContract(baseline)
	mutateTool(&emptyConstraints, func(tool *toolSchema) {
		tool.Constraints = ""
		title := tool.Parameters["title"]
		title.Required = true
		tool.Parameters["title"] = title
		delete(tool.Parameters, "format")
	})
	expanded := cloneContract(emptyConstraints)
	mutateTool(&expanded, func(tool *toolSchema) {
		tool.Constraints = `{"require_one_of":[["title","hidden-alias"]],"mutually_exclusive":[["title","hidden-alias"]]}`
		title := tool.Parameters["title"]
		title.Required = false
		tool.Parameters["title"] = title
	})
	if failures := checkCompatibility(emptyConstraints, expanded); len(failures) != 0 {
		t.Fatalf("hidden-sibling constraint expansion should pass: %v", failures)
	}

	if groups, ok := parseConstraintGroups(""); !ok || len(groups["require_one_of"]) != 0 {
		t.Fatalf("empty constraints parse = %#v ok=%v", groups, ok)
	}
	if !stringSetContainsAll(map[string]bool{"a": true, "b": true}, map[string]bool{"a": true}) ||
		stringSetContainsAll(map[string]bool{"a": true}, map[string]bool{"a": true, "b": true}) {
		t.Fatal("stringSetContainsAll classification is incorrect")
	}

	// compatibleHiddenSiblingConstraintExpansion false branches.
	if compatibleHiddenSiblingConstraintExpansion(
		toolSchema{Constraints: "", Parameters: map[string]parameterSchema{"a": {Required: true}}},
		toolSchema{Constraints: "{", Parameters: map[string]parameterSchema{"a": {}}},
	) {
		t.Fatal("invalid projected constraints must not expand")
	}
	oldRequired := toolSchema{
		Constraints: "",
		Parameters:  map[string]parameterSchema{"a": {Required: true}},
	}
	if compatibleHiddenSiblingConstraintExpansion(oldRequired, toolSchema{
		Constraints: `{"require_together":[["a","b"]],"require_one_of":[["a","hidden"]]}`,
		Parameters:  map[string]parameterSchema{"a": {}},
	}) {
		t.Fatal("require_together projection must not expand")
	}
	if compatibleHiddenSiblingConstraintExpansion(oldRequired, toolSchema{
		Constraints: `{"require_one_of":[["a"]]}`,
		Parameters:  map[string]parameterSchema{"a": {}},
	}) {
		t.Fatal("single-member group must not expand")
	}
	if compatibleHiddenSiblingConstraintExpansion(oldRequired, toolSchema{
		Constraints: `{"require_one_of":[["a","","hidden"]]}`,
		Parameters:  map[string]parameterSchema{"a": {}},
	}) {
		t.Fatal("empty constraint member must not expand")
	}
	if compatibleHiddenSiblingConstraintExpansion(oldRequired, toolSchema{
		Constraints: `{"require_one_of":[["hidden","ghost"]]}`,
		Parameters:  map[string]parameterSchema{"a": {}},
	}) {
		t.Fatal("unpublished-only group must not expand")
	}
	if compatibleHiddenSiblingConstraintExpansion(oldRequired, toolSchema{
		Constraints: `{"require_one_of":[["a","b"]]}`,
		Parameters:  map[string]parameterSchema{"a": {}, "b": {}},
	}) {
		t.Fatal("published-only group must not expand")
	}
	if compatibleHiddenSiblingConstraintExpansion(oldRequired, toolSchema{
		Constraints: `{"require_one_of":[["a","hidden"]]}`,
		Parameters:  map[string]parameterSchema{"a": {Required: true}},
	}) {
		t.Fatal("still-required sole published member must not expand")
	}

	// Changing interface_type to a different non-empty value remains incompatible.
	typed := cloneContract(baseline)
	mutateParameter(&typed, func(parameter *parameterSchema) { parameter.InterfaceType = "integer" })
	if failures := checkCompatibility(baseline, typed); !strings.Contains(strings.Join(failures, "\n"), "changed interface_type") {
		t.Fatalf("non-empty interface_type change should fail: %v", failures)
	}
}

func TestCrossPlatformCoverageSchemaCompatAdditiveConstraintEvolution(t *testing.T) {
	oldTool := toolSchema{
		Parameters: map[string]parameterSchema{
			"target": {},
			"limit":  {},
		},
		Constraints: `{"require_one_of":[["target","legacy-target"]]}`,
	}
	compatible := oldTool
	compatible.Constraints = `{"require_one_of":[["target","legacy-target","new-target"]],"mutually_exclusive":[["target","new-target"],["new-a","new-b"]],"require_together":[["new-a","new-b"]]}`
	if !compatibleAdditiveConstraintEvolution(oldTool, compatible) {
		t.Fatal("member expansion and additive alias-only groups must remain compatible")
	}

	twoHistorical := compatible
	twoHistorical.Constraints = `{"require_one_of":[["target","legacy-target","new-target"]],"mutually_exclusive":[["target","limit","new-target"]]}`
	if compatibleAdditiveConstraintEvolution(oldTool, twoHistorical) {
		t.Fatal("new mutex group restricting two historical parameters must fail")
	}
	newRequirement := compatible
	newRequirement.Constraints = `{"require_one_of":[["target","legacy-target","new-target"],["new-a","new-b"]]}`
	if compatibleAdditiveConstraintEvolution(oldTool, newRequirement) {
		t.Fatal("new require_one_of group must fail")
	}
	requireHistoricalTogether := compatible
	requireHistoricalTogether.Constraints = `{"require_one_of":[["target","legacy-target","new-target"]],"require_together":[["limit","new-limit"]]}`
	if compatibleAdditiveConstraintEvolution(oldTool, requireHistoricalTogether) {
		t.Fatal("new require_together group containing a historical parameter must fail")
	}
	removedOldGroup := compatible
	removedOldGroup.Constraints = `{"mutually_exclusive":[["new-a","new-b"]]}`
	if compatibleAdditiveConstraintEvolution(oldTool, removedOldGroup) {
		t.Fatal("removing a historical group must fail")
	}
	invalid := compatible
	invalid.Constraints = "{"
	if compatibleAdditiveConstraintEvolution(oldTool, invalid) {
		t.Fatal("invalid constraints must fail closed")
	}
	emptyHistoricalGroup := oldTool
	emptyHistoricalGroup.Constraints = `{"require_one_of":[[]]}`
	if compatibleAdditiveConstraintEvolution(emptyHistoricalGroup, compatible) {
		t.Fatal("empty historical constraint group must fail closed")
	}
	historicalMutex := oldTool
	historicalMutex.Constraints = `{"mutually_exclusive":[["target","legacy-target"]]}`
	historicalMutexExpanded := oldTool
	historicalMutexExpanded.Constraints = `{"mutually_exclusive":[["target","legacy-target","limit"]]}`
	if compatibleAdditiveConstraintEvolution(historicalMutex, historicalMutexExpanded) {
		t.Fatal("adding a historical parameter to an existing mutex group must fail")
	}
	historicalTogether := oldTool
	historicalTogether.Constraints = `{"require_together":[["target","legacy-target"]]}`
	historicalTogetherExpanded := oldTool
	historicalTogetherExpanded.Constraints = `{"require_together":[["target","legacy-target","limit"]]}`
	if compatibleAdditiveConstraintEvolution(historicalTogether, historicalTogetherExpanded) {
		t.Fatal("adding a historical parameter to an existing require-together group must fail")
	}
	historicalOneOf := oldTool
	historicalOneOf.Constraints = `{"require_one_of":[["target","legacy-target","limit"]]}`
	if !compatibleAdditiveConstraintEvolution(oldTool, historicalOneOf) {
		t.Fatal("adding a historical parameter to require-one-of only loosens the contract")
	}
	gateBaseline := baselineContract()
	mutateTool(&gateBaseline, func(tool *toolSchema) {
		tool.Parameters["folder"] = parameterSchema{Type: `"string"`}
		tool.Constraints = `{"mutually_exclusive":[["title","format"]]}`
	})
	gateCurrent := cloneContract(gateBaseline)
	mutateTool(&gateCurrent, func(tool *toolSchema) {
		tool.Constraints = `{"mutually_exclusive":[["title","format","folder"]]}`
	})
	if failures := checkCompatibility(gateBaseline, gateCurrent); len(failures) == 0 {
		t.Fatal("compatibility gate must reject an existing mutex group gaining a historical parameter")
	}
	multipleHistoricalGroups := oldTool
	multipleHistoricalGroups.Constraints = `{"require_one_of":[["target"],["limit"]]}`
	multipleExpandedGroups := oldTool
	multipleExpandedGroups.Constraints = `{"require_one_of":[["target","limit"],["limit","new-limit"]]}`
	if !compatibleAdditiveConstraintEvolution(multipleHistoricalGroups, multipleExpandedGroups) {
		t.Fatal("each historical group must match a distinct expanded group")
	}
}

// Clearing a property through the reviewed mapping exclusion table is the one
// accepted shape, mirroring the interface_type retirement allowance. A leaf
// whose backing RPC moved to a nested payload has no honest flat property to
// publish; the alternatives are naming a field the request no longer contains,
// or letting flag_name_inference publish a name that appears in no request.
func TestCrossPlatformCoverageSchemaCompatPropertyClearingExclusion(t *testing.T) {
	baseline := baselineContract()

	// Accepted: non-empty -> empty, resolved through a reviewed exclusion.
	current := cloneContract(baseline)
	mutateParameter(&current, func(parameter *parameterSchema) {
		parameter.Property = ""
		parameter.PropertySource = propertySourceReviewedMappingExclusion
	})
	if failures := checkCompatibility(baseline, current); len(failures) != 0 {
		t.Fatalf("clearing property through a reviewed exclusion should pass: %v", failures)
	}

	// Every neighbouring shape must stay incompatible, so the carve-out cannot
	// be widened by accident.
	for _, tc := range []struct {
		name     string
		property string
		source   string
	}{
		{"cleared by inference", "", "flag_name_inference"},
		{"cleared by a native annotation", "", "native_annotation"},
		{"cleared with no recorded source", "", ""},
		{"redirected to another non-empty value", "cellStyles", propertySourceReviewedMappingExclusion},
	} {
		t.Run(tc.name, func(t *testing.T) {
			drifted := cloneContract(baseline)
			mutateParameter(&drifted, func(parameter *parameterSchema) {
				parameter.Property = tc.property
				parameter.PropertySource = tc.source
			})
			failures := checkCompatibility(baseline, drifted)
			if len(failures) == 0 {
				t.Fatal("must remain incompatible")
			}
			var sawProperty bool
			for _, failure := range failures {
				if strings.Contains(failure, "changed property") {
					sawProperty = true
				}
			}
			if !sawProperty {
				t.Fatalf("expected a changed property failure, got %v", failures)
			}
		})
	}

	// A parameter that never published a property must not gain one silently
	// just because it carries an exclusion source.
	populated := cloneContract(baseline)
	mutateParameter(&populated, func(parameter *parameterSchema) { parameter.Property = "" })
	gained := cloneContract(populated)
	mutateParameter(&gained, func(parameter *parameterSchema) {
		parameter.Property = "backgroundColors"
		parameter.PropertySource = propertySourceReviewedMappingExclusion
	})
	if failures := checkCompatibility(populated, gained); len(failures) == 0 {
		t.Fatal("populating a previously empty property must stay incompatible")
	}
}

// Repointing a leaf at a different backing RPC is accepted only when the
// CLI-facing contract is provably unchanged. interface_ref is audit metadata;
// nothing reads it at runtime, so a stale value misinforms a reader rather than
// misrouting a call. The gate is that no other compatibility check for the tool
// failed — that is the operative meaning of "the contract is unchanged".
// reviewedInterfaceRefRedirect 的键值必须是 parseTool 经 canonicalRawJSON 实际产出
// 的紧凑 JSON。这条守卫是必要的：该表先前误登记为裸 RPC 名（"update_range"），后又
// 误登记为带空格的美化 JSON，两次都让豁免完全失效——而上面的用例是用测试自己注册
// 的值断言的，所以两次都通过了，只有真实门禁才报错。这里用 canonicalRawJSON 复算
// 期望值，把格式锚定到生产代码而不是作者的书写习惯。
func TestCrossPlatformCoverageReviewedRedirectKeysAreCanonicalJSON(t *testing.T) {
	canon := func(raw string) string {
		got, err := canonicalRawJSON(json.RawMessage(raw))
		if err != nil {
			t.Fatalf("canonicalRawJSON(%s): %v", raw, err)
		}
		return got
	}
	if len(reviewedInterfaceRefRedirect) == 0 {
		t.Fatal("redirect allowlist 为空：若确已清空，请同时删除这条守卫")
	}
	for toolPath, pairs := range reviewedInterfaceRefRedirect {
		if len(pairs) == 0 {
			t.Errorf("%s: 空的 redirect 表项没有意义", toolPath)
		}
		for oldRef, newRef := range pairs {
			if got := canon(oldRef); got != oldRef {
				t.Errorf("%s: old ref 不是规范形态\n  登记: %s\n  规范: %s", toolPath, oldRef, got)
			}
			if got := canon(newRef); got != newRef {
				t.Errorf("%s: new ref 不是规范形态\n  登记: %s\n  规范: %s", toolPath, newRef, got)
			}
			if oldRef == newRef {
				t.Errorf("%s: old 与 new 相同，不构成 redirect", toolPath)
			}
		}
	}
}

func TestCrossPlatformCoverageSchemaCompatInterfaceRefRedirect(t *testing.T) {
	// The redirect carve-out only accepts an explicitly reviewed tool + old→new
	// ref pair. Register the fixture's path for the duration of this test so the
	// accepted shapes below exercise the allowlist rather than a blanket rule.
	const fixturePath = "doc/doc.create"
	oldRef := `{"product_id":"sheet","rpc_name":"update_range"}`
	newRef := `{"product_id":"sheet","rpc_name":"set_cell_range"}`
	reviewedInterfaceRefRedirect[fixturePath] = map[string]string{oldRef: newRef}
	t.Cleanup(func() { delete(reviewedInterfaceRefRedirect, fixturePath) })

	// The baseline fixture is interface_mode=local; the carve-out only applies to
	// mcp-backed leaves, so establish an mcp baseline first.
	baseline := cloneContract(baselineContract())
	mutateTool(&baseline, func(tool *toolSchema) {
		tool.InterfaceMode = interfaceModeMCP
		tool.InterfaceRef = oldRef
	})

	// Accepted: mcp -> mcp, both refs non-empty, the pair is reviewed, nothing
	// else changed.
	redirected := cloneContract(baseline)
	mutateTool(&redirected, func(tool *toolSchema) { tool.InterfaceRef = newRef })
	if failures := checkCompatibility(baseline, redirected); len(failures) != 0 {
		t.Fatalf("a reviewed interface_ref redirect should pass: %v", failures)
	}

	// Paired with a reviewed property clearing, which is the realistic shape:
	// a leaf moving to a nested payload loses its flat property names.
	withCleared := cloneContract(redirected)
	mutateParameter(&withCleared, func(parameter *parameterSchema) {
		parameter.Property = ""
		parameter.PropertySource = propertySourceReviewedMappingExclusion
	})
	if failures := checkCompatibility(baseline, withCleared); len(failures) != 0 {
		t.Fatalf("redirect plus reviewed property clearing should pass: %v", failures)
	}

	// Rejected shapes. Each must still report the redirect, so a surface change
	// cannot ride along behind a backend move.
	for _, tc := range []struct {
		name   string
		mutate func(*schemaContract)
	}{
		{"ref removed rather than redirected", func(contract *schemaContract) {
			mutateTool(contract, func(tool *toolSchema) { tool.InterfaceRef = "" })
		}},
		{"mode moved to composite", func(contract *schemaContract) {
			mutateTool(contract, func(tool *toolSchema) {
				tool.InterfaceMode = "composite"
				tool.InterfaceRef = ""
			})
		}},
		{"mode moved away from mcp while the ref is redirected", func(contract *schemaContract) {
			mutateTool(contract, func(tool *toolSchema) {
				tool.InterfaceMode = "local"
				tool.InterfaceRef = `{"product_id":"sheet","rpc_name":"set_cell_range"}`
			})
		}},
		{"a parameter became required", func(contract *schemaContract) {
			mutateTool(contract, func(tool *toolSchema) { tool.InterfaceRef = `{"product_id":"sheet","rpc_name":"set_cell_range"}` })
			mutateParameter(contract, func(parameter *parameterSchema) { parameter.Required = true })
		}},
		{"a parameter type moved", func(contract *schemaContract) {
			mutateTool(contract, func(tool *toolSchema) { tool.InterfaceRef = `{"product_id":"sheet","rpc_name":"set_cell_range"}` })
			mutateParameter(contract, func(parameter *parameterSchema) { parameter.Type = "integer" })
		}},
		{"a property was cleared without a reviewed exclusion", func(contract *schemaContract) {
			mutateTool(contract, func(tool *toolSchema) { tool.InterfaceRef = `{"product_id":"sheet","rpc_name":"set_cell_range"}` })
			mutateParameter(contract, func(parameter *parameterSchema) {
				parameter.Property = ""
				parameter.PropertySource = "flag_name_inference"
			})
		}},
		{"a confirmation gate moved", func(contract *schemaContract) {
			mutateTool(contract, func(tool *toolSchema) {
				tool.InterfaceRef = `{"product_id":"sheet","rpc_name":"set_cell_range"}`
				tool.Confirmation = "user_required"
			})
		}},
		{"a parameter was dropped", func(contract *schemaContract) {
			mutateTool(contract, func(tool *toolSchema) {
				tool.InterfaceRef = newRef
				delete(tool.Parameters, "title")
			})
		}},
		// The allowlist itself must be load-bearing: a redirect that is not an
		// exact reviewed tool + old→new pair stays incompatible even when the rest
		// of the contract is untouched. Schema shape cannot prove two RPCs share
		// permissions, error taxonomy, or side effects.
		{"redirect to a ref that is not the reviewed target", func(contract *schemaContract) {
			mutateTool(contract, func(tool *toolSchema) {
				tool.InterfaceRef = `{"product_id":"sheet","rpc_name":"some_other_rpc"}`
			})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			drifted := cloneContract(baseline)
			tc.mutate(&drifted)
			failures := checkCompatibility(baseline, drifted)
			var sawRef bool
			for _, failure := range failures {
				if strings.Contains(failure, "changed interface_ref") {
					sawRef = true
				}
			}
			if !sawRef {
				t.Fatalf("expected the redirect to stay reported, got %v", failures)
			}
		})
	}

	// The allowlist must be load-bearing: the exact same old→new pair on a tool
	// with no reviewed entry stays incompatible. Asserted outside the table
	// because the registration has to survive until checkCompatibility runs.
	t.Run("redirect on a tool absent from the reviewed allowlist", func(t *testing.T) {
		saved := reviewedInterfaceRefRedirect[fixturePath]
		delete(reviewedInterfaceRefRedirect, fixturePath)
		defer func() { reviewedInterfaceRefRedirect[fixturePath] = saved }()

		unlisted := cloneContract(baseline)
		mutateTool(&unlisted, func(tool *toolSchema) { tool.InterfaceRef = newRef })
		failures := checkCompatibility(baseline, unlisted)
		var sawRef bool
		for _, failure := range failures {
			if strings.Contains(failure, "changed interface_ref") {
				sawRef = true
			}
		}
		if !sawRef {
			t.Fatalf("an unlisted tool must not get the redirect carve-out, got %v", failures)
		}
	})
}
