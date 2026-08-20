// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/interfacesnapshot"
)

func TestCrossPlatformCoverageSchemaCommandMigrationsAuthorizeOnlyExactProjection(t *testing.T) {
	baseline := schemaCommandMigrationContract(false)
	current := schemaCommandMigrationContract(true)
	migrations := schemaCommandMigrationAuthorizations()
	normalized, err := normalizeSchemaCommandMigrations(baseline, current, migrations)
	if err != nil {
		t.Fatalf("normalize exact command migrations: %v", err)
	}
	if failures := checkCompatibility(normalized, current); len(failures) != 0 {
		t.Fatalf("exact command migrations remained incompatible: %v", failures)
	}

	unrelated := cloneContract(current)
	product := unrelated.Products["chat"]
	tool := product.Tools["chat.move"]
	delete(tool.Parameters, "keep")
	product.Tools["chat.move"] = tool
	unrelated.Products["chat"] = product
	normalized, err = normalizeSchemaCommandMigrations(baseline, unrelated, migrations)
	if err != nil {
		t.Fatal(err)
	}
	if failures := strings.Join(checkCompatibility(normalized, unrelated), "\n"); !strings.Contains(failures, `lost parameter "keep"`) {
		t.Fatalf("unrelated parameter removal was hidden: %s", failures)
	}
}

func TestCrossPlatformCoverageSchemaCommandMigrationsFailClosedOnDrift(t *testing.T) {
	baseline := schemaCommandMigrationContract(false)
	migrations := schemaCommandMigrationAuthorizations()

	parameterDrift := schemaCommandMigrationContract(true)
	product := parameterDrift.Products["chat"]
	tool := product.Tools["chat.move"]
	parameter := tool.Parameters["new-id"]
	parameter.Property = "differentProperty"
	tool.Parameters["new-id"] = parameter
	product.Tools["chat.move"] = tool
	parameterDrift.Products["chat"] = product
	if _, err := normalizeSchemaCommandMigrations(baseline, parameterDrift, migrations); err == nil || !strings.Contains(err.Error(), "changed a non-name field") {
		t.Fatalf("parameter drift error=%v", err)
	}

	safetyDrift := schemaCommandMigrationContract(true)
	product = safetyDrift.Products["chat"]
	replacement := product.Tools["chat.create_topic"]
	replacement.Risk = "high"
	product.Tools["chat.create_topic"] = replacement
	safetyDrift.Products["chat"] = product
	if _, err := normalizeSchemaCommandMigrations(baseline, safetyDrift, migrations); err == nil || !strings.Contains(err.Error(), "changed interface or safety identity") {
		t.Fatalf("extraction safety drift error=%v", err)
	}

	missingReplacement := schemaCommandMigrationContract(true)
	product = missingReplacement.Products["chat"]
	delete(product.Tools, "chat.create_topic")
	missingReplacement.Products["chat"] = product
	if _, err := normalizeSchemaCommandMigrations(baseline, missingReplacement, migrations); err == nil || !strings.Contains(err.Error(), "lacks replacement Schema tool") {
		t.Fatalf("missing replacement error=%v", err)
	}
}

func TestCrossPlatformCoverageSchemaCommandMigrationRejectsUnregisteredRequiredParameters(t *testing.T) {
	baseline := schemaCommandMigrationContract(false)
	migrations := schemaCommandMigrationAuthorizations()

	for _, test := range []struct {
		name      string
		parameter parameterSchema
	}{
		{name: "required", parameter: parameterSchema{Type: `"string"`, Property: "must", Required: true}},
		{name: "cli_required", parameter: parameterSchema{Type: `"string"`, Property: "must", CLIRequired: true}},
		{name: "required_when", parameter: parameterSchema{Type: `"string"`, Property: "must", RequiredWhen: "mode=forced"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			current := schemaCommandMigrationContract(true)
			product := current.Products["chat"]
			tool := product.Tools["chat.move"]
			tool.Parameters["must"] = test.parameter
			product.Tools["chat.move"] = tool
			current.Products["chat"] = product

			if _, err := normalizeSchemaCommandMigrations(baseline, current, migrations); err == nil ||
				!strings.Contains(err.Error(), `introduced unregistered required Schema parameter "must"`) {
				t.Fatalf("unregistered required parameter error=%v", err)
			}
		})
	}

	current := schemaCommandMigrationContract(true)
	product := current.Products["chat"]
	tool := product.Tools["chat.move"]
	tool.Parameters["optional"] = parameterSchema{Type: `"string"`, Property: "optional"}
	product.Tools["chat.move"] = tool
	current.Products["chat"] = product
	normalized, err := normalizeSchemaCommandMigrations(baseline, current, migrations)
	if err != nil {
		t.Fatalf("optional addition should remain compatible: %v", err)
	}
	if failures := checkCompatibility(normalized, current); len(failures) != 0 {
		t.Fatalf("optional addition should remain compatible: %v", failures)
	}
}

