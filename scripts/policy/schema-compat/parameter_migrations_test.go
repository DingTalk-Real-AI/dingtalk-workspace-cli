// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/interfacesnapshot"
)

func enumMigrationFixture() (schemaContract, schemaContract, interfacesnapshot.SchemaParameterMigration) {
	before := baselineContract()
	tool := before.Products["doc"].Tools["doc.create"]
	p := tool.Parameters["format"]
	p.Type, p.Enum = `"string"`, []string{"local_agent", "open_code"}
	tool.Parameters["format"] = p
	after := cloneContract(before)
	setEnumFixture(&after, []string{"local_agent", "qwenwork"})
	m := interfacesnapshot.SchemaParameterMigration{
		Kind: "schema_enum_change", Command: "dws doc create",
		Schema: interfacesnapshot.SchemaParameterMigrationID{ProductID: "doc", ToolID: "doc.create", Parameter: "format"},
		Before: interfacesnapshot.SchemaEnumState{Enum: []string{"local_agent", "open_code"}},
		After:  interfacesnapshot.SchemaEnumState{Enum: []string{"local_agent", "qwenwork"}},
		State:  "pending", Reason: "Reviewed enum replacement, not general authorization.",
	}
	return before, after, m
}

func setEnumFixture(c *schemaContract, values []string) {
	tool := c.Products["doc"].Tools["doc.create"]
	p := tool.Parameters["format"]
	p.Enum = slices.Clone(values)
	tool.Parameters["format"] = p
}

func enumManifest(m ...interfacesnapshot.SchemaParameterMigration) interfacesnapshot.SchemaParameterMigrationManifest {
	return interfacesnapshot.SchemaParameterMigrationManifest{Version: 1, Migrations: m}
}

func TestCrossPlatformCoverageSchemaEnumMigrationLifecycle(t *testing.T) {
	before, after, pending := enumMigrationFixture()
	consumed := pending
	consumed.State = "consumed"
	empty := enumManifest()
	for _, test := range []struct {
		name                  string
		base, stable, current schemaContract
		approved, candidate   interfacesnapshot.SchemaParameterMigrationManifest
		wantErr               bool
	}{
		{"declaration", before, before, before, empty, enumManifest(pending), false},
		{"pending-unchanged", before, before, before, enumManifest(pending), enumManifest(pending), false},
		{"consume", before, before, after, enumManifest(pending), enumManifest(consumed), false},
		{"retain-before-stable", after, before, after, enumManifest(consumed), enumManifest(consumed), false},
		{"inert-after-stable", after, after, after, enumManifest(consumed), enumManifest(consumed), false},
		{"cleanup", after, after, after, enumManifest(consumed), empty, false},
		{"self-approve", before, before, after, empty, enumManifest(consumed), true},
		{"declare-and-change", before, before, after, empty, enumManifest(pending), true},
		{"pending-cannot-authorize", before, before, after, enumManifest(pending), enumManifest(pending), true},
		{"consume-before-product", before, before, before, enumManifest(pending), enumManifest(consumed), true},
		{"cannot-remove-pending", before, before, before, enumManifest(pending), empty, true},
		{"cannot-clean-before-stable", after, before, after, enumManifest(consumed), empty, true},
		{"cannot-clean-after-reversal", after, after, before, enumManifest(consumed), empty, true},
		{"cannot-revert-state", after, before, before, enumManifest(consumed), enumManifest(pending), true},
		{"cannot-consume-from-wrong-base", after, before, after, enumManifest(pending), enumManifest(consumed), true},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, history := range []schemaContract{test.base, test.stable} {
				got, err := normalizeSchemaParameterMigrations(history, test.base, test.stable, test.current, test.approved, test.candidate)
				if (err != nil) != test.wantErr {
					t.Fatalf("error=%v, wantErr=%v", err, test.wantErr)
				}
				if err == nil && len(checkCompatibility(got, test.current)) != 0 {
					t.Fatalf("authorized transition incompatible: %v", checkCompatibility(got, test.current))
				}
			}
		})
	}
	if !slices.Equal(before.Products["doc"].Tools["doc.create"].Parameters["format"].Enum, pending.Before.Enum) {
		t.Fatal("normalization mutated original historical contract")
	}
	normalized, err := normalizeSchemaParameterMigrations(before, before, before, after, empty, empty)
	if err != nil || !strings.Contains(strings.Join(checkCompatibility(normalized, after), "\n"), "narrowed enum") {
		t.Fatalf("unregistered enum change must remain blocked: %v", err)
	}
}

