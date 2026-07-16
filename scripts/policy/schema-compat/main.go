// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

// Command schema-compat normalizes and checks the backwards-compatible
// execution contract returned by `dws schema --all --format json`.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

const (
	schemaContractVersion              = 3
	approvedInterfaceMigrationsVersion = 1
)

type schemaContract struct {
	Version  int                      `json:"version"`
	Products map[string]productSchema `json:"products"`
}

type productSchema struct {
	Tools map[string]toolSchema `json:"tools"`
}

type toolSchema struct {
	PrimaryCLIPath string                     `json:"primary_cli_path"`
	InterfaceMode  string                     `json:"interface_mode"`
	InterfaceRef   string                     `json:"interface_ref,omitempty"`
	Availability   string                     `json:"availability"`
	Parameters     map[string]parameterSchema `json:"parameters"`
	Constraints    string                     `json:"constraints,omitempty"`
	Positionals    []positionalSchema         `json:"positionals,omitempty"`
	DryRun         string                     `json:"dry_run,omitempty"`
	Effect         string                     `json:"effect"`
	Risk           string                     `json:"risk"`
	Confirmation   string                     `json:"confirmation"`
	Idempotency    string                     `json:"idempotency"`
}

type positionalSchema struct {
	Name     string `json:"name"`
	Index    int    `json:"index"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
	Variadic bool   `json:"variadic,omitempty"`
}

type parameterSchema struct {
	Type             string   `json:"type"`
	Property         string   `json:"property,omitempty"`
	InterfaceType    string   `json:"interface_type,omitempty"`
	Required         bool     `json:"required,omitempty"`
	CLIRequired      bool     `json:"cli_required,omitempty"`
	RequiredWhen     string   `json:"required_when,omitempty"`
	Default          string   `json:"default,omitempty"`
	InterfaceDefault string   `json:"interface_default,omitempty"`
	Format           string   `json:"format,omitempty"`
	Enum             []string `json:"enum,omitempty"`
}

type approvedInterfaceMigrationManifest struct {
	Version    int                          `json:"version"`
	Migrations []approvedInterfaceMigration `json:"migrations"`
}

type approvedInterfaceMigration struct {
	Tool           string                     `json:"tool"`
	Old            interfaceMigrationEndpoint `json:"old"`
	New            interfaceMigrationEndpoint `json:"new"`
	OldConstraints json.RawMessage            `json:"old_constraints,omitempty"`
	NewConstraints json.RawMessage            `json:"new_constraints,omitempty"`
	Reason         string                     `json:"reason"`
}

type interfaceMigrationEndpoint struct {
	InterfaceMode string          `json:"interface_mode"`
	InterfaceRef  json.RawMessage `json:"interface_ref"`
}

type normalizedInterfaceMigration struct {
	Tool           string
	Old            interfaceState
	New            interfaceState
	HasConstraints bool
	OldConstraints string
	NewConstraints string
	Reason         string
}

type interfaceState struct {
	Mode string
	Ref  string
}

type approvedInterfaceRef struct {
	ProductID string `json:"product_id"`
	RPCName   string `json:"rpc_name"`
}

type constraintContract struct {
	OtherFields     string
	RequireOneOf    [][]string
	HasRequireOneOf bool
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	var normalizePath, checkPath, mergePath, currentPath string
	var approvedMigrationsPath, candidateMigrationsPath string
	flags := flag.NewFlagSet("schema-compat", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&normalizePath, "normalize", "", "normalize a raw complete Schema response")
	flags.StringVar(&checkPath, "check", "", "check against a normalized historical baseline")
	flags.StringVar(&mergePath, "merge", "", "merge additions into a normalized historical baseline")
	flags.StringVar(&currentPath, "current", "", "raw current complete Schema response")
	flags.StringVar(&approvedMigrationsPath, "approved-interface-migrations", "", "base-owned exact interface migration manifest")
	flags.StringVar(&candidateMigrationsPath, "candidate-interface-migrations", "", "candidate manifest used only to verify approval retention and consumption")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	modes := 0
	for _, value := range []string{normalizePath, checkPath, mergePath} {
		if value != "" {
			modes++
		}
	}
	if modes != 1 {
		fmt.Fprintln(stderr, "exactly one of --normalize, --check, or --merge is required")
		return 2
	}
	if approvedMigrationsPath != "" && checkPath == "" {
		fmt.Fprintln(stderr, "--approved-interface-migrations is only valid with --check")
		return 2
	}
	if candidateMigrationsPath != "" && approvedMigrationsPath == "" {
		fmt.Fprintln(stderr, "--candidate-interface-migrations requires --approved-interface-migrations")
		return 2
	}

	if normalizePath != "" {
		currentPath = normalizePath
	}
	if currentPath == "" {
		fmt.Fprintln(stderr, "--current is required with --check or --merge")
		return 2
	}
	current, err := normalizeRawFile(currentPath)
	if err != nil {
		fmt.Fprintf(stderr, "normalize current Schema contract: %v\n", err)
		return 2
	}

	switch {
	case normalizePath != "":
		if err := writeContract(stdout, current); err != nil {
			fmt.Fprintf(stderr, "write schema contract: %v\n", err)
			return 2
		}
	case checkPath != "":
		baseline, err := readContract(checkPath)
		if err != nil {
			fmt.Fprintf(stderr, "read schema baseline: %v\n", err)
			return 2
		}
		migrations := map[string]normalizedInterfaceMigration{}
		candidateMigrations := map[string]normalizedInterfaceMigration{}
		if approvedMigrationsPath != "" {
			migrations, err = readApprovedInterfaceMigrations(approvedMigrationsPath)
			if err != nil {
				fmt.Fprintf(stderr, "read approved interface migrations: %v\n", err)
				return 2
			}
			if candidateMigrationsPath != "" {
				candidateMigrations, err = readApprovedInterfaceMigrations(candidateMigrationsPath)
				if err != nil {
					fmt.Fprintf(stderr, "read candidate interface migrations: %v\n", err)
					return 2
				}
			}
		}
		failures, err := checkCompatibilityWithMigrationManifests(
			baseline,
			current,
			migrations,
			candidateMigrations,
		)
		if err != nil {
			fmt.Fprintf(stderr, "validate approved interface migrations: %v\n", err)
			return 2
		}
		if len(failures) > 0 {
			fmt.Fprintln(stderr, "Schema backwards-compatibility check failed:")
			for _, failure := range failures {
				fmt.Fprintf(stderr, "  - %s\n", failure)
			}
			return 1
		}
		fmt.Fprintf(stdout, "Schema compatibility check: ok (%d historical products; additions allowed)\n", len(baseline.Products))
	case mergePath != "":
		baseline, err := readContract(mergePath)
		if err != nil {
			fmt.Fprintf(stderr, "read schema baseline: %v\n", err)
			return 2
		}
		merged, failures := mergeContracts(baseline, current)
		if len(failures) > 0 {
			fmt.Fprintln(stderr, "cannot merge incompatible schema changes:")
			for _, failure := range failures {
				fmt.Fprintf(stderr, "  - %s\n", failure)
			}
			return 1
		}
		if err := writeContract(stdout, merged); err != nil {
			fmt.Fprintf(stderr, "write schema contract: %v\n", err)
			return 2
		}
	}
	return 0
}

func normalizeRawFile(path string) (schemaContract, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return schemaContract{}, err
	}
	var payload struct {
		Kind     string            `json:"kind"`
		Products []json.RawMessage `json:"products"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return schemaContract{}, err
	}
	if payload.Kind != "schema" {
		return schemaContract{}, fmt.Errorf("unexpected kind %q", payload.Kind)
	}
	if payload.Products == nil {
		return schemaContract{}, fmt.Errorf("products array is missing")
	}
	contract := schemaContract{Version: schemaContractVersion, Products: map[string]productSchema{}}
	for _, rawProduct := range payload.Products {
		var product struct {
			ID    string            `json:"id"`
			Tools []json.RawMessage `json:"tools"`
		}
		if err := json.Unmarshal(rawProduct, &product); err != nil {
			return schemaContract{}, err
		}
		if product.ID == "" {
			return schemaContract{}, fmt.Errorf("product without id")
		}
		if _, exists := contract.Products[product.ID]; exists {
			return schemaContract{}, fmt.Errorf("duplicate product id %q", product.ID)
		}
		normalized := productSchema{Tools: map[string]toolSchema{}}
		for _, rawTool := range product.Tools {
			id, tool, err := normalizeTool(rawTool)
			if err != nil {
				return schemaContract{}, fmt.Errorf("product %s: %w", product.ID, err)
			}
			if _, exists := normalized.Tools[id]; exists {
				return schemaContract{}, fmt.Errorf("product %s: duplicate tool id %q", product.ID, id)
			}
			normalized.Tools[id] = tool
		}
		contract.Products[product.ID] = normalized
	}
	if len(contract.Products) == 0 {
		return schemaContract{}, fmt.Errorf("complete Schema contract contains no products")
	}
	totalTools := 0
	for _, product := range contract.Products {
		totalTools += len(product.Tools)
	}
	if totalTools == 0 {
		return schemaContract{}, fmt.Errorf("complete Schema contract contains no tools")
	}
	return contract, nil
}

