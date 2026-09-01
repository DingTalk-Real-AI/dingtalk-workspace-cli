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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
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
	return c.transport.ListTools(ctx, endpoint)
}

func (c *Client) Invoke(ctx context.Context, endpoint, tool string, arguments map[string]any) (transport.ToolCallResult, error) {
	return c.transport.CallTool(ctx, endpoint, tool, arguments)
}
