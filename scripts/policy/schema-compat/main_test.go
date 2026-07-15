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

const exactDocInterfaceMigrationEntryJSON = `{
    "tool": "doc/doc.create",
    "old": {
      "interface_mode": "mcp",
      "interface_ref": {"product_id": "doc", "rpc_name": "create"}
    },
    "new": {
      "interface_mode": "composite",
      "interface_ref": null
    },
    "reason": "reviewed exact migration"
  }`

const exactDocInterfaceMigrationJSON = `{
  "version": 1,
  "migrations": [` + exactDocInterfaceMigrationEntryJSON + `]
}`

const exactDocOldConstraints = `{"require_one_of":[["title","format"]]}`

const exactDocNewConstraints = `{"mutually_exclusive":[["all","format"]],"require_one_of":[["title","format","all"]]}`

const exactDocContractMigrationEntryJSON = `{
    "tool":"doc/doc.create",
    "old":{"interface_mode":"mcp","interface_ref":{"product_id":"doc","rpc_name":"create"}},
    "new":{"interface_mode":"composite","interface_ref":null},
    "old_constraints":` + exactDocOldConstraints + `,
    "new_constraints":` + exactDocNewConstraints + `,
    "reason":"reviewed exact contract migration"
  }`

const exactDocContractMigrationJSON = `{
  "version":1,
  "migrations":[` + exactDocContractMigrationEntryJSON + `]
}`

const exactDocConstraintsOnlyMigrationJSON = `{
  "version":1,
  "migrations":[{
    "tool":"doc/doc.create",
    "old":{"interface_mode":"mcp","interface_ref":{"product_id":"doc","rpc_name":"create"}},
    "new":{"interface_mode":"mcp","interface_ref":{"product_id":"doc","rpc_name":"create"}},
    "old_constraints":` + exactDocOldConstraints + `,
    "new_constraints":` + exactDocNewConstraints + `,
    "reason":"constraints-only migration is unsupported"
  }]
}`

const exactPATOldConstraints = `{"require_one_of":[["scope","product","products","domain","domains","recommend"]]}`

const exactPATNewConstraints = `{"mutually_exclusive":[["all","scope"],["all","recommend"],["all","revoke"],["revoke","product"],["revoke","products"],["revoke","domain"],["revoke","domains"],["revoke","recommend"],["revoke","grant-type"],["revoke","session-id"]],"require_one_of":[["scope","product","products","domain","domains","recommend","all"]]}`

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
		{"--normalize", raw, "--approved-interface-migrations", raw},
		{"--check", baseline, "--current", raw, "--candidate-interface-migrations", raw},
		{"--merge", baseline, "--current", raw, "--approved-interface-migrations", raw},
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

func TestRunUsesExactApprovedInterfaceMigration(t *testing.T) {
	directory := t.TempDir()
	oldRaw := filepath.Join(directory, "old.json")
	currentRaw := filepath.Join(directory, "current.json")
	baselinePath := filepath.Join(directory, "baseline.json")
	migrationPath := filepath.Join(directory, "migrations.json")
	writeTestFile(t, oldRaw, completeSchemaWithInterface("mcp", `{"product_id":"doc","rpc_name":"create"}`))
	writeTestFile(t, currentRaw, completeSchemaWithInterface("composite", `null`))
	writeTestFile(t, migrationPath, exactDocInterfaceMigrationJSON)

	baseline, err := normalizeRawFile(oldRaw)
	if err != nil {
		t.Fatal(err)
	}
	var encoded bytes.Buffer
	if err := writeContract(&encoded, baseline); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, baselinePath, encoded.String())

	var stdout, stderr bytes.Buffer
	if code := run([]string{"--check", baselinePath, "--current", currentRaw}, &stdout, &stderr); code != 1 {
		t.Fatalf("unapproved transition code=%d stderr=%s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{
		"--check", baselinePath,
		"--current", currentRaw,
		"--approved-interface-migrations", migrationPath,
	}, &stdout, &stderr); code != 0 {
		t.Fatalf("approved transition code=%d stderr=%s", code, stderr.String())
	}
}

