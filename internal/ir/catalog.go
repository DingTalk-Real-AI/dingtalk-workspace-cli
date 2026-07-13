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

package ir

import (
	"strings"
)

type Catalog struct {
	Products []CanonicalProduct `json:"products"`
}

type LifecycleInfo struct {
	DeprecatedBy        int    `json:"deprecated_by,omitempty"`
	DeprecationDate     string `json:"deprecation_date,omitempty"`
	MigrationURL        string `json:"migration_url,omitempty"`
	DeprecatedCandidate bool   `json:"deprecated_candidate,omitempty"`
}

type ProductCLIMetadata struct {
	Command  string   `json:"command,omitempty"`
	Group    string   `json:"group,omitempty"`
	Hidden   bool     `json:"hidden,omitempty"`
	Skip     bool     `json:"skip,omitempty"`
	Aliases  []string `json:"aliases,omitempty"`
	Prefixes []string `json:"prefixes,omitempty"`
}

type CLIFlagHint struct {
	Shorthand string `json:"shorthand,omitempty"`
	Alias     string `json:"alias,omitempty"`
}

// FlagOverlay carries CLI-layer transformation metadata for a single
// MCP parameter: the flag alias the user types, the transform applied
// before dispatch, env-var fallback, default value, and whether the flag
// is hidden from help. Sourced from market.CLIToolOverride.Flags.
type FlagOverlay struct {
	Alias         string         `json:"alias,omitempty"`
	Transform     string         `json:"transform,omitempty"`
	TransformArgs map[string]any `json:"transform_args,omitempty"`
	EnvDefault    string         `json:"env_default,omitempty"`
	Default       string         `json:"default,omitempty"`
	Description   string         `json:"description,omitempty"`
	Hidden        bool           `json:"hidden,omitempty"`
}

// ToolAnnotations mirrors MCP 2025+ tool annotations. All hints are
// nullable: absence means "unknown", not "false". Populate only when the
// source has a clear signal — don't guess.
type ToolAnnotations struct {
	DestructiveHint *bool `json:"destructive_hint,omitempty"`
	ReadOnlyHint    *bool `json:"read_only_hint,omitempty"`
	IdempotentHint  *bool `json:"idempotent_hint,omitempty"`
	OpenWorldHint   *bool `json:"open_world_hint,omitempty"`
}

type CanonicalProduct struct {
	ID                        string              `json:"id"`
	DisplayName               string              `json:"display_name"`
	Description               string              `json:"description,omitempty"`
	ServerKey                 string              `json:"server_key"`
	Endpoint                  string              `json:"endpoint"`
	SchemaURI                 string              `json:"schema_uri,omitempty"`
	NegotiatedProtocolVersion string              `json:"negotiated_protocol_version,omitempty"`
	Source                    string              `json:"source,omitempty"`
	Degraded                  bool                `json:"degraded"`
	Lifecycle                 *LifecycleInfo      `json:"lifecycle,omitempty"`
	CLI                       *ProductCLIMetadata `json:"cli,omitempty"`
	Tools                     []ToolDescriptor    `json:"tools"`
}

type ToolDescriptor struct {
	RPCName         string                 `json:"rpc_name"`
	CLIName         string                 `json:"cli_name,omitempty"`
	Group           string                 `json:"group,omitempty"`
	Title           string                 `json:"title,omitempty"`
	Description     string                 `json:"description,omitempty"`
	InputSchema     map[string]any         `json:"input_schema,omitempty"`
	OutputSchema    map[string]any         `json:"output_schema,omitempty"`
	Sensitive       bool                   `json:"sensitive"`
	Auth            *ToolAuthMetadata      `json:"auth,omitempty"`
	Annotations     *ToolAnnotations       `json:"annotations,omitempty"`
	Hidden          bool                   `json:"hidden,omitempty"`
	FlagHints       map[string]CLIFlagHint `json:"flag_hints,omitempty"`
	FlagOverlay     map[string]FlagOverlay `json:"flag_overlay,omitempty"`
	SourceServerKey string                 `json:"source_server_key"`
	CanonicalPath   string                 `json:"canonical_path"`
}

type ToolAuthMetadata struct {
	Version              string   `json:"version,omitempty"`
	ProductCode          string   `json:"productCode,omitempty"`
	Domain               string   `json:"domain,omitempty"`
	ClientObservedScopes []string `json:"clientObservedScopes,omitempty"`
	RequiredScopes       []string `json:"requiredScopes,omitempty"`
	RequiredPermissions  []string `json:"requiredPermissions,omitempty"`
	RecommendedScopes    []string `json:"recommendedScopes,omitempty"`
	ExcludedScopes       []string `json:"excludedScopes,omitempty"`
	GrantProductCodes    []string `json:"grantProductCodes,omitempty"`
	RiskHint             string   `json:"riskHint,omitempty"`
	RiskAction           string   `json:"riskAction,omitempty"`
	ConfirmationRequired bool     `json:"confirmationRequired,omitempty"`
	Identities           []string `json:"identities,omitempty"`
	Source               string   `json:"source,omitempty"`
	AuthMetaVersion      string   `json:"authMetaVersion,omitempty"`
	AuthMetaHash         string   `json:"authMetaHash,omitempty"`
}

func (c Catalog) FindProduct(id string) (CanonicalProduct, bool) {
	for _, product := range c.Products {
		if product.ID == id {
			return product, true
		}
	}
	return CanonicalProduct{}, false
}

func (c Catalog) FindTool(path string) (CanonicalProduct, ToolDescriptor, bool) {
	productID, toolName, ok := strings.Cut(path, ".")
	if !ok || productID == "" || toolName == "" {
		return CanonicalProduct{}, ToolDescriptor{}, false
	}
	product, ok := c.FindProduct(productID)
	if !ok {
		return CanonicalProduct{}, ToolDescriptor{}, false
	}
	tool, ok := product.FindTool(toolName)
	if !ok {
		return CanonicalProduct{}, ToolDescriptor{}, false
	}
	return product, tool, true
}

func (p CanonicalProduct) FindTool(name string) (ToolDescriptor, bool) {
	for _, tool := range p.Tools {
		if tool.RPCName == name {
			return tool, true
		}
	}
	return ToolDescriptor{}, false
}
