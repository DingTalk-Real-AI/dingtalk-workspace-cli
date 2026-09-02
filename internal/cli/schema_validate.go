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
	"math/big"
	"reflect"
	"strconv"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

// ValidateInputSchema performs strict local validation for reviewed tool inputs.
// It enforces required/type/enum checks and rejects unknown properties by default.
func ValidateInputSchema(params map[string]any, schema map[string]any) error {
	return validateInputSchema(params, schema, true)
}

// ValidateMCPInputSchema validates a remote MCP tool input schema while
// preserving JSON Schema's default additionalProperties=true behavior.
func ValidateMCPInputSchema(params map[string]any, schema map[string]any) error {
	if err := validateMCPInputSchemaSupport("$", schema); err != nil {
		return apperrors.NewValidation(fmt.Sprintf("input schema validation failed: %v", err))
	}
	return validateInputSchema(params, schema, false)
}

func validateMCPInputSchemaSupport(path string, schema map[string]any) error {
	for keyword, raw := range schema {
		switch keyword {
		case "$schema":
			dialect, valid := raw.(string)
			if !valid {
				return fmt.Errorf("%s.$schema must be a supported dialect URI", path)
			}
			if _, supported := mcpSupportedSchemaDialects[dialect]; !supported {
				return fmt.Errorf("%s.$schema uses unsupported dialect %q", path, dialect)
			}
		case "$id", "$anchor", "$comment", "title", "description", "default", "examples", "deprecated", "readOnly", "writeOnly":
			continue
		case "type":
			types, valid := mcpSchemaStringList(raw, true)
			if !valid || len(types) == 0 {
				return fmt.Errorf("%s.type is not a supported non-empty string or string array", path)
			}
			for _, schemaType := range types {
				if _, supported := mcpSupportedSchemaTypes[schemaType]; !supported {
					return fmt.Errorf("%s.type contains unsupported type %q", path, schemaType)
				}
			}
		case "enum":
			if !mcpSchemaValidEnum(raw) {
				return fmt.Errorf("%s.enum must be a non-empty array of unique values", path)
			}
		case "required":
			if _, valid := mcpSchemaStringList(raw, false); !valid {
				return fmt.Errorf("%s.required must be an array of non-empty strings", path)
			}
		case "properties":
			properties, valid := raw.(map[string]any)
			if !valid {
				return fmt.Errorf("%s.properties must be an object", path)
			}
			for name, childRaw := range properties {
				child, valid := childRaw.(map[string]any)
				if !valid {
					return fmt.Errorf("%s.properties[%q] uses an unsupported boolean or malformed schema", path, name)
				}
				if err := validateMCPInputSchemaSupport(fmt.Sprintf("%s.properties[%q]", path, name), child); err != nil {
					return err
				}
			}
		case "items":
			items, valid := raw.(map[string]any)
			if !valid {
				return fmt.Errorf("%s.items uses an unsupported boolean, tuple, or malformed schema", path)
			}
			if err := validateMCPInputSchemaSupport(path+".items", items); err != nil {
				return err
			}
		case "additionalProperties":
			if _, valid := raw.(bool); valid {
				continue
			}
			additional, valid := raw.(map[string]any)
			if !valid {
				return fmt.Errorf("%s.additionalProperties must be a boolean or object schema", path)
			}
			if err := validateMCPInputSchemaSupport(path+".additionalProperties", additional); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%s uses unsupported JSON Schema keyword %q", path, keyword)
		}
	}
	return nil
}

var mcpSupportedSchemaTypes = map[string]struct{}{
	"array": {}, "boolean": {}, "integer": {}, "null": {}, "number": {}, "object": {}, "string": {},
}

var mcpSupportedSchemaDialects = map[string]struct{}{
	"http://json-schema.org/draft-06/schema#":      {},
	"http://json-schema.org/draft-07/schema#":      {},
	"https://json-schema.org/draft/2019-09/schema": {},
	"https://json-schema.org/draft/2020-12/schema": {},
}