func TestCrossPlatformCoverageSchemaCommandMigrationRejectsParameterTargetCollision(t *testing.T) {
	baseline := schemaCommandMigrationContract(false)
	current := schemaCommandMigrationContract(true)
	migrations := schemaCommandMigrationAuthorizations()
	migrations[0].Schema.Parameters = []interfacesnapshot.CommandParameterMigration{{From: "old-id", To: "keep"}}

	product := current.Products["chat"]
	tool := product.Tools["chat.move"]
	delete(tool.Parameters, "new-id")
	tool.Parameters["keep"] = baseline.Products["chat"].Tools["chat.move"].Parameters["old-id"]
	tool.Constraints = baseline.Products["chat"].Tools["chat.move"].Constraints
	product.Tools["chat.move"] = tool
	current.Products["chat"] = product

	normalized, err := normalizeSchemaCommandMigrations(baseline, current, migrations)
	if err == nil {
		if failures := checkCompatibility(normalized, current); len(failures) != 0 {
			t.Fatalf("hostile target-collision fixture should otherwise pass compatibility: %v", failures)
		}
		t.Fatal("parameter target collision was accepted")
	}
	if !strings.Contains(err.Error(), `target "keep" already exists in historical Schema tool`) {
		t.Fatalf("parameter target collision error=%v", err)
	}
}

func TestCrossPlatformCoverageSchemaCommandMigrationNormalizationEdges(t *testing.T) {
	baseline := schemaCommandMigrationContract(false)
	current := schemaCommandMigrationContract(true)
	migrations := schemaCommandMigrationAuthorizations()

	missingHistorical := cloneContract(baseline)
	product := missingHistorical.Products["chat"]
	delete(product.Tools, "chat.move")
	missingHistorical.Products["chat"] = product
	if _, err := normalizeSchemaCommandMigrations(missingHistorical, current, migrations); err != nil {
		t.Fatalf("missing historical tool should be a no-op: %v", err)
	}

	missingCurrent := cloneContract(current)
	product = missingCurrent.Products["chat"]
	delete(product.Tools, "chat.move")
	missingCurrent.Products["chat"] = product
	if _, err := normalizeSchemaCommandMigrations(baseline, missingCurrent, migrations); err != nil {
		t.Fatalf("missing current source should remain for ordinary checker: %v", err)
	}

	alreadyAfter := schemaCommandMigrationContract(true)
	if _, err := normalizeSchemaCommandMigrations(alreadyAfter, current, migrations[:1]); err != nil {
		t.Fatalf("already-after baseline should be a no-op: %v", err)
	}

	wrongHistoricalPath := cloneContract(baseline)
	product = wrongHistoricalPath.Products["chat"]
	tool := product.Tools["chat.move"]
	tool.PrimaryCLIPath = "chat unrelated"
	product.Tools["chat.move"] = tool
	wrongHistoricalPath.Products["chat"] = product
	if _, err := normalizeSchemaCommandMigrations(wrongHistoricalPath, current, migrations); err == nil || !strings.Contains(err.Error(), "historical Schema tool") {
		t.Fatalf("wrong historical path error=%v", err)
	}

	wrongCurrentPath := cloneContract(current)
	product = wrongCurrentPath.Products["chat"]
	tool = product.Tools["chat.move"]
	tool.PrimaryCLIPath = "chat unrelated"
	product.Tools["chat.move"] = tool
	wrongCurrentPath.Products["chat"] = product
	if _, err := normalizeSchemaCommandMigrations(baseline, wrongCurrentPath, migrations); err != nil {
		t.Fatalf("wrong current path should remain for ordinary checker: %v", err)
	}

	missingHistoricalParameter := cloneContract(baseline)
	product = missingHistoricalParameter.Products["chat"]
	tool = product.Tools["chat.move"]
	delete(tool.Parameters, "old-id")
	product.Tools["chat.move"] = tool
	missingHistoricalParameter.Products["chat"] = product
	if _, err := normalizeSchemaCommandMigrations(missingHistoricalParameter, current, migrations); err == nil || !strings.Contains(err.Error(), "lacks parameter") {
		t.Fatalf("missing historical parameter error=%v", err)
	}

	legacyPublished := cloneContract(current)
	product = legacyPublished.Products["chat"]
	tool = product.Tools["chat.move"]
	tool.Parameters["old-id"] = baseline.Products["chat"].Tools["chat.move"].Parameters["old-id"]
	product.Tools["chat.move"] = tool
	legacyPublished.Products["chat"] = product
	if _, err := normalizeSchemaCommandMigrations(baseline, legacyPublished, migrations); err == nil || !strings.Contains(err.Error(), "still publishes legacy") {
		t.Fatalf("legacy parameter error=%v", err)
	}

	missingReplacementParameter := cloneContract(current)
	product = missingReplacementParameter.Products["chat"]
	tool = product.Tools["chat.move"]
	delete(tool.Parameters, "new-id")
	product.Tools["chat.move"] = tool
	missingReplacementParameter.Products["chat"] = product
	if _, err := normalizeSchemaCommandMigrations(baseline, missingReplacementParameter, migrations); err == nil || !strings.Contains(err.Error(), "does not publish replacement") {
		t.Fatalf("missing replacement parameter error=%v", err)
	}

	constraintDrift := cloneContract(current)
	product = constraintDrift.Products["chat"]
	tool = product.Tools["chat.move"]
	tool.Constraints = `{"require_one_of":[["new-id"]]}`
	product.Tools["chat.move"] = tool
	constraintDrift.Products["chat"] = product
	normalized, err := normalizeSchemaCommandMigrations(baseline, constraintDrift, migrations)
	if err != nil {
		t.Fatal(err)
	}
	if failures := strings.Join(checkCompatibility(normalized, constraintDrift), "\n"); !strings.Contains(failures, "changed constraints") {
		t.Fatalf("constraint drift was hidden: %s", failures)
	}

	extractionWrongSource := cloneContract(current)
	product = extractionWrongSource.Products["chat"]
	tool = product.Tools["chat.create_group"]
	tool.PrimaryCLIPath = "chat unrelated"
	product.Tools["chat.create_group"] = tool
	extractionWrongSource.Products["chat"] = product
	if _, err := normalizeSchemaCommandMigrations(baseline, extractionWrongSource, migrations[1:]); err != nil {
		t.Fatalf("wrong extraction source path should remain for ordinary checker: %v", err)
	}

	extractionHistoricalMissing := cloneContract(baseline)
	product = extractionHistoricalMissing.Products["chat"]
	tool = product.Tools["chat.create_group"]
	delete(tool.Parameters, "thread")
	product.Tools["chat.create_group"] = tool
	extractionHistoricalMissing.Products["chat"] = product
	if _, err := normalizeSchemaCommandMigrations(extractionHistoricalMissing, current, migrations[1:]); err == nil || !strings.Contains(err.Error(), "historical Schema tool lacks") {
		t.Fatalf("missing extracted historical parameter error=%v", err)
	}

	extractionStillPublished := cloneContract(current)
	product = extractionStillPublished.Products["chat"]
	tool = product.Tools["chat.create_group"]
	tool.Parameters["thread"] = baseline.Products["chat"].Tools["chat.create_group"].Parameters["thread"]
	product.Tools["chat.create_group"] = tool
	extractionStillPublished.Products["chat"] = product
	if _, err := normalizeSchemaCommandMigrations(baseline, extractionStillPublished, migrations[1:]); err == nil || !strings.Contains(err.Error(), "still publishes extracted") {
		t.Fatalf("still-published extraction error=%v", err)
	}
}