func normalizeTool(raw json.RawMessage) (string, toolSchema, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return "", toolSchema{}, err
	}
	for _, field := range []string{
		"canonical_path",
		"primary_cli_path",
		"parameters",
		"effect",
		"risk",
		"confirmation",
		"idempotency",
		"interface_mode",
		"availability",
		"field_provenance",
	} {
		if _, ok := fields[field]; !ok {
			return "", toolSchema{}, fmt.Errorf("tool is not a complete schema --all leaf: missing %s", field)
		}
	}

	var tool struct {
		CanonicalPath  string                     `json:"canonical_path"`
		PrimaryCLIPath string                     `json:"primary_cli_path"`
		InterfaceMode  string                     `json:"interface_mode"`
		InterfaceRef   json.RawMessage            `json:"interface_ref"`
		Availability   string                     `json:"availability"`
		Parameters     map[string]json.RawMessage `json:"parameters"`
		Required       []string                   `json:"required"`
		Constraints    json.RawMessage            `json:"constraints"`
		Positionals    json.RawMessage            `json:"positionals"`
		DryRun         json.RawMessage            `json:"dry_run"`
		Effect         string                     `json:"effect"`
		Risk           string                     `json:"risk"`
		Confirmation   string                     `json:"confirmation"`
		Idempotency    string                     `json:"idempotency"`
	}
	if err := json.Unmarshal(raw, &tool); err != nil {
		return "", toolSchema{}, err
	}
	id := strings.TrimSpace(tool.CanonicalPath)
	if id == "" {
		return "", toolSchema{}, fmt.Errorf("tool without canonical_path")
	}
	if strings.TrimSpace(tool.PrimaryCLIPath) == "" {
		return "", toolSchema{}, fmt.Errorf("tool %s without primary_cli_path", id)
	}
	if tool.Parameters == nil {
		return "", toolSchema{}, fmt.Errorf("tool %s parameters must be an object", id)
	}
	requiredParameters := stringSet(tool.Required)
	parameters := map[string]parameterSchema{}
	for name, rawSchema := range tool.Parameters {
		parameter, err := normalizeParameter(rawSchema)
		if err != nil {
			return "", toolSchema{}, fmt.Errorf("parameter %s: %w", name, err)
		}
		if requiredParameters[name] {
			parameter.Required = true
		}
		parameters[name] = parameter
	}
	for required := range requiredParameters {
		if _, ok := parameters[required]; !ok {
			return "", toolSchema{}, fmt.Errorf("required parameter %q is missing", required)
		}
	}

	interfaceRef, err := canonicalRawJSON(tool.InterfaceRef)
	if err != nil {
		return "", toolSchema{}, fmt.Errorf("interface_ref: %w", err)
	}
	constraints, err := canonicalRawJSON(tool.Constraints)
	if err != nil {
		return "", toolSchema{}, fmt.Errorf("constraints: %w", err)
	}
	positionals, err := normalizePositionals(tool.Positionals)
	if err != nil {
		return "", toolSchema{}, fmt.Errorf("positionals: %w", err)
	}
	dryRun, err := canonicalRawJSON(tool.DryRun)
	if err != nil {
		return "", toolSchema{}, fmt.Errorf("dry_run: %w", err)
	}

	return id, toolSchema{
		PrimaryCLIPath: strings.TrimSpace(tool.PrimaryCLIPath),
		InterfaceMode:  strings.TrimSpace(tool.InterfaceMode),
		InterfaceRef:   interfaceRef,
		Availability:   strings.TrimSpace(tool.Availability),
		Parameters:     parameters,
		Constraints:    constraints,
		Positionals:    positionals,
		DryRun:         dryRun,
		Effect:         strings.TrimSpace(tool.Effect),
		Risk:           strings.TrimSpace(tool.Risk),
		Confirmation:   strings.TrimSpace(tool.Confirmation),
		Idempotency:    strings.TrimSpace(tool.Idempotency),
	}, nil
}

