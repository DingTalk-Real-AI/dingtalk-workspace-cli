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
	"reflect"
	"sort"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/interfacesnapshot"
)

const schemaContractVersion = 3

// propertySourceReviewedMappingExclusion is the provenance source a parameter
// reports when its property is deliberately omitted through the reviewed
// mapping exclusion table (internal/cli/schema_parameter_mapping_ledger.go).
// Only this source qualifies for the property-clearing carve-out in
// checkParameterCompatibility.
const propertySourceReviewedMappingExclusion = "reviewed_mapping_exclusion"

// interfaceModeMCP is the interface_mode of a tool backed by exactly one RPC.
// Only a tool that is mcp on both sides can qualify for the interface_ref
// redirect carve-out in compatibleInterfaceRefRedirect.
const interfaceModeMCP = "mcp"

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
	PropertySource   string   `json:"property_source,omitempty"`
	InterfaceType    string   `json:"interface_type,omitempty"`
	Required         bool     `json:"required,omitempty"`
	CLIRequired      bool     `json:"cli_required,omitempty"`
	RequiredWhen     string   `json:"required_when,omitempty"`
	Default          string   `json:"default,omitempty"`
	InterfaceDefault string   `json:"interface_default,omitempty"`
	Format           string   `json:"format,omitempty"`
	Enum             []string `json:"enum,omitempty"`
}

type reviewedCompatibilityException struct {
	Field string
	Old   string
	New   string
}

// reviewedCompatibilityExceptions is intentionally exact: safety fixes may
// need to tighten a historical contract, but that must not turn arbitrary
// confirmation drift into a compatible change.
var reviewedCompatibilityExceptions = map[string]reviewedCompatibilityException{
	// PR #1085: batch permission/member remove is destructive at container
	// scope — one call can revoke access for up to 30 USER / DEPT /
	// CONVERSATION / TAG members, and departments, chats, and role groups
	// can indirectly affect many more users. The review therefore asked for
	// the same user confirmation gate as other destructive removes.
	"doc/doc.remove_permission": {
		Field: "confirmation",
		Old:   "not_required",
		New:   "user_required",
	},
	"drive/drive.permission_remove": {
		Field: "confirmation",
		Old:   "not_required",
		New:   "user_required",
	},
	"wiki/wiki.remove_member": {
		Field: "confirmation",
		Old:   "not_required",
		New:   "user_required",
	},
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	var normalizePath, checkPath, mergePath, currentPath string
	var approvedFlagMigrationsPath, candidateFlagMigrationsPath string
	var approvedCommandMigrationsPath, candidateCommandMigrationsPath string
	var migrationBaseSchemaPath string
	var migrationCurrentSnapshotPath, migrationBaseSnapshotPath, migrationStableSnapshotPath string
	flags := flag.NewFlagSet("schema-compat", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&normalizePath, "normalize", "", "normalize a raw complete Schema response")
	flags.StringVar(&checkPath, "check", "", "check against a normalized historical baseline")
	flags.StringVar(&mergePath, "merge", "", "merge additions into a normalized historical baseline")
	flags.StringVar(&currentPath, "current", "", "raw current complete Schema response")
	flags.StringVar(&approvedFlagMigrationsPath, "approved-flag-migrations", "", "base-owned approved flag migration manifest")
	flags.StringVar(&candidateFlagMigrationsPath, "candidate-flag-migrations", "", "detached candidate flag migration manifest")
	flags.StringVar(&approvedCommandMigrationsPath, "approved-command-migrations", "", "base-owned approved command migration manifest")
	flags.StringVar(&candidateCommandMigrationsPath, "candidate-command-migrations", "", "detached candidate command migration manifest")
	flags.StringVar(&migrationBaseSchemaPath, "migration-base-schema", "", "normalized merge-base Schema contract used to verify cross-migration lineage")
	flags.StringVar(&migrationCurrentSnapshotPath, "migration-current-snapshot", "", "current interface snapshot used for migration authorization")
	flags.StringVar(&migrationBaseSnapshotPath, "migration-base-snapshot", "", "merge-base interface snapshot used for migration authorization")
	flags.StringVar(&migrationStableSnapshotPath, "migration-stable-snapshot", "", "stable interface snapshot used for migration authorization")
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
	flagMigrationPair := approvedFlagMigrationsPath != "" || candidateFlagMigrationsPath != ""
	commandMigrationPair := approvedCommandMigrationsPath != "" || candidateCommandMigrationsPath != ""
	if flagMigrationPair && (approvedFlagMigrationsPath == "" || candidateFlagMigrationsPath == "") {
		fmt.Fprintln(stderr, "Schema flag migration authorization requires both flag manifests")
		return 2
	}
	if commandMigrationPair && (approvedCommandMigrationsPath == "" || candidateCommandMigrationsPath == "") {
		fmt.Fprintln(stderr, "Schema command migration authorization requires both command manifests")
		return 2
	}
	migrationSnapshots := []string{
		migrationCurrentSnapshotPath,
		migrationBaseSnapshotPath,
		migrationStableSnapshotPath,
	}
	migrationSnapshotCount := 0
	for _, path := range migrationSnapshots {
		if path != "" {
			migrationSnapshotCount++
		}
	}
	migrationsEnabled := flagMigrationPair || commandMigrationPair
	if migrationsEnabled && migrationSnapshotCount != len(migrationSnapshots) {
		fmt.Fprintln(stderr, "Schema migration authorization requires all three interface snapshots")
		return 2
	}
	if !migrationsEnabled && migrationSnapshotCount != 0 {
		fmt.Fprintln(stderr, "Schema migration snapshots require a flag or command migration manifest pair")
		return 2
	}
	if migrationsEnabled && checkPath == "" {
		fmt.Fprintln(stderr, "Schema migration authorization is only valid with --check")
		return 2
	}
	if flagMigrationPair && commandMigrationPair && migrationBaseSchemaPath == "" {
		fmt.Fprintln(stderr, "combined Schema flag and command migration authorization requires --migration-base-schema")
		return 2
	}
	if migrationBaseSchemaPath != "" && (!flagMigrationPair || !commandMigrationPair) {
		fmt.Fprintln(stderr, "--migration-base-schema requires both flag and command migration manifest pairs")
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
		var flagMigrations []interfacesnapshot.FlagMigration
		if flagMigrationPair {
			flagMigrations, err = authorizeSchemaFlagMigrations(
				approvedFlagMigrationsPath,
				candidateFlagMigrationsPath,
				migrationCurrentSnapshotPath,
				migrationBaseSnapshotPath,
				migrationStableSnapshotPath,
			)
			if err != nil {
				fmt.Fprintf(stderr, "authorize Schema flag migrations: %v\n", err)
				return 2
			}
		}
		if commandMigrationPair {
			commandMigrations, err := authorizeSchemaCommandMigrations(
				approvedCommandMigrationsPath,
				candidateCommandMigrationsPath,
				migrationCurrentSnapshotPath,
				migrationBaseSnapshotPath,
				migrationStableSnapshotPath,
			)
			if err != nil {
				fmt.Fprintf(stderr, "authorize Schema command migrations: %v\n", err)
				return 2
			}
			if flagMigrationPair {
				migrationBase, readErr := readContract(migrationBaseSchemaPath)
				if readErr != nil {
					fmt.Fprintf(stderr, "read migration merge-base Schema contract: %v\n", readErr)
					return 2
				}
				baseline, err = normalizeSchemaCommandMigrationLineage(
					baseline,
					migrationBase,
					current,
					flagMigrations,
					commandMigrations,
				)
			} else {
				baseline, err = normalizeSchemaCommandMigrations(baseline, current, commandMigrations)
			}
			if err != nil {
				fmt.Fprintf(stderr, "normalize approved Schema command migrations: %v\n", err)
				return 2
			}
		} else if flagMigrationPair {
			baseline, err = normalizeSchemaFlagMigrations(baseline, current, flagMigrations)
			if err != nil {
				fmt.Fprintf(stderr, "normalize approved Schema flag migrations: %v\n", err)
				return 2
			}
		}
		failures := checkCompatibility(baseline, current)
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

func authorizeSchemaFlagMigrations(
	approvedManifestPath string,
	candidateManifestPath string,
	currentSnapshotPath string,
	baseSnapshotPath string,
	stableSnapshotPath string,
) ([]interfacesnapshot.FlagMigration, error) {
	approved, err := readFlagMigrationManifestFile(approvedManifestPath)
	if err != nil {
		return nil, fmt.Errorf("read approved flag migrations: %w", err)
	}
	candidate, err := readFlagMigrationManifestFile(candidateManifestPath)
	if err != nil {
		return nil, fmt.Errorf("read candidate flag migrations: %w", err)
	}
	current, err := readInterfaceSnapshotFile(currentSnapshotPath)
	if err != nil {
		return nil, fmt.Errorf("read migration current snapshot: %w", err)
	}
	base, err := readInterfaceSnapshotFile(baseSnapshotPath)
	if err != nil {
		return nil, fmt.Errorf("read migration base snapshot: %w", err)
	}
	stable, err := readInterfaceSnapshotFile(stableSnapshotPath)
	if err != nil {
		return nil, fmt.Errorf("read migration stable snapshot: %w", err)
	}
	return interfacesnapshot.AuthorizeFlagMigrations(
		current,
		map[string]interfacesnapshot.Snapshot{"merge-base": base, "stable": stable},
		approved,
		candidate,
	)
}

func readFlagMigrationManifestFile(path string) (interfacesnapshot.FlagMigrationManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return interfacesnapshot.FlagMigrationManifest{}, err
	}
	return interfacesnapshot.ReadFlagMigrationManifest(bytes.NewReader(data))
}

func authorizeSchemaCommandMigrations(
	approvedManifestPath string,
	candidateManifestPath string,
	currentSnapshotPath string,
	baseSnapshotPath string,
	stableSnapshotPath string,
) ([]interfacesnapshot.CommandMigration, error) {
	approved, err := readCommandMigrationManifestFile(approvedManifestPath)
	if err != nil {
		return nil, fmt.Errorf("read approved command migrations: %w", err)
	}
	candidate, err := readCommandMigrationManifestFile(candidateManifestPath)
	if err != nil {
		return nil, fmt.Errorf("read candidate command migrations: %w", err)
	}
	current, err := readInterfaceSnapshotFile(currentSnapshotPath)
	if err != nil {
		return nil, fmt.Errorf("read migration current snapshot: %w", err)
	}
	base, err := readInterfaceSnapshotFile(baseSnapshotPath)
	if err != nil {
		return nil, fmt.Errorf("read migration base snapshot: %w", err)
	}
	stable, err := readInterfaceSnapshotFile(stableSnapshotPath)
	if err != nil {
		return nil, fmt.Errorf("read migration stable snapshot: %w", err)
	}
	return interfacesnapshot.AuthorizeCommandMigrations(
		current,
		map[string]interfacesnapshot.Snapshot{"merge-base": base, "stable": stable},
		approved,
		candidate,
	)
}

func readCommandMigrationManifestFile(path string) (interfacesnapshot.CommandMigrationManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return interfacesnapshot.CommandMigrationManifest{}, err
	}
	return interfacesnapshot.ReadCommandMigrationManifest(bytes.NewReader(data))
}