func TestCrossPlatformCoverageSchemaFlagExtractionRequiresCompleteReplacementProjection(t *testing.T) {
	t.Run("missing historical mapping", func(t *testing.T) {
		baseline := schemaCommandMigrationContract(false)
		current := schemaCommandMigrationContract(true)
		migrations := schemaCommandMigrationAuthorizations()
		parameters := make([]interfacesnapshot.CommandParameterMigration, 0, len(migrations[1].Schema.Parameters)-1)
		for _, parameter := range migrations[1].Schema.Parameters {
			if parameter.From != "users" {
				parameters = append(parameters, parameter)
			}
		}
		migrations[1].Schema.Parameters = parameters

		if _, err := normalizeSchemaCommandMigrations(baseline, current, migrations); err == nil ||
			!strings.Contains(err.Error(), `does not map historical Schema parameter "users"`) {
			t.Fatalf("missing historical mapping error=%v", err)
		}
	})

	for _, test := range []struct {
		name   string
		mutate func([]interfacesnapshot.CommandMigration)
		want   string
	}{
		{
			name: "duplicate historical mapping",
			mutate: func(migrations []interfacesnapshot.CommandMigration) {
				migrations[1].Schema.Parameters = append(
					migrations[1].Schema.Parameters,
					migrations[1].Schema.Parameters[0],
				)
			},
			want: "maps historical Schema parameter \"name\" more than once",
		},
		{
			name: "legacy mapping has two targets",
			mutate: func(migrations []interfacesnapshot.CommandMigration) {
				migrations[1].Schema.Parameters[3].To = "thread"
			},
			want: "must map to one replacement constant",
		},
		{
			name: "shared mapping has no target",
			mutate: func(migrations []interfacesnapshot.CommandMigration) {
				migrations[1].Schema.Parameters[0].To = ""
			},
			want: "must map to one replacement parameter",
		},
		{
			name: "duplicate replacement target",
			mutate: func(migrations []interfacesnapshot.CommandMigration) {
				migrations[1].Schema.Parameters[1].To = "name"
			},
			want: "is mapped from both \"name\" and \"type\"",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			baseline := schemaCommandMigrationContract(false)
			current := schemaCommandMigrationContract(true)
			migrations := schemaCommandMigrationAuthorizations()
			test.mutate(migrations)
			if _, err := normalizeSchemaCommandMigrations(baseline, current, migrations); err == nil ||
				!strings.Contains(err.Error(), test.want) {
				t.Fatalf("invalid mapping error=%v, want %q", err, test.want)
			}
		})
	}

	t.Run("replacement removes historical dry run", func(t *testing.T) {
		baseline := schemaCommandMigrationContract(false)
		mutateSchemaCommandMigrationTool(&baseline, "chat.create_group", func(tool *toolSchema) {
			tool.DryRun = `{"mode":"native"}`
		})
		current := schemaCommandMigrationContract(true)
		mutateSchemaCommandMigrationTool(&current, "chat.create_group", func(tool *toolSchema) {
			tool.DryRun = `{"mode":"native"}`
		})

		if _, err := normalizeSchemaCommandMigrations(baseline, current, schemaCommandMigrationAuthorizations()); err == nil ||
			!strings.Contains(err.Error(), "changed or removed dry_run") {
			t.Fatalf("replacement dry_run error=%v, want monotonic preservation rejection", err)
		}
	})

	t.Run("replacement adds dry run", func(t *testing.T) {
		baseline := schemaCommandMigrationContract(false)
		current := schemaCommandMigrationContract(true)
		mutateSchemaCommandMigrationTool(&current, "chat.create_topic", func(tool *toolSchema) {
			tool.DryRun = `{"mode":"native"}`
		})

		if _, err := normalizeSchemaCommandMigrations(baseline, current, schemaCommandMigrationAuthorizations()); err != nil {
			t.Fatalf("additive replacement dry_run was rejected: %v", err)
		}
	})

	for _, test := range []struct {
		name   string
		mutate func(*schemaContract)
		want   string
	}{
		{
			name: "missing shared parameter",
			mutate: func(current *schemaContract) {
				mutateSchemaCommandMigrationTool(current, "chat.create_topic", func(tool *toolSchema) {
					delete(tool.Parameters, "users")
				})
			},
			want: `does not publish mapped Schema parameter "users"`,
		},
		{
			name: "shared parameter field drift",
			mutate: func(current *schemaContract) {
				mutateSchemaCommandMigrationParameter(current, "chat.create_topic", "users", func(parameter *parameterSchema) {
					parameter.Property = "differentUserIds"
				})
			},
			want: "changed a non-name field",
		},
		{
			name: "extra replacement parameter",
			mutate: func(current *schemaContract) {
				mutateSchemaCommandMigrationTool(current, "chat.create_topic", func(tool *toolSchema) {
					tool.Parameters["extra"] = parameterSchema{Type: `"string"`, Property: "extra"}
				})
			},
			want: `publishes unmapped Schema parameter "extra"`,
		},
		{
			name: "constraints drift",
			mutate: func(current *schemaContract) {
				mutateSchemaCommandMigrationTool(current, "chat.create_topic", func(tool *toolSchema) {
					tool.Constraints = `{"require_one_of":[["name","type"]]}`
				})
			},
			want: "changed constraints",
		},
		{
			name: "positionals drift",
			mutate: func(current *schemaContract) {
				mutateSchemaCommandMigrationTool(current, "chat.create_topic", func(tool *toolSchema) {
					tool.Positionals[0].Name = "topic-name"
				})
			},
			want: "changed positionals",
		},
		{
			name: "positionals length drift",
			mutate: func(current *schemaContract) {
				mutateSchemaCommandMigrationTool(current, "chat.create_topic", func(tool *toolSchema) {
					tool.Positionals = append(tool.Positionals, positionalSchema{Name: "type", Index: 1, Type: "string"})
				})
			},
			want: "changed positionals",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			baseline := schemaCommandMigrationContract(false)
			current := schemaCommandMigrationContract(true)
			test.mutate(&current)
			if _, err := normalizeSchemaCommandMigrations(baseline, current, schemaCommandMigrationAuthorizations()); err == nil ||
				!strings.Contains(err.Error(), test.want) {
				t.Fatalf("replacement projection error=%v, want %q", err, test.want)
			}
		})
	}
}