func TestRunEnforcesApprovedInterfaceMigrationLifecycle(t *testing.T) {
	directory := t.TempDir()
	oldRaw := filepath.Join(directory, "old.json")
	newRaw := filepath.Join(directory, "new.json")
	oldBaseline := filepath.Join(directory, "old-baseline.json")
	newBaseline := filepath.Join(directory, "new-baseline.json")
	baseManifest := filepath.Join(directory, "base-migrations.json")
	candidateManifest := filepath.Join(directory, "candidate-migrations.json")
	modifiedCandidateManifest := filepath.Join(directory, "modified-candidate-migrations.json")
	writeTestFile(t, oldRaw, completeSchemaWithInterface("mcp", `{"product_id":"doc","rpc_name":"create"}`))
	writeTestFile(t, newRaw, completeSchemaWithInterface("composite", `null`))
	writeNormalizedBaseline(t, oldRaw, oldBaseline)
	writeNormalizedBaseline(t, newRaw, newBaseline)
	writeTestFile(t, baseManifest, exactDocInterfaceMigrationJSON)
	writeTestFile(t, candidateManifest, exactDocInterfaceMigrationJSON)
	writeTestFile(
		t,
		modifiedCandidateManifest,
		strings.Replace(exactDocInterfaceMigrationJSON, "reviewed exact migration", "different review", 1),
	)

	assertRunCode := func(name string, wantCode int, wantError string, args ...string) {
		t.Helper()
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != wantCode {
			t.Fatalf("%s code=%d, want=%d stdout=%s stderr=%s", name, code, wantCode, stdout.String(), stderr.String())
		}
		if wantError != "" && !strings.Contains(stderr.String(), wantError) {
			t.Fatalf("%s stderr=%q, want %q", name, stderr.String(), wantError)
		}
	}

	assertRunCode(
		"pending approval retained",
		0,
		"",
		"--check", oldBaseline,
		"--current", oldRaw,
		"--approved-interface-migrations", baseManifest,
		"--candidate-interface-migrations", candidateManifest,
	)
	assertRunCode(
		"pending approval removed",
		2,
		"must retain pending",
		"--check", oldBaseline,
		"--current", oldRaw,
		"--approved-interface-migrations", baseManifest,
	)
	assertRunCode(
		"pending approval modified",
		2,
		"must retain pending interface migration \"doc/doc.create\" exactly",
		"--check", oldBaseline,
		"--current", oldRaw,
		"--approved-interface-migrations", baseManifest,
		"--candidate-interface-migrations", modifiedCandidateManifest,
	)
	assertRunCode(
		"migration consumes and removes approval",
		0,
		"",
		"--check", oldBaseline,
		"--current", newRaw,
		"--approved-interface-migrations", baseManifest,
	)
	assertRunCode(
		"migration retains consumed approval",
		2,
		"must remove consumed",
		"--check", oldBaseline,
		"--current", newRaw,
		"--approved-interface-migrations", baseManifest,
		"--candidate-interface-migrations", candidateManifest,
	)
	assertRunCode(
		"ordinary PR after clean migration",
		0,
		"",
		"--check", newBaseline,
		"--current", newRaw,
	)
	assertRunCode(
		"stale manifest recovery by removal",
		0,
		"",
		"--check", newBaseline,
		"--current", newRaw,
		"--approved-interface-migrations", baseManifest,
	)
	assertRunCode(
		"stale manifest retained",
		2,
		"must remove already-applied",
		"--check", newBaseline,
		"--current", newRaw,
		"--approved-interface-migrations", baseManifest,
		"--candidate-interface-migrations", candidateManifest,
	)
}

