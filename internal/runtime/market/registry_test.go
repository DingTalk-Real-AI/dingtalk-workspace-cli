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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func boolPtr(value bool) *bool {
	return &value
}

func stringPtr(value string) *string {
	return &value
}

func TestNormalizeServersDeduplicatesByNewestEndpoint(t *testing.T) {
	t.Parallel()

	response := ListResponse{
		Servers: []ServerEnvelope{
			{
				Server: RegistryServer{
					Name: "钉钉文档",
					Remotes: []RegistryRemote{
						{Type: "streamable-http", URL: "https://example.com/server/doc"},
					},
				},
				Meta: EnvelopeMeta{Registry: RegistryMetadata{Status: "active", UpdatedAt: "2026-03-16T00:00:00Z"}},
			},
			{
				Server: RegistryServer{
					Name: "钉钉文档",
					Remotes: []RegistryRemote{
						{Type: "streamable-http", URL: "https://example.com/server/doc"},
					},
				},
				Meta: EnvelopeMeta{Registry: RegistryMetadata{Status: "active", UpdatedAt: "2026-03-18T00:00:00Z"}},
			},
			{
				Server: RegistryServer{
					Name: "停用服务",
					Remotes: []RegistryRemote{
						{Type: "streamable-http", URL: "https://example.com/server/inactive"},
					},
				},
				Meta: EnvelopeMeta{Registry: RegistryMetadata{Status: "inactive", UpdatedAt: "2026-03-18T00:00:00Z"}},
			},
		},
	}

	servers := NormalizeServers(response, "live_market")
	if len(servers) != 1 {
		t.Fatalf("NormalizeServers() len = %d, want 1", len(servers))
	}
	if !servers[0].UpdatedAt.Equal(mustParseRFC3339(t, "2026-03-18T00:00:00Z")) {
		t.Fatalf("NormalizeServers() picked updatedAt %s, want newest record", servers[0].UpdatedAt)
	}
}

func TestNormalizeServersDeduplicatesSameNameAcrossEndpoints(t *testing.T) {
	t.Parallel()

	response := ListResponse{
		Servers: []ServerEnvelope{
			{
				Server: RegistryServer{
					Name: "钉钉文档",
					Remotes: []RegistryRemote{
						{Type: "streamable-http", URL: "https://example.com/server/doc-a"},
					},
				},
				Meta: EnvelopeMeta{Registry: RegistryMetadata{Status: "active", UpdatedAt: "2026-03-18T00:00:00Z"}},
			},
			{
				Server: RegistryServer{
					Name: "钉钉文档",
					Remotes: []RegistryRemote{
						{Type: "streamable-http", URL: "https://example.com/server/doc-b"},
					},
				},
				Meta: EnvelopeMeta{Registry: RegistryMetadata{Status: "active", UpdatedAt: "2026-03-19T00:00:00Z"}},
			},
		},
	}

	servers := NormalizeServers(response, "live_market")
	if len(servers) != 1 {
		t.Fatalf("NormalizeServers() len = %d, want 1 newest-by-name record", len(servers))
	}
	if servers[0].Endpoint != "https://example.com/server/doc-b" {
		t.Fatalf("NormalizeServers() endpoint = %q, want newest endpoint doc-b", servers[0].Endpoint)
	}
}

func TestNormalizeServersMarksLegacyNameAsDeprecatedCandidate(t *testing.T) {
	t.Parallel()

	response := ListResponse{
		Servers: []ServerEnvelope{
			{
				Server: RegistryServer{
					Name: "钉钉文档（旧）",
					Remotes: []RegistryRemote{
						{Type: "streamable-http", URL: "https://example.com/server/doc-legacy"},
					},
				},
				Meta: EnvelopeMeta{
					Registry: RegistryMetadata{
						Status: "active",
					},
				},
			},
		},
	}

	servers := NormalizeServers(response, "live_market")
	if len(servers) != 1 {
		t.Fatalf("NormalizeServers() len = %d, want 1", len(servers))
	}
	if !servers[0].Lifecycle.DeprecatedCandidate {
		t.Fatalf("NormalizeServers() deprecatedCandidate = false, want true")
	}
}

func TestFetchServers(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cli/discovery/apis" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		payload := ListResponse{
			Metadata: ListMetadata{Count: 1},
			Servers: []ServerEnvelope{
				{
					Server: RegistryServer{
						Name: "文档",
						Remotes: []RegistryRemote{
							{Type: "streamable-http", URL: "https://example.com/server/doc"},
						},
					},
					Meta: EnvelopeMeta{Registry: RegistryMetadata{Status: "active"}},
				},
			},
		}
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			t.Fatalf("encode payload: %v", err)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	response, err := client.FetchServers(context.Background(), 200)
	if err != nil {
		t.Fatalf("FetchServers() error = %v", err)
	}
	if response.Metadata.Count != 1 {
		t.Fatalf("FetchServers() count = %d, want 1", response.Metadata.Count)
	}
}