func TestCrossPlatformCoverageSchemaFlagExtractionValidatesBooleanConstant(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*schemaContract, *schemaContract, []interfacesnapshot.CommandMigration)
		want   string
	}{
		{
			name: "constant property mismatch",
			mutate: func(_, _ *schemaContract, migrations []interfacesnapshot.CommandMigration) {
				migrations[1].Schema.Parameters[3].ReplacementConstant.Property = "differentProperty"
			},
			want: "constant property",
		},
		{
			name: "constant false",
			mutate: func(_, _ *schemaContract, migrations []interfacesnapshot.CommandMigration) {
				migrations[1].Schema.Parameters[3].ReplacementConstant.Value = false
			},
			want: "constant true",
		},
		{
			name: "legacy type",
			mutate: func(baseline, _ *schemaContract, _ []interfacesnapshot.CommandMigration) {
				mutateSchemaCommandMigrationParameter(baseline, "chat.create_group", "thread", func(parameter *parameterSchema) {
					parameter.Type = `"string"`
				})
			},
			want: "must have boolean type",
		},
		{
			name: "legacy required",
			mutate: func(baseline, _ *schemaContract, _ []interfacesnapshot.CommandMigration) {
				mutateSchemaCommandMigrationParameter(baseline, "chat.create_group", "thread", func(parameter *parameterSchema) {
					parameter.Required = true
				})
			},
			want: "must remain optional",
		},
		{
			name: "legacy cli required",
			mutate: func(baseline, _ *schemaContract, _ []interfacesnapshot.CommandMigration) {
				mutateSchemaCommandMigrationParameter(baseline, "chat.create_group", "thread", func(parameter *parameterSchema) {
					parameter.CLIRequired = true
				})
			},
			want: "must remain optional",
		},
		{
			name: "legacy required when",
			mutate: func(baseline, _ *schemaContract, _ []interfacesnapshot.CommandMigration) {
				mutateSchemaCommandMigrationParameter(baseline, "chat.create_group", "thread", func(parameter *parameterSchema) {
					parameter.RequiredWhen = "mode=topic"
				})
			},
			want: "must not declare required_when",
		},
		{
			name: "legacy default true",
			mutate: func(baseline, _ *schemaContract, _ []interfacesnapshot.CommandMigration) {
				mutateSchemaCommandMigrationParameter(baseline, "chat.create_group", "thread", func(parameter *parameterSchema) {
					parameter.Default = "true"
				})
			},
			want: "default must be absent or false",
		},
		{
			name: "legacy interface default true",
			mutate: func(baseline, _ *schemaContract, _ []interfacesnapshot.CommandMigration) {
				mutateSchemaCommandMigrationParameter(baseline, "chat.create_group", "thread", func(parameter *parameterSchema) {
					parameter.InterfaceDefault = "true"
				})
			},
			want: "interface_default must be absent or false",
		},
		{
			name: "legacy interface type",
			mutate: func(baseline, _ *schemaContract, _ []interfacesnapshot.CommandMigration) {
				mutateSchemaCommandMigrationParameter(baseline, "chat.create_group", "thread", func(parameter *parameterSchema) {
					parameter.InterfaceType = "string"
				})
			},
			want: "interface_type must be empty or boolean",
		},
		{
			name: "legacy enum excludes true",
			mutate: func(baseline, _ *schemaContract, _ []interfacesnapshot.CommandMigration) {
				mutateSchemaCommandMigrationParameter(baseline, "chat.create_group", "thread", func(parameter *parameterSchema) {
					parameter.Enum = []string{"false"}
				})
			},
			want: "enum must allow true",
		},
		{
			name: "legacy format",
			mutate: func(baseline, _ *schemaContract, _ []interfacesnapshot.CommandMigration) {
				mutateSchemaCommandMigrationParameter(baseline, "chat.create_group", "thread", func(parameter *parameterSchema) {
					parameter.Format = "custom-bool"
				})
			},
			want: "format must be empty",
		},
		{
			name: "replacement publishes constant property",
			mutate: func(_, current *schemaContract, _ []interfacesnapshot.CommandMigration) {
				mutateSchemaCommandMigrationTool(current, "chat.create_topic", func(tool *toolSchema) {
					tool.Parameters["thread-copy"] = parameterSchema{Type: `"boolean"`, Property: "threadEnabled"}
				})
			},
			want: "still publishes replacement constant property",
		},
		{
			name: "source renames constant property",
			mutate: func(_, current *schemaContract, _ []interfacesnapshot.CommandMigration) {
				mutateSchemaCommandMigrationTool(current, "chat.create_group", func(tool *toolSchema) {
					tool.Parameters["thread-copy"] = parameterSchema{Type: `"boolean"`, Property: "threadEnabled"}
				})
			},
			want: "source Schema tool still publishes replacement constant property",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			baseline := schemaCommandMigrationContract(false)
			current := schemaCommandMigrationContract(true)
			migrations := schemaCommandMigrationAuthorizations()
			test.mutate(&baseline, &current, migrations)
			if _, err := normalizeSchemaCommandMigrations(baseline, current, migrations); err == nil ||
				!strings.Contains(err.Error(), test.want) {
				t.Fatalf("replacement constant error=%v, want %q", err, test.want)
			}
		})
	}
}

