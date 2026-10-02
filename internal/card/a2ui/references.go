// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package a2ui

import (
	"fmt"
	"sort"
	"strings"
)

type referenceNode struct {
	component map[string]any
	location  string
}

type referenceSnapshot struct {
	nodes    map[string]referenceNode
	location string
}

type referenceEdge struct {
	target, location string
	template         bool
}

func (s *referenceSnapshot) update(body map[string]any, location string) []Diagnostic {
	ds := []Diagnostic{}
	seen := map[string]bool{}
	for i, raw := range array(body["components"]) {
		c := object(raw)
		id := str(c["id"])
		ptr := fmt.Sprintf("%s/components/%d", location, i)
		if seen[id] {
			ds = append(ds, Diagnostic{"reference.duplicate_component_id", "error", ptr + "/id",
				fmt.Sprintf("Component %s is defined twice in one message; replacing an ID across messages is valid", id), ""})
		}
		seen[id] = true
		s.nodes[id] = referenceNode{c, ptr}
	}
	return ds
}

func referenceSlots(node any, path []string, location string, visit func(any, string)) {
	if len(path) == 0 {
		visit(node, location)
		return
	}
	if path[0] == "[]" {
		for i, child := range array(node) {
			referenceSlots(child, path[1:], fmt.Sprintf("%s/%d", location, i), visit)
		}
	} else if child, ok := object(node)[path[0]]; ok {
		referenceSlots(child, path[1:], location+pointer(path[:1]), visit)
	}
}

// checkReferences checks the final reachable graph, without expanding templates or evaluating data.
func (p *Protocol) checkReferences(s referenceSnapshot, surfaceID string) []Diagnostic {
	if _, ok := s.nodes["root"]; !ok {
		return []Diagnostic{{"reference.missing_root", "error", s.location,
			fmt.Sprintf("Surface %s is missing the root component in its final snapshot", surfaceID), ""}}
	}
	edges := map[string][]referenceEdge{}
	for id, node := range s.nodes {
		for _, raw := range array(p.assets.Explain[str(node.component["component"])]["childRefs"]) {
			ref := object(raw)
			referenceSlots(node.component, strings.Split(str(ref["path"]), "."), node.location, func(value any, ptr string) {
				if ref["kind"] == "id" {
					if child, ok := value.(string); ok {
						edges[id] = append(edges[id], referenceEdge{child, ptr, false})
					}
				} else if ref["kind"] == "list" {
					switch v := value.(type) {
					case []any:
						for i, raw := range v {
							if child, ok := raw.(string); ok {
								edges[id] = append(edges[id], referenceEdge{child, fmt.Sprintf("%s/%d", ptr, i), false})
							}
						}
					case map[string]any:
						if child, ok := v["componentId"].(string); ok {
							edges[id] = append(edges[id], referenceEdge{child, ptr + "/componentId", true})
						}
					}
				}
			})
		}
	}
	ds := []Diagnostic{}
	reachable := map[string]bool{}
	pending := []string{"root"}
	for len(pending) > 0 {
		id := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if reachable[id] {
			continue
		}
		reachable[id] = true
		for _, edge := range edges[id] {
			if _, ok := s.nodes[edge.target]; !ok {
				ds = append(ds, Diagnostic{"reference.dangling_child", "error", edge.location,
					fmt.Sprintf("Surface %s references undefined child component %s", surfaceID, edge.target), ""})
			} else if !reachable[edge.target] {
				pending = append(pending, edge.target)
			}
		}
	}
	ids := []string{}
	for id := range reachable {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	colors := map[string]int{}
	type frame struct {
		id   string
		next int
	}
	for _, start := range ids {
		if colors[start] != 0 {
			continue
		}
		colors[start] = 1
		stack := []frame{{start, 0}}
		for len(stack) > 0 {
			top := &stack[len(stack)-1]
			if top.next == len(edges[top.id]) {
				colors[top.id] = 2
				stack = stack[:len(stack)-1]
				continue
			}
			id := top.id
			edge := edges[id][top.next]
			top.next++
			if _, ok := s.nodes[edge.target]; edge.template || !ok {
				continue
			}
			if colors[edge.target] == 1 {
				ds = append(ds, Diagnostic{"reference.cycle", "error", edge.location,
					fmt.Sprintf("Surface %s has a static component-reference cycle: %s → %s", surfaceID, id, edge.target), ""})
			} else if colors[edge.target] == 0 {
				colors[edge.target] = 1
				stack = append(stack, frame{edge.target, 0})
			}
		}
	}
	ids = nil
	for id := range s.nodes {
		if !reachable[id] {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		ds = append(ds, Diagnostic{"reference.unreachable_component", "warning", s.nodes[id].location + "/id",
			fmt.Sprintf("Component %s is unreachable from root or its templates; retain it if needed for a later update", id), ""})
	}
	return ds
}