func TestFetchServersFollowsNextCursor(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cli/discovery/apis" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}

		cursor := r.URL.Query().Get("cursor")
		switch cursor {
		case "":
			payload := ListResponse{
				Metadata: ListMetadata{Count: 1, NextCursor: "page-2"},
				Servers: []ServerEnvelope{
					{
						Server: RegistryServer{
							Name: "文档一",
							Remotes: []RegistryRemote{
								{Type: "streamable-http", URL: "https://example.com/server/doc-1"},
							},
						},
						Meta: EnvelopeMeta{Registry: RegistryMetadata{Status: "active"}},
					},
				},
			}
			if err := json.NewEncoder(w).Encode(payload); err != nil {
				t.Fatalf("encode first payload: %v", err)
			}
		case "page-2":
			payload := ListResponse{
				Metadata: ListMetadata{Count: 1},
				Servers: []ServerEnvelope{
					{
						Server: RegistryServer{
							Name: "文档二",
							Remotes: []RegistryRemote{
								{Type: "streamable-http", URL: "https://example.com/server/doc-2"},
							},
						},
						Meta: EnvelopeMeta{Registry: RegistryMetadata{Status: "active"}},
					},
				},
			}
			if err := json.NewEncoder(w).Encode(payload); err != nil {
				t.Fatalf("encode second payload: %v", err)
			}
		default:
			t.Fatalf("unexpected cursor %q", cursor)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	response, err := client.FetchServers(context.Background(), 200)
	if err != nil {
		t.Fatalf("FetchServers() error = %v", err)
	}
	if response.Metadata.Count != 2 {
		t.Fatalf("FetchServers() count = %d, want 2", response.Metadata.Count)
	}
	if len(response.Servers) != 2 {
		t.Fatalf("FetchServers() len = %d, want 2", len(response.Servers))
	}
	if response.Metadata.NextCursor != "" {
		t.Fatalf("FetchServers() nextCursor = %q, want empty after aggregation", response.Metadata.NextCursor)
	}
}

func TestFetchDetailByURLRejectsUnsafeTargets(t *testing.T) {
	t.Parallel()

	client := NewClient("https://mcp.dingtalk.com", nil)

	// Private IP — should be rejected
	_, err := client.FetchDetailByURL(context.Background(), "https://127.0.0.1/detail")
	if err == nil {
		t.Fatal("FetchDetailByURL(loopback) should fail with SSRF guard")
	}

	// localhost hostname — should be rejected
	_, err = client.FetchDetailByURL(context.Background(), "https://localhost/detail")
	if err == nil {
		t.Fatal("FetchDetailByURL(localhost) should fail with SSRF guard")
	}

	// Non-HTTPS — should be rejected
	_, err = client.FetchDetailByURL(context.Background(), "http://mcp.dingtalk.com/detail")
	if err == nil {
		t.Fatal("FetchDetailByURL(http) should fail with HTTPS guard")
	}

	// Relative URL resolved against HTTPS BaseURL passes scheme check but
	// the detail endpoint in the test environment is unreachable. We only
	// verify it does NOT return an SSRF error.
	_, err = client.FetchDetailByURL(context.Background(), "/mcp/market/detail?mcpId=1")
	if err != nil && err.Error() == "market detail URL must not target private network addresses" {
		t.Fatal("FetchDetailByURL(relative) should not fail with SSRF guard")
	}
}

func TestNormalizeServersPreservesCLIMetadataAndLifecycle(t *testing.T) {
	t.Parallel()

	response := ListResponse{
		Servers: []ServerEnvelope{
			{
				Server: RegistryServer{
					Name: "钉钉文档",
					Remotes: []RegistryRemote{
						{Type: "streamable-http", URL: "https://example.com/server/doc"},
					},
				},
				Meta: EnvelopeMeta{
					Registry: RegistryMetadata{
						Status:    "active",
						MCPID:     9629,
						DetailURL: "https://example.com/detail?mcpId=9629",
						Lifecycle: LifecycleInfo{
							DeprecatedBy:    0,
							DeprecationDate: "",
							MigrationURL:    "",
						},
					},
					CLI: CLIOverlay{
						ID:      "doc",
						Command: "doc",
						Aliases: []string{"documents"},
						Tools: []CLITool{
							{
								Name:        "create_document",
								CLIName:     "create-document",
								Title:       "创建文档",
								IsSensitive: boolPtr(true),
								Flags: map[string]CLIFlagHint{
									"title": {Alias: "name", Shorthand: "t"},
								},
							},
						},
						ToolOverrides: map[string]CLIToolOverride{
							"create_document": {
								Flags: map[string]CLIFlagOverride{
									"title": {
										Transform:     "trim_space",
										TransformArgs: map[string]any{"side": "both"},
										Default:       stringPtr("Untitled"),
										Hidden:        boolPtr(true),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	servers := NormalizeServers(response, "live_market")
	if len(servers) != 1 {
		t.Fatalf("NormalizeServers() len = %d, want 1", len(servers))
	}
	if !servers[0].HasCLIMeta {
		t.Fatalf("NormalizeServers() expected HasCLIMeta")
	}
	if servers[0].CLI.ID != "doc" || servers[0].CLI.Command != "doc" {
		t.Fatalf("NormalizeServers() cli = %#v", servers[0].CLI)
	}
	if servers[0].CLI.Aliases[0] != "documents" {
		t.Fatalf("NormalizeServers() cli aliases = %#v, want documents", servers[0].CLI.Aliases)
	}
	if hint := servers[0].CLI.Tools[0].Flags["title"]; hint.Alias != "name" || hint.Shorthand != "t" {
		t.Fatalf("NormalizeServers() cli tools flags = %#v, want alias=name shorthand=t", hint)
	}
	if got := servers[0].CLI.ToolOverrides["create_document"].Flags["title"].Transform; got != "trim_space" {
		t.Fatalf("NormalizeServers() transform = %q, want trim_space", got)
	}
	if got := servers[0].CLI.ToolOverrides["create_document"].Flags["title"].TransformArgs["side"]; got != "both" {
		t.Fatalf("NormalizeServers() transform args = %#v, want side=both", servers[0].CLI.ToolOverrides["create_document"].Flags["title"].TransformArgs)
	}
	if servers[0].DetailLocator.MCPID != 9629 {
		t.Fatalf("NormalizeServers() mcpId = %d, want 9629", servers[0].DetailLocator.MCPID)
	}
}

func TestCLIOverlayJSONPreservesPresenceForOptionalBoolsAndDefaults(t *testing.T) {
	t.Parallel()

	var cli CLIOverlay
	if err := json.Unmarshal([]byte(`{
		"tools": [
			{"name": "explicit", "isSensitive": false, "hidden": false},
			{"name": "absent"}
		],
		"toolOverrides": {
			"explicit": {
				"isSensitive": false,
				"hidden": false,
				"flags": {
					"title": {"default": "", "hidden": false}
				}
			},
			"absent": {
				"flags": {
					"title": {"alias": "name"}
				}
			}
		}
	}`), &cli); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if cli.Tools[0].IsSensitive == nil || *cli.Tools[0].IsSensitive {
		t.Fatalf("tools[0].IsSensitive = %#v, want explicit false", cli.Tools[0].IsSensitive)
	}
	if cli.Tools[0].Hidden == nil || *cli.Tools[0].Hidden {
		t.Fatalf("tools[0].Hidden = %#v, want explicit false", cli.Tools[0].Hidden)
	}
	if cli.Tools[1].IsSensitive != nil {
		t.Fatalf("tools[1].IsSensitive = %#v, want nil when field absent", cli.Tools[1].IsSensitive)
	}
	if cli.Tools[1].Hidden != nil {
		t.Fatalf("tools[1].Hidden = %#v, want nil when field absent", cli.Tools[1].Hidden)
	}

	explicit := cli.ToolOverrides["explicit"]
	if explicit.IsSensitive == nil || *explicit.IsSensitive {
		t.Fatalf("override explicit IsSensitive = %#v, want explicit false", explicit.IsSensitive)
	}
	if explicit.Hidden == nil || *explicit.Hidden {
		t.Fatalf("override explicit Hidden = %#v, want explicit false", explicit.Hidden)
	}
	if explicit.Flags["title"].Default == nil || *explicit.Flags["title"].Default != "" {
		t.Fatalf("override explicit default = %#v, want explicit empty string", explicit.Flags["title"].Default)
	}
	if explicit.Flags["title"].Hidden == nil || *explicit.Flags["title"].Hidden {
		t.Fatalf("override explicit hidden = %#v, want explicit false", explicit.Flags["title"].Hidden)
	}

	absent := cli.ToolOverrides["absent"]
	if absent.IsSensitive != nil {
		t.Fatalf("override absent IsSensitive = %#v, want nil", absent.IsSensitive)
	}
	if absent.Hidden != nil {
		t.Fatalf("override absent Hidden = %#v, want nil", absent.Hidden)
	}
	if absent.Flags["title"].Default != nil {
		t.Fatalf("override absent default = %#v, want nil", absent.Flags["title"].Default)
	}
	if absent.Flags["title"].Hidden != nil {
		t.Fatalf("override absent hidden = %#v, want nil", absent.Flags["title"].Hidden)
	}
}

func mustParseRFC3339(t *testing.T, value string) (parsed time.Time) {
	t.Helper()

	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("time.Parse(%q) error = %v", value, err)
	}
	return parsed
}