func TestCrossPlatformCoverageSchemaCommandMigrationLifecycleAndRun(t *testing.T) {
	directory := t.TempDir()
	baselinePath := filepath.Join(directory, "baseline.json")
	currentPath := filepath.Join(directory, "current.json")
	approvedPath := filepath.Join(directory, "approved.json")
	candidatePath := filepath.Join(directory, "candidate.json")
	currentSnapshotPath := filepath.Join(directory, "current-snapshot.json")
	baseSnapshotPath := filepath.Join(directory, "base-snapshot.json")
	stableSnapshotPath := filepath.Join(directory, "stable-snapshot.json")

	writeSchemaContractFile(t, baselinePath, schemaCommandMigrationContract(false))
	writeRawSchemaContractFile(t, currentPath, schemaCommandMigrationContract(true))
	writeCommandMigrationManifestFile(t, approvedPath, schemaCommandMigrationManifest(interfacesnapshot.CommandMigrationPending))
	writeCommandMigrationManifestFile(t, candidatePath, schemaCommandMigrationManifest(interfacesnapshot.CommandMigrationConsumed))
	writeInterfaceSnapshotFile(t, currentSnapshotPath, schemaCommandMigrationSnapshot(true))
	writeInterfaceSnapshotFile(t, baseSnapshotPath, schemaCommandMigrationSnapshot(false))
	writeInterfaceSnapshotFile(t, stableSnapshotPath, schemaCommandMigrationSnapshot(false))

	args := []string{
		"--check", baselinePath,
		"--current", currentPath,
		"--approved-command-migrations", approvedPath,
		"--candidate-command-migrations", candidatePath,
		"--migration-current-snapshot", currentSnapshotPath,
		"--migration-base-snapshot", baseSnapshotPath,
		"--migration-stable-snapshot", stableSnapshotPath,
	}
	var stdout, stderr strings.Builder
	if code := run(args, &stdout, &stderr); code != 0 {
		t.Fatalf("command migration run code=%d stderr=%s", code, stderr.String())
	}

	paths := []string{approvedPath, candidatePath, currentSnapshotPath, baseSnapshotPath, stableSnapshotPath}
	for index := range paths {
		invalid := append([]string(nil), paths...)
		invalid[index] = filepath.Join(directory, "missing.json")
		if _, err := authorizeSchemaCommandMigrations(invalid[0], invalid[1], invalid[2], invalid[3], invalid[4]); err == nil {
			t.Fatalf("missing command migration input %d was accepted", index)
		}
	}

	stderr.Reset()
	if code := run([]string{"--check", baselinePath, "--current", currentPath, "--approved-command-migrations", approvedPath}, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "both command manifests") {
		t.Fatalf("partial command pair code=%d stderr=%q", code, stderr.String())
	}
	stderr.Reset()
	if code := run([]string{
		"--check", baselinePath,
		"--current", currentPath,
		"--approved-command-migrations", approvedPath,
		"--candidate-command-migrations", candidatePath,
	}, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "all three interface snapshots") {
		t.Fatalf("missing command snapshots code=%d stderr=%q", code, stderr.String())
	}
	stderr.Reset()
	if code := run([]string{"--check", baselinePath, "--current", currentPath, "--migration-current-snapshot", currentSnapshotPath}, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "require a flag or command") {
		t.Fatalf("orphan snapshot code=%d stderr=%q", code, stderr.String())
	}

	badArgs := append([]string(nil), args...)
	badArgs[5] = filepath.Join(directory, "missing-approved.json")
	stderr.Reset()
	if code := run(badArgs, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "authorize Schema command migrations") {
		t.Fatalf("command authorization error code=%d stderr=%q", code, stderr.String())
	}

	legacyCurrent := schemaCommandMigrationContract(true)
	product := legacyCurrent.Products["chat"]
	tool := product.Tools["chat.move"]
	tool.Parameters["old-id"] = schemaCommandMigrationContract(false).Products["chat"].Tools["chat.move"].Parameters["old-id"]
	product.Tools["chat.move"] = tool
	legacyCurrent.Products["chat"] = product
	writeRawSchemaContractFile(t, currentPath, legacyCurrent)
	stderr.Reset()
	if code := run(args, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "normalize approved Schema command migrations") {
		t.Fatalf("command normalization error code=%d stderr=%q", code, stderr.String())
	}

	requiredAddition := schemaCommandMigrationContract(true)
	product = requiredAddition.Products["chat"]
	tool = product.Tools["chat.move"]
	tool.Parameters["must"] = parameterSchema{Type: `"string"`, Property: "must", Required: true, CLIRequired: true}
	product.Tools["chat.move"] = tool
	requiredAddition.Products["chat"] = product
	writeRawSchemaContractFile(t, currentPath, requiredAddition)
	stderr.Reset()
	if code := run(args, &stdout, &stderr); code != 2 ||
		!strings.Contains(stderr.String(), `introduced unregistered required Schema parameter "must"`) {
		t.Fatalf("unregistered required addition code=%d stderr=%q", code, stderr.String())
	}
}

