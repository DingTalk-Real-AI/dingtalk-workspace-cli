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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/ir"
)

// ToolSchemaHint is the hardcoded prompt/schema hint for a non-helper MCP tool.
// Product-specific files register these hints through RegisterSchemaHints.
// Helper and runtime commands can carry curated descriptions through this
// framework without fetching live MCP schemas.
type ToolSchemaHint struct {
	Title          string
	Description    string
	PrimaryCLIPath string
	Parameters     map[string]ParameterSchemaHint
}

// ParameterSchemaHint overrides the projected flat schema for one parameter.
// It can be keyed either by the original MCP parameter name (preferred) or by
// the final CLI flag name when a hint is purely CLI-facing.
type ParameterSchemaHint struct {
	FlagName     string
	Type         string
	Description  string
	Default      string
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
	tools             map[string]ToolSchemaHint
	runtimeRoots      map[string]RuntimeSchemaRootHint
	productVisibility map[string]SchemaVisibility
}

// RuntimeSchemaRootHint opts a hardcoded top-level command tree into `dws
// schema`. The schema renderer walks the actual Cobra leaves under ProductID
// and builds parameters from registered flags. ToolNames optionally maps a CLI
// path like "aitable record query" to the real MCP tool name; unmapped leaves use
// a stable path-derived name.
type RuntimeSchemaRootHint struct {
	Source          string
	ToolNames       map[string]string
	PrimaryCLIPaths map[string]string
	IncludeCLIPaths map[string]bool
}

func boolPtr(v bool) *bool { return &v }

var defaultSchemaHintRegistry = newSchemaHintRegistry()

func newSchemaHintRegistry() *SchemaHintRegistry {
	return &SchemaHintRegistry{
		tools:             map[string]ToolSchemaHint{},
		runtimeRoots:      map[string]RuntimeSchemaRootHint{},
		productVisibility: map[string]SchemaVisibility{},
	}
}

// RegisterSchemaHints registers curated hints for one product. Keys in tools may
// be either RPC names ("send_ding_message") or canonical paths
// ("ding.send_ding_message"). Duplicate canonical paths panic during init so
// conflicting product hint files are caught early.
func RegisterSchemaHints(productID string, tools map[string]ToolSchemaHint) {
	defaultSchemaHintRegistry.RegisterProduct(productID, tools)
}

// RegisterRuntimeSchemaRoot registers a hardcoded product command tree as a
// runtime schema source. Use this for products whose runnable commands are
// maintained in Go helpers rather than generated from ToolOverrides.
func RegisterRuntimeSchemaRoot(productID string, hint RuntimeSchemaRootHint) {
	defaultSchemaHintRegistry.RegisterRuntimeRoot(productID, hint)
}

// RegisterSchemaProductVisibility controls whether a top-level implementation
// product is independently exposed through Agent schema. Internal and compat
// products remain executable but must be represented by public canonical
// helpers or aliases instead of separate tools.
func RegisterSchemaProductVisibility(productID string, visibility SchemaVisibility) {
	defaultSchemaHintRegistry.RegisterProductVisibility(productID, visibility)
}

