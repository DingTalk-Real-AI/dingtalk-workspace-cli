// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package schemaruntime

var compactPayloadKeys = map[string]bool{
	"kind": true, "level": true, "count": true, "tool_count": true,
	"products": true, "product": true, "tools": true,
	"id": true, "schema_path": true, "runtime": true,
	"canonical_path": true, "cli_path": true,
	"agent_summary": true, "description": true,
	"effect": true, "risk": true, "confirmation": true, "idempotency": true,
	"interface_mode": true, "availability": true, "interface_reason": true,
	"parameters": true, "constraints": true, "positionals": true, "dry_run": true,
	"result": true, "pagination": true,
	"examples": true, "use_when": true, "avoid_when": true,
}

var compactParamKeys = map[string]bool{
	"type": true, "description": true, "required": true,
	"cli_required": true, "required_when": true,
	"default": true, "interface_default": true, "example": true,
	"format": true, "enum": true,
}

// Compact projects a Schema payload onto the reviewed positive Agent allowlist.
// Typed result, constraint, positional, pagination, and dry-run values remain verbatim.
func Compact(payload map[string]any) map[string]any {
	if payload == nil {
		return nil
	}
	result := make(map[string]any, len(payload))
	for key, value := range payload {
		if !compactPayloadKeys[key] {
			continue
		}
		switch key {
		case "parameters":
			result[key] = compactParameters(value)
		case "product":
			if product, ok := value.(map[string]any); ok {
				result[key] = Compact(product)
			} else {
				result[key] = value
			}
		case "products", "tools":
			result[key] = compactCollection(value)
		default:
			result[key] = value
		}
	}
	return result
}

func compactCollection(value any) any {
	switch values := value.(type) {
	case []map[string]any:
		result := make([]map[string]any, len(values))
		for i, item := range values {
			result[i] = Compact(item)
		}
		return result
	case []any:
		result := make([]any, len(values))
		for i, item := range values {
			if payload, ok := item.(map[string]any); ok {
				result[i] = Compact(payload)
			} else {
				result[i] = item
			}
		}
		return result
	default:
		return value
	}
}

func compactParameters(value any) any {
	parameters, ok := value.(map[string]any)
	if !ok {
		return compactValue(value)
	}
	result := make(map[string]any, len(parameters))
	for name, raw := range parameters {
		parameter, ok := raw.(map[string]any)
		if !ok {
			result[name] = compactValue(raw)
			continue
		}
		result[name] = compactParam(parameter)
	}
	return result
}

func compactValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		_, isParam := typed["required"]
		if !isParam {
			_, isParam = typed["type"]
		}
		if isParam {
			return compactParam(typed)
		}
		return Compact(typed)
	case []map[string]any:
		result := make([]map[string]any, len(typed))
		for i, item := range typed {
			result[i] = Compact(item)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for i, item := range typed {
			result[i] = compactValue(item)
		}
		return result
	default:
		return value
	}
}

func compactParam(parameter map[string]any) map[string]any {
	result := make(map[string]any, len(parameter))
	for key, value := range parameter {
		if compactParamKeys[key] {
			result[key] = value
		}
	}
	return result
}

// CompactCollection applies Compact to structural collection members.
func CompactCollection(value any) any { return compactCollection(value) }

// CompactParameters applies the reviewed parameter allowlist.
func CompactParameters(value any) any { return compactParameters(value) }

// CompactValue applies compact projection to a nested plain value.
func CompactValue(value any) any { return compactValue(value) }

// CompactParameter applies the reviewed parameter allowlist to one parameter.
func CompactParameter(parameter map[string]any) map[string]any { return compactParam(parameter) }
