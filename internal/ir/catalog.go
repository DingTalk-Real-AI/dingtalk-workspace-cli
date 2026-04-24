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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"unicode"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/discovery"
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
	Command string `json:"command,omitempty"`
	Group   string `json:"group,omitempty"`
	Hidden  bool   `json:"hidden,omitempty"`
	Skip    bool   `json:"skip,omitempty"`
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
	Annotations     *ToolAnnotations       `json:"annotations,omitempty"`
	Hidden          bool                   `json:"hidden,omitempty"`
	FlagHints       map[string]CLIFlagHint `json:"flag_hints,omitempty"`
	FlagOverlay     map[string]FlagOverlay `json:"flag_overlay,omitempty"`
	SourceServerKey string                 `json:"source_server_key"`
	CanonicalPath   string                 `json:"canonical_path"`
}

func BuildCatalog(runtimeServers []discovery.RuntimeServer) Catalog {
	sorted := append([]discovery.RuntimeServer(nil), runtimeServers...)
	sort.Slice(sorted, func(i, j int) bool {
		left := sorted[i].Server
		right := sorted[j].Server
		if left.DisplayName != right.DisplayName {
			return left.DisplayName < right.DisplayName
		}
		if left.Endpoint != right.Endpoint {
			return left.Endpoint < right.Endpoint
		}
		return left.Key < right.Key
	})

	usedIDs := make(map[string]struct{}, len(sorted))
	products := make([]CanonicalProduct, 0, len(sorted))
	for _, runtimeServer := range sorted {
		productID := nextCanonicalProductID(
			runtimeServer.Server.Key,
			runtimeServer.Server.DisplayName,
			runtimeServer.Server.Endpoint,
			runtimeServer.Server.CLI.Command,
			usedIDs,
		)
		sensitiveByTool := make(map[string]bool, len(runtimeServer.Server.CLI.Tools))
		hasSensitiveOverride := make(map[string]bool, len(runtimeServer.Server.CLI.Tools))
		toolCLIName := make(map[string]string, len(runtimeServer.Server.CLI.Tools))
		toolGroup := make(map[string]string)
		toolHidden := make(map[string]bool, len(runtimeServer.Server.CLI.Tools))
		toolTitleOverride := make(map[string]string, len(runtimeServer.Server.CLI.Tools))
		toolDescriptionOverride := make(map[string]string, len(runtimeServer.Server.CLI.Tools))
		toolFlagHints := make(map[string]map[string]CLIFlagHint, len(runtimeServer.Server.CLI.Tools))
		toolFlagOverlay := make(map[string]map[string]FlagOverlay)

		// Merge richer market.CLIToolOverride data (group, cliName, flag
		// transforms/aliases/env-defaults) that the dynamic compat layer
		// already consumes but the IR previously dropped.
		for name, override := range runtimeServer.Server.CLI.ToolOverrides {
			trimmed := strings.TrimSpace(name)
			if trimmed == "" {
				continue
			}
			if cn := strings.TrimSpace(override.CLIName); cn != "" {
				toolCLIName[trimmed] = cn
			}
			if gp := strings.TrimSpace(override.Group); gp != "" {
				toolGroup[trimmed] = gp
			}
			if override.Hidden {
				toolHidden[trimmed] = true
			}
			if override.IsSensitive {
				sensitiveByTool[trimmed] = true
				hasSensitiveOverride[trimmed] = true
			}
			if desc := strings.TrimSpace(override.Description); desc != "" && toolDescriptionOverride[trimmed] == "" {
				toolDescriptionOverride[trimmed] = desc
			}
			if len(override.Flags) > 0 {
				overlay := make(map[string]FlagOverlay, len(override.Flags))
				for paramName, flagOverride := range override.Flags {
					paramTrim := strings.TrimSpace(paramName)
					if paramTrim == "" {
						continue
					}
					overlay[paramTrim] = FlagOverlay{
						Alias:         strings.TrimSpace(flagOverride.Alias),
						Transform:     strings.TrimSpace(flagOverride.Transform),
						TransformArgs: cloneTransformArgs(flagOverride.TransformArgs),
						EnvDefault:    strings.TrimSpace(flagOverride.EnvDefault),
						Default:       strings.TrimSpace(flagOverride.Default),
						Hidden:        flagOverride.Hidden,
					}
				}
				if len(overlay) > 0 {
					toolFlagOverlay[trimmed] = overlay
				}
			}
		}
		for _, cliTool := range runtimeServer.Server.CLI.Tools {
			name := strings.TrimSpace(cliTool.Name)
			if name == "" {
				continue
			}
			sensitiveByTool[name] = cliTool.IsSensitive
			hasSensitiveOverride[name] = true
			toolCLIName[name] = strings.TrimSpace(cliTool.CLIName)
			toolHidden[name] = cliTool.Hidden
			toolTitleOverride[name] = strings.TrimSpace(cliTool.Title)
			toolDescriptionOverride[name] = strings.TrimSpace(cliTool.Description)
			if len(cliTool.Flags) > 0 {
				hints := make(map[string]CLIFlagHint, len(cliTool.Flags))
				for key, hint := range cliTool.Flags {
					trimmed := strings.TrimSpace(key)
					if trimmed == "" {
						continue
					}
					hints[trimmed] = CLIFlagHint{
						Shorthand: strings.TrimSpace(hint.Shorthand),
						Alias:     strings.TrimSpace(hint.Alias),
					}
				}
				if len(hints) > 0 {
					toolFlagHints[name] = hints
				}
			}
		}
		tools := make([]ToolDescriptor, 0, len(runtimeServer.Tools))
		for _, tool := range runtimeServer.Tools {
			title := strings.TrimSpace(tool.Title)
			if override := strings.TrimSpace(toolTitleOverride[tool.Name]); override != "" {
				title = override
			}
			if title == "" {
				title = tool.Name
			}
			description := strings.TrimSpace(tool.Description)
			if override := strings.TrimSpace(toolDescriptionOverride[tool.Name]); override != "" {
				description = override
			}
			if description == "" {
				description = title
			}
			sensitive := tool.Sensitive
			if hasSensitiveOverride[tool.Name] {
				sensitive = sensitiveByTool[tool.Name]
			}
			cliName := strings.TrimSpace(toolCLIName[tool.Name])
			if cliName == "" {
				cliName = tool.Name
			}
			tools = append(tools, ToolDescriptor{
				RPCName:         tool.Name,
				CLIName:         cliName,
				Group:           strings.TrimSpace(toolGroup[tool.Name]),
				Title:           title,
				Description:     description,
				InputSchema:     cloneMap(tool.InputSchema),
				OutputSchema:    cloneMap(tool.OutputSchema),
				Sensitive:       sensitive,
				Annotations:     deriveAnnotations(sensitive),
				Hidden:          toolHidden[tool.Name],
				FlagHints:       cloneFlagHints(toolFlagHints[tool.Name]),
				FlagOverlay:     cloneFlagOverlay(toolFlagOverlay[tool.Name]),
				SourceServerKey: runtimeServer.Server.Key,
				CanonicalPath:   fmt.Sprintf("%s.%s", productID, tool.Name),
			})
		}
		sort.Slice(tools, func(i, j int) bool {
			return tools[i].RPCName < tools[j].RPCName
		})

		lifecycle := LifecycleInfo{
			DeprecatedBy:        runtimeServer.Server.Lifecycle.DeprecatedBy,
			DeprecationDate:     strings.TrimSpace(runtimeServer.Server.Lifecycle.DeprecationDate),
			MigrationURL:        strings.TrimSpace(runtimeServer.Server.Lifecycle.MigrationURL),
			DeprecatedCandidate: runtimeServer.Server.Lifecycle.DeprecatedCandidate,
		}
		var lifecyclePtr *LifecycleInfo
		if lifecycle.DeprecatedBy > 0 || lifecycle.DeprecationDate != "" || lifecycle.MigrationURL != "" || lifecycle.DeprecatedCandidate {
			lifecycleCopy := lifecycle
			lifecyclePtr = &lifecycleCopy
		}
		cliMeta := ProductCLIMetadata{
			Command: strings.TrimSpace(runtimeServer.Server.CLI.Command),
			Group:   strings.TrimSpace(runtimeServer.Server.CLI.Group),
			Hidden:  runtimeServer.Server.CLI.Hidden,
			Skip:    runtimeServer.Server.CLI.Skip,
		}
		var cliPtr *ProductCLIMetadata
		if cliMeta.Command != "" || cliMeta.Group != "" || cliMeta.Hidden || cliMeta.Skip {
			cliCopy := cliMeta
			cliPtr = &cliCopy
		}

		description := runtimeServer.Server.Description
		if cliDescription := strings.TrimSpace(runtimeServer.Server.CLI.Description); cliDescription != "" {
			description = cliDescription
		}

		products = append(products, CanonicalProduct{
			ID:                        productID,
			DisplayName:               runtimeServer.Server.DisplayName,
			Description:               description,
			ServerKey:                 runtimeServer.Server.Key,
			Endpoint:                  runtimeServer.Server.Endpoint,
			SchemaURI:                 runtimeServer.Server.SchemaURI,
			NegotiatedProtocolVersion: runtimeServer.NegotiatedProtocolVersion,
			Source:                    runtimeServer.Source,
			Degraded:                  runtimeServer.Degraded,
			Lifecycle:                 lifecyclePtr,
			CLI:                       cliPtr,
			Tools:                     tools,
		})
	}

	return Catalog{Products: products}
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

func nextCanonicalProductID(key, displayName, endpoint, cliCommand string, usedIDs map[string]struct{}) string {
	base := canonicalProductID(key, displayName, endpoint, cliCommand)
	id := base
	if _, exists := usedIDs[id]; exists {
		id = fmt.Sprintf("%s-%s", base, shorten(key))
	}
	usedIDs[id] = struct{}{}
	return id
}

func canonicalProductID(key, displayName, endpoint, cliCommand string) string {
	for _, candidate := range []string{
		slugify(cliCommand),
		endpointSlug(endpoint),
		slugify(displayName),
		slugify(key),
	} {
		if candidate != "" {
			return canonicalProductAlias(candidate)
		}
	}
	return "srv-" + shorten(key)
}

func canonicalProductAlias(id string) string {
	switch id {
	case "table":
		return "aitable"
	default:
		return id
	}
}

func endpointSlug(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	path := strings.Trim(parsed.Path, "/")
	if path == "" {
		return ""
	}
	parts := strings.Split(path, "/")
	return slugify(parts[len(parts)-1])
}

func slugify(value string) string {
	var builder strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
			lastDash = false
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
			lastDash = false
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			continue
		default:
			if !lastDash && builder.Len() > 0 {
				builder.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(builder.String(), "-")
}

func shorten(value string) string {
	if strings.TrimSpace(value) == "" {
		sum := sha256.Sum256([]byte("dws"))
		return hex.EncodeToString(sum[:])[:8]
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:8]
}

func cloneMap(value map[string]any) map[string]any {
	if len(value) == 0 {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var cloned map[string]any
	if err := json.Unmarshal(data, &cloned); err != nil {
		return nil
	}
	return cloned
}

func cloneFlagHints(value map[string]CLIFlagHint) map[string]CLIFlagHint {
	if len(value) == 0 {
		return nil
	}
	out := make(map[string]CLIFlagHint, len(value))
	for key, hint := range value {
		out[key] = hint
	}
	return out
}

func cloneFlagOverlay(value map[string]FlagOverlay) map[string]FlagOverlay {
	if len(value) == 0 {
		return nil
	}
	out := make(map[string]FlagOverlay, len(value))
	for key, overlay := range value {
		overlay.TransformArgs = cloneTransformArgs(overlay.TransformArgs)
		out[key] = overlay
	}
	return out
}

func cloneTransformArgs(value map[string]any) map[string]any {
	if len(value) == 0 {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var cloned map[string]any
	if err := json.Unmarshal(data, &cloned); err != nil {
		return nil
	}
	return cloned
}

// deriveAnnotations maps the coarse-grained Sensitive bool onto MCP 2025+
// annotation hints. Only DestructiveHint is populated — the other hints
// (read_only, idempotent, open_world) stay nil until the upstream tool
// manifest advertises them explicitly. Guessing from name prefixes
// ("list_", "get_") risks false signals for AI agents.
func deriveAnnotations(sensitive bool) *ToolAnnotations {
	if !sensitive {
		return nil
	}
	destructive := true
	return &ToolAnnotations{DestructiveHint: &destructive}
}