func normalizePositionals(raw json.RawMessage) ([]positionalSchema, error) {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}
	var positionals []positionalSchema
	if err := json.Unmarshal(raw, &positionals); err != nil {
		return nil, err
	}
	seenIndexes := map[int]bool{}
	for index := range positionals {
		positional := &positionals[index]
		positional.Name = strings.TrimSpace(positional.Name)
		positional.Type = strings.TrimSpace(positional.Type)
		if positional.Name == "" {
			return nil, fmt.Errorf("positional at index %d has no name", positional.Index)
		}
		if positional.Index < 0 {
			return nil, fmt.Errorf("positional %q has negative index", positional.Name)
		}
		if positional.Type == "" {
			return nil, fmt.Errorf("positional %q has no type", positional.Name)
		}
		if seenIndexes[positional.Index] {
			return nil, fmt.Errorf("duplicate positional index %d", positional.Index)
		}
		seenIndexes[positional.Index] = true
	}
	sort.Slice(positionals, func(i, j int) bool {
		return positionals[i].Index < positionals[j].Index
	})
	return positionals, nil
}

func normalizeParameter(raw json.RawMessage) (parameterSchema, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return parameterSchema{}, err
	}
	for _, field := range []string{"type", "required", "field_provenance"} {
		if _, ok := fields[field]; !ok {
			return parameterSchema{}, fmt.Errorf("not a complete schema --all parameter: missing %s", field)
		}
	}

	var parameter struct {
		Required         bool            `json:"required"`
		CLIRequired      bool            `json:"cli_required"`
		RequiredWhen     string          `json:"required_when"`
		Property         string          `json:"property"`
		InterfaceType    string          `json:"interface_type"`
		Default          json.RawMessage `json:"default"`
		InterfaceDefault json.RawMessage `json:"interface_default"`
		Format           string          `json:"format"`
		Enum             []string        `json:"enum"`
	}
	if err := json.Unmarshal(raw, &parameter); err != nil {
		return parameterSchema{}, err
	}

	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return parameterSchema{}, err
	}
	parameterType := schemaType(schema)
	if parameterType == "unspecified" {
		return parameterSchema{}, fmt.Errorf("type is missing")
	}
	defaultValue, err := canonicalRawJSON(parameter.Default)
	if err != nil {
		return parameterSchema{}, fmt.Errorf("default: %w", err)
	}
	interfaceDefault, err := canonicalRawJSON(parameter.InterfaceDefault)
	if err != nil {
		return parameterSchema{}, fmt.Errorf("interface_default: %w", err)
	}
	enum := append([]string(nil), parameter.Enum...)
	sort.Strings(enum)

	return parameterSchema{
		Type:             parameterType,
		Property:         strings.TrimSpace(parameter.Property),
		InterfaceType:    strings.TrimSpace(parameter.InterfaceType),
		Required:         parameter.Required,
		CLIRequired:      parameter.CLIRequired,
		RequiredWhen:     strings.TrimSpace(parameter.RequiredWhen),
		Default:          defaultValue,
		InterfaceDefault: interfaceDefault,
		Format:           strings.TrimSpace(parameter.Format),
		Enum:             enum,
	}, nil
}

func canonicalRawJSON(raw json.RawMessage) (string, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return "", nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return "", err
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func schemaType(schema map[string]any) string {
	if value, ok := schema["type"]; ok {
		encoded, _ := json.Marshal(value)
		return string(encoded)
	}
	for _, keyword := range []string{"oneOf", "anyOf", "allOf"} {
		if value, ok := schema[keyword]; ok {
			encoded, _ := json.Marshal(value)
			return keyword + ":" + string(encoded)
		}
	}
	return "unspecified"
}

// readApprovedInterfaceMigrations accepts only explicit, versioned endpoints;
// the authoritative shell decides which merge-base-owned file is passed here.
func readApprovedInterfaceMigrations(path string) (map[string]normalizedInterfaceMigration, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var manifest approvedInterfaceMigrationManifest
	if err := decodeStrictJSON(data, &manifest); err != nil {
		return nil, err
	}
	if manifest.Version != approvedInterfaceMigrationsVersion {
		return nil, fmt.Errorf("unsupported approved interface migrations version %d", manifest.Version)
	}
	if len(manifest.Migrations) == 0 {
		return nil, fmt.Errorf("approved interface migrations manifest contains no migrations")
	}

	migrations := make(map[string]normalizedInterfaceMigration, len(manifest.Migrations))
	for index, migration := range manifest.Migrations {
		tool := strings.TrimSpace(migration.Tool)
		if err := validateApprovedToolPath(tool); err != nil {
			return nil, fmt.Errorf("migration %d: %w", index, err)
		}
		if tool != migration.Tool {
			return nil, fmt.Errorf("migration %d tool path must not contain surrounding whitespace", index)
		}
		if _, exists := migrations[tool]; exists {
			return nil, fmt.Errorf("migration %d duplicates tool %q", index, tool)
		}
		reason := strings.TrimSpace(migration.Reason)
		if reason == "" {
			return nil, fmt.Errorf("migration %d for %q has no review reason", index, tool)
		}
		oldState, err := normalizeMigrationEndpoint("old", migration.Old)
		if err != nil {
			return nil, fmt.Errorf("migration %d for %q: %w", index, tool, err)
		}
		newState, err := normalizeMigrationEndpoint("new", migration.New)
		if err != nil {
			return nil, fmt.Errorf("migration %d for %q: %w", index, tool, err)
		}
		oldConstraints, hasOldConstraints, err := normalizeMigrationConstraints("old_constraints", migration.OldConstraints)
		if err != nil {
			return nil, fmt.Errorf("migration %d for %q: %w", index, tool, err)
		}
		newConstraints, hasNewConstraints, err := normalizeMigrationConstraints("new_constraints", migration.NewConstraints)
		if err != nil {
			return nil, fmt.Errorf("migration %d for %q: %w", index, tool, err)
		}
		if hasOldConstraints != hasNewConstraints {
			return nil, fmt.Errorf(
				"migration %d for %q must provide old_constraints and new_constraints together",
				index,
				tool,
			)
		}
		if hasOldConstraints {
			if _, err := parseConstraintContract(oldConstraints); err != nil {
				return nil, fmt.Errorf("migration %d for %q: old_constraints is invalid: %w", index, tool, err)
			}
			if _, err := parseConstraintContract(newConstraints); err != nil {
				return nil, fmt.Errorf("migration %d for %q: new_constraints is invalid: %w", index, tool, err)
			}
		}
		if oldState == newState {
			return nil, fmt.Errorf("migration %d for %q does not change the interface contract", index, tool)
		}
		migrations[tool] = normalizedInterfaceMigration{
			Tool:           tool,
			Old:            oldState,
			New:            newState,
			HasConstraints: hasOldConstraints,
			OldConstraints: oldConstraints,
			NewConstraints: newConstraints,
			Reason:         reason,
		}
	}
	return migrations, nil
}

func decodeStrictJSON(data []byte, target any) error {
	// encoding/json otherwise accepts duplicate object keys with last-value-wins.
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return err
	}
	if err := rejectNonCanonicalApprovedJSONKeys(data, target); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return fmt.Errorf("decode trailing JSON: %w", err)
	}
	return nil
}