func readInterfaceSnapshotFile(path string) (interfacesnapshot.Snapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return interfacesnapshot.Snapshot{}, err
	}
	return interfacesnapshot.Read(bytes.NewReader(data))
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
		FieldProvenance  struct {
			Property struct {
				Source string `json:"source"`
			} `json:"property"`
		} `json:"field_provenance"`
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
		PropertySource:   strings.TrimSpace(parameter.FieldProvenance.Property.Source),
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
			failures = append(failures, checkToolCompatibility(toolPath, oldTool, newTool)...)
		}
	}
	sort.Strings(failures)
	return failures
}

func checkToolCompatibility(toolPath string, oldTool, newTool toolSchema) []string {
	var failures []string
	for _, field := range []struct {
		name string
		old  string
		new  string
	}{
		{name: "primary_cli_path", old: oldTool.PrimaryCLIPath, new: newTool.PrimaryCLIPath},
		{name: "interface_mode", old: oldTool.InterfaceMode, new: newTool.InterfaceMode},
		{name: "availability", old: oldTool.Availability, new: newTool.Availability},
		{name: "effect", old: oldTool.Effect, new: newTool.Effect},
		{name: "risk", old: oldTool.Risk, new: newTool.Risk},
		{name: "confirmation", old: oldTool.Confirmation, new: newTool.Confirmation},
		{name: "idempotency", old: oldTool.Idempotency, new: newTool.Idempotency},
	} {
		if field.old != field.new && !isReviewedCompatibilityException(toolPath, field.name, field.old, field.new) {
			failures = append(failures, fmt.Sprintf("schema tool %q changed %s", toolPath, field.name))
		}
	}
	if oldTool.Constraints != newTool.Constraints &&
		!compatibleHiddenSiblingConstraintExpansion(oldTool, newTool) &&
		!compatibleAdditiveConstraintEvolution(oldTool, newTool) &&
		!compatibleReviewedConstraintTransition(toolPath, oldTool, newTool) {
		failures = append(failures, fmt.Sprintf("schema tool %q changed constraints", toolPath))
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

	// interface_ref is evaluated last, because the redirect carve-out below is
	// conditional on every other check for this tool having passed.
	if oldTool.InterfaceRef != newTool.InterfaceRef &&
		!compatibleInterfaceRefRedirect(toolPath, oldTool, newTool, failures) {
		failures = append(failures, fmt.Sprintf("schema tool %q changed interface_ref", toolPath))
	}
	sort.Strings(failures)
	return failures
}

func isReviewedCompatibilityException(toolPath, field, oldValue, newValue string) bool {
	exception, ok := reviewedCompatibilityExceptions[toolPath]
	return ok && exception.Field == field && exception.Old == oldValue && exception.New == newValue
}

// reviewedInterfaceRefRedirect enumerates the exact, individually reviewed
// backend RPC migrations this gate accepts. Schema shape alone cannot prove two
// RPCs share business semantics, permissions, error behaviour, or side effects,
// so a redirect is only accepted when the specific tool and the specific
// old→new pair appear here. Any other ref change is still reported.
//
// Keyed by tool path ("<product>/<tool id>"), then by the previous
// interface_ref, with the value being the single accepted new ref. Both refs are
// the canonicalized interface_ref JSON exactly as parseTool produces it (see
// canonicalRawJSON) — not a bare RPC name. Adding an entry is a contract
// decision and belongs in review, not in a feature change.
var reviewedInterfaceRefRedirect = map[string]map[string]string{
	// The style surface moved from update_range (which writes values) to
	// set_cell_range's cellStyles payload (style-only, preserving values). This
	// is the only channel that can express italic / underline / strike-through /
	// font family / borders. Same product, same target range semantics, same
	// permission scope; the flat style properties it loses are accepted
	// separately as reviewed mapping exclusions.
	"sheet/sheet.range_set_style": {
		`{"product_id":"sheet","rpc_name":"update_range"}`: `{"product_id":"sheet","rpc_name":"set_cell_range"}`,
	},
}

// reviewedConstraintTransition enumerates exact constraint changes that were
// explicitly reviewed even though the compatibility gate cannot infer their
// business semantics. Keep this narrow: removing a constraint can expose a new
// runtime route, so arbitrary removals must not pass as harmless drift.
var reviewedConstraintTransition = map[string]map[string]string{
	// PR #933 aligned the Shortcut and Schema with the existing doc import
	// runtime: omitting both targets imports into the default root. Keeping the
	// historical require_one_of made the documented Golden Route unreachable.
	"doc/doc.shortcut_import": {
		`{"require_one_of":[["folder","workspace"]]}`: "",
	},
	// PR #1105 adds local --file as an alternative to the historically required
	// --src input. Every historical --src invocation remains valid; publishing
	// both groups makes the final Schema express the runtime's exact-one rule.
	"sheet/sheet.create_float_image": {
		"": `{"mutually_exclusive":[["file","src"]],"require_one_of":[["file","src"]]}`,
	},
}

func compatibleReviewedConstraintTransition(toolPath string, oldTool, newTool toolSchema) bool {
	transitions, ok := reviewedConstraintTransition[toolPath]
	if !ok {
		return false
	}
	want, ok := transitions[oldTool.Constraints]
	if !ok {
		return false
	}
	return want == newTool.Constraints
}

// compatibleInterfaceRefRedirect accepts repointing a tool at a different
// backing RPC when the migration is an explicitly reviewed entry in
// reviewedInterfaceRefRedirect **and** the CLI-facing contract is provably
// unchanged.
//
// interface_ref is audit and traceability metadata: it records which RPC backs a
// leaf. Nothing reads it at runtime — the tool a leaf invokes is decided in the
// CLI source, so a stale ref does not misroute a call, it only misinforms a
// reader. When the backing RPC genuinely moves, the honest options are to update
// the ref or to keep publishing a name that no longer matches the request being
// sent.
//
// Being audit-only is why a reviewed redirect can be accepted at all; it is not
// a reason to accept redirects in general. Two RPCs with compatible Schema
// parameters may still differ in permissions, quota, error taxonomy, or side
// effects, none of which this gate can see. Hence the allowlist below, plus:
//
//   - interface_mode is unchanged and stays "mcp". A move to or from
//     "composite" is a change in kind, not a redirect, and is still reported.
//   - both refs are non-empty. Removing a ref is not a redirect.
//   - no other compatibility failure was recorded for this tool. This is the
//     operative meaning of "the CLI contract is unchanged": no parameter was
//     lost, none became required, no type / default / format / enum moved, no
//     constraint tightened, no positional or dry_run change. Any one of those
//     re-reports the redirect, so the exemption cannot smuggle a surface change
//     in behind a backend move.
//
// A cleared property that resolved through a reviewed mapping exclusion is
// already accepted by checkParameterCompatibility and so does not block this;
// that pairing is expected, since a leaf moving to a nested payload loses its
// flat property names in the same change.
func compatibleInterfaceRefRedirect(toolPath string, oldTool, newTool toolSchema, otherFailures []string) bool {
	if oldTool.InterfaceMode != newTool.InterfaceMode || newTool.InterfaceMode != interfaceModeMCP {
		return false
	}
	if oldTool.InterfaceRef == "" || newTool.InterfaceRef == "" {
		return false
	}
	if reviewedInterfaceRefRedirect[toolPath][oldTool.InterfaceRef] != newTool.InterfaceRef {
		return false
	}
	return len(otherFailures) == 0
}

// compatibleAdditiveConstraintEvolution accepts constraint evolution that
// cannot invalidate an invocation expressible by the historical public
// parameter contract. Existing groups may only gain members; additions to a
// mutually-exclusive or require-together group must not be historical public
// parameters, because that would reject an invocation expressible by the old
// contract. Adding a member to require-one-of only loosens the group. A newly
// added mutually-exclusive group is safe when it contains at most one
// historical public parameter: aliases and newly added parameters could not
// have appeared together in an old invocation. A new require-together group is
// safe only when it contains no historical public parameter. A new
// require-one-of group always adds a requirement and is therefore incompatible.
func compatibleAdditiveConstraintEvolution(oldTool, newTool toolSchema) bool {
	oldGroups, okOld := parseConstraintGroups(oldTool.Constraints)
	newGroups, okNew := parseConstraintGroups(newTool.Constraints)
	if !okOld || !okNew {
		return false
	}
	for _, key := range []string{"mutually_exclusive", "require_one_of", "require_together"} {
		used := make([]bool, len(newGroups[key]))
		for _, oldGroup := range oldGroups[key] {
			oldSet := stringSet(oldGroup)
			if len(oldSet) == 0 {
				return false
			}
			matched := false
			for index, newGroup := range newGroups[key] {
				newSet := stringSet(newGroup)
				if used[index] || !stringSetContainsAll(newSet, oldSet) {
					continue
				}
				if key == "mutually_exclusive" || key == "require_together" {
					safe := true
					for member := range newSet {
						if oldSet[member] {
							continue
						}
						if _, historical := oldTool.Parameters[member]; historical {
							safe = false
							break
						}
					}
					if !safe {
						continue
					}
				}
				used[index] = true
				matched = true
				break
			}
			if !matched {
				return false
			}
		}
		for index, newGroup := range newGroups[key] {
			if used[index] {
				continue
			}
			historicalMembers := 0
			for member := range stringSet(newGroup) {
				if _, existed := oldTool.Parameters[member]; existed {
					historicalMembers++
				}
			}
			switch key {
			case "mutually_exclusive":
				if historicalMembers > 1 {
					return false
				}
			case "require_together":
				if historicalMembers > 0 {
					return false
				}
			default: // require_one_of
				return false
			}
		}
	}
	return true
}

// compatibleHiddenSiblingConstraintExpansion allows declare≡execute repairs:
// Schema may start projecting full constraint groups that include unpublished
// (hidden) execute-side siblings when the previous contract collapsed the sole
// published member to required and omitted constraints.
func compatibleHiddenSiblingConstraintExpansion(oldTool, newTool toolSchema) bool {
	if strings.TrimSpace(oldTool.Constraints) != "" {
		return false
	}
	var projected struct {
		MutuallyExclusive [][]string `json:"mutually_exclusive"`
		RequireOneOf      [][]string `json:"require_one_of"`
		RequireTogether   [][]string `json:"require_together"`
	}
	if err := json.Unmarshal([]byte(newTool.Constraints), &projected); err != nil {
		return false
	}
	if len(projected.RequireTogether) > 0 || len(projected.RequireOneOf) == 0 {
		return false
	}
	groups := append([][]string(nil), projected.RequireOneOf...)
	groups = append(groups, projected.MutuallyExclusive...)
	for _, group := range groups {
		if len(group) < 2 {
			return false
		}
		published := 0
		hidden := 0
		for _, name := range group {
			name = strings.TrimSpace(name)
			if name == "" {
				return false
			}
			if _, ok := newTool.Parameters[name]; ok {
				published++
			} else {
				hidden++
			}
		}
		if published == 0 || hidden == 0 {
			return false
		}
		// Former collapse artifact: exactly one published member was required.
		if published == 1 {
			var sole string
			for _, name := range group {
				if _, ok := newTool.Parameters[name]; ok {
					sole = name
					break
				}
			}
			oldParam, ok := oldTool.Parameters[sole]
			newParam, okNew := newTool.Parameters[sole]
			if !ok || !okNew || !oldParam.Required || newParam.Required {
				return false
			}
		}
	}
	return true
}

func parseConstraintGroups(raw string) (map[string][][]string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string][][]string{}, true
	}
	var projected struct {
		MutuallyExclusive [][]string `json:"mutually_exclusive"`
		RequireOneOf      [][]string `json:"require_one_of"`
		RequireTogether   [][]string `json:"require_together"`
	}
	if err := json.Unmarshal([]byte(raw), &projected); err != nil {
		return nil, false
	}
	return map[string][][]string{
		"mutually_exclusive": projected.MutuallyExclusive,
		"require_one_of":     projected.RequireOneOf,
		"require_together":   projected.RequireTogether,
	}, true
}

