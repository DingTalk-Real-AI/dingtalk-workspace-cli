// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package interfacesnapshot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
)

// SchemaParameterMigrationManifest is separate from CLI flag migrations:
// enum changes do not change the Cobra flag's Go type. Only the base-owned
// Schema checker may authorize these receipts against all three contracts.
type SchemaParameterMigrationManifest struct {
	Version    int                        `json:"version"`
	Migrations []SchemaParameterMigration `json:"migrations"`
}

type SchemaParameterMigration struct {
	Kind    string                     `json:"kind"`
	Command string                     `json:"command"`
	Schema  SchemaParameterMigrationID `json:"schema"`
	Before  SchemaEnumState            `json:"before"`
	After   SchemaEnumState            `json:"after"`
	State   string                     `json:"state"`
	Reason  string                     `json:"reason"`
}

type SchemaParameterMigrationID struct {
	ProductID string `json:"product_id"`
	ToolID    string `json:"tool_id"`
	Parameter string `json:"parameter"`
}

type SchemaEnumState struct {
	Enum []string `json:"enum"`
}

func ReadSchemaParameterMigrationManifest(r io.Reader) (SchemaParameterMigrationManifest, error) {
	var result SchemaParameterMigrationManifest
	data, err := io.ReadAll(r)
	if err != nil {
		return result, err
	}
	strict := json.NewDecoder(bytes.NewReader(data))
	strict.UseNumber()
	if err := validateLabeledMigrationJSONValue(strict, "$", reflect.TypeOf(result), "Schema parameter"); err != nil {
		return result, err
	}
	if _, err := strict.Token(); err != io.EOF {
		return result, fmt.Errorf("Schema parameter migration manifest has trailing data")
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return result, err
	}
	if result.Version != 1 || result.Migrations == nil {
		return result, fmt.Errorf("Schema parameter migrations require version 1 and a migrations array")
	}
	seen := make(map[SchemaParameterMigrationID]bool)
	commands := make(map[string]bool)
	for _, m := range result.Migrations {
		if m.Kind != "schema_enum_change" || !isExactCommandPath(m.Command) ||
			!exactSchemaMigrationID(m.Schema.ProductID) || !exactSchemaMigrationID(m.Schema.ToolID) ||
			!isExactFlagName(m.Schema.Parameter) || strings.TrimSpace(m.Reason) == "" || m.Reason != strings.TrimSpace(m.Reason) ||
			(m.State != FlagMigrationPending && m.State != FlagMigrationConsumed) {
			return result, fmt.Errorf("invalid Schema parameter migration identity, kind, state or reason")
		}
		key := m.Command + "\x00" + m.Schema.Parameter
		if seen[m.Schema] || commands[key] {
			return result, fmt.Errorf("duplicate Schema parameter migration for %s --%s", m.Command, m.Schema.Parameter)
		}
		seen[m.Schema], commands[key] = true, true
		for _, values := range [][]string{m.Before.Enum, m.After.Enum} {
			if values == nil {
				return result, fmt.Errorf("enum must be an explicit array; [] denotes no restriction")
			}
			for i, value := range values {
				if value == "" || value != strings.TrimSpace(value) || (i > 0 && values[i-1] >= value) {
					return result, fmt.Errorf("enum values must be nonempty, unique and sorted")
				}
			}
		}
		if len(m.After.Enum) == 0 || reflect.DeepEqual(m.Before.Enum, m.After.Enum) {
			return result, fmt.Errorf("schema_enum_change requires a different, nonempty after enum")
		}
	}
	return result, nil
}

func exactSchemaMigrationID(value string) bool {
	return value != "" && !strings.ContainsAny(value, " \t\r\n/*?\\")
}