func rejectNonCanonicalApprovedJSONKeys(data []byte, target any) error {
	switch target.(type) {
	case *approvedInterfaceMigrationManifest:
		return validateApprovedMigrationManifestKeys(data)
	case *approvedInterfaceRef:
		if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
			return nil
		}
		fields, err := decodeStrictJSONObject(data, "interface_ref")
		if err != nil {
			return err
		}
		return requireCanonicalJSONFields("interface_ref", fields, "product_id", "rpc_name")
	default:
		return nil
	}
}

func validateApprovedMigrationManifestKeys(data []byte) error {
	fields, err := decodeStrictJSONObject(data, "manifest")
	if err != nil {
		return err
	}
	if err := requireCanonicalJSONFields("manifest", fields, "version", "migrations"); err != nil {
		return err
	}
	migrationsRaw, ok := fields["migrations"]
	if !ok {
		return nil
	}
	var migrations []json.RawMessage
	if err := json.Unmarshal(migrationsRaw, &migrations); err != nil {
		return err
	}
	for index, migrationRaw := range migrations {
		path := fmt.Sprintf("manifest.migrations[%d]", index)
		migration, err := decodeStrictJSONObject(migrationRaw, path)
		if err != nil {
			return err
		}
		if err := requireCanonicalJSONFields(
			path,
			migration,
			"tool",
			"old",
			"new",
			"old_constraints",
			"new_constraints",
			"reason",
		); err != nil {
			return err
		}
		for _, endpointName := range []string{"old", "new"} {
			endpointRaw, ok := migration[endpointName]
			if !ok {
				continue
			}
			endpointPath := path + "." + endpointName
			endpoint, err := decodeStrictJSONObject(endpointRaw, endpointPath)
			if err != nil {
				return err
			}
			if err := requireCanonicalJSONFields(endpointPath, endpoint, "interface_mode", "interface_ref"); err != nil {
				return err
			}
		}
	}
	return nil
}

func decodeStrictJSONObject(data []byte, path string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, fmt.Errorf("%s must be a JSON object: %w", path, err)
	}
	if fields == nil {
		return nil, fmt.Errorf("%s must be a JSON object", path)
	}
	return fields, nil
}

func requireCanonicalJSONFields(path string, fields map[string]json.RawMessage, allowed ...string) error {
	canonical := stringSet(allowed)
	for field := range fields {
		if !canonical[field] {
			return fmt.Errorf("unknown field %q at %s (field names must use canonical case)", field, path)
		}
	}
	return nil
}

func rejectDuplicateJSONKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := consumeJSONValue(decoder, "$"); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return fmt.Errorf("decode trailing JSON: %w", err)
	}
	return nil
}

