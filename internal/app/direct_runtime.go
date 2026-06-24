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

package app

import (
	"net"
	"net/url"
	"os"
	"strings"
	"sync"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/market"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

var (
	dynamicMu            sync.RWMutex
	dynamicEndpoints     map[string]string
	dynamicProducts      map[string]bool
	dynamicAliases       map[string]string
	dynamicToolEndpoints map[string]string // tool name → endpoint
)

var legacyDirectRuntimeAliases = map[string]string{
	"tb":                       "teambition",
	"dingtalk-discovery":       "discovery",
	"dingtalk-oa-plus":         "oa",
	"dingtalk-ai-sincere-hire": "ai-sincere-hire",
}

const (
	defaultPATProductID   = "pat"
	defaultPATDisplayName = "行为授权"
	defaultPATServerID    = "abc3c880fb90f04b52d1426aaf093766e5fc9ec38411688cbb74df42a584d374"
	devappProductID       = "devapp"
	devappServerPath      = "/server/op-app"
)

// devappMCPEndpoint resolves the open-platform app-management MCP endpoint
// from the configured gateway base URL, so it follows the active environment
// (production by default, pre when ~/.dws/mcp_url points at the pre gateway).
func devappMCPEndpoint() string {
	return defaultPATGatewayBaseURL() + devappServerPath
}

func defaultPATServerDescriptor() market.ServerDescriptor {
	return market.ServerDescriptor{
		Key:         defaultPATProductID,
		DisplayName: defaultPATDisplayName,
		Endpoint:    defaultPATMCPEndpoint(),
		CLI: market.CLIOverlay{
			ID:       defaultPATProductID,
			Command:  defaultPATProductID,
			Prefixes: []string{defaultPATProductID},
		},
	}
}

func defaultPATMCPEndpoint() string {
	return defaultPATGatewayBaseURL() + "/server/" + defaultPATServerID
}

func defaultPATGatewayBaseURL() string {
	raw := strings.TrimSpace(authpkg.GetMCPBaseURL())
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return strings.TrimRight(raw, "/")
	}

	host := parsed.Hostname()
	switch {
	case host == "mcp.dingtalk.com":
		host = "mcp-gw.dingtalk.com"
	case strings.HasPrefix(host, "pre-mcp."):
		host = strings.Replace(host, "pre-mcp.", "pre-mcp-gw.", 1)
	case strings.HasPrefix(host, "mcp."):
		host = strings.Replace(host, "mcp.", "mcp-gw.", 1)
	}

	if port := parsed.Port(); port != "" {
		parsed.Host = net.JoinHostPort(host, port)
	} else {
		parsed.Host = host
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/")
}

// SetDynamicServers injects server data discovered from servers.json.
// All product endpoints are resolved dynamically from this data.
func SetDynamicServers(servers []market.ServerDescriptor) {
	dynamicMu.Lock()
	defer dynamicMu.Unlock()

	endpoints := make(map[string]string)
	products := make(map[string]bool)
	aliases := make(map[string]string)
	toolEndpoints := make(map[string]string)
	registerDynamicServer(defaultPATServerDescriptor(), endpoints, products, aliases, toolEndpoints)
	for _, server := range servers {
		if server.CLI.Skip {
			continue
		}
		id := strings.TrimSpace(server.CLI.ID)
		endpoint := strings.TrimSpace(server.Endpoint)
		if id != "" && endpoint != "" {
			endpoints[id] = endpoint
			products[id] = true
		}
		cmd := strings.TrimSpace(server.CLI.Command)
		if cmd != "" && cmd != id && endpoint != "" {
			endpoints[cmd] = endpoint
			products[cmd] = true
		}
		for _, alias := range server.CLI.Aliases {
			alias = strings.TrimSpace(alias)
			if alias != "" && endpoint != "" {
				endpoints[alias] = endpoint
				products[alias] = true
				// Build alias → CLI.ID mapping
				aliases[alias] = id
			}
		}
		// Build tool → endpoint mapping from CLI tools and overrides.
		if endpoint != "" {
			for _, tool := range server.CLI.Tools {
				toolName := strings.TrimSpace(tool.Name)
				if toolName != "" {
					toolEndpoints[toolName] = endpoint
				}
			}
			for toolName, override := range server.CLI.ToolOverrides {
				toolName = strings.TrimSpace(toolName)
				if toolName == "" {
					continue
				}
				// Leaves with serverOverride are routed to a different server's
				// endpoint (e.g. chat's "search_my_robots" → bot). Registering
				// them here would overwrite the real owner's tool → endpoint
				// mapping and send the invocation to the wrong MCP URL.
				if strings.TrimSpace(override.ServerOverride) != "" {
					continue
				}
				toolEndpoints[toolName] = endpoint
			}
		}
	}
	dynamicEndpoints = endpoints
	dynamicProducts = products
	dynamicAliases = aliases
	dynamicToolEndpoints = toolEndpoints
}

func registerDynamicServer(server market.ServerDescriptor, endpoints map[string]string, products map[string]bool, aliases map[string]string, toolEndpoints map[string]string) {
	if server.CLI.Skip {
		return
	}
	id := strings.TrimSpace(server.CLI.ID)
	endpoint := strings.TrimSpace(server.Endpoint)
	if id != "" && endpoint != "" {
		endpoints[id] = endpoint
		products[id] = true
	}
	cmd := strings.TrimSpace(server.CLI.Command)
	if cmd != "" && cmd != id && endpoint != "" {
		endpoints[cmd] = endpoint
		products[cmd] = true
	}
	for _, alias := range server.CLI.Aliases {
		alias = strings.TrimSpace(alias)
		if alias != "" && endpoint != "" {
			endpoints[alias] = endpoint
			products[alias] = true
			// Build alias -> CLI.ID mapping.
			aliases[alias] = id
		}
	}
	// Build tool -> endpoint mapping from CLI tools and overrides.
	if endpoint != "" {
		for _, tool := range server.CLI.Tools {
			toolName := strings.TrimSpace(tool.Name)
			if toolName != "" {
				toolEndpoints[toolName] = endpoint
			}
		}
		for toolName := range server.CLI.ToolOverrides {
			toolName = strings.TrimSpace(toolName)
			if toolName != "" {
				toolEndpoints[toolName] = endpoint
			}
		}
	}
}

func shouldUseDirectRuntime(invocation executor.Invocation) bool {
	if strings.TrimSpace(os.Getenv(cli.CatalogFixtureEnv)) != "" {
		return false
	}
	switch invocation.Kind {
	case "compat_invocation", "helper_invocation":
		return true
	default:
		return false
	}
}

// directRuntimeToolEndpoint returns the MCP endpoint owned by the server
// whose toolOverrides registered this tool name. Used to correct catalog
// lookups when two envelope servers share the same cli.command and the
// per-product endpoint map collides (see runner.go cross-check).
func directRuntimeToolEndpoint(toolName string) (string, bool) {
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return "", false
	}
	dynamicMu.RLock()
	defer dynamicMu.RUnlock()

	endpoint, ok := dynamicToolEndpoints[toolName]
	return endpoint, ok && strings.TrimSpace(endpoint) != ""
}

