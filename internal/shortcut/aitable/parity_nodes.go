// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0
package aitable

import (
	"fmt"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func outputFilteredBaseNodes(rt *shortcut.RuntimeContext) error {
	response, err := rt.CallMCPData(serverHelper, "list_nsheet_nodes", map[string]any{"baseId": rt.Str("base-id")})
	if err != nil {
		return err
	}
	nodes, found := findNamedObjectList(response, "items", "nodes")
	if !found {
		return fmt.Errorf("list_nsheet_nodes did not return an explicit node collection")
	}
	out := make([]map[string]any, 0, len(nodes))
	seen := map[string]bool{}
	for _, n := range nodes {
		id, ok := n["nodeId"].(string)
		if !ok || id == "" || seen[id] {
			return fmt.Errorf("node collection has missing or duplicate nodeId")
		}
		seen[id] = true
		kind, ok := n["nodeType"].(string)
		if !ok || kind == "" {
			return fmt.Errorf("node collection lacks nodeType")
		}
		parent, ok := n["parentSectionId"].(string)
		if !ok {
			return fmt.Errorf("node collection lacks parentSectionId")
		}
		if rt.Changed("type") && kind != rt.Str("type") {
			continue
		}
		if rt.Changed("parent-id") && parent != rt.Str("parent-id") {
			continue
		}
		out = append(out, n)
	}
	return rt.Output(map[string]any{"baseId": rt.Str("base-id"), "count": len(out), "nodes": out})
}
