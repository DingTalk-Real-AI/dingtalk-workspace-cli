// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/interfacesnapshot"
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

func TestSchemaCompatibilityAcceptsReviewedRemoveConfirmationHardening(t *testing.T) {
	oldTool := baselineContract().Products["doc"].Tools["doc.create"]
	newTool := oldTool
	newTool.Confirmation = "user_required"

	for _, toolPath := range []string{
		"doc/doc.remove_permission",
		"drive/drive.permission_remove",
		"wiki/wiki.remove_member",
	} {
		if failures := checkToolCompatibility(toolPath, oldTool, newTool); len(failures) != 0 {
			t.Fatalf("reviewed confirmation hardening for %s failures = %v", toolPath, failures)
		}
	}
	if failures := checkToolCompatibility("doc/doc.other_remove", oldTool, newTool); len(failures) == 0 {
		t.Fatal("unreviewed confirmation hardening unexpectedly passed")
	}

	// A reviewed tool is only exempt for the exact reviewed transition: an
	// unrelated field drift on the same tool must still be reported.
	riskTool := oldTool
	riskTool.Risk = "high"
	if failures := checkToolCompatibility("doc/doc.remove_permission", oldTool, riskTool); len(failures) == 0 {
		t.Fatal("reviewed tool risk drift unexpectedly passed")
	}

	oldTool.Confirmation = "user_required"
	newTool.Confirmation = "not_required"
	if failures := checkToolCompatibility("doc/doc.remove_permission", oldTool, newTool); len(failures) == 0 {
		t.Fatal("reviewed tool confirmation weakening unexpectedly passed")
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

// registerReviewedParameterTypeFixture puts a fixture entry in the reviewed
// table for the duration of one test, so the behaviour tests exercise the real
// lookup instead of depending on whichever production entries happen to exist.
func registerReviewedParameterTypeFixture(t *testing.T, change parameterTypeChange) {
	t.Helper()
	if _, exists := reviewedParameterTypeChanges[change]; exists {
		t.Fatalf("fixture %+v collides with a production entry", change)
	}
	reviewedParameterTypeChanges[change] = struct{}{}
	t.Cleanup(func() { delete(reviewedParameterTypeChanges, change) })
}

func reviewedFormatTypeFixture() parameterTypeChange {
	return parameterTypeChange{
		ToolPath:  "doc/doc.create",
		Parameter: "format",
		From:      `"string"`,
		To:        `"integer"`,
	}
}

const docCreateFormatTypeFailure = `schema tool "doc/doc.create" parameter "format" changed type`

func assertSchemaFailureContains(t *testing.T, failures []string, want string) {
	t.Helper()
	for _, failure := range failures {
		if strings.Contains(failure, want) {
			return
		}
	}
	t.Fatalf("failures %v do not contain %q", failures, want)
}

func TestCrossPlatformCoverageSchemaCompatReviewedParameterTypeChange(t *testing.T) {
	baseline := cloneContract(baselineContract())
	current := cloneContract(baseline)
	mutateParameter(&current, func(parameter *parameterSchema) { parameter.Type = `"integer"` })

	// Without an entry the migration is still a blocking change.
	assertSchemaFailureContains(t, checkCompatibility(baseline, current), docCreateFormatTypeFailure)

	registerReviewedParameterTypeFixture(t, reviewedFormatTypeFixture())
	if failures := checkCompatibility(baseline, current); len(failures) != 0 {
		t.Fatalf("a reviewed parameter type migration should pass: %v", failures)
	}
}

func TestCrossPlatformCoverageSchemaCompatReviewedParameterTypeChangeIsDirectionSensitive(t *testing.T) {
	registerReviewedParameterTypeFixture(t, reviewedFormatTypeFixture())

	// The reverse migration shares tool and parameter but not direction, so the
	// string->integer entry must not admit it.
	baseline := cloneContract(baselineContract())
	mutateParameter(&baseline, func(parameter *parameterSchema) { parameter.Type = `"integer"` })
	current := cloneContract(baselineContract())
	assertSchemaFailureContains(t, checkCompatibility(baseline, current), docCreateFormatTypeFailure)
}

func TestCrossPlatformCoverageSchemaCompatReviewedParameterTypeChangeRejectsOtherToolsAndParameters(t *testing.T) {
	baseline := cloneContract(baselineContract())
	current := cloneContract(baseline)
	mutateParameter(&current, func(parameter *parameterSchema) { parameter.Type = `"integer"` })

	// Same parameter and direction, different tool.
	other := reviewedFormatTypeFixture()
	other.ToolPath = "doc/doc.elsewhere"
	registerReviewedParameterTypeFixture(t, other)
	assertSchemaFailureContains(t, checkCompatibility(baseline, current), docCreateFormatTypeFailure)

	// Same tool and direction, different parameter.
	elsewhere := reviewedFormatTypeFixture()
	elsewhere.Parameter = "title"
	registerReviewedParameterTypeFixture(t, elsewhere)
	assertSchemaFailureContains(t, checkCompatibility(baseline, current), docCreateFormatTypeFailure)
}

// A reviewed migration is only accepted when the rest of the parameter's
// contract held still. Each case bundles one unrelated regression with the
// reviewed type change and requires the type failure to reappear, so the
// exemption cannot carry a second change in behind it.
func TestCrossPlatformCoverageSchemaCompatReviewedParameterTypeChangeRejectsBundledRegression(t *testing.T) {
	registerReviewedParameterTypeFixture(t, reviewedFormatTypeFixture())

	tests := []struct {
		name   string
		mutate func(*parameterSchema)
	}{
		{name: "changed default", mutate: func(p *parameterSchema) { p.Default = `"text"` }},
		{name: "changed interface_default", mutate: func(p *parameterSchema) { p.InterfaceDefault = `"text"` }},
		{name: "changed format", mutate: func(p *parameterSchema) { p.Format = "uri" }},
		{name: "redirected property", mutate: func(p *parameterSchema) { p.Property = "renamed" }},
		{name: "redirected interface_type", mutate: func(p *parameterSchema) { p.InterfaceType = "integer" }},
		{name: "newly required", mutate: func(p *parameterSchema) { p.Required = true }},
		{name: "newly cli_required", mutate: func(p *parameterSchema) { p.CLIRequired = true }},
		{name: "changed required_when", mutate: func(p *parameterSchema) { p.RequiredWhen = "always" }},
		{name: "narrowed enum", mutate: func(p *parameterSchema) { p.Enum = []string{"markdown"} }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			baseline := cloneContract(baselineContract())
			current := cloneContract(baseline)
			mutateParameter(&current, func(parameter *parameterSchema) {
				parameter.Type = `"integer"`
				test.mutate(parameter)
			})
			assertSchemaFailureContains(t, checkCompatibility(baseline, current), docCreateFormatTypeFailure)
		})
	}
}

// Every drift below is individually *compatible* — on its own it produces no
// failure at all — which is precisely why the carve-out cannot be keyed on "the
// gate recorded no other failure for this parameter". Bundled with a reviewed
// type migration they must still be rejected: none of them were reviewed under
// that entry, and admitting them would make the exemption wider than what the
// table documents.
//
// Each case first asserts that the drift alone really is compatible, so a future
// rule change that starts reporting it turns into an explicit failure here
// rather than quietly turning this test into a duplicate of the bundled-
// regression cases above.
func TestCrossPlatformCoverageSchemaCompatReviewedParameterTypeChangeRejectsIndividuallyCompatibleDrift(t *testing.T) {
	registerReviewedParameterTypeFixture(t, reviewedFormatTypeFixture())

	tests := []struct {
		name     string
		baseline func(*parameterSchema)
		current  func(*parameterSchema)
	}{
		{
			name:     "relaxed required",
			baseline: func(p *parameterSchema) { p.Required = true },
			current:  func(p *parameterSchema) { p.Required = false },
		},
		{
			name:     "relaxed cli_required",
			baseline: func(p *parameterSchema) { p.CLIRequired = true },
			current:  func(p *parameterSchema) { p.CLIRequired = false },
		},
		{
			name:     "cleared required_when",
			baseline: func(p *parameterSchema) { p.RequiredWhen = "always" },
			current:  func(p *parameterSchema) { p.RequiredWhen = "" },
		},
		{
			name:    "widened enum",
			current: func(p *parameterSchema) { p.Enum = []string{"html", "markdown", "text"} },
		},
		{
			name:     "cleared interface_type",
			baseline: func(p *parameterSchema) { p.InterfaceType = "string" },
			current:  func(p *parameterSchema) { p.InterfaceType = "" },
		},
		{
			name: "cleared property through reviewed mapping exclusion",
			current: func(p *parameterSchema) {
				p.Property = ""
				p.PropertySource = propertySourceReviewedMappingExclusion
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			baseline := cloneContract(baselineContract())
			if test.baseline != nil {
				mutateParameter(&baseline, test.baseline)
			}

			driftOnly := cloneContract(baseline)
			mutateParameter(&driftOnly, test.current)
			if failures := checkCompatibility(baseline, driftOnly); len(failures) != 0 {
				t.Fatalf("前提不成立：该变化本身已不兼容，用例测不到相等性守卫: %v", failures)
			}

			bundled := cloneContract(baseline)
			mutateParameter(&bundled, func(parameter *parameterSchema) {
				parameter.Type = `"integer"`
				test.current(parameter)
			})
			assertSchemaFailureContains(t, checkCompatibility(baseline, bundled), docCreateFormatTypeFailure)
		})
	}
}

// The behaviour tests above register their own fixtures, so they cannot catch a
// production entry whose type values are spelled in the wrong form. That mistake
// disables the exemption silently and only a real gate run reports it — exactly
// how reviewedInterfaceRefRedirect was broken twice. Recompute both values
// through schemaType instead of trusting the spelling in the table.
func TestCrossPlatformCoverageReviewedParameterTypeChangeKeysAreCanonical(t *testing.T) {
	if len(reviewedParameterTypeChanges) == 0 {
		t.Fatal("豁免表为空：若确已清空，请连这条守卫一起删除")
	}
	for change := range reviewedParameterTypeChanges {
		if strings.Count(change.ToolPath, "/") != 1 ||
			strings.HasPrefix(change.ToolPath, "/") || strings.HasSuffix(change.ToolPath, "/") {
			t.Errorf("tool path %q 不是 \"<product id>/<tool id>\" 形态", change.ToolPath)
		}
		if strings.TrimSpace(change.Parameter) != change.Parameter || change.Parameter == "" ||
			strings.HasPrefix(change.Parameter, "-") {
			t.Errorf("%s: 参数名应是 Schema 里的裸名，登记为 %q", change.ToolPath, change.Parameter)
		}
		if change.From == change.To {
			t.Errorf("%s %s: From 与 To 相同，不构成迁移", change.ToolPath, change.Parameter)
		}
		for _, side := range []struct {
			label string
			value string
		}{{"From", change.From}, {"To", change.To}} {
			var name string
			if err := json.Unmarshal([]byte(side.value), &name); err != nil {
				t.Errorf("%s %s 的 %s=%q 不是合法的 JSON type 值（schemaType 产出带引号的形态，如 `\"string\"`）: %v",
					change.ToolPath, change.Parameter, side.label, side.value, err)
				continue
			}
			if got := schemaType(map[string]any{"type": name}); got != side.value {
				t.Errorf("%s %s 的 %s 不是规范形态\n  登记: %s\n  规范: %s",
					change.ToolPath, change.Parameter, side.label, side.value, got)
			}
			// 复算只能证明引号形态对，证明不了类型名本身没拼错——"integar" 同样
			// 能通过复算。JSON Schema 的 type 取值是封闭集合，直接校验。
			switch name {
			case "string", "integer", "number", "boolean", "array", "object", "null":
			default:
				t.Errorf("%s %s 的 %s=%q 不是 JSON Schema 的合法 type 名",
					change.ToolPath, change.Parameter, side.label, name)
			}
		}
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

func TestCrossPlatformCoverageSchemaCompatReviewedConstraintTransition(t *testing.T) {
	const toolPath = "doc/doc.shortcut_import"
	oldTool := toolSchema{Constraints: `{"require_one_of":[["folder","workspace"]]}`}
	newTool := oldTool
	newTool.Constraints = ""

	if !compatibleReviewedConstraintTransition(toolPath, oldTool, newTool) {
		t.Fatal("reviewed doc import target removal must be accepted")
	}
	if failures := checkToolCompatibility(toolPath, oldTool, newTool); len(failures) != 0 {
		t.Fatalf("reviewed doc import constraint transition failed: %v", failures)
	}

	for _, test := range []struct {
		name string
		path string
		old  string
		new  string
	}{
		{name: "unlisted tool", path: "doc/doc.other", old: oldTool.Constraints, new: ""},
		{name: "unlisted source", path: toolPath, old: `{"require_one_of":[["folder","workspace","name"]]}`, new: ""},
		{name: "unlisted target", path: toolPath, old: oldTool.Constraints, new: `{}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			if compatibleReviewedConstraintTransition(test.path, toolSchema{Constraints: test.old}, toolSchema{Constraints: test.new}) {
				t.Fatal("unreviewed constraint transition unexpectedly passed")
			}
		})
	}

	const sheetToolPath = "sheet/sheet.create_float_image"
	const sheetTarget = `{"mutually_exclusive":[["file","src"]],"require_one_of":[["file","src"]]}`
	sheetOldTool := toolSchema{}
	sheetNewTool := sheetOldTool
	sheetNewTool.Constraints = sheetTarget

	if !compatibleReviewedConstraintTransition(sheetToolPath, sheetOldTool, sheetNewTool) {
		t.Fatal("reviewed float-image local-file transition must be accepted")
	}
	if failures := checkToolCompatibility(sheetToolPath, sheetOldTool, sheetNewTool); len(failures) != 0 {
		t.Fatalf("reviewed float-image local-file transition failed: %v", failures)
	}

	for _, test := range []struct {
		name string
		path string
		old  string
		new  string
	}{
		{name: "float image unlisted tool", path: "sheet/sheet.other", new: sheetTarget},
		{name: "float image unlisted source", path: sheetToolPath, old: `{"require_one_of":[["src"]]}`, new: sheetTarget},
		{name: "float image unlisted target", path: sheetToolPath, new: `{"require_one_of":[["file","src"]]}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			if compatibleReviewedConstraintTransition(test.path, toolSchema{Constraints: test.old}, toolSchema{Constraints: test.new}) {
				t.Fatal("unreviewed float-image constraint transition unexpectedly passed")
			}
		})
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

func TestCrossPlatformCoverageSchemaFlagMigrationNormalizesExactRename(t *testing.T) {
	baseline := schemaFlagMigrationContract(false)
	current := schemaFlagMigrationContract(true)

	normalized, err := normalizeSchemaFlagMigrations(baseline, current, schemaFlagMigrationAuthorizations())
	if err != nil {
		t.Fatal(err)
	}
	if failures := checkCompatibility(normalized, current); len(failures) != 0 {
		t.Fatalf("exact flag migration should pass after normalization: %v", failures)
	}

	tool := normalized.Products["chat"].Tools["chat.edit_message"]
	for _, legacy := range []string{"group", "id", "msg-id"} {
		if _, exists := tool.Parameters[legacy]; exists {
			t.Fatalf("normalized baseline retained legacy parameter %q", legacy)
		}
	}
	if canonical := tool.Parameters["conversation-id"]; canonical.Required || canonical.CLIRequired {
		t.Fatalf("optional canonical rename changed requiredness: %#v", canonical)
	}
	if canonical := tool.Parameters["message-id"]; !canonical.Required || !canonical.CLIRequired {
		t.Fatalf("required canonical rename changed requiredness: %#v", canonical)
	}
	if tool.Constraints != current.Products["chat"].Tools["chat.edit_message"].Constraints {
		t.Fatalf("constraints were not normalized: %s", tool.Constraints)
	}
}

func TestCrossPlatformCoverageSchemaFlagRequirednessMigration(t *testing.T) {
	baseline := schemaContract{
		Version: schemaContractVersion,
		Products: map[string]productSchema{
			"report": {
				Tools: map[string]toolSchema{
					"report.entry_submit": {
						PrimaryCLIPath: "report entry submit",
						Parameters: map[string]parameterSchema{
							"to-user-ids": {Type: `"string"`, Property: "toUserIds", InterfaceType: "string"},
						},
					},
				},
			},
		},
	}
	current := cloneContract(baseline)
	product := current.Products["report"]
	tool := product.Tools["report.entry_submit"]
	parameter := tool.Parameters["to-user-ids"]
	parameter.Required = true
	parameter.CLIRequired = true
	tool.Parameters["to-user-ids"] = parameter
	product.Tools["report.entry_submit"] = tool
	current.Products["report"] = product

	migration := interfacesnapshot.FlagMigration{
		Kind:    interfacesnapshot.FlagMigrationRequirednessChange,
		Command: "dws report entry submit",
		Flag: &interfacesnapshot.FlagMigrationSide{
			Name:   "to-user-ids",
			Before: interfacesnapshot.FlagMigrationState{Present: true, Type: "string", Scope: "local"},
			After:  interfacesnapshot.FlagMigrationState{Present: true, Type: "string", Required: true, Scope: "local"},
		},
		State:  interfacesnapshot.FlagMigrationConsumed,
		Reason: "Reject report submissions that have no visible recipient.",
	}
	normalized, err := normalizeSchemaFlagMigrations(baseline, current, []interfacesnapshot.FlagMigration{migration})
	if err != nil {
		t.Fatal(err)
	}
	if failures := checkCompatibility(normalized, current); len(failures) != 0 {
		t.Fatalf("exact requiredness migration should pass after normalization: %v", failures)
	}

	t.Run("non-requiredness drift remains blocked", func(t *testing.T) {
		drifted := cloneContract(current)
		product := drifted.Products["report"]
		tool := product.Tools["report.entry_submit"]
		parameter := tool.Parameters["to-user-ids"]
		parameter.Type = `"array"`
		tool.Parameters["to-user-ids"] = parameter
		product.Tools["report.entry_submit"] = tool
		drifted.Products["report"] = product
		normalized, err := normalizeSchemaFlagMigrations(baseline, drifted, []interfacesnapshot.FlagMigration{migration})
		if err != nil {
			t.Fatal(err)
		}
		if failures := strings.Join(checkCompatibility(normalized, drifted), "\n"); !strings.Contains(failures, "changed type") {
			t.Fatalf("requiredness migration hid type drift: %s", failures)
		}
	})

	t.Run("partial Schema projection fails closed", func(t *testing.T) {
		partial := cloneContract(current)
		product := partial.Products["report"]
		tool := product.Tools["report.entry_submit"]
		parameter := tool.Parameters["to-user-ids"]
		parameter.CLIRequired = false
		tool.Parameters["to-user-ids"] = parameter
		product.Tools["report.entry_submit"] = tool
		partial.Products["report"] = product
		if _, err := normalizeSchemaFlagMigrations(baseline, partial, []interfacesnapshot.FlagMigration{migration}); err == nil || !strings.Contains(err.Error(), "required and cli_required") {
			t.Fatalf("partial requiredness projection error = %v", err)
		}
	})

	t.Run("CLI-only migration does not manufacture Schema", func(t *testing.T) {
		cliOnly := migration
		cliOnly.Command = "dws report cli-only"
		normalized, err := normalizeSchemaFlagMigrations(baseline, current, []interfacesnapshot.FlagMigration{cliOnly})
		if err != nil || !reflect.DeepEqual(normalized, baseline) {
			t.Fatalf("CLI-only requiredness migration = %#v, %v", normalized, err)
		}
	})

	t.Run("duplicate historical path fails closed", func(t *testing.T) {
		duplicate := cloneContract(baseline)
		product := duplicate.Products["report"]
		product.Tools["report.duplicate"] = product.Tools["report.entry_submit"]
		duplicate.Products["report"] = product
		if _, err := normalizeSchemaFlagMigrations(duplicate, current, []interfacesnapshot.FlagMigration{migration}); err == nil || !strings.Contains(err.Error(), "matches 2") {
			t.Fatalf("duplicate requiredness Schema path error = %v", err)
		}
	})

	t.Run("missing historical parameter is not authority", func(t *testing.T) {
		missing := cloneContract(baseline)
		product := missing.Products["report"]
		tool := product.Tools["report.entry_submit"]
		delete(tool.Parameters, "to-user-ids")
		product.Tools["report.entry_submit"] = tool
		missing.Products["report"] = product
		normalized, err := normalizeSchemaFlagMigrations(missing, current, []interfacesnapshot.FlagMigration{migration})
		if err != nil || !reflect.DeepEqual(normalized, missing) {
			t.Fatalf("missing historical requiredness parameter = %#v, %v", normalized, err)
		}
	})

	t.Run("missing current tool stays visible to ordinary checker", func(t *testing.T) {
		missing := cloneContract(current)
		delete(missing.Products, "report")
		normalized, err := normalizeSchemaFlagMigrations(baseline, missing, []interfacesnapshot.FlagMigration{migration})
		if err != nil {
			t.Fatal(err)
		}
		if failures := checkCompatibility(normalized, missing); len(failures) == 0 {
			t.Fatal("requiredness migration hid missing current Schema tool")
		}
	})

	t.Run("missing current parameter stays visible to ordinary checker", func(t *testing.T) {
		missing := cloneContract(current)
		product := missing.Products["report"]
		tool := product.Tools["report.entry_submit"]
		delete(tool.Parameters, "to-user-ids")
		product.Tools["report.entry_submit"] = tool
		missing.Products["report"] = product
		normalized, err := normalizeSchemaFlagMigrations(baseline, missing, []interfacesnapshot.FlagMigration{migration})
		if err != nil {
			t.Fatal(err)
		}
		if failures := strings.Join(checkCompatibility(normalized, missing), "\n"); !strings.Contains(failures, "lost parameter") {
			t.Fatalf("requiredness migration hid missing current parameter: %s", failures)
		}
	})
}

func TestCrossPlatformCoverageSchemaFlagMigrationRejectsSemanticDrift(t *testing.T) {
	tests := []struct {
		name   string
		want   string
		mutate func(*parameterSchema)
	}{
		{name: "required decline", want: "must be required and cli_required", mutate: func(parameter *parameterSchema) {
			parameter.Required = false
		}},
		{name: "cli required decline", want: "must be required and cli_required", mutate: func(parameter *parameterSchema) {
			parameter.CLIRequired = false
		}},
		{name: "type", want: "non-migration field", mutate: func(parameter *parameterSchema) { parameter.Type = `"integer"` }},
		{name: "property", want: "non-migration field", mutate: func(parameter *parameterSchema) { parameter.Property = "other" }},
		{name: "interface type", want: "non-migration field", mutate: func(parameter *parameterSchema) { parameter.InterfaceType = "integer" }},
		{name: "required when", want: "non-migration field", mutate: func(parameter *parameterSchema) { parameter.RequiredWhen = "mode=other" }},
		{name: "default", want: "non-migration field", mutate: func(parameter *parameterSchema) { parameter.Default = `"other"` }},
		{name: "interface default", want: "non-migration field", mutate: func(parameter *parameterSchema) { parameter.InterfaceDefault = `"other"` }},
		{name: "format", want: "non-migration field", mutate: func(parameter *parameterSchema) { parameter.Format = "uri" }},
		{name: "enum", want: "non-migration field", mutate: func(parameter *parameterSchema) { parameter.Enum = []string{"a"} }},
		{name: "enum same length", want: "non-migration field", mutate: func(parameter *parameterSchema) { parameter.Enum = []string{"a", "c"} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			current := schemaFlagMigrationContract(true)
			product := current.Products["chat"]
			tool := product.Tools["chat.edit_message"]
			canonical := tool.Parameters["message-id"]
			test.mutate(&canonical)
			tool.Parameters["message-id"] = canonical
			product.Tools["chat.edit_message"] = tool
			current.Products["chat"] = product

			_, err := normalizeSchemaFlagMigrations(
				schemaFlagMigrationContract(false),
				current,
				schemaFlagMigrationAuthorizations(),
			)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("normalizeSchemaFlagMigrations() error = %v, want %q", err, test.want)
			}
		})
	}

	for _, test := range []struct {
		name   string
		want   string
		mutate func(*parameterSchema)
	}{
		{name: "optional required promotion", want: "changed requiredness", mutate: func(parameter *parameterSchema) {
			parameter.Required = true
		}},
		{name: "optional cli_required promotion", want: "changed cli_required", mutate: func(parameter *parameterSchema) {
			parameter.CLIRequired = true
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			current := schemaFlagMigrationContract(true)
			product := current.Products["chat"]
			tool := product.Tools["chat.edit_message"]
			canonical := tool.Parameters["conversation-id"]
			test.mutate(&canonical)
			tool.Parameters["conversation-id"] = canonical
			product.Tools["chat.edit_message"] = tool
			current.Products["chat"] = product

			_, err := normalizeSchemaFlagMigrations(
				schemaFlagMigrationContract(false),
				current,
				schemaFlagMigrationAuthorizations(),
			)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("normalizeSchemaFlagMigrations() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestCrossPlatformCoverageSchemaFlagMigrationAdapterBranches(t *testing.T) {
	baseline := schemaFlagMigrationContract(false)
	current := schemaFlagMigrationContract(true)
	noConstraintsBaseline := cloneContract(baseline)
	product := noConstraintsBaseline.Products["chat"]
	tool := product.Tools["chat.edit_message"]
	tool.Constraints = ""
	product.Tools["chat.edit_message"] = tool
	noConstraintsBaseline.Products["chat"] = product
	noConstraintsCurrent := cloneContract(current)
	product = noConstraintsCurrent.Products["chat"]
	tool = product.Tools["chat.edit_message"]
	tool.Constraints = ""
	product.Tools["chat.edit_message"] = tool
	noConstraintsCurrent.Products["chat"] = product
	if _, err := normalizeSchemaFlagMigrations(noConstraintsBaseline, noConstraintsCurrent, schemaFlagMigrationAuthorizations()); err != nil {
		t.Fatalf("unchanged empty constraints: %v", err)
	}

	cliOnly := schemaFlagMigrationAuthorizations()[:1]
	cliOnly[0].Command = "dws chat cli-only"
	normalized, err := normalizeSchemaFlagMigrations(baseline, current, cliOnly)
	if err != nil || !reflect.DeepEqual(normalized, baseline) {
		t.Fatalf("CLI-only migration = %#v, %v", normalized, err)
	}

	duplicate := cloneContract(baseline)
	product = duplicate.Products["chat"]
	product.Tools["chat.another"] = product.Tools["chat.edit_message"]
	duplicate.Products["chat"] = product
	duplicate.Products["chat2"] = productSchema{Tools: map[string]toolSchema{
		"chat.third": product.Tools["chat.edit_message"],
	}}
	if matches := schemaToolsByPrimaryPath(duplicate, "chat message edit"); len(matches) != 3 || matches[0].productID != "chat" || matches[0].toolID != "chat.another" {
		t.Fatalf("sorted duplicate Schema matches = %#v", matches)
	}
	if _, err := normalizeSchemaFlagMigrations(duplicate, current, schemaFlagMigrationAuthorizations()[:1]); err == nil || !strings.Contains(err.Error(), "matches 3") {
		t.Fatalf("duplicate Schema path error = %v", err)
	}

	missingCurrent := cloneContract(current)
	delete(missingCurrent.Products, "chat")
	normalized, err = normalizeSchemaFlagMigrations(baseline, missingCurrent, schemaFlagMigrationAuthorizations())
	if err != nil {
		t.Fatal(err)
	}
	if failures := checkCompatibility(normalized, missingCurrent); len(failures) == 0 {
		t.Fatal("missing candidate product was normalized away")
	}

	canonicalOnlyMigration := schemaFlagMigrationAuthorizations()[0]
	canonicalOnlyMigration.Legacy.Name = "legacy-not-published-in-schema"
	canonicalOnlyBaseline := schemaFlagMigrationContract(true)
	driftedCanonical := cloneContract(canonicalOnlyBaseline)
	product = driftedCanonical.Products["chat"]
	tool = product.Tools["chat.edit_message"]
	canonical := tool.Parameters["conversation-id"]
	canonical.Property = "different"
	tool.Parameters["conversation-id"] = canonical
	product.Tools["chat.edit_message"] = tool
	driftedCanonical.Products["chat"] = product
	normalized, err = normalizeSchemaFlagMigrations(canonicalOnlyBaseline, driftedCanonical, []interfacesnapshot.FlagMigration{canonicalOnlyMigration})
	if err != nil {
		t.Fatal(err)
	}
	if failures := strings.Join(checkCompatibility(normalized, driftedCanonical), "\n"); !strings.Contains(failures, "changed property") {
		t.Fatalf("canonical-only Schema drift was hidden: %s", failures)
	}

	promotedCanonical := cloneContract(canonicalOnlyBaseline)
	product = promotedCanonical.Products["chat"]
	tool = product.Tools["chat.edit_message"]
	canonical = tool.Parameters["conversation-id"]
	canonical.Required = true
	canonical.CLIRequired = true
	tool.Parameters["conversation-id"] = canonical
	product.Tools["chat.edit_message"] = tool
	promotedCanonical.Products["chat"] = product
	normalized, err = normalizeSchemaFlagMigrations(
		canonicalOnlyBaseline,
		promotedCanonical,
		[]interfacesnapshot.FlagMigration{canonicalOnlyMigration},
	)
	if err != nil {
		t.Fatal(err)
	}
	if failures := strings.Join(checkCompatibility(normalized, promotedCanonical), "\n"); !strings.Contains(failures, "newly required") || !strings.Contains(failures, "newly cli_required") {
		t.Fatalf("canonical-only required promotion was hidden: %s", failures)
	}

	canonicalConflictBaseline := schemaFlagMigrationContract(false)
	product = canonicalConflictBaseline.Products["chat"]
	tool = product.Tools["chat.edit_message"]
	canonical = tool.Parameters["conversation-id"]
	canonical.Property = "historicalCanonicalProperty"
	tool.Parameters["conversation-id"] = canonical
	product.Tools["chat.edit_message"] = tool
	canonicalConflictBaseline.Products["chat"] = product
	if _, err := normalizeSchemaFlagMigrations(
		canonicalConflictBaseline,
		schemaFlagMigrationContract(true),
		schemaFlagMigrationAuthorizations()[:1],
	); err == nil || !strings.Contains(err.Error(), "non-migration field") {
		t.Fatalf("historical canonical drift error = %v", err)
	}

	conflicting := schemaFlagMigrationAuthorizations()[0]
	conflicting.Canonical.Name = "other-conversation-id"
	conflicting.Legacy.After.AliasOf = conflicting.Canonical.Name
	product = current.Products["chat"]
	tool = product.Tools["chat.edit_message"]
	tool.Parameters[conflicting.Canonical.Name] = tool.Parameters["conversation-id"]
	product.Tools["chat.edit_message"] = tool
	current.Products["chat"] = product
	if _, err := normalizeSchemaFlagMigrations(baseline, current, []interfacesnapshot.FlagMigration{
		schemaFlagMigrationAuthorizations()[0],
		conflicting,
	}); err == nil || !strings.Contains(err.Error(), "maps Schema parameter") {
		t.Fatalf("conflicting Schema mapping error = %v", err)
	}

	old := parameterSchema{Required: true, CLIRequired: true}
	if err := validateRenamedSchemaParameter(schemaFlagMigrationAuthorizations()[0], old, parameterSchema{CLIRequired: true}); err == nil || !strings.Contains(err.Error(), "changed requiredness") {
		t.Fatalf("direct required decline error = %v", err)
	}
	if err := validateRenamedSchemaParameter(schemaFlagMigrationAuthorizations()[0], old, parameterSchema{Required: true}); err == nil || !strings.Contains(err.Error(), "changed cli_required") {
		t.Fatalf("direct cli_required decline error = %v", err)
	}
	optional := parameterSchema{}
	if err := validateRenamedSchemaParameter(schemaFlagMigrationAuthorizations()[0], optional, parameterSchema{Required: true}); err == nil || !strings.Contains(err.Error(), "changed requiredness") {
		t.Fatalf("direct required promotion error = %v", err)
	}
	if err := validateRenamedSchemaParameter(schemaFlagMigrationAuthorizations()[0], optional, parameterSchema{CLIRequired: true}); err == nil || !strings.Contains(err.Error(), "changed cli_required") {
		t.Fatalf("direct cli_required promotion error = %v", err)
	}
}

func TestCrossPlatformCoverageSchemaFlagMigrationRejectsPartialAndUnrelatedChanges(t *testing.T) {
	baseline := schemaFlagMigrationContract(false)
	migrations := schemaFlagMigrationAuthorizations()

	current := schemaFlagMigrationContract(true)
	product := current.Products["chat"]
	tool := product.Tools["chat.edit_message"]
	delete(tool.Parameters, "message-id")
	product.Tools["chat.edit_message"] = tool
	current.Products["chat"] = product
	if _, err := normalizeSchemaFlagMigrations(baseline, current, migrations); err == nil || !strings.Contains(err.Error(), "does not publish canonical") {
		t.Fatalf("missing canonical error = %v", err)
	}

	current = schemaFlagMigrationContract(true)
	product = current.Products["chat"]
	tool = product.Tools["chat.edit_message"]
	tool.Parameters["msg-id"] = schemaFlagMigrationContract(false).Products["chat"].Tools["chat.edit_message"].Parameters["msg-id"]
	product.Tools["chat.edit_message"] = tool
	current.Products["chat"] = product
	if _, err := normalizeSchemaFlagMigrations(baseline, current, migrations); err == nil || !strings.Contains(err.Error(), "still publishes legacy") {
		t.Fatalf("published legacy error = %v", err)
	}

	current = schemaFlagMigrationContract(true)
	product = current.Products["chat"]
	tool = product.Tools["chat.edit_message"]
	delete(tool.Parameters, "unrelated")
	product.Tools["chat.edit_message"] = tool
	current.Products["chat"] = product
	normalized, err := normalizeSchemaFlagMigrations(baseline, current, migrations)
	if err != nil {
		t.Fatal(err)
	}
	if failures := strings.Join(checkCompatibility(normalized, current), "\n"); !strings.Contains(failures, `lost parameter "unrelated"`) {
		t.Fatalf("unrelated parameter removal was hidden: %s", failures)
	}

	current = schemaFlagMigrationContract(true)
	product = current.Products["chat"]
	tool = product.Tools["chat.edit_message"]
	tool.Constraints = `{"require_one_of":[["conversation-id"]],"require_together":[["conversation-id","unrelated"]]}`
	product.Tools["chat.edit_message"] = tool
	current.Products["chat"] = product
	normalized, err = normalizeSchemaFlagMigrations(baseline, current, migrations)
	if err != nil {
		t.Fatal(err)
	}
	if failures := strings.Join(checkCompatibility(normalized, current), "\n"); !strings.Contains(failures, "changed constraints") {
		t.Fatalf("unrelated constraint change was hidden: %s", failures)
	}

	// A CLI ledger entry alone cannot authorize a Schema constraint rewrite
	// when the historical Schema published neither the legacy nor canonical
	// parameter involved in that entry.
	noSchemaEvidence := schemaFlagMigrationContract(false)
	product = noSchemaEvidence.Products["chat"]
	tool = product.Tools["chat.edit_message"]
	for _, name := range []string{"conversation-id", "group", "id", "msg-id"} {
		delete(tool.Parameters, name)
	}
	product.Tools["chat.edit_message"] = tool
	noSchemaEvidence.Products["chat"] = product
	normalized, err = normalizeSchemaFlagMigrations(noSchemaEvidence, schemaFlagMigrationContract(true), migrations)
	if err != nil {
		t.Fatal(err)
	}
	if failures := strings.Join(checkCompatibility(normalized, schemaFlagMigrationContract(true)), "\n"); !strings.Contains(failures, "changed constraints") {
		t.Fatalf("constraint rewrite without Schema parameter evidence was hidden: %s", failures)
	}

	// A baseline that already contains only the canonical parameter is not
	// evidence that a stray legacy name in constraints belongs to the migration.
	canonicalOnly := schemaFlagMigrationContract(true)
	product = canonicalOnly.Products["chat"]
	tool = product.Tools["chat.edit_message"]
	tool.Constraints = `{"require_one_of":[["group"]]}`
	product.Tools["chat.edit_message"] = tool
	canonicalOnly.Products["chat"] = product
	normalized, err = normalizeSchemaFlagMigrations(canonicalOnly, schemaFlagMigrationContract(true), migrations)
	if err != nil {
		t.Fatal(err)
	}
	if failures := strings.Join(checkCompatibility(normalized, schemaFlagMigrationContract(true)), "\n"); !strings.Contains(failures, "changed constraints") {
		t.Fatalf("canonical-only baseline authorized a legacy constraint rewrite: %s", failures)
	}
}

func TestCrossPlatformCoverageSchemaFlagMigrationConstraintsFailClosed(t *testing.T) {
	for _, constraints := range []string{
		`{`,
		`{"`,
		`{"require_one_of":`,
		`{"unknown_kind":[["conversation-id"]]}`,
		`{"require_one_of":"conversation-id"}`,
		`{"require_one_of":["conversation-id"]}`,
		`{"require_one_of":null}`,
		`{"require_one_of":[[]]}`,
		`{"require_one_of":[[" conversation-id"]]}`,
		`{"require_one_of":[["conversation-id"]],"require_one_of":[["conversation-id"]]}`,
		`{"require_one_of":[]`,
		`{} {}`,
		`[]`,
	} {
		current := schemaFlagMigrationContract(true)
		product := current.Products["chat"]
		tool := product.Tools["chat.edit_message"]
		tool.Constraints = constraints
		product.Tools["chat.edit_message"] = tool
		current.Products["chat"] = product

		normalized, err := normalizeSchemaFlagMigrations(
			schemaFlagMigrationContract(false),
			current,
			schemaFlagMigrationAuthorizations(),
		)
		if err != nil {
			t.Fatal(err)
		}
		if failures := strings.Join(checkCompatibility(normalized, current), "\n"); !strings.Contains(failures, "changed constraints") {
			t.Fatalf("malformed constraints %s were authorized: %s", constraints, failures)
		}
	}

	if encoded, ok := canonicalizeMigratedConstraints("", nil); !ok || encoded != "" {
		t.Fatalf("empty migrated constraints = %q, %v", encoded, ok)
	}
	encoded, ok := canonicalizeMigratedConstraints(
		`{"require_one_of":[["b"],["a"],["a"],["c","c"]]}`,
		nil,
	)
	if !ok || encoded != `{"require_one_of":[["a"],["b"],["c"]]}` {
		t.Fatalf("canonical migrated constraint groups = %q, %v", encoded, ok)
	}
}

func TestCrossPlatformCoverageSchemaMigrationCLIRequiresCompleteCheckInputs(t *testing.T) {
	allInputs := []string{
		"--approved-flag-migrations", "approved.json",
		"--candidate-flag-migrations", "candidate.json",
		"--migration-current-snapshot", "current.json",
		"--migration-base-snapshot", "base.json",
		"--migration-stable-snapshot", "stable.json",
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--check", "baseline.json", "--current", "schema.json", "--approved-flag-migrations", "approved.json"}, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "both flag manifests") {
		t.Fatalf("partial migration inputs code=%d stderr=%q", code, stderr.String())
	}

	for _, mode := range []string{"--normalize", "--merge"} {
		stderr.Reset()
		args := append([]string{mode, "schema.json"}, allInputs...)
		if code := run(args, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "only valid with --check") {
			t.Fatalf("%s migration inputs code=%d stderr=%q", mode, code, stderr.String())
		}
	}
}

func TestCrossPlatformCoverageSchemaMigrationLifecycleAndRun(t *testing.T) {
	directory := t.TempDir()
	baselinePath := filepath.Join(directory, "schema-baseline.json")
	currentSchemaPath := filepath.Join(directory, "schema-current.json")
	writeSchemaContractFile(t, baselinePath, schemaFlagMigrationContract(false))
	writeRawSchemaContractFile(t, currentSchemaPath, schemaFlagMigrationContract(true))

	approvedPath := filepath.Join(directory, "approved.json")
	candidatePath := filepath.Join(directory, "candidate.json")
	currentSnapshotPath := filepath.Join(directory, "interface-current.json")
	baseSnapshotPath := filepath.Join(directory, "interface-base.json")
	stableSnapshotPath := filepath.Join(directory, "interface-stable.json")
	writeFlagMigrationManifestFile(t, approvedPath, schemaFlagMigrationManifest(interfacesnapshot.FlagMigrationPending))
	writeFlagMigrationManifestFile(t, candidatePath, schemaFlagMigrationManifest(interfacesnapshot.FlagMigrationConsumed))
	writeInterfaceSnapshotFile(t, currentSnapshotPath, schemaFlagMigrationSnapshot(true))
	writeInterfaceSnapshotFile(t, baseSnapshotPath, schemaFlagMigrationSnapshot(false))
	writeInterfaceSnapshotFile(t, stableSnapshotPath, schemaFlagMigrationSnapshot(false))

	args := []string{
		"--check", baselinePath,
		"--current", currentSchemaPath,
		"--approved-flag-migrations", approvedPath,
		"--candidate-flag-migrations", candidatePath,
		"--migration-current-snapshot", currentSnapshotPath,
		"--migration-base-snapshot", baseSnapshotPath,
		"--migration-stable-snapshot", stableSnapshotPath,
	}
	var stdout, stderr bytes.Buffer
	if code := run(args, &stdout, &stderr); code != 0 {
		t.Fatalf("governed Schema check code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "compatibility check: ok") {
		t.Fatalf("governed Schema check output = %q", stdout.String())
	}

	badArgs := append([]string(nil), args...)
	badArgs[5] = filepath.Join(directory, "missing-approved.json")
	stderr.Reset()
	if code := run(badArgs, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "authorize Schema flag migrations") {
		t.Fatalf("authorization error code=%d stderr=%q", code, stderr.String())
	}

	legacyCurrent := schemaFlagMigrationContract(true)
	product := legacyCurrent.Products["chat"]
	tool := product.Tools["chat.edit_message"]
	tool.Parameters["msg-id"] = schemaFlagMigrationContract(false).Products["chat"].Tools["chat.edit_message"].Parameters["msg-id"]
	product.Tools["chat.edit_message"] = tool
	legacyCurrent.Products["chat"] = product
	writeRawSchemaContractFile(t, currentSchemaPath, legacyCurrent)
	stderr.Reset()
	if code := run(args, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "normalize approved Schema flag migrations") {
		t.Fatalf("normalization error code=%d stderr=%q", code, stderr.String())
	}

	// Candidate-owned pending records cannot approve their own after-state.
	writeFlagMigrationManifestFile(t, approvedPath, interfacesnapshot.FlagMigrationManifest{
		Version:    interfacesnapshot.FlagMigrationManifestVersion,
		Migrations: []interfacesnapshot.FlagMigration{},
	})
	writeFlagMigrationManifestFile(t, candidatePath, schemaFlagMigrationManifest(interfacesnapshot.FlagMigrationPending))
	if _, err := authorizeSchemaFlagMigrations(approvedPath, candidatePath, currentSnapshotPath, baseSnapshotPath, stableSnapshotPath); err == nil || !strings.Contains(err.Error(), "cannot authorize its own interface change") {
		t.Fatalf("self-approval error = %v", err)
	}

	// An incomplete after-state fails lifecycle authorization before Schema is normalized.
	writeFlagMigrationManifestFile(t, approvedPath, schemaFlagMigrationManifest(interfacesnapshot.FlagMigrationPending))
	writeFlagMigrationManifestFile(t, candidatePath, schemaFlagMigrationManifest(interfacesnapshot.FlagMigrationConsumed))
	partial := schemaFlagMigrationSnapshot(true)
	partial.Commands[0].LocalFlags = partial.Commands[0].LocalFlags[:len(partial.Commands[0].LocalFlags)-1]
	writeInterfaceSnapshotFile(t, currentSnapshotPath, partial)
	if _, err := authorizeSchemaFlagMigrations(approvedPath, candidatePath, currentSnapshotPath, baseSnapshotPath, stableSnapshotPath); err == nil || !strings.Contains(err.Error(), "partially applied") {
		t.Fatalf("partial migration error = %v", err)
	}
}

func TestCrossPlatformCoverageSchemaMigrationFileErrors(t *testing.T) {
	directory := t.TempDir()
	approvedPath := filepath.Join(directory, "approved.json")
	candidatePath := filepath.Join(directory, "candidate.json")
	currentPath := filepath.Join(directory, "current.json")
	basePath := filepath.Join(directory, "base.json")
	stablePath := filepath.Join(directory, "stable.json")
	writeFlagMigrationManifestFile(t, approvedPath, schemaFlagMigrationManifest(interfacesnapshot.FlagMigrationPending))
	writeFlagMigrationManifestFile(t, candidatePath, schemaFlagMigrationManifest(interfacesnapshot.FlagMigrationConsumed))
	writeInterfaceSnapshotFile(t, currentPath, schemaFlagMigrationSnapshot(true))
	writeInterfaceSnapshotFile(t, basePath, schemaFlagMigrationSnapshot(false))
	writeInterfaceSnapshotFile(t, stablePath, schemaFlagMigrationSnapshot(false))

	paths := []string{approvedPath, candidatePath, currentPath, basePath, stablePath}
	wants := []string{
		"read approved flag migrations",
		"read candidate flag migrations",
		"read migration current snapshot",
		"read migration base snapshot",
		"read migration stable snapshot",
	}
	for index, want := range wants {
		t.Run(want, func(t *testing.T) {
			invalid := append([]string(nil), paths...)
			invalid[index] = filepath.Join(directory, "missing.json")
			_, err := authorizeSchemaFlagMigrations(invalid[0], invalid[1], invalid[2], invalid[3], invalid[4])
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("authorizeSchemaFlagMigrations() error = %v, want %q", err, want)
			}
		})
	}
}

func TestCrossPlatformCoverageSchemaConsumedReceiptIsNoOpForAfterBaseline(t *testing.T) {
	directory := t.TempDir()
	approvedPath := filepath.Join(directory, "approved.json")
	candidatePath := filepath.Join(directory, "candidate.json")
	currentSnapshotPath := filepath.Join(directory, "current.json")
	baseSnapshotPath := filepath.Join(directory, "base.json")
	stableSnapshotPath := filepath.Join(directory, "stable.json")
	writeFlagMigrationManifestFile(t, approvedPath, schemaFlagMigrationManifest(interfacesnapshot.FlagMigrationConsumed))
	writeFlagMigrationManifestFile(t, candidatePath, schemaFlagMigrationManifest(interfacesnapshot.FlagMigrationConsumed))
	writeInterfaceSnapshotFile(t, currentSnapshotPath, schemaFlagMigrationSnapshot(true))
	writeInterfaceSnapshotFile(t, baseSnapshotPath, schemaFlagMigrationSnapshot(true))
	writeInterfaceSnapshotFile(t, stableSnapshotPath, schemaFlagMigrationSnapshot(false))

	migrations, err := authorizeSchemaFlagMigrations(approvedPath, candidatePath, currentSnapshotPath, baseSnapshotPath, stableSnapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	baseline := schemaFlagMigrationContract(true)
	normalized, err := normalizeSchemaFlagMigrations(baseline, schemaFlagMigrationContract(true), migrations)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(normalized, baseline) {
		t.Fatalf("consumed receipt changed an already-after Schema baseline: %#v", normalized)
	}
	if failures := checkCompatibility(normalized, schemaFlagMigrationContract(true)); len(failures) != 0 {
		t.Fatalf("consumed receipt did not preserve compatibility: %v", failures)
	}

	// Once stable also reaches after, retaining or deleting the consumed receipt
	// yields no authorization and the already-after Schema passes.
	writeInterfaceSnapshotFile(t, stableSnapshotPath, schemaFlagMigrationSnapshot(true))
	migrations, err = authorizeSchemaFlagMigrations(approvedPath, candidatePath, currentSnapshotPath, baseSnapshotPath, stableSnapshotPath)
	if err != nil || len(migrations) != 0 {
		t.Fatalf("retained consumed receipt authorizations = %#v, %v", migrations, err)
	}
	emptyManifest := interfacesnapshot.FlagMigrationManifest{
		Version:    interfacesnapshot.FlagMigrationManifestVersion,
		Migrations: []interfacesnapshot.FlagMigration{},
	}
	writeFlagMigrationManifestFile(t, candidatePath, emptyManifest)
	migrations, err = authorizeSchemaFlagMigrations(approvedPath, candidatePath, currentSnapshotPath, baseSnapshotPath, stableSnapshotPath)
	if err != nil || len(migrations) != 0 {
		t.Fatalf("cleaned consumed receipt authorizations = %#v, %v", migrations, err)
	}

	baselinePath := filepath.Join(directory, "schema-baseline.json")
	currentSchemaPath := filepath.Join(directory, "schema-current.json")
	writeSchemaContractFile(t, baselinePath, baseline)
	writeRawSchemaContractFile(t, currentSchemaPath, schemaFlagMigrationContract(true))
	args := []string{
		"--check", baselinePath,
		"--current", currentSchemaPath,
		"--approved-flag-migrations", approvedPath,
		"--candidate-flag-migrations", candidatePath,
		"--migration-current-snapshot", currentSnapshotPath,
		"--migration-base-snapshot", baseSnapshotPath,
		"--migration-stable-snapshot", stableSnapshotPath,
	}
	var stdout, stderr bytes.Buffer
	if code := run(args, &stdout, &stderr); code != 0 {
		t.Fatalf("cleaned consumed receipt check code=%d stderr=%q", code, stderr.String())
	}
}

func schemaFlagMigrationContract(after bool) schemaContract {
	legacyMessage := parameterSchema{
		Type:             `"string"`,
		Property:         "openMessageId",
		InterfaceType:    "string",
		Required:         true,
		CLIRequired:      true,
		Default:          `"default-message"`,
		InterfaceDefault: `"default-message"`,
		Format:           "id",
		Enum:             []string{"a", "b"},
	}
	conversation := parameterSchema{
		Type:          `"string"`,
		Property:      "openConversationId",
		InterfaceType: "string",
	}
	parameters := map[string]parameterSchema{
		"unrelated": {Type: `"string"`, Property: "unrelated"},
	}
	constraints := `{"require_one_of":[["group","id"]]}`
	if after {
		parameters["conversation-id"] = conversation
		parameters["message-id"] = legacyMessage
		constraints = `{"require_one_of":[["conversation-id"]]}`
	} else {
		parameters["group"] = conversation
		parameters["id"] = conversation
		parameters["msg-id"] = legacyMessage
	}
	return schemaContract{Version: schemaContractVersion, Products: map[string]productSchema{
		"chat": {Tools: map[string]toolSchema{
			"chat.edit_message": {
				PrimaryCLIPath: "chat message edit",
				InterfaceMode:  "mcp",
				Availability:   "available",
				Parameters:     parameters,
				Constraints:    constraints,
				Effect:         "write",
				Risk:           "medium",
				Confirmation:   "not_required",
				Idempotency:    "unknown",
			},
		}},
	}}
}

func schemaFlagMigrationAuthorizations() []interfacesnapshot.FlagMigration {
	conversationBefore := interfacesnapshot.FlagMigrationState{
		Present: true,
		Type:    "string",
		Scope:   "local",
	}
	conversationAfter := conversationBefore
	messageBefore := interfacesnapshot.FlagMigrationState{
		Present:  true,
		Type:     "string",
		Required: true,
		Scope:    "local",
	}
	messageAfter := messageBefore
	return []interfacesnapshot.FlagMigration{
		{
			Command: "dws chat message edit",
			Legacy: interfacesnapshot.FlagMigrationSide{
				Name:   "group",
				Before: conversationBefore,
				After: interfacesnapshot.FlagMigrationState{
					Present: true, Type: "string", Hidden: true, Scope: "local", AliasOf: "conversation-id",
				},
			},
			Canonical: interfacesnapshot.FlagMigrationSide{
				Name:   "conversation-id",
				Before: interfacesnapshot.FlagMigrationState{},
				After:  conversationAfter,
			},
		},
		{
			Command: "dws chat message edit",
			Legacy: interfacesnapshot.FlagMigrationSide{
				Name:   "id",
				Before: conversationBefore,
				After: interfacesnapshot.FlagMigrationState{
					Present: true, Type: "string", Hidden: true, Scope: "local", AliasOf: "conversation-id",
				},
			},
			Canonical: interfacesnapshot.FlagMigrationSide{
				Name:   "conversation-id",
				Before: interfacesnapshot.FlagMigrationState{},
				After:  conversationAfter,
			},
		},
		{
			Command: "dws chat message edit",
			Legacy: interfacesnapshot.FlagMigrationSide{
				Name:   "msg-id",
				Before: messageBefore,
				After: interfacesnapshot.FlagMigrationState{
					Present: true, Type: "string", Hidden: true, Scope: "local", AliasOf: "message-id",
				},
			},
			Canonical: interfacesnapshot.FlagMigrationSide{
				Name:   "message-id",
				Before: interfacesnapshot.FlagMigrationState{},
				After:  messageAfter,
			},
		},
	}
}

func schemaFlagMigrationManifest(state string) interfacesnapshot.FlagMigrationManifest {
	migrations := schemaFlagMigrationAuthorizations()
	for index := range migrations {
		migrations[index].State = state
		migrations[index].Reason = "canonicalize historical public flag names"
	}
	return interfacesnapshot.FlagMigrationManifest{
		Version:    interfacesnapshot.FlagMigrationManifestVersion,
		Migrations: migrations,
	}
}

func schemaFlagMigrationSnapshot(after bool) interfacesnapshot.Snapshot {
	states := map[string]interfacesnapshot.FlagMigrationState{}
	for _, migration := range schemaFlagMigrationAuthorizations() {
		if after {
			states[migration.Legacy.Name] = migration.Legacy.After
			states[migration.Canonical.Name] = migration.Canonical.After
		} else {
			states[migration.Legacy.Name] = migration.Legacy.Before
			states[migration.Canonical.Name] = migration.Canonical.Before
		}
	}
	names := make([]string, 0, len(states))
	for name, state := range states {
		if state.Present {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	flags := make([]interfacesnapshot.Flag, 0, len(names))
	for _, name := range names {
		state := states[name]
		flags = append(flags, interfacesnapshot.Flag{
			Name:      name,
			Type:      state.Type,
			Required:  state.Required,
			Hidden:    state.Hidden,
			Shorthand: state.Shorthand,
			NoOpt:     state.NoOpt,
			AliasOf:   state.AliasOf,
		})
	}
	return interfacesnapshot.Snapshot{
		SchemaVersion: interfacesnapshot.SchemaVersion,
		Rules: interfacesnapshot.Rules{
			ExcludedCommandSubtrees: []string{},
			ExcludedFlags:           []string{},
		},
		Commands: []interfacesnapshot.Command{{
			Path:       "dws chat message edit",
			Runnable:   true,
			Aliases:    []string{},
			LocalFlags: flags,
		}},
	}
}

func writeFlagMigrationManifestFile(t *testing.T, path string, manifest interfacesnapshot.FlagMigrationManifest) {
	t.Helper()
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeInterfaceSnapshotFile(t *testing.T, path string, snapshot interfacesnapshot.Snapshot) {
	t.Helper()
	var encoded bytes.Buffer
	if err := interfacesnapshot.Write(&encoded, snapshot); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, encoded.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeSchemaContractFile(t *testing.T, path string, contract schemaContract) {
	t.Helper()
	var encoded bytes.Buffer
	if err := writeContract(&encoded, contract); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, encoded.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeRawSchemaContractFile(t *testing.T, path string, contract schemaContract) {
	t.Helper()
	products := make([]map[string]any, 0, len(contract.Products))
	for productID, product := range contract.Products {
		tools := make([]map[string]any, 0, len(product.Tools))
		for toolID, tool := range product.Tools {
			parameters := map[string]any{}
			for name, parameter := range tool.Parameters {
				raw := map[string]any{
					"type":             json.RawMessage(parameter.Type),
					"required":         parameter.Required,
					"cli_required":     parameter.CLIRequired,
					"property":         parameter.Property,
					"interface_type":   parameter.InterfaceType,
					"required_when":    parameter.RequiredWhen,
					"format":           parameter.Format,
					"enum":             parameter.Enum,
					"field_provenance": map[string]any{},
				}
				if parameter.Default != "" {
					raw["default"] = json.RawMessage(parameter.Default)
				}
				if parameter.InterfaceDefault != "" {
					raw["interface_default"] = json.RawMessage(parameter.InterfaceDefault)
				}
				parameters[name] = raw
			}
			rawTool := map[string]any{
				"canonical_path":   toolID,
				"primary_cli_path": tool.PrimaryCLIPath,
				"interface_mode":   tool.InterfaceMode,
				"availability":     tool.Availability,
				"parameters":       parameters,
				"effect":           tool.Effect,
				"risk":             tool.Risk,
				"confirmation":     tool.Confirmation,
				"idempotency":      tool.Idempotency,
				"field_provenance": map[string]any{},
			}
			if tool.InterfaceRef != "" {
				rawTool["interface_ref"] = json.RawMessage(tool.InterfaceRef)
			}
			if tool.Constraints != "" {
				rawTool["constraints"] = json.RawMessage(tool.Constraints)
			}
			if len(tool.Positionals) > 0 {
				rawTool["positionals"] = tool.Positionals
			}
			if tool.DryRun != "" {
				rawTool["dry_run"] = json.RawMessage(tool.DryRun)
			}
			tools = append(tools, rawTool)
		}
		products = append(products, map[string]any{"id": productID, "tools": tools})
	}
	payload := map[string]any{"kind": "schema", "products": products}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