func consumeJSONValue(decoder *json.Decoder, path string) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}

	switch delimiter {
	case '{':
		seen := map[string]bool{}
		seenFolded := map[string]string{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("JSON object key at %s is not a string", path)
			}
			if seen[key] {
				return fmt.Errorf("duplicate JSON key %q at %s", key, path)
			}
			seen[key] = true
			folded := strings.ToLower(key)
			if previous, exists := seenFolded[folded]; exists {
				return fmt.Errorf("duplicate JSON keys %q and %q at %s differ only by case", previous, key, path)
			}
			seenFolded[folded] = key
			if err := consumeJSONValue(decoder, path+"."+key); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim('}') {
			return fmt.Errorf("JSON object at %s is not closed", path)
		}
	case '[':
		index := 0
		for decoder.More() {
			if err := consumeJSONValue(decoder, fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
			index++
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim(']') {
			return fmt.Errorf("JSON array at %s is not closed", path)
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q at %s", delimiter, path)
	}
	return nil
}

func validateApprovedToolPath(tool string) error {
	if strings.Count(tool, "/") != 1 {
		return fmt.Errorf("approved tool %q must be an exact product/canonical path", tool)
	}
	productID, canonical, _ := strings.Cut(tool, "/")
	if !exactMigrationToken(productID) || !exactMigrationToken(canonical) {
		return fmt.Errorf("approved tool %q contains an empty, wildcard, or unsupported token", tool)
	}
	return nil
}

func exactMigrationToken(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		switch {
		case character >= 'a' && character <= 'z':
		case character >= 'A' && character <= 'Z':
		case character >= '0' && character <= '9':
		case character == '.', character == '_', character == '-':
		default:
			return false
		}
	}
	return true
}

func normalizeMigrationEndpoint(label string, endpoint interfaceMigrationEndpoint) (interfaceState, error) {
	mode := strings.TrimSpace(endpoint.InterfaceMode)
	if mode != endpoint.InterfaceMode {
		return interfaceState{}, fmt.Errorf("%s interface_mode must not contain surrounding whitespace", label)
	}
	refRaw := bytes.TrimSpace(endpoint.InterfaceRef)
	if len(refRaw) == 0 {
		return interfaceState{}, fmt.Errorf("%s interface_ref is missing", label)
	}

	switch mode {
	case "mcp":
		var ref approvedInterfaceRef
		if err := decodeStrictJSON(refRaw, &ref); err != nil {
			return interfaceState{}, fmt.Errorf("%s mcp interface_ref: %w", label, err)
		}
		if ref.ProductID != strings.TrimSpace(ref.ProductID) || ref.RPCName != strings.TrimSpace(ref.RPCName) {
			return interfaceState{}, fmt.Errorf("%s mcp interface_ref values must not contain surrounding whitespace", label)
		}
		if !exactMigrationToken(ref.ProductID) || !exactMigrationToken(ref.RPCName) {
			return interfaceState{}, fmt.Errorf("%s mcp interface_ref must contain exact product_id and rpc_name", label)
		}
		encoded, err := json.Marshal(ref)
		if err != nil {
			return interfaceState{}, fmt.Errorf("%s mcp interface_ref: %w", label, err)
		}
		return interfaceState{Mode: mode, Ref: string(encoded)}, nil
	case "local", "composite":
		if !bytes.Equal(refRaw, []byte("null")) {
			return interfaceState{}, fmt.Errorf("%s %s interface_ref must be explicit null", label, mode)
		}
		return interfaceState{Mode: mode, Ref: "null"}, nil
	case "":
		return interfaceState{}, fmt.Errorf("%s interface_mode is missing", label)
	default:
		return interfaceState{}, fmt.Errorf("%s interface_mode %q is not supported", label, mode)
	}
}

func normalizeMigrationConstraints(label string, raw json.RawMessage) (string, bool, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return "", false, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return "", false, fmt.Errorf("%s must be a JSON object: %w", label, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return "", false, fmt.Errorf("%s contains multiple JSON values", label)
		}
		return "", false, fmt.Errorf("%s trailing JSON: %w", label, err)
	}
	object, ok := value.(map[string]any)
	if !ok || object == nil {
		return "", false, fmt.Errorf("%s must be a JSON object", label)
	}
	encoded, err := json.Marshal(object)
	if err != nil {
		return "", false, fmt.Errorf("canonicalize %s: %w", label, err)
	}
	return string(encoded), true, nil
}

func readContract(path string) (schemaContract, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return schemaContract{}, err
	}
	var contract schemaContract
	if err := json.Unmarshal(data, &contract); err != nil {
		return schemaContract{}, err
	}
	if contract.Version != schemaContractVersion {
		return schemaContract{}, fmt.Errorf("unsupported schema contract version %d", contract.Version)
	}
	if len(contract.Products) == 0 {
		return schemaContract{}, fmt.Errorf("historical schema contract contains no products")
	}
	return contract, nil
}

func checkCompatibility(baseline, current schemaContract) []string {
	failures, _ := checkCompatibilityWithMigrations(
		baseline,
		current,
		map[string]normalizedInterfaceMigration{},
	)
	return failures
}

func checkCompatibilityWithMigrations(
	baseline, current schemaContract,
	migrations map[string]normalizedInterfaceMigration,
) ([]string, error) {
	active, _, err := classifyApprovedInterfaceMigrations(baseline, migrations)
	if err != nil {
		return nil, err
	}
	return checkCompatibilityWithActiveMigrations(baseline, current, active), nil
}

func checkCompatibilityWithMigrationManifests(
	baseline, current schemaContract,
	baseMigrations, candidateMigrations map[string]normalizedInterfaceMigration,
) ([]string, error) {
	active, alreadyApplied, err := classifyApprovedInterfaceMigrations(baseline, baseMigrations)
	if err != nil {
		return nil, err
	}
	if err := validateCandidateInterfaceMigrationLifecycle(
		current,
		baseMigrations,
		candidateMigrations,
		alreadyApplied,
	); err != nil {
		return nil, err
	}
	return checkCompatibilityWithActiveMigrations(baseline, current, active), nil
}

func checkCompatibilityWithActiveMigrations(
	baseline, current schemaContract,
	migrations map[string]normalizedInterfaceMigration,
) []string {
	var failures []string
	for productID, oldProduct := range baseline.Products {
		newProduct, ok := current.Products[productID]
		if !ok {
			failures = append(failures, fmt.Sprintf("historical schema product %q is missing", productID))
			continue
		}
		for toolID, oldTool := range oldProduct.Tools {
			newTool, ok := newProduct.Tools[toolID]
			if !ok {
				failures = append(failures, fmt.Sprintf("historical schema tool %q is missing", productID+"/"+toolID))
				continue
			}
			toolPath := productID + "/" + toolID
			migration, hasMigration := migrations[toolPath]
			if hasMigration {
				failures = append(failures, checkToolCompatibilityWithMigration(toolPath, oldTool, newTool, &migration)...)
				continue
			}
			failures = append(failures, checkToolCompatibilityWithMigration(toolPath, oldTool, newTool, nil)...)
		}
	}
	sort.Strings(failures)
	return failures
}

func checkToolCompatibility(toolPath string, oldTool, newTool toolSchema) []string {
	return checkToolCompatibilityWithMigration(toolPath, oldTool, newTool, nil)
}

