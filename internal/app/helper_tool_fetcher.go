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
	"context"
	"fmt"
	"sync"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

// newHelperToolFetcher returns a cli.HelperToolFetcher that loads a helper MCP
// server's tools/list LIVE (by source) and projects each tool into a
// cli.HelperToolSchema (name, description, inputSchema properties/required). It
// is injected into the schema command so the cli package can render
// `dws schema dev.*` from real server schema without importing app/transport.
//
// Sources: "op-app" backs the dev app commands (pinned endpoint); "devdoc"
// backs `dws dev doc search` (endpoint resolved dynamically, see
// helperSourceEndpoint). Results are memoized per source per process so
// repeated `dws schema dev.*` hit the network at most once per source. A failed
// fetch is not cached, allowing a later retry within the same process.
func newHelperToolFetcher() cli.HelperToolFetcher {
	var (
		mu     sync.Mutex
		cached = map[string]map[string]cli.HelperToolSchema{}
	)
	return func(ctx context.Context, source string) (map[string]cli.HelperToolSchema, error) {
		mu.Lock()
		if got, ok := cached[source]; ok {
			mu.Unlock()
			return got, nil
		}
		mu.Unlock()

		endpoint, err := helperSourceEndpoint(source)
		if err != nil {
			return nil, err
		}
		schemas, err := fetchHelperToolSchemas(ctx, endpoint)
		if err != nil {
			return nil, err
		}
		mu.Lock()
		cached[source] = schemas
		mu.Unlock()
		return schemas, nil
	}
}

// helperSourceEndpoint maps a schema source to its MCP endpoint. op-app (dev
// app) is pinned in source (devappMCPEndpoint, derived from the active gateway
// base — production by default, pre when ~/.dws/mcp_url points at pre); other
// sources (e.g. devdoc) are resolved the same way the runner resolves a product
// endpoint — env override → discovery → edition StaticServers/SupplementServers.
func helperSourceEndpoint(source string) (string, error) {
	switch source {
	case "", "op-app", "devapp":
		return devappMCPEndpoint(), nil
	default:
		if endpoint, ok := directRuntimeEndpoint(source, ""); ok {
			return endpoint, nil
		}
		return "", fmt.Errorf("no MCP endpoint resolved for source %q (not injected by edition/discovery)", source)
	}
}

// fetchHelperToolSchemas performs the live tools/list call against endpoint and
// converts the descriptors. Auth and identity headers are resolved the same way
// the runner does for direct-runtime invocations.
func fetchHelperToolSchemas(ctx context.Context, endpoint string) (map[string]cli.HelperToolSchema, error) {
	token := resolveRuntimeAuthToken(ctx, "")
	headers := resolveIdentityHeaders()
	client := transport.NewClient(nil).WithAuth(token, headers)

	result, err := client.ListTools(ctx, endpoint)
	if err != nil {
		return nil, err
	}

	out := make(map[string]cli.HelperToolSchema, len(result.Tools))
	for _, td := range result.Tools {
		out[td.Name] = cli.HelperToolSchema{
			Name:        td.Name,
			Description: td.Description,
			Properties:  inputSchemaProperties(td.InputSchema),
			Required:    inputSchemaRequired(td.InputSchema),
		}
	}
	return out, nil
}

// inputSchemaProperties pulls the "properties" object out of a deserialized
// MCP inputSchema map. Returns an empty (non-nil) map when absent.
func inputSchemaProperties(schema map[string]any) map[string]any {
	if schema == nil {
		return map[string]any{}
	}
	props, _ := schema["properties"].(map[string]any)
	if props == nil {
		return map[string]any{}
	}
	return props
}

// inputSchemaRequired pulls the "required" string list out of a deserialized
// MCP inputSchema map.
func inputSchemaRequired(schema map[string]any) []string {
	if schema == nil {
		return nil
	}
	raw, ok := schema["required"].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}
