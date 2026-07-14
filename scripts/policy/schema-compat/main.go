// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

// Command schema-compat normalizes and checks the backwards-compatible
// product/tool surface returned by `dws schema --all --compact --format json`.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

type schemaContract struct {
	Version  int                      `json:"version"`
	Products map[string]productSchema `json:"products"`
}

type productSchema struct {
	Tools map[string]toolSchema `json:"tools"`
}

type toolSchema struct {
	Parameters map[string]string `json:"parameters,omitempty"`
	Required   []string          `json:"required,omitempty"`
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
	contract := schemaContract{Version: 1, Products: map[string]productSchema{}}
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
	return contract, nil
}

func normalizeTool(raw json.RawMessage) (string, toolSchema, error) {
	var tool struct {
		CanonicalPath string                     `json:"canonical_path"`
		Name          string                     `json:"name"`
		CLIName       string                     `json:"cli_name"`
		Parameters    map[string]json.RawMessage `json:"parameters"`
		Required      []string                   `json:"required"`
	}
	if err := json.Unmarshal(raw, &tool); err != nil {
		return "", toolSchema{}, err
	}
	id := firstNonEmpty(tool.CanonicalPath, tool.Name, tool.CLIName)
	if id == "" {
		return "", toolSchema{}, fmt.Errorf("tool without canonical_path/name/cli_name")
	}
	parameters := map[string]string{}
	requiredParameters := append([]string(nil), tool.Required...)
	for name, rawSchema := range tool.Parameters {
		var schema map[string]any
		if err := json.Unmarshal(rawSchema, &schema); err != nil {
			return "", toolSchema{}, fmt.Errorf("parameter %s: %w", name, err)
		}
		parameters[name] = schemaType(schema)
		if value, exists := schema["required"]; exists {
			required, ok := value.(bool)
			if !ok {
				return "", toolSchema{}, fmt.Errorf("parameter %s: required must be a boolean", name)
			}
			if required {
				requiredParameters = append(requiredParameters, name)
			}
		}
	}
	required := uniqueSorted(requiredParameters)
	return id, toolSchema{Parameters: parameters, Required: required}, nil
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
	if contract.Version != 1 {
		return schemaContract{}, fmt.Errorf("unsupported schema contract version %d", contract.Version)
	}
	if contract.Products == nil {
		contract.Products = map[string]productSchema{}
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
			for parameter, oldType := range oldTool.Parameters {
				newType, ok := newTool.Parameters[parameter]
				if !ok {
					failures = append(failures, fmt.Sprintf("schema tool %q lost parameter %q", productID+"/"+toolID, parameter))
				} else if newType != oldType {
					failures = append(failures, fmt.Sprintf("schema tool %q parameter %q changed type", productID+"/"+toolID, parameter))
				}
			}
			oldRequired := stringSet(oldTool.Required)
			for _, required := range newTool.Required {
				if !oldRequired[required] {
					failures = append(failures, fmt.Sprintf("schema tool %q made parameter %q newly required", productID+"/"+toolID, required))
				}
			}
		}
	}
	sort.Strings(failures)
	return failures
}

func mergeContracts(historical, current schemaContract) (schemaContract, []string) {
	merged := cloneContract(historical)
	var failures []string
	for productID, newProduct := range current.Products {
		oldProduct, exists := merged.Products[productID]
		if !exists {
			merged.Products[productID] = newProduct
			continue
		}
		if oldProduct.Tools == nil {
			oldProduct.Tools = map[string]toolSchema{}
		}
		for toolID, newTool := range newProduct.Tools {
			oldTool, exists := oldProduct.Tools[toolID]
			if !exists {
				oldProduct.Tools[toolID] = newTool
				continue
			}
			oldRequired := stringSet(oldTool.Required)
			for _, required := range newTool.Required {
				if !oldRequired[required] {
					failures = append(failures, fmt.Sprintf("schema tool %q made parameter %q newly required", productID+"/"+toolID, required))
				}
			}
			if oldTool.Parameters == nil {
				oldTool.Parameters = map[string]string{}
			}
			for parameter, newType := range newTool.Parameters {
				if oldType, exists := oldTool.Parameters[parameter]; exists && oldType != newType {
					failures = append(failures, fmt.Sprintf("schema tool %q parameter %q changed type", productID+"/"+toolID, parameter))
					continue
				}
				oldTool.Parameters[parameter] = newType
			}
			oldProduct.Tools[toolID] = oldTool
		}
		merged.Products[productID] = oldProduct
	}
	sort.Strings(failures)
	return merged, failures
}

func cloneContract(source schemaContract) schemaContract {
	data, _ := json.Marshal(source)
	var cloned schemaContract
	_ = json.Unmarshal(data, &cloned)
	return cloned
}

func writeContract(w io.Writer, contract schemaContract) error {
	contract.Version = 1
	if contract.Products == nil {
		contract.Products = map[string]productSchema{}
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(contract)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func uniqueSorted(values []string) []string {
	set := stringSet(values)
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func stringSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}
