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
	"strings"
)

// ToolSchemaHint is reviewed field metadata for an existing registry-owned
// tool. It intentionally has no command path, alias, visibility, or canonical
// identity fields: all identity/navigation comes from CommandRegistry.
type ToolSchemaHint struct {
	Title       string
	Description string
	Parameters  map[string]ParameterSchemaHint
}

// ParameterSchemaHint overrides the projected flat schema for one parameter.
// It can be keyed either by the original MCP parameter name (preferred) or by
// the final CLI flag name when a hint is purely CLI-facing.
type ParameterSchemaHint struct {
	FlagName     string
	Type         string
	Description  string
	Required     *bool
	RequiredWhen string
}

type SchemaVisibility string

const (
	SchemaVisibilityPublic   SchemaVisibility = "public"
	SchemaVisibilityCompat   SchemaVisibility = "compat"
	SchemaVisibilityInternal SchemaVisibility = "internal"
)

type SchemaHintRegistry struct {
	tools map[string]ToolSchemaHint
}

func boolPtr(v bool) *bool { return &v }

var defaultSchemaHintRegistry = newSchemaHintRegistry()

func newSchemaHintRegistry() *SchemaHintRegistry {
	return &SchemaHintRegistry{
		tools: map[string]ToolSchemaHint{},
	}
}

// RegisterSchemaHints registers curated hints for one product. Keys in tools may
// be either RPC names ("send_ding_message") or canonical paths
// ("ding.send_ding_message"). Duplicate canonical paths panic during init so
// conflicting product hint files are caught early.
func RegisterSchemaHints(productID string, tools map[string]ToolSchemaHint) {
	defaultSchemaHintRegistry.RegisterProduct(productID, tools)
}

func (r *SchemaHintRegistry) RegisterProduct(productID string, tools map[string]ToolSchemaHint) {
	productID = strings.TrimSpace(productID)
	if productID == "" {
		panic("schema hints: product id is required")
	}
	for name, hint := range tools {
		path := canonicalHintPath(productID, name)
		if path == "" {
			panic(fmt.Sprintf("schema hints: empty tool name for product %q", productID))
		}
		if _, exists := r.tools[path]; exists {
			panic(fmt.Sprintf("schema hints: duplicate hint for %s", path))
		}
		r.tools[path] = hint
	}
}

func (r *SchemaHintRegistry) Lookup(canonicalPath string) (ToolSchemaHint, bool) {
	canonicalPath = strings.TrimSpace(canonicalPath)
	if canonicalPath == "" {
		return ToolSchemaHint{}, false
	}
	hint, ok := r.tools[canonicalPath]
	return hint, ok
}

func canonicalHintPath(productID, name string) string {
	productID = strings.TrimSpace(productID)
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if strings.Contains(name, ".") {
		return name
	}
	return productID + "." + name
}

func schemaHintForCanonicalPath(canonicalPath string) ToolSchemaHint {
	if hint, ok := defaultSchemaHintRegistry.Lookup(canonicalPath); ok {
		return hint
	}
	return ToolSchemaHint{}
}

func lookupParameterSchemaHint(hints map[string]ParameterSchemaHint, paramName, flagName string) (ParameterSchemaHint, string, bool) {
	if len(hints) == 0 {
		return ParameterSchemaHint{}, "", false
	}
	if hint, ok := hints[paramName]; ok {
		return hint, paramName, true
	}
	if hint, ok := hints[flagName]; ok {
		return hint, flagName, true
	}
	return ParameterSchemaHint{}, "", false
}
