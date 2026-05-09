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

package market

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

const (
	defaultBaseURL      = "https://pre-mcp.dingtalk.com"
	registryMetadataKey = "com.dingtalk.mcp.registry/metadata"
)

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
	Headers    map[string]string
}

type ListResponse struct {
	Metadata ListMetadata     `json:"metadata"`
	Servers  []ServerEnvelope `json:"servers"`
}

type ListMetadata struct {
	Count      int    `json:"count"`
	NextCursor string `json:"nextCursor"`
	// Warnings is populated by the Portal merge service when envelopes are
	// dropped (e.g. missing serverDeps, status != active) or when a dangling
	// serverDeps / toolOverrides.*.serverOverride reference is detected.
	// Old Portals without the field simply leave this nil. CLI side should
	// treat any non-empty slice as non-fatal informational output: print to
	// stderr so cache refreshes expose Portal drift to the user, but do not
	// fail the discovery load.
	//
	// See plan fix-wukong-discovery-missing-servers Phase 4.2/4.3.
	Warnings []ListWarning `json:"warnings,omitempty"`
}

// ListWarning describes one envelope that was filtered out of the merged
// response or flagged for dangling references. Fields mirror the JSON emitted
// by WukongDiscoveryRegistry.buildWarning on the Portal side; unknown reason
// codes are passed through verbatim for forward compatibility.
type ListWarning struct {
	ProductID string `json:"productId"`
	Reason    string `json:"reason"`
	Detail    string `json:"detail"`
}

type ServerEnvelope struct {
	Server RegistryServer `json:"server"`
	Meta   EnvelopeMeta   `json:"_meta"`
}

type EnvelopeMeta struct {
	Registry RegistryMetadata `json:"com.dingtalk.mcp.registry/metadata"`
	CLI      CLIOverlay       `json:"com.dingtalk.mcp.registry/cli"`
}

type RegistryMetadata struct {
	IsLatest    bool            `json:"isLatest"`
	PublishedAt string          `json:"publishedAt"`
	UpdatedAt   string          `json:"updatedAt"`
	Status      string          `json:"status"`
	MCPID       int             `json:"mcpId"`
	DetailURL   string          `json:"detailUrl"`
	Quality     QualityMetadata `json:"quality"`
	Lifecycle   LifecycleInfo   `json:"lifecycle"`
}

type QualityMetadata struct {
	HighQuality bool `json:"highQuality"`
	Official    bool `json:"official"`
	DTBiz       bool `json:"dtBiz"`
}

type LifecycleInfo struct {
	DeprecatedBy        int    `json:"deprecatedBy"`
	DeprecationDate     string `json:"deprecationDate"`
	MigrationURL        string `json:"migrationUrl"`
	DeprecatedCandidate bool   `json:"deprecatedCandidate,omitempty"`
}

type CLIOverlay struct {
	ID            string                     `json:"id"`
	Command       string                     `json:"command"`
	Parent        string                     `json:"parent,omitempty"`
	Description   string                     `json:"description"`
	Prefixes      []string                   `json:"prefixes"`
	Aliases       []string                   `json:"aliases"`
	Group         string                     `json:"group"`
	Skip          bool                       `json:"skip"`
	Hidden        bool                       `json:"hidden"`
	Tools         []CLITool                  `json:"tools"`
	Groups        map[string]CLIGroupDef     `json:"groups,omitempty"`
	ToolOverrides map[string]CLIToolOverride `json:"toolOverrides,omitempty"`
	// ServerDeps declares other product IDs that this overlay depends on at
	// runtime (e.g. chat depends on bot for cross-server tool routing).
	// Consumed by the portal merge service for fail-fast validation; CLI side
	// currently only stores the value for tooling/introspection.
	ServerDeps []string `json:"serverDeps,omitempty"`
	// Hints registers stub sub-commands under the overlay root that only
	// print a redirect message pointing to the canonical command path. Used
	// for deprecated command aliases and "did-you-mean" style hints.
	// Key is the sub-command name; value describes the target path.
	Hints map[string]CLIHintDef `json:"hintCommands,omitempty"`
	// RedirectTo, when non-empty, turns the entire top-level product into a
	// stub that prints "Please use: dws <target>" and performs no work. Used
	// for deprecated top-level products migrated to new paths (e.g.
	// `bot → chat bot`, `message → chat message`). See schema v3 §2.6.
	RedirectTo string `json:"redirectTo,omitempty"`
}

