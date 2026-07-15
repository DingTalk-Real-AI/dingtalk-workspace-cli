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

const schemaContractVersion = 3

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

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	var normalizePath, checkPath, mergePath, currentPath string
	flags := flag.NewFlagSet("schema-compat", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&normalizePath, "normalize", "", "normalize a raw complete Schema response")
	flags.StringVar(&checkPath, "check", "", "check against a normalized historical baseline")
	flags.StringVar(&mergePath, "merge", "", "merge additions into a normalized historical baseline")
	flags.StringVar(&currentPath, "current", "", "raw current complete Schema response")
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
		{name: "interface_ref", old: oldTool.InterfaceRef, new: newTool.InterfaceRef},
		{name: "availability", old: oldTool.Availability, new: newTool.Availability},
		{name: "constraints", old: oldTool.Constraints, new: newTool.Constraints},
		{name: "effect", old: oldTool.Effect, new: newTool.Effect},
		{name: "risk", old: oldTool.Risk, new: newTool.Risk},
		{name: "confirmation", old: oldTool.Confirmation, new: newTool.Confirmation},
		{name: "idempotency", old: oldTool.Idempotency, new: newTool.Idempotency},
	} {
		if field.old != field.new {
			failures = append(failures, fmt.Sprintf("schema tool %q changed %s", toolPath, field.name))
		}
	}
	if !equalPositionals(oldTool.Positionals, newTool.Positionals) {
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

func equalPositionals(oldPositionals, newPositionals []positionalSchema) bool {
	if len(oldPositionals) != len(newPositionals) {
		return false
	}
	for index := range oldPositionals {
		if oldPositionals[index] != newPositionals[index] {
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
