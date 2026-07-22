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

package app

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

func pluginToolInputSchema(
	tools transport.ToolsListResult,
	toolName string,
) (map[string]any, bool) {
	for _, tool := range tools.Tools {
		if strings.TrimSpace(tool.Name) == strings.TrimSpace(toolName) {
			return tool.InputSchema, true
		}
	}
	return nil, false
}

func normalizePluginInputParams(
	params map[string]any,
	schema map[string]any,
) (map[string]any, error) {
	schema = canonicalPluginInputSchema(schema)
	normalized := make(map[string]any, len(params))
	for key, value := range params {
		normalized[key] = value
	}
	if _, err := coercePluginSchemaValue(normalized, schema); err != nil {
		return nil, cliInputValidationError(err)
	}
	if err := cli.ValidateInputSchema(normalized, schema); err != nil {
		return nil, err
	}
	return normalized, nil
}

func canonicalPluginInputSchema(schema map[string]any) map[string]any {
	if len(schema) == 0 {
		return schema
	}
	cloned := make(map[string]any, len(schema))
	for key, value := range schema {
		cloned[key] = clonePluginSchemaValue(key, value)
	}
	return cloned
}

func clonePluginSchemaValue(key string, value any) any {
	switch typed := value.(type) {
	case map[string]any:
		cloned := make(map[string]any, len(typed))
		for childKey, childValue := range typed {
			cloned[childKey] = clonePluginSchemaValue(childKey, childValue)
		}
		return cloned
	case []any:
		cloned := make([]any, len(typed))
		for index, item := range typed {
			cloned[index] = clonePluginSchemaValue(key, item)
		}
		return cloned
	case []string:
		cloned := make([]string, len(typed))
		for index, item := range typed {
			if key == "type" {
				item = canonicalPluginSchemaType(item)
			}
			cloned[index] = item
		}
		return cloned
	case string:
		if key == "type" {
			return canonicalPluginSchemaType(typed)
		}
		return typed
	default:
		return value
	}
}

func canonicalPluginSchemaType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "bool":
		return "boolean"
	case "int":
		return "integer"
	case "float":
		return "number"
	default:
		return value
	}
}

func cliInputValidationError(err error) error {
	if err == nil {
		return nil
	}
	return apperrors.NewValidation(
		fmt.Sprintf("input schema normalization failed: %v", err),
		apperrors.WithReason("plugin_input_schema_invalid"),
	)
}

func coercePluginSchemaValue(value any, schema map[string]any) (any, error) {
	target := singlePluginSchemaType(schema)
	if raw, ok := value.(string); ok {
		trimmed := strings.TrimSpace(raw)
		switch target {
		case "bool", "boolean":
			parsed, err := strconv.ParseBool(trimmed)
			if err != nil {
				return nil, fmt.Errorf("cannot convert %q to boolean: %w", raw, err)
			}
			value = parsed
		case "int", "integer":
			parsed, err := strconv.Atoi(trimmed)
			if err != nil {
				return nil, fmt.Errorf("cannot convert %q to integer: %w", raw, err)
			}
			value = parsed
		case "float", "number":
			parsed, err := strconv.ParseFloat(trimmed, 64)
			if err != nil {
				return nil, fmt.Errorf("cannot convert %q to number: %w", raw, err)
			}
			value = parsed
		case "object":
			var parsed map[string]any
			if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
				return nil, fmt.Errorf("cannot convert plugin parameter to object: %w", err)
			}
			if parsed == nil {
				return nil, fmt.Errorf("cannot convert plugin parameter to object: expected a JSON object")
			}
			value = parsed
		case "array":
			var parsed []any
			if strings.HasPrefix(trimmed, "[") {
				if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
					return nil, fmt.Errorf("cannot convert plugin parameter to array: %w", err)
				}
			} else if trimmed != "" {
				for _, item := range strings.Split(trimmed, ",") {
					if item = strings.TrimSpace(item); item != "" {
						parsed = append(parsed, item)
					}
				}
			}
			value = parsed
		}
	}

	switch typed := value.(type) {
	case map[string]any:
		properties, _ := schema["properties"].(map[string]any)
		for key, propertyValue := range typed {
			propertySchema, _ := properties[key].(map[string]any)
			if len(propertySchema) == 0 {
				continue
			}
			coerced, err := coercePluginSchemaValue(propertyValue, propertySchema)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", key, err)
			}
			typed[key] = coerced
		}
		return typed, nil
	case []string:
		items := make([]any, len(typed))
		for index, item := range typed {
			items[index] = item
		}
		value = items
	}

	if items, ok := value.([]any); ok {
		itemSchema, _ := schema["items"].(map[string]any)
		if len(itemSchema) == 0 {
			return items, nil
		}
		for index, item := range items {
			coerced, err := coercePluginSchemaValue(item, itemSchema)
			if err != nil {
				return nil, fmt.Errorf("item %d: %w", index, err)
			}
			items[index] = coerced
		}
		return items, nil
	}
	return value, nil
}

func singlePluginSchemaType(schema map[string]any) string {
	var types []string
	switch typed := schema["type"].(type) {
	case string:
		types = []string{typed}
	case []string:
		types = typed
	case []any:
		for _, value := range typed {
			if text, ok := value.(string); ok {
				types = append(types, text)
			}
		}
	}
	var target string
	for _, candidate := range types {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || candidate == "null" {
			continue
		}
		if target != "" && target != candidate {
			return ""
		}
		target = candidate
	}
	return target
}