// CLIHintDef declares a stub sub-command that prints a redirect message.
// The command takes no bindings and calls no tool; its sole purpose is to
// help users migrate from an old command path to the new one.
type CLIHintDef struct {
	// Target is the canonical command path shown in the redirect message
	// (e.g. "dws chat message list").
	Target string `json:"target"`
	// Description overrides the Short/Long help text for the hint command.
	// Empty falls back to a generic "use: <target>" string.
	Description string `json:"description,omitempty"`
	// Group optionally nests the hint under a named sub-group (same syntax
	// as CLIToolOverride.Group with dot-separated paths). Empty means the
	// hint is attached directly to the overlay root.
	Group string `json:"group,omitempty"`
}

// CLIGroupDef defines a sub-command group within a CLI module.
type CLIGroupDef struct {
	Description string `json:"description"`
}

// CLIOutputFormat declares structured post-processing applied to the MCP tool
// response before the formatter prints it. See schema v3 §2.5.
//
// Apply order: Drop → Rename → Columns. All three are optional.
type CLIOutputFormat struct {
	// Rename moves fields from src key to dst key at top level and one level
	// of nested objects. Missing src keys are silently ignored.
	Rename map[string]string `json:"rename,omitempty"`
	// Drop removes these keys from the response (top level + one level deep).
	Drop []string `json:"drop,omitempty"`
	// Columns controls column order and subset for --format=table. Ignored in
	// JSON output mode.
	Columns []string `json:"columns,omitempty"`
}

// CLIToolOverride maps an MCP tool to a CLI command with flag aliases and transforms.
type CLIToolOverride struct {
	CLIName string `json:"cliName"`
	// CLIAliases registers additional cobra command aliases for the same MCP
	// tool, so the leaf command can be invoked under multiple names without
	// duplicating the override. Mirrors cobra.Command.Aliases. Each alias is
	// added to the cobra Aliases slice; conflicts with existing siblings are
	// silently ignored by cobra. Use for command-name normalisation (e.g.
	// `range read` accepts `range get` as an alias) or hardcoded-command
	// migration paths. Empty / nil means no extra aliases.
	CLIAliases  []string `json:"cliAliases,omitempty"`
	Description string   `json:"description,omitempty"`
	// Example, when non-empty, is wired to cobra.Command.Example to render
	// the "Examples:" section in --help. Mirrors hardcoded helper commands'
	// Example field (e.g. wukong/products/oa.go list-forms). Empty value
	// produces no Examples section. Multi-line strings keep "\n" literally.
	Example     string                     `json:"example,omitempty"`
	Group       string                     `json:"group,omitempty"`
	IsSensitive bool                       `json:"isSensitive,omitempty"`
	Hidden      bool                       `json:"hidden,omitempty"`
	Flags       map[string]CLIFlagOverride `json:"flags,omitempty"`
	// OutputFormat declares structured response post-processing. v3 typed form
	// supersedes v2's untyped map[string]any, but parsing stays lenient so v2
	// envelopes continue to deserialize (unknown keys are ignored).
	OutputFormat CLIOutputFormat `json:"outputFormat,omitempty"`
	// ServerOverride routes this leaf command's tool invocation to a different
	// product's MCP server (e.g. `chat bot ...` leaves live under the `chat`
	// command tree but call the `bot` endpoint). Empty means use the enclosing
	// overlay's server.
	ServerOverride string `json:"serverOverride,omitempty"`
	// BodyWrapper, when non-empty, wraps the collected params map under this
	// single key before the invocation is dispatched. Useful when the upstream
	// tool expects a typed DTO wrapper (e.g. `PersonalTodoCreateVO`). Only
	// user-provided params are wrapped; internal control keys starting with
	// '_' (e.g. `_blocked`, `_yes`) are preserved at the top level.
	BodyWrapper string `json:"bodyWrapper,omitempty"`
	// MutuallyExclusive groups flag aliases that must not be set together.
	// Each inner slice becomes one cobra.MarkFlagsMutuallyExclusive call.
	// Example: [["group","user","open-dingtalk-id"]] for `chat message list`.
	MutuallyExclusive [][]string `json:"mutuallyExclusive,omitempty"`
	// RequireOneOf groups flag aliases where at least one must be set. Each
	// inner slice becomes one cobra.MarkFlagsOneRequired call. Typically
	// paired with MutuallyExclusive to enforce "exactly one of".
	RequireOneOf [][]string `json:"requireOneOf,omitempty"`
	// RedirectTo, when non-empty, turns this entry into a stub command that
	// prints the redirect target instead of invoking a tool. All other
	// fields (Flags / BodyWrapper / IsSensitive / ServerOverride) are
	// ignored. Use for deprecated leaf commands that moved to a new path.
	RedirectTo string `json:"redirectTo,omitempty"`
}