func TestRunWithoutApprovedManifestSupportsBootstrap(t *testing.T) {
	directory := t.TempDir()
	raw := filepath.Join(directory, "raw.json")
	baselinePath := filepath.Join(directory, "baseline.json")
	writeTestFile(t, raw, completeSchemaJSON)
	baseline, err := normalizeRawFile(raw)
	if err != nil {
		t.Fatal(err)
	}
	var encoded bytes.Buffer
	if err := writeContract(&encoded, baseline); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, baselinePath, encoded.String())

	var stdout, stderr bytes.Buffer
	if code := run([]string{"--check", baselinePath, "--current", raw}, &stdout, &stderr); code != 0 {
		t.Fatalf("bootstrap without base manifest code=%d stderr=%s", code, stderr.String())
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

func TestRequireOneOfMemberAdditionsAreCompatible(t *testing.T) {
	oldConstraints := `{
  "mutually_exclusive":[["legacy","modern"]],
  "require_one_of":[["scope","product"],["token","profile"]]
}`
	compatible := []struct {
		name string
		body string
	}{
		{
			name: "one group gains a member",
			body: `{"mutually_exclusive":[["legacy","modern"]],"require_one_of":[["scope","product","all"],["token","profile"]]}`,
		},
		{
			name: "multiple groups widen and reorder",
			body: `{"require_one_of":[["profile","session","token"],["all","product","scope"]],"mutually_exclusive":[["legacy","modern"]]}`,
		},
		{
			name: "member order only",
			body: `{"mutually_exclusive":[["legacy","modern"]],"require_one_of":[["product","scope"],["profile","token"]]}`,
		},
	}
	for _, test := range compatible {
		t.Run(test.name, func(t *testing.T) {
			if !constraintsCompatible(oldConstraints, test.body) {
				t.Fatalf("expected compatible require_one_of widening: %s", test.body)
			}
		})
	}

	baseline := baselineContract()
	mutateTool(&baseline, func(tool *toolSchema) { tool.Constraints = oldConstraints })
	current := cloneContract(baseline)
	mutateTool(&current, func(tool *toolSchema) {
		tool.Constraints = compatible[1].body
		tool.Parameters["all"] = parameterSchema{Type: `"boolean"`}
		tool.Parameters["session"] = parameterSchema{Type: `"string"`}
	})
	if failures := checkCompatibility(baseline, current); len(failures) != 0 {
		t.Fatalf("contract compatibility rejected require_one_of widening: %v", failures)
	}
}

func TestRequireOneOfNewMembersMustResolveToOptionalInputs(t *testing.T) {
	optional := cloneContract(baselineContract())
	mutateTool(&optional, func(tool *toolSchema) {
		tool.Constraints = `{"require_one_of":[["title","format","all"]]}`
		tool.Parameters["all"] = parameterSchema{Type: `"boolean"`}
	})
	if failures := checkCompatibility(baselineContract(), optional); len(failures) != 0 {
		t.Fatalf("optional all widening failed: %v", failures)
	}

	positional := cloneContract(baselineContract())
	mutateTool(&positional, func(tool *toolSchema) {
		tool.Constraints = `{"require_one_of":[["title","format","content"]]}`
	})
	if failures := checkCompatibility(baselineContract(), positional); len(failures) != 0 {
		t.Fatalf("historical positional widening failed: %v", failures)
	}

	tests := []struct {
		name      string
		parameter *parameterSchema
		want      string
	}{
		{name: "unknown member", want: "without a parameter or historical positional"},
		{name: "required", parameter: &parameterSchema{Type: `"boolean"`, Required: true}, want: "added required require_one_of"},
		{name: "cli required", parameter: &parameterSchema{Type: `"boolean"`, CLIRequired: true}, want: "added cli_required require_one_of"},
		{name: "required when", parameter: &parameterSchema{Type: `"boolean"`, RequiredWhen: "product=pat"}, want: "added conditional require_one_of"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			current := cloneContract(baselineContract())
			mutateTool(&current, func(tool *toolSchema) {
				tool.Constraints = `{"require_one_of":[["title","format","all"]]}`
				if test.parameter != nil {
					tool.Parameters["all"] = *test.parameter
				}
			})
			failures := strings.Join(checkCompatibility(baselineContract(), current), "\n")
			if !strings.Contains(failures, test.want) {
				t.Fatalf("failures=%q, want %q", failures, test.want)
			}
		})
	}
}