func mcpSchemaStringList(raw any, allowSingle bool) ([]string, bool) {
	if value, valid := raw.(string); valid {
		return []string{value}, allowSingle && value != ""
	}
	var values []string
	switch typed := raw.(type) {
	case []string:
		values = typed
	case []any:
		values = make([]string, 0, len(typed))
		for _, rawValue := range typed {
			value, valid := rawValue.(string)
			if !valid {
				return nil, false
			}
			values = append(values, value)
		}
	default:
		return nil, false
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" {
			return nil, false
		}
		if _, duplicate := seen[value]; duplicate {
			return nil, false
		}
		seen[value] = struct{}{}
	}
	return values, true
}

func mcpSchemaValidEnum(raw any) bool {
	var values []any
	switch typed := raw.(type) {
	case []any:
		values = typed
	case []string:
		values = make([]any, 0, len(typed))
		for _, value := range typed {
			values = append(values, value)
		}
	default:
		return false
	}
	if len(values) == 0 {
		return false
	}
	for i, value := range values {
		for _, previous := range values[:i] {
			if valuesEqual(value, previous) {
				return false
			}
		}
	}
	return true
}

func validateInputSchema(params map[string]any, schema map[string]any, rejectUnknownByDefault bool) error {
	if len(schema) == 0 {
		return nil
	}
	if params == nil {
		params = map[string]any{}
	}
	if err := validateSchemaValueWithPolicy("$", params, schema, rejectUnknownByDefault); err != nil {
		return apperrors.NewValidation(fmt.Sprintf("input schema validation failed: %v", err))
	}
	return nil
}

// ValidateJSONSchemaValue validates one decoded JSON value against the
// required/type/enum/properties/items subset used by reviewed CLI contracts.
func ValidateJSONSchemaValue(value any, schema map[string]any) error {
	return validateSchemaValue("$", value, schema)
}

func validateSchemaValue(path string, value any, schema map[string]any) error {
	return validateSchemaValueWithPolicy(path, value, schema, true)
}

