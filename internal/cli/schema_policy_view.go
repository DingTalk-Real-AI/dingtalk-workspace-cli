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
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cmdpolicy"
)

// The command restriction policy narrows what `dws schema` shows; it never
// changes what is assembled or cached. Assembly, the persistent cache, and
// ResolveMeta stay policy-independent: this file only projects an already
// resolved payload into the view the active policy permits.
//
// schemaPolicyViewField identifies that view on catalog-level payloads.
// catalog_hash keeps describing the base catalog, so a consumer that caches by
// catalog_hash alone would not notice a rule change; the pair does.
const schemaPolicyViewField = "policy_view_hash"

// schemaPolicyQueryDenial is the pre-read check for a single-path query. It
// must run before any Schema byte is produced, including the rendered compact
// leaf fast path. A query naming a denied command (by canonical path, primary
// CLI path, alias, or --cli-path) reports the denial rather than "not found",
// so an Agent does not conclude the command is merely missing.
func schemaPolicyQueryDenial(snapshot *cmdpolicy.Snapshot, query string) error {
	if snapshot.State() != cmdpolicy.StateActive {
		return nil
	}
	query = strings.TrimSpace(query)
	if path, decision, ok := snapshot.ResolveIdentifier(query); ok {
		if decision.Allowed {
			return nil
		}
		return cmdpolicy.DenialError(path, decision, snapshot.State())
	}
	// A product or group path: denied when nothing below it is permitted.
	return cmdpolicy.DiscoveryDenial(snapshot, cmdpolicy.DeclaredPath(query))
}

// schemaPolicyView returns the projection of payload permitted by snapshot.
// The input is never modified: delivery may hand out the cache's own map, so
// the projection is built on a detached copy. Denied tools are removed, tool
// counts are recomputed, and containers left empty are dropped.
//
// allPayload supplies the complete catalog; the overview lists products
// without their tools, so its counts are derived from it.
func schemaPolicyView(snapshot *cmdpolicy.Snapshot, payload map[string]any, allPayload func() (map[string]any, error)) (map[string]any, error) {
	if snapshot.State() != cmdpolicy.StateActive || payload == nil {
		return payload, nil
	}
	view, err := detachedSchemaPayload(payload)
	if err != nil {
		return nil, err
	}
	permitted := func(tool map[string]any) bool { return schemaToolPermitted(snapshot, tool) }

	if _, leaf := view["canonical_path"]; leaf {
		if !permitted(view) {
			return nil, schemaPolicyQueryDenial(snapshot, fmt.Sprint(view["canonical_path"]))
		}
		return view, nil
	}

	switch {
	case view["products"] != nil:
		products, _ := view["products"].([]any)
		var counts map[string]int
		kept := make([]any, 0, len(products))
		total := 0
		for _, raw := range products {
			product, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			count := 0
			if _, hasTools := product["tools"]; hasTools {
				count = filterSchemaTools(product, permitted)
			} else {
				if counts == nil {
					if counts, err = permittedToolCounts(allPayload, permitted); err != nil {
						return nil, err
					}
				}
				count = counts[fmt.Sprint(product["id"])]
				product["tool_count"] = count
			}
			if count == 0 {
				continue
			}
			total += count
			kept = append(kept, product)
		}
		view["products"] = kept
		view["count"] = len(kept)
		view["tool_count"] = total
	case view["product"] != nil:
		product, _ := view["product"].(map[string]any)
		count := filterSchemaTools(product, permitted)
		if count == 0 {
			return nil, schemaPolicyEmptyDenial(snapshot, fmt.Sprint(product["id"]))
		}
		view["count"] = count
	case view["tools"] != nil:
		count := filterSchemaTools(view, permitted)
		if count == 0 {
			return nil, schemaPolicyEmptyDenial(snapshot, cmdpolicy.DeclaredPath(fmt.Sprint(view["path"])))
		}
		view["count"] = count
	}
	return view, nil
}

// schemaPolicyStampView marks a catalog-level payload with the policy view
// identity. It runs after compact stripping so the reviewed compact allowlist
// does not need to know the field, and only where catalog_hash is delivered.
func schemaPolicyStampView(snapshot *cmdpolicy.Snapshot, payload map[string]any) {
	hash := snapshot.ViewHash()
	if hash == "" || payload == nil {
		return
	}
	if _, catalogLevel := payload["catalog_hash"]; catalogLevel {
		payload[schemaPolicyViewField] = hash
	}
}

// filterSchemaTools drops denied tools from container["tools"] and refreshes
// its tool_count when it carries one. It returns the number of tools kept.
func filterSchemaTools(container map[string]any, permitted func(map[string]any) bool) int {
	tools, _ := container["tools"].([]any)
	kept := make([]any, 0, len(tools))
	for _, raw := range tools {
		if tool, ok := raw.(map[string]any); ok && permitted(tool) {
			kept = append(kept, tool)
		}
	}
	container["tools"] = kept
	if _, counted := container["tool_count"]; counted {
		container["tool_count"] = len(kept)
	}
	return len(kept)
}

// schemaToolPermitted fails closed: a tool the policy cannot attribute to an
// evaluated command is not shown.
func schemaToolPermitted(snapshot *cmdpolicy.Snapshot, tool map[string]any) bool {
	for _, field := range []string{"canonical_path", "primary_cli_path", "cli_path"} {
		identifier, _ := tool[field].(string)
		if identifier == "" {
			continue
		}
		if decision, ok := snapshot.Lookup(identifier); ok {
			return decision.Allowed
		}
	}
	return false
}

func permittedToolCounts(allPayload func() (map[string]any, error), permitted func(map[string]any) bool) (map[string]int, error) {
	all, err := allPayload()
	if err != nil {
		return nil, err
	}
	detached, err := detachedSchemaPayload(all)
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	products, _ := detached["products"].([]any)
	for _, raw := range products {
		product, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		tools, _ := product["tools"].([]any)
		for _, rawTool := range tools {
			if tool, ok := rawTool.(map[string]any); ok && permitted(tool) {
				counts[fmt.Sprint(product["id"])]++
			}
		}
	}
	return counts, nil
}

func schemaPolicyEmptyDenial(snapshot *cmdpolicy.Snapshot, path string) error {
	return cmdpolicy.DenialError(path, cmdpolicy.Decision{
		ReasonCode: cmdpolicy.ReasonAllChildrenDenied,
		Reason:     "no command under this path is permitted",
	}, snapshot.State())
}

// detachedSchemaPayload deep-copies a delivery payload into plain JSON values.
// Delivery payloads mix typed slices and maps; the JSON round trip yields one
// uniform shape to filter and guarantees no structure is shared with the
// cache. Numbers keep their exact text through json.Number.
func detachedSchemaPayload(payload map[string]any) (map[string]any, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("copy Schema payload for policy view: %w", err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(encoded)))
	decoder.UseNumber()
	var detached map[string]any
	if err := decoder.Decode(&detached); err != nil {
		return nil, fmt.Errorf("copy Schema payload for policy view: %w", err)
	}
	return detached, nil
}