func TestRequireOneOfConstraintDriftFailsClosed(t *testing.T) {
	oldConstraints := `{"mutually_exclusive":[["legacy","modern"]],"require_one_of":[["scope","product"],["token","profile"]]}`
	tests := []struct {
		name string
		body string
	}{
		{name: "member removed", body: `{"mutually_exclusive":[["legacy","modern"]],"require_one_of":[["scope"],["token","profile"]]}`},
		{name: "member replaced", body: `{"mutually_exclusive":[["legacy","modern"]],"require_one_of":[["scope","all"],["token","profile"]]}`},
		{name: "group added", body: `{"mutually_exclusive":[["legacy","modern"]],"require_one_of":[["scope","product"],["token","profile"],["extra"]]}`},
		{name: "group removed", body: `{"mutually_exclusive":[["legacy","modern"]],"require_one_of":[["scope","product"]]}`},
		{name: "groups merged", body: `{"mutually_exclusive":[["legacy","modern"]],"require_one_of":[["scope","product","token","profile"]]}`},
		{name: "other constraint changed", body: `{"mutually_exclusive":[["legacy","modern","other"]],"require_one_of":[["scope","product","all"],["token","profile"]]}`},
		{name: "duplicate member", body: `{"mutually_exclusive":[["legacy","modern"]],"require_one_of":[["scope","product","scope"],["token","profile"]]}`},
		{name: "duplicate group", body: `{"mutually_exclusive":[["legacy","modern"]],"require_one_of":[["scope","product"],["scope","product"]]}`},
		{name: "empty group", body: `{"mutually_exclusive":[["legacy","modern"]],"require_one_of":[["scope","product"],[]]}`},
		{name: "member surrounding whitespace", body: `{"mutually_exclusive":[["legacy","modern"]],"require_one_of":[["scope","product"," all"],["token","profile"]]}`},
		{name: "require_one_of null", body: `{"mutually_exclusive":[["legacy","modern"]],"require_one_of":null}`},
		{name: "malformed require_one_of", body: `{"mutually_exclusive":[["legacy","modern"]],"require_one_of":["scope"]}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if constraintsCompatible(oldConstraints, test.body) {
				t.Fatalf("unexpectedly accepted constraint drift: %s", test.body)
			}
		})
	}
}

func TestCommittedApprovedInterfaceMigrationManifestIsValidWhenPresent(t *testing.T) {
	path := "approved-interface-migrations-v1.json"
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return
	} else if err != nil {
		t.Fatal(err)
	}
	migrations, err := readApprovedInterfaceMigrations(path)
	if err != nil {
		t.Fatal(err)
	}
	for tool, migration := range migrations {
		if migration.Tool != tool || migration.Old == migration.New || strings.TrimSpace(migration.Reason) == "" {
			t.Fatalf("invalid committed migration %q: %#v", tool, migration)
		}
	}
	patMigration, ok := migrations["pat/pat.batch_grant"]
	if !ok {
		t.Fatal("committed PAT contract migration is missing")
	}
	if !patMigration.HasConstraints ||
		patMigration.OldConstraints != exactPATOldConstraints ||
		patMigration.NewConstraints != exactPATNewConstraints {
		t.Fatalf("committed PAT constraint snapshots are not exact: %#v", patMigration)
	}
}

func TestApprovedInterfaceMigrationManifestFailsClosed(t *testing.T) {
	duplicate := `{"version":1,"migrations":[` + exactDocInterfaceMigrationEntryJSON + `,` + exactDocInterfaceMigrationEntryJSON + `]}`
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "unknown version", body: strings.Replace(exactDocInterfaceMigrationJSON, `"version": 1`, `"version": 2`, 1), want: "unsupported"},
		{name: "empty list", body: `{"version":1,"migrations":[]}`, want: "contains no migrations"},
		{name: "duplicate tool", body: duplicate, want: "duplicates tool"},
		{name: "wildcard tool", body: strings.Replace(exactDocInterfaceMigrationJSON, "doc/doc.create", "doc/*", 1), want: "unsupported token"},
		{name: "tool whitespace", body: strings.Replace(exactDocInterfaceMigrationJSON, "doc/doc.create", " doc/doc.create", 1), want: "surrounding whitespace"},
		{name: "empty reason", body: strings.Replace(exactDocInterfaceMigrationJSON, "reviewed exact migration", " ", 1), want: "no review reason"},
		{name: "unknown mode", body: strings.Replace(exactDocInterfaceMigrationJSON, `"interface_mode": "mcp"`, `"interface_mode": "remote"`, 1), want: "is not supported"},
		{name: "mcp null ref", body: strings.Replace(exactDocInterfaceMigrationJSON, `{"product_id": "doc", "rpc_name": "create"}`, `null`, 1), want: "exact product_id"},
		{name: "mcp ref whitespace", body: strings.Replace(exactDocInterfaceMigrationJSON, `"product_id": "doc"`, `"product_id": " doc"`, 1), want: "surrounding whitespace"},
		{name: "mcp ref extra field", body: strings.Replace(exactDocInterfaceMigrationJSON, `"rpc_name": "create"`, `"rpc_name": "create", "wildcard": true`, 1), want: "unknown field"},
		{name: "composite non-null ref", body: strings.Replace(exactDocInterfaceMigrationJSON, `"interface_ref": null`, `"interface_ref": {}`, 1), want: "explicit null"},
		{name: "no-op migration", body: strings.Replace(strings.Replace(exactDocInterfaceMigrationJSON, `"interface_mode": "composite"`, `"interface_mode": "mcp"`, 1), `"interface_ref": null`, `"interface_ref": {"product_id":"doc","rpc_name":"create"}`, 1), want: "does not change"},
		{name: "unknown top-level field", body: strings.Replace(exactDocInterfaceMigrationJSON, `"version": 1,`, `"version": 1, "allow_all": true,`, 1), want: "unknown field"},
		{name: "duplicate top-level key", body: strings.Replace(exactDocInterfaceMigrationJSON, `"version": 1,`, `"version": 1, "version": 1,`, 1), want: "duplicate JSON key"},
		{name: "case-folded duplicate top-level key", body: strings.Replace(exactDocInterfaceMigrationJSON, `"version": 1,`, `"version": 1, "Version": 1,`, 1), want: "differ only by case"},
		{name: "duplicate migration key", body: strings.Replace(exactDocInterfaceMigrationJSON, `"tool": "doc/doc.create",`, `"tool": "doc/doc.create", "tool": "doc/doc.create",`, 1), want: "duplicate JSON key"},
		{name: "case-folded duplicate migration key", body: strings.Replace(exactDocInterfaceMigrationJSON, `"tool": "doc/doc.create",`, `"tool": "doc/doc.create", "Tool": "doc/doc.create",`, 1), want: "differ only by case"},
		{name: "duplicate old endpoint key", body: strings.Replace(exactDocInterfaceMigrationJSON, `"interface_mode": "mcp",`, `"interface_mode": "mcp", "interface_mode": "mcp",`, 1), want: "duplicate JSON key"},
		{name: "duplicate new endpoint key", body: strings.Replace(exactDocInterfaceMigrationJSON, `"interface_mode": "composite",`, `"interface_mode": "composite", "interface_mode": "composite",`, 1), want: "duplicate JSON key"},
		{name: "duplicate mcp ref key", body: strings.Replace(exactDocInterfaceMigrationJSON, `"product_id": "doc",`, `"product_id": "doc", "product_id": "doc",`, 1), want: "duplicate JSON key"},
		{name: "non-canonical top-level key", body: strings.Replace(exactDocInterfaceMigrationJSON, `"version"`, `"Version"`, 1), want: "canonical case"},
		{name: "non-canonical migration key", body: strings.Replace(exactDocInterfaceMigrationJSON, `"tool"`, `"Tool"`, 1), want: "canonical case"},
		{name: "non-canonical endpoint key", body: strings.Replace(exactDocInterfaceMigrationJSON, `"interface_mode"`, `"Interface_Mode"`, 1), want: "canonical case"},
		{name: "non-canonical mcp ref key", body: strings.Replace(exactDocInterfaceMigrationJSON, `"product_id"`, `"Product_ID"`, 1), want: "canonical case"},
		{name: "constraints snapshot pair required", body: strings.Replace(exactDocContractMigrationJSON, `"new_constraints":`+exactDocNewConstraints+`,`, ``, 1), want: "provide old_constraints and new_constraints together"},
		{name: "constraints snapshot must be object", body: strings.Replace(exactDocContractMigrationJSON, `"old_constraints":`+exactDocOldConstraints, `"old_constraints":[]`, 1), want: "old_constraints must be a JSON object"},
		{name: "duplicate constraints snapshot key", body: strings.Replace(exactDocContractMigrationJSON, `"old_constraints":`, `"old_constraints":{},"old_constraints":`, 1), want: "duplicate JSON key"},
		{name: "case-folded duplicate constraints snapshot key", body: strings.Replace(exactDocContractMigrationJSON, `"old_constraints":`, `"old_constraints":{},"Old_Constraints":`, 1), want: "differ only by case"},
		{name: "non-canonical constraints snapshot key", body: strings.Replace(exactDocContractMigrationJSON, `"old_constraints"`, `"Old_Constraints"`, 1), want: "canonical case"},
		{name: "duplicate nested constraint key", body: strings.Replace(exactDocContractMigrationJSON, `"old_constraints":{"require_one_of":`, `"old_constraints":{"require_one_of":[["x"]],"require_one_of":`, 1), want: "duplicate JSON key"},
		{name: "case-folded duplicate nested constraint key", body: strings.Replace(exactDocContractMigrationJSON, `"old_constraints":{"require_one_of":`, `"old_constraints":{"require_one_of":[["x"]],"Require_One_Of":`, 1), want: "differ only by case"},
		{name: "invalid old constraints semantics", body: strings.Replace(exactDocContractMigrationJSON, `"old_constraints":`+exactDocOldConstraints, `"old_constraints":{"require_one_of":null}`, 1), want: "old_constraints is invalid"},
		{name: "invalid new constraints semantics", body: strings.Replace(exactDocContractMigrationJSON, `"new_constraints":`+exactDocNewConstraints, `"new_constraints":{"require_one_of":[[]]}`, 1), want: "new_constraints is invalid"},
		{name: "constraints-only migration", body: exactDocConstraintsOnlyMigrationJSON, want: "does not change the interface contract"},
		{name: "trailing json", body: exactDocInterfaceMigrationJSON + `{}`, want: "multiple JSON values"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "migrations.json")
			writeTestFile(t, path, test.body)
			_, err := readApprovedInterfaceMigrations(path)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error=%v, want %q", err, test.want)
			}
		})
	}
}

func TestApprovedInterfaceMigrationCompatibilityIsExact(t *testing.T) {
	baseline := baselineContract()
	mutateTool(&baseline, func(tool *toolSchema) {
		tool.InterfaceMode = "mcp"
		tool.InterfaceRef = `{"product_id":"doc","rpc_name":"create"}`
	})
	migration := normalizedInterfaceMigration{
		Tool: "doc/doc.create",
		Old:  interfaceState{Mode: "mcp", Ref: `{"product_id":"doc","rpc_name":"create"}`},
		New:  interfaceState{Mode: "composite", Ref: "null"},
	}
	migrations := map[string]normalizedInterfaceMigration{migration.Tool: migration}

	pending := cloneContract(baseline)
	if failures, err := checkCompatibilityWithMigrations(baseline, pending, migrations); err != nil || len(failures) != 0 {
		t.Fatalf("pending approval should not block unrelated PRs: failures=%v err=%v", failures, err)
	}

	current := cloneContract(baseline)
	mutateTool(&current, func(tool *toolSchema) {
		tool.InterfaceMode = "composite"
		tool.InterfaceRef = "null"
	})
	if failures, err := checkCompatibilityWithMigrations(baseline, current, migrations); err != nil || len(failures) != 0 {
		t.Fatalf("exact migration failures=%v err=%v", failures, err)
	}
	omittedNull := cloneContract(current)
	mutateTool(&omittedNull, func(tool *toolSchema) { tool.InterfaceRef = "" })
	if failures, err := checkCompatibilityWithMigrations(baseline, omittedNull, migrations); err != nil || len(failures) != 0 {
		t.Fatalf("omitted composite ref must normalize to null: failures=%v err=%v", failures, err)
	}

	changedAvailability := cloneContract(current)
	mutateTool(&changedAvailability, func(tool *toolSchema) { tool.Availability = "unavailable" })
	failures, err := checkCompatibilityWithMigrations(baseline, changedAvailability, migrations)
	if err != nil || !strings.Contains(strings.Join(failures, "\n"), "changed availability") {
		t.Fatalf("availability drift escaped approval: failures=%v err=%v", failures, err)
	}

	wrongTarget := cloneContract(current)
	mutateTool(&wrongTarget, func(tool *toolSchema) { tool.InterfaceRef = `{"product_id":"doc","rpc_name":"other"}` })
	failures, err = checkCompatibilityWithMigrations(baseline, wrongTarget, migrations)
	if err != nil || !strings.Contains(strings.Join(failures, "\n"), "changed interface_ref") {
		t.Fatalf("non-exact target escaped approval: failures=%v err=%v", failures, err)
	}

	unknown := map[string]normalizedInterfaceMigration{
		"missing/missing.tool": {
			Tool: "missing/missing.tool",
			Old:  interfaceState{Mode: "mcp", Ref: `{"product_id":"doc","rpc_name":"create"}`},
			New:  interfaceState{Mode: "composite", Ref: "null"},
		},
	}
	if _, err := checkCompatibilityWithMigrations(baseline, current, unknown); err == nil || !strings.Contains(err.Error(), "unknown historical product") {
		t.Fatalf("unknown approval error=%v", err)
	}

	stale := migration
	stale.Old.Ref = `{"product_id":"doc","rpc_name":"stale"}`
	if _, err := checkCompatibilityWithMigrations(baseline, current, map[string]normalizedInterfaceMigration{stale.Tool: stale}); err == nil || !strings.Contains(err.Error(), "is stale") {
		t.Fatalf("stale approval error=%v", err)
	}
}

func TestApprovedConstraintMigrationIsExactAndPreservesNewMemberSafety(t *testing.T) {
	baseline := baselineContract()
	mutateTool(&baseline, func(tool *toolSchema) {
		tool.InterfaceMode = "mcp"
		tool.InterfaceRef = `{"product_id":"doc","rpc_name":"create"}`
	})
	migration := normalizedInterfaceMigration{
		Tool:           "doc/doc.create",
		Old:            interfaceState{Mode: "mcp", Ref: `{"product_id":"doc","rpc_name":"create"}`},
		New:            interfaceState{Mode: "composite", Ref: "null"},
		HasConstraints: true,
		OldConstraints: exactDocOldConstraints,
		NewConstraints: exactDocNewConstraints,
	}
	migrations := map[string]normalizedInterfaceMigration{migration.Tool: migration}

	current := cloneContract(baseline)
	mutateTool(&current, func(tool *toolSchema) {
		tool.InterfaceMode = "composite"
		tool.InterfaceRef = "null"
		tool.Constraints = exactDocNewConstraints
		tool.Parameters["all"] = parameterSchema{Type: `"boolean"`}
	})
	if failures, err := checkCompatibilityWithMigrations(baseline, current, migrations); err != nil || len(failures) != 0 {
		t.Fatalf("exact contract migration failures=%v err=%v", failures, err)
	}

	arbitrary := cloneContract(current)
	mutateTool(&arbitrary, func(tool *toolSchema) {
		tool.Constraints = `{"mutually_exclusive":[["all","title"]],"require_one_of":[["title","format","all"]]}`
	})
	if failures, err := checkCompatibilityWithMigrations(baseline, arbitrary, migrations); err != nil || !strings.Contains(strings.Join(failures, "\n"), "changed constraints") {
		t.Fatalf("unapproved constraints escaped exact approval: failures=%v err=%v", failures, err)
	}

	regrouped := cloneContract(current)
	regroupedConstraints := `{"require_one_of":[["all"],["title","format"]]}`
	mutateTool(&regrouped, func(tool *toolSchema) {
		tool.Constraints = regroupedConstraints
		parameter := tool.Parameters["all"]
		parameter.Required = true
		tool.Parameters["all"] = parameter
	})
	regroupedMigration := migration
	regroupedMigration.NewConstraints = regroupedConstraints
	if failures, err := checkCompatibilityWithMigrations(
		baseline,
		regrouped,
		map[string]normalizedInterfaceMigration{regroupedMigration.Tool: regroupedMigration},
	); err != nil || !strings.Contains(strings.Join(failures, "\n"), `added required require_one_of parameter "all"`) {
		t.Fatalf("regrouped require_one_of escaped new-member safety: failures=%v err=%v", failures, err)
	}
}

func TestApprovedConstraintMigrationLifecycleRejectsPartialState(t *testing.T) {
	current := baselineContract()
	mutateTool(&current, func(tool *toolSchema) {
		tool.InterfaceMode = "mcp"
		tool.InterfaceRef = `{"product_id":"doc","rpc_name":"create"}`
	})
	migration := normalizedInterfaceMigration{
		Tool:           "doc/doc.create",
		Old:            interfaceState{Mode: "mcp", Ref: `{"product_id":"doc","rpc_name":"create"}`},
		New:            interfaceState{Mode: "composite", Ref: "null"},
		HasConstraints: true,
		OldConstraints: exactDocOldConstraints,
		NewConstraints: exactDocNewConstraints,
	}
	migrations := map[string]normalizedInterfaceMigration{migration.Tool: migration}

	mutateTool(&current, func(tool *toolSchema) {
		tool.Constraints = `{"require_one_of":[["title","format","all"]]}`
		tool.Parameters["all"] = parameterSchema{Type: `"boolean"`}
	})
	if err := validateCandidateInterfaceMigrationLifecycle(current, migrations, migrations, map[string]bool{}); err == nil || !strings.Contains(err.Error(), "matches neither exact old nor exact new") {
		t.Fatalf("partial approved contract state error=%v", err)
	}
}

func TestCandidateManifestMayAddFutureExactMigration(t *testing.T) {
	current := baselineContract()
	mutateTool(&current, func(tool *toolSchema) {
		tool.InterfaceMode = "mcp"
		tool.InterfaceRef = `{"product_id":"doc","rpc_name":"create"}`
	})
	product := current.Products["doc"]
	product.Tools["doc.read"] = toolSchema{
		PrimaryCLIPath: "doc read",
		InterfaceMode:  "mcp",
		InterfaceRef:   `{"product_id":"doc","rpc_name":"read"}`,
		Availability:   "available",
		Parameters:     map[string]parameterSchema{},
		Effect:         "read",
		Risk:           "low",
		Confirmation:   "not_required",
		Idempotency:    "idempotent",
	}
	current.Products["doc"] = product

	baseMigration := normalizedInterfaceMigration{
		Tool:   "doc/doc.create",
		Old:    interfaceState{Mode: "mcp", Ref: `{"product_id":"doc","rpc_name":"create"}`},
		New:    interfaceState{Mode: "composite", Ref: "null"},
		Reason: "existing approval",
	}
	futureMigration := normalizedInterfaceMigration{
		Tool:   "doc/doc.read",
		Old:    interfaceState{Mode: "mcp", Ref: `{"product_id":"doc","rpc_name":"read"}`},
		New:    interfaceState{Mode: "composite", Ref: "null"},
		Reason: "future approval",
	}
	base := map[string]normalizedInterfaceMigration{baseMigration.Tool: baseMigration}
	candidate := map[string]normalizedInterfaceMigration{
		baseMigration.Tool:   baseMigration,
		futureMigration.Tool: futureMigration,
	}
	if err := validateCandidateInterfaceMigrationLifecycle(current, base, candidate, map[string]bool{}); err != nil {
		t.Fatalf("future governance entry rejected: %v", err)
	}

	staleFuture := futureMigration
	staleFuture.Old.Ref = `{"product_id":"doc","rpc_name":"other"}`
	candidate[futureMigration.Tool] = staleFuture
	if err := validateCandidateInterfaceMigrationLifecycle(current, base, candidate, map[string]bool{}); err == nil || !strings.Contains(err.Error(), "current contract does not match old") {
		t.Fatalf("stale future governance entry error=%v", err)
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

func completeSchemaWithInterface(mode, ref string) string {
	result := strings.Replace(
		completeSchemaJSON,
		`"interface_mode":"local"`,
		`"interface_mode":"`+mode+`"`,
		1,
	)
	return strings.Replace(
		result,
		`"interface_ref":{"transport":"local"}`,
		`"interface_ref":`+ref,
		1,
	)
}

func writeTestFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeNormalizedBaseline(t *testing.T, rawPath, baselinePath string) {
	t.Helper()
	baseline, err := normalizeRawFile(rawPath)
	if err != nil {
		t.Fatal(err)
	}
	var encoded bytes.Buffer
	if err := writeContract(&encoded, baseline); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, baselinePath, encoded.String())
}
