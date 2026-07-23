// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// schema_catalog.json is generated, but every command entry it delivers to
// Agents must follow one unified closed structure: a fixed required core plus
// a whitelisted optional set. Any new field must be registered here first,
// which keeps the delivered command data structure uniform across products.
// Command identity (names/paths) is owned by schema_command_registry.json and
// is deliberately out of scope for this validator.

const schemaCatalogStructureMaxViolations = 25

// schemaCatalogToolRequiredKeys is the required core present on every tool
// entry. Keep sorted; ValidateCatalogStructure rejects entries missing any of
// these.
var schemaCatalogToolRequiredKeys = []string{
	"agent_metadata_source",
	"agent_source_refs",
	"agent_summary",
	"agent_summary_source",
	"availability",
	"avoid_when",
	"canonical_path",
	"cli_name",
	"cli_path",
	"confirmation",
	"description",
	"display",
	"effect",
	"effect_source",
	"examples",
	"field_provenance",
	"has_parameters",
	"idempotency",
	"interface_mode",
	"is_alias",
	"name",
	"parameter_count",
	"parameters",
	"path",
	"primary_cli_path",
	"product_id",
	"reviewed",
	"risk",
	"source",
	"title",
	"use_when",
}

// schemaCatalogToolOptionalKeys is the optional whitelist. interface_ref and
// interface_reason are mutually exclusive and gated by interface_mode.
var schemaCatalogToolOptionalKeys = []string{
	"aliases",
	"constraints",
	"dry_run",
	"group",
	"interface_reason",
	"interface_ref",
	"metadata_source",
	"positionals",
}

var schemaCatalogToolEnums = map[string][]string{
	"effect":         {"read", "write", "destructive"},
	"risk":           {"low", "medium", "high"},
	"confirmation":   {"not_required", "user_required"},
	"interface_mode": {InterfaceModeMCP, InterfaceModeComposite, InterfaceModeLocal},
	"availability":   {InterfaceAvailable, InterfaceUnavailable},
}

// schemaCatalogParamRequiredKeys is the required core of every parameter.
var schemaCatalogParamRequiredKeys = []string{
	"description",
	"field_provenance",
	"required",
	"type",
}

// schemaCatalogParamOptionalKeys is the parameter optional whitelist.
var schemaCatalogParamOptionalKeys = []string{
	"cli_required",
	"default",
	"enum",
	"example",
	"format",
	"interface_description",
	"interface_type",
	"property",
	"required_when",
}

var schemaCatalogParamTypes = []string{"string", "integer", "number", "boolean", "array", "object"}

type schemaCatalogStructureViolation struct {
	tool    string
	message string
}

// ValidateCatalogStructure checks that every tool entry in a schema_catalog
// snapshot conforms to the unified command data structure. It returns an
// error aggregating up to schemaCatalogStructureMaxViolations violations.
func ValidateCatalogStructure(data []byte) error {
	var snapshot struct {
		Version int                       `json:"version"`
		Tools   map[string]map[string]any `json:"tools"`
	}
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return fmt.Errorf("decode schema catalog: %w", err)
	}
	if snapshot.Version != SchemaCatalogSnapshotVersion {
		return fmt.Errorf("schema catalog version = %d, want %d", snapshot.Version, SchemaCatalogSnapshotVersion)
	}
	if len(snapshot.Tools) == 0 {
		return fmt.Errorf("schema catalog has no tools")
	}

	violations := []schemaCatalogStructureViolation{}
	for toolID, entry := range snapshot.Tools {
		validateCatalogToolEntry(toolID, entry, &violations)
	}
	if len(violations) == 0 {
		return nil
	}
	sort.Slice(violations, func(i, j int) bool {
		if violations[i].tool != violations[j].tool {
			return violations[i].tool < violations[j].tool
		}
		return violations[i].message < violations[j].message
	})
	var b strings.Builder
	shown := violations
	truncated := 0
	if len(shown) > schemaCatalogStructureMaxViolations {
		truncated = len(shown) - schemaCatalogStructureMaxViolations
		shown = shown[:schemaCatalogStructureMaxViolations]
	}
	for _, v := range shown {
		fmt.Fprintf(&b, "\n  %s: %s", v.tool, v.message)
	}
	if truncated > 0 {
		fmt.Fprintf(&b, "\n  ... and %d more violations", truncated)
	}
	return fmt.Errorf("schema catalog entries violate the unified command structure (%d total):%s", len(violations), b.String())
}