func TestCrossPlatformCoverageSchemaEnumMigrationRejectsBundledDrift(t *testing.T) {
	before, after, pending := enumMigrationFixture()
	consumed := pending
	consumed.State = "consumed"
	for _, test := range []struct {
		name   string
		change func(*parameterSchema)
	}{
		{"type", func(p *parameterSchema) { p.Type = `"integer"` }},
		{"property", func(p *parameterSchema) { p.Property = "other" }},
		{"property-source", func(p *parameterSchema) { p.PropertySource = "other" }},
		{"interface-type", func(p *parameterSchema) { p.InterfaceType = "" }},
		{"required", func(p *parameterSchema) { p.Required = !p.Required }},
		{"cli-required", func(p *parameterSchema) { p.CLIRequired = !p.CLIRequired }},
		{"required-when", func(p *parameterSchema) { p.RequiredWhen = "x" }},
		{"default", func(p *parameterSchema) { p.Default = "" }},
		{"interface-default", func(p *parameterSchema) { p.InterfaceDefault = "x" }},
		{"format", func(p *parameterSchema) { p.Format = "date" }},
		{"any-of", func(p *parameterSchema) { p.AnyOf = "[]" }},
		{"extra-enum", func(p *parameterSchema) { p.Enum = []string{"qwenwork"} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			current := cloneContract(after)
			tool := current.Products["doc"].Tools["doc.create"]
			p := tool.Parameters["format"]
			test.change(&p)
			tool.Parameters["format"] = p
			if _, err := normalizeSchemaParameterMigrations(before, before, before, current, enumManifest(pending), enumManifest(consumed)); err == nil {
				t.Fatal("accepted non-exact migration")
			}
		})
	}
	for _, mutate := range []func(*interfacesnapshot.SchemaParameterMigration){
		func(m *interfacesnapshot.SchemaParameterMigration) { m.Reason = "Changed approved reason." },
		func(m *interfacesnapshot.SchemaParameterMigration) { m.Command = "dws doc other" },
		func(m *interfacesnapshot.SchemaParameterMigration) { m.Schema.ToolID = "doc.other" },
		func(m *interfacesnapshot.SchemaParameterMigration) { m.Schema.Parameter = "other" },
		func(m *interfacesnapshot.SchemaParameterMigration) { m.After.Enum = []string{"another"} },
	} {
		next := consumed
		mutate(&next)
		if _, err := normalizeSchemaParameterMigrations(before, before, before, after, enumManifest(pending), enumManifest(next)); err == nil {
			t.Fatal("accepted rewritten receipt")
		}
	}
	wrongPath := cloneContract(after)
	mutateTool(&wrongPath, func(tool *toolSchema) { tool.PrimaryCLIPath = "doc other" })
	if _, err := normalizeSchemaParameterMigrations(before, before, before, wrongPath, enumManifest(pending), enumManifest(consumed)); err == nil {
		t.Fatal("accepted wrong primary path")
	}
	wrongStable := cloneContract(before)
	setEnumFixture(&wrongStable, []string{"unknown"})
	if _, err := normalizeSchemaParameterMigrations(before, before, wrongStable, after, enumManifest(pending), enumManifest(consumed)); err == nil {
		t.Fatal("accepted unregistered stable enum")
	}
	if _, err := normalizeSchemaParameterMigrations(wrongStable, before, before, after, enumManifest(pending), enumManifest(consumed)); err == nil {
		t.Fatal("accepted unregistered historical enum")
	}
	wrongHistorical := cloneContract(before)
	mutateParameter(&wrongHistorical, func(p *parameterSchema) { p.Property = "changed" })
	if _, err := normalizeSchemaParameterMigrations(wrongHistorical, before, before, after, enumManifest(pending), enumManifest(consumed)); err == nil {
		t.Fatal("accepted historical non-enum drift")
	}
	drift := cloneContract(after)
	mutateTool(&drift, func(tool *toolSchema) { tool.Risk = "high" })
	got, err := normalizeSchemaParameterMigrations(before, before, before, drift, enumManifest(pending), enumManifest(consumed))
	if err != nil || !strings.Contains(strings.Join(checkCompatibility(got, drift), "\n"), "changed risk") {
		t.Fatalf("unrelated tool drift must remain visible: %v", err)
	}
	// [] before means an unrestricted string, not a wildcard enum matcher.
	unrestricted := cloneContract(before)
	setEnumFixture(&unrestricted, nil)
	pending.Before.Enum = []string{}
	consumed = pending
	consumed.State = "consumed"
	if _, err := normalizeSchemaParameterMigrations(unrestricted, unrestricted, unrestricted, after, enumManifest(pending), enumManifest(consumed)); err != nil {
		t.Fatalf("unrestricted string to exact enum: %v", err)
	}
}

