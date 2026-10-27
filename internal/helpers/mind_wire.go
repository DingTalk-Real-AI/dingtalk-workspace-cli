// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package helpers

import (
	"encoding/json"
	"fmt"
)

// CLI validation and recovery keep typed arguments. Only the MCP transport
// encodes structured fields, preserving the original text and idempotency key.
func mindWireArgs(args map[string]any) (map[string]any, error) {
	wire := make(map[string]any, len(args))
	for k, v := range args {
		wire[k] = v
	}
	for _, k := range []string{"node", "patch"} {
		if v, ok := args[k]; ok {
			data, err := json.Marshal(v)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", k, err)
			}
			wire[k] = string(data)
		}
	}
	return wire, nil
}

// Convert the production HSF projection into the CLI's typed result.
// HSF class names and Java version packages are never part of the CLI protocol.
func mindDecodeFlatResult(value map[string]any) (map[string]any, error) {
	if trace, ok := value["logId"]; ok {
		value["traceId"] = trace
		delete(value, "logId")
	}
	// Failure receipts may contain default/null business fields. Preserve their
	// structured diagnostics without interpreting them as confirmed content.
	detail := map[string]any{}
	for _, p := range [][2]string{{"errorStage", "stage"}, {"errorOperationId", "operationId"}, {"errorCategory", "category"}} {
		if v, ok := value[p[0]]; ok {
			detail[p[1]] = v
			delete(value, p[0])
		}
	}
	if len(detail) > 0 {
		detail["code"] = value["errorCode"]
		detail["message"] = value["errorMessage"]
		value["errorDetail"] = detail
	}
	if success, ok := value["success"].(bool); ok && !success {
		return value, nil
	}
	for _, key := range []string{"pageIdMap", "nodeIdMap"} {
		if raw, exists := value[key]; exists {
			mapping, ok := raw.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("%s must be an object", key)
			}
			for _, id := range mapping {
				if _, ok := id.(string); !ok {
					return nil, fmt.Errorf("%s values must be strings", key)
				}
			}
		}
	}
	_, hasPages := value["pages"]
	_, hasNodes := value["nodes"]
	if hasPages || hasNodes {
		if !hasPages || !hasNodes {
			return nil, fmt.Errorf("pages and nodes must both be present")
		}
		if _, exists := value["items"]; exists {
			return nil, fmt.Errorf("conflicting query projection")
		}
		pages, ok := value["pages"].([]any)
		if !ok {
			return nil, fmt.Errorf("pages must be an array")
		}
		nodes, ok := value["nodes"].([]any)
		if !ok {
			return nil, fmt.Errorf("nodes must be an array")
		}
		byID := map[string]map[string]any{}
		for _, raw := range pages {
			page, ok := raw.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid page")
			}
			id, ok := page["id"].(string)
			if !ok || id == "" || byID[id] != nil {
				return nil, fmt.Errorf("invalid or duplicate page ID")
			}
			if _, exists := page["nodes"]; exists {
				return nil, fmt.Errorf("page metadata cannot contain nested nodes")
			}
			page["nodes"] = []any{}
			byID[id] = page
		}
		for _, raw := range nodes {
			node, ok := raw.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid node")
			}
			pageID, ok := node["pageId"].(string)
			if !ok || byID[pageID] == nil {
				return nil, fmt.Errorf("node refers to an unknown page")
			}
			delete(node, "pageId")
			page := byID[pageID]
			page["nodes"] = append(page["nodes"].([]any), node)
		}
		value["items"] = pages
		delete(value, "pages")
		delete(value, "nodes")
	}
	if text, exists := value["text"]; exists {
		if _, conflict := value["node"]; conflict {
			return nil, fmt.Errorf("conflicting node projection")
		}
		if _, ok := text.(string); !ok {
			return nil, fmt.Errorf("confirmed text must be a string")
		}
		node := map[string]any{"id": value["topicId"], "type": "topic", "text": text}
		for _, key := range []string{"index", "childCount", "writeSupport"} {
			if v, ok := value[key]; ok {
				node[key] = v
			}
		}
		if v, ok := value["parentTopicId"]; ok {
			node["parentId"] = v
		}
		if raw, ok := value["unsupportedFeatures"]; ok {
			features, ok := raw.([]any)
			if !ok {
				return nil, fmt.Errorf("unsupportedFeatures must be an array")
			}
			for _, feature := range features {
				if _, ok := feature.(string); !ok {
					return nil, fmt.Errorf("unsupportedFeatures entries must be strings")
				}
			}
			node["unsupportedFeatures"] = features
		}

		value["node"] = node
		for _, key := range []string{"text", "parentTopicId", "index", "childCount", "writeSupport", "unsupportedFeatures"} {
			delete(value, key)
		}
	}
	return value, nil
}