// SchemaProductVisibilityFor exposes the reviewed public/compat/internal
// classification to the application help layer. Non-public implementation
// roots stay executable for compatibility but are hidden from help and Schema.
func SchemaProductVisibilityFor(productID string) SchemaVisibility {
	return defaultSchemaHintRegistry.ProductVisibility(productID)
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

func (r *SchemaHintRegistry) RegisterRuntimeRoot(productID string, hint RuntimeSchemaRootHint) {
	productID = strings.TrimSpace(productID)
	if productID == "" {
		panic("schema hints: runtime schema root product id is required")
	}
	if _, exists := r.runtimeRoots[productID]; exists {
		panic(fmt.Sprintf("schema hints: duplicate runtime schema root for %s", productID))
	}
	normalized := RuntimeSchemaRootHint{
		Source:          strings.TrimSpace(hint.Source),
		ToolNames:       map[string]string{},
		PrimaryCLIPaths: map[string]string{},
		IncludeCLIPaths: map[string]bool{},
	}
	for path, toolName := range hint.ToolNames {
		path = strings.Join(splitSchemaPathTokens(path), " ")
		toolName = strings.TrimSpace(toolName)
		if path == "" || toolName == "" {
			continue
		}
		normalized.ToolNames[path] = toolName
	}
	for toolName, cliPath := range hint.PrimaryCLIPaths {
		toolName = strings.TrimSpace(toolName)
		cliPath = strings.Join(splitSchemaPathTokens(cliPath), " ")
		if toolName == "" || cliPath == "" {
			continue
		}
		normalized.PrimaryCLIPaths[toolName] = cliPath
	}
	for cliPath, included := range hint.IncludeCLIPaths {
		cliPath = strings.Join(splitSchemaPathTokens(cliPath), " ")
		if cliPath == "" || !included {
			continue
		}
		normalized.IncludeCLIPaths[cliPath] = true
	}
	r.runtimeRoots[productID] = normalized
}

func (r *SchemaHintRegistry) RegisterProductVisibility(productID string, visibility SchemaVisibility) {
	productID = strings.TrimSpace(productID)
	if productID == "" {
		panic("schema hints: product visibility product id is required")
	}
	switch visibility {
	case SchemaVisibilityPublic, SchemaVisibilityCompat, SchemaVisibilityInternal:
	default:
		panic(fmt.Sprintf("schema hints: unsupported visibility %q for %s", visibility, productID))
	}
	if _, exists := r.productVisibility[productID]; exists {
		panic(fmt.Sprintf("schema hints: duplicate product visibility for %s", productID))
	}
	r.productVisibility[productID] = visibility
}

func (r *SchemaHintRegistry) Lookup(canonicalPath string) (ToolSchemaHint, bool) {
	canonicalPath = strings.TrimSpace(canonicalPath)
	if canonicalPath == "" {
		return ToolSchemaHint{}, false
	}
	hint, ok := r.tools[canonicalPath]
	return hint, ok
}

func (r *SchemaHintRegistry) RuntimeRoots() map[string]RuntimeSchemaRootHint {
	out := make(map[string]RuntimeSchemaRootHint, len(r.runtimeRoots))
	for productID, hint := range r.runtimeRoots {
		copied := RuntimeSchemaRootHint{Source: hint.Source}
		if len(hint.ToolNames) > 0 {
			copied.ToolNames = make(map[string]string, len(hint.ToolNames))
			for path, toolName := range hint.ToolNames {
				copied.ToolNames[path] = toolName
			}
		}
		if len(hint.PrimaryCLIPaths) > 0 {
			copied.PrimaryCLIPaths = make(map[string]string, len(hint.PrimaryCLIPaths))
			for toolName, cliPath := range hint.PrimaryCLIPaths {
				copied.PrimaryCLIPaths[toolName] = cliPath
			}
		}
		if len(hint.IncludeCLIPaths) > 0 {
			copied.IncludeCLIPaths = make(map[string]bool, len(hint.IncludeCLIPaths))
			for cliPath, included := range hint.IncludeCLIPaths {
				copied.IncludeCLIPaths[cliPath] = included
			}
		}
		out[productID] = copied
	}
	return out
}

func (r *SchemaHintRegistry) ProductVisibility(productID string) SchemaVisibility {
	productID = strings.TrimSpace(productID)
	if visibility := r.productVisibility[productID]; visibility != "" {
		return visibility
	}
	return SchemaVisibilityPublic
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

func schemaHintForTool(tool ir.ToolDescriptor) ToolSchemaHint {
	return schemaHintForCanonicalPath(tool.CanonicalPath)
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

func sortedParameterHintKeys(hints map[string]ParameterSchemaHint) []string {
	keys := make([]string, 0, len(hints))
	for key := range hints {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