// CLIFlagOverride describes how to map an MCP parameter to a CLI flag.
type CLIFlagOverride struct {
	Alias string `json:"alias"`
	// Aliases registers additional hidden CLI flag names for the same MCP
	// parameter. Use to preserve legacy flag names when migrating from
	// hardcoded commands (mirrors cmdutil.ValidateRequiredFlagWithAliases /
	// cmdutil.FlagOrFallback). All entries are registered as hidden flags
	// (not shown in --help); values are deduped against the primary flag
	// name and Alias, and reserved names ("json", "params") are skipped.
	// When any alias is set by the user, the binding's Required check is
	// satisfied and the value is written to params[Property].
	Aliases       []string       `json:"aliases,omitempty"`
	Transform     string         `json:"transform,omitempty"`
	TransformArgs map[string]any `json:"transformArgs,omitempty"`
	EnvDefault    string         `json:"envDefault,omitempty"`
	Hidden        bool           `json:"hidden,omitempty"`
	Default       string         `json:"default,omitempty"`
	// Shorthand registers a single-char flag alias (cobra StringP etc.).
	Shorthand string `json:"shorthand,omitempty"`
	// Required marks this flag as mandatory via cobra.MarkFlagRequired.
	// Ignored when Positional is true (positional args have their own arity rules).
	Required bool `json:"required,omitempty"`
	// Description overrides the usage string displayed in --help; takes
	// priority over the Detail API's toolDesc when non-empty.
	Description string `json:"description,omitempty"`
	// Positional, when true, binds this parameter to a positional CLI argument
	// instead of a --flag. PositionalIndex (0-based) selects which arg slot.
	Positional      bool `json:"positional,omitempty"`
	PositionalIndex int  `json:"positionalIndex,omitempty"`
	// Type explicitly declares the flag's type: "string" (default) / "int" /
	// "bool" / "stringSlice". When set, overrides the type inferred from MCP
	// tools/list inputSchema. See schema v3 §2.1.
	Type string `json:"type,omitempty"`
	// OmitWhen declares empty-value handling when building the invocation body:
	//
	//   "empty" (default): empty string / zero-length slice-or-map → omit
	//   "zero":            + zero numbers / false booleans → omit
	//   "never":           always send, even at zero value (explicit-zero semantics)
	//
	// See schema v3 §2.2.
	OmitWhen string `json:"omitWhen,omitempty"`
	// RuntimeDefault, when non-empty, injects a runtime-resolved value if the
	// user omits the flag. Allowed placeholders: "$currentUserId" / "$unionId"
	// / "$corpId" / "$now" / "$today". Unknown placeholders → warning + skip.
	// Resolution comes from edition.Hooks.RuntimeDefaults; open-source core
	// only recognises the placeholder set. See schema v3 §2.3.
	RuntimeDefault string `json:"runtimeDefault,omitempty"`
}

type CLITool struct {
	Name        string                 `json:"name"`
	CLIName     string                 `json:"cliName"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	IsSensitive bool                   `json:"isSensitive"`
	Category    string                 `json:"category"`
	Hidden      bool                   `json:"hidden"`
	Flags       map[string]CLIFlagHint `json:"flags"`
}

type CLIFlagHint struct {
	Shorthand string `json:"shorthand"`
	Alias     string `json:"alias"`
}

type RegistryServer struct {
	SchemaURI   string           `json:"$schema"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Remotes     []RegistryRemote `json:"remotes"`
}

type RegistryRemote struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

type DetailResponse struct {
	Result  DetailResult `json:"result"`
	Success bool         `json:"success"`
}

