// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package main

import (
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/interfacesnapshot"
)

func readSchemaParameterMigrations(path string) (interfacesnapshot.SchemaParameterMigrationManifest, error) {
	f, err := os.Open(path)
	if err != nil {
		return interfacesnapshot.SchemaParameterMigrationManifest{}, err
	}
	defer f.Close()
	return interfacesnapshot.ReadSchemaParameterMigrationManifest(f)
}

// normalizeSchemaParameterMigrations changes only an approved enum in a copy
// of the historical contract. It never removes findings by matching error text.
// base/stable/current must be the original complete contracts, before any other
// migration adapter; otherwise a second receipt could conceal bundled drift.
func normalizeSchemaParameterMigrations(historical, base, stable, current schemaContract,
	approved, candidate interfacesnapshot.SchemaParameterMigrationManifest,
) (schemaContract, error) {
	result := cloneContract(historical)
	proposed := make(map[interfacesnapshot.SchemaParameterMigrationID]interfacesnapshot.SchemaParameterMigration)
	for _, m := range candidate.Migrations {
		proposed[m.Schema] = m
	}
	for _, m := range approved.Migrations {
		next, exists := proposed[m.Schema]
		if !exists {
			if m.State != interfacesnapshot.FlagMigrationConsumed {
				return result, fmt.Errorf("cannot remove pending Schema parameter migration %v", m.Schema)
			}
			// Cleanup is allowed only after BOTH historical authorities reach after.
			for _, contract := range []schemaContract{base, stable, current} {
				if _, err := schemaMigrationParameter(contract, m, m.After.Enum); err != nil {
					return result, fmt.Errorf("cannot clean up Schema parameter migration: %w", err)
				}
			}
			continue
		}
		delete(proposed, m.Schema)
		unchanged := next
		unchanged.State = m.State
		if !reflect.DeepEqual(m, unchanged) {
			return result, fmt.Errorf("candidate rewrote approved Schema parameter migration %v", m.Schema)
		}
		baseEnum := m.Before.Enum
		if m.State == interfacesnapshot.FlagMigrationConsumed {
			baseEnum = m.After.Enum
			if next.State != interfacesnapshot.FlagMigrationConsumed {
				return result, fmt.Errorf("cannot restore consumed Schema parameter migration to pending")
			}
		}
		baseParam, err := schemaMigrationParameter(base, m, baseEnum)
		if err != nil {
			return result, fmt.Errorf("migration merge-base: %w", err)
		}
		candidateEnum := m.Before.Enum
		if next.State == interfacesnapshot.FlagMigrationConsumed {
			candidateEnum = m.After.Enum
		}
		currentParam, err := schemaMigrationParameter(current, m, candidateEnum)
		if err != nil {
			return result, fmt.Errorf("migration candidate: %w", err)
		}
		if !parameterUnchangedExceptEnum(baseParam, currentParam) {
			return result, fmt.Errorf("Schema enum migration changed a non-enum parameter field")
		}
		stableParam, err := schemaMigrationParameter(stable, m, m.Before.Enum)
		if err != nil && m.State == interfacesnapshot.FlagMigrationConsumed {
			stableParam, err = schemaMigrationParameter(stable, m, m.After.Enum)
		}
		if err != nil || !parameterUnchangedExceptEnum(stableParam, currentParam) {
			return result, fmt.Errorf("migration stable contract is not the exact before/after parameter")
		}
		if next.State == interfacesnapshot.FlagMigrationPending {
			continue // A declaration PR must leave the product in before.
		}
		old, err := schemaMigrationParameter(historical, m, m.Before.Enum)
		if err != nil {
			if _, afterErr := schemaMigrationParameter(historical, m, m.After.Enum); afterErr != nil {
				return result, fmt.Errorf("historical parameter matches neither before nor after")
			}
			continue // Already-after receipt is inert; ordinary checks still run.
		}
		if !parameterUnchangedExceptEnum(old, currentParam) {
			return result, fmt.Errorf("historical Schema parameter changed beyond enum")
		}
		tool := result.Products[m.Schema.ProductID].Tools[m.Schema.ToolID]
		old.Enum = slices.Clone(m.After.Enum)
		tool.Parameters[m.Schema.Parameter] = old
	}
	for _, m := range proposed {
		if m.State != interfacesnapshot.FlagMigrationPending {
			return result, fmt.Errorf("candidate-added Schema parameter migration must start pending")
		}
		var reference parameterSchema
		for index, contract := range []schemaContract{base, stable, current} {
			p, err := schemaMigrationParameter(contract, m, m.Before.Enum)
			if err != nil || (index > 0 && !reflect.DeepEqual(reference, p)) {
				return result, fmt.Errorf("new pending Schema parameter migration must preserve the exact before contract in base, stable and candidate")
			}
			reference = p
		}
	}
	return result, nil
}

func schemaMigrationParameter(contract schemaContract, m interfacesnapshot.SchemaParameterMigration, want []string) (parameterSchema, error) {
	tool, ok := contract.Products[m.Schema.ProductID].Tools[m.Schema.ToolID]
	if !ok || tool.PrimaryCLIPath != strings.TrimPrefix(m.Command, "dws ") {
		return parameterSchema{}, fmt.Errorf("missing or wrong-path Schema migration tool %v", m.Schema)
	}
	p, ok := tool.Parameters[m.Schema.Parameter]
	if !ok || p.Type != `"string"` || !slices.Equal(p.Enum, want) {
		return parameterSchema{}, fmt.Errorf("Schema migration parameter %v does not match exact string enum %v", m.Schema, want)
	}
	return p, nil
}

func parameterUnchangedExceptEnum(a, b parameterSchema) bool {
	a.Enum, b.Enum = nil, nil
	return reflect.DeepEqual(a, b)
}