func checkToolCompatibilityWithMigration(
	toolPath string,
	oldTool, newTool toolSchema,
	migration *normalizedInterfaceMigration,
) []string {
	var failures []string
	exactContractMigrationApproved := migration != nil &&
		toolMatchesMigrationOld(oldTool, *migration) &&
		toolMatchesMigrationNew(newTool, *migration)
	for _, field := range []struct {
		name string
		old  string
		new  string
	}{
		{name: "primary_cli_path", old: oldTool.PrimaryCLIPath, new: newTool.PrimaryCLIPath},
		{name: "availability", old: oldTool.Availability, new: newTool.Availability},
		{name: "effect", old: oldTool.Effect, new: newTool.Effect},
		{name: "risk", old: oldTool.Risk, new: newTool.Risk},
		{name: "confirmation", old: oldTool.Confirmation, new: newTool.Confirmation},
		{name: "idempotency", old: oldTool.Idempotency, new: newTool.Idempotency},
	} {
		if field.old != field.new {
			failures = append(failures, fmt.Sprintf("schema tool %q changed %s", toolPath, field.name))
		}
	}
	if !exactContractMigrationApproved {
		if oldTool.InterfaceMode != newTool.InterfaceMode {
			failures = append(failures, fmt.Sprintf("schema tool %q changed interface_mode", toolPath))
		}
		if oldTool.InterfaceRef != newTool.InterfaceRef {
			failures = append(failures, fmt.Sprintf("schema tool %q changed interface_ref", toolPath))
		}
	}
	constraintsMigrationApproved := exactContractMigrationApproved && migration.HasConstraints
	if constraintsMigrationApproved {
		if addedMembers, valid := addedRequireOneOfMembers(oldTool.Constraints, newTool.Constraints); valid {
			failures = append(failures, checkAddedRequireOneOfMembers(toolPath, oldTool, newTool, addedMembers)...)
		} else {
			failures = append(failures, fmt.Sprintf("schema tool %q has invalid approved constraints", toolPath))
		}
	} else {
		addedConstraintMembers, constraintsOK := compatibleConstraintAdditions(oldTool.Constraints, newTool.Constraints)
		if !constraintsOK {
			failures = append(failures, fmt.Sprintf("schema tool %q changed constraints", toolPath))
		} else {
			failures = append(failures, checkAddedRequireOneOfMembers(toolPath, oldTool, newTool, addedConstraintMembers)...)
		}
	}
	if !compatiblePositionals(oldTool.Positionals, newTool.Positionals) {
		failures = append(failures, fmt.Sprintf("schema tool %q changed positionals", toolPath))
	}
	if oldTool.DryRun != "" && oldTool.DryRun != newTool.DryRun {
		failures = append(failures, fmt.Sprintf("schema tool %q changed or removed dry_run", toolPath))
	}

	for parameter, oldParameter := range oldTool.Parameters {
		newParameter, ok := newTool.Parameters[parameter]
		if !ok {
			failures = append(failures, fmt.Sprintf("schema tool %q lost parameter %q", toolPath, parameter))
			continue
		}
		failures = append(failures, checkParameterCompatibility(toolPath, parameter, oldParameter, newParameter)...)
	}
	sort.Strings(failures)
	return failures
}

func classifyApprovedInterfaceMigrations(
	baseline schemaContract,
	migrations map[string]normalizedInterfaceMigration,
) (map[string]normalizedInterfaceMigration, map[string]bool, error) {
	// Only old-matching entries are active authority. Exact new-matching entries
	// are retained solely so a cleanup PR can remove a previously missed entry.
	active := make(map[string]normalizedInterfaceMigration, len(migrations))
	alreadyApplied := map[string]bool{}
	tools := make([]string, 0, len(migrations))
	for tool := range migrations {
		tools = append(tools, tool)
	}
	sort.Strings(tools)
	for _, toolPath := range tools {
		migration := migrations[toolPath]
		productID, toolID, _ := strings.Cut(toolPath, "/")
		product, productExists := baseline.Products[productID]
		if !productExists {
			return nil, nil, fmt.Errorf("approved interface migration %q references unknown historical product", toolPath)
		}
		tool, toolExists := product.Tools[toolID]
		if !toolExists {
			return nil, nil, fmt.Errorf("approved interface migration %q references unknown historical tool", toolPath)
		}
		switch {
		case toolMatchesMigrationOld(tool, migration):
			active[toolPath] = migration
		case toolMatchesMigrationNew(tool, migration):
			alreadyApplied[toolPath] = true
		default:
			return nil, nil, fmt.Errorf(
				"approved interface migration %q is stale: historical contract matches neither old nor new",
				toolPath,
			)
		}
	}
	return active, alreadyApplied, nil
}

func validateCandidateInterfaceMigrationLifecycle(
	current schemaContract,
	baseMigrations, candidateMigrations map[string]normalizedInterfaceMigration,
	alreadyApplied map[string]bool,
) error {
	// A candidate cannot mutate base authority in place: pending entries stay
	// exact, while consumed or already-applied entries disappear.
	tools := sortedMigrationTools(baseMigrations)
	for _, toolPath := range tools {
		baseMigration := baseMigrations[toolPath]
		candidateMigration, retained := candidateMigrations[toolPath]
		if alreadyApplied[toolPath] {
			if retained {
				return fmt.Errorf(
					"candidate must remove already-applied interface migration %q to recover the stale manifest",
					toolPath,
				)
			}
			continue
		}

		currentTool, exists := contractToolSchema(current, toolPath)
		if exists && toolMatchesMigrationNew(currentTool, baseMigration) {
			if retained {
				return fmt.Errorf("candidate must remove consumed interface migration %q", toolPath)
			}
			continue
		}
		if !exists || !toolMatchesMigrationOld(currentTool, baseMigration) {
			return fmt.Errorf(
				"candidate contract for approved interface migration %q matches neither exact old nor exact new",
				toolPath,
			)
		}
		if !retained {
			return fmt.Errorf("candidate must retain pending interface migration %q", toolPath)
		}
		if candidateMigration != baseMigration {
			return fmt.Errorf("candidate must retain pending interface migration %q exactly", toolPath)
		}
	}

	for _, toolPath := range sortedMigrationTools(candidateMigrations) {
		if _, baseOwned := baseMigrations[toolPath]; baseOwned {
			continue
		}
		migration := candidateMigrations[toolPath]
		currentTool, exists := contractToolSchema(current, toolPath)
		if !exists {
			return fmt.Errorf("candidate interface migration %q references an unknown current tool", toolPath)
		}
		if !toolMatchesMigrationOld(currentTool, migration) {
			return fmt.Errorf(
				"candidate interface migration %q is stale: current contract does not match old",
				toolPath,
			)
		}
	}
	return nil
}

func sortedMigrationTools(migrations map[string]normalizedInterfaceMigration) []string {
	tools := make([]string, 0, len(migrations))
	for tool := range migrations {
		tools = append(tools, tool)
	}
	sort.Strings(tools)
	return tools
}