type DetailResult struct {
	MCPID       int          `json:"mcpId"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Tools       []DetailTool `json:"tools"`
}

type DetailTool struct {
	ToolName      string `json:"toolName"`
	ToolTitle     string `json:"toolTitle"`
	ToolDesc      string `json:"toolDesc"`
	IsSensitive   bool   `json:"isSensitive"`
	ToolRequest   string `json:"toolRequest"`
	ToolResponse  string `json:"toolResponse"`
	ActionVersion string `json:"actionVersion"`
}

type DetailLocator struct {
	MCPID     int    `json:"mcp_id,omitempty"`
	DetailURL string `json:"detail_url,omitempty"`
}

type ServerDescriptor struct {
	Key                       string            `json:"key"`
	SourceServerID            string            `json:"source_server_id,omitempty"`
	DisplayName               string            `json:"display_name"`
	Description               string            `json:"description,omitempty"`
	Endpoint                  string            `json:"endpoint"`
	SchemaURI                 string            `json:"schema_uri,omitempty"`
	NegotiatedProtocolVersion string            `json:"negotiated_protocol_version,omitempty"`
	UpdatedAt                 time.Time         `json:"updated_at,omitempty"`
	PublishedAt               time.Time         `json:"published_at,omitempty"`
	Status                    string            `json:"status,omitempty"`
	Source                    string            `json:"source"`
	Degraded                  bool              `json:"degraded"`
	DetailLocator             DetailLocator     `json:"detail_locator,omitempty"`
	Lifecycle                 LifecycleInfo     `json:"lifecycle,omitempty"`
	CLI                       CLIOverlay        `json:"cli,omitempty"`
	HasCLIMeta                bool              `json:"has_cli_meta,omitempty"`
	AuthHeaders               map[string]string `json:"auth_headers,omitempty"` // plugin-level auth headers for third-party MCP servers
}

func NewClient(baseURL string, httpClient *http.Client) *Client {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultBaseURL
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: config.HTTPTimeout}
	}
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		HTTPClient: httpClient,
	}
}

func (c *Client) FetchServers(ctx context.Context, limit int) (ListResponse, error) {
	if limit <= 0 {
		limit = 200
	}

	allServers := make([]ServerEnvelope, 0)
	cursor := ""
	seenCursors := map[string]struct{}{}

	for {
		payload, err := c.fetchServersPage(ctx, limit, cursor)
		if err != nil {
			return ListResponse{}, err
		}

		allServers = append(allServers, payload.Servers...)
		nextCursor := strings.TrimSpace(payload.Metadata.NextCursor)
		if nextCursor == "" {
			payload.Metadata.Count = len(allServers)
			payload.Metadata.NextCursor = ""
			payload.Servers = allServers
			return payload, nil
		}
		if _, exists := seenCursors[nextCursor]; exists {
			return ListResponse{}, apperrors.NewDiscovery("market servers pagination cursor repeated")
		}
		seenCursors[nextCursor] = struct{}{}
		cursor = nextCursor
	}
}

// FetchServersFromURL fetches the server list from a full URL (no path appending).
// This is used when DWS_SERVERS_URL is set to a complete endpoint.
func (c *Client) FetchServersFromURL(ctx context.Context, fullURL string) (ListResponse, error) {
	fullURL = strings.TrimSpace(fullURL)
	if fullURL == "" {
		return ListResponse{}, apperrors.NewDiscovery("servers URL is empty")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return ListResponse{}, apperrors.NewDiscovery("failed to create servers request")
	}
	for k, v := range c.Headers {
		req.Header.Set(k, v)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return ListResponse{}, apperrors.NewDiscovery(fmt.Sprintf("servers request failed: %v", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return ListResponse{}, apperrors.NewDiscovery(fmt.Sprintf("servers request returned HTTP %d", resp.StatusCode))
	}

	var payload ListResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return ListResponse{}, apperrors.NewDiscovery("failed to decode servers response")
	}
	return payload, nil
}

func (c *Client) fetchServersPage(ctx context.Context, limit int, cursor string) (ListResponse, error) {
	reqURL, err := url.Parse(c.BaseURL + "/cli/discovery/apis")
	if err != nil {
		return ListResponse{}, apperrors.NewDiscovery("failed to build market servers URL")
	}
	query := reqURL.Query()
	query.Set("limit", fmt.Sprintf("%d", limit))
	if strings.TrimSpace(cursor) != "" {
		query.Set("cursor", strings.TrimSpace(cursor))
	}
	reqURL.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return ListResponse{}, apperrors.NewDiscovery("failed to create market servers request")
	}
	for k, v := range c.Headers {
		req.Header.Set(k, v)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return ListResponse{}, apperrors.NewDiscovery(fmt.Sprintf("market servers request failed: %v", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return ListResponse{}, apperrors.NewDiscovery(fmt.Sprintf("market servers request returned HTTP %d", resp.StatusCode))
	}

	var payload ListResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return ListResponse{}, apperrors.NewDiscovery("failed to decode market servers response")
	}
	return payload, nil
}

func (c *Client) FetchDetail(ctx context.Context, mcpID int) (DetailResponse, error) {
	reqURL, err := url.Parse(c.BaseURL + "/mcp/market/detail")
	if err != nil {
		return DetailResponse{}, apperrors.NewDiscovery("failed to build market detail URL")
	}
	query := reqURL.Query()
	query.Set("mcpId", fmt.Sprintf("%d", mcpID))
	reqURL.RawQuery = query.Encode()
	return c.fetchDetailHTTP(ctx, reqURL.String())
}

func (c *Client) FetchDetailByURL(ctx context.Context, detailURL string) (DetailResponse, error) {
	detailURL = strings.TrimSpace(detailURL)
	if detailURL == "" {
		return DetailResponse{}, apperrors.NewDiscovery("market detail URL is empty")
	}

	parsed, err := url.Parse(detailURL)
	if err != nil {
		return DetailResponse{}, apperrors.NewDiscovery("failed to parse market detail URL")
	}
	if !parsed.IsAbs() {
		base, baseErr := url.Parse(c.BaseURL)
		if baseErr != nil {
			return DetailResponse{}, apperrors.NewDiscovery("failed to parse market base URL")
		}
		parsed = base.ResolveReference(parsed)
	}

	// Guard against SSRF: require HTTPS and reject private network addresses.
	if !strings.EqualFold(parsed.Scheme, "https") {
		return DetailResponse{}, apperrors.NewDiscovery("market detail URL must use HTTPS")
	}
	if isPrivateHost(parsed.Hostname()) {
		return DetailResponse{}, apperrors.NewDiscovery("market detail URL must not target private network addresses")
	}

	return c.fetchDetailHTTP(ctx, parsed.String())
}

func (c *Client) fetchDetailHTTP(ctx context.Context, targetURL string) (DetailResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return DetailResponse{}, apperrors.NewDiscovery("failed to create market detail request")
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return DetailResponse{}, apperrors.NewDiscovery(fmt.Sprintf("market detail request failed: %v", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return DetailResponse{}, apperrors.NewDiscovery(fmt.Sprintf("market detail request returned HTTP %d", resp.StatusCode))
	}

	var payload DetailResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return DetailResponse{}, apperrors.NewDiscovery("failed to decode market detail response")
	}
	if !payload.Success {
		return DetailResponse{}, apperrors.NewDiscovery("market detail response reported success=false")
	}
	return payload, nil
}

// isPrivateHost checks whether a hostname resolves to a private/loopback address.
func isPrivateHost(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	host = strings.TrimSuffix(host, ".")
	if host == "" {
		return false
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
}

func NormalizeServers(response ListResponse, source string) []ServerDescriptor {
	bestByEndpoint := make(map[string]ServerDescriptor)

	for _, envelope := range response.Servers {
		meta := envelope.Meta.Registry
		if meta.Status != "" && !strings.EqualFold(meta.Status, "active") {
			continue
		}

		remoteURL, ok := selectRemoteURL(envelope.Server.Remotes)
		if !ok {
			continue
		}

		endpoint := NormalizeEndpoint(remoteURL)
		descriptor := ServerDescriptor{
			Key:         ServerKey(endpoint),
			DisplayName: strings.TrimSpace(envelope.Server.Name),
			Description: strings.TrimSpace(envelope.Server.Description),
			Endpoint:    endpoint,
			SchemaURI:   strings.TrimSpace(envelope.Server.SchemaURI),
			Status:      strings.TrimSpace(meta.Status),
			Source:      source,
			DetailLocator: DetailLocator{
				MCPID:     meta.MCPID,
				DetailURL: strings.TrimSpace(meta.DetailURL),
			},
			Lifecycle:  markDeprecatedCandidate(strings.TrimSpace(envelope.Server.Name), meta.Lifecycle),
			CLI:        envelope.Meta.CLI,
			HasCLIMeta: strings.TrimSpace(envelope.Meta.CLI.ID) != "" || strings.TrimSpace(envelope.Meta.CLI.Command) != "" || len(envelope.Meta.CLI.Tools) > 0 || len(envelope.Meta.CLI.ToolOverrides) > 0,
		}

		if publishedAt, ok := parseTime(meta.PublishedAt); ok {
			descriptor.PublishedAt = publishedAt
		}
		if updatedAt, ok := parseTime(meta.UpdatedAt); ok {
			descriptor.UpdatedAt = updatedAt
		}

		existing, exists := bestByEndpoint[descriptor.Key]
		if !exists || descriptorIsNewer(descriptor, existing) {
			bestByEndpoint[descriptor.Key] = descriptor
		}
	}

	bestByName := make(map[string]ServerDescriptor)
	for _, descriptor := range bestByEndpoint {
		nameKey := normalizeDisplayNameKey(descriptor.DisplayName)
		if nameKey == "" {
			nameKey = descriptor.Key
		}
		existing, exists := bestByName[nameKey]
		if !exists || descriptorIsNewer(descriptor, existing) {
			bestByName[nameKey] = descriptor
		}
	}

	out := make([]ServerDescriptor, 0, len(bestByName))
	for _, descriptor := range bestByName {
		out = append(out, descriptor)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].DisplayName != out[j].DisplayName {
			return out[i].DisplayName < out[j].DisplayName
		}
		if out[i].Endpoint != out[j].Endpoint {
			return out[i].Endpoint < out[j].Endpoint
		}
		return out[i].Key < out[j].Key
	})
	return out
}

func descriptorIsNewer(current, existing ServerDescriptor) bool {
	if current.UpdatedAt.After(existing.UpdatedAt) {
		return true
	}
	if existing.UpdatedAt.After(current.UpdatedAt) {
		return false
	}
	if current.PublishedAt.After(existing.PublishedAt) {
		return true
	}
	if existing.PublishedAt.After(current.PublishedAt) {
		return false
	}
	if current.Endpoint != existing.Endpoint {
		return current.Endpoint < existing.Endpoint
	}
	return current.Key < existing.Key
}

func normalizeDisplayNameKey(displayName string) string {
	return strings.ToLower(strings.TrimSpace(displayName))
}

func markDeprecatedCandidate(displayName string, lifecycle LifecycleInfo) LifecycleInfo {
	if lifecycle.DeprecatedCandidate {
		return lifecycle
	}
	if lifecycle.DeprecatedBy > 0 || strings.TrimSpace(lifecycle.DeprecationDate) != "" || strings.TrimSpace(lifecycle.MigrationURL) != "" {
		return lifecycle
	}
	if !hasDeprecatedMarker(displayName) {
		return lifecycle
	}
	lifecycle.DeprecatedCandidate = true
	return lifecycle
}

func hasDeprecatedMarker(displayName string) bool {
	name := strings.TrimSpace(displayName)
	if name == "" {
		return false
	}
	lower := strings.ToLower(name)
	// Check both Chinese and English patterns regardless of current locale
	return strings.Contains(name, "（旧）") || strings.Contains(name, "旧版") ||
		strings.Contains(lower, "(old)") || strings.Contains(lower, "(legacy)")
}

func NormalizeEndpoint(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return strings.TrimSpace(raw)
	}
	parsed.Fragment = ""
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	values := parsed.Query()
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	normalized := make(url.Values, len(values))
	for _, key := range keys {
		vals := append([]string(nil), values[key]...)
		sort.Strings(vals)
		for _, value := range vals {
			normalized.Add(key, value)
		}
	}
	parsed.RawQuery = normalized.Encode()
	return parsed.String()
}

func ServerKey(endpoint string) string {
	sum := sha256.Sum256([]byte(endpoint))
	return hex.EncodeToString(sum[:8])
}

func selectRemoteURL(remotes []RegistryRemote) (string, bool) {
	for _, remote := range remotes {
		if strings.EqualFold(strings.TrimSpace(remote.Type), "streamable-http") && strings.TrimSpace(remote.URL) != "" {
			return remote.URL, true
		}
	}
	return "", false
}

func parseTime(value string) (time.Time, bool) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}
