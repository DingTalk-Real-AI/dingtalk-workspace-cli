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

func TestCrossPlatformCoverageSchemaAvailabilityMigrationNormalizesOnlyAvailability(t *testing.T) {
	baseline := baselineContract()
	current := cloneContract(baseline)
	mutateTool(&current, func(tool *toolSchema) { tool.Availability = "unavailable" })
	migration := interfacesnapshot.CommandMigration{
		Kind: interfacesnapshot.CommandMigrationAvailability,
		Legacy: interfacesnapshot.CommandMigrationSide{
			Command: "dws doc create",
			Before:  interfacesnapshot.CommandMigrationState{Present: true, Runnable: true},
			After:   interfacesnapshot.CommandMigrationState{Present: true, Runnable: true, Hidden: true},
		},
		Schema: interfacesnapshot.CommandMigrationSchema{
			ProductID:    "doc",
			SourceToolID: "doc.create",
			Parameters:   []interfacesnapshot.CommandParameterMigration{},
			Availability: &interfacesnapshot.CommandAvailabilityChange{Before: "available", After: "unavailable"},
		},
		State:  interfacesnapshot.CommandMigrationPending,
		Reason: "Reviewed fail-closed availability hardening.",
	}

	normalized, err := normalizeSchemaCommandMigrations(baseline, current, []interfacesnapshot.CommandMigration{migration})
	if err != nil {
		t.Fatal(err)
	}
	if got := normalized.Products["doc"].Tools["doc.create"].Availability; got != "unavailable" {
		t.Fatalf("normalized availability = %q", got)
	}
	if failures := checkCompatibility(normalized, current); len(failures) != 0 {
		t.Fatalf("availability hardening failures = %v", failures)
	}

	unrelated := cloneContract(current)
	mutateTool(&unrelated, func(tool *toolSchema) { tool.Risk = "high" })
	normalized, err = normalizeSchemaCommandMigrations(baseline, unrelated, []interfacesnapshot.CommandMigration{migration})
	if err != nil {
		t.Fatal(err)
	}
	if failures := strings.Join(checkCompatibility(normalized, unrelated), "\n"); !strings.Contains(failures, "changed risk") {
		t.Fatalf("unrelated risk drift was hidden: %q", failures)
	}

	alreadyAfter := cloneContract(current)
	if _, err := normalizeSchemaCommandMigrations(alreadyAfter, current, []interfacesnapshot.CommandMigration{migration}); err != nil {
		t.Fatalf("consumed merge-base availability should remain inert: %v", err)
	}
	wrongPath := cloneContract(current)
	mutateTool(&wrongPath, func(tool *toolSchema) { tool.PrimaryCLIPath = "doc other" })
	normalized, err = normalizeSchemaCommandMigrations(baseline, wrongPath, []interfacesnapshot.CommandMigration{migration})
	if err != nil {
		t.Fatal(err)
	}
	if got := normalized.Products["doc"].Tools["doc.create"].Availability; got != "available" {
		t.Fatalf("wrong-path migration changed availability to %q", got)
	}
	wrong := cloneContract(current)
	mutateTool(&wrong, func(tool *toolSchema) { tool.Availability = "available" })
	if _, err := normalizeSchemaCommandMigrations(baseline, wrong, []interfacesnapshot.CommandMigration{migration}); err == nil ||
		!strings.Contains(err.Error(), "does not match Schema availability") {
		t.Fatalf("wrong availability error = %v", err)
	}

	compatibilityVisible := migration
	compatibilityVisible.Legacy.After = compatibilityVisible.Legacy.Before
	if _, err := normalizeSchemaCommandMigrations(baseline, current, []interfacesnapshot.CommandMigration{compatibilityVisible}); err != nil {
		t.Fatalf("compatibility-visible availability hardening must authorize the exact Schema transition: %v", err)
	}
	if _, err := normalizeSchemaCommandMigrations(baseline, baseline, []interfacesnapshot.CommandMigration{compatibilityVisible}); err == nil ||
		!strings.Contains(err.Error(), "does not match Schema availability") {
		t.Fatalf("compatibility-visible receipt must not authorize unchanged Schema availability: %v", err)
	}
}

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

	unchangedLegacyConstraints := cloneContract(current)
	product = unchangedLegacyConstraints.Products["chat"]
	tool = product.Tools["chat.move"]
	tool.Constraints = baseline.Products["chat"].Tools["chat.move"].Constraints
	product.Tools["chat.move"] = tool
	unchangedLegacyConstraints.Products["chat"] = product
	if _, err := normalizeSchemaCommandMigrations(baseline, unchangedLegacyConstraints, migrations); err == nil ||
		!strings.Contains(err.Error(), "still reference legacy Schema constraint parameter") {
		t.Fatalf("unchanged legacy constraints error=%v", err)
	}

	mixedLegacyConstraints := cloneContract(current)
	product = mixedLegacyConstraints.Products["chat"]
	tool = product.Tools["chat.move"]
	tool.Constraints = `{"require_together":[["keep","new-id","old-id"]]}`
	product.Tools["chat.move"] = tool
	mixedLegacyConstraints.Products["chat"] = product
	if _, err := normalizeSchemaCommandMigrations(baseline, mixedLegacyConstraints, migrations); err == nil ||
		!strings.Contains(err.Error(), "still reference legacy Schema constraint parameter") {
		t.Fatalf("mixed legacy constraints error=%v", err)
	}

	malformedHistoricalConstraints := cloneContract(baseline)
	product = malformedHistoricalConstraints.Products["chat"]
	tool = product.Tools["chat.move"]
	tool.Constraints = "{"
	product.Tools["chat.move"] = tool
	malformedHistoricalConstraints.Products["chat"] = product
	if _, err := normalizeSchemaCommandMigrations(malformedHistoricalConstraints, current, migrations); err == nil ||
		!strings.Contains(err.Error(), "historical Schema constraints are not canonicalizable") {
		t.Fatalf("malformed historical constraints error=%v", err)
	}

	malformedCurrentConstraints := cloneContract(current)
	product = malformedCurrentConstraints.Products["chat"]
	tool = product.Tools["chat.move"]
	tool.Constraints = "{"
	product.Tools["chat.move"] = tool
	malformedCurrentConstraints.Products["chat"] = product
	if _, err := normalizeSchemaCommandMigrations(baseline, malformedCurrentConstraints, migrations); err == nil ||
		!strings.Contains(err.Error(), "current Schema constraints are not canonicalizable") {
		t.Fatalf("malformed current constraints error=%v", err)
	}
	if source, found := migratedConstraintSourceParameter("{", map[string]string{"legacy": "canonical"}); found || source != "" {
		t.Fatalf("malformed migrated constraint source = %q, %v", source, found)
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

func TestCrossPlatformCoverageSchemaCommandMigrationComposesHistoricalFlagLineage(t *testing.T) {
	directory := t.TempDir()
	stableContract, baseContract, currentContract := schemaCommandLineageContracts()
	baselinePath := filepath.Join(directory, "stable-schema.json")
	baseSchemaPath := filepath.Join(directory, "base-schema.json")
	currentPath := filepath.Join(directory, "current-schema.json")
	approvedFlagPath := filepath.Join(directory, "approved-flags.json")
	candidateFlagPath := filepath.Join(directory, "candidate-flags.json")
	approvedCommandPath := filepath.Join(directory, "approved-commands.json")
	candidateCommandPath := filepath.Join(directory, "candidate-commands.json")
	currentSnapshotPath := filepath.Join(directory, "current-snapshot.json")
	baseSnapshotPath := filepath.Join(directory, "base-snapshot.json")
	stableSnapshotPath := filepath.Join(directory, "stable-snapshot.json")

	writeSchemaContractFile(t, baselinePath, stableContract)
	writeSchemaContractFile(t, baseSchemaPath, baseContract)
	writeRawSchemaContractFile(t, currentPath, currentContract)
	writeFlagMigrationManifestFile(t, approvedFlagPath, schemaCommandLineageFlagManifest())
	writeFlagMigrationManifestFile(t, candidateFlagPath, schemaCommandLineageFlagManifest())
	writeCommandMigrationManifestFile(t, approvedCommandPath, schemaCommandLineageManifest(interfacesnapshot.CommandMigrationPending))
	writeCommandMigrationManifestFile(t, candidateCommandPath, schemaCommandLineageManifest(interfacesnapshot.CommandMigrationConsumed))
	writeInterfaceSnapshotFile(t, currentSnapshotPath, schemaCommandLineageSnapshot(true, true))
	writeInterfaceSnapshotFile(t, baseSnapshotPath, schemaCommandLineageSnapshot(true, false))
	writeInterfaceSnapshotFile(t, stableSnapshotPath, schemaCommandLineageSnapshot(false, false))

	args := []string{
		"--check", baselinePath,
		"--current", currentPath,
		"--migration-base-schema", baseSchemaPath,
		"--approved-flag-migrations", approvedFlagPath,
		"--candidate-flag-migrations", candidateFlagPath,
		"--approved-command-migrations", approvedCommandPath,
		"--candidate-command-migrations", candidateCommandPath,
		"--migration-current-snapshot", currentSnapshotPath,
		"--migration-base-snapshot", baseSnapshotPath,
		"--migration-stable-snapshot", stableSnapshotPath,
	}
	var stdout, stderr strings.Builder
	if code := run(args, &stdout, &stderr); code != 0 {
		t.Fatalf("pending command lineage code=%d stderr=%s", code, stderr.String())
	}

	withoutBaseSchema := append([]string(nil), args[:4]...)
	withoutBaseSchema = append(withoutBaseSchema, args[6:]...)
	stderr.Reset()
	if code := run(withoutBaseSchema, &stdout, &stderr); code != 2 ||
		!strings.Contains(stderr.String(), "requires --migration-base-schema") {
		t.Fatalf("missing migration base Schema code=%d stderr=%q", code, stderr.String())
	}
	stderr.Reset()
	if code := run([]string{
		"--check", baselinePath,
		"--current", currentPath,
		"--migration-base-schema", baseSchemaPath,
	}, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "requires both flag and command") {
		t.Fatalf("orphan migration base Schema code=%d stderr=%q", code, stderr.String())
	}
	missingBaseSchema := append([]string(nil), args...)
	missingBaseSchema[5] = filepath.Join(directory, "missing-base-schema.json")
	stderr.Reset()
	if code := run(missingBaseSchema, &stdout, &stderr); code != 2 ||
		!strings.Contains(stderr.String(), "read migration merge-base Schema contract") {
		t.Fatalf("unreadable migration base Schema code=%d stderr=%q", code, stderr.String())
	}

	// After the command move is merged, the merge-base Schema is already at the
	// final name. The two consumed receipts must keep the stable lineage durable
	// until the stable release itself reaches the after state.
	writeSchemaContractFile(t, baseSchemaPath, currentContract)
	writeCommandMigrationManifestFile(t, approvedCommandPath, schemaCommandLineageManifest(interfacesnapshot.CommandMigrationConsumed))
	writeInterfaceSnapshotFile(t, baseSnapshotPath, schemaCommandLineageSnapshot(true, true))
	stdout.Reset()
	stderr.Reset()
	if code := run(args, &stdout, &stderr); code != 0 {
		t.Fatalf("consumed command lineage code=%d stderr=%s", code, stderr.String())
	}
}

func TestCrossPlatformCoverageSchemaCommandMigrationLineageFailsClosed(t *testing.T) {
	tests := []struct {
		name         string
		commandState string
		mutate       func(*schemaContract, *schemaContract, *schemaContract, *[]interfacesnapshot.FlagMigration, *[]interfacesnapshot.CommandMigration)
		want         string
	}{
		{
			name:         "pending merge-base missing intermediate",
			commandState: interfacesnapshot.CommandMigrationPending,
			mutate: func(_, base, _ *schemaContract, _ *[]interfacesnapshot.FlagMigration, _ *[]interfacesnapshot.CommandMigration) {
				mutateSchemaCommandMigrationTool(base, "chat.list_topic_replies", func(tool *toolSchema) {
					delete(tool.Parameters, "conversation-id")
				})
			},
			want: "merge-base Schema lacks intermediate parameter",
		},
		{
			name:         "merge-base missing source tool",
			commandState: interfacesnapshot.CommandMigrationPending,
			mutate: func(_, base, _ *schemaContract, _ *[]interfacesnapshot.FlagMigration, _ *[]interfacesnapshot.CommandMigration) {
				product := base.Products["chat"]
				delete(product.Tools, "chat.list_topic_replies")
				base.Products["chat"] = product
			},
			want: "merge-base Schema lacks source tool",
		},
		{
			name:         "pending merge-base parameter drift",
			commandState: interfacesnapshot.CommandMigrationPending,
			mutate: func(_, base, _ *schemaContract, _ *[]interfacesnapshot.FlagMigration, _ *[]interfacesnapshot.CommandMigration) {
				mutateSchemaCommandMigrationParameter(base, "chat.list_topic_replies", "conversation-id", func(parameter *parameterSchema) {
					parameter.Property = "differentProperty"
				})
			},
			want: "changed a non-migration field",
		},
		{
			name:         "pending merge-base wrong path",
			commandState: interfacesnapshot.CommandMigrationPending,
			mutate: func(_, base, _ *schemaContract, _ *[]interfacesnapshot.FlagMigration, _ *[]interfacesnapshot.CommandMigration) {
				mutateSchemaCommandMigrationTool(base, "chat.list_topic_replies", func(tool *toolSchema) {
					tool.PrimaryCLIPath = "chat unrelated"
				})
			},
			want: "merge-base Schema source tool has primary_cli_path",
		},
		{
			name:         "pending merge-base already publishes final",
			commandState: interfacesnapshot.CommandMigrationPending,
			mutate: func(_, base, _ *schemaContract, _ *[]interfacesnapshot.FlagMigration, _ *[]interfacesnapshot.CommandMigration) {
				mutateSchemaCommandMigrationTool(base, "chat.list_topic_replies", func(tool *toolSchema) {
					tool.Parameters["open-topic-id"] = tool.Parameters["conversation-id"]
				})
			},
			want: "already publishes final parameter",
		},
		{
			name:         "pending flag receipt",
			commandState: interfacesnapshot.CommandMigrationPending,
			mutate: func(_, _, _ *schemaContract, flags *[]interfacesnapshot.FlagMigration, _ *[]interfacesnapshot.CommandMigration) {
				(*flags)[0].State = interfacesnapshot.FlagMigrationPending
			},
			want: "requires a consumed flag migration receipt",
		},
		{
			name:         "flag receipt belongs to another command",
			commandState: interfacesnapshot.CommandMigrationPending,
			mutate: func(_, _, _ *schemaContract, flags *[]interfacesnapshot.FlagMigration, _ *[]interfacesnapshot.CommandMigration) {
				(*flags)[0].Command = "dws chat message other"
			},
			want: `historical Schema tool lacks parameter "conversation-id"`,
		},
		{
			name:         "current retains predecessor",
			commandState: interfacesnapshot.CommandMigrationPending,
			mutate: func(stable, _, current *schemaContract, _ *[]interfacesnapshot.FlagMigration, _ *[]interfacesnapshot.CommandMigration) {
				mutateSchemaCommandMigrationTool(current, "chat.list_topic_replies", func(tool *toolSchema) {
					tool.Parameters["group"] = stable.Products["chat"].Tools["chat.list_topic_replies"].Parameters["group"]
				})
			},
			want: "current Schema still publishes predecessor parameter",
		},
		{
			name:         "pending current constraints retain predecessor",
			commandState: interfacesnapshot.CommandMigrationPending,
			mutate: func(_, _, current *schemaContract, _ *[]interfacesnapshot.FlagMigration, _ *[]interfacesnapshot.CommandMigration) {
				mutateSchemaCommandMigrationTool(current, "chat.list_topic_replies", func(tool *toolSchema) {
					tool.Constraints = `{"require_together":[["group","open-conv-thread-id","open-topic-id"]]}`
				})
			},
			want: "current Schema constraints still reference predecessor parameter",
		},
		{
			name:         "consumed current constraints retain predecessor",
			commandState: interfacesnapshot.CommandMigrationConsumed,
			mutate: func(_, _, current *schemaContract, _ *[]interfacesnapshot.FlagMigration, _ *[]interfacesnapshot.CommandMigration) {
				mutateSchemaCommandMigrationTool(current, "chat.list_topic_replies", func(tool *toolSchema) {
					tool.Constraints = `{"require_together":[["group","open-conv-thread-id","open-topic-id"]]}`
				})
			},
			want: "current Schema constraints still reference predecessor parameter",
		},
		{
			name:         "current retains intermediate",
			commandState: interfacesnapshot.CommandMigrationPending,
			mutate: func(_, base, current *schemaContract, _ *[]interfacesnapshot.FlagMigration, _ *[]interfacesnapshot.CommandMigration) {
				mutateSchemaCommandMigrationTool(current, "chat.list_topic_replies", func(tool *toolSchema) {
					tool.Parameters["conversation-id"] = base.Products["chat"].Tools["chat.list_topic_replies"].Parameters["conversation-id"]
				})
			},
			want: `still publishes legacy Schema parameter "conversation-id"`,
		},
		{
			name:         "consumed merge-base final drift",
			commandState: interfacesnapshot.CommandMigrationConsumed,
			mutate: func(_, base, _ *schemaContract, _ *[]interfacesnapshot.FlagMigration, _ *[]interfacesnapshot.CommandMigration) {
				mutateSchemaCommandMigrationParameter(base, "chat.list_topic_replies", "open-topic-id", func(parameter *parameterSchema) {
					parameter.Required = false
				})
			},
			want: "changed a non-name field",
		},
		{
			name:         "consumed merge-base wrong path",
			commandState: interfacesnapshot.CommandMigrationConsumed,
			mutate: func(_, base, _ *schemaContract, _ *[]interfacesnapshot.FlagMigration, _ *[]interfacesnapshot.CommandMigration) {
				mutateSchemaCommandMigrationTool(base, "chat.list_topic_replies", func(tool *toolSchema) {
					tool.PrimaryCLIPath = "chat unrelated"
				})
			},
			want: "consumed command migration",
		},
		{
			name:         "consumed merge-base missing final",
			commandState: interfacesnapshot.CommandMigrationConsumed,
			mutate: func(_, base, _ *schemaContract, _ *[]interfacesnapshot.FlagMigration, _ *[]interfacesnapshot.CommandMigration) {
				mutateSchemaCommandMigrationTool(base, "chat.list_topic_replies", func(tool *toolSchema) {
					delete(tool.Parameters, "open-topic-id")
				})
			},
			want: "merge-base Schema lacks final parameter",
		},
		{
			name:         "consumed merge-base retains intermediate",
			commandState: interfacesnapshot.CommandMigrationConsumed,
			mutate: func(stable, base, _ *schemaContract, _ *[]interfacesnapshot.FlagMigration, _ *[]interfacesnapshot.CommandMigration) {
				mutateSchemaCommandMigrationTool(base, "chat.list_topic_replies", func(tool *toolSchema) {
					tool.Parameters["conversation-id"] = stable.Products["chat"].Tools["chat.list_topic_replies"].Parameters["group"]
				})
			},
			want: "still publishes intermediate parameter",
		},
		{
			name:         "pending constraints drift",
			commandState: interfacesnapshot.CommandMigrationPending,
			mutate: func(_, base, _ *schemaContract, _ *[]interfacesnapshot.FlagMigration, _ *[]interfacesnapshot.CommandMigration) {
				mutateSchemaCommandMigrationTool(base, "chat.list_topic_replies", func(tool *toolSchema) {
					tool.Constraints = `{"require_one_of":[["conversation-id"]]}`
				})
			},
			want: "changed merge-base Schema constraints",
		},
		{
			name:         "pending current keeps intermediate constraints",
			commandState: interfacesnapshot.CommandMigrationPending,
			mutate: func(_, base, current *schemaContract, _ *[]interfacesnapshot.FlagMigration, _ *[]interfacesnapshot.CommandMigration) {
				mutateSchemaCommandMigrationTool(current, "chat.list_topic_replies", func(tool *toolSchema) {
					tool.Constraints = base.Products["chat"].Tools["chat.list_topic_replies"].Constraints
				})
			},
			want: "still reference legacy Schema constraint parameter",
		},
		{
			name:         "merge-base retains predecessor",
			commandState: interfacesnapshot.CommandMigrationPending,
			mutate: func(stable, base, _ *schemaContract, _ *[]interfacesnapshot.FlagMigration, _ *[]interfacesnapshot.CommandMigration) {
				mutateSchemaCommandMigrationTool(base, "chat.list_topic_replies", func(tool *toolSchema) {
					tool.Parameters["group"] = stable.Products["chat"].Tools["chat.list_topic_replies"].Parameters["group"]
				})
			},
			want: "merge-base Schema still publishes predecessor parameter",
		},
		{
			name:         "historical constraints malformed",
			commandState: interfacesnapshot.CommandMigrationPending,
			mutate: func(stable, _, _ *schemaContract, _ *[]interfacesnapshot.FlagMigration, _ *[]interfacesnapshot.CommandMigration) {
				mutateSchemaCommandMigrationTool(stable, "chat.list_topic_replies", func(tool *toolSchema) {
					tool.Constraints = "not-json"
				})
			},
			want: "historical Schema constraints are not canonicalizable",
		},
		{
			name:         "merge-base constraints malformed",
			commandState: interfacesnapshot.CommandMigrationPending,
			mutate: func(_, base, _ *schemaContract, _ *[]interfacesnapshot.FlagMigration, _ *[]interfacesnapshot.CommandMigration) {
				mutateSchemaCommandMigrationTool(base, "chat.list_topic_replies", func(tool *toolSchema) {
					tool.Constraints = "not-json"
				})
			},
			want: "merge-base Schema constraints are not canonicalizable",
		},
		{
			name:         "consumed constraints drift",
			commandState: interfacesnapshot.CommandMigrationConsumed,
			mutate: func(_, base, _ *schemaContract, _ *[]interfacesnapshot.FlagMigration, _ *[]interfacesnapshot.CommandMigration) {
				mutateSchemaCommandMigrationTool(base, "chat.list_topic_replies", func(tool *toolSchema) {
					tool.Constraints = `{"require_one_of":[["open-topic-id"]]}`
				})
			},
			want: "changed merge-base Schema constraints",
		},
		{
			name:         "target collision",
			commandState: interfacesnapshot.CommandMigrationPending,
			mutate: func(stable, _, _ *schemaContract, _ *[]interfacesnapshot.FlagMigration, _ *[]interfacesnapshot.CommandMigration) {
				mutateSchemaCommandMigrationTool(stable, "chat.list_topic_replies", func(tool *toolSchema) {
					tool.Parameters["open-topic-id"] = tool.Parameters["group"]
				})
			},
			want: "target \"open-topic-id\" already exists",
		},
		{
			name:         "lineage cycle",
			commandState: interfacesnapshot.CommandMigrationPending,
			mutate: func(_, _, _ *schemaContract, _ *[]interfacesnapshot.FlagMigration, commands *[]interfacesnapshot.CommandMigration) {
				(*commands)[0].Schema.Parameters[0].To = "group"
			},
			want: "lineage cycle",
		},
		{
			name:         "unsupported command receipt state",
			commandState: "unknown",
			mutate: func(_, _, _ *schemaContract, _ *[]interfacesnapshot.FlagMigration, _ *[]interfacesnapshot.CommandMigration) {
			},
			want: "unsupported lineage state",
		},
		{
			name:         "duplicate historical path",
			commandState: interfacesnapshot.CommandMigrationPending,
			mutate: func(stable, _, _ *schemaContract, _ *[]interfacesnapshot.FlagMigration, _ *[]interfacesnapshot.CommandMigration) {
				product := stable.Products["chat"]
				product.Tools["chat.duplicate"] = product.Tools["chat.list_topic_replies"]
				stable.Products["chat"] = product
			},
			want: "matches 2 historical Schema tools",
		},
		{
			name:         "source tool fork",
			commandState: interfacesnapshot.CommandMigrationPending,
			mutate: func(_, _, _ *schemaContract, _ *[]interfacesnapshot.FlagMigration, commands *[]interfacesnapshot.CommandMigration) {
				fork := (*commands)[0]
				fork.Legacy.Command = "dws chat message fork"
				fork.Replacement.Command = "dws chat topic fork"
				*commands = append(*commands, fork)
			},
			want: "fork Schema source tool",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stable, baseBefore, current := schemaCommandLineageContracts()
			base := baseBefore
			if test.commandState == interfacesnapshot.CommandMigrationConsumed {
				base = cloneContract(current)
			}
			flags := append([]interfacesnapshot.FlagMigration(nil), schemaCommandLineageFlagManifest().Migrations...)
			commands := append([]interfacesnapshot.CommandMigration(nil), schemaCommandLineageManifest(test.commandState).Migrations...)
			test.mutate(&stable, &base, &current, &flags, &commands)
			if _, err := normalizeSchemaCommandMigrationLineage(stable, base, current, flags, commands); err == nil ||
				!strings.Contains(err.Error(), test.want) {
				t.Fatalf("lineage error=%v, want %q", err, test.want)
			}
		})
	}
}

