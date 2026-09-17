// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package whiteboard

import (
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strings"
)

var diffIgnoredFields = map[string]struct{}{
	"absoluteBounds":      {},
	"children":            {},
	"resolvedPath":        {},
	"source":              {},
	"writeSupport":        {},
	"unsupportedFeatures": {},
	"locked":              {},
}

func compareMappedNodes(before, after map[string]any, identity *identityMapFile, comparison string) ([]map[string]any, []string) {
	reverse := make(map[string]string, len(identity.Nodes))
	for logical, real := range identity.Nodes {
		reverse[strings.TrimSpace(real)] = strings.TrimSpace(logical)
	}
	unresolved := make([]string, 0)
	left := comparableNode(before, reverse, true, &unresolved)
	right := comparableNode(after, reverse, false, &unresolved)
	// The real Query id and request/logical id identify the matched entity but
	// are not themselves user content.
	delete(left, "id")
	delete(right, "id")
	unresolved = uniqueSortedStrings(unresolved)
	if len(unresolved) > 0 {
		return nil, unresolved
	}
	changes := make([]map[string]any, 0)
	diffJSONValue("", left, right, comparison, &changes)
	sort.Slice(changes, func(i, j int) bool {
		return changes[i]["path"].(string) < changes[j]["path"].(string)
	})
	return changes, nil
}

func comparableNode(node map[string]any, reverse map[string]string, current bool, unresolved *[]string) map[string]any {
	cloned, _ := normalizeComparableValue(node, reverse, current, "", unresolved).(map[string]any)
	nodeType, _ := nonEmptyString(cloned["type"])
	if _, present := cloned["angle"]; !present {
		cloned["angle"] = json.Number("0")
	}
	if _, present := cloned["hidden"]; !present {
		cloned["hidden"] = false
	}
	if _, present := cloned["layer"]; !present {
		if nodeType == "frame" {
			cloned["layer"] = "background"
		} else {
			cloned["layer"] = "normal"
		}
	}
	return cloned
}

func normalizeComparableValue(value any, reverse map[string]string, current bool, key string, unresolved *[]string) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for childKey, childValue := range typed {
			if _, ignored := diffIgnoredFields[childKey]; ignored {
				continue
			}
			if childKey == "scope" && key == "nodeRef" {
				result[childKey] = "logical"
				continue
			}
			if current && childKey == "parentId" {
				if real, ok := nonEmptyString(childValue); ok {
					logical, found := reverse[real]
					if !found {
						*unresolved = append(*unresolved, real)
						continue
					}
					result[childKey] = logical
					continue
				}
			}
			if current && childKey == "id" && key == "nodeRef" {
				if real, ok := nonEmptyString(childValue); ok {
					logical, found := reverse[real]
					if !found {
						*unresolved = append(*unresolved, real)
						continue
					}
					result[childKey] = logical
					continue
				}
			}
			result[childKey] = normalizeComparableValue(childValue, reverse, current, childKey, unresolved)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for index, child := range typed {
			result[index] = normalizeComparableValue(child, reverse, current, key, unresolved)
		}
		return result
	default:
		return value
	}
}

func diffJSONValue(path string, before, after any, comparison string, changes *[]map[string]any) {
	if valuesEqual(path, before, after, comparison) {
		return
	}
	beforeObject, beforeIsObject := before.(map[string]any)
	afterObject, afterIsObject := after.(map[string]any)
	if beforeIsObject && afterIsObject {
		keys := make(map[string]struct{}, len(beforeObject)+len(afterObject))
		for key := range beforeObject {
			keys[key] = struct{}{}
		}
		for key := range afterObject {
			keys[key] = struct{}{}
		}
		ordered := make([]string, 0, len(keys))
		for key := range keys {
			ordered = append(ordered, key)
		}
		sort.Strings(ordered)
		for _, key := range ordered {
			diffJSONValue(path+"/"+escapeJSONPointer(key), beforeObject[key], afterObject[key], comparison, changes)
		}
		return
	}
	beforeArray, beforeIsArray := before.([]any)
	afterArray, afterIsArray := after.([]any)
	if beforeIsArray && afterIsArray && len(beforeArray) == len(afterArray) {
		for index := range beforeArray {
			diffJSONValue(path+"/"+fmt.Sprint(index), beforeArray[index], afterArray[index], comparison, changes)
		}
		return
	}
	if path == "" {
		path = "/"
	}
	*changes = append(*changes, map[string]any{"path": path, "before": before, "after": after})
}

func valuesEqual(path string, before, after any, comparison string) bool {
	if before == nil || after == nil {
		return before == nil && after == nil
	}
	if left, ok := numericValue(before); ok {
		right, rightOK := numericValue(after)
		if !rightOK {
			return false
		}
		delta := new(big.Rat).Sub(left, right)
		if delta.Sign() < 0 {
			delta.Neg(delta)
		}
		if comparison == "semantic" && isCoordinatePath(path) {
			return delta.Cmp(whiteboardCoordinateTolerance) <= 0
		}
		return delta.Sign() == 0
	}
	switch left := before.(type) {
	case string:
		right, ok := after.(string)
		return ok && left == right
	case bool:
		right, ok := after.(bool)
		return ok && left == right
	case map[string]any:
		right, ok := after.(map[string]any)
		if !ok || len(left) != len(right) {
			return false
		}
		for key, value := range left {
			if !valuesEqual(path+"/"+escapeJSONPointer(key), value, right[key], comparison) {
				return false
			}
		}
		return true
	case []any:
		right, ok := after.([]any)
		if !ok || len(left) != len(right) {
			return false
		}
		for index := range left {
			if !valuesEqual(path+"/"+fmt.Sprint(index), left[index], right[index], comparison) {
				return false
			}
		}
		return true
	default:
		return fmt.Sprint(before) == fmt.Sprint(after)
	}
}

func isCoordinatePath(path string) bool {
	return strings.HasSuffix(path, "/x") || strings.HasSuffix(path, "/y")
}

func escapeJSONPointer(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~", "~0"), "/", "~1")
}

func uniqueSortedStrings(values []string) []string {
	sort.Strings(values)
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}
