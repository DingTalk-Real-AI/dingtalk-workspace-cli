// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitableprotocol

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ParseJSONC removes comments outside strings and trailing commas. It does not
// repair invalid JSON or change URLs, escaped quotes or comment-like text.
func ParseJSONC(raw string) (map[string]any, error) {
	if len(raw) > 8<<20 {
		return nil, fmt.Errorf("JSONC 模板超过 8 MiB")
	}
	data := []byte(raw)
	inString, escaped := false, false
	for i := 0; i < len(data); i++ {
		if inString {
			if escaped {
				escaped = false
			} else if data[i] == '\\' {
				escaped = true
			} else if data[i] == '"' {
				inString = false
			}
			continue
		}
		if data[i] == '"' {
			inString = true
			continue
		}
		if data[i] != '/' || i+1 >= len(data) {
			continue
		}
		switch data[i+1] {
		case '/':
			for ; i < len(data) && data[i] != '\n' && data[i] != '\r'; i++ {
				data[i] = ' '
			}
			i--
		case '*':
			start, end := i, i+2
			for end+1 < len(data) && !(data[end] == '*' && data[end+1] == '/') {
				end++
			}
			if end+1 >= len(data) {
				return nil, fmt.Errorf("JSONC 注释未闭合（位置 %d）", start)
			}
			for ; i <= end+1; i++ {
				if data[i] != '\n' && data[i] != '\r' {
					data[i] = ' '
				}
			}
			i--
		}
	}
	inString, escaped = false, false
	for i := range data {
		if inString {
			if escaped {
				escaped = false
			} else if data[i] == '\\' {
				escaped = true
			} else if data[i] == '"' {
				inString = false
			}
			continue
		}
		if data[i] == '"' {
			inString = true
			continue
		}
		if data[i] == ',' {
			previous := i - 1
			for previous >= 0 && strings.ContainsRune(" \t\r\n", rune(data[previous])) {
				previous--
			}
			// A comma without a preceding value is invalid JSONC, not a trailing comma.
			if previous < 0 || strings.ContainsRune("[{,:", rune(data[previous])) {
				continue
			}
			j := i + 1
			for j < len(data) && strings.ContainsRune(" \t\r\n", rune(data[j])) {
				j++
			}
			if j < len(data) && (data[j] == '}' || data[j] == ']') {
				data[i] = ' '
			}
		}
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("JSONC 模板解析失败: %w", err)
	}
	if result == nil {
		return nil, fmt.Errorf("JSONC 模板必须是对象")
	}
	return result, nil
}

// ProjectChartTemplates selects exact published chart types; unknown types fail
// before a writer sees a partially generated configuration.
func ProjectChartTemplates(raw string, types []string) (map[string]any, error) {
	all, err := ParseJSONC(raw)
	if err != nil {
		return nil, err
	}
	selected := make(map[string]any, len(types))
	if len(types) == 0 {
		return nil, fmt.Errorf("至少指定一个图表类型")
	}
	for _, name := range types {
		name = strings.TrimSpace(name)
		config, ok := all[name].(map[string]any)
		if !ok || config["chartType"] != name {
			return nil, fmt.Errorf("模板没有合法的图表类型 %q", name)
		}
		if _, exists := selected[name]; exists {
			return nil, fmt.Errorf("重复的图表类型 %q", name)
		}
		selected[name] = config
	}
	return selected, nil
}

// BindReferences substitutes only an entire {"$ref":"logical.property"}
// object, including nested object properties. Unresolved or ambiguous references
// stop execution; strings are literal.
func BindReferences(value any, resources map[string]map[string]any) (any, error) {
	switch v := value.(type) {
	case map[string]any:
		if ref, exists := v["$ref"]; exists {
			name, ok := ref.(string)
			parts := strings.Split(name, ".")
			if !ok || len(v) != 1 || len(parts) < 2 {
				return nil, fmt.Errorf("资源引用必须为独立对象 {$ref:逻辑名.属性}")
			}
			var bound any = resources[parts[0]]
			for _, key := range parts[1:] {
				object, ok := bound.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("资源引用 %q 路径不是对象", name)
				}
				child, exists := object[key]
				if !exists || child == nil {
					return nil, fmt.Errorf("资源引用 %q 未绑定", name)
				}
				bound = child
			}
			if s, ok := bound.(string); ok && s == "" {
				return nil, fmt.Errorf("资源引用 %q 为空", name)
			}
			return bound, nil
		}
		result := make(map[string]any, len(v))
		for key, child := range v {
			bound, err := BindReferences(child, resources)
			if err != nil {
				return nil, err
			}
			result[key] = bound
		}
		return result, nil
	case []any:
		result := make([]any, len(v))
		for i, child := range v {
			bound, err := BindReferences(child, resources)
			if err != nil {
				return nil, err
			}
			result[i] = bound
		}
		return result, nil
	default:
		return value, nil
	}
}

// ExpandChartTemplate copies a complete template then applies typed parameters.
// Nested objects merge, arrays replace, and chartType cannot change identity.
func ExpandChartTemplate(template, parameters map[string]any) (map[string]any, error) {
	if template == nil {
		return nil, fmt.Errorf("图表模板缺失")
	}
	encoded, err := json.Marshal(template)
	if err != nil {
		return nil, err
	}
	var config map[string]any
	if err = json.Unmarshal(encoded, &config); err != nil {
		return nil, err
	}
	typeName, ok := config["chartType"].(string)
	if !ok || typeName == "" {
		return nil, fmt.Errorf("模板缺少 chartType")
	}
	if declared, exists := parameters["chartType"]; exists && declared != typeName {
		return nil, fmt.Errorf("参数不能更改模板 chartType")
	}
	var merge func(map[string]any, map[string]any)
	merge = func(target, patch map[string]any) {
		for key, value := range patch {
			object, ok := value.(map[string]any)
			existing, exists := target[key].(map[string]any)
			if ok && exists {
				merge(existing, object)
			} else {
				target[key] = value
			}
		}
	}
	merge(config, parameters)
	if err = ValidateDashboardPersistentMetadata("config", config); err != nil {
		return nil, err
	}
	return config, nil
}

// ProjectStructuredChartTemplates shares the typed-response projection used by
// native and composite callers. Both accept only the same validated templates.
func ProjectStructuredChartTemplates(value any, types []string) (map[string]any, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return ProjectChartTemplates(string(encoded), types)
}