func directRuntimeEndpoint(productID, toolName string) (string, bool) {
	// Priority 0: env-var override always wins (DINGTALK_<PRODUCT>_MCP_URL).
	normalized := normalizeDirectRuntimeProductID(productID)
	for _, candidate := range []string{strings.TrimSpace(productID), normalized} {
		if candidate == "" {
			continue
		}
		if override, ok := productEndpointOverride(candidate); ok {
			return override, true
		}
	}

	// Hardcoded built-in: devapp is pinned to the open-platform app-management
	// MCP server in source (NOT service discovery), per product decision.
	for _, candidate := range []string{strings.TrimSpace(productID), normalized} {
		if candidate == devappProductID {
			return devappMCPEndpoint(), true
		}
	}

	// Priority 1: product-level endpoint.
	// When the caller already knows the productID (e.g. "drive"), the product
	// endpoint is authoritative. This prevents cross-product tool name
	// collisions (e.g. both "drive" and "doc" register "create_folder") from
	// routing the request to the wrong MCP server. See issue #219.
	dynamicMu.RLock()
	for _, candidate := range []string{strings.TrimSpace(productID), normalized} {
		if candidate == "" {
			continue
		}
		if endpoint, ok := dynamicEndpoints[candidate]; ok {
			dynamicMu.RUnlock()
			return endpoint, true
		}
	}

	// Priority 2: tool-level endpoint (fallback for unknown productID).
	// This path is used when the caller does not know the productID but has a
	// tool name, e.g. in helper invocations or plugin routes where only the
	// tool name is available.
	if tool := strings.TrimSpace(toolName); tool != "" {
		if endpoint, ok := dynamicToolEndpoints[tool]; ok {
			dynamicMu.RUnlock()
			return endpoint, true
		}
	}
	dynamicMu.RUnlock()

	// Priority 3: built-in PAT fallback for cold-start paths that run before
	// discovery/plugin registration has populated the dynamic registry.
	for _, candidate := range []string{strings.TrimSpace(productID), normalized} {
		if candidate == defaultPATProductID {
			return defaultPATMCPEndpoint(), true
		}
	}

	// Priority 4: edition-owned static/supplement endpoints. Helper-only
	// products such as devapp intentionally do not depend on Market discovery,
	// so the internal edition may provide only an endpoint and no tool list.
	for _, candidate := range []string{strings.TrimSpace(productID), normalized} {
		if endpoint, ok := editionServerEndpoint(candidate); ok {
			return endpoint, true
		}
	}
	return "", false
}