func schemaCommandMigrationContract(after bool) schemaContract {
	id := parameterSchema{Type: `"string"`, Property: "resourceId", Required: true, CLIRequired: true}
	keep := parameterSchema{Type: `"string"`, Property: "keep"}
	name := parameterSchema{Type: `"string"`, Property: "name"}
	groupType := parameterSchema{Type: `"string"`, Property: "type"}
	users := parameterSchema{Type: `"array"`, Property: "userIds"}
	thread := parameterSchema{
		Type:             `"boolean"`,
		Property:         "threadEnabled",
		InterfaceType:    "boolean",
		Default:          "false",
		InterfaceDefault: "false",
		Enum:             []string{"false", "true"},
	}
	group := toolSchema{
		PrimaryCLIPath: "chat group create",
		InterfaceMode:  "mcp",
		InterfaceRef:   `{"product_id":"im","rpc_name":"create_group"}`,
		Availability:   "available",
		Parameters: map[string]parameterSchema{
			"name":   name,
			"thread": thread,
			"type":   groupType,
			"users":  users,
		},
		Constraints:  `{"require_together":[["name","type"]]}`,
		Positionals:  []positionalSchema{{Name: "name", Index: 0, Type: "string", Required: true}},
		Effect:       "write",
		Risk:         "medium",
		Confirmation: "not_required",
		Idempotency:  "unknown",
	}
	move := toolSchema{
		PrimaryCLIPath: "chat message old",
		InterfaceMode:  "mcp",
		InterfaceRef:   `{"product_id":"chat","rpc_name":"move"}`,
		Availability:   "available",
		Parameters:     map[string]parameterSchema{"old-id": id, "keep": keep},
		Constraints:    `{"require_together":[["keep","old-id"]]}`,
		Effect:         "read",
		Risk:           "low",
		Confirmation:   "not_required",
		Idempotency:    "idempotent",
	}
	tools := map[string]toolSchema{"chat.create_group": group, "chat.move": move}
	if after {
		delete(group.Parameters, "thread")
		tools["chat.create_group"] = group
		replacement := group
		replacement.Parameters = make(map[string]parameterSchema, len(group.Parameters))
		for name, parameter := range group.Parameters {
			replacement.Parameters[name] = parameter
		}
		replacement.Positionals = append([]positionalSchema(nil), group.Positionals...)
		replacement.PrimaryCLIPath = "chat topic create"
		tools["chat.create_topic"] = replacement
		delete(move.Parameters, "old-id")
		move.Parameters["new-id"] = id
		move.PrimaryCLIPath = "chat topic new"
		move.Constraints = `{"require_together":[["keep","new-id"]]}`
		tools["chat.move"] = move
	}
	return schemaContract{Version: schemaContractVersion, Products: map[string]productSchema{"chat": {Tools: tools}}}
}