func TestCrossPlatformCoverageSchemaCommandMigrationLineagePreservesOrdinaryChecks(t *testing.T) {
	stable, base, current := schemaCommandLineageContracts()
	mutateSchemaCommandMigrationTool(&current, "chat.list_topic_replies", func(tool *toolSchema) {
		tool.Positionals = []positionalSchema{{Name: "open-topic-id", Index: 0, Type: "string", Required: true}}
	})
	normalized, err := normalizeSchemaCommandMigrationLineage(
		stable,
		base,
		current,
		schemaCommandLineageFlagManifest().Migrations,
		schemaCommandLineageManifest(interfacesnapshot.CommandMigrationPending).Migrations,
	)
	if err != nil {
		t.Fatal(err)
	}
	if failures := strings.Join(checkCompatibility(normalized, current), "\n"); !strings.Contains(failures, "changed positionals") {
		t.Fatalf("lineage hid positional drift: %s", failures)
	}

	requiredness := interfacesnapshot.FlagMigration{
		Kind:    interfacesnapshot.FlagMigrationRequirednessChange,
		Command: "dws report cli-only",
		Flag: &interfacesnapshot.FlagMigrationSide{
			Name:   "recipient",
			Before: interfacesnapshot.FlagMigrationState{Present: true, Type: "string", Scope: "local"},
			After:  interfacesnapshot.FlagMigrationState{Present: true, Type: "string", Required: true, Scope: "local"},
		},
		State:  interfacesnapshot.FlagMigrationConsumed,
		Reason: "Exercise requiredness receipts beside command lineage without treating them as renames.",
	}
	flagsWithRequiredness := append([]interfacesnapshot.FlagMigration(nil), schemaCommandLineageFlagManifest().Migrations...)
	flagsWithRequiredness = append(flagsWithRequiredness, requiredness)
	if _, err := normalizeSchemaCommandMigrationLineage(
		stable,
		base,
		current,
		flagsWithRequiredness,
		schemaCommandLineageManifest(interfacesnapshot.CommandMigrationPending).Migrations,
	); err != nil {
		t.Fatalf("requiredness receipt interfered with command lineage: %v", err)
	}

	// Multiple historical names may converge only when every predecessor carries
	// the exact same contract and every receipt is already consumed.
	stable, base, current = schemaCommandLineageContracts()
	mutateSchemaCommandMigrationTool(&stable, "chat.list_topic_replies", func(tool *toolSchema) {
		tool.Parameters["id"] = tool.Parameters["group"]
	})
	flags := schemaCommandLineageFlagManifest().Migrations
	second := flags[0]
	second.Legacy.Name = "id"
	flags = append(flags, second)
	normalized, err = normalizeSchemaCommandMigrationLineage(
		stable,
		base,
		current,
		flags,
		schemaCommandLineageManifest(interfacesnapshot.CommandMigrationPending).Migrations,
	)
	if err != nil {
		t.Fatalf("equivalent predecessor aliases should pass: %v", err)
	}
	if failures := checkCompatibility(normalized, current); len(failures) != 0 {
		t.Fatalf("equivalent predecessor aliases remained incompatible: %v", failures)
	}

	mutateSchemaCommandMigrationParameter(&stable, "chat.list_topic_replies", "id", func(parameter *parameterSchema) {
		parameter.Property = "differentProperty"
	})
	if _, err := normalizeSchemaCommandMigrationLineage(
		stable,
		base,
		current,
		flags,
		schemaCommandLineageManifest(interfacesnapshot.CommandMigrationPending).Migrations,
	); err == nil || !strings.Contains(err.Error(), "changed a non-migration field") {
		t.Fatalf("drifted predecessor alias error=%v", err)
	}
}