func contractToolSchema(contract schemaContract, toolPath string) (toolSchema, bool) {
	productID, toolID, ok := strings.Cut(toolPath, "/")
	if !ok {
		return toolSchema{}, false
	}
	product, ok := contract.Products[productID]
	if !ok {
		return toolSchema{}, false
	}
	tool, ok := product.Tools[toolID]
	if !ok {
		return toolSchema{}, false
	}
	return tool, true
}

func toolMatchesMigrationOld(tool toolSchema, migration normalizedInterfaceMigration) bool {
	return toolMatchesMigrationSnapshot(tool, migration.Old, migration.HasConstraints, migration.OldConstraints)
}

func toolMatchesMigrationNew(tool toolSchema, migration normalizedInterfaceMigration) bool {
	return toolMatchesMigrationSnapshot(tool, migration.New, migration.HasConstraints, migration.NewConstraints)
}

func toolMatchesMigrationSnapshot(
	tool toolSchema,
	endpoint interfaceState,
	hasConstraints bool,
	constraints string,
) bool {
	if normalizedToolInterfaceState(tool) != endpoint {
		return false
	}
	return !hasConstraints || tool.Constraints == constraints
}

func normalizedToolInterfaceState(tool toolSchema) interfaceState {
	ref := tool.InterfaceRef
	if ref == "" && (tool.InterfaceMode == "local" || tool.InterfaceMode == "composite") {
		ref = "null"
	}
	return interfaceState{Mode: tool.InterfaceMode, Ref: ref}
}

// constraintsCompatible permits only member additions within the same set of
// require_one_of groups. Every other normalized constraint field remains exact.
func constraintsCompatible(oldConstraints, newConstraints string) bool {
	_, compatible := compatibleConstraintAdditions(oldConstraints, newConstraints)
	return compatible
}

func compatibleConstraintAdditions(oldConstraints, newConstraints string) ([]string, bool) {
	if oldConstraints == newConstraints {
		return nil, true
	}
	oldContract, err := parseConstraintContract(oldConstraints)
	if err != nil {
		return nil, false
	}
	newContract, err := parseConstraintContract(newConstraints)
	if err != nil {
		return nil, false
	}
	if oldContract.OtherFields != newContract.OtherFields ||
		oldContract.HasRequireOneOf != newContract.HasRequireOneOf {
		return nil, false
	}
	return requireOneOfWideningMembersFromContracts(oldContract, newContract)
}

func addedRequireOneOfMembers(oldConstraints, newConstraints string) ([]string, bool) {
	oldContract, err := parseConstraintContract(oldConstraints)
	if err != nil {
		return nil, false
	}
	newContract, err := parseConstraintContract(newConstraints)
	if err != nil {
		return nil, false
	}

	oldMembers := map[string]bool{}
	if oldContract.HasRequireOneOf {
		for _, group := range oldContract.RequireOneOf {
			for _, member := range group {
				oldMembers[member] = true
			}
		}
	}
	added := map[string]bool{}
	if newContract.HasRequireOneOf {
		for _, group := range newContract.RequireOneOf {
			for _, member := range group {
				if !oldMembers[member] {
					added[member] = true
				}
			}
		}
	}
	result := make([]string, 0, len(added))
	for member := range added {
		result = append(result, member)
	}
	sort.Strings(result)
	return result, true
}

func requireOneOfWideningMembersFromContracts(oldContract, newContract constraintContract) ([]string, bool) {
	if !oldContract.HasRequireOneOf {
		return nil, false
	}
	if !requireOneOfGroupsWidened(oldContract.RequireOneOf, newContract.RequireOneOf) {
		return nil, false
	}

	oldMembers := map[string]bool{}
	for _, group := range oldContract.RequireOneOf {
		for _, member := range group {
			oldMembers[member] = true
		}
	}
	added := map[string]bool{}
	for _, group := range newContract.RequireOneOf {
		for _, member := range group {
			if !oldMembers[member] {
				added[member] = true
			}
		}
	}
	result := make([]string, 0, len(added))
	for member := range added {
		result = append(result, member)
	}
	sort.Strings(result)
	return result, true
}

func checkAddedRequireOneOfMembers(toolPath string, oldTool, newTool toolSchema, members []string) []string {
	historicalPositionals := map[string]bool{}
	for _, positional := range oldTool.Positionals {
		historicalPositionals[positional.Name] = true
	}

	var failures []string
	for _, member := range members {
		parameter, exists := newTool.Parameters[member]
		if !exists {
			if !historicalPositionals[member] {
				failures = append(failures, fmt.Sprintf(
					"schema tool %q added require_one_of member %q without a parameter or historical positional",
					toolPath, member,
				))
			}
			continue
		}
		if _, historical := oldTool.Parameters[member]; historical {
			continue
		}
		if parameter.Required {
			failures = append(failures, fmt.Sprintf(
				"schema tool %q added required require_one_of parameter %q", toolPath, member,
			))
		}
		if parameter.CLIRequired {
			failures = append(failures, fmt.Sprintf(
				"schema tool %q added cli_required require_one_of parameter %q", toolPath, member,
			))
		}
		if parameter.RequiredWhen != "" {
			failures = append(failures, fmt.Sprintf(
				"schema tool %q added conditional require_one_of parameter %q", toolPath, member,
			))
		}
	}
	return failures
}

