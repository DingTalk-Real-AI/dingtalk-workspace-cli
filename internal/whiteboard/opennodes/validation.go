// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package opennodes

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// NodeValidationError carries the same issue code and JSON pointer used by the
// OpenNodes V1 server. It never includes the user's field value.
type NodeValidationError struct {
	Code string
	Path string
	Hint string // Validator-authored guidance, never a user's field value.
}

func (e *NodeValidationError) Error() string {
	return fmt.Sprintf("OpenNodes %s: %s%s；请修正源文件后重新 render/+diff 并确认", e.Code, e.Path, e.Hint)
}

func nodeIssue(index int, field, code string) error {
	return &NodeValidationError{Code: code, Path: fmt.Sprintf("/source/nodes/%d/%s", index, field)}
}

// ValidateNodes rejects known server-invalid fields and request-local graphs.
// It deliberately keeps unknown node fields/types open for catalog evolution;
// this is not a replacement for the server's full schema and model compiler.
func ValidateNodes(nodes []map[string]any) error {
	if err := ValidatePresentationOrder(nodes); err != nil {
		return err
	}
	if err := ValidateText(nodes); err != nil {
		return err
	}
	indexByID := make(map[string]int, len(nodes))
	for index, node := range nodes {
		if err := validateCommonFields(node, index); err != nil {
			return err
		}
		if id, _ := node["id"].(string); id != "" {
			if _, exists := indexByID[id]; exists {
				return nodeIssue(index, "id", "duplicateTemporaryId")
			}
			indexByID[id] = index
		}
	}
	parents := make([]int, len(nodes))
	for index, node := range nodes {
		parents[index] = -1
		if parentID, _ := node["parentId"].(string); parentID != "" {
			parentIndex, exists := indexByID[parentID]
			if !exists {
				return nodeIssue(index, "parentId", "referenceNotFound")
			}
			parentType, _ := nodes[parentIndex]["type"].(string)
			if (parentType != "group" && parentType != "frame") || node["type"] == "connector" || node["type"] == "frame" {
				return nodeIssue(index, "parentId", "invalidParent")
			}
			parents[index] = parentIndex
		}
		if node["type"] == "connector" {
			if err := validateConnectorReferences(node, index, nodes, indexByID); err != nil {
				return err
			}
		}
	}
	// Each node is visited at most twice, including for deep, untrusted graphs.
	colors := make([]uint8, len(nodes))
	for index := range nodes {
		cursor := index
		for cursor >= 0 && colors[cursor] == 0 {
			colors[cursor] = 1
			cursor = parents[cursor]
		}
		if cursor >= 0 && colors[cursor] == 1 {
			return nodeIssue(cursor, "parentId", "parentCycle")
		}
		cursor = index
		for cursor >= 0 && colors[cursor] == 1 {
			colors[cursor] = 2
			cursor = parents[cursor]
		}
	}
	return nil
}

func validateCommonFields(node map[string]any, index int) error {
	for _, key := range []string{"id", "parentId"} {
		if value, present := node[key]; present {
			if text, ok := value.(string); !ok || strings.TrimSpace(text) == "" {
				return nodeIssue(index, key, "invalidValue")
			}
		}
	}
	for _, key := range []string{"x", "y", "width", "height", "angle", "zIndex", "presentationOrder"} {
		value, present := node[key]
		if !present {
			continue
		}
		number, ok := finiteNumber(value)
		if !ok || ((key == "width" || key == "height") && number <= 0) ||
			((key == "zIndex" || key == "presentationOrder") && math.Trunc(number) != number) ||
			((key == "presentationOrder" || key == "zIndex") && number < 0) || (key == "angle" && node["type"] == "frame" && number != 0) {
			return nodeIssue(index, key, "invalidValue")
		}
	}
	if value, present := node["hidden"]; present {
		if _, ok := value.(bool); !ok {
			return nodeIssue(index, "hidden", "invalidType")
		}
	}
	if value, present := node["layer"]; present && value != "background" && value != "normal" && value != "foreground" {
		return nodeIssue(index, "layer", "invalidValue")
	}
	return nil
}

func finiteNumber(value any) (float64, bool) {
	var number float64
	switch value := value.(type) {
	case json.Number:
		var err error
		number, err = value.Float64()
		if err != nil {
			return 0, false
		}
	case float64:
		number = value
	case int:
		number = float64(value)
	default:
		return 0, false
	}
	return number, !math.IsNaN(number) && !math.IsInf(number, 0)
}

func validateConnectorReferences(node map[string]any, index int, nodes []map[string]any, indexByID map[string]int) error {
	var firstID string
	for _, key := range []string{"start", "end"} {
		endpoint, _ := node[key].(map[string]any)
		if endpoint["type"] != "node" {
			continue
		}
		ref, _ := endpoint["nodeRef"].(map[string]any)
		if ref["scope"] != "request" {
			return nodeIssue(index, key+"/nodeRef/scope", "referenceScopeUnsupported")
		}
		id, _ := ref["id"].(string)
		targetIndex, exists := indexByID[id]
		if !exists {
			return nodeIssue(index, key+"/nodeRef/id", "referenceNotFound")
		}
		target := nodes[targetIndex]
		switch target["type"] {
		case "shape", "text", "stickyNote", "frame", "group", "path":
		default:
			return nodeIssue(index, key+"/nodeRef/id", "connectorTargetUnsupported")
		}
		if target["hidden"] == true {
			return nodeIssue(index, key+"/nodeRef/id", "connectorTargetUnsupported")
		}
		if key == "start" {
			firstID = id
		} else if firstID == id {
			return nodeIssue(index, key+"/nodeRef/id", "connectorSelfLoop")
		}
	}
	return nil
}