func editionServerEndpoint(productID string) (string, bool) {
	productID = strings.TrimSpace(productID)
	if productID == "" {
		return "", false
	}
	hooks := edition.Get()
	if hooks == nil {
		return "", false
	}
	if endpoint, ok := endpointFromEditionServers(productID, hooks.StaticServers); ok {
		return endpoint, true
	}
	if endpoint, ok := endpointFromEditionServers(productID, hooks.SupplementServers); ok {
		return endpoint, true
	}
	return "", false
}

func endpointFromEditionServers(productID string, fn func() []edition.ServerInfo) (string, bool) {
	if fn == nil {
		return "", false
	}
	for _, server := range fn() {
		endpoint := strings.TrimSpace(server.Endpoint)
		if endpoint == "" {
			continue
		}
		if strings.TrimSpace(server.ID) == productID {
			return endpoint, true
		}
		for _, prefix := range server.Prefixes {
			if strings.TrimSpace(prefix) == productID {
				return endpoint, true
			}
		}
	}
	return "", false
}

// DirectRuntimeProductIDs returns product IDs that should stay visible for
// direct runtime execution. Dynamic products come from MCP discovery/plugin
// registration; built-in helper products such as devapp resolve their endpoint
// through DINGTALK_<PRODUCT>_MCP_URL instead of requiring discovery.
func DirectRuntimeProductIDs() map[string]bool {
	dynamicMu.RLock()
	defer dynamicMu.RUnlock()

	ids := make(map[string]bool, len(dynamicProducts)+2)
	ids[defaultPATProductID] = true
	ids[devappProductID] = true
	for key := range dynamicProducts {
		ids[key] = true
	}
	return ids
}

// AppendDynamicServer adds a single server descriptor to the existing
// dynamic server registry without replacing the current entries. This
// is used by the plugin loader to inject plugin servers alongside
// Market-discovered servers.
func AppendDynamicServer(server market.ServerDescriptor) {
	dynamicMu.Lock()
	defer dynamicMu.Unlock()

	if dynamicEndpoints == nil {
		dynamicEndpoints = make(map[string]string)
	}
	if dynamicProducts == nil {
		dynamicProducts = make(map[string]bool)
	}
	if dynamicAliases == nil {
		dynamicAliases = make(map[string]string)
	}
	if dynamicToolEndpoints == nil {
		dynamicToolEndpoints = make(map[string]string)
	}

	if server.CLI.Skip {
		return
	}

	id := strings.TrimSpace(server.CLI.ID)
	endpoint := strings.TrimSpace(server.Endpoint)
	if id != "" && endpoint != "" {
		dynamicEndpoints[id] = endpoint
		dynamicProducts[id] = true
	}
	cmd := strings.TrimSpace(server.CLI.Command)
	if cmd != "" && cmd != id && endpoint != "" {
		if _, exists := dynamicEndpoints[cmd]; !exists {
			dynamicEndpoints[cmd] = endpoint
		}
		dynamicProducts[cmd] = true
	}
	for _, alias := range server.CLI.Aliases {
		alias = strings.TrimSpace(alias)
		if alias != "" && endpoint != "" {
			dynamicEndpoints[alias] = endpoint
			dynamicProducts[alias] = true
			dynamicAliases[alias] = id
		}
	}
	if endpoint != "" {
		for _, tool := range server.CLI.Tools {
			toolName := strings.TrimSpace(tool.Name)
			if toolName != "" {
				dynamicToolEndpoints[toolName] = endpoint
			}
		}
		for toolName, override := range server.CLI.ToolOverrides {
			toolName = strings.TrimSpace(toolName)
			if toolName == "" {
				continue
			}
			// Leaves with serverOverride are routed to a different server's
			// endpoint; skip to avoid overwriting the real owner's mapping.
			if strings.TrimSpace(override.ServerOverride) != "" {
				continue
			}
			dynamicToolEndpoints[toolName] = endpoint
		}
	}
}

func normalizeDirectRuntimeProductID(productID string) string {
	trimmed := strings.TrimSpace(productID)
	dynamicMu.RLock()
	if normalizedID, ok := dynamicAliases[trimmed]; ok && normalizedID != "" {
		dynamicMu.RUnlock()
		return normalizedID
	}
	dynamicMu.RUnlock()

	if normalizedID, ok := legacyDirectRuntimeAliases[trimmed]; ok {
		return normalizedID
	}
	return trimmed
}
