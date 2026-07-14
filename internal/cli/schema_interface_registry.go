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
	"fmt"
	"sort"
	"strings"
)

// InterfaceRefKey is the exact, typed identity of an embedded MCP operation.
// It is deliberately independent from a CLI canonical path: multiple commands
// may explicitly project the same operation.
type InterfaceRefKey struct {
	ProductID string
	RPCName   string
}

// InterfaceRegistryEntry is one operation projection from the fixed embedded
// MCP metadata. CanonicalPath is a metadata lookup key, not command identity.
type InterfaceRegistryEntry struct {
	CanonicalPath string
	Ref           InterfaceRefSpec
	Metadata      embeddedMCPToolMetadata
}

// InterfaceRegistry is the typed interface fact model. ByCanonical supports
// exact metadata navigation; ByInterfaceRef answers whether an explicitly
// selected (product_id, rpc_name) exists. The latter retains every canonical
// projection because sharing one RPC is valid and intentional.
type InterfaceRegistry struct {
	ByCanonical    map[string]InterfaceRegistryEntry
	ByInterfaceRef map[InterfaceRefKey][]string
}

// buildInterfaceRegistry creates deterministic exact indexes over embedded
// MCP metadata. It rejects malformed or conflicting canonical projections and
// never manufactures an interface from a command identity.
func buildInterfaceRegistry(tools map[string]embeddedMCPToolMetadata) (InterfaceRegistry, error) {
	registry := InterfaceRegistry{
		ByCanonical:    make(map[string]InterfaceRegistryEntry, len(tools)),
		ByInterfaceRef: make(map[InterfaceRefKey][]string),
	}

	rawCanonicals := make([]string, 0, len(tools))
	for canonical := range tools {
		rawCanonicals = append(rawCanonicals, canonical)
	}
	sort.Strings(rawCanonicals)

	for _, rawCanonical := range rawCanonicals {
		canonical := strings.TrimSpace(rawCanonical)
		if canonical == "" {
			return InterfaceRegistry{}, fmt.Errorf("embedded MCP interface has empty canonical path")
		}
		if existing, exists := registry.ByCanonical[canonical]; exists {
			return InterfaceRegistry{}, fmt.Errorf(
				"embedded MCP canonical path %q conflicts with %q",
				rawCanonical,
				existing.CanonicalPath,
			)
		}

		metadata := tools[rawCanonical]
		if metadata.InterfaceRef == nil {
			return InterfaceRegistry{}, fmt.Errorf("embedded MCP interface %s has no interface_ref", canonical)
		}
		key := InterfaceRefKey{
			ProductID: strings.TrimSpace(metadata.InterfaceRef.ProductID),
			RPCName:   strings.TrimSpace(metadata.InterfaceRef.RPCName),
		}
		if key.ProductID == "" || key.RPCName == "" {
			return InterfaceRegistry{}, fmt.Errorf("embedded MCP interface %s has incomplete interface_ref", canonical)
		}

		entry := InterfaceRegistryEntry{
			CanonicalPath: canonical,
			Ref:           InterfaceRefSpec(key),
			Metadata:      metadata,
		}
		registry.ByCanonical[canonical] = entry
		registry.ByInterfaceRef[key] = append(registry.ByInterfaceRef[key], canonical)
	}

	for key := range registry.ByInterfaceRef {
		sort.Strings(registry.ByInterfaceRef[key])
	}
	return registry, nil
}

// validateSchemaRegistryInterfaces proves every ToolSpec's declared mode and
// explicit RPC reference against the fixed embedded MCP metadata. It does not
// infer an RPC from canonical identity or use aggregate counts as coverage.
func validateSchemaRegistryInterfaces(schema SchemaRegistry) error {
	return validateSchemaRegistryInterfacesWithMetadata(schema, runtimeMCPMetadata())
}

func validateSchemaRegistryInterfacesWithMetadata(schema SchemaRegistry, metadata embeddedMCPMetadata) error {
	interfaces, err := buildInterfaceRegistry(metadata.Tools)
	if err != nil {
		return fmt.Errorf("build typed Interface registry: %w", err)
	}

	var problems []string
	for _, product := range schema.Products {
		for _, tool := range product.Tools {
			canonical := strings.TrimSpace(tool.Identity.CanonicalPath)
			if canonical == "" {
				canonical = strings.Trim(strings.TrimSpace(tool.Identity.ProductID)+"."+strings.TrimSpace(tool.Identity.Name), ".")
			}
			if canonical == "" {
				canonical = "<unknown>"
			}

			if err := tool.Interface.Validate(canonical); err != nil {
				problems = append(problems, err.Error())
				continue
			}
			if mode := strings.TrimSpace(tool.Interface.Mode); mode == InterfaceModeMCP && tool.Interface.AgentExecutable() {
				if err := validateToolInterfaceRef(canonical, mode, tool.Interface.Ref, interfaces); err != nil {
					problems = append(problems, err.Error())
				}
			}
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return nil
}

func validateToolInterfaceRef(canonical, mode string, ref *InterfaceRefSpec, interfaces InterfaceRegistry) error {
	if ref == nil {
		return fmt.Errorf("schema tool %s with interface mode %s has no interface_ref", canonical, mode)
	}
	key := InterfaceRefKey{
		ProductID: strings.TrimSpace(ref.ProductID),
		RPCName:   strings.TrimSpace(ref.RPCName),
	}
	if key.ProductID == "" || key.RPCName == "" {
		return fmt.Errorf("schema tool %s with interface mode %s has incomplete interface_ref", canonical, mode)
	}
	if _, exists := interfaces.ByInterfaceRef[key]; !exists {
		return fmt.Errorf(
			"schema tool %s references unknown MCP interface %s.%s",
			canonical,
			key.ProductID,
			key.RPCName,
		)
	}
	return nil
}