func validateCatalogToolEntry(toolID string, entry map[string]any, violations *[]schemaCatalogStructureViolation) {
	report := func(format string, args ...any) {
		*violations = append(*violations, schemaCatalogStructureViolation{
			tool:    toolID,
			message: fmt.Sprintf(format, args...),
		})
	}

	allowed := make(map[string]bool, len(schemaCatalogToolRequiredKeys)+len(schemaCatalogToolOptionalKeys))
	for _, k := range schemaCatalogToolRequiredKeys {
		allowed[k] = true
	}
	for _, k := range schemaCatalogToolOptionalKeys {
		allowed[k] = true
	}
	for key := range entry {
		if !allowed[key] {
			report("unknown field %q (register it in schema_catalog_structure.go first)", key)
		}
	}

	stringKeys := []string{
		"agent_metadata_source", "agent_summary", "agent_summary_source",
		"availability", "canonical_path", "cli_name", "cli_path", "confirmation",
		"description", "display", "effect", "effect_source", "idempotency",
		"interface_mode", "name", "path", "primary_cli_path", "product_id",
		"risk", "source", "title",
	}
	for _, key := range stringKeys {
		if !requireNonEmptyString(entry, key) {
			report("field %q must be a non-empty string", key)
		}
	}
	stringArrayKeys := []string{"agent_source_refs", "avoid_when", "examples", "use_when"}
	for _, key := range stringArrayKeys {
		if !requireStringArray(entry, key) {
			report("field %q must be an array of strings", key)
		}
	}
	for _, key := range []string{"has_parameters", "is_alias", "reviewed"} {
		if _, ok := entry[key].(bool); !ok {
			report("field %q must be a boolean", key)
		}
	}
	if _, ok := entry["field_provenance"].(map[string]any); !ok {
		report("field %q must be an object", "field_provenance")
	}
	for field, values := range schemaCatalogToolEnums {
		raw, ok := entry[field].(string)
		if !ok {
			continue // already reported as missing/typed above
		}
		if !stringSliceContains(values, raw) {
			report("field %q = %q, want one of {%s}", field, raw, strings.Join(values, ", "))
		}
	}

	parameters, paramsOK := entry["parameters"].(map[string]any)
	if !paramsOK {
		report("field %q must be an object", "parameters")
		parameters = nil
	}
	count, countOK := entry["parameter_count"].(float64)
	if !countOK {
		report("field %q must be a number", "parameter_count")
	} else if paramsOK && int(count) != len(parameters) {
		report("parameter_count = %d, want %d (len(parameters))", int(count), len(parameters))
	}
	if has, ok := entry["has_parameters"].(bool); ok && paramsOK && has != (len(parameters) > 0) {
		report("has_parameters = %v, want %v (len(parameters) > 0)", has, len(parameters) > 0)
	}

	validateCatalogInterface(toolID, entry, violations)
	for paramName, raw := range parameters {
		param, ok := raw.(map[string]any)
		if !ok {
			report("parameter %q must be an object", paramName)
			continue
		}
		validateCatalogParam(toolID, paramName, param, violations)
	}
}

func validateCatalogInterface(toolID string, entry map[string]any, violations *[]schemaCatalogStructureViolation) {
	report := func(format string, args ...any) {
		*violations = append(*violations, schemaCatalogStructureViolation{
			tool:    toolID,
			message: fmt.Sprintf(format, args...),
		})
	}
	mode, _ := entry["interface_mode"].(string)
	ref, hasRef := entry["interface_ref"]
	reason, _ := entry["interface_reason"].(string)
	switch mode {
	case InterfaceModeMCP:
		if !hasRef {
			report("interface_mode=mcp requires interface_ref")
			return
		}
		refObj, ok := ref.(map[string]any)
		if !ok {
			report("interface_ref must be an object")
			return
		}
		for _, key := range []string{"product_id", "rpc_name"} {
			if s, ok := refObj[key].(string); !ok || strings.TrimSpace(s) == "" {
				report("interface_ref.%s must be a non-empty string", key)
			}
		}
		if strings.TrimSpace(reason) != "" {
			report("interface_mode=mcp must not set interface_reason")
		}
	case InterfaceModeComposite, InterfaceModeLocal:
		if hasRef {
			report("interface_mode=%s must not set interface_ref", mode)
		}
		if strings.TrimSpace(reason) == "" {
			report("interface_mode=%s requires a non-empty interface_reason", mode)
		}
	}
}

func validateCatalogParam(toolID, paramName string, param map[string]any, violations *[]schemaCatalogStructureViolation) {
	report := func(format string, args ...any) {
		*violations = append(*violations, schemaCatalogStructureViolation{
			tool:    toolID,
			message: fmt.Sprintf(format, args...),
		})
	}

	allowed := make(map[string]bool, len(schemaCatalogParamRequiredKeys)+len(schemaCatalogParamOptionalKeys))
	for _, k := range schemaCatalogParamRequiredKeys {
		allowed[k] = true
	}
	for _, k := range schemaCatalogParamOptionalKeys {
		allowed[k] = true
	}
	for key := range param {
		if !allowed[key] {
			report("parameter %q: unknown field %q", paramName, key)
		}
	}

	typ, _ := param["type"].(string)
	if !stringSliceContains(schemaCatalogParamTypes, typ) {
		report("parameter %q: type = %q, want one of {%s}", paramName, typ, strings.Join(schemaCatalogParamTypes, ", "))
	}
	if s, ok := param["description"].(string); !ok || strings.TrimSpace(s) == "" {
		report("parameter %q: description must be a non-empty string", paramName)
	}
	if _, ok := param["required"].(bool); !ok {
		report("parameter %q: required must be a boolean", paramName)
	}
	if _, ok := param["field_provenance"].(map[string]any); !ok {
		report("parameter %q: field_provenance must be an object", paramName)
	}

	for _, key := range []string{"property", "interface_description", "interface_type", "format", "default", "example", "required_when"} {
		if raw, present := param[key]; present {
			if _, ok := raw.(string); !ok {
				report("parameter %q: %s must be a string", paramName, key)
			}
		}
	}
	if raw, present := param["cli_required"]; present {
		if _, ok := raw.(bool); !ok {
			report("parameter %q: cli_required must be a boolean", paramName)
		}
	}
	if raw, present := param["enum"]; present {
		items, ok := raw.([]any)
		if !ok {
			report("parameter %q: enum must be an array", paramName)
		} else {
			for i, item := range items {
				if _, ok := item.(string); !ok {
					report("parameter %q: enum[%d] must be a string", paramName, i)
				}
			}
		}
	}
}

func requireNonEmptyString(entry map[string]any, key string) bool {
	s, ok := entry[key].(string)
	return ok && strings.TrimSpace(s) != ""
}

func requireStringArray(entry map[string]any, key string) bool {
	raw, ok := entry[key].([]any)
	if !ok {
		return false
	}
	for _, item := range raw {
		if _, ok := item.(string); !ok {
			return false
		}
	}
	return true
}

func stringSliceContains(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}