func parseConstraintContract(raw string) (constraintContract, error) {
	if raw == "" {
		return constraintContract{OtherFields: ""}, nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return constraintContract{}, err
	}
	if fields == nil {
		return constraintContract{}, fmt.Errorf("constraints must be an object")
	}
	requireOneOfRaw, hasRequireOneOf := fields["require_one_of"]
	delete(fields, "require_one_of")
	otherFields, err := json.Marshal(fields)
	if err != nil {
		return constraintContract{}, err
	}
	contract := constraintContract{
		OtherFields:     string(otherFields),
		HasRequireOneOf: hasRequireOneOf,
	}
	if !hasRequireOneOf {
		return contract, nil
	}
	if bytes.Equal(bytes.TrimSpace(requireOneOfRaw), []byte("null")) {
		return constraintContract{}, fmt.Errorf("require_one_of must be an array")
	}
	if err := json.Unmarshal(requireOneOfRaw, &contract.RequireOneOf); err != nil {
		return constraintContract{}, err
	}
	if len(contract.RequireOneOf) == 0 {
		return constraintContract{}, fmt.Errorf("require_one_of must contain at least one group")
	}
	seenGroups := map[string]bool{}
	for groupIndex, group := range contract.RequireOneOf {
		if len(group) == 0 {
			return constraintContract{}, fmt.Errorf("require_one_of group %d is empty", groupIndex)
		}
		seenMembers := map[string]bool{}
		for memberIndex, member := range group {
			if member != strings.TrimSpace(member) {
				return constraintContract{}, fmt.Errorf("require_one_of group %d member %d contains surrounding whitespace", groupIndex, memberIndex)
			}
			if member == "" {
				return constraintContract{}, fmt.Errorf("require_one_of group %d member %d is empty", groupIndex, memberIndex)
			}
			if seenMembers[member] {
				return constraintContract{}, fmt.Errorf("require_one_of group %d duplicates member %q", groupIndex, member)
			}
			seenMembers[member] = true
			contract.RequireOneOf[groupIndex][memberIndex] = member
		}
		sort.Strings(contract.RequireOneOf[groupIndex])
		groupKey := strings.Join(contract.RequireOneOf[groupIndex], "\x00")
		if seenGroups[groupKey] {
			return constraintContract{}, fmt.Errorf("require_one_of duplicates group %d", groupIndex)
		}
		seenGroups[groupKey] = true
	}
	return contract, nil
}

func requireOneOfGroupsWidened(oldGroups, newGroups [][]string) bool {
	if len(oldGroups) != len(newGroups) {
		return false
	}
	matchedOldByNew := make([]int, len(newGroups))
	for index := range matchedOldByNew {
		matchedOldByNew[index] = -1
	}
	var match func(int, []bool) bool
	match = func(oldIndex int, visited []bool) bool {
		for newIndex, newGroup := range newGroups {
			if visited[newIndex] || !stringSliceSubset(oldGroups[oldIndex], newGroup) {
				continue
			}
			visited[newIndex] = true
			if matchedOldByNew[newIndex] == -1 || match(matchedOldByNew[newIndex], visited) {
				matchedOldByNew[newIndex] = oldIndex
				return true
			}
		}
		return false
	}
	for oldIndex := range oldGroups {
		if !match(oldIndex, make([]bool, len(newGroups))) {
			return false
		}
	}
	return true
}

func stringSliceSubset(oldValues, newValues []string) bool {
	newSet := stringSet(newValues)
	for _, value := range oldValues {
		if !newSet[value] {
			return false
		}
	}
	return true
}

func compatiblePositionals(oldPositionals, newPositionals []positionalSchema) bool {
	if len(newPositionals) < len(oldPositionals) {
		return false
	}
	for index, oldPositional := range oldPositionals {
		newPositional := newPositionals[index]
		if oldPositional.Name != newPositional.Name ||
			oldPositional.Index != newPositional.Index ||
			oldPositional.Type != newPositional.Type {
			return false
		}
		if !oldPositional.Required && newPositional.Required {
			return false
		}
		if oldPositional.Variadic && !newPositional.Variadic {
			return false
		}
		if !oldPositional.Variadic && newPositional.Variadic && index != len(newPositionals)-1 {
			return false
		}
	}

	if len(newPositionals) == len(oldPositionals) {
		return true
	}
	if len(oldPositionals) > 0 && newPositionals[len(oldPositionals)-1].Variadic {
		return false
	}
	for index := len(oldPositionals); index < len(newPositionals); index++ {
		if newPositionals[index].Required {
			return false
		}
		if index > len(oldPositionals) && newPositionals[index-1].Variadic {
			return false
		}
	}
	return true
}

func checkParameterCompatibility(toolPath, name string, oldParameter, newParameter parameterSchema) []string {
	var failures []string
	for _, field := range []struct {
		name string
		old  string
		new  string
	}{
		{name: "type", old: oldParameter.Type, new: newParameter.Type},
		{name: "property", old: oldParameter.Property, new: newParameter.Property},
		{name: "interface_type", old: oldParameter.InterfaceType, new: newParameter.InterfaceType},
		{name: "default", old: oldParameter.Default, new: newParameter.Default},
		{name: "interface_default", old: oldParameter.InterfaceDefault, new: newParameter.InterfaceDefault},
		{name: "format", old: oldParameter.Format, new: newParameter.Format},
	} {
		if field.old != field.new {
			failures = append(failures, fmt.Sprintf("schema tool %q parameter %q changed %s", toolPath, name, field.name))
		}
	}
	if !oldParameter.Required && newParameter.Required {
		failures = append(failures, fmt.Sprintf("schema tool %q made parameter %q newly required", toolPath, name))
	}
	if !oldParameter.CLIRequired && newParameter.CLIRequired {
		failures = append(failures, fmt.Sprintf("schema tool %q made parameter %q newly cli_required", toolPath, name))
	}
	if oldParameter.RequiredWhen != newParameter.RequiredWhen && newParameter.RequiredWhen != "" {
		failures = append(failures, fmt.Sprintf("schema tool %q parameter %q changed required_when", toolPath, name))
	}
	if enumNarrowed(oldParameter.Enum, newParameter.Enum) {
		failures = append(failures, fmt.Sprintf("schema tool %q parameter %q narrowed enum", toolPath, name))
	}
	sort.Strings(failures)
	return failures
}

func enumNarrowed(oldValues, newValues []string) bool {
	if len(oldValues) == 0 {
		return len(newValues) > 0
	}
	if len(newValues) == 0 {
		return false
	}
	current := stringSet(newValues)
	for _, value := range oldValues {
		if !current[value] {
			return true
		}
	}
	return false
}

func mergeContracts(historical, current schemaContract) (schemaContract, []string) {
	failures := checkCompatibility(historical, current)
	if len(failures) > 0 {
		return cloneContract(historical), failures
	}
	return cloneContract(current), nil
}

func cloneContract(source schemaContract) schemaContract {
	data, _ := json.Marshal(source)
	var cloned schemaContract
	_ = json.Unmarshal(data, &cloned)
	return cloned
}

func writeContract(w io.Writer, contract schemaContract) error {
	contract.Version = schemaContractVersion
	if contract.Products == nil {
		contract.Products = map[string]productSchema{}
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(contract)
}

func stringSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}
