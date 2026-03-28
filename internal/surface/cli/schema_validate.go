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
	"math"
	"reflect"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/platform/errors"
)

// ValidateInputSchema performs strict local validation for MCP tool inputs.
// It enforces required/type/enum checks and rejects unknown properties by default.
func ValidateInputSchema(params map[string]any, schema map[string]any) error {
	if len(schema) == 0 {
		return nil
	}
	if params == nil {
		params = map[string]any{}
	}
	if err := validateSchemaValue("$", params, schema); err != nil {
		return apperrors.NewValidation(fmt.Sprintf("input schema validation failed: %v", err))
	}
	return nil
}

func validateSchemaValue(path string, value any, schema map[string]any) error {
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
	if object, ok := value.(map[string]any); ok && (len(required) > 0 || len(properties) > 0 || hasType(expectedTypes, "object")) {
		for _, field := range required {
			if _, exists := object[field]; !exists {
				return fmt.Errorf("%s.%s is required", path, field)
			}
		}

		allowUnknown, additionalSchema, hasAdditionalSchema := additionalProperties(schema)
		strictUnknown := len(properties) > 0 && !allowUnknown && !hasAdditionalSchema

		for key, raw := range object {
			childPath := path + "." + key
			if propertySchema, known := properties[key]; known {
				if err := validateSchemaValue(childPath, raw, propertySchema); err != nil {
					return err
				}
				continue
			}

			if strictUnknown {
				return fmt.Errorf("%s is not allowed", childPath)
			}
			if hasAdditionalSchema {
				if err := validateSchemaValue(childPath, raw, additionalSchema); err != nil {
					return err
				}
			}
		}
	}

	if itemsSchema, ok := schema["items"].(map[string]any); ok {
		if list, ok := value.([]any); ok {
			for idx, item := range list {
				if err := validateSchemaValue(fmt.Sprintf("%s[%d]", path, idx), item, itemsSchema); err != nil {
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
		return math.Trunc(n) == n
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
		return lNum == rNum
	}
	return reflect.DeepEqual(left, right)
}

func numberValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int8:
		return float64(typed), true
	case int16:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case uint:
		return float64(typed), true
	case uint8:
		return float64(typed), true
	case uint16:
		return float64(typed), true
	case uint32:
		return float64(typed), true
	case uint64:
		return float64(typed), true
	case json.Number:
		parsed, err := typed.Float64()
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
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
			if ok && strings.TrimSpace(text) != "" {
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