func TestCrossPlatformCoverageSchemaCommandMigrationLineageDefensiveEdges(t *testing.T) {
	stable, base, current := schemaCommandLineageContracts()
	flags := schemaCommandLineageFlagManifest().Migrations
	commands := schemaCommandLineageManifest(interfacesnapshot.CommandMigrationPending).Migrations

	flagExtraction := schemaCommandMigrationAuthorizations()[1]
	if _, err := stageSchemaCommandMigrationPredecessors(stable, base, current, flags, []interfacesnapshot.CommandMigration{flagExtraction}); err != nil {
		t.Fatalf("non-move command migration should be ignored: %v", err)
	}

	missingTool := cloneContract(stable)
	product := missingTool.Products["chat"]
	delete(product.Tools, "chat.list_topic_replies")
	missingTool.Products["chat"] = product
	if _, err := stageSchemaCommandMigrationPredecessors(missingTool, base, current, flags, commands); err != nil {
		t.Fatalf("missing historical source should remain for the ordinary checker: %v", err)
	}
	if _, err := stageSchemaCommandMigrationPredecessors(current, current, current, flags, schemaCommandLineageManifest(interfacesnapshot.CommandMigrationConsumed).Migrations); err != nil {
		t.Fatalf("already-after historical source should be a no-op: %v", err)
	}
	wrongPath := cloneContract(stable)
	mutateSchemaCommandMigrationTool(&wrongPath, "chat.list_topic_replies", func(tool *toolSchema) {
		tool.PrimaryCLIPath = "chat unrelated"
	})
	if _, err := stageSchemaCommandMigrationPredecessors(wrongPath, base, current, flags, commands); err != nil {
		t.Fatalf("wrong historical path should remain for the command normalizer: %v", err)
	}

	bothNames := cloneContract(stable)
	mutateSchemaCommandMigrationTool(&bothNames, "chat.list_topic_replies", func(tool *toolSchema) {
		tool.Parameters["conversation-id"] = tool.Parameters["group"]
	})
	if _, err := stageSchemaCommandMigrationPredecessors(bothNames, base, current, flags, commands); err == nil ||
		!strings.Contains(err.Error(), "publishes both predecessor") {
		t.Fatalf("ambiguous predecessor error=%v", err)
	}

	duplicatePath := cloneContract(stable)
	product = duplicatePath.Products["chat"]
	product.Tools["chat.duplicate"] = product.Tools["chat.list_topic_replies"]
	duplicatePath.Products["chat"] = product
	if _, err := stageSchemaCommandMigrationPredecessors(duplicatePath, base, current, flags, commands); err == nil ||
		!strings.Contains(err.Error(), "requires one exact historical Schema tool") {
		t.Fatalf("duplicate primary path error=%v", err)
	}

	forkStable, forkBase, forkCurrent := schemaCommandLineageContracts()
	mutateSchemaCommandMigrationTool(&forkBase, "chat.list_topic_replies", func(tool *toolSchema) {
		tool.Parameters["other-mid"] = tool.Parameters["conversation-id"]
	})
	mutateSchemaCommandMigrationTool(&forkCurrent, "chat.list_topic_replies", func(tool *toolSchema) {
		tool.Parameters["other-final"] = tool.Parameters["open-topic-id"]
	})
	forkFlags := append([]interfacesnapshot.FlagMigration(nil), flags...)
	forkFlag := flags[0]
	forkFlag.Canonical.Name = "other-mid"
	forkFlags = append(forkFlags, forkFlag)
	forkCommands := schemaCommandLineageManifest(interfacesnapshot.CommandMigrationPending).Migrations
	forkCommands[0].Schema.Parameters = append(forkCommands[0].Schema.Parameters,
		interfacesnapshot.CommandParameterMigration{From: "other-mid", To: "other-final"})
	if _, err := stageSchemaCommandMigrationPredecessors(forkStable, forkBase, forkCurrent, forkFlags, forkCommands); err == nil ||
		!strings.Contains(err.Error(), "forks Schema predecessor") {
		t.Fatalf("forked predecessor error=%v", err)
	}

	cycleStable, cycleBase, _ := schemaCommandLineageContracts()
	mutateSchemaCommandMigrationTool(&cycleStable, "chat.list_topic_replies", func(tool *toolSchema) {
		tool.Parameters["id"] = tool.Parameters["group"]
	})
	mutateSchemaCommandMigrationTool(&cycleBase, "chat.list_topic_replies", func(tool *toolSchema) {
		tool.Parameters["other-mid"] = tool.Parameters["conversation-id"]
	})
	cycleFlags := append([]interfacesnapshot.FlagMigration(nil), flags...)
	cycleFlag := flags[0]
	cycleFlag.Legacy.Name = "id"
	cycleFlag.Canonical.Name = "other-mid"
	cycleFlags = append(cycleFlags, cycleFlag)
	cycleCommands := schemaCommandLineageManifest(interfacesnapshot.CommandMigrationPending).Migrations
	cycleCommands[0].Schema.Parameters[0].To = "id"
	cycleCommands[0].Schema.Parameters = append(cycleCommands[0].Schema.Parameters,
		interfacesnapshot.CommandParameterMigration{From: "other-mid", To: "other-final"})
	missingCurrent := schemaContract{Version: schemaContractVersion, Products: map[string]productSchema{}}
	if _, err := stageSchemaCommandMigrationPredecessors(cycleStable, cycleBase, missingCurrent, cycleFlags, cycleCommands); err == nil ||
		!strings.Contains(err.Error(), "lineage cycle") {
		t.Fatalf("cross-lineage cycle error=%v", err)
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

func schemaCommandLineageContracts() (schemaContract, schemaContract, schemaContract) {
	conversation := parameterSchema{
		Type:          `"string"`,
		Property:      "openconversationId",
		InterfaceType: "string",
		Required:      true,
		CLIRequired:   true,
	}
	topic := parameterSchema{
		Type:          `"string"`,
		Property:      "openConversationThreadId",
		InterfaceType: "string",
		Required:      true,
		CLIRequired:   true,
	}
	tool := toolSchema{
		PrimaryCLIPath: "chat message list-topic-replies",
		InterfaceMode:  "mcp",
		InterfaceRef:   `{"product_id":"im","rpc_name":"list_topic_replies"}`,
		Availability:   "available",
		Parameters: map[string]parameterSchema{
			"group":    conversation,
			"topic-id": topic,
		},
		Constraints:  `{"require_together":[["group","topic-id"]]}`,
		Effect:       "read",
		Risk:         "low",
		Confirmation: "not_required",
		Idempotency:  "idempotent",
	}
	stable := schemaContract{Version: schemaContractVersion, Products: map[string]productSchema{
		"chat": {Tools: map[string]toolSchema{"chat.list_topic_replies": tool}},
	}}

	base := cloneContract(stable)
	mutateSchemaCommandMigrationTool(&base, "chat.list_topic_replies", func(tool *toolSchema) {
		delete(tool.Parameters, "group")
		tool.Parameters["conversation-id"] = conversation
		tool.Constraints = `{"require_together":[["conversation-id","topic-id"]]}`
	})

	current := cloneContract(base)
	mutateSchemaCommandMigrationTool(&current, "chat.list_topic_replies", func(tool *toolSchema) {
		delete(tool.Parameters, "conversation-id")
		delete(tool.Parameters, "topic-id")
		tool.Parameters["open-topic-id"] = conversation
		tool.Parameters["open-conv-thread-id"] = topic
		tool.PrimaryCLIPath = "chat topic list-replies"
		tool.Constraints = `{"require_together":[["open-conv-thread-id","open-topic-id"]]}`
	})
	return stable, base, current
}

func schemaCommandLineageFlagManifest() interfacesnapshot.FlagMigrationManifest {
	beforeCanonical := interfacesnapshot.FlagMigrationState{Present: true, Type: "string", Hidden: true, Scope: "local"}
	afterCanonical := interfacesnapshot.FlagMigrationState{Present: true, Type: "string", Required: true, Scope: "local"}
	return interfacesnapshot.FlagMigrationManifest{
		Version: interfacesnapshot.FlagMigrationManifestVersion,
		Migrations: []interfacesnapshot.FlagMigration{{
			Command: "dws chat message list-topic-replies",
			Legacy: interfacesnapshot.FlagMigrationSide{
				Name:   "group",
				Before: interfacesnapshot.FlagMigrationState{Present: true, Type: "string", Required: true, Scope: "local"},
				After:  interfacesnapshot.FlagMigrationState{Present: true, Type: "string", Hidden: true, Scope: "local", AliasOf: "conversation-id"},
			},
			Canonical: interfacesnapshot.FlagMigrationSide{
				Name:   "conversation-id",
				Before: beforeCanonical,
				After:  afterCanonical,
			},
			State:  interfacesnapshot.FlagMigrationConsumed,
			Reason: "preserve the reviewed group to conversation-id lineage",
		}},
	}
}

func schemaCommandLineageManifest(state string) interfacesnapshot.CommandMigrationManifest {
	return interfacesnapshot.CommandMigrationManifest{
		Version: interfacesnapshot.CommandMigrationManifestVersion,
		Migrations: []interfacesnapshot.CommandMigration{{
			Kind: interfacesnapshot.CommandMigrationMove,
			Legacy: interfacesnapshot.CommandMigrationSide{
				Command: "dws chat message list-topic-replies",
				Before:  interfacesnapshot.CommandMigrationState{Present: true, Runnable: true},
				After:   interfacesnapshot.CommandMigrationState{Present: true, Runnable: true, Hidden: true},
			},
			Replacement: interfacesnapshot.CommandMigrationSide{
				Command: "dws chat topic list-replies",
				Before:  interfacesnapshot.CommandMigrationState{},
				After:   interfacesnapshot.CommandMigrationState{Present: true, Runnable: true},
			},
			Schema: interfacesnapshot.CommandMigrationSchema{
				ProductID:         "chat",
				SourceToolID:      "chat.list_topic_replies",
				ReplacementToolID: "chat.list_topic_replies",
				Parameters: []interfacesnapshot.CommandParameterMigration{
					{From: "conversation-id", To: "open-topic-id"},
					{From: "topic-id", To: "open-conv-thread-id"},
				},
			},
			State:  state,
			Reason: "move topic reply listing while retaining the legacy command",
		}},
	}
}

func schemaCommandLineageSnapshot(flagAfter, commandAfter bool) interfacesnapshot.Snapshot {
	legacyFlags := []interfacesnapshot.Flag{
		{Name: "conversation-id", Type: "string", Hidden: true},
		{Name: "group", Type: "string", Required: true},
		{Name: "topic-id", Type: "string", Required: true},
	}
	if flagAfter {
		legacyFlags[0].Hidden = false
		legacyFlags[0].Required = true
		legacyFlags[1].Required = false
		legacyFlags[1].Hidden = true
		legacyFlags[1].AliasOf = "conversation-id"
	}
	commands := []interfacesnapshot.Command{
		{Path: "dws", Runnable: true, Aliases: []string{}, LocalFlags: []interfacesnapshot.Flag{}, InheritedFlags: []interfacesnapshot.Flag{}},
		{
			Path:           "dws chat message list-topic-replies",
			Runnable:       true,
			Aliases:        []string{},
			LocalFlags:     legacyFlags,
			InheritedFlags: []interfacesnapshot.Flag{},
		},
	}
	if commandAfter {
		commands[1].Hidden = true
		commands = append(commands, interfacesnapshot.Command{
			Path:           "dws chat topic list-replies",
			Runnable:       true,
			Aliases:        []string{},
			LocalFlags:     []interfacesnapshot.Flag{},
			InheritedFlags: []interfacesnapshot.Flag{},
		})
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