func schemaCommandMigrationAuthorizations() []interfacesnapshot.CommandMigration {
	return []interfacesnapshot.CommandMigration{
		{
			Kind: interfacesnapshot.CommandMigrationMove,
			Legacy: interfacesnapshot.CommandMigrationSide{
				Command: "dws chat message old",
				Before:  interfacesnapshot.CommandMigrationState{Present: true, Runnable: true},
				After:   interfacesnapshot.CommandMigrationState{Present: true, Runnable: true, Hidden: true},
			},
			Replacement: interfacesnapshot.CommandMigrationSide{
				Command: "dws chat topic new",
				After:   interfacesnapshot.CommandMigrationState{Present: true, Runnable: true},
			},
			Schema: interfacesnapshot.CommandMigrationSchema{
				ProductID:         "chat",
				SourceToolID:      "chat.move",
				ReplacementToolID: "chat.move",
				Parameters:        []interfacesnapshot.CommandParameterMigration{{From: "old-id", To: "new-id"}},
			},
		},
		{
			Kind: interfacesnapshot.CommandMigrationFlagExtraction,
			Legacy: interfacesnapshot.CommandMigrationSide{
				Command: "dws chat group create",
				Before:  interfacesnapshot.CommandMigrationState{Present: true, Runnable: true},
				After:   interfacesnapshot.CommandMigrationState{Present: true, Runnable: true},
			},
			Replacement: interfacesnapshot.CommandMigrationSide{
				Command: "dws chat topic create",
				After:   interfacesnapshot.CommandMigrationState{Present: true, Runnable: true},
			},
			LegacyFlag: interfacesnapshot.CommandMigrationFlag{
				Name:   "thread",
				Before: interfacesnapshot.FlagMigrationState{Present: true, Type: "bool", NoOpt: "true", Scope: "local"},
				After:  interfacesnapshot.FlagMigrationState{Present: true, Type: "bool", Hidden: true, NoOpt: "true", Scope: "local"},
			},
			Schema: interfacesnapshot.CommandMigrationSchema{
				ProductID:         "chat",
				SourceToolID:      "chat.create_group",
				ReplacementToolID: "chat.create_topic",
				Parameters: []interfacesnapshot.CommandParameterMigration{
					{From: "name", To: "name"},
					{From: "type", To: "type"},
					{From: "users", To: "users"},
					{
						From: "thread",
						ReplacementConstant: &interfacesnapshot.CommandReplacementConstant{
							Property: "threadEnabled",
							Value:    true,
						},
					},
				},
			},
		},
	}
}

