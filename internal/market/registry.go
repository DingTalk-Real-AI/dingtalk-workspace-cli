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
}

type ListResponse struct {
	Metadata ListMetadata     `json:"metadata"`
	Servers  []ServerEnvelope `json:"servers"`
}

type ListMetadata struct {
	Count      int    `json:"count"`
	NextCursor string `json:"nextCursor"`
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
}

// CLIGroupDef defines a sub-command group within a CLI module.
type CLIGroupDef struct {
	Description string `json:"description"`
}

// CLIToolOverride maps an MCP tool to a CLI command with flag aliases and transforms.
type CLIToolOverride struct {
	CLIName      string                     `json:"cliName"`
	Description  string                     `json:"description,omitempty"`
	Group        string                     `json:"group,omitempty"`
	IsSensitive  bool                       `json:"isSensitive,omitempty"`
	Hidden       bool                       `json:"hidden,omitempty"`
	Flags        map[string]CLIFlagOverride `json:"flags,omitempty"`
	OutputFormat map[string]any             `json:"outputFormat,omitempty"`
}

// CLIFlagOverride describes how to map an MCP parameter to a CLI flag.
type CLIFlagOverride struct {
	Alias         string         `json:"alias"`
	Transform     string         `json:"transform,omitempty"`
	TransformArgs map[string]any `json:"transformArgs,omitempty"`
	EnvDefault    string         `json:"envDefault,omitempty"`
	Hidden        bool           `json:"hidden,omitempty"`
	Default       string         `json:"default,omitempty"`
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