func TestCrossPlatformCoverageSchemaEnumMigrationCLI(t *testing.T) {
	dir := t.TempDir()
	_, _, m := enumMigrationFixture()
	baseFile, rawFile, approvedFile, candidateFile := filepath.Join(dir, "base.json"), filepath.Join(dir, "raw.json"), filepath.Join(dir, "approved.json"), filepath.Join(dir, "candidate.json")
	raw := strings.Replace(completeSchemaJSON, `"type":["string","null"]`, `"type":"string"`, 1)
	raw = strings.Replace(raw, `["markdown","text"]`, `["local_agent","qwenwork"]`, 1)
	writeTestFile(t, rawFile, raw)
	parsed, err := normalizeRawFile(rawFile)
	if err != nil {
		t.Fatal(err)
	}
	before := cloneContract(parsed)
	setEnumFixture(&before, m.Before.Enum)
	var encoded bytes.Buffer
	if err := writeContract(&encoded, before); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, baseFile, encoded.String())
	writeManifest := func(path string, m interfacesnapshot.SchemaParameterMigrationManifest) {
		t.Helper()
		b, e := json.Marshal(m)
		if e != nil {
			t.Fatal(e)
		}
		writeTestFile(t, path, string(b))
	}
	writeManifest(approvedFile, enumManifest(m))
	m.State = "consumed"
	writeManifest(candidateFile, enumManifest(m))
	args := []string{"--check", baseFile, "--current", rawFile, "--approved-schema-parameter-migrations", approvedFile, "--candidate-schema-parameter-migrations", candidateFile, "--parameter-migration-base-schema", baseFile, "--parameter-migration-stable-schema", baseFile}
	var out, stderr bytes.Buffer
	if code := run(args, &out, &stderr); code != 0 {
		t.Fatalf("code=%d error=%s", code, stderr.String())
	}
	for index := 4; index < len(args); index += 2 {
		incomplete := slices.Concat(args[:index], args[index+2:])
		stderr.Reset()
		if code := run(incomplete, &out, &stderr); code != 2 {
			t.Fatalf("accepted incomplete pair %v", incomplete)
		}
	}
	for _, index := range []int{5, 9, 11} {
		bad := slices.Clone(args)
		bad[index] = filepath.Join(dir, "missing")
		if code := run(bad, &out, &stderr); code != 2 {
			t.Fatal("accepted missing approval or historical authority")
		}
	}
	m.State = "pending"
	writeManifest(candidateFile, enumManifest(m))
	if code := run(args, &out, &stderr); code != 2 {
		t.Fatal("accepted product change without consumed receipt")
	}
	writeTestFile(t, candidateFile, "{")
	if code := run(args, &out, &stderr); code != 2 {
		t.Fatal("accepted invalid candidate manifest")
	}
	if err := os.Remove(candidateFile); err != nil {
		t.Fatal(err)
	}
	if code := run(args, &out, &stderr); code != 2 {
		t.Fatal("accepted missing candidate manifest")
	}
	if _, err := readSchemaParameterMigrations(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("accepted missing file")
	}
}

func TestSchemaParameterBootstrapDeclarations(t *testing.T) {
	_, _, pending := enumMigrationFixture()
	rawBefore := strings.Replace(completeSchemaJSON, `"type":["string","null"]`, `"type":"string"`, 1)
	rawBefore = strings.Replace(rawBefore, `["markdown","text"]`, `["local_agent","open_code"]`, 1)
	for _, test := range []struct {
		name, state, raw, want string
	}{
		{"pending-before", "pending", rawBefore, ""},
		{"consumed-without-base-approval", "consumed", rawBefore, "must start pending"},
		{"pending-and-product-change", "pending", strings.Replace(rawBefore, "open_code", "qwenwork", 1), "must preserve the exact before contract"},
		{"unrelated-contract-change", "pending", strings.Replace(rawBefore, `"risk":"medium"`, `"risk":"high"`, 1), "changed risk"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			rawFile, currentFile := filepath.Join(dir, "before.json"), filepath.Join(dir, "current.json")
			writeTestFile(t, rawFile, rawBefore)
			before, err := normalizeRawFile(rawFile)
			if err != nil {
				t.Fatal(err)
			}
			var baseline bytes.Buffer
			if err := writeContract(&baseline, before); err != nil {
				t.Fatal(err)
			}
			baseFile, emptyFile, candidateFile := filepath.Join(dir, "base.json"), filepath.Join(dir, "empty.json"), filepath.Join(dir, "pending.json")
			writeTestFile(t, baseFile, baseline.String())
			writeTestFile(t, emptyFile, `{"version":1,"migrations":[]}`)
			m := pending
			m.State = test.state
			data, err := json.Marshal(enumManifest(m))
			if err != nil {
				t.Fatal(err)
			}
			writeTestFile(t, candidateFile, string(data))
			writeTestFile(t, currentFile, test.raw)
			var out, stderr bytes.Buffer
			code := run([]string{"--check", baseFile, "--current", currentFile,
				"--approved-schema-parameter-migrations", emptyFile,
				"--candidate-schema-parameter-migrations", candidateFile,
				"--parameter-migration-base-schema", baseFile,
				"--parameter-migration-stable-schema", baseFile}, &out, &stderr)
			if test.want == "" {
				if code != 0 {
					t.Fatalf("pending-before code=%d: %s", code, stderr.String())
				}
			} else if code == 0 || !strings.Contains(stderr.String(), test.want) {
				t.Fatalf("code=%d stderr=%s; want %s", code, stderr.String(), test.want)
			}
		})
	}
}