func validateSchemaValueWithPolicy(path string, value any, schema map[string]any, rejectUnknownByDefault bool) error {
	if len(schema) == 0 {
		return nil
	}

	expectedTypes := schemaTypes(schema)
	if len(expectedTypes) > 0 && !matchesAnyType(value, expectedTypes) {
		return fmt.Errorf("%s must be %s", path, strings.Join(expectedTypes, " or "))
	}

	if enumValues := schemaEnum(schema); len(enumValues) > 0 && !matchesEnum(value, enumValues) {
		return fmt.Errorf("%s must be one of %v", path, enumValues)
	}

	properties := schemaProperties(schema)
	required := schemaRequired(schema)
	_, additionalPropertiesDeclared := schema["additionalProperties"]
	if object, ok := value.(map[string]any); ok && (len(required) > 0 || len(properties) > 0 || hasType(expectedTypes, "object") || additionalPropertiesDeclared) {
		for _, field := range required {
			if _, exists := object[field]; !exists {
				return fmt.Errorf("%s.%s is required", path, field)
			}
		}

		allowUnknown, additionalSchema, hasAdditionalSchema := additionalProperties(schema)
		strictUnknown := !allowUnknown && !hasAdditionalSchema && (additionalPropertiesDeclared || rejectUnknownByDefault && len(properties) > 0)

		for key, raw := range object {
			childPath := path + "." + key
			if propertySchema, known := properties[key]; known {
				if err := validateSchemaValueWithPolicy(childPath, raw, propertySchema, rejectUnknownByDefault); err != nil {
					return err
				}
				continue
			}

			if strictUnknown {
				return fmt.Errorf("%s is not allowed", childPath)
			}
			if hasAdditionalSchema {
				if err := validateSchemaValueWithPolicy(childPath, raw, additionalSchema, rejectUnknownByDefault); err != nil {
					return err
				}
			}
		}
	}

	if itemsSchema, ok := schema["items"].(map[string]any); ok {
		if list, ok := value.([]any); ok {
			for idx, item := range list {
				if err := validateSchemaValueWithPolicy(fmt.Sprintf("%s[%d]", path, idx), item, itemsSchema, rejectUnknownByDefault); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func schemaTypes(schema map[string]any) []string {
	switch typed := schema["type"].(type) {
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		return []string{typed}
	case []string:
		out := make([]string, 0, len(typed))
		for _, entry := range typed {
			if strings.TrimSpace(entry) != "" {
				out = append(out, entry)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(typed))
		for _, entry := range typed {
			text, ok := entry.(string)
			if ok && strings.TrimSpace(text) != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func hasType(types []string, target string) bool {
	for _, item := range types {
		if item == target {
			return true
		}
	}
	return false
}

func matchesAnyType(value any, types []string) bool {
	for _, expected := range types {
		if matchesType(value, expected) {
			return true
		}
	}
	return false
}

func matchesType(value any, expected string) bool {
	switch expected {
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "number":
		_, ok := numberValue(value)
		return ok
	case "integer":
		n, ok := numberValue(value)
		if !ok {
			return false
		}
		return n.IsInt()
	case "null":
		return value == nil
	default:
		return true
	}
}

func schemaEnum(schema map[string]any) []any {
	switch typed := schema["enum"].(type) {
	case []any:
		return typed
	case []string:
		out := make([]any, 0, len(typed))
		for _, entry := range typed {
			out = append(out, entry)
		}
		return out
	default:
		return nil
	}
}

func matchesEnum(value any, candidates []any) bool {
	for _, candidate := range candidates {
		if valuesEqual(value, candidate) {
			return true
		}
	}
	return false
}

func valuesEqual(left, right any) bool {
	lNum, lOK := numberValue(left)
	rNum, rOK := numberValue(right)
	if lOK && rOK {
		return lNum.Cmp(rNum) == 0
	}
	return reflect.DeepEqual(left, right)
}

func numberValue(value any) (*big.Rat, bool) {
	var text string
	switch typed := value.(type) {
	case float64:
		text = strconv.FormatFloat(typed, 'g', -1, 64)
	case float32:
		text = strconv.FormatFloat(float64(typed), 'g', -1, 32)
	case int:
		text = strconv.FormatInt(int64(typed), 10)
	case int8:
		text = strconv.FormatInt(int64(typed), 10)
	case int16:
		text = strconv.FormatInt(int64(typed), 10)
	case int32:
		text = strconv.FormatInt(int64(typed), 10)
	case int64:
		text = strconv.FormatInt(typed, 10)
	case uint:
		text = strconv.FormatUint(uint64(typed), 10)
	case uint8:
		text = strconv.FormatUint(uint64(typed), 10)
	case uint16:
		text = strconv.FormatUint(uint64(typed), 10)
	case uint32:
		text = strconv.FormatUint(uint64(typed), 10)
	case uint64:
		text = strconv.FormatUint(typed, 10)
	case json.Number:
		text = typed.String()
	default:
		return nil, false
	}
	parsed, ok := new(big.Rat).SetString(text)
	return parsed, ok
}

func schemaProperties(schema map[string]any) map[string]map[string]any {
	raw, ok := schema["properties"].(map[string]any)
	if !ok {
		return map[string]map[string]any{}
	}

	properties := make(map[string]map[string]any, len(raw))
	for key, value := range raw {
		child, ok := value.(map[string]any)
		if !ok {
			continue
		}
		properties[key] = child
	}
	return properties
}

func schemaRequired(schema map[string]any) []string {
	switch typed := schema["required"].(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, value := range typed {
			text, ok := value.(string)
			if ok && text != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func additionalProperties(schema map[string]any) (allowUnknown bool, additionalSchema map[string]any, hasAdditionalSchema bool) {
	raw, exists := schema["additionalProperties"]
	if !exists {
		return false, nil, false
	}

	switch typed := raw.(type) {
	case bool:
		return typed, nil, false
	case map[string]any:
		return false, typed, true
	default:
		return false, nil, false
	}
}