func stringSetContainsAll(superset, subset map[string]bool) bool {
	for value := range subset {
		if !superset[value] {
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

// parameterTypeChange identifies one published parameter type migration
// exactly: which tool, which parameter, and in which direction. Nothing is
// matched by wildcard — an entry differing in any of the four fields does not
// apply.
//
// ToolPath is "<product id>/<tool id>" exactly as the comparison builds it (see
// where toolPath is assembled), e.g.
// "minutes/minutes.apply_minutes_permission". From and To are the canonical
// type values schemaType produces — the JSON encoding of the "type" keyword,
// so `"string"` **with** its quotes, not a bare string. Spelling either one any
// other way silently disables the exemption instead of failing loudly (exactly
// how reviewedInterfaceRefRedirect was broken twice), so
// TestCrossPlatformCoverageReviewedParameterTypeChangeKeysAreCanonical
// recomputes both through schemaType rather than trusting the table.
type parameterTypeChange struct {
	ToolPath  string
	Parameter string
	From      string
	To        string
}

// reviewedParameterTypeChanges enumerates the individually reviewed parameter
// type migrations this gate accepts. A published type is part of the Schema
// contract and a swap cannot be proven safe from the type names alone, so a
// migration is accepted only when this exact tool, parameter and old→new pair
// appear here. Adding an entry is a contract decision and belongs in review,
// not in a feature change.
//
// Entries are direction-sensitive by construction: "string" → "integer" is a
// separate key from "integer" → "string", and only the reviewed direction is
// accepted.
//
// The table alone does not admit a change: the migration is rejected unless
// every other published field of the parameter is identical — not merely free
// of compatibility failures. See compatibleReviewedParameterTypeChange.
var reviewedParameterTypeChanges = map[parameterTypeChange]struct{}{
	// "dws minutes permission apply --policy" moved from a String flag to a
	// native Int flag, so the published type follows it from "string" to
	// "integer". A parameter's type is projected from the Cobra flag type
	// (provenance source cobra_flag_type), so it describes how the CLI accepts
	// the value, not how the backing RPC receives it.
	//
	// Accepted because no historical caller is invalidated:
	//
	//   - Consumers read this Schema to construct CLI invocations, and a command
	//     line is text either way: "--policy 4" is the same argv under both
	//     declarations, and a quoted "--policy \"4\"" still reaches pflag as 4.
	//   - The flag keeps enforcing the same [2,4] domain in RunE, and pflag's
	//     base-0 parse accepts every base-10 spelling the previous
	//     strconv.ParseInt(v, 10, 64) accepted, so the set of successful
	//     invocations only widens.
	//   - "integer" also matches what actually goes on the wire: this parameter
	//     maps to property "policyId", which the command has always sent as a
	//     number, so the new declaration is closer to the request than the old.
	//
	// The parameter's default stays absent on both sides here (a zero default is
	// not published) and every other published field is identical, which is why
	// the equality guard admits this entry. A migration that also moved the
	// default — or relaxed required, cleared required_when, widened enum, cleared
	// interface_type — would be rejected even though several of those are
	// individually compatible, because none of them were reviewed under this
	// entry.
	{
		ToolPath:  "minutes/minutes.apply_minutes_permission",
		Parameter: "policy",
		From:      `"string"`,
		To:        `"integer"`,
	}: {},
}

// parameterContractUnchangedExceptType reports whether every published field of
// the parameter other than its type is identical.
//
// This is a full equality check rather than "the gate recorded no other
// failure", because several field changes are individually *compatible* and so
// produce no failure at all: relaxing required or cli_required, clearing
// required_when, widening enum, clearing interface_type, and clearing property
// through a reviewed mapping exclusion. Keying the carve-out on the failure list
// would let any of those ride along with a reviewed type migration without ever
// having been reviewed under that entry — the exemption would be wider than what
// it documents.
//
// Comparing the struct also means a field added to parameterSchema later is
// covered here automatically, instead of silently widening every existing entry.
func parameterContractUnchangedExceptType(oldParameter, newParameter parameterSchema) bool {
	oldParameter.Type = ""
	newParameter.Type = ""
	return reflect.DeepEqual(oldParameter, newParameter)
}

// compatibleReviewedParameterTypeChange accepts a published parameter type
// migration when it is an explicitly reviewed entry **and** the rest of the
// parameter's published contract is unchanged.
func compatibleReviewedParameterTypeChange(toolPath, name string, oldParameter, newParameter parameterSchema) bool {
	_, reviewed := reviewedParameterTypeChanges[parameterTypeChange{
		ToolPath:  toolPath,
		Parameter: name,
		From:      oldParameter.Type,
		To:        newParameter.Type,
	}]
	return reviewed && parameterContractUnchangedExceptType(oldParameter, newParameter)
}

func checkParameterCompatibility(toolPath, name string, oldParameter, newParameter parameterSchema) []string {
	var failures []string
	if oldParameter.Type != newParameter.Type &&
		!compatibleReviewedParameterTypeChange(toolPath, name, oldParameter, newParameter) {
		failures = append(failures, fmt.Sprintf("schema tool %q parameter %q changed type", toolPath, name))
	}
	for _, field := range []struct {
		name string
		old  string
		new  string
	}{
		{name: "default", old: oldParameter.Default, new: newParameter.Default},
		{name: "interface_default", old: oldParameter.InterfaceDefault, new: newParameter.InterfaceDefault},
		{name: "format", old: oldParameter.Format, new: newParameter.Format},
	} {
		if field.old != field.new {
			failures = append(failures, fmt.Sprintf("schema tool %q parameter %q changed %s", toolPath, name, field.name))
		}
	}
	// Clearing property is accepted as compatible only when the new value is
	// omitted through a reviewed mapping exclusion. This is the same shape as
	// the interface_type carve-out below: a declaration that the flag has no
	// single top-level RPC property, replacing a value that is no longer true.
	//
	// It exists because a leaf whose backing RPC gains a nested payload has no
	// honest flat property to publish. The alternative outcomes are both worse:
	// keep naming a field the request no longer contains, or let assembly fall
	// back to flag_name_inference and publish a name that appears in no request
	// at all.
	//
	// The predicate is deliberately narrow:
	//   - old non-empty AND new empty (a redirect to a different non-empty
	//     value stays a contract break)
	//   - the new value resolved through reviewed_mapping_exclusion, so an
	//     accidental or silent drop is still reported
	//
	// The exclusion table cannot be abused to wave through arbitrary clearing:
	// internal/cli/schema_parameter_bindings.go verifies that every parameter
	// claiming an exclusion really does deliver an empty property, and every
	// entry carries a non-empty reviewed reason.
	//
	// Consumers must treat a missing property as "no direct mapping" and read
	// the provenance reason for where the value actually lands. Re-populating a
	// property requires an explicit ParamDecl declaration.
	if oldParameter.Property != newParameter.Property &&
		!(oldParameter.Property != "" && newParameter.Property == "" &&
			newParameter.PropertySource == propertySourceReviewedMappingExclusion) {
		failures = append(failures, fmt.Sprintf("schema tool %q parameter %q changed property", toolPath, name))
	}
	// Clearing interface_type is accepted as compatible: a deliberate,
	// wire-visible policy decision taken with the pinned MCP metadata
	// retirement. Production no longer projects MCP-sourced types unless
	// ParamDecl declares them, so unverifiable pinned values are dropped
	// rather than kept. Consumers that used interface_type for coercion must
	// treat a missing value as "unknown" — re-populating a value requires an
	// explicit ParamDecl declaration, not a new pin. Changing to a different
	// non-empty value remains a contract break.
	if oldParameter.InterfaceType != newParameter.InterfaceType &&
		!(oldParameter.InterfaceType != "" && newParameter.InterfaceType == "") {
		failures = append(failures, fmt.Sprintf("schema tool %q parameter %q changed interface_type", toolPath, name))
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

type schemaToolRef struct {
	productID string
	toolID    string
}

// normalizeSchemaFlagMigrations projects only an already-authorized CLI flag
// rename or requiredness change onto a cloned historical Schema contract. The
// ordinary compatibility checker still makes the final decision; this adapter
// never drops findings by matching their rendered text.
func normalizeSchemaFlagMigrations(
	baseline schemaContract,
	current schemaContract,
	migrations []interfacesnapshot.FlagMigration,
) (schemaContract, error) {
	normalized := cloneContract(baseline)
	renamesByTool := map[schemaToolRef]map[string]string{}

	for _, migration := range migrations {
		if migration.EffectiveKind() == interfacesnapshot.FlagMigrationRequirednessChange {
			var err error
			normalized, err = normalizeSchemaFlagRequirednessMigration(normalized, baseline, current, migration)
			if err != nil {
				return schemaContract{}, err
			}
			continue
		}
		primaryPath := strings.TrimPrefix(migration.Command, "dws ")
		matches := schemaToolsByPrimaryPath(baseline, primaryPath)
		if len(matches) == 0 {
			// A reviewed CLI-only command has no Schema compatibility surface.
			continue
		}
		if len(matches) != 1 {
			return schemaContract{}, fmt.Errorf(
				"approved flag migration %q matches %d historical Schema tools",
				migration.Command,
				len(matches),
			)
		}

		ref := matches[0]
		oldTool := baseline.Products[ref.productID].Tools[ref.toolID]
		newProduct, productExists := current.Products[ref.productID]
		newTool, toolExists := newProduct.Tools[ref.toolID]
		if !productExists || !toolExists || newTool.PrimaryCLIPath != primaryPath {
			// Preserve the original baseline so the ordinary checker reports the
			// missing tool or changed primary_cli_path.
			continue
		}
		oldLegacy, legacyExisted := oldTool.Parameters[migration.Legacy.Name]
		oldCanonical, canonicalExisted := oldTool.Parameters[migration.Canonical.Name]
		if !legacyExisted && !canonicalExisted {
			// A CLI migration is not evidence that an unrelated Schema surface
			// published either name. In particular, it must not authorize a
			// constraints rewrite with no historical parameter to rename.
			continue
		}

		if _, exists := newTool.Parameters[migration.Legacy.Name]; exists {
			return schemaContract{}, fmt.Errorf(
				"approved flag migration %q still publishes legacy Schema parameter %q",
				migration.Command,
				migration.Legacy.Name,
			)
		}
		newCanonical, exists := newTool.Parameters[migration.Canonical.Name]
		if !exists {
			return schemaContract{}, fmt.Errorf(
				"approved flag migration %q does not publish canonical Schema parameter %q",
				migration.Command,
				migration.Canonical.Name,
			)
		}
		if migration.Canonical.After.Required && (!newCanonical.Required || !newCanonical.CLIRequired) {
			return schemaContract{}, fmt.Errorf(
				"approved flag migration %q canonical Schema parameter %q must be required and cli_required",
				migration.Command,
				migration.Canonical.Name,
			)
		}
		if !legacyExisted {
			// A consumed receipt may be evaluated after the merge-base Schema has
			// already reached the canonical-only state. In that case the ordinary
			// checker needs no projection. More importantly, a CLI migration is not
			// authority to promote a canonical-only historical Schema parameter from
			// optional to required: without legacy Schema evidence there is no rename.
			continue
		}

		normalizedProduct := normalized.Products[ref.productID]
		normalizedTool := normalizedProduct.Tools[ref.toolID]
		if err := validateRenamedSchemaParameter(migration, oldLegacy, newCanonical); err != nil {
			return schemaContract{}, err
		}
		delete(normalizedTool.Parameters, migration.Legacy.Name)
		if canonicalExisted {
			if err := validateRenamedSchemaParameter(migration, oldCanonical, newCanonical); err != nil {
				return schemaContract{}, err
			}
			normalizedCanonical := normalizedTool.Parameters[migration.Canonical.Name]
			normalizedCanonical.Required = newCanonical.Required
			normalizedCanonical.CLIRequired = newCanonical.CLIRequired
			normalizedTool.Parameters[migration.Canonical.Name] = normalizedCanonical
		} else {
			normalizedTool.Parameters[migration.Canonical.Name] = newCanonical
		}
		normalizedProduct.Tools[ref.toolID] = normalizedTool
		normalized.Products[ref.productID] = normalizedProduct

		if renamesByTool[ref] == nil {
			renamesByTool[ref] = map[string]string{}
		}
		if existing, ok := renamesByTool[ref][migration.Legacy.Name]; ok && existing != migration.Canonical.Name {
			return schemaContract{}, fmt.Errorf(
				"approved flag migration %q maps Schema parameter %q to both %q and %q",
				migration.Command,
				migration.Legacy.Name,
				existing,
				migration.Canonical.Name,
			)
		}
		renamesByTool[ref][migration.Legacy.Name] = migration.Canonical.Name
	}

	for ref, renames := range renamesByTool {
		oldTool := baseline.Products[ref.productID].Tools[ref.toolID]
		newTool := current.Products[ref.productID].Tools[ref.toolID]
		if oldTool.Constraints == newTool.Constraints {
			continue
		}
		oldConstraints, oldOK := canonicalizeMigratedConstraints(oldTool.Constraints, renames)
		newConstraints, newOK := canonicalizeMigratedConstraints(newTool.Constraints, nil)
		if !oldOK || !newOK || oldConstraints != newConstraints {
			// Keep the historical constraints unchanged. The ordinary checker
			// will report every non-rename constraint change.
			continue
		}
		product := normalized.Products[ref.productID]
		tool := product.Tools[ref.toolID]
		tool.Constraints = newTool.Constraints
		product.Tools[ref.toolID] = tool
		normalized.Products[ref.productID] = product
	}

	return normalized, nil
}

func normalizeSchemaFlagRequirednessMigration(
	normalized schemaContract,
	baseline schemaContract,
	current schemaContract,
	migration interfacesnapshot.FlagMigration,
) (schemaContract, error) {
	primaryPath := strings.TrimPrefix(migration.Command, "dws ")
	matches := schemaToolsByPrimaryPath(baseline, primaryPath)
	if len(matches) == 0 {
		// A reviewed CLI-only command has no Schema compatibility surface.
		return normalized, nil
	}
	if len(matches) != 1 {
		return schemaContract{}, fmt.Errorf(
			"approved flag requiredness migration %q matches %d historical Schema tools",
			migration.Command,
			len(matches),
		)
	}

	ref := matches[0]
	oldTool := baseline.Products[ref.productID].Tools[ref.toolID]
	oldParameter, existed := oldTool.Parameters[migration.Flag.Name]
	if !existed {
		// CLI requiredness is not authority to create or mutate an unrelated
		// historical Schema parameter.
		return normalized, nil
	}
	newProduct, productExists := current.Products[ref.productID]
	newTool, toolExists := newProduct.Tools[ref.toolID]
	if !productExists || !toolExists || newTool.PrimaryCLIPath != primaryPath {
		// Preserve the baseline so the ordinary checker reports the missing tool
		// or primary path change.
		return normalized, nil
	}
	newParameter, exists := newTool.Parameters[migration.Flag.Name]
	if !exists {
		// Preserve the baseline so the ordinary checker reports parameter loss.
		return normalized, nil
	}
	if !newParameter.Required || !newParameter.CLIRequired {
		return schemaContract{}, fmt.Errorf(
			"approved flag requiredness migration %q Schema parameter %q must be required and cli_required",
			migration.Command,
			migration.Flag.Name,
		)
	}

	normalizedProduct := normalized.Products[ref.productID]
	normalizedTool := normalizedProduct.Tools[ref.toolID]
	normalizedParameter := oldParameter
	normalizedParameter.Required = newParameter.Required
	normalizedParameter.CLIRequired = newParameter.CLIRequired
	normalizedTool.Parameters[migration.Flag.Name] = normalizedParameter
	normalizedProduct.Tools[ref.toolID] = normalizedTool
	normalized.Products[ref.productID] = normalizedProduct
	return normalized, nil
}

// normalizeSchemaCommandMigrationLineage composes two independently reviewed
// migration receipts without inventing a second alias authority. A consumed
// flag migration may supply the historical predecessor of a command_move
// parameter, but only after the merge-base Schema or a retained consumed
// command receipt proves the corresponding next hop.
func normalizeSchemaCommandMigrationLineage(
	historical schemaContract,
	mergeBase schemaContract,
	current schemaContract,
	flagMigrations []interfacesnapshot.FlagMigration,
	commandMigrations []interfacesnapshot.CommandMigration,
) (schemaContract, error) {
	normalized, err := normalizeSchemaFlagMigrations(historical, current, flagMigrations)
	if err != nil {
		return schemaContract{}, fmt.Errorf("normalize ordinary flag migrations: %w", err)
	}
	staged, err := stageSchemaCommandMigrationPredecessors(
		normalized,
		mergeBase,
		current,
		flagMigrations,
		commandMigrations,
	)
	if err != nil {
		return schemaContract{}, err
	}
	return normalizeSchemaCommandMigrations(staged, current, commandMigrations)
}

// stageSchemaCommandMigrationPredecessors replays only the name-changing edge
// recorded by a base-owned consumed flag migration. It leaves interface,
// safety, dry-run, and positional facts untouched so the ordinary checker
// remains authoritative for every non-name change.
func stageSchemaCommandMigrationPredecessors(
	historical schemaContract,
	mergeBase schemaContract,
	current schemaContract,
	flagMigrations []interfacesnapshot.FlagMigration,
	commandMigrations []interfacesnapshot.CommandMigration,
) (schemaContract, error) {
	staged := cloneContract(historical)
	moveBySource := map[schemaToolRef]string{}
	for _, migration := range commandMigrations {
		if migration.Kind != interfacesnapshot.CommandMigrationMove {
			continue
		}
		ref := schemaToolRef{productID: migration.Schema.ProductID, toolID: migration.Schema.SourceToolID}
		if previous, exists := moveBySource[ref]; exists {
			return schemaContract{}, fmt.Errorf(
				"approved command migrations fork Schema source tool %q between %q and %q",
				migration.Schema.SourceToolID,
				previous,
				migration.Legacy.Command,
			)
		}
		moveBySource[ref] = migration.Legacy.Command
	}

	for _, migration := range commandMigrations {
		if migration.Kind != interfacesnapshot.CommandMigrationMove {
			continue
		}
		oldProduct, productExists := historical.Products[migration.Schema.ProductID]
		oldTool, toolExists := oldProduct.Tools[migration.Schema.SourceToolID]
		if !productExists || !toolExists {
			continue
		}
		legacyPath := strings.TrimPrefix(migration.Legacy.Command, "dws ")
		replacementPath := strings.TrimPrefix(migration.Replacement.Command, "dws ")
		if oldTool.PrimaryCLIPath == replacementPath {
			continue
		}
		if oldTool.PrimaryCLIPath != legacyPath {
			// Preserve the existing command normalizer's deterministic path error.
			continue
		}

		baseProduct, baseProductExists := mergeBase.Products[migration.Schema.ProductID]
		baseTool, baseToolExists := baseProduct.Tools[migration.Schema.SourceToolID]
		currentProduct, currentProductExists := current.Products[migration.Schema.ProductID]
		currentTool, currentToolExists := currentProduct.Tools[migration.Schema.SourceToolID]
		firstHopRenames := map[string]string{}
		composedRenames := map[string]string{}
		lineageApplied := false

		for _, parameter := range migration.Schema.Parameters {
			if _, direct := oldTool.Parameters[parameter.From]; direct {
				for _, flagMigration := range flagMigrations {
					if flagMigration.EffectiveKind() != interfacesnapshot.FlagMigrationRename {
						continue
					}
					if flagMigration.Command != migration.Legacy.Command ||
						flagMigration.Canonical.Name != parameter.From {
						continue
					}
					if _, legacyAlsoPublished := oldTool.Parameters[flagMigration.Legacy.Name]; legacyAlsoPublished {
						return schemaContract{}, fmt.Errorf(
							"approved command migration %q historical Schema tool publishes both predecessor %q and intermediate %q",
							migration.Legacy.Command,
							flagMigration.Legacy.Name,
							parameter.From,
						)
					}
				}
				composedRenames[parameter.From] = parameter.To
				continue
			}

			predecessors := make([]interfacesnapshot.FlagMigration, 0, 1)
			for _, flagMigration := range flagMigrations {
				if flagMigration.EffectiveKind() != interfacesnapshot.FlagMigrationRename {
					continue
				}
				if flagMigration.Command != migration.Legacy.Command ||
					flagMigration.Canonical.Name != parameter.From {
					continue
				}
				if flagMigration.State != interfacesnapshot.FlagMigrationConsumed {
					return schemaContract{}, fmt.Errorf(
						"approved command migration %q Schema predecessor %q -> %q requires a consumed flag migration receipt",
						migration.Legacy.Command,
						flagMigration.Legacy.Name,
						parameter.From,
					)
				}
				if _, published := oldTool.Parameters[flagMigration.Legacy.Name]; published {
					predecessors = append(predecessors, flagMigration)
				}
			}
			if len(predecessors) == 0 {
				continue
			}

			lineageApplied = true
			if !baseProductExists || !baseToolExists {
				return schemaContract{}, fmt.Errorf(
					"approved command migration %q merge-base Schema lacks source tool %q",
					migration.Legacy.Command,
					migration.Schema.SourceToolID,
				)
			}
			var stagedParameter parameterSchema
			switch migration.State {
			case interfacesnapshot.CommandMigrationPending:
				if baseTool.PrimaryCLIPath != legacyPath {
					return schemaContract{}, fmt.Errorf(
						"pending command migration %q merge-base Schema source tool has primary_cli_path %q",
						migration.Legacy.Command,
						baseTool.PrimaryCLIPath,
					)
				}
				intermediate, exists := baseTool.Parameters[parameter.From]
				if !exists {
					return schemaContract{}, fmt.Errorf(
						"pending command migration %q merge-base Schema lacks intermediate parameter %q",
						migration.Legacy.Command,
						parameter.From,
					)
				}
				if _, exists := baseTool.Parameters[parameter.To]; exists {
					return schemaContract{}, fmt.Errorf(
						"pending command migration %q merge-base Schema already publishes final parameter %q",
						migration.Legacy.Command,
						parameter.To,
					)
				}
				stagedParameter = intermediate
			case interfacesnapshot.CommandMigrationConsumed:
				if baseTool.PrimaryCLIPath != replacementPath {
					return schemaContract{}, fmt.Errorf(
						"consumed command migration %q merge-base Schema source tool has primary_cli_path %q",
						migration.Legacy.Command,
						baseTool.PrimaryCLIPath,
					)
				}
				finalParameter, exists := baseTool.Parameters[parameter.To]
				if !exists {
					return schemaContract{}, fmt.Errorf(
						"consumed command migration %q merge-base Schema lacks final parameter %q",
						migration.Legacy.Command,
						parameter.To,
					)
				}
				if _, exists := baseTool.Parameters[parameter.From]; exists {
					return schemaContract{}, fmt.Errorf(
						"consumed command migration %q merge-base Schema still publishes intermediate parameter %q",
						migration.Legacy.Command,
						parameter.From,
					)
				}
				stagedParameter = oldTool.Parameters[predecessors[0].Legacy.Name]
				for _, predecessor := range predecessors {
					oldParameter := oldTool.Parameters[predecessor.Legacy.Name]
					composite := interfacesnapshot.CommandParameterMigration{From: predecessor.Legacy.Name, To: parameter.To}
					if err := validateEquivalentCommandSchemaParameter(migration, composite, oldParameter, finalParameter); err != nil {
						return schemaContract{}, err
					}
				}
			default:
				return schemaContract{}, fmt.Errorf(
					"approved command migration %q has unsupported lineage state %q",
					migration.Legacy.Command,
					migration.State,
				)
			}

			stagedProduct := staged.Products[migration.Schema.ProductID]
			stagedTool := stagedProduct.Tools[migration.Schema.SourceToolID]
			for _, predecessor := range predecessors {
				if predecessor.Legacy.Name == parameter.To {
					return schemaContract{}, fmt.Errorf(
						"approved command migration %q forms a Schema parameter lineage cycle through %q",
						migration.Legacy.Command,
						parameter.To,
					)
				}
				if existing, claimed := firstHopRenames[predecessor.Legacy.Name]; claimed && existing != parameter.From {
					return schemaContract{}, fmt.Errorf(
						"approved command migration %q forks Schema predecessor %q to both %q and %q",
						migration.Legacy.Command,
						predecessor.Legacy.Name,
						existing,
						parameter.From,
					)
				}
				oldParameter := oldTool.Parameters[predecessor.Legacy.Name]
				if migration.State == interfacesnapshot.CommandMigrationPending {
					if err := validateRenamedSchemaParameter(predecessor, oldParameter, stagedParameter); err != nil {
						return schemaContract{}, err
					}
				}
				if _, exists := baseTool.Parameters[predecessor.Legacy.Name]; exists {
					return schemaContract{}, fmt.Errorf(
						"approved command migration %q merge-base Schema still publishes predecessor parameter %q",
						migration.Legacy.Command,
						predecessor.Legacy.Name,
					)
				}
				if currentProductExists && currentToolExists {
					if _, exists := currentTool.Parameters[predecessor.Legacy.Name]; exists {
						return schemaContract{}, fmt.Errorf(
							"approved command migration %q current Schema still publishes predecessor parameter %q",
							migration.Legacy.Command,
							predecessor.Legacy.Name,
						)
					}
				}
				delete(stagedTool.Parameters, predecessor.Legacy.Name)
				firstHopRenames[predecessor.Legacy.Name] = parameter.From
				composedRenames[predecessor.Legacy.Name] = parameter.To
			}
			stagedTool.Parameters[parameter.From] = stagedParameter
			stagedProduct.Tools[migration.Schema.SourceToolID] = stagedTool
			staged.Products[migration.Schema.ProductID] = stagedProduct
		}

		if !lineageApplied {
			continue
		}
		matches := schemaToolsByPrimaryPath(historical, legacyPath)
		wantRef := schemaToolRef{productID: migration.Schema.ProductID, toolID: migration.Schema.SourceToolID}
		if len(matches) != 1 || matches[0] != wantRef {
			return schemaContract{}, fmt.Errorf(
				"approved command migration %q predecessor lineage requires one exact historical Schema tool, got %#v",
				migration.Legacy.Command,
				matches,
			)
		}
		if currentProductExists && currentToolExists {
			if source, found := migratedConstraintSourceParameter(currentTool.Constraints, firstHopRenames); found {
				return schemaContract{}, fmt.Errorf(
					"approved command migration %q current Schema constraints still reference predecessor parameter %q",
					migration.Legacy.Command,
					source,
				)
			}
		}
		for _, finalTarget := range composedRenames {
			if _, cycle := firstHopRenames[finalTarget]; cycle {
				return schemaContract{}, fmt.Errorf(
					"approved command migration %q forms a Schema parameter lineage cycle through %q",
					migration.Legacy.Command,
					finalTarget,
				)
			}
		}

		firstHopConstraints, firstHopOK := canonicalizeMigratedConstraints(oldTool.Constraints, firstHopRenames)
		if !firstHopOK {
			return schemaContract{}, fmt.Errorf(
				"approved command migration %q historical Schema constraints are not canonicalizable",
				migration.Legacy.Command,
			)
		}
		baseConstraints, baseOK := canonicalizeMigratedConstraints(baseTool.Constraints, nil)
		if !baseOK {
			return schemaContract{}, fmt.Errorf(
				"approved command migration %q merge-base Schema constraints are not canonicalizable",
				migration.Legacy.Command,
			)
		}
		stagedProduct := staged.Products[migration.Schema.ProductID]
		stagedTool := stagedProduct.Tools[migration.Schema.SourceToolID]
		switch migration.State {
		case interfacesnapshot.CommandMigrationPending:
			if firstHopConstraints != baseConstraints {
				return schemaContract{}, fmt.Errorf(
					"pending command migration %q predecessor lineage changed merge-base Schema constraints",
					migration.Legacy.Command,
				)
			}
			stagedTool.Constraints = baseTool.Constraints
		case interfacesnapshot.CommandMigrationConsumed:
			composedConstraints, composedOK := canonicalizeMigratedConstraints(oldTool.Constraints, composedRenames)
			if !composedOK || composedConstraints != baseConstraints {
				return schemaContract{}, fmt.Errorf(
					"consumed command migration %q predecessor lineage changed merge-base Schema constraints",
					migration.Legacy.Command,
				)
			}
			stagedTool.Constraints = firstHopConstraints
		}
		stagedProduct.Tools[migration.Schema.SourceToolID] = stagedTool
		staged.Products[migration.Schema.ProductID] = stagedProduct
	}
	return staged, nil
}

// normalizeSchemaCommandMigrations projects only the Schema consequences that
// are coupled to an already-authorized CLI command migration. It rewrites a
// cloned historical contract; the ordinary checker still rejects every field
// not proven equivalent here.
func normalizeSchemaCommandMigrations(
	baseline schemaContract,
	current schemaContract,
	migrations []interfacesnapshot.CommandMigration,
) (schemaContract, error) {
	normalized := cloneContract(baseline)
	for _, migration := range migrations {
		oldProduct, productExists := baseline.Products[migration.Schema.ProductID]
		oldTool, toolExists := oldProduct.Tools[migration.Schema.SourceToolID]
		if !productExists || !toolExists {
			// The historical baseline predates this Schema tool, so it has no
			// compatibility surface for this migration.
			continue
		}
		newProduct, productExists := current.Products[migration.Schema.ProductID]
		newSource, sourceExists := newProduct.Tools[migration.Schema.SourceToolID]
		if !productExists || !sourceExists {
			// Preserve the baseline so the ordinary checker reports the removal.
			continue
		}
		legacyPath := strings.TrimPrefix(migration.Legacy.Command, "dws ")
		replacementPath := strings.TrimPrefix(migration.Replacement.Command, "dws ")
		if migration.Kind == interfacesnapshot.CommandMigrationFlagExtraction &&
			migration.State == interfacesnapshot.CommandMigrationConsumed {
			_, historicalPublishesLegacy := oldTool.Parameters[migration.LegacyFlag.Name]
			_, currentPublishesLegacy := newSource.Parameters[migration.LegacyFlag.Name]
			historicalReplacement, historicalReplacementExists := oldProduct.Tools[migration.Schema.ReplacementToolID]
			currentReplacement, currentReplacementExists := newProduct.Tools[migration.Schema.ReplacementToolID]
			if oldTool.PrimaryCLIPath == legacyPath &&
				newSource.PrimaryCLIPath == legacyPath &&
				!historicalPublishesLegacy &&
				!currentPublishesLegacy &&
				historicalReplacementExists &&
				historicalReplacement.PrimaryCLIPath == replacementPath &&
				currentReplacementExists &&
				currentReplacement.PrimaryCLIPath == replacementPath {
				// The merge-base has already reached the extraction's after state.
				// Retain the receipt for older stable baselines without replaying it
				// against the already-extracted source tool.
				continue
			}
		}
		if oldTool.PrimaryCLIPath == replacementPath {
			// A consumed receipt can still be needed for the stable baseline after
			// main has already reached the after state.
			continue
		}
		if oldTool.PrimaryCLIPath != legacyPath {
			return schemaContract{}, fmt.Errorf(
				"approved command migration %s historical Schema tool %q has primary_cli_path %q",
				migration.Kind,
				migration.Schema.SourceToolID,
				oldTool.PrimaryCLIPath,
			)
		}

		normalizedProduct := normalized.Products[migration.Schema.ProductID]
		normalizedTool := normalizedProduct.Tools[migration.Schema.SourceToolID]
		switch migration.Kind {
		case interfacesnapshot.CommandMigrationAvailability:
			change := migration.Schema.Availability
			if change != nil && oldTool.Availability == change.After && newSource.Availability == change.After {
				continue
			}
			if change == nil || oldTool.Availability != change.Before || newSource.Availability != change.After {
				return schemaContract{}, fmt.Errorf(
					"approved availability hardening %q does not match Schema availability %q -> %q",
					migration.Legacy.Command,
					oldTool.Availability,
					newSource.Availability,
				)
			}
			if newSource.PrimaryCLIPath != legacyPath {
				continue
			}
			normalizedTool.Availability = newSource.Availability

		case interfacesnapshot.CommandMigrationMove:
			if newSource.PrimaryCLIPath != replacementPath {
				continue
			}
			renames := make(map[string]string, len(migration.Schema.Parameters))
			renameTargets := make(map[string]struct{}, len(migration.Schema.Parameters))
			for _, parameter := range migration.Schema.Parameters {
				oldParameter, existed := oldTool.Parameters[parameter.From]
				if !existed {
					return schemaContract{}, fmt.Errorf(
						"approved command migration %q historical Schema tool lacks parameter %q",
						migration.Legacy.Command,
						parameter.From,
					)
				}
				if _, exists := oldTool.Parameters[parameter.To]; exists {
					return schemaContract{}, fmt.Errorf(
						"approved command migration %q Schema parameter target %q already exists in historical Schema tool %q",
						migration.Legacy.Command,
						parameter.To,
						migration.Schema.SourceToolID,
					)
				}
				if _, exists := newSource.Parameters[parameter.From]; exists {
					return schemaContract{}, fmt.Errorf(
						"approved command migration %q still publishes legacy Schema parameter %q",
						migration.Legacy.Command,
						parameter.From,
					)
				}
				newParameter, exists := newSource.Parameters[parameter.To]
				if !exists {
					return schemaContract{}, fmt.Errorf(
						"approved command migration %q does not publish replacement Schema parameter %q",
						migration.Replacement.Command,
						parameter.To,
					)
				}
				if err := validateEquivalentCommandSchemaParameter(migration, parameter, oldParameter, newParameter); err != nil {
					return schemaContract{}, err
				}
				delete(normalizedTool.Parameters, parameter.From)
				normalizedTool.Parameters[parameter.To] = newParameter
				renames[parameter.From] = parameter.To
				renameTargets[parameter.To] = struct{}{}
			}
			parameterNames := make([]string, 0, len(newSource.Parameters))
			for name := range newSource.Parameters {
				parameterNames = append(parameterNames, name)
			}
			sort.Strings(parameterNames)
			for _, name := range parameterNames {
				parameter := newSource.Parameters[name]
				if _, existed := oldTool.Parameters[name]; existed {
					continue
				}
				if _, approvedRename := renameTargets[name]; approvedRename {
					continue
				}
				if parameter.Required || parameter.CLIRequired || parameter.RequiredWhen != "" {
					return schemaContract{}, fmt.Errorf(
						"approved command migration %q replacement Schema tool %q introduced unregistered required Schema parameter %q",
						migration.Legacy.Command,
						migration.Schema.SourceToolID,
						name,
					)
				}
			}
			oldConstraints, oldOK := canonicalizeMigratedConstraints(oldTool.Constraints, renames)
			if !oldOK {
				return schemaContract{}, fmt.Errorf(
					"approved command migration %q historical Schema constraints are not canonicalizable",
					migration.Legacy.Command,
				)
			}
			newConstraints, newOK := canonicalizeMigratedConstraints(newSource.Constraints, nil)
			if !newOK {
				return schemaContract{}, fmt.Errorf(
					"approved command migration %q current Schema constraints are not canonicalizable",
					migration.Legacy.Command,
				)
			}
			if source, found := migratedConstraintSourceParameter(newSource.Constraints, renames); found {
				return schemaContract{}, fmt.Errorf(
					"approved command migration %q current constraints still reference legacy Schema constraint parameter %q",
					migration.Legacy.Command,
					source,
				)
			}
			normalizedTool.Constraints = oldConstraints
			if oldConstraints == newConstraints {
				normalizedTool.Constraints = newSource.Constraints
			}
			normalizedTool.PrimaryCLIPath = replacementPath

		case interfacesnapshot.CommandMigrationFlagExtraction:
			if newSource.PrimaryCLIPath != legacyPath {
				continue
			}
			replacement, exists := newProduct.Tools[migration.Schema.ReplacementToolID]
			if !exists || replacement.PrimaryCLIPath != replacementPath {
				return schemaContract{}, fmt.Errorf(
					"approved flag extraction %q lacks replacement Schema tool %q at %q",
					migration.Legacy.Command,
					migration.Schema.ReplacementToolID,
					replacementPath,
				)
			}
			if oldTool.InterfaceMode != replacement.InterfaceMode ||
				oldTool.InterfaceRef != replacement.InterfaceRef ||
				oldTool.Availability != replacement.Availability ||
				oldTool.Effect != replacement.Effect ||
				oldTool.Risk != replacement.Risk ||
				oldTool.Confirmation != replacement.Confirmation ||
				oldTool.Idempotency != replacement.Idempotency {
				return schemaContract{}, fmt.Errorf(
					"approved flag extraction %q replacement Schema tool changed interface or safety identity",
					migration.Legacy.Command,
				)
			}
			if oldTool.DryRun != "" && oldTool.DryRun != replacement.DryRun {
				return schemaContract{}, fmt.Errorf(
					"approved flag extraction %q replacement Schema tool changed or removed dry_run",
					migration.Legacy.Command,
				)
			}
			var err error
			normalizedTool, err = normalizeFlagExtractionSchemaMigration(
				migration,
				oldTool,
				newSource,
				replacement,
				normalizedTool,
			)
			if err != nil {
				return schemaContract{}, err
			}
		}
		normalizedProduct.Tools[migration.Schema.SourceToolID] = normalizedTool
		normalized.Products[migration.Schema.ProductID] = normalizedProduct
	}
	return normalized, nil
}

func normalizeFlagExtractionSchemaMigration(
	migration interfacesnapshot.CommandMigration,
	oldTool toolSchema,
	newSource toolSchema,
	replacement toolSchema,
	normalizedSource toolSchema,
) (toolSchema, error) {
	mappingsBySource := make(map[string]interfacesnapshot.CommandParameterMigration, len(migration.Schema.Parameters))
	for _, mapping := range migration.Schema.Parameters {
		if _, duplicate := mappingsBySource[mapping.From]; duplicate {
			return toolSchema{}, fmt.Errorf(
				"approved flag extraction %q maps historical Schema parameter %q more than once",
				migration.Legacy.Command,
				mapping.From,
			)
		}
		if _, exists := oldTool.Parameters[mapping.From]; !exists {
			return toolSchema{}, fmt.Errorf(
				"approved flag extraction %q historical Schema tool lacks parameter %q",
				migration.Legacy.Command,
				mapping.From,
			)
		}
		mappingsBySource[mapping.From] = mapping
	}

	historicalNames := make([]string, 0, len(oldTool.Parameters))
	for name := range oldTool.Parameters {
		historicalNames = append(historicalNames, name)
	}
	sort.Strings(historicalNames)
	for _, name := range historicalNames {
		if _, mapped := mappingsBySource[name]; !mapped {
			return toolSchema{}, fmt.Errorf(
				"approved flag extraction %q does not map historical Schema parameter %q",
				migration.Legacy.Command,
				name,
			)
		}
	}

	legacyName := migration.LegacyFlag.Name
	legacyMapping, mapped := mappingsBySource[legacyName]
	if !mapped || legacyMapping.To != "" || legacyMapping.ReplacementConstant == nil {
		return toolSchema{}, fmt.Errorf(
			"approved flag extraction %q legacy Schema parameter %q must map to one replacement constant",
			migration.Legacy.Command,
			legacyName,
		)
	}
	legacyParameter := oldTool.Parameters[legacyName]
	if err := validateFlagExtractionBooleanConstant(migration, legacyParameter, legacyMapping.ReplacementConstant); err != nil {
		return toolSchema{}, err
	}
	constantProperty := legacyMapping.ReplacementConstant.Property
	if _, exists := newSource.Parameters[legacyName]; exists {
		return toolSchema{}, fmt.Errorf(
			"approved flag extraction %q still publishes extracted Schema parameter %q",
			migration.Legacy.Command,
			legacyName,
		)
	}
	sourceNames := make([]string, 0, len(newSource.Parameters))
	for name := range newSource.Parameters {
		sourceNames = append(sourceNames, name)
	}
	sort.Strings(sourceNames)
	for _, name := range sourceNames {
		if newSource.Parameters[name].Property == constantProperty {
			return toolSchema{}, fmt.Errorf(
				"approved flag extraction %q source Schema tool still publishes replacement constant property %q through parameter %q on %q",
				migration.Legacy.Command,
				constantProperty,
				name,
				migration.Schema.SourceToolID,
			)
		}
	}

	replacementNames := make([]string, 0, len(replacement.Parameters))
	for name := range replacement.Parameters {
		replacementNames = append(replacementNames, name)
	}
	sort.Strings(replacementNames)
	for _, name := range replacementNames {
		if replacement.Parameters[name].Property == constantProperty {
			return toolSchema{}, fmt.Errorf(
				"approved flag extraction %q replacement Schema tool %q still publishes replacement constant property %q through parameter %q",
				migration.Legacy.Command,
				migration.Schema.ReplacementToolID,
				constantProperty,
				name,
			)
		}
	}

	replacementTargets := make(map[string]string, len(historicalNames)-1)
	renames := make(map[string]string, len(historicalNames)-1)
	for _, sourceName := range historicalNames {
		if sourceName == legacyName {
			continue
		}
		mapping := mappingsBySource[sourceName]
		if mapping.To == "" || mapping.ReplacementConstant != nil {
			return toolSchema{}, fmt.Errorf(
				"approved flag extraction %q shared Schema parameter %q must map to one replacement parameter",
				migration.Legacy.Command,
				sourceName,
			)
		}
		if previous, duplicate := replacementTargets[mapping.To]; duplicate {
			return toolSchema{}, fmt.Errorf(
				"approved flag extraction %q replacement Schema parameter %q is mapped from both %q and %q",
				migration.Legacy.Command,
				mapping.To,
				previous,
				sourceName,
			)
		}
		replacementParameter, exists := replacement.Parameters[mapping.To]
		if !exists {
			return toolSchema{}, fmt.Errorf(
				"approved flag extraction %q replacement Schema tool %q does not publish mapped Schema parameter %q",
				migration.Legacy.Command,
				migration.Schema.ReplacementToolID,
				mapping.To,
			)
		}
		if err := validateEquivalentCommandSchemaParameter(migration, mapping, oldTool.Parameters[sourceName], replacementParameter); err != nil {
			return toolSchema{}, err
		}
		replacementTargets[mapping.To] = sourceName
		renames[sourceName] = mapping.To
	}
	for _, name := range replacementNames {
		if _, mapped := replacementTargets[name]; !mapped {
			return toolSchema{}, fmt.Errorf(
				"approved flag extraction %q replacement Schema tool %q publishes unmapped Schema parameter %q",
				migration.Legacy.Command,
				migration.Schema.ReplacementToolID,
				name,
			)
		}
	}

	oldConstraints, oldConstraintsOK := canonicalizeMigratedConstraints(oldTool.Constraints, renames)
	replacementConstraints, replacementConstraintsOK := canonicalizeMigratedConstraints(replacement.Constraints, nil)
	if !oldConstraintsOK || !replacementConstraintsOK || oldConstraints != replacementConstraints {
		return toolSchema{}, fmt.Errorf(
			"approved flag extraction %q replacement Schema tool changed constraints",
			migration.Legacy.Command,
		)
	}
	if !equivalentMigratedPositionals(oldTool.Positionals, replacement.Positionals, renames) {
		return toolSchema{}, fmt.Errorf(
			"approved flag extraction %q replacement Schema tool changed positionals",
			migration.Legacy.Command,
		)
	}

	delete(normalizedSource.Parameters, legacyName)
	return normalizedSource, nil
}

func validateFlagExtractionBooleanConstant(
	migration interfacesnapshot.CommandMigration,
	parameter parameterSchema,
	constant *interfacesnapshot.CommandReplacementConstant,
) error {
	if strings.TrimSpace(constant.Property) == "" || parameter.Property != constant.Property {
		return fmt.Errorf(
			"approved flag extraction %q replacement constant property %q does not match historical Schema property %q",
			migration.Legacy.Command,
			constant.Property,
			parameter.Property,
		)
	}
	if !constant.Value {
		return fmt.Errorf(
			"approved flag extraction %q replacement constant for Schema property %q must be constant true",
			migration.Legacy.Command,
			constant.Property,
		)
	}
	if parameter.Type != `"boolean"` {
		return fmt.Errorf(
			"approved flag extraction %q legacy Schema parameter %q must have boolean type",
			migration.Legacy.Command,
			migration.LegacyFlag.Name,
		)
	}
	if parameter.Required || parameter.CLIRequired {
		return fmt.Errorf(
			"approved flag extraction %q legacy Schema parameter %q must remain optional",
			migration.Legacy.Command,
			migration.LegacyFlag.Name,
		)
	}
	if parameter.RequiredWhen != "" {
		return fmt.Errorf(
			"approved flag extraction %q legacy Schema parameter %q must not declare required_when",
			migration.Legacy.Command,
			migration.LegacyFlag.Name,
		)
	}
	if parameter.Default != "" && parameter.Default != "false" {
		return fmt.Errorf(
			"approved flag extraction %q legacy Schema parameter %q default must be absent or false",
			migration.Legacy.Command,
			migration.LegacyFlag.Name,
		)
	}
	if parameter.InterfaceDefault != "" && parameter.InterfaceDefault != "false" {
		return fmt.Errorf(
			"approved flag extraction %q legacy Schema parameter %q interface_default must be absent or false",
			migration.Legacy.Command,
			migration.LegacyFlag.Name,
		)
	}
	if parameter.InterfaceType != "" && parameter.InterfaceType != "boolean" {
		return fmt.Errorf(
			"approved flag extraction %q legacy Schema parameter %q interface_type must be empty or boolean",
			migration.Legacy.Command,
			migration.LegacyFlag.Name,
		)
	}
	if parameter.Format != "" {
		return fmt.Errorf(
			"approved flag extraction %q legacy Schema parameter %q format must be empty",
			migration.Legacy.Command,
			migration.LegacyFlag.Name,
		)
	}
	if len(parameter.Enum) > 0 {
		allowsTrue := false
		for _, value := range parameter.Enum {
			if value == "true" {
				allowsTrue = true
				break
			}
		}
		if !allowsTrue {
			return fmt.Errorf(
				"approved flag extraction %q legacy Schema parameter %q enum must allow true",
				migration.Legacy.Command,
				migration.LegacyFlag.Name,
			)
		}
	}
	return nil
}

func equivalentMigratedPositionals(
	historical []positionalSchema,
	replacement []positionalSchema,
	renames map[string]string,
) bool {
	if len(historical) != len(replacement) {
		return false
	}
	for index, positional := range historical {
		if renamed := renames[positional.Name]; renamed != "" {
			positional.Name = renamed
		}
		if positional != replacement[index] {
			return false
		}
	}
	return true
}

func validateEquivalentCommandSchemaParameter(
	migration interfacesnapshot.CommandMigration,
	parameter interfacesnapshot.CommandParameterMigration,
	oldParameter parameterSchema,
	newParameter parameterSchema,
) error {
	if oldParameter.Type != newParameter.Type ||
		oldParameter.Property != newParameter.Property ||
		oldParameter.InterfaceType != newParameter.InterfaceType ||
		oldParameter.Required != newParameter.Required ||
		oldParameter.CLIRequired != newParameter.CLIRequired ||
		oldParameter.RequiredWhen != newParameter.RequiredWhen ||
		oldParameter.Default != newParameter.Default ||
		oldParameter.InterfaceDefault != newParameter.InterfaceDefault ||
		oldParameter.Format != newParameter.Format ||
		!equalStringSlices(oldParameter.Enum, newParameter.Enum) {
		return fmt.Errorf(
			"approved command migration %q Schema parameter %q -> %q changed a non-name field",
			migration.Legacy.Command,
			parameter.From,
			parameter.To,
		)
	}
	return nil
}

func schemaToolsByPrimaryPath(contract schemaContract, primaryPath string) []schemaToolRef {
	var matches []schemaToolRef
	for productID, product := range contract.Products {
		for toolID, tool := range product.Tools {
			if tool.PrimaryCLIPath == primaryPath {
				matches = append(matches, schemaToolRef{productID: productID, toolID: toolID})
			}
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].productID != matches[j].productID {
			return matches[i].productID < matches[j].productID
		}
		return matches[i].toolID < matches[j].toolID
	})
	return matches
}

func validateRenamedSchemaParameter(
	migration interfacesnapshot.FlagMigration,
	oldParameter parameterSchema,
	newParameter parameterSchema,
) error {
	// The migration authorizes only the CLI spelling change. Requiredness is
	// part of the parameter contract in both projections and must remain exact.
	if oldParameter.Type != newParameter.Type ||
		oldParameter.Property != newParameter.Property ||
		oldParameter.InterfaceType != newParameter.InterfaceType ||
		oldParameter.RequiredWhen != newParameter.RequiredWhen ||
		oldParameter.Default != newParameter.Default ||
		oldParameter.InterfaceDefault != newParameter.InterfaceDefault ||
		oldParameter.Format != newParameter.Format ||
		!equalStringSlices(oldParameter.Enum, newParameter.Enum) {
		return fmt.Errorf(
			"approved flag migration %q Schema parameter %q -> %q changed a non-migration field",
			migration.Command,
			migration.Legacy.Name,
			migration.Canonical.Name,
		)
	}
	if oldParameter.Required != newParameter.Required {
		return fmt.Errorf(
			"approved flag migration %q Schema parameter %q -> %q changed requiredness",
			migration.Command,
			migration.Legacy.Name,
			migration.Canonical.Name,
		)
	}
	if oldParameter.CLIRequired != newParameter.CLIRequired {
		return fmt.Errorf(
			"approved flag migration %q Schema parameter %q -> %q changed cli_required",
			migration.Command,
			migration.Legacy.Name,
			migration.Canonical.Name,
		)
	}
	return nil
}

func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func canonicalizeMigratedConstraints(raw string, renames map[string]string) (string, bool) {
	groups, ok := parseMigrationConstraintsStrict(raw)
	if !ok {
		return "", false
	}
	normalized := map[string][][]string{}
	for _, kind := range []string{"mutually_exclusive", "require_one_of", "require_together"} {
		for _, group := range groups[kind] {
			members := make([]string, 0, len(group))
			for _, member := range group {
				if replacement := renames[member]; replacement != "" {
					member = replacement
				}
				members = append(members, member)
			}
			sort.Strings(members)
			members = compactConstraintStrings(members)
			normalized[kind] = append(normalized[kind], members)
		}
		sort.Slice(normalized[kind], func(i, j int) bool {
			return strings.Join(normalized[kind][i], "\x00") < strings.Join(normalized[kind][j], "\x00")
		})
		normalized[kind] = compactConstraintGroups(normalized[kind])
		if len(normalized[kind]) == 0 {
			delete(normalized, kind)
		}
	}
	if len(normalized) == 0 {
		return "", true
	}
	encoded, err := json.Marshal(normalized)
	return string(encoded), err == nil
}

func migratedConstraintSourceParameter(raw string, renames map[string]string) (string, bool) {
	groups, ok := parseMigrationConstraintsStrict(raw)
	if !ok {
		return "", false
	}
	for _, kind := range []string{"mutually_exclusive", "require_one_of", "require_together"} {
		for _, group := range groups[kind] {
			for _, member := range group {
				if _, renamed := renames[member]; renamed {
					return member, true
				}
			}
		}
	}
	return "", false
}

func parseMigrationConstraintsStrict(raw string) (map[string][][]string, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return map[string][][]string{}, true
	}
	decoder := json.NewDecoder(strings.NewReader(trimmed))
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return nil, false
	}
	groups := map[string][][]string{}
	for decoder.More() {
		fieldToken, err := decoder.Token()
		if err != nil {
			return nil, false
		}
		// encoding/json guarantees that an object member name is a string token.
		field := fieldToken.(string)
		switch field {
		case "mutually_exclusive", "require_one_of", "require_together":
		default:
			return nil, false
		}
		if _, duplicate := groups[field]; duplicate {
			return nil, false
		}
		var encodedGroups json.RawMessage
		if err := decoder.Decode(&encodedGroups); err != nil {
			return nil, false
		}
		encodedGroups = bytes.TrimSpace(encodedGroups)
		if len(encodedGroups) == 0 || encodedGroups[0] != '[' {
			return nil, false
		}
		var fieldGroups [][]string
		if err := json.Unmarshal(encodedGroups, &fieldGroups); err != nil {
			return nil, false
		}
		for _, group := range fieldGroups {
			if len(group) == 0 {
				return nil, false
			}
			for _, member := range group {
				if member == "" || member != strings.TrimSpace(member) {
					return nil, false
				}
			}
		}
		groups[field] = fieldGroups
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') {
		return nil, false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, false
	}
	return groups, true
}

func compactConstraintStrings(values []string) []string {
	if len(values) < 2 {
		return values
	}
	out := values[:1]
	for _, value := range values[1:] {
		if value != out[len(out)-1] {
			out = append(out, value)
		}
	}
	return out
}

func compactConstraintGroups(groups [][]string) [][]string {
	if len(groups) < 2 {
		return groups
	}
	out := groups[:1]
	for _, group := range groups[1:] {
		if !equalStringSlices(group, out[len(out)-1]) {
			out = append(out, group)
		}
	}
	return out
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