func schemaCommandMigrationManifest(state string) interfacesnapshot.CommandMigrationManifest {
	migrations := schemaCommandMigrationAuthorizations()
	for index := range migrations {
		migrations[index].State = state
		migrations[index].Reason = "Reviewed Schema command migration."
	}
	return interfacesnapshot.CommandMigrationManifest{
		Version:    interfacesnapshot.CommandMigrationManifestVersion,
		Migrations: migrations,
	}
}

func schemaCommandMigrationSnapshot(after bool) interfacesnapshot.Snapshot {
	commands := []interfacesnapshot.Command{
		{Path: "dws", Runnable: true, Aliases: []string{}, LocalFlags: []interfacesnapshot.Flag{}, InheritedFlags: []interfacesnapshot.Flag{}},
		{
			Path:     "dws chat group create",
			Runnable: true,
			Aliases:  []string{},
			LocalFlags: []interfacesnapshot.Flag{{
				Name: "thread", Type: "bool", NoOpt: "true",
			}},
			InheritedFlags: []interfacesnapshot.Flag{},
		},
		{Path: "dws chat message old", Runnable: true, Aliases: []string{}, LocalFlags: []interfacesnapshot.Flag{}, InheritedFlags: []interfacesnapshot.Flag{}},
	}
	if after {
		commands[1].LocalFlags[0].Hidden = true
		commands[2].Hidden = true
		commands = append(commands,
			interfacesnapshot.Command{
				Path:            "dws chat topic create",
				Runnable:        true,
				Aliases:         []string{},
				LocalFlags:      []interfacesnapshot.Flag{},
				InheritedFlags:  []interfacesnapshot.Flag{},
				BoolConstParams: map[string]bool{"threadEnabled": true},
			},
			interfacesnapshot.Command{Path: "dws chat topic new", Runnable: true, Aliases: []string{}, LocalFlags: []interfacesnapshot.Flag{}, InheritedFlags: []interfacesnapshot.Flag{}},
		)
	}
	return interfacesnapshot.Snapshot{
		SchemaVersion: interfacesnapshot.SchemaVersion,
		Rules: interfacesnapshot.Rules{
			ExcludedCommandSubtrees: []string{"dws __complete", "dws __completeNoDesc", "dws completion", "dws help"},
			ExcludedFlags:           []string{"help"},
		},
		Commands: commands,
	}
}

func mutateSchemaCommandMigrationTool(contract *schemaContract, toolID string, mutate func(*toolSchema)) {
	product := contract.Products["chat"]
	tool := product.Tools[toolID]
	mutate(&tool)
	product.Tools[toolID] = tool
	contract.Products["chat"] = product
}

func mutateSchemaCommandMigrationParameter(
	contract *schemaContract,
	toolID string,
	parameterName string,
	mutate func(*parameterSchema),
) {
	mutateSchemaCommandMigrationTool(contract, toolID, func(tool *toolSchema) {
		parameter := tool.Parameters[parameterName]
		mutate(&parameter)
		tool.Parameters[parameterName] = parameter
	})
}

func writeCommandMigrationManifestFile(t *testing.T, path string, manifest interfacesnapshot.CommandMigrationManifest) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(file).Encode(manifest); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
