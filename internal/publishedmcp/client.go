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

// Package publishedmcp owns runtime protocol access to published MCP servers.
// Command construction and Schema assembly depend only on its static Client
// methods and never perform discovery during startup.
package publishedmcp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

const (
	maxToolListPages        = 100
	maxToolListPayloadBytes = 20 << 20
)

type Client struct {
	transport *transport.Client
}

func New(base *transport.Client, token string, headers map[string]string) *Client {
	if base == nil {
		base = transport.NewClient(nil)
	}
	return &Client{
		transport: base.WithAuth(token, headers),
	}
}

func (c *Client) Tools(ctx context.Context, endpoint string) (transport.ToolsListResult, error) {
	return c.tools(ctx, endpoint, maxToolListPages, maxToolListPayloadBytes)
}

func (c *Client) tools(ctx context.Context, endpoint string, maxPages, maxPayloadBytes int) (transport.ToolsListResult, error) {
	var aggregate transport.ToolsListResult
	cursor := ""
	seenCursors := map[[sha256.Size]byte]struct{}{}
	aggregateBytes := 0
	for page := 1; page <= maxPages; page++ {
		result, err := c.transport.ListToolsPage(ctx, endpoint, cursor)
		if err != nil {
			return transport.ToolsListResult{}, err
		}
		aggregateBytes, err = appendToolPage(&aggregate, result.Tools, aggregateBytes, maxPayloadBytes)
		if err != nil {
			return transport.ToolsListResult{}, err
		}
		nextCursor := result.NextCursor
		if nextCursor == "" {
			return aggregate, nil
		}
		cursorDigest := sha256.Sum256([]byte(nextCursor))
		if _, exists := seenCursors[cursorDigest]; exists {
			return transport.ToolsListResult{}, apperrors.NewDiscovery(
				fmt.Sprintf("tools/list returned repeated cursor after page %d", page),
			)
		}
		seenCursors[cursorDigest] = struct{}{}
		cursor = nextCursor
	}
	return transport.ToolsListResult{}, apperrors.NewDiscovery(
		fmt.Sprintf("tools/list exceeded the %d-page safety limit", maxPages),
	)
}

func appendToolPage(aggregate *transport.ToolsListResult, tools []transport.ToolDescriptor, currentBytes, maxBytes int) (int, error) {
	payload, err := json.Marshal(tools)
	if err != nil {
		return currentBytes, apperrors.NewDiscovery("tools/list returned tool metadata that could not be measured safely")
	}
	if len(payload) > maxBytes-currentBytes {
		return currentBytes, apperrors.NewDiscovery(
			fmt.Sprintf("tools/list exceeded the %d-byte aggregate safety limit", maxBytes),
		)
	}
	aggregate.Tools = append(aggregate.Tools, tools...)
	return currentBytes + len(payload), nil
}

func (c *Client) Invoke(ctx context.Context, endpoint, tool string, arguments map[string]any) (transport.ToolCallResult, error) {
	return c.transport.CallTool(ctx, endpoint, tool, arguments)
}
